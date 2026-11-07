// Package storefront: landing page publik per tenant (/s/{slug}). Pemilik toko menulis HTML + class Tailwind + CSS sendiri
// dengan penanda {namabarang} dst. (lihat template.go). Semua teks disaring (sanitize.go) dan dilayani tanpa script
// (handler.go). Data barang dibaca dengan konteks tenant (RLS) dari slug publik, tanpa token.
package storefront

import (
	"context"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

var (
	ErrSlugTaken = errors.New("alamat halaman sudah dipakai")
	ErrNotFound  = errors.New("halaman tidak ditemukan")
)

const keepVersions = 20

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)

type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "validasi gagal" }

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type Config struct {
	Slug      string    `json:"slug"`
	Enabled   bool      `json:"enabled"`
	Title     string    `json:"title"`
	HTML      string    `json:"html"`
	CSS       string    `json:"css"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VersionInfo struct {
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	Bytes     int       `json:"bytes"`
}

type SaveInput struct {
	Slug    string
	Enabled bool
	Title   string
	HTML    string
	CSS     string
}

// Get membaca pengaturan halaman milik tenant. Belum pernah disimpan -> nil (UI menawarkan templat awal).
func (s *Service) Get(ctx context.Context, a authz.Actor) (*Config, error) {
	var c Config
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT slug, enabled, title, html, css, version, updated_at FROM storefronts WHERE tenant_id = $1`, a.TenantID).
			Scan(&c.Slug, &c.Enabled, &c.Title, &c.HTML, &c.CSS, &c.Version, &c.UpdatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

func validate(in SaveInput) (SaveInput, FieldErrors) {
	f := FieldErrors{}
	if !slugRe.MatchString(in.Slug) {
		f["slug"] = "INVALID" // 3-40 huruf kecil/angka/strip
	}
	if utf8.RuneCountInString(in.Title) > 80 {
		f["title"] = "INVALID"
	}
	if len(in.HTML) > 60000 {
		f["html"] = "TOO_LONG"
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

// Save menyimpan versi baru. Mengembalikan peringatan = bagian HTML yang akan dibuang penyaring saat tampil.
func (s *Service) Save(ctx context.Context, a authz.Actor, in SaveInput) (*Config, []string, error) {
	in, f := validate(in)
	if len(f) > 0 {
		return nil, nil, f
	}
	_, issues := SanitizeHTML(in.HTML, true)
	var c Config
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO storefronts (tenant_id, slug, enabled, title, html, css, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (tenant_id) DO UPDATE SET slug = EXCLUDED.slug, enabled = EXCLUDED.enabled, title = EXCLUDED.title,
			       html = EXCLUDED.html, css = EXCLUDED.css, updated_by = EXCLUDED.updated_by, version = storefronts.version + 1
			RETURNING slug, enabled, title, html, css, version, updated_at`,
			a.TenantID, in.Slug, in.Enabled, in.Title, in.HTML, in.CSS, a.UserID).
			Scan(&c.Slug, &c.Enabled, &c.Title, &c.HTML, &c.CSS, &c.Version, &c.UpdatedAt)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO storefront_versions (tenant_id, version, title, html, css, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
			a.TenantID, c.Version, c.Title, c.HTML, c.CSS, a.UserID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM storefront_versions WHERE tenant_id = $1 AND version <= $2`, a.TenantID, c.Version-keepVersions); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStorefrontSave, Entity: audit.EntityStorefront, EntityID: a.TenantID.String(),
			Details: map[string]any{"slug": c.Slug, "enabled": c.Enabled, "version": c.Version, "html_bytes": len(c.HTML), "css_bytes": len(c.CSS)},
		})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "storefronts_slug_key" {
		return nil, nil, ErrSlugTaken
	}
	if err != nil {
		return nil, nil, err
	}
	return &c, issues, nil
}

func (s *Service) Versions(ctx context.Context, a authz.Actor) ([]VersionInfo, error) {
	out := []VersionInfo{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT version, title, created_at, octet_length(html) + octet_length(css) FROM storefront_versions
			WHERE tenant_id = $1 ORDER BY version DESC LIMIT $2`, a.TenantID, keepVersions)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v VersionInfo
			if err := rows.Scan(&v.Version, &v.Title, &v.CreatedAt, &v.Bytes); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) GetVersion(ctx context.Context, a authz.Actor, version int) (*Config, error) {
	var c Config
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		c.Version = version
		return tx.QueryRow(ctx, `SELECT title, html, css, created_at FROM storefront_versions WHERE tenant_id = $1 AND version = $2`, a.TenantID, version).
			Scan(&c.Title, &c.HTML, &c.CSS, &c.UpdatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// ---- publik ----

// Published = halaman aktif hasil pencarian slug.
type Published struct {
	TenantID   uuid.UUID
	TenantName string
	Title      string
	HTML, CSS  string
	Version    int
}

func (s *Service) Lookup(ctx context.Context, slug string) (*Published, error) {
	if !slugRe.MatchString(slug) {
		return nil, ErrNotFound
	}
	var p Published
	err := s.pool.QueryRow(ctx, `SELECT tenant_id, tenant_name, title, html, css, version FROM storefront_public_lookup($1)`, slug).
		Scan(&p.TenantID, &p.TenantName, &p.Title, &p.HTML, &p.CSS, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// ImageOK memastikan gambar milik barang aktif tenant itu (agar URL gambar tak bisa dipakai menebak berkas tenant lain).
func (s *Service) ImageOK(ctx context.Context, tenant, imageID uuid.UUID) (bool, error) {
	var ok bool
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM item_images ii JOIN items i ON i.tenant_id = ii.tenant_id AND i.id = ii.item_id
			WHERE ii.tenant_id = $1 AND ii.id = $2 AND i.active)`, tenant, imageID).Scan(&ok)
	})
	return ok, err
}

// dbSource = Source untuk satu tenant; tiap panggilan satu transaksi baca ber-RLS.
type dbSource struct {
	pool   *pgxpool.Pool
	tenant uuid.UUID
}

func (s *Service) source(tenant uuid.UUID) Source { return &dbSource{pool: s.pool, tenant: tenant} }

func (d *dbSource) Categories(ctx context.Context, limit int) ([]Category, error) {
	var out []Category
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name FROM categories WHERE tenant_id = $1 AND active ORDER BY lower(name), id LIMIT $2`, d.tenant, int32(limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Category
			if err := rows.Scan(&c.ID, &c.Name); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

var orderSQL = map[string]string{
	"nama":     "lower(i.name), i.id",
	"terbaru":  "i.created_at DESC, i.id DESC",
	"termurah": "i.sell_price, i.id",
	"termahal": "i.sell_price DESC, i.id",
}

func (d *dbSource) Items(ctx context.Context, q ItemQuery) ([]Item, error) {
	order, ok := orderSQL[q.Order]
	if !ok {
		order = orderSQL["nama"]
	}
	var out []Item
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		var cat any
		if q.CategoryID != nil {
			cat = *q.CategoryID
		}
		// order berasal dari peta tetap di atas (bukan masukan pengguna).
		rows, err := tx.Query(ctx, `
			SELECT i.id, i.sku, i.name, i.description, i.sell_price, coalesce(c.name, ''), mi.id
			  FROM items i
			  LEFT JOIN categories c  ON c.tenant_id = i.tenant_id AND c.id = i.category_id
			  LEFT JOIN item_images mi ON mi.tenant_id = i.tenant_id AND mi.item_id = i.id AND mi.is_main
			 WHERE i.tenant_id = $1 AND i.active AND ($2::uuid IS NULL OR i.category_id = $2::uuid)
			 ORDER BY `+order+` LIMIT $3`, d.tenant, cat, int32(q.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				it    Item
				price decimal.Decimal
				img   *uuid.UUID
			)
			if err := rows.Scan(&it.ID, &it.SKU, &it.Name, &it.Description, &price, &it.Category, &img); err != nil {
				return err
			}
			it.Price, it.ImageID = price, img
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}
