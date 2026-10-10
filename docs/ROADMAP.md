# Roadmap Bertahap

> Dipindah dari AGENTS.md §2b. Centang `[x]` saat selesai; satu sesi = satu kotak (atau sebagian). Status terkini: `.agent/SESSION.md`.


Aturan: kerjakan berurutan; jangan lompat fase tanpa persetujuan pengguna. Setiap kotak selesai = bisa dijalankan + dites + commit.

**Fase 0 — Fondasi (tanpa fitur bisnis)**
- [x] 0.1 `git init`, `.gitignore` (termasuk `.env`, `reference/template/` bila repo akan publik — template berlisensi), struktur folder §5
- [x] 0.2 `source/deploy/docker-compose.yml`: postgres 16, redis 7; `.env.example`
- [x] 0.3 Install Go (atau dev container `golang`), `source/backend/` go module, `cmd/api` + `/healthz` (cek DB & Redis)
- [x] 0.4 goose + sqlc terpasang; migration pertama: `tenants`, `outlets`, `users`, `roles`
- [x] 0.5 `source/web/` SvelteKit SPA + Tailwind v4; layout shell (sidebar/topbar) diport dari `index.html` template; halaman `login` dari `login.html`

**Fase 1 — Aturan bisnis & spec test (paralel dengan Fase 0, tanpa menulis kode produk)**
- [ ] 1.1 Daftar fitur/menu legacy (dari `Routes.php` CI4 + router Node) → tanya pengguna mana yang dipakai → tandai IN/OUT scope di `docs/business-rules.md`
- [ ] 1.2 Rangkum aturan perhitungan dari legacy (harga grosir, diskon, potongan, pajak toko/negara, biaya lain, pembulatan `PEMBULATANANGKA`, poin, piutang/DP, retur) ke `docs/business-rules.md`; tandai yang perlu keputusan pengguna (PRD Q6)
- [ ] 1.3 Tulis 15–20 contoh kasus (tunai, split, kredit, grosir, diskon, pajak, biaya lain, edit, void, retur, saldo awal) dengan hasil yang dihitung manual → `source/tests/spec/*.yaml`; minta persetujuan pengguna

**Fase 2 — Auth & Tenant**
- [x] 2.1 Login email + password (akun baru, bukan migrasi), JWT akses + refresh di Redis, middleware tenant/outlet, RLS — selesai 2026-10-07 (sisa kecil: lihat §2 langkah 2)
- [x] 2.2 Role & permission, UI Pengguna & Role — selesai 2026-10-07 (registri izin baru per menu sidebar, bukan port 1:1 `JSONMENU`; UI mengikuti `permission-matrix.html`)

**Fase 3 — Master data**
- [x] 3.1 Satuan, kategori, brand, principal, supplier — selesai 2026-10-08 (migration `00011`, `internal/catalog`, halaman master + `Combobox`)
- [ ] 3.2 Barang + harga + grosir + diskon (UI `products-list.html`, `product-add.html`) — **sebagian: irisan B + C + D selesai 2026-10-08** (B: item inti + harga per cabang, `00012`; C: gambar, `00013`; D: grosir + konversi satuan, `00014`); sisa: diskon item
- [x] 3.3 Customer/member + poin — selesai 2026-10-08 (`00023_members.sql`, `internal/member`, halaman `/members` + `/member-levels`, member di kasir). Deposit member selesai 2026-10-10 (`00049`, `internal/wallet`)
- [x] 3.3b Metode pembayaran (master per tenant) — selesai 2026-10-08 (`00029_payment_methods.sql`, `internal/paymentmethod`, halaman `/payment-methods`, pilihan metode di layar bayar kasir)
- [ ] 3.4 Import barang & master pendukung dari Excel/CSV: template unduhan, pratinjau, validasi per baris, laporan error (PRD FR-ONB-01/02)

**Fase 4 — Stok (inti)**
- [x] 4.1 `stock_balances` + `stock_movements` (3 bucket), service + test konkuren (2 kasir jual barang sama) — selesai 2026-10-08 (`00016_stock.sql`, `internal/stock`; belum ada endpoint/UI)
- [x] 4.2 Saldo awal stok (movement `OPENING`, manual + import) + kunci tanggal mulai operasional (FR-ONB-03, FR-ONB-07) — **manual + kunci selesai 2026-10-08** (`00017`, halaman `/stock-opening`); import Excel/CSV ikut 3.4
- [x] 4.3 Opname, mutasi antar outlet/bucket (UI `stock-transfer.html`), pecah satuan, kartu stok (UI `stock-movement-report.html`) — **sebagian: pecah satuan + kartu stok per barang selesai 2026-10-08** (`00020`, `/stock-conversion`; `/stock-card`); opname selesai 2026-10-09 (`00033`, `/stock-opname`); mutasi antar cabang/bucket selesai 2026-10-09 (`00043`, `/stock-transfer`) → **4.3 selesai**

**Fase 5 — Kasir/Penjualan (POS)**
- [ ] 5.1 API simpan penjualan: harga dihitung server, idempotency, multi-payment, piutang otomatis, poin (tanpa bug §8) — **sebagian: tunai + split bayar selesai 2026-10-08** **+ kredit/piutang selesai 2026-10-09**; poin, kupon, override harga + PIN, edit/batal juga sudah (sisa: diskon master)
- [ ] 5.2 UI kasir (layar `/kasir` sudah terhubung ke simpan nota 2026-10-08; pelanggan, pending, cetak, **shortcut keyboard lengkap + bantuan F1 (2026-10-10)** sudah; sisa: catatan per baris, lulus spec test) (acuan `restaurant-pos.html`), keyboard-first; lulus spec test penjualan
- [ ] 5.3 Edit/void nota, retur penjualan, pembayaran piutang, saldo awal piutang (FR-ONB-04) — **sebagian: edit/batal nota, pembayaran piutang/cicilan + daftar piutang, dan retur penjualan (FR-AR-01..03) selesai 2026-10-09**; **batal retur penjualan + deposit member selesai 2026-10-10**; **saldo awal piutang (FR-ONB-04) selesai 2026-10-10**; sisa: pembatalan pembayaran piutang (FR-AR-07, tidak menghalangi go-live)
- [x] 5.4 print-agent Go + cetak struk — **selesai 2026-10-10 (untuk web): cetak struk lewat browser selesai 2026-10-10** (model struk server `GET /sales/{id}/receipt`, cetak ulang tercatat, 58/80 mm, Chrome `--kiosk-printing`; panduan `source/deploy/PC-KASIR.md`); **print-agent ESC/POS selesai 2026-10-10** (`source/print-agent`, template tetap di web `escpos.ts`); **struk tutup shift + cetak rekap Penjualan Hari Ini selesai 2026-10-10** (`lib/pos/report-receipts.ts`); uji printer fisik oleh pengguna: berhasil; sisa: struk retur/bayar piutang/top-up (belum diminta), Bluetooth/Android (Fase 10). Laci uang tidak dipakai Kotak Cantik
- [x] 5.5 **Shift kasir (FR-POS-16)** — selesai 2026-10-10: buka shift wajib + modal awal, tutup shift dengan hitung fisik per metode, selisih wajib catatan + PIN (`shift_close.approve`), rekap beku, Daftar Shift (`/shifts`)

**Fase 6 — Pembelian** (keputusan 2026-10-09; UI `purchase-orders.html`, `suppliers.html`)
- [x] 6.1 **HPP per cabang**: tabel `item_outlet_costs` (`00035`); `items.avg_cost/last_cost` = HPP awal/bawaan cabang yang belum punya baris. Penjualan, pecah satuan, opname, daftar/detail item memakai HPP cabang
- [x] 6.2 **(selesai 2026-10-09)** Pembelian langsung tunai/kredit (`purchases`, `purchase_lines`, biaya lain dinamis `purchase_costs`; stok masuk Display/Gudang per baris; HPP rata-rata tertimbang per cabang dalam transaksi yang sama)
- [ ] 6.3 Hutang + pembayaran (meniru `receivable`) + saldo awal hutang (FR-ONB-05) + aging — **pembayaran + daftar + aging + tolak edit/batal nota terbayar selesai 2026-10-09** (`00041`, `internal/payable`, `/supplier-payables`); pelunasan kolektif per pemasok/member selesai 2026-10-09 (`00042`); sisa: saldo awal hutang, pembatalan pembayaran
- [ ] 6.4 PO → penerimaan bertahap (PO opsional; pembelian tanpa PO tetap ada; PO tidak menggerakkan stok)
- [x] 6.5 Mutasi antar bucket — **selesai 2026-10-09** bersama mutasi antar cabang (dokumen `stock_transfers` yang sama); prasyarat retur beli (6.6) terpenuhi
- [x] 6.6 Retur beli: **hanya dari bucket Retur** (tanpa bypass ke Display/Gudang; barang dimutasi dulu ke Retur), merujuk nota asal, qty ≤ dibeli − diretur; potong hutang, kelebihan = dana dikembalikan pemasok — **selesai 2026-10-09** (`00044`, `/purchase-returns`); kredit pemasok (saldo) belum ada
- [x] 6.7 Edit/batal pembelian (pola revisi seperti penjualan) — selesai 2026-10-09 (`00040`); sisa: tolak bila hutang sudah dibayar → setelah 6.3

**Keputusan Fase 6 (pengguna, 2026-10-09):** HPP **per cabang**, metode **rata-rata tertimbang** = `(stok_cabang_sebelum × HPP lama + qty × HPP baris) ÷ (stok_sebelum + qty)`; "stok sebelum" = semua bucket cabang itu, dihitung SEBELUM movement masuk (bug legacy: dihitung sesudah); stok sebelum ≤ 0 → HPP baru = HPP baris; HPP baris = harga setelah diskon bertingkat + alokasi biaya lain nota (proporsional nilai baris), **PPN masukan dipisah dari HPP**. Satuan pembelian = **satuan dasar** saja. Diskon baris **4 tingkat persen bertingkat** (opsional). Biaya beban per barang & EXP ditiadakan; cukup **biaya lain di level nota** (dinamis). PO opsional. Edit/batal setelah barang terjual: HPP dihitung mundur `(stok × HPP − qty × HPP baris) ÷ (stok − qty)` (dibiarkan bila stok sisa ≤ 0), dibatasi batas hari edit tenant.

**Fase 7 — Laporan**: penjualan, pembelian, stok, piutang/hutang (UI `sales-report.html`, `stock-summary-report.html`, dll.) — query langsung/materialized view, bukan SP generik

**Fase 8 — Modul opsional (sesuai scope 1.1)**: Resto/KDS (`table-floor-map.html`, `kds-queue.html`, Redis Pub/Sub), SIAK akuntansi (`ledger-explorer.html`, `accounting-dashboard.html`), Acipay, payment gateway

**Fase 8a — Akuntansi SIAK (dimulai 2026-10-11; rancangan `docs/ARCHITECTURE.md` §4b, PRD §7.9)**
- [x] 8a.1 Fase A: COA (+template retail), periode, jurnal manual JU/KM/KK/TK, buku besar, kas/bank
- [x] 8a.2 Fase B: neraca saldo, laba rugi, neraca, kas/bank, jurnal umum (baca-saja, agregat; selesai 2026-10-11)
- [ ] 8a.3 Fase C: jurnal otomatis + pemetaan akun per metode bayar (penjualan harian per outlet; pembelian/piutang/hutang/retur per dokumen)

**Fase 10 — Kasir Mobile Android [PRD §7.11, §6.2, §15.4]**
- **Keputusan pengguna 2026-10-10: aplikasi kasir mobile dibuat dengan Flutter di `source/mobile/` (menggantikan usulan Capacitor di 10.1), dikerjakan LEBIH CEPAT dari rencana karena dibutuhkan segera; import barang (3.4) menyusul. Konsekuensi: template struk (`receipt.ts`/`escpos.ts`) harus ditulis ulang di Dart, UI kasir dibuat dari nol memakai API Go yang sama. PRD §7.11/§15.4 belum diselaraskan.**
- [ ] 10.0 Validasi (M0): klien peminta, perangkat & printer nyata (PRD Q11–Q17); jangan mulai 10.1 sebelum go-live pilot web kecuali pengguna menyetujui
- [ ] 10.1 (M1) Flutter (bukan Capacitor) — **sebagian 2026-10-10: proyek `source/mobile/` + login (jalur auth native tanpa cookie, sesi pulih dari Keystore) selesai**; **kasir mobile 2026-10-11: member/poin, diskon/biaya, kredit+DP, nota pending, pintasan, salesman selesai**; sisa: tabel perangkat + cabut per perangkat; kanal/perangkat di nota; filter kategori, kupon; cetak Bluetooth ESC/POS dari model struk server (prasyarat 5.4); tahan sinyal buruk
- [ ] 10.2 (M2) printer bawaan perangkat POS, tablet lanskap, scanner HID
- [ ] 10.3 (M3) offline — hanya bila PRD Q13 = ya dan aturan konflik disetujui

**Fase 9 — Onboarding & go-live (tanpa migrasi data, PRD §12)**
- [ ] 9.1 Wizard setup tenant: profil → outlet (pajak, zona waktu, format nota) → user & role → import barang → saldo awal (FR-ONB-06)
- [ ] 9.2 Checklist & panduan go-live toko pilot (opname malam H-1, ekspor laporan legacy untuk histori, pelatihan, rencana mundur)
- [ ] 9.3 Toko pilot go-live; perbaikan minggu pertama; lalu onboarding toko lain bertahap

