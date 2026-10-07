package item

// Harga grosir dan konversi satuan item (Fase 3.2, irisan D). Aturan harga ada di pricing.go (fungsi murni);
// berkas ini = validasi masukan, baca/tulis DB, dan audit perubahannya.

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/sanitize"
)

const (
	maxAltUnits = 5
	maxQty      = 1_000_000_000 // batas atas jumlah tier & faktor konversi
)

// ---- masukan dari klien (string desimal) ----

type TierInput struct{ MinQty, Price string }

type OutletTiersInput struct {
	OutletID string
	Tiers    []TierInput
}

// WholesaleInput: Default = set tier semua cabang; Outlets = set khusus cabang (menggantikan default sepenuhnya di
// cabang itu). Cabang yang boleh diakses pemanggil tetapi tidak tercantum kembali ke set default.
type WholesaleInput struct {
	Default []TierInput
	Outlets []OutletTiersInput
}

// UnitInput = satuan tambahan: 1 UnitID = Factor satuan dasar. Barcode dan SellPrice opsional.
type UnitInput struct{ UnitID, Factor, Barcode, SellPrice string }

// ---- keluaran ----

type OutletTiers struct {
	OutletID   uuid.UUID `json:"outlet_id"`
	OutletName string    `json:"outlet_name"`
	Tiers      []Tier    `json:"tiers"`
}

type Wholesale struct {
	Default []Tier        `json:"default"`
	Outlets []OutletTiers `json:"outlets"`
}

// AltUnit: SellPrice nil = harga dihitung saat dijual (Factor × harga satuan dasar).
type AltUnit struct {
	UnitID    uuid.UUID        `json:"unit_id"`
	UnitName  string           `json:"unit_name"`
	Factor    decimal.Decimal  `json:"factor"`
	Barcode   string           `json:"barcode"`
	SellPrice *decimal.Decimal `json:"sell_price"`
}

// ---- hasil validasi ----

type cleanWholesale struct {
	def     []Tier
	outlets map[uuid.UUID][]Tier
}

type cleanUnit struct {
	unit    uuid.UUID
	factor  decimal.Decimal
	barcode string
	price   *decimal.Decimal
}

func parseTiers(in []TierInput) ([]Tier, string) {
	out := make([]Tier, 0, len(in))
	for _, t := range in {
		q, ok := parseDec(t.MinQty, 3, decimal.NewFromInt(maxQty))
		p, ok2 := money(t.Price)
		if !ok || !ok2 || strings.TrimSpace(t.MinQty) == "" || strings.TrimSpace(t.Price) == "" || !q.IsPositive() {
			return nil, sanitize.Invalid
		}
		out = append(out, Tier{MinQty: q, Price: p})
	}
	return normalizeTiers(out)
}

// validateVariants memvalidasi grosir + satuan tambahan ke c; galat dicatat di f (field "wholesale"/"units").
func validateVariants(in Input, c *clean, f FieldErrors) {
	if in.Wholesale != nil {
		cw := &cleanWholesale{outlets: map[uuid.UUID][]Tier{}}
		var code string
		if cw.def, code = parseTiers(in.Wholesale.Default); code != "" {
			f["wholesale"] = code
		}
		for _, o := range in.Wholesale.Outlets {
			oid, ok := optID(o.OutletID)
			if !ok || oid == uuid.Nil {
				f["wholesale"] = sanitize.Invalid
				continue
			}
			if _, dup := cw.outlets[oid]; dup {
				f["wholesale"] = codeDuplicate
				continue
			}
			ts, code := parseTiers(o.Tiers)
			if code != "" {
				f["wholesale"] = code
				continue
			}
			if len(ts) > 0 { // set kosong = tidak punya set sendiri
				cw.outlets[oid] = ts
			}
		}
		c.wholesale = cw
	}

	if in.Units != nil {
		units := make([]cleanUnit, 0, len(*in.Units))
		seen := map[uuid.UUID]bool{}
		codes := map[string]bool{}
		if c.barcode != "" {
			codes[c.barcode] = true
		}
		if len(*in.Units) > maxAltUnits {
			f["units"] = codeTooMany
		}
		for _, u := range *in.Units {
			uid, ok := optID(u.UnitID)
			factor, ok2 := parseDec(u.Factor, 6, decimal.NewFromInt(maxQty))
			if !ok || uid == uuid.Nil || !ok2 || strings.TrimSpace(u.Factor) == "" || !factor.IsPositive() || uid == c.unit {
				f["units"] = sanitize.Invalid
				continue
			}
			if seen[uid] {
				f["units"] = codeDuplicate
				continue
			}
			seen[uid] = true
			bc, code := scanValue(u.Barcode)
			if code != "" {
				f["units"] = code
				continue
			}
			if bc != "" {
				if codes[bc] {
					f["units"] = codeDuplicate
					continue
				}
				codes[bc] = true
			}
			cu := cleanUnit{unit: uid, factor: factor, barcode: bc}
			if strings.TrimSpace(u.SellPrice) != "" {
				p, ok := money(u.SellPrice)
				if !ok {
					f["units"] = sanitize.Invalid
					continue
				}
				cu.price = &p
			}
			units = append(units, cu)
		}
		c.units = &units
	}
}

// ---- baca ----

func groupTiers(rows []gen.ItemTierListRow) map[uuid.UUID][]Tier {
	g := map[uuid.UUID][]Tier{}
	for _, r := range rows {
		key := uuid.Nil // Nil = set default
		if r.OutletID.Valid {
			key = uuid.UUID(r.OutletID.Bytes)
		}
		g[key] = append(g[key], Tier{MinQty: r.MinQty, Price: r.Price})
	}
	return g
}

// loadVariants mengisi grosir (default + set cabang yang boleh dilihat pemanggil) dan satuan tambahan.
func (s *Service) loadVariants(ctx context.Context, q *gen.Queries, a authz.Actor, it *Item) error {
	rows, err := q.ItemTierList(ctx, gen.ItemTierListParams{TenantID: a.TenantID, ItemID: it.ID})
	if err != nil {
		return err
	}
	g := groupTiers(rows)
	if d := g[uuid.Nil]; d != nil {
		it.Wholesale.Default = d
	}
	for _, op := range it.OutletPrices { // outlet yang boleh diakses (urut sama dengan harga cabang)
		if ts := g[op.OutletID]; len(ts) > 0 {
			it.Wholesale.Outlets = append(it.Wholesale.Outlets, OutletTiers{OutletID: op.OutletID, OutletName: op.OutletName, Tiers: ts})
		}
	}
	units, err := q.ItemUnitList(ctx, gen.ItemUnitListParams{TenantID: a.TenantID, ItemID: it.ID})
	if err != nil {
		return err
	}
	for _, u := range units {
		au := AltUnit{UnitID: u.UnitID, UnitName: u.UnitName, Factor: u.Factor, Barcode: u.Barcode.String}
		if p, ok := numeric(u.SellPrice); ok {
			au.SellPrice = &p
		}
		it.Units = append(it.Units, au)
	}
	return nil
}

// ---- tulis ----

func toNumeric(d *decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	if d != nil {
		_ = n.Scan(d.String())
	}
	return n
}

// checkVariantOutlets: set grosir hanya boleh diatur untuk cabang yang dapat diakses pemanggil.
func checkVariantOutlets(a authz.Actor, c clean) error {
	if c.wholesale == nil {
		return nil
	}
	for oid := range c.wholesale.outlets {
		if !a.Outlets[oid] {
			return ErrOutletForbidden
		}
	}
	return nil
}

// lockAndCheckBarcodes memastikan setiap barcode (barcode item + barcode satuan tambahan) belum dipakai item LAIN,
// lintas tabel items/item_units. Kunci advisory per (tenant, barcode), diambil terurut agar tidak deadlock, membuat
// pemeriksaan ini aman dari dua transaksi bersamaan yang membawa barcode sama. itemID = uuid.Nil saat item baru.
func lockAndCheckBarcodes(ctx context.Context, q *gen.Queries, tenant, itemID uuid.UUID, codes []string) error {
	uniq := slices.Compact(slices.Sorted(slices.Values(slices.DeleteFunc(slices.Clone(codes), func(s string) bool { return s == "" }))))
	for _, code := range uniq {
		if err := q.ItemBarcodeLock(ctx, tenant.String()+":"+code); err != nil {
			return err
		}
	}
	for _, code := range uniq {
		taken, err := q.ItemBarcodeTaken(ctx, gen.ItemBarcodeTakenParams{TenantID: tenant, Barcode: code, ItemID: itemID})
		if err != nil {
			return err
		}
		if taken.Bool {
			return ErrBarcodeTaken
		}
	}
	return nil
}

func barcodesOf(c clean) []string {
	codes := []string{c.barcode}
	if c.units != nil {
		for _, u := range *c.units {
			codes = append(codes, u.barcode)
		}
	}
	return codes
}

// checkAltUnitRefs: satuan tambahan yang BARU harus ada dan aktif; yang sudah terpasang di item ini (existing)
// boleh tetap walau kemudian diarsipkan.
func checkAltUnitRefs(ctx context.Context, q *gen.Queries, tenant uuid.UUID, units []cleanUnit, existing map[uuid.UUID]bool) error {
	var want []uuid.UUID
	for _, u := range units {
		if !existing[u.unit] {
			want = append(want, u.unit)
		}
	}
	if len(want) == 0 {
		return nil
	}
	rows, err := q.ItemUnitStates(ctx, gen.ItemUnitStatesParams{TenantID: tenant, Ids: want})
	if err != nil {
		return err
	}
	active := map[uuid.UUID]bool{}
	for _, r := range rows {
		active[r.ID] = r.Active
	}
	for _, id := range want {
		if !active[id] {
			return FieldErrors{"units": sanitize.Invalid}
		}
	}
	return nil
}

func tierString(ts []Tier) string {
	parts := make([]string, 0, len(ts))
	for _, t := range ts {
		parts = append(parts, t.MinQty.String()+":"+t.Price.StringFixed(2))
	}
	return strings.Join(parts, ",")
}

func unitsString(us []AltUnit) string {
	parts := make([]string, 0, len(us))
	for _, u := range us {
		price := ""
		if u.SellPrice != nil {
			price = u.SellPrice.StringFixed(2)
		}
		parts = append(parts, fmt.Sprintf("%s*%s|%s|%s", u.UnitID, u.Factor.String(), u.Barcode, price))
	}
	return strings.Join(parts, ";")
}

// writeVariants menerapkan grosir/satuan tambahan yang dikirim (nil = tidak disentuh) dan mencatat audit perubahannya.
// Dipanggil di dalam transaksi setelah baris item ada. Barcode sudah dikunci/diperiksa oleh pemanggil.
func (s *Service) writeVariants(ctx context.Context, tx pgx.Tx, a authz.Actor, itemID uuid.UUID, sku, name string, c clean) error {
	q := gen.New(tx)

	if c.wholesale != nil {
		rows, err := q.ItemTierList(ctx, gen.ItemTierListParams{TenantID: a.TenantID, ItemID: itemID})
		if err != nil {
			return err
		}
		old := groupTiers(rows)
		if err := q.ItemTierDeleteDefault(ctx, gen.ItemTierDeleteDefaultParams{TenantID: a.TenantID, ItemID: itemID}); err != nil {
			return err
		}
		if err := q.ItemTierDeleteOutlets(ctx, gen.ItemTierDeleteOutletsParams{TenantID: a.TenantID, ItemID: itemID, OutletIds: outletIDs(a)}); err != nil {
			return err
		}
		insert := func(outlet uuid.UUID, ts []Tier) error {
			for _, t := range ts {
				if err := q.ItemTierInsert(ctx, gen.ItemTierInsertParams{TenantID: a.TenantID, ItemID: itemID, OutletID: nz(outlet), MinQty: t.MinQty, Price: t.Price}); err != nil {
					return err
				}
			}
			return nil
		}
		if err := insert(uuid.Nil, c.wholesale.def); err != nil {
			return err
		}
		for oid, ts := range c.wholesale.outlets {
			if err := insert(oid, ts); err != nil {
				return err
			}
		}

		before, after := map[string]any{}, map[string]any{}
		note := func(key string, o, n []Tier) {
			if tierString(o) != tierString(n) {
				before[key], after[key] = tierString(o), tierString(n)
			}
		}
		note("default", old[uuid.Nil], c.wholesale.def)
		for oid := range a.Outlets { // hanya cabang yang boleh dikelola pemanggil yang berubah
			note(oid.String(), old[oid], c.wholesale.outlets[oid])
		}
		if len(before) > 0 {
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
				Action: audit.ActionItemPrice, Entity: audit.EntityItem, EntityID: itemID.String(),
				Details: map[string]any{"sku": sku, "name": name, "wholesale_before": before, "wholesale_after": after},
			}); err != nil {
				return err
			}
		}
	}

	if c.units != nil {
		oldRows, err := q.ItemUnitList(ctx, gen.ItemUnitListParams{TenantID: a.TenantID, ItemID: itemID})
		if err != nil {
			return err
		}
		existing := map[uuid.UUID]bool{}
		var oldUnits []AltUnit
		for _, r := range oldRows {
			existing[r.UnitID] = true
			au := AltUnit{UnitID: r.UnitID, Factor: r.Factor, Barcode: r.Barcode.String}
			if p, ok := numeric(r.SellPrice); ok {
				au.SellPrice = &p
			}
			oldUnits = append(oldUnits, au)
		}
		if err := checkAltUnitRefs(ctx, q, a.TenantID, *c.units, existing); err != nil {
			return err
		}
		if err := q.ItemUnitDeleteAll(ctx, gen.ItemUnitDeleteAllParams{TenantID: a.TenantID, ItemID: itemID}); err != nil {
			return err
		}
		var newUnits []AltUnit
		for i, u := range *c.units {
			if err := q.ItemUnitInsert(ctx, gen.ItemUnitInsertParams{TenantID: a.TenantID, ItemID: itemID, UnitID: u.unit, Factor: u.factor,
				Barcode: pgtype.Text{String: u.barcode, Valid: u.barcode != ""}, SellPrice: toNumeric(u.price), Position: int32(i + 1)}); err != nil {
				return err
			}
			newUnits = append(newUnits, AltUnit{UnitID: u.unit, Factor: u.factor, Barcode: u.barcode, SellPrice: u.price})
		}
		sortUnits := func(us []AltUnit) {
			sort.Slice(us, func(i, j int) bool { return us[i].UnitID.String() < us[j].UnitID.String() })
		}
		sortUnits(oldUnits)
		sortUnits(newUnits)
		if unitsString(oldUnits) != unitsString(newUnits) {
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
				Action: audit.ActionItemUpdate, Entity: audit.EntityItem, EntityID: itemID.String(),
				Details: map[string]any{"sku": sku, "name": name, "units_before": unitsString(oldUnits), "units_after": unitsString(newUnits)},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkBaseUnitConflict: satuan dasar item tidak boleh sama dengan salah satu satuan tambahannya. Bila daftar satuan
// ikut dikirim, validateVariants sudah memeriksanya; bila tidak, satuan tambahan yang tersimpan diperiksa di sini.
func checkBaseUnitConflict(ctx context.Context, q *gen.Queries, tenant, itemID uuid.UUID, c clean) error {
	if c.units != nil {
		return nil
	}
	rows, err := q.ItemUnitList(ctx, gen.ItemUnitListParams{TenantID: tenant, ItemID: itemID})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.UnitID == c.unit {
			return FieldErrors{"unit_id": sanitize.Invalid}
		}
	}
	return nil
}
