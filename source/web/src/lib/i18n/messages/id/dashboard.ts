export default {
  docTitle: 'Dasbor · ACIRABA',
  live: {
    title: 'Penjualan Langsung',
    subtitle: 'Hari ini · {outlet}',
    statusLive: 'LIVE',
    statusConnecting: 'Menyambung…',
    statusOffline: 'Terputus, mencoba lagi…',
    failed: 'Gagal memuat data terbaru. Menampilkan data terakhir.',
    sales: 'Omzet hari ini',
    receipts: 'Jumlah nota',
    average: 'Rata-rata per nota',
    returns: 'Retur hari ini',
    net: 'Bersih: {amount}',
    vsYesterday: 'vs kemarin jam ini',
    noBaseline: 'kemarin belum ada penjualan',
    perHour: 'Penjualan per jam',
    recent: 'Transaksi terbaru',
    empty: 'Belum ada penjualan hari ini.',
    walkIn: 'Umum',
    lines: { one: '{count} barang', other: '{count} barang' },
    updated: 'Diperbarui {time}'
  },
  title: 'Ringkasan Toko',
  refresh: 'Muat ulang',
  updatedAt: 'Diperbarui {time}',
  loadError: 'Ringkasan belum bisa dimuat. Periksa koneksi lalu coba lagi.',
  retry: 'Coba lagi',
  noAccess: 'Akun Anda belum punya izin untuk melihat ringkasan. Hubungi pemilik toko.',
  scope: { outlet: 'Cabang {name}', all: 'Semua cabang ({count})' },
  verdict: {
    ok: { title: 'Toko Anda baik-baik saja', text: 'Semua pemeriksaan beres. Tidak ada yang perlu ditindak sekarang.' },
    warn: { title: 'Ada yang perlu diperhatikan', text: 'Toko berjalan, tetapi ada {count} hal yang sebaiknya Anda cek.' },
    bad: { title: 'Ada yang perlu segera ditangani', text: 'Ada masalah serius pada {count} hal. Lihat daftar di bawah.' }
  },
  checksTitle: 'Yang sudah diperiksa',
  checksSub: 'Hijau berarti aman, kuning perlu dilihat, merah perlu segera ditangani.',
  open: 'Lihat',
  check: {
    sales_pace: {
      ok: 'Penjualan hari ini {pct} dibanding biasanya pada jam yang sama — wajar.',
      warn: 'Penjualan hari ini {pct} dibanding biasanya pada jam yang sama. Pastikan kasir sudah buka dan toko ramai seperti biasa.'
    },
    sales_pace_nodata: { ok: 'Belum cukup riwayat 4 minggu untuk membandingkan penjualan — otomatis muncul begitu data cukup.' },
    voids: {
      ok: 'Nota dibatalkan hari ini: {count} — wajar.',
      warn: '{count} nota dibatalkan hari ini, lebih banyak dari wajar. Periksa alasannya di Daftar Penjualan.',
      bad: '{count} nota dibatalkan hari ini — sangat banyak. Periksa segera di Daftar Penjualan.'
    },
    shift_open_long: {
      ok: 'Tidak ada shift kasir yang terlupa ditutup.',
      warn: '{count} shift kasir sudah terbuka lebih dari 18 jam — kemungkinan lupa ditutup.'
    },
    shift_diff: {
      ok: 'Uang di laci cocok dengan catatan dalam 7 hari terakhir.',
      warn: '{count} shift dalam 7 hari terakhir ada selisih kas (total {amount}).'
    },
    stock_low_unset: { ok: 'Batas stok minimum belum diatur — isi di form item agar barang yang menipis diperingatkan.' },
    stock_low: {
      ok: 'Tidak ada barang yang stoknya menipis (di bawah batas minimum).',
      warn: '{count} barang stoknya menipis — sudah di bawah batas minimum, saatnya belanja ulang.'
    },
    stock_negative: {
      ok: 'Tidak ada stok yang minus.',
      warn: '{count} barang stoknya minus — biasanya barang terjual sebelum barang masuk dicatat.'
    },
    receivable_overdue: {
      ok: 'Tidak ada piutang member yang lewat jatuh tempo.',
      warn: 'Piutang member lewat jatuh tempo sebesar {amount}.',
      bad: 'Lebih dari separuh piutang member ({amount}) sudah lewat jatuh tempo.'
    },
    payable_overdue: {
      ok: 'Tidak ada hutang ke pemasok yang lewat jatuh tempo.',
      warn: 'Hutang ke pemasok lewat jatuh tempo sebesar {amount}.',
      bad: 'Hutang ke pemasok lewat jatuh tempo lebih dari 30 hari ({amount}).'
    }
  },
  kpi: {
    salesToday: 'Penjualan hari ini',
    yesterday: 'Kemarin sehari penuh: {amount}',
    vsUsual: '{pct} dari biasanya',
    vsUsualNone: 'Belum ada pembanding',
    vsUsualHint: 'Dibanding rata-rata 4 hari yang sama sebelumnya, sampai jam sekarang.',
    receipts: 'Jumlah nota',
    average: 'Rata-rata {amount} per nota',
    cancelled: '{count} dibatalkan',
    profit: 'Laba kotor hari ini',
    profitHint: 'Setelah HPP dan retur, sebelum pajak & biaya lain.',
    week: '7 hari terakhir',
    month: 'Bulan ini',
    monthVs: '{pct} dari bulan lalu',
    monthVsHint: 'Dibanding bulan lalu pada tanggal dan jam yang sama: {amount}.',
    monthVsNone: 'Bulan lalu belum ada penjualan',
    returns: 'Retur hari ini: {count} nota ({amount})'
  },
  chart: {
    trend: 'Penjualan harian',
    days: '{count} hari',
    hourly: 'Penjualan per jam hari ini',
    peak: 'Tersibuk pukul {hour}',
    today: 'hari ini',
    receiptsCount: '{count} nota',
    noSales: 'Belum ada penjualan pada periode ini. Grafik akan terisi setelah kasir mulai berjualan.'
  },
  top: { title: 'Barang terlaris', sub: '7 hari terakhir, menurut omzet', qty: '{qty} terjual', empty: 'Belum ada barang terjual.' },
  stock: {
    title: 'Kondisi stok',
    tracked: 'Barang bersaldo',
    empty: 'Stok habis',
    negative: 'Stok minus',
    low: 'Menipis',
    lowList: 'Paling menipis (stok / batas minimum)',
    lowUnset: 'Atur "Batas stok minimum" di form item agar barang yang menipis muncul di sini.',
    value: 'Nilai persediaan',
    valueHint: 'Jumlah stok × HPP rata-rata.',
    all: 'Lihat daftar item',
    negativeList: 'Stok minus terbesar'
  },
  receivable: { title: 'Piutang member', outstanding: 'Belum dibayar', overdue: 'Lewat jatuh tempo', open: '{count} nota belum lunas', all: 'Lihat daftar piutang' },
  payable: { title: 'Hutang pemasok', outstanding: 'Belum dibayar', overdue: 'Lewat jatuh tempo', open: '{count} faktur belum lunas', all: 'Lihat daftar hutang' },
  branches: {
    title: 'Perbandingan cabang',
    sub: 'Penjualan dan kondisi tiap cabang sekarang',
    outlet: 'Cabang',
    today: 'Hari ini',
    receipts: 'Nota',
    yesterday: 'Kemarin',
    week: '7 hari',
    stock: 'Stok',
    shifts: 'Shift buka',
    stockOk: 'Aman',
    negative: '{count} minus',
    low: '{count} menipis',
    empty: '{count} habis'
  },
  wide: 'Piutang dan hutang selalu mencakup semua cabang yang bisa Anda akses.',
  shifts: {
    title: 'Kas & shift kasir',
    running: 'Shift sedang berjalan',
    none: 'Tidak ada shift yang terbuka sekarang.',
    since: 'sejak {time}',
    long: 'terlalu lama',
    diff: 'Selisih kas 7 hari terakhir',
    noDiff: 'Tidak ada selisih',
    closed: '{count} shift ditutup minggu ini',
    all: 'Lihat daftar shift'
  }
};
