package catalog

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	maxContact = 100
	maxAddress = 500
	maxNote    = 500
)

var supplierCodeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,29}$`)

type Supplier struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	ContactName string    `json:"contact_name"`
	Phone       string    `json:"phone"`
	Email       string    `json:"email"`
	Address     string    `json:"address"`
	Note        string    `json:"note"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// SupplierInput: semua field selain Name opsional (kosong = tidak diisi).
type SupplierInput struct {
	Code, Name, ContactName, Phone, Email, Address, Note string
}

type supplierClean struct {
	code                                    pgtype.Text
	name, contact, phone, email, addr, note string
}

func optionalText(raw string, max int) (string, string) {
	v, ok := sanitize.Text(raw)
	switch {
	case !ok:
		return "", sanitize.Invalid
	case len([]rune(v)) > max:
		return "", sanitize.TooLong
	}
	return v, ""
}

func validateSupplier(in SupplierInput) (supplierClean, FieldErrors) {
	f := FieldErrors{}
	var c supplierClean
	var code string
	if c.name, code = sanitize.Name(in.Name, maxName); code != "" {
		f["name"] = code
	}
	if in.Code != "" {
		v, ok := sanitize.Text(in.Code)
		if !ok || !supplierCodeRe.MatchString(v) {
			f["code"] = sanitize.Invalid
		} else {
			c.code = pgtype.Text{String: v, Valid: true}
		}
	}
	if c.contact, code = optionalText(in.ContactName, maxContact); code != "" {
		f["contact_name"] = code
	} else if containsMarkup(c.contact) {
		f["contact_name"] = sanitize.Invalid
	}
	if in.Phone != "" {
		if c.phone, code = sanitize.Phone(in.Phone); code != "" {
			f["phone"] = code
		}
	}
	if in.Email != "" {
		if c.email, code = sanitize.Email(in.Email); code != "" {
			f["email"] = code
		}
	}
	if c.addr, code = optionalText(in.Address, maxAddress); code != "" {
		f["address"] = code
	}
	if c.note, code = optionalText(in.Note, maxNote); code != "" {
		f["note"] = code
	}
	if len(f) > 0 {
		return supplierClean{}, f
	}
	return c, nil
}

func containsMarkup(s string) bool {
	for _, r := range s {
		if r == '<' || r == '>' {
			return true
		}
	}
	return false
}

func supplierOf(id uuid.UUID, code pgtype.Text, name, contact, phone, email, addr, note string, active bool, created pgtype.Timestamptz) Supplier {
	return Supplier{ID: id, Code: code.String, Name: name, ContactName: contact, Phone: phone, Email: email,
		Address: addr, Note: note, Active: active, CreatedAt: created.Time}
}

func (s *Service) ListSuppliers(ctx context.Context, actor authz.Actor, p ListParams) ([]Supplier, int, error) {
	q, limit, offset, err := p.norm()
	if err != nil {
		return nil, 0, err
	}
	arg := gen.SupplierListParams{TenantID: actor.TenantID, Q: likeEscape(q), PageLimit: int32(limit), PageOffset: int32(offset)}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Supplier{}
	total := 0
	err = db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).SupplierList(ctx, arg)
		for _, r := range rows {
			out = append(out, supplierOf(r.ID, r.Code, r.Name, r.ContactName, r.Phone, r.Email, r.Address, r.Note, r.Active, r.CreatedAt))
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

func supplierConflict(err error) error {
	switch {
	case uniqueViolation(err, "suppliers_tenant_name_key"):
		return ErrNameTaken
	case uniqueViolation(err, "suppliers_tenant_code_key"):
		return ErrCodeTaken
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

func (s *Service) CreateSupplier(ctx context.Context, actor authz.Actor, in SupplierInput) (*Supplier, error) {
	c, f := validateSupplier(in)
	if f != nil {
		return nil, f
	}
	var out Supplier
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).SupplierCreate(ctx, gen.SupplierCreateParams{TenantID: actor.TenantID, Code: c.code, Name: c.name,
			ContactName: c.contact, Phone: c.phone, Email: c.email, Address: c.addr, Note: c.note})
		if err != nil {
			return err
		}
		out = supplierOf(r.ID, r.Code, r.Name, r.ContactName, r.Phone, r.Email, r.Address, r.Note, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSupplierCreate, Entity: audit.EntitySupplier, EntityID: r.ID.String(),
			Details: map[string]any{"code": out.Code, "name": out.Name},
		})
	})
	if err = supplierConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) UpdateSupplier(ctx context.Context, actor authz.Actor, id uuid.UUID, in SupplierInput) (*Supplier, error) {
	c, f := validateSupplier(in)
	if f != nil {
		return nil, f
	}
	var out Supplier
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.SupplierGet(ctx, gen.SupplierGetParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		r, err := q.SupplierUpdate(ctx, gen.SupplierUpdateParams{TenantID: actor.TenantID, ID: id, Code: c.code, Name: c.name,
			ContactName: c.contact, Phone: c.phone, Email: c.email, Address: c.addr, Note: c.note})
		if err != nil {
			return err
		}
		out = supplierOf(r.ID, r.Code, r.Name, r.ContactName, r.Phone, r.Email, r.Address, r.Note, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSupplierUpdate, Entity: audit.EntitySupplier, EntityID: id.String(),
			Details: map[string]any{
				"before": map[string]any{"code": cur.Code.String, "name": cur.Name, "phone": cur.Phone, "email": cur.Email},
				"after":  map[string]any{"code": out.Code, "name": out.Name, "phone": out.Phone, "email": out.Email},
			},
		})
	})
	if err = supplierConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) SetSupplierActive(ctx context.Context, actor authz.Actor, id uuid.UUID, active bool) (*Supplier, error) {
	var out Supplier
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).SupplierSetActive(ctx, gen.SupplierSetActiveParams{TenantID: actor.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		out = supplierOf(r.ID, r.Code, r.Name, r.ContactName, r.Phone, r.Email, r.Address, r.Note, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSupplierActive, Entity: audit.EntitySupplier, EntityID: id.String(),
			Details: map[string]any{"name": out.Name, "active": active},
		})
	})
	if err = supplierConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}
