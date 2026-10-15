package extapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

const (
	// Prefix = awalan path API eksternal; sisanya diteruskan ke router yang sama tanpa awalan ini.
	Prefix = "/ext/v1"

	maxSkew      = 5 * time.Minute
	nonceTTL     = 2*maxSkew + time.Minute // nonce harus diingat selama timestamp-nya masih bisa lolos
	maxBodyBytes = 10 << 20

	// ipLimitPerMin = batas kasar per IP pemanggil (di atas batas per kredensial) untuk melindungi lookup DB.
	ipLimitPerMin = 1200
)

var nonceRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// blockedPrefixes = rute yang TIDAK boleh dicapai lewat gateway (sesi pengguna, operator platform, OAuth/MCP, internal).
var blockedPrefixes = []string{"/auth", "/platform", "/public", "/mcp", "/oauth", "/.well-known", "/healthz", Prefix}

// Blocked: apakah path (tanpa Prefix) tidak tersedia untuk API eksternal.
func Blocked(path string) bool {
	for _, p := range blockedPrefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// Gateway memverifikasi request bertanda tangan lalu meneruskannya ke `next` (router utama) dengan JWT internal.
type Gateway struct {
	store  *Store
	rdb    *redis.Client
	tokens *pauth.TokenIssuer
	next   http.Handler
	log    *slog.Logger
	now    func() time.Time
}

func NewGateway(store *Store, rdb *redis.Client, tokens *pauth.TokenIssuer, next http.Handler, log *slog.Logger) *Gateway {
	return &Gateway{store: store, rdb: rdb, tokens: tokens, next: next, log: log, now: time.Now}
}

// Mount memasang gateway di /ext/v1/* pada router yang SAMA dengan `next` (router utama).
func (g *Gateway) Mount(r chi.Router) {
	r.HandleFunc(Prefix+"/*", g.serve)
}

func (g *Gateway) deny(w http.ResponseWriter, status int, code, msg string) {
	httpx.Error(w, status, code, msg)
}

const badCredential = "Kredensial atau tanda tangan tidak valid."

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keyID := strings.TrimSpace(r.Header.Get(HeaderKey))
	ts := strings.TrimSpace(r.Header.Get(HeaderTimestamp))
	nonce := strings.TrimSpace(r.Header.Get(HeaderNonce))
	sig := strings.TrimSpace(r.Header.Get(HeaderSignature))
	if keyID == "" || ts == "" || nonce == "" || sig == "" {
		g.deny(w, http.StatusUnauthorized, "SIGNATURE_MISSING", "Header "+HeaderKey+", "+HeaderTimestamp+", "+HeaderNonce+", dan "+HeaderSignature+" wajib diisi.")
		return
	}
	if !nonceRe.MatchString(nonce) || len(keyID) > 64 {
		g.deny(w, http.StatusUnauthorized, "SIGNATURE_INVALID", badCredential)
		return
	}
	tsec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		g.deny(w, http.StatusUnauthorized, "TIMESTAMP_INVALID", "Timestamp harus berupa unix detik.")
		return
	}
	now := g.now()
	if d := now.Sub(time.Unix(tsec, 0)); d > maxSkew || d < -maxSkew {
		g.deny(w, http.StatusUnauthorized, "TIMESTAMP_SKEW", "Timestamp di luar toleransi ±5 menit; sinkronkan jam server Anda (NTP).")
		return
	}

	// 0) batas kasar per IP SEBELUM menyentuh DB: penebak Key ID tidak bisa membanjiri lookup (Redis gagal = tolak).
	ip := httpx.ClientIP(r)
	okIP, wait, err := httpx.Allow(ctx, g.rdb, "rl:ext-ip:"+ip, ipLimitPerMin, time.Minute)
	if err != nil {
		g.deny(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if !okIP {
		httpx.Retry(w, "RATE_LIMITED", "Terlalu banyak permintaan dari alamat IP ini.", wait)
		return
	}

	// 1) kredensial & IP
	cred, err := g.store.lookup(ctx, keyID)
	if errors.Is(err, ErrNotFound) {
		g.deny(w, http.StatusUnauthorized, "SIGNATURE_INVALID", badCredential)
		return
	}
	if err != nil {
		g.log.Error("ext lookup gagal", "err", err)
		g.deny(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	if !IPAllowed(cred.IPAllowlist, ip) {
		g.log.Warn("ext ip ditolak", "key_id", keyID, "ip", ip)
		g.deny(w, http.StatusForbidden, "IP_NOT_ALLOWED", "Alamat IP "+ip+" tidak ada di daftar yang diizinkan untuk kredensial ini.")
		return
	}

	// 2) tanda tangan (badan dibaca penuh, di-hash, lalu dipulihkan untuk handler)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		g.deny(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Badan request terlalu besar.")
		return
	}
	sts := StringToSign(r.Method, r.URL.Path, r.URL.Query(), ts, nonce, BodyHash(body))
	if !g.verify(cred, keyID, sts, sig, now) {
		g.deny(w, http.StatusUnauthorized, "SIGNATURE_INVALID", badCredential)
		return
	}

	// 3) status kredensial (setelah tanda tangan sah, agar penebak tak bisa menguji status)
	if !cred.Active {
		g.deny(w, http.StatusUnauthorized, "CREDENTIAL_DISABLED", "Kredensial dinonaktifkan.")
		return
	}
	if cred.ExpiresAt != nil && !now.Before(*cred.ExpiresAt) {
		g.deny(w, http.StatusUnauthorized, "CREDENTIAL_EXPIRED", "Kredensial sudah kedaluwarsa.")
		return
	}

	// 4) nonce sekali pakai (Redis gagal = tolak)
	okNonce, err := g.rdb.SetNX(ctx, fmt.Sprintf("ext:nonce:%s:%s", cred.ID, nonce), 1, nonceTTL).Result()
	if err != nil {
		g.deny(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if !okNonce {
		g.deny(w, http.StatusUnauthorized, "REPLAY_DETECTED", "Nonce ini sudah pernah dipakai.")
		return
	}

	// 5) rate limit per kredensial
	allowed, wait, err := httpx.Allow(ctx, g.rdb, "rl:ext:"+cred.ID.String(), cred.RateLimit, time.Minute)
	if err != nil {
		g.deny(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Layanan sementara tidak tersedia.")
		return
	}
	if !allowed {
		httpx.Retry(w, "RATE_LIMITED", "Batas permintaan per menit terlampaui.", wait)
		return
	}

	// 6) hanya rute bisnis yang diteruskan
	inner := strings.TrimPrefix(r.URL.Path, Prefix)
	if inner == "" || inner == "/" || Blocked(inner) {
		g.deny(w, http.StatusNotFound, "NOT_FOUND", "Endpoint tidak tersedia untuk API eksternal.")
		return
	}

	token, err := g.tokens.Issue(cred.UserID.String(), cred.TenantID.String(), cred.OutletID.String(), "api", now)
	if err != nil {
		g.deny(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	go g.store.touch(cred.ID, ip)

	// Teruskan ke router utama: path ditulis ulang, header tanda tangan & cookie dibuang, konteks rute baru.
	r2 := r.Clone(context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext()))
	r2.URL.Path, r2.URL.RawPath, r2.RequestURI = inner, "", inner
	if r.URL.RawQuery != "" {
		r2.RequestURI += "?" + r.URL.RawQuery
	}
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.ContentLength = int64(len(body))
	for _, h := range []string{HeaderKey, HeaderTimestamp, HeaderNonce, HeaderSignature, "Cookie", "X-Client"} {
		r2.Header.Del(h)
	}
	r2.Header.Set("Authorization", "Bearer "+token)
	w.Header().Set("X-Api-Key-Id", keyID)
	g.next.ServeHTTP(w, r2)
}

// verify mencocokkan tanda tangan dengan secret aktif, atau secret lama selama masa transisi rotasi.
func (g *Gateway) verify(c *lookup, keyID, stringToSign, sig string, now time.Time) bool {
	if sec, err := g.store.box.open(c.SecretEnc, keyID); err == nil && Equal(Sign(sec, stringToSign), sig) {
		return true
	}
	if c.PrevSecretEnc != nil && c.PrevExpiresAt != nil && now.Before(*c.PrevExpiresAt) {
		if sec, err := g.store.box.open(*c.PrevSecretEnc, keyID); err == nil && Equal(Sign(sec, stringToSign), sig) {
			return true
		}
	}
	return false
}
