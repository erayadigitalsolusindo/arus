// Package voucher: kupon belanja global per tenant. Kode dibuat sendiri oleh pemilik; potongan persen atau nominal,
// berlaku pada rentang tanggal tertentu, dengan minimal belanja dan batas jumlah pemakaian (opsional). Kasir memasukkan
// kode SEBELUM bayar; penjualan (`internal/sales`) memakai Load/Evaluate/Consume di dalam transaksi nota.
package voucher

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// ModuleID = id modul izin (menu "Kupon Belanja").
const ModuleID = "coupons"

const (
	KindPercent = "percent"
	KindAmount  = "amount"

	maxName  = 100
	maxLimit = 100
	defLimit = 25
)

var (
	ErrNotFound  = errors.New("kupon tidak ditemukan")
	ErrCodeTaken = errors.New("kode kupon sudah dipakai")

	// CodeRe = bentuk kode yang sah (sudah HURUF BESAR).
	CodeRe   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,31}$`)
	maxMoney = decimal.New(1, 12)
)

// FieldErrors = kode galat per field (diterjemahkan klien: errors.FIELD_<kode>).
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

// NormalizeCode merapikan kode ketikan pengguna: spasi dibuang di ujung, huruf besar.
func NormalizeCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// Voucher = bentuk respons API.
type Voucher struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Value       string    `json:"value"`
	MaxDiscount string    `json:"max_discount"` // "" = tanpa batas
	MinSpend    string    `json:"min_spend"`
	StartsOn    string    `json:"starts_on"` // YYYY-MM-DD, "" = langsung
	EndsOn      string    `json:"ends_on"`   // YYYY-MM-DD, "" = tanpa akhir
	MaxUses     *int32    `json:"max_uses"`  // nil = tak terbatas
	UsedCount   int32     `json:"used_count"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// Input = isi form kupon. Angka sebagai string desimal; tanggal "YYYY-MM-DD" atau kosong.
type Input struct {
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	Kind        string      `json:"kind"`
	Value       json.Number `json:"value"`
	MaxDiscount json.Number `json:"max_discount"`
	MinSpend    json.Number `json:"min_spend"`
	StartsOn    string      `json:"starts_on"`
	EndsOn      string      `json:"ends_on"`
	MaxUses     *int32      `json:"max_uses"`
}

type clean struct {
	code, name, kind string
	value, minSpend  decimal.Decimal
	maxDiscount      pgtype.Numeric
	startsOn, endsOn pgtype.Date
	maxUses          pgtype.Int4
}

func money(n json.Number, required bool) (decimal.Decimal, string) {
	s := strings.TrimSpace(n.String())
	if s == "" {
		if required {
			return decimal.Zero, sanitize.Required
		}
		return decimal.Zero, ""
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(maxMoney) {
		return decimal.Zero, sanitize.Invalid
	}
	return d, ""
}

func date(s string) (pgtype.Date, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Date{}, true
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}, false
	}
	return pgtype.Date{Time: t, Valid: true}, true
}

func validate(in Input) (clean, FieldErrors) {
	f := FieldErrors{}
	var c clean
	var code string
	if c.name, code = sanitize.Name(in.Name, maxName); code != "" {
		f["name"] = code
	}
	switch c.code = NormalizeCode(in.Code); {
	case c.code == "":
		f["code"] = sanitize.Required
	case !CodeRe.MatchString(c.code):
		f["code"] = sanitize.Invalid
	}
	switch in.Kind {
	case KindPercent, KindAmount:
		c.kind = in.Kind
	default:
		f["kind"] = sanitize.Invalid
	}
	if c.value, code = money(in.Value, true); code != "" {
		f["value"] = code
	} else if !c.value.IsPositive() || (c.kind == KindPercent && c.value.GreaterThan(decimal.NewFromInt(100))) {
		f["value"] = sanitize.Invalid
	}
	if c.minSpend, code = money(in.MinSpend, false); code != "" {
		f["min_spend"] = code
	}
	if md, code := money(in.MaxDiscount, false); code != "" {
		f["max_discount"] = code
	} else if md.IsPositive() {
		if c.kind != KindPercent {
			f["max_discount"] = sanitize.Invalid // batas potongan hanya bermakna untuk kupon persen
		} else {
			c.maxDiscount = pgtype.Numeric{Int: md.Coefficient(), Exp: md.Exponent(), Valid: true}
		}
	}
	var ok bool
	if c.startsOn, ok = date(in.StartsOn); !ok {
		f["starts_on"] = sanitize.Invalid
	}
	if c.endsOn, ok = date(in.EndsOn); !ok {
		f["ends_on"] = sanitize.Invalid
	}
	if c.startsOn.Valid && c.endsOn.Valid && c.endsOn.Time.Before(c.startsOn.Time) {
		f["ends_on"] = "BEFORE_START"
	}
	if in.MaxUses != nil {
		if *in.MaxUses < 1 || *in.MaxUses > 1_000_000_000 {
			f["max_uses"] = sanitize.Invalid
		} else {
			c.maxUses = pgtype.Int4{Int32: *in.MaxUses, Valid: true}
		}
	}
	if len(f) > 0 {
		return clean{}, f
	}
	return c, nil
}

func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func capStr(n decimal.NullDecimal) string {
	if !n.Valid {
		return ""
	}
	return n.Decimal.StringFixed(2)
}

func maxUsesPtr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}

func view(id uuid.UUID, code, name, kind string, value decimal.Decimal, maxDisc pgtype.Numeric, minSpend decimal.Decimal,
	starts, ends pgtype.Date, maxUses pgtype.Int4, used int32, active bool, created pgtype.Timestamptz) Voucher {
	return Voucher{ID: id, Code: code, Name: name, Kind: kind, Value: value.StringFixed(2), MaxDiscount: capStr(numNull(maxDisc)),
		MinSpend: minSpend.StringFixed(2), StartsOn: dateStr(starts), EndsOn: dateStr(ends), MaxUses: maxUsesPtr(maxUses),
		UsedCount: used, Active: active, CreatedAt: created.Time}
}

func conflict(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "vouchers_tenant_code_key":
		return ErrCodeTaken
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type ListParams struct {
	Q      string
	Active *bool
	Limit  int
	Offset int
}

func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) ([]Voucher, int, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || len([]rune(q)) > maxName {
		return nil, 0, FieldErrors{"q": sanitize.Invalid}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	arg := gen.VoucherListParams{TenantID: a.TenantID, Q: likeEscape(q), PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(p.Offset, 0))}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Voucher{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).VoucherList(ctx, arg)
		for _, r := range rows {
			out = append(out, view(r.ID, r.Code, r.Name, r.Kind, r.Value, r.MaxDiscount, r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active, r.CreatedAt))
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

func (s *Service) Create(ctx context.Context, a authz.Actor, in Input) (*Voucher, error) {
	c, f := validate(in)
	if f != nil {
		return nil, f
	}
	var out Voucher
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).VoucherCreate(ctx, gen.VoucherCreateParams{TenantID: a.TenantID, Code: c.code, Name: c.name, Kind: c.kind,
			Value: c.value, MaxDiscount: c.maxDiscount, MinSpend: c.minSpend, StartsOn: c.startsOn, EndsOn: c.endsOn, MaxUses: c.maxUses})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Code, r.Name, r.Kind, r.Value, r.MaxDiscount, r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionVoucherCreate, Entity: audit.EntityVoucher, EntityID: r.ID.String(), Details: details(out),
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func details(v Voucher) map[string]any {
	return map[string]any{"code": v.Code, "name": v.Name, "kind": v.Kind, "value": v.Value, "max_discount": v.MaxDiscount,
		"min_spend": v.MinSpend, "starts_on": v.StartsOn, "ends_on": v.EndsOn, "max_uses": v.MaxUses}
}

func (s *Service) Update(ctx context.Context, a authz.Actor, id uuid.UUID, in Input) (*Voucher, error) {
	c, f := validate(in)
	if f != nil {
		return nil, f
	}
	var out Voucher
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.VoucherGet(ctx, gen.VoucherGetParams{TenantID: a.TenantID, ID: id}) // mengunci baris
		if err != nil {
			return err
		}
		// Kode sudah tercetak/diberikan ke pelanggan dan tercatat di nota → tak boleh diganti setelah dipakai.
		if cur.UsedCount > 0 && cur.Code != c.code {
			return FieldErrors{"code": "LOCKED"}
		}
		if c.maxUses.Valid && c.maxUses.Int32 < cur.UsedCount {
			return FieldErrors{"max_uses": "BELOW_USED"}
		}
		r, err := q.VoucherUpdate(ctx, gen.VoucherUpdateParams{TenantID: a.TenantID, ID: id, Code: c.code, Name: c.name, Kind: c.kind,
			Value: c.value, MaxDiscount: c.maxDiscount, MinSpend: c.minSpend, StartsOn: c.startsOn, EndsOn: c.endsOn, MaxUses: c.maxUses})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Code, r.Name, r.Kind, r.Value, r.MaxDiscount, r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active, r.CreatedAt)
		before := view(cur.ID, cur.Code, cur.Name, cur.Kind, cur.Value, cur.MaxDiscount, cur.MinSpend, cur.StartsOn, cur.EndsOn, cur.MaxUses, cur.UsedCount, cur.Active, cur.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionVoucherUpdate, Entity: audit.EntityVoucher, EntityID: id.String(),
			Details: map[string]any{"before": details(before), "after": details(out)},
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) SetActive(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) (*Voucher, error) {
	var out Voucher
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).VoucherSetActive(ctx, gen.VoucherSetActiveParams{TenantID: a.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Code, r.Name, r.Kind, r.Value, r.MaxDiscount, r.MinSpend, r.StartsOn, r.EndsOn, r.MaxUses, r.UsedCount, r.Active, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionVoucherActive, Entity: audit.EntityVoucher, EntityID: id.String(),
			Details: map[string]any{"code": out.Code, "active": active},
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}
