// Stok (saldo awal, dst.).
export default {
  opening: {
    docTitle: 'Saldo Awal Stok | ACIRABA',
    title: 'Saldo Awal Stok',
    subtitle: 'Isi stok awal per barang untuk outlet yang sedang aktif. Ketik jumlah lalu tekan Enter atau pindah kolom untuk menyimpan.',
    search: 'Cari nama, kode, atau barcode…',
    col: { code: 'Kode', name: 'Nama', unit: 'Satuan' },
    bucket: { display: 'Display', warehouse: 'Gudang', returns: 'Retur' },
    saved: 'Tersimpan',
    saving: 'Menyimpan…',
    empty: 'Belum ada barang (jenis barang, bukan jasa).',
    emptySearch: 'Tidak ada barang yang cocok.',
    loadFailed: 'Gagal memuat data.',
    range: 'Menampilkan {from}–{to} dari {total}',
    prev: 'Sebelumnya',
    next: 'Berikutnya',
    status: {
      open: 'Saldo awal masih bisa diisi dan diubah.',
      lockedOn: 'Saldo awal dikunci. Mulai operasional: {date}. Perubahan stok selanjutnya lewat stok opname.'
    },
    lock: {
      button: 'Kunci & mulai operasional',
      title: 'Kunci saldo awal',
      body: 'Setelah dikunci, saldo awal outlet ini tidak dapat diubah lagi; koreksi stok hanya lewat stok opname. Pastikan semua jumlah sudah benar.',
      date: 'Tanggal mulai operasional',
      confirm: 'Kunci sekarang',
      cancel: 'Batal',
      done: 'Saldo awal dikunci. Outlet siap bertransaksi.'
    }
  },
  conversion: {
    docTitle: 'Pecah Satuan | ACIRABA',
    title: 'Pecah Satuan',
    subtitle: 'Pindahkan stok dari satu barang ke barang lain di outlet aktif (mis. 1 Karung menjadi 12 PCS). Kedua barang punya stok sendiri, jumlah diisi dalam satuan dasar masing-masing.',
    form: {
      title: 'Dokumen baru',
      from: 'Barang asal (stok dikurangi)',
      to: 'Barang tujuan (stok ditambah)',
      pick: 'Cari nama, kode, atau barcode…',
      qty: 'Jumlah',
      stock: 'Stok display: {qty} {unit}',
      note: 'Catatan (opsional)',
      submit: 'Pecah sekarang',
      submitting: 'Memproses…',
      summary: {
        title: 'Ringkasan: apa yang akan terjadi',
        meaning: '{fromQty} {fromUnit} {fromName} dipecah menjadi {toQty} {toUnit} {toName}. Dengan kata lain, 1 {fromUnit} {fromName} dihitung sama dengan {ratio} {toUnit} {toName}.',
        down: 'Stok {name} berkurang {qty} {unit}',
        up: 'Stok {name} bertambah {qty} {unit}',
        change: '{before} → {after} {unit}',
        minus: 'Stok {name} akan menjadi minus. Hanya barang yang diatur boleh stok minus yang bisa diproses; selain itu sistem menolak dengan pesan stok tidak cukup.',
        costNew: 'HPP {name} akan dihitung ulang dari HPP barang asal, karena barang ini belum punya stok.',
        costKept: 'HPP {name} tidak berubah.',
        final: 'Dokumen yang sudah dibuat tidak bisa diubah atau dihapus. Jika salah, buat dokumen kebalikannya.'
      },
      hint: 'Jumlah tujuan diisi bebas, jadi selisih atau susut (mis. 1 karung hanya menjadi 11 pcs) langsung terwakili.',
      done: 'Dokumen {doc} dibuat.',
      loadFailed: 'Gagal memuat data.'
    },
    history: {
      title: 'Riwayat',
      empty: 'Belum ada dokumen pecah satuan di outlet ini.',
      doc: 'Dokumen',
      date: 'Waktu',
      move: 'Perpindahan',
      cost: 'HPP hasil',
      costApplied: 'dipasang ke barang tujuan',
      costKept: 'HPP barang tujuan tidak diubah',
      by: 'Oleh'
    }
  }
};
