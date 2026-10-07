package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// Filter semua opsional. ActionPrefix mencocokkan awalan (mis. "role." atau "auth.login").
type Filter struct {
	Entity       string
	EntityID     string
	ActionPrefix string
	ActorID      uuid.UUID
	From, To     time.Time
	Cursor       string
	Limit        int
}

type Item struct {
	ID        int64          `json:"id"`
	OutletID  *uuid.UUID     `json:"outlet_id"`
	ActorID   *uuid.UUID     `json:"actor_id"`
	ActorName string         `json:"actor_name"`
	Action    string         `json:"action"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entity_id"`
	Details   map[string]any `json:"details"`
	IP        string         `json:"ip"`
	RequestID string         `json:"request_id"`
	CreatedAt time.Time      `json:"created_at"`
}

type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// ErrBadCursor: kursor rusak atau dimodifikasi klien.
var ErrBadCursor = fmt.Errorf("kursor tidak valid")

func (s *Service) List(ctx context.Context, tenantID uuid.UUID, f Filter) (*Page, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)

	p := gen.AuditListParams{TenantID: tenantID, MaxRows: int32(limit + 1)} // +1 untuk mengetahui ada halaman berikutnya
	p.Entity = text(f.Entity)
	p.EntityID = text(f.EntityID)
	if f.ActionPrefix != "" {
		p.ActionPrefix = text(escapeLike(f.ActionPrefix))
	}
	if f.ActorID != uuid.Nil {
		p.ActorID = pgtype.UUID{Bytes: f.ActorID, Valid: true}
	}
	if !f.From.IsZero() {
		p.FromAt = pgtype.Timestamptz{Time: f.From, Valid: true}
	}
	if !f.To.IsZero() {
		p.ToAt = pgtype.Timestamptz{Time: f.To, Valid: true}
	}
	if f.Cursor != "" {
		at, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, err
		}
		p.CursorAt = pgtype.Timestamptz{Time: at, Valid: true}
		p.CursorID = pgtype.Int8{Int64: id, Valid: true}
	}

	var rows []gen.AuditListRow
	err := db.WithTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var qerr error
		rows, qerr = gen.New(tx).AuditList(ctx, p)
		return qerr
	})
	if err != nil {
		return nil, err
	}

	page := &Page{Items: make([]Item, 0, min(len(rows), limit))}
	for i, r := range rows {
		if i == limit {
			last := page.Items[limit-1]
			page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
			break
		}
		page.Items = append(page.Items, itemOf(r))
	}
	return page, nil
}

func itemOf(r gen.AuditListRow) Item {
	it := Item{ID: r.ID, ActorName: r.ActorName, Action: r.Action, Entity: r.Entity, EntityID: r.EntityID, IP: r.Ip, RequestID: r.RequestID, CreatedAt: r.CreatedAt.Time}
	if r.OutletID.Valid {
		id := uuid.UUID(r.OutletID.Bytes)
		it.OutletID = &id
	}
	if r.ActorID.Valid {
		id := uuid.UUID(r.ActorID.Bytes)
		it.ActorID = &id
	}
	it.Details = map[string]any{}
	_ = json.Unmarshal(r.Details, &it.Details)
	return it
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// escapeLike menetralkan karakter khusus LIKE agar awalan dicocokkan apa adanya.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func encodeCursor(at time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + ":" + strconv.FormatInt(id, 10)))
}

func decodeCursor(c string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, 0, ErrBadCursor
	}
	micro, idStr, ok := strings.Cut(string(raw), ":")
	if !ok {
		return time.Time{}, 0, ErrBadCursor
	}
	us, err1 := strconv.ParseInt(micro, 10, 64)
	id, err2 := strconv.ParseInt(idStr, 10, 64)
	if err1 != nil || err2 != nil {
		return time.Time{}, 0, ErrBadCursor
	}
	return time.UnixMicro(us).UTC(), id, nil
}
