#!/usr/bin/env bash
# Build apk-uploader.exe (Windows, konsol) ke dist/.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/apk-uploader.exe .
echo "Selesai: dist/apk-uploader.exe"
