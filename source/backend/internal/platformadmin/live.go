package platformadmin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/live"
	"aciraba/internal/platform/httpx"
)

const (
	livePage      = 100
	liveHeartbeat = 20 * time.Second
	liveMaxStream = 10 * time.Minute
	liveActiveWin = 15 * time.Minute // "sedang bertransaksi" = ada nota dalam rentang ini
)

type LiveSale struct {
	ID         uuid.UUID `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	DocNo      string    `json:"doc_no"`
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	OutletName string    `json:"outlet_name"`
	Cashier    string    `json:"cashier"`
	LineCount  int       `json:"line_count"`
	Total      string    `json:"total"`
}

type LiveFeed struct {
	Items []LiveSale `json:"items"`
	// Cursor halaman berikutnya (keyset); kosong = sudah habis untuk hari ini.
	NextAt *time.Time `json:"next_at,omitempty"`
	NextID *uuid.UUID `json:"next_id,omitempty"`
}

type LivePerson struct {
	TenantID   uuid.UUID  `json:"tenant_id"`
	TenantName string     `json:"tenant_name"`
	UserID     uuid.UUID  `json:"user_id"`
	UserName   string     `json:"user_name"`
	SeenAt     time.Time  `json:"seen_at"`
	LastSaleAt *time.Time `json:"last_sale_at"`
	Selling    bool       `json:"selling"`
}

type LiveSummary struct {
	Since       time.Time    `json:"since"`
	SalesCount  int64        `json:"sales_count"`
	SalesTotal  string       `json:"sales_total"`
	TenantCount int64        `json:"tenant_count"`
	Online      []LivePerson `json:"online"`
}

// LiveFeed memuat satu halaman nota hari ini lintas tenant, terbaru dulu. before = cursor (nil = halaman pertama).
func (s *Service) LiveFeed(ctx context.Context, a Actor, beforeAt *time.Time, beforeID *uuid.UUID) (LiveFeed, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, created_at, doc_no, tenant_id, tenant_name, outlet_name, cashier, line_count, total
		FROM platform_live_feed($1, $2, $3, $4)`, a.ID, beforeAt, beforeID, livePage)
	if err != nil {
		return LiveFeed{}, err
	}
	defer rows.Close()
	out := LiveFeed{Items: []LiveSale{}}
	for rows.Next() {
		var x LiveSale
		var tot decimal.Decimal
		if err := rows.Scan(&x.ID, &x.CreatedAt, &x.DocNo, &x.TenantID, &x.TenantName, &x.OutletName, &x.Cashier, &x.LineCount, &tot); err != nil {
			return LiveFeed{}, err
		}
		x.Total = tot.StringFixed(2)
		out.Items = append(out.Items, x)
	}
	if err := rows.Err(); err != nil {
		return LiveFeed{}, err
	}
	if n := len(out.Items); n == livePage {
		l := out.Items[n-1]
		out.NextAt, out.NextID = &l.CreatedAt, &l.ID
	}
	return out, nil
}

// LiveSummary = omzet & jumlah nota hari ini + pengguna online (presence Redis, nama dari DB).
func (s *Service) LiveSummary(ctx context.Context, a Actor, pr *live.Presence) (LiveSummary, error) {
	var out LiveSummary
	var tot decimal.Decimal
	if err := s.Pool.QueryRow(ctx, `SELECT sales_count, sales_total, tenant_count, since FROM platform_live_today($1)`, a.ID).
		Scan(&out.SalesCount, &tot, &out.TenantCount, &out.Since); err != nil {
		return out, err
	}
	out.SalesTotal = tot.StringFixed(2)
	out.Online = []LivePerson{}
	if pr == nil {
		return out, nil
	}
	on, err := pr.Online(ctx)
	if err != nil || len(on) == 0 {
		return out, nil // Redis gagal: ringkasan tetap tampil tanpa daftar online
	}
	seen := make(map[[2]uuid.UUID]time.Time, len(on))
	ts, us := make([]uuid.UUID, len(on)), make([]uuid.UUID, len(on))
	for i, o := range on {
		ts[i], us[i], seen[[2]uuid.UUID{o.Tenant, o.User}] = o.Tenant, o.User, o.Seen
	}
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, tenant_name, user_id, user_name, last_sale_at FROM platform_live_people($1, $2, $3)`, a.ID, ts, us)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var p LivePerson
		if err := rows.Scan(&p.TenantID, &p.TenantName, &p.UserID, &p.UserName, &p.LastSaleAt); err != nil {
			return out, err
		}
		p.SeenAt = seen[[2]uuid.UUID{p.TenantID, p.UserID}]
		p.Selling = p.LastSaleAt != nil && time.Since(*p.LastSaleAt) < liveActiveWin
		out.Online = append(out.Online, p)
	}
	return out, rows.Err()
}

func (h *Handler) LiveSummary(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	res, err := h.svc.LiveSummary(r.Context(), a, h.Presence)
	if err != nil {
		h.Log.Error("platform live summary", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) LiveFeed(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	var at *time.Time
	var id *uuid.UUID
	if v := r.URL.Query().Get("before_at"); v != "" {
		t, err1 := time.Parse(time.RFC3339Nano, v)
		u, err2 := uuid.Parse(r.URL.Query().Get("before_id"))
		if err1 != nil || err2 != nil {
			httpx.Error(w, http.StatusBadRequest, "VALIDATION", "Cursor tidak valid.")
			return
		}
		at, id = &t, &u
	}
	res, err := h.svc.LiveFeed(r.Context(), a, at, id)
	if err != nil {
		h.Log.Error("platform live feed", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// LiveStream = SSE sinyal (event `sales` = ada nota baru di tenant mana pun); klien mengambil ulang. Pola sama dengan live.Handler.Stream.
func (h *Handler) LiveStream(w http.ResponseWriter, r *http.Request) {
	if h.Live == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Fitur tidak aktif.")
		return
	}
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Aliran tidak didukung.")
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache, no-transform")
	hd.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sig, stop, done := h.Live.SubscribeAll()
	defer stop()
	write := func(s string) bool {
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: 3000\nevent: ready\ndata: {}\n\n") {
		return
	}
	hb, end := time.NewTicker(liveHeartbeat), time.NewTimer(liveMaxStream)
	defer hb.Stop()
	defer end.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-end.C:
			return
		case <-done:
			return
		case <-sig:
			if !write("event: sales\ndata: {}\n\n") {
				return
			}
		case <-hb.C:
			if !write(": ping\n\n") {
				return
			}
		}
	}
}
