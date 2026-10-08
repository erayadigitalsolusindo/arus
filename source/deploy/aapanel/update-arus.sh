#!/usr/bin/env bash
# Satu perintah update: ambil kode -> build -> backup -> migrasi -> pasang -> restart -> cek kesehatan.
# Jalankan sebagai user biasa (sudo diminta di tahap pemasangan). Lihat DEPLOY-AAPANEL.md langkah 10.
set -euo pipefail
cd "$HOME/arus-src"
git pull --ff-only
echo "==> kode sekarang:"; git log --oneline -1
"$HOME/build-release.sh"
sudo env RESTART_CMD='systemctl restart arus-api' \
  bash /www/wwwroot/arus/deploy/aapanel/update.sh "$HOME/release.tar.gz"
