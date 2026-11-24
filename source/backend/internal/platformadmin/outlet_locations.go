package platformadmin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

// Lokasi outlet dengan pagination cursor (keyset) atas (tenant_name, tenant_id, outlet_name, outlet_id):
// biaya halaman tetap walau outlet puluhan ribu, tanpa OFFSET. Filter dijalankan di server.

type locCursor struct {
	T string    `json:"t"`
	U uuid.UUID `json:"u"`
	O string    `json:"o"`
	I uuid.UUID `json:"i"`
}

type locFilter struct{ q, status, loc string }

type OutletLocationPage struct {
	Items   []OutletLocation `json:"items"`
	Next    string           `json:"next"`            // kosong = halaman terakhir
	Total   *int             `json:"total,omitempty"` // hanya pada halaman pertama
	Located *int             `json:"located,omitempty"`
}

func likeEscape(v string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v) + "%"
}

// where membangun klausa filter + argumen (mulai $2; $1 = id admin).
func (f locFilter) where() (string, []any) {
	var cond []string
	args := []any{}
	add := func(c string, v any) { args = append(args, v); cond = append(cond, fmt.Sprintf(c, len(args)+1)) }
	if f.q != "" {
		add("(l.tenant_name ILIKE $%[1]d OR l.outlet_name ILIKE $%[1]d OR l.address ILIKE $%[1]d)", likeEscape(f.q))
	}
	switch f.status {
	case "active":
		cond = append(cond, "l.active")
	case "inactive":
		cond = append(cond, "NOT l.active")
	}
	switch f.loc {
	case "has":
		cond = append(cond, "l.latitude IS NOT NULL")
	case "missing":
		cond = append(cond, "l.latitude IS NULL")
	}
	if len(cond) == 0 {
		return "TRUE", args
	}
	return strings.Join(cond, " AND "), args
}

const locCols = `l.tenant_id, l.tenant_name, l.outlet_id, l.outlet_name, l.address, l.active, l.latitude::float8, l.longitude::float8`

// OutletLocationPage mengambil satu halaman; mapOnly = hanya yang berkoordinat, tanpa batas halaman (untuk peta).
func (s *Service) OutletLocationPage(ctx context.Context, a Actor, f locFilter, cur *locCursor, limit int, mapOnly bool) (*OutletLocationPage, error) {
	where, args := f.where()
	if mapOnly {
		where += " AND l.latitude IS NOT NULL"
	}
	if cur != nil {
		n := len(args) + 2
		where += fmt.Sprintf(" AND (l.tenant_name, l.tenant_id, l.outlet_name, l.outlet_id) > ($%d, $%d, $%d, $%d)", n, n+1, n+2, n+3)
		args = append(args, cur.T, cur.U, cur.O, cur.I)
	}
	sql := `SELECT ` + locCols + ` FROM platform_outlet_locations($1) l WHERE ` + where +
		` ORDER BY l.tenant_name, l.tenant_id, l.outlet_name, l.outlet_id`
	if !mapOnly {
		sql += fmt.Sprintf(" LIMIT %d", limit+1) // +1 untuk tahu masih ada halaman berikutnya
	}
	rows, err := s.Pool.Query(ctx, sql, append([]any{a.ID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &OutletLocationPage{Items: []OutletLocation{}}
	for rows.Next() {
		var x OutletLocation
		if err := rows.Scan(&x.TenantID, &x.TenantName, &x.OutletID, &x.OutletName, &x.Address, &x.Active, &x.Latitude, &x.Longitude); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !mapOnly && len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		raw, _ := json.Marshal(locCursor{T: last.TenantName, U: last.TenantID, O: last.OutletName, I: last.OutletID})
		out.Next = base64.RawURLEncoding.EncodeToString(raw)
	}
	if cur == nil && !mapOnly { // hitungan hanya sekali, di halaman pertama
		w, wa := f.where()
		var total, located int
		if err := s.Pool.QueryRow(ctx, `SELECT count(*), count(l.latitude) FROM platform_outlet_locations($1) l WHERE `+w,
			append([]any{a.ID}, wa...)...).Scan(&total, &located); err != nil {
			return nil, err
		}
		out.Total, out.Located = &total, &located
	}
	return out, nil
}

// OutletLocations: GET /platform/reports/locations?q=&status=active|inactive&loc=has|missing&after=<cursor>&limit=1..100&map=1
func (h *Handler) OutletLocations(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	q := r.URL.Query()
	f := locFilter{q: strings.TrimSpace(q.Get("q")), status: q.Get("status"), loc: q.Get("loc")}
	if len(f.q) > 100 {
		f.q = f.q[:100]
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 40
	}
	var cur *locCursor
	if v := q.Get("after"); v != "" {
		raw, err := base64.RawURLEncoding.DecodeString(v)
		cur = &locCursor{}
		if err != nil || json.Unmarshal(raw, cur) != nil {
			httpx.ValidationError(w, map[string]string{"after": "INVALID"})
			return
		}
	}
	page, err := h.svc.OutletLocationPage(r.Context(), a, f, cur, limit, q.Get("map") == "1")
	if err != nil {
		h.internal(w, r, "lokasi outlet platform gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}
