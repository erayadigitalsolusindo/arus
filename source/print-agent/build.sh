#!/usr/bin/env bash
# Build print-agent untuk Windows (tanpa jendela konsol) dan Linux, lalu kemas bersama skrip pemasangan.
set -euo pipefail
cd "$(dirname "$0")"
out=dist
rm -rf "$out" && mkdir -p "$out/windows" "$out/linux"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui -s -w" -o "$out/windows/print-agent.exe" .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$out/linux/print-agent" .
cp windows/*.cmd "$out/windows/"
(cd "$out" && zip -qr arus-print-agent-windows.zip windows)
echo "Selesai: $out/arus-print-agent-windows.zip"
