// Package outlet: pengelolaan outlet (cabang/toko) dalam satu tenant dan daftar outlet yang boleh dipilih pengguna.
package outlet

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

var (
	ErrNotFound      = errors.New("outlet tidak ditemukan")
	ErrCodeTaken     = errors.New("kode outlet sudah dipakai")
	ErrLastOutlet    = errors.New("outlet aktif terakhir tidak boleh dinonaktifkan")
	ErrCurrentOutlet = errors.New("outlet yang sedang dipakai tidak boleh dinonaktifkan")
	ErrNotAccessible = errors.New("tidak punya akses ke outlet ini")
)

// FieldErrors = kode galat per field (REQUIRED/INVALID/TOO_LONG), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

const (
	maxName         = 100
	defaultTimezone = "Asia/Jakarta"
	maxAddress      = 200
	maxPhone        = 30
	maxReceiptText  = 300
	// Kertas 58 mm muat ±32 karakter per baris; batas baris menjaga struk tidak memanjang tanpa sengaja.
	maxReceiptLines = 6
)

var codeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,19}$`)

type Outlet struct {
	ID          uuid.UUID       `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	TaxStorePct decimal.Decimal `json:"tax_store_pct"`
	TaxGovPct   decimal.Decimal `json:"tax_gov_pct"`
	Timezone    string          `json:"timezone"`
	Active      bool            `json:"active"`
	CreatedAt   time.Time       `json:"created_at"`
	// Data struk (FR-POS-15): alamat/telepon dicetak di kepala struk, header/footer = teks bebas beberapa baris.
	Address       string `json:"address"`
	Phone         string `json:"phone"`
	ReceiptHeader string `json:"receipt_header"`
	ReceiptFooter string `json:"receipt_footer"`
}

type Service struct {
	pool     *pgxpool.Pool
	resolver *authz.Resolver
}

func NewService(pool *pgxpool.Pool, resolver *authz.Resolver) *Service {
	return &Service{pool: pool, resolver: resolver}
}

// Input untuk membuat/mengubah outlet. Pajak dalam persen (0..100, maks 2 desimal).
type Input struct {
	Code        string // hanya saat membuat; tidak dapat diubah
	Name        string
	Timezone    string
	TaxStorePct string
	TaxGovPct   string
	Active      bool // hanya saat mengubah
	// Data struk; kosong = tidak dicetak.
	Address       string
	Phone         string
	ReceiptHeader string
	ReceiptFooter string
}

type clean struct {
	code, name, tz   string
	taxStore, taxGov decimal.Decimal
	receipt          receiptText
}

// receiptText = isian struk yang sudah dibersihkan.
type receiptText struct{ address, phone, header, footer string }

func validate(in Input, creating bool) (clean, FieldErrors) {
	f := FieldErrors{}
	var c clean
	var code string
	c.name, code = sanitize.Name(in.Name, maxName)
	if code != "" {
		f["name"] = code
	}
	if creating {
		c.code = strings.ToLower(strings.TrimSpace(in.Code))
		if !codeRe.MatchString(c.code) {
			f["code"] = "INVALID"
		}
	}
	c.tz = strings.TrimSpace(in.Timezone)
	if c.tz == "" {
		c.tz = defaultTimezone
	}
	if _, err := time.LoadLocation(c.tz); err != nil || c.tz == "Local" {
		f["timezone"] = "INVALID"
	}
	var ok bool
	if c.taxStore, ok = pct(in.TaxStorePct); !ok {
		f["tax_store_pct"] = "INVALID"
	}
	if c.taxGov, ok = pct(in.TaxGovPct); !ok {
		f["tax_gov_pct"] = "INVALID"
	}
	c.receipt = validateReceipt(in, f)
	if len(f) > 0 {
		return clean{}, f
	}
	return c, nil
}

// validateReceipt membersihkan isian struk dan menulis galat per field ke f.
func validateReceipt(in Input, f FieldErrors) receiptText {
	var r receiptText
	multi := func(field, raw string, max int) string {
		v, code := sanitize.Multiline(raw, max)
		if code == "" && strings.Count(v, "\n") >= maxReceiptLines {
			code = sanitize.TooLong
		}
		if code != "" {
			f[field] = code
		}
		return v
	}
	r.address = multi("address", in.Address, maxAddress)
	r.header = multi("receipt_header", in.ReceiptHeader, maxReceiptText)
	r.footer = multi("receipt_footer", in.ReceiptFooter, maxReceiptText)
	phone, ok := sanitize.Text(in.Phone)
	switch {
	case !ok:
		f["phone"] = sanitize.Invalid
	case utf8.RuneCountInString(phone) > maxPhone:
		f["phone"] = sanitize.TooLong
	}
	r.phone = phone
	return r
}

// pct memvalidasi persen 0..100 dengan maksimal 2 desimal; kosong dianggap 0.
func pct(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, true
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(100)) || d.Exponent() < -2 {
		return decimal.Zero, false
	}
	return d, true
}

// outletRow = kolom outlet yang sama pada semua query sqlc (OutletList/Get/Create/Update).
type outletRow interface {
	gen.OutletListRow | gen.OutletGetRow | gen.OutletCreateRow | gen.OutletUpdateRow
}

func outletOf[R outletRow](row R) Outlet {
	r := gen.OutletListRow(row)
	return Outlet{ID: r.ID, Code: r.Code, Name: r.Name, TaxStorePct: r.TaxStorePct, TaxGovPct: r.TaxGovPct, Timezone: r.Timezone,
		Active: r.Active, CreatedAt: r.CreatedAt.Time, Address: r.Address, Phone: r.Phone, ReceiptHeader: r.ReceiptHeader, ReceiptFooter: r.ReceiptFooter}
}

// List: pemilik melihat semua outlet (termasuk nonaktif); pengguna lain hanya outlet aktif yang ditugaskan.
func (s *Service) List(ctx context.Context, actor authz.Actor) ([]Outlet, error) {
	out := []Outlet{}
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).OutletList(ctx, actor.TenantID)
		for _, r := range rows {
			if actor.Perms.All || actor.Outlets[r.ID] {
				out = append(out, outletOf(r))
			}
		}
		return err
	})
	return out, err
}

// Accessible = outlet aktif yang boleh dipilih pengguna (untuk pemilih outlet di header/sidebar).
func (s *Service) Accessible(ctx context.Context, actor authz.Actor) ([]Outlet, error) {
	all, err := s.List(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, o := range all {
		if o.Active && actor.Outlets[o.ID] {
			out = append(out, o)
		}
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, actor authz.Actor, in Input) (*Outlet, error) {
	c, f := validate(in, true)
	if f != nil {
		return nil, f
	}
	var row gen.OutletCreateRow
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		var err error
		row, err = q.OutletCreate(ctx, gen.OutletCreateParams{TenantID: actor.TenantID, Code: c.code, Name: c.name, TaxStorePct: c.taxStore, TaxGovPct: c.taxGov, Timezone: c.tz,
			Address: c.receipt.address, Phone: c.receipt.phone, ReceiptHeader: c.receipt.header, ReceiptFooter: c.receipt.footer})
		if err != nil {
			return err
		}
		// Pembuat non-pemilik otomatis mendapat akses ke outlet yang ia buat (pemilik selalu punya akses semua outlet).
		if !actor.Perms.All {
			if err := q.OutletAssignUser(ctx, gen.OutletAssignUserParams{TenantID: actor.TenantID, UserID: actor.UserID, OutletID: row.ID}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionOutletCreate, Entity: audit.EntityOutlet, EntityID: row.ID.String(),
			Details: map[string]any{"code": row.Code, "name": row.Name},
		})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "outlets_tenant_code_key" {
		return nil, ErrCodeTaken
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	o := outletOf(row)
	return &o, nil
}

func (s *Service) Update(ctx context.Context, actor authz.Actor, id uuid.UUID, in Input) (*Outlet, error) {
	c, f := validate(in, false)
	if f != nil {
		return nil, f
	}
	var row gen.OutletUpdateRow
	err := db.WithTenant(ctx, s.pool, actor.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		// Kunci outlet aktif lebih dulu supaya penonaktifan bersamaan tidak menyisakan nol outlet aktif.
		active, err := q.OutletLockActive(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		cur, err := q.OutletGet(ctx, gen.OutletGetParams{TenantID: actor.TenantID, ID: id})
		if err != nil {
			return err
		}
		if !actor.Perms.All && !actor.Outlets[id] {
			return ErrNotAccessible
		}
		if cur.Active && !in.Active {
			if id == actor.OutletID {
				return ErrCurrentOutlet
			}
			if len(active) <= 1 {
				return ErrLastOutlet
			}
		}
		row, err = q.OutletUpdate(ctx, gen.OutletUpdateParams{TenantID: actor.TenantID, ID: id, Name: c.name, TaxStorePct: c.taxStore, TaxGovPct: c.taxGov, Timezone: c.tz, Active: in.Active,
			Address: c.receipt.address, Phone: c.receipt.phone, ReceiptHeader: c.receipt.header, ReceiptFooter: c.receipt.footer})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(actor), audit.Entry{
			Action: audit.ActionOutletUpdate, Entity: audit.EntityOutlet, EntityID: id.String(),
			Details: map[string]any{
				"before": map[string]any{"name": cur.Name, "timezone": cur.Timezone, "tax_store_pct": cur.TaxStorePct.String(), "tax_gov_pct": cur.TaxGovPct.String(), "active": cur.Active,
					"address": cur.Address, "phone": cur.Phone, "receipt_header": cur.ReceiptHeader, "receipt_footer": cur.ReceiptFooter},
				"after": map[string]any{"name": row.Name, "timezone": row.Timezone, "tax_store_pct": row.TaxStorePct.String(), "tax_gov_pct": row.TaxGovPct.String(), "active": row.Active,
					"address": row.Address, "phone": row.Phone, "receipt_header": row.ReceiptHeader, "receipt_footer": row.ReceiptFooter},
			},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.resolver.InvalidateTenant(actor.TenantID)
	o := outletOf(row)
	return &o, nil
}
