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
| Migrasi | **Per tenant**, bukan big-bang: ETL MySQL→Postgres, jalan paralel, rekonsiliasi laporan harian, lalu cutover. |
| Verifikasi | **Golden test** dari nota nyata legacy: total, stok, poin, piutang, (jurnal) harus identik — kecuali bug legacy yang sengaja diperbaiki (§8). |

## 2. Status Saat Ini  ← PERBARUI SETIAP SESI

- **Fase:** 0 — Fondasi, sebagian selesai (0.1, 0.2, 0.3, 0.5). Sisa: **0.4** (goose + sqlc + migration pertama). Roadmap: §2b.
- **Toolchain di mesin pengguna (Windows 11, 2026-10-06):** Node v26 ✅, Docker ✅, **Go 1.27 ✅ (dipasang via winget; shell yang sudah terbuka perlu refresh PATH)**. psql/redis-cli lokal ❌ → pakai `docker exec`. Python ❌.
- **Infra dev:** `cd source/deploy && docker compose --env-file .env up -d` → `aciraba_postgres` (PG16, `127.0.0.1:5433`) & `aciraba_redis` (Redis 7, `127.0.0.1:6380`). Port sengaja bukan default: di mesin ini sudah ada container proyek lain (`eds_postgres` :6969, `eds_redis` :6379, folder `erp-docker/`) — jangan disentuh. Kredensial dev di `deploy/.env` & `backend/.env` (di-gitignore; contoh di `.env.example`).
- **Menjalankan:** API `cd source/backend && go run ./cmd/api` (:8080, `GET /healthz`); web: `.claude/launch.json` config `web` atau `cd source/web && npm run dev` (:5173). Test: `cd source/backend && go test ./...`, `cd source/web && npm run check`.
- **Catatan SvelteKit 3 (terpasang 3.0.1, bukan 2):** konfigurasi adapter ada di `vite.config.ts` (tidak ada `svelte.config.js`); alias `$lib` dihapus → pakai `#lib/...` **dengan ekstensi** (`#lib/nav.ts`, `#lib/components/X.svelte`) lewat `imports` di `source/web/package.json`; butuh `typescript@6`.
- **UI:** `source/web/src/lib/styles/dreams/dreams-core.css` = salinan build CSS template (token, komponen `.btn`/`.surface-card`/`.app-sidebar`, ikon lucide `icon-*` & phosphor `ph-*`) + font; 6 URL gambar demo diganti GIF 1px. Template menyembunyikan `<html>` sampai `data-theme` terpasang → di-set sinkron di `web/src/app.html`. Tailwind v4 (`@tailwindcss/vite`) hanya menambah utilitas baru. Logo ACIRABA = placeholder SVG di `web/static/`.
- **Langkah berikutnya:**
  1. 0.4: goose + sqlc, migration `tenants`, `outlets`, `users`, `roles` (desain §4).
  2. Fase 2.1: login nyata (`POST /auth/login`). Halaman login sudah memanggilnya; sekarang API membalas 404 `NOT_FOUND` dan UI menampilkan pesannya. Route `(app)/*` belum dijaga auth; sidebar masih placeholder "Belum masuk".
  3. Fase 1 (legacy discovery) bisa paralel.
- **Belum dikerjakan:** pemetaan `kondisi` stored procedure yang masih dipakai (§9), golden test, ETL.

## 2b. Roadmap Bertahap  ← centang `[x]` saat selesai; satu sesi = satu kotak (atau sebagian)

Aturan: kerjakan berurutan; jangan lompat fase tanpa persetujuan pengguna. Setiap kotak selesai = bisa dijalankan + dites + commit.

**Fase 0 — Fondasi (tanpa fitur bisnis)**
- [x] 0.1 `git init`, `.gitignore` (termasuk `.env`, `reference/template/` bila repo akan publik — template berlisensi), struktur folder §5
- [x] 0.2 `source/deploy/docker-compose.yml`: postgres 16, redis 7; `.env.example`
- [x] 0.3 Install Go (atau dev container `golang`), `source/backend/` go module, `cmd/api` + `/healthz` (cek DB & Redis)
- [ ] 0.4 goose + sqlc terpasang; migration pertama: `tenants`, `outlets`, `users`, `roles`
- [x] 0.5 `source/web/` SvelteKit SPA + Tailwind v4; layout shell (sidebar/topbar) diport dari `index.html` template; halaman `login` dari `login.html`

**Fase 1 — Legacy discovery (paralel dengan Fase 0, tanpa menulis kode produk)**
- [ ] 1.1 Daftar endpoint Node (205 route) + `KONDISI` SP yang benar-benar dipanggil CI4 → `docs/legacy-map.md`
- [ ] 1.2 Tanyakan ke pengguna modul yang masih dipakai klien (§10) → tandai IN/OUT scope
- [ ] 1.3 Ambil 10–20 nota nyata (tunai, kredit, split, retur, edit, grosir, diskon, resto) dari DB legacy → `source/tests/golden/` (anonimkan data pelanggan)

**Fase 2 — Auth & Tenant**
- [ ] 2.1 Login (bcrypt kompatibel hash legacy), JWT akses + refresh di Redis, middleware tenant/outlet, RLS
- [ ] 2.2 Role & permission (port dari `JSONMENU`), UI dari `permission-matrix.html` / `role-builder.html`

**Fase 3 — Master data**
- [ ] 3.1 Satuan, kategori, brand, principal, supplier
- [ ] 3.2 Barang + harga + grosir + diskon (UI `products-list.html`, `product-add.html`)
- [ ] 3.3 Customer/member + poin

**Fase 4 — Stok (inti)**
- [ ] 4.1 `stock_balances` + `stock_movements` (3 bucket), service + test konkuren (2 kasir jual barang sama)
- [ ] 4.2 Opname, mutasi antar outlet/bucket (UI `stock-transfer.html`), pecah satuan, kartu stok (UI `stock-movement-report.html`)

**Fase 5 — Kasir/Penjualan (POS)**
- [ ] 5.1 API simpan penjualan: harga dihitung server, idempotency, multi-payment, piutang otomatis, poin (tanpa bug §8)
- [ ] 5.2 UI kasir (acuan `restaurant-pos.html`), keyboard-first; lulus golden test penjualan
- [ ] 5.3 Edit/void nota, retur penjualan, pembayaran piutang
- [ ] 5.4 print-agent Go + cetak struk

**Fase 6 — Pembelian**: PO/pembelian, retur beli, hutang & pembayaran (UI `purchase-orders.html`, `suppliers.html`)

**Fase 7 — Laporan**: penjualan, pembelian, stok, piutang/hutang (UI `sales-report.html`, `stock-summary-report.html`, dll.) — query langsung/materialized view, bukan SP generik

**Fase 8 — Modul opsional (sesuai scope 1.2)**: Resto/KDS (`table-floor-map.html`, `kds-queue.html`, Redis Pub/Sub), SIAK akuntansi (`ledger-explorer.html`, `accounting-dashboard.html`), Acipay, payment gateway

**Fase 9 — Migrasi data & cutover**
- [ ] 9.1 `source/tools/legacy-etl`: MySQL → Postgres per tenant (mapping §4), validasi saldo stok & piutang
- [ ] 9.2 Tenant pilot: jalan paralel 1–2 minggu, rekonsiliasi laporan harian
- [ ] 9.3 Cutover bertahap tenant lain; legacy jadi read-only

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
    tools/legacy-etl/        ETL MySQL→Postgres + skrip rekonsiliasi
    tests/golden/            fixture nota legacy + expected output
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

- Nama tabel/kolom/kode: **Inggris, snake_case** (`sale_lines`, `tenant_id`). Simpan pemetaan ke nama legacy di §7/§4 dan di `source/tools/legacy-etl`.
- Teks UI dan pesan error untuk pengguna akhir: **Bahasa Indonesia**.
- Error API: JSON `{ "error": { "code": "STOCK_INSUFFICIENT", "message": "..." } }` + HTTP status yang benar (bukan selalu 200 seperti legacy).
- Auth: access token JWT pendek (≤15 menit) + refresh token di Redis (httpOnly cookie). Klaim: `sub`, `tid` (tenant), `oid` (outlet aktif), `role`.
- Setiap modul baru wajib punya test untuk invariant-nya (stok tidak minus, total = Σ lines − diskon + pajak + biaya lain, idempotensi).

## 7. Peta Legacy (buka hanya yang relevan)

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
| `AFTEREDIT` on `01_trs_barangkeluar` | Hapus & buat ulang piutang; **tambah poin lagi tanpa mengurangi poin lama**. | **Bug legacy:** poin menggelembung setiap edit; piutang yang sudah dibayar sebagian kembali penuh. Desain baru: hitung selisih. Tandai di golden test sebagai perbedaan yang disengaja. |
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
- Format nomor nota yang wajib dipertahankan (legacy: awalan nota + komputer lokal + tanggal; lihat `notamenupenjualan`).
- Aturan pembulatan harga (fungsi legacy `PEMBULATANANGKA`) — belum dibaca.

## 11. Log Sesi  ← TAMBAHKAN DI ATAS, terbaru dulu

- **2026-10-06 (UI mirip template)** — Header diport penuh seperti template (pencarian + Ctrl K, Buat Cepat, tema, notifikasi, chip pengguna; dropdown state-based Svelte), dasbor: breadcrumb, hero, 4 kartu KPI placeholder (tanpa angka karangan), footer. `app.css` menambah `@theme` breakpoint agar utilitas responsif baru (`sm:`/`xl:`) ter-generate tanpa menimpa token template.
- **2026-10-06 (restruktur)** — Semua source code (backend, web, print-agent, tools, tests, deploy) dipindah ke `source/` sebagai root kode; `AGENTS.md`, `CLAUDE.md`, `docs/`, `reference/`, `.claude`, `.cursor` tetap di root repo.
- **2026-10-06 (Fase 0.1–0.3, 0.5)** — Repo git + struktur §5 + `.gitignore` (`reference/template/` dan `erp-docker/` tidak ikut git). `deploy/docker-compose.yml` (PG16/Redis7, port 5433/6380). Go 1.27 dipasang; `backend/` modul `aciraba`: config (env/.env), pgx pool, go-redis, chi + middleware (request id, log slog, recover, CORS), `/healthz` (200; 503 bila dependensi down — diuji dengan mematikan Redis, plus `go test`). `web/`: SvelteKit 3 SPA + shell (sidebar/topbar) dan halaman `/login` (zod, memanggil `POST /auth/login`) dari template; diverifikasi di browser pane termasuk CORS web→API; `npm run build` sukses. 0.4 belum.

- **2026-10-06 (PRD)** — Dibuat `docs/PRD.md` v0.1 (draf): tujuan, metrik, persona, rilis R0–R5, FR per modul, NFR, migrasi, risiko, 10 pertanyaan terbuka (Q1–Q10, sama dengan §10).
- **2026-10-06 (lanjutan)** — Pengguna menambahkan template Dreams Core (Tailwind v4, 213 halaman, hasil build) di `reference/template/`. Ditambahkan keputusan UI (§1), roadmap bertahap (§2b), dan pemetaan halaman template → modul (§5b). Sesi berikutnya dimulai dari §2b kotak 0.1.

- **2026-10-06** — Analisis legacy (DB dump, alur CI4↔Node↔MySQL, keamanan). Diputuskan stack baru Go + PostgreSQL + Redis + SvelteKit SPA, migrasi per tenant dengan golden test. Isi semua trigger dibaca dan dirangkum di §8 (temuan baru: bug poin saat edit nota, tabel `BARANG_LOG` hilang, stok 3 bucket). agent.md ditulis ulang ke format ini, lalu di-rename jadi AGENTS.md + pointer CLAUDE.md & .cursor/rules. Belum ada kode.
