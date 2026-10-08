package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// SalespersonModule = id modul izin salesman (authz.Modules). Kasir (sales_orders.create) boleh membaca lookup-nya.
const SalespersonModule = "salespeople"

var salespersonCodeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,29}$`)

// Salesman = label per tenant untuk nota (bukan akun login). commission_pct hanya disimpan; belum ada hitung komisi.
type Salesperson struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	Note          string    `json:"note"`
	CommissionPct string    `json:"commission_pct"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
}

type SalespersonInput struct {
	Code, Name, Phone, Note string
	CommissionPct           json.Number
}

type salespersonClean struct {
	code        pgtype.Text
	name, phone string
	note        string
	commission  decimal.Decimal
}

func validateSalesperson(in SalespersonInput) (salespersonClean, FieldErrors) {
	f := FieldErrors{}
	var c salespersonClean
	var code string
	if c.name, code = sanitize.Name(in.Name, maxName); code != "" {
		f["name"] = code
	}
	if in.Code != "" {
		v, ok := sanitize.Text(in.Code)
		if !ok || !salespersonCodeRe.MatchString(v) {
			f["code"] = sanitize.Invalid
		} else {
			c.code = pgtype.Text{String: v, Valid: true}
		}
	}
	if in.Phone != "" {
		if c.phone, code = sanitize.Phone(in.Phone); code != "" {
			f["phone"] = code
		}
	}
	if c.note, code = optionalText(in.Note, maxNote); code != "" {
		f["note"] = code
	}
	if s := in.CommissionPct.String(); s != "" {
		d, err := decimal.NewFromString(s)
		switch {
		case err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)) || d.Exponent() < -2:
			f["commission_pct"] = sanitize.Invalid
		default:
			c.commission = d
		}
	}
	if len(f) > 0 {
		return salespersonClean{}, f
	}
	return c, nil
}

func salespersonOf(id uuid.UUID, code pgtype.Text, name, phone, note string, pct decimal.Decimal, active bool, created pgtype.Timestamptz) Salesperson {
	return Salesperson{ID: id, Code: code.String, Name: name, Phone: phone, Note: note, CommissionPct: pct.StringFixed(2),
		Active: active, CreatedAt: created.Time}
}

func salespersonConflict(err error) error {
	switch {
	case uniqueViolation(err, "salespeople_tenant_name_key"):
		return ErrNameTaken
	case uniqueViolation(err, "salespeople_tenant_code_key"):
		return ErrCodeTaken
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

func (s *Service) ListSalespeople(ctx context.Context, actor authz.Actor, p ListParams) ([]Salesperson, int, error) {
	q, limit, offset, err := p.norm()
	if err != nil {
		return nil, 0, err
	}
	arg := gen.SalespersonListParams{TenantID: actor.TenantID, Q: likeEscape(q), PageLimit: int32(limit), PageOffset: int32(offset)}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Salesperson{}
	total := 0
	err = db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).SalespersonList(ctx, arg)
		for _, r := range rows {
			out = append(out, salespersonOf(r.ID, r.Code, r.Name, r.Phone, r.Note, r.CommissionPct, r.Active, r.CreatedAt))
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

func (s *Service) CreateSalesperson(ctx context.Context, actor authz.Actor, in SalespersonInput) (*Salesperson, error) {
	c, f := validateSalesperson(in)
	if f != nil {
		return nil, f
	}
	var out Salesperson
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).SalespersonCreate(ctx, gen.SalespersonCreateParams{TenantID: actor.TenantID, Code: c.code, Name: c.name,
			Phone: c.phone, Note: c.note, CommissionPct: c.commission})
		if err != nil {
			return err
		}
		out = salespersonOf(r.ID, r.Code, r.Name, r.Phone, r.Note, r.CommissionPct, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSalespersonCreate, Entity: audit.EntitySalesperson, EntityID: r.ID.String(),
			Details: map[string]any{"code": out.Code, "name": out.Name, "commission_pct": out.CommissionPct},
		})
	})
	if err = salespersonConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) UpdateSalesperson(ctx context.Context, actor authz.Actor, id uuid.UUID, in SalespersonInput) (*Salesperson, error) {
	c, f := validateSalesperson(in)
	if f != nil {
		return nil, f
	}
	var out Salesperson
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.SalespersonGet(ctx, gen.SalespersonGetParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		r, err := q.SalespersonUpdate(ctx, gen.SalespersonUpdateParams{TenantID: actor.TenantID, ID: id, Code: c.code, Name: c.name,
			Phone: c.phone, Note: c.note, CommissionPct: c.commission})
		if err != nil {
			return err
		}
		out = salespersonOf(r.ID, r.Code, r.Name, r.Phone, r.Note, r.CommissionPct, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSalespersonUpdate, Entity: audit.EntitySalesperson, EntityID: id.String(),
			Details: map[string]any{
				"before": map[string]any{"code": cur.Code.String, "name": cur.Name, "phone": cur.Phone, "commission_pct": cur.CommissionPct.StringFixed(2)},
				"after":  map[string]any{"code": out.Code, "name": out.Name, "phone": out.Phone, "commission_pct": out.CommissionPct},
			},
		})
	})
	if err = salespersonConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) SetSalespersonActive(ctx context.Context, actor authz.Actor, id uuid.UUID, active bool) (*Salesperson, error) {
	var out Salesperson
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).SalespersonSetActive(ctx, gen.SalespersonSetActiveParams{TenantID: actor.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		out = salespersonOf(r.ID, r.Code, r.Name, r.Phone, r.Note, r.CommissionPct, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionSalespersonActive, Entity: audit.EntitySalesperson, EntityID: id.String(),
			Details: map[string]any{"name": out.Name, "active": active},
		})
	})
	if err = salespersonConflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}
