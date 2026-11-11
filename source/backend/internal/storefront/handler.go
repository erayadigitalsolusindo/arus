package storefront

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/storage"
)

// Halaman publik dilayani sebagai HTML statis dari API (bukan SPA, tanpa token/cookie aplikasi) dengan CSP yang melarang
// script sama sekali dan sandbox tanpa allow-same-origin; lihat pageHeaders.
const pageCSP = "default-src 'none'; img-src 'self' https:; style-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; " +
	"frame-ancestors 'none'; sandbox allow-popups allow-popups-to-escape-sandbox"

const pageCacheTTL = 60 * time.Second

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	redis    *redis.Client
	store    *storage.Local
	log      *slog.Logger
	// domainTarget = tujuan CNAME untuk DNS sub-host (CUSTOM_DOMAIN_TARGET); hanya ditampilkan ke tenant.
	domainTarget string
}

// WithDomainTarget mengisi tujuan CNAME yang ditampilkan di daftar host tenant.
func (h *Handler) WithDomainTarget(t string) *Handler { h.domainTarget = t; return h }

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, rdb *redis.Client, store *storage.Local, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, redis: rdb, store: store, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	limit := httpx.RateLimit(h.redis, "public-storefront", 300, time.Minute)
	r.Get("/public/s-assets/tailwind.css", h.tailwind)
	r.With(limit).Get("/public/s/{slug}", h.page)
	r.With(limit).Get("/s/{slug}", h.page)
	r.With(limit).Get("/public/domain-root", h.domainRoot)
	r.With(limit).Get("/public/s/{slug}/style.css", h.style)
	r.With(limit).Get("/public/s/{slug}/img/{id}", h.image)

	r.Route("/storefront", func(r chi.Router) {
		r.Use(httpx.RequireAuth(h.tokens), h.resolver.Authenticate)
		r.With(authz.Require("storefront", authz.ActView)).Get("/", h.get)
		r.With(authz.Require("storefront", authz.ActUpdate)).Put("/", h.save)
		r.With(authz.Require("storefront", authz.ActView)).Post("/preview", h.preview)
		r.With(authz.Require("storefront", authz.ActView)).Get("/versions", h.versions)
		r.With(authz.Require("storefront", authz.ActView)).Get("/versions/{version}", h.version)
		r.With(authz.Require("storefront", authz.ActView)).Get("/templates", h.templates)
		r.With(authz.Require("storefront", authz.ActView)).Get("/templates/{id}", h.template)
		h.domainsRoutes(r)
	})
}

func actor(r *http.Request) authz.Actor {
	a, _ := authz.ActorFrom(r.Context())
	return a
}

// ---- publik ----

func publicHeaders(w http.ResponseWriter, contentType string, maxAge int) {
	hd := w.Header()
	hd.Set("Content-Type", contentType)
	hd.Set("Content-Security-Policy", pageCSP)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("X-Frame-Options", "DENY")
	hd.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	hd.Set("Cross-Origin-Resource-Policy", "cross-origin")
	hd.Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
}

func (h *Handler) tailwind(w http.ResponseWriter, r *http.Request) {
	publicHeaders(w, "text/css; charset=utf-8", 86400)
	w.Header().Set("ETag", `"`+tailwindVersion+`"`)
	if r.Header.Get("If-None-Match") == `"`+tailwindVersion+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(tailwindCSS)
}

func notFoundPage(w http.ResponseWriter) {
	publicHeaders(w, "text/html; charset=utf-8", 30)
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, Document("Halaman tidak ditemukan", `<main style="font-family:system-ui;padding:4rem 1.5rem;text-align:center"><h1>Halaman tidak ditemukan</h1></main>`, tailwindPath+"?v="+tailwindVersion, "", ""))
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	h.servePage(w, r, strings.ToLower(chi.URLParam(r, "slug")))
}

// domainRoot melayani landing page di root domain kustom (reverse proxy meneruskan GET / sebagai ?host=). Host tanpa
// landing -> 404 kosong agar proxy jatuh kembali ke aplikasi (SPA).
func (h *Handler) domainRoot(w http.ResponseWriter, r *http.Request) {
	slug, err := h.svc.DomainLandingSlug(r.Context(), r.URL.Query().Get("host"))
	if errors.Is(err, ErrNotFound) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		h.log.ErrorContext(r.Context(), "storefront: domain root gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan.")
		return
	}
	h.servePage(w, r, slug)
}

func (h *Handler) servePage(w http.ResponseWriter, r *http.Request, slug string) {
	key := "sf:page:" + slug
	if b, err := h.redis.Get(r.Context(), key).Bytes(); err == nil {
		publicHeaders(w, "text/html; charset=utf-8", 60)
		_, _ = w.Write(b)
		return
	}
	p, err := h.svc.Lookup(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		notFoundPage(w)
		return
	}
	if err != nil {
		h.log.ErrorContext(r.Context(), "storefront: lookup gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan.")
		return
	}
	body, err := h.svc.Body(r.Context(), p.TenantID, p.TenantName, slug, p.HTML, "")
	if err != nil {
		h.log.ErrorContext(r.Context(), "storefront: render gagal", "err", err, "slug", slug)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan.")
		return
	}
	title := p.Title
	if title == "" {
		title = p.TenantName
	}
	cssHref := ""
	if strings.TrimSpace(p.CSS) != "" {
		cssHref = "/public/s/" + slug + "/style.css?v=" + strconv.Itoa(p.Version)
	}
	doc := []byte(Document(title, body, tailwindPath+"?v="+tailwindVersion, cssHref, ""))
	_ = h.redis.Set(r.Context(), key, doc, pageCacheTTL).Err() // cache gagal bukan galat
	publicHeaders(w, "text/html; charset=utf-8", 60)
	_, _ = w.Write(doc)
}

func (h *Handler) style(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Lookup(r.Context(), strings.ToLower(chi.URLParam(r, "slug")))
	if err != nil {
		publicHeaders(w, "text/css; charset=utf-8", 30)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	css, err := SanitizeCSS(p.CSS) // disaring lagi saat tampil; yang tersimpan bisa saja dari versi penyaring lama
	if err != nil {
		css = ""
	}
	publicHeaders(w, "text/css; charset=utf-8", 300)
	_, _ = io.WriteString(w, css)
}

func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	slug := strings.ToLower(chi.URLParam(r, "slug"))
	if err != nil || !slugRe.MatchString(slug) {
		http.NotFound(w, r)
		return
	}
	var tenant uuid.UUID
	if err := h.svc.pool.QueryRow(r.Context(), `SELECT storefront_slug_tenant($1)`, slug).Scan(&tenant); err != nil || tenant == uuid.Nil {
		http.NotFound(w, r)
		return
	}
	ok, err := h.svc.ImageOK(r.Context(), tenant, id)
	if err != nil || !ok || h.store == nil {
		http.NotFound(w, r)
		return
	}
	f, err := h.store.Open(r.Context(), tenant.String()+"/"+id.String()+"-t.jpg") // kunci sama dengan item.thumbKey
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	publicHeaders(w, "image/jpeg", 3600)
	http.ServeContent(w, r, "", time.Time{}, f)
}

// ---- editor (terautentikasi) ----

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"config": c, "starter": map[string]string{"html": StarterHTML, "css": StarterCSS}})
}

type saveRequest struct {
	Slug    string `json:"slug"`
	Enabled bool   `json:"enabled"`
	Title   string `json:"title"`
	HTML    string `json:"html"`
	CSS     string `json:"css"`
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 200_000) {
		return
	}
	a := actor(r)
	old, _ := h.svc.Get(r.Context(), a)
	c, issues, err := h.svc.Save(r.Context(), a, SaveInput{Slug: strings.ToLower(strings.TrimSpace(req.Slug)), Enabled: req.Enabled, Title: strings.TrimSpace(req.Title), HTML: req.HTML, CSS: req.CSS})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	keys := []string{"sf:page:" + c.Slug}
	if old != nil && old.Slug != c.Slug {
		keys = append(keys, "sf:page:"+old.Slug)
	}
	_ = h.redis.Del(r.Context(), keys...).Err()
	if issues == nil {
		issues = []string{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"config": c, "warnings": issues})
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 200_000) {
		return
	}
	in, f := validate(SaveInput{Slug: "pratinjau", HTML: req.HTML, CSS: req.CSS})
	delete(f, "slug")
	if len(f) > 0 {
		h.fail(w, r, f)
		return
	}
	a := actor(r)
	var shop, slug string
	err := db.WithTenant(r.Context(), h.svc.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&shop); err != nil {
			return err
		}
		err := tx.QueryRow(r.Context(), `SELECT slug FROM storefronts WHERE tenant_id = $1`, a.TenantID).Scan(&slug)
		if errors.Is(err, pgx.ErrNoRows) {
			slug = ""
			return nil
		}
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if slug == "" {
		slug = strings.ToLower(strings.TrimSpace(req.Slug)) // belum pernah disimpan: gambar baru tampil setelah disimpan
	}
	base := assetBase(r)
	body, err := h.svc.Body(r.Context(), a.TenantID, shop, slug, in.HTML, base)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = shop
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"html": Document(title, body, base+tailwindPath+"?v="+tailwindVersion, "", in.CSS)})
}

func assetBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *Handler) versions(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Versions(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": v})
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Versi tidak ditemukan.")
		return
	}
	c, err := h.svc.GetVersion(r.Context(), actor(r), n)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListTemplates(r.Context(), true, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *Handler) template(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Template tidak ditemukan.")
		return
	}
	t, err := h.svc.GetTemplate(r.Context(), id, true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var fe FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case errors.Is(err, ErrSlugTaken):
		httpx.Error(w, http.StatusConflict, "SLUG_TAKEN", "Alamat halaman sudah dipakai toko lain.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tidak ditemukan.")
	case errors.Is(err, context.Canceled):
		return
	default:
		h.log.ErrorContext(r.Context(), "storefront: gagal", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan.")
	}
}
