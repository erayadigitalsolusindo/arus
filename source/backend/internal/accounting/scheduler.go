package accounting

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
)

// Penjadwal jurnal otomatis: tiap outlet, sekali per hari (setelah jam runHour waktu lokal outlet), membuat/menyegarkan draf jurnal
// untuk lookbackDays hari TERAKHIR YANG SUDAH LEWAT (hari berjalan tidak disentuh agar draf tidak diposting sebelum hari usai).
// Hanya membuat DRAF (posting tetap oleh akuntan); jurnal yang sudah diposting hanya menerima draf selisih. Aman dijalankan di beberapa
// instance: tiap (sumber, outlet, hari) dikunci advisory lock dan dijaga UNIQUE (source_type, source_ref) di DB.
const (
	schedulerInterval = 30 * time.Minute
	schedulerRunHour  = 1 // 01:00 waktu lokal outlet
	schedulerLookback = 7 // hari
)

type Scheduler struct {
	svc  *Service
	pool *pgxpool.Pool
	log  *slog.Logger
	now  func() time.Time

	// OnlyTenant != uuid.Nil membatasi penjadwal ke satu tenant (test dan alat operasi).
	OnlyTenant uuid.UUID

	mu   sync.Mutex
	done map[uuid.UUID]string // outlet -> tanggal lokal (YYYY-MM-DD) yang sudah dijalankan di proses ini
}

func NewScheduler(svc *Service, pool *pgxpool.Pool, log *slog.Logger) *Scheduler {
	return &Scheduler{svc: svc, pool: pool, log: log, now: time.Now, done: map[uuid.UUID]string{}}
}

// WithClock mengganti jam (test).
func (s *Scheduler) WithClock(now func() time.Time) *Scheduler {
	s.now = now
	return s
}

// Run berjalan sampai ctx selesai.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(schedulerInterval)
	defer t.Stop()
	s.log.Info("penjadwal jurnal otomatis aktif", "interval", schedulerInterval.String(), "run_hour", schedulerRunHour, "lookback_days", schedulerLookback)
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

type target struct {
	tenant, outlet uuid.UUID
	tz             string
}

// Tick memproses semua outlet yang sudah waktunya. Mengembalikan jumlah outlet yang dijalankan (untuk test).
func (s *Scheduler) Tick(ctx context.Context) int {
	rows, err := s.pool.Query(ctx, `SELECT tenant_id, outlet_id, timezone FROM auto_journal_targets()`)
	if err != nil {
		s.log.Warn("jurnal otomatis: gagal membaca daftar outlet", "err", err)
		return 0
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.tenant, &t.outlet, &t.tz); err != nil {
			rows.Close()
			s.log.Warn("jurnal otomatis: baris outlet tidak terbaca", "err", err)
			return 0
		}
		targets = append(targets, t)
	}
	rows.Close()
	if rows.Err() != nil {
		s.log.Warn("jurnal otomatis: gagal membaca daftar outlet", "err", rows.Err())
		return 0
	}

	ran := 0
	for _, t := range targets {
		if ctx.Err() != nil {
			return ran
		}
		if s.OnlyTenant != uuid.Nil && t.tenant != s.OnlyTenant {
			continue
		}
		loc, err := time.LoadLocation(t.tz)
		if err != nil {
			s.log.Warn("jurnal otomatis: zona waktu outlet tidak dikenal", "outlet", t.outlet, "tz", t.tz)
			continue
		}
		local := s.now().In(loc)
		today := local.Format(dateLayout)
		s.mu.Lock()
		already := s.done[t.outlet] == today
		s.mu.Unlock()
		if already || local.Hour() < schedulerRunHour {
			continue
		}
		if s.runOutlet(ctx, t, local) {
			s.mu.Lock()
			s.done[t.outlet] = today
			s.mu.Unlock()
		}
		ran++
	}
	return ran
}

// runOutlet menghasilkan draf untuk hari-hari lewat. false = gagal sementara (dicoba lagi pada tick berikutnya).
func (s *Scheduler) runOutlet(ctx context.Context, t target, local time.Time) bool {
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	actor := authz.Actor{TenantID: t.tenant, OutletID: t.outlet, Name: "Jurnal otomatis", Perms: authz.ParseStored([]byte(`{"*":true}`))}
	res, err := s.svc.GenerateDrafts(ctx, actor, GenerateInput{
		From:     today.AddDate(0, 0, -schedulerLookback).Format(dateLayout),
		To:       today.AddDate(0, 0, -1).Format(dateLayout),
		OutletID: t.outlet,
	})
	var missing *MissingMappingError
	switch {
	case errors.As(err, &missing):
		// Pemetaan akun belum lengkap: menunggu akuntan melengkapinya; coba lagi besok (bukan galat sistem).
		s.log.Info("jurnal otomatis dilewati: pemetaan akun belum lengkap", "tenant", t.tenant, "outlet", t.outlet, "detail", missing.Error())
		return true
	case errors.Is(err, ErrPeriodClosed):
		s.log.Info("jurnal otomatis dilewati: periode tertutup", "tenant", t.tenant, "outlet", t.outlet)
		return true
	case err != nil:
		s.log.Warn("jurnal otomatis gagal", "tenant", t.tenant, "outlet", t.outlet, "err", err)
		return false
	}
	if res.Created+res.Updated+res.Adjusted+res.Deleted > 0 {
		s.log.Info("jurnal otomatis dibuat", "tenant", t.tenant, "outlet", t.outlet, "created", res.Created, "updated", res.Updated,
			"adjusted", res.Adjusted, "deleted", res.Deleted)
	}
	return true
}
