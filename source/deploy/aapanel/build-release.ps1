# Jalankan di laptop (PowerShell) dari folder source/deploy/aapanel:  .\build-release.ps1
# Hasil: release.tar.gz (binary Linux + migration + web statis). Unggah ke server lalu jalankan update.sh.
$ErrorActionPreference = 'Stop'
$ApiUrl = 'https://arus-api.erayadigital.co.id'
$root = Resolve-Path "$PSScriptRoot\..\.."
$out  = Join-Path $PSScriptRoot 'release'

# $ErrorActionPreference tidak menangkap kode keluar program eksternal (go, npm, tar):
# tanpa pemeriksaan ini langkah yang gagal diam-diam menghasilkan rilis rusak
# (update.sh memakai rsync --delete, jadi web kosong akan menghapus web yang live).
function Assert-Ok([string]$step) {
    if ($LASTEXITCODE -ne 0) { throw "GAGAL: $step (kode keluar $LASTEXITCODE). Rilis tidak dibuat." }
}

Remove-Item $out, (Join-Path $PSScriptRoot 'release.tar.gz') -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory "$out\web" | Out-Null

Push-Location "$root\backend"
try {
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    go build -trimpath -ldflags '-s -w' -o "$out\aciraba-api" ./cmd/api
    $code = $LASTEXITCODE
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
$LASTEXITCODE = $code; Assert-Ok 'go build'
Copy-Item "$root\backend\db\migrations" "$out\migrations" -Recurse

Push-Location "$root\web"
try {
    $env:VITE_API_URL = $ApiUrl
    npm ci;           Assert-Ok 'npm ci (hentikan npm run dev / editor yang memakai source\web bila EPERM)'
    npm run build;    Assert-Ok 'npm run build'
} finally {
    Remove-Item Env:VITE_API_URL -ErrorAction SilentlyContinue
    Pop-Location
}
Copy-Item "$root\web\build\*" "$out\web" -Recurse

foreach ($f in 'aciraba-api', 'migrations\00001_init_tenancy.sql', 'web\index.html') {
    if (-not (Test-Path (Join-Path $out $f))) { throw "GAGAL: $f tidak ada di hasil build. Rilis tidak dibuat." }
}

Push-Location $PSScriptRoot
tar -czf release.tar.gz -C release .
$code = $LASTEXITCODE
Pop-Location
$LASTEXITCODE = $code; Assert-Ok 'tar'
Write-Host "Selesai: $PSScriptRoot\release.tar.gz"
