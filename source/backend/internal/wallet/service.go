package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Modul izin.
const (
	ModuleDeposits = "member_deposits"  // view = saldo & riwayat; create = top-up & tarik
	ModuleCredits  = "supplier_credits" // view = saldo & riwayat; create = pencairan kredit
)

var (
	ErrNotFound      = errors.New("data tidak ditemukan")
	ErrKeyRequired   = errors.New("idempotency key wajib")
	ErrKeyMismatch   = errors.New("idempotency key dipakai untuk isi berbeda")
	ErrOutletGone    = errors.New("outlet aktif tidak tersedia")
	idemKeyPattern   = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
	maxAmount        = decimal.New(1, 12)
	historyPageLimit = 50
)

// FieldErrors = kode galat per field.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// CashInput = top-up / tarik deposit, atau pencairan kredit pemasok. Uang lewat metode bayar biasa (bukan saldo titipan).
type CashInput struct {
	Amount   json.Number `json:"amount"`
	MethodID uuid.UUID   `json:"method_id"`
	RefNo    string      `json:"ref_no"`
	Note     string      `json:"note"`
}

type cashNorm struct {
	amount     dec
	method     uuid.UUID
	ref, note  string
	ownerID    uuid.UUID
	ledgerKind string
}

func normalizeCash(in CashInput) (cashNorm, FieldErrors) {
	f := FieldErrors{}
	n := cashNorm{method: in.MethodID}
	s := strings.TrimSpace(in.Amount.String())
	if s == "" {
		f["amount"] = sanitize.Required
	} else if d, err := decimal.NewFromString(s); err != nil || !d.IsPositive() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(maxAmount) {
		f["amount"] = sanitize.Invalid
	} else {
		n.amount = d
	}
	if in.MethodID == uuid.Nil {
		f["method_id"] = sanitize.Required
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

func (n cashNorm) hash() string {
	raw, _ := json.Marshal([]string{n.ledgerKind, n.ownerID.String(), n.amount.String(), n.method.String(), n.ref, n.note})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Account = saldo satu pemilik beserta riwayatnya (terbaru dulu).
type Account struct {
	OwnerID    uuid.UUID `json:"owner_id"`
	Name       string    `json:"name"`
	Code       string    `json:"code"`
	Balance    string    `json:"balance"`
	Entries    []Entry   `json:"entries"`
	HasMore    bool      `json:"has_more"`
	NextBefore int64     `json:"next_before"` // kirim sebagai before untuk halaman berikutnya
}

func (l Ledger) ownerInfo(ctx context.Context, tx pgx.Tx, tenant, owner uuid.UUID) (name, code string, err error) {
	q := `SELECT name, code FROM members WHERE tenant_id = $1 AND id = $2`
	if l.owner == "supplier_id" {
		q = `SELECT name, coalesce(code, '') FROM suppliers WHERE tenant_id = $1 AND id = $2`
	}
	err = tx.QueryRow(ctx, q, tenant, owner).Scan(&name, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return name, code, err
}

// Account = saldo + riwayat (keyset id: before = 0 → halaman pertama).
func (s *Service) Account(ctx context.Context, a authz.Actor, l Ledger, owner uuid.UUID, before int64) (Account, error) {
	out := Account{OwnerID: owner, Entries: []Entry{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		name, code, err := l.ownerInfo(ctx, tx, a.TenantID, owner)
		if err != nil {
			return err
		}
		out.Name, out.Code = name, code
		bal, err := l.Balance(ctx, tx, a.TenantID, owner)
		if err != nil {
			return err
		}
		out.Balance = bal.StringFixed(2)
		out.Entries, out.HasMore, err = l.History(ctx, tx, a.TenantID, owner, before, historyPageLimit)
		if err == nil && out.HasMore && len(out.Entries) > 0 {
			out.NextBefore = out.Entries[len(out.Entries)-1].ID
		}
		return err
	})
	return out, err
}

type cashReplay struct{}

func (cashReplay) Error() string { return "replay" }

// Cash mencatat uang tunai/non-tunai yang mengubah saldo titipan di outlet aktif:
//   - DepTopup    : member menitip uang (saldo +, uang masuk ke toko)
//   - DepWithdraw : member mengambil deposit (saldo −, uang keluar dari toko)
//   - CrCashOut   : pemasok mencairkan kreditnya (saldo −, uang MASUK ke toko dari pemasok)
//
// Idempoten per Idempotency-Key (isi berbeda → ErrKeyMismatch). Mengembalikan saldo terbaru + riwayat.
func (s *Service) Cash(ctx context.Context, a authz.Actor, l Ledger, kind string, owner uuid.UUID, key string, in CashInput) (Account, bool, error) {
	if !idemKeyPattern.MatchString(key) {
		return Account{}, false, ErrKeyRequired
	}
	n, f := normalizeCash(in)
	if len(f) > 0 {
		return Account{}, false, f
	}
	n.ownerID, n.ledgerKind = owner, kind
	h := n.hash()
	replayed := false
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		check := func() error {
			var rh string
			e := tx.QueryRow(ctx, `SELECT request_hash FROM `+l.table+` WHERE tenant_id = $1 AND idempotency_key = $2`, a.TenantID, key).Scan(&rh)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
			if rh != h {
				return ErrKeyMismatch
			}
			return cashReplay{}
		}
		if e := check(); e != nil {
			return e
		}
		name, _, err := l.ownerInfo(ctx, tx, a.TenantID, owner)
		if err != nil {
			return err
		}
		var mKind, mName string
		var mActive bool
		e := tx.QueryRow(ctx, `SELECT kind, name, active FROM payment_methods WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.method).Scan(&mKind, &mName, &mActive)
		switch {
		case errors.Is(e, pgx.ErrNoRows):
			return FieldErrors{"method_id": sanitize.Invalid}
		case e != nil:
			return e
		case !mActive:
			return FieldErrors{"method_id": "METHOD_INACTIVE"}
		case mKind == KindDeposit || mKind == KindSupplierCredit:
			return FieldErrors{"method_id": sanitize.Invalid} // saldo titipan tidak bisa diisi dari saldo titipan
		case mKind != "cash" && n.ref == "":
			return FieldErrors{"ref_no": sanitize.Required}
		}
		var outletActive bool
		if e := tx.QueryRow(ctx, `SELECT active FROM outlets WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.OutletID).Scan(&outletActive); errors.Is(e, pgx.ErrNoRows) || (e == nil && !outletActive) {
			return ErrOutletGone
		} else if e != nil {
			return e
		}
		// Kunci saldo dulu, lalu periksa ulang kunci idempotensi (kiriman ganda bersamaan menunggu di sini).
		if e := l.Lock(ctx, tx, a.TenantID, owner); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		docNo, e := l.NextDocNo(ctx, tx, a.TenantID, a.OutletID)
		if e != nil {
			return e
		}
		amt := n.amount
		if kind != DepTopup {
			amt = amt.Neg()
		}
		after, e := l.Apply(ctx, tx, Move{TenantID: a.TenantID, OwnerID: owner, OutletID: a.OutletID, Kind: kind, Amount: amt, DocNo: docNo,
			Method: mKind, MethodID: n.method, MethodName: mName, RefNo: n.ref, Note: n.note, IdemKey: key, RequestHash: h, ActorID: a.UserID})
		if e != nil {
			if errors.Is(e, ErrInsufficient) {
				return FieldErrors{"amount": "BALANCE_INSUFFICIENT"}
			}
			return e
		}
		action, entity := audit.ActionDepositCash, audit.EntityMemberDeposit
		if l.owner == "supplier_id" {
			action, entity = audit.ActionSupplierCreditCashOut, audit.EntitySupplierCredit
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: action, Entity: entity, EntityID: owner.String(),
			Details: map[string]any{"doc_no": docNo, "kind": kind, "name": name, "amount": n.amount.StringFixed(2), "method": mName,
				"balance_after": after.StringFixed(2), "outlet_id": a.OutletID.String()}})
	})
	var rp cashReplay
	if errors.As(err, &rp) {
		replayed, err = true, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	acc, err := s.Account(ctx, a, l, owner, 0)
	return acc, replayed, err
}

// CreditRow = satu pemasok pada daftar kredit pemasok.
type CreditRow struct {
	SupplierID uuid.UUID `json:"supplier_id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Balance    string    `json:"balance"`
}

// CreditList = pemasok yang punya riwayat kredit (saldo > 0 saja bila positive), urut nama; keyset (nama, id).
func (s *Service) CreditList(ctx context.Context, a authz.Actor, q string, positive bool, cursor string, limit int) ([]CreditRow, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) > 100 {
		q = string([]rune(q)[:100])
	}
	pattern := ""
	if q != "" {
		pattern = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	}
	var curName string
	var curID uuid.UUID
	if cursor != "" {
		i := strings.LastIndex(cursor, "|")
		id, err := uuid.Parse(cursor[i+1:])
		if i < 0 || err != nil {
			return nil, "", FieldErrors{"cursor": sanitize.Invalid}
		}
		curName, curID = cursor[:i], id
	}
	out := []CreditRow{}
	next := ""
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.id, coalesce(s.code, ''), s.name, b.balance_after
			FROM suppliers s
			CROSS JOIN LATERAL (SELECT balance_after FROM supplier_credit_movements c
			                    WHERE c.tenant_id = s.tenant_id AND c.supplier_id = s.id ORDER BY c.id DESC LIMIT 1) b
			WHERE s.tenant_id = $1 AND (NOT $2::bool OR b.balance_after > 0)
			  AND ($3::text = '' OR s.name ILIKE $3 OR s.code ILIKE $3)
			  AND ($4::text = '' OR (lower(s.name), s.id) > (lower($4::text), $5::uuid))
			ORDER BY lower(s.name), s.id LIMIT $6`, a.TenantID, positive, pattern, curName, curID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r CreditRow
			var bal dec
			if err := rows.Scan(&r.SupplierID, &r.Code, &r.Name, &bal); err != nil {
				return err
			}
			r.Balance = bal.StringFixed(2)
			out = append(out, r)
		}
		return rows.Err()
	})
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = last.Name + "|" + last.SupplierID.String()
	}
	return out, next, err
}
