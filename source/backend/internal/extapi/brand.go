package extapi

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
	domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	accentRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	moduleRe = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
)

// Brand = profil white label (sisi Platform Admin: lengkap).
type Brand struct {
	ID            uuid.UUID `json:"id"`
	Slug          string    `json:"slug"`
	Domain        *string   `json:"domain"`
	IsDefault     bool      `json:"is_default"`
	Active        bool      `json:"active"`
	Name          string    `json:"name"`
	Tagline       string    `json:"tagline"`
	LogoURL       string    `json:"logo_url"`
	Accent        string    `json:"accent"`
	SupportEmail  string    `json:"support_email"`
	SupportURL    string    `json:"support_url"`
	Footer        string    `json:"footer"`
	BaseURL       string    `json:"base_url"`
	HiddenModules []string  `json:"hidden_modules"`
	SandboxNote   string    `json:"sandbox_note"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PublicBrand = bagian profil yang boleh dilihat publik (tanpa id/domain internal).
type PublicBrand struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Tagline       string   `json:"tagline"`
	LogoURL       string   `json:"logo_url"`
	Accent        string   `json:"accent"`
	SupportEmail  string   `json:"support_email"`
	SupportURL    string   `json:"support_url"`
	Footer        string   `json:"footer"`
	BaseURL       string   `json:"base_url"`
	HiddenModules []string `json:"hidden_modules"`
	SandboxNote   string   `json:"sandbox_note"`
}

// BrandInput = isian simpan (ID nil = buat baru).
type BrandInput struct {
	ID            *uuid.UUID
	Slug          string
	Domain        string
	IsDefault     bool
	Active        bool
	Name          string
	Tagline       string
	LogoURL       string
	Accent        string
	SupportEmail  string
	SupportURL    string
	Footer        string
	BaseURL       string
	HiddenModules []string
	SandboxNote   string
}

func httpURLOrEmpty(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validLogo(s string) bool {
	return s == "" || strings.HasPrefix(s, "data:image/") || httpURLOrEmpty(s)
}

// Validate mengembalikan galat per field (kode, bukan teks) dan menormalkan isian.
func (in *BrandInput) Validate() map[string]string {
	f := map[string]string{}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))
	in.Name = strings.TrimSpace(in.Name)
	in.Accent = strings.TrimSpace(in.Accent)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.LogoURL = strings.TrimSpace(in.LogoURL)
	if !slugRe.MatchString(in.Slug) {
		f["slug"] = "INVALID"
	}
	if in.Domain != "" && !domainRe.MatchString(in.Domain) {
		f["domain"] = "INVALID"
	}
	if l := len([]rune(in.Name)); l < 1 || l > 80 {
		f["name"] = "INVALID"
	}
	if !accentRe.MatchString(in.Accent) {
		f["accent"] = "INVALID"
	}
	if len([]rune(in.Tagline)) > 200 {
		f["tagline"] = "TOO_LONG"
	}
	if len([]rune(in.Footer)) > 500 {
		f["footer"] = "TOO_LONG"
	}
	if len([]rune(in.SandboxNote)) > 1000 {
		f["sandbox_note"] = "TOO_LONG"
	}
	if len(in.LogoURL) > 300000 || !validLogo(in.LogoURL) {
		f["logo_url"] = "INVALID"
	}
	if !httpURLOrEmpty(in.BaseURL) {
		f["base_url"] = "INVALID"
	}
	if !httpURLOrEmpty(in.SupportURL) {
		f["support_url"] = "INVALID"
	}
	if in.SupportEmail != "" && (len(in.SupportEmail) > 200 || !strings.Contains(in.SupportEmail, "@")) {
		f["support_email"] = "INVALID"
	}
	if len(in.HiddenModules) > maxHidden {
		f["hidden_modules"] = "INVALID"
	}
	for i, m := range in.HiddenModules {
		in.HiddenModules[i] = strings.ToLower(strings.TrimSpace(m))
		if !moduleRe.MatchString(in.HiddenModules[i]) {
			f["hidden_modules"] = "INVALID"
		}
	}
	if in.HiddenModules == nil {
		in.HiddenModules = []string{}
	}
	return f
}

func (s *Store) ListBrands(ctx context.Context, admin uuid.UUID) ([]Brand, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, slug, domain, is_default, active, name, tagline, logo_url, accent, support_email,
		support_url, footer, base_url, hidden_modules, sandbox_note, created_at, updated_at FROM platform_ext_brand_list($1)`, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Brand{}
	for rows.Next() {
		var b Brand
		if err := rows.Scan(&b.ID, &b.Slug, &b.Domain, &b.IsDefault, &b.Active, &b.Name, &b.Tagline, &b.LogoURL, &b.Accent, &b.SupportEmail,
			&b.SupportURL, &b.Footer, &b.BaseURL, &b.HiddenModules, &b.SandboxNote, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SaveBrand membuat/mengubah profil. in harus sudah lolos Validate.
func (s *Store) SaveBrand(ctx context.Context, admin uuid.UUID, in BrandInput) (uuid.UUID, error) {
	var domain *string
	if in.Domain != "" {
		domain = &in.Domain
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT platform_ext_brand_save($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		admin, in.ID, in.Slug, domain, in.IsDefault, in.Active, in.Name, in.Tagline, in.LogoURL, in.Accent, in.SupportEmail,
		in.SupportURL, in.Footer, in.BaseURL, in.HiddenModules, in.SandboxNote).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return uuid.Nil, ErrTaken
	}
	return id, err
}

func (s *Store) DeleteBrand(ctx context.Context, admin, id uuid.UUID) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT platform_ext_brand_delete($1,$2)`, admin, id).Scan(&ok)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "P0001" {
		return ErrDefault
	}
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// ResolveBrand = profil untuk host (domain khusus) atau profil bawaan. Dipakai portal publik.
func (s *Store) ResolveBrand(ctx context.Context, host string) (*PublicBrand, error) {
	var b PublicBrand
	err := s.pool.QueryRow(ctx, `SELECT slug, name, tagline, logo_url, accent, support_email, support_url, footer, base_url, hidden_modules, sandbox_note
		FROM ext_brand_resolve($1)`, strings.ToLower(host)).
		Scan(&b.Slug, &b.Name, &b.Tagline, &b.LogoURL, &b.Accent, &b.SupportEmail, &b.SupportURL, &b.Footer, &b.BaseURL, &b.HiddenModules, &b.SandboxNote)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}
