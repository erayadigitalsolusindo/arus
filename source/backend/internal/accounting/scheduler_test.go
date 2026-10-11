package accounting_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

// Penjadwal membuat draf untuk hari yang sudah lewat saja, tanpa pembuat manusia (created_by kosong), sekali per hari lokal outlet.
func TestSchedulerNightlyDrafts(t *testing.T) {
	s := newSalesEnv(t)
	ctx := context.Background()
	cash := func(amount string) payRow { return payRow{s.cash, "cash", "Tunai", amount, "0", "store"} }
	s.sale(t, "2026-03-24 10:00+07", "completed", "10000", "0", "0", "0", "10000", "10000", "0", "0", "1", "6000", "", cash("10000"))
	s.sale(t, "2026-03-25 02:00+07", "completed", "5000", "0", "0", "0", "5000", "5000", "0", "0", "1", "3000", "", cash("5000")) // hari berjalan

	wib := time.FixedZone("WIB", 7*3600)
	at := func(h, m int) func() time.Time {
		return func() time.Time { return time.Date(2026, 3, 25, h, m, 0, 0, wib) }
	}
	sched := func(now func() time.Time) *accounting.Scheduler {
		sc := accounting.NewScheduler(s.svc, s.app, slog.New(slog.NewTextHandler(logSink(t), nil))).WithClock(now)
		sc.OnlyTenant = s.other // jangan menyentuh tenant lain di DB bersama
		return sc
	}

	// Sebelum jam 01:00 lokal: belum jalan.
	if n := sched(at(0, 30)).Tick(ctx); n != 0 {
		t.Fatalf("sebelum jam jalan: %d outlet diproses", n)
	}
	var cnt int
	count := func() int {
		if err := s.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND source_type = 'sales_daily'`, s.other).Scan(&cnt); err != nil {
			t.Fatal(err)
		}
		return cnt
	}
	if count() != 0 {
		t.Fatal("draf terbuat sebelum waktunya")
	}

	sc := sched(at(3, 0))
	if n := sc.Tick(ctx); n != 1 {
		t.Fatalf("tick pertama: %d outlet diproses", n)
	}
	if count() != 1 {
		t.Fatalf("mau 1 draf (hanya 24 Maret), dapat %d", cnt)
	}
	var nilCreator bool
	var date, status string
	if err := s.admin.QueryRow(ctx, `SELECT created_by IS NULL, entry_date::text, status FROM journal_entries WHERE tenant_id = $1 AND source_type = 'sales_daily'`, s.other).Scan(&nilCreator, &date, &status); err != nil {
		t.Fatal(err)
	}
	if !nilCreator || date != "2026-03-24" || status != "draft" {
		t.Fatalf("draf: pembuat kosong=%v tanggal=%s status=%s", nilCreator, date, status)
	}

	// Sekali per hari lokal; hari berikutnya jalan lagi.
	if n := sc.Tick(ctx); n != 0 {
		t.Fatalf("tick kedua hari yang sama: %d", n)
	}
	next := func() time.Time { return time.Date(2026, 3, 26, 2, 0, 0, 0, wib) }
	if n := sched(next).Tick(ctx); n != 1 {
		t.Fatalf("hari berikutnya: %d", n)
	}
	if count() != 2 { // 24 dan 25 Maret
		t.Fatalf("setelah 26 Maret mau 2 draf, dapat %d", cnt)
	}

	// Draf tanpa pembuat bisa dibaca dan diposting akuntan.
	var id string
	if err := s.admin.QueryRow(ctx, `SELECT id::text FROM journal_entries WHERE tenant_id = $1 AND source_type = 'sales_daily' AND entry_date = '2026-03-24'`, s.other).Scan(&id); err != nil {
		t.Fatal(err)
	}
	jid := uuid.MustParse(id)
	j, err := s.svc.Get(ctx, s.otherOwner, jid)
	if err != nil || j.CreatedBy != nil {
		t.Fatalf("baca draf: %v %+v", err, j.CreatedBy)
	}
	if _, err := s.svc.Post(ctx, s.otherOwner, jid); err != nil {
		t.Fatalf("posting draf otomatis: %v", err)
	}
}

type testLog struct{ t *testing.T }

func (l testLog) Write(p []byte) (int, error) { l.t.Log(string(p)); return len(p), nil }

func logSink(t *testing.T) io.Writer { return testLog{t} }
