# Arsitektur, Desain Data, Struktur Repo, Template UI

> Dipindah dari AGENTS.md §4, §5, §5b. Baca saat membuat modul/tabel baru atau mem-porting halaman template.

## 4. Desain Data Target (draf — boleh disempurnakan, tapi pertahankan invariant §3)

Komentar `/* LEGACY */` di bawah hanya padanan istilah untuk membaca kode lama — **bukan** pemetaan migrasi; skema bebas didesain ulang. `stock_movements.ref_type` mencakup `OPENING` (saldo awal), `SALE`, `SALE_VOID`, `SALE_RETURN`, `PURCHASE`, `PURCHASE_RETURN`, `OPNAME`, `TRANSFER_OUT/IN`, `UNIT_CONVERSION`. Piutang/hutang saldo awal = dokumen bertipe `OPENING` tanpa nota sumber.

```
tenants(id uuid pk, code text unique /*= legacy KODEUNIKMEMBER*/, name, ...)
outlets(id, tenant_id fk, code /*= legacy KODEOUTLET/LOKASI*/, unique(tenant_id, code), tax_store_pct, tax_gov_pct)
users(id, tenant_id, username, password_hash, pin_hash, role_id, active, unique(tenant_id, username))
roles(id, tenant_id, name, permissions jsonb)      -- legacy 01_tms_penggunaaplikasiha.JSONMENU
items(id, tenant_id, sku /*= BARANG_ID*/, barcode, name, unit_id, category_id, brand_id, principal_id, supplier_id,
      kind enum(goods, service) /*JENISBARANG: 'BUKAN JASA' = goods*/, allow_negative_stock bool /*STOKDAPATMINUS*/,
      sell_below_cost bool /*JUAL_DIBAWAH_HPP*/, unique(tenant_id, sku))
item_prices / wholesale_tiers                      -- legacy HARGAJUAL, 01_tms_bestbuybaranggrosir(_new)
customers(id, tenant_id, code /*MEMBER_ID*/, points numeric, points_divisor numeric /*MINIMALPOIN*/, ...)
sales(id, tenant_id, outlet_id, doc_no, cashier_id, customer_id, sales_person_id, payment_type, status,
      subtotal, discount, tax_store, tax_gov, other_cost, total, change, due_date, created_at,
      unique(tenant_id, doc_no))
sale_lines(id, sale_id fk, item_id fk, qty, unit_price, unit_cost /*snapshot HPP*/, discount, note, extras jsonb)
sale_payments(id, sale_id fk, method enum(cash, debit, credit_card, ewallet, transfer, receivable), amount, ref_no, bank)
stock_balances(tenant_id, outlet_id, item_id, bucket enum(display, warehouse, returns), qty, pk(tenant_id,outlet_id,item_id,bucket))
stock_movements(id, tenant_id, outlet_id, item_id, bucket, qty_delta, ref_type, ref_id, actor_id, created_at)  -- partisi per bulan
receivables / receivable_payments, payables / payable_payments   -- saldo = total - sum(payments), BUKAN kolom yang diubah trigger
audit_log(id, tenant_id, actor_id, entity, entity_id, field, old, new, at)   -- pengganti 01_log_barangkharisma
```
Catatan: legacy menyimpan stok per outlet dalam **3 bucket**: `DISPLAY`, `GUDANG`, `RETUR`. Penjualan hanya mengurangi `DISPLAY`. Mutasi bisa memindah antar bucket dan antar outlet.

## 4b. Akuntansi SIAK (draf rancangan 2026-10-11 — disetujui pengguna: dokumen dulu)

Padanan legacy (`SiakData.js`, `01_siak_*`) hanya untuk memahami aturan; **tidak dimigrasikan**. Legacy = akuntansi **manual** (JU/KM/KK/TK); jurnal otomatis dari penjualan/pembelian tidak pernah ada → itu fitur baru. Kelemahan yang tidak disalin: edit/hapus jurnal via DELETE+INSERT, saldo `SUM` mentah, `double`, `DATE(waktu)` mematikan indeks, tanpa FK/UNIQUE/cek debit=kredit, neraca separuh jadi.

```
accounts(id, tenant_id, parent_id fk null, code, name, kind enum(group, ledger), normal_side enum(debit, credit),
         class enum(asset, liability, equity, revenue, cogs, expense), is_cash_bank bool, active, unique(tenant_id, code))
accounting_periods(id, tenant_id, start_date, end_date, status enum(open, closed), closed_at, closed_by)  -- 1 baris = 1 bulan kalender (CHECK), unique(tenant_id, start_date), dibuat otomatis saat pertama dipakai
journal_entries(id, tenant_id, outlet_id, doc_no /*null selama draf; diberikan saat posting, mis. JU/2026/000001*/, entry_date date, type enum(JU, KM, KK, TK, SALES, PURCHASE, AR, AP, OPENING),
                status enum(draft, posted), narration, source_type, source_ref, reverses_id fk null, created_by, posted_by, posted_at,
                unique(tenant_id, doc_no), unique(tenant_id, source_type, source_ref) /*idempotensi jurnal otomatis*/)
journal_lines(id, entry_id fk, tenant_id, account_id fk, debit numeric(18,2), credit numeric(18,2), line_no, memo,
              check ((debit = 0) <> (credit = 0)), index (tenant_id, account_id, entry_date, seq))
account_period_balances(tenant_id, outlet_id, account_id, period_month date, debit, credit, pk(...))   -- agregat, diupdate di transaksi posting
account_mappings(tenant_id, key text /*sales_revenue, cash, receivable, cogs, inventory, tax_out, payment_method:<id>, fee:<id> ...*/, account_id)
```

Aturan (diuji di service Go, bukan trigger):
1. Entri `posted` **immutable**; koreksi = jurnal balik (`reverses_id`, satu per jurnal). Draf boleh diedit/dihapus. Dijaga juga di DB: policy RLS RESTRICTIVE (UPDATE/DELETE header hanya `draft`; baris jurnal hanya bisa ditambah/dihapus saat header draf, tanpa hak UPDATE).
2. Posting wajib Σdebit = Σkredit, ≥ 2 baris, semua akun `ledger` & aktif, tanggal jatuh di periode `open`.
3. Akun tak bisa dihapus bila punya anak atau baris jurnal (nonaktifkan saja). Kode akun unik per tenant; saldo normal diturunkan dari `class`.
4. Tutup buku mengunci periode (tak ada posting/balik ke tanggal itu); buka kembali hanya izin khusus + audit. Tutup tahun: jurnal penutup pendapatan/beban → laba ditahan.
5. Saldo awal = jurnal `OPENING` di tanggal awal periode pertama (bukan kolom di COA seperti legacy); wajib seimbang.
6. Pemetaan akun per **metode bayar** (`account_mappings`), bukan per jenis (PRD FR-ACC-02).
7. **Jurnal otomatis (fase C):** penjualan **diringkas per outlet per hari** (bukan per nota → hindari jutaan baris): satu entri `SALES` berkunci `(source_type, source_ref = outlet+tanggal)`, dibuat/diperbarui idempoten dari agregat nota pada tanggal itu; bisa ditelusuri ke nota sumber lewat laporan penjualan. Pembelian, pembayaran piutang/hutang, dan retur terjurnal per dokumen di transaksi yang sama dengan sumbernya (prinsip §3.2). Penjualan hari yang periodenya sudah ditutup tidak diubah (selisih masuk periode terbuka berikutnya).
8. Big data: buku besar keyset `(account_id, entry_date, seq)`; saldo dari `account_period_balances` + mutasi periode berjalan; neraca/laba rugi = agregat per akun, tanpa memindai `journal_lines`.

Fase: **A** COA (+template retail) + periode + jurnal manual (JU/KM/KK/TK) + buku besar → **B** neraca saldo, laba rugi, neraca (baca-saja) → **C** jurnal otomatis + pemetaan akun.

## 5. Struktur Repo Target

```
aciraba_newgen/
  AGENTS.md                ← file ini (sumber tunggal; dibaca native oleh Codex & Cursor)
  CLAUDE.md                ← pointer `@AGENTS.md` untuk Claude Code
  .cursor/rules/aciraba.mdc ← pointer alwaysApply untuk Cursor (cadangan)
  docs/PRD.md              ← Product Requirements Document (scope, FR/NFR, rilis R0–R5)
  docs/business-rules.md   ← (Fase 1) aturan perhitungan & daftar fitur IN/OUT scope
  source/                  ← ROOT seluruh source code aplikasi (semua di bawah ini)
    backend/                 Go module
      cmd/api/               main.go
      internal/platform/     config, db (pgx pool, tx helper), redis, httpx (router, errors, middleware auth/tenant/idempotency), logger
      internal/<modul>/      handler.go · service.go · queries.sql (sqlc) · service_test.go
                             modul: auth, authz, audit, iam, outlet, tenant, catalog, customer, sales, purchasing, stock, receivable, payable, ledger(siak), resto, acipay, report
      db/migrations/         goose *.sql
      sqlc.yaml
    web/                     SvelteKit SPA — src/routes/(auth)|(kasir)|(admin)|(laporan), src/lib/api, src/lib/stores
    print-agent/             Go — ESC/POS lokal
    mobile/                  Flutter — ARUS Mobile (paket arus_mobile, Android): lib/core · lib/features/<modul> · lib/shell; aturan di source/mobile/AGENTS.md
    tools/                   skrip bantu dev (seed data demo, generator template import)
    tests/spec/              contoh kasus perhitungan (YAML: input → expected) yang disetujui pengguna
    deploy/                  docker-compose.yml (postgres, redis, api, web), .env.example
```

## 5b. Template UI → Modul (acuan visual di `reference/template/`)

| Modul NewGen | Halaman template |
|---|---|
| Shell/layout, dashboard | `index.html`, `analytics-ecommerce.html`, `sales-analytics.html` |
| Auth | `login.html`, `register.html`, `forgot-password.html`, `reset-password.html`, `verify-otp.html`, `lock-screen.html` |
| User & hak akses | `user-directory.html`, `permission-matrix.html`, `role-builder.html`, `audit-log.html`, `activity-log.html` |
| Master barang | `products-list.html`, `product-add.html`, `categories.html`, `suppliers.html`, `customer-add.html` |
| Kasir/POS | `restaurant-pos.html` (adaptasi untuk retail + resto), `order-summary-report.html`, `invoice-workspace.html` |
| Stok | `inventory.html`, `stock-transfer.html`, `stock-movement-report.html`, `stock-summary-report.html` |
| Pembelian | `purchase-orders.html`, `purchase-order-report.html`, `supplier-performance-report.html` |
| Resto/KDS | `table-floor-map.html`, `floor-layout.html`, `kds-queue.html`, `order-board.html`, `menu-builder.html`, `reservation-timeline.html` |
| Akuntansi SIAK | `accounting-dashboard.html`, `ledger-explorer.html`, `finance-dashboard.html`, `expenses-list.html` |
| Laporan | `sales-report.html`, `revenue-report.html`, `product-performance-report.html`, `discount-coupon-report.html`, `report-builder.html` |
| Komponen dasar | `ui-*.html`, `form-*.html`, `tables-basic.html`, `data-tables.html`, `error-404.html`, `error-500.html` |

**Cara melihat template:**
- **Membaca markup/class (cukup untuk porting):** baca file `.html` langsung sebagai teks — tidak perlu server.
- **Melihat tampilan visual:** template wajib dilayani lewat HTTP (script `type="module"` tidak jalan via `file://`). Jalankan dari root repo:
  `npx --yes serve reference/template -l 4321` → buka `http://localhost:4321/<halaman>` (URL bersih juga bisa, mis. `/restaurant-pos`).
  Claude Code: sudah ada konfigurasi `template` di `.claude/launch.json` (pakai preview/browser pane). Tool lain: jalankan perintah di atas di terminal.
- Jangan mengedit file di `reference/template/` — itu acuan, bukan kode produk.

Cara porting: buka HTML halaman terkait → salin struktur & class Tailwind ke komponen `.svelte` → ganti data statis dengan data API → interaksi (modal, dropdown, tabs) pakai bits-ui/Svelte, bukan JS template. Buat komponen bersama dulu di `web/src/lib/ui/` (Button, Card, Table, Modal, Input, Badge) sebelum halaman.

