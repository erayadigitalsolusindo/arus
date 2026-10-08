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
  /** Member (opsional) dan poin yang ditukar jadi potongan nota. */
  member_id?: string;
  /** Salesman (opsional) untuk laporan/komisi; kosong = Umum. */
  salesperson_id?: string;
  redeem_points?: number;
  /** Kode kupon belanja (boleh lebih dari satu); potongannya dihitung server sebelum pajak. */
  voucher_codes?: string[];
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
  member?: { id: string; code: string; name: string };
  points_earned: number;
  points_redeemed: number;
  redeem_amount: string;
  vouchers: { code: string; name: string; kind: string; value: string; amount: string }[];
};

/** Hasil hitung server tanpa menyimpan (pratinjau kasir): harga grosir/satuan/pajak outlet sudah diterapkan. */
export type Quote = {
  lines: { unit_price: string; list_price?: string; price_override?: boolean; line_total: string; discount: string; qty: string; issue?: 'STOCK_INSUFFICIENT' | 'BELOW_COST'; available?: string }[];
  subtotal: string;
  discount: string;
  tax_store_pct: string;
  tax_gov_pct: string;
  tax_store: string;
  tax_gov: string;
  other_cost: string;
  total: string;
  /** Bila member dipilih: potongan dari tukar poin (sudah termasuk di discount) dan poin yang akan diperoleh. */
  member?: { id: string; code: string; name: string; points: number };
  redeem_amount: string;
  points_earn: number;
  /** Kupon yang lolos (amount = potongan rupiah; sudah termasuk di discount). */
  vouchers: { code: string; name: string; kind: string; value: string; amount: string }[];
  voucher_amount: string;
};

/** Satu baris daftar penjualan kasir; methods = jumlah per metode (tunai sudah bersih dari kembalian). */
export type SaleListRow = {
  id: string;
  doc_no: string;
  status: string;
  created_at: string;
  cashier: string;
  member?: string;
  line_count: number;
  total: string;
  methods: Partial<Record<PayMethod, string>>;
};
export type SaleList = { data: SaleListRow[]; total: string; totals: Partial<Record<PayMethod, string>>; from: string; to: string; truncated: boolean };

export const sales = {
  list: (p: { from?: string; to?: string; q?: string }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v) qs.set(k, v);
    return api<SaleList>(`/sales/?${qs}`);
  },
  quote: (input: Omit<SaleInput, 'payments'>) => api<Quote>('/sales/quote', { method: 'POST', body: JSON.stringify(input) }),
  create: (input: SaleInput, idempotencyKey: string) =>
    api<Sale>('/sales/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  get: (id: string) => api<Sale>(`/sales/${id}`)
};

/** Kunci acak per percobaan bayar; sama selama modal bayar terbuka agar klik ganda/jaringan putus tidak membuat nota ganda. */
export const newIdempotencyKey = () => `pos-${crypto.randomUUID()}`;
