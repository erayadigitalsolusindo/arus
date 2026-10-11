package shift

// Kas kecil kasir: uang keluar/masuk laci di luar penjualan (beli galon, parkir, tambah receh). Terikat shift terbuka
// kasir dan otomatis masuk "arus lain" di rekap (recapSQL), sehingga uang laci yang seharusnya tetap cocok.
// Kas keluar di atas batas outlet butuh PIN penyetuju (izin shift_close.approve). Pembatalan hanya selama shift terbuka.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	categoryMax  = 40
	pettyNoteMax = 300
	pettyListMax = 200 // batas baris kas kecil yang dimuat per shift
)

var (
	ErrMovementNotFound = errors.New("catatan kas kecil tidak ditemukan")
	ErrMovementVoided   = errors.New("catatan kas kecil sudah dibatalkan")
	ErrNoCashMethod     = errors.New("metode tunai belum tersedia")
)

// Movement = satu catatan kas kecil.
type Movement struct {
	ID           uuid.UUID  `json:"id"`
	Direction    string     `json:"direction"` // in|out
	Amount       string     `json:"amount"`
	Category     string     `json:"category"`
	Note         string     `json:"note"`
	CreatedAt    time.Time  `json:"created_at"`
	CreatedLocal string     `json:"created_local"`
	ApprovedBy   string     `json:"approved_by_name,omitempty"`
	VoidedAt     *time.Time `json:"voided_at,omitempty"`
}

type MovementInput struct {
	Direction string      `json:"direction"`
	Amount    json.Number `json:"amount"`
	Category  string      `json:"category"`
	Note      string      `json:"note"`
	Approval  *ApprovalIn `json:"approval"`
}

// loadMovements: catatan kas kecil satu shift (terbaru dulu), dipakai Get/Current.
func loadMovements(ctx context.Context, tx pgx.Tx, tenant, shift uuid.UUID, tz string) ([]Movement, error) {
	rows, err := tx.Query(ctx, `SELECT id, direction, amount, category, note, created_at, coalesce(approved_name, ''), voided_at
		FROM cash_movements WHERE tenant_id = $1 AND shift_id = $2 ORDER BY created_at DESC, id DESC LIMIT $3`, tenant, shift, pettyListMax)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Movement{}
	for rows.Next() {
		var m Movement
		var amt dec
		if err := rows.Scan(&m.ID, &m.Direction, &amt, &m.Category, &m.Note, &m.CreatedAt, &m.ApprovedBy, &m.VoidedAt); err != nil {
			return nil, err
		}
		m.Amount, m.CreatedLocal = amt.StringFixed(2), localTime(m.CreatedAt, tz)
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMovement mencatat kas kecil pada shift terbuka milik pemanggil. Idempoten lewat `key`.
func (s *Service) AddMovement(ctx context.Context, a authz.Actor, key string, in MovementInput) (Movement, error) {
	if !keyPattern.MatchString(key) {
		return Movement{}, ErrKeyRequired
	}
	fe := FieldErrors{}
	if in.Direction != "in" && in.Direction != "out" {
		fe["direction"] = sanitize.Invalid
	}
	amt, ok := parseMoney(in.Amount, false)
	if !ok || !amt.IsPositive() {
		fe["amount"] = sanitize.Invalid
	}
	cat, code := sanitize.Name(in.Category, categoryMax)
	if code != "" {
		fe["category"] = code
	}
	note, code := sanitize.Multiline(in.Note, pettyNoteMax)
	if code != "" {
		fe["note"] = code
	}
	if len(fe) > 0 {
		return Movement{}, fe
	}

	var out Movement
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		// Kunci baris shift FOR SHARE: tutup shift (FOR UPDATE) menunggu catatan yang sedang disimpan, dan catatan
		// sesudah shift ditutup ditolak.
		var shiftID uuid.UUID
		var limit dec
		var tz string
		err := tx.QueryRow(ctx, `SELECT c.id, o.petty_cash_approval_limit, o.timezone
			FROM cash_shifts c JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
			WHERE c.tenant_id = $1 AND c.outlet_id = $2 AND c.user_id = $3 AND c.status = 'open' FOR SHARE OF c`,
			a.TenantID, a.OutletID, a.UserID).Scan(&shiftID, &limit, &tz)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRequired
		}
		if err != nil {
			return err
		}
		// Kirim ulang dengan kunci sama → hasil yang sama (bila isinya sama).
		var prior uuid.UUID
		var pShift uuid.UUID
		var pAmt dec
		var pDir string
		err = tx.QueryRow(ctx, `SELECT id, shift_id, amount, direction FROM cash_movements WHERE tenant_id = $1 AND idem_key = $2`, a.TenantID, key).
			Scan(&prior, &pShift, &pAmt, &pDir)
		if err == nil {
			if pShift != shiftID || !pAmt.Equal(amt) || pDir != in.Direction {
				return ErrKeyMismatch
			}
			ms, err := loadMovements(ctx, tx, a.TenantID, shiftID, tz)
			if err != nil {
				return err
			}
			for _, m := range ms {
				if m.ID == prior {
					out = m
				}
			}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var approverID *uuid.UUID
		var approverName *string
		if in.Direction == "out" && amt.GreaterThan(limit) {
			if in.Approval == nil || s.approvals == nil {
				return approval.ErrPinRequired
			}
			ap, err := s.approvals.VerifyFor(ctx, tx, a, ModuleApprove, a.OutletID, in.Approval.UserID, in.Approval.Pin)
			if err != nil {
				return err
			}
			approverID, approverName = &ap.ID, &ap.Name
		}
		var cashID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash' ORDER BY is_system DESC, created_at LIMIT 1`, a.TenantID).Scan(&cashID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoCashMethod
		}
		if err != nil {
			return err
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO cash_movements (tenant_id, outlet_id, shift_id, method_id, direction, amount, category, note, created_by, approved_by, approved_name, idem_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
			a.TenantID, a.OutletID, shiftID, cashID, in.Direction, amt, cat, note, a.UserID, approverID, approverName, key).Scan(&id); err != nil {
			return err
		}
		det := map[string]any{"direction": in.Direction, "amount": amt.StringFixed(2), "category": cat}
		if approverID != nil {
			det["approved_by"], det["approver_name"] = approverID.String(), *approverName
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionCashMovementCreate, Entity: audit.EntityCashMovement, EntityID: id.String(), Details: det}); err != nil {
			return err
		}
		ms, err := loadMovements(ctx, tx, a.TenantID, shiftID, tz)
		if err != nil {
			return err
		}
		for _, m := range ms {
			if m.ID == id {
				out = m
			}
		}
		return nil
	})
	return out, err
}

// VoidMovement membatalkan catatan kas kecil selama shift-nya masih terbuka. Pemilik shift atau pemegang
// cash_shifts.update (di outlet itu) boleh membatalkan.
func (s *Service) VoidMovement(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var outlet, owner, shiftID uuid.UUID
		var status, dir string
		var amt dec
		var voided *time.Time
		err := tx.QueryRow(ctx, `SELECT m.outlet_id, c.user_id, c.id, c.status, m.direction, m.amount, m.voided_at
			FROM cash_movements m JOIN cash_shifts c ON c.tenant_id = m.tenant_id AND c.id = m.shift_id
			WHERE m.tenant_id = $1 AND m.id = $2 FOR UPDATE OF m`, a.TenantID, id).Scan(&outlet, &owner, &shiftID, &status, &dir, &amt, &voided)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMovementNotFound
		}
		if err != nil {
			return err
		}
		own := owner == a.UserID && outlet == a.OutletID
		if !own && !(a.Perms.Has(Module, authz.ActUpdate) && (outlet == a.OutletID || a.Outlets[outlet])) {
			if outlet != a.OutletID && !a.Outlets[outlet] {
				return ErrMovementNotFound
			}
			return ErrForbidden
		}
		if voided != nil {
			return ErrMovementVoided
		}
		// Shift ditutup bersamaan: kunci shift (FOR SHARE) agar tutup shift tidak menyela di antara cek dan update.
		if err := tx.QueryRow(ctx, `SELECT status FROM cash_shifts WHERE tenant_id = $1 AND id = $2 FOR SHARE`, a.TenantID, shiftID).Scan(&status); err != nil {
			return err
		}
		if status != "open" {
			return ErrNotOpen
		}
		if _, err := tx.Exec(ctx, `UPDATE cash_movements SET voided_at = clock_timestamp(), voided_by = $3 WHERE tenant_id = $1 AND id = $2`, a.TenantID, id, a.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionCashMovementVoid, Entity: audit.EntityCashMovement, EntityID: id.String(),
			Details: map[string]any{"direction": dir, "amount": amt.StringFixed(2)}})
	})
}

// SetPettyCashLimit mengatur batas kas keluar tanpa PIN untuk outlet aktif (0 = semua kas keluar butuh PIN).
func (s *Service) SetPettyCashLimit(ctx context.Context, a authz.Actor, raw json.Number) (string, error) {
	v, ok := parseMoney(raw, true)
	if !ok {
		return "", FieldErrors{"limit": sanitize.Invalid}
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE outlets SET petty_cash_approval_limit = $3 WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.OutletID, v)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrOutletInactive
		}
		return nil
	})
	return v.StringFixed(2), err
}
