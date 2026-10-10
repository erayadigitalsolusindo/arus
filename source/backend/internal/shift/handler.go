package shift

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
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
	r.Route("/shifts", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		cashier := authz.Require(ModuleCashier, authz.ActCreate)
		r.With(cashier).Get("/current", h.Current)
		r.With(cashier).Post("/open", h.Open)
		r.With(authz.Require(Module, authz.ActView)).Get("/", h.List)
		// Rincian & tutup: pemilik shift (kasir) atau supervisor; aturan detailnya di service.
		r.With(authz.RequireAny([2]string{ModuleCashier, authz.ActCreate}, [2]string{Module, authz.ActView})).Get("/{id}", h.Get)
		r.With(authz.RequireAny([2]string{ModuleCashier, authz.ActCreate}, [2]string{Module, authz.ActUpdate})).Post("/{id}/close", h.Close)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// Current: GET /shifts/current → {shift: null | Shift} (shift terbuka milik pemanggil di outlet aktif + rekap berjalan).
func (h *Handler) Current(w http.ResponseWriter, r *http.Request) {
	sh, err := h.svc.Current(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"shift": sh})
}

// Open: POST /shifts/open {opening_cash}.
func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	var in OpenInput
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	sh, err := h.svc.Open(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sh)
}

// List: GET /shifts/?from=&to=&user_id=&status=open|closed&diff=1&cursor=&limit=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{From: qs.Get("from"), To: qs.Get("to"), Status: qs.Get("status"), DiffOnly: qs.Get("diff") == "1", Cursor: qs.Get("cursor")}
	if s := qs.Get("user_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"user_id": "INVALID"})
			return
		}
		p.UserID = &id
	}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	res, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	sh, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sh)
}

// Close: POST /shifts/{id}/close (header Idempotency-Key wajib) {counts:[{method_id,counted}], note, approval}.
func (h *Handler) Close(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var in CloseInput
	if !httpx.DecodeJSONLimit(w, r, &in, 16<<10) {
		return
	}
	sh, err := h.svc.Close(r.Context(), actor(r), id, r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sh)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	var de *DiffError
	if approval.Fail(w, err) {
		return
	}
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.As(err, &de):
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "SHIFT_DIFF_NOTE_REQUIRED",
			"message": "Ada selisih uang; isi catatan dan minta persetujuan penyetuju (PIN).", "diff": de.Diff}})
	case errors.Is(err, ErrAlreadyOpen):
		httpx.Error(w, http.StatusConflict, "SHIFT_ALREADY_OPEN", "Shift Anda di outlet ini sudah terbuka.")
	case errors.Is(err, ErrNotOpen):
		httpx.Error(w, http.StatusConflict, "SHIFT_CLOSED", "Shift ini sudah ditutup.")
	case errors.Is(err, ErrRecapChanged):
		httpx.Error(w, http.StatusConflict, "SHIFT_RECAP_CHANGED", "Ada transaksi baru sejak rekap ditampilkan. Muat ulang rekap lalu hitung lagi.")
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk permintaan yang berbeda.")
	case errors.Is(err, ErrOutletInactive):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, "FORBIDDEN", "Anda tidak berhak atas shift ini.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	default:
		h.log.ErrorContext(r.Context(), "shift", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}
