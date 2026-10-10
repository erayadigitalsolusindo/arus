// Package live = tampilan penjualan waktu nyata (SSE). Server hanya mengirim SINYAL "ada perubahan" (bukan data);
// klien lalu mengambil ringkasan terbaru. Hasilnya: payload kecil, tidak ada data sensitif di aliran, dan lonjakan
// transaksi otomatis menyatu (satu sinyal tertunda per klien).
package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const channel = "live:sales"

type message struct {
	Tenant uuid.UUID `json:"t"`
	Outlet uuid.UUID `json:"o"` // uuid.Nil = semua outlet tenant
}

type sub struct {
	tenant, outlet uuid.UUID
	ch             chan struct{} // kapasitas 1: sinyal yang belum dibaca menyatu dengan sinyal baru
}

// Hub meneruskan pesan Redis Pub/Sub ke klien SSE di proses ini. Satu langganan Redis untuk semua klien.
type Hub struct {
	rdb  *redis.Client
	log  *slog.Logger
	mu   sync.Mutex
	set  map[*sub]struct{}
	done chan struct{} // ditutup saat Run berakhir (shutdown): aliran SSE ikut berhenti
}

func NewHub(rdb *redis.Client, log *slog.Logger) *Hub {
	return &Hub{rdb: rdb, log: log, set: map[*sub]struct{}{}, done: make(chan struct{})}
}

// Notify memberi tahu semua dasbor bahwa penjualan outlet berubah. Dipanggil SETELAH commit; kegagalan hanya dicatat
// (dasbor tetap menyegarkan diri berkala). outlet = uuid.Nil → semua outlet tenant.
func (h *Hub) Notify(tenant, outlet uuid.UUID) {
	if h == nil || h.rdb == nil {
		return
	}
	b, _ := json.Marshal(message{Tenant: tenant, Outlet: outlet})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.rdb.Publish(ctx, channel, b).Err(); err != nil {
		h.log.Warn("live notify", "err", err)
	}
}

// Run berlangganan sampai ctx selesai; terputus → sambung ulang dengan jeda.
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for ctx.Err() == nil {
		ps := h.rdb.Subscribe(ctx, channel)
		if _, err := ps.Receive(ctx); err != nil {
			_ = ps.Close()
			if ctx.Err() == nil {
				h.log.Warn("live subscribe", "err", err)
				sleep(ctx, 3*time.Second)
			}
			continue
		}
		h.loop(ctx, ps)
		_ = ps.Close()
		// Pesan yang terlewat saat putus tidak diulang; klien mengambil ulang ringkasan tiap kali aliran dibuka.
		h.broadcast(message{})
	}
}

func (h *Hub) loop(ctx context.Context, ps *redis.PubSub) {
	ch := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			var msg message
			if json.Unmarshal([]byte(m.Payload), &msg) == nil {
				h.broadcast(msg)
			}
		}
	}
}

func (h *Hub) broadcast(m message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.set {
		// m kosong (tenant Nil) = sambungan Redis baru pulih: bangunkan semua.
		if m.Tenant != uuid.Nil && (s.tenant != m.Tenant || (m.Outlet != uuid.Nil && s.outlet != m.Outlet)) {
			continue
		}
		select {
		case s.ch <- struct{}{}:
		default: // sudah ada sinyal tertunda
		}
	}
}

// subscribe mengembalikan kanal sinyal dan fungsi berhenti.
func (h *Hub) subscribe(tenant, outlet uuid.UUID) (<-chan struct{}, func()) {
	s := &sub{tenant: tenant, outlet: outlet, ch: make(chan struct{}, 1)}
	h.mu.Lock()
	h.set[s] = struct{}{}
	h.mu.Unlock()
	return s.ch, func() {
		h.mu.Lock()
		delete(h.set, s)
		h.mu.Unlock()
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
