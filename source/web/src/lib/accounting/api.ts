// Klien API akuntansi SIAK (COA, periode, jurnal, buku besar). Semua angka uang berupa string desimal dari server.
import { api } from '#lib/api/client.ts';

export type AccClass = 'asset' | 'liability' | 'equity' | 'revenue' | 'cogs' | 'expense';
export type JournalType = 'JU' | 'KM' | 'KK' | 'TK' | 'SALES' | 'PURCHASE' | 'AR' | 'AP' | 'OPENING';

export type Account = {
  id: string;
  parent_id: string | null;
  code: string;
  name: string;
  kind: 'group' | 'ledger';
  class: AccClass;
  normal_side: 'debit' | 'credit';
  is_cash_bank: boolean;
  active: boolean;
  is_system: boolean;
};

export type AccountInput = { parent_id: string | null; code: string; name: string; kind?: 'group' | 'ledger'; class: AccClass; is_cash_bank: boolean; active?: boolean };

export type Period = { id: string; start_date: string; end_date: string; status: 'open' | 'closed'; closed_at?: string };

export type JournalLine = { line_no: number; account_id: string; account_code: string; account_name: string; debit: string; credit: string; memo: string };

export type Journal = {
  id: string;
  outlet_id: string;
  doc_no?: string;
  date: string;
  type: JournalType;
  status: 'draft' | 'posted';
  narration: string;
  reverses_id?: string;
  created_at: string;
  posted_at?: string;
  total_debit: string;
  total_credit: string;
  lines: JournalLine[];
};

export type JournalSummary = {
  id: string;
  outlet_id: string;
  doc_no: string;
  date: string;
  type: JournalType;
  status: 'draft' | 'posted';
  narration: string;
  total: string;
  lines: number;
  created_by: string;
  posted_at?: string;
  source: string;
  is_reversal: boolean;
  reversed: boolean;
  debit_account: string;
  credit_account: string;
};
export type JournalPage = { items: JournalSummary[]; next_cursor?: string };

export type LineInput = { account_id: string; debit: string; credit: string; memo: string };
export type JournalInput = { date: string; type: string; narration: string; lines: LineInput[] };

export type LedgerRow = { line_id: string; entry_id: string; date: string; doc_no: string; type: JournalType; narration: string; memo: string; debit: string; credit: string; balance: string };
export type Ledger = { account: Account; opening_balance: string; rows: LedgerRow[]; next_cursor?: string; total_debit?: string; total_credit?: string };

export type TrialRow = {
  account_id: string; code: string; name: string; class: AccClass;
  opening_debit: string; opening_credit: string; debit: string; credit: string; closing_debit: string; closing_credit: string;
};
export type TrialBalance = { from: string; to: string; rows: TrialRow[]; totals: TrialRow; balanced: boolean };

export type StatementLine = { account_id: string; code: string; name: string; amount: string };
export type StatementSection = { key: string; lines: StatementLine[]; total: string };
export type IncomeStatement = { from: string; to: string; revenue: StatementSection; cogs: StatementSection; gross_profit: string; expenses: StatementSection; net_profit: string };
export type BalanceSheet = {
  as_of: string; assets: StatementSection; liabilities: StatementSection; equity: StatementSection;
  unclosed_profit: string; total_assets: string; total_liabilities_equity: string; balanced: boolean;
};

export type CashBankRow = { account_id: string; code: string; name: string; opening: string; debit: string; credit: string; closing: string };
export type CashBank = { from: string; to: string; rows: CashBankRow[]; total: CashBankRow };

export type GeneralJournalEntry = { id: string; doc_no: string; date: string; type: JournalType; narration: string; lines: JournalLine[] };
export type GeneralJournalPage = { items: GeneralJournalEntry[]; next_cursor?: string };

const qs = (p: Record<string, string | number | undefined>) => {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '') q.set(k, String(v));
  return q.toString();
};
const json = (method: string, body?: unknown): RequestInit => ({ method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });

export const accounting = {
  accounts: () => api<{ items: Account[] }>('/accounting/accounts').then((r) => r.items),
  createAccount: (b: AccountInput) => api<Account>('/accounting/accounts', json('POST', b)),
  updateAccount: (id: string, b: AccountInput) => api<Account>(`/accounting/accounts/${id}`, json('PUT', b)),
  deleteAccount: (id: string) => api<void>(`/accounting/accounts/${id}`, json('DELETE')),
  seedRetail: () => api<{ items: Account[] }>('/accounting/accounts/seed-retail', json('POST')),

  periods: () => api<{ items: Period[] }>('/accounting/periods').then((r) => r.items),
  closePeriod: (month: string) => api<Period>(`/accounting/periods/${month}/close`, json('POST')),
  reopenPeriod: (month: string, reason: string) => api<Period>(`/accounting/periods/${month}/reopen`, json('POST', { reason })),

  journals: (p: { from: string; to: string; status?: string; type?: string; cursor?: string; limit?: number }) => api<JournalPage>(`/accounting/journals?${qs(p)}`),
  journal: (id: string) => api<Journal>(`/accounting/journals/${id}`),
  createJournal: (b: JournalInput) => api<Journal>('/accounting/journals', json('POST', b)),
  updateJournal: (id: string, b: JournalInput) => api<Journal>(`/accounting/journals/${id}`, json('PUT', b)),
  deleteJournal: (id: string) => api<void>(`/accounting/journals/${id}`, json('DELETE')),
  postJournal: (id: string) => api<Journal>(`/accounting/journals/${id}/post`, json('POST')),
  reverseJournal: (id: string, date: string, narration: string) => api<Journal>(`/accounting/journals/${id}/reverse`, json('POST', { date, narration })),
  opening: (b: { date: string; narration: string; lines: LineInput[] }) => api<Journal>('/accounting/opening', json('POST', b)),

  trialBalance: (p: { from: string; to: string }) => api<TrialBalance>(`/accounting/reports/trial-balance?${qs(p)}`),
  incomeStatement: (p: { from: string; to: string }) => api<IncomeStatement>(`/accounting/reports/income-statement?${qs(p)}`),
  balanceSheet: (asOf: string) => api<BalanceSheet>(`/accounting/reports/balance-sheet?${qs({ as_of: asOf })}`),
  cashBank: (p: { from: string; to: string }) => api<CashBank>(`/accounting/reports/cash-bank?${qs(p)}`),
  generalJournal: (p: { from: string; to: string; cursor?: string; limit?: number }) => api<GeneralJournalPage>(`/accounting/reports/general-journal?${qs(p)}`),

  ledger: (p: { account_id: string; from: string; to: string; cursor?: string; limit?: number }) => api<Ledger>(`/accounting/ledger?${qs(p)}`)
};

export const todayISO = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};
