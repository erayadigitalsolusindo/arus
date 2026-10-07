# Product Requirements Document (PRD)
# ACIRABA NewGen — Sistem POS & ERP Ritel Multi-Tenant

| Atribut | Nilai |
|---|---|
| Versi dokumen | 0.2 (Draf) |
| Status | Draf — menunggu review pemilik produk |
| Tanggal | 7 Oktober 2026 |
| Pemilik produk | Pemilik ACIRABA (Erayadigital) |
| Penyusun | Tim pengembang (dibantu AI) |
| Dokumen terkait | `AGENTS.md` (keputusan teknis, status, roadmap), `reference/template/` (acuan UI) |

### Riwayat Revisi

| Versi | Tanggal | Perubahan | Oleh |
|---|---|---|---|
| 0.2 | 2026-10-07 | Pendekatan **data baru**: tidak ada migrasi data legacy. Golden test diganti *spec test* (contoh kasus yang dihitung manual); ETL/cutover diganti onboarding (import & saldo awal); ditambah FR-ONB; Q4/Q6 diganti | Tim |
| 0.1 | 2026-10-06 | Draf awal berdasarkan analisis sistem legacy ACIRABA SIAK OS | Tim |

### Konvensi Penandaan

- **[KONFIRMASI]**: asumsi yang belum diverifikasi ke pemilik produk/klien. Wajib dijawab sebelum fase terkait dimulai.
- **Prioritas (MoSCoW):** **M** = Must (wajib di rilis), **S** = Should, **C** = Could, **W** = Won't (tidak di rilis ini).
- ID kebutuhan: `FR-<MODUL>-<NN>` (fungsional), `NFR-<KATEGORI>-<NN>` (non-fungsional).

---

## 1. Ringkasan Eksekutif

ACIRABA adalah aplikasi Point of Sale dan ERP ringan berbasis web untuk usaha ritel dan restoran multi-outlet. Sistem saat ini (*legacy*: CodeIgniter 4 + Node.js/Express + MySQL) sudah dipakai klien, tetapi memiliki masalah mendasar:

- Keamanan: API tanpa autentikasi dan identitas tenant diambil dari browser.
- Integritas data: transaksi database tidak atomik, stok rawan selisih, nilai uang disimpan sebagai floating point.
- Pemeliharaan: logika bisnis tersebar di PHP, Node.js, 5 stored procedure generik (178 cabang), dan 22 trigger.

**ACIRABA NewGen** membangun ulang sistem dengan arsitektur modern: **Go** untuk backend, **PostgreSQL** untuk database, **Redis** untuk cache dan realtime, serta **SvelteKit** untuk frontend. Sasarannya:

1. Transaksi kasir yang **cepat, benar, dan tidak pernah ganda**.
2. Stok dan saldo yang **selalu dapat ditelusuri** (berbasis ledger).
3. **Isolasi data antar-tenant** yang dijamin di level server dan database.
4. Kode yang **mudah dikembangkan dan dites**.

NewGen adalah **aplikasi baru dengan data baru**. Data transaksi legacy **tidak dimigrasikan**. Toko memulai dengan *onboarding*: import master barang, input saldo awal stok, dan input saldo awal piutang/hutang. Sistem legacy hanya dipakai sebagai referensi aturan bisnis. Kebenaran perhitungan dijamin dengan *spec test*, yaitu contoh kasus yang dihitung manual dan disetujui pemilik produk.

---

## 2. Latar Belakang & Pernyataan Masalah

### 2.1 Kondisi Saat Ini

| Aspek | Kondisi legacy | Dampak bisnis |
|---|---|---|
| Keamanan API | Endpoint Node.js tidak menerapkan autentikasi; CORS `*` | Pihak luar yang menjangkau server bisa membaca atau mengubah data semua toko |
| Identitas tenant | Kode tenant, kasir, dan outlet dikirim dari browser | Satu tenant berpotensi memalsukan data tenant lain |
| Transaksi | `BEGIN/COMMIT` dijalankan lewat pool sehingga bisa memakai koneksi berbeda | Nota bisa tersimpan sebagian; stok tidak sesuai penjualan |
| Stok | Saldo kartu stok dihitung di memori; tabel stok tanpa kunci unik | Selisih stok saat beberapa kasir bertransaksi bersamaan |
| Nilai uang | Tipe `DOUBLE` (86 kolom) | Selisih pembulatan di laporan dan neraca |
| Harga | Harga jual dikirim dari browser dan disimpan apa adanya | Risiko manipulasi harga di kasir |
| Aturan bisnis | Tersembunyi di trigger (poin, piutang, hutang, mutasi) | Sulit diubah; ada bug, misalnya poin member bertambah ganda setiap nota diedit |
| Performa query | 512 pencarian `LIKE '%…%'`, fungsi stok dipanggil per baris | Pencarian dan laporan lambat seiring data tumbuh (±10 juta baris kartu stok) |
| Akses database | PHP dan Node.js sama-sama menulis ke database | Aturan tidak konsisten; ada celah SQL injection |
| Frontend | jQuery, rendering di server, 469 blok pemanggilan API yang diduplikasi | Pengembangan fitur lambat; UX kasir sulit ditingkatkan |
| Ketersediaan | Wajib online (minimal 1 Mbps) | Kasir berhenti saat internet putus |

### 2.2 Pernyataan Masalah

> Pemilik toko membutuhkan sistem kasir dan pengelolaan toko yang **dapat dipercaya angkanya** (penjualan, stok, piutang, hutang) dan **aman datanya**. Sistem saat ini tidak dapat menjamin keduanya, dan biaya untuk menambal arsitektur lama lebih tinggi daripada membangun ulang dengan fondasi yang benar.

---

## 3. Tujuan & Non-Tujuan

### 3.1 Tujuan Produk

| ID | Tujuan |
|---|---|
| G1 | Kasir dapat menyelesaikan transaksi dengan cepat menggunakan keyboard/scanner, tanpa nota ganda. |
| G2 | Stok per outlet selalu konsisten dan setiap perubahan dapat ditelusuri ke dokumen sumbernya. |
| G3 | Data setiap tenant terisolasi penuh; tidak ada kebocoran lintas tenant. |
| G4 | Angka penjualan, piutang, hutang, dan laporan sesuai aturan bisnis yang terdokumentasi (diverifikasi spec test). |
| G5 | Toko (baru maupun pindahan dari legacy) dapat mulai beroperasi dalam ≤ 1 hari melalui onboarding: import barang, saldo awal stok, dan saldo awal piutang/hutang. |
| G6 | Fondasi kode yang modular sehingga fitur baru dapat ditambahkan tanpa merusak fitur lain. |

### 3.2 Non-Tujuan (di luar cakupan rilis awal)

- Aplikasi mobile native (Android/iOS). Web responsif sudah cukup.
- Marketplace atau integrasi e-commerce.
- Akuntansi lengkap setara software akuntansi khusus (pajak e-Faktur, konsolidasi multi-entitas).
- **Migrasi data transaksi/histori dari legacy.** Histori lama tetap dapat dilihat di sistem legacy (mode baca-saja) atau dari ekspor laporan.
- Kompatibilitas dengan legacy: akun/password, format nomor nota, dan struktur data tidak harus sama.
- Mempertahankan bug legacy (lihat §15.3).

---

## 4. Metrik Keberhasilan

| ID | Metrik | Target | Cara ukur |
|---|---|---|---|
| M1 | Latensi simpan transaksi kasir (p95, server) | < 300 ms untuk ≤ 50 item | Log/metrics API |
| M2 | Pencarian barang di kasir (p95) | < 150 ms untuk 50.000 SKU | Metrics API + cache |
| M3 | Nota ganda akibat klik ganda/jaringan | 0 | Constraint unik + idempotency key, audit mingguan |
| M4 | Kelulusan spec test (contoh kasus perhitungan yang disetujui pemilik produk) | 100% | Test otomatis di CI |
| M5 | Selisih stok sistem vs. ledger | 0 | Job rekonsiliasi harian `Σ movements = balance` |
| M6 | Insiden akses lintas tenant | 0 | Test otomatis RLS + pentest sebelum go-live |
| M7 | Waktu onboarding toko pilot (import barang + saldo awal hingga transaksi pertama) | ≤ 1 hari kerja | Observasi di toko pilot |
| M8 | Ketersediaan layanan (jam operasional) | ≥ 99,5% per bulan | Uptime monitor |
| M9 | Waktu pelatihan kasir baru | ≤ 30 menit hingga mandiri | Observasi di tenant pilot **[KONFIRMASI]** |

---

## 5. Pengguna & Persona

| Persona | Deskripsi | Kebutuhan utama |
|---|---|---|
| **Owner / Admin Tenant** | Pemilik usaha dengan 1..n outlet | Laporan omzet & laba, kontrol harga, hak akses pegawai, stok semua outlet |
| **Manajer Outlet** | Penanggung jawab satu outlet | Stok outlet, opname, approval edit/void nota, laporan harian |
| **Kasir** | Operator transaksi | Input cepat (scanner/keyboard), pembayaran campuran, cetak struk, hold transaksi |
| **Staf Gudang / Pembelian** | Penerimaan & mutasi barang | Pembelian, retur ke supplier, mutasi antar outlet/lokasi, opname |
| **Akuntan / Keuangan** | Pencatatan keuangan | Piutang, hutang, kas/bank, jurnal, neraca, laba rugi |
| **Dapur (Resto)** | Staf dapur restoran | Antrean pesanan realtime (KDS), ubah status pesanan |
| **Super Admin Platform** | Tim Erayadigital | Kelola tenant, lisensi, paket, monitoring |
| **Pelanggan/Member** | Pembeli (tidak login di rilis awal) | Poin, deposit, piutang tercatat dengan benar |

---

## 6. Ruang Lingkup & Rencana Rilis

| Rilis | Nama | Isi | Kriteria selesai |
|---|---|---|---|
| **R0** | Fondasi | Infrastruktur, auth, tenant, layout UI | Login berjalan; isolasi tenant lolos test |
| **R1 (MVP)** | Toko Bisa Jualan | Master data, **onboarding (import barang, saldo awal stok/piutang)**, stok (ledger), kasir, retur jual, piutang, cetak struk, laporan penjualan & stok dasar | Lulus spec test penjualan; toko pilot beroperasi harian di NewGen |
| **R2** | Operasional Lengkap | Pembelian, retur beli, hutang (+ saldo awal hutang), opname, mutasi, pecah satuan, laporan lengkap | Toko pilot memakai NewGen untuk seluruh operasional harian |
| **R3** | Restoran | Meja, pesanan, dine-in/takeaway, KDS realtime | **[KONFIRMASI]** jumlah klien resto aktif |
| **R4** | Keuangan & Ekstensi | Akuntansi SIAK, Acipay/PPOB, payment gateway, notifikasi WhatsApp | **[KONFIRMASI]** modul mana yang masih dipakai |
| **R5** | Peluncuran Luas | Onboarding toko lain secara bertahap; legacy toko yang sudah pindah menjadi baca-saja untuk histori | Semua toko aktif beroperasi di NewGen |

Pemetaan ke roadmap teknis ada di `AGENTS.md` §2b.

### 6.1 Prioritas Modul

| Modul | R0 | R1 | R2 | R3 | R4 |
|---|---|---|---|---|---|
| Auth, tenant, outlet, user, role | M | | | | |
| Master barang, harga, grosir, diskon | | M | | | |
| Member, poin | | M | | | |
| Onboarding: import barang, saldo awal stok & piutang | | M | | | |
| Saldo awal hutang | | | M | | |
| Stok ledger (3 lokasi) | | M | | | |
| Kasir/penjualan + pembayaran | | M | | | |
| Retur penjualan, piutang | | M | | | |
| Cetak struk (print-agent) | | M | | | |
| Pembelian, retur beli, hutang | | | M | | |
| Opname, mutasi, pecah satuan | | S | M | | |
| Laporan | | S (dasar) | M | | |
| Restoran & KDS | | | | M | |
| Akuntansi SIAK | | | | | S |
| Acipay / PPOB | | | | | C |
| Payment gateway (Duitku/Tripay/Midtrans) | | | | | C |
| Mode offline kasir | | **[KONFIRMASI]** | | | |

---

## 7. Kebutuhan Fungsional

> Kriteria penerimaan ditulis dengan format *Given / When / Then* untuk kebutuhan berprioritas **M** di R0–R1. Modul lain dirinci saat fase terkait dimulai.

### 7.1 Autentikasi, Tenant & Hak Akses (AUTH)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-AUTH-01 | Pengguna login dengan email + password. Akun dibuat baru di NewGen; akun legacy tidak dimigrasikan. | M |
| FR-AUTH-02 | Sesi memakai access token berumur pendek (≤ 15 menit) dan refresh token (disimpan di server, dapat dicabut). | M |
| FR-AUTH-03 | Setelah login, pengguna memilih outlet aktif bila memiliki akses ke lebih dari satu outlet. | M |
| FR-AUTH-04 | Tenant, outlet, dan identitas pengguna **selalu** ditentukan server dari token, bukan dari input klien. | M |
| FR-AUTH-05 | Admin dapat membuat role dengan matriks izin per menu/aksi (lihat, tambah, ubah, hapus, approve). | M |
| FR-AUTH-06 | Aksi sensitif (edit/void nota, override harga, diskon di atas batas) memerlukan izin **dan** PIN transaksi. | M |
| FR-AUTH-07 | Admin dapat menonaktifkan pengguna dan mencabut seluruh sesinya seketika. | M |
| FR-AUTH-08 | Lupa password via OTP email/WhatsApp; OTP berlaku ≥ 5 menit, sekali pakai, dengan batas percobaan. | S |
| FR-AUTH-09 | Semua login, gagal login, dan aksi sensitif tercatat di audit log. | M |

**Kriteria penerimaan (contoh):**
- *Given* pengguna tenant A sudah login, *when* ia memanggil API dengan ID dokumen milik tenant B, *then* API merespons 404 dan tidak ada data tenant B yang dikembalikan.
- *Given* 5 kali gagal login berturut-turut, *when* percobaan ke-6 dilakukan dalam 15 menit, *then* login ditolak sementara dan kejadian tercatat di audit log.

### 7.2 Master Data (MD)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-MD-01 | CRUD barang: SKU (unik per tenant), barcode, nama, satuan, kategori, brand, principal, supplier utama, jenis (barang/jasa), boleh stok minus, boleh jual di bawah HPP. | M |
| FR-MD-02 | Harga jual dasar per barang; opsional harga khusus per outlet **[KONFIRMASI]**. | M |
| FR-MD-03 | Harga grosir bertingkat berdasarkan kuantitas. | M |
| FR-MD-04 | Diskon per barang dengan periode berlaku; voucher dengan kode unik per tenant. | S |
| FR-MD-05 | Varian barang dan catatan/tambahan per item (mis. topping resto). | S |
| FR-MD-06 | Master satuan, kategori (bertingkat), brand, principal, supplier, salesman, metode pembayaran. | M |
| FR-MD-07 | Member/pelanggan: kode, kontak, batas kredit, poin, deposit. | M |
| FR-MD-08 | Setiap perubahan harga dan atribut penting barang tercatat di audit log (nilai lama → baru, oleh siapa, kapan). | M |
| FR-MD-09 | Ekspor barang ke Excel/CSV (import: lihat FR-ONB-01). | S |
| FR-MD-10 | Cetak label barcode/harga. | C |

### 7.3 Stok & Inventori (INV)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-INV-01 | Stok dicatat per **tenant × outlet × barang × lokasi** dengan 3 lokasi: **Display**, **Gudang**, **Retur**. | M |
| FR-INV-02 | Setiap perubahan stok dicatat sebagai *movement* yang tidak dapat diubah (append-only) dengan referensi dokumen sumber. | M |
| FR-INV-03 | Saldo stok selalu sama dengan jumlah seluruh movement-nya (diverifikasi job harian). | M |
| FR-INV-04 | Penjualan mengurangi stok **Display**. Bila stok tidak cukup dan barang tidak boleh minus, transaksi ditolak dengan pesan jelas. | M |
| FR-INV-05 | Barang jenis jasa tidak memengaruhi stok. | M |
| FR-INV-06 | Kartu stok per barang/outlet/periode menampilkan saldo awal, masuk, keluar, dan saldo akhir per baris. | M |
| FR-INV-07 | Stok opname: hitung fisik → selisih → approval → movement penyesuaian. | S (R1) / M (R2) |
| FR-INV-08 | Mutasi stok antar lokasi dan antar outlet dalam satu dokumen. | M (R2) |
| FR-INV-09 | Pecah satuan (mis. 1 dus → 12 pcs) sebagai dua movement yang seimbang. | M (R2) |
| FR-INV-10 | Peringatan stok minimum per barang/outlet. | C |

**Kriteria penerimaan (contoh):**
- *Given* stok Display barang X = 1 dan barang tidak boleh minus, *when* dua kasir menjual 1 unit barang X pada saat bersamaan, *then* tepat satu transaksi berhasil, yang lain ditolak "stok tidak cukup", dan saldo akhir = 0.

### 7.4 Kasir / Penjualan (POS)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-POS-01 | Tambah barang via scan barcode, ketik SKU, atau pencarian nama (hasil < 150 ms). | M |
| FR-POS-02 | Ubah qty, hapus baris, catatan per item; seluruh operasi utama dapat dilakukan dengan keyboard (shortcut terdokumentasi). | M |
| FR-POS-03 | **Harga, grosir, diskon, dan pajak dihitung server.** Tampilan klien hanya pratinjau. | M |
| FR-POS-04 | Override harga per item hanya dengan izin + PIN; tercatat di audit log. | M |
| FR-POS-05 | Potongan per item, potongan global, pajak toko dan pajak negara (tarif per outlet), biaya lain-lain. | M |
| FR-POS-06 | Pembayaran **campuran** (split): tunai, kartu debit, kartu kredit, e-money, transfer, dan kredit/piutang; kembalian dihitung otomatis. | M |
| FR-POS-07 | Transaksi kredit membuat piutang otomatis dengan jatuh tempo (hari) dan wajib memilih member. | M |
| FR-POS-08 | Uang muka (DP) untuk pesanan. | S |
| FR-POS-09 | Hold/tunda transaksi dengan keterangan, lalu lanjutkan kembali (pengganti "keranjang pending"). | M |
| FR-POS-10 | Nomor nota unik per tenant dengan format yang dapat dikonfigurasi per tenant (prefix, kode outlet, tanggal, nomor urut). Default **[KONFIRMASI]**: `{OUTLET}-{YYMMDD}-{NNNN}`. | M |
| FR-POS-11 | Penyimpanan transaksi bersifat **idempoten**: kirim ulang (klik ganda/jaringan putus) tidak membuat nota ganda. | M |
| FR-POS-12 | Poin member bertambah sesuai aturan (total belanja ÷ kelipatan poin); edit/void menyesuaikan poin secara **selisih**, bukan menambah ulang. | M |
| FR-POS-13 | Edit nota (izin `edit_nota` + PIN) menghasilkan movement koreksi dan menyesuaikan piutang **dengan mempertahankan pembayaran yang sudah ada**. | M |
| FR-POS-14 | Void/batal nota: tidak menghapus data; status *void*, stok dikembalikan via movement, poin & piutang disesuaikan. | M |
| FR-POS-15 | Cetak struk otomatis/manual via print-agent lokal; cetak ulang tercatat. | M |
| FR-POS-16 | Daftar penjualan hari ini per kasir; rekap tutup kasir (shift) dengan setoran per metode bayar. | S |
| FR-POS-17 | Salesman per transaksi (untuk komisi/laporan). | S |
| FR-POS-18 | Mode offline: transaksi disimpan lokal dan disinkronkan saat online. | **[KONFIRMASI]** |

**Kriteria penerimaan (contoh):**
- *Given* kasir menekan "Bayar" lalu jaringan putus sebelum respons diterima, *when* aplikasi mengirim ulang dengan idempotency key yang sama, *then* server mengembalikan nota yang sama dan hanya ada satu nota di database.
- *Given* nota kredit Rp1.000.000 dengan pembayaran piutang Rp400.000, *when* nota diedit menjadi Rp900.000, *then* sisa piutang = Rp500.000 dan riwayat pembayaran tetap ada.
- *Given* setiap contoh kasus di spec test penjualan (`source/tests/spec/`), *when* dihitung oleh NewGen, *then* total, pajak, kembalian, perubahan stok, dan poin sama persis dengan nilai yang diharapkan.

**Urutan perhitungan nota (default — [KONFIRMASI] pemilik produk):**
1. Harga satuan = harga grosir (bila qty memenuhi tier) atau harga dasar, lalu dikurangi diskon barang yang berlaku.
2. Subtotal baris = harga satuan × qty − potongan per item.
3. Subtotal nota = Σ subtotal baris.
4. Dikurangi potongan global (nominal atau persen).
5. Ditambah pajak toko lalu pajak negara (persen dari hasil langkah 4).
6. Ditambah biaya lain-lain, lalu dibulatkan (aturan pembulatan per tenant **[KONFIRMASI]**) = **total**.
7. Kembalian = Σ pembayaran non-kredit − total. Sisa kurang bayar menjadi piutang (hanya untuk transaksi kredit).

### 7.5 Retur Penjualan & Piutang (AR)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-AR-01 | Retur penjualan berdasarkan nota asal; qty retur ≤ qty terjual dikurangi retur sebelumnya. | M |
| FR-AR-02 | Barang retur masuk ke lokasi **Retur** atau **Display** (dipilih) via movement. | M |
| FR-AR-03 | Pengembalian dana tunai atau potong piutang. | M |
| FR-AR-04 | Daftar piutang per member dengan umur piutang (aging) dan jatuh tempo. | M |
| FR-AR-05 | Pembayaran piutang sebagian/penuh; sisa piutang = total − Σ pembayaran (dihitung, bukan kolom yang dimutasi). | M |

### 7.5b Onboarding Toko (ONB)

Pengganti migrasi data. Dipakai untuk toko baru maupun toko pindahan dari legacy.

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-ONB-01 | Import barang dari Excel/CSV menggunakan template unduhan. Tahap pratinjau menampilkan validasi per baris (SKU duplikat, satuan/kategori belum ada, harga tidak valid); hanya baris valid yang disimpan; laporan error dapat diunduh. | M |
| FR-ONB-02 | Import master pendukung (kategori, satuan, supplier, member) dengan mekanisme yang sama. | S |
| FR-ONB-03 | Input **saldo awal stok** per outlet × barang × lokasi (manual atau import). Dicatat sebagai movement bertipe `OPENING` dengan tanggal mulai operasional dan HPP awal. | M |
| FR-ONB-04 | Input **saldo awal piutang** per member (nomor referensi lama, nominal, jatuh tempo). Dapat dibayar seperti piutang biasa. | M |
| FR-ONB-05 | Input **saldo awal hutang** per supplier. | M (R2) |
| FR-ONB-06 | Wizard setup tenant: profil usaha → outlet (pajak, zona waktu, format nota) → user & role → import barang → saldo awal → siap transaksi. | S |
| FR-ONB-07 | Saldo awal hanya dapat diinput/diubah sebelum "tanggal mulai operasional" dikunci. Setelah dikunci, perubahan stok hanya lewat opname. | M |

**Kriteria penerimaan (contoh):**
- *Given* file import berisi 1.000 barang dengan 3 baris SKU duplikat, *when* pengguna menekan "Import", *then* 997 barang tersimpan, 3 baris dilaporkan beserta alasannya, dan tidak ada barang yang tersimpan setengah.
- *Given* saldo awal stok barang X = 50 di Display outlet A, *when* kartu stok dibuka, *then* baris pertama adalah `OPENING` +50 dan saldo = 50.

### 7.6 Pembelian, Retur Beli & Hutang (PUR) — R2

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-PUR-01 | Purchase order (opsional) → penerimaan barang (stok masuk ke Gudang/Display). | M |
| FR-PUR-02 | Pembelian tunai/kredit; kredit membuat hutang dengan jatuh tempo. | M |
| FR-PUR-03 | Pembaruan HPP (metode **[KONFIRMASI]**: harga beli terakhir vs. rata-rata tertimbang). | M |
| FR-PUR-04 | Retur pembelian berdasarkan nota + supplier. | M |
| FR-PUR-05 | Pembayaran hutang sebagian/penuh; aging hutang. | M |

### 7.7 Laporan (RPT)

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-RPT-01 | Penjualan per periode/outlet/kasir/metode bayar/barang/kategori. | M (R1 dasar) |
| FR-RPT-02 | Laba kotor (penjualan − HPP snapshot saat transaksi). | M |
| FR-RPT-03 | Stok saat ini, kartu stok, nilai persediaan. | M |
| FR-RPT-04 | Pembelian, retur, piutang, hutang. | M (R2) |
| FR-RPT-05 | Ekspor Excel/PDF; laporan berat diproses di background dan diberi notifikasi saat selesai. | S |
| FR-RPT-06 | Dashboard owner: omzet hari ini, tren, barang terlaris, stok kritis. | S |

### 7.8 Restoran & KDS (RES) — R3

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-RES-01 | Denah dan status meja (kosong/terisi/dipesan). | M |
| FR-RES-02 | Pesanan dine-in/takeaway, gabung/pindah meja, reservasi dengan DP. | M |
| FR-RES-03 | KDS: antrean pesanan realtime per outlet; dapur mengubah status (diproses/siap/tersaji). | M |
| FR-RES-04 | Notifikasi realtime ke kasir/pelayan saat status berubah. | M |

### 7.9 Akuntansi SIAK (ACC) — R4

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-ACC-01 | Bagan akun (COA) per tenant; periode akuntansi dengan tutup buku. | S |
| FR-ACC-02 | Jurnal otomatis dari penjualan, pembelian, pembayaran piutang/hutang (aturan pemetaan akun dapat dikonfigurasi). | S |
| FR-ACC-03 | Jurnal manual, buku besar, kas/bank, neraca saldo, neraca, laba rugi. | S |

### 7.10 Platform & Ekstensi (PLT) — R4

| ID | Kebutuhan | Prioritas |
|---|---|---|
| FR-PLT-01 | Super admin: kelola tenant, paket/lisensi, status aktif. | M (R0 minimal) |
| FR-PLT-02 | Acipay/PPOB (Digiflazz): katalog produk digital, transaksi, deposit, webhook dengan verifikasi signature. | C |
| FR-PLT-03 | Payment gateway (Duitku/Tripay/Midtrans) untuk pembayaran non-tunai. | C |
| FR-PLT-04 | Notifikasi WhatsApp/email (struk digital, pengingat piutang). | C |

---

## 8. Kebutuhan Non-Fungsional

### 8.1 Keamanan (SEC)

| ID | Kebutuhan |
|---|---|
| NFR-SEC-01 | Seluruh endpoint (kecuali login, health, webhook bertanda tangan) wajib terautentikasi. |
| NFR-SEC-02 | Isolasi tenant berlapis: filter di service **dan** Row Level Security PostgreSQL. |
| NFR-SEC-03 | Seluruh query memakai parameter binding (sqlc); tidak ada SQL dari konkatenasi string. |
| NFR-SEC-04 | Password & PIN di-hash (bcrypt/argon2); hash tidak pernah dikirim ke klien. |
| NFR-SEC-05 | Secret hanya dari environment/secret manager; tidak ada secret di repository. |
| NFR-SEC-06 | CORS dibatasi ke origin aplikasi; rate limit per IP dan per pengguna pada login & endpoint sensitif. |
| NFR-SEC-07 | HTTPS wajib di produksi; cookie refresh token `HttpOnly`, `Secure`, `SameSite`. |
| NFR-SEC-08 | Pentest/security review dilakukan sebelum go-live tenant pilot. |
| NFR-SEC-09 | Pemrosesan data pribadi (nama, telepon, email member) mematuhi **UU No. 27/2022 tentang Pelindungan Data Pribadi**: minimisasi data, akses berbasis peran, dan data uji yang dianonimkan. |

### 8.2 Integritas Data (DATA)

| ID | Kebutuhan |
|---|---|
| NFR-DATA-01 | Setiap use-case yang menulis data berjalan dalam **satu transaksi database**. |
| NFR-DATA-02 | Nilai uang `NUMERIC(18,2)`, kuantitas `NUMERIC(18,3)`; tidak ada floating point dalam perhitungan uang. |
| NFR-DATA-03 | Kunci bisnis dijamin constraint database (unik per tenant, foreign key). |
| NFR-DATA-04 | Dokumen transaksi tidak dihapus fisik; pembatalan memakai status + movement balik. |
| NFR-DATA-05 | Efek samping eksternal (notifikasi, cetak, webhook) dikirim **setelah commit**. |

### 8.3 Performa & Skalabilitas (PERF)

| ID | Kebutuhan |
|---|---|
| NFR-PERF-01 | Target latensi sesuai M1–M2. |
| NFR-PERF-02 | Mendukung ≥ 50 kasir aktif bersamaan per instance tanpa degradasi (target awal) **[KONFIRMASI jumlah tenant/outlet aktif]**. |
| NFR-PERF-03 | Tabel movement dipartisi per bulan; laporan berat tidak memblokir transaksi kasir. |
| NFR-PERF-04 | Halaman kasir dimuat < 2 detik pada koneksi 1 Mbps (aset statis di-cache). |

### 8.4 Ketersediaan & Pemulihan (OPS)

| ID | Kebutuhan |
|---|---|
| NFR-OPS-01 | Ketersediaan ≥ 99,5% di jam operasional. |
| NFR-OPS-02 | Backup database harian + WAL archiving; **RPO ≤ 15 menit, RTO ≤ 2 jam**. Prosedur restore diuji minimal per bulan. |
| NFR-OPS-03 | Log terstruktur (JSON) dengan request ID; metrik & alert untuk error rate, latensi, dan koneksi DB. |
| NFR-OPS-04 | Deployment dengan Docker; migrasi skema otomatis dan reversible. |

### 8.5 Usability & Lokalisasi (UX)

| ID | Kebutuhan |
|---|---|
| NFR-UX-01 | Antarmuka Bahasa Indonesia; format Rupiah (`Rp1.234.567`) dan tanggal `dd/mm/yyyy`. |
| NFR-UX-02 | Zona waktu per outlet (WIB/WITA/WIT); penyimpanan dalam UTC (`timestamptz`). |
| NFR-UX-03 | Kasir dapat dioperasikan penuh dengan keyboard + scanner; layar responsif untuk tablet. |
| NFR-UX-04 | Desain konsisten mengikuti template Dreams Core (Tailwind v4); mendukung mode terang/gelap. |
| NFR-UX-05 | Browser yang didukung: Chrome/Edge 2 versi terakhir, Firefox terbaru. |

### 8.6 Kualitas Kode (QA)

| ID | Kebutuhan |
|---|---|
| NFR-QA-01 | Setiap modul memiliki unit test untuk invariant bisnis; test konkurensi untuk stok dan penomoran nota. |
| NFR-QA-02 | Spec test (contoh kasus perhitungan) penjualan, retur, piutang, dan onboarding berjalan otomatis di CI. |
| NFR-QA-03 | Lint & test wajib lulus sebelum merge. |

---

## 9. Arsitektur Solusi (Ringkas)

```
┌──────────────┐   HTTPS/JSON    ┌───────────────────────────┐     ┌─────────────┐
│ SvelteKit SPA│ ──────────────► │  Go API (modular monolith)│ ──► │ PostgreSQL  │
│ (browser)    │ ◄── SSE/WS ──── │  auth · tenant · catalog  │     │ (sumber     │
└──────┬───────┘                 │  stock · sales · purchase │     │  kebenaran) │
       │ localhost               │  report · resto · acc     │     └─────────────┘
┌──────▼───────┐                 └──────────┬────────────────┘     ┌─────────────┐
│ Print-agent  │                            └────────────────────► │ Redis       │
│ (Go, PC kasir│                                                   │ cache · pub/│
│  ESC/POS)    │                                                   │ sub · queue │
└──────────────┘                                                   └─────────────┘
```

Detail keputusan teknis (library, struktur folder, konvensi) ada di `AGENTS.md` §1, §3, §5, §6.

---

## 10. Model Data Inti (Ringkas)

| Entitas | Keterangan | Padanan di legacy (referensi istilah saja, data tidak dimigrasi) |
|---|---|---|
| Tenant | Usaha/pelanggan ACIRABA | `KODEUNIKMEMBER` |
| Outlet | Cabang/toko | `01_set_outlet` |
| User, Role | Pegawai dan hak akses | `01_tms_penggunaaplikasi`, `01_tms_penggunaaplikasiha` |
| Item, Price, Wholesale tier | Barang dan harga | `01_tms_barangkharisma`, `01_tms_bestbuybaranggrosir` |
| Customer | Member, poin, deposit | `01_tms_member`, `01_tms_memberdeposit` |
| Sale, Sale line, Sale payment | Penjualan | `01_trs_barangkeluar`, `_detail`, `_dp` |
| Stock balance, Stock movement | Saldo & ledger stok | `01_tms_stok`, `01_trs_kartustok` |
| Receivable / Payable (+ payments) | Piutang/hutang | `01_tms_piutangkredit*`, `01_tms_hutangtoko*` |
| Purchase (+ lines), Purchase return | Pembelian | `01_trs_barangmasuk*` |
| Audit log | Jejak perubahan | `01_log_*` |

Rancangan kolom: `AGENTS.md` §4.

---

## 11. Integrasi Eksternal

| Integrasi | Arah | Rilis | Catatan |
|---|---|---|---|
| Printer thermal (ESC/POS) | API/Web → print-agent lokal | R1 | Menggantikan 2 implementasi printer legacy |
| Barcode scanner | Input keyboard (HID) | R1 | Tanpa driver khusus |
| WhatsApp gateway (WaSender) | Keluar | R4 | OTP, struk, pengingat piutang |
| Email SMTP | Keluar | R0/R4 | OTP/reset password |
| Digiflazz (PPOB) | Dua arah + webhook | R4 | Verifikasi signature webhook wajib |
| Duitku / Tripay / Midtrans | Dua arah + callback | R4 | **[KONFIRMASI]** yang masih aktif |
| Google (login/Drive) | — | **[KONFIRMASI]** | Ditemukan di legacy, kegunaan belum jelas |

---

## 12. Strategi Peluncuran (Data Baru)

**Prinsip:** NewGen dimulai dengan data baru. Tidak ada ETL, tidak ada sistem paralel, dan tidak ada rekonsiliasi dengan legacy.

1. **Spesifikasi aturan bisnis:** baca kode legacy hanya untuk memahami aturan (poin, pajak, grosir, piutang, pembulatan). Tulis contoh kasus beserta hasil yang benar, minta pemilik produk menyetujuinya, lalu jadikan spec test (`source/tests/spec/`).
2. **Pemilihan fitur:** tandai fitur legacy yang masih dipakai (masuk scope) dan yang dibuang (Q1).
3. **Toko pilot:** satu toko memulai di NewGen pada tanggal mulai operasional yang disepakati:
   - H-3 s.d. H-1: setup tenant, import barang, pelatihan kasir & admin.
   - Malam H-1 (setelah tutup): stok opname fisik → input saldo awal stok; input saldo awal piutang/hutang dari laporan legacy per tanggal itu.
   - Hari H: transaksi berjalan di NewGen; legacy untuk toko tersebut berhenti menerima transaksi.
4. **Histori:** sebelum hari H, ekspor laporan legacy yang dibutuhkan (penjualan, piutang, hutang, stok) ke PDF/Excel. Legacy tetap dapat diakses dalam mode baca-saja selama masa retensi **[KONFIRMASI]**.
5. **Rencana mundur:** jika ada masalah kritis di minggu pertama, toko kembali ke legacy. Transaksi yang sudah terjadi di NewGen dicatat ulang manual di legacy (volume kecil karena hanya satu toko).
6. **Peluncuran luas (R5):** toko lain onboarding bertahap memakai prosedur yang sama.

---

## 13. Risiko & Mitigasi

| Risiko | Kemungkinan | Dampak | Mitigasi |
|---|---|---|---|
| Aturan bisnis legacy terlewat | Sedang | Tinggi | Inventaris SP/trigger (`AGENTS.md` §8); spec test disetujui pemilik produk sebelum modul dibangun |
| Proyek berlarut, legacy terus dipakai dengan risiko keamanan | Sedang | Tinggi | Tambal kritis legacy dulu (tutup port API, rotasi kredensial); rilis bertahap R1 → R5 |
| Klien keberatan kehilangan histori di aplikasi baru | Sedang | Sedang | Ekspor laporan sebelum mulai; legacy baca-saja selama masa retensi |
| Saldo awal salah input (stok/piutang) | Sedang | Tinggi | Opname fisik di malam H-1; import dengan pratinjau & validasi; saldo awal dikunci setelah tanggal mulai (FR-ONB-07) |
| Klien menolak perubahan UI | Sedang | Sedang | Libatkan kasir tenant pilot sejak prototipe kasir; shortcut keyboard mirip legacy |
| Internet toko tidak stabil | Sedang | Tinggi | Idempotency + **[KONFIRMASI]** mode offline |
| Kapasitas pengembang terbatas (tim kecil) | Tinggi | Sedang | Scope MoSCoW ketat; modul opsional ditunda |
| Lisensi template UI | Rendah | Sedang | Pastikan lisensi Dreams Core mencakup produk SaaS komersial **[KONFIRMASI]** |

---

## 14. Asumsi, Dependensi & Pertanyaan Terbuka

### 14.1 Asumsi
- Satu instance backend melayani banyak tenant (SaaS), dengan opsi on-premise di kemudian hari.
- Seluruh outlet memakai Rupiah dan berada di Indonesia.
- Data legacy tidak dimigrasikan; setiap toko memulai dengan onboarding (§7.5b).

### 14.2 Dependensi
- Go ≥ 1.23, PostgreSQL ≥ 16, Redis ≥ 7, Docker.
- Kode legacy (`../aciraba_siak_os`) sebagai referensi aturan bisnis.
- Ketersediaan toko pilot yang bersedia memulai dengan saldo awal baru.

### 14.3 Pertanyaan Terbuka (wajib dijawab pemilik produk)

| # | Pertanyaan | Memengaruhi |
|---|---|---|
| Q1 | Modul legacy mana yang masih aktif dipakai klien (resto, SIAK, Acipay, payment gateway)? | Scope R3–R4 |
| Q2 | Apakah kasir wajib bisa berjalan offline? | Arsitektur frontend R1 |
| Q3 | Hosting: VPS/cloud terpusat, on-premise per toko, atau keduanya? | Deployment, lisensi |
| Q4 | Format nomor nota default untuk NewGen? Usulan: `{OUTLET}-{YYMMDD}-{NNNN}` | FR-POS-10 |
| Q5 | Metode HPP: harga beli terakhir atau rata-rata tertimbang? | FR-PUR-03, laporan laba |
| Q6 | Urutan perhitungan nota (§7.4) dan aturan pembulatan sudah sesuai? Berapa lama legacy tetap bisa diakses baca-saja untuk histori? | FR-POS, §12 |
| Q7 | Jumlah tenant, outlet, dan kasir aktif saat ini? | Target performa, biaya server |
| Q8 | Apakah harga dapat berbeda per outlet? | FR-MD-02 |
| Q9 | Apakah lisensi template Dreams Core mencakup penggunaan SaaS komersial? | Risiko legal UI |
| Q10 | Siapa tenant pilot dan kapan target go-live R1? | Jadwal |

---

## 15. Lampiran

### 15.1 Glosarium

| Istilah | Arti |
|---|---|
| Tenant | Satu usaha/pelanggan ACIRABA (legacy: `KODEUNIKMEMBER`) |
| Outlet | Cabang/toko milik tenant (legacy: `LOKASI`, `OUTLET`, `KODEOUTLET`) |
| Lokasi stok | Display (rak jual), Gudang, Retur |
| Movement | Catatan perubahan stok yang tidak dapat diubah |
| HPP | Harga Pokok Penjualan |
| Spec test | Test otomatis dari contoh kasus perhitungan yang ditulis manual dan disetujui pemilik produk |
| Saldo awal (opening) | Stok/piutang/hutang yang diinput saat toko mulai memakai NewGen |
| Onboarding | Proses menyiapkan toko di NewGen: setup, import barang, saldo awal |
| Idempotency key | Kunci unik per permintaan agar pengiriman ulang tidak membuat data ganda |
| RLS | Row Level Security: pembatasan baris data di level database |
| KDS | Kitchen Display System |
| Tanggal mulai operasional | Tanggal toko mulai bertransaksi di NewGen; saldo awal dikunci sejak tanggal ini |

### 15.2 Referensi
- `AGENTS.md`: keputusan teknis, roadmap, peta legacy, aturan trigger.
- `reference/template/`: acuan UI Dreams Core (pemetaan halaman: `AGENTS.md` §5b).
- Sistem legacy: `../aciraba_siak_os/` (read-only).

### 15.3 Perbedaan Perilaku terhadap Legacy (bug yang tidak ditiru)

| Perilaku legacy | Perilaku NewGen | Alasan |
|---|---|---|
| Edit nota menambah poin member lagi tanpa mengurangi poin lama | Poin disesuaikan sebesar selisih | Bug: poin menggelembung |
| Edit nota kredit membuat ulang piutang sehingga pembayaran sebelumnya hilang dari saldo | Piutang disesuaikan; pembayaran dipertahankan | Bug: saldo piutang salah |
| Catatan DP kolom kredit berisi nomor kartu | Berisi nominal | Bug trigger |
| Hapus nota menghapus data fisik | Void dengan status + movement balik | Jejak audit |
| Harga dari klien diterima apa adanya | Harga dihitung server; override dengan izin + PIN | Keamanan |
| Diskon/voucher unik global lintas tenant | Unik per tenant | Bug multi-tenant |

---

### Persetujuan

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Pemilik produk | | | |
| Lead developer | | | |
