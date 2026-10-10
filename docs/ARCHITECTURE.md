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
    mobile/                  Flutter — aplikasi kasir Android (Fase 10; belum dibuat)
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

