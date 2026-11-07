package storefront

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
)

// tailwind.css = stylesheet Tailwind v4 hasil build (tools/storefront-css di web): kumpulan utility umum yang sudah
// dihasilkan, karena server Go tidak mengompilasi Tailwind. Class di luar himpunan itu tidak berefek; CSS tambahan
// penulis ada di kolom css.
//
//go:embed tailwind.css
var tailwindCSS []byte

var tailwindVersion = func() string {
	h := sha256.Sum256(tailwindCSS)
	return hex.EncodeToString(h[:4])
}()

const tailwindPath = "/public/s-assets/tailwind.css"

// Body merender templat menjadi potongan HTML yang SUDAH disaring (tanpa script, URL aman). imageBase mengawali alamat
// gambar ("" = path relatif; pratinjau memakai alamat absolut karena dibuka dari halaman lain).
func (s *Service) Body(ctx context.Context, tenant uuid.UUID, shopName, slug, tmpl, imageBase string) (string, error) {
	site := Site{
		Name: shopName, Slug: slug, Now: time.Now(),
		ImageURL: func(id uuid.UUID) string { return imageBase + "/public/s/" + slug + "/img/" + id.String() },
	}
	raw, err := Render(ctx, tmpl, s.source(tenant), site)
	if err != nil {
		return "", err
	}
	out, _ := SanitizeHTML(raw, false)
	return out, nil
}

// Document membungkus isi menjadi halaman utuh. cssHref = stylesheet CSS tenant (halaman publik); inlineCSS dipakai
// pratinjau (srcdoc) dan SUDAH lolos SanitizeCSS. Tidak ada <script> di mana pun.
func Document(title, body, tailwindHref, cssHref, inlineCSS string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<meta name="referrer" content="strict-origin-when-cross-origin"><title>` + html.EscapeString(title) + `</title>`)
	b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(tailwindHref) + `">`)
	if cssHref != "" {
		b.WriteString(`<link rel="stylesheet" href="` + html.EscapeString(cssHref) + `">`)
	}
	if inlineCSS != "" {
		// SanitizeCSS menolak "<", jadi isi tak bisa menutup elemen style.
		b.WriteString(`<style>` + inlineCSS + `</style>`)
	}
	b.WriteString(`</head><body>` + body + `</body></html>`)
	return b.String()
}
