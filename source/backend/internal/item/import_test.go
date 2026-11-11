package item

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

const csvHeader = "Kode,Barcode,Nama,Satuan,Kategori,Merek,HPP,Harga Jual,Stok Minimum,Saldo Awal\n"

func parseCSV(t *testing.T, body string) [][]string {
	t.Helper()
	rows, err := ParseImport("data.csv", []byte(csvHeader+body))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestParseImport(t *testing.T) {
	// titik koma + BOM + header alias Inggris
	rows, err := ParseImport("a.csv", []byte("\xef\xbb\xbfcode;name;unit;price\nA1;Teh;Pcs;5000\n"))
	if err != nil || len(rows) != 2 || rows[1][1] != "Teh" {
		t.Fatalf("csv titik koma: %v %v", rows, err)
	}
	idx, err := headerIndex(rows[0])
	if err != nil || idx["sell_price"] != 3 || idx["sku"] != 0 {
		t.Fatalf("headerIndex: %v %v", idx, err)
	}
	if _, err := headerIndex([]string{"Kode", "Nama"}); !errors.Is(err, ErrImportNoHeader) {
		t.Fatalf("tanpa kolom satuan = %v", err)
	}
	if _, err := ParseImport("a.csv", []byte("Nama,Satuan\n")); !errors.Is(err, ErrImportEmpty) {
		t.Fatalf("hanya header = %v", err)
	}
	if _, err := ParseImport("a.pdf", []byte("x")); !errors.Is(err, ErrImportFormat) {
		t.Fatalf("ekstensi asing = %v", err)
	}
	// template xlsx dan csv bisa dibaca kembali oleh parser yang sama.
	for _, f := range []string{"xlsx", "csv"} {
		data, err := ImportTemplate(f)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := ParseImport("t."+f, data)
		if err != nil || len(rows) != 3 || rows[1][2] != "Contoh Barang A" || rows[1][7] != "10000" {
			t.Fatalf("template %s: %v %v", f, rows, err)
		}
		if _, err := headerIndex(rows[0]); err != nil {
			t.Fatalf("header template %s: %v", f, err)
		}
	}
	// xlsx: sel angka dibaca mentah, bukan sebagai teks berformat.
	x := excelize.NewFile()
	_ = x.SetSheetRow("Sheet1", "A1", &[]any{"Nama", "Satuan", "Harga Jual"})
	_ = x.SetSheetRow("Sheet1", "A2", &[]any{"Gula", "Kg", 12500.5})
	buf, _ := x.WriteToBuffer()
	rows, err = ParseImport("x.xlsx", buf.Bytes())
	if err != nil || rows[1][2] != "12500.5" {
		t.Fatalf("xlsx mentah: %v %v", rows, err)
	}
}

func TestImportPreviewValidates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.master(t, "units", e.a.TenantID, "Pcs", true)
	e.master(t, "categories", e.a.TenantID, "Minuman", true)
	e.master(t, "categories", e.a.TenantID, "Arsip", false)
	if _, err := e.svc.Create(ctx, e.a, Input{SKU: "ADA-1", Name: "Sudah Ada", UnitID: e.master(t, "units", e.a.TenantID, "Box", true).String()}); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.ImportPreview(ctx, e.a, parseCSV(t,
		"A-1,,Teh,Pcs,Minuman,,1000,2000,,\n"+ // valid
			"A-1,,Teh Kembar,Pcs,,,,,,\n"+ // kode ganda di berkas
			"ADA-1,,Bentrok,Pcs,,,,,,\n"+ // kode sudah dipakai
			"A-3,,Tanpa Satuan,,,,,,,\n"+ // satuan wajib
			"A-4,,Satuan Asing,Lusin,,,,,,\n"+ // satuan tak dikenal
			"A-5,,Kategori Arsip,Pcs,Arsip,,,,,\n"+ // master nonaktif = tidak dikenal
			"A-6,,Harga Rusak,Pcs,,,abc,-5,,\n"+
			"A-7,,Stok Rusak,Pcs,,,,,,1.2345\n"+
			",,,,,,,,,\n"+ // baris kosong dilewati
			",,Otomatis,Pcs,,,,,,\n")) // kode kosong = otomatis
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 9 || res.Valid != 2 || res.Invalid != 7 || res.Created != 0 {
		t.Fatalf("ringkasan: %+v", res)
	}
	want := map[int]map[string]string{
		3:  {"sku": codeDuplicateInFile},
		4:  {"sku": codeCodeTaken},
		5:  {"unit": sanitizeRequired},
		6:  {"unit": codeNotFound},
		7:  {"category": codeNotFound},
		8:  {"cost": "INVALID", "sell_price": "INVALID"},
		9:  {"opening_stock": "INVALID"},
		10: nil,
	}
	got := map[int]map[string]string{}
	for _, r := range res.Errors {
		got[r.Line] = r.Errors
	}
	for line, cols := range want {
		for col, code := range cols {
			if got[line][col] != code {
				t.Errorf("baris %d kolom %s = %q, want %q (semua: %v)", line, col, got[line][col], code, got[line])
			}
		}
	}
	if _, bad := got[2]; bad {
		t.Errorf("baris 2 harus valid: %v", got[2])
	}
	// Pratinjau tidak menulis apa pun.
	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM items WHERE tenant_id = $1`, e.a.TenantID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pratinjau mengubah data: %d %v", n, err)
	}
}

const sanitizeRequired = "REQUIRED"

func TestImportCommitCreatesItemsAndOpeningStock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = e.admin.Exec(ctx, `DELETE FROM stock_movements WHERE tenant_id = $1`, e.a.TenantID) })
	e.master(t, "units", e.a.TenantID, "Pcs", true)
	e.master(t, "categories", e.a.TenantID, "Minuman", true)

	res, err := e.svc.ImportCommit(ctx, e.a, parseCSV(t,
		"T-1,8991,Teh Botol,PCS,minuman,,7000.50,10000,5,12.5\n"+
			",,Tanpa Kode,Pcs,,,,3000,,\n"+
			"T-1,,Duplikat,Pcs,,,,,,\n"+
			"T-9,,Gagal,Lusin,,,,,,\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 || res.Created != 2 || res.Invalid != 2 || res.Valid != 2 {
		t.Fatalf("ringkasan: %+v", res)
	}
	var cost, sell, minStock string
	var balance decimal.Decimal
	if err := e.admin.QueryRow(ctx, `SELECT last_cost::text, sell_price::text, min_stock::text FROM items WHERE tenant_id = $1 AND sku = 'T-1'`, e.a.TenantID).Scan(&cost, &sell, &minStock); err != nil {
		t.Fatal(err)
	}
	if cost != "7000.50" || sell != "10000.00" || minStock != "5.000" {
		t.Fatalf("nilai barang: %s %s %s", cost, sell, minStock)
	}
	// Saldo awal = movement OPENING + saldo bucket display outlet aktif; barang tanpa saldo tidak punya movement.
	if err := e.admin.QueryRow(ctx, `SELECT coalesce(sum(m.qty_delta), 0) FROM stock_movements m JOIN items i ON i.id = m.item_id
		WHERE m.tenant_id = $1 AND m.ref_type = 'OPENING' AND m.outlet_id = $2 AND i.sku = 'T-1'`, e.a.TenantID, e.a.OutletID).Scan(&balance); err != nil || !balance.Equal(decimal.RequireFromString("12.5")) {
		t.Fatalf("movement OPENING = %v %v", balance, err)
	}
	var movements int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE tenant_id = $1`, e.a.TenantID).Scan(&movements); err != nil || movements != 1 {
		t.Fatalf("jumlah movement = %d %v", movements, err)
	}
	// Mengimpor ulang berkas yang sama: semua kode sudah dipakai, tidak ada duplikat dan stok tidak bertambah.
	res, err = e.svc.ImportCommit(ctx, e.a, parseCSV(t, "T-1,8991,Teh Botol,Pcs,,,,10000,,99\n"))
	if err != nil || res.Created != 0 || res.Errors[0].Errors["sku"] != codeCodeTaken {
		t.Fatalf("impor ulang: %+v %v", res, err)
	}
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM stock_movements WHERE tenant_id = $1`, e.a.TenantID).Scan(&movements); err != nil || movements != 1 {
		t.Fatalf("impor ulang menambah movement: %d %v", movements, err)
	}
	// Tenant lain tidak terpengaruh dan satuan tenant lain tidak dikenal.
	other, err := e.svc.ImportPreview(ctx, e.b, parseCSV(t, ",,X,Pcs,,,,,,\n"))
	if err != nil || other.Errors[0].Errors["unit"] != codeNotFound {
		t.Fatalf("isolasi tenant: %+v %v", other, err)
	}
}

func TestImportRespectsLocksAndPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.master(t, "units", e.a.TenantID, "Pcs", true)
	body := "L-1,,Barang,Pcs,,,,,,5\n"

	// Tanpa izin saldo awal: kolom saldo ditolak, barang tidak dibuat.
	noStock := e.a
	noStock.Perms.All = false
	noStock.Perms.Grants = map[string][]string{ModuleID: {"view", "create"}}
	res, err := e.svc.ImportCommit(ctx, noStock, parseCSV(t, body))
	if err != nil || res.Created != 0 || res.Errors[0].Errors["opening_stock"] != codeForbidden {
		t.Fatalf("tanpa izin: %+v %v", res, err)
	}
	// Outlet terkunci: saldo awal > 0 ditolak.
	if _, err := e.admin.Exec(ctx, `UPDATE outlets SET stock_locked_at = now(), ops_start_date = current_date WHERE id = $1`, e.a.OutletID); err != nil {
		t.Fatal(err)
	}
	res, err = e.svc.ImportPreview(ctx, e.a, parseCSV(t, body))
	if err != nil || res.Invalid != 1 || res.Errors[0].Errors["opening_stock"] != codeLocked {
		t.Fatalf("outlet terkunci: %+v %v", res, err)
	}
}

func TestImportStopsAtPlanLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.master(t, "units", e.a.TenantID, "Pcs", true)
	planID := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO plans (id, code, name, max_items) VALUES ($1, $2, 'UJI', 2)`, planID, "imp-"+planID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO tenant_subscriptions (tenant_id, plan_id) VALUES ($1, $2)`, e.a.TenantID, planID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM tenant_subscriptions WHERE tenant_id = $1`, e.a.TenantID)
		_, _ = e.admin.Exec(ctx, `DELETE FROM plans WHERE id = $1`, planID)
	})
	res, err := e.svc.ImportCommit(ctx, e.a, parseCSV(t, "P-1,,Satu,Pcs,,,,,,\nP-2,,Dua,Pcs,,,,,,\nP-3,,Tiga,Pcs,,,,,,\nP-4,,Empat,Pcs,,,,,,\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 2 || res.Invalid != 2 || res.Errors[0].Errors["name"] != codePlanLimit || res.Errors[1].Errors["name"] != codePlanLimit {
		t.Fatalf("batas paket: %+v", res)
	}
}
