# Deploy ACIRABA tanpa aaPanel (Ubuntu 22.04/24.04, VPS sendiri)

Domain: **web** `arus.erayadigital.co.id` · **API** `arus-api.erayadigital.co.id` (satu domain induk → cookie refresh `SameSite=Lax` berfungsi).
Semua perintah memakai `sudo` (login sebagai user biasa), tinggal salin-tempel.

> **Jangan pasang panduan ini di server yang sudah ber-aaPanel.** aaPanel memakai nginx-nya sendiri di port 80/443; nginx dari `apt` akan bentrok. Panduan ini untuk server bersih, atau bila aaPanel dicopot. Pilih salah satu jalur per server.

```
Browser ──https──► nginx (apt) ─┬─ arus.…     → file statis /srv/arus/web
                                └─ arus-api.… → 127.0.0.1:8080 (binary Go, systemd)
                                                 ├─ 127.0.0.1:5433 Postgres 16 (Docker)
                                                 └─ 127.0.0.1:6380 Redis 7 (Docker)
Sertifikat: certbot (Let's Encrypt, perpanjangan otomatis lewat systemd timer)
```

Bedanya dengan jalur aaPanel hanya di **cara mengelola**: nginx, SSL, proses API, cron, dan firewall dikerjakan lewat file/perintah, bukan klik di panel. Arsitektur, env, migrasi, dan skrip rilis sama.

## 0. Prasyarat
- DNS: record **A** `arus` dan `arus-api` (zona `erayadigital.co.id`) → IP VPS. Cek: `dig +short arus.erayadigital.co.id`.
- SMTP: akun pengirim (SPF/DKIM `erayadigital.co.id`); API menolak start tanpa `SMTP_HOST`.
- Akses SSH berkunci ke VPS dan user dengan `sudo`.

## 1. Paket dasar dan firewall
```bash
sudo apt update && sudo apt install -y nginx certbot python3-certbot-nginx docker.io docker-compose-v2 rsync git curl ufw
sudo systemctl enable --now docker nginx

# goose (migrasi database)
sudo curl -fsSL https://github.com/pressly/goose/releases/latest/download/goose_linux_x86_64 -o /usr/local/bin/goose
sudo chmod +x /usr/local/bin/goose
goose --version

# Firewall: hanya SSH, HTTP, HTTPS. SSH diizinkan DULU agar tidak terkunci.
sudo ufw allow OpenSSH
sudo ufw allow 80,443/tcp
sudo ufw enable
sudo ufw status
```
Postgres/Redis di-bind ke `127.0.0.1` oleh compose, jadi tidak terbuka walau Docker menulis aturan iptables sendiri (Docker mengabaikan ufw untuk port yang dipublikasikan ke semua alamat; itulah alasan bind ke localhost wajib).

## 2. User layanan dan folder
```bash
sudo useradd --system --home-dir /srv/arus --shell /usr/sbin/nologin arus
sudo mkdir -p /srv/arus/{api,web,deploy,data/uploads,backups}
sudo chown -R arus:arus /srv/arus/{api,web,data}
```
`arus` tidak bisa login dan hanya memiliki folder aplikasinya; API berjalan sebagai user ini, bukan root.

## 3. Ambil file deploy dari GitHub
```bash
sudo git clone --depth 1 -b claude/awesome-keller-mdoqpo https://github.com/erayadigitalsolusindo/arus.git /srv/arus/repo
sudo cp -r /srv/arus/repo/source/deploy/. /srv/arus/deploy/
ls /srv/arus/deploy        # harus ada: docker-compose.yml  initdb  aapanel  manual
```
Repo privat: *Username* = akun GitHub, *Password* = **Personal Access Token** fine-grained (repo ini saja, **Contents: Read-only**). Setelah PR di-merge, ganti `-b` dengan `main`. Nama repo mengikuti URL PR; bila "not found", cek nama di GitHub.
Update berikutnya: `sudo git -C /srv/arus/repo pull` lalu ulangi `cp -r` (file `.env` Anda aman, tidak ada di repo).

## 4. Postgres + Redis (Docker)
```bash
cd /srv/arus/deploy
sudo cp .env.example .env && sudo nano .env     # semua password: openssl rand -hex 24 ; POSTGRES_PORT=5433 REDIS_PORT=6380
sudo chmod 600 .env
sudo docker compose --env-file .env up -d
sudo docker ps                                  # aciraba_postgres & aciraba_redis (healthy)
sudo ss -tlnp | grep -E '5433|6380'             # harus 127.0.0.1, bukan 0.0.0.0
```
Container ber-`restart: unless-stopped` dan Docker aktif saat boot, jadi hidup lagi setelah reboot. `POSTGRES_APP_PASSWORD` hanya dipakai saat volume pertama dibuat; mengubahnya kemudian butuh `ALTER ROLE aciraba_app PASSWORD '…'`.

## 5. Konfigurasi API
```bash
sudo cp /srv/arus/deploy/manual/env.production.example /srv/arus/api/.env
sudo nano /srv/arus/api/.env
sudo chmod 600 /srv/arus/api/.env && sudo chown arus:arus /srv/arus/api/.env
```
Isian (password harus sama dengan `deploy/.env`: `sudo grep PASSWORD /srv/arus/deploy/.env`):

| Baris | Isi |
|---|---|
| `DATABASE_URL` | ganti `GANTI_APP_PASSWORD` → `POSTGRES_APP_PASSWORD` |
| `MIGRATE_DATABASE_URL` | ganti `GANTI_OWNER_PASSWORD` → `POSTGRES_PASSWORD` |
| `REDIS_URL` | ganti `GANTI_REDIS_PASSWORD` → `REDIS_PASSWORD` |
| `JWT_SECRET` | `openssl rand -base64 48`. **Simpan di password manager; jangan diganti setelah live** (PIN penyetuju dan 2FA admin jadi tak terbaca) |
| `PLATFORM_SETUP_TOKEN` | `openssl rand -hex 24`; hapus barisnya setelah admin platform dibuat |
| `SMTP_HOST/PORT/USER/PASS/FROM` | dari penyedia email (587 STARTTLS atau 465 TLS) |

Sisanya (`APP_ENV`, `CORS_ORIGINS`, `APP_BASE_URL`, `TRUST_PROXY`, `UPLOAD_DIR`) sudah benar untuk domain Anda.

## 6. Rilis pertama (build di laptop)
PowerShell, dari `source/deploy/aapanel` (skrip yang sama dipakai kedua jalur): `.\build-release.ps1` → `release.tar.gz`.
Unggah ke home user di server (`scp release.tar.gz kotakcantik@IP_VPS:~/`). Lalu **pertama kali** (restart sengaja dilewati karena service belum dibuat):
```bash
sudo env RESTART_CMD=true bash /srv/arus/deploy/manual/update.sh ~/release.tar.gz
```
Ini membuat backup (DB kosong, tak masalah), menjalankan migrasi, memasang binary ke `/srv/arus/api/` dan web ke `/srv/arus/web/`. Pesan `PERINGATAN: /healthz tidak menjawab` di akhir wajar pada tahap ini.

## 7. Jalankan API sebagai service systemd
```bash
sudo cp /srv/arus/deploy/manual/arus-api.service /etc/systemd/system/arus-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now arus-api
sudo systemctl status arus-api --no-pager
curl http://127.0.0.1:8080/healthz          # 200, db & redis ok
```
Log: `sudo journalctl -u arus-api -f`. Proses otomatis hidup lagi bila mati (`Restart=always`) dan saat boot. Unit ini juga mengeraskan proses: tanpa hak istimewa baru, sistem file hanya-baca kecuali `/srv/arus/data`, `/home` tak terlihat.
Tidak boleh ada `konfigurasi tidak valid` di log; bila ada, biasanya `SMTP_HOST` kosong, `JWT_SECRET` < 32 karakter, atau `APP_BASE_URL` bukan https.

## 8. nginx + SSL
```bash
sudo cp /srv/arus/deploy/manual/nginx-web.conf /etc/nginx/sites-available/arus-web
sudo cp /srv/arus/deploy/manual/nginx-api.conf /etc/nginx/sites-available/arus-api
sudo ln -s /etc/nginx/sites-available/arus-web /etc/nginx/sites-enabled/
sudo ln -s /etc/nginx/sites-available/arus-api /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx

# SSL: certbot menambah blok 443 + redirect HTTP→HTTPS ke file di atas
sudo certbot --nginx -d arus.erayadigital.co.id -d arus-api.erayadigital.co.id --redirect --agree-tos -m EMAIL_ANDA
sudo systemctl list-timers | grep certbot       # perpanjangan otomatis aktif
sudo certbot renew --dry-run
```
Uji: `https://arus-api.erayadigital.co.id/healthz`, lalu buka `https://arus.erayadigital.co.id/login`. Login, buka `/items`, refresh: tidak boleh 404.

## 9. Admin platform pertama
Buka `https://arus.erayadigital.co.id/platform/setup`, masukkan `PLATFORM_SETUP_TOKEN`, buat admin, daftarkan 2FA (wajib). Lalu hapus baris `PLATFORM_SETUP_TOKEN` dari `/srv/arus/api/.env` dan `sudo systemctl restart arus-api`.

## 10. Update rilis berikutnya
`.\build-release.ps1` → `scp release.tar.gz …:~/` →
```bash
sudo bash /srv/arus/deploy/manual/update.sh ~/release.tar.gz
```
Urutan: backup → migrasi → ganti binary → ganti web → restart → cek `/healthz`. Mundur: migrasi hanya maju. Kembalikan `aciraba-api.prev` **hanya** bila migrasi rilis itu kompatibel; kalau tidak, restore `pre-update-*.dump`.

## 11. Backup terjadwal
```bash
sudo tee /etc/cron.d/arus-backup >/dev/null <<'CRON'
30 2 * * * root /srv/arus/deploy/manual/backup.sh >> /var/log/arus-backup.log 2>&1
CRON
sudo bash /srv/arus/deploy/manual/backup.sh && ls -lh /srv/arus/backups     # uji sekali sekarang
```
DB + unggahan, disimpan 14 hari di `/srv/arus/backups`. **Wajib** disalin ke luar VPS (rclone ke S3/Drive atau rsync ke server lain) dan **uji restore sekali** sebelum toko pertama go-live:
`sudo docker exec -i aciraba_postgres pg_restore -U aciraba -d <db_uji> --no-owner < db-YYYY-MM-DD.dump`.

## 12. Pengerasan dasar (disarankan)
```bash
sudo apt install -y unattended-upgrades fail2ban && sudo dpkg-reconfigure -plow unattended-upgrades
```
- SSH: nonaktifkan login password (`PasswordAuthentication no` di `/etc/ssh/sshd_config.d/`) setelah kunci SSH terbukti jalan; jangan tutup sesi lama sebelum menguji sesi baru.
- Pantau disk (`df -h`): unggahan, backup, dan log Docker tumbuh terus.

## Pemecahan masalah
| Gejala | Penyebab umum |
|---|---|
| `nginx -t` gagal / port 80 dipakai | aaPanel atau web server lain masih aktif di server yang sama (`sudo ss -tlnp \| grep ':80 '`) |
| certbot gagal | DNS belum mengarah ke VPS, atau port 80 diblokir firewall penyedia |
| Login berhasil lalu langsung keluar / refresh 401 | `CORS_ORIGINS`/`APP_BASE_URL` tak persis `https://arus.erayadigital.co.id` (tanpa slash akhir), atau web & API beda domain induk |
| 502 Bad Gateway di API | service mati: `sudo systemctl status arus-api`, `journalctl -u arus-api -n 50` |
| Unggah gambar 413 | `client_max_body_size 12m` tak terpasang di server block API |
| Rate limit/lockout memukul semua pengguna sekaligus | `TRUST_PROXY=true` belum diset atau nginx tak meneruskan `X-Forwarded-For` |
| PIN penyetuju/2FA mendadak tak valid | `JWT_SECRET` berubah |
| `permission denied` menulis unggahan | `sudo chown -R arus:arus /srv/arus/data` |

## aaPanel vs manual: perbandingan

| Aspek | aaPanel | Manual (panduan ini) |
|---|---|---|
| Kemudahan awal | Klik di panel untuk situs, SSL, cron, Supervisor | Perlu nyaman dengan terminal; sekali salin-tempel lalu selesai |
| SSL | Let's Encrypt via panel | certbot + systemd timer (setara) |
| Proses API | Supervisor (plugin) | systemd: bawaan OS, ikut boot, log lewat `journalctl`, ada pengerasan sandbox |
| Konfigurasi nginx | Lewat UI/kolom config; aaPanel menulis ulang vhost | File biasa di `/etc/nginx`, bisa disimpan di git dan direproduksi |
| Permukaan serangan | Panel web tambahan (port panel, login, plugin) yang harus diamankan dan di-update | Tidak ada panel; hanya SSH, 80, 443 |
| Dipakai bersama situs lain | Cocok bila VPS menampung banyak situs/PHP/MySQL | Cocok untuk VPS khusus ACIRABA |
| Mengulang/pindah server | Banyak langkah klik, sulit direproduksi persis | Seluruh langkah berupa perintah; mudah diulang atau diotomatisasi |
| Pemecahan masalah | Log tersebar (panel, nginx, Supervisor) | `journalctl`, `nginx -t`, satu pola baku di semua distro Ubuntu/Debian |
| Pemeliharaan | Update panel + paket OS | Hanya paket OS (unattended-upgrades) |

**Rekomendasi:** bila VPS ini juga menampung situs lain yang Anda kelola lewat aaPanel, tetap pakai jalur aaPanel. Bila VPS ini khusus ACIRABA (atau akan dipindah ke server produksi sungguhan), jalur manual lebih ramping dan lebih mudah diaudit: satu komponen berkurang yang harus diamankan, dan seluruh konfigurasi berupa file.
