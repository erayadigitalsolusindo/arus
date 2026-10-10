# Status Sesi

> **Perbarui di akhir setiap sesi** (ringkas, tanggal absolut, buang info usang; target ≤ ±80 baris). Roadmap/centang: `docs/ROADMAP.md`. Lingkungan dev: `docs/DEV-ENV.md`. Riwayat: `docs/SESSION-LOG.md` (Grep, jangan dibaca penuh).

## Posisi (2026-10-11)
- Fondasi, auth/RLS/role/audit, master katalog, item, stok (saldo awal, opname, mutasi, pecah satuan, kartu stok), member + poin + deposit, kasir web lengkap (kredit/piutang, retur, kupon, shift, struk), pembelian (HPP per cabang, hutang, retur beli), dasbor "toko baik-baik saja?" (verdict + pemeriksaan, stok menipis, perbandingan cabang, bulan lalu) sudah ada. Detail per kotak: `docs/ROADMAP.md`. Sisa Fase 6: saldo awal hutang, pembatalan pembayaran, PO.
- Kesiapan go-live pilot ±60–65%; penghalangnya kini di luar layar kasir. Kasir web: PRD FR-POS 20/22 ✅ (sisa catatan per baris + offline).
- Git (2026-10-11): `main` bersih; PR #29 (dasbor + `items.min_stock`, migration `00055`), #30 (kasir klasik), #31 (matriks izin + halaman 404/500 bertema bawah laut) sudah ter-merge. Migration terakhir = `00055`. Tidak ada `.github/` (CI belum ada).
- **Penjualan Langsung (SSE)** sudah di `main` dan digabung ke branch ini (2026-10-10): menu Utama → Penjualan Langsung (`/live-sales`, izin `sales_list.view`; paket `internal/live`, komponen `LiveSalesToday`, teks `dashboard.live.*`). `/dashboard` = dasbor lengkap buatan pengguna; jangan menimpa halaman yang sudah ada, fitur baru = menu/halaman baru.
- **Kasir klasik `/kasirb` (2026-10-10):** layar kasir bergaya aplikasi desktop (isian nota di atas, tabel keranjang, tombol F di bawah, dialog saldo awal). Logikanya sama dengan `/kasir`: halaman dipindah ke komponen `lib/components/PosScreen.svelte` (prop `variant`), kedua rute hanya membungkusnya; ubah logika kasir = sekali di `PosScreen`. Pilihan tampilan disimpan di localStorage (`pos.layout`, `lib/pos/layout.ts`); `/kasir` dan `/kasirb` mengarahkan ke pilihan itu saat login berikutnya. Belum ada di klasik: T.O.P/J. Tempo (dipilih di dialog bayar), INV Website, Pisah Tumpukan, Manual (Rp).

## Toko pilot: TOKO KOTAK CANTIK MAGELANG (keputusan 2026-10-10)
- Klien legacy yang sama dengan dump `kotakcantik.sql` (±430 rb nota, ±10 jt baris kartu stok di legacy). Prioritas = apa yang menghalangi toko ini berjualan.
- Go-live **akhir bulan** (usulan: opname malam 31 Okt 2026, mulai 1 Nov 2026); 2 cabang, pilot di **pusat** dulu; printer thermal **58 mm USB** (Bluetooth/Android → Fase 10); laporan: penjualan, mutasi stok, dll.; **>100 rb jenis barang**; kontak: Bobby Kurniawan.
- Dasbor: `GET /dashboard/overview?scope=all`; stok dicache 60 dtk di memori proses (pemindaian 150–300 rb saldo ±0,6–1 dtk). Ukur 330 rb nota + 150 rb barang × 2 cabang: hangat 75–190 ms, dingin 1,2–1,7 dtk. Belum ada: jatuh tempo 7 hari ke depan, batas minimum per cabang, peringatan backup.
- Pencarian kasir sudah cepat (`GET /items/search`). Daftar admin `/items` masih `count(*) OVER()` + OFFSET (±0,9 dtk untuk 150 rb barang) — perlu keyset.
- Struk: template di web (`lib/pos/receipt.ts`, `escpos.ts`), cetak `auto|agent|browser`; `source/print-agent` = kurir RAW lokal `127.0.0.1:9100`. Panduan: `source/deploy/PC-KASIR.md`.
- **ARUS Mobile (Flutter, `source/mobile/`, Fase 10):** proyek + login jadi (analyze bersih, 5 test lulus, APK debug terbangun); backend punya jalur auth native (`X-Client: mobile`, refresh token di badan JSON). Cakupan v1 = semua fitur kasir web kecuali retur; printer Bluetooth (jenis/model belum diketahui). **Belum:** dijalankan di perangkat (belum ada HP/emulator), pilih/pindah outlet, layar kasir, cetak Bluetooth, lupa sandi, ikon app. **2026-10-10 lanjutan:** tema terang/gelap, latar lautan di login, layar kasir (cari, keranjang + quote server, bayar tunai/split + buka shift; panel outlet tertutup bawaan) selesai; 8 test lulus, APK debug terbangun. Berikutnya mobile: member, kupon, kredit, nota pending, scan kamera, struk Bluetooth, tutup shift.

## Langkah berikutnya (urutan go-live, sama dengan PRD §6.3)
1. **Import barang + saldo awal stok dari Excel/CSV (3.4, FR-ONB-01/03)** — penghalang utama. Sumber = ekspor master barang legacy toko pilot; cek kebersihan data (barcode, satuan, harga per cabang). Setelah itu ukur kasir dengan katalog asli.
2. **Backup terjadwal ke luar server + sekali uji restore** (belum ada).
3. **Spec test penjualan (1.3)**: 10–15 kasus tersering → `source/tests/spec/`, disetujui pengguna.
4. **CI GitHub Actions** (`go test`, `npm run check`); `.github/` belum ada.
5. **Gladi bersih** ±25 Okt di PC kasir: shift, struk/tutup shift/rekap di printer asli, retur, piutang.
6. Boleh menyusul: pembatalan pembayaran piutang/hutang (FR-AR-07, FR-PUR-09), saldo awal hutang (FR-ONB-05), catatan per baris kasir, edit/batal tutup shift, laporan Fase 7.

## Sisa kecil / belum diputuskan
- Trial/paket (register langsung aktif); email verifikasi tidak memblokir login (putuskan fitur yang butuh email terverifikasi); ganti password saat sudah masuk (endpoint belum; audit `auth.password_change` siap).
- Syarat Layanan/Privasi (`legal.ts`) masih draf, perlu tinjauan hukum sebelum rilis publik. Email produksi butuh SMTP; email via `background.Runner` tanpa retry (pindah ke antrean bila perlu jaminan).
- Belum ada: daftar fitur IN/OUT scope, `docs/business-rules.md`, spec test, import Excel/CSV, CI, backup di luar server.
