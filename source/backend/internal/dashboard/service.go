// Package dashboard: ringkasan "apakah toko saya baik-baik saja" — satu endpoint baca-saja yang merangkum penjualan,
// stok, piutang, hutang, dan kas shift, lalu menyimpulkannya menjadi daftar pemeriksaan (ok/warn/bad).
// Tidak ada tabel baru: semua dibaca dari data transaksi. Tiap bagian hanya dikirim bila pemanggil berhak melihatnya.
// Angka piutang/hutang memanggil layanan modulnya agar sama persis dengan halaman daftar masing-masing.
package dashboard

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/payable"
	"aciraba/internal/platform/db"
	"aciraba/internal/receivable"
)

type dec = decimal.Decimal

const (
	trendDays   = 28
	openTooLong = 18 * time.Hour // shift terbuka lebih lama dari ini dianggap lupa ditutup
)

type Service struct {
	pool        *pgxpool.Pool
	receivables *receivable.Service
	payables    *payable.Service
	now         func() time.Time

	// Bagian stok memindai seluruh saldo (ratusan ribu baris di toko besar, ±0,6 dtk untuk 150 rb barang), sedangkan
	// dasbor menyegarkan diri tiap menit — hasilnya disimpan sebentar di memori proses. Basi paling lama stockTTL.
	stockTTL time.Duration
	mu       sync.Mutex
	stocks   map[string]stockEntry
}

type stockEntry struct {
	at  time.Time
	val *Stock
}

const maxStockCache = 512

func NewService(pool *pgxpool.Pool, r *receivable.Service, p *payable.Service) *Service {
	return &Service{pool: pool, receivables: r, payables: p, now: time.Now, stockTTL: time.Minute, stocks: map[string]stockEntry{}}
}

// WithStockTTL mengubah masa simpan ringkasan stok; 0 = selalu hitung ulang (dipakai tes).
func (s *Service) WithStockTTL(d time.Duration) *Service {
	s.stockTTL = d
	return s
}

func stockKey(tenant uuid.UUID, outlets []uuid.UUID, cost bool) string {
	ids := make([]string, len(outlets))
	for i, o := range outlets {
		ids[i] = o.String()
	}
	sort.Strings(ids)
	return tenant.String() + "|" + strings.Join(ids, ",") + "|" + map[bool]string{true: "c", false: "n"}[cost]
}

// ---- bentuk respons ----

type Day struct {
	Date  string `json:"date"` // YYYY-MM-DD menurut zona waktu outlet
	Total string `json:"total"`
	Count int    `json:"count"`
}

type Hour struct {
	Hour  int    `json:"hour"`
	Total string `json:"total"`
	Count int    `json:"count"`
}

type TopItem struct {
	Name    string `json:"name"`
	Qty     string `json:"qty"` // satuan dasar
	Revenue string `json:"revenue"`
}

type Today struct {
	Total      string  `json:"total"`
	Count      int     `json:"count"`
	Average    string  `json:"average"` // rata-rata per nota
	Voids      int     `json:"voids"`
	ReturnsCnt int     `json:"returns_count"`
	ReturnsAmt string  `json:"returns_total"`
	Profit     *string `json:"profit,omitempty"`    // laba kotor hari ini (sebelum pajak & biaya lain), setelah retur
	ProfitCost *string `json:"cost,omitempty"`      // HPP nota hari ini
	PeakHour   *int    `json:"peak_hour,omitempty"` // jam tersibuk (omzet)
}

// Compare = hari ini sampai jam sekarang vs rata-rata 4 hari yang sama (mis. 4 Sabtu terakhir) sampai jam yang sama.
type Compare struct {
	BaselineTotal string   `json:"baseline_total"`
	BaselineCount string   `json:"baseline_count"` // rata-rata jumlah nota
	Pct           *float64 `json:"pct,omitempty"`  // nil = belum ada pembanding
}

// MonthCompare = bulan berjalan sampai jam sekarang vs bulan lalu pada rentang yang sama (hari ke-1 s.d. hari ini, jam yang sama).
type MonthCompare struct {
	This string   `json:"this"`
	Last string   `json:"last"`
	Pct  *float64 `json:"pct,omitempty"` // nil = bulan lalu belum ada penjualan
}

type Sales struct {
	Today        Today        `json:"today"`
	Yesterday    Day          `json:"yesterday"`
	Compare      Compare      `json:"compare"`
	Week         string       `json:"week_total"`  // 7 hari terakhir termasuk hari ini
	Month        string       `json:"month_total"` // bulan berjalan
	MonthCompare MonthCompare `json:"month_compare"`
	Trend        []Day        `json:"trend"`     // 28 hari, hari tanpa nota = 0
	Hourly       []Hour       `json:"hourly"`    // hari ini, 0–23
	TopItems     []TopItem    `json:"top_items"` // 7 hari terakhir
}

type StockSample struct {
	Name   string `json:"name"`
	Qty    string `json:"qty"`
	Min    string `json:"min,omitempty"`    // batas minimum (stok menipis)
	Outlet string `json:"outlet,omitempty"` // hanya di mode semua cabang
}

// stockCounts = hitungan stok satu cabang (barang yang pernah punya saldo).
type stockCounts struct {
	Tracked, Empty, Negative, Low int
}

type Stock struct {
	Tracked   int           `json:"tracked"`         // barang berstok yang pernah punya saldo
	Active    int           `json:"active"`          // barang aktif (bukan jasa)
	Empty     int           `json:"empty"`           // saldo total tepat 0
	Negative  int           `json:"negative"`        // minus
	Low       int           `json:"low"`             // 0 < saldo <= batas minimum barang
	Monitored int           `json:"monitored"`       // barang aktif yang punya batas minimum
	Value     *string       `json:"value,omitempty"` // nilai persediaan (HPP × qty), hanya dengan izin HPP
	Negatives []StockSample `json:"negatives"`
	Lows      []StockSample `json:"lows"`
	byOutlet  map[uuid.UUID]stockCounts
}

// OutletRow = ringkasan satu cabang (hanya di mode semua cabang).
type OutletRow struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Today     string    `json:"today_total"`
	Count     int       `json:"today_count"`
	Yesterday string    `json:"yesterday_total"`
	Week      string    `json:"week_total"`
	Voids     int       `json:"voids"`
	Negative  *int      `json:"stock_negative,omitempty"`
	Low       *int      `json:"stock_low,omitempty"`
	Empty     *int      `json:"stock_empty,omitempty"`
	OpenShift *int      `json:"open_shifts,omitempty"`
}

type Receivable struct {
	Outstanding string `json:"outstanding"`
	Overdue     string `json:"overdue"`
	OpenCount   int64  `json:"open_count"`
}

type Payable struct {
	Outstanding string `json:"outstanding"`
	Overdue     string `json:"overdue"`
	OpenCount   int64  `json:"open_count"`
	Over30      string `json:"over_30"` // terlambat > 30 hari
}

type OpenShift struct {
	User     string    `json:"user"`
	Outlet   string    `json:"outlet"`
	OpenedAt time.Time `json:"opened_at"`
	Long     bool      `json:"long"`
	outletID uuid.UUID
}

type Shifts struct {
	Open       []OpenShift `json:"open"`
	DiffCount  int         `json:"diff_count"` // shift ditutup 7 hari terakhir yang berselisih
	DiffAbs    string      `json:"diff_abs"`
	ClosedWeek int         `json:"closed_week"`
}

// Check = satu pemeriksaan. Code + Params dirangkai jadi kalimat oleh klien (i18n); server tidak mengirim teks.
type Check struct {
	Code   string         `json:"code"`
	Level  string         `json:"level"` // ok | warn | bad
	Params map[string]any `json:"params,omitempty"`
}

type Overview struct {
	Scope       string      `json:"scope"` // outlet | all
	Outlets     int         `json:"outlets"`
	LocalDate   string      `json:"local_date"`
	GeneratedAt time.Time   `json:"generated_at"`
	Verdict     string      `json:"verdict"`
	Checks      []Check     `json:"checks"`
	Sales       *Sales      `json:"sales,omitempty"`
	Stock       *Stock      `json:"stock,omitempty"`
	Receivable  *Receivable `json:"receivable,omitempty"`
	Payable     *Payable    `json:"payable,omitempty"`
	Shifts      *Shifts     `json:"shifts,omitempty"`
	ByOutlet    []OutletRow `json:"by_outlet,omitempty"`
}

// Izin per bagian (modul sudah ada di registri izin).
const (
	modSales  = "sales_list"
	modOrders = "sales_orders"
	modCost   = "sales_cost"
	modItems  = "items"
	modShifts = "cash_shifts"
)

func canSales(a authz.Actor) bool {
	return a.Perms.Has(modSales, authz.ActView) || a.Perms.Has(modOrders, authz.ActView)
}

func (s *Service) Overview(ctx context.Context, a authz.Actor, all bool) (Overview, error) {
	outlets := []uuid.UUID{a.OutletID}
	scope := "outlet"
	if all && len(a.Outlets) > 1 {
		outlets = outlets[:0]
		for id := range a.Outlets {
			outlets = append(outlets, id)
		}
		scope = "all"
	}
	res := Overview{Scope: scope, Outlets: len(outlets), Checks: []Check{}, GeneratedAt: s.now().UTC()}

	showSales := canSales(a)
	showCost := a.Perms.Has(modCost, authz.ActView)
	showStock := a.Perms.Has(modItems, authz.ActView)
	showShifts := a.Perms.Has(modShifts, authz.ActView)

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var tz string
		var localDay pgtype.Date
		var dayStart time.Time
		if err := tx.QueryRow(ctx, `SELECT timezone, (now() AT TIME ZONE timezone)::date, ((now() AT TIME ZONE timezone)::date)::timestamp AT TIME ZONE timezone
			FROM outlets WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.OutletID).Scan(&tz, &localDay, &dayStart); err != nil {
			return err
		}
		res.LocalDate = localDay.Time.Format("2006-01-02")
		var per map[uuid.UUID]*outletSales
		if showSales {
			sl, p, err := s.sales(ctx, tx, a.TenantID, outlets, tz, localDay, dayStart, showCost)
			if err != nil {
				return err
			}
			res.Sales, per = sl, p
		}
		if showStock {
			st, err := s.cachedStock(ctx, tx, a.TenantID, outlets, showCost)
			if err != nil {
				return err
			}
			res.Stock = st
		}
		if showShifts {
			sh, err := s.shifts(ctx, tx, a.TenantID, outlets)
			if err != nil {
				return err
			}
			res.Shifts = sh
		}
		if scope == "all" {
			rows, err := tx.Query(ctx, `SELECT id, code, name FROM outlets WHERE tenant_id = $1 AND id = ANY($2::uuid[]) ORDER BY lower(name), id`, a.TenantID, outlets)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var r OutletRow
				if err := rows.Scan(&r.ID, &r.Code, &r.Name); err != nil {
					return err
				}
				p := per[r.ID]
				if p == nil {
					p = &outletSales{}
				}
				r.Today, r.Count, r.Yesterday, r.Week, r.Voids = p.today.StringFixed(2), p.count, p.yesterday.StringFixed(2), p.week.StringFixed(2), p.voids
				if res.Stock != nil {
					c := res.Stock.byOutlet[r.ID]
					r.Negative, r.Low, r.Empty = &c.Negative, &c.Low, &c.Empty
				}
				if res.Shifts != nil {
					n := 0
					for _, o := range res.Shifts.Open {
						if o.outletID == r.ID {
							n++
						}
					}
					r.OpenShift = &n
				}
				res.ByOutlet = append(res.ByOutlet, r)
			}
			return rows.Err()
		}
		return nil
	})
	if err != nil {
		return res, err
	}

	// Piutang & hutang memakai layanan modulnya (transaksi sendiri); selalu mencakup semua outlet yang boleh diakses.
	if a.Perms.Has(receivable.Module, authz.ActView) {
		l, err := s.receivables.List(ctx, a, receivable.ListParams{Status: "open", Limit: 1})
		if err != nil {
			return res, err
		}
		res.Receivable = &Receivable{Outstanding: l.Summary.Outstanding, Overdue: l.Summary.Overdue, OpenCount: l.Summary.OpenCount}
	}
	if a.Perms.Has(payable.Module, authz.ActView) {
		l, err := s.payables.List(ctx, a, payable.ListParams{Status: "open", Limit: 1})
		if err != nil {
			return res, err
		}
		res.Payable = &Payable{Outstanding: l.Summary.Outstanding, Overdue: l.Summary.Overdue, OpenCount: l.Summary.OpenCount,
			Over30: addStr(l.Summary.Aging.D31to60, l.Summary.Aging.D60Plus)}
	}

	res.Checks, res.Verdict = evaluate(res)
	return res, nil
}

func addStr(a, b string) string {
	x, _ := decimal.NewFromString(a)
	y, _ := decimal.NewFromString(b)
	return x.Add(y).StringFixed(2)
}

// sales: semua angka penjualan. Nota 'completed' = penjualan; 'void' hanya dihitung sebagai pembatalan; 'superseded' diabaikan.
type outletSales struct {
	today, yesterday, week dec
	count, voids           int
}

func (s *Service) sales(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, outlets []uuid.UUID, tz string, localDay pgtype.Date, dayStart time.Time, cost bool) (*Sales, map[uuid.UUID]*outletSales, error) {
	out := &Sales{Trend: make([]Day, 0, trendDays), Hourly: make([]Hour, 24), TopItems: []TopItem{}}

	// 28 hari per hari (zona waktu outlet). Batas bawah sargable pada created_at.
	rows, err := tx.Query(ctx, `
		SELECT outlet_id, (created_at AT TIME ZONE $3)::date AS d,
		       count(*) FILTER (WHERE status = 'completed'),
		       coalesce(sum(total) FILTER (WHERE status = 'completed'), 0),
		       count(*) FILTER (WHERE status = 'void')
		FROM sales
		WHERE tenant_id = $1 AND outlet_id = ANY($2::uuid[]) AND status IN ('completed', 'void')
		  AND created_at >= (($4::date - ($5::int - 1))::timestamp AT TIME ZONE $3)
		GROUP BY 1, 2`, tenant, outlets, tz, localDay, trendDays)
	if err != nil {
		return nil, nil, err
	}
	type dayAgg struct {
		count, voids int
		total        dec
	}
	byDay := map[string]dayAgg{}
	per := map[uuid.UUID]*outletSales{}
	todayKey := localDay.Time.Format("2006-01-02")
	yesterdayKey := localDay.Time.AddDate(0, 0, -1).Format("2006-01-02")
	weekFrom := localDay.Time.AddDate(0, 0, -6).Format("2006-01-02")
	for rows.Next() {
		var o uuid.UUID
		var d pgtype.Date
		var ag dayAgg
		if err := rows.Scan(&o, &d, &ag.count, &ag.total, &ag.voids); err != nil {
			rows.Close()
			return nil, nil, err
		}
		key := d.Time.Format("2006-01-02")
		cur := byDay[key]
		cur.count, cur.voids, cur.total = cur.count+ag.count, cur.voids+ag.voids, cur.total.Add(ag.total)
		byDay[key] = cur
		po := per[o]
		if po == nil {
			po = &outletSales{}
			per[o] = po
		}
		switch {
		case key == todayKey:
			po.today, po.count, po.voids = po.today.Add(ag.total), po.count+ag.count, po.voids+ag.voids
		case key == yesterdayKey:
			po.yesterday = po.yesterday.Add(ag.total)
		}
		if key >= weekFrom {
			po.week = po.week.Add(ag.total)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	week := dec{}
	for i := trendDays - 1; i >= 0; i-- {
		d := localDay.Time.AddDate(0, 0, -i)
		key := d.Format("2006-01-02")
		ag := byDay[key]
		out.Trend = append(out.Trend, Day{Date: key, Total: ag.total.StringFixed(2), Count: ag.count})
		if i < 7 {
			week = week.Add(ag.total)
		}
	}
	out.Week = week.StringFixed(2)

	// Bulan berjalan vs bulan lalu pada rentang yang sama (hari ke-1 s.d. hari ini, sampai jam yang sama).
	// Bulan lalu dibatasi akhir bulan itu (mis. hari ini tanggal 31, bulan lalu hanya 30 hari).
	monthStart := time.Date(localDay.Time.Year(), localDay.Time.Month(), 1, 0, 0, 0, 0, time.UTC)
	var mThis, mLast dec
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(sum(s.total) FILTER (WHERE s.created_at >= p.m0 AND s.created_at < now()), 0),
		       coalesce(sum(s.total) FILTER (WHERE s.created_at >= p.l0 AND s.created_at < least(p.l0 + (($6::int - 1) * interval '1 day') + p.el, p.m0)), 0)
		FROM (SELECT ($4::date)::timestamp AT TIME ZONE $3 AS m0, ($5::date)::timestamp AT TIME ZONE $3 AS l0,
		             now() - (($7::date)::timestamp AT TIME ZONE $3) AS el) p
		JOIN sales s ON s.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed'
		            AND s.created_at >= p.l0 AND s.created_at < now()`,
		tenant, outlets, tz, monthStart, monthStart.AddDate(0, -1, 0), localDay.Time.Day(), localDay).Scan(&mThis, &mLast); err != nil {
		return nil, nil, err
	}
	out.Month = mThis.StringFixed(2)
	out.MonthCompare = MonthCompare{This: mThis.StringFixed(2), Last: mLast.StringFixed(2)}
	if mLast.IsPositive() {
		p, _ := mThis.Sub(mLast).Div(mLast).Mul(decimal.NewFromInt(100)).Round(1).Float64()
		out.MonthCompare.Pct = &p
	}
	today := byDay[localDay.Time.Format("2006-01-02")]
	out.Today.Total, out.Today.Count, out.Today.Voids = today.total.StringFixed(2), today.count, today.voids
	avg := dec{}
	if today.count > 0 {
		avg = today.total.Div(decimal.NewFromInt(int64(today.count))).Round(2)
	}
	out.Today.Average = avg.StringFixed(2)
	yk := localDay.Time.AddDate(0, 0, -1).Format("2006-01-02")
	y := byDay[yk]
	out.Yesterday = Day{Date: yk, Total: y.total.StringFixed(2), Count: y.count}

	// Per jam, hari ini.
	for h := range out.Hourly {
		out.Hourly[h] = Hour{Hour: h, Total: "0.00"}
	}
	hrows, err := tx.Query(ctx, `
		SELECT extract(hour FROM created_at AT TIME ZONE $3)::int, count(*), coalesce(sum(total), 0)
		FROM sales WHERE tenant_id = $1 AND outlet_id = ANY($2::uuid[]) AND status = 'completed' AND created_at >= $4
		GROUP BY 1`, tenant, outlets, tz, dayStart)
	if err != nil {
		return nil, nil, err
	}
	peak, peakTotal := -1, dec{}
	for hrows.Next() {
		var h, n int
		var tot dec
		if err := hrows.Scan(&h, &n, &tot); err != nil {
			hrows.Close()
			return nil, nil, err
		}
		if h >= 0 && h < 24 {
			out.Hourly[h] = Hour{Hour: h, Total: tot.StringFixed(2), Count: n}
			if peak < 0 || tot.GreaterThan(peakTotal) {
				peak, peakTotal = h, tot
			}
		}
	}
	hrows.Close()
	if err := hrows.Err(); err != nil {
		return nil, nil, err
	}
	if peak >= 0 {
		out.Today.PeakHour = &peak
	}

	// Pembanding: 4 hari yang sama pada minggu-minggu sebelumnya, sampai jam yang sama.
	var bTotal dec
	var bCount int
	if err := tx.QueryRow(ctx, `
		WITH w AS (
		  SELECT (($4::date - 7 * k)::timestamp AT TIME ZONE $3) AS a FROM generate_series(1, 4) k
		), el AS (SELECT now() - (($4::date)::timestamp AT TIME ZONE $3) AS e)
		SELECT coalesce(sum(s.total), 0), count(*)
		FROM sales s
		WHERE s.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed'
		  AND s.created_at >= (($4::date - 28)::timestamp AT TIME ZONE $3) AND s.created_at < ($4::date::timestamp AT TIME ZONE $3)
		  AND EXISTS (SELECT 1 FROM w, el WHERE s.created_at >= w.a AND s.created_at < w.a + el.e)`,
		tenant, outlets, tz, localDay).Scan(&bTotal, &bCount); err != nil {
		return nil, nil, err
	}
	avgTotal := bTotal.Div(decimal.NewFromInt(4))
	avgCount := decimal.NewFromInt(int64(bCount)).Div(decimal.NewFromInt(4))
	out.Compare = Compare{BaselineTotal: avgTotal.StringFixed(2), BaselineCount: avgCount.StringFixed(1)}
	if avgTotal.IsPositive() {
		p, _ := today.total.Sub(avgTotal).Div(avgTotal).Mul(decimal.NewFromInt(100)).Round(1).Float64()
		out.Compare.Pct = &p
	}

	// Retur hari ini (menurut tanggal retur) — nilai & laba yang batal.
	var rCnt int
	var rTotal, rValue, rCost dec
	if err := tx.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(r.total), 0), coalesce(sum(r.subtotal - r.discount), 0)
		FROM sales_returns r
		WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[]) AND r.return_date = $3::date AND r.status = 'completed'`,
		tenant, outlets, localDay).Scan(&rCnt, &rTotal, &rValue); err != nil {
		return nil, nil, err
	}
	if rCnt > 0 {
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(sum(l.qty * l.unit_cost), 0)
			FROM sales_returns r JOIN sales_return_lines l ON l.tenant_id = r.tenant_id AND l.return_id = r.id
			WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[]) AND r.return_date = $3::date AND r.status = 'completed'`,
			tenant, outlets, localDay).Scan(&rCost); err != nil {
			return nil, nil, err
		}
	}
	out.Today.ReturnsCnt, out.Today.ReturnsAmt = rCnt, rTotal.StringFixed(2)

	if cost {
		var net, hpp dec
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(sum(s.subtotal - s.discount), 0),
			       coalesce(sum((SELECT sum(l.qty * l.unit_cost) FROM sale_lines l WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id)), 0)
			FROM sales s
			WHERE s.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed' AND s.created_at >= $3`,
			tenant, outlets, dayStart).Scan(&net, &hpp); err != nil {
			return nil, nil, err
		}
		profit := net.Sub(hpp).Sub(rValue.Sub(rCost))
		ps, cs := profit.StringFixed(2), hpp.StringFixed(2)
		out.Today.Profit, out.Today.ProfitCost = &ps, &cs
	}

	// Barang terlaris 7 hari terakhir (omzet per barang).
	trows, err := tx.Query(ctx, `
		SELECT l.name, sum(l.qty * l.factor), sum(l.qty * l.unit_price - l.discount) AS rev
		FROM sales s JOIN sale_lines l ON l.tenant_id = s.tenant_id AND l.sale_id = s.id
		WHERE s.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed'
		  AND s.created_at >= (($3::date - 6)::timestamp AT TIME ZONE $4)
		GROUP BY l.item_id, l.name ORDER BY rev DESC LIMIT 5`, tenant, outlets, localDay, tz)
	if err != nil {
		return nil, nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var name string
		var qty, rev dec
		if err := trows.Scan(&name, &qty, &rev); err != nil {
			return nil, nil, err
		}
		out.TopItems = append(out.TopItems, TopItem{Name: name, Qty: qty.StringFixed(3), Revenue: rev.StringFixed(2)})
	}
	return out, per, trows.Err()
}

func (s *Service) cachedStock(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, outlets []uuid.UUID, cost bool) (*Stock, error) {
	if s.stockTTL <= 0 {
		return s.stock(ctx, tx, tenant, outlets, cost)
	}
	key, now := stockKey(tenant, outlets, cost), s.now()
	s.mu.Lock()
	e, ok := s.stocks[key]
	s.mu.Unlock()
	if ok && now.Sub(e.at) < s.stockTTL {
		v := *e.val
		return &v, nil
	}
	st, err := s.stock(ctx, tx, tenant, outlets, cost)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if len(s.stocks) >= maxStockCache {
		for k, v := range s.stocks { // buang yang sudah basi; bila masih penuh, kosongkan
			if now.Sub(v.at) >= s.stockTTL {
				delete(s.stocks, k)
			}
		}
		if len(s.stocks) >= maxStockCache {
			s.stocks = map[string]stockEntry{}
		}
	}
	v := *st
	s.stocks[key] = stockEntry{at: now, val: &v}
	s.mu.Unlock()
	return st, nil
}

func (s *Service) stock(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, outlets []uuid.UUID, cost bool) (*Stock, error) {
	out := &Stock{Negatives: []StockSample{}, Lows: []StockSample{}, byOutlet: map[uuid.UUID]stockCounts{}}
	if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE min_stock > 0) FROM items WHERE tenant_id = $1 AND kind = 'goods' AND active`, tenant).
		Scan(&out.Active, &out.Monitored); err != nil {
		return nil, err
	}
	// Satu pemindaian saldo (per cabang+barang), hasilnya dikelompokkan per cabang. Menipis: 0 < saldo <= batas minimum barang.
	rows, err := tx.Query(ctx, `
		SELECT x.outlet_id, count(*), count(*) FILTER (WHERE x.total = 0), count(*) FILTER (WHERE x.total < 0 OR x.disp < 0),
		       count(*) FILTER (WHERE x.min > 0 AND x.total > 0 AND x.total <= x.min),
		       coalesce(sum(x.total * x.cost) FILTER (WHERE x.total > 0), 0)
		FROM (
		  SELECT b.outlet_id, b.item_id, sum(b.qty) AS total, coalesce(sum(b.qty) FILTER (WHERE b.bucket = 'display'), 0) AS disp,
		         i.min_stock AS min, coalesce(oc.avg_cost, i.avg_cost) AS cost
		  FROM stock_balances b
		  JOIN items i ON i.tenant_id = b.tenant_id AND i.id = b.item_id AND i.kind = 'goods' AND i.active
		  LEFT JOIN item_outlet_costs oc ON oc.tenant_id = b.tenant_id AND oc.outlet_id = b.outlet_id AND oc.item_id = b.item_id
		  WHERE b.tenant_id = $1 AND b.outlet_id = ANY($2::uuid[])
		  GROUP BY b.outlet_id, b.item_id, i.min_stock, oc.avg_cost, i.avg_cost
		) x GROUP BY x.outlet_id`, tenant, outlets)
	if err != nil {
		return nil, err
	}
	value := dec{}
	for rows.Next() {
		var o uuid.UUID
		var c stockCounts
		var v dec
		if err := rows.Scan(&o, &c.Tracked, &c.Empty, &c.Negative, &c.Low, &v); err != nil {
			rows.Close()
			return nil, err
		}
		out.byOutlet[o] = c
		out.Tracked, out.Empty, out.Negative, out.Low = out.Tracked+c.Tracked, out.Empty+c.Empty, out.Negative+c.Negative, out.Low+c.Low
		value = value.Add(v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cost {
		v := value.StringFixed(2)
		out.Value = &v
	}
	multi := len(outlets) > 1
	sample := func(cond, order string) ([]StockSample, error) {
		rs, err := tx.Query(ctx, `
			SELECT i.name, o.name, x.total, i.min_stock FROM (
			  SELECT b.outlet_id, b.item_id, sum(b.qty) AS total, coalesce(sum(b.qty) FILTER (WHERE b.bucket = 'display'), 0) AS disp
			  FROM stock_balances b WHERE b.tenant_id = $1 AND b.outlet_id = ANY($2::uuid[]) GROUP BY b.outlet_id, b.item_id
			) x JOIN items i ON i.tenant_id = $1 AND i.id = x.item_id AND i.kind = 'goods' AND i.active
			    JOIN outlets o ON o.tenant_id = $1 AND o.id = x.outlet_id`+" WHERE "+cond+" ORDER BY "+order+" LIMIT 5", tenant, outlets)
		if err != nil {
			return nil, err
		}
		defer rs.Close()
		var list []StockSample
		for rs.Next() {
			var n, on string
			var q, m dec
			if err := rs.Scan(&n, &on, &q, &m); err != nil {
				return nil, err
			}
			smp := StockSample{Name: n, Qty: q.StringFixed(3)}
			if m.IsPositive() {
				smp.Min = m.StringFixed(3)
			}
			if multi {
				smp.Outlet = on
			}
			list = append(list, smp)
		}
		return list, rs.Err()
	}
	if out.Negative > 0 {
		if out.Negatives, err = sample("x.total < 0 OR x.disp < 0", "least(x.total, x.disp)"); err != nil {
			return nil, err
		}
	}
	if out.Low > 0 {
		if out.Lows, err = sample("i.min_stock > 0 AND x.total > 0 AND x.total <= i.min_stock", "x.total / i.min_stock, i.name"); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) shifts(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, outlets []uuid.UUID) (*Shifts, error) {
	out := &Shifts{Open: []OpenShift{}}
	rows, err := tx.Query(ctx, `
		SELECT coalesce(u.name, ''), o.name, sh.opened_at, sh.outlet_id
		FROM cash_shifts sh
		JOIN outlets o ON o.tenant_id = sh.tenant_id AND o.id = sh.outlet_id
		LEFT JOIN users u ON u.tenant_id = sh.tenant_id AND u.id = sh.user_id
		WHERE sh.tenant_id = $1 AND sh.outlet_id = ANY($2::uuid[]) AND sh.status = 'open'
		ORDER BY sh.opened_at LIMIT 20`, tenant, outlets)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o OpenShift
		if err := rows.Scan(&o.User, &o.Outlet, &o.OpenedAt, &o.outletID); err != nil {
			rows.Close()
			return nil, err
		}
		o.Long = s.now().Sub(o.OpenedAt) > openTooLong
		out.Open = append(out.Open, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var diff dec
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE diff_abs > 0), coalesce(sum(diff_abs), 0)
		FROM cash_shifts
		WHERE tenant_id = $1 AND outlet_id = ANY($2::uuid[]) AND status = 'closed' AND closed_at >= now() - interval '7 days'`,
		tenant, outlets).Scan(&out.ClosedWeek, &out.DiffCount, &diff); err != nil {
		return nil, err
	}
	out.DiffAbs = diff.StringFixed(2)
	return out, nil
}

// evaluate menyimpulkan kesehatan toko. Setiap pemeriksaan selalu dikirim (juga yang beres) agar pemilik melihat
// apa saja yang sudah dicek, bukan hanya masalahnya.
func evaluate(o Overview) ([]Check, string) {
	var cs []Check
	add := func(code, level string, p map[string]any) {
		cs = append(cs, Check{Code: code, Level: level, Params: p})
	}

	if o.Sales != nil {
		sl := o.Sales
		switch {
		case sl.Compare.Pct == nil:
			add("sales_pace_nodata", "ok", nil)
		default:
			base, _ := decimal.NewFromString(sl.Compare.BaselineCount)
			p := map[string]any{"pct": *sl.Compare.Pct}
			if base.LessThan(decimal.NewFromInt(3)) { // pembanding terlalu sedikit untuk dinilai
				add("sales_pace_nodata", "ok", nil)
			} else if *sl.Compare.Pct <= -40 {
				add("sales_pace", "warn", p)
			} else {
				add("sales_pace", "ok", p)
			}
		}
		if t := sl.Today; t.Voids >= 3 && t.Voids*100 >= (t.Count+t.Voids)*10 {
			lvl := "warn"
			if t.Voids*100 >= (t.Count+t.Voids)*25 {
				lvl = "bad"
			}
			add("voids", lvl, map[string]any{"count": t.Voids})
		} else {
			add("voids", "ok", map[string]any{"count": t.Voids})
		}
	}
	if o.Shifts != nil {
		long := 0
		for _, s := range o.Shifts.Open {
			if s.Long {
				long++
			}
		}
		if long > 0 {
			add("shift_open_long", "warn", map[string]any{"count": long})
		} else {
			add("shift_open_long", "ok", nil)
		}
		if o.Shifts.DiffCount > 0 {
			add("shift_diff", "warn", map[string]any{"count": o.Shifts.DiffCount, "amount": o.Shifts.DiffAbs})
		} else {
			add("shift_diff", "ok", nil)
		}
	}
	if o.Stock != nil {
		if o.Stock.Negative > 0 {
			add("stock_negative", "warn", map[string]any{"count": o.Stock.Negative})
		} else {
			add("stock_negative", "ok", nil)
		}
		switch {
		case o.Stock.Monitored == 0:
			add("stock_low_unset", "ok", nil)
		case o.Stock.Low > 0:
			add("stock_low", "warn", map[string]any{"count": o.Stock.Low})
		default:
			add("stock_low", "ok", nil)
		}
	}
	if o.Receivable != nil {
		od, _ := decimal.NewFromString(o.Receivable.Overdue)
		out, _ := decimal.NewFromString(o.Receivable.Outstanding)
		switch {
		case !od.IsPositive():
			add("receivable_overdue", "ok", nil)
		case out.IsPositive() && od.Div(out).GreaterThanOrEqual(decimal.NewFromFloat(0.5)):
			add("receivable_overdue", "bad", map[string]any{"amount": o.Receivable.Overdue})
		default:
			add("receivable_overdue", "warn", map[string]any{"amount": o.Receivable.Overdue})
		}
	}
	if o.Payable != nil {
		od, _ := decimal.NewFromString(o.Payable.Overdue)
		o30, _ := decimal.NewFromString(o.Payable.Over30)
		switch {
		case !od.IsPositive():
			add("payable_overdue", "ok", nil)
		case o30.IsPositive():
			add("payable_overdue", "bad", map[string]any{"amount": o.Payable.Overdue})
		default:
			add("payable_overdue", "warn", map[string]any{"amount": o.Payable.Overdue})
		}
	}

	verdict := "ok"
	for _, c := range cs {
		switch {
		case c.Level == "bad":
			verdict = "bad"
		case c.Level == "warn" && verdict != "bad":
			verdict = "warn"
		}
	}
	if cs == nil {
		cs = []Check{}
	}
	return cs, verdict
}
