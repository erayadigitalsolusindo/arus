# Domain Kustom (White Label) per Tenant

Tenant bisa memakai domain sendiri, mis. `arus.kotakcantik.com`, untuk membuka ARUS. Tenant tetap ditentukan dari akun login, jadi tidak ada perubahan data; domain hanya alamat lain ke aplikasi yang sama.

```
Browser ──► arus.kotakcantik.com (CNAME → edge.erayadigital.co.id)
              │
           Caddy (server edge, TLS otomatis per domain)
              ├─ halaman & berkas statis → https://arus.erayadigital.co.id        (web)
              └─ panggilan API           → https://arus-api.erayadigital.co.id    (API Go)
```

Web dan API terlihat satu origin oleh browser, jadi cookie refresh tetap first-party dan CORS tidak terlibat (`CORS_ORIGINS` tidak perlu diubah).

Sertifikat TLS hanya diterbitkan untuk domain yang berstatus **Aktif** di Platform Admin: Caddy bertanya ke `GET /public/domain-check?domain=...` sebelum menerbitkan.

> Status: kode backend/web sudah dibuat dan lolos build + tes unit. **Caddyfile belum pernah dijalankan** dan alur end-to-end belum diuji; ikuti bagian "Uji pertama" dan periksa log.

## A. Server edge (sekali)

Mesin terpisah dengan IP publik sendiri. Port 80 dan 443 harus bebas, jadi bukan di server aaPanel.

1. DNS: `edge.erayadigital.co.id` → A record ke IP server edge. Di Cloudflare set **DNS only** (awan abu-abu).
2. Firewall: buka port **80** (wajib, untuk verifikasi Let's Encrypt) dan **443**.
3. Pasang Caddy:
   ```bash
   sudo apt install -y caddy
   ```
4. Salin `source/deploy/edge/Caddyfile` ke `/etc/caddy/Caddyfile`, lalu sesuaikan:
   - `email` → email Anda;
   - host `arus-api.…` (di `ask` dan `api_upstream`) dan `arus.…` (di `web_upstream`) → host produksi Anda.
5. Jalankan:
   ```bash
   sudo caddy validate --config /etc/caddy/Caddyfile
   sudo systemctl reload caddy
   ```

## B. API produksi (sekali)

1. Env produksi:
   ```
   CUSTOM_DOMAIN_TARGET=edge.erayadigital.co.id
   TRUST_PROXY=true
   ```
   `CUSTOM_DOMAIN_TARGET` = host yang harus dituju CNAME klien. Bila kosong, bawaannya host dari `APP_BASE_URL`.
2. Terapkan migration `00086` dan `00087` ke DB produksi (skrip rilis `update-arus.sh` biasanya menjalankannya).
3. Deploy backend baru, restart `arus-api`.

## C. Web (sekali)

1. Env build web: `VITE_PRIMARY_HOSTS=arus.erayadigital.co.id` (host utama; pisahkan dengan koma bila lebih dari satu). `VITE_API_URL` tidak berubah.
2. Build dan rilis seperti biasa.

Halaman yang dibuka dari host di luar daftar itu (dan bukan `localhost`) memanggil API lewat origin yang sama. Tanpa langkah ini, login di domain klien gagal karena cookie lintas origin.

## D. Cek cepat

```bash
curl -i "https://arus-api.erayadigital.co.id/public/domain-check?domain=belum-ada.example.com"
```
Harus `404`. Lalu login ke Platform Admin dan pastikan menu **Domain** muncul.

## E. Menambah klien (tiap domain)

1. **Platform → Domain → Tambah Domain**: isi kode tenant dan domain (mis. `arus.kotakcantik.com`). Status awal "Menunggu DNS".
2. Klien membuat record DNS di domain mereka:
   ```
   arus   CNAME   edge.erayadigital.co.id
   ```
   Bila mereka memakai Cloudflare, record itu harus **DNS only**.
3. Tunggu DNS menyebar, cek:
   ```bash
   nslookup -type=CNAME arus.kotakcantik.com
   ```
4. Klik **Verifikasi**. Cocok → status **Aktif**. Gagal → alasan tampil di kolom "Pemeriksaan terakhir". Verifikasi menerima CNAME tepat ke target, atau IP yang sama dengan host target.
5. Buka `https://arus.kotakcantik.com`. Akses pertama 5–10 detik (sertifikat baru diterbitkan).
6. Uji: halaman login menampilkan "Portal <nama tenant>"; masuk, lalu reload halaman, harus tetap login.

Mematikan domain: tombol **Matikan** (sertifikat baru tidak diterbitkan, branding mati; domain kembali ke "Menunggu DNS" bila dihidupkan lagi dan wajib diverifikasi ulang). **Hapus** menghilangkan domain sepenuhnya.

## Uji pertama (wajib sebelum dipakai klien)

Caddyfile membedakan halaman dari panggilan API lewat header `Accept: text/html` karena banyak path web sama dengan path API (`/items`, `/sales`, `/platform/...`). Pastikan:

- Membuka `https://<domain>/items` langsung di browser menampilkan halaman web, bukan JSON.
- Setelah login, data muncul (panggilan API sampai ke API, bukan ke web).
- Unduhan APK di halaman login jalan (`/public/*` selalu ke API).
- Di log audit, IP yang tercatat adalah IP pengguna asli, bukan IP server edge. Rantai `X-Forwarded-For` melewati Caddy → Cloudflare/Apache → API.

## Pemecahan masalah

| Gejala | Kemungkinan penyebab |
|---|---|
| Verifikasi gagal terus | CNAME belum menyebar; klien memakai proxy Cloudflare (awan oranye) |
| Error TLS / sertifikat ditolak | Domain belum Aktif sehingga Caddy menolak menerbitkan; lihat `journalctl -u caddy` |
| Login gagal / langsung logout | `VITE_PRIMARY_HOSTS` belum di-build, web masih memanggil API lintas origin |
| Halaman kosong / 404 di rute web | Header `Accept` tidak sampai ke Caddy, atau upstream web tidak mengembalikan `index.html` untuk path tak dikenal |
| Semua pengguna terlihat satu IP (rate limit salah sasaran) | `TRUST_PROXY=true` belum aktif, atau rantai `X-Forwarded-For` terputus |

## Batasan

- `GET /public/domain-check` dibatasi 6000 permintaan/jam per IP; `GET /public/branding` 600/jam. Tanpa cache; bila trafik penerbitan sertifikat tinggi, tambahkan cache Redis.
- Branding saat ini hanya nama tenant (tabel `tenants` belum punya logo/warna).
- Domain apex (mis. `kotakcantik.com` tanpa subdomain) tidak bisa memakai CNAME di sebagian penyedia DNS; gunakan subdomain, atau ALIAS/ANAME ke IP yang sama dengan edge (verifikasi menerima IP yang sama).

## F. Landing page di root domain

Secara bawaan `https://arus.kotakcantik.com/` membuka aplikasi. Admin platform bisa menampilkan landing page tenant (halaman `/s/<slug>` dari menu Landing Page tenant) di root domain itu:

1. Domain harus berstatus **Aktif**, dan tenant sudah menghidupkan landing page-nya.
2. **Platform → Domain** → saklar **Landing page di domain utama** pada kartu domain.
3. `GET /` di domain itu dijawab `/public/domain-root?host=...` oleh API; aplikasi tetap bisa dibuka di `/login`. Bila landing belum dihidupkan tenant, API menjawab 404 dan Caddy kembali ke SPA.

Perlu migration `00103` dan Caddyfile terbaru (blok `@root`).

## G. Login Google di domain kustom

`GOOGLE_CLIENT_IDS` (env) hanya satu set ID untuk host utama. Tiap domain kustom bisa memakai OAuth client ID Google sendiri, disimpan di database lewat **Platform → Domain** (kolom "Login dengan Google" pada kartu domain). Tanpa perubahan env dan tanpa restart.

1. Tenant (atau admin) membuat OAuth client **Web** di Google Cloud Console. Pada **Authorized JavaScript origins** isi `https://arus.kotakcantik.com`.
2. Tempel client ID-nya (`...apps.googleusercontent.com`) di kartu domain, lalu Simpan. Kosongkan lalu simpan untuk mematikan.
3. Server menerima ID token dengan `aud` = ID itu paling lambat satu menit setelah disimpan (domain harus berstatus Aktif).

Perlu migration `00104`.

## H. White label penuh

Di **Platform → Domain**, kartu domain punya bagian **Tampilan white label**: nama aplikasi, teks footer, logo penuh, dan ikon/favicon (gambar maks 512 KB, disimpan di database). Di domain itu, kata "ARUS" pada teks aplikasi, judul tab, logo, dan footer diganti sesuai pengaturan. Bila dikosongkan: tampilan ARUS bawaan (logo dan nama).

Landing ARUS (`/landing`) tidak dipakai di domain kustom: `/landing` diarahkan ke `/`, dan `/` menampilkan landing tenant (bila saklarnya nyala) atau halaman login. Perlu migration `00105`, web terbaru, dan Caddyfile terbaru (blok `@arus_landing`).
