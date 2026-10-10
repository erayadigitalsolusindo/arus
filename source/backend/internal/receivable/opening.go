package receivable

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// ModuleOpening = izin saldo awal piutang (create = catat, delete = batalkan).
const ModuleOpening = "receivable_opening"

// maxOpeningAgeDays = tanggal dokumen lama paling jauh ke belakang (±10 tahun).
const maxOpeningAgeDays = 3660

var (
	ErrNotOpening  = errors.New("bukan saldo awal piutang")
	ErrHasPayments = errors.New("saldo awal sudah dibayar")
	ErrVoided      = errors.New("saldo awal sudah dibatalkan")
)

// lockForPay mengunci piutang sebelum saldonya dibaca: piutang nota → baris nota (berbagi kunci dengan edit/batal nota);
// saldo awal → baris piutangnya sendiri (berbagi kunci dengan pembatalan). Saldo awal yang dibatalkan = tidak ditemukan.
func lockForPay(ctx context.Context, q *gen.Queries, tenant, id uuid.UUID) (gen.ReceivableLockForPayRow, error) {
	kind, err := q.ReceivableKind(ctx, gen.ReceivableKindParams{TenantID: tenant, ID: id})
	if err != nil {
		return gen.ReceivableLockForPayRow{}, err
	}
	if kind != "opening" {
		return q.ReceivableLockForPay(ctx, gen.ReceivableLockForPayParams{TenantID: tenant, ID: id})
	}
	r, err := q.ReceivableLockOpening(ctx, gen.ReceivableLockOpeningParams{TenantID: tenant, ID: id})
	if err != nil {
		return gen.ReceivableLockForPayRow{}, err
	}
	if r.VoidedAt.Valid {
		return gen.ReceivableLockForPayRow{}, pgx.ErrNoRows
	}
	return gen.ReceivableLockForPayRow{ID: r.ID, OutletID: r.OutletID, MemberID: r.MemberID, Amount: r.Amount, DocNo: r.DocNo.String, Returned: decimal.Zero}, nil
}

// OpeningInput = satu saldo awal piutang member dari catatan lama toko.
type OpeningInput struct {
	MemberID uuid.UUID   `json:"member_id"`
	RefNo    string      `json:"ref_no"`   // no. nota/bon lama (opsional, unik per member)
	DocDate  string      `json:"doc_date"` // tanggal dokumen lama YYYY-MM-DD (≤ hari ini)
	Amount   json.Number `json:"amount"`   // sisa piutang yang belum dibayar
	DueDate  string      `json:"due_date"` // opsional, ≥ doc_date
	Note     string      `json:"note"`
}

type openingNorm struct {
	member         uuid.UUID
	ref, note      string
	docDate, due   time.Time
	hasDue         bool
	amount         dec
	docRaw, dueRaw string
}

func normalizeOpening(in OpeningInput) (openingNorm, FieldErrors) {
	f := FieldErrors{}
	n := openingNorm{member: in.MemberID}
	if in.MemberID == uuid.Nil {
		f["member_id"] = sanitize.Required
	}
	ref, ok := sanitize.Text(in.RefNo)
	if !ok || utf8.RuneCountInString(ref) > 64 {
		f["ref_no"] = sanitize.Invalid
	}
	n.ref = ref
	note, ok := sanitize.Text(in.Note)
	if !ok || utf8.RuneCountInString(note) > 200 {
		f["note"] = sanitize.Invalid
	}
	n.note = note
	str := strings.TrimSpace(in.Amount.String())
	switch d, err := decimal.NewFromString(str); {
	case str == "":
		f["amount"] = sanitize.Required
	case err != nil || !d.IsPositive() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxAmount)):
		f["amount"] = sanitize.Invalid
	default:
		n.amount = d
	}
	n.docRaw = strings.TrimSpace(in.DocDate)
	if n.docRaw == "" {
		f["doc_date"] = sanitize.Required
	} else if d, err := time.Parse("2006-01-02", n.docRaw); err != nil {
		f["doc_date"] = sanitize.Invalid
	} else {
		n.docDate = d
	}
	n.dueRaw = strings.TrimSpace(in.DueDate)
	if n.dueRaw != "" {
		if d, err := time.Parse("2006-01-02", n.dueRaw); err != nil {
			f["due_date"] = sanitize.Invalid
		} else {
			n.due, n.hasDue = d, true
			if f["doc_date"] == "" && !n.docDate.IsZero() && d.Before(n.docDate) {
				f["due_date"] = "BEFORE_START"
			}
		}
	}
	return n, f
}

func (n openingNorm) hash() string {
	raw, _ := json.Marshal([]string{n.member.String(), n.ref, n.docRaw, n.amount.String(), n.dueRaw, n.note})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CreateOpening mencatat saldo awal piutang di outlet aktif (idempoten lewat Idempotency-Key). Saldo awal tidak dicek
// terhadap limit kredit member (utang lama yang sudah terjadi), tetapi sesudahnya ikut dihitung untuk nota kredit baru.
func (s *Service) CreateOpening(ctx context.Context, a authz.Actor, key string, in OpeningInput) (d Detail, replayed bool, err error) {
	if !idemKey.MatchString(key) {
		return Detail{}, false, ErrKeyRequired
	}
	n, f := normalizeOpening(in)
	if len(f) > 0 {
		return Detail{}, false, f
	}
	h := n.hash()
	var id uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		check := func() error {
			ex, e := q.ReceivableOpeningByIdemKey(ctx, gen.ReceivableOpeningByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: pgtype.Text{String: key, Valid: true}})
			if e == nil {
				if ex.RequestHash.String != h {
					return ErrKeyMismatch
				}
				id = ex.ID
				return replay{ex.ID}
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		out, e := q.ReceivablePayOutlet(ctx, gen.ReceivablePayOutletParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !out.Active) {
			return ErrOutletGone
		}
		if e != nil {
			return e
		}
		fe := FieldErrors{}
		if n.docDate.After(out.LocalDay.Time) || out.LocalDay.Time.Sub(n.docDate) > maxOpeningAgeDays*24*time.Hour {
			fe["doc_date"] = sanitize.Invalid
		}
		var memberName string
		if e := tx.QueryRow(ctx, `SELECT name FROM members WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.member).Scan(&memberName); errors.Is(e, pgx.ErrNoRows) {
			fe["member_id"] = sanitize.Invalid
		} else if e != nil {
			return e
		}
		if len(fe) > 0 {
			return fe
		}
		if n.ref != "" {
			taken, e := q.ReceivableOpeningRefTaken(ctx, gen.ReceivableOpeningRefTakenParams{TenantID: a.TenantID, MemberID: n.member, RefNo: n.ref})
			if e != nil {
				return e
			}
			if taken {
				return FieldErrors{"ref_no": "DUPLICATE"}
			}
		}
		no, e := q.ReceivableOpeningNextNo(ctx, gen.ReceivableOpeningNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if e != nil {
			return e
		}
		docNo := fmt.Sprintf("SA-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		due := pgtype.Date{}
		if n.hasDue {
			due = pgtype.Date{Time: n.due, Valid: true}
		}
		id, e = q.ReceivableOpeningInsert(ctx, gen.ReceivableOpeningInsertParams{TenantID: a.TenantID, OutletID: a.OutletID, MemberID: n.member,
			Amount: n.amount, DueDate: due, DocNo: pgtype.Text{String: docNo, Valid: true}, RefNo: n.ref, DocDate: pgtype.Date{Time: n.docDate, Valid: true},
			Note: n.note, CreatedBy: pgtype.UUID{Bytes: a.UserID, Valid: true}, IdempotencyKey: pgtype.Text{String: key, Valid: true},
			RequestHash: pgtype.Text{String: h, Valid: true}})
		if e != nil {
			var pe *pgconn.PgError
			if errors.As(e, &pe) && pe.Code == "23505" {
				switch pe.ConstraintName {
				case "receivables_opening_ref_key":
					return FieldErrors{"ref_no": "DUPLICATE"}
				case "receivables_idem_key":
					return ErrKeyMismatch // kiriman bersamaan dengan kunci sama: yang lain sudah menyimpan
				}
			}
			return e
		}
		det := map[string]any{"doc_no": docNo, "member_id": n.member.String(), "member": memberName, "amount": n.amount.String(),
			"doc_date": n.docRaw, "ref_no": n.ref}
		if n.hasDue {
			det["due_date"] = n.dueRaw
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionReceivableOpening, Entity: audit.EntityReceivable, EntityID: id.String(), Details: det})
	})
	var rp replay
	if errors.As(err, &rp) {
		d, err = s.Get(ctx, a, rp.id)
		return d, true, err
	}
	if err != nil {
		return Detail{}, false, err
	}
	d, err = s.Get(ctx, a, id)
	return d, false, err
}

// VoidOpening membatalkan saldo awal yang belum pernah dibayar (salah input). Piutang nota tidak bisa dibatalkan di
// sini (batalkan notanya). Baris dikunci sehingga pembayaran dan pembatalan bersamaan tepat satu yang menang.
func (s *Service) VoidOpening(ctx context.Context, a authz.Actor, id uuid.UUID, reason string) (Detail, error) {
	reason, ok := sanitize.Text(reason)
	if !ok || utf8.RuneCountInString(reason) < 3 || utf8.RuneCountInString(reason) > 200 {
		return Detail{}, FieldErrors{"reason": sanitize.Invalid}
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		kind, e := q.ReceivableKind(ctx, gen.ReceivableKindParams{TenantID: a.TenantID, ID: id})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if kind != "opening" {
			return ErrNotOpening
		}
		r, e := q.ReceivableLockOpening(ctx, gen.ReceivableLockOpeningParams{TenantID: a.TenantID, ID: id})
		if e != nil {
			return e
		}
		if !a.Outlets[r.OutletID] {
			return ErrNotFound
		}
		if r.VoidedAt.Valid {
			return ErrVoided
		}
		paid, e := q.ReceivablePaidTotal(ctx, gen.ReceivablePaidTotalParams{TenantID: a.TenantID, ReceivableID: id})
		if e != nil {
			return e
		}
		if paid.IsPositive() {
			return ErrHasPayments
		}
		if e := q.ReceivableVoid(ctx, gen.ReceivableVoidParams{TenantID: a.TenantID, ID: id, VoidedBy: pgtype.UUID{Bytes: a.UserID, Valid: true},
			VoidReason: pgtype.Text{String: reason, Valid: true}}); e != nil {
			return e
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionReceivableOpeningVoid, Entity: audit.EntityReceivable, EntityID: id.String(),
			Details: map[string]any{"doc_no": r.DocNo.String, "amount": r.Amount.String(), "member_id": r.MemberID.String(), "reason": reason}})
	})
	if err != nil {
		return Detail{}, err
	}
	return s.Get(ctx, a, id)
}
