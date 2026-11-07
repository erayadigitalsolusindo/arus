package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/aipolicy"
	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/item"
	"aciraba/internal/member"
	"aciraba/internal/storefront"
	"aciraba/internal/voucher"
)

// masterEnv = actEnv + service master + pengguna "admin data" yang boleh mengubah master (tanpa izin lain).
type masterEnv struct {
	*actEnv
	editor authz.Actor
	noPerm authz.Actor
}

func newMasterEnv(t *testing.T) *masterEnv {
	t.Helper()
	e := newActEnv(t)
	ctx := context.Background()
	e.svc.WithMasters(Masters{
		Items: item.NewService(e.app, nil), Members: member.NewService(e.app, nil), Catalog: catalog.NewService(e.app),
		Vouchers: voucher.NewService(e.app), Storefront: storefront.NewService(e.app),
	})
	noPerm := e.cashier
	noPerm.UserID = uuid.New()
	noPerm.Perms = authz.ParseStored([]byte(`{"ai_assistant":["view","create"]}`))
	e.cashier.Perms = authz.ParseStored([]byte(`{"sales_orders":["view","create"],"ai_assistant":["view","create"],` +
		`"items":["view","create","update"],"members":["view","create","update"],"suppliers":["view","create","update"],` +
		`"coupons":["view","create","update"],"storefront":["view","update"]}`))
	m := &masterEnv{actEnv: e, editor: e.cashier, noPerm: noPerm}
	if _, err := e.admin.Exec(ctx, `INSERT INTO units (tenant_id, name) VALUES ($1, 'Pcs')`, e.tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { // semua tabel bertenant milik tenant sementara ini, beberapa putaran agar urutan FK terselesaikan
		rows, err := e.admin.Query(ctx, `SELECT table_name FROM information_schema.columns WHERE column_name = 'tenant_id' AND table_schema = 'public' AND table_name <> 'tenants'`)
		if err != nil {
			return
		}
		var tables []string
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			tables = append(tables, n)
		}
		rows.Close()
		for pass := 0; pass < 8; pass++ {
			for _, tb := range tables {
				_, _ = e.admin.Exec(ctx, `DELETE FROM "`+tb+`" WHERE tenant_id = $1`, e.tenant)
			}
		}
	})
	return m
}

func (m *masterEnv) master(t *testing.T, a authz.Actor, in string) (PendingAction, string, error) {
	t.Helper()
	return m.svc.propose(context.Background(), a, ToolProposeMaster, json.RawMessage(in))
}

func (m *masterEnv) approve(t *testing.T, a authz.Actor, id string) error {
	t.Helper()
	_, err := m.svc.ConfirmAction(context.Background(), a, id, m.good(), totpOK)
	return err
}

func TestMasterItemCreateUpdateDeactivate(t *testing.T) {
	m := newMasterEnv(t)
	ctx := context.Background()

	// Usulan saja tidak menulis; model hanya diberi nama kolom, bukan nilai.
	pa, out, err := m.master(t, m.editor, `{"entity":"item","op":"create","fields":{"name":"Kopi Susu","sell_price":"18000","cost":"9000","unit":"pcs"}}`)
	if err != nil || pa.Kind != KindMaster || !pa.NeedsPin || !pa.NeedsTotp || pa.Fields["chg.sell_price"] != "18000.00" {
		t.Fatalf("propose create: %+v %v", pa, err)
	}
	if strings.Contains(out, "18000") {
		t.Errorf("nilai tidak boleh dikirim ke model: %s", out)
	}
	count := func(q string, args ...any) int { return m.count(t, q, append([]any{m.tenant}, args...)...) }
	if count(`SELECT count(*) FROM items WHERE tenant_id = $1`) != 0 {
		t.Fatal("usulan belum boleh menulis")
	}
	// Pengguna lain / TOTP salah tidak menulis.
	if _, err := m.svc.ConfirmAction(ctx, m.other, pa.ID, m.good(), totpOK); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("pengguna lain: %v", err)
	}
	if _, err := m.svc.ConfirmAction(ctx, m.editor, pa.ID, m.good(), "000000"); !errors.Is(err, ErrTOTPInvalid) {
		t.Fatalf("TOTP salah: %v", err)
	}
	if count(`SELECT count(*) FROM items WHERE tenant_id = $1`) != 0 {
		t.Fatal("gagal verifikasi tidak boleh menulis")
	}
	if err := m.approve(t, m.editor, pa.ID); err != nil {
		t.Fatalf("approve create: %v", err)
	}
	var id uuid.UUID
	if err := m.admin.QueryRow(ctx, `SELECT id FROM items WHERE tenant_id = $1 AND name = 'Kopi Susu' AND sell_price = 18000`, m.tenant).Scan(&id); err != nil {
		t.Fatalf("item harus tercatat: %v", err)
	}
	if count(`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.action' AND entity_id = $2`, pa.ID) != 1 {
		t.Error("audit ai.action harus 1")
	}

	// Lookup hanya id + nama/sku/harga, dan menghormati izin lihat.
	pol := aipolicy.Default()
	res, err := m.svc.lookupMaster(ctx, m.editor, pol, json.RawMessage(`{"entity":"item","q":"kopi"}`))
	if err != nil || !strings.Contains(res, id.String()) {
		t.Fatalf("lookup: %v %s", err, res)
	}
	if _, err := m.svc.lookupMaster(ctx, m.noPerm, pol, json.RawMessage(`{"entity":"item","q":"kopi"}`)); err == nil {
		t.Error("tanpa izin lihat item harus ditolak")
	}

	// Update: kartu memuat sebelum → sesudah; kolom di luar daftar putih ditolak.
	if _, _, err := m.master(t, m.editor, `{"entity":"item","op":"update","id":"`+id.String()+`","fields":{"cost":"1"}}`); err == nil {
		t.Error("kolom di luar daftar putih harus ditolak")
	}
	up, _, err := m.master(t, m.editor, `{"entity":"item","op":"update","id":"`+id.String()+`","fields":{"sell_price":"20000"}}`)
	if err != nil || up.Fields["chg.sell_price"] != "18000.00 → 20000.00" || up.Fields["target"] != "Kopi Susu" {
		t.Fatalf("propose update: %+v %v", up, err)
	}
	if _, _, err := m.master(t, m.editor, `{"entity":"item","op":"update","id":"`+id.String()+`","fields":{"sell_price":"18000"}}`); err == nil {
		t.Error("nilai sama dengan data sekarang harus ditolak")
	}
	if _, _, err := m.master(t, m.noPerm, `{"entity":"item","op":"update","id":"`+id.String()+`","fields":{"sell_price":"1"}}`); err == nil {
		t.Error("tanpa izin ubah item harus ditolak")
	}
	if err := m.approve(t, m.editor, up.ID); err != nil {
		t.Fatalf("approve update: %v", err)
	}
	if count(`SELECT count(*) FROM items WHERE tenant_id = $1 AND id = $2 AND sell_price = 20000 AND name = 'Kopi Susu'`, id) != 1 {
		t.Fatal("harga harus berubah, kolom lain tetap")
	}

	// "Hapus" = nonaktifkan, tetap PIN + TOTP.
	da, _, err := m.master(t, m.editor, `{"entity":"item","op":"set_active","id":"`+id.String()+`","active":false}`)
	if err != nil || da.Fields["chg.active"] != "aktif → nonaktif" {
		t.Fatalf("propose nonaktif: %+v %v", da, err)
	}
	if _, err := m.svc.ConfirmAction(ctx, m.editor, da.ID, nil, totpOK); err == nil {
		t.Fatal("tanpa PIN harus ditolak")
	}
	if err := m.approve(t, m.editor, da.ID); err != nil {
		t.Fatalf("approve nonaktif: %v", err)
	}
	if count(`SELECT count(*) FROM items WHERE tenant_id = $1 AND id = $2 AND NOT active`, id) != 1 {
		t.Fatal("item harus nonaktif (baris tetap ada)")
	}
	// Tidak ada tool hapus permanen / transaksi.
	for _, d := range masterToolDefs(m.svc.entities()) {
		n := strings.ToLower(d.Name)
		if strings.Contains(n, "delete") || strings.Contains(n, "sale") || strings.Contains(n, "purchase") || strings.Contains(n, "payment") {
			t.Errorf("tool terlarang: %s", d.Name)
		}
	}
}

func TestMasterMemberVoucherSupplierStorefront(t *testing.T) {
	m := newMasterEnv(t)
	ctx := context.Background()
	exec := func(in string) {
		t.Helper()
		pa, _, err := m.master(t, m.editor, in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if err := m.approve(t, m.editor, pa.ID); err != nil {
			t.Fatalf("approve %s: %v", in, err)
		}
	}
	exec(`{"entity":"member","op":"create","fields":{"name":"Budi Santoso","phone":"08123456789","credit_limit":"500000"}}`)
	var mid uuid.UUID
	if err := m.admin.QueryRow(ctx, `SELECT id FROM members WHERE tenant_id = $1 AND name = 'Budi Santoso'`, m.tenant).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	// Ubah member: model tidak diberi nilai lama (telepon/nama), hanya nama kolom.
	upd := `{"entity":"member","op":"update","id":"` + mid.String() + `","fields":{"phone":"08991234567"}}`
	pa, out, err := m.master(t, m.editor, upd)
	if err != nil || strings.Contains(out, "08123456789") || strings.Contains(out, "Budi") {
		t.Fatalf("data pelanggan bocor ke model: %v %s", err, out)
	}
	if err := m.approve(t, m.editor, pa.ID); err != nil {
		var me *MasterError
		if errors.As(err, &me) {
			t.Fatalf("%v %v", err, me.Fields)
		}
		t.Fatal(err)
	}
	if m.count(t, `SELECT count(*) FROM members WHERE tenant_id = $1 AND phone <> '08123456789' AND phone <> '' AND name = 'Budi Santoso'`, m.tenant) != 1 {
		t.Fatal("telepon harus berubah, nama tetap")
	}
	exec(`{"entity":"supplier","op":"create","fields":{"name":"PT Maju","phone":"0211234567"}}`)
	exec(`{"entity":"voucher","op":"create","fields":{"code":"HEMAT10","name":"Hemat 10%","kind":"percent","value":"10"}}`)
	if m.count(t, `SELECT count(*) FROM vouchers WHERE tenant_id = $1 AND code = 'HEMAT10'`, m.tenant) != 1 {
		t.Fatal("voucher harus tercatat")
	}

	// Storefront: tanpa id, hanya update; teks yang dicari harus persis sekali.
	if _, _, err := m.master(t, m.editor, `{"entity":"storefront","op":"create","fields":{"title":"x"}}`); err == nil {
		t.Error("storefront tidak boleh create")
	}
	if _, _, err := storefront.NewService(m.app).Save(ctx, m.editor, storefront.SaveInput{Slug: "uji-" + m.tenant.String()[:8], Enabled: true, Title: "Toko", HTML: "<h1>Halo Toko</h1><p>Selamat datang</p>"}); err != nil {
		t.Fatal(err)
	}
	sp, _, err := m.master(t, m.editor, `{"entity":"storefront","op":"update","fields":{"find":"Halo Toko","replace":"Promo Akhir Tahun"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.approve(t, m.editor, sp.ID); err != nil {
		t.Fatal(err)
	}
	if m.count(t, `SELECT count(*) FROM storefronts WHERE tenant_id = $1 AND html LIKE '%Promo Akhir Tahun%' AND html NOT LIKE '%Halo Toko%'`, m.tenant) != 1 {
		t.Fatal("teks halaman harus terganti")
	}
	bad, _, err := m.master(t, m.editor, `{"entity":"storefront","op":"update","fields":{"find":"tidak-ada","replace":"x"}}`)
	if err != nil {
		t.Fatal(err)
	}
	var me *MasterError
	if err := m.approve(t, m.editor, bad.ID); !errors.As(err, &me) {
		t.Fatalf("teks tak ditemukan harus MasterError, got %v", err)
	}
}
