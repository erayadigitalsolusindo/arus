package ai

// Penyedia AI kustom per tenant (API gaya OpenAI /chat/completions). Base URL diisi tenant, jadi SERVER yang memanggilnya:
// PRINSIP KEAMANAN (jangan dilonggarkan):
//   - Hanya https, tanpa userinfo/query/fragment; host harus resolve ke alamat publik (bukan loopback/privat/link-local/CGNAT/metadata).
//   - Pemeriksaan alamat juga dilakukan SAAT KONEKSI (Dialer.Control pada IP yang benar-benar dituju), sehingga DNS rebinding tidak
//     bisa menyelundupkan IP internal setelah validasi simpan.
//   - Tanpa proxy dari lingkungan, redirect tidak diikuti, dan API key hanya dikirim ke base URL yang tersimpan; mengganti base URL
//     mewajibkan memasukkan ulang key (key lama tidak ikut dikirim ke host baru).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	maxCustomProviders = 5
	customLabelMax     = 40
	customURLMax       = 300
)

var (
	ErrBadBaseURL    = errors.New("base URL tidak diizinkan")
	ErrTooManyCustom = errors.New("jumlah penyedia kustom melebihi batas")

	customID    = regexp.MustCompile(`^custom_[a-z0-9]{8}$`)
	customModel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$`)
)

// IsCustom = id penyedia kustom milik tenant.
func IsCustom(id string) bool { return customID.MatchString(id) }

func customKeyOK(k string) bool {
	return len(k) >= 8 && len(k) <= 300 && !strings.ContainsAny(k, " \r\n\t")
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "2001:db8::/32"} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// blockedIP = alamat yang tidak boleh dituju penyedia kustom (internal/khusus).
func blockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || !ip.IsGlobalUnicast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// checkResolve: host (literal IP atau nama) tidak boleh menunjuk ke alamat terlarang. Dipakai saat simpan agar pengguna mendapat
// pesan yang jelas; pertahanan sebenarnya ada di customDialControl.
var checkResolve = func(ctx context.Context, host string) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		if blockedIP(ip) {
			return ErrBadBaseURL
		}
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(rctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return ErrBadBaseURL
	}
	for _, ip := range ips {
		if blockedIP(ip) {
			return ErrBadBaseURL
		}
	}
	return nil
}

// validateBaseURL menormalkan dan memeriksa bentuk URL (tanpa resolve DNS). Hasil tanpa garis miring penutup.
func validateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > customURLMax {
		return "", ErrBadBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(raw, " \t\r\n\\") {
		return "", ErrBadBaseURL
	}
	host := u.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return "", ErrBadBaseURL
	}
	if p := u.Port(); p != "" && (len(p) > 5 || p == "0") {
		return "", ErrBadBaseURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// customDialControl dipanggil tepat sebelum connect() dengan alamat IP hasil resolve.
var customDialControl = func(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBadBaseURL
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || blockedIP(ip) {
		return ErrBadBaseURL
	}
	return nil
}

func safeHTTPClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: customDialControl}
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil, // jangan lewat proxy lingkungan: Control harus melihat tujuan sebenarnya
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 100 * time.Second,
			MaxIdleConns:          2,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// newCustomClient: klien OpenAI-compatible ke base URL tenant. Verifikasi memakai chat 1 token (menguji URL, key, dan model sekaligus).
// Variabel agar tes bisa memakai server TLS lokal; kode produksi tidak pernah mengubahnya.
var newCustomClient = func(base string) *openaiClient {
	return &openaiClient{http: safeHTTPClient(), base: base, label: "penyedia kustom", probe: true}
}

// CustomInput = isian form penyedia kustom. ID kosong = penyedia baru.
type CustomInput struct {
	ID      string
	Label   string
	BaseURL string
	Model   string
	APIKey  string
}

func newCustomID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "custom_" + hex.EncodeToString(b)
}

// SaveCustom menambah/mengubah satu penyedia kustom tenant lalu menjadikannya aktif. Key diverifikasi ke endpoint-nya lebih dulu.
func (s *Service) SaveCustom(ctx context.Context, a authz.Actor, in CustomInput) (string, error) {
	if a.Impersonator != uuid.Nil {
		return "", ErrImpersonating
	}
	if pol, err := s.mustPolicy(ctx, a); err != nil {
		return "", err
	} else if !pol.AllowCustom {
		return "", ErrPolicyBlocked
	}
	label, code := sanitize.Name(in.Label, customLabelMax)
	model := strings.TrimSpace(in.Model)
	apiKey := strings.TrimSpace(in.APIKey)
	if code != "" || !customModel.MatchString(model) || (apiKey != "" && !customKeyOK(apiKey)) {
		return "", ErrBadInput
	}
	base, err := validateBaseURL(in.BaseURL)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(base)
	if err := checkResolve(ctx, u.Hostname()); err != nil {
		return "", err
	}
	id, isNew := in.ID, in.ID == ""
	if isNew {
		id = newCustomID()
	} else if !IsCustom(id) {
		return "", ErrBadInput
	}

	var oldEnc, oldBase string
	if !isNew {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT api_key_enc, base_url FROM ai_settings WHERE tenant_id = $1 AND provider = $2`, a.TenantID, id).Scan(&oldEnc, &oldBase)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotConfigured
		}
		if err != nil {
			return "", err
		}
	}
	// Key hanya boleh dikirim ke host yang tersimpan: ganti base URL → wajib key baru.
	if apiKey == "" && (isNew || oldBase != base) {
		return "", ErrBadKey
	}
	var enc, hint string
	verifyKey := apiKey
	if apiKey != "" {
		if enc, err = s.box.seal(apiKey, a.TenantID.String()); err != nil {
			return "", err
		}
		hint = apiKey[max(len(apiKey)-4, 0):]
	} else if verifyKey, err = s.box.open(oldEnc, a.TenantID.String()); err != nil {
		return "", err
	}
	if err := newCustomClient(base).verify(ctx, verifyKey, model); err != nil {
		return "", err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if isNew {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM ai_settings WHERE tenant_id = $1 AND provider LIKE 'custom\_%'`, a.TenantID).Scan(&n); err != nil {
				return err
			}
			if n >= maxCustomProviders {
				return ErrTooManyCustom
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE ai_settings SET active = false WHERE tenant_id = $1 AND provider <> $2 AND active`, a.TenantID, id); err != nil {
			return err
		}
		if apiKey == "" {
			if _, err := tx.Exec(ctx, `UPDATE ai_settings SET label = $3, model = $4, active = true, updated_by = $5, updated_at = now() WHERE tenant_id = $1 AND provider = $2`,
				a.TenantID, id, label, model, a.UserID); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `INSERT INTO ai_settings (tenant_id, provider, api_key_enc, key_hint, model, base_url, label, active, updated_by) VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8)
			ON CONFLICT (tenant_id, provider) DO UPDATE SET api_key_enc = EXCLUDED.api_key_enc, key_hint = EXCLUDED.key_hint, model = EXCLUDED.model, base_url = EXCLUDED.base_url,
			label = EXCLUDED.label, active = true, updated_by = EXCLUDED.updated_by, updated_at = now()`, a.TenantID, id, enc, hint, model, base, label, a.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAISettings, Entity: audit.EntityAI, EntityID: a.TenantID.String(),
			Details: map[string]any{"provider": id, "custom": true, "label": label, "model": model, "base_host": u.Host, "new": isNew, "key_changed": apiKey != ""}})
	})
	return id, err
}
