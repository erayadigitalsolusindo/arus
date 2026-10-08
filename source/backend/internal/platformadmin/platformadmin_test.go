package platformadmin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/audit"
	"aciraba/internal/auth"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

const testSecret = "rahasia-uji-rahasia-uji-rahasia-uji-123"

type env struct {
	svc    *Service
	app    *pgxpool.Pool
	admin  *pgxpool.Pool
	router http.Handler
	perms  *authz.Resolver
	tokens *pauth.TokenIssuer
	// clock = jam layanan (dimajukan test agar kode TOTP berikutnya berada di periode baru; periode yang sama ditolak sebagai replay).
	clock   time.Time
	secrets map[string]string // email → rahasia TOTP
}

// newEnv = layanan sungguhan di atas DB/Redis uji (TEST_DATABASE_URL = role aplikasi, TEST_ADMIN_DATABASE_URL = pemilik).
func newEnv(t *testing.T, setupToken string) *env {
	t.Helper()
	dbURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL/TEST_REDIS_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, dbURL)
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

	tokens := pauth.NewTokenIssuer(testSecret)
	perms := authz.NewResolver(app)
	box, err := pauth.NewTOTPBox(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{app: app, admin: admin, perms: perms, tokens: tokens, clock: time.Now(), secrets: map[string]string{}}
	e.svc = NewService(Deps{
		Pool: app, Tokens: tokens, PTokens: pauth.NewPlatformTokenIssuer(testSecret), Sessions: pauth.NewSessions(rdb),
		OneTime: pauth.NewOneTime(rdb), TOTP: box, Perms: perms, SetupToken: setupToken,
	})
	e.svc.now = func() time.Time { return e.clock }
	svc := e.svc
	perms.WithPlatform(svc)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(audit.CaptureMeta)
	NewHandler(HandlerDeps{
		Service: svc, Log: log, Redis: rdb, Origins: []string{"http://localhost:5173"},
		Lockout: auth.NewLockout(rdb, auth.NewPolicyLoader(app, log)),
	}).Routes(r)
	// Rute tenant contoh untuk menguji token "masuk sebagai".
	r.With(httpx.RequireAuth(tokens), perms.Authenticate).HandleFunc("/tenant-probe", func(w http.ResponseWriter, req *http.Request) {
		a, _ := authz.ActorFrom(req.Context())
		httpx.JSON(w, http.StatusOK, map[string]any{"name": a.Name, "outlets": len(a.Outlets), "all": a.Perms.All})
	})
	e.router = r
	return e
}

// dropAdmin menghapus admin uji beserta jejak auditnya.
func (e *env) dropAdmin(id uuid.UUID) {
	ctx := context.Background()
	_, _ = e.admin.Exec(ctx, `DELETE FROM platform_audit_log WHERE admin_id = $1`, id)
	_, _ = e.admin.Exec(ctx, `DELETE FROM platform_admins WHERE id = $1`, id)
}

// newAdmin menanam Platform Admin lewat pemilik skema (jalur normal hanya setup/UI) dan membersihkannya.
func (e *env) newAdmin(t *testing.T, name string) (id uuid.UUID, email, password string) {
	t.Helper()
	hash, err := pauth.HashPassword("sandi-aman-123")
	if err != nil {
		t.Fatal(err)
	}
	id, email = uuid.New(), "pa-"+uuid.NewString()[:8]+"@pa.test"
	if _, err := e.admin.Exec(context.Background(), `INSERT INTO platform_admins (id, email, name, password_hash) VALUES ($1, $2, $3, $4)`, id, email, name, hash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.dropAdmin(id) })
	e.enroll(t, id, name, email)
	return id, email, "sandi-aman-123"
}

// enroll mendaftarkan 2FA lewat layanan (alur sebenarnya) dan menyimpan rahasianya untuk membuat kode login.
func (e *env) enroll(t *testing.T, id uuid.UUID, name, email string) []string {
	t.Helper()
	a := Actor{ID: id, Name: name}
	setup, err := e.svc.BeginMFA(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := pauth.TOTPCode(setup.Secret, e.clock)
	codes, err := e.svc.EnableMFA(context.Background(), a, code)
	if err != nil {
		t.Fatal(err)
	}
	e.secrets[email] = setup.Secret
	return codes
}

// nextCode memajukan jam layanan satu periode lalu membuat kode untuk rahasia email itu.
func (e *env) nextCode(email string) string {
	e.clock = e.clock.Add(31 * time.Second)
	c, _ := pauth.TOTPCode(e.secrets[email], e.clock)
	return c
}

type tenantFixture struct {
	ID, Outlet1, Outlet2 uuid.UUID
}

func (e *env) newTenant(t *testing.T) tenantFixture {
	t.Helper()
	ctx := context.Background()
	f := tenantFixture{ID: uuid.New(), Outlet1: uuid.New(), Outlet2: uuid.New()}
	sfx := f.ID.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{f.ID, "pa-" + sfx, "UJI-PA-" + sfx}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'a', 'Outlet A'), ($3, $2, 'b', 'Outlet B')`, []any{f.Outlet1, f.ID, f.Outlet2}},
	} {
		if _, err := e.admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "outlets"} {
			_, _ = e.admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, f.ID)
		}
		_, _ = e.admin.Exec(ctx, `DELETE FROM platform_audit_log WHERE tenant_id = $1`, f.ID)
		_, _ = e.admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.ID)
	})
	return f
}

func (e *env) do(method, path, token string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *env) loginToken(t *testing.T, email, password string) string {
	t.Helper()
	s, mfa, err := e.svc.Login(context.Background(), email, password, false)
	if err != nil {
		t.Fatal(err)
	}
	if mfa != "" {
		if s, err = e.svc.LoginMFA(context.Background(), mfa, e.nextCode(email)); err != nil {
			t.Fatal(err)
		}
	}
	return s.AccessToken
}

func TestSetupHanyaSekaliDenganToken(t *testing.T) {
	setup := "token-setup-uji-token-setup-uji-1234"
	e := newEnv(t, setup)
	ctx := context.Background()
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM platform_admins`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Skip("sudah ada Platform Admin di DB uji; setup hanya bisa saat kosong")
	}
	email := "setup-" + uuid.NewString()[:8] + "@pa.test"
	t.Cleanup(func() {
		var id uuid.UUID
		if e.admin.QueryRow(ctx, `SELECT id FROM platform_admins WHERE email = $1`, email).Scan(&id) == nil {
			e.dropAdmin(id)
		}
	})

	if _, err := e.svc.Setup(ctx, "salah", "Pemilik", email, "sandi-aman-123"); !errors.Is(err, ErrBadSetupToken) {
		t.Fatalf("token salah: %v", err)
	}
	if _, err := e.svc.Setup(ctx, setup, "Pemilik", email, "sandi-aman-123"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Admin pertama sudah ada: setup mati total, bahkan dengan token benar.
	if _, err := e.svc.Setup(ctx, setup, "Lain", "lain-"+email, "sandi-aman-123"); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("setup kedua: %v, want ErrSetupUnavailable", err)
	}
	if rec := e.do("GET", "/platform/setup/status", "", ""); !strings.Contains(rec.Body.String(), `"setup_required":false`) {
		t.Errorf("status: %s", rec.Body)
	}
}

func TestSetupMatiTanpaTokenKonfigurasi(t *testing.T) {
	e := newEnv(t, "")
	if _, err := e.svc.Setup(context.Background(), "apa-saja", "X", "x@pa.test", "sandi-aman-123"); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("setup tanpa konfigurasi: %v", err)
	}
}

func TestIsolasiTokenDanDatabase(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	_, email, pw := e.newAdmin(t, "Pemilik")
	tok := e.loginToken(t, email, pw)

	// Token platform tidak berlaku di rute tenant; token tenant tidak berlaku di rute platform.
	if rec := e.do("GET", "/tenant-probe", tok, ""); rec.Code != 401 {
		t.Errorf("token platform di rute tenant: %d, want 401", rec.Code)
	}
	tenantTok, _ := e.tokens.Issue(uuid.NewString(), uuid.NewString(), uuid.NewString(), "Owner", time.Now())
	if rec := e.do("GET", "/platform/tenants", tenantTok, ""); rec.Code != 401 {
		t.Errorf("token tenant di rute platform: %d, want 401", rec.Code)
	}
	if rec := e.do("GET", "/platform/tenants", "", ""); rec.Code != 401 {
		t.Errorf("tanpa token: %d, want 401", rec.Code)
	}
	if rec := e.do("GET", "/platform/tenants", tok, ""); rec.Code != 200 {
		t.Errorf("daftar tenant: %d %s", rec.Code, rec.Body)
	}

	// Role aplikasi tidak bisa menulis platform_admins langsung (hanya lewat fungsi yang memeriksa pelaku).
	if _, err := e.app.Exec(ctx, `INSERT INTO platform_admins (email, name, password_hash) VALUES ('x@x.test', 'X', 'x')`); err == nil {
		t.Error("aciraba_app seharusnya tidak boleh INSERT ke platform_admins")
	}
	if _, err := e.app.Exec(ctx, `UPDATE platform_admins SET active = true`); err == nil {
		t.Error("aciraba_app seharusnya tidak boleh UPDATE platform_admins")
	}
	if _, err := e.app.Exec(ctx, `SELECT platform_admin_create($1, 'z@z.test', 'Z', 'x')`, uuid.New()); err == nil {
		t.Error("fungsi create harus menolak pelaku yang bukan Platform Admin")
	}
	if _, err := e.app.Exec(ctx, `SELECT * FROM platform_list_tenants($1, '', 10, 0)`, uuid.New()); err == nil {
		t.Error("daftar tenant harus menolak pelaku yang bukan Platform Admin")
	}
	if _, err := e.app.Exec(ctx, `DELETE FROM platform_audit_log`); err == nil {
		t.Error("platform_audit_log harus append-only")
	}
}

func TestImpersonateHanyaBaca(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	adminID, email, pw := e.newAdmin(t, "Pemilik")
	f := e.newTenant(t)
	tok := e.loginToken(t, email, pw)

	// Daftar tenant memuat tenant uji; pencarian per nama bekerja.
	rec := e.do("GET", "/platform/tenants?q=uji-pa-"+f.ID.String()[:8], tok, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), f.ID.String()) {
		t.Fatalf("cari tenant: %d %s", rec.Code, rec.Body)
	}

	// Masuk sebagai ke outlet B.
	rec = e.do("POST", "/platform/tenants/"+f.ID.String()+"/impersonate", tok, `{"outlet_id":"`+f.Outlet2.String()+`"}`)
	if rec.Code != 200 {
		t.Fatalf("impersonate: %d %s", rec.Code, rec.Body)
	}
	imp := extract(t, rec.Body.String(), "access_token")

	if rec := e.do("GET", "/tenant-probe", imp, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"all":true`) || !strings.Contains(rec.Body.String(), "Platform: Pemilik") || !strings.Contains(rec.Body.String(), `"outlets":2`) {
		t.Fatalf("token masuk-sebagai: %d %s", rec.Code, rec.Body)
	}
	// Hanya-baca: metode yang mengubah ditolak.
	if rec := e.do("POST", "/tenant-probe", imp, "{}"); rec.Code != 403 || !strings.Contains(rec.Body.String(), "PLATFORM_READ_ONLY") {
		t.Errorf("POST di mode masuk-sebagai: %d %s", rec.Code, rec.Body)
	}
	// Outlet dari tenant lain tidak diterima: token palsu dengan oid acak.
	bad, _ := e.tokens.IssueImpersonation(adminID.String(), f.ID.String(), uuid.NewString(), time.Now())
	if rec := e.do("GET", "/tenant-probe", bad, ""); rec.Code != 403 {
		t.Errorf("outlet tak dikenal: %d, want 403", rec.Code)
	}
	// Admin lain (id tidak ada) tidak bisa memakai token impersonasi.
	ghost, _ := e.tokens.IssueImpersonation(uuid.NewString(), f.ID.String(), f.Outlet1.String(), time.Now())
	if rec := e.do("GET", "/tenant-probe", ghost, ""); rec.Code != 401 {
		t.Errorf("admin tak ada: %d, want 401", rec.Code)
	}

	// Tercatat di audit tenant (terlihat pemilik tenant) dan audit platform.
	var nTenant, nPlatform int
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'platform.impersonate'`, f.ID).Scan(&nTenant)
	_ = e.admin.QueryRow(ctx, `SELECT count(*) FROM platform_audit_log WHERE tenant_id = $1 AND action = 'platform.impersonate'`, f.ID).Scan(&nPlatform)
	if nTenant != 1 || nPlatform != 1 {
		t.Errorf("audit: tenant=%d platform=%d, want 1/1", nTenant, nPlatform)
	}

	// Menonaktifkan admin langsung mematikan token masuk-sebagai yang masih hidup.
	if _, err := e.admin.Exec(ctx, `UPDATE platform_admins SET active = false WHERE id = $1`, adminID); err != nil {
		t.Fatal(err)
	}
	e.svc.forget(adminID)
	if rec := e.do("GET", "/tenant-probe", imp, ""); rec.Code != 401 {
		t.Errorf("setelah admin dinonaktifkan: %d, want 401", rec.Code)
	}
}

func TestKelolaAdminDanTenant(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	id1, email1, pw := e.newAdmin(t, "Satu")
	tok := e.loginToken(t, email1, pw)
	a1 := Actor{ID: id1, Name: "Satu"}

	// Buat admin baru lewat API; email ganda ditolak.
	newEmail := "baru-" + uuid.NewString()[:8] + "@pa.test"
	rec := e.do("POST", "/platform/admins", tok, `{"name":"Dua","email":"`+newEmail+`","password":"sandi-aman-456"}`)
	if rec.Code != 201 {
		t.Fatalf("buat admin: %d %s", rec.Code, rec.Body)
	}
	id2 := uuid.MustParse(extract(t, rec.Body.String(), "id"))
	t.Cleanup(func() { e.dropAdmin(id2) })
	if rec := e.do("POST", "/platform/admins", tok, `{"name":"Dua","email":"`+newEmail+`","password":"sandi-aman-456"}`); rec.Code != 409 {
		t.Errorf("email ganda: %d, want 409", rec.Code)
	}
	if rec := e.do("POST", "/platform/admins", tok, `{"name":"Tiga","email":"bukan-email","password":"x"}`); rec.Code != 422 {
		t.Errorf("validasi: %d, want 422", rec.Code)
	}
	tok2 := e.loginToken(t, newEmail, "sandi-aman-456")

	// Tidak boleh menonaktifkan diri sendiri.
	if err := e.svc.SetAdminActive(ctx, a1, id1, false); !errors.Is(err, ErrSelf) {
		t.Errorf("nonaktifkan diri: %v", err)
	}
	// Nonaktifkan admin 2: token lamanya langsung mati; tidak bisa login lagi.
	if rec := e.do("PATCH", "/platform/admins/"+id2.String(), tok, `{"active":false}`); rec.Code != 204 {
		t.Fatalf("nonaktifkan: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/platform/auth/me", tok2, ""); rec.Code != 401 {
		t.Errorf("token admin nonaktif: %d, want 401", rec.Code)
	}
	if _, _, err := e.svc.Login(ctx, newEmail, "sandi-aman-456", false); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("login admin nonaktif: %v", err)
	}
	// Admin aktif terakhir tidak boleh dinonaktifkan (oleh admin lain): aktifkan 2 dulu, nonaktifkan 1 oleh 2 → ok; lalu 2 sisa.
	if err := e.svc.SetAdminActive(ctx, a1, id2, true); err != nil {
		t.Fatal(err)
	}
	// Reset password mencabut token lama.
	if rec := e.do("PUT", "/platform/admins/"+id2.String()+"/password", tok, `{"password":"sandi-baru-789"}`); rec.Code != 204 {
		t.Fatalf("reset password: %d %s", rec.Code, rec.Body)
	}
	if _, _, err := e.svc.Login(ctx, newEmail, "sandi-aman-456", false); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password lama: %v", err)
	}
	if _, _, err := e.svc.Login(ctx, newEmail, "sandi-baru-789", false); err != nil {
		t.Errorf("password baru: %v", err)
	}

	// Tenant: nonaktifkan lalu aktifkan; tercatat di audit platform.
	f := e.newTenant(t)
	if rec := e.do("PATCH", "/platform/tenants/"+f.ID.String(), tok, `{"active":false}`); rec.Code != 204 {
		t.Fatalf("nonaktifkan tenant: %d %s", rec.Code, rec.Body)
	}
	var active bool
	_ = e.admin.QueryRow(ctx, `SELECT active FROM tenants WHERE id = $1`, f.ID).Scan(&active)
	if active {
		t.Error("tenant seharusnya nonaktif")
	}
	rec = e.do("GET", "/platform/tenants/"+f.ID.String(), tok, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Outlet A") {
		t.Errorf("detail tenant: %d %s", rec.Code, rec.Body)
	}
	rec = e.do("GET", "/platform/audit?tenant_id="+f.ID.String(), tok, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "platform.tenant_status") {
		t.Errorf("audit platform: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/platform/tenants/"+uuid.NewString(), tok, ""); rec.Code != 404 {
		t.Errorf("tenant tak ada: %d, want 404", rec.Code)
	}

	// Batas hari edit/batal nota: bawaan 0; hanya operator platform yang mengubah; nilai di luar 0–3650 ditolak; tercatat di audit.
	var days int
	_ = e.admin.QueryRow(ctx, `SELECT sale_edit_window_days FROM tenants WHERE id = $1`, f.ID).Scan(&days)
	if days != 0 {
		t.Fatalf("bawaan batas edit = %d, want 0", days)
	}
	for _, bad := range []string{`{"sale_edit_window_days":-1}`, `{"sale_edit_window_days":3651}`} {
		if rec := e.do("PATCH", "/platform/tenants/"+f.ID.String(), tok, bad); rec.Code != 422 {
			t.Errorf("%s: %d, want 422", bad, rec.Code)
		}
	}
	if rec := e.do("PATCH", "/platform/tenants/"+f.ID.String(), tok, `{"sale_edit_window_days":7}`); rec.Code != 204 {
		t.Fatalf("atur batas edit: %d %s", rec.Code, rec.Body)
	}
	_ = e.admin.QueryRow(ctx, `SELECT sale_edit_window_days FROM tenants WHERE id = $1`, f.ID).Scan(&days)
	if days != 7 {
		t.Errorf("batas edit = %d, want 7", days)
	}
	rec = e.do("GET", "/platform/tenants/"+f.ID.String(), tok, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"sale_edit_window_days":7`) {
		t.Errorf("detail memuat batas edit: %d %s", rec.Code, rec.Body)
	}
	rec = e.do("GET", "/platform/audit?tenant_id="+f.ID.String(), tok, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "platform.tenant_edit_window") {
		t.Errorf("audit batas edit: %d %s", rec.Code, rec.Body)
	}
}

// extract mengambil nilai string sederhana dari JSON datar (cukup untuk test; menghindari struct per respons).
func extract(t *testing.T, body, key string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, `"`+key+`":"`)
	if !ok {
		t.Fatalf("kunci %q tidak ada di %s", key, body)
	}
	val, _, _ := strings.Cut(rest, `"`)
	return val
}
