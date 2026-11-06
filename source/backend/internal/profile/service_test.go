package profile

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/storage"
)

type fx struct {
	svc    *Service
	admin  *pgxpool.Pool
	a1, a2 authz.Actor // dua tenant berbeda
}

func setup(t *testing.T) *fx {
	t.Helper()
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
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var sessions *pauth.Sessions
	if u := os.Getenv("TEST_REDIS_URL"); u != "" {
		if opt, err := redis.ParseURL(u); err == nil {
			sessions = pauth.NewSessions(redis.NewClient(opt))
		}
	}
	f := &fx{svc: NewService(app, store, sessions, authz.NewResolver(app)), admin: admin}
	hash, err := pauth.HashPassword("Rahasia12345")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name string) authz.Actor {
		tid, outlet, role, user := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		exec := func(sql string, args ...any) {
			t.Helper()
			if _, err := admin.Exec(ctx, sql, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec(`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-PROFIL')`, tid, "pf-"+tid.String()[:8])
		exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, outlet, tid)
		exec(`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'Kasir')`, role, tid)
		exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, $6)`,
			user, tid, role, user.String()+"@uji.test", name, hash)
		t.Cleanup(func() {
			for _, tbl := range []string{"audit_log", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		})
		return authz.Actor{TenantID: tid, UserID: user, OutletID: outlet, Name: name, Outlets: map[uuid.UUID]bool{outlet: true}}
	}
	f.a1, f.a2 = mk("Budi"), mk("Siti")
	return f
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for x := 0; x < 64; x++ {
		for y := 0; y < 32; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 4), G: 120, B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestUpdateAndCompleteness(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	p, err := f.svc.Get(ctx, f.a1)
	if err != nil {
		t.Fatal(err)
	}
	base := p.Completeness
	if base >= 100 || len(p.Missing) == 0 || p.Name != "Budi" || p.Role != "Kasir" || p.OutletCount != 0 && p.OutletCount != 1 {
		t.Fatalf("profil awal tidak wajar: %+v", p)
	}

	p, err = f.svc.Update(ctx, f.a1, UpdateInput{Name: "  Budi  Santoso ", Phone: "0812-3456-7890", JobTitle: "Kepala Toko", Location: "Bandung", Bio: "Halo\ndunia"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Budi Santoso" || p.Phone != "+6281234567890" || p.JobTitle != "Kepala Toko" || p.Bio != "Halo\ndunia" {
		t.Fatalf("hasil update: %+v", p)
	}
	if p.Completeness <= base {
		t.Errorf("kelengkapan harus naik: %d → %d", base, p.Completeness)
	}

	// Mengosongkan isian opsional menghapusnya; tenant lain tidak terpengaruh.
	p, err = f.svc.Update(ctx, f.a1, UpdateInput{Name: "Budi Santoso"})
	if err != nil || p.Phone != "" || p.Bio != "" {
		t.Fatalf("kosongkan: %+v %v", p, err)
	}
	if o, _ := f.svc.Get(ctx, f.a2); o.Name != "Siti" {
		t.Errorf("tenant lain berubah: %s", o.Name)
	}

	var fe FieldErrors
	_, err = f.svc.Update(ctx, f.a1, UpdateInput{Name: "<b>x</b>", Phone: "abc", Bio: string(make([]rune, 0)) + string(bytes.Repeat([]byte("a"), 1001))})
	if !errors.As(err, &fe) || fe["name"] == "" || fe["phone"] == "" || fe["bio"] != "TOO_LONG" {
		t.Fatalf("validasi: %v", err)
	}

	// Jejak aktivitas: perubahan profil tadi tercatat, hanya milik pengguna ini, dan keyset berjalan.
	pg, err := f.svc.ActivityList(ctx, f.a1, 1, "")
	if err != nil || len(pg.Data) != 1 || pg.Data[0].Action != "user.profile" || pg.Next == "" {
		t.Fatalf("aktivitas: %+v %v", pg, err)
	}
	pg2, err := f.svc.ActivityList(ctx, f.a1, 1, pg.Next)
	if err != nil || len(pg2.Data) != 1 || pg2.Data[0].ID >= pg.Data[0].ID {
		t.Fatalf("halaman 2: %+v %v", pg2, err)
	}
	if other, _ := f.svc.ActivityList(ctx, f.a2, 10, ""); len(other.Data) != 0 {
		t.Errorf("aktivitas pengguna lain bocor: %+v", other.Data)
	}
	if _, err := f.svc.ActivityList(ctx, f.a1, 5, "ngawur"); err == nil {
		t.Error("kursor rusak harus ditolak")
	}

	imp := f.a1
	imp.Impersonator = uuid.New()
	if _, err := f.svc.Update(ctx, imp, UpdateInput{Name: "Hacker"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("impersonasi harus ditolak: %v", err)
	}
}

func TestImages(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	p, err := f.svc.SetImage(ctx, f.a1, Avatar, bytes.NewReader(pngBytes(t)))
	if err != nil || p.AvatarID == nil || p.BannerID != nil {
		t.Fatalf("set avatar: %+v %v", p, err)
	}
	first := *p.AvatarID
	p, err = f.svc.SetImage(ctx, f.a1, Banner, bytes.NewReader(pngBytes(t)))
	if err != nil || p.BannerID == nil {
		t.Fatalf("set banner: %+v %v", p, err)
	}
	// Mengganti foto membuat id baru dan membersihkan berkas lama.
	p, err = f.svc.SetImage(ctx, f.a1, Avatar, bytes.NewReader(pngBytes(t)))
	if err != nil || *p.AvatarID == first {
		t.Fatalf("ganti avatar: %+v %v", p, err)
	}
	if _, err := f.svc.store.Open(ctx, fullKey(f.a1.TenantID, uuid.MustParse(first))); err == nil {
		t.Error("berkas lama seharusnya sudah dihapus")
	}

	r, id, err := f.svc.OpenImage(ctx, f.a1, f.a1.UserID, Avatar, true)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if len(b) == 0 || id != *p.AvatarID {
		t.Errorf("thumbnail kosong atau id salah (%s)", id)
	}

	// Isolasi tenant: tenant lain tidak bisa membuka gambar pengguna ini.
	if _, _, err := f.svc.OpenImage(ctx, f.a2, f.a1.UserID, Avatar, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("lintas tenant harus NotFound: %v", err)
	}
	if _, err := f.svc.SetImage(ctx, f.a1, Avatar, bytes.NewReader([]byte("bukan gambar"))); err == nil {
		t.Error("berkas non-gambar harus ditolak")
	}

	p, err = f.svc.RemoveImage(ctx, f.a1, Avatar)
	if err != nil || p.AvatarID != nil || p.BannerID == nil {
		t.Fatalf("hapus avatar: %+v %v", p, err)
	}
	if _, _, err := f.svc.OpenImage(ctx, f.a1, f.a1.UserID, Avatar, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("setelah dihapus harus NotFound: %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	if err := f.svc.ChangePassword(ctx, f.a1, "salah-salah1", "BaruBanget123"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("password lama salah: %v", err)
	}
	var fe FieldErrors
	if err := f.svc.ChangePassword(ctx, f.a1, "Rahasia12345", "pendek"); !errors.As(err, &fe) || fe["password"] == "" {
		t.Fatalf("password lemah: %v", err)
	}
	if err := f.svc.ChangePassword(ctx, f.a1, "Rahasia12345", "Rahasia12345"); !errors.As(err, &fe) || fe["password"] != "SAME_AS_CURRENT" {
		t.Fatalf("password sama: %v", err)
	}
	if f.svc.sessions == nil {
		t.Skip("TEST_REDIS_URL tidak di-set: bagian pencabutan sesi dilewati")
	}
	if err := f.svc.ChangePassword(ctx, f.a1, "Rahasia12345", "BaruBanget123"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ChangePassword(ctx, f.a1, "Rahasia12345", "LainLagi12345"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("password lama sudah tidak berlaku: %v", err)
	}
}
