package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Penyimpanan klien OAuth dan izin (grant) connector MCP. Token buram berbentuk
// `<prefix>_<tenant>_<grant>_<64 hex>`: tenant dan id izin ikut di token agar server bisa membuka transaksi bertenant (RLS)
// sebelum membandingkan hash; kerahasiaan ada pada 256 bit acak di ujungnya, dan hanya hash SHA-256 yang disimpan.

const (
	accessPrefix  = "mat"
	refreshPrefix = "mrt"
	accessTTL     = time.Hour
	refreshTTL    = 30 * 24 * time.Hour
)

var (
	errInvalidGrant = errors.New("izin tidak valid atau kedaluwarsa")
	errNoClient     = errors.New("klien tidak dikenal")
)

func newToken(prefix string, tenant, grant uuid.UUID) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "_" + tenant.String() + "_" + grant.String() + "_" + hex.EncodeToString(b), nil
}

func parseToken(prefix, tok string) (tenant, grant uuid.UUID, ok bool) {
	parts := strings.Split(tok, "_")
	if len(parts) != 4 || parts[0] != prefix || len(parts[3]) != 64 {
		return uuid.Nil, uuid.Nil, false
	}
	t, err1 := uuid.Parse(parts[1])
	g, err2 := uuid.Parse(parts[2])
	if err1 != nil || err2 != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return t, g, true
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func hashEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// validRedirect: https, atau http ke loopback (alat pengembangan); tanpa fragmen/userinfo.
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 300 || u.Fragment != "" || u.User != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

func (s *Service) registerClient(ctx context.Context, name string, uris []string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `INSERT INTO oauth_clients (name, redirect_uris) VALUES ($1, $2) RETURNING client_id`, name, uris).Scan(&id)
	return id, err
}

func (s *Service) client(ctx context.Context, id uuid.UUID) (name string, uris []string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT name, redirect_uris FROM oauth_clients WHERE client_id = $1`, id).Scan(&name, &uris)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, errNoClient
	}
	return name, uris, err
}

type tokenPair struct {
	Access, Refresh string
	ExpiresIn       int
}

// createGrant membuat izin baru + pasangan token pertama.
func (s *Service) createGrant(ctx context.Context, tenant, user, outlet, client uuid.UUID, clientName string) (tokenPair, error) {
	id := uuid.New()
	tp, hashes, err := s.mintPair(tenant, id)
	if err != nil {
		return tp, err
	}
	err = db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO mcp_grants (id, tenant_id, user_id, outlet_id, client_id, client_name, access_hash, access_expires_at, refresh_hash, refresh_expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now() + make_interval(secs => $8), $9, now() + make_interval(secs => $10))`,
			id, tenant, user, outlet, client, clientName, hashes[0], accessTTL.Seconds(), hashes[1], refreshTTL.Seconds())
		return err
	})
	return tp, err
}

func (s *Service) mintPair(tenant, grant uuid.UUID) (tokenPair, [2]string, error) {
	var h [2]string
	at, err := newToken(accessPrefix, tenant, grant)
	if err != nil {
		return tokenPair{}, h, err
	}
	rt, err := newToken(refreshPrefix, tenant, grant)
	if err != nil {
		return tokenPair{}, h, err
	}
	h[0], h[1] = hashToken(at), hashToken(rt)
	return tokenPair{Access: at, Refresh: rt, ExpiresIn: int(accessTTL.Seconds())}, h, nil
}

// refreshGrant menukar refresh token dengan pasangan baru (rotasi). Refresh token lama langsung tak berlaku.
func (s *Service) refreshGrant(ctx context.Context, refresh string, client uuid.UUID) (tokenPair, error) {
	tenant, grant, ok := parseToken(refreshPrefix, refresh)
	if !ok {
		return tokenPair{}, errInvalidGrant
	}
	var out tokenPair
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		var hash string
		var gc uuid.UUID
		err := tx.QueryRow(ctx, `SELECT refresh_hash, client_id FROM mcp_grants
			WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL AND refresh_expires_at > now() FOR UPDATE`, grant, tenant).Scan(&hash, &gc)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidGrant
		}
		if err != nil {
			return err
		}
		if gc != client || !hashEqual(hash, hashToken(refresh)) {
			return errInvalidGrant
		}
		tp, h, err := s.mintPair(tenant, grant)
		if err != nil {
			return err
		}
		out = tp
		_, err = tx.Exec(ctx, `UPDATE mcp_grants SET access_hash = $3, access_expires_at = now() + make_interval(secs => $4), refresh_hash = $5, refresh_expires_at = now() + make_interval(secs => $6)
			WHERE id = $1 AND tenant_id = $2`, grant, tenant, h[0], accessTTL.Seconds(), h[1], refreshTTL.Seconds())
		return err
	})
	return out, err
}

type grantInfo struct {
	ID       uuid.UUID
	Actor    authz.Actor
	Client   string
	TenantID uuid.UUID
}

// authenticateAccess memeriksa access token (belum dicabut/kedaluwarsa) lalu memuat hak akses pengguna TERKINI dari DB
// (perubahan role/penonaktifan langsung berlaku). last_used_at diperbarui paling sering tiap 5 menit.
func (s *Service) authenticateAccess(ctx context.Context, resolver *authz.Resolver, tok string) (grantInfo, error) {
	tenant, grant, ok := parseToken(accessPrefix, tok)
	if !ok {
		return grantInfo{}, errInvalidGrant
	}
	var gi grantInfo
	var user, outlet uuid.UUID
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		var hash string
		err := tx.QueryRow(ctx, `SELECT user_id, outlet_id, client_name, access_hash FROM mcp_grants
			WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL AND access_expires_at > now()`, grant, tenant).Scan(&user, &outlet, &gi.Client, &hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidGrant
		}
		if err != nil {
			return err
		}
		if !hashEqual(hash, hashToken(tok)) {
			return errInvalidGrant
		}
		_, err = tx.Exec(ctx, `UPDATE mcp_grants SET last_used_at = now() WHERE id = $1 AND tenant_id = $2 AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes')`, grant, tenant)
		return err
	})
	if err != nil {
		return gi, err
	}
	access, err := resolver.For(ctx, tenant, user)
	if err != nil {
		return gi, errInvalidGrant // pengguna/tenant nonaktif
	}
	if !access.Outlets[outlet] { // outlet saat persetujuan tak lagi boleh diakses: pakai salah satu outlet yang masih boleh
		outlet = uuid.Nil
		for id, ok := range access.Outlets {
			if ok {
				outlet = id
				break
			}
		}
		if outlet == uuid.Nil {
			return gi, errInvalidGrant
		}
	}
	gi.ID, gi.TenantID = grant, tenant
	gi.Actor = authz.Actor{TenantID: tenant, UserID: user, OutletID: outlet, Name: access.Name, Perms: access.Perms, Outlets: access.Outlets}
	return gi, nil
}

// Connection = izin connector yang terlihat di pengaturan HalloArus.
type Connection struct {
	ID         uuid.UUID  `json:"id"`
	Client     string     `json:"client"`
	UserName   string     `json:"user_name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Own        bool       `json:"own"`
}

// Connections: pengelola (ai_assistant.update) melihat semua izin di tenant; pengguna biasa hanya miliknya.
func (s *Service) Connections(ctx context.Context, a authz.Actor) ([]Connection, error) {
	all := a.Perms.Has(Module, authz.ActUpdate)
	out := []Connection{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT g.id, g.client_name, coalesce(u.name, ''), g.created_at, g.last_used_at, g.user_id = $2
			FROM mcp_grants g LEFT JOIN users u ON u.tenant_id = g.tenant_id AND u.id = g.user_id
			WHERE g.tenant_id = $1 AND g.revoked_at IS NULL AND g.refresh_expires_at > now() AND ($3 OR g.user_id = $2)
			ORDER BY g.created_at DESC LIMIT 100`, a.TenantID, a.UserID, all)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Connection
			if err := rows.Scan(&c.ID, &c.Client, &c.UserName, &c.CreatedAt, &c.LastUsedAt, &c.Own); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// RevokeConnection mencabut izin (milik sendiri, atau milik siapa pun bagi pengelola). Token langsung tak berlaku.
func (s *Service) RevokeConnection(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	all := a.Perms.Has(Module, authz.ActUpdate)
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE mcp_grants SET revoked_at = now() WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL AND ($4 OR user_id = $3)`,
			id, a.TenantID, a.UserID, all)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
