// Package accounting: akuntansi SIAK fase A (FR-ACC-01/04/05; docs/ARCHITECTURE.md §4b) — bagan akun (COA), periode
// bulanan dengan tutup buku, dan jurnal manual JU/KM/KK/TK + saldo awal. Alur jurnal: draf → posting; jurnal terposting
// immutable (dijaga juga oleh policy RLS di DB), koreksi lewat jurnal balik. Saldo per akun/bulan (account_period_balances)
// diperbarui di transaksi posting yang sama.
package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/sanitize"
)

var (
	ErrNotFound          = errors.New("data tidak ditemukan")
	ErrCodeTaken         = errors.New("kode akun sudah dipakai")
	ErrParentInvalid     = errors.New("induk akun tidak valid")
	ErrAccountInUse      = errors.New("akun masih dipakai (punya anak atau baris jurnal)")
	ErrAccountSystem     = errors.New("akun sistem tidak boleh dihapus, dinonaktifkan, atau diubah kodenya")
	ErrCOAExists         = errors.New("bagan akun sudah ada")
	ErrAccountInvalid    = errors.New("akun jurnal tidak valid") // bukan akun buku, tidak aktif, atau tidak ada
	ErrJournalUnbalanced = errors.New("debit dan kredit tidak seimbang")
	ErrJournalShape      = errors.New("susunan baris tidak sesuai jenis jurnal")
	ErrJournalPosted     = errors.New("jurnal sudah diposting")
	ErrNotPosted         = errors.New("jurnal belum diposting")
	ErrAlreadyReversed   = errors.New("jurnal sudah dibalik")
	ErrPeriodClosed      = errors.New("periode sudah ditutup")
	ErrPeriodNotClosed   = errors.New("periode belum ditutup")
	ErrDraftsInPeriod    = errors.New("masih ada jurnal draf di periode ini")
	ErrOpeningExists     = errors.New("jurnal pembuka sudah ada")
	ErrOutletForbidden   = errors.New("tidak berhak atas outlet ini")

	maxAmount = decimal.NewFromInt(1_000_000_000_000_000) // < 1e15 (CHECK DB)
)

// FieldErrors = galat validasi per field (kode stabil, diterjemahkan klien).
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

type dec = decimal.Decimal

// parseMoney: nominal ≥ 0, maksimum 2 desimal, < 1e15. Kosong = 0.
func parseMoney(raw json.Number) (dec, bool) {
	s := strings.TrimSpace(raw.String())
	if s == "" {
		return decimal.Zero, true
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(maxAmount) {
		return dec{}, false
	}
	return d, true
}

const dateLayout = "2006-01-02"

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	return t, err == nil
}

func pgDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func uuidPtr(p pgtype.UUID) *uuid.UUID {
	if !p.Valid {
		return nil
	}
	id := uuid.UUID(p.Bytes)
	return &id
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func cleanText(s string, max int, required bool) (string, string) {
	v, ok := sanitize.Text(s)
	switch {
	case !ok:
		return "", sanitize.Invalid
	case v == "" && required:
		return "", sanitize.Required
	case len([]rune(v)) > max:
		return "", sanitize.TooLong
	}
	return v, ""
}

// outletAllowed: pemilik (izin penuh) atau pengguna yang ditugaskan ke outlet itu.
func outletAllowed(a authz.Actor, outlet uuid.UUID) bool {
	return a.Perms.All || a.OutletID == outlet || a.Outlets[outlet]
}

// activeOutlet memastikan outlet milik tenant (RLS) dan aktif.
func activeOutlet(ctx context.Context, tx pgx.Tx, tenant, outlet uuid.UUID) error {
	var active bool
	err := tx.QueryRow(ctx, `SELECT active FROM outlets WHERE tenant_id = $1 AND id = $2`, tenant, outlet).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
		return FieldErrors{"outlet_id": sanitize.Invalid}
	}
	return err
}
