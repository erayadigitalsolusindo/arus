// Package payable: hutang pemasok dari pembelian kredit (Fase 6.3). Hutang lahir dari nota kredit (purchasing) dan dilunasi
// lewat payable_payments. Saldo = amount − Σ pembayaran (dihitung, tidak disimpan; AGENTS.md §8). Pola meniru `receivable`.
// SQL ditulis tangan (pgx) karena daftar/ringkasan memakai CTE bersama.
package payable

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Module = modul izin Hutang Pemasok: view = lihat, create = bayar hutang.
const Module = "supplier_payables"

const (
	pageSize  = 50
	maxPage   = 200
	maxAmount = 1_000_000_000_000
)

var (
	ErrNotFound    = errors.New("hutang tidak ditemukan")
	ErrKeyRequired = errors.New("Idempotency-Key wajib diisi")
	ErrKeyMismatch = errors.New("Idempotency-Key sudah dipakai untuk permintaan yang berbeda")
	ErrOutletGone  = errors.New("outlet tidak aktif")
	idemKey        = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type dec = decimal.Decimal

// HasPayments = hutang nota ini sudah pernah dibayar (nota pembelian tidak boleh diedit/dibatalkan lagi).
func HasPayments(ctx context.Context, tx pgx.Tx, tenant, purchase uuid.UUID) (bool, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM payable_payments pp JOIN payables p ON p.tenant_id = pp.tenant_id AND p.id = pp.payable_id
		WHERE p.tenant_id = $1 AND p.purchase_id = $2`, tenant, purchase).Scan(&n)
	return n > 0, err
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Row struct {
	ID                uuid.UUID `json:"id"`
	PurchaseID        uuid.UUID `json:"purchase_id"`
	DocNo             string    `json:"doc_no"`
	SupplierInvoiceNo string    `json:"supplier_invoice_no"`
	SupplierID        uuid.UUID `json:"supplier_id"`
	SupplierCode      string    `json:"supplier_code"`
	SupplierName      string    `json:"supplier_name"`
	PurchaseDate      string    `json:"purchase_date"`
	Amount            string    `json:"amount"`
	Paid              string    `json:"paid"`
	Balance           string    `json:"balance"`
	DueDate           *string   `json:"due_date,omitempty"`
	Status            string    `json:"status"` // open | overdue | paid
}

// Summary: aging = sisa hutang menurut umur lewat jatuh tempo (current = belum jatuh tempo / tanpa jatuh tempo).
type Summary struct {
	Outstanding string `json:"outstanding"`
	Overdue     string `json:"overdue"`
	OpenCount   int64  `json:"open_count"`
	TotalCount  int64  `json:"total_count"`
	Aging       Aging  `json:"aging"`
}

type Aging struct {
	Current string `json:"current"`
	D1to30  string `json:"d1_30"`
	D31to60 string `json:"d31_60"`
	D60Plus string `json:"d60_plus"`
}

type ListResult struct {
	Data    []Row   `json:"data"`
	Summary Summary `json:"summary"`
	HasMore bool    `json:"has_more"`
}

type Payment struct {
	ID         uuid.UUID `json:"id"`
	DocNo      string    `json:"doc_no"`
	Method     string    `json:"method"`
	MethodID   uuid.UUID `json:"method_id"`
	MethodName string    `json:"method_name"`
	Amount     string    `json:"amount"`
	RefNo      string    `json:"ref_no"`
	Note       string    `json:"note"`
	PaidBy     string    `json:"paid_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type Detail struct {
	Row
	PurchaseTotal string    `json:"purchase_total"`
	Payments      []Payment `json:"payments"`
}

type ListParams struct {
	SupplierID *uuid.UUID
	Status     string // open (bawaan) | overdue | paid | all
	Q          string
	Limit      int
	Offset     int
}

func statusOf(balance dec, due pgtype.Date, today pgtype.Date) string {
	switch {
	case !balance.IsPositive():
		return "paid"
	case due.Valid && today.Valid && due.Time.Before(today.Time):
		return "overdue"
	}
	return "open"
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func outletIDs(a authz.Actor) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(a.Outlets))
	for id := range a.Outlets {
		ids = append(ids, id)
	}
	return ids
}

// baseCTE: hutang aktif (nota belum dibatalkan/digantikan) di outlet yang boleh diakses, lengkap dengan jumlah terbayar.
// $1 tenant, $2 outlet ids, $3 supplier (NULL = semua).
const baseCTE = `
WITH base AS (
  SELECT pb.id, pb.purchase_id, pb.supplier_id, pb.outlet_id, pb.amount, pb.due_date,
         coalesce(pp.paid, 0) AS paid, pu.doc_no, pu.supplier_invoice_no, pu.purchase_date, pu.total AS purchase_total,
         s.name AS supplier_name, coalesce(s.code, '') AS supplier_code,
         (now() AT TIME ZONE o.timezone)::date AS today
  FROM payables pb
  JOIN purchases pu ON pu.tenant_id = pb.tenant_id AND pu.id = pb.purchase_id
  JOIN suppliers s  ON s.tenant_id = pb.tenant_id AND s.id = pb.supplier_id
  JOIN outlets o    ON o.tenant_id = pb.tenant_id AND o.id = pb.outlet_id
  LEFT JOIN LATERAL (SELECT sum(x.amount) AS paid FROM payable_payments x WHERE x.tenant_id = pb.tenant_id AND x.payable_id = pb.id) pp ON true
  WHERE pb.tenant_id = $1 AND pb.voided_at IS NULL AND pb.outlet_id = ANY($2::uuid[]) AND ($3::uuid IS NULL OR pb.supplier_id = $3)
)`

const rowCols = `id, purchase_id, supplier_id, supplier_code, supplier_name, doc_no, supplier_invoice_no, purchase_date, amount, paid, due_date, today, purchase_total`

type scanned struct {
	Row
	amount, paid  dec
	due, today    pgtype.Date
	purchaseTotal dec
	pdate         pgtype.Date
}

func scanRow(rows pgx.Row) (scanned, error) {
	var r scanned
	err := rows.Scan(&r.ID, &r.PurchaseID, &r.SupplierID, &r.SupplierCode, &r.SupplierName, &r.DocNo, &r.SupplierInvoiceNo, &r.pdate,
		&r.amount, &r.paid, &r.due, &r.today, &r.purchaseTotal)
	if err != nil {
		return r, err
	}
	bal := r.amount.Sub(r.paid)
	r.PurchaseDate = r.pdate.Time.Format("2006-01-02")
	r.Amount, r.Paid, r.Balance = r.amount.StringFixed(2), r.paid.StringFixed(2), bal.StringFixed(2)
	r.DueDate = dateStr(r.due)
	r.Status = statusOf(bal, r.due, r.today)
	return r, nil
}

// List = hutang di semua outlet yang boleh diakses pemanggil (hutang ke satu pemasok lintas cabang tetap satu daftar).
func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) (ListResult, error) {
	switch p.Status {
	case "overdue", "paid", "all":
	default:
		p.Status = "open"
	}
	if p.Limit <= 0 {
		p.Limit = pageSize
	}
	p.Limit = min(p.Limit, maxPage)
	if p.Offset < 0 {
		p.Offset = 0
	}
	q := strings.TrimSpace(p.Q)
	if utf8.RuneCountInString(q) > 100 {
		q = string([]rune(q)[:100])
	}
	pattern := ""
	if q != "" {
		pattern = "%" + escapeLike(q) + "%"
	}
	supplier := pgtype.UUID{}
	if p.SupplierID != nil {
		supplier = pgtype.UUID{Bytes: *p.SupplierID, Valid: true}
	}
	ids := outletIDs(a)
	res := ListResult{Data: []Row{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, baseCTE+` SELECT `+rowCols+` FROM base
			WHERE ($4::text = '' OR doc_no ILIKE $4 OR supplier_invoice_no ILIKE $4 OR supplier_name ILIKE $4 OR supplier_code ILIKE $4)
			  AND CASE $5::text WHEN 'all' THEN true
			        WHEN 'paid' THEN paid >= amount
			        WHEN 'overdue' THEN paid < amount AND due_date IS NOT NULL AND due_date < today
			        ELSE paid < amount END
			ORDER BY (paid >= amount), coalesce(due_date, DATE '9999-12-31'), purchase_date DESC, id
			LIMIT $6 OFFSET $7`, a.TenantID, ids, supplier, pattern, p.Status, p.Limit+1, p.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRow(rows)
			if err != nil {
				return err
			}
			res.Data = append(res.Data, r.Row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(res.Data) > p.Limit {
			res.HasMore = true
			res.Data = res.Data[:p.Limit]
		}
		var out, over, cur, d1, d2, d3 dec
		err = tx.QueryRow(ctx, baseCTE+` SELECT
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount), 0),
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount AND due_date < today), 0),
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount AND (due_date IS NULL OR due_date >= today)), 0),
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount AND today - due_date BETWEEN 1 AND 30), 0),
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount AND today - due_date BETWEEN 31 AND 60), 0),
			coalesce(sum(amount - paid) FILTER (WHERE paid < amount AND today - due_date > 60), 0),
			count(*) FILTER (WHERE paid < amount), count(*)
			FROM base`, a.TenantID, ids, supplier).Scan(&out, &over, &cur, &d1, &d2, &d3, &res.Summary.OpenCount, &res.Summary.TotalCount)
		if err != nil {
			return err
		}
		res.Summary.Outstanding, res.Summary.Overdue = out.StringFixed(2), over.StringFixed(2)
		res.Summary.Aging = Aging{Current: cur.StringFixed(2), D1to30: d1.StringFixed(2), D31to60: d2.StringFixed(2), D60Plus: d3.StringFixed(2)}
		return nil
	})
	return res, err
}

// Get = satu hutang beserta riwayat pembayarannya.
func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Detail, error) {
	var out Detail
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		d, err := s.detail(ctx, tx, a, id)
		out = d
		return err
	})
	return out, err
}

func (s *Service) detail(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID) (Detail, error) {
	r, err := scanRow(tx.QueryRow(ctx, baseCTE+` SELECT `+rowCols+` FROM base WHERE id = $4`, a.TenantID, outletIDs(a), pgtype.UUID{}, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Row: r.Row, PurchaseTotal: r.purchaseTotal.StringFixed(2), Payments: []Payment{}}
	rows, err := tx.Query(ctx, `
		SELECT pp.id, pp.doc_no, pp.method, pp.method_id, pp.method_name, pp.amount, pp.ref_no, pp.note, coalesce(u.name, ''), pp.created_at
		FROM payable_payments pp LEFT JOIN users u ON u.tenant_id = pp.tenant_id AND u.id = pp.paid_by
		WHERE pp.tenant_id = $1 AND pp.payable_id = $2 ORDER BY pp.created_at, pp.id`, a.TenantID, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Payment
		var amt dec
		if err := rows.Scan(&p.ID, &p.DocNo, &p.Method, &p.MethodID, &p.MethodName, &amt, &p.RefNo, &p.Note, &p.PaidBy, &p.CreatedAt); err != nil {
			return d, err
		}
		p.Amount = amt.StringFixed(2)
		d.Payments = append(d.Payments, p)
	}
	return d, rows.Err()
}

// PayInput = satu pembayaran hutang lewat satu metode (cicilan boleh; jumlah tidak boleh melebihi sisa).
type PayInput struct {
	MethodID uuid.UUID   `json:"method_id"`
	Amount   json.Number `json:"amount"`
	RefNo    string      `json:"ref_no"`
	Note     string      `json:"note"`
}

type norm struct {
	method uuid.UUID
	amount dec
	ref    string
	note   string
}

func normalize(in PayInput) (norm, FieldErrors) {
	f := FieldErrors{}
	n := norm{method: in.MethodID}
	if in.MethodID == uuid.Nil {
		f["method_id"] = sanitize.Required
	}
	str := strings.TrimSpace(in.Amount.String())
	switch d, err := decimal.NewFromString(str); {
	case str == "":
		f["amount"] = sanitize.Required
	case err != nil || !d.IsPositive() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxAmount)):
		f["amount"] = sanitize.Invalid
	default:
		n.amount = d
	}
	ref, ok := sanitize.Text(in.RefNo)
	if !ok || utf8.RuneCountInString(ref) > 100 {
		f["ref_no"] = sanitize.Invalid
	}
	n.ref = ref
	note, ok := sanitize.Text(in.Note)
	if !ok || utf8.RuneCountInString(note) > 200 {
		f["note"] = sanitize.Invalid
	}
	n.note = note
	return n, f
}

func (n norm) hash(payable uuid.UUID) string {
	raw, _ := json.Marshal([]string{payable.String(), n.method.String(), n.amount.String(), n.ref, n.note})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type replay struct{}

func (replay) Error() string { return "replay" }

// checkKey: nil = kunci belum dipakai; replay{} = pembayaran yang sama sudah tercatat; ErrKeyMismatch = kunci dipakai isi lain.
func checkKey(ctx context.Context, tx pgx.Tx, tenant, payable uuid.UUID, key, hash string) error {
	var h string
	var pid uuid.UUID
	err := tx.QueryRow(ctx, `SELECT request_hash, payable_id FROM payable_payments WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, key).Scan(&h, &pid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if h != hash || pid != payable {
		return ErrKeyMismatch
	}
	return replay{}
}

// Pay mencatat satu pembayaran hutang di outlet aktif pemanggil (uang keluar dari outlet itu). replayed=true bila
// Idempotency-Key yang sama sudah pernah mencatat pembayaran ini (hasil sama; tidak ada pembayaran ganda).
func (s *Service) Pay(ctx context.Context, a authz.Actor, id uuid.UUID, key string, in PayInput) (d Detail, replayed bool, err error) {
	if !idemKey.MatchString(key) {
		return Detail{}, false, ErrKeyRequired
	}
	n, f := normalize(in)
	if len(f) > 0 {
		return Detail{}, false, f
	}
	h := n.hash(id)
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if e := checkKey(ctx, tx, a.TenantID, id, key, h); e != nil {
			return e
		}
		// Kunci NOTA pembelian (sama dengan yang dikunci edit/batal) → edit/batal dan pembayaran saling menunggu, tidak berpapasan.
		var (
			outletID uuid.UUID
			amount   dec
			status   string
			docNo    string
			voided   pgtype.Timestamptz
		)
		e := tx.QueryRow(ctx, `
			SELECT pb.outlet_id, pb.amount, pb.voided_at, pu.status, pu.doc_no
			FROM payables pb JOIN purchases pu ON pu.tenant_id = pb.tenant_id AND pu.id = pb.purchase_id
			WHERE pb.tenant_id = $1 AND pb.id = $2
			FOR UPDATE OF pu`, a.TenantID, id).Scan(&outletID, &amount, &voided, &status, &docNo)
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !a.Outlets[outletID]) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if voided.Valid || status != "completed" {
			return ErrNotFound
		}
		// Pengiriman ganda bersamaan: yang kedua menunggu kunci di atas, jadi kunci idempotensinya baru terlihat sekarang.
		if e := checkKey(ctx, tx, a.TenantID, id, key, h); e != nil {
			return e
		}
		var paid dec
		if e := tx.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM payable_payments WHERE tenant_id = $1 AND payable_id = $2`, a.TenantID, id).Scan(&paid); e != nil {
			return e
		}
		balance := amount.Sub(paid)
		switch {
		case !balance.IsPositive():
			return FieldErrors{"amount": "SETTLED"}
		case n.amount.GreaterThan(balance):
			return FieldErrors{"amount": "OVERPAID"}
		}
		var (
			mKind, mName string
			mActive      bool
		)
		e = tx.QueryRow(ctx, `SELECT kind, name, active FROM payment_methods WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.method).Scan(&mKind, &mName, &mActive)
		switch {
		case errors.Is(e, pgx.ErrNoRows):
			return FieldErrors{"method_id": sanitize.Invalid}
		case e != nil:
			return e
		case !mActive:
			return FieldErrors{"method_id": "METHOD_INACTIVE"}
		}
		var (
			code   string
			active bool
			day    pgtype.Date
		)
		e = tx.QueryRow(ctx, `SELECT code, active, (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.OutletID).Scan(&code, &active, &day)
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !active) {
			return ErrOutletGone
		}
		if e != nil {
			return e
		}
		var no int64
		if e := tx.QueryRow(ctx, `
			INSERT INTO payable_payment_counters (tenant_id, outlet_id, day, last_no) VALUES ($1, $2, $3, 1)
			ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = payable_payment_counters.last_no + 1
			RETURNING last_no`, a.TenantID, a.OutletID, day).Scan(&no); e != nil {
			return e
		}
		payNo := fmt.Sprintf("PH-%s-%s-%04d", strings.ToUpper(code), day.Time.Format("060102"), no)
		if _, e := tx.Exec(ctx, `
			INSERT INTO payable_payments (tenant_id, payable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name, amount, ref_no, note, paid_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			a.TenantID, id, a.OutletID, payNo, key, h, mKind, n.method, mName, n.amount, n.ref, n.note,
			pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}); e != nil {
			return e
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionPayablePay, Entity: audit.EntityPayable, EntityID: id.String(),
			Details: map[string]any{"doc_no": payNo, "purchase_doc_no": docNo, "method": mName, "amount": n.amount.String(),
				"balance_after": balance.Sub(n.amount).String(), "outlet_id": a.OutletID.String()}})
	})
	var rp replay
	if errors.As(err, &rp) {
		d, err = s.Get(ctx, a, id)
		return d, true, err
	}
	if err != nil {
		return Detail{}, false, err
	}
	d, err = s.Get(ctx, a, id)
	return d, false, err
}
