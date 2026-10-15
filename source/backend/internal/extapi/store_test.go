package extapi

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integrasi Store (fungsi SECURITY DEFINER): kredensial, rotasi, white label, dan pagar akses tabel.
// Butuh TEST_DATABASE_URL + TEST_ADMIN_DATABASE_URL (jika tidak ada: SKIP).
func TestStoreCredentialsAndBrands(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
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

	adminID, tenant, other, roleID, userID, outlet := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	sfx := tenant.String()[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO platform_admins (id, email, name, password_hash) VALUES ($1, $2, 'Uji', 'x')`, []any{adminID, "uji-" + sfx + "@ex.test"}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{tenant, "st-" + sfx, "UJI-ST-" + sfx}},
		{`INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`, []any{other, "so-" + sfx, "UJI-SO-" + sfx}},
		{`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'a', 'A')`, []any{outlet, tenant}},
		{`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, 'R', '{}')`, []any{roleID, tenant}},
		{`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Svc', 'x')`, []any{userID, tenant, roleID, "st-" + sfx + "@ex.test"}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("siapkan: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DELETE FROM api_credentials WHERE tenant_id = ANY($1)`, []uuid.UUID{tenant, other})
		_, _ = admin.Exec(ctx, `DELETE FROM api_doc_brandings WHERE slug LIKE 'uji-%'`)
		for _, tbl := range []string{"users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = ANY($1)`, []uuid.UUID{tenant, other})
		_, _ = admin.Exec(ctx, `DELETE FROM platform_admins WHERE id = $1`, adminID)
	})

	st, err := NewStore(app, "rahasia-uji-rahasia-uji-rahasia-uji-123")
	if err != nil {
		t.Fatal(err)
	}

	// pagar: aplikasi tidak boleh membaca tabel secara langsung; bukan-admin ditolak
	var pe *pgconn.PgError
	if _, err := app.Exec(ctx, `SELECT secret_enc FROM api_credentials`); !errors.As(err, &pe) || pe.Code != "42501" {
		t.Errorf("aciraba_app harus ditolak membaca api_credentials, got %v", err)
	}
	if _, err := st.ListCredentials(ctx, uuid.New(), nil); !errors.As(err, &pe) || pe.Code != "42501" {
		t.Errorf("bukan Platform Admin harus ditolak, got %v", err)
	}

	// validasi sebelum menyentuh DB
	in := CredentialInput{TenantID: tenant, UserID: userID, OutletID: outlet, Label: "Mitra", IPAllowlist: []string{"203.0.113.7", "198.51.100.0/24"}, RateLimit: 60}
	bad := in
	bad.IPAllowlist = nil
	if _, err := st.CreateCredential(ctx, adminID, bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("IP wajib: %v", err)
	}
	bad = in
	bad.UserID = uuid.New() // bukan milik tenant
	if _, err := st.CreateCredential(ctx, adminID, bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("akun layanan harus milik tenant: %v", err)
	}
	bad = in
	bad.TenantID = other // user/outlet milik tenant lain
	if _, err := st.CreateCredential(ctx, adminID, bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("lintas tenant harus ditolak: %v", err)
	}

	// buat, daftar, lookup, ubah, rotasi, hapus
	created, err := st.CreateCredential(ctx, adminID, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.KeyID) < 10 || len(created.Secret) < 40 || created.KeyID[:3] != "ak_" || created.Secret[:3] != "sk_" {
		t.Fatalf("format kredensial: %+v", created)
	}
	var stored string
	if err := admin.QueryRow(ctx, `SELECT secret_enc FROM api_credentials WHERE id = $1`, created.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == created.Secret || len(stored) == 0 {
		t.Fatal("secret tidak boleh tersimpan polos")
	}
	if plain, err := st.box.open(stored, created.KeyID); err != nil || plain != created.Secret {
		t.Fatalf("dekripsi: %v", err)
	}
	if _, err := st.box.open(stored, "ak_lain"); err == nil {
		t.Fatal("ciphertext harus terikat ke key_id (AAD)")
	}

	list, err := st.ListCredentials(ctx, adminID, &tenant)
	if err != nil || len(list) != 1 || list[0].Label != "Mitra" || len(list[0].IPAllowlist) != 2 || list[0].Rotating {
		t.Fatalf("daftar: %v %+v", err, list)
	}
	l, err := st.lookup(ctx, created.KeyID)
	if err != nil || !l.Active || l.TenantID != tenant || l.UserID != userID || l.OutletID != outlet || l.RateLimit != 60 {
		t.Fatalf("lookup: %v %+v", err, l)
	}
	if _, err := st.lookup(ctx, "ak_tidak_ada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("lookup tak dikenal: %v", err)
	}

	exp := time.Now().Add(24 * time.Hour)
	if err := st.UpdateCredential(ctx, adminID, created.ID, CredentialUpdate{Label: "Mitra 2", IPAllowlist: []string{"10.0.0.0/8"}, RateLimit: 5, Active: false, ExpiresAt: &exp}); err != nil {
		t.Fatal(err)
	}
	if l, _ = st.lookup(ctx, created.KeyID); l.Active || l.RateLimit != 5 || l.ExpiresAt == nil || l.IPAllowlist[0] != "10.0.0.0/8" {
		t.Errorf("setelah ubah: %+v", l)
	}
	if err := st.UpdateCredential(ctx, adminID, uuid.New(), CredentialUpdate{Label: "x", IPAllowlist: []string{"10.0.0.1"}, RateLimit: 1, Active: true}); !errors.Is(err, ErrNotFound) {
		t.Errorf("ubah tak ada: %v", err)
	}

	// rotasi dengan transisi → secret lama disimpan sebagai prev; tanpa transisi → prev dihapus
	rot, err := st.RotateCredential(ctx, adminID, created.ID, created.KeyID, time.Hour)
	if err != nil || rot.Secret == created.Secret {
		t.Fatalf("rotasi: %v", err)
	}
	if l, _ = st.lookup(ctx, created.KeyID); l.PrevSecretEnc == nil || l.PrevExpiresAt == nil {
		t.Error("masa transisi harus menyimpan secret lama")
	}
	if list, _ = st.ListCredentials(ctx, adminID, &tenant); !list[0].Rotating {
		t.Error("status rotating harus tampil")
	}
	if _, err := st.RotateCredential(ctx, adminID, created.ID, created.KeyID, 0); err != nil {
		t.Fatal(err)
	}
	if l, _ = st.lookup(ctx, created.KeyID); l.PrevSecretEnc != nil {
		t.Error("rotasi tanpa transisi harus mematikan secret lama")
	}

	actors, err := st.TenantActors(ctx, adminID, tenant)
	if err != nil || len(actors) < 20 {
		t.Errorf("pilihan akun layanan: %v %s", err, actors)
	}

	// akun layanan dinonaktifkan → lookup.active=false
	if err := st.UpdateCredential(ctx, adminID, created.ID, CredentialUpdate{Label: "Mitra 2", IPAllowlist: []string{"10.0.0.1"}, RateLimit: 5, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE users SET active = false WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if l, _ = st.lookup(ctx, created.KeyID); l.Active {
		t.Error("akun layanan nonaktif harus mematikan kredensial")
	}

	if err := st.DeleteCredential(ctx, adminID, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCredential(ctx, adminID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("hapus dua kali: %v", err)
	}

	// ---- white label ----
	b := BrandInput{Slug: "uji-" + sfx, Domain: "docs-" + sfx + ".uji.test", Active: true, Name: "Mitra API", Accent: "#ff5500", BaseURL: "https://api.uji.test", HiddenModules: []string{"ai", "accounting"}}
	if f := b.Validate(); len(f) != 0 {
		t.Fatal(f)
	}
	id, err := st.SaveBrand(ctx, adminID, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveBrand(ctx, adminID, b); !errors.Is(err, ErrTaken) {
		t.Errorf("slug/domain ganda: %v", err)
	}
	pub, err := st.ResolveBrand(ctx, "DOCS-"+sfx+".uji.test")
	if err != nil || pub.Name != "Mitra API" || pub.Accent != "#ff5500" || len(pub.HiddenModules) != 2 {
		t.Fatalf("resolve per domain: %v %+v", err, pub)
	}
	if def, err := st.ResolveBrand(ctx, "tidak-dikenal.example"); err != nil || def.Slug != "default" {
		t.Errorf("fallback ke profil bawaan: %v %+v", err, def)
	}
	b.ID, b.Active = &id, false
	if _, err := st.SaveBrand(ctx, adminID, b); err != nil {
		t.Fatal(err)
	}
	if pub, _ = st.ResolveBrand(ctx, "docs-"+sfx+".uji.test"); pub.Slug == b.Slug {
		t.Error("profil nonaktif tidak boleh dipakai")
	}
	brands, err := st.ListBrands(ctx, adminID)
	if err != nil || len(brands) < 2 || !brands[0].IsDefault {
		t.Errorf("daftar profil: %v", err)
	}
	var defID uuid.UUID
	for _, x := range brands {
		if x.IsDefault {
			defID = x.ID
		}
	}
	if err := st.DeleteBrand(ctx, adminID, defID); !errors.Is(err, ErrDefault) {
		t.Errorf("profil bawaan tak boleh dihapus: %v", err)
	}
	if err := st.DeleteBrand(ctx, adminID, id); err != nil {
		t.Fatal(err)
	}
}
