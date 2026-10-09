package purchasing

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	"aciraba/internal/platform/httpx"
)

// PriceHistoryModule = modul izin laporan Riwayat Harga Beli (authz.Modules).
const PriceHistoryModule = "buy_price_history"

// PriceHistoryRow = satu baris pembelian (nota aktif) sebagai kejadian harga beli. Tanpa tabel baru: dibaca dari purchase_lines.
type PriceHistoryRow struct {
	PurchaseID   uuid.UUID `json:"purchase_id"`
	Position     int       `json:"position"`
	DocNo        string    `json:"doc_no"`
	PurchaseDate string    `json:"purchase_date"`
	CreatedAt    time.Time `json:"created_at"`
	OutletName   string    `json:"outlet_name"`
	SupplierID   uuid.UUID `json:"supplier_id"`
	SupplierName string    `json:"supplier_name"`
	ItemID       uuid.UUID `json:"item_id"`
	SKU          string    `json:"sku"`
	Name         string    `json:"name"`
	Unit         string    `json:"unit"`
	Qty          string    `json:"qty"`
	UnitPrice    string    `json:"unit_price"`
	Discounts    []string  `json:"discounts"`
	UnitCost     string    `json:"unit_cost"`            // HPP baris (sudah termasuk diskon & alokasi biaya lain)
	PrevPrice    *string   `json:"prev_price,omitempty"` // harga beli pembelian sebelumnya untuk barang yang sama (pemasok mana pun)
}

type PriceHistoryParams struct {
	From, To   string
	Q          string
	SupplierID *uuid.UUID
	ItemID     *uuid.UUID
	Cursor     string
	Limit      int
}

type PriceHistoryResult struct {
	Data       []PriceHistoryRow `json:"data"`
	NextCursor string            `json:"next_cursor"`
}

func encodePHCursor(d time.Time, at time.Time, id uuid.UUID, pos int) string {
	raw := d.Format("2006-01-02") + "|" + strconv.FormatInt(at.UnixMicro(), 10) + "|" + id.String() + "|" + strconv.Itoa(pos)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodePHCursor(c string) (d time.Time, at time.Time, id uuid.UUID, pos int, ok bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 4 {
		return
	}
	var e1, e2, e3 error
	d, e1 = time.Parse("2006-01-02", parts[0])
	us, e2 := strconv.ParseInt(parts[1], 10, 64)
	id, e3 = uuid.Parse(parts[2])
	pos, e4 := strconv.Atoi(parts[3])
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return
	}
	return d, time.UnixMicro(us).UTC(), id, pos, true
}

func lastDay(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// PriceHistory = riwayat harga beli lintas barang di outlet yang boleh diakses pemanggil, terbaru dulu (keyset).
func (s *Service) PriceHistory(ctx context.Context, a authz.Actor, p PriceHistoryParams) (PriceHistoryResult, error) {
	f := FieldErrors{}
	var from, to time.Time
	var ok bool
	if p.From == "" && p.To == "" {
		now := time.Now().UTC()
		to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
		from = to.AddDate(0, 0, -31)
	} else {
		if from, ok = parseDate(p.From); !ok {
			f["from"] = "INVALID"
		}
		if to, ok = parseDate(p.To); !ok {
			f["to"] = "INVALID"
		}
	}
	if len(f) == 0 && (to.Before(from) || to.Sub(from) > maxRangeDays*24*time.Hour) {
		f["to"] = "INVALID"
	}
	q := strings.TrimSpace(p.Q)
	if len([]rune(q)) > 100 {
		f["q"] = "TOO_LONG"
	}
	var cd pgtype.Date
	var curAt pgtype.Timestamptz
	var cid uuid.UUID
	var cpos int
	hasCur := false
	if p.Cursor != "" {
		d, at, id, pos, ok := decodePHCursor(p.Cursor)
		if !ok {
			f["cursor"] = "INVALID"
		}
		cd, curAt, cid, cpos, hasCur = pgDate(d), pgtype.Timestamptz{Time: at, Valid: true}, id, pos, true
	}
	if len(f) > 0 {
		return PriceHistoryResult{}, f
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := ""
	if q != "" {
		pattern = "%" + escapeLike(q) + "%"
	}
	var sup, itm pgtype.UUID
	if p.SupplierID != nil {
		sup = pgtype.UUID{Bytes: *p.SupplierID, Valid: true}
	}
	if p.ItemID != nil {
		itm = pgtype.UUID{Bytes: *p.ItemID, Valid: true}
	}
	ids := make([]uuid.UUID, 0, len(a.Outlets))
	for id := range a.Outlets {
		ids = append(ids, id)
	}
	res := PriceHistoryResult{Data: []PriceHistoryRow{}}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT pl.purchase_id, pl.position, pu.doc_no, pu.purchase_date, pu.created_at, o.name, pu.supplier_id, s.name,
			       pl.item_id, pl.item_sku, pl.item_name, pl.unit_name, pl.qty, pl.unit_price,
			       pl.disc1, pl.disc2, pl.disc3, pl.disc4, pl.unit_cost, prev.unit_price
			FROM purchase_lines pl
			JOIN purchases pu ON pu.tenant_id = pl.tenant_id AND pu.id = pl.purchase_id
			JOIN suppliers s  ON s.tenant_id = pu.tenant_id AND s.id = pu.supplier_id
			JOIN outlets o    ON o.tenant_id = pu.tenant_id AND o.id = pu.outlet_id
			LEFT JOIN LATERAL (
			    SELECT pl2.unit_price FROM purchase_lines pl2
			    JOIN purchases p2 ON p2.tenant_id = pl2.tenant_id AND p2.id = pl2.purchase_id
			    WHERE pl2.tenant_id = pl.tenant_id AND pl2.item_id = pl.item_id AND p2.status = 'completed'
			      AND (p2.purchase_date, p2.created_at, p2.id) < (pu.purchase_date, pu.created_at, pu.id)
			    ORDER BY p2.purchase_date DESC, p2.created_at DESC, p2.id DESC LIMIT 1) prev ON true
			WHERE pl.tenant_id = $1 AND pu.status = 'completed' AND pu.outlet_id = ANY($2::uuid[])
			  AND pu.purchase_date BETWEEN $3 AND $4
			  AND ($5::uuid IS NULL OR pu.supplier_id = $5) AND ($6::uuid IS NULL OR pl.item_id = $6)
			  AND ($7::text = '' OR pl.item_sku ILIKE $7 OR pl.item_name ILIKE $7 OR pu.doc_no ILIKE $7 OR s.name ILIKE $7)
			  AND (NOT $8::boolean OR (pu.purchase_date, pu.created_at, pu.id, pl.position) < ($9::date, $10::timestamptz, $11::uuid, $12::int))
			ORDER BY pu.purchase_date DESC, pu.created_at DESC, pu.id DESC, pl.position DESC
			LIMIT $13`,
			a.TenantID, ids, pgDate(from), pgDate(to), sup, itm, pattern, hasCur, cd, curAt, cid, cpos, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r PriceHistoryRow
			var pd pgtype.Date
			var qty, price, cost dec
			var d1, d2, d3, d4 dec
			var prev *dec
			if err := rows.Scan(&r.PurchaseID, &r.Position, &r.DocNo, &pd, &r.CreatedAt, &r.OutletName, &r.SupplierID, &r.SupplierName,
				&r.ItemID, &r.SKU, &r.Name, &r.Unit, &qty, &price, &d1, &d2, &d3, &d4, &cost, &prev); err != nil {
				return err
			}
			if len(res.Data) == limit {
				last := res.Data[len(res.Data)-1]
				res.NextCursor = encodePHCursor(lastDay(last.PurchaseDate), last.CreatedAt, last.PurchaseID, last.Position)
				break
			}
			r.PurchaseDate = pd.Time.Format("2006-01-02")
			r.Qty, r.UnitPrice, r.UnitCost = qty.String(), priceString(price), cost.StringFixed(2)
			r.Discounts = []string{}
			for _, d := range []dec{d1, d2, d3, d4} {
				if d.IsPositive() {
					r.Discounts = append(r.Discounts, d.StringFixed(2))
				}
			}
			if prev != nil {
				ps := priceString(*prev)
				r.PrevPrice = &ps
			}
			res.Data = append(res.Data, r)
		}
		return rows.Err()
	})
	return res, err
}

// PriceHistory: GET /purchases/price-history?from=&to=&q=&supplier_id=&item_id=&cursor=&limit=
func (h *Handler) PriceHistory(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := PriceHistoryParams{From: qs.Get("from"), To: qs.Get("to"), Q: qs.Get("q"), Cursor: qs.Get("cursor")}
	for k, dst := range map[string]**uuid.UUID{"supplier_id": &p.SupplierID, "item_id": &p.ItemID} {
		if s := qs.Get(k); s != "" {
			id, err := uuid.Parse(s)
			if err != nil {
				httpx.ValidationError(w, map[string]string{k: "INVALID"})
				return
			}
			*dst = &id
		}
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.PriceHistory(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
