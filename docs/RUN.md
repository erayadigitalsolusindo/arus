# Cara Menjalankan ARUS (dev)

> Ringkas untuk menjalankan aplikasi di mesin lokal. Detail env, seed, dan akun demo: `docs/DEV-ENV.md`. Deploy: `source/deploy/`.

## Prasyarat

Go 1.27+, [Bun](https://bun.sh) 1.4+ (teruji: Go 1.27.4, Bun 1.4.4, goose 3.30.1, Docker 29), Docker + Compose, [`goose`](https://github.com/pressly/goose) (`go install github.com/pressly/goose/v3/cmd/goose@latest`). Mobile (opsional): Flutter 3.47 + JDK 17 + Android SDK.

## Langkah (dari root repo)

**1. Database + Redis** (PostgreSQL `127.0.0.1:5433`, Redis `127.0.0.1:6380`)

```bash
cp source/deploy/.env.example source/deploy/.env
```

Lalu edit `source/deploy/.env` agar cocok dengan `source/backend/.env.example` (password contoh sudah sama; bila memakai nilai lain, samakan di kedua file). Password hanya berlaku saat volume data **pertama kali** dibuat; bila diganti kemudian, jalankan `docker compose ... down -v` (menghapus data dev) lalu `up -d` lagi.

```ini
POSTGRES_PASSWORD=aciraba_dev_pw
POSTGRES_APP_PASSWORD=aciraba_app_dev_pw
REDIS_PASSWORD=aciraba_redis_dev_pw
```

Jalankan:

```bash
docker compose -f source/deploy/docker-compose.yml --env-file source/deploy/.env up -d
```

**2. Konfigurasi API**

```bash
cp source/backend/.env.example source/backend/.env
```

> **Jangan memakai `.env` produksi untuk dev.** `.env.example` di folder backend adalah contoh **dev**; contoh produksi ada di `source/deploy/` (pakai `APP_ENV=production`, `SMTP_HOST` wajib, `CORS_ORIGINS`/`APP_BASE_URL` berupa https, `TRUST_PROXY=true`). Bila `backend/.env` Anda berisi nilai produksi (domain `*.erayadigital.co.id`, SMTP asli, `AI_PLATFORM_KEY`), API dev akan mengirim email sungguhan dan menolak origin `localhost`. Simpan sebagai `.env.production` di luar repo, lalu buat `.env` dev baru dari `.env.example`.

Isi `source/backend/.env` (dev):

- `APP_ENV=development`, `HTTP_ADDR=:8080`, `TRUST_PROXY=false`, `SMTP_HOST` kosong;
- password di `DATABASE_URL`, `MIGRATE_DATABASE_URL`, `REDIS_URL` samakan dengan `source/deploy/.env` (`POSTGRES_APP_PASSWORD`, `POSTGRES_PASSWORD`, `REDIS_PASSWORD`);
- `JWT_SECRET` minimal 32 karakter (`openssl rand -base64 48`);
- `CORS_ORIGINS` dan `APP_BASE_URL` ubah dari `http://localhost:5173` (bawaan contoh) menjadi `http://localhost:5200` (port web sebenarnya);
- `ERAYA_*` di contoh hanyalah isian SSO. Hapus baris `ERAYA_ISSUER`, `ERAYA_CLIENT_ID`, `ERAYA_CLIENT_SECRET` bila tidak punya client Eraya (kosong = tombol Eraya mati). Jangan biarkan teks placeholder `<...>` sebagai nilai.

**3. Migration** (pakai `MIGRATE_DATABASE_URL` = pemilik skema dari `backend/.env`)

```bash
cd source/backend
export MIGRATE_DATABASE_URL="$(grep '^MIGRATE_DATABASE_URL=' .env | cut -d= -f2-)"
goose -dir db/migrations postgres "$MIGRATE_DATABASE_URL" up
```

**4. Jalankan API** → <http://localhost:8080/healthz>

```bash
cd source/backend
go run ./cmd/api
```

**5. Jalankan web** (terminal lain) → <http://localhost:5200>

```bash
cd source/web
cp .env.example .env   # VITE_API_URL=http://localhost:8080; hapus/kosongkan VITE_ERAYA_* bila tanpa SSO Eraya
bun install
bun run dev
```

Buka <http://localhost:5200> → **Register** untuk membuat tenant dan Owner pertama. Di dev email tidak dikirim; tautan verifikasi/reset hanya tercatat di log API.

## Sesudah `git pull`

Terapkan migration baru (langkah 3) dan **restart API**. Rute atau kolom baru tidak muncul bila proses API lama masih jalan.

## Opsional

| Kebutuhan | Cara |
|---|---|
| Data contoh | `cd source/backend && go run ./cmd/seeddemo` |
| Penjualan dummy untuk laporan | `go run ./cmd/seedsales` (lihat `docs/DEV-ENV.md`) |
| Platform Admin | isi `PLATFORM_SETUP_TOKEN` (≥ 32 karakter) di `backend/.env`, restart API, buka `/platform/setup`; hapus token sesudahnya |
| Cetak struk | jalankan print-agent: `source/print-agent` (lihat `source/deploy/PC-KASIR.md`) |
| Login Google / Eraya (SSO) | backend: `GOOGLE_CLIENT_IDS`, `ERAYA_ISSUER`, `ERAYA_CLIENT_ID`; web: `VITE_GOOGLE_CLIENT_ID`, `VITE_ERAYA_ISSUER`, `VITE_ERAYA_CLIENT_ID` (nilai harus sama). Callback Eraya: `http://localhost:5200/auth/eraya/callback`. Detail Google: `docs/DEV-ENV.md` |
| Live Penjualan (platform) | tanpa setup tambahan; migration terbaru (`00112_platform_live`) harus sudah diterapkan |
| Mobile | `cd source/mobile && flutter pub get && flutter run` |

## Cek kualitas sebelum push

```bash
cd source/backend && go vet ./... && go test ./...
cd source/web && bun run check && bun run build
```

Test integrasi di-skip bila `TEST_DATABASE_URL`, `TEST_ADMIN_DATABASE_URL`, `TEST_REDIS_URL` kosong; isi dari `DATABASE_URL`, `MIGRATE_DATABASE_URL`, `REDIS_URL`.

## Masalah umum

| Gejala | Penyebab |
|---|---|
| API menolak start | `JWT_SECRET` < 32 karakter |
| `/healthz` → `redis: down` | password Redis di `backend/.env` ≠ `deploy/.env` |
| UI "Server unreachable" | API mati, `VITE_API_URL` ≠ alamat API, atau origin belum ada di `CORS_ORIGINS` |
| Kasir tak bisa simpan nota (`SHIFT_REQUIRED`) | shift belum dibuka |
| Login Eraya gagal / `ERAYA_DISABLED` | `ERAYA_ISSUER`/`ERAYA_CLIENT_ID` kosong atau tak cocok dengan `VITE_ERAYA_*`; issuer lokal (`:8090`) belum jalan |
| `docker compose up` dari WSL: `accessing specified distro mount service` | Integrasi WSL Docker Desktop belum aktif untuk distro ini. Aktifkan di Docker Desktop → Settings → Resources → WSL integration, atau jalankan compose dari Windows (PowerShell) |
| `docker compose` error `docker-credential-desktop` tidak ditemukan | pakai Docker dari WSL tanpa integrasi; aktifkan WSL integration (di atas) |
| Web gagal start di port 5200 | port dipakai proses lain (`strictPort` aktif); matikan prosesnya |

> Jangan `source` file `.env` utuh di shell (nilai berspasi rusak); cukup `export` variabel yang perlu. Folder `docker/` di root adalah compose mesin dev pengembang, bukan compose ARUS.
