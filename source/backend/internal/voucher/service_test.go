package voucher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
)

func i32(v int32) *int32 { return &v }

func TestValidate(t *testing.T) {
	ok := Input{Code: " hemat-10 ", Name: "Hemat", Kind: "percent", Value: json.Number("10"), MaxDiscount: json.Number("15000"),
		MinSpend: json.Number("50000"), StartsOn: "2026-10-01", EndsOn: "2026-10-31", MaxUses: i32(100)}
	c, f := validate(ok)
	if f != nil || c.code != "HEMAT-10" || !c.maxDiscount.Valid || c.maxUses.Int32 != 100 {
		t.Fatalf("valid: %+v %v", c, f)
	}
	bad := map[string]Input{
		"code":         {Code: "a b", Name: "x", Kind: "amount", Value: "1"},
		"value":        {Code: "ABC", Name: "x", Kind: "percent", Value: "101"},
		"kind":         {Code: "ABC", Name: "x", Kind: "bogus", Value: "1"},
		"max_discount": {Code: "ABC", Name: "x", Kind: "amount", Value: "1000", MaxDiscount: "500"},
		"ends_on":      {Code: "ABC", Name: "x", Kind: "amount", Value: "1", StartsOn: "2026-10-05", EndsOn: "2026-10-01"},
		"max_uses":     {Code: "ABC", Name: "x", Kind: "amount", Value: "1", MaxUses: i32(0)},
		"min_spend":    {Code: "ABC", Name: "x", Kind: "amount", Value: "1", MinSpend: "-5"},
		"starts_on":    {Code: "ABC", Name: "x", Kind: "amount", Value: "1", StartsOn: "01/10/2026"},
	}
	for field, in := range bad {
		if _, f := validate(in); f[field] == "" {
			t.Errorf("%s: want galat, dapat %v", field, f)
		}
	}
}

func TestAmountFor(t *testing.T) {
	d := decimal.RequireFromString
	pct := Rule{Kind: KindPercent, Value: d("10"), MaxDiscount: decimal.NullDecimal{Decimal: d("15000"), Valid: true}}
	if got := AmountFor(pct, d("100000")); !got.Equal(d("10000")) {
		t.Fatalf("10%% × 100.000 = %s", got)
	}
	if got := AmountFor(pct, d("200000")); !got.Equal(d("15000")) {
		t.Fatalf("dibatasi max = %s", got)
	}
	if got := AmountFor(Rule{Kind: KindPercent, Value: d("12.5")}, d("333.33")); !got.Equal(d("41.67")) {
		t.Fatalf("pembulatan = %s", got)
	}
	if got := AmountFor(Rule{Kind: KindAmount, Value: d("7000")}, d("1")); !got.Equal(d("7000")) {
		t.Fatalf("nominal = %s", got)
	}
}

func TestLifecycle(t *testing.T) {
	appURL, adminURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	tenant, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenant, other} {
		if _, err := admin.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-KUPON')`, id, "kp-"+id.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []uuid.UUID{tenant, other} {
			_, _ = admin.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, id)
			_, _ = admin.Exec(ctx, `DELETE FROM vouchers WHERE tenant_id = $1`, id)
			_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
		}
	})
	svc := NewService(app)
	a := authz.Actor{TenantID: tenant}
	b := authz.Actor{TenantID: other}

	v, err := svc.Create(ctx, a, Input{Code: "promo-1", Name: "Promo", Kind: "percent", Value: "10", MaxUses: i32(5)})
	if err != nil || v.Code != "PROMO-1" || v.Value != "10.00" || v.MaxUses == nil || *v.MaxUses != 5 {
		t.Fatalf("create: %+v %v", v, err)
	}
	if _, err = svc.Create(ctx, a, Input{Code: "PROMO-1", Name: "Lagi", Kind: "amount", Value: "1000"}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("kode ganda: %v", err)
	}
	// Tenant lain boleh memakai kode yang sama dan tidak melihat kupon ini.
	if _, err = svc.Create(ctx, b, Input{Code: "PROMO-1", Name: "Lain", Kind: "amount", Value: "1000"}); err != nil {
		t.Fatal(err)
	}
	if list, total, err := svc.List(ctx, b, ListParams{}); err != nil || total != 1 || list[0].Name != "Lain" {
		t.Fatalf("isolasi: %v %d %v", list, total, err)
	}
	if _, err = svc.Update(ctx, b, v.ID, Input{Code: "PROMO-1", Name: "x", Kind: "amount", Value: "1"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ubah milik tenant lain: %v", err)
	}

	// Setelah dipakai: kode terkunci dan batas pemakaian tak boleh di bawah yang sudah terpakai.
	if _, err = admin.Exec(ctx, `UPDATE vouchers SET used_count = 3 WHERE id = $1`, v.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(ctx, a, v.ID, Input{Code: "BARU-1", Name: "Promo", Kind: "percent", Value: "10"})
	var f FieldErrors
	if !errors.As(err, &f) || f["code"] != "LOCKED" {
		t.Fatalf("kode terkunci: %v", err)
	}
	_, err = svc.Update(ctx, a, v.ID, Input{Code: "PROMO-1", Name: "Promo", Kind: "percent", Value: "10", MaxUses: i32(2)})
	if !errors.As(err, &f) || f["max_uses"] != "BELOW_USED" {
		t.Fatalf("batas di bawah terpakai: %v", err)
	}
	u, err := svc.Update(ctx, a, v.ID, Input{Code: "promo-1", Name: "Promo 2", Kind: "amount", Value: "2500", MaxUses: i32(10)})
	if err != nil || u.Name != "Promo 2" || u.Kind != "amount" || u.UsedCount != 3 {
		t.Fatalf("update: %+v %v", u, err)
	}
	if off, err := svc.SetActive(ctx, a, v.ID, false); err != nil || off.Active {
		t.Fatalf("arsip: %+v %v", off, err)
	}
	f2 := false
	if list, total, err := svc.List(ctx, a, ListParams{Active: &f2, Q: "promo"}); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("filter: %v %d %v", list, total, err)
	}
}
