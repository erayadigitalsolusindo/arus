# Jalankan di laptop (PowerShell) dari folder source/deploy/aapanel:  .\build-release.ps1
# Hasil: release.tar.gz (binary Linux + migration + web statis). Unggah ke server lalu jalankan update.sh.
$ErrorActionPreference = 'Stop'
$ApiUrl = 'https://api.arus.erayadigital.co.id'
$root = Resolve-Path "$PSScriptRoot\..\.."
$out  = Join-Path $PSScriptRoot 'release'
Remove-Item $out -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory "$out\web" | Out-Null

Push-Location "$root\backend"
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build -trimpath -ldflags '-s -w' -o "$out\aciraba-api" ./cmd/api
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Copy-Item db\migrations "$out\migrations" -Recurse
Pop-Location

Push-Location "$root\web"
$env:VITE_API_URL = $ApiUrl
npm ci
npm run build
Remove-Item Env:VITE_API_URL
Copy-Item build\* "$out\web" -Recurse
Pop-Location

Push-Location $PSScriptRoot
tar -czf release.tar.gz -C release .
Pop-Location
Write-Host "Selesai: $PSScriptRoot\release.tar.gz"
