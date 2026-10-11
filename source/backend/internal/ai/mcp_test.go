package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
)

type mcpEnv struct {
	h      *MCPHandler
	admin  *pgxpool.Pool
	actor  authz.Actor
	tenant uuid.UUID
}

// newMCP membuat tenant sementara (Owner + satu outlet) dan menghapusnya setelah test; tenant dev pengguna tidak disentuh.
func newMCP(t *testing.T) *mcpEnv {
	t.Helper()
	appURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if appURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("env test integrasi tidak di-set")
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
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	tenant, outlet, role, user := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants (id, code, name) VALUES ($1::uuid, 'mcp-' || substr($1::uuid::text, 1, 8), 'UJI-MCP')`, tenant)
	exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat Uji')`, outlet, tenant)
	exec(`INSERT INTO roles (id, tenant_id, name, permissions, is_system) VALUES ($1, $2, 'Owner', '{"*":true}'::jsonb, true)`, role, tenant)
	exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Bu Owner', 'x')`, user, tenant, role, user.String()+"@example.test")
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "mcp_grants", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	})

	svc, err := NewService(app, rdb, "0123456789abcdef0123456789abcdef", Free{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := NewMCPHandler(svc, authz.NewResolver(app), nil, rdb, slog.New(slog.NewTextHandler(io.Discard, nil)), "https://api.test", "https://app.test")
	return &mcpEnv{h: h, admin: admin, tenant: tenant, actor: authz.Actor{TenantID: tenant, UserID: user, OutletID: outlet, Name: "Bu Owner",
		Perms: authz.Permissions{All: true}, Outlets: map[uuid.UUID]bool{outlet: true}}}
}

func do(h http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func pkce() (verifier, challenge string) {
	verifier = strings.Repeat("a1B2c3D4e5", 5) // 50 karakter
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func jsonReq(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// register + approve → kode otorisasi. Mengembalikan client_id dan kode.
func (e *mcpEnv) authorize(t *testing.T, challenge string) (clientID, code string) {
	t.Helper()
	w := do(e.h.register, jsonReq("POST", "/oauth/register", map[string]any{"client_name": "Claude", "redirect_uris": []string{"https://claude.ai/api/mcp/auth_callback"}}))
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reg)
	req := jsonReq("POST", "/oauth/authorize/approve", map[string]string{
		"client_id": reg.ClientID, "redirect_uri": "https://claude.ai/api/mcp/auth_callback", "state": "st-1",
		"code_challenge": challenge, "code_challenge_method": "S256", "decision": "allow"})
	w = do(e.h.approve, req.WithContext(authz.WithActor(req.Context(), e.actor)))
	if w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	var ap struct {
		RedirectURL string `json:"redirect_url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ap)
	u, err := url.Parse(ap.RedirectURL)
	if err != nil || u.Host != "claude.ai" || u.Query().Get("state") != "st-1" || u.Query().Get("code") == "" {
		t.Fatalf("redirect: %s", ap.RedirectURL)
	}
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM mcp_grants WHERE client_id = $1`, reg.ClientID)
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM oauth_clients WHERE client_id = $1`, reg.ClientID)
	})
	return reg.ClientID, u.Query().Get("code")
}

func (e *mcpEnv) tokenReq(form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(e.h.token, r)
}

type tokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
	Type    string `json:"token_type"`
}

func (e *mcpEnv) rpc(t *testing.T, access string, body any) (int, map[string]any) {
	t.Helper()
	r := jsonReq("POST", "/mcp", body)
	if access != "" {
		r.Header.Set("Authorization", "Bearer "+access)
	}
	w := do(e.h.mcp, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if access == "" && w.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 wajib memuat WWW-Authenticate")
	}
	return w.Code, out
}

func TestOAuthAndMCPFlow(t *testing.T) {
	e := newMCP(t)
	verifier, challenge := pkce()
	cid, code := e.authorize(t, challenge)

	// PKCE salah → ditolak, dan kode sekali pakai (tidak bisa dicoba ulang dengan verifier benar).
	w := e.tokenReq(url.Values{"grant_type": {"authorization_code"}, "client_id": {cid}, "code": {code}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "code_verifier": {strings.Repeat("z", 50)}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_grant") {
		t.Fatalf("PKCE salah harus ditolak: %d %s", w.Code, w.Body)
	}
	good := url.Values{"grant_type": {"authorization_code"}, "client_id": {cid}, "code": {code}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "code_verifier": {verifier}}
	if w = e.tokenReq(good); w.Code != http.StatusBadRequest {
		t.Fatalf("kode harus sekali pakai: %d", w.Code)
	}

	// Alur benar.
	cid, code = e.authorize(t, challenge)
	good.Set("client_id", cid)
	good.Set("code", code)
	// redirect_uri berbeda → ditolak
	bad := url.Values{}
	for k, v := range good {
		bad[k] = v
	}
	bad.Set("redirect_uri", "https://evil.example/cb")
	if w = e.tokenReq(bad); w.Code != http.StatusBadRequest {
		t.Fatalf("redirect_uri beda harus ditolak: %d", w.Code)
	}
	cid, code = e.authorize(t, challenge)
	good.Set("client_id", cid)
	good.Set("code", code)
	w = e.tokenReq(good)
	if w.Code != http.StatusOK {
		t.Fatalf("token: %d %s", w.Code, w.Body)
	}
	var tk tokens
	_ = json.Unmarshal(w.Body.Bytes(), &tk)
	if tk.Type != "Bearer" || !strings.HasPrefix(tk.Access, "mat_") || !strings.HasPrefix(tk.Refresh, "mrt_") {
		t.Fatalf("token: %+v", tk)
	}

	// Tanpa / dengan token palsu → 401.
	if code, _ := e.rpc(t, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); code != http.StatusUnauthorized {
		t.Fatalf("tanpa token: %d", code)
	}
	if code, _ := e.rpc(t, "mat_"+e.tenant.String()+"_"+uuid.NewString()+"_"+strings.Repeat("0", 64), map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); code != http.StatusUnauthorized {
		t.Fatalf("token palsu: %d", code)
	}

	// initialize + tools/list (semua read-only) + tools/call
	code2, res := e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-03-26"}})
	if code2 != 200 || res["result"].(map[string]any)["protocolVersion"] != "2025-03-26" {
		t.Fatalf("initialize: %v", res)
	}
	_, res = e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	tools := res["result"].(map[string]any)["tools"].([]any)
	if len(tools) != len(registry()) {
		t.Fatalf("jumlah tool: %d", len(tools))
	}
	for _, x := range tools {
		if x.(map[string]any)["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Fatalf("semua tool harus readOnlyHint: %v", x)
		}
	}
	_, res = e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "list_outlets", "arguments": map[string]any{}}})
	r := res["result"].(map[string]any)
	if r["isError"] != false || !strings.Contains(r["content"].([]any)[0].(map[string]any)["text"].(string), "Pusat Uji") {
		t.Fatalf("list_outlets: %v", res)
	}
	_, res = e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "drop_table"}})
	if res["error"] == nil {
		t.Fatalf("tool tak dikenal harus galat: %v", res)
	}
	// notifikasi → 202 tanpa badan
	r2 := jsonReq("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	r2.Header.Set("Authorization", "Bearer "+tk.Access)
	if w := do(e.h.mcp, r2); w.Code != http.StatusAccepted {
		t.Fatalf("notifikasi: %d", w.Code)
	}

	// Rotasi refresh: access lama mati, refresh lama mati.
	w = e.tokenReq(url.Values{"grant_type": {"refresh_token"}, "client_id": {cid}, "refresh_token": {tk.Refresh}})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", w.Code, w.Body)
	}
	var tk2 tokens
	_ = json.Unmarshal(w.Body.Bytes(), &tk2)
	if c, _ := e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); c != http.StatusUnauthorized {
		t.Fatalf("access lama harus mati: %d", c)
	}
	if w = e.tokenReq(url.Values{"grant_type": {"refresh_token"}, "client_id": {cid}, "refresh_token": {tk.Refresh}}); w.Code != http.StatusBadRequest {
		t.Fatalf("refresh lama harus mati: %d", w.Code)
	}
	if c, _ := e.rpc(t, tk2.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); c != 200 {
		t.Fatalf("access baru: %d", c)
	}

	// Pencabutan: pengguna melihat izinnya lalu mencabut → token langsung tak berlaku.
	list, err := e.h.svc.Connections(context.Background(), e.actor)
	if err != nil || len(list) == 0 {
		t.Fatalf("connections: %v %v", list, err)
	}
	for _, c := range list {
		if err := e.h.svc.RevokeConnection(context.Background(), e.actor, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if c, _ := e.rpc(t, tk2.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); c != http.StatusUnauthorized {
		t.Fatalf("setelah dicabut: %d", c)
	}
}

// Pengguna yang kehilangan izin ai_assistant tidak bisa lagi memakai token lamanya (izin dibaca TERKINI dari DB).
func TestMCPPermissionRevoked(t *testing.T) {
	e := newMCP(t)
	verifier, challenge := pkce()
	cid, code := e.authorize(t, challenge)
	w := e.tokenReq(url.Values{"grant_type": {"authorization_code"}, "client_id": {cid}, "code": {code}, "redirect_uri": {"https://claude.ai/api/mcp/auth_callback"}, "code_verifier": {verifier}})
	var tk tokens
	_ = json.Unmarshal(w.Body.Bytes(), &tk)
	if c, _ := e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); c != 200 {
		t.Fatalf("awal: %d", c)
	}
	if _, err := e.admin.Exec(context.Background(), `INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, e.tenant, e.actor.UserID, e.actor.OutletID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(context.Background(), `UPDATE roles SET permissions = '{"items":["view"]}'::jsonb, is_system = false WHERE tenant_id = $1`, e.tenant); err != nil {
		t.Fatal(err)
	}
	e.h.resolver.InvalidateTenant(e.tenant)
	if c, _ := e.rpc(t, tk.Access, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}); c != http.StatusForbidden {
		t.Fatalf("tanpa izin ai_assistant harus 403: %d", c)
	}
}

func TestOAuthValidation(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://claude.ai/cb": true, "http://localhost:3000/cb": true, "http://127.0.0.1/cb": true,
		"http://evil.example/cb": false, "javascript:alert(1)": false, "https://a.b/cb#frag": false, "https://user:pw@a.b/cb": false, "claude://cb": false,
	} {
		if validRedirect(raw) != want {
			t.Errorf("validRedirect(%q) != %v", raw, want)
		}
	}
	tn, g := uuid.New(), uuid.New()
	tok, _ := newToken(accessPrefix, tn, g)
	if a, b, ok := parseToken(accessPrefix, tok); !ok || a != tn || b != g {
		t.Fatal("parseToken roundtrip")
	}
	if _, _, ok := parseToken(refreshPrefix, tok); ok {
		t.Fatal("prefix harus cocok")
	}
}
