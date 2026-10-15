package sales

// Laporan penjualan PER KASIR (rekonsiliasi laci): dibaca dari shift kasir (cash_shifts + cash_shift_counts).
// Angka "seharusnya", "fisik" dan "selisih" dibekukan saat shift ditutup, jadi identik dengan struk tutup shift dan
// tidak berubah bila nota diedit kemudian. Retur/kas kecil/bayar piutang sudah ikut di "seharusnya" (diatribusikan ke
// petugas yang memprosesnya, lihat shift/service.go recapSQL). Shift yang masih terbuka tampil tanpa rekonsiliasi.
//
// Siap big data: satu baris per shift (ratusan per bulan), bound waktu per outlet sargable, keyset (opened_at, id),
// ringkasan per kasir/metode hanya pada halaman pertama, rentang ≤ 366 hari.

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

const (
	defaultCashierLimit = 100
	maxCashierLimit     = 300
	maxCashierExport    = 20000
)

// CashierParams filter laporan penjualan kasir.
type CashierParams struct {
	AllOutlets bool
	OutletID   string
	From, To   string
	CashierID  string
	Status     string // "", "open", "closed"
	DiffOnly   bool
	Cursor     string
	Limit      int
	ForExport  bool
}

// CashierMethod = total satu metode bayar (dari rekap beku shift tertutup).
type CashierMethod struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Sales    string `json:"sales"`
	Flows    string `json:"flows"`
	Expected string `json:"expected"`
	Counted  string `json:"counted"`
	Diff     string `json:"diff"`
}

// CashierTotal = ringkasan satu kasir (atau seluruh kasir pada Summary).
type CashierTotal struct {
	UserID     string          `json:"user_id,omitempty"`
	Name       string          `json:"name"`
	Shifts     int             `json:"shifts"`
	OpenShifts int             `json:"open_shifts"`
	DiffShifts int             `json:"diff_shifts"` // shift yang selisihnya ≠ 0
	SaleCount  int             `json:"sale_count"`
	VoidCount  int             `json:"void_count"`
	SalesTotal string          `json:"sales_total"`
	Expected   string          `json:"expected"`
	Counted    string          `json:"counted"`
	Diff       string          `json:"diff"`
	DiffAbs    string          `json:"diff_abs"`
	Methods    []CashierMethod `json:"methods"`
	// Laci tunai: seharusnya / fisik / selisih khusus metode tunai (inti rekonsiliasi).
	CashOpening  string `json:"cash_opening"`
	CashExpected string `json:"cash_expected"`
	CashCounted  string `json:"cash_counted"`
	CashDiff     string `json:"cash_diff"`
}

// CashierShift = satu shift.
type CashierShift struct {
	ID           string `json:"id"`
	DocNo        string `json:"doc_no"`
	Status       string `json:"status"`
	OutletCode   string `json:"outlet_code"`
	OutletName   string `json:"outlet_name"`
	UserID       string `json:"user_id"`
	CashierName  string `json:"cashier_name"`
	OpenedDate   string `json:"opened_date"`
	OpenedTime   string `json:"opened_time"`
	ClosedTime   string `json:"closed_time"`
	Opening      string `json:"opening_cash"`
	SaleCount    int    `json:"sale_count"`
	VoidCount    int    `json:"void_count"`
	SalesTotal   string `json:"sales_total"`
	CashSales    string `json:"cash_sales"`
	CashFlows    string `json:"cash_flows"` // kas kecil, retur, bayar piutang tunai, dst. (bertanda)
	CashExpected string `json:"cash_expected"`
	CashCounted  string `json:"cash_counted"`
	CashDiff     string `json:"cash_diff"`
	Diff         string `json:"diff"` // selisih semua metode
	Note         string `json:"note"`
	ApprovedBy   string `json:"approved_by_name"`
}

// CashierResult hasil laporan.
type CashierResult struct {
	From        string `json:"from"`
	To          string `json:"to"`
	TenantName  string `json:"tenant_name"`
	OutletName  string `json:"outlet_name"`
	OutletCode  string `json:"outlet_code"`
	CashierName string `json:"cashier_name"`
	StatusLabel string `json:"status_label"`

	Data       []CashierShift `json:"data"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Truncated  bool           `json:"truncated,omitempty"`
	// Hanya halaman pertama / ekspor.
	HasSummary bool           `json:"has_summary"`
	Summary    CashierTotal   `json:"summary"`
	Cashiers   []CashierTotal `json:"cashiers"`
}

const cashierFilterSQL = `
      AND ($5::uuid = '00000000-0000-0000-0000-000000000000' OR c.user_id = $5::uuid)
      AND ($6::text = '' OR c.status = $6::text)
      AND (NOT $7::bool OR c.diff_abs > 0)
`

const cashierTotalsSQL = invoiceOutletsCTE + `
SELECT c.user_id, coalesce(u.name, ''), count(*)::int, (count(*) FILTER (WHERE c.status = 'open'))::int,
       (count(*) FILTER (WHERE c.diff_abs > 0))::int,
       coalesce(sum(c.sale_count), 0)::int, coalesce(sum(c.void_count), 0)::int, coalesce(sum(c.sales_total), 0),
       coalesce(sum(c.expected_total), 0), coalesce(sum(c.counted_total), 0), coalesce(sum(c.diff_total), 0), coalesce(sum(c.diff_abs), 0)
FROM ol
JOIN cash_shifts c ON c.tenant_id = $1 AND c.outlet_id = ol.id AND c.opened_at >= ol.lo AND c.opened_at < ol.hi
` + cashierFilterSQL + `
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = c.user_id
GROUP BY c.user_id, u.name
ORDER BY coalesce(sum(c.sales_total), 0) DESC, u.name
`

const cashierMethodsSQL = invoiceOutletsCTE + `
SELECT c.user_id, k.method_name, k.kind, sum(k.opening), sum(k.sales), sum(k.flows), sum(k.expected), sum(k.counted), sum(k.diff)
FROM ol
JOIN cash_shifts c ON c.tenant_id = $1 AND c.outlet_id = ol.id AND c.opened_at >= ol.lo AND c.opened_at < ol.hi
` + cashierFilterSQL + `
JOIN cash_shift_counts k ON k.tenant_id = $1 AND k.shift_id = c.id
GROUP BY c.user_id, k.method_name, k.kind
ORDER BY k.kind <> 'cash', k.method_name
`

const cashierShiftsSQL = invoiceOutletsCTE + `
SELECT c.id, c.doc_no, c.status, ol.code, ol.name, c.user_id, coalesce(u.name, ''), c.opened_at,
       to_char(c.opened_at AT TIME ZONE ol.timezone, 'YYYY-MM-DD'), to_char(c.opened_at AT TIME ZONE ol.timezone, 'HH24:MI'),
       coalesce(to_char(c.closed_at AT TIME ZONE ol.timezone, 'HH24:MI'), ''),
       c.opening_cash, coalesce(c.sale_count, 0), coalesce(c.void_count, 0), coalesce(c.sales_total, 0),
       coalesce(k.sales, 0), coalesce(k.flows, 0), coalesce(k.expected, 0), coalesce(k.counted, 0), coalesce(k.diff, 0),
       coalesce(c.diff_total, 0), c.note, coalesce(c.approved_name, '')
FROM ol
JOIN cash_shifts c ON c.tenant_id = $1 AND c.outlet_id = ol.id AND c.opened_at >= ol.lo AND c.opened_at < ol.hi
` + cashierFilterSQL + `
  AND ($8::bool = false OR (c.opened_at, c.id) < ($9::timestamptz, $10::uuid))
LEFT JOIN users u ON u.tenant_id = $1 AND u.id = c.user_id
LEFT JOIN LATERAL (
  SELECT sum(k.sales) AS sales, sum(k.flows) AS flows, sum(k.expected) AS expected, sum(k.counted) AS counted, sum(k.diff) AS diff
  FROM cash_shift_counts k WHERE k.tenant_id = $1 AND k.shift_id = c.id AND k.kind = 'cash'
) k ON true
ORDER BY c.opened_at DESC, c.id DESC
LIMIT $11
`

func ds(d decimal.Decimal) string { return d.StringFixed(2) }

// Cashiers: laporan penjualan per kasir / rekonsiliasi laci.
func (s *Service) Cashiers(ctx context.Context, a authz.Actor, p CashierParams) (CashierResult, error) {
	var result CashierResult
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var localDay pgtype.Date
		if err := tx.QueryRow(ctx, `SELECT (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&localDay); err != nil {
			return err
		}
		today := time.Now().UTC()
		if localDay.Valid {
			today = localDay.Time
		}
		fd, td := today, today
		fe := FieldErrors{}
		if p.From != "" {
			var perr error
			if fd, perr = time.Parse("2006-01-02", p.From); perr != nil {
				fe["from"] = "INVALID"
			}
		}
		if p.To != "" {
			var perr error
			if td, perr = time.Parse("2006-01-02", p.To); perr != nil {
				fe["to"] = "INVALID"
			}
		}
		if len(fe) == 0 && td.Before(fd) {
			fe["to"] = "MUST_BE_AFTER_FROM"
		}
		if td.Sub(fd) > maxDetailDays*24*time.Hour {
			fe["to"] = "MAX_366_DAYS"
		}
		status := strings.TrimSpace(p.Status)
		if status != "" && status != "open" && status != "closed" {
			fe["status"] = "INVALID"
		}
		cashier := uuid.Nil
		if p.CashierID != "" {
			id, err := uuid.Parse(p.CashierID)
			if err != nil || id == uuid.Nil {
				fe["cashier_id"] = "INVALID"
			}
			cashier = id
		}
		hasCursor := p.Cursor != "" && !p.ForExport
		var cursorAt time.Time
		cursorID := uuid.Nil
		if hasCursor {
			var ok bool
			if cursorAt, cursorID, ok = detailDecodeCursor(p.Cursor); !ok {
				fe["cursor"] = "INVALID"
			}
		}
		if len(fe) > 0 {
			return fe
		}

		outlets := []uuid.UUID{a.OutletID}
		if p.AllOutlets && len(a.Outlets) > 0 {
			outlets = outlets[:0]
			for id := range a.Outlets {
				outlets = append(outlets, id)
			}
		}
		if p.OutletID != "" {
			id, err := uuid.Parse(p.OutletID)
			if err != nil || id == uuid.Nil {
				return FieldErrors{"outlet_id": "INVALID"}
			}
			if id != a.OutletID && !a.Outlets[id] {
				return ErrOutletForbidden
			}
			outlets = []uuid.UUID{id}
		}
		limit := p.Limit
		if limit <= 0 {
			limit = defaultCashierLimit
		}
		if limit > maxCashierLimit {
			limit = maxCashierLimit
		}
		rowLimit := limit + 1
		if p.ForExport {
			rowLimit = maxCashierExport + 1
		}

		result.From, result.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		if cashier != uuid.Nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE tenant_id = $1 AND id = $2`, a.TenantID, cashier).Scan(&result.CashierName); err != nil {
				return err
			}
		}
		switch status {
		case "open":
			result.StatusLabel = "Open"
		case "closed":
			result.StatusLabel = "Closed"
		}
		if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, a.TenantID).Scan(&result.TenantName); err != nil {
			return err
		}
		if len(outlets) == 1 {
			if err := tx.QueryRow(ctx, `SELECT name, code FROM outlets WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, outlets[0]).Scan(&result.OutletName, &result.OutletCode); err != nil {
				return err
			}
		} else {
			result.OutletName = "Semua Outlet"
		}

		if !hasCursor {
			if err := s.cashierSummary(ctx, tx, &result, a, outlets, fd, td, cashier, status, p.DiffOnly); err != nil {
				return err
			}
		}

		rows, err := tx.Query(ctx, cashierShiftsSQL, a.TenantID, outlets, fd, td, cashier, status, p.DiffOnly,
			hasCursor, cursorAt, cursorID, rowLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []CashierShift{}
		var stamps []time.Time
		for rows.Next() {
			var x CashierShift
			var uid, id uuid.UUID
			var at time.Time
			var op, st, cs, cf, ce, cc, cd, df decimal.Decimal
			if err := rows.Scan(&id, &x.DocNo, &x.Status, &x.OutletCode, &x.OutletName, &uid, &x.CashierName, &at,
				&x.OpenedDate, &x.OpenedTime, &x.ClosedTime, &op, &x.SaleCount, &x.VoidCount, &st,
				&cs, &cf, &ce, &cc, &cd, &df, &x.Note, &x.ApprovedBy); err != nil {
				return err
			}
			x.ID, x.UserID = id.String(), uid.String()
			x.Opening, x.SalesTotal = ds(op), ds(st)
			x.CashSales, x.CashFlows, x.CashExpected, x.CashCounted, x.CashDiff, x.Diff = ds(cs), ds(cf), ds(ce), ds(cc), ds(cd), ds(df)
			list = append(list, x)
			stamps = append(stamps, at)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if p.ForExport {
			if len(list) > maxCashierExport {
				list = list[:maxCashierExport]
				result.Truncated = true
			}
		} else if len(list) > limit {
			list = list[:limit]
			result.NextCursor = detailEncodeCursor(stamps[limit-1], uuid.MustParse(list[limit-1].ID))
		}
		result.Data = list
		return nil
	})
	return result, err
}

func (s *Service) cashierSummary(ctx context.Context, tx pgx.Tx, res *CashierResult, a authz.Actor, outlets []uuid.UUID,
	fd, td time.Time, cashier uuid.UUID, status string, diffOnly bool) error {
	res.HasSummary = true
	res.Cashiers = []CashierTotal{}
	byUser := map[string]*CashierTotal{}
	all := CashierTotal{Name: "", Methods: []CashierMethod{}}
	var tSales, tExp, tCnt, tDiff, tAbs decimal.Decimal

	rows, err := tx.Query(ctx, cashierTotalsSQL, a.TenantID, outlets, fd, td, cashier, status, diffOnly)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid uuid.UUID
		var c CashierTotal
		var sales, exp, cnt, diff, abs decimal.Decimal
		if err := rows.Scan(&uid, &c.Name, &c.Shifts, &c.OpenShifts, &c.DiffShifts, &c.SaleCount, &c.VoidCount, &sales, &exp, &cnt, &diff, &abs); err != nil {
			rows.Close()
			return err
		}
		c.UserID = uid.String()
		c.SalesTotal, c.Expected, c.Counted, c.Diff, c.DiffAbs = ds(sales), ds(exp), ds(cnt), ds(diff), ds(abs)
		c.CashOpening, c.CashExpected, c.CashCounted, c.CashDiff = "0.00", "0.00", "0.00", "0.00"
		c.Methods = []CashierMethod{}
		res.Cashiers = append(res.Cashiers, c)
		all.Shifts += c.Shifts
		all.OpenShifts += c.OpenShifts
		all.DiffShifts += c.DiffShifts
		all.SaleCount += c.SaleCount
		all.VoidCount += c.VoidCount
		tSales, tExp, tCnt, tDiff, tAbs = tSales.Add(sales), tExp.Add(exp), tCnt.Add(cnt), tDiff.Add(diff), tAbs.Add(abs)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range res.Cashiers {
		byUser[res.Cashiers[i].UserID] = &res.Cashiers[i]
	}
	all.SalesTotal, all.Expected, all.Counted, all.Diff, all.DiffAbs = ds(tSales), ds(tExp), ds(tCnt), ds(tDiff), ds(tAbs)

	mrows, err := tx.Query(ctx, cashierMethodsSQL, a.TenantID, outlets, fd, td, cashier, status, diffOnly)
	if err != nil {
		return err
	}
	defer mrows.Close()
	allBy := map[string]int{}
	var cOp, cExp, cCnt, cDiff decimal.Decimal
	for mrows.Next() {
		var uid uuid.UUID
		var m CashierMethod
		var op, sl, fl, ex, cn, df decimal.Decimal
		if err := mrows.Scan(&uid, &m.Name, &m.Kind, &op, &sl, &fl, &ex, &cn, &df); err != nil {
			return err
		}
		m.Sales, m.Flows, m.Expected, m.Counted, m.Diff = ds(sl), ds(fl), ds(ex), ds(cn), ds(df)
		if c := byUser[uid.String()]; c != nil {
			c.Methods = append(c.Methods, m)
			if m.Kind == "cash" {
				prevOp, _ := decimal.NewFromString(c.CashOpening)
				prevEx, _ := decimal.NewFromString(c.CashExpected)
				prevCn, _ := decimal.NewFromString(c.CashCounted)
				prevDf, _ := decimal.NewFromString(c.CashDiff)
				c.CashOpening, c.CashExpected, c.CashCounted, c.CashDiff = ds(prevOp.Add(op)), ds(prevEx.Add(ex)), ds(prevCn.Add(cn)), ds(prevDf.Add(df))
			}
		}
		if m.Kind == "cash" {
			cOp, cExp, cCnt, cDiff = cOp.Add(op), cExp.Add(ex), cCnt.Add(cn), cDiff.Add(df)
		}
		key := m.Kind + "|" + m.Name
		i, ok := allBy[key]
		if !ok {
			i = len(all.Methods)
			allBy[key] = i
			all.Methods = append(all.Methods, CashierMethod{Name: m.Name, Kind: m.Kind, Sales: "0.00", Flows: "0.00", Expected: "0.00", Counted: "0.00", Diff: "0.00"})
		}
		g := &all.Methods[i]
		g.Sales, g.Flows, g.Expected = addStr(g.Sales, m.Sales), addStr(g.Flows, m.Flows), addStr(g.Expected, m.Expected)
		g.Counted, g.Diff = addStr(g.Counted, m.Counted), addStr(g.Diff, m.Diff)
	}
	if err := mrows.Err(); err != nil {
		return err
	}
	all.CashOpening, all.CashExpected, all.CashCounted, all.CashDiff = ds(cOp), ds(cExp), ds(cCnt), ds(cDiff)
	res.Summary = all
	return nil
}

// addStr menjumlahkan dua string desimal ("" dianggap 0).
func addStr(a, b string) string {
	x, _ := decimal.NewFromString(a)
	y, _ := decimal.NewFromString(b)
	return x.Add(y).StringFixed(2)
}
