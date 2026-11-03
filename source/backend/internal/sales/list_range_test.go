package sales

import (
	"testing"
	"time"
)

// Rentang daftar penjualan: tanggal saja = hari penuh; tanggal+jam:menit = menit akhir ikut; zona waktu outlet dipakai.
func TestListRangeBounds(t *testing.T) {
	today := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	wib := time.FixedZone("WIB", 7*3600)

	t0, t1, fe := listRange(today, "Asia/Jakarta", "", "")
	if len(fe) > 0 || !t0.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, wib)) || !t1.Equal(time.Date(2026, 11, 4, 0, 0, 0, 0, wib)) {
		t.Fatalf("default hari ini: %v %v %v", t0, t1, fe)
	}
	t0, t1, fe = listRange(today, "Asia/Jakarta", "2026-11-03T08:30", "2026-11-03T14:45")
	if len(fe) > 0 || !t0.Equal(time.Date(2026, 11, 3, 8, 30, 0, 0, wib)) || !t1.Equal(time.Date(2026, 11, 3, 14, 46, 0, 0, wib)) {
		t.Fatalf("jam menit: %v %v %v", t0, t1, fe)
	}
	if _, _, fe = listRange(today, "Asia/Jakarta", "2026-11-03T14:45", "2026-11-03T08:30"); fe["to"] == "" {
		t.Fatal("akhir sebelum awal harus ditolak")
	}
	if _, _, fe = listRange(today, "Asia/Jakarta", "2026-13-40", ""); fe["from"] == "" {
		t.Fatal("tanggal salah harus ditolak")
	}
	if _, _, fe = listRange(today, "Asia/Jakarta", "2026-01-01", "2026-12-31"); fe["to"] == "" {
		t.Fatal("rentang kepanjangan harus ditolak")
	}
	// Lebih dari sebulan (44 hari, dan batas 62 hari penuh) masih sah.
	if _, _, fe = listRange(today, "Asia/Jakarta", "2026-09-20T00:00", "2026-11-03T23:59"); len(fe) > 0 {
		t.Fatalf("44 hari harus sah: %v", fe)
	}
	if _, _, fe = listRange(today, "Asia/Jakarta", "2026-09-02", "2026-11-03"); len(fe) > 0 {
		t.Fatalf("63 hari kalender (batas) harus sah: %v", fe)
	}
	if got := echoBound("2026-11-03 08:30", today); got != "2026-11-03T08:30" {
		t.Fatalf("echo: %s", got)
	}
}
