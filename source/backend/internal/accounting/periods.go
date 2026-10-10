package accounting

import (
	"context"
	"errors"
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

type Period struct {
	ID       uuid.UUID  `json:"id"`
	Start    string     `json:"start_date"`
	End      string     `json:"end_date"`
	Status   string     `json:"status"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

type periodRow interface {
	gen.PeriodListRow | gen.PeriodLockShareRow | gen.PeriodLockUpdateRow
}

func periodOf[R periodRow](row R) Period {
	r := gen.PeriodListRow(row)
	p := Period{ID: r.ID, Start: r.StartDate.Time.Format(dateLayout), End: r.EndDate.Time.Format(dateLayout), Status: r.Status}
	if r.ClosedAt.Valid {
		t := r.ClosedAt.Time
		p.ClosedAt = &t
	}
	return p
}

func (s *Service) ListPeriods(ctx context.Context, a authz.Actor) ([]Period, error) {
	out := []Period{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).PeriodList(ctx, a.TenantID)
		for _, r := range rows {
			out = append(out, periodOf(r))
		}
		return err
	})
	return out, err
}

// parseMonth menerima "2006-01" atau tanggal penuh; hasilnya tanggal 1 bulan itu.
func parseMonth(s string) (time.Time, bool) {
	if t, err := time.Parse("2006-01", s); err == nil {
		return t, true
	}
	if t, ok := parseDate(s); ok {
		return monthStart(t), true
	}
	return time.Time{}, false
}

// lockPeriodShare memastikan periode bulan itu ada lalu menahannya FOR SHARE (tutup buku menunggu); tertutup → ErrPeriodClosed.
func lockPeriodShare(ctx context.Context, q *gen.Queries, tenant uuid.UUID, month time.Time) error {
	if err := q.PeriodEnsure(ctx, gen.PeriodEnsureParams{TenantID: tenant, StartDate: pgDate(month)}); err != nil {
		return err
	}
	p, err := q.PeriodLockShare(ctx, gen.PeriodLockShareParams{TenantID: tenant, StartDate: pgDate(month)})
	if err != nil {
		return err
	}
	if p.Status == "closed" {
		return ErrPeriodClosed
	}
	return nil
}

// ClosePeriod = tutup buku satu bulan: tidak ada lagi posting/jurnal balik bertanggal di bulan itu. Ditolak bila masih ada draf.
func (s *Service) ClosePeriod(ctx context.Context, a authz.Actor, month string) (Period, error) {
	m, ok := parseMonth(month)
	if !ok {
		return Period{}, FieldErrors{"month": sanitize.Invalid}
	}
	var out Period
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := q.PeriodEnsure(ctx, gen.PeriodEnsureParams{TenantID: a.TenantID, StartDate: pgDate(m)}); err != nil {
			return err
		}
		p, err := q.PeriodLockUpdate(ctx, gen.PeriodLockUpdateParams{TenantID: a.TenantID, StartDate: pgDate(m)})
		if err != nil {
			return err
		}
		if p.Status == "closed" {
			return ErrPeriodClosed
		}
		// Kunci FOR UPDATE sudah didapat: posting yang menahan FOR SHARE selesai lebih dulu, yang baru akan ditolak.
		drafts, err := q.PeriodDraftCount(ctx, gen.PeriodDraftCountParams{TenantID: a.TenantID, EntryDate: p.StartDate, EntryDate_2: p.EndDate})
		if err != nil {
			return err
		}
		if drafts > 0 {
			return ErrDraftsInPeriod
		}
		if err := q.PeriodSetStatus(ctx, gen.PeriodSetStatusParams{TenantID: a.TenantID, ID: p.ID, Status: "closed",
			ClosedAt: pgtype.Timestamptz{Time: s.now(), Valid: true}, ClosedBy: pgUUID(a.UserID)}); err != nil {
			return err
		}
		p.Status = "closed"
		p.ClosedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
		out = periodOf(p)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionPeriodClose, Entity: audit.EntityAccountingPeriod,
			EntityID: p.ID.String(), Details: map[string]any{"period": m.Format("2006-01")}})
	})
	return out, err
}

// ReopenPeriod membuka kembali periode tertutup; alasan wajib dan tercatat di audit. Pemanggil (handler) wajib memeriksa izin khusus.
func (s *Service) ReopenPeriod(ctx context.Context, a authz.Actor, month, reason string) (Period, error) {
	m, ok := parseMonth(month)
	if !ok {
		return Period{}, FieldErrors{"month": sanitize.Invalid}
	}
	reason, e := cleanText(reason, 200, true)
	if e == "" && len([]rune(reason)) < 3 {
		e = sanitize.TooShort
	}
	if e != "" {
		return Period{}, FieldErrors{"reason": e}
	}
	var out Period
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		p, err := q.PeriodLockUpdate(ctx, gen.PeriodLockUpdateParams{TenantID: a.TenantID, StartDate: pgDate(m)})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Status != "closed" {
			return ErrPeriodNotClosed
		}
		if err := q.PeriodSetStatus(ctx, gen.PeriodSetStatusParams{TenantID: a.TenantID, ID: p.ID, Status: "open"}); err != nil {
			return err
		}
		p.Status, p.ClosedAt = "open", pgtype.Timestamptz{}
		out = periodOf(p)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionPeriodReopen, Entity: audit.EntityAccountingPeriod,
			EntityID: p.ID.String(), Details: map[string]any{"period": m.Format("2006-01"), "reason": reason}})
	})
	return out, err
}
