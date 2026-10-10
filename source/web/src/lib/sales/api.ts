// Klien API penjualan. Uang/qty sebagai string desimal. Harga & total dihitung SERVER; klien hanya mengirim
// barang, satuan, qty, potongan, dan pembayaran. Idempotency-Key dibuat per percobaan bayar (dipakai ulang saat dicoba lagi).
import { api } from '#lib/api/client.ts';

export type PayMethod = 'cash' | 'debit' | 'credit_card' | 'ewallet' | 'transfer' | 'deposit';
/** Arus uang lain lewat kasir di outlet aktif (bertanda: + masuk, − keluar). */
export type CashFlowSource = 'receivable_payment' | 'deposit_topup' | 'deposit_withdraw' | 'sale_return' | 'payable_payment' | 'purchase_return' | 'supplier_credit_cashout';
export type CashFlow = { source: CashFlowSource; method_id: string; name: string; kind: PayMethod; amount: string; count: number };
export const PAY_METHODS: PayMethod[] = ['cash', 'debit', 'credit_card', 'ewallet', 'transfer'];

export type SaleLineInput = { item_id: string; unit_id?: string; qty: string; discount?: string; note?: string; unit_price?: string };
/** method_id = metode dari master Metode Pembayaran (lihat paymentMethodsLookup). */
export type SalePaymentInput = { method_id: string; amount: string; ref_no?: string };
export type SaleInput = {
  lines: SaleLineInput[];
  discount?: string;
  other_cost?: string;
  /** Rincian biaya lain-lain (nama + jumlah); bila ada, totalnya menjadi other_cost di server. */
  other_costs?: { name: string; amount: string }[];
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
  /** Nota kredit (hanya member): sisa yang belum dibayar menjadi piutang. Melewati limit member butuh approval berizin credit_limit. */
  credit?: boolean;
  /** Wajib bila ada baris dengan unit_price (ubah harga): penyetuju Owner/Supervisor + PIN-nya. */
  approval?: { user_id: string; pin: string };
};

/** Piutang yang melekat pada nota kredit (keadaan sekarang). */
export type SaleCredit = { id: string; amount: string; paid: string; returned: string; balance: string; due_date?: string; status: 'open' | 'overdue' | 'paid' };

export type Sale = {
  id: string;
  doc_no: string;
  status: string;
  cashier: string;
  created_at: string;
  note: string;
  outlet_id: string;
  lines: { item_id: string; unit_id: string; sku: string; name: string; unit: string; qty: string; unit_price: string; list_price?: string; price_override?: boolean; discount: string; line_total: string }[];
  payments: { method: PayMethod; method_id: string; method_name: string; amount: string; ref_no: string; fee_pct: string; fee: string; fee_bearer: 'store' | 'customer' }[];
  /** Biaya metode yang ditagihkan ke pelanggan (di luar total). Ditagih = total + surcharge. */
  surcharge: string;
  /** Sisa yang belum dibayar saat nota dibuat (nota kredit); credit = keadaan piutangnya sekarang. */
  receivable: string;
  credit?: SaleCredit;
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
  /** Rincian biaya lain-lain (kosong bila nota hanya punya satu angka); other_cost tetap totalnya. */
  other_costs: { name: string; amount: string }[];
  tax_store_pct: string;
  tax_gov_pct: string;
  salesperson?: { id: string; name: string };
  /** Edit/batal: revision 1 = asli; superseded_by terisi = sudah digantikan revisi; root_id = nota asli rantai. */
  revision: number;
  root_id: string;
  supersedes_id?: string;
  superseded_by?: string;
  revision_reason?: string;
  void_reason?: string;
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
  member?: { id: string; code: string; name: string; points: number; deposit?: string };
  redeem_amount: string;
  points_earn: number;
  /** Kupon yang lolos (amount = potongan rupiah; sudah termasuk di discount). */
  vouchers: { code: string; name: string; kind: string; value: string; amount: string }[];
  voucher_amount: string;
  /** Bila member dipilih: syarat kredit (limit "0.00" = tanpa batas) dan piutangnya sekarang. */
  credit?: { limit: string; outstanding: string; due_days: number };
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
  /** Biaya metode yang ditagihkan ke pelanggan; ditagih = total + surcharge. */
  surcharge: string;
  /** Bagian nota yang dikreditkan (piutang member). */
  receivable: string;
  methods: Partial<Record<PayMethod, string>>;
  pays: { method_id: string; name: string; kind: PayMethod; amount: string }[];
  /** Σ nilai retur aktif nota ini (nota sendiri tidak berubah). */
  returned: string;
};
export type SaleList = { data: SaleListRow[]; total: string; surcharge: string; received: string; totals: Partial<Record<PayMethod, string>>; by_method: { method_id: string; name: string; kind: PayMethod; amount: string }[]; flows: CashFlow[]; drawer: { method_id: string; name: string; kind: PayMethod; amount: string }[]; from: string; to: string; truncated: boolean;
  /** Retur yang dibuat kasir ini pada rentang yang sama (menurut tanggal retur); tidak ada saat mencari nomor nota. */
  returns?: { count: number; total: string };
  net_total?: string;
};

/** Satu nota di Daftar Penjualan (semua kasir). cost/profit hanya ada bila pengguna punya izin sales_cost. */
export type SaleAllRow = {
  id: string;
  doc_no: string;
  status: 'completed' | 'void';
  created_at: string;
  outlet: { id: string; code: string; name: string };
  cashier: string;
  member?: string;
  salesperson?: string;
  line_count: number;
  revision: number;
  /** Σ nilai retur aktif nota ini. */
  returned: string;
  subtotal: string;
  line_discount: string;
  discount: string;
  manual_discount: string;
  voucher_amount: string;
  voucher_codes: string[];
  redeem_amount: string;
  points_redeemed: number;
  points_earned: number;
  price_overrides: number;
  tax_store: string;
  tax_gov: string;
  other_cost: string;
  total: string;
  /** Bagian yang dikreditkan (piutang member). */
  receivable: string;
  methods: Partial<Record<PayMethod, string>>;
  cost?: string;
  profit?: string;
};
export type SaleAllSummary = { count: number; completed_count: number; total: string; receivable: string; discount: string; methods: Partial<Record<PayMethod, string>>; by_method: { method_id: string; name: string; kind: PayMethod; amount: string; fee: string; surcharge: string }[]; cost?: string; profit?: string;
  /** Retur yang terjadi di rentang ini menurut TANGGAL RETUR; tidak ada bila filter cari/metode/status batal aktif. */
  returns?: { count: number; total: string; value: string; cost?: string; profit?: string };
  /** Laba kotor − laba yang batal karena retur (izin HPP). */
  profit_net?: string;
};
export type SaleAllList = { data: SaleAllRow[]; summary: SaleAllSummary; from: string; to: string; all_outlets: boolean; next_cursor?: string };
export type SaleAllParams = { from?: string; to?: string; q?: string; status?: string; method?: string; cashier_id?: string; allOutlets?: boolean; cursor?: string | null };

export type SaleDetailLine = {
  position: number;
  item_id: string;
  sku: string;
  name: string;
  unit: string;
  factor: string;
  qty: string;
  base_qty: string;
  list_price: string;
  unit_price: string;
  price_override: boolean;
  discount: string;
  line_total: string;
  note: string;
  /** Qty (satuan jual) yang sudah diretur lewat retur aktif. Baris nota sendiri tidak berubah. */
  returned_qty: string;
  unit_cost?: string;
  line_cost?: string;
  profit?: string;
};
export type SaleStockMove = { id: number; at: string; type: 'SALE' | 'SALE_VOID' | 'SALE_RETURN'; bucket: 'display' | 'warehouse' | 'returns'; item_id: string; sku: string; name: string; unit: string; delta: string; balance_after: string; actor: string };
export type SaleEvent = { action: string; actor: string; at: string; details: Record<string, unknown> | null };
export type SaleReturnRef = {
  id: string;
  doc_no: string;
  return_date: string;
  created_at: string;
  created_by: string;
  status: 'completed' | 'void';
  void_reason?: string;
  total: string;
  receivable_cut: string;
  refund: string;
  refund_method?: string;
  lines: { sale_position: number; name: string; unit: string; qty: string }[];
};
/** Nota lengkap untuk panel detail: harga daftar → jual, potongan per sumber, stok, riwayat. cost/profit hanya dengan izin sales_cost. */
export type SaleDetail = Omit<Sale, 'lines'> & {
  lines: SaleDetailLine[];
  outlet: { id: string; code: string; name: string };
  approved_by?: string;
  salesperson?: { id: string; name: string };
  tax_store_pct: string;
  tax_gov_pct: string;
  line_discount: string;
  manual_discount: string;
  voucher_amount: string;
  base_qty_total: string;
  stock: SaleStockMove[];
  events: SaleEvent[];
  /** Seluruh versi nota (asli + revisi), urut revisi. */
  /** Dokumen retur yang merujuk nota ini (aktif & batal), terbaru dulu. */
  returns: SaleReturnRef[];
  returned_total: string;
  /** Total nota − retur aktif. */
  net_total: string;
  revisions: { id: string; doc_no: string; revision: number; status: 'completed' | 'void' | 'superseded'; created_at: string; revised_at?: string; total: string; reason?: string }[];
  cost?: string;
  profit?: string;
  /** HPP barang yang kembali lewat retur aktif, dan laba nota setelah retur (izin HPP). */
  returned_cost?: string;
  profit_net?: string;
};

export type Approval = { user_id: string; pin: string };

/** Model struk dari server (FR-POS-15): identitas toko + nota. Klien hanya menata letak. */
export type ReceiptStore = { tenant_name: string; outlet_code: string; outlet_name: string; address: string; phone: string; header: string; footer: string };
export type Receipt = { store: ReceiptStore; sale: Sale; local_time: string; reprints: number };

export const sales = {
  /** Edit nota = revisi baru (nomor sama + -R2…). Idempotency-Key per percobaan simpan, seperti nota baru. */
  edit: (id: string, input: SaleInput & { reason: string }, idempotencyKey: string) =>
    api<Sale>(`/sales/${id}`, { method: 'PUT', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  /** Pratinjau hitung untuk edit: dampak nota lama diperhitungkan, harga baris lama dipertahankan; tidak menyimpan apa pun. */
  quoteEdit: (id: string, input: Omit<SaleInput, 'payments'>) => api<Quote>(`/sales/${id}/quote`, { method: 'POST', body: JSON.stringify(input) }),
  /** Batalkan nota (alasan + PIN penyetuju); stok, poin, dan kupon dibalik. */
  void: (id: string, input: { reason: string; approval: Approval }) => api<Sale>(`/sales/${id}/void`, { method: 'POST', body: JSON.stringify(input) }),
  detail: (id: string) => api<SaleDetail>(`/sales/${id}/detail`),
  listAll: (p: SaleAllParams) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v && k !== 'allOutlets') qs.set(k, String(v));
    if (p.allOutlets) qs.set('scope', 'all');
    return api<SaleAllList>(`/sales/all?${qs}`);
  },
  list: (p: { from?: string; to?: string; q?: string }) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) if (v) qs.set(k, v);
    return api<SaleList>(`/sales/?${qs}`);
  },
  quote: (input: Omit<SaleInput, 'payments'>) => api<Quote>('/sales/quote', { method: 'POST', body: JSON.stringify(input) }),
  create: (input: SaleInput, idempotencyKey: string) =>
    api<Sale>('/sales/', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }),
  get: (id: string) => api<Sale>(`/sales/${id}`),
  receipt: (id: string) => api<Receipt>(`/sales/${id}/receipt`),
  /** Catat satu cetak ulang (audit) dan dapatkan nomor salinannya. */
  reprint: (id: string) => api<{ copy: number }>(`/sales/${id}/receipt/reprint`, { method: 'POST' })
};

/** Kunci acak per percobaan bayar; sama selama modal bayar terbuka agar klik ganda/jaringan putus tidak membuat nota ganda. */
export const newIdempotencyKey = () => `pos-${crypto.randomUUID()}`;
