package stock

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

// Kartu stok per barang (Fase 4.3): bacaan atas ledger `stock_movements` untuk outlet aktif sesi. Periode menurut
// zona waktu outlet; saldo awal = saldo-setelah movement terakhir sebelum periode; saldo berjalan dihitung dari
// saldo awal + mutasi berurutan (jumlah semua bucket yang dipilih).

// CardModule = modul izin Kartu Stok (authz.Modules): hanya view.
const CardModule = "stock_card"

const (
	cardDefLimit = 100
	cardMaxLimit = 200
	cardMaxDays  = 366
)

type CardItem struct {
	ID   uuid.UUID `json:"id"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
	Unit string    `json:"unit"`
}

// CardRow = satu mutasi. Note memuat nomor dokumen sumber (nota, pecah satuan) bila ada.
type CardRow struct {
	ID       int64      `json:"id"`
	At       time.Time  `json:"at"`
	Bucket   string     `json:"bucket"`
	RefType  string     `json:"ref_type"`
	RefID    *uuid.UUID `json:"ref_id,omitempty"`
	Note     string     `json:"note"`
	Actor    string     `json:"actor"`
	QtyDelta string     `json:"qty_delta"`
	Balance  string     `json:"balance"`
}

// CardResult: Opening/In/Out/Closing mencakup SELURUH periode (bukan hanya halaman ini). NextCursor ada bila masih ada baris.
type CardResult struct {
	Item       CardItem  `json:"item"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Bucket     string    `json:"bucket"`
	Opening    string    `json:"opening"`
	In         string    `json:"in"`
	Out        string    `json:"out"`
	Closing    string    `json:"closing"`
	Rows       []CardRow `json:"rows"`
	NextCursor *int64    `json:"next_cursor"`
}

type CardParams struct {
	ItemID   uuid.UUID
	From, To string // YYYY-MM-DD; kosong = 30 hari terakhir sampai hari ini (zona waktu outlet)
	Bucket   string // kosong = semua bucket
	After    int64
	Limit    int
}

func parseDay(s string) (pgtype.Date, bool) {
	if s == "" {
		return pgtype.Date{}, true
	}
	d, err := time.Parse("2006-01-02", s)
	return pgtype.Date{Time: d, Valid: err == nil}, err == nil
}

func (s *Service) Card(ctx context.Context, a authz.Actor, p CardParams) (CardResult, error) {
	fe := FieldErrors{}
	if p.ItemID == uuid.Nil {
		fe["item_id"] = sanitize.Required
	}
	if p.Bucket != "" && !validBuckets[p.Bucket] {
		fe["bucket"] = sanitize.Invalid
	}
	from, okFrom := parseDay(p.From)
	to, okTo := parseDay(p.To)
	if !okFrom {
		fe["from"] = sanitize.Invalid
	}
	if !okTo {
		fe["to"] = sanitize.Invalid
	}
	if p.After < 0 {
		fe["cursor"] = sanitize.Invalid
	}
	if len(fe) > 0 {
		return CardResult{}, fe
	}
	limit := p.Limit
	if limit <= 0 {
		limit = cardDefLimit
	}
	limit = min(limit, cardMaxLimit)

	res := CardResult{Rows: []CardRow{}, Bucket: p.Bucket}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		b, err := q.StockCardBounds(ctx, gen.StockCardBoundsParams{TenantID: a.TenantID, OutletID: a.OutletID, FromDay: from, ToDay: to})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		if b.ToDay.Time.Before(b.FromDay.Time) || b.ToDay.Time.Sub(b.FromDay.Time) > cardMaxDays*24*time.Hour {
			return FieldErrors{"to": sanitize.Invalid}
		}
		it, err := q.StockCardItem(ctx, gen.StockCardItemParams{TenantID: a.TenantID, ID: p.ItemID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		if err != nil {
			return err
		}
		if it.Kind != "goods" {
			return ErrNotStocked
		}
		res.Item = CardItem{ID: it.ID, SKU: it.Sku, Name: it.Name, Unit: it.UnitName}
		res.From, res.To = b.FromDay.Time.Format("2006-01-02"), b.ToDay.Time.Format("2006-01-02")

		opening, err := q.StockCardOpening(ctx, gen.StockCardOpeningParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: p.ItemID, Bucket: p.Bucket, StartAt: b.StartAt})
		if err != nil {
			return err
		}
		tot, err := q.StockCardTotals(ctx, gen.StockCardTotalsParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: p.ItemID, Bucket: p.Bucket, StartAt: b.StartAt, EndAt: b.EndAt})
		if err != nil {
			return err
		}
		res.Opening, res.In, res.Out = opening.String(), tot.QtyIn.String(), tot.QtyOut.String()
		res.Closing = opening.Add(tot.QtyIn).Sub(tot.QtyOut).String()

		// Saldo berjalan halaman ini dimulai dari saldo awal + mutasi sampai kursor.
		running := opening
		if p.After > 0 {
			before, err := q.StockCardBefore(ctx, gen.StockCardBeforeParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: p.ItemID, Bucket: p.Bucket, StartAt: b.StartAt, EndAt: b.EndAt, AfterID: p.After})
			if err != nil {
				return err
			}
			running = running.Add(before)
		}
		rows, err := q.StockCardRows(ctx, gen.StockCardRowsParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: p.ItemID, Bucket: p.Bucket,
			StartAt: b.StartAt, EndAt: b.EndAt, AfterID: p.After, PageLimit: int32(limit + 1)})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			next := rows[limit-1].ID
			res.NextCursor = &next
		}
		for _, r := range rows {
			running = running.Add(r.QtyDelta)
			row := CardRow{ID: r.ID, At: r.CreatedAt.Time, Bucket: r.Bucket, RefType: r.RefType, Note: r.Note, Actor: r.ActorName,
				QtyDelta: r.QtyDelta.String(), Balance: running.String()}
			if r.RefID.Valid {
				id := uuid.UUID(r.RefID.Bytes)
				row.RefID = &id
			}
			res.Rows = append(res.Rows, row)
		}
		return nil
	})
	return res, err
}

// CardItems = pemilih barang kartu stok (barang berstok, termasuk yang diarsipkan).
func (s *Service) CardItems(ctx context.Context, a authz.Actor, q string) ([]CardItem, error) {
	q, ok := sanitize.Text(q)
	if !ok || utf8.RuneCountInString(q) > maxQ {
		return nil, FieldErrors{"q": sanitize.Invalid}
	}
	out := []CardItem{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).StockCardItemSearch(ctx, gen.StockCardItemSearchParams{TenantID: a.TenantID, Q: likeEscape(q)})
		for _, r := range rows {
			out = append(out, CardItem{ID: r.ID, SKU: r.Sku, Name: r.Name, Unit: r.UnitName})
		}
		return err
	})
	return out, err
}

// Card: GET /stock/card?item_id=&from=&to=&bucket=&cursor=&limit=
func (h *Handler) Card(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := CardParams{From: qs.Get("from"), To: qs.Get("to"), Bucket: qs.Get("bucket")}
	if id := qs.Get("item_id"); id != "" {
		var err error
		if p.ItemID, err = uuid.Parse(id); err != nil {
			httpx.ValidationError(w, FieldErrors{"item_id": sanitize.Invalid})
			return
		}
	}
	if c := qs.Get("cursor"); c != "" {
		var err error
		if p.After, err = strconv.ParseInt(c, 10, 64); err != nil {
			httpx.ValidationError(w, FieldErrors{"cursor": sanitize.Invalid})
			return
		}
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.Card(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// CardItems: GET /stock/card/items?q=
func (h *Handler) CardItems(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.CardItems(r.Context(), actor(r), r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}
