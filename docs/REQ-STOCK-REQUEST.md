# Usulan Klien: Permintaan Stok Toko → Gudang (2026-10-15)

> Status: **usulan klien, belum masuk PRD, belum diputuskan**. Scope/prioritas tidak diubah tanpa persetujuan pengguna (AGENTS.md). Sumber: chat klien (Ryan Doni Pratama) 15 Okt 2026.

## Maksud klien
- SPG / SPV / leader toko, dibantu orang kantor (**alokator**), mengajukan **permintaan stok** barang yang habis di toko ke gudang.
- Gudang melihat permintaan dari **semua toko se-Indonesia**, lalu memutuskan menurut produksi dan ketersediaan stok. **Tidak otomatis disetujui** (setuju penuh / sebagian / tolak).
- Mirip mutasi, tetapi **barang tidak langsung pindah**: permintaan dulu, kirim setelah disetujui, ada waktu perjalanan karena jarak.
- **Satu user bisa mengakses banyak gudang/toko** (satu SPV memegang lebih dari satu toko).
- Konsep "supplier tidak bisa jualan sendiri, harus lewat store atau sales": barang dilempar ke store/sales dulu, baru dijual. *Tafsir belum pasti, lihat pertanyaan terbuka.*

## Kondisi aplikasi saat ini
| Kebutuhan | Status |
|---|---|
| Mutasi antar cabang dua tahap kirim → terima, selisih terima, batal, `in_transit` | ✅ FR-INV-08 (`/stock-transfer`, `internal/stock/transfer.go`) |
| 1 user banyak outlet | ✅ `user_outlets`, tetapi 1 outlet aktif per sesi (`/auth/switch-outlet`; kasir butuh PIN `outlet_switch.approve`) |
| Dokumen permintaan stok + approve/tolak/sebagian | ❌ |
| Antrean permintaan lintas outlet untuk gudang | ❌ (butuh filter berdasarkan daftar outlet yang boleh diakses, bukan outlet aktif) |
| Permintaan disetujui → otomatis membuat mutasi `sent` | ❌ |
| Tipe outlet gudang vs toko (gudang tanpa kasir/nota jual) | ❌ (hanya ada bucket stok DISPLAY/GUDANG/RETUR) |
| Peran alokator (input atas nama toko) | ❌ |
| Stok per sales (sales kanvas) | ❌ (master `salespeople` ada, stoknya belum) |

## Rancangan awal (usulan, belum disetujui)
- Tabel `stock_requests` (header + baris): status `draft → submitted → approved | partially_approved | rejected → fulfilled`; baris punya `qty_requested` dan `qty_approved`.
- Approve membuat mutasi `sent` gudang → toko dengan `request_id`; alur kirim/terima yang ada dipakai ulang tanpa diubah.
- Permission `stock_request.create` dan `stock_request.approve`.
- Tipe outlet `store` / `warehouse`; gudang tidak boleh punya kasir dan nota penjualan.
- Wajib siap big data: antrean pakai keyset, indeks `(tenant_id, status, created_at, id)`, tanpa OFFSET.

## Pertanyaan terbuka ke klien
1. Gudang itu satu pusat atau beberapa gudang regional? (menentukan tujuan permintaan)
2. Approve sebagian diperbolehkan? Sisanya ditolak atau menggantung (backorder)?
3. Alokator boleh meminta untuk toko mana saja, atau hanya toko yang ditugaskan?
4. Perlu notifikasi (WA/push) saat disetujui/dikirim?
5. "Supplier" = pemasok luar (titip jual / konsinyasi) atau pusat/produksi yang menyuplai toko?
6. "Dilempar ke sales" = sales membawa stok sendiri (kanvas, butuh stok per sales)?
7. Perlu pembedaan outlet gudang vs toko, dengan gudang tanpa kasir?
