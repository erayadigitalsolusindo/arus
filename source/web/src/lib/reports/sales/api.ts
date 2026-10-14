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
	data: DayRow[];
};

// GET /reports/sales/by-date
export async function salesByDate(params: {
	from: string;
	to: string;
	outlet_id?: string;
	cashier_id?: string;
	all_outlets?: boolean;
	hide_empty?: boolean;
}): Promise<SalesByDateResult> {
	const qs = new URLSearchParams();
	qs.set('from', params.from);
	qs.set('to', params.to);
	if (params.outlet_id) qs.set('outlet_id', params.outlet_id);
	if (params.cashier_id) qs.set('cashier_id', params.cashier_id);
	if (params.all_outlets) qs.set('all_outlets', 'true');
	if (params.hide_empty) qs.set('hide_empty', 'true');
	return api(`/reports/sales/by-date?${qs}`);
}
