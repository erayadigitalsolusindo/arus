package storefront

import (
	"context"
	_ "embed"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// Pustaka template milik platform: Platform Admin menulis template, tenant memilih lalu isinya DISALIN ke editor tenant
// (tidak ditautkan, jadi mengubah template tidak mengubah halaman tenant yang sudah memakainya).

var (
	ErrTemplateCodeTaken = errors.New("kode template sudah dipakai")
	ErrInvalidAdmin      = errors.New("sesi admin tidak valid")

	templateCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,39}$`)
)

//go:embed templates/kopi-ekspor.html
var kopiEksporHTML string

//go:embed templates/kopi-ekspor.css
var kopiEksporCSS string

//go:embed templates/tech-ai.html
var techAIHTML string

//go:embed templates/tech-ai.css
var techAICSS string

type builtin struct {
	Code, Name, Description, Category, HTML, CSS string
	Sort                                         int
}

// builtins = template bawaan. Ditambahkan ke DB bila kodenya belum ada; suntingan Platform Admin tidak ditimpa.
var builtins = []builtin{
	{
		Code: "kopi-ekspor", Name: "Kopi Ekspor", Category: "Pangan & Ekspor", Sort: 10,
		Description: "Landing page eksportir kopi: hero beranimasi, asal kopi, proses, katalog dari barang Anda, syarat ekspor, FAQ, dan kontak. Animasi murni CSS.",
		HTML:        kopiEksporHTML, CSS: kopiEksporCSS,
	},
	{
		Code: "tech-ai", Name: "Nusa Tech AI", Category: "Teknologi & AI", Sort: 15,
		Description: "Landing page perusahaan IT/AI bertema terminal neon: grid bergerak, terminal mengetik, jaringan saraf, katalog dari barang Anda, plus bagian \"Bedah CSS\" yang menjelaskan tiap animasi untuk belajar. Murni CSS.",
		HTML:        techAIHTML, CSS: techAICSS,
	},
}

type Template struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	HTML        string    `json:"html,omitempty"`
	CSS         string    `json:"css,omitempty"`
	Active      bool      `json:"active"`
	SortOrder   int       `json:"sort_order"`
	Bytes       int       `json:"bytes"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SeedTemplates menambahkan template bawaan yang belum ada. Aman dipanggil tiap start API.
func (s *Service) SeedTemplates(ctx context.Context) error {
	for _, b := range builtins {
		if _, err := s.pool.Exec(ctx, `SELECT storefront_template_seed($1, $2, $3, $4, $5, $6, $7)`, b.Code, b.Name, b.Description, b.Category, b.HTML, b.CSS, int32(b.Sort)); err != nil {
			return err
		}
	}
	return nil
}

// ListTemplates: onlyActive untuk tenant; withBody=false menghemat transfer pada daftar (isi diambil per template).
func (s *Service) ListTemplates(ctx context.Context, onlyActive, withBody bool) ([]Template, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name, description, category, CASE WHEN $2 THEN html ELSE '' END, CASE WHEN $2 THEN css ELSE '' END,
			active, sort_order, octet_length(html) + octet_length(css), updated_at
		  FROM storefront_templates WHERE (NOT $1 OR active) ORDER BY sort_order, name LIMIT 200`, onlyActive, withBody)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.Category, &t.HTML, &t.CSS, &t.Active, &t.SortOrder, &t.Bytes, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) GetTemplate(ctx context.Context, id uuid.UUID, onlyActive bool) (*Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx, `SELECT id, code, name, description, category, html, css, active, sort_order, octet_length(html) + octet_length(css), updated_at
		FROM storefront_templates WHERE id = $1 AND (NOT $2 OR active)`, id, onlyActive).
		Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.Category, &t.HTML, &t.CSS, &t.Active, &t.SortOrder, &t.Bytes, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

type TemplateInput struct {
	ID                                           *uuid.UUID
	Code, Name, Description, Category, HTML, CSS string
	Active                                       bool
	Sort                                         int
}

func (in TemplateInput) validate() (TemplateInput, FieldErrors) {
	f := FieldErrors{}
	if in.ID == nil && !templateCodeRe.MatchString(in.Code) {
		f["code"] = "INVALID"
	}
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 60 {
		f["name"] = "INVALID"
	}
	if utf8.RuneCountInString(in.Description) > 300 {
		f["description"] = "TOO_LONG"
	}
	if utf8.RuneCountInString(in.Category) > 40 {
		f["category"] = "TOO_LONG"
	}
	if in.Sort < -1000 || in.Sort > 100000 {
		f["sort_order"] = "RANGE"
	}
	if len(in.HTML) == 0 || len(in.HTML) > 60000 {
		f["html"] = "INVALID"
	} else if _, err := parse(in.HTML); err != nil {
		f["html"] = "TOO_COMPLEX"
	}
	if css, err := SanitizeCSS(in.CSS); err != nil {
		f["css"] = "FORBIDDEN"
	} else {
		in.CSS = css
	}
	return in, f
}

// SaveTemplate menyimpan template (admin platform). Mengembalikan peringatan = bagian HTML yang akan dibuang penyaring.
func (s *Service) SaveTemplate(ctx context.Context, admin uuid.UUID, in TemplateInput) (uuid.UUID, []string, error) {
	in, f := in.validate()
	if len(f) > 0 {
		return uuid.Nil, nil, f
	}
	_, issues := SanitizeHTML(in.HTML, true)
	var id any
	if in.ID != nil {
		id = *in.ID
	}
	var out *uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT platform_storefront_template_save($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		admin, id, in.Code, in.Name, in.Description, in.Category, in.HTML, in.CSS, in.Active, int32(in.Sort)).Scan(&out)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		return uuid.Nil, nil, ErrTemplateCodeTaken
	case errors.As(err, &pgErr) && pgErr.Code == "42501":
		return uuid.Nil, nil, ErrInvalidAdmin
	case err != nil:
		return uuid.Nil, nil, err
	case out == nil:
		return uuid.Nil, nil, ErrNotFound
	}
	return *out, issues, nil
}

func (s *Service) DeleteTemplate(ctx context.Context, admin, id uuid.UUID) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT platform_storefront_template_delete($1, $2)`, admin, id).Scan(&ok)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "42501":
		return ErrInvalidAdmin
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	return nil
}

// ---- pratinjau dengan data contoh (tanpa tenant) ----

type sampleSource struct{}

var sampleCats = []Category{{ID: uuid.MustParse("00000000-0000-0000-0000-0000000000a1"), Name: "Green Bean"}, {ID: uuid.MustParse("00000000-0000-0000-0000-0000000000a2"), Name: "Roasted Bean"}}

var sampleItems = []Item{
	{SKU: "GB-001", Name: "Gayo Arabica Grade 1", Price: decimal.NewFromInt(135000), Category: "Green Bean", Description: "Body tebal, cokelat gelap."},
	{SKU: "GB-002", Name: "Toraja Kalosi", Price: decimal.NewFromInt(148000), Category: "Green Bean", Description: "Kompleks, buah gelap."},
	{SKU: "GB-003", Name: "Kintamani Washed", Price: decimal.NewFromInt(142000), Category: "Green Bean", Description: "Asam jeruk, madu."},
	{SKU: "GB-004", Name: "Flores Bajawa Natural", Price: decimal.NewFromInt(139000), Category: "Green Bean", Description: "Floral, cokelat susu."},
	{SKU: "RB-001", Name: "House Blend 250 g", Price: decimal.NewFromInt(85000), Category: "Roasted Bean", Description: "Medium roast."},
	{SKU: "RB-002", Name: "Single Origin Gayo 250 g", Price: decimal.NewFromInt(98000), Category: "Roasted Bean", Description: "Medium-dark roast."},
	{SKU: "RB-003", Name: "Espresso Robusta 500 g", Price: decimal.NewFromInt(110000), Category: "Roasted Bean", Description: "Dark roast."},
	{SKU: "RB-004", Name: "Kintamani Drip Bag (10)", Price: decimal.NewFromInt(72000), Category: "Roasted Bean", Description: "Praktis."},
}

func (sampleSource) Categories(_ context.Context, limit int) ([]Category, error) {
	return sampleCats[:min(limit, len(sampleCats))], nil
}

func (sampleSource) Items(_ context.Context, q ItemQuery) ([]Item, error) {
	var out []Item
	for _, it := range sampleItems {
		if q.CategoryID != nil && it.Category != nameOf(sampleCats, *q.CategoryID) {
			continue
		}
		it.ID = uuid.New()
		out = append(out, it)
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (sampleSource) Levels(context.Context, int) ([]Level, error) {
	return []Level{
		{Name: "Reguler", MinPoints: 0, SpendPerPoint: decimal.NewFromInt(10000), PointValue: decimal.NewFromInt(100)},
		{Name: "Gold", MinPoints: 500, SpendPerPoint: decimal.NewFromInt(6000), PointValue: decimal.NewFromInt(100)},
	}, nil
}

func (sampleSource) Coupons(context.Context, time.Time, int) ([]Coupon, error) {
	return []Coupon{{Code: "HEMAT10", Name: "Diskon 10%", Kind: "percent", Value: decimal.NewFromInt(10), MaxDiscount: decimal.NewFromInt(25000), MinSpend: decimal.NewFromInt(100000)}}, nil
}

func (sampleSource) Outlets(context.Context, int) ([]Outlet, error) {
	return []Outlet{{Name: "Outlet Pusat"}, {Name: "Outlet Cabang"}}, nil
}

func (sampleSource) PayMethods(context.Context, int) ([]PayMethod, error) {
	return []PayMethod{{Name: "Tunai", Kind: "cash"}, {Name: "QRIS", Kind: "ewallet"}}, nil
}

func (sampleSource) MemberTotal(context.Context) (int64, error) { return 1250, nil }

func nameOf(cs []Category, id uuid.UUID) string {
	for _, c := range cs {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// PreviewSample merender templat dengan data contoh dan membungkusnya jadi halaman utuh (CSS disisipkan inline karena
// dibuka lewat iframe srcdoc ber-sandbox). Dipakai editor template Platform Admin.
func (s *Service) PreviewSample(ctx context.Context, html, css, assetBase string) (string, error) {
	in, f := TemplateInput{Code: "preview", Name: "preview", HTML: html, CSS: css}.validate()
	if len(f) > 0 {
		return "", f
	}
	raw, err := Render(ctx, in.HTML, sampleSource{}, Site{Name: "Nusantara Coffee Export", Slug: "contoh", Now: time.Now()})
	if err != nil {
		return "", err
	}
	body, _ := SanitizeHTML(raw, false)
	return Document("Pratinjau template", body, assetBase+tailwindPath+"?v="+tailwindVersion, "", in.CSS), nil
}
