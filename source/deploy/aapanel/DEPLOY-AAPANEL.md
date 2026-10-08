# Deploy ACIRABA di aaPanel (VPS sendiri)

Jalur ini **sudah dijalankan di server nyata (2026-10-08)**: aaPanel dengan **Apache**, POS lama di server yang sama.
Domain: **web** `arus.erayadigital.co.id` · **API** `arus-api.erayadigital.co.id` (dua nama di bawah satu domain induk = *same-site*, jadi cookie refresh `SameSite=Lax` berfungsi).

```
Browser ──► Cloudflare (proxy, SSL Full strict) ──► Apache aaPanel (80/443)
                                                      ├─ arus.…     → berkas statis /www/wwwroot/arus/web
                                                      └─ arus-api.… → reverse proxy → 127.0.0.1:8080 (program Go, systemd)
                                                                        ├─ 127.0.0.1:5433 PostgreSQL 16 (Docker)
                                                                        ├─ 127.0.0.1:6380 Redis 7 (Docker)
                                                                        └─ /www/wwwroot/arus/data/uploads
```

Prinsip: **tidak mengubah apa pun milik situs lain** di server (web server, Node bawaan, PHP). Go dan Node untuk build dipasang di folder sendiri; build dilakukan di server (bukan di laptop).
Bila aaPanel Anda memakai **Nginx**, langkah 7 punya varian Nginx (berkas `nginx-*.conf`).

## 0. Prasyarat
- DNS: record `arus` dan `arus-api` mengarah ke server (boleh lewat Cloudflare). Di Cloudflare, SSL/TLS mode **Full (strict)**, bukan Flexible (Flexible membuat cookie `Secure` dan login berputar).
- aaPanel: sudah ada **Docker**. Situs `arus.…` dan `arus-api.…` sudah dibuat di Website (SSL Let's Encrypt aktif, Force HTTPS). **Jangan** memasang Nginx/Supervisor tambahan hanya untuk ini.
- Di server: `git`, `rsync`, `goose` (`goose --version`). Pasang goose bila belum:
  ```bash
  sudo curl -fsSL https://github.com/pressly/goose/releases/latest/download/goose_linux_x86_64 -o /usr/local/bin/goose
  sudo chmod +x /usr/local/bin/goose
  ```
- Siapkan akun SMTP (API menolak start di mode production tanpa `SMTP_HOST`).
- Firewall: buka hanya 80, 443, SSH, dan port panel. **Jangan buka 5433/6380/8080.**

## 1. Folder dan file deploy
```bash
sudo mkdir -p /www/wwwroot/arus/{api,web,deploy,data/uploads,backups}
sudo chown -R www:www /www/wwwroot/arus/{api,web,data}

git clone --depth 1 -b main https://github.com/erayadigitalsolusindo/arus.git ~/arus-src
sudo cp -r ~/arus-src/source/deploy/. /www/wwwroot/arus/deploy/
ls /www/wwwroot/arus/deploy        # harus ada: docker-compose.yml  initdb  aapanel
```
- Repo privat: *Username* = akun GitHub, *Password* = **Personal Access Token** fine-grained (repo ini saja, **Contents: Read-only**).
- Folder `/www/wwwroot/arus/deploy` hanya menyimpan compose, skrip, dan `.env` rahasia. Kode sumber hidup di `~/arus-src`.

## 2. PostgreSQL + Redis (Docker)
```bash
cd /www/wwwroot/arus/deploy
sudo cp .env.example .env && sudo nano .env      # semua password: openssl rand -hex 24 ; POSTGRES_PORT=5433 REDIS_PORT=6380
sudo chmod 600 .env
sudo docker compose --env-file .env up -d
sudo docker ps                                    # aciraba_postgres & aciraba_redis (healthy)
sudo ss -tlnp | grep -E '5433|6380'               # harus 127.0.0.1, bukan 0.0.0.0
```
Gunakan password **hex** (huruf-angka) agar aman di URL koneksi. `POSTGRES_APP_PASSWORD` hanya terbaca saat volume pertama dibuat; mengubahnya kemudian butuh `ALTER ROLE aciraba_app PASSWORD '…'`.

## 3. Konfigurasi API (`api/.env`)
Perintah ini membuat file dan mengisi password dari `deploy/.env` serta `JWT_SECRET` dan token setup secara otomatis. Tidak ada rahasia yang dicetak, dan berhenti bila `api/.env` sudah ada:
```bash
sudo bash <<'EOF'
set -euo pipefail
D=/www/wwwroot/arus
get() { grep -E "^$1=" "$D/deploy/.env" | head -1 | cut -d= -f2-; }
grep -q ganti "$D/deploy/.env" && { echo "STOP: deploy/.env masih berisi password contoh"; exit 1; }
[ -e "$D/api/.env" ] && { echo "STOP: api/.env sudah ada, tidak ditimpa"; exit 1; }
cp "$D/deploy/aapanel/env.production.example" "$D/api/.env"
sed -i \
  -e "s|GANTI_APP_PASSWORD|$(get POSTGRES_APP_PASSWORD)|" \
  -e "s|GANTI_OWNER_PASSWORD|$(get POSTGRES_PASSWORD)|" \
  -e "s|GANTI_REDIS_PASSWORD|$(get REDIS_PASSWORD)|" \
  -e "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -base64 48 | tr -d '\n')|" \
  -e "s|^PLATFORM_SETUP_TOKEN=.*|PLATFORM_SETUP_TOKEN=$(openssl rand -hex 24)|" \
  "$D/api/.env"
chmod 600 "$D/api/.env"; chown www:www "$D/api/.env"
echo "api/.env dibuat"
EOF
sudo nano /www/wwwroot/arus/api/.env      # isi SMTP_HOST/PORT/USER/PASS/FROM
```
- **Simpan `JWT_SECRET` di password manager** (`sudo grep ^JWT_SECRET /www/wwwroot/arus/api/.env`). Jangan diganti setelah live: PIN penyetuju dan 2FA admin jadi tak terbaca.
- Cek tanpa mencetak rahasia: `sudo grep -c GANTI /www/wwwroot/arus/api/.env` harus `0`.
- Password Redis di `api/.env` **harus sama** dengan `deploy/.env`:
  ```bash
  a=$(sudo grep -E '^REDIS_PASSWORD=' /www/wwwroot/arus/deploy/.env | cut -d= -f2-)
  b=$(sudo grep -E '^REDIS_URL=' /www/wwwroot/arus/api/.env | sed -E 's#^REDIS_URL=redis://[^:]*:([^@]*)@.*#\1#')
  [ "$a" = "$b" ] && echo SAMA || echo BEDA
  ```

## 4. Alat build di server (sekali, terisolasi)
Go ke `/usr/local/go`, Node ke `/opt/node26`. Node sistem **tidak diganti** (aplikasi lama mungkin memakainya).
```bash
# Go 1.27.1 — cocokkan hash dengan https://go.dev/dl/
cd /tmp
curl -fLO https://go.dev/dl/go1.27.1.linux-amd64.tar.gz
echo "63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445  go1.27.1.linux-amd64.tar.gz" | sha256sum -c -
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.27.1.linux-amd64.tar.gz
/usr/local/go/bin/go version && rm go1.27.1.linux-amd64.tar.gz

# Node 26 — dicek lewat SHASUMS256.txt
BASE=https://nodejs.org/dist/latest-v26.x
curl -fsSO $BASE/SHASUMS256.txt
F=$(grep -o 'node-v26[0-9.]*-linux-x64\.tar\.xz' SHASUMS256.txt | head -1)
curl -fsSO $BASE/$F
grep " $F\$" SHASUMS256.txt | sha256sum -c -
sudo mkdir -p /opt/node26 && sudo tar -xJf $F -C /opt/node26 --strip-components=1
/opt/node26/bin/node -v && rm -f $F SHASUMS256.txt
```
Kedua pemeriksaan hash harus `OK`; bila tidak, jangan dipasang. Node 26 dipakai (bukan 22) karena `package-lock.json` dibuat dengan npm versi itu; npm 10 menolak lock tersebut (`npm ci` gagal "lock file out of sync" akibat peer opsional `bits-ui`).

## 5. Skrip build, update, dan cek
Salin ke home user admin (bukan dijalankan dari repo, supaya `git pull` tidak mengubah skrip yang sedang berjalan):
```bash
cp ~/arus-src/source/deploy/aapanel/{build-release,update-arus,cek-arus}.sh ~/
chmod +x ~/build-release.sh ~/update-arus.sh ~/cek-arus.sh
```
Bila skrip di repo berubah, salin ulang dengan perintah yang sama.

## 6. Rilis pertama dan service API
Restart sengaja dilewati pada rilis pertama karena service belum ada:
```bash
~/build-release.sh                                  # menghasilkan ~/release.tar.gz (beberapa menit)
sudo env RESTART_CMD=true bash /www/wwwroot/arus/deploy/aapanel/update.sh ~/release.tar.gz
```
Di akhir muncul `PERINGATAN: /healthz tidak menjawab`; itu wajar. Lalu daftarkan API sebagai service **systemd** (bukan Supervisor):
```bash
sudo cp ~/arus-src/source/deploy/aapanel/arus-api.service /etc/systemd/system/arus-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now arus-api
sleep 3; curl -s http://127.0.0.1:8080/healthz; echo
```
Harus: `{"checks":{"postgres":"ok","redis":"ok"},"status":"ok"}`. Log: `sudo journalctl -u arus-api -n 50 --no-pager`.
Service hidup lagi sendiri saat mati (`Restart=always`) dan saat boot. Unit ini juga membatasi proses: tanpa hak istimewa baru, sistem berkas hanya-baca kecuali `/www/wwwroot/arus/data`.

## 7. Situs di aaPanel (Apache)

**Situs API `arus-api.erayadigital.co.id`** — Website → situs itu → **Reverse proxy** → Add:

| Kolom | Isi |
|---|---|
| Proxy name | `arus-api` |
| Target URL | `http://127.0.0.1:8080` |
| Send domain | `$host` |
| Cache | mati |

**Situs web `arus.erayadigital.co.id`:**
1. Settings → **Site directory** → `/www/wwwroot/arus/web`.
2. **Matikan "Anti-cross-site attack" (`open_basedir`).** Bila menyala, aaPanel menaruh `.user.ini` terkunci (`chattr +i`) di folder itu dan `rsync --delete` di `update.sh` gagal.
3. **URL rewrite** (Apache), tempel isi `apache-web-rewrite.conf` (fallback SPA agar refresh `/items` tidak 404):
   ```apache
   RewriteEngine On
   RewriteCond %{DOCUMENT_ROOT}%{REQUEST_URI} !-f
   RewriteCond %{DOCUMENT_ROOT}%{REQUEST_URI} !-d
   RewriteRule . /index.html [L]
   ```
   Pakai `%{DOCUMENT_ROOT}%{REQUEST_URI}` persis seperti itu, **bukan** `%{REQUEST_FILENAME}`: di konteks vhost, `REQUEST_FILENAME` membuat semua berkas (termasuk `/_app/…`) dianggap tidak ada, sehingga CSS/JS diganti `index.html` dan halaman putih.

*Varian Nginx:* tempel `nginx-api.conf` / `nginx-web.conf` di Config situs masing-masing (jangan dipakai bersama fitur Reverse proxy aaPanel).

Uji:
```bash
curl -s https://arus-api.erayadigital.co.id/healthz; echo
curl -sI https://arus.erayadigital.co.id/items | head -3      # 200, bukan 404
```
Lalu buka `https://arus.erayadigital.co.id/login`.

## 8. Admin platform pertama
```bash
sudo grep ^PLATFORM_SETUP_TOKEN /www/wwwroot/arus/api/.env
```
Buka `https://arus.erayadigital.co.id/platform/setup`, isi token, buat admin, daftarkan 2FA (wajib). Lalu **hapus baris `PLATFORM_SETUP_TOKEN` dari `api/.env`** dan `sudo systemctl restart arus-api`.

## 9. Backup terjadwal
```bash
sudo bash -c 'echo "30 2 * * * root bash /www/wwwroot/arus/deploy/aapanel/backup.sh >> /var/log/arus-backup.log 2>&1" > /etc/cron.d/arus-backup'
sudo bash /www/wwwroot/arus/deploy/aapanel/backup.sh && ls -lh /www/wwwroot/arus/backups
```
Database + unggahan, disimpan 14 hari. **Wajib** disalin ke luar server (rclone/Cloud Storage aaPanel) dan **uji restore sekali** sebelum toko pertama go-live:
`sudo docker exec -i aciraba_postgres pg_restore -U aciraba -d <db_uji> --no-owner < db-YYYY-MM-DD.dump`.

## 10. Update rilis berikutnya

### Update biasa
Sebagai user biasa (bukan root), dari folder mana pun:
```bash
~/update-arus.sh
~/cek-arus.sh
```
Lalu buka aplikasi di browser, tekan **Ctrl+Shift+R**, dan uji satu nota di `/kasir`.

`update-arus.sh` mengerjakan berurutan dan berhenti di langkah pertama yang gagal:

| # | Langkah | Catatan |
|---|---|---|
| 1 | `git pull --ff-only` di `~/arus-src` | Riwayat dangkal: lihat Pemecahan masalah |
| 2 | Build (`build-release.sh`) | Go + Node 26 di server, hasil `~/release.tar.gz` |
| 3 | Backup database | `backups/pre-update-*.dump` |
| 4 | Migrasi (goose) | Hanya maju, tidak bisa dimundurkan otomatis |
| 5 | Ganti program | Yang lama disimpan sebagai `aciraba-api.prev` |
| 6 | Ganti web | `rsync` ke `/www/wwwroot/arus/web` (`.user.ini` dikecualikan) |
| 7 | Restart `arus-api` + cek `/healthz` | |

### Sebelum update
- Pastikan ada dump terbaru di luar VPS (`backup.sh` + salin keluar). Dump `pre-update-*` ada di VPS yang sama, jadi tidak melindungi dari kerusakan disk.
- Lakukan saat toko sepi. Ada jeda beberapa detik saat API restart.
- Bila skrip di repo berubah (`build-release.sh`, `update-arus.sh`, `cek-arus.sh`), salin ulang ke home dan sinkronkan folder deploy:
  ```bash
  cp ~/arus-src/source/deploy/aapanel/{build-release,update-arus,cek-arus}.sh ~/
  chmod +x ~/build-release.sh ~/update-arus.sh ~/cek-arus.sh
  cp -r ~/arus-src/source/deploy/. /www/wwwroot/arus/deploy/
  ```
  (File `.env` aman, tidak ada di repo.)

### Setelah update
- Rilis yang menambah modul atau izin baru: role selain Owner **tidak otomatis** mendapat izin itu. Buka *Role & Hak Akses* dan centang untuk role yang perlu.
- Jalankan `~/cek-arus.sh`: semua `[OK]`. `[MASALAH]` pada baris Backup wajar sampai langkah 9 dijalankan.

### Bila update berhenti di tengah
Lihat langkah terakhir yang tercetak, lalu:

| Berhenti di | Artinya | Tindakan |
|---|---|---|
| `build` atau `git pull` | Belum ada yang berubah di server | Perbaiki penyebabnya, jalankan lagi |
| `migrasi` | Skema mungkin sebagian berubah | Baca pesan goose; bila perlu restore dump, lalu ulangi |
| `ganti web` (`rsync`) | **Database sudah baru, program sudah baru, tetapi API belum di-restart** | Selesaikan manual di bawah |

Menyelesaikan manual setelah gagal di `rsync`:
```bash
tmp=$(mktemp -d) && tar -xzf ~/release.tar.gz -C "$tmp"
sudo rsync -a --delete --exclude='.user.ini' "$tmp/web/" /www/wwwroot/arus/web/
sudo find /www/wwwroot/arus/web ! -name .user.ini -exec chown www:www {} +
rm -rf "$tmp"
sudo systemctl restart arus-api
sleep 2 && curl -fsS http://127.0.0.1:8080/healthz && echo
```
`.user.ini` dikunci aaPanel (`chattr +i`); jangan dilepas, cukup dikecualikan.

### Mundur
Migrasi hanya maju. Kembalikan program lama hanya bila migrasi rilis itu kompatibel:
```bash
cd /www/wwwroot/arus/api
sudo install -m 755 -o www -g www aciraba-api.prev aciraba-api
sudo systemctl restart arus-api
```
Bila tidak kompatibel, hentikan API dan restore dump `pre-update-*.dump` (semua transaksi sesudah backup itu hilang).

## 11. Cek kesehatan
```bash
~/cek-arus.sh
```
Memeriksa API, akses dari internet, Docker, versi migrasi, umur backup, disk, dan error 1 jam terakhir. `[MASALAH]` pada baris Backup wajar sampai langkah 9 dijalankan.

## Pemecahan masalah
| Gejala | Penyebab umum |
|---|---|
| API menolak start: `JWT_SECRET wajib diisi, minimal 32 karakter` | `JWT_SECRET` kosong atau masih placeholder |
| `healthz` → `redis: down` | Password di `REDIS_URL` beda dengan `REDIS_PASSWORD` di `deploy/.env` (langkah 3) |
| `npm ci` gagal "lock file out of sync" | Memakai npm 10; pakai Node 26 (langkah 4) |
| `npm ci` gagal `EPERM … lightningcss` (Windows) | Dev server/editor memegang `node_modules`; build di server menghindarinya |
| Halaman putih, CSS/JS tak termuat | Rewrite memakai `%{REQUEST_FILENAME}` (lihat langkah 7) |
| `update.sh` gagal di `rsync` (`unlink(.user.ini)`) | `.user.ini` dikunci aaPanel. `update.sh` kini memakai `--exclude='.user.ini'`; bila masih memakai versi lama, salin ulang folder deploy (§10) atau selesaikan manual (§10 "Bila update berhenti di tengah"). Jangan menghapus berkasnya |
| Login berhasil lalu langsung keluar / refresh 401 | `CORS_ORIGINS`/`APP_BASE_URL` tak persis `https://arus.erayadigital.co.id` (tanpa slash akhir), atau Cloudflare mode Flexible |
| Unggah gambar gagal 413 | Batas ukuran unggahan di situs API (Apache `LimitRequestBody`, Nginx `client_max_body_size 12m`) |
| Semua pengguna terlihat dari satu IP (rate limit salah sasaran) | `TRUST_PROXY=true` belum diset |
| PIN penyetuju/2FA mendadak tak valid | `JWT_SECRET` berubah |
| `git pull` menolak (riwayat dangkal) | Bila tak ada perubahan lokal: `git fetch --depth 1 origin main && git reset --hard origin/main` |
| Gambar hilang setelah update | `UPLOAD_DIR` harus `/www/wwwroot/arus/data/uploads` (di luar folder yang ditimpa) |
