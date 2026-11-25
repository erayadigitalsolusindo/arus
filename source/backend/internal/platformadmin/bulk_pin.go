package platformadmin

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// ActionTenantPinSet = audit platform: PIN Penyetuju Owner tenant diatur massal oleh Platform Admin.
const ActionTenantPinSet = "platform.tenant_pin_set"

// BulkSetTenantPin menetapkan satu PIN Penyetuju yang sama untuk Owner aktif tiap tenant (program UMKM); sama dengan
// "masuk sebagai" lalu SetPin, tanpa sandi pemilik. Tenant di luar cakupan admin / tanpa Owner masuk `failed`.
func (s *Service) BulkSetTenantPin(ctx context.Context, a Actor, ids []uuid.UUID, pin string) (ok int, failed []uuid.UUID, err error) {
	if len(ids) == 0 || len(ids) > maxBulkPlan {
		return 0, nil, FieldErrors{"tenant_ids": "INVALID"}
	}
	if s.Approvals == nil {
		return 0, nil, errors.New("layanan PIN tidak tersedia")
	}
	if e := approval.ValidatePin(pin); e != nil {
		return 0, nil, e // PIN salah berlaku untuk semua, hentikan
	}
	for _, id := range ids {
		if !a.Allows(id) {
			failed = append(failed, id)
			continue
		}
		var owner uuid.UUID
		var name string
		e := db.WithTenant(ctx, s.Pool, id, func(tx pgx.Tx) error { // sama dengan pemilihan pelaku di sesi "masuk sebagai"
			if err := tx.QueryRow(ctx, `SELECT u.id FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
				WHERE u.tenant_id = $1 AND u.active AND r.is_system ORDER BY u.created_at LIMIT 1`, id).Scan(&owner); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, id).Scan(&name)
		})
		if e == nil {
			e = s.Approvals.SetPin(ctx, authz.Actor{TenantID: id, UserID: owner, Name: "Platform: " + a.Name, Impersonator: a.ID}, "", pin)
		}
		if e != nil {
			failed = append(failed, id)
			continue
		}
		ok++
		s.record(ctx, a, ActionTenantPinSet, id, name, map[string]any{"bulk": true}) // PIN tidak pernah dicatat
	}
	return ok, failed, nil
}

// BulkSetTenantPin: POST /platform/tenants/bulk-pin {tenant_ids, pin}.
func (h *Handler) BulkSetTenantPin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantIDs []uuid.UUID `json:"tenant_ids"`
		PIN       string      `json:"pin"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	n, failed, err := h.svc.BulkSetTenantPin(r.Context(), actor(r), req.TenantIDs, req.PIN)
	var fe FieldErrors
	switch {
	case errors.Is(err, approval.ErrPinFormat), errors.Is(err, approval.ErrPinWeak):
		httpx.ValidationError(w, map[string]string{"pin": "INVALID"})
		return
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
		return
	case err != nil:
		h.internal(w, r, "ubah PIN penyetuju massal gagal", err)
		return
	}
	if failed == nil {
		failed = []uuid.UUID{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"updated": n, "failed": failed})
}
