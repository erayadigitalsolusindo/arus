// Package catalog: master pendukung katalog barang per tenant — satuan, kategori, brand, principal, supplier
// (Fase 3.1). Master tidak pernah dihapus permanen; dinonaktifkan agar item/transaksi yang merujuknya tetap utuh.
package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

var (
	ErrNotFound  = errors.New("data tidak ditemukan")
	ErrNameTaken = errors.New("nama sudah dipakai")
	ErrCodeTaken = errors.New("kode sudah dipakai")
)

// FieldErrors = kode galat per field (REQUIRED/INVALID/TOO_LONG), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

const (
	maxName = 100
	defLim  = 20
	maxLim  = 100
)

// Kind = satu master sederhana (hanya nama + status). Tabel dan modul izin adalah konstanta di kode ini, tidak
// pernah dari input pengguna — itulah sebabnya nama tabel boleh disisipkan ke SQL (sqlc tidak bisa memparameterkan
// nama tabel, dan keempat master ini bentuknya identik).
type Kind struct {
	Path   string // segmen URL sekaligus id modul izin (authz.Modules)
	table  string
	entity string
	create string
	update string
	active string
}

var Kinds = []Kind{
	{"units", "units", audit.EntityUnit, audit.ActionUnitCreate, audit.ActionUnitUpdate, audit.ActionUnitActive},
	{"categories", "categories", audit.EntityCategory, audit.ActionCategoryCreate, audit.ActionCategoryUpdate, audit.ActionCategoryActive},
	{"brands", "brands", audit.EntityBrand, audit.ActionBrandCreate, audit.ActionBrandUpdate, audit.ActionBrandActive},
	{"principals", "principals", audit.EntityPrincipal, audit.ActionPrincipalCreate, audit.ActionPrincipalUpdate, audit.ActionPrincipalActive},
}

// Entry = satu baris master sederhana.
type Entry struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// ListParams: Q = pencarian nama (bagian kata, tanpa membedakan huruf besar/kecil); Active nil = semua.
type ListParams struct {
	Q      string
	Active *bool
	Limit  int
	Offset int
}

func (p ListParams) norm() (q string, limit, offset int, err error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || len([]rune(q)) > maxName {
		return "", 0, 0, FieldErrors{"q": sanitize.Invalid}
	}
	limit, offset = p.Limit, p.Offset
	if limit <= 0 {
		limit = defLim
	}
	limit = min(limit, maxLim)
	return q, limit, max(offset, 0), nil
}

// likeEscape membuat input pengguna dicocokkan apa adanya oleh ILIKE (karakter % _ \ tidak jadi wildcard).
func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", `%`, `\%`, `_`, `\_`).Replace(s)
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func (s *Service) List(ctx context.Context, actor authz.Actor, k Kind, p ListParams) ([]Entry, int, error) {
	q, limit, offset, err := p.norm()
	if err != nil {
		return nil, 0, err
	}
	out := []Entry{}
	total := 0
	err = db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, name, active, created_at, count(*) OVER ()
			FROM `+k.table+`
			WHERE tenant_id = $1 AND ($2::text = '' OR name ILIKE '%' || $2::text || '%') AND ($3::boolean IS NULL OR active = $3)
			ORDER BY lower(name), id LIMIT $4 OFFSET $5`,
			actor.TenantID, likeEscape(q), p.Active, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Entry
			if err := rows.Scan(&e.ID, &e.Name, &e.Active, &e.CreatedAt, &total); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, total, err
}

func cleanName(raw string) (string, FieldErrors) {
	name, code := sanitize.Name(raw, maxName)
	if code != "" {
		return "", FieldErrors{"name": code}
	}
	return name, nil
}

func (s *Service) Create(ctx context.Context, actor authz.Actor, k Kind, rawName string) (*Entry, error) {
	name, f := cleanName(rawName)
	if f != nil {
		return nil, f
	}
	var e Entry
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO `+k.table+` (tenant_id, name) VALUES ($1, $2)
			RETURNING id, name, active, created_at`, actor.TenantID, name).
			Scan(&e.ID, &e.Name, &e.Active, &e.CreatedAt)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: k.create, Entity: k.entity, EntityID: e.ID.String(), Details: map[string]any{"name": e.Name},
		})
	})
	if uniqueViolation(err, k.table+"_tenant_name_key") {
		return nil, ErrNameTaken
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Service) Rename(ctx context.Context, actor authz.Actor, k Kind, id uuid.UUID, rawName string) (*Entry, error) {
	name, f := cleanName(rawName)
	if f != nil {
		return nil, f
	}
	var e Entry
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		var before string
		// Nama lama dibaca dengan kunci baris agar catatan audit "sebelum" selalu benar walau ada ubah bersamaan.
		if err := tx.QueryRow(ctx, `SELECT name FROM `+k.table+` WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, actor.TenantID, id).Scan(&before); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `UPDATE `+k.table+` SET name = $3 WHERE tenant_id = $1 AND id = $2
			RETURNING id, name, active, created_at`, actor.TenantID, id, name).
			Scan(&e.ID, &e.Name, &e.Active, &e.CreatedAt)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: k.update, Entity: k.entity, EntityID: id.String(),
			Details: map[string]any{"before": map[string]any{"name": before}, "after": map[string]any{"name": e.Name}},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if uniqueViolation(err, k.table+"_tenant_name_key") {
		return nil, ErrNameTaken
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// SetActive mengarsipkan (false) atau mengaktifkan kembali (true).
func (s *Service) SetActive(ctx context.Context, actor authz.Actor, k Kind, id uuid.UUID, active bool) (*Entry, error) {
	var e Entry
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE `+k.table+` SET active = $3 WHERE tenant_id = $1 AND id = $2
			RETURNING id, name, active, created_at`, actor.TenantID, id, active).
			Scan(&e.ID, &e.Name, &e.Active, &e.CreatedAt)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: k.active, Entity: k.entity, EntityID: id.String(), Details: map[string]any{"name": e.Name, "active": active},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
