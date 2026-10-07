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
  }
};
