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
	methods: string;
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

export function salesInvoicesFileUrl(params: SalesInvoicesParams): string {
	const qs = invoicesQuery({ ...params, cursor: undefined });
	qs.set('format', 'xlsx');
	return `/reports/sales/invoices?${qs}`;
}
