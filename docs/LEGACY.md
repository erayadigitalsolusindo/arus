# Peta & Pelajaran Legacy

> Dipindah dari AGENTS.md §7–§10. Legacy = referensi aturan bisnis, read-only. Baca hanya bagian yang relevan dengan slice yang dikerjakan.

## 7. Peta Legacy (buka hanya yang relevan)

Tujuan membuka legacy: **memahami aturan bisnis**, bukan menyalin skema atau data.

Root legacy: `../aciraba_siak_os/`. Stack: CI4 (`aciraba_website`) → curl → Express/mysql2 (`aciraba_server`, port 1111; `apiaciraba_public.js` port 1112) → MySQL 8 (`kotakcantik.sql`, dump Navicat).

| Butuh perilaku apa | Buka |
|---|---|
| Simpan/edit penjualan, potong stok, kartu stok | `aciraba_server/model/PenjualanData.js` → `simpantransaksi` (≈ baris 579–900) |
| Input yang dikirim kasir saat checkout | `aciraba_website/app/Controllers/Penjualan.php` → `simpantransaksi` (≈ baris 1181) |
| Keranjang kasir (ditulis langsung PHP→MySQL) | `aciraba_website/app/Models/KasirModel.php`; juga `PenjualanModel.php`, `PembelianModel.php`, `PenyesuaianModel.php` |
| Pembelian, retur beli, hutang | `aciraba_server/model/PembelianData.js` |
| Opname/penyesuaian | `aciraba_server/model/PenyesuaianData.js` |
| Login, registrasi, OTP | `aciraba_server/model/Auth.js`, `aciraba_website/app/Controllers/Auth.php` |
| Hak akses UI | `aciraba_website/app/Filters/CheckPermission.php`, `app/Helpers/HakAksesHelper.php`, `app/Config/Routes.php` |
| Query generik `KONDISI`/`DIMANA1..N` | `aciraba_server/model/SPMysql.js` + body `proc_Controller_{Website,Admin,Kasir,API,Report}` di `kotakcantik.sql` (cari `CREATE DEFINER`). 178 cabang `kondisi = N`. |
| Laporan | `aciraba_server/model/LaporanData.js` + `proc_Controller_Report` |
| Akuntansi SIAK | `aciraba_server/model/SiakData.js`, tabel `01_siak_*` |
| Resto/KDS + realtime | `aciraba_server/model/RestoData.js`, event Socket.IO `NOTIFDINEINTAKEAWAY{LOKASI}{KODEUNIKMEMBER}` |
| Acipay / payment gateway | `aciraba_server/model/Acipay.js`, `Digiflazz.js`, `config/api/{digiflazz,duitku,midtrans,tripay,wasender}.js` |

**Skema legacy (fakta terverifikasi dari dump 2026-10-06):** 78 tabel InnoDB, 193 indeks, 0 foreign key, 75/78 tabel tanpa UNIQUE selain PK, 5 SP, 8 function, 22 trigger. Prefix: `01_tms_*` master/saldo, `01_trs_*` transaksi, `01_siak_*` akuntansi, `01_acipay_*` produk digital. Tenant = `KODEUNIKMEMBER`, outlet = `LOKASI`/`OUTLET`/`KODEAKUN` (nama kolom tidak konsisten). Tabel inti volume: `01_trs_kartustok` (~9,9 jt baris), `01_trs_kartustok_new` (~8 jt, sisa), `01_trs_barangkeluar_detail` (~3,3 jt), `01_trs_barangkeluar` (~430 rb). Master barang = `01_tms_barangkharisma`. Stok = `01_tms_stok` (kolom `DISPLAY`,`GUDANG`,`RETUR`; **tanpa UNIQUE** tenant+outlet+barang). Tabel keranjang `01_tms_keranjang`, `_pending`, `_barangmasuk` **tidak ada di dump** (dipakai PHP).

## 8. Aturan Bisnis Legacy yang Tersembunyi di Trigger (harus diimplementasi ulang di service Go)

| Trigger legacy | Perilaku | Status di NewGen |
|---|---|---|
| `AFTERINSERTTRX` on `01_trs_barangkeluar` | Jika `ENUM_JENISTRANSAKSI='KREDIT'` → buat piutang, total = `-KEMBALIAN`, jatuh tempo = now + `JATUHTEMPO` hari. Jika `TIPETRANSAKSI=1` → catat DP. Tambah poin member `ROUND(TOTALBELANJA / MINIMALPOIN)`. | Implementasi ulang. **Perbaiki:** DP kolom kredit legacy memakai `NOMORKARTUKREDIT` (bug, harusnya nominal); bagi-nol bila `MINIMALPOIN=0`. |
| `AFTEREDIT` on `01_trs_barangkeluar` | Hapus & buat ulang piutang; **tambah poin lagi tanpa mengurangi poin lama**. | **Bug legacy:** poin menggelembung setiap edit; piutang yang sudah dibayar sebagian kembali penuh. Desain baru: hitung selisih; buat spec test khusus edit nota. |
| `AFTERDELETE` on `01_trs_barangkeluar` | Hapus beban, DP, pesanan meja; kurangi poin. | Implementasi ulang (lebih baik: void/soft-cancel + movement balik, bukan delete). |
| `AFTERINESRT` / `AFTERUBAHPIUTANG` / `AFTERDELETEPIUTANG` on `01_tms_piutangkredit_detail` | `SISAKREDIT` ± `BAYAR`. (`AFTERUBAHPIUTANG` tanpa filter tenant.) | Saldo dihitung dari pembayaran, bukan kolom yang dimutasi. |
| `AFIHUTANG` / `AUPBHUTANG` / `ADELBHUTANG` on `01_tms_hutangtoko_detail` | Sama untuk hutang supplier. | Idem. |
| `AIMUTASIBARANGD` on `01_trs_mutasibarang_detail` | Kurangi bucket asal (D/G/R) di outlet asal, tambah bucket tujuan di outlet tujuan (insert jika belum ada), tulis kartu stok. | Implementasi di modul stock (transfer). |
| `AFTER_INSERT` on `01_tmp_pecahsatuan` | Pecah satuan: kurangi `DISPLAY` barang asal, tambah `DISPLAY` barang baru, tulis kartu stok (`PS…`). | Implementasi di modul stock (unit conversion). |
| `UpdateLogBarang` on `01_tms_barangkharisma` | Audit per kolom ke `01_log_barangkharisma` jika `TERAKHIR_UBAH` terisi. Kolom `STOKDAPATMINUS`/`PEMILIK` menulis ke tabel `BARANG_LOG` yang **tidak ada di dump**. | Ganti dengan `audit_log` generik. |
| `*_bestbuybaranggrosir*_AFTER_INSERT/DELETE` | Set `APAKAHGROSIR` AKTIF/TIDAK AKTIF di master barang. | Turunan: cukup cek ada/tidaknya tier grosir. |
| `01_tms_member_AFTER_INSERT/DELETE` | Buat/hapus baris deposit member (tanpa filter tenant). | Deposit = tabel transaksi sendiri. |
| `01_tms_penggunaaplikasiha_AFTER_DELETE` | User dengan role terhapus → role `DEFAULT`. | FK `ON DELETE` + aturan service. |
| `AFDELETE` on `01_tms_resto_daftarmeja` | Hapus pesanan meja terkait. | FK cascade / cegah hapus meja aktif. |

## 9. Kelemahan Legacy yang **Tidak Boleh** Disalin

(Hasil inspeksi kode, belum direproduksi sebagai exploit.)
- API Node tanpa auth: `tokenAPIMiddleware` hanya dirujuk di `routes/authrouter.js` dan dikomentari; CORS `*`; rate limit 100 rb/10 menit.
- Tenant/kasir/lokasi diambil dari POST browser (`getPost('KODEUNIKMEMBER')`), tidak konsisten dengan session.
- `BEGIN/COMMIT` via `pool.query()` → transaksi lintas koneksi (tidak atomik). Ada di Penjualan, Pembelian, Auth, MasterData, Penyesuaian.
- Saldo kartu stok dihitung di memori dari SELECT sebelumnya → race antar kasir.
- SQL injection: `KasirModel.php` (konkatenasi string), `handleUpdateBarangKeluar` (CASE WHEN dengan nilai disisipkan).
- Variabel global implisit di Node (`cekHash`, `passplain`, `pesanbalik` level modul) → bocor antar-request; `bacaPINTrx` mengembalikan hash PIN.
- Harga jual/beli dikirim klien dan disimpan apa adanya.
- `UPDATE 01_trs_barangkeluar ... WHERE PK_NOTAPENJUALAN = ?` tanpa filter tenant.
- Uang `DOUBLE`; tanggal sebagai `BIGINT` di beberapa tabel; `LIKE '%x%'` 512× di SP; fungsi skalar stok per baris.
- UNIQUE salah cakupan: `01_tms_diskon(BARANG_ID)`, `01_tms_voucherbarang(NAMAVOUCHER)` tanpa tenant.
- Kunci AES & salt hardcode di `aciraba_server/config/utils.js`; OTP period 1 detik dengan secret yang bisa ditebak.
- CSRF CI4 nonaktif.

## 10. Pertanyaan Terbuka (tanyakan ke pengguna bila relevan, jangan diasumsikan)

- Apakah kasir harus bisa **offline**? (Mempengaruhi desain SvelteKit: IndexedDB queue + idempotency.)
- Modul mana yang masih benar-benar dipakai klien? (Acipay, payment gateway, SIAK, resto — bisa ditunda/dibuang.)
- Apakah ada **source** template Dreams Core (folder `src/`, file Tailwind config/partials)? Yang ada sekarang hasil build saja; source akan mempercepat porting.
- Hosting target: VPS tunggal vs on-premise per toko (`01_set_onpremise` ada di legacy)?
- Format nomor nota default NewGen (usulan `{OUTLET}-{YYMMDD}-{NNNN}`; tidak wajib sama dengan legacy) — PRD Q4.
- Urutan perhitungan nota (PRD §7.4) & aturan pembulatan (fungsi legacy `PEMBULATANANGKA` belum dibaca) — PRD Q6.
- Berapa lama legacy tetap bisa diakses baca-saja untuk histori toko yang sudah pindah — PRD Q6.

