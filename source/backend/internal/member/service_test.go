package member

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

type fx struct {
	svc    *Service
	app    *pgxpool.Pool
	admin  *pgxpool.Pool
	t1, t2 uuid.UUID
	a1, a2 authz.Actor
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
	f := &fx{svc: NewService(app, nil), app: app, admin: admin, t1: uuid.New(), t2: uuid.New()}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	mkActor := func(tid uuid.UUID) authz.Actor {
		outlet, role, user := uuid.New(), uuid.New(), uuid.New()
		exec(`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-MEMBER')`, tid, "mb-"+tid.String()[:8])
		exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, outlet, tid)
		exec(`INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, 'r')`, role, tid)
		exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir', 'x')`, user, tid, role, user.String()+"@uji.test")
		// Migration mengisi level bawaan hanya untuk tenant yang ada saat itu; tenant uji mengisinya seperti Register.
		if err := db.WithTenant(ctx, app, tid, func(tx pgx.Tx) error { return SeedDefaultLevels(ctx, tx, tid) }); err != nil {
			t.Fatal(err)
		}
		return authz.Actor{TenantID: tid, UserID: user, OutletID: outlet, Name: "Kasir", Outlets: map[uuid.UUID]bool{outlet: true}}
	}
	f.a1, f.a2 = mkActor(f.t1), mkActor(f.t2)
	t.Cleanup(func() {
		for _, tid := range []uuid.UUID{f.t1, f.t2} {
			for _, tbl := range []string{"audit_log", "member_point_movements", "members", "member_counters", "member_levels", "users", "roles", "outlets"} {
				_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, tid)
			}
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
		}
	})
	return f
}

func in(name string) Input { return Input{Name: name} }

func TestPointsFor(t *testing.T) {
	for _, c := range []struct {
		base, spend string
		want        int
	}{
		{"99999", "10000", 9}, {"100000", "10000", 10}, {"9999.99", "10000", 0}, {"250000", "0", 0}, {"0", "10000", 0}, {"15000", "5000", 3},
	} {
		if got := PointsFor(decimal.RequireFromString(c.base), decimal.RequireFromString(c.spend)); got != c.want {
			t.Errorf("PointsFor(%s,%s) = %d, want %d", c.base, c.spend, got, c.want)
		}
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]Input{
		"name":         {Name: "  ", Active: ptr(true)},
		"phone":        {Name: "A", Phone: "abc", Active: ptr(true)},
		"email":        {Name: "A", Email: "bukan-email", Active: ptr(true)},
		"gender":       {Name: "A", Gender: "X", Active: ptr(true)},
		"credit_limit": {Name: "A", CreditLimit: "-1", Active: ptr(true)},
		"due_days":     {Name: "A", DueDays: "99999", Active: ptr(true)},
		"valid_until":  {Name: "A", ValidUntil: "31-12-2030", Active: ptr(true)},
		"address":      {Name: "A", Address: "<script>", Active: ptr(true)},
		"code":         {Name: "A", Code: "ada spasi", Active: ptr(true)},
		"postal_code":  {Name: "A", PostalCode: "<<<", Active: ptr(true)},
	}
	for field, input := range cases {
		if _, f := validate(input, true); f[field] == "" {
			t.Errorf("%s: galat tidak terdeteksi (f=%v)", field, f)
		}
	}
	c, f := validate(Input{Name: "Budi", Phone: "0812-3456-7890", Email: "Budi@Contoh.COM", CreditLimit: "1500000", DueDays: "14", ValidUntil: "2030-12-31", Active: ptr(true)}, true)
	if f != nil || c.phone != "+6281234567890" || c.email != "budi@contoh.com" || c.dueDays != 14 || !c.validUntil.Valid {
		t.Errorf("input sah ditolak/dinormalkan salah: %+v %v", c, f)
	}
}

func TestMemberLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, err := f.svc.Create(ctx, f.a1, Input{Name: "Budi Santoso", Phone: "081234567890", Active: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if m.Code != "MBR-000001" || m.Level.Name != "Reguler" || m.Points != 0 || m.Stats.Deposit != "0.00" {
		t.Fatalf("member baru: %+v", m)
	}
	m2, _ := f.svc.Create(ctx, f.a1, in("Siti"))
	if m2.Code != "MBR-000002" {
		t.Errorf("kode otomatis kedua = %s", m2.Code)
	}
	// Kode manual yang bentrok dengan nomor otomatis berikutnya dilewati.
	if _, err := f.svc.Create(ctx, f.a1, Input{Name: "Manual", Code: "MBR-000003", Active: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	m4, _ := f.svc.Create(ctx, f.a1, in("Otomatis"))
	if m4.Code != "MBR-000004" {
		t.Errorf("kode otomatis setelah kode manual = %s, want MBR-000004", m4.Code)
	}
	if _, err := f.svc.Create(ctx, f.a1, Input{Name: "Dobel", Code: "mbr-000001", Active: ptr(true)}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("kode ganda (beda huruf besar): err = %v", err)
	}

	upd := Input{Code: m.Code, Name: "Budi S.", Phone: "081234567890", City: "Bandung", CreditLimit: "500000", DueDays: "7", Active: ptr(true)}
	got, err := f.svc.Update(ctx, f.a1, m.ID, upd)
	if err != nil || got.Name != "Budi S." || got.City != "Bandung" || got.CreditLimit != "500000.00" || got.DueDays != 7 {
		t.Fatalf("update: %+v err=%v", got, err)
	}
	if got, err = f.svc.SetActive(ctx, f.a1, m.ID, false); err != nil || got.Active {
		t.Fatalf("nonaktif: %+v err=%v", got, err)
	}
	if rows, total, _ := f.svc.List(ctx, f.a1, ListParams{Q: "budi"}); total != 1 || len(rows) != 1 {
		t.Errorf("cari: total=%d rows=%d", total, len(rows))
	}
	if rows, _, _ := f.svc.List(ctx, f.a1, ListParams{Q: "%"}); len(rows) != 0 {
		t.Errorf("wildcard %% harus dicocokkan apa adanya, dapat %d baris", len(rows))
	}
	// Member nonaktif tidak muncul di pencarian kasir.
	if rows, _ := f.svc.Lookup(ctx, f.a1, "budi"); len(rows) != 0 {
		t.Errorf("lookup kasir memuat member nonaktif: %v", rows)
	}
	if rows, _ := f.svc.Lookup(ctx, f.a1, "Siti"); len(rows) != 1 {
		t.Errorf("lookup member aktif: %v", rows)
	}
	// Isolasi tenant.
	if _, err := f.svc.Get(ctx, f.a2, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain bisa membaca member: err = %v", err)
	}
	if _, err := f.svc.Update(ctx, f.a2, m.ID, upd); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain bisa mengubah member: err = %v", err)
	}
}

func TestLevelsByLifetimePoints(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, _ := f.svc.Create(ctx, f.a1, in("Dewi"))
	got, err := f.svc.Adjust(ctx, f.a1, m.ID, 150, "Saldo awal poin")
	if err != nil || got.Level.Name != "Silver" || got.Points != 150 || got.LifetimePoints != 150 {
		t.Fatalf("setelah +150: %+v err=%v", got, err)
	}
	if got.NextLevel == nil || got.NextLevel.Name != "Gold" || got.NextLevel.PointsNeeded != 350 {
		t.Errorf("level berikutnya: %+v", got.NextLevel)
	}
	// Mengurangi poin tidak menurunkan level (lifetime tidak berkurang).
	if got, err = f.svc.Adjust(ctx, f.a1, m.ID, -140, "Koreksi"); err != nil || got.Points != 10 || got.Level.Name != "Silver" {
		t.Fatalf("setelah -140: %+v err=%v", got, err)
	}
	// Tidak boleh minus + alasan wajib.
	if _, err = f.svc.Adjust(ctx, f.a1, m.ID, -11, "Terlalu banyak"); !isField(err, "points", "POINTS_INSUFFICIENT") {
		t.Errorf("saldo minus: err = %v", err)
	}
	if _, err = f.svc.Adjust(ctx, f.a1, m.ID, 5, ""); !isField(err, "note", "REQUIRED") {
		t.Errorf("alasan kosong: err = %v", err)
	}
	if _, err = f.svc.Adjust(ctx, f.a1, m.ID, 0, "nol"); !isField(err, "points", "INVALID") {
		t.Errorf("poin nol: err = %v", err)
	}

	// Aturan level dasar.
	levels, _ := f.svc.Levels(ctx, f.a1, nil)
	if len(levels) != 4 || levels[0].Name != "Reguler" || levels[0].MemberCount != 0 || levels[1].MemberCount != 1 {
		t.Fatalf("daftar level: %+v", levels)
	}
	base := levels[0]
	if _, err = f.svc.SetLevelActive(ctx, f.a1, base.ID, false); !isField(err, "active", "BASE_LEVEL") {
		t.Errorf("arsip level dasar: err = %v", err)
	}
	if _, err = f.svc.UpdateLevel(ctx, f.a1, base.ID, LevelInput{Name: "Reguler", MinPoints: "5", SpendPerPoint: "10000", PointValue: "100"}); !isField(err, "min_points", "BASE_LEVEL") {
		t.Errorf("ubah ambang level dasar: err = %v", err)
	}
	if _, err = f.svc.CreateLevel(ctx, f.a1, LevelInput{Name: "Dobel", MinPoints: "100", SpendPerPoint: "1", PointValue: "1"}); !isField(err, "min_points", "DUPLICATE") {
		t.Errorf("ambang ganda: err = %v", err)
	}
	if _, err = f.svc.CreateLevel(ctx, f.a1, LevelInput{Name: "silver", MinPoints: "700", SpendPerPoint: "1", PointValue: "1"}); !isField(err, "name", "DUPLICATE") {
		t.Errorf("nama ganda: err = %v", err)
	}
	// Mengarsipkan Silver → member jatuh ke level aktif tertinggi di bawahnya (Reguler).
	silver := levels[1]
	if _, err = f.svc.SetLevelActive(ctx, f.a1, silver.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Get(ctx, f.a1, m.ID); got.Level.Name != "Reguler" {
		t.Errorf("setelah Silver diarsipkan level = %s", got.Level.Name)
	}
}

func isField(err error, field, code string) bool {
	var fe FieldErrors
	return errors.As(err, &fe) && fe[field] == code
}

// sale menjalankan satu "nota" poin di transaksi sendiri (seperti sales.Create): kunci member, tukar, lalu peroleh.
func (f *fx) sale(ctx context.Context, memberID, saleID uuid.UUID, redeem, earn int) error {
	return db.WithTenant(ctx, f.app, f.a1.TenantID, func(tx pgx.Tx) error {
		sm, err := LockForSale(ctx, tx, f.a1.TenantID, memberID, dateNow(), true)
		if err != nil {
			return err
		}
		if redeem > sm.Points {
			return ErrInsufficientPoints
		}
		return ApplySale(ctx, tx, f.a1, memberID, saleID, "T-0001", redeem, earn)
	})
}

func TestSalePointsAndReversal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, _ := f.svc.Create(ctx, f.a1, in("Rina"))
	if _, err := f.svc.Adjust(ctx, f.a1, m.ID, 20, "Awal"); err != nil {
		t.Fatal(err)
	}
	// Nota tidak bisa menukar lebih dari saldo.
	if err := f.sale(ctx, m.ID, uuid.New(), 21, 0); !errors.Is(err, ErrInsufficientPoints) {
		t.Errorf("tukar melebihi saldo: err = %v", err)
	}
	saleID := uuid.New()
	if err := f.sale(ctx, m.ID, saleID, 5, 12); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(ctx, f.a1, m.ID)
	if got.Points != 27 || got.LifetimePoints != 32 { // 20 − 5 + 12 ; lifetime 20 + 12
		t.Fatalf("setelah nota: points=%d lifetime=%d", got.Points, got.LifetimePoints)
	}
	rev := func() error {
		return db.WithTenant(ctx, f.app, f.t1, func(tx pgx.Tx) error { return ReverseSale(ctx, tx, f.a1, saleID, "void") })
	}
	if err := rev(); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Get(ctx, f.a1, m.ID); got.Points != 20 || got.LifetimePoints != 20 {
		t.Fatalf("setelah dibalik: points=%d lifetime=%d (harus kembali 20/20)", got.Points, got.LifetimePoints)
	}
	// Idempoten: membalik lagi tidak mengubah apa pun (bukan bug 'poin berlipat' legacy).
	if err := rev(); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Get(ctx, f.a1, m.ID); got.Points != 20 || got.LifetimePoints != 20 {
		t.Fatalf("pembalikan ganda mengubah saldo: %d/%d", got.Points, got.LifetimePoints)
	}

	// Poin hasil nota sudah terpakai → penarikan dibatasi sampai 0, tidak minus.
	s2 := uuid.New()
	if err := f.sale(ctx, m.ID, s2, 0, 10); err != nil {
		t.Fatal(err)
	}
	if err := f.sale(ctx, m.ID, uuid.New(), 30, 0); err != nil { // habiskan saldo (30)
		t.Fatal(err)
	}
	if err := db.WithTenant(ctx, f.app, f.t1, func(tx pgx.Tx) error { return ReverseSale(ctx, tx, f.a1, s2, "void") }); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Get(ctx, f.a1, m.ID); got.Points != 0 || got.LifetimePoints != 20 {
		t.Errorf("pembalikan dengan saldo habis: points=%d lifetime=%d", got.Points, got.LifetimePoints)
	}

	// Ledger: saldo setelah tiap movement konsisten dan tidak bisa diubah/dihapus oleh role aplikasi.
	moves, total, err := f.svc.Points(ctx, f.a1, m.ID, 50, 0)
	if err != nil || total != len(moves) || len(moves) == 0 {
		t.Fatalf("riwayat: %d/%d err=%v", len(moves), total, err)
	}
	if moves[0].BalanceAfter != 0 {
		t.Errorf("saldo setelah movement terbaru = %d, want 0", moves[0].BalanceAfter)
	}
	if _, err := f.app.Exec(ctx, `UPDATE member_point_movements SET points = 999 WHERE tenant_id = $1`, f.t1); err == nil {
		t.Error("UPDATE ledger poin seharusnya ditolak")
	}
	if _, err := f.app.Exec(ctx, `DELETE FROM member_point_movements WHERE tenant_id = $1`, f.t1); err == nil {
		t.Error("DELETE ledger poin seharusnya ditolak")
	}
}

func TestConcurrentRedeem(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	m, _ := f.svc.Create(ctx, f.a1, in("Konkuren"))
	if _, err := f.svc.Adjust(ctx, f.a1, m.ID, 10, "Awal"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f.sale(ctx, m.ID, uuid.New(), 1, 0); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	got, _ := f.svc.Get(ctx, f.a1, m.ID)
	if ok != 10 || got.Points != 0 {
		t.Errorf("25 penukaran serentak dari saldo 10: berhasil=%d saldo=%d (harus 10 dan 0)", ok, got.Points)
	}
}

func dateNow() pgtype.Date { return pgtype.Date{Time: time.Now(), Valid: true} }

func ptr[T any](v T) *T { return &v }
