package stock

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// convActor membuat pengguna sungguhan (dokumen mengacu FK ke users) dan mengembalikan aktor di outlet tertentu.
func (e *env) convActor(t *testing.T, outlet uuid.UUID) authz.Actor {
	t.Helper()
	ctx := context.Background()
	role, user := uuid.New(), uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO roles (id, tenant_id, name) VALUES ($1, $2, $3)`, role, e.tenant, "r-"+role.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, 'Kasir Uji', 'x')`,
		user, e.tenant, role, user.String()+"@uji.test"); err != nil {
		t.Fatal(err)
	}
	return authz.Actor{TenantID: e.tenant, UserID: user, OutletID: outlet, Name: "Kasir Uji", Perms: authz.Permissions{All: true}}
}

func (e *env) setCost(t *testing.T, item uuid.UUID, cost string) {
	t.Helper()
	if _, err := e.admin.Exec(context.Background(), `UPDATE items SET avg_cost = $2, last_cost = $2 WHERE id = $1`, item, cost); err != nil {
		t.Fatal(err)
	}
}

// cost = HPP efektif di outlet e.outlet: HPP cabang bila ada, selain itu HPP awal barang.
func (e *env) cost(t *testing.T, item uuid.UUID) string {
	t.Helper()
	var c string
	if err := e.admin.QueryRow(context.Background(), `SELECT coalesce(oc.avg_cost, i.avg_cost)::text FROM items i
		LEFT JOIN item_outlet_costs oc ON oc.tenant_id = i.tenant_id AND oc.item_id = i.id AND oc.outlet_id = $2 WHERE i.id = $1`, item, e.outlet).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *env) docs(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), `SELECT count(*) FROM stock_conversions WHERE tenant_id = $1`, e.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) seed(t *testing.T, a authz.Actor, svc *Service, item uuid.UUID, qty string) {
	t.Helper()
	if _, err := svc.SetOpening(context.Background(), a, item, BucketDisplay, qty); err != nil {
		t.Fatal(err)
	}
}

func cin(from uuid.UUID, fq string, to uuid.UUID, tq string) ConvertInput {
	return ConvertInput{FromItemID: from, FromQty: fq, ToItemID: to, ToQty: tq}
}

func key() string { return "k-" + uuid.NewString() }

func TestConvert(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() {
		_, _ = e.admin.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, e.tenant)
	})
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	karung, pcs := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, karung, "5")
	e.setCost(t, karung, "62000")

	c, replayed, err := svc.Convert(ctx, a, key(), cin(karung, "1", pcs, "12"))
	if err != nil || replayed {
		t.Fatalf("pecah: %v replay=%v", err, replayed)
	}
	if !strings.HasPrefix(c.DocNo, "PS-MAIN-") || !strings.HasSuffix(c.DocNo, "-0001") {
		t.Errorf("nomor dokumen = %s", c.DocNo)
	}
	if e.balance(t, karung, BucketDisplay) != "4" || e.balance(t, pcs, BucketDisplay) != "12" {
		t.Errorf("saldo karung=%s pcs=%s", e.balance(t, karung, BucketDisplay), e.balance(t, pcs, BucketDisplay))
	}
	// Dua movement UNIT_CONVERSION dengan ref_id dokumen.
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE ref_type='UNIT_CONVERSION' AND ref_id=$1`, c.ID).Scan(&n); err != nil || n != 2 {
		t.Errorf("movement UNIT_CONVERSION = %d %v", n, err)
	}
	// HPP hasil = 62000 × 1 ÷ 12 = 5166.67, dipasang karena HPP tujuan masih kosong.
	if c.ToUnitCost != "5166.67" || !c.CostApplied || e.cost(t, pcs) != "5166.67" {
		t.Errorf("HPP: to=%s applied=%v db=%s", c.ToUnitCost, c.CostApplied, e.cost(t, pcs))
	}
	if c.From.Qty != "1" || c.To.Qty != "12" || c.Actor != "Kasir Uji" {
		t.Errorf("isi dokumen: %+v", c)
	}

	// HPP awal barang (items.avg_cost) tidak berubah: yang terpasang adalah HPP cabang.
	var base string
	if err := e.admin.QueryRow(ctx, `SELECT avg_cost::text FROM items WHERE id = $1`, pcs).Scan(&base); err != nil || base != "0.00" {
		t.Errorf("HPP awal barang berubah: %s %v", base, err)
	}

	// Pecah kedua: tujuan sudah punya stok & HPP cabang → HPP tidak ditimpa; nomor berurutan.
	c2, _, err := svc.Convert(ctx, a, key(), cin(karung, "2", pcs, "24"))
	if err != nil || !strings.HasSuffix(c2.DocNo, "-0002") || c2.CostApplied || e.cost(t, pcs) != "5166.67" {
		t.Fatalf("pecah kedua: %+v %v cost=%s", c2, err, e.cost(t, pcs))
	}
	if e.balance(t, karung, BucketDisplay) != "2" || e.balance(t, pcs, BucketDisplay) != "36" {
		t.Errorf("saldo akhir karung=%s pcs=%s", e.balance(t, karung, BucketDisplay), e.balance(t, pcs, BucketDisplay))
	}
	// Catatan markdown: baris baru dan tab dipertahankan (tidak dipadatkan jadi satu baris).
	md := "**Susut**\n\n- rusak 1\n- sisa 11"
	if cm, _, err := svc.Convert(ctx, a, key(), ConvertInput{FromItemID: karung, FromQty: "1", ToItemID: pcs, ToQty: "11", Note: md}); err != nil || cm.Note != md {
		t.Errorf("catatan markdown: %q %v", cm.Note, err)
	}
	// Susut: 1 karung hanya jadi 11 pcs → qty tujuan bebas.
	if _, _, err := svc.Convert(ctx, a, key(), cin(karung, "1", pcs, "11")); err != nil {
		t.Errorf("susut: %v", err)
	}
	// Kebalikan (gabung): pcs → karung.
	if _, _, err := svc.Convert(ctx, a, key(), cin(pcs, "12", karung, "1")); err != nil {
		t.Errorf("gabung: %v", err)
	}
	if n := e.auditCount(t, "stock.convert"); n != 5 {
		t.Errorf("audit stock.convert = %d", n)
	}
	rows, total, err := svc.ListConversions(ctx, a, 10, 0)
	if err != nil || total != 5 || len(rows) != 5 || rows[0].DocNo <= rows[4].DocNo {
		t.Errorf("daftar: total=%d len=%d %v", total, len(rows), err)
	}
	// Outlet lain: nomor dan stok terpisah.
	a2 := e.convActor(t, e.outlet2)
	e.seed(t, a2, svc, karung, "3")
	c3, _, err := svc.Convert(ctx, a2, key(), cin(karung, "1", pcs, "12"))
	if err != nil || !strings.HasPrefix(c3.DocNo, "PS-CAB2-") || !strings.HasSuffix(c3.DocNo, "-0001") {
		t.Errorf("outlet 2: %+v %v", c3, err)
	}
	if _, total, _ := svc.ListConversions(ctx, a2, 10, 0); total != 1 {
		t.Errorf("daftar outlet 2 = %d", total)
	}
}

func TestConvertValidationAndAtomicity(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "3")
	foreign := e.item(t, e.other, "goods", false)

	for name, c := range map[string]struct {
		in    ConvertInput
		field string
	}{
		"barang sama":     {cin(x, "1", x, "1"), "to_item_id"},
		"qty nol":         {cin(x, "0", y, "1"), "from_qty"},
		"qty negatif":     {cin(x, "1", y, "-1"), "to_qty"},
		"4 desimal":       {cin(x, "1.0001", y, "1"), "from_qty"},
		"bukan angka":     {cin(x, "abc", y, "1"), "from_qty"},
		"qty kosong":      {cin(x, "", y, "1"), "from_qty"},
		"asal kosong":     {cin(uuid.Nil, "1", y, "1"), "from_item_id"},
		"tujuan kosong":   {cin(x, "1", uuid.Nil, "1"), "to_item_id"},
		"catatan kontrol": {ConvertInput{FromItemID: x, FromQty: "1", ToItemID: y, ToQty: "1", Note: "a\x00b"}, "note"},
		"catatan panjang": {ConvertInput{FromItemID: x, FromQty: "1", ToItemID: y, ToQty: "1", Note: strings.Repeat("a", 1001)}, "note"},
		"asal tak ada":    {cin(uuid.New(), "1", y, "1"), "from_item_id"},
		"tujuan tak ada":  {cin(x, "1", uuid.New(), "1"), "to_item_id"},
		"tenant lain":     {cin(x, "1", foreign, "1"), "to_item_id"},
	} {
		var f FieldErrors
		if _, _, err := svc.Convert(ctx, a, key(), c.in); !errors.As(err, &f) || f[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := svc.Convert(ctx, a, "pendek", cin(x, "1", y, "1")); !errors.Is(err, ErrKeyRequired) {
		t.Errorf("kunci pendek: %v", err)
	}
	if _, _, err := svc.Convert(ctx, a, key(), cin(x, "1", e.item(t, e.tenant, "service", false), "1")); !errors.Is(err, ErrNotStocked) {
		t.Errorf("jasa: %v", err)
	}
	// Stok kurang: seluruh dokumen batal (tanpa dokumen, movement, saldo tujuan, atau nomor terpakai).
	if _, _, err := svc.Convert(ctx, a, key(), cin(x, "4", y, "40")); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("stok kurang: %v", err)
	}
	if e.docs(t) != 0 || e.balance(t, x, BucketDisplay) != "3" || e.balance(t, y, BucketDisplay) != "0" || e.movements(t, y) != 0 {
		t.Error("kegagalan harus membatalkan semuanya")
	}
	c, _, err := svc.Convert(ctx, a, key(), cin(x, "1", y, "1"))
	if err != nil || !strings.HasSuffix(c.DocNo, "-0001") {
		t.Errorf("nomor tidak boleh terpakai oleh percobaan gagal: %+v %v", c, err)
	}
	// Barang boleh-minus: asal boleh turun di bawah nol (display).
	neg := e.item(t, e.tenant, "goods", true)
	if _, _, err := svc.Convert(ctx, a, key(), cin(neg, "2", y, "2")); err != nil || e.balance(t, neg, BucketDisplay) != "-2" {
		t.Errorf("boleh minus: %v saldo=%s", err, e.balance(t, neg, BucketDisplay))
	}
}

func TestConvertIdempotency(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "50")

	k := key()
	first, replayed, err := svc.Convert(ctx, a, k, cin(x, "1", y, "12"))
	if err != nil || replayed {
		t.Fatal(err, replayed)
	}
	again, replayed, err := svc.Convert(ctx, a, k, cin(x, "1", y, "12"))
	if err != nil || !replayed || again.ID != first.ID {
		t.Errorf("ulang: %+v replay=%v %v", again, replayed, err)
	}
	if _, _, err := svc.Convert(ctx, a, k, cin(x, "2", y, "12")); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("isi beda: %v", err)
	}
	if e.docs(t) != 1 || e.balance(t, x, BucketDisplay) != "49" || e.balance(t, y, BucketDisplay) != "12" {
		t.Errorf("dokumen=%d x=%s y=%s", e.docs(t), e.balance(t, x, BucketDisplay), e.balance(t, y, BucketDisplay))
	}
	// Delapan kiriman bersamaan dengan kunci sama → satu dokumen.
	k2 := key()
	var wg sync.WaitGroup
	var fresh int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, rp, err := svc.Convert(ctx, a, k2, cin(x, "1", y, "12")); err != nil {
				t.Error(err)
			} else if !rp {
				atomic.AddInt32(&fresh, 1)
			}
		}()
	}
	wg.Wait()
	if fresh != 1 || e.docs(t) != 2 || e.balance(t, x, BucketDisplay) != "48" {
		t.Errorf("bersamaan: baru=%d dokumen=%d x=%s", fresh, e.docs(t), e.balance(t, x, BucketDisplay))
	}
}

func TestConvertConcurrent(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "10")

	// 25 pecah satuan bersamaan dari stok 10 → tepat 10 berhasil, stok tidak minus.
	var wg sync.WaitGroup
	var ok, short int32
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := svc.Convert(ctx, a, key(), cin(x, "1", y, "12"))
			switch {
			case err == nil:
				atomic.AddInt32(&ok, 1)
			case errors.Is(err, ErrInsufficient):
				atomic.AddInt32(&short, 1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok != 10 || short != 15 || e.balance(t, x, BucketDisplay) != "0" || e.balance(t, y, BucketDisplay) != "120" {
		t.Errorf("ok=%d kurang=%d x=%s y=%s", ok, short, e.balance(t, x, BucketDisplay), e.balance(t, y, BucketDisplay))
	}

	// Arah berlawanan bersamaan (x→y dan y→x) tidak boleh deadlock, dan nomor dokumen tidak kembar.
	e.seed(t, a, svc, x, "30")
	e.seed(t, a, svc, y, "300")
	var errs int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := x, y
			if i%2 == 1 {
				from, to = y, x
			}
			if _, _, err := svc.Convert(ctx, a, key(), cin(from, "1", to, "1")); err != nil {
				atomic.AddInt32(&errs, 1)
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var distinct int
	if err := e.admin.QueryRow(ctx, `SELECT count(DISTINCT doc_no) FROM stock_conversions WHERE tenant_id=$1`, e.tenant).Scan(&distinct); err != nil || errs != 0 || distinct != 40 {
		t.Errorf("nomor unik = %d (harus 40), galat=%d %v", distinct, errs, err)
	}
}

func TestConvertImmutableAndRLS(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "5")
	c, _, err := svc.Convert(ctx, a, key(), cin(x, "1", y, "2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE stock_conversions SET to_qty = 99 WHERE id = $1`,
		`DELETE FROM stock_conversions WHERE id = $1`,
	} {
		err := db.WithTenant(ctx, e.app, e.tenant, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, sql, c.ID); return err })
		if err == nil {
			t.Errorf("harus ditolak: %s", sql)
		}
	}
	// Tenant lain tidak melihat dokumen ini.
	other := authz.Actor{TenantID: e.other, UserID: uuid.New(), OutletID: uuid.New(), Perms: authz.Permissions{All: true}}
	if _, err := svc.GetConversion(ctx, other, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("tenant lain: %v", err)
	}
	if _, total, err := svc.ListConversions(ctx, other, 10, 0); err != nil || total != 0 {
		t.Errorf("daftar tenant lain: %d %v", total, err)
	}
}

// Dokumen pecah satuan cabang lain dalam tenant yang sama tidak boleh dibuka lewat id (RLS hanya memisahkan tenant).
func TestGetConversionIsScopedToAccessibleOutlets(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.app)
	ctx := context.Background()
	a := e.convActor(t, e.outlet)
	x, y := e.item(t, e.tenant, "goods", false), e.item(t, e.tenant, "goods", false)
	e.seed(t, a, svc, x, "5")
	c, _, err := svc.Convert(ctx, a, key(), cin(x, "1", y, "2"))
	if err != nil {
		t.Fatal(err)
	}
	b := e.convActor(t, e.outlet2)
	b.Outlets = map[uuid.UUID]bool{e.outlet2: true}
	if _, err := svc.GetConversion(ctx, b, c.ID); !errors.Is(err, ErrOutletForbidden) {
		t.Fatalf("lintas cabang harus ditolak: %v", err)
	}
	b.Outlets = map[uuid.UUID]bool{e.outlet: true, e.outlet2: true}
	if got, err := svc.GetConversion(ctx, b, c.ID); err != nil || got.ID != c.ID {
		t.Fatalf("akses dua cabang: %v", err)
	}
	if got, err := svc.GetConversion(ctx, a, c.ID); err != nil || got.ID != c.ID {
		t.Fatalf("cabang sendiri: %v", err)
	}
}
