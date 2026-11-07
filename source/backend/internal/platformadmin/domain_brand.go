package platformadmin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/httpx"
)

// White label per domain kustom: nama aplikasi, footer, logo penuh, ikon. Diatur admin platform; ditampilkan web di host itu.
const (
	ActionDomainBrand = "platform.domain_brand"

	maxBrandImage = 512 << 10
)

var (
	ErrBrandInvalid = errors.New("tampilan white label tidak valid")
	ErrBrandImage   = errors.New("gambar tidak valid")
)

// PublicBrand = tampilan yang dibaca web di host domain kustom. URL gambar memuat versi agar cache ikut berganti.
type PublicBrand struct {
	Name    string `json:"name"`
	Footer  string `json:"footer"`
	LogoURL string `json:"logo_url"`
	IconURL string `json:"icon_url"`
}

func cleanBrandText(raw string, max int) (string, bool) {
	v := strings.TrimSpace(raw)
	if len([]rune(v)) > max {
		return "", false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return v, true
}

// SetDomainBrand mengubah nama dan/atau footer (nil = tidak diubah).
func (s *Service) SetDomainBrand(ctx context.Context, a Actor, id uuid.UUID, name, footer *string) error {
	d, err := s.getDomain(ctx, a, id)
	if err != nil {
		return err
	}
	n, f := d.BrandName, d.BrandFooter
	var ok bool
	if name != nil {
		if n, ok = cleanBrandText(*name, 60); !ok {
			return ErrBrandInvalid
		}
	}
	if footer != nil {
		if f, ok = cleanBrandText(*footer, 200); !ok {
			return ErrBrandInvalid
		}
	}
	var found bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_domain_set_brand($1, $2, $3, $4)`, a.ID, id, n, f).Scan(&found); err != nil {
		if pgCode(err) == "42501" {
			return ErrInvalidSession
		}
		return err
	}
	if !found {
		return ErrNotFound
	}
	s.record(ctx, a, ActionDomainBrand, d.TenantID, d.TenantName, map[string]any{"host": d.Host, "brand_name": n, "brand_footer": f})
	return nil
}

// imageType mengenali PNG/JPEG/WebP/GIF dari isinya, dan SVG tanpa skrip. "" = bukan gambar yang diizinkan.
func imageType(b []byte) string {
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return ct
	}
	head := strings.ToLower(string(b[:min(len(b), 2048)]))
	if !strings.Contains(head, "<svg") {
		return ""
	}
	low := strings.ToLower(string(b))
	for _, bad := range []string{"<script", "javascript:", "<foreignobject", "<iframe", "<!entity", " onload=", " onerror=", " onclick=", " onmouseover="} {
		if strings.Contains(low, bad) {
			return ""
		}
	}
	return "image/svg+xml"
}

// SetDomainAsset menyimpan (data != nil) atau menghapus (nil) logo/ikon domain.
func (s *Service) SetDomainAsset(ctx context.Context, a Actor, id uuid.UUID, kind string, data []byte) error {
	if kind != "logo" && kind != "icon" {
		return ErrNotFound
	}
	d, err := s.getDomain(ctx, a, id)
	if err != nil {
		return err
	}
	ct := ""
	if data != nil {
		if ct = imageType(data); ct == "" {
			return ErrBrandImage
		}
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_domain_set_asset($1, $2, $3, $4, $5)`, a.ID, id, kind, ct, data).Scan(&ok); err != nil {
		if pgCode(err) == "42501" {
			return ErrInvalidSession
		}
		return err
	}
	if !ok {
		return ErrNotFound
	}
	s.record(ctx, a, ActionDomainBrand, d.TenantID, d.TenantName, map[string]any{"host": d.Host, "asset": kind, "removed": data == nil, "bytes": len(data)})
	return nil
}

// publicBrand: tampilan white label host aktif; nil bila host tidak punya pengaturan sama sekali.
func (s *Service) publicBrand(ctx context.Context, host string) (*PublicBrand, error) {
	var (
		name, footer     string
		ver              int64
		hasLogo, hasIcon bool
	)
	err := s.Pool.QueryRow(ctx, `SELECT brand_name, brand_footer, brand_ver, has_logo, has_icon FROM public_domain_brand($1)`, host).
		Scan(&name, &footer, &ver, &hasLogo, &hasIcon)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if name == "" && footer == "" && !hasLogo && !hasIcon {
		return nil, nil
	}
	b := &PublicBrand{Name: name, Footer: footer}
	q := "&host=" + host + "&v=" + strconv.FormatInt(ver, 10)
	if hasLogo {
		b.LogoURL = "/public/brand-asset?kind=logo" + q
	}
	if hasIcon {
		b.IconURL = "/public/brand-asset?kind=icon" + q
	}
	return b, nil
}

// ---- HTTP ----

// PutDomainAsset: PUT /platform/domains/{id}/brand/{kind} — badan = berkas gambar mentah (maks 512 KB).
func (h *Handler) PutDomainAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBrandImage))
	if err != nil || len(data) == 0 {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "Gambar maksimal 512 KB.")
		return
	}
	h.finishAsset(w, r, h.svc.SetDomainAsset(r.Context(), actor(r), id, chi.URLParam(r, "kind"), data))
}

func (h *Handler) DeleteDomainAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.finishAsset(w, r, h.svc.SetDomainAsset(r.Context(), actor(r), id, chi.URLParam(r, "kind"), nil))
}

func (h *Handler) finishAsset(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrBrandImage):
		httpx.Error(w, http.StatusUnprocessableEntity, "INVALID_IMAGE", "Gunakan PNG, JPEG, WebP, GIF, atau SVG (tanpa skrip), maks 512 KB.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Domain tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "simpan gambar domain gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// BrandAsset: GET /public/brand-asset?host=&kind=&v= — logo/ikon domain aktif, tanpa login.
func (h *Handler) BrandAsset(w http.ResponseWriter, r *http.Request) {
	host, err := normalizeHost(r.URL.Query().Get("host"))
	kind := r.URL.Query().Get("kind")
	if err != nil || (kind != "logo" && kind != "icon") {
		http.NotFound(w, r)
		return
	}
	var (
		ct   string
		data []byte
		at   time.Time
	)
	err = h.svc.Pool.QueryRow(r.Context(), `SELECT content_type, data, updated_at FROM public_domain_asset($1, $2)`, host, kind).Scan(&ct, &data, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.internal(w, r, "ambil gambar domain gagal", err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", ct)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	hd.Set("Cross-Origin-Resource-Policy", "cross-origin")
	hd.Set("Cache-Control", "public, max-age=3600") // URL memuat versi (?v=), jadi berganti saat gambar diubah
	http.ServeContent(w, r, "", at, bytes.NewReader(data))
}
