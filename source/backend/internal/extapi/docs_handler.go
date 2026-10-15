package extapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/platform/httpx"
)

// DocsHandler = endpoint publik portal dokumentasi (tanpa login): profil white label + katalog endpoint.
type DocsHandler struct {
	store   *Store
	catalog *CatalogBuilder
	rdb     *redis.Client
	log     *slog.Logger
}

func NewDocsHandler(store *Store, catalog *CatalogBuilder, rdb *redis.Client, log *slog.Logger) *DocsHandler {
	return &DocsHandler{store: store, catalog: catalog, rdb: rdb, log: log}
}

func (h *DocsHandler) Routes(r chi.Router) {
	rl := httpx.RateLimit(h.rdb, "public-api-docs", 600, time.Hour)
	r.With(rl).Get("/public/api-docs/config", h.Config)
	r.With(rl).Get("/public/api-docs/catalog", h.CatalogJSON)
}

// SpecInfo = ringkasan skema tanda tangan agar portal tidak menulis ulang konstanta.
type SpecInfo struct {
	Algorithm      string            `json:"algorithm"`
	Prefix         string            `json:"prefix"`
	Headers        map[string]string `json:"headers"`
	SkewSeconds    int               `json:"skew_seconds"`
	MaxBodyBytes   int               `json:"max_body_bytes"`
	NonceHint      string            `json:"nonce_hint"`
	BlockedPrefix  []string          `json:"blocked_prefixes"`
	DefaultRatePer int               `json:"default_rate_per_min"`
}

type configResponse struct {
	Brand *PublicBrand `json:"brand"`
	Spec  SpecInfo     `json:"spec"`
}

func hostParam(r *http.Request) string {
	h := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("host")))
	if h == "" || len(h) > 253 || !domainRe.MatchString(h) {
		return ""
	}
	return h
}

func (h *DocsHandler) brand(w http.ResponseWriter, r *http.Request) *PublicBrand {
	b, err := h.store.ResolveBrand(r.Context(), hostParam(r))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Portal dokumentasi tidak tersedia.")
		return nil
	}
	if err != nil {
		h.log.Error("resolve brand gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return nil
	}
	return b
}

func (h *DocsHandler) Config(w http.ResponseWriter, r *http.Request) {
	b := h.brand(w, r)
	if b == nil {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, configResponse{Brand: b, Spec: SpecInfo{
		Algorithm: Algorithm, Prefix: Prefix, SkewSeconds: int(maxSkew.Seconds()), MaxBodyBytes: maxBodyBytes,
		Headers:       map[string]string{"key": HeaderKey, "timestamp": HeaderTimestamp, "nonce": HeaderNonce, "signature": HeaderSignature},
		NonceHint:     "16–64 karakter [A-Za-z0-9_-], unik per request",
		BlockedPrefix: blockedPrefixes[:len(blockedPrefixes)-1], DefaultRatePer: 120,
	}})
}

func (h *DocsHandler) CatalogJSON(w http.ResponseWriter, r *http.Request) {
	b := h.brand(w, r)
	if b == nil {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, h.catalog.Get().Filter(b.HiddenModules))
}
