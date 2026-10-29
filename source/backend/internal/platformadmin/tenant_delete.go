package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/platform/httpx"
)

// Hapus tenant permanen. Dua tahap: jadwalkan (tenant wajib nonaktif, masa tunggu ditentukan admin, 0 = langsung) lalu
// purge setelah jatuh tempo (manual oleh admin, atau otomatis oleh RunTenantPurger). Isi database dihapus oleh fungsi
// `tenant_purge_core`; catatan audit platform sengaja bertahan.

const (
	maxDeleteDays      = 365
	purgeInterval      = 10 * time.Minute
	purgeBatch         = 5
	purgeTimeout       = 30 * time.Minute
	errCodeRaiseCustom = "P0001"
)

var (
	ErrTenantActive    = errors.New("tenant masih aktif")
	ErrInvalidDays     = errors.New("masa tunggu tidak valid")
	ErrNameMismatch    = errors.New("nama tenant tidak cocok")
	ErrPurgeNotDue     = errors.New("belum dijadwalkan atau belum jatuh tempo")
	ErrNotScheduledDel = errors.New("tenant tidak dijadwalkan dihapus")
)

// DeleteSchedule = status jadwal hapus satu tenant (nil = tidak dijadwalkan).
type DeleteSchedule struct {
	DeleteAt    time.Time `json:"delete_at"`
	RequestedAt time.Time `json:"requested_at"`
	Due         bool      `json:"due"`
}

// DeleteSchedule membaca jadwal hapus tenant; nil bila tidak dijadwalkan.
func (s *Service) DeleteSchedule(ctx context.Context, a Actor, tenantID uuid.UUID) (*DeleteSchedule, error) {
	var at, req pgtype.Timestamptz
	err := s.Pool.QueryRow(ctx, `SELECT delete_at, delete_requested_at FROM platform_tenant_delete_info($1, $2)`, a.ID, tenantID).Scan(&at, &req)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !at.Valid {
		return nil, nil
	}
	return &DeleteSchedule{DeleteAt: at.Time, RequestedAt: req.Time, Due: !at.Time.After(s.now())}, nil
}

// ScheduleDelete menjadwalkan hapus permanen setelah `days` hari (0 = langsung boleh dipurge). Tenant harus nonaktif.
// Mengulang panggilan = mengubah masa tunggu.
func (s *Service) ScheduleDelete(ctx context.Context, a Actor, tenantID uuid.UUID, days int) (time.Time, error) {
	if days < 0 || days > maxDeleteDays {
		return time.Time{}, ErrInvalidDays
	}
	t, err := s.tenantName(ctx, tenantID)
	if err != nil {
		return time.Time{}, err
	}
	var at pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, `SELECT platform_tenant_schedule_delete($1, $2, $3)`, a.ID, tenantID, days).Scan(&at); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == errCodeRaiseCustom && strings.Contains(pe.Message, "masih aktif") {
			return time.Time{}, ErrTenantActive
		}
		return time.Time{}, err
	}
	if !at.Valid {
		return time.Time{}, ErrNotFound
	}
	s.record(ctx, a, ActionTenantDeleteSchedule, tenantID, t, map[string]any{"days": days, "delete_at": at.Time})
	return at.Time, nil
}

// CancelDelete membatalkan jadwal hapus.
func (s *Service) CancelDelete(ctx context.Context, a Actor, tenantID uuid.UUID) error {
	t, err := s.tenantName(ctx, tenantID)
	if err != nil {
		return err
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_tenant_cancel_delete($1, $2)`, a.ID, tenantID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotScheduledDel
	}
	s.record(ctx, a, ActionTenantDeleteCancel, tenantID, t, nil)
	return nil
}

// PurgeTenant menghapus tenant permanen bila sudah jatuh tempo. confirmName harus sama persis dengan nama tenant.
func (s *Service) PurgeTenant(ctx context.Context, a Actor, tenantID uuid.UUID, confirmName string) (map[string]int64, error) {
	name, err := s.tenantName(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(confirmName) != name {
		return nil, ErrNameMismatch
	}
	// Tenant besar bisa butuh beberapa menit: jangan batalkan di tengah jalan bila klien memutus koneksi.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), purgeTimeout)
	defer cancel()
	rows, err := s.purge(pctx, `SELECT platform_tenant_purge($1, $2)`, a.ID, tenantID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, ErrPurgeNotDue
	}
	s.record(ctx, a, ActionTenantPurge, tenantID, name, map[string]any{"rows": rows})
	s.afterPurge(ctx, tenantID)
	return rows, nil
}

// purge menjalankan fungsi purge; nil = tidak dijalankan (belum jatuh tempo / sudah tidak ada).
func (s *Service) purge(ctx context.Context, sql string, args ...any) (map[string]int64, error) {
	var raw []byte
	if err := s.Pool.QueryRow(ctx, sql, args...).Scan(&raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var res struct {
		Rows map[string]int64 `json:"rows"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	if res.Rows == nil {
		res.Rows = map[string]int64{}
	}
	return res.Rows, nil
}

// afterPurge membersihkan efek di luar database: cache izin dan berkas unggahan tenant.
func (s *Service) afterPurge(ctx context.Context, tenantID uuid.UUID) {
	s.Perms.InvalidateTenant(tenantID)
	if s.Files != nil {
		_ = s.Files.DeleteTree(ctx, tenantID.String()) // berkas yatim tak berbahaya; gagal hapus tidak menggagalkan purge
	}
}

func (s *Service) tenantName(ctx context.Context, tenantID uuid.UUID) (string, error) {
	var name string
	err := s.Pool.QueryRow(ctx, `SELECT name FROM platform_tenant_name($1)`, tenantID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// RunTenantPurger menghapus tenant nonaktif yang masa tunggunya habis, berkala, sampai ctx selesai.
func (s *Service) RunTenantPurger(ctx context.Context, log *slog.Logger) {
	t := time.NewTicker(purgeInterval)
	defer t.Stop()
	for {
		s.purgeDue(ctx, log)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// purgeDue mengembalikan jumlah tenant yang dihapus (untuk test).
func (s *Service) purgeDue(ctx context.Context, log *slog.Logger) int {
	rows, err := s.Pool.Query(ctx, `SELECT id, name FROM platform_tenant_due_purges($1)`, purgeBatch)
	if err != nil {
		log.Warn("hapus tenant terjadwal: gagal membaca daftar", "err", err)
		return 0
	}
	type due struct {
		id   uuid.UUID
		name string
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.name); err != nil {
			rows.Close()
			return 0
		}
		list = append(list, d)
	}
	rows.Close()
	n := 0
	for _, d := range list {
		res, err := s.purge(ctx, `SELECT platform_tenant_purge_due($1)`, d.id)
		if err != nil {
			log.Error("hapus tenant terjadwal gagal", "tenant", d.id, "err", err)
			continue
		}
		if res == nil {
			continue
		}
		s.record(ctx, Actor{Name: "Penjadwal otomatis"}, ActionTenantPurge, d.id, d.name, map[string]any{"rows": res, "auto": true})
		s.afterPurge(ctx, d.id)
		log.Info("tenant dihapus permanen sesuai jadwal", "tenant", d.id, "name", d.name)
		n++
	}
	return n
}

// --- HTTP ---

type scheduleDeleteRequest struct {
	Days *int `json:"days"`
}

type purgeRequest struct {
	ConfirmName string `json:"confirm_name"`
}

// GetDeleteSchedule: GET /platform/tenants/{id}/deletion.
func (h *Handler) GetDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	sch, err := h.svc.DeleteSchedule(r.Context(), actor(r), id)
	if h.deletionError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"schedule": sch})
}

// ScheduleDelete: PUT /platform/tenants/{id}/deletion {"days": n}.
func (h *Handler) ScheduleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req scheduleDeleteRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Days == nil {
		httpx.ValidationError(w, map[string]string{"days": "REQUIRED"})
		return
	}
	at, err := h.svc.ScheduleDelete(r.Context(), actor(r), id, *req.Days)
	if h.deletionError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"delete_at": at})
}

// CancelDelete: DELETE /platform/tenants/{id}/deletion.
func (h *Handler) CancelDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if h.deletionError(w, r, h.svc.CancelDelete(r.Context(), actor(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PurgeTenant: POST /platform/tenants/{id}/purge {"confirm_name": "..."}.
func (h *Handler) PurgeTenant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req purgeRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	rows, err := h.svc.PurgeTenant(r.Context(), actor(r), id, req.ConfirmName)
	if h.deletionError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deleted": true, "rows": rows})
}

// deletionError menulis respons galat dan mengembalikan true bila err != nil.
func (h *Handler) deletionError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
	case errors.Is(err, ErrTenantActive):
		httpx.Error(w, http.StatusConflict, "TENANT_ACTIVE", "Nonaktifkan tenant lebih dulu sebelum menghapusnya.")
	case errors.Is(err, ErrInvalidDays):
		httpx.ValidationError(w, map[string]string{"days": "INVALID"})
	case errors.Is(err, ErrNameMismatch):
		httpx.ValidationError(w, map[string]string{"confirm_name": "MISMATCH"})
	case errors.Is(err, ErrPurgeNotDue):
		httpx.Error(w, http.StatusConflict, "NOT_DUE", "Tenant belum dijadwalkan hapus atau masa tunggu belum habis.")
	case errors.Is(err, ErrNotScheduledDel):
		httpx.Error(w, http.StatusConflict, "NOT_SCHEDULED", "Tenant tidak sedang dijadwalkan dihapus.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	default:
		h.internal(w, r, "hapus tenant gagal", err)
	}
	return true
}
