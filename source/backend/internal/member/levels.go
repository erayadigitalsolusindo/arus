package member

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const maxLevelPoints = 100_000_000

// Level = satu tingkat member beserta aturan poinnya.
type Level struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	MinPoints     int       `json:"min_points"`
	SpendPerPoint string    `json:"spend_per_point"`
	PointValue    string    `json:"point_value"`
	Active        bool      `json:"active"`
	MemberCount   int64     `json:"member_count"` // member aktif yang saat ini berada di level ini
	CreatedAt     time.Time `json:"created_at"`
}

type LevelInput struct {
	Name          string
	MinPoints     string
	SpendPerPoint string
	PointValue    string
}

// DefaultLevels = level bawaan tenant baru (aturan awal yang masuk akal; pemilik bebas mengubahnya).
var defaultLevels = []struct {
	name      string
	min       int32
	spend, pv int64
}{
	{"Reguler", 0, 10000, 100}, {"Silver", 100, 8000, 100}, {"Gold", 500, 6000, 100}, {"Platinum", 1000, 5000, 100},
}

// SeedDefaultLevels mengisi level bawaan; dipanggil Register di dalam transaksi pembuatan tenant.
func SeedDefaultLevels(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	q := gen.New(tx)
	for _, l := range defaultLevels {
		if _, err := q.MemberLevelCreate(ctx, gen.MemberLevelCreateParams{TenantID: tenant, Name: l.name, MinPoints: l.min,
			SpendPerPoint: decimal.NewFromInt(l.spend), PointValue: decimal.NewFromInt(l.pv)}); err != nil {
			return err
		}
	}
	return nil
}

func validateLevel(in LevelInput) (name string, min int32, spend, pv decimal.Decimal, f FieldErrors) {
	f = FieldErrors{}
	var code string
	if name, code = sanitize.Name(in.Name, 60); code != "" {
		f["name"] = code
	}
	n, err := strconv.Atoi(strings.TrimSpace(in.MinPoints))
	if err != nil || n < 0 || n > maxLevelPoints {
		f["min_points"] = sanitize.Invalid
	}
	min = int32(n)
	var ok bool
	if spend, ok = parseMoney(in.SpendPerPoint); !ok {
		f["spend_per_point"] = sanitize.Invalid
	}
	if pv, ok = parseMoney(in.PointValue); !ok {
		f["point_value"] = sanitize.Invalid
	}
	if len(f) == 0 {
		f = nil
	}
	return
}

func levelOf(id uuid.UUID, name string, min int32, spend, pv decimal.Decimal, active bool, created time.Time, count int64) Level {
	return Level{ID: id, Name: name, MinPoints: int(min), SpendPerPoint: spend.StringFixed(2), PointValue: pv.StringFixed(2),
		Active: active, MemberCount: count, CreatedAt: created}
}

func mapLevelErr(err error) error {
	switch {
	case uniqueViolation(err, "member_levels_name_key"):
		return FieldErrors{"name": "DUPLICATE"}
	case uniqueViolation(err, "member_levels_min_points_key"):
		return FieldErrors{"min_points": "DUPLICATE"}
	}
	return mapWriteErr(err)
}

// Levels mengembalikan semua level (urut ambang poin). active nil = semua.
func (s *Service) Levels(ctx context.Context, a authz.Actor, active *bool) ([]Level, error) {
	arg := gen.MemberLevelListParams{TenantID: a.TenantID}
	if active != nil {
		arg.Active.Bool, arg.Active.Valid = *active, true
	}
	out := []Level{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).MemberLevelList(ctx, arg)
		for _, r := range rows {
			out = append(out, levelOf(r.ID, r.Name, r.MinPoints, r.SpendPerPoint, r.PointValue, r.Active, r.CreatedAt.Time, r.MemberCount))
		}
		return err
	})
	return out, err
}

func (s *Service) CreateLevel(ctx context.Context, a authz.Actor, in LevelInput) (*Level, error) {
	name, min, spend, pv, f := validateLevel(in)
	if f != nil {
		return nil, f
	}
	var out Level
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).MemberLevelCreate(ctx, gen.MemberLevelCreateParams{TenantID: a.TenantID, Name: name, MinPoints: min, SpendPerPoint: spend, PointValue: pv})
		if err != nil {
			return err
		}
		out = levelOf(r.ID, r.Name, r.MinPoints, r.SpendPerPoint, r.PointValue, r.Active, r.CreatedAt.Time, 0)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberLevelCreate, Entity: audit.EntityMemberLevel, EntityID: r.ID.String(),
			Details: map[string]any{"name": name, "min_points": min, "spend_per_point": spend.StringFixed(2), "point_value": pv.StringFixed(2)}})
	})
	if err = mapLevelErr(err); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateLevel mengubah level. Level dasar (min_points = 0) tidak boleh dipindahkan ambangnya: kalau tidak, member
// dengan poin kecil tidak punya level sama sekali.
func (s *Service) UpdateLevel(ctx context.Context, a authz.Actor, id uuid.UUID, in LevelInput) (*Level, error) {
	name, min, spend, pv, f := validateLevel(in)
	if f != nil {
		return nil, f
	}
	var out Level
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberLevelGetForUpdate(ctx, gen.MemberLevelGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.MinPoints == 0 && min != 0 {
			return FieldErrors{"min_points": "BASE_LEVEL"}
		}
		r, err := q.MemberLevelUpdate(ctx, gen.MemberLevelUpdateParams{TenantID: a.TenantID, ID: id, Name: name, MinPoints: min, SpendPerPoint: spend, PointValue: pv})
		if err != nil {
			return err
		}
		out = levelOf(r.ID, r.Name, r.MinPoints, r.SpendPerPoint, r.PointValue, r.Active, r.CreatedAt.Time, 0)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberLevelUpdate, Entity: audit.EntityMemberLevel, EntityID: id.String(),
			Details: map[string]any{"name": name,
				"before": map[string]any{"name": cur.Name, "min_points": cur.MinPoints, "spend_per_point": cur.SpendPerPoint.StringFixed(2), "point_value": cur.PointValue.StringFixed(2)},
				"after":  map[string]any{"name": name, "min_points": min, "spend_per_point": spend.StringFixed(2), "point_value": pv.StringFixed(2)}}})
	})
	if err = mapLevelErr(err); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetLevelActive mengarsipkan/mengaktifkan level. Level dasar tidak boleh diarsipkan.
func (s *Service) SetLevelActive(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) (*Level, error) {
	var out Level
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberLevelGetForUpdate(ctx, gen.MemberLevelGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.MinPoints == 0 && !active {
			return FieldErrors{"active": "BASE_LEVEL"}
		}
		r, err := q.MemberLevelSetActive(ctx, gen.MemberLevelSetActiveParams{TenantID: a.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		out = levelOf(r.ID, r.Name, r.MinPoints, r.SpendPerPoint, r.PointValue, r.Active, r.CreatedAt.Time, 0)
		if cur.Active == active {
			return nil
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberLevelActive, Entity: audit.EntityMemberLevel, EntityID: id.String(),
			Details: map[string]any{"name": cur.Name, "active": active}})
	})
	if err = mapLevelErr(err); err != nil {
		return nil, err
	}
	return &out, nil
}
