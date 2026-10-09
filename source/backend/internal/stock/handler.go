package stock

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// OpeningModule = modul izin Saldo Awal Stok (authz.Modules): view = lihat, create = isi/ubah, approve = kunci.
const OpeningModule = "stock_opening"

// ConversionModule = modul izin Pecah Satuan: view = lihat riwayat, create = membuat dokumen.
const ConversionModule = "stock_conversion"

// CountModule = modul izin Stok Opname: view = lihat, create = buat/isi/batal draf, approve = selesaikan (menerapkan selisih).
const CountModule = "stock_opname"

// TransferModule = modul izin Mutasi Stok: view = lihat, create = kirim, approve = terima/batal.
const TransferModule = "stock_transfer"

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
	r.Route("/stock/opening", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(OpeningModule, authz.ActView)).Get("/status", h.Status)
		r.With(authz.Require(OpeningModule, authz.ActView)).Get("/items", h.List)
		r.With(authz.Require(OpeningModule, authz.ActCreate)).Put("/items/{id}", h.Set)
		r.With(authz.Require(OpeningModule, authz.ActApprove)).Post("/lock", h.Lock)
	})
	r.Route("/stock/card", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate, authz.Require(CardModule, authz.ActView))
		r.Get("/", h.Card)
		r.Get("/items", h.CardItems)
	})
	r.Route("/stock/conversions", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/", h.ListConversions)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/items", h.List)
		r.With(authz.Require(ConversionModule, authz.ActCreate)).Post("/", h.Convert)
		r.With(authz.Require(ConversionModule, authz.ActView)).Get("/{id}", h.GetConversion)
	})
	r.Route("/stock/transfers", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(TransferModule, authz.ActView)).Get("/", h.ListTransfers)
		r.With(authz.RequireAny([2]string{TransferModule, authz.ActCreate})).Get("/destinations", h.TransferDestinations)
		r.With(authz.RequireAny([2]string{TransferModule, authz.ActCreate})).Get("/items", h.List)
		r.With(authz.Require(TransferModule, authz.ActCreate)).Post("/", h.SendTransfer)
		r.With(authz.Require(TransferModule, authz.ActView)).Get("/{id}", h.GetTransfer)
		r.With(authz.Require(TransferModule, authz.ActApprove)).Post("/{id}/receive", h.ReceiveTransfer)
		r.With(authz.Require(TransferModule, authz.ActApprove)).Post("/{id}/cancel", h.CancelTransfer)
	})
	r.Route("/stock/counts", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(CountModule, authz.ActView)).Get("/", h.ListCounts)
		r.With(authz.Require(CountModule, authz.ActCreate)).Post("/", h.CreateCount)
		r.With(authz.RequireAny([2]string{CountModule, authz.ActCreate}, [2]string{CountModule, authz.ActApprove})).Get("/items", h.List)
		r.With(authz.Require(CountModule, authz.ActApprove)).Post("/quick", h.QuickCount)
		r.With(authz.Require(CountModule, authz.ActView)).Get("/{id}", h.GetCount)
		r.With(authz.Require(CountModule, authz.ActCreate)).Post("/{id}/items", h.AddCountItems)
		r.With(authz.Require(CountModule, authz.ActCreate)).Put("/{id}/items/{itemId}", h.SetCounted)
		r.With(authz.Require(CountModule, authz.ActCreate)).Delete("/{id}/items/{itemId}", h.RemoveCountItem)
		r.With(authz.Require(CountModule, authz.ActApprove)).Post("/{id}/complete", h.CompleteCount)
		r.With(authz.Require(CountModule, authz.ActCreate)).Post("/{id}/cancel", h.CancelCount)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Status(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

// List: ?q=&limit=&offset=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	rows, total, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req struct {
		Bucket string      `json:"bucket"`
		Qty    json.Number `json:"qty"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	bal, err := h.svc.SetOpening(r.Context(), actor(r), id, req.Bucket, req.Qty.String())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bucket": req.Bucket, "qty": bal.String()})
}

func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartDate string `json:"start_date"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	st, err := h.svc.Lock(r.Context(), actor(r), req.StartDate)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet dokumen ini.")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrItemNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrNotStocked):
		httpx.Error(w, http.StatusUnprocessableEntity, "NOT_STOCKED", "Barang ini tidak memiliki stok.")
	case errors.Is(err, ErrInsufficient):
		httpx.Error(w, http.StatusConflict, "STOCK_INSUFFICIENT", "Stok barang tidak mencukupi.")
	case errors.Is(err, ErrTransferNotPending):
		httpx.Error(w, http.StatusConflict, "TRANSFER_NOT_PENDING", "Mutasi sudah diterima atau dibatalkan.")
	case errors.Is(err, ErrKeyRequired):
		httpx.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Header Idempotency-Key wajib (8–100 karakter).")
	case errors.Is(err, ErrKeyMismatch):
		httpx.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH", "Idempotency-Key sudah dipakai untuk permintaan yang berbeda.")
	case errors.Is(err, ErrOutletInactive):
		httpx.Error(w, http.StatusConflict, "OUTLET_NOT_FOUND", "Outlet aktif tidak ditemukan atau tidak aktif.")
	case errors.Is(err, ErrOpeningLocked):
		httpx.Error(w, http.StatusConflict, "OPENING_LOCKED", "Saldo awal sudah dikunci; gunakan stok opname.")
	case errors.Is(err, ErrAlreadyLocked):
		httpx.Error(w, http.StatusConflict, "ALREADY_LOCKED", "Tanggal mulai operasional sudah dikunci.")
	default:
		h.log.ErrorContext(r.Context(), "stock opening", "err", err, "path", r.URL.Path)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

// Convert: POST /stock/conversions — header `Idempotency-Key` wajib. 201 = dokumen baru; 200 + `Idempotent-Replay: true` = kunci sama.
func (h *Handler) Convert(w http.ResponseWriter, r *http.Request) {
	var req ConvertInput
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	conv, replayed, err := h.svc.Convert(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, conv)
}

// ListConversions: ?limit=&offset=
func (h *Handler) ListConversions(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	offset, _ := strconv.Atoi(qs.Get("offset"))
	rows, total, err := h.svc.ListConversions(r.Context(), actor(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

func (h *Handler) GetConversion(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	conv, err := h.svc.GetConversion(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, conv)
}

func idParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

// ListCounts: ?status=draft|completed|cancelled&limit=&offset=
func (h *Handler) ListCounts(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	offset, _ := strconv.Atoi(qs.Get("offset"))
	rows, total, err := h.svc.ListCounts(r.Context(), actor(r), qs.Get("status"), qs.Get("kind"), qs.Get("q"), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	sum, err := h.svc.CountsSummary(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total, "summary": sum})
}

func (h *Handler) CreateCount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bucket string `json:"bucket"`
		Note   string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, err := h.svc.CreateCount(r.Context(), actor(r), req.Bucket, req.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.GetCount(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

// QuickCount: POST /stock/counts/quick — opname langsung (tanpa draf), langsung menyesuaikan stok. Butuh izin approve.
func (h *Handler) QuickCount(w http.ResponseWriter, r *http.Request) {
	var req QuickInput
	if !httpx.DecodeJSONLimit(w, r, &req, 64<<10) {
		return
	}
	id, err := h.svc.QuickCount(r.Context(), actor(r), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.GetCount(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handler) GetCount(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	d, err := h.svc.GetCount(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) AddCountItems(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var req AddItemsInput
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	n, err := h.svc.AddCountItems(r.Context(), actor(r), id, req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"added": n})
}

func (h *Handler) SetCounted(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	itemID, ok := idParam(w, r, "itemId")
	if !ok {
		return
	}
	var req struct {
		CountedQty json.Number `json:"counted_qty"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetCounted(r.Context(), actor(r), id, itemID, req.CountedQty.String()); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RemoveCountItem(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	itemID, ok := idParam(w, r, "itemId")
	if !ok {
		return
	}
	if err := h.svc.RemoveCountItem(r.Context(), actor(r), id, itemID); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CompleteCount(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.CompleteCount(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.svc.GetCount(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) CancelCount(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.CancelCount(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SendTransfer: POST /stock/transfers — header `Idempotency-Key` wajib. 201 = dokumen baru; 200 + `Idempotent-Replay: true` = kunci sama.
func (h *Handler) SendTransfer(w http.ResponseWriter, r *http.Request) {
	var req TransferInput
	if !httpx.DecodeJSONLimit(w, r, &req, 64<<10) {
		return
	}
	t, replayed, err := h.svc.SendTransfer(r.Context(), actor(r), r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, t)
}

func (h *Handler) TransferDestinations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Destinations(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// ListTransfers: ?direction=out|in&status=&q=&cursor=&limit=
func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := TransferListParams{Direction: qs.Get("direction"), Status: qs.Get("status"), Q: qs.Get("q"), Cursor: qs.Get("cursor")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	page, err := h.svc.ListTransfers(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	t, err := h.svc.GetTransfer(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// ReceiveTransfer: POST /stock/transfers/{id}/receive — body {lines:[{item_id,qty}]} opsional (kosong = diterima penuh).
func (h *Handler) ReceiveTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Lines []ReceiveLine `json:"lines"`
	}
	if !httpx.DecodeJSONLimit(w, r, &req, 64<<10) {
		return
	}
	if err := h.svc.ReceiveTransfer(r.Context(), actor(r), id, req.Lines); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.GetTransfer(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.CancelTransfer(r.Context(), actor(r), id, req.Reason); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.svc.GetTransfer(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}
