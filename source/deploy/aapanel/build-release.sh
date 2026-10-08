#!/usr/bin/env bash
# Build rilis DI SERVER: kode dari ~/arus-src -> ~/release.tar.gz (aciraba-api, migrations, web).
# Salin ke ~ lalu jalankan; lihat DEPLOY-AAPANEL.md langkah 4-5.
set -euo pipefail
export PATH=/opt/node26/bin:/usr/local/go/bin:$PATH
SRC=$HOME/arus-src/source
API_URL=https://arus-api.erayadigital.co.id
OUT=$(mktemp -d)

[ "$(uname -m)" = x86_64 ] || { echo "GAGAL: server bukan x86_64"; exit 1; }
echo "==> alat"; go version; node -v

echo "==> 1/3 build API (Go)"
( cd "$SRC/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$OUT/aciraba-api" ./cmd/api )
cp -r "$SRC/backend/db/migrations" "$OUT/migrations"

echo "==> 2/3 build web (SvelteKit)"
( cd "$SRC/web" && npm ci && VITE_API_URL="$API_URL" npm run build )
mkdir -p "$OUT/web"
cp -r "$SRC/web/build/." "$OUT/web/"

echo "==> 3/3 cek isi dan kemas"
for f in aciraba-api migrations/00001_init_tenancy.sql web/index.html; do
  [ -e "$OUT/$f" ] || { echo "GAGAL: $f tidak ada, rilis tidak dibuat"; exit 1; }
done
tar -czf "$HOME/release.tar.gz" -C "$OUT" .
rm -rf "$OUT"
ls -lh "$HOME/release.tar.gz"
echo "SELESAI"
