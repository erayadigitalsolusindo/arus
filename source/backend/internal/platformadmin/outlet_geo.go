package platformadmin

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

type outletGeoRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// SetOutletLocation mengisi/menghapus koordinat satu outlet (keduanya terisi atau keduanya null; sama dengan CHECK di DB).
func (s *Service) SetOutletLocation(ctx context.Context, a Actor, tenantID, outletID uuid.UUID, lat, lng *float64) error {
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `UPDATE outlets SET latitude = $3, longitude = $4 WHERE tenant_id = $1 AND id = $2 RETURNING name`,
			tenantID, outletID, lat, lng).Scan(&name); err != nil {
			return err
		}
		d := map[string]any{"outlet_id": outletID.String(), "outlet": name, "latitude": lat, "longitude": lng}
		if err := s.recordTx(ctx, tx, a, ActionOutletGeo, tenantID, "", d); err != nil {
			return err
		}
		d["admin_id"] = a.ID.String()
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformOutletGeo, Entity: audit.EntityOutlet, EntityID: outletID.String(), Details: d,
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// SetOutletLocation: PUT /platform/tenants/{id}/outlets/{outletId}/location
func (h *Handler) SetOutletLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	outletID, err := uuid.Parse(chi.URLParam(r, "outletId"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Outlet tidak ditemukan.")
		return
	}
	var req outletGeoRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if (req.Latitude == nil) != (req.Longitude == nil) ||
		(req.Latitude != nil && (*req.Latitude < -90 || *req.Latitude > 90 || *req.Longitude < -180 || *req.Longitude > 180)) {
		httpx.ValidationError(w, map[string]string{"latitude": "INVALID"})
		return
	}
	switch err := h.svc.SetOutletLocation(r.Context(), actor(r), id, outletID, req.Latitude, req.Longitude); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Outlet tidak ditemukan.")
	default:
		h.internal(w, r, "ubah lokasi outlet gagal", err)
	}
}
