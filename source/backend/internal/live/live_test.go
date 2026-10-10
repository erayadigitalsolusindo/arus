package live

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/authz"
)

func testHub(t *testing.T) *Hub {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL tidak di-set")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })
	h := NewHub(rdb, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(cancel)
	// Tunggu langganan Redis aktif.
	for i := 0; i < 50; i++ {
		if n, _ := rdb.PubSubNumSub(ctx, channel).Result(); n[channel] > 0 {
			return h
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("langganan tidak aktif")
	return nil
}

func recv(ch <-chan struct{}, wait time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(wait):
		return false
	}
}

// Sinyal hanya sampai ke tenant/outlet yang tepat; sinyal beruntun menyatu.
func TestHubRoutesAndCoalesces(t *testing.T) {
	h := testHub(t)
	tA, tB, o1, o2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	a1, stopA1 := h.subscribe(tA, o1)
	defer stopA1()
	a2, stopA2 := h.subscribe(tA, o2)
	defer stopA2()
	b1, stopB := h.subscribe(tB, o1)
	defer stopB()

	h.Notify(tA, o1)
	if !recv(a1, time.Second) {
		t.Fatal("outlet sama harus menerima")
	}
	if recv(a2, 150*time.Millisecond) || recv(b1, 50*time.Millisecond) {
		t.Fatal("outlet/tenant lain tidak boleh menerima")
	}
	// Tenant-wide (outlet Nil) → semua outlet tenant A, bukan tenant B.
	h.Notify(tA, uuid.Nil)
	if !recv(a1, time.Second) || !recv(a2, time.Second) || recv(b1, 100*time.Millisecond) {
		t.Fatal("sinyal tenant-wide")
	}
	// 20 sinyal beruntun tanpa dibaca → tepat satu tertunda.
	for i := 0; i < 20; i++ {
		h.Notify(tA, o1)
	}
	time.Sleep(200 * time.Millisecond)
	if !recv(a1, 100*time.Millisecond) || recv(a1, 100*time.Millisecond) {
		t.Fatal("sinyal harus menyatu menjadi satu")
	}
}

// SSE: header benar, event ready, event sales setelah Notify, berhenti saat klien putus.
func TestStreamDeliversSignal(t *testing.T) {
	h := testHub(t)
	tid, oid := uuid.New(), uuid.New()
	hd := &Handler{hub: h, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd.Stream(w, r.WithContext(authz.WithActor(r.Context(), authz.Actor{TenantID: tid, OutletID: oid})))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	want := func(sub string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("aliran berakhir sebelum %q", sub)
				}
				if strings.Contains(l, sub) {
					return
				}
			case <-deadline:
				t.Fatalf("tidak menerima %q", sub)
			}
		}
	}
	want("event: ready")
	h.Notify(tid, oid)
	want("event: sales")
	cancel()
	_ = res.Body.Close()
}
