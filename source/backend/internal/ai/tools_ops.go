package ai

import "github.com/shopspring/decimal"

// subDec = a − b untuk string desimal (kosong/rusak dianggap 0).
func subDec(a, b string) string {
	x, _ := decimal.NewFromString(a)
	y, _ := decimal.NewFromString(b)
	return x.Sub(y).StringFixed(2)
}

// ---- Persediaan ----

// stockQty = saldo per (outlet, barang) lintas bucket (display + gudang + retur), hanya barang fisik aktif. Alias: b, i, oc.
const stockFrom = `FROM stock_balances b
	JOIN outlets o ON o.tenant_id = b.tenant_id AND o.id = b.outlet_id
	JOIN items i ON i.tenant_id = b.tenant_id AND i.id = b.item_id AND i.kind = 'goods' AND i.active
	LEFT JOIN item_outlet_costs oc ON oc.tenant_id = b.tenant_id AND oc.outlet_id = b.outlet_id AND oc.item_id = b.item_id
	WHERE b.tenant_id = $1 AND b.outlet_id = ANY($2::uuid[])`

func toolStockOverview() tool {
	return tool{
		name: "stock_overview", module: "stock_card",
		desc:   "Inventory health per outlet: tracked items, items out of stock, negative stock, low stock (at or below the item's minimum) and, only if the user may see costs, the stock value at average cost.",
		schema: schema(map[string]any{"outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			outs, err := e.outlets(p)
			if err != nil {
				return nil, err
			}
			val := `NULL::numeric`
			if e.canCost {
				val = `coalesce(sum(x.total * x.cost) FILTER (WHERE x.total > 0), 0)`
			}
			rows, tr, err := e.rows(`
				SELECT o.name AS outlet, count(*) AS tracked_items, count(*) FILTER (WHERE x.total = 0) AS empty,
				       count(*) FILTER (WHERE x.total < 0 OR x.disp < 0) AS negative,
				       count(*) FILTER (WHERE x.min > 0 AND x.total > 0 AND x.total <= x.min) AS low, `+val+` AS stock_value
				FROM (
				  SELECT b.outlet_id, b.item_id, sum(b.qty) AS total, coalesce(sum(b.qty) FILTER (WHERE b.bucket = 'display'), 0) AS disp,
				         i.min_stock AS min, coalesce(oc.avg_cost, i.avg_cost) AS cost
				  `+stockFrom+`
				  GROUP BY b.outlet_id, b.item_id, i.min_stock, oc.avg_cost, i.avg_cost
				) x JOIN outlets o ON o.tenant_id = $1 AND o.id = x.outlet_id
				GROUP BY o.name ORDER BY o.name`, e.a.TenantID, outs)
			if err == nil && !e.canCost {
				for _, r := range rows {
					delete(r, "stock_value")
				}
			}
			return result(rows, tr, nil), err
		},
	}
}

func toolStockItems() tool {
	return tool{
		name: "stock_items", module: "stock_card",
		desc: "List items needing attention: filter=low (stock at/below minimum, most urgent first), empty (zero stock) or negative (stock below zero). " +
			"Per outlet: sku, name, qty (base units), min_stock; stock_value only if the user may see costs.",
		schema: schema(map[string]any{
			"filter": enum("Default low.", "low", "empty", "negative"), "outlet_id": pOutlet, "limit": intProp("Rows (default 30).", 1, 100),
		}),
		run: func(e *env, p params) (any, error) {
			outs, err := e.outlets(p)
			if err != nil {
				return nil, err
			}
			f, err := p.oneOf("filter", "low", "low", "empty", "negative")
			if err != nil {
				return nil, err
			}
			sel := map[string]struct{ cond, order string }{
				"low":      {`x.min_stock > 0 AND x.qty > 0 AND x.qty <= x.min_stock`, `x.qty / x.min_stock, x.name`},
				"empty":    {`x.qty = 0`, `x.name`},
				"negative": {`x.qty < 0`, `x.qty, x.name`},
			}[f]
			cond, order := sel.cond, sel.order
			rows, tr, err := e.rows(`
				SELECT x.outlet, x.sku, x.name, x.qty, x.min_stock, round(x.qty * x.cost, 2) AS stock_value FROM (
				  SELECT o.name AS outlet, i.sku, i.name, sum(b.qty) AS qty, i.min_stock, coalesce(oc.avg_cost, i.avg_cost) AS cost
				  `+stockFrom+`
				  GROUP BY o.name, i.id, i.sku, i.name, i.min_stock, oc.avg_cost, i.avg_cost
				) x WHERE `+cond+` ORDER BY `+order+` LIMIT $3`, e.a.TenantID, outs, p.integer("limit", 30, 1, 100))
			if err == nil && !e.canCost {
				for _, r := range rows {
					delete(r, "stock_value")
				}
			}
			return result(rows, tr, map[string]any{"filter": f}), err
		},
	}
}

func toolSlowMoving() tool {
	return tool{
		name: "slow_moving_items", module: "stock_card",
		desc:   "Dead/slow stock: items that still have stock on hand but had NO sale in the last N days. Shows qty and (if the user may see costs) the capital tied up. Use for clearance, promo or purchasing advice.",
		schema: schema(map[string]any{"days": intProp("Look-back window in days (default 60).", 7, 180), "outlet_id": pOutlet, "limit": intProp("Rows (default 20).", 1, 50)}),
		run: func(e *env, p params) (any, error) {
			outs, err := e.outlets(p)
			if err != nil {
				return nil, err
			}
			days := p.integer("days", 60, 7, 180)
			val, order := `NULL::numeric`, `qty DESC`
			if e.canCost {
				val, order = `round(sum(b.qty * coalesce(oc.avg_cost, i.avg_cost)), 2)`, `stock_value DESC`
			}
			rows, tr, err := e.rows(`
				WITH sold AS (
				  SELECT DISTINCT l.item_id FROM sale_lines l JOIN sales s ON s.tenant_id = l.tenant_id AND s.id = l.sale_id
				  WHERE l.tenant_id = $1 AND s.outlet_id = ANY($2::uuid[]) AND s.status = 'completed' AND s.created_at >= now() - make_interval(days => $3)
				)
				SELECT i.sku, i.name, sum(b.qty) AS qty, `+val+` AS stock_value
				`+stockFrom+` AND NOT EXISTS (SELECT 1 FROM sold WHERE sold.item_id = i.id)
				GROUP BY i.id, i.sku, i.name HAVING sum(b.qty) > 0 ORDER BY `+order+` LIMIT $4`,
				e.a.TenantID, outs, int32(days), p.integer("limit", 20, 1, 50))
			if err == nil && !e.canCost {
				for _, r := range rows {
					delete(r, "stock_value")
				}
			}
			return result(rows, tr, map[string]any{"days_without_sale": days}), err
		},
	}
}

func toolOpnameSummary() tool {
	return tool{
		name: "stock_opname_summary", module: "stock_opname",
		desc:   "Completed stock counts (opname) in a date range with their variances: lines counted, absolute qty difference, and (if the user may see costs) net and loss value. Highlights shrinkage/theft/process errors.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			val := ``
			if e.canCost {
				val = `, round(sum(l.diff * l.unit_cost), 2) AS net_value, round(sum(l.diff * l.unit_cost) FILTER (WHERE l.diff < 0), 2) AS loss_value`
			}
			rows, tr, err := e.rows(`
				SELECT c.doc_no, to_char(c.completed_at AT TIME ZONE o.timezone, 'YYYY-MM-DD') AS date, o.name AS outlet, count(*) AS lines,
				       sum(abs(l.diff)) AS abs_diff_qty`+val+`
				FROM stock_counts c
				JOIN stock_count_lines l ON l.tenant_id = c.tenant_id AND l.count_id = c.id
				JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
				WHERE c.tenant_id = $1 AND c.outlet_id = ANY($2::uuid[]) AND c.status = 'completed'
				  AND c.completed_at >= $3::date::timestamp AT TIME ZONE o.timezone AND c.completed_at < ($4::date + 1)::timestamp AT TIME ZONE o.timezone
				GROUP BY c.id, c.doc_no, c.completed_at, o.name, o.timezone ORDER BY c.completed_at DESC LIMIT 60`, e.a.TenantID, outs, from, to)
			return result(rows, tr, nil), err
		},
	}
}

// ---- Piutang & hutang ----

const agingBucket = `CASE WHEN days_over IS NULL THEN 'no_due_date' WHEN days_over <= 0 THEN 'not_due' WHEN days_over <= 30 THEN 'overdue_1_30'
	WHEN days_over <= 60 THEN 'overdue_31_60' WHEN days_over <= 90 THEN 'overdue_61_90' ELSE 'overdue_90_plus' END`

func toolReceivables() tool {
	return tool{
		name: "receivables_summary", module: "member_receivables",
		desc:   "Customer receivables (piutang) still outstanding, with aging buckets (not_due, overdue_1_30, 31_60, 61_90, 90_plus, no_due_date) and the 10 largest debtors ANONYMISED as 'Pelanggan N' (balance, open documents, days overdue). Use for collection priorities and credit-risk advice.",
		schema: schema(map[string]any{"outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			outs, err := e.outlets(p)
			if err != nil {
				return nil, err
			}
			const cte = `WITH ob AS (
				SELECT r.member_id, r.amount - coalesce(pd.paid, 0) - coalesce(rr.returned, 0) AS bal,
				       CASE WHEN r.due_date IS NULL THEN NULL ELSE (now() AT TIME ZONE o.timezone)::date - r.due_date END AS days_over
				FROM receivables r
				LEFT JOIN sales s ON s.tenant_id = r.tenant_id AND s.id = r.sale_id
				JOIN outlets o ON o.tenant_id = r.tenant_id AND o.id = r.outlet_id
				LEFT JOIN LATERAL (SELECT sum(rp.amount) AS paid FROM receivable_payments rp WHERE rp.tenant_id = r.tenant_id AND rp.receivable_id = r.id) pd ON true
				LEFT JOIN LATERAL (SELECT sum(sr.receivable_cut) AS returned FROM sales_returns sr
				                   WHERE sr.tenant_id = r.tenant_id AND sr.sale_id = r.sale_id AND sr.status = 'completed') rr ON true
				WHERE r.tenant_id = $1 AND r.outlet_id = ANY($2::uuid[]) AND r.voided_at IS NULL AND (r.sale_id IS NULL OR s.status = 'completed')
				  AND r.amount - coalesce(pd.paid, 0) - coalesce(rr.returned, 0) > 0
			)`
			aging, _, err := e.rows(cte+` SELECT `+agingBucket+` AS bucket, count(*) AS documents, sum(bal) AS balance FROM ob GROUP BY 1 ORDER BY 1`, e.a.TenantID, outs)
			if err != nil {
				return nil, err
			}
			top, _, err := e.rows(cte+` SELECT count(*) AS open_documents, sum(bal) AS balance, max(days_over) AS max_days_overdue
				FROM ob GROUP BY member_id ORDER BY balance DESC LIMIT 10`, e.a.TenantID, outs)
			if err != nil {
				return nil, err
			}
			anonymize(top)
			return map[string]any{"aging": aging, "top_debtors": top}, nil
		},
	}
}

func toolPayables() tool {
	return tool{
		name: "payables_summary", module: "supplier_payables",
		desc:   "Supplier payables (hutang) still outstanding with aging buckets by due date and the 10 largest suppliers by balance (supplier business names are shown). Use for cash-flow planning and payment priorities.",
		schema: schema(map[string]any{"outlet_id": pOutlet}),
		run: func(e *env, p params) (any, error) {
			outs, err := e.outlets(p)
			if err != nil {
				return nil, err
			}
			const cte = `WITH ob AS (
				SELECT pb.supplier_id,
				       pb.amount - coalesce((SELECT sum(x.amount) FROM payable_payments x WHERE x.tenant_id = pb.tenant_id AND x.payable_id = pb.id), 0)
				       - coalesce((SELECT sum(r.payable_cut) FROM purchase_returns r WHERE r.tenant_id = pb.tenant_id AND r.purchase_id = pb.purchase_id AND r.status = 'completed'), 0) AS bal,
				       CASE WHEN pb.due_date IS NULL THEN NULL ELSE (now() AT TIME ZONE o.timezone)::date - pb.due_date END AS days_over
				FROM payables pb JOIN outlets o ON o.tenant_id = pb.tenant_id AND o.id = pb.outlet_id
				WHERE pb.tenant_id = $1 AND pb.outlet_id = ANY($2::uuid[]) AND pb.voided_at IS NULL
			)`
			aging, _, err := e.rows(cte+` SELECT `+agingBucket+` AS bucket, count(*) AS documents, sum(bal) AS balance FROM ob WHERE bal > 0 GROUP BY 1 ORDER BY 1`, e.a.TenantID, outs)
			if err != nil {
				return nil, err
			}
			top, _, err := e.rows(cte+` SELECT s.name AS supplier, count(*) AS open_documents, sum(ob.bal) AS balance, max(ob.days_over) AS max_days_overdue
				FROM ob JOIN suppliers s ON s.tenant_id = $1 AND s.id = ob.supplier_id WHERE ob.bal > 0 GROUP BY s.id, s.name ORDER BY balance DESC LIMIT 10`, e.a.TenantID, outs)
			if err != nil {
				return nil, err
			}
			return map[string]any{"aging": aging, "top_suppliers": top}, nil
		},
	}
}

func toolPurchasesSummary() tool {
	return tool{
		name: "purchases_summary", module: "purchase_list",
		desc: "Purchase invoices (non-void) in a date range grouped by day/month/supplier/outlet/payment_type: count, subtotal, other_cost, total. Use for buying trends, supplier concentration and cash-vs-credit mix.",
		schema: schema(map[string]any{
			"from": pFrom, "to": pTo, "outlet_id": pOutlet,
			"group_by": enum("Default total.", "total", "day", "month", "supplier", "outlet", "payment_type"),
		}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			g, err := p.oneOf("group_by", "total", "total", "day", "month", "supplier", "outlet", "payment_type")
			if err != nil {
				return nil, err
			}
			grp := map[string]string{"total": `'total'`, "day": `to_char(pu.purchase_date, 'YYYY-MM-DD')`, "month": `to_char(pu.purchase_date, 'YYYY-MM')`,
				"supplier": `s.name`, "outlet": `o.name`, "payment_type": `pu.payment_type`}[g]
			order := "1"
			if g == "supplier" || g == "outlet" {
				order = "total DESC"
			}
			rows, tr, err := e.rows(`SELECT `+grp+` AS grp, count(*) AS invoices, sum(pu.subtotal) AS subtotal, sum(pu.other_cost) AS other_cost, sum(pu.total) AS total
				FROM purchases pu JOIN outlets o ON o.tenant_id = pu.tenant_id AND o.id = pu.outlet_id
				LEFT JOIN suppliers s ON s.tenant_id = pu.tenant_id AND s.id = pu.supplier_id
				WHERE pu.tenant_id = $1 AND pu.outlet_id = ANY($2::uuid[]) AND pu.status = 'completed' AND pu.purchase_date BETWEEN $3::date AND $4::date
				GROUP BY 1 ORDER BY `+order+` LIMIT 200`, e.a.TenantID, outs, from, to)
			return result(rows, tr, map[string]any{"group_by": g}), err
		},
	}
}

// ---- Kas & shift ----

func toolCashShifts() tool {
	return tool{
		name: "cash_shifts_summary", module: "cash_shifts",
		desc: "Cashier shift control for opened shifts in a date range, grouped by outlet/cashier/day: shifts, closed, shifts_with_difference, net_diff and abs_diff (cash over/short at closing), sales_total, voids. " +
			"Also lists the 5 largest cash differences. Use to spot cash-handling problems and unusually many voids.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "outlet_id": pOutlet, "group_by": enum("Default outlet.", "outlet", "cashier", "day")}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			g, err := p.oneOf("group_by", "outlet", "outlet", "cashier", "day")
			if err != nil {
				return nil, err
			}
			grp := map[string]string{"outlet": `o.name`, "cashier": `coalesce(u.name, '-')`, "day": `to_char((cs.opened_at AT TIME ZONE o.timezone)::date, 'YYYY-MM-DD')`}[g]
			const win = `cs.tenant_id = $1 AND cs.outlet_id = ANY($2::uuid[])
				AND cs.opened_at >= $3::date::timestamp AT TIME ZONE o.timezone AND cs.opened_at < ($4::date + 1)::timestamp AT TIME ZONE o.timezone`
			const from_ = ` FROM cash_shifts cs JOIN outlets o ON o.tenant_id = cs.tenant_id AND o.id = cs.outlet_id
				LEFT JOIN users u ON u.tenant_id = cs.tenant_id AND u.id = cs.user_id`
			rows, tr, err := e.rows(`SELECT `+grp+` AS grp, count(*) AS shifts, count(*) FILTER (WHERE cs.status = 'closed') AS closed,
				count(*) FILTER (WHERE cs.diff_abs > 0) AS shifts_with_difference, coalesce(sum(cs.diff_total), 0) AS net_diff, coalesce(sum(cs.diff_abs), 0) AS abs_diff,
				coalesce(sum(cs.sales_total), 0) AS sales_total, coalesce(sum(cs.void_count), 0) AS voids, coalesce(sum(cs.sale_count), 0) AS sale_count`+from_+`
				WHERE `+win+` GROUP BY 1 ORDER BY `+map[string]string{"outlet": "abs_diff DESC", "cashier": "abs_diff DESC", "day": "1"}[g]+` LIMIT 200`, e.a.TenantID, outs, from, to)
			if err != nil {
				return nil, err
			}
			worst, _, err := e.rows(`SELECT cs.doc_no, o.name AS outlet, coalesce(u.name, '-') AS cashier, to_char((cs.opened_at AT TIME ZONE o.timezone)::date, 'YYYY-MM-DD') AS date,
				cs.diff_total, cs.sales_total`+from_+` WHERE `+win+` AND cs.diff_abs > 0 ORDER BY cs.diff_abs DESC LIMIT 5`, e.a.TenantID, outs, from, to)
			if err != nil {
				return nil, err
			}
			return result(rows, tr, map[string]any{"group_by": g, "largest_differences": worst}), nil
		},
	}
}

func toolPettyCash() tool {
	return tool{
		name: "petty_cash_summary", module: "cash_shifts",
		desc:   "Petty cash (kas kecil) movements in a date range (voided entries excluded), grouped by category/outlet/day and direction (in/out): count and amount. Use to review small cash spending patterns.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "outlet_id": pOutlet, "group_by": enum("Default category.", "category", "outlet", "day")}),
		run: func(e *env, p params) (any, error) {
			from, to, outs, err := e.window(p)
			if err != nil {
				return nil, err
			}
			g, err := p.oneOf("group_by", "category", "category", "outlet", "day")
			if err != nil {
				return nil, err
			}
			grp := map[string]string{"category": `m.category`, "outlet": `o.name`, "day": `to_char((m.created_at AT TIME ZONE o.timezone)::date, 'YYYY-MM-DD')`}[g]
			rows, tr, err := e.rows(`SELECT `+grp+` AS grp, m.direction, count(*) AS entries, sum(m.amount) AS amount
				FROM cash_movements m JOIN outlets o ON o.tenant_id = m.tenant_id AND o.id = m.outlet_id
				WHERE m.tenant_id = $1 AND m.outlet_id = ANY($2::uuid[]) AND m.voided_at IS NULL
				  AND m.created_at >= $3::date::timestamp AT TIME ZONE o.timezone AND m.created_at < ($4::date + 1)::timestamp AT TIME ZONE o.timezone
				GROUP BY 1, 2 ORDER BY 1, 2`, e.a.TenantID, outs, from, to)
			return result(rows, tr, map[string]any{"group_by": g}), err
		},
	}
}

// ---- Akuntansi ----

func toolAccounting() tool {
	return tool{
		name: "accounting_balances", module: "general_ledger",
		desc: "General-ledger movement per account from POSTED journals in a date range (drafts excluded): code, name, class, debit, credit and balance on the account's normal side. " +
			"Also totals by class (asset, liability, equity, revenue, cogs, expense) with a derived net_profit = revenue - cogs - expense, and how many draft journals are still unposted. Not limited by outlet.",
		schema: schema(map[string]any{"from": pFrom, "to": pTo, "class": enum("Optional filter to one account class.", "asset", "liability", "equity", "revenue", "cogs", "expense")}),
		run: func(e *env, p params) (any, error) {
			from, to, err := e.dateRange(p)
			if err != nil {
				return nil, err
			}
			class, err := p.oneOf("class", "", "asset", "liability", "equity", "revenue", "cogs", "expense")
			if err != nil {
				return nil, err
			}
			rows, tr, err := e.rows(`SELECT a.code, a.name, a.class, sum(l.debit) AS debit, sum(l.credit) AS credit,
					CASE WHEN a.normal_side = 'debit' THEN sum(l.debit) - sum(l.credit) ELSE sum(l.credit) - sum(l.debit) END AS balance
				FROM journal_lines l
				JOIN journal_entries je ON je.tenant_id = l.tenant_id AND je.id = l.entry_id AND je.status = 'posted'
				JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
				WHERE l.tenant_id = $1 AND l.entry_date BETWEEN $2::date AND $3::date AND ($4 = '' OR a.class = $4)
				GROUP BY a.id, a.code, a.name, a.class, a.normal_side ORDER BY a.code`, e.a.TenantID, from, to, class)
			if err != nil {
				return nil, err
			}
			classes, _, err := e.rows(`SELECT a.class,
					sum(CASE WHEN a.normal_side = 'debit' THEN l.debit - l.credit ELSE l.credit - l.debit END) AS balance
				FROM journal_lines l
				JOIN journal_entries je ON je.tenant_id = l.tenant_id AND je.id = l.entry_id AND je.status = 'posted'
				JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
				WHERE l.tenant_id = $1 AND l.entry_date BETWEEN $2::date AND $3::date GROUP BY a.class ORDER BY a.class`, e.a.TenantID, from, to)
			if err != nil {
				return nil, err
			}
			drafts, _, err := e.rows(`SELECT count(*) AS draft_journals FROM journal_entries WHERE tenant_id = $1 AND status = 'draft' AND entry_date BETWEEN $2::date AND $3::date`, e.a.TenantID, from, to)
			if err != nil {
				return nil, err
			}
			return result(rows, tr, map[string]any{"by_class": classes, "drafts": drafts[0], "net_profit": netProfit(classes)}), nil
		},
	}
}

// netProfit = pendapatan − HPP − beban dari saldo per kelas (string desimal); nil bila tidak ada kelas laba-rugi.
func netProfit(classes []map[string]any) any {
	var rev, cogs, exp string
	for _, c := range classes {
		bal, _ := c["balance"].(string)
		switch c["class"] {
		case "revenue":
			rev = bal
		case "cogs":
			cogs = bal
		case "expense":
			exp = bal
		}
	}
	if rev == "" && cogs == "" && exp == "" {
		return nil
	}
	return subDec(subDec(rev, cogs), exp)
}

func registry() []tool {
	return []tool{
		toolListOutlets(), toolSalesSummary(), toolTopItems(), toolSalesByCategory(), toolReturnsSummary(), toolMemberOverview(),
		toolStockOverview(), toolStockItems(), toolSlowMoving(), toolOpnameSummary(),
		toolReceivables(), toolPayables(), toolPurchasesSummary(),
		toolCashShifts(), toolPettyCash(), toolAccounting(),
	}
}
