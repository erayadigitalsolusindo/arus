const settle = {
  payable: { button: 'Bayar per Pemasok', title: 'Bayar Hutang per Pemasok', party: 'Pemasok', partyHint: 'Cari pemasok…', doc: 'No. Pembelian', empty: 'Pemasok ini tidak punya hutang yang belum lunas.' },
  receivable: { button: 'Terima per Member', title: 'Terima Pembayaran per Member', party: 'Member', partyHint: 'Cari member…', doc: 'No. Nota', empty: 'Member ini tidak punya piutang yang belum lunas.' },
  mode: { auto: 'Otomatis (nota terlama dulu)', manual: 'Pilih nota sendiri' },
  autoHelp: 'Isi total uang. Sistem melunasi nota paling lama lebih dulu; nota terakhir boleh terbayar sebagian.',
  manualHelp: 'Centang nota yang ingin dibayar. Jumlah terisi sisa penuh dan boleh diubah (cicilan).',
  outstanding: 'Total tunggakan: {amount} ({count} nota)',
  amount: 'Total uang yang dibayar',
  payAll: 'Bayar semua',
  selectAll: 'Pilih semua',
  clear: 'Kosongkan',
  col: { pick: 'Pilih', date: 'Tanggal', due: 'Jatuh tempo', balance: 'Sisa', pay: 'Dibayar', after: 'Sisa akhir' },
  preview: 'Pembagian uang',
  previewEmpty: 'Isi jumlah untuk melihat pembagian ke nota.',
  total: 'Total: {amount}',
  method: 'Metode',
  ref: 'No. referensi',
  note: 'Catatan',
  save: 'Simpan pelunasan',
  saving: 'Menyimpan…',
  close: 'Tutup',
  done: 'Pelunasan {doc} tersimpan',
  doneDetail: '{count} nota terbayar, total {amount}.',
  fee: 'Biaya metode {fee}'
};

export default settle;
