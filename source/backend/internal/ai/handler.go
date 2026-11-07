package ai

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/shift"
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	r.Route("/ai", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(Module, authz.ActView)).Get("/settings", h.GetSettings)
		r.With(authz.Require(Module, authz.ActView)).Post("/chat", h.Chat)
		r.With(authz.Require(Module, authz.ActView)).Get("/conversations", h.ListConversations)
		r.With(authz.Require(Module, authz.ActView)).Delete("/conversations", h.DeleteAllConversations)
		r.With(authz.Require(Module, authz.ActView)).Get("/conversations/{id}", h.GetConversation)
		r.With(authz.Require(Module, authz.ActView)).Patch("/conversations/{id}", h.RenameConversation)
		r.With(authz.Require(Module, authz.ActView)).Delete("/conversations/{id}", h.DeleteConversation)
		r.With(authz.Require(Module, authz.ActUpdate)).Put("/prefs", h.PutPrefs)
		r.With(authz.Require(Module, authz.ActView)).Get("/connections", h.Connections)
		r.With(authz.Require(Module, authz.ActView)).Delete("/connections/{id}", h.RevokeConnection)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/actions/{id}/confirm", h.ConfirmAction)
		r.With(authz.Require(Module, authz.ActCreate)).Post("/actions/{id}/cancel", h.CancelAction)
		r.With(authz.Require(Module, authz.ActUpdate)).Post("/custom", h.SaveCustom)
		r.With(authz.Require(Module, authz.ActUpdate)).Put("/custom/{id}", h.SaveCustom)
		r.With(authz.Require(Module, authz.ActUpdate)).Put("/settings", h.PutSettings)
		r.With(authz.Require(Module, authz.ActUpdate)).Delete("/settings/key", h.DeleteKey)
	})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var up *upstreamError
	var fields shift.FieldErrors
	var tl *TOTPLockedError
	var me *MasterError
	if errors.As(err, &me) {
		if me.Fields != nil {
			httpx.ValidationError(w, me.Fields)
		} else {
			httpx.Error(w, me.Status, me.Code, me.Msg)
		}
		return
	}
	if errors.As(err, &tl) {
		httpx.Retry(w, "TOTP_LOCKED", "Terlalu banyak kode 2FA salah. Coba lagi nanti.", time.Duration(tl.RetryAfter)*time.Second)
		return
	}
	if approval.Fail(w, err) {
		return
	}
	switch {
	case errors.Is(err, ErrConvNotFound):
		httpx.Error(w, http.StatusNotFound, "AI_CONVERSATION_NOT_FOUND", "Percakapan tidak ditemukan atau sudah dihapus.")
	case errors.Is(err, ErrConvFull):
		httpx.Error(w, http.StatusConflict, "AI_CONVERSATION_FULL", "Percakapan ini sudah penuh; mulai percakapan baru.")
	case errors.Is(err, ErrBadBaseURL):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BASE_URL_INVALID", "Base URL tidak diizinkan: gunakan alamat https publik (bukan localhost atau jaringan internal).")
	case errors.Is(err, ErrTooManyCustom):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_CUSTOM_LIMIT", "Jumlah penyedia kustom sudah mencapai batas; hapus salah satu dulu.")
	case errors.Is(err, ErrImageProvider):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_IMAGE_UNSUPPORTED", "Foto nota hanya bisa dibaca dengan penyedia Claude (Anthropic).")
	case errors.Is(err, ErrTOTPRequired):
		httpx.Error(w, http.StatusUnprocessableEntity, "TOTP_REQUIRED", "Kode 2FA wajib diisi.")
	case errors.Is(err, ErrTOTPInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "TOTP_INVALID", "Kode 2FA salah.")
	case errors.Is(err, ErrMFANotEnabled):
		httpx.Error(w, http.StatusConflict, "MFA_NOT_ENABLED", "Aktifkan 2FA di profil Anda dulu untuk menjalankan aksi tulis lewat HalloArus.")
	case errors.Is(err, ErrActionNotFound):
		httpx.Error(w, http.StatusNotFound, "AI_ACTION_NOT_FOUND", "Usulan tidak ditemukan atau sudah kedaluwarsa; minta asisten mengusulkan ulang.")
	case errors.Is(err, ErrActionForbid), errors.Is(err, shift.ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak berhak menjalankan aksi ini.")
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, shift.ErrRequired):
		httpx.Error(w, http.StatusConflict, "SHIFT_REQUIRED", "Buka shift kasir terlebih dahulu.")
	case errors.Is(err, shift.ErrNotOpen):
		httpx.Error(w, http.StatusConflict, "SHIFT_CLOSED", "Shift ini sudah ditutup.")
	case errors.Is(err, shift.ErrMovementNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, shift.ErrMovementVoided):
		httpx.Error(w, http.StatusConflict, "PETTY_CASH_VOIDED", "Catatan kas kecil sudah dibatalkan.")
	case errors.Is(err, shift.ErrNoCashMethod):
		httpx.Error(w, http.StatusConflict, "NO_CASH_METHOD", "Metode pembayaran tunai belum tersedia.")
	case errors.Is(err, shift.ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Permintaan ini sudah dipakai untuk isian yang berbeda.")
	case errors.Is(err, ErrNotConfigured):
		httpx.Error(w, http.StatusConflict, "AI_NOT_CONFIGURED", "API key AI belum diatur.")
	case errors.Is(err, ErrBadKey):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_KEY_INVALID", "API key ditolak oleh penyedia AI.")
	case errors.Is(err, ErrRateLimited):
		httpx.Error(w, http.StatusTooManyRequests, "AI_RATE_LIMITED", "Terlalu banyak pertanyaan; coba lagi beberapa menit lagi.")
	case errors.Is(err, ErrFreeQuota):
		httpx.Error(w, http.StatusTooManyRequests, "AI_FREE_QUOTA_EXHAUSTED", "Kuota akun gratis hari ini habis.")
	case errors.Is(err, ErrBadInput):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_BAD_INPUT", "Percakapan tidak valid.")
	case errors.Is(err, ErrTooManySteps):
		httpx.Error(w, http.StatusUnprocessableEntity, "AI_TOO_MANY_STEPS", "Analisis terlalu panjang; pecah menjadi pertanyaan yang lebih spesifik.")
	case errors.Is(err, ErrPolicyDisabled):
		httpx.Error(w, http.StatusForbidden, "AI_DISABLED_BY_PLATFORM", "HalloArus dinonaktifkan oleh operator platform untuk akun ini.")
	case errors.Is(err, ErrPolicyBlocked):
		httpx.Error(w, http.StatusForbidden, "AI_BLOCKED_BY_PLATFORM", "Fitur ini dibatasi oleh operator platform untuk akun ini.")
	case errors.Is(err, ErrImpersonating):
		httpx.Error(w, http.StatusForbidden, "AI_FORBIDDEN", "Asisten AI tidak tersedia pada sesi masuk-sebagai.")
	case errors.As(err, &up):
		status := http.StatusBadGateway
		if up.Code == "AI_KEY_INVALID" || up.Code == "AI_NO_CREDIT" {
			status = http.StatusUnprocessableEntity
		}
		h.log.WarnContext(r.Context(), "ai upstream", "code", up.Code, "status", up.Status)
		httpx.Error(w, status, up.Code, "Layanan AI tidak dapat memproses permintaan.")
	case errors.Is(err, context.Canceled):
		// klien menutup koneksi; tidak perlu respons
	default:
		h.log.ErrorContext(r.Context(), "ai", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

type putSettingsReq struct {
	Provider string `json:"provider"` // kosong = anthropic
	APIKey   string `json:"api_key"`  // kosong = hanya ganti model / pindah penyedia
	Model    string `json:"model"`
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in putSettingsReq
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	if in.Provider == "" {
		in.Provider = ProviderAnthropic
	}
	if err := h.svc.Save(r.Context(), a, in.Provider, in.APIKey, in.Model); err != nil {
		h.fail(w, r, err)
		return
	}
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = ProviderAnthropic
	}
	if err := h.svc.RemoveKey(r.Context(), a, provider); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chatBodyLimit: teks + sampai maxImages foto nota (base64 ≈ 4/3 ukuran asli).
const chatBodyLimit = 256<<10 + maxImages*(maxImageBytes*4/3+1024)

type chatReq struct {
	Messages       []ChatMessage `json:"messages"`
	ConversationID string        `json:"conversation_id"` // lanjutkan percakapan tersimpan; kosong = baru
	Temporary      bool          `json:"temporary"`       // true = jangan simpan ke riwayat
}

// Chat: POST /ai/chat. Satu pertanyaan bisa memakan beberapa putaran tool (puluhan detik): batas tulis server (30 dtk) dilonggarkan
// khusus untuk request ini.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in chatReq
	if !httpx.DecodeJSONLimit(w, r, &in, chatBodyLimit) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	res, err := h.svc.ChatWith(ctx, a, in.Messages, ChatOptions{ConversationID: in.ConversationID, Temporary: in.Temporary})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Connections(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	list, err := h.svc.Connections(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"connections": list})
}

func (h *Handler) RevokeConnection(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
		return
	}
	if err := h.svc.RevokeConnection(r.Context(), a, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
			return
		}
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type confirmReq struct {
	Approval *shift.ApprovalIn `json:"approval"` // penyetuju + PIN (wajib untuk semua aksi tulis)
	TOTP     string            `json:"totp"`     // kode 2FA pengguna sendiri (wajib untuk semua aksi tulis)
}

// ConfirmAction: POST /ai/actions/{id}/confirm. Pengguna menekan Setujui pada kartu usulan; PIN dikirim langsung dari UI.
func (h *Handler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in confirmReq
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	res, err := h.svc.ConfirmAction(r.Context(), a, chi.URLParam(r, "id"), in.Approval, in.TOTP)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) CancelAction(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	if err := h.svc.CancelAction(r.Context(), a, chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type customReq struct {
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"` // kosong (saat ubah) = pakai key tersimpan, hanya bila base URL tidak berubah
}

// SaveCustom: POST /ai/custom (baru) atau PUT /ai/custom/{id}. Key diverifikasi ke endpoint penyedia sebelum disimpan terenkripsi.
func (h *Handler) SaveCustom(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in customReq
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if _, err := h.svc.SaveCustom(ctx, a, CustomInput{ID: chi.URLParam(r, "id"), Label: in.Label, BaseURL: in.BaseURL, Model: in.Model, APIKey: in.APIKey}); err != nil {
		h.fail(w, r, err)
		return
	}
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 30
	}
	page, err := h.svc.ListConversations(r.Context(), a, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) convID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "AI_CONVERSATION_NOT_FOUND", "Percakapan tidak ditemukan atau sudah dihapus.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	id, ok := h.convID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.GetConversation(r.Context(), a, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) RenameConversation(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	id, ok := h.convID(w, r)
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 2<<10) {
		return
	}
	if err := h.svc.RenameConversation(r.Context(), a, id, in.Title); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	id, ok := h.convID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteConversation(r.Context(), a, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteAllConversations(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	n, err := h.svc.DeleteAllConversations(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int64{"deleted": n})
}

// PutPrefs: saklar simpan riwayat milik tenant (pemilik/pengelola AI).
func (h *Handler) PutPrefs(w http.ResponseWriter, r *http.Request) {
	a, _ := authz.ActorFrom(r.Context())
	var in struct {
		SaveHistory bool `json:"save_history"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 1<<10) {
		return
	}
	if err := h.svc.SetHistoryEnabled(r.Context(), a, in.SaveHistory); err != nil {
		h.fail(w, r, err)
		return
	}
	st, err := h.svc.Settings(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}
