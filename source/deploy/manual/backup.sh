#!/usr/bin/env bash
# Dipasang di /etc/cron.d/arus-backup (lihat DEPLOY-MANUAL.md langkah 11). Berjalan sebagai root.
# Simpan salinan di LUAR VPS juga (rclone/rsync ke server lain) — backup di disk yang sama tidak menolong bila VPS hilang.
set -euo pipefail
BASE=/srv/arus
D="$BASE/backups"; mkdir -p "$D"
ts=$(date +%F)
docker exec aciraba_postgres pg_dump -U aciraba -Fc aciraba > "$D/db-$ts.dump"
tar -czf "$D/uploads-$ts.tar.gz" -C "$BASE/data" uploads
find "$D" -name 'db-*.dump' -mtime +14 -delete
find "$D" -name 'uploads-*.tar.gz' -mtime +14 -delete
find "$D" -name 'pre-update-*.dump' -mtime +30 -delete
