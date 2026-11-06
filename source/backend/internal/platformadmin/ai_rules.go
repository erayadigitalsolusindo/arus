package platformadmin

// Aturan operator HalloArus (markdown) oleh Platform Admin: satu dokumen global + satu dokumen per tenant, berversi.
// Tenant tidak punya jalan menulis/melihatnya dari UI-nya; penulisan hanya lewat fungsi DB yang memverifikasi admin aktif.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"aciraba/internal/ai"
	"aciraba/internal/platform/httpx"
)

const (
	ActionAIRulesSave = "platform.ai_rules_save"
	ActionAIRulesTest = "platform.ai_rules_test"
	MaxAIRulesChars   = 6000
	aiRulesHistory    = 30
)

type AIRuleVersion struct {
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Author    string    `json:"author"`
}

type AIRuleTenant struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantCode string    `json:"tenant_code"`
	TenantName string    `json:"tenant_name"`
	Version    int       `json:"version"`
	Size       int       `json:"size"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListAIRules = riwayat versi satu cakupan (tenantID == uuid.Nil: global), terbaru dulu.
func (s *Service) ListAIRules(ctx context.Context, a Actor, tenantID uuid.UUID) ([]AIRuleVersion, error) {
	var tid any
	if tenantID != uuid.Nil {
		tid = tenantID
	}
	rows, err := s.Pool.Query(ctx, `SELECT version, body, created_at, author FROM platform_ai_rules_list($1, $2, $3)`, a.ID, tid, aiRulesHistory)
	if err != nil {
		return nil, mapPolicyErr(err)
	}
	defer rows.Close()
	out := []AIRuleVersion{}
	for rows.Next() {
		var v AIRuleVersion
		if err := rows.Scan(&v.Version, &v.Body, &v.CreatedAt, &v.Author); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) ListAIRuleTenants(ctx context.Context, a Actor) ([]AIRuleTenant, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, tenant_code, tenant_name, version, size, created_at FROM platform_ai_rules_tenants($1)`, a.ID)
	if err != nil {
		return nil, mapPolicyErr(err)
	}
	defer rows.Close()
	out := []AIRuleTenant{}
	for rows.Next() {
		var t AIRuleTenant
		if err := rows.Scan(&t.TenantID, &t.TenantCode, &t.TenantName, &t.Version, &t.Size, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveAIRules menyimpan versi baru dan mengembalikan nomornya. Isi kosong = tidak ada aturan (versi kosong tetap tercatat).
func (s *Service) SaveAIRules(ctx context.Context, a Actor, tenantID uuid.UUID, body string) (int, error) {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if strings.ContainsRune(body, 0) {
		return 0, FieldErrors{"body": "INVALID"}
	}
	if utf8.RuneCountInString(body) > MaxAIRulesChars {
		return 0, FieldErrors{"body": "TOO_LONG"}
	}
	var tid any
	if tenantID != uuid.Nil {
		tid = tenantID
	}
	var ver int
	if err := s.Pool.QueryRow(ctx, `SELECT platform_ai_rules_save($1, $2, $3)`, a.ID, tid, body).Scan(&ver); err != nil {
		return 0, mapPolicyErr(err)
	}
	s.record(ctx, a, ActionAIRulesSave, tenantID, "", map[string]any{"global": tenantID == uuid.Nil, "version": ver, "chars": utf8.RuneCountInString(body)})
	return ver, nil
}

// ---- HTTP ----

func (h *Handler) AIRulesTenants(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListAIRuleTenants(r.Context(), actor(r))
	if err != nil {
		h.planErr(w, r, "daftar aturan AI gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "max_chars": MaxAIRulesChars})
}

func (h *Handler) listAIRules(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	items, err := h.svc.ListAIRules(r.Context(), actor(r), tenantID)
	if err != nil {
		h.planErr(w, r, "riwayat aturan AI gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GlobalAIRules(w http.ResponseWriter, r *http.Request) {
	h.listAIRules(w, r, uuid.Nil)
}

func (h *Handler) TenantAIRules(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.listAIRules(w, r, id)
	}
}

type aiRulesRequest struct {
	Body string `json:"body"`
}

func (h *Handler) saveAIRules(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req aiRulesRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	ver, err := h.svc.SaveAIRules(r.Context(), actor(r), tenantID, req.Body)
	if err != nil {
		h.planErr(w, r, "simpan aturan AI gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"version": ver})
}

func (h *Handler) SaveGlobalAIRules(w http.ResponseWriter, r *http.Request) {
	h.saveAIRules(w, r, uuid.Nil)
}

func (h *Handler) SaveTenantAIRules(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.saveAIRules(w, r, id)
	}
}

type aiRulesTestRequest struct {
	TenantID string `json:"tenant_id"`
	Draft    string `json:"draft"`
	Message  string `json:"message"`
}

// TestAIRules mencoba draf aturan pada satu pertanyaan contoh (tanpa data tenant, tanpa menyimpan apa pun).
func (h *Handler) TestAIRules(w http.ResponseWriter, r *http.Request) {
	if h.AI == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "Uji aturan tidak tersedia.")
		return
	}
	var req aiRulesTestRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	a := actor(r)
	tenantID := uuid.Nil
	if req.TenantID != "" {
		id, err := uuid.Parse(req.TenantID)
		if err != nil {
			httpx.Error(w, http.StatusUnprocessableEntity, "AI_BAD_INPUT", "Percakapan tidak valid.")
			return
		}
		tenantID = id
	}
	res, err := h.AI.TestRules(r.Context(), ai.TestRulesInput{AdminID: a.ID, TenantID: tenantID, Draft: req.Draft, Message: req.Message})
	switch {
	case err == nil:
		h.svc.record(r.Context(), a, ActionAIRulesTest, tenantID, "", map[string]any{"global": tenantID == uuid.Nil, "draft_chars": res.RulesChars, "message_chars": len([]rune(req.Message))})
		httpx.JSON(w, http.StatusOK, res)
	case errors.Is(err, ai.ErrBadInput):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BAD_INPUT", "Pertanyaan uji tidak valid.")
	case errors.Is(err, ai.ErrPlatformKeyMissing):
		httpx.Error(w, http.StatusConflict, "AI_PLATFORM_KEY_MISSING", "API key platform belum diatur.")
	case errors.Is(err, ai.ErrRateLimited):
		httpx.Error(w, http.StatusTooManyRequests, "AI_RATE_LIMITED", "Terlalu banyak uji; coba lagi sebentar.")
	case errors.Is(err, ai.ErrRuleTestFailed):
		httpx.Error(w, http.StatusBadGateway, "AI_UNAVAILABLE", "Layanan AI tidak dapat dihubungi.")
	default:
		h.internal(w, r, "uji aturan AI gagal", err)
	}
}
