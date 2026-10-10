# Setup PC Kasir (cetak struk)

Struk dicetak dari browser (FR-POS-15). Supaya kasir tidak perlu menekan tombol di dialog cetak setiap transaksi,
Chrome dijalankan dengan `--kiosk-printing`: struk langsung dikirim ke **printer default Windows**.

## 1. Printer

1. Pasang driver printer thermal (58 mm USB) dari pabrikan. Uji cetak "Test Page" dari Windows.
2. Jadikan printer itu **default**: Settings → Bluetooth & devices → Printers & scanners → pilih printer → *Set as default*.
   Matikan "Let Windows manage my default printer".
3. Printing preferences printer: ukuran kertas **58 mm** (atau 80 mm), panjang *continuous/roll*, margin 0.

## 2. Shortcut Chrome khusus kasir

Buat shortcut baru di desktop dengan target (sesuaikan alamat aplikasi):

```
"C:\Program Files\Google\Chrome\Application\chrome.exe" --kiosk-printing --user-data-dir=C:\ArusKasir https://arus.erayadigital.co.id/kasir
```

- `--kiosk-printing` = cetak langsung tanpa dialog.
- `--user-data-dir` = profil Chrome terpisah khusus kasir (login ARUS tersimpan di sini, tidak tercampur profil pribadi).
- Tutup **semua** jendela Chrome dulu sebelum membuka shortcut ini; bila Chrome lain masih jalan, opsi tidak berlaku.

## 3. Pengaturan di aplikasi

- Data kepala/kaki struk: menu **Outlet → Ubah → bagian Struk** (alamat, telepon, teks kepala, teks kaki).
- Di layar sukses setelah bayar → *Pengaturan struk (perangkat ini)*: cetak otomatis on/off dan lebar kertas 58/80 mm.
  Pengaturan ini tersimpan per PC (browser), bukan per akun.
- Cetak ulang: tombol printer di **Penjualan Hari Ini** atau tombol *Cetak ulang* di layar sukses. Setiap cetak ulang
  tercatat di audit dan struknya bertanda `*** SALINAN KE-n ***`.

## Masalah umum

| Gejala | Penyebab / solusi |
|---|---|
| Dialog cetak tetap muncul | Chrome tidak dibuka lewat shortcut di atas, atau masih ada jendela Chrome lain. Tutup semua Chrome. |
| Struk tercetak di printer lain / PDF | Printer default salah (langkah 1.2). |
| Tulisan terpotong di kanan | Lebar kertas di aplikasi tidak sama dengan printer (58 vs 80), atau margin driver bukan 0. |
| Kertas keluar panjang kosong | Ukuran kertas driver masih A4/Letter; ubah ke roll 58 mm. |
