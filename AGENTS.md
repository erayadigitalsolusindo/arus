# ACIRABA NewGen — Agent Handoff

> **Baca file ini dulu, seluruhnya, sebelum melakukan apa pun.** File ini menggantikan penelusuran ulang legacy.
> Jangan menjelajah ulang seluruh `../aciraba_siak_os`; buka hanya file legacy yang dirujuk untuk slice yang sedang dikerjakan.
> Di akhir setiap sesi, **perbarui §2 (Status) dan §11 (Log Sesi)**. Ringkas, faktual, tanggal absolut.

> **Aturan file instruksi:** semua konteks teknis proyek HANYA ditulis di `AGENTS.md`. `CLAUDE.md` dan `.cursor/rules/aciraba.mdc` hanya pointer — jangan isi konten di sana.
> **Kebutuhan produk (APA & MENGAPA)** ada di `docs/PRD.md` — rujuk ID kebutuhan (mis. `FR-POS-11`) saat mengerjakan fitur; jangan mengubah scope/prioritas PRD tanpa persetujuan pengguna. `AGENTS.md` = BAGAIMANA & status.

Bahasa komunikasi dengan pengguna: **Bahasa Indonesia**. Identifier kode: Bahasa Inggris (lihat §6).

---

## 1. Keputusan yang Sudah Dikunci (jangan diperdebatkan ulang)

| Area | Keputusan |
|---|---|
| Strategi | **Rewrite** di folder ini (`aciraba_newgen`). Legacy `../aciraba_siak_os` = referensi perilaku, **read-only** kecuali pengguna minta. |
| Backend | **Go**, modular monolith (bukan microservices). Router `chi`, DB `pgx/v5` + **`sqlc`**, migration `goose`, log `slog`, validasi `go-playground/validator`. |
| Database | **PostgreSQL 16+**. `NUMERIC` untuk uang/qty, `timestamptz`, FK + UNIQUE wajib, Row Level Security per tenant. Tanpa stored procedure bisnis. Trigger hanya untuk `updated_at`/audit generik. |
| Cache/Realtime | **Redis**: session/refresh token, rate limit, cache master barang/harga, Pub/Sub realtime (KDS/dashboard), idempotency key, job queue (`asynq`). **Tidak pernah** menjadi sumber kebenaran stok/uang. |
| Frontend | **SvelteKit mode SPA** (`adapter-static`) memanggil API Go langsung. TanStack Query (svelte), **Tailwind v4**, bits-ui (headless) untuk komponen interaktif, zod. **Tidak ada lapisan CI4/PHP lagi.** |
| UI/Visual | Template berbayar **Dreams Core** (Tailwind v4) di `reference/template/` = **acuan visual saja**. Yang ada adalah hasil build (HTML + asset ber-hash), bukan source. Porting ke komponen Svelte: ambil markup/class Tailwind & token warna (CSS variable `--sidebar-*` dll. di `assets/script-*.css`); **jangan** memakai JS bawaan template (vanilla/DOM manipulation). Lihat §5b. |
| Printer | Satu **print-agent Go** (binary lokal di PC kasir, ESC/POS). Menggantikan `aciraba_printlocal` (Node) dan `aciraba_printer` (Python). |
| Data | **Aplikasi baru, data baru (diputuskan 2026-10-07).** Data legacy **tidak dimigrasikan**: tidak ada ETL, tidak ada sistem paralel, tidak ada kompatibilitas akun/password/format nota/skema dengan legacy. Toko mulai lewat **onboarding**: import barang (Excel/CSV), saldo awal stok (movement `OPENING`), saldo awal piutang/hutang (PRD §7.5b, §12). Legacy dipakai **hanya untuk memahami aturan bisnis**. |
| Verifikasi | **Spec test**: contoh kasus perhitungan yang ditulis manual (input → total, pajak, kembalian, stok, poin, piutang) dan disetujui pengguna, disimpan di `source/tests/spec/`. Aturan mengikuti PRD (urutan hitung nota PRD §7.4), **bukan** meniru legacy — termasuk tidak meniru bug di §8. |

## 2. Status Saat Ini  ← PERBARUI SETIAP SESI

- **Fase:** 0 — Fondasi selesai (0.1–0.5). Fase 2 **selesai** (2.1 + 2.2): register, login, refresh (rotasi), logout, `/auth/me`, `RequireAuth`, kunci login, penjaga route SPA, **RLS Postgres**, **role & permission** (registri izin, `iam.Require`, halaman Pengguna & Role). Berikutnya Fase 3 (master data). Roadmap: §2b.
- **Toolchain di mesin pengguna (Windows 11, 2026-10-06):** Node v26 ✅, Docker ✅, **Go 1.27 ✅ (dipasang via winget; shell yang sudah terbuka perlu refresh PATH)**. psql/redis-cli lokal ❌ → pakai `docker exec`. Python ❌.
- **Infra dev:** `cd source/deploy && docker compose --env-file .env up -d` → `aciraba_postgres` (PG16, `127.0.0.1:5433`) & `aciraba_redis` (Redis 7, `127.0.0.1:6380`). Port sengaja bukan default: di mesin ini sudah ada container proyek lain (`eds_postgres` :6969, `eds_redis` :6379, folder `erp-docker/`) — jangan disentuh. Kredensial dev di `deploy/.env` & `backend/.env` (di-gitignore; contoh di `.env.example`).
- **DB dev — dua role Postgres (RLS):** `MIGRATE_DATABASE_URL` = pemilik skema (hanya goose/admin; melewati RLS), `DATABASE_URL` = role runtime `aciraba_app` (bukan pemilik, tanpa BYPASSRLS → RLS selalu berlaku; dipakai API). Migration: `cd source/backend && goose -dir db/migrations postgres "$MIGRATE_DATABASE_URL" up`. Role `aciraba_app` dibuat sekali oleh superuser (di compose lewat `deploy/initdb/01-app-role.sh`; di `eds_postgres` dibuat manual). Test integrasi butuh `TEST_DATABASE_URL` (=aciraba_app), `TEST_ADMIN_DATABASE_URL` (=pemilik), `TEST_REDIS_URL`; tanpa itu test integrasi di-skip. Kode query dari `sqlc generate` (queries di `internal/<modul>/queries.sql`, hasil di `internal/gen`, dikomit). Mesin dev memakai Postgres `eds_postgres` :6969 (db `aciraba`) & Redis `eds_redis` db 1 (lihat `backend/.env`).
- **Env backend:** `CORS_ORIGINS` (juga dipakai CSRFGuard), `JWT_SECRET` (wajib, ≥32 karakter), `TRUST_PROXY` (default false; `true` hanya di belakang reverse proxy tepercaya — mengaktifkan `X-Forwarded-For` untuk IP klien/rate limit).
- **Menjalankan:** API `cd source/backend && go run ./cmd/api` (:8080, `GET /healthz`); web: `.claude/launch.json` config `web` atau `cd source/web && npm run dev` (:5173). Test: `cd source/backend && go test ./...`, `cd source/web && npm run check`.
- **Catatan SvelteKit 3 (terpasang 3.0.1, bukan 2):** konfigurasi adapter ada di `vite.config.ts` (tidak ada `svelte.config.js`); alias `$lib` dihapus → pakai `#lib/...` **dengan ekstensi** (`#lib/nav.ts`, `#lib/components/X.svelte`) lewat `imports` di `source/web/package.json`; butuh `typescript@6`.
- **UI:** `source/web/src/lib/styles/dreams/dreams-core.css` = salinan build CSS template (token, komponen `.btn`/`.surface-card`/`.app-sidebar`, ikon lucide `icon-*` & phosphor `ph-*`) + font; 6 URL gambar demo diganti GIF 1px. Template menyembunyikan `<html>` sampai `data-theme` terpasang → di-set sinkron di `web/src/app.html`. Tailwind v4 (`@tailwindcss/vite`) hanya menambah utilitas baru. Logo ACIRABA = placeholder SVG di `web/static/`.
- **Langkah berikutnya:**
  1. Fase 3 (master data). **Aturan wajib untuk setiap tabel bertenant baru:** kolom `tenant_id` + `ENABLE ROW LEVEL SECURITY` + policy `tenant_isolation` (lihat `00004_rls.sql`) + test isolasi (`platform/db/rls_test.go` sebagai contoh); query lewat `db.WithTenant(ctx, pool, tid, fn)` dengan `tid` dari token; endpoint dipasang `httpx.RequireAuth` → `resolver.Authenticate` → `iam.Require("<modul>", iam.ActXxx)`; modul baru didaftarkan di `internal/iam/permissions.go` (+ `nav.ts` `module`, kamus `iam.module.<id>`).
  2. Sisa kecil Fase 2: verifikasi email/OTP, lupa/reset password, syarat layanan, trial/paket (belum diputuskan; register langsung aktif); pindah outlet (butuh endpoint token baru); pencabutan seluruh sesi refresh saat reset password/penonaktifan (butuh indeks sesi per user di Redis); audit log (§4 `audit_log`).
  3. Fase 1 (aturan bisnis + spec test) bisa paralel.
- **Belum dikerjakan:** daftar fitur IN/OUT scope, `docs/business-rules.md`, spec test.

## 2b. Roadmap Bertahap  ← centang `[x]` saat selesai; satu sesi = satu kotak (atau sebagian)

Aturan: kerjakan berurutan; jangan lompat fase tanpa persetujuan pengguna. Setiap kotak selesai = bisa dijalankan + dites + commit.

**Fase 0 — Fondasi (tanpa fitur bisnis)**
- [x] 0.1 `git init`, `.gitignore` (termasuk `.env`, `reference/template/` bila repo akan publik — template berlisensi), struktur folder §5
- [x] 0.2 `source/deploy/docker-compose.yml`: postgres 16, redis 7; `.env.example`
- [x] 0.3 Install Go (atau dev container `golang`), `source/backend/` go module, `cmd/api` + `/healthz` (cek DB & Redis)
- [x] 0.4 goose + sqlc terpasang; migration pertama: `tenants`, `outlets`, `users`, `roles`
- [x] 0.5 `source/web/` SvelteKit SPA + Tailwind v4; layout shell (sidebar/topbar) diport dari `index.html` template; halaman `login` dari `login.html`

**Fase 1 — Aturan bisnis & spec test (paralel dengan Fase 0, tanpa menulis kode produk)**
- [ ] 1.1 Daftar fitur/menu legacy (dari `Routes.php` CI4 + router Node) → tanya pengguna mana yang dipakai → tandai IN/OUT scope di `docs/business-rules.md`
- [ ] 1.2 Rangkum aturan perhitungan dari legacy (harga grosir, diskon, potongan, pajak toko/negara, biaya lain, pembulatan `PEMBULATANANGKA`, poin, piutang/DP, retur) ke `docs/business-rules.md`; tandai yang perlu keputusan pengguna (PRD Q6)
- [ ] 1.3 Tulis 15–20 contoh kasus (tunai, split, kredit, grosir, diskon, pajak, biaya lain, edit, void, retur, saldo awal) dengan hasil yang dihitung manual → `source/tests/spec/*.yaml`; minta persetujuan pengguna

**Fase 2 — Auth & Tenant**
- [x] 2.1 Login email + password (akun baru, bukan migrasi), JWT akses + refresh di Redis, middleware tenant/outlet, RLS — selesai 2026-10-07 (sisa kecil: lihat §2 langkah 2)
- [x] 2.2 Role & permission, UI Pengguna & Role — selesai 2026-10-07 (registri izin baru per menu sidebar, bukan port 1:1 `JSONMENU`; UI mengikuti `permission-matrix.html`)

**Fase 3 — Master data**
- [ ] 3.1 Satuan, kategori, brand, principal, supplier
- [ ] 3.2 Barang + harga + grosir + diskon (UI `products-list.html`, `product-add.html`)
- [ ] 3.3 Customer/member + poin
- [ ] 3.4 Import barang & master pendukung dari Excel/CSV: template unduhan, pratinjau, validasi per baris, laporan error (PRD FR-ONB-01/02)

**Fase 4 — Stok (inti)**
- [ ] 4.1 `stock_balances` + `stock_movements` (3 bucket), service + test konkuren (2 kasir jual barang sama)
- [ ] 4.2 Saldo awal stok (movement `OPENING`, manual + import) + kunci tanggal mulai operasional (FR-ONB-03, FR-ONB-07)
- [ ] 4.3 Opname, mutasi antar outlet/bucket (UI `stock-transfer.html`), pecah satuan, kartu stok (UI `stock-movement-report.html`)

**Fase 5 — Kasir/Penjualan (POS)**
- [ ] 5.1 API simpan penjualan: harga dihitung server, idempotency, multi-payment, piutang otomatis, poin (tanpa bug §8)
- [ ] 5.2 UI kasir (acuan `restaurant-pos.html`), keyboard-first; lulus spec test penjualan
- [ ] 5.3 Edit/void nota, retur penjualan, pembayaran piutang, saldo awal piutang (FR-ONB-04)
- [ ] 5.4 print-agent Go + cetak struk

**Fase 6 — Pembelian**: PO/pembelian, retur beli, hutang & pembayaran, saldo awal hutang (FR-ONB-05) (UI `purchase-orders.html`, `suppliers.html`)

**Fase 7 — Laporan**: penjualan, pembelian, stok, piutang/hutang (UI `sales-report.html`, `stock-summary-report.html`, dll.) — query langsung/materialized view, bukan SP generik

**Fase 8 — Modul opsional (sesuai scope 1.1)**: Resto/KDS (`table-floor-map.html`, `kds-queue.html`, Redis Pub/Sub), SIAK akuntansi (`ledger-explorer.html`, `accounting-dashboard.html`), Acipay, payment gateway

**Fase 9 — Onboarding & go-live (tanpa migrasi data, PRD §12)**
- [ ] 9.1 Wizard setup tenant: profil → outlet (pajak, zona waktu, format nota) → user & role → import barang → saldo awal (FR-ONB-06)
- [ ] 9.2 Checklist & panduan go-live toko pilot (opname malam H-1, ekspor laporan legacy untuk histori, pelatihan, rencana mundur)
- [ ] 9.3 Toko pilot go-live; perbaikan minggu pertama; lalu onboarding toko lain bertahap

## 3. Prinsip Desain (invariant — wajib dipatuhi di semua modul)

1. **Tenant, outlet, user diambil dari token di server.** Request body tidak boleh membawa `tenant_id`/`outlet_id` sebagai sumber kebenaran. Setiap query difilter `tenant_id`; RLS sebagai lapis kedua.
2. **Satu use-case = satu transaksi DB** (`pgx.BeginTxFunc`). Efek samping eksternal (Pub/Sub, print, webhook, WA) dikirim **setelah commit** (outbox atau after-commit hook).
3. **Harga dihitung di server** dari master harga/grosir/diskon. Klien hanya kirim `item_id` + `qty` (+ override harga yang wajib lolos cek permission + PIN, tercatat di audit).
4. **Stok = ledger.** `stock_movements` append-only + `stock_balances` diupdate pada transaksi yang sama dengan guard `WHERE qty >= $n` (kecuali item boleh minus). Kartu stok dibaca dari movements.
5. **Idempotensi** untuk endpoint yang membuat transaksi (`Idempotency-Key` header, disimpan Redis + UNIQUE di DB).
6. **Kunci bisnis dijaga DB**: UNIQUE `(tenant_id, document_no)`, FK ke master. Jangan pakai pola SELECT-lalu-INSERT untuk cek duplikat.
7. **Uang tidak pernah float.** Go: `shopspring/decimal` atau integer rupiah; Postgres: `NUMERIC(18,2)`; qty `NUMERIC(18,3)`.
8. **Semua aturan bisnis ada di service Go** (bisa dites), bukan di trigger/SP.
9. **Tanpa secret di repo.** `.env` di-gitignore; sediakan `.env.example`. Jangan salin kredensial/dump pelanggan dari legacy (legacy pernah meng-commit file service account Firebase/Google — jangan dibaca/disalin).

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
                             modul: auth, tenant, catalog, customer, sales, purchasing, stock, receivable, payable, ledger(siak), resto, acipay, report
      db/migrations/         goose *.sql
      sqlc.yaml
    web/                     SvelteKit SPA — src/routes/(auth)|(kasir)|(admin)|(laporan), src/lib/api, src/lib/stores
    print-agent/             Go — ESC/POS lokal
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

## 6. Konvensi

- Nama tabel/kolom/kode: **Inggris, snake_case** (`sale_lines`, `tenant_id`). Padanan istilah legacy cukup di §4/§7.
- Teks UI dan pesan error untuk pengguna akhir: **multi-bahasa (id = sumber kebenaran + fallback, en)**. **Dilarang menulis teks UI langsung di komponen** — pakai `t('domain.kunci')` dari `#lib/i18n/index.ts`. Kamus di `web/src/lib/i18n/messages/{id,en}/<domain>.ts`; `id` menentukan bentuk, bahasa lain bertipe `Messages` (kunci kurang/typo gagal di `npm run check`). Modul baru = satu file domain per bahasa + daftarkan di `index.ts`. Plural: `{ one, other }` + param `count`. Angka/uang/tanggal lewat `formatNumber/formatCurrency/formatDate/formatDateTime` (bukan `toLocaleString` manual). Error API: terjemahan lewat `errors.<CODE>` (`errorMessage()`), server cukup mengirim `code` stabil; klien mengirim `Accept-Language`. Bahasa baru: tambah di `locales.ts` + folder kamus.
- Error API: JSON `{ "error": { "code": "STOCK_INSUFFICIENT", "message": "..." } }` + HTTP status yang benar (bukan selalu 200 seperti legacy).
- Auth: access token JWT pendek (≤15 menit) + refresh token di Redis (httpOnly cookie). Klaim: `sub`, `tid` (tenant), `oid` (outlet aktif), `role`.
- Setiap modul baru wajib punya test untuk invariant-nya (stok tidak minus, total = Σ lines − diskon + pajak + biaya lain, idempotensi).

## 7. Peta Legacy (buka hanya yang relevan)

Tujuan membuka legacy: **memahami aturan bisnis**, bukan menyalin skema atau data.

Root legacy: `../aciraba_siak_os/`. Stack: CI4 (`aciraba_website`) → curl → Express/mysql2 (`aciraba_server`, port 1111; `apiaciraba_public.js` port 1112) → MySQL 8 (`kotakcantik.sql`, dump Navicat).

| Butuh perilaku apa | Buka |
|---|---|
| Simpan/edit penjualan, potong stok, kartu stok | `aciraba_server/model/PenjualanData.js` → `simpantransaksi` (≈ baris 579–900) |
| Input yang dikirim kasir saat checkout | `aciraba_website/app/Controllers/Penjualan.php` → `simpantransaksi` (≈ baris 1181) |
| Keranjang kasir (ditulis langsung PHP→MySQL) | `aciraba_website/app/Models/KasirModel.php`; juga `PenjualanModel.php`, `PembelianModel.php`, `PenyesuaianModel.php` |
| Pembelian, retur beli, hutang | `aciraba_server/model/PembelianData.js` |
| Opname/penyesuaian | `aciraba_server/model/PenyesuaianData.js` |
| Login, registrasi, OTP | `aciraba_server/model/Auth.js`, `aciraba_website/app/Controllers/Auth.php` |
| Hak akses UI | `aciraba_website/app/Filters/CheckPermission.php`, `app/Helpers/HakAksesHelper.php`, `app/Config/Routes.php` |
| Query generik `KONDISI`/`DIMANA1..N` | `aciraba_server/model/SPMysql.js` + body `proc_Controller_{Website,Admin,Kasir,API,Report}` di `kotakcantik.sql` (cari `CREATE DEFINER`). 178 cabang `kondisi = N`. |
| Laporan | `aciraba_server/model/LaporanData.js` + `proc_Controller_Report` |
| Akuntansi SIAK | `aciraba_server/model/SiakData.js`, tabel `01_siak_*` |
| Resto/KDS + realtime | `aciraba_server/model/RestoData.js`, event Socket.IO `NOTIFDINEINTAKEAWAY{LOKASI}{KODEUNIKMEMBER}` |
| Acipay / payment gateway | `aciraba_server/model/Acipay.js`, `Digiflazz.js`, `config/api/{digiflazz,duitku,midtrans,tripay,wasender}.js` |

**Skema legacy (fakta terverifikasi dari dump 2026-10-06):** 78 tabel InnoDB, 193 indeks, 0 foreign key, 75/78 tabel tanpa UNIQUE selain PK, 5 SP, 8 function, 22 trigger. Prefix: `01_tms_*` master/saldo, `01_trs_*` transaksi, `01_siak_*` akuntansi, `01_acipay_*` produk digital. Tenant = `KODEUNIKMEMBER`, outlet = `LOKASI`/`OUTLET`/`KODEAKUN` (nama kolom tidak konsisten). Tabel inti volume: `01_trs_kartustok` (~9,9 jt baris), `01_trs_kartustok_new` (~8 jt, sisa), `01_trs_barangkeluar_detail` (~3,3 jt), `01_trs_barangkeluar` (~430 rb). Master barang = `01_tms_barangkharisma`. Stok = `01_tms_stok` (kolom `DISPLAY`,`GUDANG`,`RETUR`; **tanpa UNIQUE** tenant+outlet+barang). Tabel keranjang `01_tms_keranjang`, `_pending`, `_barangmasuk` **tidak ada di dump** (dipakai PHP).

## 8. Aturan Bisnis Legacy yang Tersembunyi di Trigger (harus diimplementasi ulang di service Go)

| Trigger legacy | Perilaku | Status di NewGen |
|---|---|---|
| `AFTERINSERTTRX` on `01_trs_barangkeluar` | Jika `ENUM_JENISTRANSAKSI='KREDIT'` → buat piutang, total = `-KEMBALIAN`, jatuh tempo = now + `JATUHTEMPO` hari. Jika `TIPETRANSAKSI=1` → catat DP. Tambah poin member `ROUND(TOTALBELANJA / MINIMALPOIN)`. | Implementasi ulang. **Perbaiki:** DP kolom kredit legacy memakai `NOMORKARTUKREDIT` (bug, harusnya nominal); bagi-nol bila `MINIMALPOIN=0`. |
| `AFTEREDIT` on `01_trs_barangkeluar` | Hapus & buat ulang piutang; **tambah poin lagi tanpa mengurangi poin lama**. | **Bug legacy:** poin menggelembung setiap edit; piutang yang sudah dibayar sebagian kembali penuh. Desain baru: hitung selisih; buat spec test khusus edit nota. |
| `AFTERDELETE` on `01_trs_barangkeluar` | Hapus beban, DP, pesanan meja; kurangi poin. | Implementasi ulang (lebih baik: void/soft-cancel + movement balik, bukan delete). |
| `AFTERINESRT` / `AFTERUBAHPIUTANG` / `AFTERDELETEPIUTANG` on `01_tms_piutangkredit_detail` | `SISAKREDIT` ± `BAYAR`. (`AFTERUBAHPIUTANG` tanpa filter tenant.) | Saldo dihitung dari pembayaran, bukan kolom yang dimutasi. |
| `AFIHUTANG` / `AUPBHUTANG` / `ADELBHUTANG` on `01_tms_hutangtoko_detail` | Sama untuk hutang supplier. | Idem. |
| `AIMUTASIBARANGD` on `01_trs_mutasibarang_detail` | Kurangi bucket asal (D/G/R) di outlet asal, tambah bucket tujuan di outlet tujuan (insert jika belum ada), tulis kartu stok. | Implementasi di modul stock (transfer). |
| `AFTER_INSERT` on `01_tmp_pecahsatuan` | Pecah satuan: kurangi `DISPLAY` barang asal, tambah `DISPLAY` barang baru, tulis kartu stok (`PS…`). | Implementasi di modul stock (unit conversion). |
| `UpdateLogBarang` on `01_tms_barangkharisma` | Audit per kolom ke `01_log_barangkharisma` jika `TERAKHIR_UBAH` terisi. Kolom `STOKDAPATMINUS`/`PEMILIK` menulis ke tabel `BARANG_LOG` yang **tidak ada di dump**. | Ganti dengan `audit_log` generik. |
| `*_bestbuybaranggrosir*_AFTER_INSERT/DELETE` | Set `APAKAHGROSIR` AKTIF/TIDAK AKTIF di master barang. | Turunan: cukup cek ada/tidaknya tier grosir. |
| `01_tms_member_AFTER_INSERT/DELETE` | Buat/hapus baris deposit member (tanpa filter tenant). | Deposit = tabel transaksi sendiri. |
| `01_tms_penggunaaplikasiha_AFTER_DELETE` | User dengan role terhapus → role `DEFAULT`. | FK `ON DELETE` + aturan service. |
| `AFDELETE` on `01_tms_resto_daftarmeja` | Hapus pesanan meja terkait. | FK cascade / cegah hapus meja aktif. |

## 9. Kelemahan Legacy yang **Tidak Boleh** Disalin

(Hasil inspeksi kode, belum direproduksi sebagai exploit.)
- API Node tanpa auth: `tokenAPIMiddleware` hanya dirujuk di `routes/authrouter.js` dan dikomentari; CORS `*`; rate limit 100 rb/10 menit.
- Tenant/kasir/lokasi diambil dari POST browser (`getPost('KODEUNIKMEMBER')`), tidak konsisten dengan session.
- `BEGIN/COMMIT` via `pool.query()` → transaksi lintas koneksi (tidak atomik). Ada di Penjualan, Pembelian, Auth, MasterData, Penyesuaian.
- Saldo kartu stok dihitung di memori dari SELECT sebelumnya → race antar kasir.
- SQL injection: `KasirModel.php` (konkatenasi string), `handleUpdateBarangKeluar` (CASE WHEN dengan nilai disisipkan).
- Variabel global implisit di Node (`cekHash`, `passplain`, `pesanbalik` level modul) → bocor antar-request; `bacaPINTrx` mengembalikan hash PIN.
- Harga jual/beli dikirim klien dan disimpan apa adanya.
- `UPDATE 01_trs_barangkeluar ... WHERE PK_NOTAPENJUALAN = ?` tanpa filter tenant.
- Uang `DOUBLE`; tanggal sebagai `BIGINT` di beberapa tabel; `LIKE '%x%'` 512× di SP; fungsi skalar stok per baris.
- UNIQUE salah cakupan: `01_tms_diskon(BARANG_ID)`, `01_tms_voucherbarang(NAMAVOUCHER)` tanpa tenant.
- Kunci AES & salt hardcode di `aciraba_server/config/utils.js`; OTP period 1 detik dengan secret yang bisa ditebak.
- CSRF CI4 nonaktif.

## 10. Pertanyaan Terbuka (tanyakan ke pengguna bila relevan, jangan diasumsikan)

- Apakah kasir harus bisa **offline**? (Mempengaruhi desain SvelteKit: IndexedDB queue + idempotency.)
- Modul mana yang masih benar-benar dipakai klien? (Acipay, payment gateway, SIAK, resto — bisa ditunda/dibuang.)
- Apakah ada **source** template Dreams Core (folder `src/`, file Tailwind config/partials)? Yang ada sekarang hasil build saja; source akan mempercepat porting.
- Hosting target: VPS tunggal vs on-premise per toko (`01_set_onpremise` ada di legacy)?
- Format nomor nota default NewGen (usulan `{OUTLET}-{YYMMDD}-{NNNN}`; tidak wajib sama dengan legacy) — PRD Q4.
- Urutan perhitungan nota (PRD §7.4) & aturan pembulatan (fungsi legacy `PEMBULATANANGKA` belum dibaca) — PRD Q6.
- Berapa lama legacy tetap bisa diakses baca-saja untuk histori toko yang sudah pindah — PRD Q6.

## 11. Log Sesi  ← TAMBAHKAN DI ATAS, terbaru dulu

- **2026-10-07 (RLS + Fase 2.2 role & permission + menu)** — **RLS (migration `00004_rls.sql`):** `ENABLE ROW LEVEL SECURITY` + policy `tenant_isolation` (`tenant_id = app_tenant_id()`, USING + WITH CHECK) di `tenants/outlets/roles/users`; `app_tenant_id()` = `NULLIF(current_setting('app.tenant_id', true), '')::uuid` (tanpa tenant → nol baris, fail closed). Tidak di-`FORCE`: pemilik skema (migration) melewati RLS; **aplikasi memakai role `aciraba_app`** (LOGIN, bukan pemilik, tanpa BYPASSRLS) sehingga RLS selalu berlaku. Helper `platform/db.WithTenant`/`SetTenant` (`set_config('app.tenant_id', tid, true)` per transaksi → tidak bocor antar koneksi pool). Pencarian akun sebelum tenant diketahui (login: email; refresh: user id dari Redis) lewat fungsi `SECURITY DEFINER` `auth_account_by_email/_by_id` (satu akun per panggilan, `search_path` dikunci, wrapper manual `auth/accounts.go` karena sqlc tak membaca `RETURNS TABLE`). Register membuat tenant dengan UUID dari aplikasi lalu seluruh transaksi berjalan di bawah tenant itu. Dua URL DB: `DATABASE_URL` (aciraba_app) dan `MIGRATE_DATABASE_URL` (pemilik); compose: `deploy/initdb/01-app-role.sh` + `POSTGRES_APP_PASSWORD`. Test: `platform/db/rls_test.go` (role bukan privileged, tanpa tenant = 0 baris, isolasi baca/ubah/hapus/sisip lintas tenant, 200 transaksi bergantian A/B tanpa bocor) — terbukti mendeteksi bila RLS dimatikan. **Role & permission (`internal/iam`):** registri izin `Modules` (id = id menu sidebar; aksi view/create/update/delete/approve per modul); `roles.permissions` = `{"*":true}` (Owner, role sistem, kebal ubah/hapus) atau `{"modul":["view",...]}`; `Normalize` (tolak modul/aksi asing & wildcard dari API; `view` otomatis menyertai aksi lain), `Covers` (cegah eskalasi: hanya boleh memberi izin yang dimiliki sendiri; tidak boleh menyentuh role/akun yang izinnya melebihi). `Resolver` membaca izin efektif **dari DB** (cache 10 dtk, dibuang saat role/user berubah) sehingga ganti role/nonaktif berlaku cepat walau token akses masih hidup; `Authenticate` (401 bila nonaktif) + `Require(modul, aksi)` (403). Endpoint `/iam/registry`, `/iam/roles` (CRUD), `/iam/assignable-roles`, `/iam/users` (list/buat/ubah), `/iam/users/{id}/password`; aturan: pemilik aktif terakhir tidak boleh dinonaktifkan/diganti (baris pemilik dikunci `FOR UPDATE`, diuji balapan), tidak boleh menonaktifkan/mengganti role akun sendiri, role terpakai tidak bisa dihapus. `Profile.permissions` ikut respons register/login/refresh/me. **Frontend:** `can()`/`requirePermission()` di `session.svelte.ts`, `nav.ts` tiap item punya `module` + `visibleNav()` (menu disaring menurut izin), halaman `/users` & `/roles` (matriks izin; checkbox di luar izin sendiri dinonaktifkan), komponen `Modal.svelte`, kamus `iam.*` (id/en) + kode error baru. **Menu sidebar** mengikuti menu legacy (Master Data, Penjualan, Pembelian, Penyesuaian + Sistem); semua submenu tertutup saat load; perbaikan tinggi baris (margin submenu tertutup dari template); blok "Outlet Sekarang" (dropdown belum berfungsi, hanya outlet aktif). **Verifikasi:** `go test ./...` hijau (iam: eskalasi, role terpakai, pemilik terakhir + balapan, isolasi tenant), `npm run check` 0 error, `npm run build` sukses; uji curl + browser (owner buat role/pegawai; pegawai hanya melihat menu miliknya, `/users` dialihkan; akun nonaktif → 401). **Belum:** lihat §2 langkah 2 (OTP/lupa password, pindah outlet, cabut semua sesi saat reset password, audit log).

- **2026-10-07 (login, sesi, lockout, guard SPA)** — **Backend (`internal/auth`):** `POST /auth/login` ({email,password,remember}; email salah/password salah/akun tak ada → satu `INVALID_CREDENTIALS` dengan hash dummy agar waktu respons sama; `ACCOUNT_DISABLED` hanya setelah password benar), `POST /auth/refresh` (rotasi refresh token; jendela grace untuk permintaan paralel → hanya token akses baru; pemakaian ulang token lama di luar grace mencabut seluruh rantai sesi), `POST /auth/logout`, `GET /auth/me` (di balik `RequireAuth`). Endpoint ber-cookie (refresh/logout) dilindungi `httpx.CSRFGuard` (cek Origin vs `CORS_ORIGINS`) + header khusus (`CSRF_HEADERS` di klien). `httpx.RequireAuth` (`platform/httpx/authmw.go`) memverifikasi Bearer JWT, membedakan `TOKEN_EXPIRED`; tenant/outlet/user hanya dari klaim (§3.1). **Lockout login** (`lockout.go`): kebijakan di tabel baru `app_settings` (migration `00003`, key `auth.login_lockout`: 5 gagal per IP+email → kunci 1/5/15/60 mnt, reset 24 jam, batas 50 gagal/jam per email dari semua IP) — bisa diubah via SQL tanpa deploy; bawaan dipakai bila nilai rusak; state di Redis. Rate limit login per IP longgar (60/15 mnt) karena toko berbagi IP. **Frontend:** `web/src/lib/auth/session.svelte.ts` (status sesi, pulihkan dari cookie refresh saat load, refresh proaktif 60 dtk sebelum token habis, logout), `(app)/+layout.ts` mengalihkan ke `/login` bila tanpa sesi, `login`/`register` `+page.ts` mengalihkan bila sudah masuk; `api()` memakai token akses di memori + retry setelah refresh; Header/Sidebar menampilkan identitas dari sesi (inisial via `initials.ts`); logo ACIRABA SVG (dengan/tanpa teks) di `web/static/`. **Verifikasi:** `go build/vet`, `go test ./...` hijau (lockout, login, authmw), `npm run check` 0 error. **Belum:** RLS, role/permission (2.2), verifikasi email/OTP, lupa password. **Rencana deploy VPS (belum dibuat file):** container Docker bind `127.0.0.1` port non-default (PG/Redis tidak dipublikasikan), reverse proxy lewat nginx aaPanel (hanya vhost) atau Cloudflare Tunnel; `TRUST_PROXY=true`; backup `pg_dump` terjadwal; perlu Dockerfile api/web + `docker-compose.prod.yml` + step migrasi goose.

- **2026-10-07 (register tenant + sanitasi input)** — **Backend:** `POST /auth/register` (`internal/auth`): satu transaksi membuat tenant (kode = slug nama + 4 hex acak, retry bila bentrok), outlet `main`, role sistem **Owner** (`{"*":true}`), user pemilik; lalu token akses JWT HS256 (15 mnt; klaim `sub/tid/oid/role`) + refresh token acak 256-bit di Redis (hanya hash SHA-256 yang jadi key) via cookie httpOnly `refresh_token` (Path `/auth`, SameSite Lax, Secure di non-dev). Respons 201 `{access_token, expires_in, user, tenant, outlet}`; error `VALIDATION` (422, `fields` = kode per field: REQUIRED/INVALID/TOO_LONG/TOO_SHORT/WEAK), `EMAIL_TAKEN` (409; dijaga UNIQUE `users_email_key`, bukan SELECT-lalu-INSERT), `RATE_LIMITED` (429; 10/jam/IP, Redis, fail-closed), `PAYLOAD_TOO_LARGE`, `UNSUPPORTED_MEDIA_TYPE`, `BAD_REQUEST`. Migration `00002`: `users.phone`, `tenants.onboarding_completed_at` (untuk wizard 9.1), CHECK panjang/format. Paket baru: `platform/auth` (argon2id m=64MiB t=3 p=2; JWT menolak alg selain HS256), `platform/sanitize` (NFC, tolak karakter kontrol/zero-width/bidi dan `< >` pada nama; email ASCII polos; HP → `+62…`; password 10–128 huruf+angka), `platform/httpx` (`DecodeJSON` ketat: Content-Type, batas 16 KB, tolak field tak dikenal/data tambahan; `SecurityHeaders`: nosniff, CSP `default-src 'none'`, no-store; `RateLimit`). Hash argon2id dibatasi 4 bersamaan. **`middleware.RealIP` kini hanya aktif bila `TRUST_PROXY=true`** (header XFF bisa dipalsukan → bypass rate limit). `sqlc.yaml`: `queries: internal/*/queries.sql` (path direktori saja tidak terbaca). **Frontend:** halaman `/register` (acuan `register.html`; form: nama bisnis, outlet pertama, pemilik, email, HP, password + konfirmasi + indikator kekuatan), `#lib/validation.ts` (aturan sama dengan server; hanya UX), `fieldMessage()` di `i18n/errors.ts`, kamus `auth.register.*` + kode error baru (id/en), `api()` memuat `fields` dan token akses **hanya di memori** (`setAccessToken`; bukan localStorage). Output selalu lewat escape Svelte — jangan pakai `{@html}` untuk data pengguna. Tanpa checkbox syarat layanan (halamannya belum ada). **Verifikasi:** `go test ./...` (unit sanitasi/hash/token; integrasi register termasuk email ganda atomik + 6 pendaftaran bersamaan → 1 menang, jalan bila `TEST_DATABASE_URL` & `TEST_REDIS_URL` di-set; data uji dibersihkan), `npm run check` 0 error, `npm run build` sukses; uji curl (XSS, tipe salah, field asing, body besar, rate limit) dan uji form di browser pane sampai masuk dasbor. Proses `api` lama di :8080 dihentikan agar build baru berjalan.

- **2026-10-07 (sesi sebelumnya: 0.4 + login)** — Fase 0.4 selesai: goose + sqlc terpasang, migration `00001_init_tenancy.sql` (`tenants`, `outlets`, `roles`, `users`; FK komposit (tenant_id, id) agar anak tak menunjuk tenant lain; email unik global case-insensitive; trigger `set_updated_at`). Tampilan login: latar ikon melayang, scanner barcode, feed transaksi, footer status server; i18n id/en (lihat entri berikutnya).

- **2026-10-07 (i18n id/en)** — Inti multi-bahasa di `source/web/src/lib/i18n/` (tanpa library: rune Svelte 5 + `Intl`): `t()` bertipe & reaktif (ganti bahasa tanpa reload), fallback ke `id`, plural, interpolasi `{param}`, formatter angka/uang/tanggal, deteksi bahasa browser (fallback `id`) + simpan di `localStorage`, `<html lang>` ikut berubah, `LanguageSwitcher` di header & footer login. Seluruh teks login, shell (header/sidebar/footer), dasbor, dan klien API dimigrasi ke kamus; `nav.ts` memakai `labelKey`. Verifikasi: `npm run check` 0 error, `npm run build` sukses, uji ganti bahasa di browser pane. Catatan: pesan error/validasi form yang sudah tampil tidak diterjemahkan ulang saat bahasa diganti (hilang pada submit berikutnya). Sisi Go belum melokalkan apa pun — cukup kirim `code` stabil.

- **2026-10-07 (keputusan data baru)** — Pengguna memutuskan NewGen = aplikasi baru dengan **data baru** (tanpa migrasi). §1 (Data, Verifikasi), §2b (Fase 1 → aturan bisnis + spec test; 2.1 tanpa kompat hash legacy; tambah 3.4 import, 4.2 saldo awal stok, saldo awal piutang/hutang di 5.3 & 6; Fase 9 → onboarding & go-live), §4, §5, §10 diperbarui. PRD → v0.2 (FR-ONB baru di §7.5b, urutan hitung nota default di §7.4, §12 strategi peluncuran tanpa ETL, Q4/Q6 diganti). Folder kosong `source/tools/legacy-etl` dihapus, `source/tests/golden` → `source/tests/spec`.

- **2026-10-07 (login = template, font)** — Form login disamakan 100% dengan `login.html` (teks Inggris, field **email**, bukan username; body `POST /auth/login` = {email, password, remember}; pesan validasi tetap Indonesia; logo ACIRABA, panel kanan teks sendiri). **Font:** `@import` Google Fonts di CSS salinan hilang saat bundling Vite → Plus Jakarta Sans tidak termuat; diganti `<link>` di `app.html` (butuh internet; untuk kasir offline nanti self-host). Ikon Google/GitHub kosong juga di template asli (font FA tidak dimuat) → dibiarkan identik. Catatan: DB dev memakai role/db `aciraba` di container `eds_postgres` (:6969), Redis `eds_redis` db 1.

- **2026-10-06 (UI mirip template)** — Header diport penuh seperti template (pencarian + Ctrl K, Buat Cepat, tema, notifikasi, chip pengguna; dropdown state-based Svelte), dasbor: breadcrumb, hero, 4 kartu KPI placeholder (tanpa angka karangan), footer. `app.css` menambah `@theme` breakpoint agar utilitas responsif baru (`sm:`/`xl:`) ter-generate tanpa menimpa token template.
- **2026-10-06 (restruktur)** — Semua source code (backend, web, print-agent, tools, tests, deploy) dipindah ke `source/` sebagai root kode; `AGENTS.md`, `CLAUDE.md`, `docs/`, `reference/`, `.claude`, `.cursor` tetap di root repo.
- **2026-10-06 (Fase 0.1–0.3, 0.5)** — Repo git + struktur §5 + `.gitignore` (`reference/template/` dan `erp-docker/` tidak ikut git). `deploy/docker-compose.yml` (PG16/Redis7, port 5433/6380). Go 1.27 dipasang; `backend/` modul `aciraba`: config (env/.env), pgx pool, go-redis, chi + middleware (request id, log slog, recover, CORS), `/healthz` (200; 503 bila dependensi down — diuji dengan mematikan Redis, plus `go test`). `web/`: SvelteKit 3 SPA + shell (sidebar/topbar) dan halaman `/login` (zod, memanggil `POST /auth/login`) dari template; diverifikasi di browser pane termasuk CORS web→API; `npm run build` sukses. 0.4 belum.

- **2026-10-06 (PRD)** — Dibuat `docs/PRD.md` v0.1 (draf): tujuan, metrik, persona, rilis R0–R5, FR per modul, NFR, migrasi, risiko, 10 pertanyaan terbuka (Q1–Q10, sama dengan §10).
- **2026-10-06 (lanjutan)** — Pengguna menambahkan template Dreams Core (Tailwind v4, 213 halaman, hasil build) di `reference/template/`. Ditambahkan keputusan UI (§1), roadmap bertahap (§2b), dan pemetaan halaman template → modul (§5b). Sesi berikutnya dimulai dari §2b kotak 0.1.

- **2026-10-06** — Analisis legacy (DB dump, alur CI4↔Node↔MySQL, keamanan). Diputuskan stack baru Go + PostgreSQL + Redis + SvelteKit SPA, migrasi per tenant dengan golden test. Isi semua trigger dibaca dan dirangkum di §8 (temuan baru: bug poin saat edit nota, tabel `BARANG_LOG` hilang, stok 3 bucket). agent.md ditulis ulang ke format ini, lalu di-rename jadi AGENTS.md + pointer CLAUDE.md & .cursor/rules. Belum ada kode.
