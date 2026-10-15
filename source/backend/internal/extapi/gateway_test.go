package extapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Integrasi gateway: tanda tangan, IP, replay, timestamp, rate limit, status kredensial, dan jalur ke handler asli dengan izin
// akun layanan. Butuh TEST_DATABASE_URL, TEST_ADMIN_DATABASE_URL, TEST_REDIS_URL (jika tidak ada: SKIP).
func TestGatewayEndToEnd(t *testing.T) {
	appURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if appURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL/TEST_REDIS_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	ropt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(ropt)
	t.Cleanup(func() { _ = rdb.Close() })

	tenant, roleID, userID, outlet := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sfx := tenant.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tenant, "ex-" + sfx, "UJI-EX-" + sfx}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'a', 'A')`, []any{outlet, tenant}},
		{`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, 'Integrasi', '{"items":["view"]}')`, []any{roleID, tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Svc', 'x')`, []any{userID, tenant, roleID, "ex-" + sfx + "@ex.test"}},
		{`INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, []any{tenant, userID, outlet}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("siapkan: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"api_credentials", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})

	const jwtSecret = "rahasia-uji-rahasia-uji-rahasia-uji-123"
	store, err := NewStore(app, jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	keyID, secret := "ak_uji"+sfx, "sk_uji_"+sfx
	sealed, _ := store.box.seal(secret, keyID)
	credID := uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO api_credentials (id, tenant_id, user_id, outlet_id, label, key_id, secret_enc, ip_allowlist, rate_limit_min)
		VALUES ($1,$2,$3,$4,'uji',$5,$6,'{192.0.2.0/24}',5)`, credID, tenant, userID, outlet, keyID, sealed); err != nil {
		t.Fatalf("kredensial: %v", err)
	}

	tokens := pauth.NewTokenIssuer(jwtSecret)
	res := authz.NewResolver(app)
	mux := chi.NewRouter()
	mux.Use(func(next http.Handler) http.Handler { // RemoteAddr dari header uji
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := r.Header.Get("X-Test-IP"); ip != "" {
				r.RemoteAddr = ip + ":1234"
			}
			next.ServeHTTP(w, r)
		})
	})
	echo := func(w http.ResponseWriter, r *http.Request) {
		a, _ := authz.ActorFrom(r.Context())
		b, _ := io.ReadAll(r.Body)
		httpx.JSON(w, 200, map[string]any{"user": a.UserID, "tenant": a.TenantID, "outlet": a.OutletID, "body": string(b), "q": r.URL.RawQuery, "path": r.URL.Path})
	}
	mux.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth(tokens), res.Authenticate)
		r.With(authz.Require("items", authz.ActView)).Get("/items", echo)
		r.With(authz.Require("items", authz.ActView)).Post("/items", echo)
		r.With(authz.Require("journals", authz.ActCreate)).Post("/accounting/journals", echo)
	})
	mux.Get("/auth/me", echo)
	NewGateway(store, rdb, tokens, mux, slogDiscard()).Mount(mux)

	// call menandatangani request dengan `secret` terkini; mut boleh mengubah header / string yang ditandatangani.
	call := func(method, target, body string, mut func(h http.Header, sts *string), ip string) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		u := httptest.NewRequest(method, target, strings.NewReader(body)).URL
		ts, nonce := strconv.FormatInt(time.Now().Unix(), 10), uuid.NewString()
		h := http.Header{}
		sts := StringToSign(method, u.Path, u.Query(), ts, nonce, BodyHash([]byte(body)))
		h.Set(HeaderKey, keyID)
		h.Set(HeaderTimestamp, ts)
		h.Set(HeaderNonce, nonce)
		if mut != nil {
			mut(h, &sts)
		}
		if h.Get("X-Bad-Sig") != "" {
			h.Set(HeaderSignature, strings.Repeat("0", 64))
		} else {
			h.Set(HeaderSignature, Sign(secret, sts))
		}
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header = h
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-IP", ip)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}
	errCode := func(m map[string]any) string {
		e, _ := m["error"].(map[string]any)
		c, _ := e["code"].(string)
		return c
	}
	const ok = "192.0.2.10"
	fresh := func() { _ = rdb.Del(ctx, "rl:ext:"+credID.String()).Err() }

	// 1) request sah → handler asli berjalan atas nama akun layanan; path diteruskan tanpa prefix.
	rec, out := call("POST", Prefix+"/items?b=2&a=1", `{"x":1}`, nil, ok)
	if rec.Code != 200 || out["user"] != userID.String() || out["tenant"] != tenant.String() || out["outlet"] != outlet.String() ||
		out["body"] != `{"x":1}` || out["path"] != "/items" || out["q"] != "b=2&a=1" {
		t.Fatalf("request sah: %d %s", rec.Code, rec.Body)
	}
	if rec, out = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 200 {
		t.Fatalf("GET sah: %d %v", rec.Code, out)
	}

	// 2) penolakan
	if rec, out = call("GET", Prefix+"/items", "", func(h http.Header, _ *string) { h.Set("X-Bad-Sig", "1") }, ok); rec.Code != 401 || errCode(out) != "SIGNATURE_INVALID" {
		t.Errorf("tanda tangan salah: %d %v", rec.Code, out)
	}
	if rec, out = call("GET", Prefix+"/items", "", nil, "198.51.100.9"); rec.Code != 403 || errCode(out) != "IP_NOT_ALLOWED" {
		t.Errorf("IP di luar allowlist: %d %v", rec.Code, out)
	}
	if rec, out = call("POST", Prefix+"/items", `{"x":1}`, func(h http.Header, sts *string) {
		*sts = StringToSign("POST", Prefix+"/items", nil, h.Get(HeaderTimestamp), h.Get(HeaderNonce), BodyHash([]byte(`{"x":2}`)))
	}, ok); rec.Code != 401 {
		t.Errorf("badan dimanipulasi: %d %v", rec.Code, out)
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if rec, out = call("GET", Prefix+"/items", "", func(h http.Header, sts *string) {
		h.Set(HeaderTimestamp, old)
		*sts = StringToSign("GET", Prefix+"/items", nil, old, h.Get(HeaderNonce), BodyHash(nil))
	}, ok); rec.Code != 401 || errCode(out) != "TIMESTAMP_SKEW" {
		t.Errorf("timestamp lama: %d %v", rec.Code, out)
	}
	if rec, out = call("GET", Prefix+"/items", "", func(h http.Header, _ *string) { h.Del(HeaderKey) }, ok); rec.Code != 401 {
		t.Errorf("header kurang: %d %v", rec.Code, out)
	}
	if rec, out = call("GET", Prefix+"/auth/me", "", nil, ok); rec.Code != 404 {
		t.Errorf("rute /auth tidak boleh lewat gateway: %d %v", rec.Code, out)
	}
	if rec, out = call("POST", Prefix+"/accounting/journals", `{}`, nil, ok); rec.Code != 403 || errCode(out) != "FORBIDDEN" {
		t.Errorf("izin akun layanan ditegakkan handler asli: %d %v", rec.Code, out)
	}

	// 3) replay: nonce yang sama dua kali
	nonce := uuid.NewString()
	same := func(h http.Header, sts *string) {
		h.Set(HeaderNonce, nonce)
		*sts = StringToSign("GET", Prefix+"/items", nil, h.Get(HeaderTimestamp), nonce, BodyHash(nil))
	}
	if rec, _ = call("GET", Prefix+"/items", "", same, ok); rec.Code != 200 {
		t.Fatalf("nonce pertama: %d", rec.Code)
	}
	if rec, out = call("GET", Prefix+"/items", "", same, ok); rec.Code != 401 || errCode(out) != "REPLAY_DETECTED" {
		t.Errorf("replay: %d %v", rec.Code, out)
	}

	// 4) rate limit (batas 5/menit)
	fresh()
	for i := 0; i < 5; i++ {
		if rec, _ = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 200 {
			t.Fatalf("permintaan %d: %d", i+1, rec.Code)
		}
	}
	if rec, out = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 429 || errCode(out) != "RATE_LIMITED" || rec.Header().Get("Retry-After") == "" {
		t.Errorf("rate limit: %d %v", rec.Code, out)
	}
	fresh()

	// 5) rotasi dengan masa transisi: secret lama tetap sah sampai batas waktu, lalu mati
	oldSecret, newSecret := secret, "sk_baru_"+sfx
	newSealed, _ := store.box.seal(newSecret, keyID)
	if _, err := admin.Exec(ctx, `UPDATE api_credentials SET prev_secret_enc = secret_enc, prev_expires_at = now() + interval '1 hour', secret_enc = $2 WHERE id = $1`, credID, newSealed); err != nil {
		t.Fatal(err)
	}
	if rec, _ = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 200 {
		t.Errorf("secret lama saat transisi: %d", rec.Code)
	}
	if _, err := admin.Exec(ctx, `UPDATE api_credentials SET prev_expires_at = now() - interval '1 minute' WHERE id = $1`, credID); err != nil {
		t.Fatal(err)
	}
	if rec, _ = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 401 {
		t.Errorf("secret lama sesudah transisi: %d", rec.Code)
	}
	secret = newSecret
	if rec, _ = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 200 {
		t.Errorf("secret baru: %d", rec.Code)
	}
	_ = oldSecret

	// 6) nonaktif, kedaluwarsa, akun layanan dinonaktifkan
	if _, err := admin.Exec(ctx, `UPDATE api_credentials SET active = false WHERE id = $1`, credID); err != nil {
		t.Fatal(err)
	}
	if rec, out = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 401 || errCode(out) != "CREDENTIAL_DISABLED" {
		t.Errorf("nonaktif: %d %v", rec.Code, out)
	}
	if _, err := admin.Exec(ctx, `UPDATE api_credentials SET active = true, expires_at = now() - interval '1 minute' WHERE id = $1`, credID); err != nil {
		t.Fatal(err)
	}
	if rec, out = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 401 || errCode(out) != "CREDENTIAL_EXPIRED" {
		t.Errorf("kedaluwarsa: %d %v", rec.Code, out)
	}
	if _, err := admin.Exec(ctx, `UPDATE api_credentials SET expires_at = NULL WHERE id = $1`, credID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE users SET active = false WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	res.InvalidateTenant(tenant)
	if rec, out = call("GET", Prefix+"/items", "", nil, ok); rec.Code != 401 || errCode(out) != "CREDENTIAL_DISABLED" {
		t.Errorf("akun layanan nonaktif: %d %v", rec.Code, out)
	}
	fresh()
}
