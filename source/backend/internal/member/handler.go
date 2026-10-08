package member

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	"aciraba/internal/item"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// Modul izin: member memakai "members"; level memakai "member_categories" (menu Kategori Member di sidebar).
// Penyesuaian poin manual butuh aksi `approve` pada "members".
const (
	ModuleID      = "members"
	LevelModuleID = "member_categories"
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
	r.Route("/members", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		// Kasir cukup izin membuat nota untuk mencari member.
		r.With(authz.RequireAny([2]string{ModuleID, authz.ActView}, [2]string{"sales_orders", authz.ActCreate})).Get("/lookup", h.Lookup)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(ModuleID, authz.ActCreate)).Post("/", h.Create)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}", h.Get)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}", h.Update)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}/active", h.SetActive)
		// Foto tampil di header kasir, jadi cukup izin yang sama dengan pencarian member di kasir.
		r.With(authz.RequireAny([2]string{ModuleID, authz.ActView}, [2]string{"sales_orders", authz.ActCreate})).Get("/{id}/cover", h.CoverFile)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Post("/{id}/cover", h.UploadCover)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Delete("/{id}/cover", h.DeleteCover)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}/points", h.Points)
		r.With(authz.Require(ModuleID, authz.ActApprove)).Post("/{id}/points/adjust", h.Adjust)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}/sales", h.Sales)
	})
	r.Route("/member-levels", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(LevelModuleID, authz.ActView)).Get("/", h.Levels)
		r.With(authz.Require(LevelModuleID, authz.ActCreate)).Post("/", h.CreateLevel)
		r.With(authz.Require(LevelModuleID, authz.ActUpdate)).Put("/{id}", h.UpdateLevel)
		r.With(authz.Require(LevelModuleID, authz.ActUpdate)).Put("/{id}/active", h.SetLevelActive)
	})
}

const maxBody = 32 << 10

type request struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	District    string `json:"district"`
	City        string `json:"city"`
	Province    string `json:"province"`
	PostalCode  string `json:"postal_code"`
	CreditLimit numStr `json:"credit_limit"`
	DueDays     numStr `json:"due_days"`
	ValidUntil  string `json:"valid_until"`
	Notes       string `json:"notes"`
	Active      *bool  `json:"active"`
}

func (q request) input() Input {
	return Input{Code: q.Code, Name: q.Name, Gender: q.Gender, Phone: q.Phone, Email: q.Email, Address: q.Address, District: q.District,
		City: q.City, Province: q.Province, PostalCode: q.PostalCode, CreditLimit: q.CreditLimit.String(), DueDays: q.DueDays.String(),
		ValidUntil: q.ValidUntil, Notes: q.Notes, Active: q.Active}
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

func pathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	p.Offset, _ = strconv.Atoi(qs.Get("offset"))
	switch qs.Get("active") {
	case "true":
		t := true
		p.Active = &t
	case "false":
		f := false
		p.Active = &f
	}
	list, total, err := h.svc.List(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Lookup(r.Context(), actor(r), r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req request
	if !httpx.DecodeJSONLimit(w, r, &req, maxBody) {
		return
	}
	m, err := h.svc.Create(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req request
	if !httpx.DecodeJSONLimit(w, r, &req, maxBody) {
		return
	}
	m, err := h.svc.Update(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handler) SetActive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Active *bool `json:"active"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	m, err := h.svc.SetActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handler) Points(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	offset, _ := strconv.Atoi(qs.Get("offset"))
	list, total, err := h.svc.Points(r.Context(), actor(r), id, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

func (h *Handler) Sales(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	limit, _ := strconv.Atoi(qs.Get("limit"))
	offset, _ := strconv.Atoi(qs.Get("offset"))
	list, total, err := h.svc.Sales(r.Context(), actor(r), id, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list, "total": total})
}

func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Points numStr `json:"points"`
		Note   string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	n, err := strconv.Atoi(req.Points.String())
	if err != nil {
		httpx.ValidationError(w, map[string]string{"points": "INVALID"})
		return
	}
	m, err := h.svc.Adjust(r.Context(), actor(r), id, n, req.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// ---- level ----

type levelRequest struct {
	Name          string `json:"name"`
	MinPoints     numStr `json:"min_points"`
	SpendPerPoint numStr `json:"spend_per_point"`
	PointValue    numStr `json:"point_value"`
}

func (q levelRequest) input() LevelInput {
	return LevelInput{Name: q.Name, MinPoints: q.MinPoints.String(), SpendPerPoint: q.SpendPerPoint.String(), PointValue: q.PointValue.String()}
}

func (h *Handler) Levels(w http.ResponseWriter, r *http.Request) {
	var active *bool
	switch r.URL.Query().Get("active") {
	case "true":
		t := true
		active = &t
	case "false":
		f := false
		active = &f
	}
	list, err := h.svc.Levels(r.Context(), actor(r), active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *Handler) CreateLevel(w http.ResponseWriter, r *http.Request) {
	var req levelRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	l, err := h.svc.CreateLevel(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, l)
}

func (h *Handler) UpdateLevel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req levelRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	l, err := h.svc.UpdateLevel(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, l)
}

func (h *Handler) SetLevelActive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Active *bool `json:"active"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		httpx.ValidationError(w, map[string]string{"active": "REQUIRED"})
		return
	}
	l, err := h.svc.SetLevelActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, l)
}

// ---- cover ----

const maxUploadBody = item.MaxUploadBytes + 1<<20

func (h *Handler) UploadCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus multipart/form-data.")
		return
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				h.fail(w, r, item.ErrImageTooLarge)
				return
			}
			httpx.ValidationError(w, map[string]string{"file": "REQUIRED"})
			return
		}
		if part.FormName() != "file" {
			continue
		}
		m, err := h.svc.SetCover(r.Context(), actor(r), id, part)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, m)
		return
	}
}

func (h *Handler) DeleteCover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.RemoveCover(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// CoverFile menyajikan foto cover (?size=thumb|full). Id cover berubah tiap ganti foto, jadi aman di-cache lama.
func (h *Handler) CoverFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	f, coverID, err := h.svc.OpenCover(r.Context(), actor(r), id, r.URL.Query().Get("size") == "thumb")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+coverID+`"`)
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode member sudah dipakai.")
	case errors.Is(err, item.ErrImageTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "Ukuran gambar terlalu besar.")
	case errors.Is(err, item.ErrImageUnsupported):
		httpx.Error(w, http.StatusUnsupportedMediaType, "IMAGE_UNSUPPORTED", "Format gambar tidak didukung. Gunakan JPG, PNG, atau WebP.")
	case errors.Is(err, item.ErrImageDimensions):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_DIMENSIONS", "Dimensi gambar terlalu besar.")
	case errors.Is(err, item.ErrImageCorrupt):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_CORRUPT", "Gambar rusak atau tidak dapat dibaca.")
	case errors.Is(err, ErrNoStorage):
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Penyimpanan gambar belum tersedia.")
	default:
		h.log.Error("member gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

// numStr menerima angka JSON, string angka ("12.5"), string kosong, atau null — semuanya menjadi teks apa adanya.
// (json.Number menolak string kosong, padahal form mengirim "" untuk isian yang dikosongkan.) Validasi nilai
// tetap di service.
type numStr string

func (n *numStr) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*n = ""
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*n = numStr(v)
		return nil
	}
	*n = numStr(s)
	return nil
}

func (n numStr) String() string { return string(n) }
