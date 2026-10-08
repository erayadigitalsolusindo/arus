const receivables = {
  docTitle: 'Piutang Anggota | ACIRABA',
  title: 'Daftar Piutang Anggota',
  subtitle: 'Piutang dari penjualan kredit ke member di cabang yang boleh Anda akses. Pembayaran boleh dicicil.',
  search: 'Cari no. nota, kode, atau nama member…',
  loadFailed: 'Gagal memuat daftar piutang.',
  empty: 'Belum ada piutang.',
  emptySearch: 'Tidak ada piutang yang cocok dengan filter.',
  loadMore: 'Muat lebih banyak',
  status: { open: 'Belum lunas', overdue: 'Lewat jatuh tempo', paid: 'Lunas', all: 'Semua' },
  stat: { outstanding: 'Total piutang', overdue: 'Lewat jatuh tempo', open: 'Nota belum lunas' },
  col: { docNo: 'No. Nota', member: 'Member', date: 'Tanggal', due: 'Jatuh tempo', amount: 'Piutang', paid: 'Dibayar', balance: 'Sisa', status: 'Status', action: 'Aksi' },
  noDue: '—',
  pay: 'Bayar',
  detail: 'Rincian',
  modal: {
    title: 'Piutang {doc}',
    info: 'Rincian piutang',
    history: 'Riwayat pembayaran',
    noPayments: 'Belum ada pembayaran.',
    payTitle: 'Terima pembayaran',
    method: 'Metode',
    amount: 'Jumlah dibayar',
    ref: 'No. referensi',
    note: 'Catatan',
    full: 'Lunasi seluruhnya',
    save: 'Simpan pembayaran',
    saving: 'Menyimpan…',
    saved: 'Pembayaran {doc} tersimpan.',
    settled: 'Piutang ini sudah lunas.',
    receivedBy: 'Diterima oleh',
    fee: 'Biaya metode {fee}',
    afterPay: 'Sisa setelah pembayaran ini: {balance}',
    close: 'Tutup'
  }
};

export default receivables;
