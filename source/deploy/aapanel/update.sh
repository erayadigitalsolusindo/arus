#!/usr/bin/env bash
# Jalankan di server sebagai root:  bash update.sh /path/release.tar.gz
# Urutan: backup DB -> migrasi -> ganti binary -> ganti web -> restart. Berhenti di langkah pertama yang gagal.
set -euo pipefail

BASE=/www/wwwroot/arus
RELEASE=${1:?pakai: update.sh /path/release.tar.gz}
RESTART_CMD=${RESTART_CMD:-supervisorctl restart arus-api}   # sesuaikan dengan nama proses di Supervisor aaPanel
APP_USER=${APP_USER:-www}

# Jangan `source` .env: nilai seperti SMTP_FROM=ACIRABA <x@y> memuat karakter khusus shell.
MIGRATE_DATABASE_URL=$(grep -E '^MIGRATE_DATABASE_URL=' "$BASE/api/.env" | head -1 | cut -d= -f2-)
[ -n "$MIGRATE_DATABASE_URL" ] || { echo "MIGRATE_DATABASE_URL kosong di $BASE/api/.env"; exit 1; }
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
tar -xzf "$RELEASE" -C "$tmp"

echo "==> 1/5 backup database"
mkdir -p "$BASE/backups"
docker exec aciraba_postgres pg_dump -U aciraba -Fc aciraba > "$BASE/backups/pre-update-$(date +%F-%H%M%S).dump"

echo "==> 2/5 migrasi"
goose -dir "$tmp/migrations" postgres "$MIGRATE_DATABASE_URL" up

echo "==> 3/5 ganti binary"
[ -f "$BASE/api/aciraba-api" ] && cp "$BASE/api/aciraba-api" "$BASE/api/aciraba-api.prev"
install -m 755 -o "$APP_USER" -g "$APP_USER" "$tmp/aciraba-api" "$BASE/api/aciraba-api.new"
mv "$BASE/api/aciraba-api.new" "$BASE/api/aciraba-api"

echo "==> 4/5 ganti web"
rsync -a --delete "$tmp/web/" "$BASE/web/"
chown -R "$APP_USER:$APP_USER" "$BASE/web"

echo "==> 5/5 restart API"
$RESTART_CMD
sleep 2
curl -fsS http://127.0.0.1:8080/healthz && echo && echo "OK" || { echo "PERINGATAN: /healthz tidak menjawab — cek log Supervisor (normal pada pemasangan pertama sebelum proses dibuat)"; exit 1; }
