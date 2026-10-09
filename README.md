# ARUS (ACIRABA NewGen)

**Sistem POS & ERP ritel multi-tenant, multi-outlet.** Dibangun ulang dari nol (Go + PostgreSQL + Redis + SvelteKit) untuk menggantikan sistem lama ACIRABA SIAK OS.

> Status: **dalam pengembangan aktif**. Fondasi, auth, hak akses, master data, stok, kasir, piutang, dan pembelian sudah berjalan. Lihat [Status & Roadmap](#status--roadmap).

---

## Daftar Isi

1. [Apa itu ARUS?](#apa-itu-arus)
2. [5W + 1H](#5w--1h)
3. [Fitur](#fitur)
4. [Arsitektur & Teknologi](#arsitektur--teknologi)
5. [Prinsip Desain](#prinsip-desain)
6. [Struktur Repo](#struktur-repo)
7. [Instalasi untuk Pengembangan (lokal)](#instalasi-untuk-pengembangan-lokal)
8. [Konfigurasi (environment)](#konfigurasi-environment)
9. [Menjalankan Test](#menjalankan-test)
10. [Deploy ke Server (produksi)](#deploy-ke-server-produksi)
11. [Update / Upgrade](#update--upgrade)
12. [Backup & Pemulihan](#backup--pemulihan)
13. [Troubleshooting](#troubleshooting)
14. [Status & Roadmap](#status--roadmap)
15. [Dokumentasi Lain](#dokumentasi-lain)
16. [Kontribusi](#kontribusi)

---

## Apa itu ARUS?

ARUS adalah aplikasi **Point of Sale (kasir) dan ERP ringan berbasis web** untuk toko ritel dan restoran yang punya satu atau banyak cabang. Satu instalasi melayani banyak bisnis (*tenant*) sekaligus, dengan data yang terisolasi satu sama lain.

Cakupan utama: kasir, stok 3 lokasi (display/gudang/retur), harga grosir, member & poin, kupon, piutang, pembelian & hutang pemasok, opname, mutasi stok antar cabang, serta pengelolaan pengguna dan hak akses per peran.

**Mengapa dibangun ulang?** Sistem lama (CodeIgniter 4 + Node.js + MySQL) punya masalah mendasar: API tanpa autentikasi, identitas tenant diambil dari browser, transaksi database tidak atomik, uang disimpan sebagai `DOUBLE`, dan aturan bisnis tersembunyi di trigger/stored procedure. ARUS memperbaiki semuanya dengan desain yang bisa dites.

**Penting:** ARUS adalah **aplikasi baru dengan data baru**. Data sistem lama **tidak dimigrasikan**; toko mulai lewat *onboarding* (import barang, saldo awal stok, saldo awal piutang/hutang). Sistem lama hanya menjadi referensi aturan bisnis.

---

## 5W + 1H

| | |
|---|---|
| **What** (Apa) | Aplikasi POS + ERP ritel multi-tenant: kasir, stok berbasis ledger, master data, member, kupon, piutang/hutang, pembelian, hak akses, audit log. |
| **Why** (Mengapa) | Menggantikan sistem lama yang tidak aman dan rawan selisih stok/uang. Sasaran: transaksi kasir cepat, benar, dan tidak pernah ganda; stok & saldo selalu dapat ditelusuri; data antar-tenant terisolasi di level server **dan** database. |
| **Who** (Siapa) | **Pemilik usaha** (laporan, pengaturan, hak akses), **supervisor** (persetujuan harga/limit kredit via PIN), **kasir** (layar kasir), **staf gudang/pembelian** (stok, opname, pembelian), dan **operator platform** (Platform Admin: mengelola tenant). |
| **Where** (Di mana) | Web (SPA) yang diakses lewat browser di PC kasir/tablet/ponsel. Server di VPS tunggal (contoh produksi: aaPanel + Apache) atau lokal untuk pengembangan. |
| **When** (Kapan) | Dikembangkan bertahap sejak Oktober 2026 per fase (lihat [Roadmap](#status--roadmap)). Pemakaian harian: setiap transaksi toko; stok dan pembayaran tercatat saat itu juga. |
| **How** (Bagaimana) | Backend **Go** (modular monolith) + **PostgreSQL 16** (Row Level Security per tenant) + **Redis** (sesi, rate limit, idempotensi) + frontend **SvelteKit** mode SPA. Semua aturan bisnis ada di service Go yang dites; harga dihitung di server, bukan di browser. |

---

## Fitur

**Platform & keamanan**
- Registrasi tenant mandiri, login email + password (argon2id), JWT akses pendek + refresh token rotasi (httpOnly cookie), kunci login bertahap, lupa/reset password, verifikasi email, persetujuan syarat layanan.
- **Row Level Security** PostgreSQL per tenant (role aplikasi bukan pemilik tabel, tanpa BYPASSRLS).
- Role & permission per menu (view/create/update/delete/approve), anti-eskalasi hak, pindah outlet, cabut semua sesi, **audit log append-only**.
- **Platform Admin** terpisah (wajib 2FA TOTP) untuk melihat/mengelola tenant dan "masuk sebagai" tenant dengan jejak audit.
- PIN penyetuju (Owner/Supervisor) untuk ubah harga, edit nota, melewati limit kredit, dan pindah outlet di kasir.

**Master data**
- Satuan, kategori, brand, principal, pemasok, salesman, metode pembayaran (dengan biaya MDR ditanggung toko/pelanggan).
- Daftar item: kode & barcode (boleh kembar antar barang), harga jual **per cabang**, harga **grosir bertingkat**, konversi satuan, gambar (diproses & di-resize di server), deskripsi markdown, HPP per cabang.
- Member dengan **level berdasarkan poin**, ledger poin, tukar poin di kasir, kupon belanja global.

**Kasir (POS)**
- Layar kasir keyboard-first: scan barcode, kolom QTY, 16 slot pintasan barang, nota pending, biaya lain dinamis, pilih member/salesman/kupon.
- Total dihitung server (pratinjau `quote` = hasil simpan), pembayaran tunai/non-tunai/split, **kredit/piutang** dengan DP dan limit member.
- Idempotensi (kiriman ganda tetap satu nota), nomor nota harian per outlet tanpa celah, edit & batal nota lewat **revisi** (nota lama tidak pernah ditimpa).
- Peringatan dini stok kurang dan harga di bawah HPP.

**Stok**
- Stok berbasis **ledger** (`stock_movements` append-only) dengan 3 bucket, penjaga stok atomik (aman untuk banyak kasir bersamaan).
- Saldo awal + kunci tanggal operasional, **opname** (sesi & langsung), **pecah satuan**, **mutasi antar cabang/bucket** (kirim → terima), kartu stok.

**Pembelian & hutang**
- Pembelian langsung tunai/kredit, diskon baris bertingkat, biaya lain, **HPP rata-rata tertimbang per cabang**, edit/batal dengan revisi.
- Hutang pemasok: pembayaran/cicilan, pelunasan kolektif, aging, riwayat harga beli.

**Antarmuka**
- Tema Dreams Core (Tailwind v4), multi-bahasa **Indonesia/Inggris**, tema terang/gelap, tab halaman dengan draf form tersimpan.

---

## Arsitektur & Teknologi

```
Browser (SvelteKit SPA)
   │  HTTPS, JWT di memori + refresh cookie
   ▼
API Go (chi)  ── modular monolith ──►  PostgreSQL 16  (RLS per tenant, NUMERIC untuk uang/qty)
   │                                    Redis 7        (sesi, rate limit, idempotensi, cache)
   └── disk lokal (UPLOAD_DIR) untuk gambar
```

| Lapisan | Teknologi |
|---|---|
| Backend | Go, `chi`, `pgx/v5`, `sqlc`, `goose` (migration), `slog`, argon2id |
| Database | PostgreSQL 16+ (RLS, FK komposit, UNIQUE, tanpa stored procedure bisnis) |
| Cache | Redis 7 (**bukan** sumber kebenaran stok/uang) |
| Frontend | SvelteKit 3 (SPA, `adapter-static`), Tailwind v4, bits-ui, TanStack Query, zod |
| Deploy | Docker Compose (Postgres + Redis), binary Go sebagai service systemd, web statis di Apache/Nginx |

---

## Prinsip Desain

1. **Tenant, outlet, user diambil dari token di server**, bukan dari body request. Setiap query difilter `tenant_id`, RLS sebagai lapis kedua.
2. **Satu use-case = satu transaksi DB.** Efek samping eksternal dikirim setelah commit.
3. **Harga dihitung di server.** Klien hanya mengirim item + qty (+ override harga yang wajib PIN penyetuju dan tercatat di audit).
4. **Stok = ledger.** Movement append-only, saldo diperbarui dengan guard atomik.
5. **Idempotensi** untuk endpoint pembuat transaksi (`Idempotency-Key`).
6. **Kunci bisnis dijaga database** (UNIQUE/FK), bukan SELECT-lalu-INSERT.
7. **Uang tidak pernah float** (`NUMERIC(18,2)` / qty `NUMERIC(18,3)`).
8. **Aturan bisnis di service Go** yang dites, bukan di trigger/SP.
9. **Tanpa secret di repo.**

Detail lengkap, keputusan produk, dan log per sesi ada di [`AGENTS.md`](AGENTS.md).

---

## Struktur Repo

```
.
├── AGENTS.md            # sumber tunggal konteks teknis, keputusan, status, log sesi
├── CLAUDE.md            # pointer ke AGENTS.md
├── docs/PRD.md          # Product Requirements Document
├── reference/           # acuan visual (template UI) — bukan kode produk
└── source/
    ├── backend/         # Go: cmd/api, internal/<modul>, db/migrations, sqlc.yaml
    ├── web/             # SvelteKit SPA
    └── deploy/          # docker-compose, skrip & panduan deploy (aapanel/, manual/)
```

---

## Instalasi untuk Pengembangan (lokal)

### Prasyarat

| Alat | Versi |
|---|---|
| Go | 1.27+ |
| Node.js | 26 (npm ikut) |
| Docker + Docker Compose | untuk PostgreSQL & Redis |
| `goose` | migration (`go install github.com/pressly/goose/v3/cmd/goose@latest`) |
| `sqlc` | hanya jika mengubah `queries.sql` |
| Git | |

### Langkah

**1. Ambil kode**
```bash
git clone https://github.com/erayadigitalsolusindo/erp.git
cd erp
```

**2. Jalankan PostgreSQL & Redis**
```bash
cd source/deploy
cp .env.example .env          # ubah password sesuai selera
docker compose --env-file .env up -d
```
Ini membuat `aciraba_postgres` (127.0.0.1:**5433**) dan `aciraba_redis` (127.0.0.1:**6380**). Port sengaja bukan default agar tidak bentrok. Role runtime `aciraba_app` dibuat otomatis oleh `deploy/initdb/01-app-role.sh` saat volume pertama kali dibuat.

**3. Konfigurasi backend**
```bash
cd ../backend
cp .env.example .env
```
Lalu edit `.env`: samakan password dengan `deploy/.env`, dan isi `JWT_SECRET` (minimal 32 karakter, mis. `openssl rand -base64 48`). Lihat [Konfigurasi](#konfigurasi-environment).

> Dua URL database dipakai sengaja: `MIGRATE_DATABASE_URL` (pemilik skema, hanya untuk migration) dan `DATABASE_URL` (role `aciraba_app`, dipakai API sehingga RLS selalu berlaku).

**4. Jalankan migration**
```bash
cd source/backend
set -a; . ./.env; set +a      # atau export MIGRATE_DATABASE_URL secara manual
goose -dir db/migrations postgres "$MIGRATE_DATABASE_URL" up
```
> Jika `.env` memuat nilai berspasi (mis. `SMTP_FROM`), jangan di-`source`; cukup `export MIGRATE_DATABASE_URL='postgres://...'`.

**5. Jalankan API**
```bash
cd source/backend
go run ./cmd/api              # http://localhost:8080
curl http://localhost:8080/healthz
```
`/healthz` menjawab 200 bila PostgreSQL dan Redis sehat (503 bila salah satu mati).

**6. Jalankan frontend**
```bash
cd source/web
cp .env.example .env          # VITE_API_URL=http://localhost:8080
npm install
npm run dev                   # http://localhost:5173
```

**7. Buka aplikasi**
Buka <http://localhost:5173>, lalu **Register** untuk membuat tenant & akun Owner pertama. Di mode development, email (verifikasi/reset password) tidak dikirim, hanya dicatat di log API (`LogMailer`).

**8. (Opsional) Platform Admin**
Isi `PLATFORM_SETUP_TOKEN` (≥ 32 karakter) di `backend/.env`, restart API, buka `/platform/setup` untuk membuat admin platform pertama (wajib mendaftarkan 2FA). **Hapus token itu dari `.env` setelah selesai.**

---

## Konfigurasi (environment)

Variabel utama `source/backend/.env`:

| Variabel | Keterangan |
|---|---|
| `APP_ENV` | `development` / `production` |
| `HTTP_ADDR` | alamat listen API, mis. `:8080` |
| `DATABASE_URL` | koneksi runtime (role `aciraba_app`) |
| `MIGRATE_DATABASE_URL` | koneksi pemilik skema, khusus goose |
| `REDIS_URL` | koneksi Redis |
| `JWT_SECRET` | wajib, ≥ 32 karakter. **Mengganti nilai ini membuat PIN penyetuju dan rahasia 2FA tidak berlaku** |
| `CORS_ORIGINS` | origin SPA yang diizinkan (juga dipakai CSRFGuard) |
| `APP_BASE_URL` | alamat SPA untuk tautan email; wajib `https://` di luar dev |
| `SMTP_HOST/PORT/USER/PASS/FROM` | email; kosong di dev = hanya log; **wajib di produksi** |
| `UPLOAD_DIR` | folder gambar unggahan (di produksi: volume yang dicadangkan) |
| `TRUST_PROXY` | `true` hanya bila di belakang reverse proxy tepercaya |
| `PLATFORM_SETUP_TOKEN` | membuat Platform Admin pertama; hapus setelah dipakai |

Frontend (`source/web/.env`): `VITE_API_URL` — alamat API.

---

## Menjalankan Test

```bash
# Backend (test integrasi butuh database & redis; tanpa env berikut akan di-skip)
cd source/backend
export TEST_DATABASE_URL=...        # = DATABASE_URL (aciraba_app)
export TEST_ADMIN_DATABASE_URL=...  # = MIGRATE_DATABASE_URL (pemilik)
export TEST_REDIS_URL=...           # = REDIS_URL
go vet ./...
go test ./...

# Frontend
cd source/web
npm run check                       # svelte-check + typecheck
npm run build
```
Gunakan database dev sementara; test membuat dan membersihkan tenant uji sendiri.

---

## Deploy ke Server (produksi)

Dua jalur terdokumentasi lengkap di repo:

| Jalur | Panduan | Cocok untuk |
|---|---|---|
| **aaPanel (Apache/Nginx)** — jalur yang sudah diuji di server nyata | [`source/deploy/aapanel/DEPLOY-AAPANEL.md`](source/deploy/aapanel/DEPLOY-AAPANEL.md) | VPS yang sudah memakai aaPanel |
| **Manual (nginx + certbot + systemd)** — belum diuji di server nyata | [`source/deploy/manual/DEPLOY-MANUAL.md`](source/deploy/manual/DEPLOY-MANUAL.md) | VPS Ubuntu 22.04/24.04 polos |

Gambaran arsitektur produksi:

```
Browser ─► (Cloudflare) ─► Web server (80/443)
                             ├─ arus.<domain>      → berkas statis SPA
                             └─ arus-api.<domain>  → reverse proxy → 127.0.0.1:8080 (binary Go, systemd)
                                                      ├─ 127.0.0.1:5433 PostgreSQL (Docker)
                                                      ├─ 127.0.0.1:6380 Redis (Docker)
                                                      └─ folder unggahan
```

Ringkasan langkah (detail di panduan):

1. DNS untuk dua nama di bawah **satu domain induk** (web & API harus *same-site* agar cookie refresh `SameSite=Lax` bekerja). Jika memakai Cloudflare, SSL mode **Full (strict)**.
2. Buat folder `/www/wwwroot/arus/{api,web,deploy,data/uploads,backups}`, clone repo, salin `source/deploy/` ke folder deploy.
3. Jalankan PostgreSQL + Redis lewat Docker Compose (bind ke `127.0.0.1`, **jangan** buka port 5433/6380/8080 di firewall).
4. Buat `api/.env` dari `env.production.example` (secret acak, `APP_ENV=production`, SMTP wajib, `APP_BASE_URL` https, `TRUST_PROXY=true`).
5. Build rilis di server (`build-release.sh` → `release.tar.gz` berisi binary, migrations, dan web).
6. Pasang service systemd `arus-api` dan konfigurasi web server (reverse proxy API + rewrite SPA).
7. Jalankan `update.sh` untuk migrasi + pasang; cek `cek-arus.sh` dan `/healthz`.
8. Buat Platform Admin pertama lewat `/platform/setup`, lalu **hapus `PLATFORM_SETUP_TOKEN`**.
9. Pasang cron backup harian (lihat [Backup](#backup--pemulihan)).

---

## Update / Upgrade

### Lingkungan pengembangan
```bash
git pull
cd source/backend
goose -dir db/migrations postgres "$MIGRATE_DATABASE_URL" up   # terapkan migration baru
go run ./cmd/api                                               # restart API
cd ../web && npm install && npm run dev                        # dependensi baru bila ada
```
Bila `go test`/API gagal setelah pull, hampir selalu karena **migration belum diterapkan** atau proses API lama masih berjalan.

### Server produksi (aaPanel)
Satu perintah, dijalankan sebagai user biasa di server (skrip disalin ke `~` dari `source/deploy/aapanel/`):

```bash
bash ~/update-arus.sh
```
Skrip ini berurutan: `git pull --ff-only` → build rilis → **backup database** → `goose up` → ganti binary (versi lama disimpan sebagai `aciraba-api.prev`) → ganti web → restart `arus-api` → cek `/healthz`. Berhenti di langkah pertama yang gagal.

Setelah update:
```bash
bash ~/cek-arus.sh        # ringkasan kesehatan: [OK] / [MASALAH]
```

### Rollback
- Binary: kembalikan `aciraba-api.prev` ke `aciraba-api`, lalu `systemctl restart arus-api`.
- Database: pulihkan dump `pre-update-*.dump` dari folder `backups/` (`pg_restore`). Migration tidak otomatis di-*down*; pulihkan dari backup bila skema harus dikembalikan.

> Update yang menambah migration bersifat maju (forward). Selalu pastikan backup `pre-update-*.dump` terbentuk sebelum melanjutkan.

---

## Backup & Pemulihan

- `source/deploy/aapanel/backup.sh` (atau `manual/backup.sh`) — jalankan lewat cron harian, mis. 02:30.
- **Simpan salinan di luar VPS** (rclone / cloud storage). Backup di disk yang sama tidak menolong bila server hilang.
- Cadangkan juga folder unggahan (`data/uploads`) dan file `.env` (secara aman).
- **Uji restore** secara berkala; backup yang tidak pernah diuji belum bisa dianggap ada.

---

## Troubleshooting

| Gejala | Penyebab umum |
|---|---|
| API menolak start | `JWT_SECRET` < 32 karakter, atau `SMTP_HOST` kosong di mode production |
| `/healthz` → `redis: down` | password Redis di `api/.env` berbeda dengan `deploy/.env` |
| UI "Server unreachable" | API tidak berjalan / `VITE_API_URL` tidak cocok dengan port API / origin belum ada di `CORS_ORIGINS` |
| Login berputar di produksi | Cloudflare SSL mode *Flexible* (harus *Full strict*), atau web & API beda domain induk |
| Tes integrasi terlewat (skip) | `TEST_DATABASE_URL`, `TEST_ADMIN_DATABASE_URL`, `TEST_REDIS_URL` belum diatur |
| Rute/kolom baru tidak ada setelah pull | migration belum dijalankan atau proses API lama belum di-restart |
| `rsync --delete` gagal saat update | proteksi anti-cross-site/open_basedir aaPanel pada folder web aktif; matikan untuk situs web ARUS |
| PIN/2FA tiba-tiba tidak valid | `JWT_SECRET` diganti; PIN harus diatur ulang dan 2FA direset oleh admin lain |

---

## Status & Roadmap

| Fase | Isi | Status |
|---|---|---|
| 0 | Fondasi (repo, Docker, Go, migration, SPA shell) | ✅ Selesai |
| 2 | Auth, tenant, RLS, role & permission, audit | ✅ Selesai |
| 3 | Master data (3.1), item (3.2: sisa diskon item), member (3.3), metode bayar | 🟡 Sebagian — import Excel/CSV (3.4) belum |
| 4 | Stok: ledger, saldo awal, opname, mutasi, pecah satuan, kartu stok | ✅ Selesai |
| 5 | Kasir/POS | 🟡 Sebagian — kredit, edit/batal, piutang selesai; retur penjualan, saldo awal piutang, print-agent/cetak struk belum |
| 6 | Pembelian & hutang | 🟡 Sebagian — retur beli, PO/penerimaan bertahap, saldo awal hutang belum |
| 7 | Laporan | ⏳ Belum |
| 8 | Modul opsional (resto/KDS, akuntansi SIAK, payment gateway) | ⏳ Belum |
| 9 | Wizard onboarding & go-live | ⏳ Belum |

**Catatan yang perlu diketahui:**
- **Deposit member sengaja ditunda** (kartu menampilkan "Belum tersedia"); direncanakan bersama modul piutang/kas.
- Teks **Syarat Layanan & Kebijakan Privasi masih draf** dan harus ditinjau konsultan hukum sebelum rilis publik.
- Cetak struk memerlukan *print-agent* lokal (Fase 5.4) yang belum dibuat.
- Kasir belum mendukung mode offline.

Roadmap rinci per kotak ada di [`AGENTS.md`](AGENTS.md) §2b.

---

## Dokumentasi Lain

- [`AGENTS.md`](AGENTS.md) — keputusan teknis, status, aturan bisnis, dan log setiap sesi pengembangan (sumber tunggal).
- [`docs/PRD.md`](docs/PRD.md) — kebutuhan produk (scope, FR/NFR, rilis).
- [`source/deploy/aapanel/DEPLOY-AAPANEL.md`](source/deploy/aapanel/DEPLOY-AAPANEL.md) — deploy produksi (aaPanel).
- [`source/deploy/manual/DEPLOY-MANUAL.md`](source/deploy/manual/DEPLOY-MANUAL.md) — deploy produksi (manual).

---

## Kontribusi

- Baca `AGENTS.md` seluruhnya sebelum mengubah kode; konteks teknis hanya ditulis di sana.
- Tabel baru bertenant wajib: kolom `tenant_id`, `ENABLE ROW LEVEL SECURITY` + policy `tenant_isolation`, dan test isolasi.
- Teks UI wajib lewat kamus i18n (`t('domain.kunci')`), bukan ditulis langsung di komponen.
- Setiap modul baru wajib punya test untuk invariannya (stok tidak minus, total = Σ baris − diskon + pajak + biaya lain, idempotensi).
- Jangan mengomit `.env`, kredensial, atau dump data pelanggan.

---

© Eraya Digital Solusindo. Hak cipta dilindungi; lisensi belum ditetapkan.
