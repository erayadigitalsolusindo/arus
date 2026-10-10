# Setup PC Kasir (cetak struk)

Ada dua cara cetak struk (FR-POS-15). Pengaturannya per PC di layar sukses setelah bayar →
*Pengaturan struk (perangkat ini)* → **Cara cetak**:

| Cara | Hasil | Kebutuhan |
|---|---|---|
| **Langsung ke printer (ESC/POS)** — disarankan | Huruf bawaan printer (paling tajam), cepat, kertas terpotong otomatis | `print-agent` terpasang di PC kasir |
| Lewat browser — cadangan | Struk dirender sebagai gambar oleh driver | Chrome dengan `--kiosk-printing` |
| Otomatis (bawaan) | Pakai print-agent bila berjalan; bila tidak ada → browser | — |

Template struk ada di aplikasi (`web/src/lib/pos/receipt.ts` + `escpos.ts`). Agent hanya meneruskan byte ke printer,
jadi perubahan tampilan struk cukup dengan update aplikasi web — agent di PC kasir tidak perlu dipasang ulang.

## 1. Printer

1. Pasang driver printer thermal dari pabrikan (atau driver **Generic / Text Only** bawaan Windows), uji "Print Test Page".
2. Jadikan printer itu **default** (Settings → Bluetooth & devices → Printers & scanners → *Set as default*;
   matikan "Let Windows manage my default printer"). Atau tulis nama printernya di konfigurasi agent (lihat di bawah).

## 2. Pasang print-agent (cara ESC/POS)

1. Ambil `arus-print-agent-windows.zip` (hasil `source/print-agent/build.sh`), ekstrak.
2. Klik dua kali **`pasang.cmd`**. Program disalin ke `%LOCALAPPDATA%\ArusPrintAgent`, dipasang di folder Startup
   (berjalan otomatis setiap login Windows), lalu langsung dijalankan. Tidak ada jendela yang terbuka — itu normal.
   Bila Windows SmartScreen memperingatkan program tak dikenal: *More info* → *Run anyway*.
3. Klik dua kali **`uji-cetak.cmd`** → printer harus mencetak halaman uji "ARUS". Bila tidak keluar, buka
   `%LOCALAPPDATA%\ArusPrintAgent\print-agent.log`.
4. Di aplikasi: bayar satu transaksi → *Pengaturan struk* harus menampilkan
   "Print-agent … aktif · printer: <nama printer>". Tekan **Tes printer**.
5. Chrome versi baru dapat menanyakan izin "akses perangkat di jaringan lokal" saat pertama kali — pilih **Izinkan**.

Konfigurasi `%LOCALAPPDATA%\ArusPrintAgent\print-agent.json` (dibuat otomatis saat pertama jalan):

```json
{
  "listen": "127.0.0.1:9100",
  "origins": ["https://arus.erayadigital.co.id"],
  "printer": ""
}
```

- `origins` = alamat aplikasi ARUS yang boleh mencetak. Situs lain yang dibuka di PC kasir **ditolak**.
- `printer` = nama printer Windows persis seperti di daftar printer; kosong = printer default.
- Setelah mengubah konfigurasi: jalankan `pasang.cmd` lagi (menghentikan dan menjalankan ulang agent).
- Hapus agent: `hapus.cmd`.

## 3. Cara browser (cadangan)

Bila agent tidak dipakai, buat shortcut Chrome khusus kasir agar tidak muncul dialog cetak:

```
"C:\Program Files\Google\Chrome\Application\chrome.exe" --kiosk-printing --user-data-dir=C:\ArusKasir https://arus.erayadigital.co.id/kasir
```

Tutup semua jendela Chrome lain sebelum membuka shortcut ini. Ukuran kertas driver: roll 58 mm, margin 0.

## 4. Pengaturan lain di aplikasi

- Data kepala/kaki struk: **Outlet → Ubah → bagian Struk** (alamat, telepon, teks kepala, teks kaki).
- Cetak otomatis on/off, lebar kertas 58/80 mm, potong kertas otomatis — per PC.
- Cetak ulang: tombol printer di **Penjualan Hari Ini** atau *Cetak ulang* di layar sukses; tercatat di audit dan
  struknya bertanda `*** SALINAN KE-n ***`.

## Masalah umum

| Gejala | Penyebab / solusi |
|---|---|
| "Print-agent tidak ditemukan" | Agent belum berjalan: jalankan `pasang.cmd` lagi; cek log. Port 9100 dipakai program lain → ubah `listen` (dan alamat agent di aplikasi). |
| "Printer gagal mencetak: OpenPrinter …" | Nama `printer` salah atau tidak ada printer default. |
| Agent aktif tapi tetap tidak tercetak | Printer bukan ESC/POS / driver menahan antrean: cek antrean printer Windows; coba driver "Generic / Text Only". |
| Huruf aneh/acak | Printer tidak memakai ESC/POS standar — pakai cara browser, laporkan merek/model printer. |
| Kertas tidak terpotong | Printer tanpa pisau: matikan "Potong kertas otomatis" (sobek manual). |
