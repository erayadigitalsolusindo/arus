package accounting

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Peran akuntansi yang dirujuk jurnal otomatis (FR-ACC-02). Kode bawaan harus sama dengan backfill migration 00062 dan
// retailTemplate. Akun per metode bayar bukan di sini, tetapi di payment_methods.account_id.
const (
	MapSalesRevenue    = "sales_revenue"
	MapOtherIncome     = "other_income"     // biaya lain / biaya tambahan metode bayar yang ditanggung pelanggan
	MapReceivable      = "receivable"       // piutang usaha
	MapCOGS            = "cogs"             // harga pokok penjualan
	MapInventory       = "inventory"        // persediaan
	MapTaxOut          = "tax_out"          // PPN keluaran
	MapTaxIn           = "tax_in"           // PPN masukan
	MapPayable         = "payable"          // hutang usaha
	MapFeeExpense      = "fee_expense"      // beban admin metode bayar (MDR)
	MapStockVariance   = "stock_variance"   // selisih stok opname
	MapMemberDeposit   = "member_deposit"   // titipan deposit member
	MapSupplierAdvance = "supplier_advance" // uang muka / kredit pemasok
	MapCashOver        = "cash_over"        // selisih kas lebih
	MapCashShort       = "cash_short"       // selisih kas kurang
)

type mappingDef struct {
	key, code string
	classes   []string // kelas akun yang boleh dipilih
}

// mappingDefs berurutan sesuai tampilan di layar pengaturan.
var mappingDefs = []mappingDef{
	{MapSalesRevenue, "4100", []string{"revenue"}},
	{MapOtherIncome, "4200", []string{"revenue"}},
	{MapReceivable, "1210", []string{"asset"}},
	{MapCOGS, "5100", []string{"cogs", "expense"}},
	{MapInventory, "1310", []string{"asset"}},
	{MapTaxOut, "2130", []string{"liability"}},
	{MapTaxIn, "1330", []string{"asset"}},
	{MapPayable, "2110", []string{"liability"}},
	{MapFeeExpense, "6160", []string{"expense"}},
	{MapStockVariance, "5200", []string{"cogs", "expense"}},
	{MapMemberDeposit, "2140", []string{"liability"}},
	{MapSupplierAdvance, "1320", []string{"asset"}},
	{MapCashOver, "4900", []string{"revenue"}},
	{MapCashShort, "6900", []string{"expense"}},
}

func mappingDefOf(key string) (mappingDef, bool) {
	for _, d := range mappingDefs {
		if d.key == key {
			return d, true
		}
	}
	return mappingDef{}, false
}

func (d mappingDef) allows(class string) bool {
	for _, c := range d.classes {
		if c == class {
			return true
		}
	}
	return false
}

// Mapping = satu peran beserta akun terpilihnya (Account nil bila belum dipetakan).
type Mapping struct {
	Key     string   `json:"key"`
	Classes []string `json:"classes"`
	Account *Account `json:"account"`
}

// MappingInput = peran -> id akun. Hanya peran yang dikirim yang diubah.
type MappingInput struct {
	Items map[string]uuid.UUID `json:"items"`
}

func listMappings(ctx context.Context, q *gen.Queries, tenant uuid.UUID) ([]Mapping, error) {
	rows, err := q.MappingList(ctx, tenant)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]gen.MappingListRow, len(rows))
	for _, r := range rows {
		byKey[r.Key] = r
	}
	out := make([]Mapping, 0, len(mappingDefs))
	for _, d := range mappingDefs {
		m := Mapping{Key: d.key, Classes: d.classes}
		if r, ok := byKey[d.key]; ok {
			m.Account = &Account{ID: r.AccountID, Code: r.Code, Name: r.Name, Class: r.Class, Active: r.Active}
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Service) ListMappings(ctx context.Context, a authz.Actor) ([]Mapping, error) {
	var out []Mapping
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
		out, err = listMappings(ctx, gen.New(tx), a.TenantID)
		return err
	})
	return out, err
}

// seedMappings mengisi pemetaan bawaan menurut kode akun (dipanggil setelah template retail dipasang).
func seedMappings(ctx context.Context, q *gen.Queries, tenant uuid.UUID) error {
	for _, d := range mappingDefs {
		if err := q.MappingInsertDefault(ctx, gen.MappingInsertDefaultParams{TenantID: tenant, Column2: d.key, Column3: d.code}); err != nil {
			return err
		}
	}
	return nil
}

// UpdateMappings: akun harus akun buku aktif dengan kelas yang sesuai peran.
func (s *Service) UpdateMappings(ctx context.Context, a authz.Actor, in MappingInput) ([]Mapping, error) {
	fe := FieldErrors{}
	for key := range in.Items {
		if _, ok := mappingDefOf(key); !ok {
			fe[key] = sanitize.Invalid
		}
	}
	if len(in.Items) == 0 {
		fe["items"] = sanitize.Required
	}
	if len(fe) > 0 {
		return nil, fe
	}
	var out []Mapping
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		changed := map[string]string{}
		for key, id := range in.Items {
			acc, err := q.AccountGet(ctx, gen.AccountGetParams{TenantID: a.TenantID, ID: id})
			if errors.Is(err, pgx.ErrNoRows) {
				fe[key] = sanitize.Invalid
				continue
			}
			if err != nil {
				return err
			}
			def, _ := mappingDefOf(key)
			if acc.Kind != KindLedger || !acc.Active || !def.allows(acc.Class) {
				fe[key] = sanitize.Invalid
				continue
			}
			if err := q.MappingUpsert(ctx, gen.MappingUpsertParams{TenantID: a.TenantID, Key: key, AccountID: id}); err != nil {
				return err
			}
			changed[key] = acc.Code
		}
		if len(fe) > 0 {
			return fe
		}
		var err error
		if out, err = listMappings(ctx, q, a.TenantID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAccountMapUpdate, Entity: audit.EntityAccount,
			Details: map[string]any{"changed": changed}})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
