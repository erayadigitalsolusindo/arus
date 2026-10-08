#!/usr/bin/env bash
# Cron harian di aaPanel (Cron → Shell Script), mis. 02:30:  bash /www/wwwroot/arus/deploy/aapanel/backup.sh
# Simpan salinan di LUAR VPS juga (aaPanel: plugin Cloud Storage / rclone) — backup di disk yang sama tidak menolong bila VPS hilang.
set -euo pipefail
BASE=/www/wwwroot/arus
D="$BASE/backups"; mkdir -p "$D"
ts=$(date +%F)
docker exec aciraba_postgres pg_dump -U aciraba -Fc aciraba > "$D/db-$ts.dump"
tar -czf "$D/uploads-$ts.tar.gz" -C "$BASE/data" uploads
find "$D" -name 'db-*.dump' -mtime +14 -delete
find "$D" -name 'uploads-*.tar.gz' -mtime +14 -delete
find "$D" -name 'pre-update-*.dump' -mtime +30 -delete
