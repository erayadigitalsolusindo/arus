// Package item: master Daftar Item per tenant — data barang dan harga jual per cabang (Fase 3.2, irisan B).
// Item tidak pernah dihapus permanen; dinonaktifkan agar transaksi yang merujuknya tetap utuh.
package item

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"golang.org/x/text/unicode/norm"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/platform/storage"
)

var (
	ErrNotFound        = errors.New("item tidak ditemukan")
	ErrCodeTaken       = errors.New("kode barang sudah dipakai")
	ErrOutletForbidden = errors.New("tidak punya akses ke outlet ini")
)

// FieldErrors = kode galat per field (REQUIRED/INVALID/TOO_LONG), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

const (
	maxSKU     = 40
	maxName    = 200
	maxBarcode = 200
	maxDesc    = 5000
	defLimit   = 20
	maxLimit   = 100

	kindGoods   = "goods"
	kindService = "service"
)

var (
	skuRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,39}$`)
	maxMoney = decimal.New(1, 12) // 1 triliun
	maxGrams = decimal.New(1, 8)  // 100 ton
)

// Ref = master yang dirujuk item (id + nama untuk ditampilkan).
type Ref struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// OutletPrice = harga khusus satu cabang; SellPrice nil = cabang memakai harga default.
type OutletPrice struct {
	OutletID   uuid.UUID        `json:"outlet_id"`
	OutletName string           `json:"outlet_name"`
	SellPrice  *decimal.Decimal `json:"sell_price"`
}

type Item struct {
	ID                 uuid.UUID     `json:"id"`
	SKU                string        `json:"sku"`
	Barcode            string        `json:"barcode"`
	Origin             string        `json:"origin"`
	Name               string        `json:"name"`
	WeightGrams        string        `json:"weight_grams"`
	LastCost           string        `json:"last_cost"`
	AvgCost            string        `json:"avg_cost"`
	SellPrice          string        `json:"sell_price"`
	Kind               string        `json:"kind"`
	AllowNegativeStock bool          `json:"allow_negative_stock"`
	SellBelowCost      bool          `json:"sell_below_cost"`
	Description        string        `json:"description"`
	Active             bool          `json:"active"`
	Unit               Ref           `json:"unit"`
	Category           *Ref          `json:"category"`
	Brand              *Ref          `json:"brand"`
	Principal          *Ref          `json:"principal"`
	Supplier           *Ref          `json:"supplier"`
	OutletPrices       []OutletPrice `json:"outlet_prices"`
	Images             []Image       `json:"images"`
	Wholesale          Wholesale     `json:"wholesale"`
	Units              []AltUnit     `json:"units"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// Row = baris daftar item. Price = harga efektif untuk outlet aktif sesi (harga cabang bila ada, selain itu default).
type Row struct {
	ID            uuid.UUID  `json:"id"`
	SKU           string     `json:"sku"`
	Barcode       string     `json:"barcode"`
	Origin        string     `json:"origin"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Active        bool       `json:"active"`
	Unit          string     `json:"unit"`
	Category      string     `json:"category"`
	Brand         string     `json:"brand"`
	Price         string     `json:"price"`
	PriceOverride bool       `json:"price_override"`
	AvgCost       string     `json:"avg_cost"`  // Harga rata (HPP rata-rata)
	LastCost      string     `json:"last_cost"` // Harga beli akhir
	MainImageID   *uuid.UUID `json:"main_image_id"`
	// Stok outlet aktif sesi per bucket, dalam satuan dasar. Item jasa tidak punya stok (klien menampilkan "-").
	Stock StockQty `json:"stock"`
}

// StockQty = stok satu item per bucket (string desimal).
type StockQty struct {
	Display   string `json:"display"`
	Warehouse string `json:"warehouse"`
	Returns   string `json:"returns"`
	Total     string `json:"total"`
}

// OutletPriceInput = harga khusus satu cabang dari klien (string desimal).
type OutletPriceInput struct {
	OutletID  string
	SellPrice string
}

// Input untuk membuat/mengubah item. Id master kosong ("") = tidak diisi (kecuali UnitID, wajib).
// Cost (HPP awal) hanya dipakai saat membuat. OutletPrices nil saat mengubah = harga cabang tidak disentuh;
// selain itu daftar ini menggantikan seluruh harga khusus pada cabang yang boleh diakses pemanggil.
type Input struct {
	SKU     string
	Barcode string
	// Origin = pembeda opsional (mis. negara asal) yang tampil di pilihan kasir untuk barcode kembar.
	Origin             string
	Name               string
	Weight             string
	Cost               string
	SellPrice          string
	UnitID             string
	CategoryID         string
	BrandID            string
	PrincipalID        string
	SupplierID         string
	Kind               string
	AllowNegativeStock bool
	SellBelowCost      bool
	Description        string
	OutletPrices       *[]OutletPriceInput
	// Wholesale nil = grosir tidak disentuh; selain itu menggantikan set default dan set cabang (yang boleh diakses pemanggil).
	Wholesale *WholesaleInput
	// Units nil = satuan tambahan tidak disentuh; selain itu daftar ini menggantikan seluruhnya.
	Units *[]UnitInput
}

type ListParams struct {
	Q          string
	Active     *bool
	CategoryID string
	Limit      int
	Offset     int
}

type clean struct {
	sku, name, barcode, origin, kind, desc string
	weight, cost, sell                     decimal.Decimal
	unit                                   uuid.UUID
	category, brand, principal             uuid.UUID
	supplier                               uuid.UUID
	prices                                 map[uuid.UUID]decimal.Decimal // nil = tidak disentuh
	wholesale                              *cleanWholesale               // nil = tidak disentuh
	units                                  *[]cleanUnit                  // nil = tidak disentuh
}

type Service struct {
	pool  *pgxpool.Pool
	store storage.Store // penyimpanan gambar; nil = fitur gambar mati (ErrNoStorage)
}

func NewService(pool *pgxpool.Pool, store storage.Store) *Service {
	return &Service{pool: pool, store: store}
}

// ---- validasi ----

func parseDec(s string, maxFrac int32, max decimal.Decimal) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, true
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || d.GreaterThan(max) || d.Exponent() < -maxFrac {
		return decimal.Zero, false
	}
	return d, true
}

func money(s string) (decimal.Decimal, bool) { return parseDec(s, 2, maxMoney) }
func grams(s string) (decimal.Decimal, bool) { return parseDec(s, 3, maxGrams) }
func optID(s string) (uuid.UUID, bool) {
	if strings.TrimSpace(s) == "" {
		return uuid.Nil, true
	}
	id, err := uuid.Parse(strings.TrimSpace(s))
	return id, err == nil
}

// multiline membersihkan teks bebas (markdown): NFC, CRLF→LF, tab/baris baru diizinkan, karakter kontrol,
// format tak terlihat (zero-width, bidi) dan karakter pengganti ditolak. Teks TIDAK di-trim per baris.
func multiline(raw string, max int) (string, string) {
	if !utf8.ValidString(raw) {
		return "", sanitize.Invalid
	}
	s := norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r), r == unicode.ReplacementChar:
			return "", sanitize.Invalid
		}
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", sanitize.TooLong
	}
	return s, ""
}

// scanValue memvalidasi nilai pemindai (barcode/QR): apa adanya (spasi di tengah boleh), tanpa karakter
// kontrol/format tak terlihat.
func scanValue(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if !utf8.ValidString(s) {
		return "", sanitize.Invalid
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) || r == unicode.ReplacementChar {
			return "", sanitize.Invalid
		}
	}
	if utf8.RuneCountInString(s) > maxBarcode {
		return "", sanitize.TooLong
	}
	return s, ""
}

func validate(in Input, creating bool) (clean, FieldErrors) {
	f := FieldErrors{}
	var c clean
	var code string

	if c.name, code = sanitize.Name(in.Name, maxName); code != "" {
		f["name"] = code
	}
	if sku := strings.TrimSpace(in.SKU); sku == "" {
		if !creating {
			f["sku"] = sanitize.Required // saat mengubah, kode wajib ada (kosong = otomatis hanya untuk item baru)
		}
	} else if !skuRe.MatchString(sku) {
		f["sku"] = sanitize.Invalid
	} else {
		c.sku = sku
	}
	if c.barcode, code = scanValue(in.Barcode); code != "" {
		f["barcode"] = code
	}
	if c.origin, code = originValue(in.Origin); code != "" {
		f["origin"] = code
	}

	var ok bool
	if c.weight, ok = grams(in.Weight); !ok {
		f["weight"] = sanitize.Invalid
	}
	if c.sell, ok = money(in.SellPrice); !ok {
		f["sell_price"] = sanitize.Invalid
	}
	if creating {
		if c.cost, ok = money(in.Cost); !ok {
			f["cost"] = sanitize.Invalid
		}
	}

	if id, ok := optID(in.UnitID); !ok || id == uuid.Nil {
		f["unit_id"] = sanitize.Required
		if !ok {
			f["unit_id"] = sanitize.Invalid
		}
	} else {
		c.unit = id
	}
	for field, pair := range map[string]struct {
		raw string
		dst *uuid.UUID
	}{
		"category_id":  {in.CategoryID, &c.category},
		"brand_id":     {in.BrandID, &c.brand},
		"principal_id": {in.PrincipalID, &c.principal},
		"supplier_id":  {in.SupplierID, &c.supplier},
	} {
		id, ok := optID(pair.raw)
		if !ok {
			f[field] = sanitize.Invalid
			continue
		}
		*pair.dst = id
	}

	switch k := strings.TrimSpace(in.Kind); k {
	case "", kindGoods:
		c.kind = kindGoods
	case kindService:
		c.kind = kindService
	default:
		f["kind"] = sanitize.Invalid
	}
	if c.desc, code = multiline(in.Description, maxDesc); code != "" {
		f["description"] = code
	}

	if in.OutletPrices != nil {
		c.prices = map[uuid.UUID]decimal.Decimal{}
		for _, p := range *in.OutletPrices {
			oid, ok := optID(p.OutletID)
			price, ok2 := money(p.SellPrice)
			switch {
			case !ok || oid == uuid.Nil || !ok2 || strings.TrimSpace(p.SellPrice) == "":
				f["outlet_prices"] = sanitize.Invalid
			default:
				if _, dup := c.prices[oid]; dup {
					f["outlet_prices"] = sanitize.Invalid
				}
				c.prices[oid] = price
			}
		}
	}
	validateVariants(in, &c, f)
	if len(f) > 0 {
		return clean{}, f
	}
	return c, nil
}

// ---- konversi ----

func nz(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

func ref(id pgtype.UUID, name pgtype.Text) *Ref {
	if !id.Valid {
		return nil
	}
	return &Ref{ID: uuid.UUID(id.Bytes), Name: name.String}
}

func numeric(n pgtype.Numeric) (decimal.Decimal, bool) {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero, false
	}
	return decimal.NewFromBigInt(n.Int, n.Exp), true
}

func itemOf(r gen.ItemGetRow) Item {
	return Item{
		ID: r.ID, SKU: r.Sku, Barcode: r.Barcode.String, Origin: r.Origin, Name: r.Name,
		WeightGrams: r.WeightGrams.String(), LastCost: r.LastCost.StringFixed(2), AvgCost: r.AvgCost.StringFixed(2), SellPrice: r.SellPrice.StringFixed(2),
		Kind: r.Kind, AllowNegativeStock: r.AllowNegativeStock, SellBelowCost: r.SellBelowCost, Description: r.Description, Active: r.Active,
		Unit:     Ref{ID: r.UnitID, Name: r.UnitName},
		Category: ref(r.CategoryID, r.CategoryName), Brand: ref(r.BrandID, r.BrandName),
		Principal: ref(r.PrincipalID, r.PrincipalName), Supplier: ref(r.SupplierID, r.SupplierName),
		OutletPrices: []OutletPrice{}, Images: []Image{}, Units: []AltUnit{}, Wholesale: Wholesale{Default: []Tier{}, Outlets: []OutletTiers{}},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func outletIDs(a authz.Actor) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(a.Outlets))
	for id := range a.Outlets {
		ids = append(ids, id)
	}
	return ids
}

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case uniqueViolation(err, "items_tenant_sku_key"):
		return ErrCodeTaken
	case errors.As(err, &pgErr) && pgErr.Code == "23503": // master dirujuk hilang karena balapan hapus/tenant lain
		field := map[string]string{"items_unit_fk": "unit_id", "items_category_fk": "category_id", "items_brand_fk": "brand_id",
			"items_principal_fk": "principal_id", "items_supplier_fk": "supplier_id", "item_units_unit_fk": "units"}[pgErr.ConstraintName]
		if field != "" {
			return FieldErrors{field: sanitize.Invalid}
		}
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

// ---- baca ----

func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) ([]Row, int, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || utf8.RuneCountInString(q) > maxName {
		return nil, 0, FieldErrors{"q": sanitize.Invalid}
	}
	cat, ok := optID(p.CategoryID)
	if !ok {
		return nil, 0, FieldErrors{"category_id": sanitize.Invalid}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	arg := gen.ItemListParams{
		TenantID: a.TenantID, OutletID: a.OutletID, Q: likeEscape(q), CategoryID: nz(cat),
		PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(p.Offset, 0)),
	}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Row{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).ItemList(ctx, arg)
		for _, r := range rows {
			price, override := r.DefaultPrice, false
			if op, ok := numeric(r.OutletPrice); ok {
				price, override = op, true
			}
			out = append(out, Row{ID: r.ID, SKU: r.Sku, Barcode: r.Barcode.String, Origin: r.Origin, Name: r.Name, Kind: r.Kind, Active: r.Active,
				Unit: r.UnitName, Category: r.CategoryName.String, Brand: r.BrandName.String,
				Price: price.StringFixed(2), PriceOverride: override, MainImageID: uuidPtr(r.MainImageID),
				AvgCost: r.AvgCost.StringFixed(2), LastCost: r.LastCost.StringFixed(2),
				Stock: StockQty{Display: r.StockDisplay.String(), Warehouse: r.StockWarehouse.String(), Returns: r.StockReturns.String(),
					Total: r.StockDisplay.Add(r.StockWarehouse).Add(r.StockReturns).String()}})
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

// likeEscape membuat input pengguna dicocokkan apa adanya oleh ILIKE (karakter % _ \ tidak jadi wildcard).
func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(s)
}

func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (*Item, error) {
	var it Item
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		it, err = s.load(ctx, tx, a, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &it, nil
}

// load membaca item + harga cabang yang boleh dilihat pemanggil (di dalam transaksi pemanggil).
func (s *Service) load(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID) (Item, error) {
	q := gen.New(tx)
	r, err := q.ItemGet(ctx, gen.ItemGetParams{TenantID: a.TenantID, ID: id})
	if err != nil {
		return Item{}, err
	}
	it := itemOf(r)
	prices, err := q.ItemOutletPrices(ctx, gen.ItemOutletPricesParams{TenantID: a.TenantID, ItemID: id, OutletIds: outletIDs(a)})
	if err != nil {
		return Item{}, err
	}
	for _, p := range prices {
		op := OutletPrice{OutletID: p.OutletID, OutletName: p.OutletName}
		if d, ok := numeric(p.SellPrice); ok {
			op.SellPrice = &d
		}
		it.OutletPrices = append(it.OutletPrices, op)
	}
	if it.Images, err = s.listImages(ctx, q, a, id); err != nil {
		return Item{}, err
	}
	if err := s.loadVariants(ctx, q, a, &it); err != nil {
		return Item{}, err
	}
	return it, nil
}

// ---- tulis ----

// checkRefs memastikan master yang baru dirujuk ada dan aktif. Master yang tidak berubah (sudah dirujuk item ini)
// tidak diperiksa: item boleh tetap menunjuk master yang kemudian diarsipkan.
func checkRefs(ctx context.Context, q *gen.Queries, tenant uuid.UUID, c clean, cur *gen.ItemGetForUpdateRow) error {
	want := map[string]uuid.UUID{"unit_id": c.unit, "category_id": c.category, "brand_id": c.brand, "principal_id": c.principal, "supplier_id": c.supplier}
	if cur != nil {
		same := map[string]pgtype.UUID{"category_id": cur.CategoryID, "brand_id": cur.BrandID, "principal_id": cur.PrincipalID, "supplier_id": cur.SupplierID}
		for k, v := range same {
			if v.Valid && uuid.UUID(v.Bytes) == want[k] {
				want[k] = uuid.Nil
			}
		}
		if cur.UnitID == want["unit_id"] {
			want["unit_id"] = uuid.Nil
		}
	}
	st, err := q.ItemRefState(ctx, gen.ItemRefStateParams{TenantID: tenant, UnitID: nz(want["unit_id"]), CategoryID: nz(want["category_id"]),
		BrandID: nz(want["brand_id"]), PrincipalID: nz(want["principal_id"]), SupplierID: nz(want["supplier_id"])})
	if err != nil {
		return err
	}
	f := FieldErrors{}
	for field, state := range map[string]string{"unit_id": st.UnitState, "category_id": st.CategoryState, "brand_id": st.BrandState,
		"principal_id": st.PrincipalState, "supplier_id": st.SupplierState} {
		if want[field] != uuid.Nil && state != "active" {
			f[field] = sanitize.Invalid
		}
	}
	if len(f) > 0 {
		return f
	}
	return nil
}

func (s *Service) checkOutlets(a authz.Actor, prices map[uuid.UUID]decimal.Decimal) error {
	for id := range prices {
		if !a.Outlets[id] {
			return ErrOutletForbidden
		}
	}
	return nil
}

func (s *Service) Create(ctx context.Context, a authz.Actor, in Input) (*Item, error) {
	c, f := validate(in, true)
	if f != nil {
		return nil, f
	}
	if err := s.checkOutlets(a, c.prices); err != nil {
		return nil, err
	}
	if err := checkVariantOutlets(a, c); err != nil {
		return nil, err
	}
	var it Item
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := checkRefs(ctx, q, a.TenantID, c, nil); err != nil {
			return err
		}
		sku := c.sku
		if sku == "" {
			var err error
			if sku, err = nextSKU(ctx, q, a.TenantID); err != nil {
				return err
			}
		}
		id, err := q.ItemCreate(ctx, gen.ItemCreateParams{
			TenantID: a.TenantID, Sku: sku, Barcode: pgtype.Text{String: c.barcode, Valid: c.barcode != ""}, Name: c.name, Origin: c.origin,
			WeightGrams: c.weight, Cost: c.cost, SellPrice: c.sell, UnitID: c.unit,
			CategoryID: nz(c.category), BrandID: nz(c.brand), PrincipalID: nz(c.principal), SupplierID: nz(c.supplier),
			Kind: c.kind, AllowNegativeStock: in.AllowNegativeStock, SellBelowCost: in.SellBelowCost, Description: c.desc,
		})
		if err != nil {
			return err
		}
		for oid, price := range c.prices {
			if err := q.ItemOutletPriceUpsert(ctx, gen.ItemOutletPriceUpsertParams{TenantID: a.TenantID, ItemID: id, OutletID: oid, SellPrice: price}); err != nil {
				return err
			}
		}
		if err := s.writeVariants(ctx, tx, a, id, sku, c.name, c); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemCreate, Entity: audit.EntityItem, EntityID: id.String(),
			Details: map[string]any{"sku": sku, "name": c.name, "sell_price": c.sell.StringFixed(2), "outlet_prices": len(c.prices)},
		}); err != nil {
			return err
		}
		it, err = s.load(ctx, tx, a, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &it, nil
}

// nextSKU membuat kode otomatis ITM-000001, … Nomor berikutnya diambil atomik; nomor yang sudah dipakai kode manual dilewati.
func nextSKU(ctx context.Context, q *gen.Queries, tenant uuid.UUID) (string, error) {
	for range 100 {
		n, err := q.ItemNextNo(ctx, tenant)
		if err != nil {
			return "", err
		}
		sku := fmt.Sprintf("ITM-%06d", n)
		taken, err := q.ItemSkuExists(ctx, gen.ItemSkuExistsParams{TenantID: tenant, Lower: sku})
		if err != nil {
			return "", err
		}
		if !taken {
			return sku, nil
		}
	}
	return "", errors.New("gagal membuat kode barang otomatis")
}

func (s *Service) Update(ctx context.Context, a authz.Actor, id uuid.UUID, in Input) (*Item, error) {
	c, f := validate(in, false)
	if f != nil {
		return nil, f
	}
	if err := s.checkOutlets(a, c.prices); err != nil {
		return nil, err
	}
	if err := checkVariantOutlets(a, c); err != nil {
		return nil, err
	}
	var it Item
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.ItemGetForUpdate(ctx, gen.ItemGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if err := checkRefs(ctx, q, a.TenantID, c, &cur); err != nil {
			return err
		}
		if err := checkBaseUnitConflict(ctx, q, a.TenantID, id, c); err != nil {
			return err
		}
		beforePrices, err := q.ItemOutletPrices(ctx, gen.ItemOutletPricesParams{TenantID: a.TenantID, ItemID: id, OutletIds: outletIDs(a)})
		if err != nil {
			return err
		}
		if err := q.ItemUpdate(ctx, gen.ItemUpdateParams{
			TenantID: a.TenantID, ID: id, Sku: c.sku, Barcode: pgtype.Text{String: c.barcode, Valid: c.barcode != ""}, Name: c.name, Origin: c.origin,
			WeightGrams: c.weight, SellPrice: c.sell, UnitID: c.unit,
			CategoryID: nz(c.category), BrandID: nz(c.brand), PrincipalID: nz(c.principal), SupplierID: nz(c.supplier),
			Kind: c.kind, AllowNegativeStock: in.AllowNegativeStock, SellBelowCost: in.SellBelowCost, Description: c.desc,
		}); err != nil {
			return err
		}
		if c.prices != nil {
			if err := q.ItemOutletPriceDelete(ctx, gen.ItemOutletPriceDeleteParams{TenantID: a.TenantID, ItemID: id, OutletIds: outletIDs(a)}); err != nil {
				return err
			}
			for oid, price := range c.prices {
				if err := q.ItemOutletPriceUpsert(ctx, gen.ItemOutletPriceUpsertParams{TenantID: a.TenantID, ItemID: id, OutletID: oid, SellPrice: price}); err != nil {
					return err
				}
			}
		}
		if err := recordUpdate(ctx, tx, a, id, cur, c, in, beforePrices); err != nil {
			return err
		}
		if err := s.writeVariants(ctx, tx, a, id, c.sku, c.name, c); err != nil {
			return err
		}
		it, err = s.load(ctx, tx, a, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &it, nil
}

// recordUpdate mencatat perubahan: `item.update` untuk data barang (hanya kolom yang berubah) dan `item.price`
// untuk harga default/cabang — terpisah agar riwayat harga mudah difilter di audit log.
func recordUpdate(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID, cur gen.ItemGetForUpdateRow, c clean, in Input, beforePrices []gen.ItemOutletPricesRow) error {
	before, after := map[string]any{}, map[string]any{}
	diff := func(k string, b, n any) {
		if fmt.Sprint(b) != fmt.Sprint(n) {
			before[k], after[k] = b, n
		}
	}
	uid := func(p pgtype.UUID) string {
		if !p.Valid {
			return ""
		}
		return uuid.UUID(p.Bytes).String()
	}
	nid := func(u uuid.UUID) string {
		if u == uuid.Nil {
			return ""
		}
		return u.String()
	}
	diff("sku", cur.Sku, c.sku)
	diff("barcode", cur.Barcode.String, c.barcode)
	diff("name", cur.Name, c.name)
	diff("origin", cur.Origin, c.origin)
	diff("weight_grams", cur.WeightGrams.String(), c.weight.String())
	diff("unit_id", cur.UnitID.String(), c.unit.String())
	diff("category_id", uid(cur.CategoryID), nid(c.category))
	diff("brand_id", uid(cur.BrandID), nid(c.brand))
	diff("principal_id", uid(cur.PrincipalID), nid(c.principal))
	diff("supplier_id", uid(cur.SupplierID), nid(c.supplier))
	diff("kind", cur.Kind, c.kind)
	diff("allow_negative_stock", cur.AllowNegativeStock, in.AllowNegativeStock)
	diff("sell_below_cost", cur.SellBelowCost, in.SellBelowCost)
	if len(before) > 0 {
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemUpdate, Entity: audit.EntityItem, EntityID: id.String(),
			Details: map[string]any{"sku": c.sku, "name": c.name, "before": before, "after": after},
		}); err != nil {
			return err
		}
	}

	pb, pa := map[string]any{}, map[string]any{}
	if !cur.SellPrice.Equal(c.sell) {
		pb["default"], pa["default"] = cur.SellPrice.StringFixed(2), c.sell.StringFixed(2)
	}
	if c.prices != nil {
		old := map[uuid.UUID]string{}
		for _, p := range beforePrices {
			if d, ok := numeric(p.SellPrice); ok {
				old[p.OutletID] = d.StringFixed(2)
			}
		}
		for oid := range a.Outlets {
			o, hadOld := old[oid]
			n, hasNew := c.prices[oid]
			switch {
			case hadOld && (!hasNew || !n.Equal(decimal.RequireFromString(o))):
				pb[oid.String()] = o
				if hasNew {
					pa[oid.String()] = n.StringFixed(2)
				} else {
					pa[oid.String()] = nil
				}
			case !hadOld && hasNew:
				pb[oid.String()], pa[oid.String()] = nil, n.StringFixed(2)
			}
		}
	}
	if len(pb) > 0 {
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemPrice, Entity: audit.EntityItem, EntityID: id.String(),
			Details: map[string]any{"sku": c.sku, "name": c.name, "before": pb, "after": pa},
		})
	}
	return nil
}

// SetActive mengarsipkan (false) atau mengaktifkan kembali (true) item.
func (s *Service) SetActive(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) (*Item, error) {
	var it Item
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		r, err := q.ItemSetActive(ctx, gen.ItemSetActiveParams{TenantID: a.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemActive, Entity: audit.EntityItem, EntityID: id.String(),
			Details: map[string]any{"sku": r.Sku, "name": r.Name, "active": active},
		}); err != nil {
			return err
		}
		it, err = s.load(ctx, tx, a, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &it, nil
}

func uuidPtr(p pgtype.UUID) *uuid.UUID {
	if !p.Valid {
		return nil
	}
	id := uuid.UUID(p.Bytes)
	return &id
}

const maxOrigin = 100

// originValue memvalidasi pembeda opsional (mis. negara asal): teks polos ≤ 100 karakter tanpa markup; kosong boleh.
func originValue(raw string) (string, string) {
	v, ok := sanitize.Text(raw)
	switch {
	case !ok, strings.ContainsAny(v, "<>"):
		return "", sanitize.Invalid
	case utf8.RuneCountInString(v) > maxOrigin:
		return "", sanitize.TooLong
	}
	return v, ""
}
