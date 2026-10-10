// Package wallet: dua saldo titipan berbentuk ledger append-only (keputusan pengguna 2026-10-10):
//   - deposit member   (member_deposit_movements): uang pelanggan yang dititip di toko;
//   - kredit pemasok   (supplier_credit_movements): uang pemasok yang masih ditahan toko (mis. dari retur pembelian).
//
// Saldo = balance_after baris terakhir pemilik saldo (bukan kolom yang dimutasi, AGENTS.md §8). Setiap penulisan mengambil
// advisory lock transaksi per pemilik saldo, membaca saldo terakhir, lalu menulis baris baru; saldo tidak pernah minus
// (ErrInsufficient + CHECK di DB). Fungsi di sini dipanggil DI DALAM transaksi use-case pemanggil (nota, piutang, hutang,
// retur), yang bertanggung jawab atas izin dan akses outlet.
package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type dec = decimal.Decimal

// Jenis metode bayar internal (lihat payment_methods; satu metode sistem per tenant).
const (
	KindDeposit        = "deposit"
	KindSupplierCredit = "supplier_credit"
)

// Ledger = satu jenis saldo.
type Ledger struct {
	table    string
	owner    string // kolom pemilik saldo
	lockTag  string
	docPrefx string // prefiks nomor dokumen tunai (top-up/tarik, pencairan)
}

var (
	MemberDeposit  = Ledger{table: "member_deposit_movements", owner: "member_id", lockTag: "deposit:", docPrefx: "DM"}
	SupplierCredit = Ledger{table: "supplier_credit_movements", owner: "supplier_id", lockTag: "suppliercredit:", docPrefx: "KP"}
)

// Jenis baris ledger.
const (
	DepTopup             = "TOPUP"
	DepWithdraw          = "WITHDRAW"
	DepSalePayment       = "SALE_PAYMENT"
	DepSaleReversal      = "SALE_REVERSAL"
	DepReceivablePayment = "RECEIVABLE_PAYMENT"
	DepSaleReturn        = "SALE_RETURN"
	DepSaleReturnVoid    = "SALE_RETURN_VOID"

	CrPurchaseReturn     = "PURCHASE_RETURN"
	CrPurchaseReturnVoid = "PURCHASE_RETURN_VOID"
	CrPayablePayment     = "PAYABLE_PAYMENT"
	CrCashOut            = "CASH_OUT"
)

// ErrInsufficient = saldo tidak cukup untuk pengurangan.
var ErrInsufficient = errors.New("saldo tidak mencukupi")

// InsufficientError membawa saldo yang tersedia (untuk pesan ke pengguna).
type InsufficientError struct{ Balance dec }

func (e *InsufficientError) Error() string { return "saldo tidak mencukupi" }
func (e *InsufficientError) Unwrap() error { return ErrInsufficient }

// Move = satu baris ledger. Amount bertanda: positif menambah saldo, negatif mengurangi.
type Move struct {
	TenantID, OwnerID, OutletID uuid.UUID
	Kind                        string
	Amount                      dec
	RefID                       uuid.UUID // dokumen sumber (nota, retur, pembayaran); Nil = tidak ada
	DocNo                       string
	Method, MethodName          string
	MethodID                    uuid.UUID
	RefNo, Note                 string
	IdemKey, RequestHash        string
	ActorID                     uuid.UUID
}

// Lock mengambil advisory lock transaksi untuk saldo pemilik. Dipanggil sebelum membaca saldo bila keputusan bergantung
// padanya (Apply melakukannya sendiri).
func (l Ledger) Lock(ctx context.Context, tx pgx.Tx, tenant, owner uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, l.lockTag+tenant.String()+":"+owner.String())
	return err
}

// Balance = saldo terakhir (0 bila belum ada baris).
func (l Ledger) Balance(ctx context.Context, tx pgx.Tx, tenant, owner uuid.UUID) (dec, error) {
	var b dec
	err := tx.QueryRow(ctx, `SELECT balance_after FROM `+l.table+` WHERE tenant_id = $1 AND `+l.owner+` = $2 ORDER BY id DESC LIMIT 1`,
		tenant, owner).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, nil
	}
	return b, err
}

// Apply menulis satu baris (mengunci saldo pemilik dulu). Pengurangan melebihi saldo → *InsufficientError.
func (l Ledger) Apply(ctx context.Context, tx pgx.Tx, m Move) (dec, error) {
	if m.Amount.IsZero() {
		return decimal.Zero, errors.New("wallet: jumlah nol")
	}
	if err := l.Lock(ctx, tx, m.TenantID, m.OwnerID); err != nil {
		return decimal.Zero, err
	}
	bal, err := l.Balance(ctx, tx, m.TenantID, m.OwnerID)
	if err != nil {
		return decimal.Zero, err
	}
	after := bal.Add(m.Amount)
	if after.IsNegative() {
		return bal, &InsufficientError{Balance: bal}
	}
	_, err = tx.Exec(ctx, `INSERT INTO `+l.table+` (tenant_id, `+l.owner+`, outlet_id, kind, amount, balance_after, ref_id, doc_no,
		method, method_id, method_name, ref_no, note, idempotency_key, request_hash, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		m.TenantID, m.OwnerID, m.OutletID, m.Kind, m.Amount, after, optUUID(m.RefID), m.DocNo,
		optText(m.Method), optUUID(m.MethodID), m.MethodName, m.RefNo, m.Note, optText(m.IdemKey), optText(m.RequestHash), optUUID(m.ActorID))
	return after, err
}

// SumByRef = Σ amount baris berjenis kind yang merujuk dokumen ref (mis. deposit yang dipakai sebuah nota).
func (l Ledger) SumByRef(ctx context.Context, tx pgx.Tx, tenant, ref uuid.UUID, kind string) (dec, error) {
	var s dec
	err := tx.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM `+l.table+` WHERE tenant_id = $1 AND ref_id = $2 AND kind = $3`,
		tenant, ref, kind).Scan(&s)
	return s, err
}

// NextDocNo = nomor dokumen tunai berikutnya {PREFIKS}-{OUTLET}-{YYMMDD}-{NNNN} (hari menurut zona waktu outlet).
func (l Ledger) NextDocNo(ctx context.Context, tx pgx.Tx, tenant, outlet uuid.UUID) (string, error) {
	var code string
	var day pgtype.Date
	if err := tx.QueryRow(ctx, `SELECT code, (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`, tenant, outlet).
		Scan(&code, &day); err != nil {
		return "", err
	}
	var no int64
	if err := tx.QueryRow(ctx, `INSERT INTO wallet_doc_counters (tenant_id, outlet_id, prefix, day, last_no) VALUES ($1, $2, $3, $4, 1)
		ON CONFLICT (tenant_id, outlet_id, prefix, day) DO UPDATE SET last_no = wallet_doc_counters.last_no + 1 RETURNING last_no`,
		tenant, outlet, l.docPrefx, day).Scan(&no); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s-%04d", l.docPrefx, strings.ToUpper(code), day.Time.Format("060102"), no), nil
}

// MethodID = id metode sistem berjenis kind (deposit / supplier_credit) milik tenant.
func MethodID(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, kind string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var name string
	err := tx.QueryRow(ctx, `SELECT id, name FROM payment_methods WHERE tenant_id = $1 AND kind = $2`, tenant, kind).Scan(&id, &name)
	return id, name, err
}

// Entry = satu baris ledger untuk ditampilkan (riwayat).
type Entry struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Amount     string    `json:"amount"`
	Balance    string    `json:"balance_after"`
	RefID      *string   `json:"ref_id,omitempty"`
	DocNo      string    `json:"doc_no"`
	MethodName string    `json:"method_name"`
	RefNo      string    `json:"ref_no"`
	Note       string    `json:"note"`
	OutletName string    `json:"outlet_name"`
	ActorName  string    `json:"actor_name"`
	CreatedAt  time.Time `json:"created_at"`
}

// History = riwayat saldo pemilik, terbaru dulu, keyset id (before = 0 → halaman pertama). hasMore bila masih ada.
func (l Ledger) History(ctx context.Context, tx pgx.Tx, tenant, owner uuid.UUID, before int64, limit int) ([]Entry, bool, error) {
	rows, err := tx.Query(ctx, `SELECT m.id, m.kind, m.amount, m.balance_after, m.ref_id, m.doc_no, m.method_name, m.ref_no, m.note,
		       coalesce(o.name, ''), coalesce(u.name, ''), m.created_at
		FROM `+l.table+` m
		LEFT JOIN outlets o ON o.tenant_id = m.tenant_id AND o.id = m.outlet_id
		LEFT JOIN users u ON u.tenant_id = m.tenant_id AND u.id = m.actor_id
		WHERE m.tenant_id = $1 AND m.`+l.owner+` = $2 AND ($3::bigint = 0 OR m.id < $3)
		ORDER BY m.id DESC LIMIT $4`, tenant, owner, before, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var amt, bal dec
		var ref pgtype.UUID
		var at pgtype.Timestamptz
		if err := rows.Scan(&e.ID, &e.Kind, &amt, &bal, &ref, &e.DocNo, &e.MethodName, &e.RefNo, &e.Note, &e.OutletName, &e.ActorName, &at); err != nil {
			return nil, false, err
		}
		e.Amount, e.Balance, e.CreatedAt = amt.StringFixed(2), bal.StringFixed(2), at.Time
		if ref.Valid {
			s := uuid.UUID(ref.Bytes).String()
			e.RefID = &s
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

func optUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }
func optText(s string) pgtype.Text     { return pgtype.Text{String: s, Valid: s != ""} }

// ReverseByRef membalik semua baris berjenis fromKind yang merujuk dokumen ref (per pemilik saldo) dengan baris toKind
// bertanda berlawanan, mis. deposit yang dipakai nota dikembalikan saat nota dibatalkan. Total yang dibalik dikembalikan
// per pemilik; pemilik tanpa baris dilewati. Pengurangan yang melebihi saldo → *InsufficientError (seluruh transaksi batal).
func (l Ledger) ReverseByRef(ctx context.Context, tx pgx.Tx, tenant, ref uuid.UUID, fromKind, toKind string, outlet, actor uuid.UUID, docNo, note string) (map[uuid.UUID]dec, error) {
	rows, err := tx.Query(ctx, `SELECT `+l.owner+`, sum(amount) FROM `+l.table+` WHERE tenant_id = $1 AND ref_id = $2 AND kind = $3
		GROUP BY `+l.owner+` ORDER BY `+l.owner, tenant, ref, fromKind)
	if err != nil {
		return nil, err
	}
	type pair struct {
		owner uuid.UUID
		sum   dec
	}
	var ps []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.owner, &p.sum); err != nil {
			rows.Close()
			return nil, err
		}
		ps = append(ps, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[uuid.UUID]dec{}
	for _, p := range ps {
		if p.sum.IsZero() {
			continue
		}
		if _, err := l.Apply(ctx, tx, Move{TenantID: tenant, OwnerID: p.owner, OutletID: outlet, Kind: toKind, Amount: p.sum.Neg(),
			RefID: ref, DocNo: docNo, Note: note, ActorID: actor}); err != nil {
			return nil, err
		}
		out[p.owner] = p.sum.Neg()
	}
	return out, nil
}
