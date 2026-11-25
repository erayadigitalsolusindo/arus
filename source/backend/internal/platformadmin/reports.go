package platformadmin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"aciraba/internal/platform/httpx"
)

// Laporan lanjutan Platform Admin: penjualan per tenant, langganan yang mau habis, lokasi outlet.
// Uang dikirim sebagai string desimal (seperti live.go); hitung turunan (pertumbuhan, sisa hari) dilakukan klien.

type SalesReportRow struct {
	TenantID    uuid.UUID  `json:"tenant_id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	PlanName    string     `json:"plan_name"`
	OutletCount int64      `json:"outlet_count"`
	SalesCount  int64      `json:"sales_count"`
	Revenue     string     `json:"revenue"`
	PrevCount   int64      `json:"prev_count"`
	PrevRevenue string     `json:"prev_revenue"`
	ActiveDays  int64      `json:"active_days"`
	LastSaleAt  *time.Time `json:"last_sale_at"`
	TenantType  string     `json:"tenant_type"`
	Location    string     `json:"location"`
}

type SalesMonth struct {
	Month      string `json:"month"` // YYYY-MM
	SalesCount int64  `json:"sales_count"`
	Revenue    string `json:"revenue"`
}

type ExpiringRow struct {
	TenantID    uuid.UUID  `json:"tenant_id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	OwnerEmail  string     `json:"owner_email"`
	PlanName    string     `json:"plan_name"`
	PlanPrice   string     `json:"plan_price"`
	ExpiresAt   time.Time  `json:"expires_at"`
	Note        string     `json:"note"`
	OutletCount int64      `json:"outlet_count"`
	UserCount   int64      `json:"user_count"`
	LastLoginAt *time.Time `json:"last_login_at"`
	Revenue30d  string     `json:"revenue_30d"`
}

type OutletLocation struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	OutletID   uuid.UUID `json:"outlet_id"`
	OutletName string    `json:"outlet_name"`
	Address    string    `json:"address"`
	Active     bool      `json:"active"`
	Latitude   *float64  `json:"latitude"`
	Longitude  *float64  `json:"longitude"`
}

func (s *Service) SalesReport(ctx context.Context, a Actor, from, to time.Time) ([]SalesReportRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, code, name, active, plan_name, outlet_count, sales_count, revenue, prev_count, prev_revenue, active_days, last_sale_at, tenant_type, location
		FROM platform_report_sales($1, $2, $3)`, a.ID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SalesReportRow{}
	for rows.Next() {
		var x SalesReportRow
		var rev, prev decimal.Decimal
		if err := rows.Scan(&x.TenantID, &x.Code, &x.Name, &x.Active, &x.PlanName, &x.OutletCount, &x.SalesCount, &rev, &x.PrevCount, &prev, &x.ActiveDays, &x.LastSaleAt, &x.TenantType, &x.Location); err != nil {
			return nil, err
		}
		if !a.Allows(x.TenantID) {
			continue
		}
		x.Revenue, x.PrevRevenue = rev.StringFixed(2), prev.StringFixed(2)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) SalesMonthly(ctx context.Context, a Actor, tenant uuid.UUID) ([]SalesMonth, error) {
	rows, err := s.Pool.Query(ctx, `SELECT month, sales_count, revenue FROM platform_report_sales_monthly($1, $2)`, a.ID, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SalesMonth{}
	for rows.Next() {
		var m time.Time
		var x SalesMonth
		var rev decimal.Decimal
		if err := rows.Scan(&m, &x.SalesCount, &rev); err != nil {
			return nil, err
		}
		x.Month, x.Revenue = m.Format("2006-01"), rev.StringFixed(2)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) ExpiringReport(ctx context.Context, a Actor, days int) ([]ExpiringRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id, code, name, active, owner_email, plan_name, plan_price, expires_at, note, outlet_count, user_count, last_login_at, revenue_30d
		FROM platform_report_expiring($1, $2)`, a.ID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExpiringRow{}
	for rows.Next() {
		var x ExpiringRow
		var owner *string
		var price, rev decimal.Decimal
		if err := rows.Scan(&x.TenantID, &x.Code, &x.Name, &x.Active, &owner, &x.PlanName, &price, &x.ExpiresAt, &x.Note, &x.OutletCount, &x.UserCount, &x.LastLoginAt, &rev); err != nil {
			return nil, err
		}
		if !a.Allows(x.TenantID) {
			continue
		}
		if owner != nil {
			x.OwnerEmail = *owner
		}
		x.PlanPrice, x.Revenue30d = price.StringFixed(0), rev.StringFixed(2)
		out = append(out, x)
	}
	return out, rows.Err()
}

// wibDay membaca tanggal YYYY-MM-DD sebagai awal hari WIB (zona operator, sama seperti filter daftar tenant).
func wibDay(v string) (time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", v, time.FixedZone("WIB", 7*3600))
	return d, err == nil
}

// SalesReport: GET /platform/reports/sales?from=YYYY-MM-DD&to=YYYY-MM-DD (to inklusif; bawaan 30 hari terakhir, maks 366 hari).
func (h *Handler) SalesReport(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	q := r.URL.Query()
	to := time.Now().In(time.FixedZone("WIB", 7*3600))
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location()).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -30)
	var ok bool
	if v := q.Get("to"); v != "" {
		if to, ok = wibDay(v); !ok {
			httpx.ValidationError(w, map[string]string{"to": "INVALID"})
			return
		}
		to = to.AddDate(0, 0, 1)
	}
	if v := q.Get("from"); v != "" {
		if from, ok = wibDay(v); !ok {
			httpx.ValidationError(w, map[string]string{"from": "INVALID"})
			return
		}
	} else if q.Get("to") != "" {
		from = to.AddDate(0, 0, -30)
	}
	if !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
		httpx.ValidationError(w, map[string]string{"from": "INVALID"})
		return
	}
	items, err := h.svc.SalesReport(r.Context(), a, from, to)
	if err != nil {
		h.internal(w, r, "laporan penjualan platform gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "from": from, "to": to})
}

// SalesMonthly: GET /platform/reports/sales/{id}/monthly.
func (h *Handler) SalesMonthly(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	items, err := h.svc.SalesMonthly(r.Context(), a, id)
	if err != nil {
		h.internal(w, r, "tren penjualan tenant gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// ExpiringReport: GET /platform/reports/expiring?days=30.
func (h *Handler) ExpiringReport(w http.ResponseWriter, r *http.Request) {
	a, _ := ActorFrom(r.Context())
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			httpx.ValidationError(w, map[string]string{"days": "INVALID"})
			return
		}
		days = n
	}
	items, err := h.svc.ExpiringReport(r.Context(), a, days)
	if err != nil {
		h.internal(w, r, "laporan paket berakhir gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}
