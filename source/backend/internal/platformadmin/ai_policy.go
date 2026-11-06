package platformadmin

// Kebijakan HalloArus oleh Platform Admin: aturan global + pengecualian per tenant (lihat internal/aipolicy dan migrasi 00096).
// Penulisan hanya lewat fungsi DB yang memverifikasi pelaku adalah admin aktif; tiap perubahan tercatat di audit platform.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/ai"
	"aciraba/internal/aipolicy"
	"aciraba/internal/platform/httpx"
)

const (
	ActionAIPolicySave   = "platform.ai_policy_save"
	ActionAIPolicyDelete = "platform.ai_policy_delete"
	maxAIPolicyNote      = 300
)

// AIPolicyRow = satu baris kebijakan; TenantID nil = aturan global.
type AIPolicyRow struct {
	TenantID   *uuid.UUID `json:"tenant_id"`
	TenantCode string     `json:"tenant_code,omitempty"`
	TenantName string     `json:"tenant_name,omitempty"`
	aipolicy.Values
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIPolicies struct {
	Global  AIPolicyRow   `json:"global"`
	Tenants []AIPolicyRow `json:"tenants"`
}

func (s *Service) ListAIPolicies(ctx context.Context, a Actor) (AIPolicies, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, COALESCE(tenant_code, ''), COALESCE(tenant_name, ''), enabled, domains, allow_write, allow_cost,
		allow_byok, allow_custom, allow_images, allow_history, allow_mcp, free_enabled, free_daily_limit, free_model, chat_limit, max_range_days, note, updated_at
		FROM platform_ai_policy_list($1)`, a.ID)
	if err != nil {
		return AIPolicies{}, mapPolicyErr(err)
	}
	defer rows.Close()
	out := AIPolicies{Tenants: []AIPolicyRow{}}
	for rows.Next() {
		var r AIPolicyRow
		if err := rows.Scan(&r.TenantID, &r.TenantCode, &r.TenantName, &r.Enabled, &r.Domains, &r.AllowWrite, &r.AllowCost, &r.AllowBYOK, &r.AllowCustom,
			&r.AllowImages, &r.AllowHistory, &r.AllowMCP, &r.FreeEnabled, &r.FreeDailyLimit, &r.FreeModel, &r.ChatLimit, &r.MaxRangeDays, &r.Note, &r.UpdatedAt); err != nil {
			return AIPolicies{}, err
		}
		if r.TenantID == nil {
			out.Global = r
		} else {
			out.Tenants = append(out.Tenants, r)
		}
	}
	return out, rows.Err()
}

func mapPolicyErr(err error) error {
	switch pgCode(err) {
	case "42501":
		return ErrInvalidSession
	case "23503": // tenant tidak ada
		return ErrNotFound
	case "23514", "22P02", "22003": // CHECK / tipe / rentang
		return FieldErrors{"policy": "INVALID"}
	}
	return err
}

// SaveAIPolicy menyimpan aturan global (tenantID == uuid.Nil) atau pengecualian satu tenant.
func (s *Service) SaveAIPolicy(ctx context.Context, a Actor, tenantID uuid.UUID, v aipolicy.Values, note string) error {
	global := tenantID == uuid.Nil
	if f := v.Validate(global, ai.ValidFreeModel); len(f) > 0 {
		return FieldErrors(f)
	}
	if v.Domains != nil {
		v.Domains = dedupe(v.Domains)
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxAIPolicyNote {
		return FieldErrors{"note": "TOO_LONG"}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var tid any
	if !global {
		tid = tenantID
	}
	var found bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_ai_policy_save($1, $2, $3::jsonb, $4)`, a.ID, tid, string(raw), note).Scan(&found); err != nil {
		return mapPolicyErr(err)
	}
	if !found {
		return ErrNotFound
	}
	s.record(ctx, a, ActionAIPolicySave, tenantID, "", map[string]any{"global": global, "values": v, "note": note})
	return nil
}

// DeleteAIPolicy menghapus pengecualian tenant (kembali ikut aturan global).
func (s *Service) DeleteAIPolicy(ctx context.Context, a Actor, tenantID uuid.UUID) error {
	var found bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_ai_policy_delete($1, $2)`, a.ID, tenantID).Scan(&found); err != nil {
		return mapPolicyErr(err)
	}
	if !found {
		return ErrNotFound
	}
	s.record(ctx, a, ActionAIPolicyDelete, tenantID, "", nil)
	return nil
}

// ---- HTTP ----

func (h *Handler) ListAIPolicies(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.ListAIPolicies(r.Context(), actor(r))
	if err != nil {
		h.planErr(w, r, "daftar kebijakan AI gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"global": p.Global, "tenants": p.Tenants, "domains": aipolicy.Domains, "domain_tools": aipolicy.DomainTools(), "free_models": ai.FreeModels()})
}

type aiPolicyRequest struct {
	aipolicy.Values
	Note string `json:"note"`
}

func (h *Handler) saveAIPolicy(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req aiPolicyRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SaveAIPolicy(r.Context(), actor(r), tenantID, req.Values, req.Note); err != nil {
		h.planErr(w, r, "simpan kebijakan AI gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) SaveGlobalAIPolicy(w http.ResponseWriter, r *http.Request) {
	h.saveAIPolicy(w, r, uuid.Nil)
}

func (h *Handler) SaveTenantAIPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.saveAIPolicy(w, r, id)
}

func (h *Handler) DeleteTenantAIPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAIPolicy(r.Context(), actor(r), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Pengecualian tidak ditemukan.")
			return
		}
		h.planErr(w, r, "hapus kebijakan AI gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
