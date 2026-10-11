package ai

import (
	"errors"
	"time"

	"aciraba/internal/authz"
)

// Syarat jendela waktu penjualan: batas kasar (tanpa zona waktu, agar indeks (tenant, outlet, created_at) terpakai) lalu batas
// tepat menurut zona waktu tiap outlet. Pola sama dengan modul sales. Alias wajib: s (sales), o (outlets).
// Parameter: $1 tenant, $2 outlet[], $3 from, $4 to.
const saleWin = `s.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed'
  AND s.created_at >= ($3::date - 1)::timestamptz AND s.created_at < ($4::date + 2)::timestamptz
  AND s.created_at >= $3::date::timestamp AT TIME ZONE o.timezone AND s.created_at < ($4::date + 1)::timestamp AT TIME ZONE o.timezone`

const saleLocal = `(s.created_at AT TIME ZONE o.timezone)`

var saleGroups = map[string]string{
	"total":   `'total'`,
	"day":     `to_char(` + saleLocal + `::date, 'YYYY-MM-DD')`,
	"week":    `to_char(date_trunc('week', ` + saleLocal + `), 'YYYY-MM-DD')`,
	"month":   `to_char(` + saleLocal + `, 'YYYY-MM')`,
	"outlet":  `o.name`,
	"cashier": `coalesce(u.name, '-')`,
	"weekday": `extract(isodow FROM ` + saleLocal + `)::int`, // 1 = Senin … 7 = Minggu
	"hour":    `extract(hour FROM ` + saleLocal + `)::int`,
}

func (e *env) window(p params) (from, to time.Time, outs any, err error) {
	if from, to, err = e.dateRange(p); err != nil {
		return
	}
	o, err := e.outlets(p)
	return from, to, o, err
}

func toolListOutlets() tool {
	return tool{
		name: "list_outlets", desc: "List the outlets (branches) the current user may access, with id, code, name and timezone. Call this first when you need an outlet_id or want to compare branches.",
		schema: schema(map[string]any{}),
		run: func(e *env, _ params) (any, error) {
			outs, err := e.outlets(params{})
			if err != nil {
				return nil, err
			}
			rows, tr, err := e.rows(`SELECT id, code, name, timezone FROM outlets WHERE tenant_id = $1 AND id = ANY($2::uuid[]) AND active ORDER BY name`, e.a.TenantID, outs)
			if err != nil {
				return nil, err
			}
			return result(rows, tr, map[string]any{"today": e.today.Format(time.DateOnly)}), nil
		},
	}
}

func toolSalesSummary() tool {
	return tool{
		name: "sales_summary", module: "sales_list",
		desc: "Sales totals for completed (non-void) invoices in a date range, grouped by day/week/month/outlet/cashier/weekday (1=Mon..7=Sun)/hour/payment_method. " +
			"Columns: invoices, revenue (invoice total incl. tax), discount, avg_invoice; gross_profit, cogs and item_revenue only if the user may see costs. " +
			"For payment_method: amount received per method (cash may include change). Use it to find trends, peak hours, best/worst branches, payment mix.",
		schema: schema(map[string]any{
			"from": pFrom, "to": pTo, "outlet_id": pOutlet,
			"group_by": enum("How to group the rows. Default total.", "total", "day", "week", "month", "outlet", "cashier", "weekday", "hour", "payment_method"),
		}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			g, err := p.oneOf("group_by", "total", "total", "day", "week", "month", "outlet", "cashier", "weekday", "hour", "payment_method")
			if err != nil {
				return nil, err
			}
			if g == "payment_method" {
				rows, tr, err := e.rows(`
					SELECT sp.method_name AS grp, count(DISTINCT s.id) AS invoices, sum(sp.amount) AS amount
					FROM sale_payments sp
					JOIN sales s ON s.tenant_id = sp.tenant_id AND s.id = sp.sale_id
					JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
					WHERE `+saleWin+` AND sp.tenant_id = $1
					GROUP BY 1 ORDER BY 3 DESC`, e.a.TenantID, outs, from, to)
				return result(rows, tr, map[string]any{"group_by": g}), err
			}
			q := `SELECT ` + saleGroups[g] + ` AS grp, count(*) AS invoices, sum(s.total) AS revenue, sum(s.discount) AS discount, round(avg(s.total), 2) AS avg_invoice`
			if e.canCost {
				q += `, sum(c.rev) AS item_revenue, sum(c.cogs) AS cogs, sum(c.rev - c.cogs) AS gross_profit`
			}
			q += ` FROM sales s JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
				LEFT JOIN users u ON u.tenant_id = s.tenant_id AND u.id = s.cashier_id`
			if e.canCost {
				q += ` LEFT JOIN LATERAL (SELECT sum(l.line_total) AS rev, sum(l.qty * l.unit_cost) AS cogs FROM sale_lines l
					WHERE l.tenant_id = s.tenant_id AND l.sale_id = s.id) c ON true`
			}
			order := "1"
			if g == "outlet" || g == "cashier" {
				order = "revenue DESC"
			}
			q += ` WHERE ` + saleWin + ` GROUP BY 1 ORDER BY ` + order
			rows, tr, err := e.rows(q, e.a.TenantID, outs, from, to)
			return result(rows, tr, map[string]any{"group_by": g, "from": from.Format(time.DateOnly), "to": to.Format(time.DateOnly)}), err
		},
	}
}

func toolTopItems() tool {
	return tool{
		name: "top_items", module: "sales_list",
		desc: "Best or worst selling items in a date range, ranked by revenue, quantity (in base units) or profit (profit only if the user may see costs). " +
			"Returns sku, name, qty_base, revenue (and profit). 'bottom' ranks the least sold among items that did sell.",
		schema: schema(map[string]any{
			"from": pFrom, "to": pTo, "outlet_id": pOutlet,
			"rank_by":   enum("Ranking metric. Default revenue.", "revenue", "qty", "profit"),
			"direction": enum("top = highest first (default), bottom = lowest first.", "top", "bottom"),
			"limit":     intProp("Rows to return (default 15).", 1, 50),
		}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			by, err := p.oneOf("rank_by", "revenue", "revenue", "qty", "profit")
			if err != nil {
				return nil, err
			}
			dir, err := p.oneOf("direction", "top", "top", "bottom")
			if err != nil {
				return nil, err
			}
			if by == "profit" && !e.canCost {
				return nil, errors.New("pengguna tidak boleh melihat HPP/laba; gunakan rank_by revenue atau qty")
			}
			metric := map[string]string{"revenue": "revenue", "qty": "qty_base", "profit": "profit"}[by]
			order := "DESC"
			if dir == "bottom" {
				order = "ASC"
			}
			q := `SELECT l.item_id, max(l.sku) AS sku, max(l.name) AS name, sum(l.qty * l.factor) AS qty_base, sum(l.line_total) AS revenue`
			if e.canCost {
				q += `, sum(l.line_total - l.qty * l.unit_cost) AS profit`
			}
			q += ` FROM sale_lines l
				JOIN sales s ON s.tenant_id = l.tenant_id AND s.id = l.sale_id
				JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
				WHERE l.tenant_id = $1 AND ` + saleWin + `
				GROUP BY l.item_id ORDER BY ` + metric + ` ` + order + `, 1 LIMIT $5`
			rows, tr, err := e.rows(q, e.a.TenantID, outs, from, to, p.integer("limit", 15, 1, 50))
			return result(rows, tr, map[string]any{"rank_by": by, "direction": dir}), err
		},
	}
}

func toolSalesByCategory() tool {
	return tool{
		name: "sales_by_category", module: "sales_list",
		desc:   "Revenue and quantity by item category in a date range (profit only if the user may see costs). Shows which product categories drive sales.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			q := `SELECT coalesce(c.name, '(no category)') AS category, sum(l.qty * l.factor) AS qty_base, sum(l.line_total) AS revenue`
			if e.canCost {
				q += `, sum(l.line_total - l.qty * l.unit_cost) AS profit`
			}
			q += ` FROM sale_lines l
				JOIN sales s ON s.tenant_id = l.tenant_id AND s.id = l.sale_id
				JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
				LEFT JOIN items i ON i.tenant_id = l.tenant_id AND i.id = l.item_id
				LEFT JOIN categories c ON c.tenant_id = i.tenant_id AND c.id = i.category_id
				WHERE l.tenant_id = $1 AND ` + saleWin + ` GROUP BY 1 ORDER BY revenue DESC`
			rows, tr, err := e.rows(q, e.a.TenantID, outs, from, to)
			return result(rows, tr, nil), err
		},
	}
}

func toolReturnsSummary() tool {
	return tool{
		name: "returns_summary",
		desc: "Returns (retur) in a date range. kind=sales: customer returns (count, total, refund, receivable_cut) plus the most returned items; " +
			"kind=purchase: returns to suppliers (count, total, payable_cut, refund). Group by total/month/day/outlet. High return rates hint at quality or process problems.",
		schema: schema(map[string]any{
			"kind": enum("sales or purchase. Default sales.", "sales", "purchase"), "from": pFrom, "to": pTo, "outlet_id": pOutlet,
			"group_by": enum("Default total.", "total", "month", "day", "outlet"),
		}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			kind, err := p.oneOf("kind", "sales", "sales", "purchase")
			if err != nil {
				return nil, err
			}
			g, err := p.oneOf("group_by", "total", "total", "month", "day", "outlet")
			if err != nil {
				return nil, err
			}
			mod, tbl, cols := "sales_returns", "sales_returns", `sum(r.total) AS total, sum(r.refund) AS refund, sum(r.receivable_cut) AS receivable_cut`
			if kind == "purchase" {
				mod, tbl, cols = "purchase_returns", "purchase_returns", `sum(r.total) AS total, sum(r.refund) AS refund, sum(r.payable_cut) AS payable_cut`
			}
			if !e.a.Perms.Has(mod, authz.ActView) {
				return nil, errors.New("pengguna tidak punya izin untuk data ini")
			}
			grp := map[string]string{"total": `'total'`, "month": `to_char(r.return_date, 'YYYY-MM')`, "day": `to_char(r.return_date, 'YYYY-MM-DD')`, "outlet": `o.name`}[g]
			rows, tr, err := e.rows(`SELECT `+grp+` AS grp, count(*) AS returns, `+cols+`
				FROM `+tbl+` r JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
				WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[]) AND r.status = 'completed' AND r.return_date BETWEEN $3::date AND $4::date
				GROUP BY 1 ORDER BY 1`, e.a.TenantID, outs, from, to)
			if err != nil {
				return nil, err
			}
			extra := map[string]any{"kind": kind, "group_by": g}
			if kind == "sales" {
				items, _, err := e.rows(`SELECT l.item_id, max(l.item_sku) AS sku, max(l.item_name) AS name, sum(l.base_qty) AS qty_base, sum(l.value) AS value
					FROM sales_return_lines l JOIN sales_returns r ON r.tenant_id = l.tenant_id AND r.id = l.return_id
					WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[]) AND r.status = 'completed' AND r.return_date BETWEEN $3::date AND $4::date
					GROUP BY l.item_id ORDER BY value DESC LIMIT 10`, e.a.TenantID, outs, from, to)
				if err != nil {
					return nil, err
				}
				extra["most_returned_items"] = items
			}
			return result(rows, tr, extra), nil
		},
	}
}

func toolMemberOverview() tool {
	return tool{
		name: "member_overview", module: "members",
		desc: "Customer (member) overview, ANONYMISED: active members, new members in the range, outstanding loyalty points, total deposit balance, " +
			"and (if the user may see sales) the share of revenue from members vs walk-ins and the top spenders shown only as 'Pelanggan N' with spend and invoice count. " +
			"Names, phones and addresses are never available.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			head, _, err := e.rows(`SELECT count(*) FILTER (WHERE active) AS active_members,
					count(*) FILTER (WHERE created_at >= $2::date::timestamptz AND created_at < ($3::date + 1)::timestamptz) AS new_members,
					coalesce(sum(points) FILTER (WHERE active), 0) AS points_outstanding
				FROM members WHERE tenant_id = $1`, e.a.TenantID, from, to)
			if err != nil {
				return nil, err
			}
			dep, _, err := e.rows(`SELECT coalesce(sum(x.b), 0) AS deposit_balance_total FROM
				(SELECT DISTINCT ON (member_id) balance_after AS b FROM member_deposit_movements WHERE tenant_id = $1 ORDER BY member_id, created_at DESC, id DESC) x`, e.a.TenantID)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"members": head[0], "deposit": dep[0]}
			if e.a.Perms.Has("sales_list", authz.ActView) {
				share, _, err := e.rows(`SELECT (s.member_id IS NOT NULL) AS is_member, count(*) AS invoices, sum(s.total) AS revenue
					FROM sales s JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
					WHERE `+saleWin+` GROUP BY 1 ORDER BY 1`, e.a.TenantID, outs, from, to)
				if err != nil {
					return nil, err
				}
				top, _, err := e.rows(`SELECT count(*) AS invoices, sum(s.total) AS spend
					FROM sales s JOIN outlets o ON o.tenant_id = s.tenant_id AND o.id = s.outlet_id
					WHERE `+saleWin+` AND s.member_id IS NOT NULL GROUP BY s.member_id ORDER BY spend DESC LIMIT 10`, e.a.TenantID, outs, from, to)
				if err != nil {
					return nil, err
				}
				anonymize(top)
				out["revenue_member_vs_walkin"], out["top_spenders"] = share, top
			}
			return out, nil
		},
	}
}
