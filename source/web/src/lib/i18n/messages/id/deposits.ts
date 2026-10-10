const deposits = {
  title: 'Deposit',
  balance: 'Saldo deposit',
  hint: 'Uang titipan member di toko. Bisa dipakai membayar nota dan piutang; dana kembali retur penjualan bisa masuk ke sini.',
  topup: 'Top-up',
  withdraw: 'Tarik',
  empty: 'Belum ada riwayat deposit.',
  loadMore: 'Muat lebih banyak',
  form: {
    topupTitle: 'Top-up deposit',
    withdrawTitle: 'Tarik deposit',
    topupHint: 'Member menitip uang ke toko (uang masuk ke laci/rekening outlet aktif).',
    withdrawHint: 'Member mengambil sebagian/seluruh depositnya (uang keluar dari outlet aktif).',
    amount: 'Jumlah',
    method: 'Lewat metode',
    ref: 'No. referensi',
    refHint: 'Wajib untuk transfer, debit, kartu, dan e-wallet.',
    note: 'Catatan (opsional)',
    save: 'Simpan',
    saving: 'Menyimpan…',
    saved: 'Dokumen {doc} tersimpan.',
    cancel: 'Batal'
  },
  history: { time: 'Waktu', kind: 'Jenis', doc: 'Dokumen', amount: 'Jumlah', balance: 'Saldo', by: 'Oleh' },
  kind: {
    TOPUP: 'Top-up',
    WITHDRAW: 'Tarik',
    SALE_PAYMENT: 'Bayar nota',
    SALE_REVERSAL: 'Nota dibatalkan/diubah',
    RECEIVABLE_PAYMENT: 'Bayar piutang',
    SALE_RETURN: 'Dana kembali retur',
    SALE_RETURN_VOID: 'Batal retur',
    PURCHASE_RETURN: 'Dana kembali retur beli',
    PURCHASE_RETURN_VOID: 'Batal retur beli',
    PAYABLE_PAYMENT: 'Bayar hutang',
    CASH_OUT: 'Pencairan'
  }
};

export default deposits;
