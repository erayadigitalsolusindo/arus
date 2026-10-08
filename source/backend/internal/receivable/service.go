// Package receivable: piutang penjualan kredit member (Fase 5.1/5.3). Piutang lahir dari nota kredit (sales.receivable) dan
// dilunasi lewat receivable_payments. Saldo = amount − Σ pembayaran (dihitung, tidak disimpan; AGENTS.md §8).
//
// Paket ini tidak mengimpor `sales`: nota memanggil fungsi di sini (Create, Outstanding, ...) di dalam transaksinya.
package receivable

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
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Module = modul izin Daftar Piutang Anggota: view = lihat, create = terima pembayaran.
const Module = "member_receivables"

const (
	pageSize  = 50
	maxPage   = 200
	maxAmount = 1_000_000_000_000
)

var (
	ErrNotFound    = errors.New("piutang tidak ditemukan")
	ErrKeyRequired = errors.New("Idempotency-Key wajib diisi")
	ErrKeyMismatch = errors.New("Idempotency-Key sudah dipakai untuk permintaan yang berbeda")
	ErrOutletGone  = errors.New("outlet tidak aktif")
	idemKey        = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// FieldErrors = kode galat per field (diterjemahkan klien: errors.FIELD_<kode>).
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type dec = decimal.Decimal

// ---- Dipakai nota (di dalam transaksi nota) ----

// Terms = syarat kredit member: Limit 0 = tanpa batas nominal; DueDays 0 = tanpa jatuh tempo.
type Terms struct {
	Limit   dec
	DueDays int
}

func MemberTerms(ctx context.Context, tx pgx.Tx, tenant, member uuid.UUID) (Terms, error) {
	r, err := gen.New(tx).ReceivableMemberTerms(ctx, gen.ReceivableMemberTermsParams{TenantID: tenant, ID: member})
	if err != nil {
		return Terms{}, err
	}
	return Terms{Limit: r.CreditLimit, DueDays: int(r.DueDays)}, nil
}

// Outstanding = sisa piutang member dari nota yang masih berlaku, di luar nota `exclude` (uuid.Nil = tidak ada).
func Outstanding(ctx context.Context, tx pgx.Tx, tenant, member, exclude uuid.UUID) (dec, error) {
	return gen.New(tx).ReceivableOutstanding(ctx, gen.ReceivableOutstandingParams{TenantID: tenant, MemberID: member, ExcludeSaleID: exclude})
}

// DueDate = hari bisnis nota + days; kosong (NULL) bila days = 0.
func DueDate(day pgtype.Date, days int) pgtype.Date {
	if days <= 0 || !day.Valid {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: day.Time.AddDate(0, 0, days), Valid: true}
}

func Create(ctx context.Context, tx pgx.Tx, tenant, outlet, sale, member uuid.UUID, amount dec, due pgtype.Date) error {
	return gen.New(tx).ReceivableInsert(ctx, gen.ReceivableInsertParams{TenantID: tenant, OutletID: outlet, SaleID: sale, MemberID: member, Amount: amount, DueDate: due})
}

// HasPayments = piutang nota ini sudah pernah dibayar (nota tidak boleh diedit/dibatalkan lagi).
func HasPayments(ctx context.Context, tx pgx.Tx, tenant, sale uuid.UUID) (bool, error) {
	n, err := gen.New(tx).ReceivablePaymentCountForSale(ctx, gen.ReceivablePaymentCountForSaleParams{TenantID: tenant, SaleID: sale})
	return n > 0, err
}

// ---- Layanan ----

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Row struct {
	ID         uuid.UUID `json:"id"`
	SaleID     uuid.UUID `json:"sale_id"`
	DocNo      string    `json:"doc_no"`
	MemberID   uuid.UUID `json:"member_id"`
	MemberCode string    `json:"member_code"`
	MemberName string    `json:"member_name"`
	Amount     string    `json:"amount"`
	Paid       string    `json:"paid"`
	Balance    string    `json:"balance"`
	DueDate    *string   `json:"due_date,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Status     string    `json:"status"` // open | overdue | paid
}

type Summary struct {
	Outstanding string `json:"outstanding"`
	Overdue     string `json:"overdue"`
	OpenCount   int64  `json:"open_count"`
	TotalCount  int64  `json:"total_count"`
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
	FeePct     string    `json:"fee_pct"`
	Fee        string    `json:"fee"`
	FeeBearer  string    `json:"fee_bearer"`
	Note       string    `json:"note"`
	ReceivedBy string    `json:"received_by"`
	CreatedAt  time.Time `json:"created_at"`
}

type Detail struct {
	Row
	SaleTotal string    `json:"sale_total"`
	SaleAt    time.Time `json:"sale_at"`
	Payments  []Payment `json:"payments"`
}

type ListParams struct {
	MemberID *uuid.UUID
	Status   string // open (bawaan) | overdue | paid | all
	Q        string
	Limit    int
	Offset   int
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

// List = piutang di semua outlet yang boleh diakses pemanggil (piutang member lintas cabang tetap satu daftar).
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
	res := ListResult{Data: []Row{}}
	member := pgtype.UUID{}
	if p.MemberID != nil {
		member = pgtype.UUID{Bytes: *p.MemberID, Valid: true}
	}
	ids := outletIDs(a)
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		qr := gen.New(tx)
		rows, err := qr.ReceivableList(ctx, gen.ReceivableListParams{TenantID: a.TenantID, OutletIds: ids, MemberID: member,
			Q: escapeLike(q), Status: p.Status, Lim: int32(p.Limit + 1), Off: int32(p.Offset)})
		if err != nil {
			return err
		}
		if len(rows) > p.Limit {
			res.HasMore = true
			rows = rows[:p.Limit]
		}
		for _, r := range rows {
			bal := r.Amount.Sub(r.Paid)
			res.Data = append(res.Data, Row{ID: r.ID, SaleID: r.SaleID, DocNo: r.DocNo, MemberID: r.MemberID, MemberCode: r.MemberCode,
				MemberName: r.MemberName, Amount: r.Amount.StringFixed(2), Paid: r.Paid.StringFixed(2), Balance: bal.StringFixed(2),
				DueDate: dateStr(r.DueDate), CreatedAt: r.CreatedAt.Time, Status: statusOf(bal, r.DueDate, r.Today)})
		}
		sm, err := qr.ReceivableSummary(ctx, gen.ReceivableSummaryParams{TenantID: a.TenantID, OutletIds: ids, MemberID: member})
		if err != nil {
			return err
		}
		res.Summary = Summary{Outstanding: sm.Outstanding.StringFixed(2), Overdue: sm.Overdue.StringFixed(2), OpenCount: sm.OpenCount, TotalCount: sm.TotalCount}
		return nil
	})
	return res, err
}

// Get = satu piutang beserta riwayat pembayarannya.
func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Detail, error) {
	var out Detail
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		out2, err := s.detail(ctx, tx, a, id)
		out = out2
		return err
	})
	return out, err
}

func (s *Service) detail(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID) (Detail, error) {
	q := gen.New(tx)
	r, err := q.ReceivableGet(ctx, gen.ReceivableGetParams{TenantID: a.TenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !a.Outlets[r.OutletID]) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	bal := r.Amount.Sub(r.Paid)
	d := Detail{
		Row: Row{ID: r.ID, SaleID: r.SaleID, DocNo: r.DocNo, MemberID: r.MemberID, MemberCode: r.MemberCode, MemberName: r.MemberName,
			Amount: r.Amount.StringFixed(2), Paid: r.Paid.StringFixed(2), Balance: bal.StringFixed(2), DueDate: dateStr(r.DueDate),
			CreatedAt: r.CreatedAt.Time, Status: statusOf(bal, r.DueDate, r.Today)},
		SaleTotal: r.SaleTotal.StringFixed(2), SaleAt: r.SaleAt.Time, Payments: []Payment{},
	}
	ps, err := q.ReceivablePaymentsList(ctx, gen.ReceivablePaymentsListParams{TenantID: a.TenantID, ReceivableID: id})
	if err != nil {
		return d, err
	}
	for _, p := range ps {
		d.Payments = append(d.Payments, Payment{ID: p.ID, DocNo: p.DocNo, Method: p.Method, MethodID: p.MethodID, MethodName: p.MethodName,
			Amount: p.Amount.StringFixed(2), RefNo: p.RefNo, FeePct: p.FeePct.StringFixed(2), Fee: p.FeeAmount.StringFixed(2),
			FeeBearer: p.FeeBearer, Note: p.Note, ReceivedBy: p.ReceivedByName, CreatedAt: p.CreatedAt.Time})
	}
	return d, nil
}

// PayInput = satu pembayaran piutang lewat satu metode (cicilan boleh; jumlah tidak boleh melebihi sisa).
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

func (n norm) hash(receivable uuid.UUID) string {
	raw, _ := json.Marshal([]string{receivable.String(), n.method.String(), n.amount.String(), n.ref, n.note})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func methodFee(amount, pct, flat dec) dec {
	if !pct.IsPositive() && !flat.IsPositive() {
		return decimal.Zero
	}
	return amount.Mul(pct).Div(decimal.NewFromInt(100)).Add(flat).Round(2)
}

type replay struct{ id uuid.UUID }

func (replay) Error() string { return "replay" }

// Pay mencatat satu pembayaran piutang di outlet aktif pemanggil (uang masuk ke laci outlet itu). replayed=true bila
// Idempotency-Key yang sama sudah pernah mencatat pembayaran ini (hasilnya sama; tidak ada pembayaran ganda).
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
		q := gen.New(tx)
		if ex, e := q.ReceivablePaymentByIdemKey(ctx, gen.ReceivablePaymentByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); e == nil {
			if ex.RequestHash != h || ex.ReceivableID != id {
				return ErrKeyMismatch
			}
			return replay{ex.ID}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		r, e := q.ReceivableLockForPay(ctx, gen.ReceivableLockForPayParams{TenantID: a.TenantID, ID: id})
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !a.Outlets[r.OutletID]) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		// Pengiriman ganda bersamaan: yang kedua menunggu kunci di atas, jadi kunci idempotensinya baru terlihat sekarang.
		if ex, e := q.ReceivablePaymentByIdemKey(ctx, gen.ReceivablePaymentByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); e == nil {
			if ex.RequestHash != h || ex.ReceivableID != id {
				return ErrKeyMismatch
			}
			return replay{ex.ID}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		paid, e := q.ReceivablePaidTotal(ctx, gen.ReceivablePaidTotalParams{TenantID: a.TenantID, ReceivableID: id})
		if e != nil {
			return e
		}
		balance := r.Amount.Sub(paid)
		switch {
		case !balance.IsPositive():
			return FieldErrors{"amount": "SETTLED"}
		case n.amount.GreaterThan(balance):
			return FieldErrors{"amount": "OVERPAID"}
		}
		m, e := q.ReceivablePaymentMethod(ctx, gen.ReceivablePaymentMethodParams{TenantID: a.TenantID, ID: n.method})
		switch {
		case errors.Is(e, pgx.ErrNoRows):
			return FieldErrors{"method_id": sanitize.Invalid}
		case e != nil:
			return e
		case !m.Active:
			return FieldErrors{"method_id": "METHOD_INACTIVE"}
		}
		out, e := q.ReceivablePayOutlet(ctx, gen.ReceivablePayOutletParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !out.Active) {
			return ErrOutletGone
		}
		if e != nil {
			return e
		}
		no, e := q.ReceivablePaymentNextNo(ctx, gen.ReceivablePaymentNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if e != nil {
			return e
		}
		docNo := fmt.Sprintf("PP-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		fee := methodFee(n.amount, m.FeePct, m.FeeFlat)
		if e := q.ReceivablePaymentInsert(ctx, gen.ReceivablePaymentInsertParams{TenantID: a.TenantID, ReceivableID: id, OutletID: a.OutletID,
			DocNo: docNo, IdempotencyKey: key, RequestHash: h, Method: m.Kind, MethodID: m.ID, MethodName: m.Name, Amount: n.amount, RefNo: n.ref,
			FeePct: m.FeePct, FeeFlat: m.FeeFlat, FeeAmount: fee, FeeBearer: m.FeeBearer, Note: n.note,
			ReceivedBy: pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}}); e != nil {
			return e
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionReceivablePay, Entity: audit.EntityReceivable, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "sale_doc_no": r.DocNo, "method": m.Name, "amount": n.amount.String(), "balance_after": balance.Sub(n.amount).String(),
				"member_id": r.MemberID.String(), "outlet_id": a.OutletID.String()}})
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

// SaleInfo = piutang yang melekat pada satu nota kredit.
type SaleInfo struct {
	ID      uuid.UUID `json:"id"`
	Amount  string    `json:"amount"`
	Paid    string    `json:"paid"`
	Balance string    `json:"balance"`
	DueDate *string   `json:"due_date,omitempty"`
	Status  string    `json:"status"` // open | overdue | paid
}

// ForSale = piutang nota (nil bila nota bukan kredit).
func ForSale(ctx context.Context, tx pgx.Tx, tenant, sale uuid.UUID) (*SaleInfo, error) {
	r, err := gen.New(tx).ReceivableForSale(ctx, gen.ReceivableForSaleParams{TenantID: tenant, SaleID: sale})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	bal := r.Amount.Sub(r.Paid)
	return &SaleInfo{ID: r.ID, Amount: r.Amount.StringFixed(2), Paid: r.Paid.StringFixed(2), Balance: bal.StringFixed(2),
		DueDate: dateStr(r.DueDate), Status: statusOf(bal, r.DueDate, r.Today)}, nil
}
