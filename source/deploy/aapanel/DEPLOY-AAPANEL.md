# Deploy ACIRABA di aaPanel (VPS sendiri)

Domain: **web** `arus.erayadigital.co.id` · **API** `arus-api.erayadigital.co.id`
(dua nama di bawah satu domain induk = *same-site*, jadi cookie refresh `SameSite=Lax` berfungsi.)

```
Browser ──https──► nginx aaPanel ─┬─ arus.…     → file statis /www/wwwroot/arus/web
                                  └─ arus-api.… → 127.0.0.1:8080 (binary Go, Supervisor)
                                                   ├─ 127.0.0.1:5433 Postgres 16 (Docker)
                                                   └─ 127.0.0.1:6380 Redis 7 (Docker)
```

## 0. Prasyarat (sekali)
- DNS: record **A** `arus` dan `arus-api` (di zona `erayadigital.co.id`) → IP VPS. Tunggu propagasi sebelum minta SSL.
- aaPanel App Store: pasang **Nginx**, **Docker** (Docker Manager), **Supervisor**. PostgreSQL/Redis bawaan aaPanel **tidak dipakai**.
- Firewall (aaPanel + panel penyedia VPS): buka hanya 80, 443, SSH, dan port panel. **Jangan buka 5433/6380.**
- SMTP: siapkan akun pengirim (SPF/DKIM untuk `erayadigital.co.id`); API menolak start tanpa `SMTP_HOST`.
- Di server (login sebagai user biasa; semua perintah di bawah memakai `sudo`, tinggal salin-tempel): pasang rsync dan goose:
  ```bash
  sudo apt install -y rsync
  sudo curl -fsSL https://github.com/pressly/goose/releases/latest/download/goose_linux_x86_64 -o /usr/local/bin/goose
  sudo chmod +x /usr/local/bin/goose
  goose --version
  ```

## 1. Struktur folder
```bash
sudo mkdir -p /www/wwwroot/arus/{api,web,deploy,data/uploads,backups}
sudo chown -R www:www /www/wwwroot/arus/{api,web,data}
```
Ambil file deploy dari GitHub. Server **hanya butuh folder `source/deploy/`** (compose, initdb, skrip); kode aplikasi dibangun di laptop dan dikirim sebagai `release.tar.gz` (langkah 4).
```bash
sudo apt install -y git
sudo git clone --depth 1 -b claude/awesome-keller-mdoqpo https://github.com/erayadigitalsolusindo/arus.git /www/wwwroot/arus/repo
sudo cp -r /www/wwwroot/arus/repo/source/deploy/. /www/wwwroot/arus/deploy/
ls /www/wwwroot/arus/deploy        # harus ada: docker-compose.yml  initdb  aapanel
```
- Repo **privat**, jadi git meminta login: *Username* = akun GitHub Anda, *Password* = **Personal Access Token** (bukan password akun). Buat di GitHub → Settings → Developer settings → Fine-grained tokens, pilih repo ini saja, izin **Contents: Read-only**.
- `-b claude/awesome-keller-mdoqpo` dipakai karena PR belum di-merge; setelah merge ganti dengan `-b main`.
- Nama repo mengikuti URL PR (`arus`). Bila clone gagal "not found", cek nama repo di GitHub.
- Tanpa git: dari laptop, `scp -r source/deploy kotakcantik@IP_VPS:~/deploy` lalu `sudo cp -r ~/deploy/. /www/wwwroot/arus/deploy/`.
- Update berikutnya (bila skrip berubah): `sudo git -C /www/wwwroot/arus/repo pull` lalu ulangi `cp -r` (file `.env` Anda tidak tertimpa karena tidak ada di repo).

## 2. Postgres + Redis (Docker)
```bash
cd /www/wwwroot/arus/deploy
sudo cp .env.example .env && sudo nano .env      # semua password: openssl rand -hex 24 ; POSTGRES_PORT=5433 REDIS_PORT=6380
sudo chmod 600 .env
sudo docker compose --env-file .env up -d
sudo docker ps                              # aciraba_postgres & aciraba_redis healthy
sudo ss -tlnp | grep -E '5433|6380'         # harus 127.0.0.1, bukan 0.0.0.0
```
`POSTGRES_APP_PASSWORD` hanya dipakai saat volume pertama kali dibuat; mengubahnya kemudian butuh `ALTER ROLE aciraba_app PASSWORD '…'`.

## 3. Konfigurasi API
```bash
sudo cp /www/wwwroot/arus/deploy/aapanel/env.production.example /www/wwwroot/arus/api/.env
sudo nano /www/wwwroot/arus/api/.env        # isi password (sama dengan deploy/.env), JWT_SECRET, SMTP, PLATFORM_SETUP_TOKEN
sudo chmod 600 /www/wwwroot/arus/api/.env && sudo chown www:www /www/wwwroot/arus/api/.env
```

## 4. Rilis pertama (build di laptop)
PowerShell, dari `source/deploy/aapanel`: `.\build-release.ps1` → `release.tar.gz`.
Unggah ke server (SCP/SFTP/File aaPanel), lalu **pertama kali** jalankan update.sh (ia melakukan migrasi, memasang binary & web; restart akan gagal karena Supervisor belum dibuat — wajar, lanjut langkah 5):
```bash
# release.tar.gz diasumsikan diunggah ke home user (~/release.tar.gz)
sudo env RESTART_CMD=true bash /www/wwwroot/arus/deploy/aapanel/update.sh ~/release.tar.gz
```
(`pg_dump` pertama tetap jalan di DB kosong; tak masalah.)

## 5. Jalankan API via Supervisor
aaPanel → App Store → Supervisor → **Add daemon**:
| Field | Nilai |
|---|---|
| Name | `arus-api` |
| Run user | `www` |
| Run directory | `/www/wwwroot/arus/api` |
| Start command | `/www/wwwroot/arus/api/aciraba-api` |
| Process count | 1 |

Cek log: tidak ada `konfigurasi tidak valid`. Tes: `curl http://127.0.0.1:8080/healthz` → 200 (db & redis ok).
Bila nama proses/perintah restart di aaPanel-mu berbeda, set `RESTART_CMD` saat menjalankan update.sh (default `supervisorctl restart arus-api`).

## 6. Situs di aaPanel
1. **Website → Add site** `arus-api.erayadigital.co.id` (PHP: static/pure) → **SSL → Let's Encrypt** → Force HTTPS. Lalu *Config*: tempel isi `nginx-api.conf` di dalam blok `server { }`.
2. **Add site** `arus.erayadigital.co.id`, root `/www/wwwroot/arus/web` → SSL + Force HTTPS → *Config*: tempel `nginx-web.conf`.
3. Uji: `https://arus-api.erayadigital.co.id/healthz` dan buka `https://arus.erayadigital.co.id/login`; reload di `/items` (setelah login) tidak boleh 404.

## 7. Admin platform pertama
Buka `https://arus.erayadigital.co.id/platform/setup`, masukkan `PLATFORM_SETUP_TOKEN`, buat admin, daftarkan 2FA (wajib).
Lalu **hapus baris `PLATFORM_SETUP_TOKEN` dari `.env` dan restart API.**

## 8. Backup (cron aaPanel → Shell Script, harian)
`bash /www/wwwroot/arus/deploy/aapanel/backup.sh` (cron aaPanel berjalan sebagai root, tidak perlu `sudo`) → `/www/wwwroot/arus/backups` (DB + unggahan, simpan 14 hari).
Wajib: kirim folder itu ke penyimpanan **di luar VPS** (aaPanel Cloud Storage/rclone). **Uji restore sekali** sebelum toko pertama go-live:
`docker exec -i aciraba_postgres pg_restore -U aciraba -d <db_uji> --no-owner < db-YYYY-MM-DD.dump`.

## 9. Update rilis berikutnya
`.\build-release.ps1` → unggah ke `~/release.tar.gz` → `sudo bash /www/wwwroot/arus/deploy/aapanel/update.sh ~/release.tar.gz`.
Urutannya sudah: backup → migrasi → ganti binary → ganti web → restart → cek `/healthz`.
Mundur: migrasi bersifat maju-saja. Bila rilis bermasalah, kembalikan binary (`aciraba-api.prev`) **hanya** bila migrasi rilis itu kompatibel; kalau tidak, restore dump `pre-update-*.dump`.

## Pemecahan masalah
| Gejala | Penyebab umum |
|---|---|
| Login berhasil lalu langsung keluar / refresh 401 | Web & API bukan satu domain induk; atau `CORS_ORIGINS`/`APP_BASE_URL` tak persis `https://arus.erayadigital.co.id` (tanpa slash akhir) |
| Unggah gambar gagal 413 | `client_max_body_size 12m` belum dipasang di situs API |
| Semua pengguna terlihat dari IP server (rate limit/lockout salah sasaran) | `TRUST_PROXY=true` belum diset, atau header `X-Forwarded-For` tak diteruskan nginx |
| API tak mau start | Lihat log Supervisor: biasanya `SMTP_HOST` kosong, `JWT_SECRET` < 32 karakter, `APP_BASE_URL` bukan https |
| PIN penyetuju/2FA mendadak tak valid | `JWT_SECRET` berubah |
| Gambar tak tampil / hilang setelah update | `UPLOAD_DIR` ada di dalam folder yang ditimpa; harus `/www/wwwroot/arus/data/uploads` |

## Varian Apache + systemd + build di server (terbukti 2026-10-08)

Dipakai bila aaPanel memakai **Apache** (bukan nginx) dan server juga menjalankan situs lain: jangan pasang nginx, jangan ganti web server, jangan ganti Node sistem.

- **Alat build di server (terisolasi):** Go ke `/usr/local/go` (cek sha256 dari go.dev/dl), Node ke `/opt/node26` (tarball resmi + `SHASUMS256.txt`). Skrip build memasang `PATH=/opt/node26/bin:/usr/local/go/bin:$PATH` hanya di dalam skrip. Kode dari `git clone` ke `~/arus-src`; hasil build = `~/release.tar.gz` (aciraba-api, migrations, web) dengan isi diperiksa sebelum dikemas.
- **API:** unit systemd (bukan Supervisor): `User=www`, `WorkingDirectory=/www/wwwroot/arus/api`, `ExecStart=.../aciraba-api`, `Restart=always`, `ReadWritePaths=/www/wwwroot/arus/data`. Restart di update: `sudo env RESTART_CMD='systemctl restart arus-api' bash /www/wwwroot/arus/deploy/aapanel/update.sh ~/release.tar.gz`.
- **Situs API (`arus-api...`):** aaPanel → Website → Reverse proxy → target `http://127.0.0.1:8080`, cache mati.
- **Situs web (`arus...`):** Site directory `/www/wwwroot/arus/web`, **matikan anti-cross-site attack** (`.user.ini` immutable merusak `rsync --delete`), URL rewrite Apache:
  ```apache
  RewriteEngine On
  RewriteCond %{DOCUMENT_ROOT}%{REQUEST_URI} !-f
  RewriteCond %{DOCUMENT_ROOT}%{REQUEST_URI} !-d
  RewriteRule . /index.html [L]
  ```
- **Cloudflare:** SSL/TLS mode **Full (strict)**.
- **Cek kesehatan:** `curl -s http://127.0.0.1:8080/healthz` harus `postgres: ok` dan `redis: ok`. Redis `down` hampir selalu password `REDIS_URL` di `api/.env` tak sama dengan `REDIS_PASSWORD` di `deploy/.env`.
- **npm 10:** `npm ci` bisa gagal 'lock file out of sync' (peer opsional `bits-ui`); pakai Node 26 (npm yang sama dengan pembuat lock) atau `npm ci --legacy-peer-deps`.
