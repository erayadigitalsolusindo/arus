package accounting

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
)

type tplAccount struct {
	code, name, parent, kind, class string
	cashBank                        bool
}

// retailTemplate = bagan akun retail bawaan (FR-ACC-05). Induk selalu ditulis sebelum anaknya.
var retailTemplate = []tplAccount{
	{"1000", "ASET", "", KindGroup, "asset", false},
	{"1100", "Aset Lancar", "1000", KindGroup, "asset", false},
	{"1110", "Kas Toko", "1100", KindLedger, "asset", true},
	{"1120", "Bank", "1100", KindLedger, "asset", true},
	{"1130", "QRIS / Dompet Digital", "1100", KindLedger, "asset", true},
	{"1140", "Piutang Kartu / EDC", "1100", KindLedger, "asset", false},
	{"1210", "Piutang Usaha", "1100", KindLedger, "asset", false},
	{"1220", "Piutang Lain-lain", "1100", KindLedger, "asset", false},
	{"1310", "Persediaan Barang Dagang", "1100", KindLedger, "asset", false},
	{"1320", "Uang Muka Pembelian", "1100", KindLedger, "asset", false},
	{"1330", "PPN Masukan", "1100", KindLedger, "asset", false},
	{"1400", "Aset Tetap", "1000", KindGroup, "asset", false},
	{"1410", "Peralatan Toko", "1400", KindLedger, "asset", false},
	{"1420", "Kendaraan", "1400", KindLedger, "asset", false},
	{"2000", "KEWAJIBAN", "", KindGroup, "liability", false},
	{"2100", "Kewajiban Lancar", "2000", KindGroup, "liability", false},
	{"2110", "Hutang Usaha", "2100", KindLedger, "liability", false},
	{"2120", "Hutang Lain-lain", "2100", KindLedger, "liability", false},
	{"2130", "PPN Keluaran", "2100", KindLedger, "liability", false},
	{"2140", "Deposit Member", "2100", KindLedger, "liability", false},
	{"2150", "Uang Muka Penjualan", "2100", KindLedger, "liability", false},
	{"2200", "Hutang Bank", "2000", KindLedger, "liability", false},
	{"3000", "EKUITAS", "", KindGroup, "equity", false},
	{"3100", "Modal Pemilik", "3000", KindLedger, "equity", false},
	{"3200", "Laba Ditahan", "3000", KindLedger, "equity", false},
	{"4000", "PENDAPATAN", "", KindGroup, "revenue", false},
	{"4100", "Penjualan", "4000", KindLedger, "revenue", false},
	{"4200", "Pendapatan Jasa / Biaya Lain", "4000", KindLedger, "revenue", false},
	{"4900", "Pendapatan Lain-lain", "4000", KindLedger, "revenue", false},
	{"5000", "HARGA POKOK", "", KindGroup, "cogs", false},
	{"5100", "Harga Pokok Penjualan", "5000", KindLedger, "cogs", false},
	{"5200", "Selisih Stok", "5000", KindLedger, "cogs", false},
	{"6000", "BEBAN OPERASIONAL", "", KindGroup, "expense", false},
	{"6100", "Beban Gaji", "6000", KindLedger, "expense", false},
	{"6110", "Beban Sewa", "6000", KindLedger, "expense", false},
	{"6120", "Beban Listrik, Air, Telepon", "6000", KindLedger, "expense", false},
	{"6130", "Beban Perlengkapan", "6000", KindLedger, "expense", false},
	{"6140", "Beban Transportasi", "6000", KindLedger, "expense", false},
	{"6150", "Beban Pemasaran", "6000", KindLedger, "expense", false},
	{"6160", "Beban Admin Bank & Pembayaran Digital", "6000", KindLedger, "expense", false},
	{"6170", "Beban Penyusutan", "6000", KindLedger, "expense", false},
	{"6180", "Beban Pajak", "6000", KindLedger, "expense", false},
	{"6900", "Beban Lain-lain", "6000", KindLedger, "expense", false},
}

// systemCodes = akun krusial yang dirujuk otomatis oleh aplikasi (kas, piutang, persediaan, PPN, ekuitas, penjualan, HPP) beserta
// induk strukturalnya; tidak boleh dihapus/dinonaktifkan/diubah kodenya. Harus sama dengan backfill migration 00059.
var systemCodes = map[string]bool{
	"1000": true, "1100": true, "1110": true, "1120": true, "1130": true, "1140": true, "1210": true, "1310": true, "1330": true,
	"2000": true, "2100": true, "2110": true, "2130": true, "2140": true,
	"3000": true, "3100": true, "3200": true,
	"4000": true, "4100": true, "5000": true, "5100": true, "5200": true, "6000": true,
}

// SeedRetailTemplate mengisi bagan akun retail bawaan. Hanya untuk tenant yang belum punya akun sama sekali.
func (s *Service) SeedRetailTemplate(ctx context.Context, a authz.Actor) ([]Account, error) {
	out := []Account{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		n, err := q.AccountCount(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrCOAExists
		}
		ids := map[string]pgtype.UUID{}
		for _, t := range retailTemplate {
			row, err := q.AccountCreate(ctx, gen.AccountCreateParams{TenantID: a.TenantID, ParentID: ids[t.parent], Code: t.code, Name: t.name,
				Kind: t.kind, Class: t.class, NormalSide: normalSide(t.class), IsCashBank: t.cashBank, IsSystem: systemCodes[t.code]})
			if pgCode(err) == "23505" { // seed bersamaan: yang kalah dibatalkan
				return ErrCOAExists
			}
			if err != nil {
				return err
			}
			ids[t.code] = pgUUID(row.ID)
			out = append(out, accountOf(row))
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAccountSeed, Entity: audit.EntityAccount,
			Details: map[string]any{"template": "retail", "count": len(out)}})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
