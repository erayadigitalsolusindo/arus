# Cara Menjalankan ARUS (dev)

> Ringkas untuk menjalankan aplikasi di mesin lokal. Detail env, seed, dan akun demo: `docs/DEV-ENV.md`. Deploy: `source/deploy/`.

## Prasyarat

Go 1.27+, [Bun](https://bun.sh) 1.4+, Docker + Compose, [`goose`](https://github.com/pressly/goose) (`go install github.com/pressly/goose/v3/cmd/goose@latest`). Mobile (opsional): Flutter 3.47 + JDK 17 + Android SDK.

## Langkah (dari root repo)

**1. Database + Redis** (PostgreSQL `127.0.0.1:5433`, Redis `127.0.0.1:6380`)

```bash
cp source/deploy/.env.example source/deploy/.env
docker compose -f source/deploy/docker-compose.yml --env-file source/deploy/.env up -d
```

**2. Konfigurasi API**

```bash
cp source/backend/.env.example source/backend/.env
```

Isi `source/backend/.env`: password DB/Redis samakan dengan `source/deploy/.env`, `JWT_SECRET` minimal 32 karakter, `CORS_ORIGINS` memuat `http://localhost:5200`.

**3. Migration** (pakai `MIGRATE_DATABASE_URL` = pemilik skema dari `backend/.env`)

```bash
cd source/backend
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
cp .env.example .env   # VITE_API_URL=http://localhost:8080
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
| Web gagal start di port 5200 | port dipakai proses lain (`strictPort` aktif); matikan prosesnya |

> Jangan `source` file `.env` utuh di shell (nilai berspasi rusak); cukup `export` variabel yang perlu. Folder `docker/` di root adalah compose mesin dev pengembang, bukan compose ARUS.
