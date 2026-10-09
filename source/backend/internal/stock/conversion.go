package stock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Pecah satuan (Fase 4.3): memindahkan stok antar dua barang di outlet aktif, pada bucket display. Qty keduanya dalam
// satuan dasar masing-masing barang dan diisi pengguna (bukan dari rasio tetap), sehingga selisih/susut ikut terwakili.

var (
	ErrKeyRequired    = errors.New("idempotency key wajib")
	ErrKeyMismatch    = errors.New("idempotency key dipakai untuk permintaan berbeda")
	ErrOutletInactive = errors.New("outlet tidak aktif")

	idemKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// maxNote = batas catatan (markdown, boleh beberapa baris).
const maxNote = 1000

// ConvertInput = permintaan pecah satuan. Qty = string desimal > 0, maks 3 desimal.
type ConvertInput struct {
	FromItemID uuid.UUID `json:"from_item_id"`
	FromQty    string    `json:"from_qty"`
	ToItemID   uuid.UUID `json:"to_item_id"`
	ToQty      string    `json:"to_qty"`
	Note       string    `json:"note"`
}

type ConvItem struct {
	ID   uuid.UUID `json:"id"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
	Unit string    `json:"unit"`
	Qty  string    `json:"qty"`
}

// Conversion = satu dokumen pecah satuan.
type Conversion struct {
	ID           uuid.UUID `json:"id"`
	DocNo        string    `json:"doc_no"`
	From         ConvItem  `json:"from"`
	To           ConvItem  `json:"to"`
	FromUnitCost string    `json:"from_unit_cost"`
	ToUnitCost   string    `json:"to_unit_cost"`
	CostApplied  bool      `json:"cost_applied"`
	Note         string    `json:"note"`
	Actor        string    `json:"actor"`
	CreatedAt    time.Time `json:"created_at"`
}

type convNorm struct {
	from, to   uuid.UUID
	fromQ, toQ decimal.Decimal
	note       string
}

func (n convNorm) hash() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", n.from, n.fromQ, n.to, n.toQ, n.note)))
	return hex.EncodeToString(sum[:])
}

func parseQty(s string) (decimal.Decimal, string) {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero, sanitize.Required
	}
	q, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil || !q.IsPositive() || !q.Equal(q.Round(3)) || q.GreaterThanOrEqual(maxQty) {
		return decimal.Zero, sanitize.Invalid
	}
	return q, ""
}

func normalizeConv(in ConvertInput) (convNorm, FieldErrors) {
	f := FieldErrors{}
	n := convNorm{from: in.FromItemID, to: in.ToItemID}
	var c string
	if n.fromQ, c = parseQty(in.FromQty); c != "" {
		f["from_qty"] = c
	}
	if n.toQ, c = parseQty(in.ToQty); c != "" {
		f["to_qty"] = c
	}
	if n.from == uuid.Nil {
		f["from_item_id"] = sanitize.Required
	}
	if n.to == uuid.Nil {
		f["to_item_id"] = sanitize.Required
	} else if n.to == n.from {
		f["to_item_id"] = "SAME_ITEM" // barang asal dan tujuan harus berbeda
	}
	note, code := sanitize.Multiline(in.Note, maxNote)
	if code != "" {
		f["note"] = code
	}
	n.note = note
	if len(f) > 0 {
		return n, f
	}
	return n, nil
}

type convReplay struct{ id uuid.UUID }

func (convReplay) Error() string { return "replay" }

// Convert membuat satu dokumen pecah satuan di outlet aktif. replayed=true bila Idempotency-Key yang sama sudah pernah
// dipakai dengan isi yang sama (dokumen lama dikembalikan, stok tidak berubah lagi).
func (s *Service) Convert(ctx context.Context, a authz.Actor, key string, in ConvertInput) (conv Conversion, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Conversion{}, false, ErrKeyRequired
	}
	n, f := normalizeConv(in)
	if f != nil {
		return Conversion{}, false, f
	}
	h := n.hash()

	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if ex, err := q.StockConvByIdemKey(ctx, gen.StockConvByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); err == nil {
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return convReplay{ex.ID}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		out, err := q.StockConvOutletInfo(ctx, gen.StockConvOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !out.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}

		// Kunci kedua barang terurut menurut id agar dua dokumen berlawanan arah tidak saling deadlock.
		ids := []uuid.UUID{n.from, n.to}
		if ids[0].String() > ids[1].String() {
			ids[0], ids[1] = ids[1], ids[0]
		}
		items := map[uuid.UUID]gen.StockConvItemLockRow{}
		for _, id := range ids {
			it, err := q.StockConvItemLock(ctx, gen.StockConvItemLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
			if errors.Is(err, pgx.ErrNoRows) {
				if id == n.from {
					return FieldErrors{"from_item_id": sanitize.Invalid}
				}
				return FieldErrors{"to_item_id": sanitize.Invalid}
			}
			if err != nil {
				return err
			}
			if it.Kind != "goods" {
				return ErrNotStocked
			}
			items[id] = it
		}
		from, to := items[n.from], items[n.to]

		// HPP hasil = HPP asal × qty asal ÷ qty tujuan (HPP cabang ini). Dipasang ke barang tujuan di cabang ini hanya bila
		// HPP-nya kosong atau stok cabangnya ≤ 0 (belum ada nilai yang harus dirata-ratakan).
		toCost := from.AvgCost.Mul(n.fromQ).Div(n.toQ).Round(2)
		toStock, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: n.to})
		if err != nil {
			return err
		}
		apply := toCost.IsPositive() && (to.AvgCost.IsZero() || !toStock.IsPositive())

		no, err := q.StockConvNextNo(ctx, gen.StockConvNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if err != nil {
			return err
		}
		docNo := fmt.Sprintf("PS-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		id := uuid.New()
		_, err = q.StockConvInsert(ctx, gen.StockConvInsertParams{
			ID: id, TenantID: a.TenantID, OutletID: a.OutletID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
			FromItemID: n.from, FromQty: n.fromQ, ToItemID: n.to, ToQty: n.toQ,
			FromUnitCost: from.AvgCost, ToUnitCost: toCost, CostApplied: apply, Note: n.note, ActorID: nullUUID(a.UserID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Pengiriman ganda bersamaan: yang lain menang. Batalkan transaksi ini (nomor tak terpakai) dan kembalikan dokumen itu.
			ex, e2 := q.StockConvByIdemKey(ctx, gen.StockConvByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
			if e2 != nil {
				return e2
			}
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return convReplay{ex.ID}
		}
		if err != nil {
			return err
		}

		if _, err := ApplyAll(ctx, tx, []Movement{
			{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: n.from, Bucket: BucketDisplay, Delta: n.fromQ.Neg(), RefType: RefUnitConversion, RefID: id, Note: docNo, ActorID: a.UserID},
			{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: n.to, Bucket: BucketDisplay, Delta: n.toQ, RefType: RefUnitConversion, RefID: id, Note: docNo, ActorID: a.UserID},
		}); err != nil {
			return err
		}
		if apply {
			if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: n.to, AvgCost: toCost, LastCost: toCost}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockConvert, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(),
				"from_sku": from.Sku, "from_qty": n.fromQ.String(), "to_sku": to.Sku, "to_qty": n.toQ.String(),
				"to_unit_cost": toCost.String(), "cost_applied": apply},
		})
	})
	var rp convReplay
	if errors.As(err, &rp) {
		conv, err = s.GetConversion(ctx, a, rp.id)
		return conv, true, err
	}
	if err != nil {
		return Conversion{}, false, err
	}
	var id uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		ex, err := gen.New(tx).StockConvByIdemKey(ctx, gen.StockConvByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		id = ex.ID
		return err
	})
	if err != nil {
		return Conversion{}, false, err
	}
	conv, err = s.GetConversion(ctx, a, id)
	return conv, false, err
}

func (s *Service) GetConversion(ctx context.Context, a authz.Actor, id uuid.UUID) (Conversion, error) {
	var out Conversion
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).StockConvGet(ctx, gen.StockConvGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if r.OutletID != a.OutletID && !a.Outlets[r.OutletID] {
			return ErrOutletForbidden
		}
		out = Conversion{ID: r.ID, DocNo: r.DocNo, FromUnitCost: r.FromUnitCost.StringFixed(2), ToUnitCost: r.ToUnitCost.StringFixed(2),
			CostApplied: r.CostApplied, Note: r.Note, Actor: r.ActorName, CreatedAt: r.CreatedAt.Time,
			From: ConvItem{ID: r.FromID, SKU: r.FromSku, Name: r.FromName, Unit: r.FromUnit, Qty: r.FromQty.String()},
			To:   ConvItem{ID: r.ToID, SKU: r.ToSku, Name: r.ToName, Unit: r.ToUnit, Qty: r.ToQty.String()}}
		return nil
	})
	return out, err
}

// ListConversions = dokumen pecah satuan outlet aktif, terbaru dulu.
func (s *Service) ListConversions(ctx context.Context, a authz.Actor, limit, offset int) ([]Conversion, int, error) {
	if limit <= 0 {
		limit = defLimit
	}
	out := []Conversion{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).StockConvList(ctx, gen.StockConvListParams{
			TenantID: a.TenantID, OutletID: a.OutletID, PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(offset, 0))})
		for _, r := range rows {
			out = append(out, Conversion{ID: r.ID, DocNo: r.DocNo, FromUnitCost: r.FromUnitCost.StringFixed(2), ToUnitCost: r.ToUnitCost.StringFixed(2),
				CostApplied: r.CostApplied, Note: r.Note, Actor: r.ActorName, CreatedAt: r.CreatedAt.Time,
				From: ConvItem{ID: r.FromID, SKU: r.FromSku, Name: r.FromName, Unit: r.FromUnit, Qty: r.FromQty.String()},
				To:   ConvItem{ID: r.ToID, SKU: r.ToSku, Name: r.ToName, Unit: r.ToUnit, Qty: r.ToQty.String()}})
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}
