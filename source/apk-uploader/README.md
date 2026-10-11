# apk-uploader

Unggah APK ARUS Kasir dari PC Windows ke server lewat SFTP (SSH), melewati Cloudflare/Apache/API.

- Pakai: seret berkas `.apk` ke `apk-uploader.exe` (atau jalankan lalu ketik path-nya). Default: `root@200.200.200.2:22`.
- Opsi: `-host -port -user -dir -owner -version` (lihat `apk-uploader.exe -h`).
- Berkas ditaruh di `<UPLOAD_DIR>/mobile/arus-kasir.apk` + `arus-kasir-info.json` (format sama dengan `platformadmin/apk.go`).
  Ubah `-dir` bila `UPLOAD_DIR` server bukan `/www/wwwroot/arus/data/uploads`.
- Password SSH diketik tiap kali (tidak disimpan). Sidik jari server disimpan di `known_host.txt` di samping exe (trust-on-first-use).
- Melewati validasi API dan entri audit `platform.apk_upload`; APK diperiksa di PC (ZIP + `AndroidManifest.xml`, ≤150 MB).
- Build: `./build.sh` → `dist/apk-uploader.exe`.
