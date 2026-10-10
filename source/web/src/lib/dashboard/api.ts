// Klien API dasbor. Uang sebagai string desimal; bagian yang tidak boleh dilihat pemanggil tidak dikirim server (undefined).
import { api } from '#lib/api/client.ts';

export type Level = 'ok' | 'warn' | 'bad';
export type Check = { code: string; level: Level; params?: Record<string, unknown> };
export type Day = { date: string; total: string; count: number };
export type Hour = { hour: number; total: string; count: number };

export type Sales = {
  today: {
    total: string;
    count: number;
    average: string;
    voids: number;
    returns_count: number;
    returns_total: string;
    profit?: string;
    cost?: string;
    peak_hour?: number;
  };
  yesterday: Day;
  compare: { baseline_total: string; baseline_count: string; pct?: number };
  week_total: string;
  month_total: string;
  /** Bulan berjalan vs bulan lalu pada rentang tanggal & jam yang sama; pct kosong = bulan lalu belum ada penjualan. */
  month_compare: { this: string; last: string; pct?: number };
  trend: Day[];
  hourly: Hour[];
  top_items: { name: string; qty: string; revenue: string }[];
};

export type StockSample = { name: string; qty: string; min?: string; outlet?: string };

/** Ringkasan satu cabang; hanya ada di mode semua cabang. Bagian stok/shift kosong bila tak punya izin. */
export type OutletRow = {
  id: string;
  code: string;
  name: string;
  today_total: string;
  today_count: number;
  yesterday_total: string;
  week_total: string;
  voids: number;
  stock_negative?: number;
  stock_low?: number;
  stock_empty?: number;
  open_shifts?: number;
};

export type Overview = {
  scope: 'outlet' | 'all';
  outlets: number;
  local_date: string;
  generated_at: string;
  verdict: Level;
  checks: Check[];
  sales?: Sales;
  stock?: {
    tracked: number;
    active: number;
    empty: number;
    negative: number;
    /** 0 < saldo <= batas minimum barang. */
    low: number;
    /** Jumlah barang aktif yang punya batas minimum (0 = fitur belum dipakai). */
    monitored: number;
    value?: string;
    negatives: StockSample[];
    lows: StockSample[];
  };
  receivable?: { outstanding: string; overdue: string; open_count: number };
  payable?: { outstanding: string; overdue: string; open_count: number; over_30: string };
  by_outlet?: OutletRow[];
  shifts?: { open: { user: string; outlet: string; opened_at: string; long: boolean }[]; diff_count: number; diff_abs: string; closed_week: number };
};

export const dashboard = {
  overview: (all: boolean) => api<Overview>(`/dashboard/overview${all ? '?scope=all' : ''}`)
};
