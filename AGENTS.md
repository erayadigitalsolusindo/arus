# ARUS — Agent Handoff

> Pintu masuk ringkas. Detail ada di `docs/` dan di `AGENTS.md` per folder (`source/backend/`, `source/web/`) yang dimuat saat bekerja di folder itu. Jangan menulis ulang konten di sini; perbarui berkas yang tepat.
> Bahasa komunikasi dengan pengguna: **Bahasa Indonesia**. Identifier kode: Bahasa Inggris.
> Legacy `../aciraba_siak_os` = referensi aturan bisnis, **read-only**; jangan menjelajah ulang, buka hanya berkas yang dirujuk `docs/LEGACY.md`.

## 0. Alur Sesi

**Awal sesi:**
1. Baca berkas ini dan `.agent/SESSION.md` (status, langkah berikutnya).
2. `git status --short` dan `git log -5 --oneline`; perubahan lokal yang belum di-commit tetap diperiksa.
3. Baca dokumen/kode yang relevan saja (tabel di bawah). Jangan mengulang investigasi yang sudah selesai kecuali perlu verifikasi.
4. Bila dokumen bertentangan dengan kode/skema, **kode dan skema yang benar**; perbaiki dokumennya.

**Akhir sesi:**
1. Perbarui `.agent/SESSION.md`: kondisi aktual, keputusan, langkah berikutnya, hasil test/migration. Ringkas dan buang info usang (target ≤ ±80 baris).
2. Tambahkan entri **singkat** di atas `docs/SESSION-LOG.md` hanya bila ada keputusan produk atau pelajaran baru (bukan daftar endpoint/kolom yang sudah ada di kode).
3. Perbarui `docs/ROADMAP.md` (centang) bila kotak selesai.
4. Jangan menyalin source code ke dokumen; jangan menulis penjelasan ganda.

## Peta Dokumen (baca sesuai kebutuhan)

| Berkas | Isi | Baca saat |
|---|---|---|
| `.agent/SESSION.md` | Status terkini, langkah berikutnya | Awal sesi |
| `docs/PRD.md` | Kebutuhan produk (APA & MENGAPA), ID `FR-*`; scope/prioritas tak diubah tanpa persetujuan pengguna | Mengerjakan fitur |
| `docs/ROADMAP.md` | Fase dan kotak centang | Memilih pekerjaan |
| `docs/ARCHITECTURE.md` | Desain data, struktur repo, pemetaan template UI | Modul/tabel baru, porting halaman |
| `docs/DEV-ENV.md` | Toolchain, DB/Redis dev, env, cara menjalankan, akun demo | Setup, menjalankan, test integrasi |
| `docs/LEGACY.md` | Peta legacy, trigger tersembunyi, kelemahan yang tak boleh disalin | Mempelajari aturan bisnis lama |
| `docs/SESSION-LOG.md` | Arsip riwayat per sesi | Hanya bila perlu riwayat (Grep) |
| `source/backend/AGENTS.md` | Aturan tabel bertenant, tata letak modul, big data | Bekerja di backend |
| `source/web/AGENTS.md` | SvelteKit 3, i18n, DateRange | Bekerja di web |
| `source/mobile/AGENTS.md` | Flutter (ARUS Mobile): struktur, stack, auth native, l10n | Bekerja di mobile |

`README.md` = halaman depan GitHub (fitur, mulai cepat, deploy); bukan sumber kebenaran, bila beda **ikuti berkas ini dan `docs/`**. `CLAUDE.md` dan `.cursor/rules/aciraba.mdc` hanya pointer ke berkas ini.
Penomoran bagian berlompat (§0, §1, §3, §6) karena §2/§4/§5/§7–§10 dipindah ke `docs/`; jangan dirapikan, banyak dokumen merujuknya.

---

## 1. Keputusan yang Sudah Dikunci (jangan diperdebatkan ulang)

| Area | Keputusan |
|---|---|
| Strategi | **Rewrite** di folder ini (`aciraba_newgen`). Legacy `../aciraba_siak_os` = referensi perilaku, **read-only** kecuali pengguna minta. |
| Backend | **Go**, modular monolith (bukan microservices). Router `chi`, DB `pgx/v5` + **`sqlc`**, migration `goose`, log `slog`, validasi `go-playground/validator`. |
| Database | **PostgreSQL 16+**. `NUMERIC` untuk uang/qty, `timestamptz`, FK + UNIQUE wajib, Row Level Security per tenant. Tanpa stored procedure bisnis. Trigger hanya untuk `updated_at`/audit generik. |
| Cache/Realtime | **Redis**: session/refresh token, rate limit, cache master barang/harga, Pub/Sub realtime (KDS/dashboard), idempotency key, job queue (`asynq`). **Tidak pernah** menjadi sumber kebenaran stok/uang. |
| Frontend | **SvelteKit mode SPA** (`adapter-static`) memanggil API Go langsung. TanStack Query (svelte), **Tailwind v4**, bits-ui (headless) untuk komponen interaktif, zod. **Tidak ada lapisan CI4/PHP lagi.** |
| UI/Visual | Template berbayar **Dreams Core** (Tailwind v4) di `reference/template/` = **acuan visual saja**. Yang ada adalah hasil build (HTML + asset ber-hash), bukan source. Porting ke komponen Svelte: ambil markup/class Tailwind & token warna (CSS variable `--sidebar-*` dll. di `assets/script-*.css`); **jangan** memakai JS bawaan template (vanilla/DOM manipulation). Lihat `docs/ARCHITECTURE.md` (§5b). |
| Printer | Satu **print-agent Go** (binary lokal di PC kasir, ESC/POS). Menggantikan `aciraba_printlocal` (Node) dan `aciraba_printer` (Python). Template struk ada di web (`lib/pos/receipt.ts`, `escpos.ts`); agent hanya kurir RAW. Mobile: printer Bluetooth (Fase 10). |
| Mobile | **ARUS Mobile = Flutter** di `source/mobile/` (monorepo, Android; diputuskan 2026-10-10, menggantikan usulan Capacitor). Memakai API Go yang sama; auth native lewat header `X-Client: mobile`. Aturan: `source/mobile/AGENTS.md`. |
| Layar kasir web | `/kasir` (modern) dan `/kasirb` (klasik) berbagi satu logika di `lib/components/PosScreen.svelte` (prop `variant`). Ubah logika kasir **sekali di `PosScreen`**; rute hanya pembungkus. |
| Data | **Aplikasi baru, data baru (diputuskan 2026-10-07).** Data legacy **tidak dimigrasikan**: tidak ada ETL, tidak ada sistem paralel, tidak ada kompatibilitas akun/password/format nota/skema dengan legacy. Toko mulai lewat **onboarding**: import barang (Excel/CSV), saldo awal stok (movement `OPENING`), saldo awal piutang/hutang (PRD §7.5b, §12). Legacy dipakai **hanya untuk memahami aturan bisnis**. |
| Verifikasi | **Spec test**: contoh kasus perhitungan yang ditulis manual (input → total, pajak, kembalian, stok, poin, piutang) dan disetujui pengguna, disimpan di `source/tests/spec/`. Aturan mengikuti PRD (urutan hitung nota PRD §7.4), **bukan** meniru legacy — termasuk tidak meniru bug di `docs/LEGACY.md` (§8). |


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


## 6. Konvensi Global

- Nama tabel/kolom/kode: **Inggris, snake_case** (`sale_lines`, `tenant_id`). Padanan istilah legacy cukup di `docs/ARCHITECTURE.md` dan `docs/LEGACY.md`.
- Error API: JSON `{ "error": { "code": "STOCK_INSUFFICIENT", "message": "..." } }` + HTTP status yang benar (bukan selalu 200 seperti legacy).
- Auth: access token JWT pendek (≤15 menit) + refresh token di Redis (httpOnly cookie). Klaim: `sub`, `tid` (tenant), `oid` (outlet aktif), `role`.
- Setiap modul baru wajib punya test untuk invariant-nya (stok tidak minus, total = Σ lines − diskon + pajak + biaya lain, idempotensi).
- Aturan big data (keyset, indeks, RLS vs trigram): `source/backend/AGENTS.md`. Aturan i18n dan DateRange: `source/web/AGENTS.md`.
