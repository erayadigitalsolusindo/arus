package accounting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func mappingOf(t *testing.T, ms []accounting.Mapping, key string) accounting.Mapping {
	t.Helper()
	for _, m := range ms {
		if m.Key == key {
			return m
		}
	}
	t.Fatalf("peran %s tidak ada", key)
	return accounting.Mapping{}
}

// Template retail mengisi semua peran ke akun bawaan; pilihan pengguna divalidasi menurut kelas akun.
func TestAccountMappings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Tenant tanpa bagan akun: semua peran kosong.
	ms, err := e.svc.ListMappings(ctx, e.otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Account != nil {
			t.Fatalf("peran %s sudah terisi sebelum seed", m.Key)
		}
	}

	accs, err := e.svc.SeedRetailTemplate(ctx, e.otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]uuid.UUID{}
	for _, a := range accs {
		byCode[a.Code] = a.ID
	}
	ms, err = e.svc.ListMappings(ctx, e.otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"sales_revenue": "4100", "other_income": "4200", "receivable": "1210", "cogs": "5100", "inventory": "1310",
		"tax_out": "2130", "tax_in": "1330", "payable": "2110", "fee_expense": "6160", "stock_variance": "5200",
		"member_deposit": "2140", "supplier_advance": "1320", "cash_over": "4900", "cash_short": "6900"}
	if len(ms) != len(want) {
		t.Fatalf("jumlah peran %d, mau %d", len(ms), len(want))
	}
	for key, code := range want {
		if m := mappingOf(t, ms, key); m.Account == nil || m.Account.Code != code {
			t.Fatalf("peran %s -> %+v, mau %s", key, m.Account, code)
		}
	}

	// Ganti akun penjualan ke akun pendapatan lain: berhasil.
	res, err := e.svc.UpdateMappings(ctx, e.otherOwner, accounting.MappingInput{Items: map[string]uuid.UUID{"sales_revenue": byCode["4900"]}})
	if err != nil {
		t.Fatal(err)
	}
	if m := mappingOf(t, res, "sales_revenue"); m.Account == nil || m.Account.Code != "4900" {
		t.Fatalf("pemetaan tidak berubah: %+v", m.Account)
	}

	// Kelas salah, akun grup, peran tak dikenal, akun tenant lain, dan kosong: ditolak sebagai galat field.
	for name, in := range map[string]accounting.MappingInput{
		"kelas salah":   {Items: map[string]uuid.UUID{"sales_revenue": byCode["6100"]}},
		"akun grup":     {Items: map[string]uuid.UUID{"sales_revenue": byCode["4000"]}},
		"peran asing":   {Items: map[string]uuid.UUID{"xyz": byCode["4100"]}},
		"akun tenant x": {Items: map[string]uuid.UUID{"sales_revenue": e.acc["4100"]}},
		"kosong":        {},
	} {
		var fe accounting.FieldErrors
		if _, err := e.svc.UpdateMappings(ctx, e.otherOwner, in); !errors.As(err, &fe) {
			t.Fatalf("%s: mau FieldErrors, dapat %v", name, err)
		}
	}

	// Akun yang dipetakan tidak boleh dihapus, dinonaktifkan, atau diganti kelasnya.
	off := false
	if err := e.svc.DeleteAccount(ctx, e.otherOwner, byCode["4900"]); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Fatalf("hapus akun terpetakan: %v", err)
	}
	if _, err := e.svc.UpdateAccount(ctx, e.otherOwner, byCode["4900"], accounting.AccountInput{Code: "4900", Name: "Pendapatan Lain-lain", Class: "revenue", Active: &off}); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Fatalf("nonaktifkan akun terpetakan: %v", err)
	}
	if _, err := e.svc.UpdateAccount(ctx, e.otherOwner, byCode["4900"], accounting.AccountInput{Code: "4900", Name: "Pendapatan Lain-lain", Class: "expense"}); !errors.Is(err, accounting.ErrAccountInUse) {
		t.Fatalf("ubah kelas akun terpetakan: %v", err)
	}

	// Isolasi tenant: pemetaan tenant e.tenant tetap kosong.
	other, err := e.svc.ListMappings(ctx, e.owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range other {
		if m.Account != nil {
			t.Fatalf("pemetaan bocor ke tenant lain: %s", m.Key)
		}
	}
}
