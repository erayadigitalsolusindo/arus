package item

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// Riwayat harga jual = pembacaan atas audit `item.price` (+ `item.create` sebagai harga awal); tanpa tabel baru.
// Hanya perubahan yang tercatat sejak fitur audit ada; harga yang diubah lewat jalur tanpa audit tidak muncul.

// Jenis baris riwayat.
const (
	HistoryPrice     = "price"     // harga jual default / per cabang
	HistoryWholesale = "wholesale" // set tier grosir
	HistoryInitial   = "initial"   // harga saat item dibuat
)

// PriceChange satu perubahan dalam satu kejadian. OutletID nil = harga default semua cabang.
// Before/After nil = belum ada (mis. harga khusus cabang baru dibuat atau dihapus).
type PriceChange struct {
	OutletID *uuid.UUID `json:"outlet_id"`
	Before   *string    `json:"before"`
	After    *string    `json:"after"`
}

type PriceEvent struct {
	ID        int64         `json:"id"`
	At        time.Time     `json:"at"`
	ActorName string        `json:"actor_name"`
	Kind      string        `json:"kind"`
	Changes   []PriceChange `json:"changes"`
}

type PriceHistory struct {
	Items      []PriceEvent `json:"items"`
	NextCursor string       `json:"next_cursor"`
}

// PriceHistory mengembalikan riwayat terbaru dulu. Baris awal (`initial`) ikut hanya di halaman terakhir.
// Perubahan pada cabang di luar akses pemanggil tidak ditampilkan (kejadian yang hanya berisi itu dibuang).
func (s *Service) PriceHistory(ctx context.Context, a authz.Actor, id uuid.UUID, cursor string, limit int) (*PriceHistory, error) {
	if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := gen.New(tx).ItemGet(ctx, gen.ItemGetParams{TenantID: a.TenantID, ID: id})
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	as := audit.NewService(s.pool)
	page, err := as.List(ctx, a.TenantID, audit.Filter{Entity: audit.EntityItem, EntityID: id.String(), ActionPrefix: audit.ActionItemPrice, Cursor: cursor, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := &PriceHistory{Items: []PriceEvent{}, NextCursor: page.NextCursor}
	for _, e := range page.Items {
		if ev, ok := priceEvent(e, a); ok {
			out.Items = append(out.Items, ev)
		}
	}
	if page.NextCursor == "" {
		first, err := as.List(ctx, a.TenantID, audit.Filter{Entity: audit.EntityItem, EntityID: id.String(), ActionPrefix: audit.ActionItemCreate, Limit: 1})
		if err != nil {
			return nil, err
		}
		for _, e := range first.Items {
			if v, ok := e.Details["sell_price"].(string); ok {
				out.Items = append(out.Items, PriceEvent{ID: e.ID, At: e.CreatedAt, ActorName: e.ActorName, Kind: HistoryInitial, Changes: []PriceChange{{After: &v}}})
			}
		}
	}
	return out, nil
}

func priceEvent(e audit.Item, a authz.Actor) (PriceEvent, bool) {
	ev := PriceEvent{ID: e.ID, At: e.CreatedAt, ActorName: e.ActorName, Kind: HistoryPrice}
	bk, ak := "before", "after"
	b, _ := e.Details[bk].(map[string]any)
	af, _ := e.Details[ak].(map[string]any)
	if b == nil && af == nil {
		bk, ak = "wholesale_before", "wholesale_after"
		b, _ = e.Details[bk].(map[string]any)
		af, _ = e.Details[ak].(map[string]any)
		ev.Kind = HistoryWholesale
	}
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range af {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted) // "default" tampil di antara id cabang secara deterministik; UI mengurutkan ulang
	str := func(m map[string]any, k string) *string {
		switch v := m[k].(type) {
		case string:
			return &v
		case nil:
			return nil
		default:
			s := fmt.Sprint(v)
			return &s
		}
	}
	for _, k := range sorted {
		c := PriceChange{Before: str(b, k), After: str(af, k)}
		if k != "default" {
			oid, err := uuid.Parse(k)
			if err != nil {
				continue
			}
			if !a.Outlets[oid] {
				continue
			}
			c.OutletID = &oid
		}
		ev.Changes = append(ev.Changes, c)
	}
	return ev, len(ev.Changes) > 0
}

// PriceHistory: GET /items/{id}/price-history?cursor=&limit=
func (h *Handler) PriceHistory(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.ActorFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan.")
		return
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 {
			httpx.ValidationError(w, map[string]string{"limit": "INVALID"})
			return
		}
		limit = min(n, 100)
	}
	res, err := h.svc.PriceHistory(r.Context(), actor, id, r.URL.Query().Get("cursor"), limit)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Item tidak ditemukan.")
	case errors.Is(err, audit.ErrBadCursor):
		httpx.ValidationError(w, map[string]string{"cursor": "INVALID"})
	case err != nil:
		h.log.Error("riwayat harga gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	default:
		httpx.JSON(w, http.StatusOK, res)
	}
}
