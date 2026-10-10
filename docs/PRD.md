# Product Requirements Document (PRD)
# ARUS — Sistem POS & ERP Ritel Multi-Tenant (Web + Kasir Mobile)

| Atribut | Nilai |
|---|---|
| Versi dokumen | 0.4 (Draf) |
| Status | Draf — menunggu review pemilik produk (bagian Kasir Mobile = **usulan**, belum disetujui) |
| Tanggal | 10 Oktober 2026 |
| Pemilik produk | Pemilik ACIRABA (Erayadigital) |
| Penyusun | Tim pengembang (dibantu AI) |
| Dokumen terkait | `AGENTS.md` (BAGAIMANA: keputusan teknis), `docs/ROADMAP.md` (roadmap), `.agent/SESSION.md` (status), `reference/template/` (acuan UI) |

### Riwayat Revisi

| Versi | Tanggal | Perubahan | Oleh |
|---|---|---|---|
| 0.4 | 2026-10-10 | Status diselaraskan dengan kode di `main` (PR #21): cetak struk + print-agent, shift kasir, retur penjualan + batal retur, retur pembelian, deposit member, kredit pemasok, saldo awal piutang selesai. FR-AR-02 disesuaikan dengan keputusan 2026-10-09 (retur selalu ke lokasi Retur). Q10 terjawab (toko pilot Kotak Cantik Magelang). Posisi saat ini dan jalur kritis (§6, §6.3) diperbarui. | Tim |
| 0.3 | 2026-10-09 | (1) PRD diselaraskan dengan keputusan produk 8–9 Okt yang sebelumnya hanya tercatat di `AGENTS.md`: urutan hitung nota tanpa pembulatan, format nomor nota, harga per cabang, barcode boleh kembar, kredit hanya untuk member + limit, edit nota = revisi, biaya metode bayar (MDR), kupon, level member, HPP per cabang, opname, mutasi dua tahap, pelunasan kolektif. (2) FR-POS-13 **diubah**: nota yang piutangnya sudah dibayar tidak dapat diedit. (3) Kolom **Status** pada setiap tabel FR. (4) Tambahan **§7.11 Kasir Mobile (MOB)**, NFR mobile, metrik, risiko, pertanyaan Q11–Q17, dan analisis opsi platform (§15.4). (5) Q4, Q5, Q8 ditandai terjawab; Q3 dan Q6 sebagian. | Tim |
| 0.2 | 2026-10-07 | Pendekatan **data baru**: tidak ada migrasi data legacy. Golden test diganti *spec test*; ETL/cutover diganti onboarding; ditambah FR-ONB; Q4/Q6 diganti | Tim |
| 0.1 | 2026-10-06 | Draf awal berdasarkan analisis sistem legacy ACIRABA SIAK OS | Tim |

### Konvensi Penandaan

- **[KONFIRMASI]**: asumsi yang belum diverifikasi ke pemilik produk atau klien. Wajib dijawab sebelum fase terkait dimulai.
- **[USULAN]**: rancangan baru yang belum disetujui pemilik produk. Belum boleh dikerjakan.
- **Prioritas (MoSCoW):** **M** = Must (wajib di rilis), **S** = Should, **C** = Could, **W** = Won't (tidak di rilis ini).
- **Status implementasi** (ringkas; sumber kebenaran tetap `docs/ROADMAP.md`): ✅ selesai · ◐ sebagian · ○ belum · ⏸ ditunda atas keputusan.
- ID kebutuhan: `FR-<MODUL>-<NN>` (fungsional), `NFR-<KATEGORI>-<NN>` (non-fungsional). ID yang sudah ada **tidak dinomori ulang**. Kebutuhan baru ditambahkan di akhir tabel.
- "Keputusan YYYY-MM-DD" = aturan yang sudah dipilih pemilik produk dan **tidak diperdebatkan ulang** tanpa persetujuannya.

---

## 1. Ringkasan Eksekutif

ACIRABA adalah aplikasi Point of Sale dan ERP ringan berbasis web untuk usaha ritel dan restoran multi-outlet. Sistem saat ini (*legacy*: CodeIgniter 4 + Node.js/Express + MySQL) sudah dipakai klien, tetapi punya tiga masalah mendasar:

- **Keamanan:** API tanpa autentikasi, dan identitas tenant diambil dari browser.
- **Integritas data:** transaksi database tidak atomik, stok rawan selisih, dan nilai uang disimpan sebagai floating point.
- **Pemeliharaan:** logika bisnis tersebar di PHP, Node.js, 5 stored procedure generik (178 cabang), dan 22 trigger.

**ARUS** membangun ulang sistem ini di atas **Go** (backend), **PostgreSQL** (database), **Redis** (cache/realtime), dan **SvelteKit** (frontend web). Sasarannya:

1. Transaksi kasir yang **cepat, benar, dan tidak pernah tercatat ganda**.
2. Stok, piutang, dan hutang yang **selalu dapat ditelusuri** (berbasis ledger dan dihitung, bukan kolom yang ditimpa).
3. **Isolasi data antar-tenant** yang dijamin di server dan di database.
4. Kode yang **mudah dikembangkan dan dites**.

ARUS adalah **aplikasi baru dengan data baru**. Data transaksi legacy **tidak dimigrasikan**. Toko memulai lewat *onboarding*: import master barang, input saldo awal stok, serta saldo awal piutang/hutang. Legacy hanya dipakai sebagai referensi aturan bisnis. Kebenaran perhitungan dijamin oleh *spec test*: contoh kasus yang dihitung manual dan disetujui pemilik produk.

**Pengembangan baru [USULAN]: Kasir Mobile.** Aplikasi kasir untuk ponsel/tablet Android, ditujukan untuk toko tanpa PC kasir, penjualan di luar meja kasir (bazar, *canvassing*, pramuniaga yang melayani di lorong), dan perangkat POS genggam dengan printer bawaan. Kasir Mobile **tidak** membawa logika bisnis sendiri. Seluruh harga, stok, piutang, dan nomor nota tetap dihitung server yang sama. Tujuannya menambah **kanal penjualan**, bukan membuat sistem kedua (§7.11, §15.4).

---

## 2. Latar Belakang & Pernyataan Masalah

### 2.1 Kondisi Saat Ini (legacy)

| Aspek | Kondisi legacy | Dampak bisnis |
|---|---|---|
| Keamanan API | Endpoint Node.js tidak menerapkan autentikasi; CORS `*` | Pihak luar yang menjangkau server bisa membaca atau mengubah data semua toko |
| Identitas tenant | Kode tenant, kasir, dan outlet dikirim dari browser | Satu tenant berpotensi memalsukan data tenant lain |
| Transaksi | `BEGIN/COMMIT` dijalankan lewat pool sehingga bisa memakai koneksi berbeda | Nota bisa tersimpan sebagian; stok tidak sesuai penjualan |
| Stok | Saldo kartu stok dihitung di memori; tabel stok tanpa kunci unik | Selisih stok saat beberapa kasir bertransaksi bersamaan |
| Nilai uang | Tipe `DOUBLE` (86 kolom) | Selisih pembulatan di laporan dan neraca |
| Harga | Harga jual dikirim dari browser dan disimpan apa adanya | Risiko manipulasi harga di kasir |
| Aturan bisnis | Tersembunyi di trigger (poin, piutang, hutang, mutasi) | Sulit diubah; ada bug, misalnya poin member bertambah ganda setiap nota diedit |
| Performa query | 512 pencarian `LIKE '%…%'`, fungsi stok dipanggil per baris | Pencarian dan laporan makin lambat seiring data tumbuh (±10 juta baris kartu stok) |
| Akses database | PHP dan Node.js sama-sama menulis ke database | Aturan tidak konsisten; ada celah SQL injection |
| Frontend | jQuery, rendering di server, 469 blok pemanggilan API yang diduplikasi | Pengembangan fitur lambat; UX kasir sulit ditingkatkan |
| Perangkat kasir | Hanya browser di PC/laptop; wajib online (minimal 1 Mbps) | Kasir berhenti saat internet putus; tidak ada pilihan untuk toko tanpa PC |

### 2.2 Pernyataan Masalah

> Pemilik toko membutuhkan sistem kasir dan pengelolaan toko yang **angkanya dapat dipercaya** (penjualan, stok, piutang, hutang) dan **datanya aman**. Sistem saat ini tidak dapat menjamin keduanya, dan biaya menambal arsitektur lama lebih tinggi daripada membangun ulang dengan fondasi yang benar.

### 2.3 Masalah yang Dijawab Kasir Mobile [USULAN]

> Sebagian toko tidak punya, atau tidak mau membeli, PC + printer kasir. Sebagian lain perlu berjualan di luar meja kasir. Saat ini mereka tidak dapat memakai ACIRABA sama sekali, atau mencatat penjualan di kertas lalu memasukkannya belakangan. Akibatnya stok dan uang tidak sinkron.

**Bukti yang masih dibutuhkan (Q15):** berapa klien atau calon klien yang meminta fitur ini, dan perangkat apa yang mereka pakai. Tanpa bukti ini, Kasir Mobile berisiko menjadi pekerjaan yang menunda go-live R1.

---

## 3. Tujuan & Non-Tujuan

### 3.1 Tujuan Produk

| ID | Tujuan |
|---|---|
| G1 | Kasir dapat menyelesaikan transaksi dengan cepat menggunakan keyboard/scanner, tanpa nota ganda. |
| G2 | Stok per outlet selalu konsisten, dan setiap perubahan dapat ditelusuri ke dokumen sumbernya. |
| G3 | Data setiap tenant terisolasi penuh; tidak ada kebocoran lintas tenant. Data antar-cabang dibatasi menurut akses pengguna. |
| G4 | Angka penjualan, piutang, hutang, dan laporan sesuai aturan bisnis yang terdokumentasi (diverifikasi spec test). |
| G5 | Toko (baru maupun pindahan dari legacy) dapat mulai beroperasi dalam ≤ 1 hari melalui onboarding. |
| G6 | Fondasi kode yang modular sehingga fitur baru dapat ditambahkan tanpa merusak fitur lain. |
| G7 [USULAN] | Toko dapat berjualan dari ponsel/tablet Android **dengan aturan, harga, stok, dan nomor nota yang sama persis** dengan kasir web, lalu mencetak struk ke printer Bluetooth atau printer bawaan perangkat. |

### 3.2 Non-Tujuan (di luar cakupan rilis awal)

- **Aplikasi back-office mobile.** Kelola barang, pembelian, opname, dan laporan lengkap tetap di web (web sudah responsif). Kasir Mobile hanya untuk **berjualan** (§7.11).
- **Logika bisnis terpisah di aplikasi mobile.** Mobile tidak menghitung harga, pajak, atau stok sendiri (kecuali pratinjau tampilan).
- **iOS pada rilis mobile pertama** **[KONFIRMASI Q16]**.
- Aplikasi untuk pelanggan/member (cek poin, pesan online).
- Marketplace atau integrasi e-commerce.
- Akuntansi lengkap setara software akuntansi khusus (e-Faktur, konsolidasi multi-entitas).
- **Migrasi data transaksi/histori dari legacy.** Histori lama tetap dapat dilihat di legacy (baca-saja) atau dari ekspor laporan.
- Kompatibilitas dengan legacy: akun/password, format nomor nota, dan struktur data tidak harus sama.
- Mempertahankan bug legacy (lihat §15.3).

---

## 4. Metrik Keberhasilan

| ID | Metrik | Target | Cara ukur |
|---|---|---|---|
| M1 | Latensi simpan transaksi kasir (p95, server) | < 300 ms untuk ≤ 50 baris; < 1 dtk untuk ≤ 500 baris | Log/metrics API (ukuran awal: 400 baris ≈ 570 ms) |
| M2 | Pencarian barang di kasir (p95) | < 150 ms untuk 50.000 SKU | Metrics API + cache |
| M3 | Nota ganda akibat klik ganda/jaringan | 0 | Constraint unik + idempotency key, audit mingguan |
| M4 | Kelulusan spec test | 100% | Test otomatis di CI |
| M5 | Selisih stok sistem vs. ledger | 0 | Job rekonsiliasi harian `Σ movements = balance` |
| M6 | Insiden akses lintas tenant | 0 | Test otomatis RLS + pentest sebelum go-live |
| M7 | Waktu onboarding toko pilot (import barang + saldo awal hingga transaksi pertama) | ≤ 1 hari kerja | Observasi di toko pilot |
| M8 | Ketersediaan layanan (jam operasional) | ≥ 99,5% per bulan | Uptime monitor |
| M9 | Waktu pelatihan kasir baru | ≤ 30 menit hingga mandiri | Observasi di tenant pilot **[KONFIRMASI]** |
| M10 [USULAN] | Waktu transaksi di Kasir Mobile (scan kamera 5 barang + bayar tunai + cetak) | ≤ 45 detik oleh kasir terlatih | Observasi + telemetri durasi layar |
| M11 [USULAN] | Sesi aplikasi mobile tanpa crash | ≥ 99,5% | Laporan crash (Play Console / crash reporter) |
| M12 [USULAN] | Struk gagal tercetak tanpa bisa dicetak ulang | 0 (setiap nota tersimpan dapat dicetak ulang) | Log cetak per nota |
| M13 [USULAN] | Waktu buka aplikasi hingga siap scan (cold start, HP kelas menengah 3 GB RAM) | ≤ 3 detik | Uji perangkat acuan |

---

## 5. Pengguna & Persona

| Persona | Deskripsi | Kebutuhan utama | Kanal |
|---|---|---|---|
| **Owner / Admin Tenant** | Pemilik usaha dengan 1..n outlet | Laporan omzet & laba, kontrol harga, hak akses pegawai, stok semua outlet, menyetujui aksi sensitif (PIN) | Web |
| **Manajer Outlet / Supervisor** | Penanggung jawab satu outlet | Stok outlet, opname, persetujuan edit/void nota, ubah harga, pindah outlet di kasir, laporan harian | Web (+ PIN di perangkat kasir) |
| **Kasir** | Operator transaksi di meja kasir | Input cepat (scanner/keyboard), pembayaran campuran, cetak struk, nota pending | Web (PC) |
| **Kasir Mobile / Pramuniaga** [USULAN] | Kasir tanpa PC, penjual di bazar/lorong, sales keliling | Scan kamera, layar sentuh, bayar cepat, cetak struk Bluetooth, tetap jalan saat sinyal buruk | Android |
| **Pemilik toko kecil** [USULAN] | Usaha 1 outlet, 1–2 orang, tanpa PC | Jualan dan lihat penjualan hari ini dari satu HP | Android (+ web untuk master) |
| **Staf Gudang / Pembelian** | Penerimaan & mutasi barang | Pembelian, retur ke supplier, mutasi antar outlet/lokasi, opname | Web |
| **Akuntan / Keuangan** | Pencatatan keuangan | Piutang, hutang, kas/bank, jurnal, neraca, laba rugi | Web |
| **Dapur (Resto)** | Staf dapur restoran | Antrean pesanan realtime (KDS), ubah status pesanan | Web (tablet) |
| **Platform Admin** | Tim Erayadigital (operator ARUS) | Kelola tenant, masuk-sebagai untuk bantuan (dengan kesepakatan tertulis), audit, 2FA wajib | Web (panel terpisah) |
| **Pelanggan/Member** | Pembeli (tidak login) | Poin, level, piutang tercatat dengan benar | — |

---

## 6. Ruang Lingkup & Rencana Rilis

| Rilis | Nama | Isi | Kriteria selesai |
|---|---|---|---|
| **R0** | Fondasi | Infrastruktur, auth, tenant, RLS, role, layout UI | Login berjalan; isolasi tenant lolos test — **✅ selesai** |
| **R1 (MVP)** | Toko Bisa Jualan | Master data, onboarding (import barang, saldo awal stok/piutang), stok ledger, kasir web, retur jual, piutang, **cetak struk**, laporan penjualan & stok dasar | Lulus spec test penjualan; toko pilot beroperasi harian di ARUS |
| **R2** | Operasional Lengkap | Pembelian, retur beli, hutang (+ saldo awal hutang), opname, mutasi, pecah satuan, laporan lengkap | Toko pilot memakai ARUS untuk seluruh operasional harian |
| **RM** [USULAN] | Kasir Mobile | Aplikasi Android untuk berjualan (§7.11) dalam tahap M1–M4 | Lihat §6.2 |
| **R3** | Restoran | Meja, pesanan, dine-in/takeaway, KDS realtime | **[KONFIRMASI Q1]** jumlah klien resto aktif |
| **R4** | Keuangan & Ekstensi | Akuntansi SIAK, Acipay/PPOB, payment gateway, notifikasi WhatsApp | **[KONFIRMASI Q1]** modul mana yang masih dipakai |
| **R5** | Peluncuran Luas | Onboarding toko lain bertahap; legacy toko yang sudah pindah menjadi baca-saja | Semua toko aktif beroperasi di ARUS |

Pemetaan ke roadmap teknis ada di `docs/ROADMAP.md`.

**Posisi saat ini (10 Okt 2026):** R0 selesai. Fitur kasir web R1 praktis lengkap (FR-POS 20 dari 22 selesai; sisa catatan per baris dan offline yang masih [KONFIRMASI]). Cetak struk, shift kasir, retur penjualan, saldo awal piutang, deposit member sudah ada. Sebagian besar R2 juga sudah dibangun (pembelian, retur beli, hutang, opname, mutasi). Toko pilot: **Kotak Cantik Magelang**, target mulai 1 Nov 2026. Yang masih menghalangi go-live kini **bukan layar kasir**, melainkan: import barang (FR-ONB-01, katalog > 100 ribu barang), spec test yang disetujui (M4), backup + uji restore (NFR-OPS), dan uji coba di toko. Lihat §6.3.

### 6.1 Prioritas Modul

| Modul | R0 | R1 | R2 | RM | R3 | R4 |
|---|---|---|---|---|---|---|
| Auth, tenant, outlet, user, role | M | | | | | |
| Master barang, harga per cabang, grosir, satuan tambahan | | M | | | | |
| Diskon barang (master) | | S | | | | |
| Member, level, poin | | M | | | | |
| Kupon belanja | | S | | | | |
| Metode pembayaran + biaya (MDR) | | M | | | | |
| Onboarding: import barang, saldo awal stok & piutang | | M | | | | |
| Saldo awal hutang | | | M | | | |
| Stok ledger (3 lokasi) | | M | | | | |
| Kasir web + pembayaran + kredit | | M | | | | |
| Edit/void nota (revisi) | | M | | | | |
| Retur penjualan, piutang | | M | | | | |
| Cetak struk (print-agent PC) | | M | | | | |
| Pembelian, retur beli, hutang | | | M | | | |
| Opname, mutasi, pecah satuan | | S | M | | | |
| Laporan | | S (dasar) | M | | | |
| **Kasir Mobile Android (online)** | | | | M | | |
| **Cetak Bluetooth / printer bawaan** | | | | M | | |
| **Kasir offline** | | **[KONFIRMASI Q2]** | | **[KONFIRMASI Q13]** | | |
| Restoran & KDS | | | | | M | |
| Akuntansi SIAK | | | | | | S |
| Acipay / PPOB | | | | | | C |
| Payment gateway (Duitku/Tripay/Midtrans) | | | | | | C |

### 6.2 Tahapan Kasir Mobile [USULAN]

| Tahap | Isi | Prasyarat | Kriteria selesai |
|---|---|---|---|
| **M0 — Validasi** | Wawancara 3–5 klien; daftar perangkat & printer nyata yang mereka pakai (Q12, Q15) | — | Ada ≥ 1 toko yang bersedia menjadi pilot mobile, dengan perangkat yang sudah diketahui |
| **M1 — Kasir online Android** | FR-MOB-01..14, 17, 19 | Go-live toko pilot web (R1) **atau** persetujuan eksplisit untuk menjalankannya paralel; model struk dari FR-POS-15 | Toko pilot mobile berjualan 1 minggu tanpa nota ganda dan tanpa selisih stok |
| **M2 — Perangkat POS & tablet** | Printer bawaan perangkat POS genggam (FR-MOB-09b), tata letak tablet lanskap, scanner HID/Bluetooth | M1 | Lulus uji di ≥ 1 model perangkat POS yang dipakai klien |
| **M3 — Offline terbatas** | FR-MOB-15 (hanya bila Q13 = ya) | M1 stabil; aturan offline disetujui | Spec test offline lulus; tidak ada nota ganda saat sinkronisasi |
| **M4 — iOS** | Build iOS | Q16 = ya | Lulus review App Store |

### 6.3 Jalur Kritis ke Go-Live Pilot (R1)

Urutan yang disarankan agar R1 benar-benar selesai sebelum fitur baru ditambah (diperbarui 2026-10-10):

1. ~~Cetak struk + model struk bersama (FR-POS-15)~~ — selesai 2026-10-10.
2. ~~Retur penjualan (FR-AR-01..03)~~ — selesai 2026-10-09/10.
3. ~~Saldo awal piutang manual (FR-ONB-04)~~ — selesai 2026-10-10. ~~Shift kasir (FR-POS-16)~~ — selesai 2026-10-10.
4. **Import barang + saldo awal stok dari Excel/CSV (FR-ONB-01, FR-ONB-03 import)** — penghalang utama; sumber awal = ekspor master barang legacy toko pilot.
5. **Backup terjadwal di luar server + satu kali uji restore** (NFR-OPS).
6. **Spec test penjualan** yang disetujui pemilik produk (M4, `docs/ROADMAP.md` Fase 1.3).
7. CI otomatis (test + check setiap PR).
8. Uji coba satu hari di PC kasir toko (buka/tutup shift, cetak, retur, piutang), lalu go-live toko pilot (`docs/ROADMAP.md` Fase 9).

Tidak menghalangi go-live, boleh menyusul: pembatalan pembayaran piutang/hutang (FR-AR-07, FR-PUR-09), saldo awal hutang (FR-ONB-05), catatan per baris (FR-POS-02), laporan lanjutan (FR-RPT).

---

## 7. Kebutuhan Fungsional

> Kriteria penerimaan ditulis dengan format *Given / When / Then* untuk kebutuhan berprioritas **M**. Modul lain dirinci saat fase terkait dimulai.
> Kebutuhan yang berlaku untuk **web dan mobile** cukup ditulis sekali di modulnya (mis. FR-POS). §7.11 hanya memuat yang khas mobile.

### 7.1 Autentikasi, Tenant & Hak Akses (AUTH)

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-AUTH-01 | Pengguna login dengan email + password. Akun dibuat baru di ARUS; akun legacy tidak dimigrasikan. | M | ✅ |
| FR-AUTH-02 | Sesi memakai access token berumur pendek (≤ 15 menit) dan refresh token yang disimpan di server, dirotasi setiap dipakai, dan dapat dicabut. Pemakaian ulang refresh token lama mencabut seluruh rantai sesi. | M | ✅ |
| FR-AUTH-03 | Pengguna bekerja pada **satu outlet aktif** dan dapat berpindah ke outlet lain yang boleh diaksesnya. **Di layar kasir**, pindah outlet membutuhkan PIN penyetuju (izin `outlet_switch.approve`) dan mengosongkan keranjang (keputusan 2026-10-08). | M | ✅ |
| FR-AUTH-04 | Tenant, outlet, dan identitas pengguna **selalu** ditentukan server dari token, bukan dari input klien. Dokumen milik cabang yang tidak boleh diakses → 403 `OUTLET_FORBIDDEN`; milik tenant lain → 404. | M | ✅ |
| FR-AUTH-05 | Admin membuat role dengan matriks izin per menu × aksi (lihat, tambah, ubah, hapus, setujui). Pengguna tidak dapat memberikan izin yang tidak dimilikinya sendiri (anti-eskalasi). Izin penuh hanya milik role sistem **Owner**. | M | ✅ |
| FR-AUTH-06 | Aksi sensitif membutuhkan **persetujuan penyetuju + PIN 6 digit**: ubah harga, edit/void nota, melewati limit kredit, pindah outlet di kasir. Satu PIN per pengguna; 5 kali salah → terkunci 15 menit. Potongan di atas batas tertentu belum memakai PIN. | M | ◐ |
| FR-AUTH-07 | Admin dapat menonaktifkan pengguna dan mencabut seluruh sesinya seketika (semua perangkat, termasuk mobile). | M | ✅ |
| FR-AUTH-08 | Lupa password lewat **tautan email** sekali pakai berbatas waktu, tanpa mengungkap apakah email terdaftar. Reset mencabut semua sesi. OTP WhatsApp = C. | S | ✅ (email) |
| FR-AUTH-09 | Semua login, gagal login, dan aksi sensitif tercatat di audit log (append-only). | M | ✅ |
| FR-AUTH-10 | Registrasi tenant mandiri dengan persetujuan Syarat Layanan (versi tercatat) dan verifikasi email. Email yang belum terverifikasi hanya menampilkan banner (tidak memblokir); admin dapat menandai terverifikasi secara manual. | S | ✅ (teks hukum masih draf) |
| FR-AUTH-11 | Kunci login bertahap per IP+email (5 gagal → 1/5/15/60 menit) dengan sisa percobaan ditampilkan. Kebijakan dapat diubah tanpa deploy. | M | ✅ |
| FR-AUTH-12 | Mode **"Hanya Kasir"**: pengguna dengan izin ini langsung masuk ke layar kasir dan tidak dapat membuka menu lain. | S | ✅ |
| FR-AUTH-13 | **Platform Admin** (operator ARUS) terpisah dari pengguna tenant, wajib 2FA TOTP. Dapat "masuk sebagai" tenant (baca & tulis, atas dasar kesepakatan tertulis dengan tenant; keputusan 2026-10-08) selama 30 menit; setiap sesi tercatat di audit platform **dan** audit tenant. | M | ✅ |
| FR-AUTH-14 [USULAN] | Sesi perangkat mobile tidak bergantung pada cookie browser; setiap perangkat terdaftar dan dapat dicabut satu per satu (FR-MOB-16). | M (RM) | ○ |

**Kriteria penerimaan:**
- *Given* pengguna tenant A sudah login, *when* ia memanggil API dengan ID dokumen milik tenant B, *then* API merespons 404 dan tidak ada data tenant B yang dikembalikan.
- *Given* 5 kali gagal login berturut-turut dari IP dan email yang sama, *when* percobaan ke-6 dilakukan, *then* login ditolak dengan `ACCOUNT_LOCKED` + waktu tunggu, dan kejadian tercatat.
- *Given* kasir tanpa izin `outlet_switch.approve` di layar kasir, *when* ia memilih outlet lain tanpa PIN penyetuju yang punya akses ke outlet tujuan, *then* pindah outlet ditolak dan sesi tidak berubah.

### 7.2 Master Data (MD)

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-MD-01 | CRUD barang: kode (unik per tenant, otomatis bila kosong), barcode, nama, satuan dasar, kategori, brand, principal, supplier, jenis (barang/jasa), berat, boleh stok minus, boleh jual di bawah HPP, keterangan (markdown). Hapus = arsip. | M | ✅ |
| FR-MD-01a | **Barcode boleh sama antar barang** (barcode pabrik tidak dijamin unik). Barang dapat diberi pembeda (mis. negara asal). Saat dipindai dan ditemukan > 1 barang, kasir memilih. Barcode ganda **dalam satu barang** dilarang. Form memperingatkan barcode kembar dan meminta konfirmasi. (Keputusan 2026-10-08.) | M | ✅ |
| FR-MD-02 | Harga jual **default per tenant** + harga khusus **per cabang**. Cabang tanpa harga khusus memakai default. (Keputusan, Q8.) | M | ✅ |
| FR-MD-03 | **Harga grosir** bertingkat: tier = "mulai qty X (satuan dasar), harga Y". Harga tier berlaku untuk **seluruh qty** (bukan potongan bertingkat). Qty dijumlahkan per barang di seluruh nota. Harga tidak boleh naik pada tier yang lebih besar; maks 10 tier. Set tier khusus cabang **menggantikan penuh** set default. | M | ✅ |
| FR-MD-03a | **Satuan tambahan** (kemasan jual, mis. Dus = 12 Pcs): faktor ke satuan dasar, barcode sendiri, harga sendiri opsional (kosong = faktor × harga dasar; satuan berharga manual tidak memakai grosir). Stok tetap satu, dalam satuan dasar. Maks 5 per barang. Bila kemasan perlu stok terpisah, dibuat barang baru + pecah satuan (FR-INV-09). | M | ✅ |
| FR-MD-04 | Diskon per barang dengan periode berlaku. | S | ○ |
| FR-MD-04a | **Kupon belanja**: kode unik per tenant, persen (dengan batas maksimum) atau nominal, minimal belanja, masa berlaku, kuota pemakaian. Satu nota boleh memakai maks 5 kupon, boleh digabung dengan tukar poin. | S | ✅ |
| FR-MD-05 | Varian barang dan tambahan per item (mis. topping resto). | S (R3) | ○ |
| FR-MD-06 | Master satuan, kategori, brand, principal, supplier, **salesman** (label, bukan akun; komisi disimpan, belum dihitung). | M | ✅ |
| FR-MD-06a | **Metode pembayaran** per tenant: nama bebas (mis. "QRIS BCA") + jenis dasar (tunai/transfer/debit/kartu kredit/e-wallet) + **biaya (MDR)** persen dan/atau nominal + penanggung biaya (**toko** atau **pelanggan**). Tunai bawaan tidak bisa diarsipkan dan tidak berbiaya. Nota menyimpan snapshot nama, jenis, dan tarif. | M | ✅ |
| FR-MD-07 | Member: kode, biodata, telepon (dinormalisasi `+62`), batas kredit, jatuh tempo (hari), masa berlaku, foto sampul, poin. **Level member** dihitung dari poin seumur hidup (tidak disimpan). Setiap level punya aturan "belanja per poin" dan nilai tukar poin. | M | ✅ |
| FR-MD-07a | **Deposit member** sebagai ledger sendiri (top-up, tarik, pakai bayar nota/piutang, dana retur). Dipakai lewat metode bayar sistem `deposit`. | S | ✅ (saldo awal deposit & laporan saldo belum) |
| FR-MD-08 | Setiap perubahan harga dan atribut penting barang tercatat di audit log. Riwayat harga jual per barang dan lintas barang dapat dilihat. | M | ✅ |
| FR-MD-09 | Ekspor barang ke Excel/CSV (import: lihat FR-ONB-01). | S | ○ |
| FR-MD-10 | Cetak label barcode/harga. | C | ○ |
| FR-MD-11 | **Gambar barang**: maks 5 per barang, 1 utama; JPEG/PNG/WebP maks 10 MB, diperkecil otomatis (1200 px + thumbnail 320 px), metadata lokasi dibuang. | S | ✅ |

### 7.3 Stok & Inventori (INV)

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-INV-01 | Stok dicatat per **tenant × outlet × barang × lokasi**, dengan 3 lokasi: **Display**, **Gudang**, **Retur**. Satuan stok = satuan dasar barang. | M | ✅ |
| FR-INV-02 | Setiap perubahan stok dicatat sebagai *movement* yang tidak dapat diubah atau dihapus (append-only), merujuk dokumen sumbernya. Koreksi dicatat sebagai movement baru. | M | ✅ |
| FR-INV-03 | Saldo stok selalu sama dengan jumlah seluruh movement-nya. Ini dijaga dalam transaksi yang sama dan diverifikasi oleh job rekonsiliasi harian. | M | ◐ (job harian belum) |
| FR-INV-04 | Penjualan mengurangi stok **Display**. Bila stok tidak cukup → transaksi ditolak dengan pesan jelas. **Stok minus hanya boleh untuk barang bertanda "boleh minus" dan hanya di Display**; Gudang dan Retur tidak pernah minus. | M | ✅ |
| FR-INV-05 | Barang jenis jasa tidak memengaruhi stok. | M | ✅ |
| FR-INV-06 | Kartu stok per barang/outlet/periode/lokasi: saldo awal, masuk, keluar, saldo berjalan, dan nomor dokumen per baris. Periode maks 366 hari, menurut zona waktu outlet. | M | ✅ |
| FR-INV-07 | **Stok opname**: (a) sesi draf → pilih barang (manual/semua/kategori/brand) → isi hitung fisik → selesaikan (izin *setujui*). Selisih dihitung terhadap snapshot saat barang ditambahkan, sehingga penjualan di tengah hitungan tidak tertimpa. (b) **Opname langsung** mode *ganti* atau *tambah/kurang* untuk koreksi cepat. | M | ✅ |
| FR-INV-08 | **Mutasi stok** antar lokasi dan antar cabang dalam satu tenant, **dua tahap kirim → terima**. Barang yang diterima kurang dicatat sebagai selisih. Mutasi dapat dibatalkan selama belum diterima. Mutasi antar lokasi di cabang yang sama langsung diterima. | M | ✅ |
| FR-INV-09 | **Pecah satuan** (mis. 1 karung → 12 pcs) antar dua barang berbeda sebagai dua movement seimbang dengan qty tujuan diisi bebas (susut terwakili). | M | ✅ |
| FR-INV-10 | Peringatan stok minimum per barang/outlet. | C | ○ |
| FR-INV-11 | **HPP per cabang**, metode **rata-rata tertimbang** (keputusan 2026-10-09; rumus di FR-PUR-03). Mutasi masuk memperbarui HPP cabang tujuan dengan rumus yang sama. | M | ✅ |
| FR-INV-12 | Saldo awal stok dan kunci tanggal mulai operasional (lihat FR-ONB-03, FR-ONB-07). | M | ✅ (manual) |

**Kriteria penerimaan:**
- *Given* stok Display barang X = 1 dan barang tidak boleh minus, *when* dua kasir (web atau mobile) menjual 1 unit barang X pada saat bersamaan, *then* tepat satu transaksi berhasil, yang lain ditolak "stok tidak cukup", dan saldo akhir = 0.
- *Given* sesi opname barang X dengan snapshot 10 dan hasil hitung 8, *when* terjadi penjualan 3 unit sebelum sesi diselesaikan, *then* saat selesai stok menjadi 10 − 3 − 2 = 5 (selisih −2 diterapkan pada saldo terkini).

### 7.4 Kasir / Penjualan (POS)

Berlaku untuk **kasir web dan Kasir Mobile**.

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-POS-01 | Tambah barang lewat scan barcode (barang atau satuan tambahan), ketik kode, atau cari nama (hasil < 150 ms). Barcode kembar → kasir memilih. Kolom QTY sebelum scan. | M | ✅ |
| FR-POS-02 | Ubah qty, hapus baris, catatan per item. Operasi utama dapat dilakukan dengan keyboard; daftar shortcut terdokumentasi. | M | ◐ (shortcut lengkap + bantuan F1 selesai 2026-10-10; catatan per baris belum ada di layar kasir — kolom `sale_lines.note` sudah ada) |
| FR-POS-03 | **Harga, grosir, potongan, kupon, poin, dan pajak dihitung server.** Klien hanya menampilkan pratinjau dari endpoint hitung (`quote`). Keranjang menandai lebih awal baris yang stoknya kurang atau di bawah HPP. | M | ✅ |
| FR-POS-04 | Ubah harga per baris hanya dengan persetujuan penyetuju + PIN; harga di bawah HPP tetap ditolak walau disetujui (kecuali barang "boleh di bawah HPP"); tercatat di audit (harga daftar → harga baru, penyetuju). | M | ✅ |
| FR-POS-05 | Potongan per baris, potongan nota, pajak toko & pajak negara (tarif per outlet, dapat dimatikan per nota), dan **biaya lain-lain dengan rincian** (nama + jumlah, maks 20). | M | ✅ |
| FR-POS-06 | Pembayaran **campuran** (split) memakai metode dari master (FR-MD-06a); setiap metode dipakai sekali per nota. Non-tunai tidak boleh melebihi total; kembalian hanya dari tunai. | M | ✅ |
| FR-POS-07 | **Kredit hanya untuk member** (pelanggan umum ditolak). Sisa setelah DP menjadi piutang dengan jatuh tempo = hari nota + hari jatuh tempo member (0 = tanpa jatuh tempo). Melewati limit kredit member hanya dengan persetujuan + PIN (izin `credit_limit.approve`). Limit 0 = tanpa batas. (Keputusan 2026-10-09.) | M | ✅ |
| FR-POS-08 | Uang muka (DP) pada nota kredit memakai metode bayar biasa. | S | ✅ |
| FR-POS-09 | **Nota pending** (tunda) dengan keterangan, lalu lanjutkan; membuka pending saat keranjang berisi = tukar tempat. Disimpan di perangkat, maks 20, kedaluwarsa 24 jam; harga dihitung ulang saat dibuka. | M | ✅ (lokal perangkat) |
| FR-POS-10 | Nomor nota unik: `{KODE-OUTLET}-{YYMMDD}-{NNNN}`, urut per outlet per hari menurut zona waktu outlet, tanpa celah. Revisi memakai akhiran `-R2`, `-R3`. (Keputusan 2026-10-08, Q4.) | M | ✅ |
| FR-POS-11 | Penyimpanan transaksi **idempoten**: kirim ulang (klik ganda, jaringan putus, aplikasi ditutup paksa) dengan kunci yang sama mengembalikan nota yang sama. Kunci sama + isi berbeda → ditolak. | M | ✅ |
| FR-POS-12 | Poin member = `floor((subtotal − seluruh potongan) ÷ belanja-per-poin level)`. Tukar poin = potongan nota (poin × nilai poin). Edit/void menyesuaikan poin sebesar **selisih**, bukan menambah ulang. | M | ✅ |
| FR-POS-13 | **Edit nota = revisi** (keputusan 2026-10-08): nota lama menjadi *superseded*, nota revisi baru dibuat dan tertaut. Stok, poin, dan kupon dibalik lalu diterapkan ulang dalam satu transaksi. Harga dan HPP baris lama dipertahankan. Butuh izin + PIN penyetuju + alasan; hanya dalam batas hari edit tenant (bawaan: hari yang sama; diatur operator platform). **Nota yang piutangnya sudah dibayar (sebagian/seluruh) tidak dapat diedit atau dibatalkan** (keputusan 2026-10-09, menggantikan v0.2). | M | ✅ |
| FR-POS-14 | Void/batal nota: tidak menghapus data. Status *void* + alasan; stok kembali lewat movement; poin dan kupon dikembalikan. | M | ✅ |
| FR-POS-15 | **Cetak struk** otomatis/manual. Server menyusun **satu model struk** (header toko, baris, potongan, pajak, biaya, pembayaran, biaya metode yang dibebankan ke pelanggan, kembalian, piutang, poin, catatan kaki). Model ini dipakai oleh print-agent PC **dan** Kasir Mobile sehingga isinya identik. Cetak ulang tercatat. | M | ✅ (browser + print-agent ESC/POS 58/80 mm, diuji di printer fisik; logo belum) |
| FR-POS-16 | Daftar penjualan hari ini **per kasir** (untuk mencocokkan uang laci), dipecah per metode. **Shift kasir** (keputusan 2026-10-10): buka shift wajib dengan modal awal; tutup shift dengan hitung fisik per metode; selisih ≠ 0 wajib catatan + PIN penyetuju (`shift_close.approve`); rekap dibekukan; struk tutup shift + cetak rekap harian. | S | ✅ (edit/batal tutup shift belum ada) |
| FR-POS-17 | Salesman per transaksi (opsional). | S | ✅ |
| FR-POS-18 | Mode offline (lihat §7.11 FR-MOB-15 dan Q2). | **[KONFIRMASI]** | ○ |
| FR-POS-19 | Kupon belanja di nota (FR-MD-04a): semua kupon dihitung dari dasar yang sama (urutan tidak berpengaruh); kuota dipakai secara atomik saat nota disimpan. | S | ✅ |
| FR-POS-20 | **Biaya metode bayar**: ditanggung toko → dicatat sebagai biaya, total nota tetap. Ditanggung pelanggan → menjadi tagihan tambahan di luar total nota; pelanggan membayar total + biaya, tanpa pajak atas biaya tersebut. (Keputusan 2026-10-08.) | M | ✅ |
| FR-POS-21 | 16 slot **pintasan barang** per kasir (sinkron di server). | C | ✅ |
| FR-POS-22 | Nota maks 500 baris. | M | ✅ |

**Urutan perhitungan nota (keputusan 2026-10-08; Q6 sebagian):**
1. Harga satuan per baris = harga cabang (atau default) → harga grosir bila total qty dasar barang itu memenuhi tier → atau harga satuan tambahan.
2. Nilai baris = harga satuan × qty − potongan baris.
3. **Subtotal** = Σ nilai baris.
4. Dikurangi **potongan nota manual**.
5. Dikurangi **kupon**. Setiap kupon dihitung dari hasil langkah 4; persen dibulatkan 2 desimal lalu dibatasi nilai maksimumnya.
6. Dikurangi **tukar poin** (maks sebesar sisa setelah langkah 5).
7. **Pajak toko** dan **pajak negara** = tarif outlet × hasil langkah 6. Keduanya dihitung dari dasar yang sama (tidak bertingkat), hanya bila pajak aktif untuk nota itu.
8. Ditambah **biaya lain-lain** = **TOTAL NOTA**. **Tanpa pembulatan.**
9. **Ditagih ke pelanggan** = total + biaya metode bayar yang ditanggung pelanggan (FR-POS-20).
10. Kembalian = Σ pembayaran − total, hanya dari tunai. Pada nota kredit, sisa setelah DP = piutang.
11. Poin diperoleh = `floor(hasil langkah 6 ÷ belanja-per-poin)`. Pajak dan biaya lain tidak menghasilkan poin.

Nota yang setelah potongan/kupon/tukar poin jatuh di bawah HPP ditolak (kecuali barang "boleh di bawah HPP").

**Kriteria penerimaan:**
- *Given* kasir menekan "Bayar" lalu jaringan putus sebelum respons diterima, *when* aplikasi mengirim ulang dengan idempotency key yang sama, *then* server mengembalikan nota yang sama dan hanya ada satu nota di database.
- *Given* nota kredit Rp1.000.000 yang piutangnya sudah dibayar Rp400.000, *when* kasir mencoba mengedit atau membatalkan nota, *then* sistem menolak dengan `RECEIVABLE_PAID` dan tidak ada data yang berubah.
- *Given* limit kredit member Rp100.000 dan 8 nota kredit Rp30.000 disimpan bersamaan, *when* semuanya diproses, *then* tepat 3 nota berhasil dan sisanya ditolak `CREDIT_LIMIT_EXCEEDED`.
- *Given* setiap contoh kasus di spec test penjualan (`source/tests/spec/`), *when* dihitung oleh ARUS, *then* total, pajak, kembalian, perubahan stok, dan poin sama persis dengan nilai yang diharapkan.

### 7.5 Retur Penjualan & Piutang (AR)

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-AR-01 | Retur penjualan merujuk nota asal; qty retur ≤ qty terjual − retur sebelumnya. Nota asal tetap utuh. Retur dapat dibatalkan (alasan + izin) selama dana kembali belum diserahkan lewat metode biasa. | M | ✅ |
| FR-AR-02 | Barang retur **selalu** masuk ke lokasi **Retur** lewat movement `SALE_RETURN` (keputusan 2026-10-09; menggantikan pilihan Retur/Display). | M | ✅ |
| FR-AR-03 | Nilai retur memotong piutang nota dulu, sisanya dikembalikan lewat metode bayar (referensi wajib untuk non-tunai) atau ke **deposit member**. Biaya lain dan biaya metode yang dibebankan ke pelanggan tidak dikembalikan. Poin diperoleh dikurangi, poin yang ditukar dikembalikan (saldo boleh negatif). | M | ✅ |
| FR-AR-04 | Daftar piutang per member: status terbuka/lewat tempo/lunas, jatuh tempo, ringkasan. Aging per kelompok umur. | M | ◐ (aging berkelompok belum) |
| FR-AR-05 | Pembayaran piutang sebagian/penuh, idempoten, tidak boleh melebihi sisa. Sisa = total − Σ pembayaran (dihitung, bukan kolom yang dimutasi). | M | ✅ |
| FR-AR-06 | **Pelunasan kolektif** per member: otomatis ke nota terlama dulu, atau pilih nota sendiri. | S | ✅ |
| FR-AR-07 | Pembatalan/koreksi pembayaran piutang (dengan izin + alasan). Ini juga jalur koreksi bila nota yang sudah dicicil perlu diedit. | M | ○ |

### 7.5b Onboarding Toko (ONB)

Pengganti migrasi data. Dipakai untuk toko baru maupun toko pindahan dari legacy.

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-ONB-01 | Import barang dari Excel/CSV memakai template unduhan. Pratinjau menampilkan validasi per baris (kode duplikat, satuan/kategori belum ada, harga tidak valid); hanya baris valid yang disimpan; laporan error dapat diunduh. | M | ○ |
| FR-ONB-02 | Import master pendukung (kategori, satuan, supplier, member, metode bayar) dengan mekanisme yang sama. | S | ○ |
| FR-ONB-03 | Input **saldo awal stok** per outlet × barang × lokasi (manual atau import), dicatat sebagai movement `OPENING`. | M | ✅ manual / ○ import |
| FR-ONB-04 | Input **saldo awal piutang** per member (nomor referensi lama, nominal, jatuh tempo); dapat dibayar seperti piutang biasa; dapat dibatalkan selama belum dibayar. | M | ✅ manual / ○ import |
| FR-ONB-05 | Input **saldo awal hutang** per supplier. | M (R2) | ○ |
| FR-ONB-06 | Wizard setup tenant: profil usaha → outlet (pajak, zona waktu) → user & role → import barang → saldo awal → siap transaksi. | S | ○ |
| FR-ONB-07 | Saldo awal hanya dapat diinput/diubah sebelum "tanggal mulai operasional" dikunci. Setelah dikunci, perubahan stok hanya lewat opname. | M | ✅ |

**Kriteria penerimaan:**
- *Given* file import berisi 1.000 barang dengan 3 baris kode duplikat, *when* pengguna menekan "Import", *then* 997 barang tersimpan, 3 baris dilaporkan beserta alasannya, dan tidak ada barang yang tersimpan setengah.
- *Given* saldo awal stok barang X = 50 di Display outlet A, *when* kartu stok dibuka, *then* baris pertama adalah `OPENING` +50 dan saldo = 50.

### 7.6 Pembelian, Retur Beli & Hutang (PUR) — R2

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-PUR-01 | Pembelian langsung (tanpa PO): stok masuk ke Display dan/atau Gudang per baris. PO opsional → penerimaan bertahap; PO tidak menggerakkan stok. | M | ✅ langsung / ○ PO |
| FR-PUR-02 | Pembelian tunai/kredit; kredit membuat hutang dengan jatuh tempo opsional. Nomor faktur pemasok unik per pemasok. | M | ✅ |
| FR-PUR-03 | **HPP per cabang, rata-rata tertimbang** (keputusan 2026-10-09): `(stok_sebelum × HPP_lama + qty × HPP_baris) ÷ (stok_sebelum + qty)`. Stok sebelum = semua lokasi di cabang itu, dihitung **sebelum** barang masuk (legacy menghitung sesudahnya). Bila stok sebelum ≤ 0 atau HPP lama = 0 → HPP baru = HPP baris. HPP baris = harga setelah diskon + alokasi biaya lain nota (proporsional nilai baris). **PPN masukan dipisah dari HPP.** | M | ✅ |
| FR-PUR-04 | Retur pembelian **hanya dari lokasi Retur** (barang dimutasi dulu ke Retur), merujuk nota asal; qty ≤ dibeli − diretur; potong hutang, kelebihannya = dana kembali atau **kredit pemasok**. Biaya lain nota tidak dikembalikan. | M | ✅ |
| FR-PUR-05 | Pembayaran hutang sebagian/penuh, aging 4 kelompok, pelunasan kolektif per pemasok (otomatis terlama dulu atau pilih nota). | M | ✅ |
| FR-PUR-06 | Edit/batal pembelian dengan pola revisi (`-R2`) dan alasan. HPP dihitung mundur. Ditolak bila stok sudah terjual sehingga kurang, atau bila hutang sudah dibayar. | M | ✅ |
| FR-PUR-07 | Baris pembelian: diskon **4 tingkat bertingkat**, masing-masing persen (< 100) atau rupiah (≥ 100); harga beli sampai 4 desimal; sub total boleh diketik (harga = sub total ÷ qty); satuan pembelian = satuan dasar. Biaya lain dinamis di level nota. | M | ✅ |
| FR-PUR-08 | Riwayat harga beli lintas barang/pemasok dengan perubahan % terhadap pembelian sebelumnya. | S | ✅ |
| FR-PUR-09 | Pembatalan pembayaran hutang. | S | ○ |

### 7.7 Laporan (RPT)

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-RPT-01 | Penjualan per periode/outlet/kasir/metode bayar/barang/kategori/salesman. | M (R1 dasar) | ◐ (daftar penjualan + ringkasan per metode, rekap harian & shift; per barang/kategori/salesman belum) |
| FR-RPT-02 | Laba kotor (penjualan − potongan − HPP snapshot saat transaksi), hanya untuk pemegang izin `sales_cost`. Laba bersih setelah biaya metode bayar. | M | ◐ |
| FR-RPT-03 | Stok saat ini, kartu stok, nilai persediaan (stok × HPP cabang). | M | ◐ (kartu stok) |
| FR-RPT-04 | Pembelian, retur, piutang, hutang (+ aging). | M (R2) | ◐ |
| FR-RPT-05 | Ekspor Excel/PDF; laporan berat diproses di background dan diberi notifikasi saat selesai. | S | ○ |
| FR-RPT-06 | Dashboard owner: omzet hari ini, tren, barang terlaris, stok kritis. | S | ○ |
| FR-RPT-07 | Pemakaian kupon, komisi salesman, mutasi stok, selisih opname. | S | ○ |
| FR-RPT-08 | Semua laporan memisahkan sumber transaksi **web vs mobile** (perangkat), untuk mengukur adopsi Kasir Mobile. | S (RM) | ○ |

### 7.8 Restoran & KDS (RES) — R3

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-RES-01 | Denah dan status meja (kosong/terisi/dipesan). | M | ○ |
| FR-RES-02 | Pesanan dine-in/takeaway, gabung/pindah meja, reservasi dengan DP. | M | ○ |
| FR-RES-03 | KDS: antrean pesanan realtime per outlet; dapur mengubah status (diproses/siap/tersaji). | M | ○ |
| FR-RES-04 | Notifikasi realtime ke kasir/pelayan saat status berubah. | M | ○ |
| FR-RES-05 | Pelayan mengambil pesanan dari meja memakai Kasir Mobile (mode pesanan, tanpa pembayaran). | C | ○ |

### 7.9 Akuntansi SIAK (ACC) — R4

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-ACC-01 | Bagan akun (COA) per tenant; periode akuntansi dengan tutup buku. | S | ✅ |
| FR-ACC-02 | Jurnal otomatis dari penjualan, pembelian, pembayaran piutang/hutang. Pemetaan akun dikonfigurasi per **metode bayar** (bukan per jenis). | S | ○ |
| FR-ACC-03 | Jurnal manual, buku besar, kas/bank, neraca saldo, neraca, laba rugi. | S | ○ |
| FR-ACC-04 | Jurnal manual: umum (JU), kas masuk/keluar (KM/KK), transfer kas (TK). Alur draf → posting; jurnal terposting tidak bisa diubah/dihapus, koreksi lewat jurnal balik. Posting ditolak bila debit ≠ kredit atau tanggal di periode tertutup. | S | ✅ |
| FR-ACC-05 | Saldo awal akun lewat jurnal pembuka (wajib seimbang), bukan kolom di COA. Template COA retail bawaan saat tenant mengaktifkan akuntansi. | S | ✅ |
| FR-ACC-06 | Jurnal penjualan otomatis diringkas per outlet per hari (idempoten, bisa ditelusuri ke nota); pembelian/piutang/hutang/retur per dokumen. Desain: `docs/ARCHITECTURE.md` §4b. | S | ○ |

*Given* jurnal Rp 100.000 debit dan Rp 90.000 kredit, *when* diposting, *then* ditolak `JOURNAL_UNBALANCED`. *Given* periode Oktober ditutup, *when* jurnal bertanggal 15 Okt diposting, *then* ditolak `PERIOD_CLOSED`.

### 7.10 Platform & Ekstensi (PLT) — R4

| ID | Kebutuhan | Prioritas | Status |
|---|---|---|---|
| FR-PLT-01 | Platform admin: kelola tenant, status aktif, batas hari edit nota, admin lain (lihat FR-AUTH-13). Paket/lisensi/trial menyusul. | M (R0 minimal) | ◐ |
| FR-PLT-02 | Acipay/PPOB (Digiflazz): katalog produk digital, transaksi, deposit, webhook dengan verifikasi signature. | C | ○ |
| FR-PLT-03 | Payment gateway (Duitku/Tripay/Midtrans) untuk pembayaran non-tunai (QRIS dinamis). | C | ○ |
| FR-PLT-04 | Notifikasi WhatsApp/email (struk digital, pengingat piutang). | C | ○ |
| FR-PLT-05 [USULAN] | Pengaturan versi minimum aplikasi mobile per platform (paksa update) dan saklar fitur mobile per tenant. | M (RM) | ○ |

### 7.11 Kasir Mobile (MOB) — [USULAN]

#### 7.11.1 Tujuan & batasan

- **Satu fungsi saja: berjualan.** Mobile memakai API, izin, aturan hitung, nomor nota, dan idempotensi yang **sama** dengan kasir web. Setiap aturan FR-POS berlaku tanpa pengecualian.
- **Bukan back-office.** Master barang, pembelian, opname, dan laporan lengkap tetap di web (web sudah dapat dibuka di HP bila perlu).
- **Platform awal: Android** (mayoritas perangkat toko kecil dan POS genggam di Indonesia) **[KONFIRMASI Q11, Q16]**.
- **Online dulu.** Rilis pertama butuh internet, tetapi tahan sinyal buruk (FR-MOB-12). Offline penuh adalah keputusan terpisah (FR-MOB-15, Q13) karena mengubah aturan "harga & stok dihitung server".

#### 7.11.2 Perangkat sasaran **[KONFIRMASI Q12]**

| Kelas | Contoh | Cetak | Scan | Prioritas |
|---|---|---|---|---|
| HP Android biasa | Android 8.0+, RAM ≥ 3 GB, layar 5,5–6,7" | Printer thermal Bluetooth 58/80 mm (ESC/POS) | Kamera | M (M1) |
| Perangkat POS genggam | Sunmi V2/V2s, iMin, sejenisnya | Printer bawaan (SDK vendor) | Kamera / scanner bawaan | S (M2) |
| Tablet Android | 8–11", lanskap | Bluetooth / jaringan | Kamera / scanner HID Bluetooth | S (M2) |
| iPhone/iPad | — | Bluetooth (terbatas) | Kamera | C (M4) |

#### 7.11.3 Kebutuhan

| ID | Kebutuhan | Prioritas | Tahap |
|---|---|---|---|
| FR-MOB-01 | **Masuk** dengan email + password akun ARUS yang sama. Hanya pengguna dengan izin kasir (`sales_orders.create`) yang dapat memakai aplikasi; pengguna lain mendapat pesan jelas. Perangkat tetap masuk antar pemakaian (FR-MOB-16). | M | M1 |
| FR-MOB-02 | **Kunci layar cepat**: setelah tidak aktif N menit (diatur tenant, bawaan 5) atau saat aplikasi kembali dari latar belakang, aplikasi meminta PIN/biometrik perangkat. Pergantian kasir di perangkat yang sama = keluar lalu masuk dengan akun kasir lain (tidak berbagi akun). **[KONFIRMASI Q14]** | M | M1 |
| FR-MOB-03 | **Outlet aktif** ditampilkan jelas di header. Pindah outlet mengikuti FR-AUTH-03 (PIN penyetuju, keranjang dikosongkan). | M | M1 |
| FR-MOB-04 | **Katalog jual**: cari nama/kode, filter kategori, kisi barang dengan gambar thumbnail, harga efektif outlet, dan stok Display. Pintasan barang (FR-POS-21) sama dengan kasir web milik pengguna itu. | M | M1 |
| FR-MOB-05 | **Scan kamera** barcode 1D umum (EAN-13, EAN-8, UPC-A, Code 128, Code 39) dan QR, secara berturut-turut tanpa menutup kamera. Bunyi/getar saat berhasil; jeda anti-scan-ganda untuk kode yang sama ≥ 1 detik (dapat diatur). Barcode kembar → pilih barang (FR-MD-01a). Kode tidak dikenal → pesan + pindah ke pencarian. Senter dapat dinyalakan. | M | M1 |
| FR-MOB-06 | **Scanner eksternal**: scanner Bluetooth/USB mode keyboard (HID) dan scanner bawaan perangkat POS dikenali sebagai input scan tanpa membuka kamera. | S | M2 |
| FR-MOB-07 | **Keranjang**: ubah qty (tombol +/− dan ketik, termasuk desimal untuk barang timbang), hapus baris (geser), pilih satuan tambahan, catatan baris, member, kupon, tukar poin, salesman, potongan, biaya lain, pajak aktif/tidak, keterangan nota. Total, pajak, dan peringatan baris (stok kurang / di bawah HPP) **berasal dari quote server**. Tombol Bayar nonaktif selama quote belum mutakhir. | M | M1 |
| FR-MOB-08 | **Bayar**: tunai dengan tombol nominal cepat (uang pas, 50rb, 100rb, dan pecahan di atas total), non-tunai (pilih metode dari master), split, dan kredit member + DP. Biaya metode yang ditanggung pelanggan ditampilkan sebelum konfirmasi (FR-POS-20). Persetujuan PIN (ubah harga, melewati limit kredit) dimasukkan penyetuju di perangkat yang sama (FR-AUTH-06). | M | M1 |
| FR-MOB-09 | **Cetak struk ke printer thermal Bluetooth** 58 mm/80 mm (ESC/POS) memakai model struk server (FR-POS-15). Printer dipasangkan sekali per perangkat dan diingat; cetak otomatis setelah bayar (dapat dimatikan) + cetak ulang. **Gagal cetak tidak membatalkan nota**: nota tetap tersimpan dan antre cetak ulang. Dukungan logo dan buka laci uang (bila printer mendukung) = C. | M | M1 |
| FR-MOB-09b | Cetak ke **printer bawaan perangkat POS** (SDK vendor) untuk model yang disetujui (Q12). | S | M2 |
| FR-MOB-10 | **Struk digital**: bagikan struk sebagai gambar/PDF lewat menu berbagi Android (WhatsApp, dll.), tanpa gateway WhatsApp. | S | M1 |
| FR-MOB-11 | **Penjualan hari ini** milik kasir itu (FR-POS-16): daftar nota, total per metode, detail nota, cetak ulang. Edit/void nota **tidak** tersedia di mobile pada M1 (tetap di web). | M | M1 |
| FR-MOB-12 | **Tahan sinyal buruk**: (a) kunci idempotensi dibuat sebelum kirim dan **disimpan di perangkat** sampai server menjawab; (b) bila koneksi putus atau aplikasi ditutup paksa saat menyimpan, saat dibuka kembali aplikasi menampilkan "Nota menunggu konfirmasi" dan mengirim ulang dengan kunci yang sama **sampai jelas berhasil atau ditolak**, tanpa pernah membuat nota kedua; (c) indikator status koneksi selalu terlihat; (d) keranjang tersimpan di perangkat sehingga tidak hilang saat aplikasi ditutup. | M | M1 |
| FR-MOB-13 | **Nota pending** (FR-POS-09) tersimpan di perangkat. Pending tidak berpindah antar perangkat. | S | M1 |
| FR-MOB-14 | **Bahasa** Indonesia (sumber) dan Inggris, memakai kamus yang sama dengan web. Tema terang/gelap mengikuti sistem. | M | M1 |
| FR-MOB-15 | **Mode offline** (hanya bila Q13 = ya): jualan dengan katalog & harga dari cache terakhir; nota offline diberi kunci idempotensi dan **nomor sementara**, lalu dikirim saat online dan mendapat nomor resmi dari server. Aturan yang **wajib diputuskan sebelum dibangun**: (a) batas umur cache harga (mis. 24 jam); (b) nota yang ditolak server saat sinkron (stok kurang, harga berubah, kupon habis): diterima dengan stok minus + ditandai untuk ditinjau, atau ditolak dan diselesaikan manual; (c) metode yang boleh offline (tunai saja? tanpa kredit, tanpa kupon, tanpa ubah harga); (d) batas jumlah/nilai nota offline per perangkat. | **[KONFIRMASI]** | M3 |
| FR-MOB-16 | **Manajemen perangkat**: setiap pemasangan aplikasi terdaftar (nama perangkat, model, versi aplikasi, pengguna, outlet, terakhir aktif). Owner/admin dapat mencabut satu perangkat dari web; perangkat yang dicabut keluar pada permintaan berikutnya. Menonaktifkan pengguna mencabut semua perangkatnya. | M | M1 |
| FR-MOB-17 | **Versi minimum**: bila versi aplikasi di bawah minimum yang ditetapkan platform (FR-PLT-05), aplikasi meminta pembaruan dan tidak mengizinkan transaksi baru. Nota yang menunggu konfirmasi tetap dikirim lebih dulu. | M | M1 |
| FR-MOB-18 | **Tutup kasir dari HP**: rekap per metode untuk shift berjalan + input uang fisik + selisih (mengikuti FR-POS-16 saat dibangun). | S | M2 |
| FR-MOB-19 | **Jejak sumber**: setiap nota mencatat kanal (`web`/`mobile`) dan perangkat pembuatnya; terlihat di detail nota dan audit. | M | M1 |
| FR-MOB-20 | Tata letak **tablet lanskap** (katalog kiri, keranjang kanan) dan **ponsel potret** (katalog dan keranjang sebagai dua layar). | M (ponsel) / S (tablet) | M1 / M2 |

#### 7.11.4 Kriteria penerimaan

- *Given* kasir mobile menekan "Bayar" dan sinyal hilang sebelum respons diterima, *when* aplikasi ditutup paksa lalu dibuka lagi saat sinyal kembali, *then* aplikasi menampilkan nota menunggu konfirmasi, mengirim ulang dengan kunci yang sama, menampilkan nomor nota yang sama, dan di server hanya ada **satu** nota.
- *Given* printer Bluetooth mati saat pembayaran selesai, *when* nota berhasil disimpan, *then* layar menampilkan nomor nota + "Struk belum tercetak", dan kasir dapat mencetak ulang dari Penjualan Hari Ini setelah printer menyala.
- *Given* barcode `1111` dipakai dua barang (asal CC dan DD), *when* dipindai dengan kamera, *then* muncul pilihan dua barang beserta pembedanya, dan tidak ada barang yang masuk keranjang sebelum dipilih.
- *Given* kasir web dan kasir mobile menjual barang X (stok 1, tidak boleh minus) bersamaan, *when* keduanya menyimpan, *then* tepat satu berhasil (sama dengan FR-INV-04).
- *Given* owner mencabut perangkat P dari web, *when* perangkat P mengirim permintaan berikutnya, *then* permintaan ditolak 401, aplikasi kembali ke layar masuk, dan keranjang lokal dibuang.
- *Given* quote dan simpan nota untuk keranjang yang sama, *when* dibuat dari mobile dan web, *then* total, pajak, dan poin identik sampai sen (aturan tunggal di server).

---

## 8. Kebutuhan Non-Fungsional

### 8.1 Keamanan (SEC)

| ID | Kebutuhan |
|---|---|
| NFR-SEC-01 | Seluruh endpoint (kecuali login, health, webhook bertanda tangan) wajib terautentikasi. |
| NFR-SEC-02 | Isolasi tenant berlapis: filter di service **dan** Row Level Security PostgreSQL (aplikasi memakai role DB tanpa hak melewati RLS). Isolasi cabang dijaga di service. |
| NFR-SEC-03 | Seluruh query memakai parameter binding; tidak ada SQL dari konkatenasi string. |
| NFR-SEC-04 | Password & PIN di-hash (argon2id); hash tidak pernah dikirim ke klien. |
| NFR-SEC-05 | Secret hanya dari environment/secret manager; tidak ada secret di repository **atau di dalam paket aplikasi mobile**. |
| NFR-SEC-06 | CORS dibatasi ke origin aplikasi; rate limit per IP dan per pengguna pada login & endpoint sensitif. |
| NFR-SEC-07 | HTTPS wajib di produksi; cookie refresh token web `HttpOnly`, `Secure`, `SameSite`. |
| NFR-SEC-08 | Pentest/security review sebelum go-live tenant pilot **dan** sebelum rilis publik aplikasi mobile. |
| NFR-SEC-09 | Pemrosesan data pribadi (nama, telepon, email member) mematuhi **UU No. 27/2022 tentang Pelindungan Data Pribadi**: minimisasi data, akses berbasis peran, data uji dianonimkan. |
| NFR-SEC-10 [USULAN] | **Mobile:** refresh token disimpan di penyimpanan aman yang didukung Android Keystore (bukan SharedPreferences/localStorage biasa), terikat ke ID perangkat terdaftar, dan dapat dicabut per perangkat. Tidak memakai cookie lintas origin. |
| NFR-SEC-11 [USULAN] | **Mobile:** data lokal (keranjang, pending, cache katalog, antrean nota) dibuang saat keluar atau saat perangkat dicabut. Tidak ada PIN penyetuju yang disimpan di perangkat (hanya di memori selama satu nota). |
| NFR-SEC-12 [USULAN] | **Mobile:** koneksi hanya HTTPS. Pertimbangkan *certificate pinning* (C) dengan rencana rotasi agar tidak mengunci pengguna saat sertifikat diganti. |

### 8.2 Integritas Data (DATA)

| ID | Kebutuhan |
|---|---|
| NFR-DATA-01 | Setiap use-case yang menulis data berjalan dalam **satu transaksi database**. |
| NFR-DATA-02 | Nilai uang `NUMERIC(18,2)`, kuantitas `NUMERIC(18,3)`, harga beli sampai 4 desimal; tidak ada floating point dalam perhitungan uang di server. Pratinjau klien (web/mobile) juga tanpa float (integer sen). |
| NFR-DATA-03 | Kunci bisnis dijamin constraint database (unik per tenant, foreign key). |
| NFR-DATA-04 | Dokumen transaksi tidak dihapus fisik; pembatalan memakai status + movement balik; ledger (stok, poin, pembayaran) append-only. |
| NFR-DATA-05 | Efek samping eksternal (notifikasi, cetak, webhook) dikirim **setelah commit**. |
| NFR-DATA-06 | Nilai turunan (HPP, saldo) dihitung setelah kunci yang benar-benar menyerialkan penulis lain didapat; diuji dengan test konkuren yang memeriksa nilai turunan, bukan hanya stok. |

### 8.3 Performa & Skalabilitas (PERF)

| ID | Kebutuhan |
|---|---|
| NFR-PERF-01 | Target latensi sesuai M1–M2. |
| NFR-PERF-02 | Mendukung ≥ 50 kasir aktif bersamaan per instance (web + mobile) tanpa degradasi (target awal) **[KONFIRMASI Q7]**. Nomor nota tetap unik dan tanpa celah dengan 12 kasir bersamaan di satu outlet (sudah diuji). |
| NFR-PERF-03 | Daftar transaksi memakai paginasi keyset; pencarian teks memakai indeks trigram. Tabel movement dipartisi per bulan bila volume menuntut; laporan berat tidak memblokir transaksi kasir. |
| NFR-PERF-04 | Halaman kasir web dimuat < 2 detik pada koneksi 1 Mbps (aset statis di-cache). |
| NFR-PERF-05 [USULAN] | **Mobile:** ukuran unduhan aplikasi ≤ 30 MB; cold start ≤ 3 detik (M13); quote berikutnya tidak memblokir input (scan tetap jalan selama quote diproses); pemakaian data seluler wajar (gambar thumbnail di-cache, bukan gambar penuh). |

### 8.4 Ketersediaan & Pemulihan (OPS)

| ID | Kebutuhan |
|---|---|
| NFR-OPS-01 | Ketersediaan ≥ 99,5% di jam operasional. |
| NFR-OPS-02 | Backup database harian + WAL archiving; **RPO ≤ 15 menit, RTO ≤ 2 jam**. Salinan di luar server; prosedur restore diuji minimal per bulan. |
| NFR-OPS-03 | Log terstruktur (JSON) dengan request ID; metrik & alert untuk error rate, latensi, dan koneksi DB. |
| NFR-OPS-04 | Deployment dengan migrasi skema otomatis; build dan update terdokumentasi (`source/deploy/`). |
| NFR-OPS-05 [USULAN] | **Mobile:** API bersifat **kompatibel ke belakang** minimal untuk 2 versi aplikasi terakhir (perubahan yang memecah kompatibilitas harus lewat versi minimum FR-MOB-17). Laporan crash dan versi aplikasi per perangkat terpantau. Distribusi lewat Google Play (track internal → tertutup → produksi) **[KONFIRMASI Q17]**. |

### 8.5 Usability & Lokalisasi (UX)

| ID | Kebutuhan |
|---|---|
| NFR-UX-01 | Antarmuka multi-bahasa (Indonesia = sumber, Inggris); format Rupiah (`Rp1.234.567`) dan tanggal `dd/mm/yyyy`. |
| NFR-UX-02 | Zona waktu per outlet (WIB/WITA/WIT); penyimpanan dalam UTC (`timestamptz`). Hari bisnis (nomor nota, laporan harian) mengikuti zona waktu outlet. |
| NFR-UX-03 | Kasir web dapat dioperasikan penuh dengan keyboard + scanner. |
| NFR-UX-04 | Desain konsisten mengikuti template Dreams Core; mendukung mode terang/gelap; animasi menghormati `prefers-reduced-motion`. |
| NFR-UX-05 | Browser: Chrome/Edge 2 versi terakhir, Firefox terbaru. |
| NFR-UX-06 [USULAN] | **Mobile:** target sentuh ≥ 48 dp; dapat dipakai satu tangan pada ponsel (tombol Bayar di area jempol); terbaca di bawah sinar matahari (kontras tinggi); umpan balik scan dengan bunyi dan getar yang dapat dimatikan. Android 8.0 (API 26) ke atas **[KONFIRMASI Q12]**. |

### 8.6 Kualitas Kode (QA)

| ID | Kebutuhan |
|---|---|
| NFR-QA-01 | Setiap modul memiliki unit test untuk invariant bisnis; test konkurensi untuk stok, penomoran nota, limit kredit, dan kuota kupon. |
| NFR-QA-02 | Spec test (contoh kasus perhitungan) penjualan, retur, piutang, dan onboarding berjalan otomatis di CI. |
| NFR-QA-03 | Lint & test wajib lulus sebelum merge. |
| NFR-QA-04 [USULAN] | **Mobile:** uji di perangkat acuan (≥ 1 HP kelas menengah, ≥ 1 HP kelas bawah, 1 perangkat POS genggam bila M2) dan ≥ 2 model printer Bluetooth yang dipakai klien. Skenario wajib: putus sinyal saat simpan, aplikasi ditutup paksa, printer mati, perangkat dicabut. |

---

## 9. Arsitektur Solusi (Ringkas)

```
┌───────────────────┐
│ SvelteKit SPA     │── HTTPS/JSON ──┐
│ (browser PC/HP)   │                │      ┌───────────────────────────┐     ┌─────────────┐
└────────┬──────────┘                ├────► │  Go API (modular monolith)│ ──► │ PostgreSQL  │
         │ localhost                 │      │  auth · tenant · catalog  │     │ (sumber     │
┌────────▼──────────┐                │      │  stock · sales · purchase │     │  kebenaran) │
│ Print-agent (Go,  │                │      │  receipt · report · ...   │     └─────────────┘
│ PC kasir, ESC/POS)│                │      └──────────┬────────────────┘     ┌─────────────┐
└───────────────────┘                │                 └────────────────────► │ Redis       │
┌───────────────────┐                │                                        │ cache · pub/│
│ Kasir Mobile      │── HTTPS/JSON ──┘                                        │ sub · queue │
│ (Android) [USULAN]│── Bluetooth ──► Printer thermal / printer bawaan        └─────────────┘
└───────────────────┘
```

**Prinsip arsitektur Kasir Mobile [USULAN] (rinci di `AGENTS.md` saat disetujui):**
1. **Satu sumber aturan.** Mobile memanggil endpoint yang sama (`/sales/quote`, `/sales/`, `/catalog/...`). Tidak ada endpoint "khusus mobile" yang menghitung ulang.
2. **Pakai ulang kode web bila memungkinkan.** Opsi yang direkomendasikan adalah membungkus SPA SvelteKit yang sudah ada dengan **Capacitor**, ditambah plugin native untuk kamera/scan, Bluetooth printer, penyimpanan aman, dan SDK printer bawaan (alasan dan alternatif: §15.4).
3. **Perubahan backend yang dibutuhkan:** (a) alur sesi tanpa cookie untuk aplikasi native (refresh token di body, terikat perangkat), karena cookie `SameSite=Lax` + cek Origin milik web tidak cocok untuk origin aplikasi; (b) tabel perangkat + pencabutan per perangkat; (c) kanal/perangkat pada nota (FR-MOB-19); (d) endpoint model struk (FR-POS-15); (e) endpoint versi minimum (FR-PLT-05).
4. **Model struk dibuat sekali di server** dan dirender ke ESC/POS oleh print-agent (PC) maupun aplikasi (Bluetooth), agar isi struk tidak pernah berbeda antar kanal.

Detail keputusan teknis (library, struktur folder, konvensi) ada di `AGENTS.md` §1, §3, §5, §6.

---

## 10. Model Data Inti (Ringkas)

| Entitas | Keterangan | Padanan di legacy (referensi istilah saja, data tidak dimigrasi) |
|---|---|---|
| Tenant | Usaha/pelanggan ACIRABA | `KODEUNIKMEMBER` |
| Outlet | Cabang/toko | `01_set_outlet` |
| User, Role, User–Outlet | Pegawai, hak akses, akses cabang | `01_tms_penggunaaplikasi`, `01_tms_penggunaaplikasiha` |
| Item, Outlet price, Wholesale tier, Item unit, Item image | Barang, harga, kemasan, gambar | `01_tms_barangkharisma`, `01_tms_bestbuybaranggrosir` |
| Item outlet cost | HPP per cabang | (tidak ada; legacy global) |
| Member, Member level, Point movement | Member, level, ledger poin | `01_tms_member` |
| Payment method | Metode bayar + MDR | (tetap di kode legacy) |
| Voucher, Salesperson | Kupon, salesman | `01_tms_voucherbarang` |
| Sale, Sale line, Sale payment, Sale voucher, Sale cost | Penjualan (revisi tertaut) | `01_trs_barangkeluar`, `_detail`, `_dp` |
| Stock balance, Stock movement | Saldo & ledger stok | `01_tms_stok`, `01_trs_kartustok` |
| Stock count, Stock transfer, Stock conversion | Opname, mutasi, pecah satuan | `01_trs_mutasibarang*`, `01_tmp_pecahsatuan` |
| Receivable / Payable (+ payments, settlements) | Piutang/hutang | `01_tms_piutangkredit*`, `01_tms_hutangtoko*` |
| Purchase (+ lines, costs) | Pembelian | `01_trs_barangmasuk*` |
| Device [USULAN] | Perangkat Kasir Mobile terdaftar | (tidak ada) |
| Audit log, Platform audit log | Jejak perubahan | `01_log_*` |

Rancangan kolom: `docs/ARCHITECTURE.md` (§4) dan migration di `source/backend/db/migrations/`.

---

## 11. Integrasi Eksternal

| Integrasi | Arah | Rilis | Catatan |
|---|---|---|---|
| Printer thermal (ESC/POS) — PC | Web → print-agent lokal | R1 | Menggantikan 2 implementasi printer legacy |
| Printer thermal Bluetooth — mobile [USULAN] | Aplikasi → printer (Bluetooth Classic SPP / BLE) | RM-M1 | Banyak printer murah hanya Bluetooth Classic, yang **tidak** dapat diakses dari browser (Web Bluetooth hanya BLE). Inilah salah satu alasan mobile perlu aplikasi, bukan sekadar web |
| Printer bawaan perangkat POS [USULAN] | Aplikasi → SDK vendor | RM-M2 | Per vendor (Sunmi/iMin) **[KONFIRMASI Q12]** |
| Barcode scanner | Input keyboard (HID) / kamera | R1 / RM | Tanpa driver khusus |
| WhatsApp gateway (WaSender) | Keluar | R4 | OTP, struk, pengingat piutang |
| Email SMTP | Keluar | R0 | Verifikasi email, reset password |
| Google Play | Distribusi aplikasi | RM | Akun developer atas nama Erayadigital **[KONFIRMASI Q17]** |
| Digiflazz (PPOB) | Dua arah + webhook | R4 | Verifikasi signature webhook wajib |
| Duitku / Tripay / Midtrans | Dua arah + callback | R4 | **[KONFIRMASI Q1]** yang masih aktif |
| Google (login/Drive) | — | **[KONFIRMASI]** | Ditemukan di legacy, kegunaan belum jelas |

---

## 12. Strategi Peluncuran (Data Baru)

**Prinsip:** ARUS dimulai dengan data baru. Tidak ada ETL, tidak ada sistem paralel, dan tidak ada rekonsiliasi dengan legacy.

1. **Spesifikasi aturan bisnis:** baca kode legacy hanya untuk memahami aturan. Tulis contoh kasus beserta hasil yang benar, minta pemilik produk menyetujuinya, lalu jadikan spec test (`source/tests/spec/`).
2. **Pemilihan fitur:** tandai fitur legacy yang masih dipakai (masuk scope) dan yang dibuang (Q1).
3. **Toko pilot (web):** satu toko memulai di ARUS pada tanggal mulai operasional yang disepakati:
   - H-3 s.d. H-1: setup tenant, import barang, pelatihan kasir & admin.
   - Malam H-1 (setelah tutup): stok opname fisik → input saldo awal stok; input saldo awal piutang/hutang dari laporan legacy per tanggal itu → kunci tanggal mulai operasional.
   - Hari H: transaksi berjalan di ARUS; legacy untuk toko tersebut berhenti menerima transaksi.
4. **Histori:** sebelum hari H, ekspor laporan legacy yang dibutuhkan ke PDF/Excel. Legacy tetap dapat diakses baca-saja selama masa retensi **[KONFIRMASI Q6]**.
5. **Rencana mundur:** jika ada masalah kritis di minggu pertama, toko kembali ke legacy; transaksi yang sudah terjadi di ARUS dicatat ulang manual di legacy.
6. **Pilot Kasir Mobile [USULAN]:** dimulai di toko yang **sudah stabil di ARUS web** (bukan toko yang sekaligus pindah dari legacy), agar masalah onboarding dan masalah mobile tidak tercampur. Uji tertutup ≥ 1 minggu dengan perangkat & printer nyata toko itu. Rencana mundur: kembali ke kasir web, tanpa kehilangan data karena servernya sama.
7. **Peluncuran luas (R5):** toko lain onboarding bertahap memakai prosedur yang sama.

---

## 13. Risiko & Mitigasi

| Risiko | Kemungkinan | Dampak | Mitigasi |
|---|---|---|---|
| Aturan bisnis legacy terlewat | Sedang | Tinggi | Inventaris SP/trigger (`docs/LEGACY.md` (§8)); spec test disetujui pemilik produk sebelum modul dibangun |
| Proyek berlarut, legacy terus dipakai dengan risiko keamanan | Sedang | Tinggi | Tambal kritis legacy dulu; **jalur kritis go-live pilot (§6.3) didahulukan dari fitur baru** |
| Fitur terus bertambah tanpa toko pilot yang benar-benar berjalan | **Tinggi** | Tinggi | Gerbang: Kasir Mobile M1 tidak dimulai sebelum go-live pilot web, kecuali disetujui eksplisit |
| Klien keberatan kehilangan histori di aplikasi baru | Sedang | Sedang | Ekspor laporan sebelum mulai; legacy baca-saja selama masa retensi |
| Saldo awal salah input (stok/piutang) | Sedang | Tinggi | Opname fisik di malam H-1; import dengan pratinjau & validasi; saldo awal dikunci (FR-ONB-07) |
| Klien menolak perubahan UI | Sedang | Sedang | Libatkan kasir tenant pilot sejak prototipe; shortcut keyboard mirip legacy |
| Internet toko tidak stabil | Sedang | Tinggi | Idempotency + FR-MOB-12; **[KONFIRMASI Q2/Q13]** mode offline |
| Kapasitas pengembang terbatas (tim kecil) | Tinggi | Sedang | Scope MoSCoW ketat; modul opsional ditunda; mobile memakai ulang kode web |
| Lisensi template UI | Rendah | Sedang | Pastikan lisensi Dreams Core mencakup SaaS komersial **dan aplikasi mobile yang didistribusikan** **[KONFIRMASI Q9]** |
| **Mobile:** fragmentasi printer Bluetooth (perintah ESC/POS, lebar kertas, karakter) | Tinggi | Sedang | Daftar printer yang didukung resmi; uji ≥ 2 model nyata; mode "cetak teks polos" sebagai cadangan |
| **Mobile:** nota ganda atau hilang karena sinyal/aplikasi ditutup | Sedang | Tinggi | Kunci idempotensi disimpan di perangkat sebelum kirim (FR-MOB-12); test skenario wajib (NFR-QA-04) |
| **Mobile:** offline bertentangan dengan "harga & stok dihitung server" | Tinggi (bila offline dipilih) | Tinggi | Offline jadi tahap terpisah (M3) dengan aturan konflik yang disetujui lebih dulu; M1 online saja |
| **Mobile:** HP pribadi kasir hilang/dicuri dengan sesi aktif | Sedang | Sedang | Kunci layar cepat (FR-MOB-02), cabut perangkat (FR-MOB-16), data lokal dibuang |
| **Mobile:** biaya pemeliharaan dua kanal | Sedang | Sedang | Satu basis kode UI (Capacitor), aturan hanya di server, kompatibilitas API 2 versi |
| **Mobile:** permintaan belum tervalidasi | Sedang | Sedang | Tahap M0 (Q15) sebelum membangun |

---

## 14. Asumsi, Dependensi & Pertanyaan Terbuka

### 14.1 Asumsi
- Satu instance backend melayani banyak tenant (SaaS) di VPS terpusat; on-premise mungkin di kemudian hari.
- Seluruh outlet memakai Rupiah dan berada di Indonesia.
- Data legacy tidak dimigrasikan; setiap toko memulai dengan onboarding (§7.5b).
- [USULAN] Mayoritas klien yang membutuhkan kasir mobile memakai Android.

### 14.2 Dependensi
- Go ≥ 1.23, PostgreSQL ≥ 16, Redis ≥ 7, Docker.
- Kode legacy (`../aciraba_siak_os`) sebagai referensi aturan bisnis.
- Ketersediaan toko pilot yang bersedia memulai dengan saldo awal baru.
- [USULAN] Akun Google Play Developer; perangkat & printer uji milik klien pilot; model struk server (FR-POS-15).

### 14.3 Pertanyaan Terbuka (wajib dijawab pemilik produk)

| # | Pertanyaan | Memengaruhi | Status |
|---|---|---|---|
| Q1 | Modul legacy mana yang masih aktif dipakai klien (resto, SIAK, Acipay, payment gateway)? | Scope R3–R4 | Terbuka |
| Q2 | Apakah kasir **web** wajib bisa berjalan offline? | Arsitektur frontend R1 | Terbuka |
| Q3 | Hosting: VPS terpusat, on-premise per toko, atau keduanya? | Deployment, lisensi | Sebagian: VPS terpusat sudah berjalan; on-premise belum diputuskan |
| Q4 | Format nomor nota default | FR-POS-10 | **Dijawab 2026-10-08:** `{KODE-OUTLET}-{YYMMDD}-{NNNN}` |
| Q5 | Metode HPP | FR-PUR-03 | **Dijawab 2026-10-09:** rata-rata tertimbang, per cabang |
| Q6 | Urutan hitung nota & pembulatan; lama akses baca-saja legacy | FR-POS, §12 | Sebagian: urutan + **tanpa pembulatan** dijawab 2026-10-08; masa retensi legacy terbuka |
| Q7 | Jumlah tenant, outlet, dan kasir aktif saat ini? | Target performa, biaya server | Terbuka |
| Q8 | Apakah harga dapat berbeda per outlet? | FR-MD-02 | **Dijawab:** ya, default + harga khusus per cabang |
| Q9 | Apakah lisensi template Dreams Core mencakup SaaS komersial **dan aplikasi mobile**? | Risiko legal UI | Terbuka |
| Q10 | Siapa tenant pilot dan kapan target go-live R1? | Jadwal | **Dijawab 2026-10-10:** TOKO KOTAK CANTIK MAGELANG (cabang pusat dulu); go-live akhir bulan — usulan opname malam 31 Okt, mulai 1 Nov 2026 |
| Q11 | Kasir Mobile disetujui masuk scope? Bila ya, mulai **setelah** go-live pilot web atau paralel? | §6.2, jadwal | Terbuka |
| Q12 | Perangkat & printer apa yang **benar-benar** dipakai calon pengguna mobile (merek/model HP, perangkat POS genggam, printer Bluetooth, lebar kertas)? Android minimum? | FR-MOB-05/09/09b, NFR-UX-06 | Terbuka |
| Q13 | Apakah Kasir Mobile wajib bisa berjualan **tanpa internet**? Bila ya, metode bayar apa yang boleh offline dan bagaimana nota yang ditolak saat sinkron diselesaikan? | FR-MOB-15, arsitektur | Terbuka |
| Q14 | Apakah satu perangkat dipakai bergantian oleh beberapa kasir dalam sehari (perlu ganti pengguna cepat), atau satu perangkat = satu kasir? | FR-MOB-02 | Terbuka |
| Q15 | Berapa klien/calon klien yang meminta kasir mobile, dan apakah ada yang bersedia menjadi pilot? Apakah ini fitur berbayar terpisah? | Prioritas RM, harga paket | Terbuka |
| Q16 | Apakah iOS dibutuhkan? | Tahap M4 | Terbuka |
| Q17 | Distribusi aplikasi: Google Play publik, Play tertutup per tenant, atau APK langsung? Atas nama siapa akun developer? | NFR-OPS-05 | Terbuka |

---

## 15. Lampiran

### 15.1 Glosarium

| Istilah | Arti |
|---|---|
| Tenant | Satu usaha/pelanggan ACIRABA (legacy: `KODEUNIKMEMBER`) |
| Outlet / Cabang | Toko milik tenant (legacy: `LOKASI`, `OUTLET`, `KODEOUTLET`) |
| Lokasi stok | Display (rak jual), Gudang, Retur |
| Movement | Catatan perubahan stok yang tidak dapat diubah |
| HPP | Harga Pokok Penjualan; di ARUS per cabang, rata-rata tertimbang |
| Quote | Hitungan nota oleh server tanpa menyimpan (pratinjau resmi untuk layar kasir) |
| Revisi nota | Nota baru yang menggantikan nota lama saat diedit; nota lama berstatus *superseded* |
| Penyetuju | Pengguna dengan izin *setujui* yang memasukkan PIN untuk aksi sensitif |
| MDR | Biaya metode pembayaran (persen dan/atau nominal) |
| Spec test | Test otomatis dari contoh kasus perhitungan yang ditulis manual dan disetujui pemilik produk |
| Saldo awal (opening) | Stok/piutang/hutang yang diinput saat toko mulai memakai ARUS |
| Onboarding | Proses menyiapkan toko di ARUS: setup, import barang, saldo awal |
| Idempotency key | Kunci unik per permintaan agar pengiriman ulang tidak membuat data ganda |
| RLS | Row Level Security: pembatasan baris data di level database |
| KDS | Kitchen Display System |
| Tanggal mulai operasional | Tanggal toko mulai bertransaksi di ARUS; saldo awal dikunci sejak tanggal ini |
| Kasir Mobile | Aplikasi Android untuk berjualan memakai server ARUS yang sama |
| ESC/POS | Bahasa perintah standar printer struk thermal |
| Bluetooth Classic (SPP) / BLE | Dua jenis koneksi Bluetooth; kebanyakan printer struk murah memakai Classic |
| Capacitor | Kerangka yang membungkus aplikasi web menjadi aplikasi Android/iOS dengan akses fitur native |
| Perangkat POS genggam | HP Android khusus kasir dengan printer bawaan (mis. Sunmi) |

### 15.2 Referensi
- `AGENTS.md`: keputusan teknis, roadmap, status, peta legacy, aturan trigger.
- `reference/template/`: acuan UI Dreams Core (pemetaan halaman: `AGENTS.md` §5b).
- Sistem legacy: `../aciraba_siak_os/` (read-only).

### 15.3 Perbedaan Perilaku terhadap Legacy (bug yang tidak ditiru)

| Perilaku legacy | Perilaku ARUS | Alasan |
|---|---|---|
| Edit nota menambah poin member lagi tanpa mengurangi poin lama | Poin disesuaikan sebesar selisih (pembalikan + penerapan ulang) | Bug: poin menggelembung |
| Edit nota kredit membuat ulang piutang sehingga pembayaran sebelumnya hilang dari saldo | Nota yang piutangnya sudah dibayar tidak dapat diedit; koreksi lewat pembatalan pembayaran/retur | Bug: saldo piutang salah |
| Catatan DP kolom kredit berisi nomor kartu | Berisi nominal | Bug trigger |
| Hapus nota menghapus data fisik | Void/revisi dengan status + movement balik | Jejak audit |
| Harga dari klien diterima apa adanya | Harga dihitung server; ubah harga dengan persetujuan + PIN | Keamanan |
| Diskon/voucher unik global lintas tenant | Unik per tenant | Bug multi-tenant |
| HPP rata-rata dihitung memakai stok **sesudah** barang masuk | Memakai stok **sebelum** barang masuk | Bug: HPP salah |
| HPP global untuk semua cabang | HPP per cabang | Laba per cabang tidak akurat |
| Poin dibagi `MINIMALPOIN` tanpa cek nol | Aturan 0 = tidak ada poin | Bug: bagi nol |

### 15.4 Analisis Opsi Platform Kasir Mobile [USULAN]

| Opsi | Cara | Kelebihan | Kekurangan | Penilaian |
|---|---|---|---|---|
| **A. Web responsif / PWA saja** | Layar `/kasir` dibuat responsif + dapat dipasang ke layar utama | Hampir tanpa biaya; satu basis kode; tanpa Play Store | **Tidak dapat mencetak ke printer Bluetooth Classic** (mayoritas printer murah); scan kamera di browser kurang andal; penyimpanan lokal dapat dihapus browser; tidak ada akses SDK printer bawaan | Cukup untuk validasi M0 (jualan + struk digital), **tidak cukup** untuk toko yang butuh struk cetak |
| **B. Capacitor membungkus SPA yang ada** (rekomendasi) | SvelteKit `adapter-static` dibungkus menjadi APK + plugin native (scan, Bluetooth, penyimpanan aman, SDK Sunmi) | Memakai ulang ±80% UI & seluruh kamus/validasi web; satu tim; akses penuh ke Bluetooth Classic & SDK vendor; iOS kelak dari kode yang sama | Perlu layar kasir mobile tersendiri (bukan sekadar mengecilkan layar web); butuh alur sesi native; performa WebView di HP sangat murah perlu diuji | **Rasio biaya/manfaat terbaik untuk tim kecil** |
| **C. Native Kotlin / Flutter** | Aplikasi terpisah | Performa & integrasi perangkat terbaik | Basis kode dan bahasa kedua; kamus, validasi, dan pratinjau hitung harus ditulis ulang dan dijaga tetap sama; biaya pemeliharaan ganda | Belum sebanding dengan kebutuhan saat ini |

**Rekomendasi:** mulai dari **M0 dengan opsi A** (layar kasir responsif + struk digital) untuk memvalidasi permintaan dengan biaya hampir nol. Bila pilot membutuhkan struk cetak di HP (kemungkinan besar), lanjutkan ke **opsi B**. Opsi C hanya dipertimbangkan bila WebView terbukti tidak memadai di perangkat nyata klien.

---

### Persetujuan

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Pemilik produk | | | |
| Lead developer | | | |
