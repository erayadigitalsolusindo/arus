// Package member: master member/pelanggan, level member, dan poin (Fase 3.3).
//
// Aturan poin (diputuskan 2026-10-08):
//   - Level ditentukan OTOMATIS dari lifetime_points (total poin yang pernah diperoleh): level aktif dengan min_points
//     terbesar yang ≤ lifetime. Level min_points = 0 ("level dasar") selalu ada.
//   - Poin diperoleh per nota = floor((subtotal − potongan) ÷ spend_per_point level member); 0 bila spend_per_point = 0.
//   - Poin ditukar = potongan nota sebesar poin × point_value level member.
//   - Poin = ledger append-only (member_point_movements); saldo diubah dalam transaksi yang sama dengan ledger.
//     Edit/void nota memakai pembalikan (ReverseSale), tidak menambah ulang (bug legacy).
package member

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	"aciraba/internal/platform/storage"
)

var (
	ErrNotFound   = errors.New("member tidak ditemukan")
	ErrCodeTaken  = errors.New("kode member sudah dipakai")
	ErrNoStorage  = errors.New("penyimpanan gambar tidak dikonfigurasi")
	codeRe        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,39}$`)
	postalRe      = regexp.MustCompile(`^[A-Za-z0-9 -]{0,10}$`)
	maxMoney      = decimal.New(1, 12)
	defLimit      = 20
	maxLimit      = 100
	maxDueDays    = 3650
	maxNotes      = 1000
	maxAddress    = 300
	maxPlaceField = 100
)

// FieldErrors = kode galat per field (REQUIRED/INVALID/TOO_LONG/DUPLICATE), diterjemahkan klien.
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type Service struct {
	pool  *pgxpool.Pool
	store storage.Store // nil = fitur foto cover mati (ErrNoStorage)
}

func NewService(pool *pgxpool.Pool, store storage.Store) *Service {
	return &Service{pool: pool, store: store}
}

// ---- tipe respons ----

type LevelRef struct {
	ID            *uuid.UUID `json:"id"`
	Name          string     `json:"name"`
	MinPoints     int        `json:"min_points"`
	SpendPerPoint string     `json:"spend_per_point"`
	PointValue    string     `json:"point_value"`
}

type NextLevel struct {
	Name         string `json:"name"`
	MinPoints    int    `json:"min_points"`
	PointsNeeded int    `json:"points_needed"`
}

type Stats struct {
	TotalSales string `json:"total_sales"`
	TotalTrx   int64  `json:"total_trx"`
	Points     int    `json:"points"`
	Deposit    string `json:"deposit"` // belum ada fitur deposit (menyusul bersama piutang/kas); selalu 0
}

type Member struct {
	ID             uuid.UUID  `json:"id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Gender         string     `json:"gender"`
	Phone          string     `json:"phone"`
	Email          string     `json:"email"`
	Address        string     `json:"address"`
	District       string     `json:"district"`
	City           string     `json:"city"`
	Province       string     `json:"province"`
	PostalCode     string     `json:"postal_code"`
	CreditLimit    string     `json:"credit_limit"`
	DueDays        int        `json:"due_days"`
	ValidUntil     *string    `json:"valid_until"` // YYYY-MM-DD; null = selalu aktif
	Active         bool       `json:"active"`
	Notes          string     `json:"notes"`
	CoverImageID   *uuid.UUID `json:"cover_image_id"`
	Points         int        `json:"points"`
	LifetimePoints int        `json:"lifetime_points"`
	Level          LevelRef   `json:"level"`
	NextLevel      *NextLevel `json:"next_level"`
	Stats          Stats      `json:"stats"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Row struct {
	ID             uuid.UUID  `json:"id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Phone          string     `json:"phone"`
	City           string     `json:"city"`
	Active         bool       `json:"active"`
	ValidUntil     *string    `json:"valid_until"`
	Points         int        `json:"points"`
	LifetimePoints int        `json:"lifetime_points"`
	Level          string     `json:"level"`
	CoverImageID   *uuid.UUID `json:"cover_image_id"`
}

// Input untuk membuat/mengubah member (semua string mentah dari klien).
type Input struct {
	Code        string
	Name        string
	Gender      string
	Phone       string
	Email       string
	Address     string
	District    string
	City        string
	Province    string
	PostalCode  string
	CreditLimit string
	DueDays     string
	ValidUntil  string
	Notes       string
	Active      bool
}

type ListParams struct {
	Q      string
	Active *bool
	Limit  int
	Offset int
}

type clean struct {
	code, name, gender, phone, email, address, district, city, province, postal, notes string
	credit                                                                             decimal.Decimal
	dueDays                                                                            int32
	validUntil                                                                         pgtype.Date
	active                                                                             bool
}

// ---- validasi ----

func parseMoney(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, true
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || d.GreaterThanOrEqual(maxMoney) || d.Exponent() < -2 {
		return decimal.Zero, false
	}
	return d, true
}

// plain memvalidasi teks polos opsional (kosong boleh), tanpa markup, ≤ max karakter.
func plain(raw string, max int) (string, string) {
	v, ok := sanitize.Text(raw)
	switch {
	case !ok, strings.ContainsAny(v, "<>"):
		return "", sanitize.Invalid
	case utf8.RuneCountInString(v) > max:
		return "", sanitize.TooLong
	}
	return v, ""
}

func validate(in Input, creating bool) (clean, FieldErrors) {
	f := FieldErrors{}
	c := clean{active: in.Active}
	var code string
	if c.name, code = sanitize.Name(in.Name, 200); code != "" {
		f["name"] = code
	}
	if v := strings.TrimSpace(in.Code); v == "" {
		if !creating {
			f["code"] = sanitize.Required
		}
	} else if !codeRe.MatchString(v) {
		f["code"] = sanitize.Invalid
	} else {
		c.code = v
	}
	switch g := strings.TrimSpace(in.Gender); g {
	case "", "M", "F":
		c.gender = g
	default:
		f["gender"] = sanitize.Invalid
	}
	if strings.TrimSpace(in.Phone) != "" {
		if c.phone, code = sanitize.Phone(in.Phone); code != "" {
			f["phone"] = code
		}
	}
	if strings.TrimSpace(in.Email) != "" {
		if c.email, code = sanitize.Email(in.Email); code != "" {
			f["email"] = code
		}
	}
	for field, p := range map[string]struct {
		raw string
		max int
		dst *string
	}{
		"address": {in.Address, maxAddress, &c.address}, "district": {in.District, maxPlaceField, &c.district},
		"city": {in.City, maxPlaceField, &c.city}, "province": {in.Province, maxPlaceField, &c.province},
	} {
		v, code := plain(p.raw, p.max)
		if code != "" {
			f[field] = code
			continue
		}
		*p.dst = v
	}
	if v := strings.TrimSpace(in.PostalCode); !postalRe.MatchString(v) {
		f["postal_code"] = sanitize.Invalid
	} else {
		c.postal = v
	}
	var ok bool
	if c.credit, ok = parseMoney(in.CreditLimit); !ok {
		f["credit_limit"] = sanitize.Invalid
	}
	if v := strings.TrimSpace(in.DueDays); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > maxDueDays {
			f["due_days"] = sanitize.Invalid
		} else {
			c.dueDays = int32(n)
		}
	}
	if v := strings.TrimSpace(in.ValidUntil); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil || t.Year() < 2000 || t.Year() > 2200 {
			f["valid_until"] = sanitize.Invalid
		} else {
			c.validUntil = pgtype.Date{Time: t, Valid: true}
		}
	}
	if c.notes, code = sanitize.Multiline(in.Notes, maxNotes); code != "" {
		f["notes"] = code
	}
	if len(f) > 0 {
		return clean{}, f
	}
	return c, nil
}

// ---- konversi ----

func uuidPtr(p pgtype.UUID) *uuid.UUID {
	if !p.Valid {
		return nil
	}
	id := uuid.UUID(p.Bytes)
	return &id
}

// nilUUID: kolom hasil LEFT JOIN LATERAL dipetakan sqlc ke uuid biasa; Nil = tidak ada baris.
func nilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func memberOf(r gen.MemberGetRow) Member {
	m := Member{
		ID: r.ID, Code: r.Code, Name: r.Name, Gender: r.Gender, Phone: r.Phone, Email: r.Email, Address: r.Address, District: r.District,
		City: r.City, Province: r.Province, PostalCode: r.PostalCode, CreditLimit: r.CreditLimit.StringFixed(2), DueDays: int(r.DueDays),
		ValidUntil: datePtr(r.ValidUntil), Active: r.Active, Notes: r.Notes, CoverImageID: uuidPtr(r.CoverImageID),
		Points: int(r.Points), LifetimePoints: int(r.LifetimePoints), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		Level: LevelRef{ID: nilUUID(r.LevelID), Name: r.LevelName, MinPoints: int(r.LevelMinPoints),
			SpendPerPoint: r.LevelSpendPerPoint.StringFixed(2), PointValue: r.LevelPointValue.StringFixed(2)},
		Stats: Stats{TotalSales: r.TotalSales.StringFixed(2), TotalTrx: r.TotalTrx, Points: int(r.Points), Deposit: "0.00"},
	}
	if r.NextLevelID != uuid.Nil {
		m.NextLevel = &NextLevel{Name: r.NextLevelName, MinPoints: int(r.NextLevelMinPoints),
			PointsNeeded: int(r.NextLevelMinPoints) - int(r.LifetimePoints)}
	}
	return m
}

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func mapWriteErr(err error) error {
	switch {
	case uniqueViolation(err, "members_code_key"):
		return ErrCodeTaken
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(s)
}

// ---- baca ----

func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) ([]Row, int, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || utf8.RuneCountInString(q) > 200 {
		return nil, 0, FieldErrors{"q": sanitize.Invalid}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	arg := gen.MemberListParams{TenantID: a.TenantID, Q: likeEscape(q), PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(p.Offset, 0))}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Row{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).MemberList(ctx, arg)
		for _, r := range rows {
			out = append(out, Row{ID: r.ID, Code: r.Code, Name: r.Name, Phone: r.Phone, City: r.City, Active: r.Active,
				ValidUntil: datePtr(r.ValidUntil), Points: int(r.Points), LifetimePoints: int(r.LifetimePoints), Level: r.LevelName,
				CoverImageID: uuidPtr(r.CoverImageID)})
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (*Member, error) {
	var m Member
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &m, nil
}

func load(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Member, error) {
	r, err := gen.New(tx).MemberGet(ctx, gen.MemberGetParams{TenantID: tenant, ID: id})
	if err != nil {
		return Member{}, err
	}
	return memberOf(r), nil
}

// LookupRow = hasil pencarian kasir.
type LookupRow struct {
	ID             uuid.UUID  `json:"id"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Phone          string     `json:"phone"`
	Points         int        `json:"points"`
	LifetimePoints int        `json:"lifetime_points"`
	Level          string     `json:"level"`
	CoverImageID   *uuid.UUID `json:"cover_image_id"`
	SpendPerPoint  string     `json:"spend_per_point"`
	PointValue     string     `json:"point_value"`
}

// Lookup mencari member yang boleh dipilih di kasir (aktif dan belum kedaluwarsa menurut hari di outlet aktif).
func (s *Service) Lookup(ctx context.Context, a authz.Actor, rawQ string) ([]LookupRow, error) {
	q, ok := sanitize.Text(rawQ)
	if !ok || utf8.RuneCountInString(q) > 200 {
		return nil, FieldErrors{"q": sanitize.Invalid}
	}
	out := []LookupRow{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		qr := gen.New(tx)
		day, err := qr.MemberLocalDay(ctx, gen.MemberLocalDayParams{TenantID: a.TenantID, ID: a.OutletID})
		if err != nil {
			return err
		}
		rows, err := qr.MemberLookup(ctx, gen.MemberLookupParams{TenantID: a.TenantID, Q: likeEscape(q), LocalDay: day})
		for _, r := range rows {
			out = append(out, LookupRow{ID: r.ID, Code: r.Code, Name: r.Name, Phone: r.Phone, Points: int(r.Points),
				LifetimePoints: int(r.LifetimePoints), Level: r.LevelName, CoverImageID: uuidPtr(r.CoverImageID), SpendPerPoint: r.SpendPerPoint.StringFixed(2), PointValue: r.PointValue.StringFixed(2)})
		}
		return err
	})
	return out, err
}

// ---- tulis ----

// nextCode membuat kode otomatis MBR-000001, …; nomor yang sudah dipakai kode manual dilewati.
func nextCode(ctx context.Context, q *gen.Queries, tenant uuid.UUID) (string, error) {
	for range 100 {
		n, err := q.MemberNextNo(ctx, tenant)
		if err != nil {
			return "", err
		}
		code := fmt.Sprintf("MBR-%06d", n)
		taken, err := q.MemberCodeExists(ctx, gen.MemberCodeExistsParams{TenantID: tenant, Lower: code})
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", errors.New("gagal membuat kode member otomatis")
}

func (s *Service) Create(ctx context.Context, a authz.Actor, in Input) (*Member, error) {
	c, f := validate(in, true)
	if f != nil {
		return nil, f
	}
	var m Member
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		code := c.code
		if code == "" {
			var err error
			if code, err = nextCode(ctx, q, a.TenantID); err != nil {
				return err
			}
		}
		id, err := q.MemberCreate(ctx, gen.MemberCreateParams{TenantID: a.TenantID, Code: code, Name: c.name, Gender: c.gender, Phone: c.phone,
			Email: c.email, Address: c.address, District: c.district, City: c.city, Province: c.province, PostalCode: c.postal,
			CreditLimit: c.credit, DueDays: c.dueDays, ValidUntil: c.validUntil, Notes: c.notes, Active: c.active})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberCreate, Entity: audit.EntityMember, EntityID: id.String(),
			Details: map[string]any{"code": code, "name": c.name}}); err != nil {
			return err
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) Update(ctx context.Context, a authz.Actor, id uuid.UUID, in Input) (*Member, error) {
	c, f := validate(in, false)
	if f != nil {
		return nil, f
	}
	var m Member
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if err := q.MemberUpdate(ctx, gen.MemberUpdateParams{TenantID: a.TenantID, ID: id, Code: c.code, Name: c.name, Gender: c.gender, Phone: c.phone,
			Email: c.email, Address: c.address, District: c.district, City: c.city, Province: c.province, PostalCode: c.postal,
			CreditLimit: c.credit, DueDays: c.dueDays, ValidUntil: c.validUntil, Notes: c.notes, Active: c.active}); err != nil {
			return err
		}
		before, after := map[string]any{}, map[string]any{}
		diff := func(k string, b, n any) {
			if fmt.Sprint(b) != fmt.Sprint(n) {
				before[k], after[k] = b, n
			}
		}
		ds := func(d pgtype.Date) string {
			if p := datePtr(d); p != nil {
				return *p
			}
			return ""
		}
		diff("code", cur.Code, c.code)
		diff("name", cur.Name, c.name)
		diff("gender", cur.Gender, c.gender)
		diff("phone", cur.Phone, c.phone)
		diff("email", cur.Email, c.email)
		diff("address", cur.Address, c.address)
		diff("district", cur.District, c.district)
		diff("city", cur.City, c.city)
		diff("province", cur.Province, c.province)
		diff("postal_code", cur.PostalCode, c.postal)
		diff("credit_limit", cur.CreditLimit.StringFixed(2), c.credit.StringFixed(2))
		diff("due_days", cur.DueDays, c.dueDays)
		diff("valid_until", ds(cur.ValidUntil), ds(c.validUntil))
		diff("active", cur.Active, c.active)
		diff("notes", cur.Notes, c.notes)
		if len(before) > 0 {
			act := audit.ActionMemberUpdate
			if len(before) == 1 && before["active"] != nil {
				act = audit.ActionMemberActive
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: act, Entity: audit.EntityMember, EntityID: id.String(),
				Details: map[string]any{"code": c.code, "name": c.name, "before": before, "after": after}}); err != nil {
				return err
			}
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &m, nil
}

// SetActive mengaktifkan/menonaktifkan (arsip) member. Member tidak pernah dihapus permanen.
func (s *Service) SetActive(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) (*Member, error) {
	var m Member
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.Active != active {
			if err := q.MemberSetActive(ctx, gen.MemberSetActiveParams{TenantID: a.TenantID, ID: id, Active: active}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberActive, Entity: audit.EntityMember, EntityID: id.String(),
				Details: map[string]any{"code": cur.Code, "name": cur.Name, "active": active}}); err != nil {
				return err
			}
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &m, nil
}
