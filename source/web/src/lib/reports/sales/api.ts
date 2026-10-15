// Klien API laporan penjualan.
import { api } from '#lib/api/client.ts';

// Tipe data respons: GET /reports/sales/by-date
export type MethodRow = {
	method_id?: string;
	name: string;
	kind: string;
	amount: string;
	fee: string;
	surcharge: string;
};

export type DayRow = {
	date: string;
	sale_count: number;
	void_count: number;
	subtotal: string;
	discount: string;
	tax_store: string;
	tax_gov: string;
	total: string;
	paid: string;
	receivable: string;
	void_total: string;
	methods?: MethodRow[];
	return_count: number;
	return_total: string;
	net_total: string;
};

export type SalesByDateResult = {
	from: string;
	to: string;
	tenant_name: string;
	outlet_name: string;
	outlet_code: string;
	data: DayRow[];
};

// GET /reports/sales/by-date (JSON) — parameter filter.
export async function salesByDate(params: {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	category_id?: string;
	payment_method_id?: string;
	all_outlets?: boolean;
	hide_empty?: boolean;
}): Promise<SalesByDateResult> {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.category_id) qs.set('category_id', params.category_id);
	if (params.payment_method_id) qs.set('payment_method_id', params.payment_method_id);
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.hide_empty) qs.set('hide_empty', 'true');
	return api(`/reports/sales/by-date?${qs}`);
}

// ── Laporan penjualan detail per tanggal ─────────────────────────────────────

export type DetailLine = {
	position: number;
	sku: string;
	name: string;
	unit_name: string;
	category_name: string;
	qty: string;
	unit_price: string;
	discount: string;
	line_total: string;
};

export type DetailMethod = {
	name: string;
	kind: string;
	amount: string;
};

export type DetailSale = {
	id: string;
	doc_no: string;
	time: string;
	status: string;
	outlet_code: string;
	outlet_name: string;
	cashier_name: string;
	member_name: string;
	salesperson_name: string;
	subtotal: string;
	discount: string;
	tax_store: string;
	tax_gov: string;
	other_cost: string;
	total: string;
	paid: string;
	change: string;
	receivable: string;
	lines_total: string;
	lines: DetailLine[];
	methods?: DetailMethod[];
};

export type DetailDay = {
	date: string;
	sale_count: number;
	void_count: number;
	subtotal: string;
	discount: string;
	tax_store: string;
	tax_gov: string;
	total: string;
	paid: string;
	receivable: string;
	sales: DetailSale[];
};

export type DetailSummary = {
	sale_count: number;
	void_count: number;
	subtotal: string;
	discount: string;
	tax_store: string;
	tax_gov: string;
	total: string;
	paid: string;
	receivable: string;
};

export type SalesDetailResult = {
	from: string;
	to: string;
	tenant_name: string;
	outlet_name: string;
	outlet_code: string;
	cashier_name: string;
	category_name: string;
	status_label: string;
	data: DetailDay[];
	summary: DetailSummary;
	next_cursor?: string;
	truncated?: boolean;
};

export type SalesDetailParams = {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	category_id?: string;
	status?: string;
	all_outlets?: boolean;
	cursor?: string;
	limit?: number;
};

// GET /reports/sales/detail (JSON, paginasi keyset).
export async function salesDetail(params: SalesDetailParams): Promise<SalesDetailResult> {
	const qs = detailQuery(params);
	return api(`/reports/sales/detail?${qs}`);
}

// URL unduh berkas ekspor (format=xlsx|pdf) — dipakai apiBlob.
export function salesDetailFileUrl(params: SalesDetailParams, format: 'xlsx' | 'pdf'): string {
	const qs = detailQuery(params);
	qs.set('format', format);
	return `/reports/sales/detail?${qs}`;
}

function detailQuery(params: SalesDetailParams): URLSearchParams {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.category_id) qs.set('category_id', params.category_id);
	if (params.status) qs.set('status', params.status);
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.cursor) qs.set('cursor', params.cursor);
	if (params.limit) qs.set('limit', String(params.limit));
	return qs;
}

// ── Penjualan per faktur ────────────────────────────────────────────────────────

export type InvoiceRow = {
	id: string;
	doc_no: string;
	date: string;
	time: string;
	status: string;
	outlet_code: string;
	outlet_name: string;
	cashier_name: string;
	member_name: string;
	salesperson_name: string;
	subtotal: string;
	discount: string;
	tax_store: string;
	tax_gov: string;
	other_cost: string;
	total: string;
	paid: string;
	change: string;
	receivable: string;
	methods: DetailMethod[];
};

export type SalesInvoicesResult = {
	from: string;
	to: string;
	data: InvoiceRow[];
	summary: DetailSummary;
	has_summary: boolean;
	next_cursor?: string;
};

export type SalesInvoicesParams = {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	status?: string;
	q?: string;
	all_outlets?: boolean;
	cursor?: string;
	limit?: number;
};

function invoicesQuery(params: SalesInvoicesParams): URLSearchParams {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.status) qs.set('status', params.status);
	if (params.q) qs.set('q', params.q);
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.cursor) qs.set('cursor', params.cursor);
	if (params.limit) qs.set('limit', String(params.limit));
	return qs;
}

// GET /reports/sales/invoices (JSON, paginasi keyset; ringkasan hanya halaman pertama).
export async function salesInvoices(params: SalesInvoicesParams): Promise<SalesInvoicesResult> {
	return api(`/reports/sales/invoices?${invoicesQuery(params)}`);
}

export function salesInvoicesFileUrl(params: SalesInvoicesParams, format: 'xlsx' | 'pdf'): string {
	const qs = invoicesQuery({ ...params, cursor: undefined });
	qs.set('format', format);
	return `/reports/sales/invoices?${qs}`;
}

// ── Penjualan per barang ────────────────────────────────────────────────────────

export type ItemSaleRow = {
	item_id: string;
	sku: string;
	name: string;
	category_name: string;
	unit_name: string;
	qty: string;
	line_count: number;
	gross: string;
	discount: string;
	net: string;
};

export type SalesItemsResult = {
	from: string;
	to: string;
	data: ItemSaleRow[];
	summary: { item_count: number; gross: string; discount: string; net: string };
	truncated?: boolean;
};

export type SalesItemsParams = {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	category_id?: string;
	q?: string;
	sort?: string;
	all_outlets?: boolean;
	limit?: number;
};

function itemsQuery(params: SalesItemsParams): URLSearchParams {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.category_id) qs.set('category_id', params.category_id);
	if (params.q) qs.set('q', params.q);
	if (params.sort) qs.set('sort', params.sort);
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.limit) qs.set('limit', String(params.limit));
	return qs;
}

// GET /reports/sales/items (JSON; top-N barang, total seluruh barang di summary).
export async function salesItems(params: SalesItemsParams): Promise<SalesItemsResult> {
	return api(`/reports/sales/items?${itemsQuery(params)}`);
}

export function salesItemsFileUrl(params: SalesItemsParams, format: 'xlsx' | 'pdf'): string {
	const qs = itemsQuery({ ...params, limit: undefined });
	qs.set('format', format);
	return `/reports/sales/items?${qs}`;
}

// ── Penjualan per kasir (rekonsiliasi laci) ─────────────────────────────────────

export type CashierMethod = { name: string; kind: string; sales: string; flows: string; expected: string; counted: string; diff: string };

export type CashierTotal = {
	user_id?: string;
	name: string;
	shifts: number;
	open_shifts: number;
	diff_shifts: number;
	sale_count: number;
	void_count: number;
	sales_total: string;
	expected: string;
	counted: string;
	diff: string;
	diff_abs: string;
	methods: CashierMethod[];
	cash_opening: string;
	cash_expected: string;
	cash_counted: string;
	cash_diff: string;
};

export type CashierShift = {
	id: string;
	doc_no: string;
	status: 'open' | 'closed';
	outlet_code: string;
	outlet_name: string;
	user_id: string;
	cashier_name: string;
	opened_date: string;
	opened_time: string;
	closed_time: string;
	opening_cash: string;
	sale_count: number;
	void_count: number;
	sales_total: string;
	cash_sales: string;
	cash_flows: string;
	cash_expected: string;
	cash_counted: string;
	cash_diff: string;
	diff: string;
	note: string;
	approved_by_name: string;
};

export type SalesCashiersResult = {
	from: string;
	to: string;
	data: CashierShift[];
	next_cursor?: string;
	truncated?: boolean;
	has_summary: boolean;
	summary: CashierTotal;
	cashiers: CashierTotal[];
};

export type SalesCashiersParams = {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	status?: string;
	diff_only?: boolean;
	all_outlets?: boolean;
	cursor?: string;
	limit?: number;
};

function cashiersQuery(params: SalesCashiersParams): URLSearchParams {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.status) qs.set('status', params.status);
	if (params.diff_only) qs.set('diff_only', 'true');
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.cursor) qs.set('cursor', params.cursor);
	if (params.limit) qs.set('limit', String(params.limit));
	return qs;
}

// GET /reports/sales/cashiers (JSON, keyset; ringkasan per kasir hanya halaman pertama).
export async function salesCashiers(params: SalesCashiersParams): Promise<SalesCashiersResult> {
	return api(`/reports/sales/cashiers?${cashiersQuery(params)}`);
}

export function salesCashiersFileUrl(params: SalesCashiersParams, format: 'xlsx' | 'pdf'): string {
	const qs = cashiersQuery({ ...params, cursor: undefined });
	qs.set('format', format);
	return `/reports/sales/cashiers?${qs}`;
}
