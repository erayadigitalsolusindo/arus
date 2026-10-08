// Klien API penjualan. Uang/qty sebagai string desimal. Harga & total dihitung SERVER; klien hanya mengirim
// barang, satuan, qty, potongan, dan pembayaran. Idempotency-Key dibuat per percobaan bayar (dipakai ulang saat dicoba lagi).
import { api } from '#lib/api/client.ts';

export type PayMethod = 'cash' | 'debit' | 'credit_card' | 'ewallet' | 'transfer';
export const PAY_METHODS: PayMethod[] = ['cash', 'debit', 'credit_card', 'ewallet', 'transfer'];

export type SaleLineInput = { item_id: string; unit_id?: string; qty: string; discount?: string; note?: string; unit_price?: string };
export type SalePaymentInput = { method: PayMethod; amount: string; ref_no?: string };
export type SaleInput = {
  lines: SaleLineInput[];
  discount?: string;
  other_cost?: string;
  apply_tax: boolean;
  payments: SalePaymentInput[];
  note?: string;
  /** Wajib bila ada baris dengan unit_price (ubah harga): penyetuju Owner/Supervisor + PIN-nya. */
  approval?: { user_id: string; pin: string };
};

export type Sale = {
  id: string;
  doc_no: string;
  status: string;
  cashier: string;
  created_at: string;
  note: string;
  lines: { item_id: string; sku: string; name: string; unit: string; qty: string; unit_price: string; discount: string; line_total: string }[];
  payments: { method: PayMethod; amount: string; ref_no: string }[];
  subtotal: string;
  discount: string;
  tax_store: string;
  tax_gov: string;
  other_cost: string;
  total: string;
  paid: string;
  change: string;
};

/** Hasil hitung server tanpa menyimpan (pratinjau kasir): harga grosir/satuan/pajak outlet sudah diterapkan. */
export type Quote = {
  lines: { unit_price: string; list_price?: string; price_override?: boolean; line_total: string; qty: string; issue?: 'STOCK_INSUFFICIENT' | 'BELOW_COST'; available?: string }[];
  subtotal: string;
  discount: string;
  tax_store_pct: string;
  tax_gov_pct: string;
  tax_store: string;
  tax_gov: string;
  other_cost: string;
  total: string;
};

export const sales = {
  quote: (input: Omit<SaleInput, 'payments'>) => api<Quote>('/sales/quote', { method: 'POST', body: JSON.stringify(input) }),
  create: (input: SaleInput, idempotencyKey: string) =>
    api<Sale>('/sales/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  get: (id: string) => api<Sale>(`/sales/${id}`)
};

/** Kunci acak per percobaan bayar; sama selama modal bayar terbuka agar klik ganda/jaringan putus tidak membuat nota ganda. */
export const newIdempotencyKey = () => `pos-${crypto.randomUUID()}`;
