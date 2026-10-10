package purchasing

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

type ReturnLine struct {
	Position         int       `json:"position"`
	PurchasePosition int       `json:"purchase_position"`
	ItemID           uuid.UUID `json:"item_id"`
	SKU              string    `json:"sku"`
	Name             string    `json:"name"`
	Unit             string    `json:"unit"`
	Qty              string    `json:"qty"`
	Value            string    `json:"value"`
	UnitCost         string    `json:"unit_cost"`
}

type PurchaseReturn struct {
	ID                uuid.UUID    `json:"id"`
	DocNo             string       `json:"doc_no"`
	OutletID          uuid.UUID    `json:"outlet_id"`
	OutletCode        string       `json:"outlet_code"`
	OutletName        string       `json:"outlet_name"`
	PurchaseID        uuid.UUID    `json:"purchase_id"`
	PurchaseDocNo     string       `json:"purchase_doc_no"`
	SupplierID        uuid.UUID    `json:"supplier_id"`
	SupplierName      string       `json:"supplier_name"`
	SupplierInvoiceNo string       `json:"supplier_invoice_no"`
	ReturnDate        string       `json:"return_date"`
	Status            string       `json:"status"`
	Note              string       `json:"note"`
	Subtotal          string       `json:"subtotal"`
	TaxAmount         string       `json:"tax_amount"`
	Total             string       `json:"total"`
	PayableCut        string       `json:"payable_cut"`
	Refund            string       `json:"refund"`
	RefundMethod      string       `json:"refund_method"` // jenis metode dana kembali ("" = tanpa dana kembali)
	RefundMethodName  string       `json:"refund_method_name"`
	RefundRef         string       `json:"refund_ref"`
	CreatedAt         time.Time    `json:"created_at"`
	CreatedBy         string       `json:"created_by"`
	VoidReason        string       `json:"void_reason"`
	VoidedAt          *time.Time   `json:"voided_at"`
	VoidedBy          string       `json:"voided_by"`
	Lines             []ReturnLine `json:"lines"`
}

// GetReturn membaca satu dokumen retur. Outlet di luar akses = ErrOutletForbidden; lintas tenant = tidak ada (RLS).
func (s *Service) GetReturn(ctx context.Context, a authz.Actor, id uuid.UUID) (PurchaseReturn, error) {
	var r PurchaseReturn
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		var (
			date                         pgtype.Date
			sub, tax, total, cut, refund dec
			voidedAt                     pgtype.Timestamptz
			createdAt                    pgtype.Timestamptz
		)
		err := tx.QueryRow(ctx, `
			SELECT r.id, r.doc_no, r.outlet_id, o.code, o.name, r.purchase_id, p.doc_no, r.supplier_id, s.name, p.supplier_invoice_no,
			       r.return_date, r.status, r.note, r.subtotal, r.tax_amount, r.total, r.payable_cut, r.refund, coalesce(r.refund_method, ''), r.refund_method_name, r.refund_ref,
			       r.created_at, coalesce(cu.name, ''), r.void_reason, r.voided_at, coalesce(vu.name, '')
			FROM purchase_returns r
			JOIN outlets o   ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
			JOIN purchases p ON p.tenant_id = r.tenant_id AND p.id = r.purchase_id
			JOIN suppliers s ON s.tenant_id = r.tenant_id AND s.id = r.supplier_id
			LEFT JOIN users cu ON cu.tenant_id = r.tenant_id AND cu.id = r.created_by
			LEFT JOIN users vu ON vu.tenant_id = r.tenant_id AND vu.id = r.voided_by
			WHERE r.tenant_id = $1 AND r.id = $2`, a.TenantID, id).Scan(
			&r.ID, &r.DocNo, &r.OutletID, &r.OutletCode, &r.OutletName, &r.PurchaseID, &r.PurchaseDocNo, &r.SupplierID, &r.SupplierName,
			&r.SupplierInvoiceNo, &date, &r.Status, &r.Note, &sub, &tax, &total, &cut, &refund, &r.RefundMethod, &r.RefundMethodName, &r.RefundRef,
			&createdAt, &r.CreatedBy, &r.VoidReason, &voidedAt, &r.VoidedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !canAccessOutlet(a, r.OutletID) {
			return ErrOutletForbidden
		}
		r.ReturnDate = date.Time.Format("2006-01-02")
		r.Subtotal, r.TaxAmount, r.Total = sub.StringFixed(2), tax.StringFixed(2), total.StringFixed(2)
		r.PayableCut, r.Refund = cut.StringFixed(2), refund.StringFixed(2)
		r.CreatedAt, r.VoidedAt = createdAt.Time, timePtr(voidedAt)
		r.Lines = []ReturnLine{}
		rows, err := tx.Query(ctx, `
			SELECT position, purchase_position, item_id, item_sku, item_name, unit_name, qty, value, unit_cost
			FROM purchase_return_lines WHERE tenant_id = $1 AND return_id = $2 ORDER BY position`, a.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l ReturnLine
			var pos, ppos int32
			var qty, val, cost dec
			if err := rows.Scan(&pos, &ppos, &l.ItemID, &l.SKU, &l.Name, &l.Unit, &qty, &val, &cost); err != nil {
				return err
			}
			l.Position, l.PurchasePosition = int(pos), int(ppos)
			l.Qty, l.Value, l.UnitCost = qty.String(), val.StringFixed(2), cost.StringFixed(2)
			r.Lines = append(r.Lines, l)
		}
		return rows.Err()
	})
	return r, err
}

// ---- Daftar ----

type ReturnListParams struct {
	From, To string // tanggal retur; kosong = 30 hari terakhir
	Status   string // "" | completed | void
	Q        string // nomor retur, nomor nota, nomor faktur pemasok, nama pemasok
	Limit    int
	Cursor   string
}

type ReturnRow struct {
	ID            uuid.UUID `json:"id"`
	DocNo         string    `json:"doc_no"`
	PurchaseID    uuid.UUID `json:"purchase_id"`
	PurchaseDocNo string    `json:"purchase_doc_no"`
	SupplierName  string    `json:"supplier_name"`
	ReturnDate    string    `json:"return_date"`
	Status        string    `json:"status"`
	Total         string    `json:"total"`
	PayableCut    string    `json:"payable_cut"`
	Refund        string    `json:"refund"`
	Lines         int       `json:"lines"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
}

type ReturnSummary struct {
	Count      int    `json:"count"`
	Total      string `json:"total"`
	PayableCut string `json:"payable_cut"`
	Refund     string `json:"refund"`
}

type ReturnListResult struct {
	Data       []ReturnRow   `json:"data"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor"`
	Summary    ReturnSummary `json:"summary"` // hanya retur aktif (completed)
}

// returnFilter: $1 tenant, $2 outlet, $3 dari, $4 sampai, $5 status, $6 pola cari (sudah di-escape, ” = semua),
// $7 pakai kandidat, $8 kandidat dari purchase_return_search_ids (00052; bila dipakai, $6 dikosongkan).
const returnFilter = `
	FROM purchase_returns r
	JOIN purchases p ON p.tenant_id = r.tenant_id AND p.id = r.purchase_id
	JOIN suppliers s ON s.tenant_id = r.tenant_id AND s.id = r.supplier_id
	WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.return_date BETWEEN $3 AND $4
	  AND ($5::text = '' OR r.status = $5)
	  AND ($6::text = '' OR r.doc_no ILIKE $6 OR p.doc_no ILIKE $6 OR p.supplier_invoice_no ILIKE $6 OR s.name ILIKE $6)
	  AND (NOT $7::bool OR r.id = ANY($8::uuid[]))`

// ListReturns = retur pembelian di cabang aktif (keyset: tanggal retur, waktu input, id — terbaru dulu).
func (s *Service) ListReturns(ctx context.Context, a authz.Actor, p ReturnListParams) (ReturnListResult, error) {
	f := FieldErrors{}
	var from, to time.Time
	var ok bool
	if p.From == "" && p.To == "" {
		now := time.Now().UTC()
		to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		from = to.AddDate(0, 0, -30)
		to = to.AddDate(0, 0, 1)
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
	if p.Status != "" && p.Status != "completed" && p.Status != "void" {
		f["status"] = "INVALID"
	}
	var cur struct {
		on   bool
		date time.Time
		at   time.Time
		id   uuid.UUID
	}
	if p.Cursor != "" {
		d, at, id, ok := decodeCursor(p.Cursor)
		if !ok {
			f["cursor"] = "INVALID"
		}
		cur.on, cur.date, cur.at, cur.id = true, d, at, id
	}
	if len(f) > 0 {
		return ReturnListResult{}, f
	}
	limit := p.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	pattern := ""
	if p.Q != "" {
		pattern = "%" + escapeLike(p.Q) + "%"
	}
	res := ReturnListResult{Data: []ReturnRow{}}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		var ids []uuid.UUID
		byIDs := false
		if pattern != "" {
			var err error
			if ids, byIDs, err = db.SearchCandidates(ctx, tx, `SELECT purchase_return_search_ids($1, $2, $3, $4, $5)`,
				a.OutletID, pgDate(from), pgDate(to), pattern, db.SearchCap+1); err != nil {
				return err
			}
			if byIDs {
				pattern = ""
			}
		}
		filter := []any{a.TenantID, a.OutletID, pgDate(from), pgDate(to), p.Status, pattern, byIDs, ids}
		args := append(append([]any{}, filter...), cur.on, pgDate(cur.date), pgtype.Timestamptz{Time: cur.at, Valid: cur.on}, cur.id, limit+1)
		rows, err := tx.Query(ctx, `
			SELECT r.id, r.doc_no, r.purchase_id, p.doc_no, s.name, r.return_date, r.status, r.total, r.payable_cut, r.refund,
			       (SELECT count(*) FROM purchase_return_lines rl WHERE rl.tenant_id = r.tenant_id AND rl.return_id = r.id),
			       r.created_at, coalesce((SELECT u.name FROM users u WHERE u.tenant_id = r.tenant_id AND u.id = r.created_by), '')
			`+returnFilter+`
			  AND (NOT $9::bool OR (r.return_date, r.created_at, r.id) < ($10::date, $11::timestamptz, $12::uuid))
			ORDER BY r.return_date DESC, r.created_at DESC, r.id DESC
			LIMIT $13`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ReturnRow
			var date pgtype.Date
			var total, cut, refund dec
			var n int64
			var at pgtype.Timestamptz
			if err := rows.Scan(&r.ID, &r.DocNo, &r.PurchaseID, &r.PurchaseDocNo, &r.SupplierName, &date, &r.Status, &total, &cut, &refund,
				&n, &at, &r.CreatedBy); err != nil {
				return err
			}
			r.ReturnDate = date.Time.Format("2006-01-02")
			r.Total, r.PayableCut, r.Refund, r.Lines, r.CreatedAt = total.StringFixed(2), cut.StringFixed(2), refund.StringFixed(2), int(n), at.Time
			res.Data = append(res.Data, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(res.Data) > limit {
			res.Data = res.Data[:limit]
			res.HasMore = true
			last := res.Data[limit-1]
			res.NextCursor = encodeCursor(last.ReturnDate, last.CreatedAt, last.ID)
		}
		var cnt int64
		var total, cut, refund dec
		if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(sum(r.total), 0), coalesce(sum(r.payable_cut), 0), coalesce(sum(r.refund), 0)
			`+returnFilter+` AND r.status = 'completed'`, filter...).
			Scan(&cnt, &total, &cut, &refund); err != nil {
			return err
		}
		res.Summary = ReturnSummary{Count: int(cnt), Total: total.StringFixed(2), PayableCut: cut.StringFixed(2), Refund: refund.StringFixed(2)}
		return nil
	})
	return res, err
}

// ---- Pemilih nota ----

type ReturnablePurchase struct {
	ID                uuid.UUID `json:"id"`
	DocNo             string    `json:"doc_no"`
	SupplierName      string    `json:"supplier_name"`
	SupplierInvoiceNo string    `json:"supplier_invoice_no"`
	PurchaseDate      string    `json:"purchase_date"`
	PaymentType       string    `json:"payment_type"`
	Total             string    `json:"total"`
	PayableBalance    *string   `json:"payable_balance"` // nil = tanpa hutang (tunai); "0.00" = hutang sudah lunas
}

// ReturnablePurchases = nota completed di cabang aktif yang masih punya sisa qty untuk diretur (terbaru dulu, maks 20).
func (s *Service) ReturnablePurchases(ctx context.Context, a authz.Actor, q string) ([]ReturnablePurchase, error) {
	pattern := ""
	if q != "" {
		pattern = "%" + escapeLike(q) + "%"
	}
	out := []ReturnablePurchase{}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		var ids []uuid.UUID
		byIDs := false
		if pattern != "" {
			var err error
			if ids, byIDs, err = db.SearchCandidates(ctx, tx, `SELECT purchase_search_ids($1, NULL, NULL, $2, $3)`,
				a.OutletID, pattern, db.SearchCap+1); err != nil {
				return err
			}
			if byIDs {
				pattern = ""
			}
		}
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.doc_no, s.name, p.supplier_invoice_no, p.purchase_date, p.payment_type, p.total,
			       pb.amount - (SELECT coalesce(sum(x.amount), 0) FROM payable_payments x WHERE x.tenant_id = pb.tenant_id AND x.payable_id = pb.id)
			                 - (SELECT coalesce(sum(r.payable_cut), 0) FROM purchase_returns r WHERE r.tenant_id = pb.tenant_id AND r.purchase_id = p.id AND r.status = 'completed')
			FROM purchases p JOIN suppliers s ON s.tenant_id = p.tenant_id AND s.id = p.supplier_id
			LEFT JOIN payables pb ON pb.tenant_id = p.tenant_id AND pb.purchase_id = p.id AND pb.voided_at IS NULL
			WHERE p.tenant_id = $1 AND p.outlet_id = $2 AND p.status = 'completed'
			  AND ($3::text = '' OR p.doc_no ILIKE $3 OR p.supplier_invoice_no ILIKE $3 OR s.name ILIKE $3)
			  AND (NOT $4::bool OR p.id = ANY($5::uuid[]))
			  AND EXISTS (
			    SELECT 1 FROM purchase_lines l
			    WHERE l.tenant_id = p.tenant_id AND l.purchase_id = p.id
			      AND l.qty > coalesce((SELECT sum(rl.qty) FROM purchase_return_lines rl
			        JOIN purchase_returns r ON r.tenant_id = rl.tenant_id AND r.id = rl.return_id
			        WHERE rl.tenant_id = l.tenant_id AND r.purchase_id = p.id AND rl.purchase_position = l.position AND r.status = 'completed'), 0))
			ORDER BY p.purchase_date DESC, p.created_at DESC, p.id DESC
			LIMIT 20`, a.TenantID, a.OutletID, pattern, byIDs, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ReturnablePurchase
			var date pgtype.Date
			var total dec
			var bal decimal.NullDecimal
			if err := rows.Scan(&r.ID, &r.DocNo, &r.SupplierName, &r.SupplierInvoiceNo, &date, &r.PaymentType, &total, &bal); err != nil {
				return err
			}
			if bal.Valid {
				b := bal.Decimal.StringFixed(2)
				r.PayableBalance = &b
			}
			r.PurchaseDate, r.Total = date.Time.Format("2006-01-02"), total.StringFixed(2)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
