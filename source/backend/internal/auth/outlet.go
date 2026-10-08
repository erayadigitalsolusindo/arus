package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// SwitchOutlet memindahkan sesi ke outlet lain: memeriksa akses lewat fungsi akun (satu aturan akses yang sama dengan
// login/refresh), menyimpan pilihan pada rantai sesi (bertahan lintas refresh), dan menerbitkan token akses dengan oid
// baru. Refresh token tidak dirotasi (RefreshToken kosong), sehingga cookie tidak berubah.
func (s *Service) SwitchOutlet(ctx context.Context, a authz.Actor, refreshToken string, outletID uuid.UUID) (*Session, error) {
	return s.SwitchOutletWith(ctx, a, refreshToken, outletID, SwitchOpts{})
}

// SwitchOpts: pindah outlet dari layar kasir (POS=true) wajib disetujui. Pemegang izin `outlet_switch.approve`
// (Owner/Supervisor) memindahkan dirinya sendiri tanpa PIN; selain itu Approval wajib dan diverifikasi di server.
type SwitchOpts struct {
	POS      bool
	Approval *ApprovalIn
}

type ApprovalIn struct {
	UserID uuid.UUID `json:"user_id"`
	PIN    string    `json:"pin"`
}

// SwitchOutletWith = SwitchOutlet dengan aturan persetujuan kasir.
func (s *Service) SwitchOutletWith(ctx context.Context, a authz.Actor, refreshToken string, outletID uuid.UUID, o SwitchOpts) (*Session, error) {
	uid, family, ok, err := s.Sessions.FamilyOf(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	// Cookie refresh harus milik pengguna yang sama dengan token akses (mencegah menggabungkan dua identitas).
	if !ok || uid != a.UserID.String() {
		return nil, ErrInvalidSession
	}
	acc, err := accountByUserID(ctx, s.Pool, a.UserID, outletID)
	if isNoAccount(err) || (err == nil && (!acc.UserActive || !acc.TenantActive || acc.TenantID != a.TenantID)) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	// Fungsi akun memilih outlet yang diminta HANYA bila boleh diakses; selain itu mengembalikan outlet lain.
	if acc.OutletID != outletID {
		return nil, ErrOutletForbidden
	}
	// Persetujuan dicek setelah akses outlet terbukti (PIN tidak terbakar oleh outlet terlarang) dan sebelum sesi diubah.
	var approvedBy *approval.Approver
	if o.POS && outletID != a.OutletID && !a.Perms.Has(approval.ModuleOutletSwitch, authz.ActApprove) {
		if s.Approvals == nil || o.Approval == nil {
			return nil, approval.ErrPinRequired
		}
		err := db.WithTenant(ctx, s.Pool, a.TenantID, func(tx pgx.Tx) error {
			ap, err := s.Approvals.VerifyFor(ctx, tx, a, approval.ModuleOutletSwitch, outletID, o.Approval.UserID, o.Approval.PIN)
			approvedBy = &ap
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	if err := s.Sessions.SetOutlet(ctx, family, outletID.String()); err != nil {
		return nil, err
	}
	access, err := s.issueAccess(acc, s.now())
	if err != nil {
		return nil, err
	}
	// Audit pelengkap: kegagalan mencatat tidak membatalkan perpindahan.
	details := map[string]any{"from": a.OutletID.String(), "to": outletID.String()}
	if o.POS {
		details["pos"] = true
	}
	if approvedBy != nil {
		details["approved_by"] = approvedBy.Name
	}
	_ = db.WithTenant(ctx, s.Pool, acc.TenantID, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, auditActor(acc), audit.Entry{
			Action: audit.ActionOutletSwitch, Entity: audit.EntityOutlet, EntityID: outletID.String(),
			Details: details,
		})
	})
	return buildSession(acc, a.Perms, access, "", false), nil
}
