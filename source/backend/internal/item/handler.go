package item

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// ModuleID = modul izin Daftar Item (authz.Modules).
const ModuleID = "items"

// maxBody: keterangan markdown sampai 5.000 karakter (hingga ~20 KB dalam UTF-8) + harga per cabang.
const maxBody = 64 << 10

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
	r.Route("/items", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/", h.List)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/by-barcode", h.ByBarcode)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/search", h.Search)
		r.With(authz.Require(PriceReportModuleID, authz.ActView)).Get("/price-history", h.PriceReport)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}", h.Get)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}/price-history", h.PriceHistory)
		r.With(authz.Require(ModuleID, authz.ActCreate)).Post("/", h.Create)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}", h.Update)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}/active", h.SetActive)
		r.With(authz.Require(ModuleID, authz.ActView)).Get("/{id}/images/{imageId}/file", h.ImageFile)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Post("/{id}/images", h.UploadImage)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Put("/{id}/images/{imageId}/main", h.SetMainImage)
		r.With(authz.Require(ModuleID, authz.ActUpdate)).Delete("/{id}/images/{imageId}", h.DeleteImage)
	})
}

type outletPriceRequest struct {
	OutletID  string      `json:"outlet_id"`
	SellPrice json.Number `json:"sell_price"`
}

type tierRequest struct {
	MinQty json.Number `json:"min_qty"`
	Price  json.Number `json:"price"`
}

type outletTiersRequest struct {
	OutletID string        `json:"outlet_id"`
	Tiers    []tierRequest `json:"tiers"`
}

type wholesaleRequest struct {
	Default []tierRequest        `json:"default"`
	Outlets []outletTiersRequest `json:"outlets"`
}

type unitRequest struct {
	UnitID    string      `json:"unit_id"`
	Factor    json.Number `json:"factor"`
	Barcode   string      `json:"barcode"`
	SellPrice json.Number `json:"sell_price"`
}

func tiersInput(in []tierRequest) []TierInput {
	out := make([]TierInput, 0, len(in))
	for _, t := range in {
		out = append(out, TierInput{MinQty: t.MinQty.String(), Price: t.Price.String()})
	}
	return out
}

// request: angka sebagai json.Number agar presisi desimal terjaga (tidak lewat float64); id master "" = tidak diisi.
type request struct {
	SKU                string                `json:"sku"`
	Barcode            string                `json:"barcode"`
	Origin             string                `json:"origin"`
	Name               string                `json:"name"`
	WeightGrams        json.Number           `json:"weight_grams"`
	MinStock           json.Number           `json:"min_stock"`
	Cost               json.Number           `json:"cost"`
	SellPrice          json.Number           `json:"sell_price"`
	UnitID             string                `json:"unit_id"`
	CategoryID         string                `json:"category_id"`
	BrandID            string                `json:"brand_id"`
	PrincipalID        string                `json:"principal_id"`
	SupplierID         string                `json:"supplier_id"`
	Kind               string                `json:"kind"`
	AllowNegativeStock bool                  `json:"allow_negative_stock"`
	SellBelowCost      bool                  `json:"sell_below_cost"`
	Description        string                `json:"description"`
	OutletPrices       *[]outletPriceRequest `json:"outlet_prices"`
	Wholesale          *wholesaleRequest     `json:"wholesale"`
	Units              *[]unitRequest        `json:"units"`
}

func (q request) input() Input {
	in := Input{
		SKU: q.SKU, Barcode: q.Barcode, Origin: q.Origin, Name: q.Name, Weight: q.WeightGrams.String(), MinStock: q.MinStock.String(), Cost: q.Cost.String(), SellPrice: q.SellPrice.String(),
		UnitID: q.UnitID, CategoryID: q.CategoryID, BrandID: q.BrandID, PrincipalID: q.PrincipalID, SupplierID: q.SupplierID,
		Kind: q.Kind, AllowNegativeStock: q.AllowNegativeStock, SellBelowCost: q.SellBelowCost, Description: q.Description,
	}
	if q.OutletPrices != nil {
		list := make([]OutletPriceInput, 0, len(*q.OutletPrices))
		for _, p := range *q.OutletPrices {
			list = append(list, OutletPriceInput{OutletID: p.OutletID, SellPrice: p.SellPrice.String()})
		}
		in.OutletPrices = &list
	}
	if q.Wholesale != nil {
		w := &WholesaleInput{Default: tiersInput(q.Wholesale.Default)}
		for _, o := range q.Wholesale.Outlets {
			w.Outlets = append(w.Outlets, OutletTiersInput{OutletID: o.OutletID, Tiers: tiersInput(o.Tiers)})
		}
		in.Wholesale = w
	}
	if q.Units != nil {
		units := make([]UnitInput, 0, len(*q.Units))
		for _, u := range *q.Units {
			units = append(units, UnitInput{UnitID: u.UnitID, Factor: u.Factor.String(), Barcode: u.Barcode, SellPrice: u.SellPrice.String()})
		}
		in.Units = &units
	}
	return in
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

// List: ?q=&active=true|false&category_id=&outlet=all&limit=&offset= (active kosong = semua).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := ListParams{Q: qs.Get("q"), CategoryID: qs.Get("category_id"), AllOutlets: qs.Get("outlet") == "all"}
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

// Search: GET /items/search?q=&cursor=&limit= → {data, exact, next_cursor}. Barang aktif, tanpa total (lihat search.go).
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	p := SearchParams{Q: qs.Get("q"), Cursor: qs.Get("cursor"), CategoryID: qs.Get("category_id")}
	p.Limit, _ = strconv.Atoi(qs.Get("limit"))
	page, err := h.svc.Search(r.Context(), actor(r), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := h.svc.Get(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req request
	if !httpx.DecodeJSONLimit(w, r, &req, maxBody) {
		return
	}
	it, err := h.svc.Create(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, it)
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
	it, err := h.svc.Update(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
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
	it, err := h.svc.SetActive(r.Context(), actor(r), id, *req.Active)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fields FieldErrors
	switch {
	case errors.As(err, &fields):
		httpx.ValidationError(w, fields)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan.")
	case errors.Is(err, ErrCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode barang sudah dipakai.")
	case errors.Is(err, ErrImageNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Gambar tidak ditemukan.")
	case errors.Is(err, ErrImageLimit):
		httpx.Error(w, http.StatusConflict, "IMAGE_LIMIT", "Jumlah gambar item sudah maksimal.")
	case errors.Is(err, ErrImageTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "IMAGE_TOO_LARGE", "Ukuran gambar terlalu besar.")
	case errors.Is(err, ErrImageUnsupported):
		httpx.Error(w, http.StatusUnsupportedMediaType, "IMAGE_UNSUPPORTED", "Format gambar tidak didukung. Gunakan JPG, PNG, atau WebP.")
	case errors.Is(err, ErrImageDimensions):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_DIMENSIONS", "Dimensi gambar terlalu besar.")
	case errors.Is(err, ErrImageCorrupt):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMAGE_CORRUPT", "Gambar rusak atau tidak dapat dibaca.")
	case errors.Is(err, ErrNoStorage):
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Penyimpanan gambar belum tersedia.")
	case errors.Is(err, ErrOutletForbidden):
		httpx.Error(w, http.StatusForbidden, "OUTLET_FORBIDDEN", "Anda tidak memiliki akses ke outlet yang dipilih.")
	default:
		h.log.Error("item gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	}
}

// ---- gambar ----

// maxUploadBody = batas body multipart: ukuran gambar + sedikit ruang untuk header bagian.
const maxUploadBody = MaxUploadBytes + 1<<20

func imageID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "imageId"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return uuid.Nil, false
	}
	return id, true
}

// UploadImage: multipart/form-data dengan satu bagian "file". Dibaca streaming (tidak ada ParseMultipartForm yang
// menampung seluruh body di memori/disk sementara) dan dibatasi MaxBytesReader.
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Server punya ReadTimeout pendek (30 dtk) untuk API JSON; unggahan dari jaringan seluler butuh lebih lama.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus multipart/form-data.")
		return
	}
	for {
		part, err := mr.NextPart()
		if err != nil { // io.EOF = tidak ada bagian "file"; selain itu body rusak/terlalu besar
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				h.fail(w, r, ErrImageTooLarge)
				return
			}
			httpx.ValidationError(w, map[string]string{"file": "REQUIRED"})
			return
		}
		if part.FormName() != "file" {
			continue
		}
		img, err := h.svc.AddImage(r.Context(), actor(r), id, part)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, img)
		return
	}
}

func (h *Handler) SetMainImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	iid, ok := imageID(w, r)
	if !ok {
		return
	}
	if err := h.svc.SetMainImage(r.Context(), actor(r), id, iid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	iid, ok := imageID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteImage(r.Context(), actor(r), id, iid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ImageFile menyajikan berkas gambar (?size=thumb|full, bawaan full). Isi gambar tidak pernah berubah untuk id yang
// sama, jadi boleh di-cache lama oleh browser (private: hanya cache pengguna).
func (h *Handler) ImageFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	iid, ok := imageID(w, r)
	if !ok {
		return
	}
	f, created, err := h.svc.OpenImage(r.Context(), actor(r), id, iid, r.URL.Query().Get("size") == "thumb")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", created, f)
}

// ByBarcode: GET /items/by-barcode?code=…&exclude_id=… → semua barang yang memakai barcode itu (boleh lebih dari satu).
func (h *Handler) ByBarcode(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	exclude, err := uuid.Parse(qs.Get("exclude_id"))
	if err != nil {
		exclude = uuid.Nil // kosong/tidak valid = tidak ada yang dikecualikan
	}
	list, err := h.svc.ByBarcode(r.Context(), actor(r), qs.Get("code"), exclude)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}
