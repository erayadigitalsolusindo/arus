package stock

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Laporan master (member, supplier, salesman): daftar data induk tenant, urut nama, paginasi keyset
// (cursor = id baris terakhir; urutan (lower(name), id) memakai indeks nama per tenant).
// Tanpa total/count seluruh tabel agar tetap cepat di data besar.

const (
	defaultMasterLimit = 100
	maxMasterLimit     = 500
)

// MasterParams filter laporan master.
type MasterParams struct {
	Q      string // awalan nama/kode/telepon; kosong = semua
	Status string // "active" | "inactive" | "" (semua)
	Limit  int
	Cursor string // id baris terakhir halaman sebelumnya
}

func (p MasterParams) normalize() (limit int, q string, active any, cursor any, err error) {
	limit = p.Limit
	if limit <= 0 {
		limit = defaultMasterLimit
	}
	if limit > maxMasterLimit {
		limit = maxMasterLimit
	}
	switch p.Status {
	case "":
	case "active":
		active = true
	case "inactive":
		active = false
	default:
		return 0, "", nil, nil, FieldErrors{"status": "INVALID"}
	}
	if p.Cursor != "" {
		cursor = parseUUID(p.Cursor)
		if cursor == nil {
			return 0, "", nil, nil, FieldErrors{"cursor": "INVALID"}
		}
	}
	// Escape wildcard LIKE agar input pengguna dibaca harfiah.
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(strings.TrimSpace(p.Q)))
	return limit, q, active, cursor, nil
}

// MasterMemberRow satu member.
type MasterMemberRow struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	City        string `json:"city"`
	Level       string `json:"level"`
	Points      int    `json:"points"`
	CreditLimit string `json:"credit_limit"`
	DueDays     int    `json:"due_days"`
	ValidUntil  string `json:"valid_until,omitempty"`
	Active      bool   `json:"active"`
}

// MasterMemberResult hasil laporan master member.
type MasterMemberResult struct {
	Data       []MasterMemberRow `json:"data"`
	Limit      int               `json:"limit"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// MasterMembers daftar member + level (dihitung dari lifetime_points, seperti modul member).
func (s *Service) MasterMembers(ctx context.Context, a authz.Actor, p MasterParams) (MasterMemberResult, error) {
	res := MasterMemberResult{Data: []MasterMemberRow{}}
	limit, q, active, cursor, err := p.normalize()
	if err != nil {
		return res, err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.id::text, m.code, m.name, m.gender, m.phone, m.email, m.address, m.city,
			       coalesce(lv.name, ''), m.points, m.credit_limit, m.due_days,
			       coalesce(to_char(m.valid_until, 'YYYY-MM-DD'), ''), m.active
			FROM members m
			LEFT JOIN LATERAL (
				SELECT l.name FROM member_levels l
				WHERE l.tenant_id = m.tenant_id AND l.active AND l.min_points <= m.lifetime_points
				ORDER BY l.min_points DESC LIMIT 1
			) lv ON true
			WHERE m.tenant_id = $1
			  AND ($2::boolean IS NULL OR m.active = $2::boolean)
			  AND ($3::text = '' OR lower(m.name) LIKE $3 || '%' OR lower(m.code) LIKE $3 || '%' OR m.phone LIKE $3 || '%')
			  AND ($4::uuid IS NULL OR (lower(m.name), m.id) > (SELECT lower(c.name), c.id FROM members c WHERE c.tenant_id = $1 AND c.id = $4::uuid))
			ORDER BY lower(m.name), m.id
			LIMIT $5`, a.TenantID, active, q, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r MasterMemberRow
			var credit pgtype.Numeric
			if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Gender, &r.Phone, &r.Email, &r.Address, &r.City,
				&r.Level, &r.Points, &credit, &r.DueDays, &r.ValidUntil, &r.Active); err != nil {
				return err
			}
			r.CreditLimit = numericString(credit)
			res.Data = append(res.Data, r)
		}
		return rows.Err()
	})
	if err != nil {
		return res, err
	}
	if len(res.Data) > limit {
		res.Data = res.Data[:limit]
		res.NextCursor = res.Data[limit-1].ID
	}
	res.Limit = limit
	return res, nil
}

// MasterSupplierRow satu supplier.
type MasterSupplierRow struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Address     string `json:"address"`
	Note        string `json:"note"`
	Active      bool   `json:"active"`
}

// MasterSupplierResult hasil laporan master supplier.
type MasterSupplierResult struct {
	Data       []MasterSupplierRow `json:"data"`
	Limit      int                 `json:"limit"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

// MasterSuppliers daftar supplier.
func (s *Service) MasterSuppliers(ctx context.Context, a authz.Actor, p MasterParams) (MasterSupplierResult, error) {
	res := MasterSupplierResult{Data: []MasterSupplierRow{}}
	limit, q, active, cursor, err := p.normalize()
	if err != nil {
		return res, err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.id::text, coalesce(m.code, ''), m.name, m.contact_name, m.phone, m.email, m.address, m.note, m.active
			FROM suppliers m
			WHERE m.tenant_id = $1
			  AND ($2::boolean IS NULL OR m.active = $2::boolean)
			  AND ($3::text = '' OR lower(m.name) LIKE $3 || '%' OR lower(m.code) LIKE $3 || '%' OR m.phone LIKE $3 || '%')
			  AND ($4::uuid IS NULL OR (lower(m.name), m.id) > (SELECT lower(c.name), c.id FROM suppliers c WHERE c.tenant_id = $1 AND c.id = $4::uuid))
			ORDER BY lower(m.name), m.id
			LIMIT $5`, a.TenantID, active, q, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r MasterSupplierRow
			if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.ContactName, &r.Phone, &r.Email, &r.Address, &r.Note, &r.Active); err != nil {
				return err
			}
			res.Data = append(res.Data, r)
		}
		return rows.Err()
	})
	if err != nil {
		return res, err
	}
	if len(res.Data) > limit {
		res.Data = res.Data[:limit]
		res.NextCursor = res.Data[limit-1].ID
	}
	res.Limit = limit
	return res, nil
}

// MasterSalespersonRow satu salesman.
type MasterSalespersonRow struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	CommissionPct string `json:"commission_pct"`
	Note          string `json:"note"`
	Active        bool   `json:"active"`
}

// MasterSalespersonResult hasil laporan master salesman.
type MasterSalespersonResult struct {
	Data       []MasterSalespersonRow `json:"data"`
	Limit      int                    `json:"limit"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

// MasterSalespeople daftar salesman.
func (s *Service) MasterSalespeople(ctx context.Context, a authz.Actor, p MasterParams) (MasterSalespersonResult, error) {
	res := MasterSalespersonResult{Data: []MasterSalespersonRow{}}
	limit, q, active, cursor, err := p.normalize()
	if err != nil {
		return res, err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.id::text, coalesce(m.code, ''), m.name, m.phone, m.commission_pct, m.note, m.active
			FROM salespeople m
			WHERE m.tenant_id = $1
			  AND ($2::boolean IS NULL OR m.active = $2::boolean)
			  AND ($3::text = '' OR lower(m.name) LIKE $3 || '%' OR lower(m.code) LIKE $3 || '%' OR m.phone LIKE $3 || '%')
			  AND ($4::uuid IS NULL OR (lower(m.name), m.id) > (SELECT lower(c.name), c.id FROM salespeople c WHERE c.tenant_id = $1 AND c.id = $4::uuid))
			ORDER BY lower(m.name), m.id
			LIMIT $5`, a.TenantID, active, q, cursor, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r MasterSalespersonRow
			var pct pgtype.Numeric
			if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.Phone, &pct, &r.Note, &r.Active); err != nil {
				return err
			}
			r.CommissionPct = numericString(pct)
			res.Data = append(res.Data, r)
		}
		return rows.Err()
	})
	if err != nil {
		return res, err
	}
	if len(res.Data) > limit {
		res.Data = res.Data[:limit]
		res.NextCursor = res.Data[limit-1].ID
	}
	res.Limit = limit
	return res, nil
}
