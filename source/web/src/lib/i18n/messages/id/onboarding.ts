export default {
  docTitle: 'Penyiapan awal | ACIRABA',
  title: 'Siapkan ARUS Anda',
  subtitle: 'Beberapa langkah singkat agar aplikasi siap dipakai.',
  skip: 'Lewati penyiapan',
  back: 'Kembali',
  next: 'Lanjut',
  stepOf: 'Langkah {n} dari {total}',
  steps: { email: 'Email', coa: 'Akuntansi', ready: 'Kesiapan', done: 'Selesai' },
  email: {
    title: 'Konfirmasi email Anda',
    text: 'Kami mengirim tautan verifikasi ke {email}. Buka tautan itu di perangkat apa pun; halaman ini akan memperbarui diri.',
    verified: 'Email {email} sudah terverifikasi.',
    waiting: 'Menunggu konfirmasi…',
    resend: 'Kirim ulang email',
    sending: 'Mengirim…',
    sent: 'Email dikirim. Periksa kotak masuk dan folder spam.',
    later: 'Lanjut dulu, verifikasi nanti'
  },
  coa: {
    title: 'Pasang template akuntansi',
    text: 'Bagan akun retail siap pakai (aset, kewajiban, ekuitas, pendapatan, HPP, beban) beserta pemetaan jurnal otomatis dan akun kas/bank untuk metode pembayaran.',
    point1: 'Jurnal penjualan, pembelian, stok, dan kas dibuat otomatis',
    point2: 'Laporan laba rugi, neraca, dan buku besar langsung terisi',
    point3: 'Akun dan pemetaan masih bisa diubah kapan saja',
    install: 'Pasang template retail',
    installing: 'Memasang…',
    installed: 'Template akuntansi sudah terpasang ({count} akun).',
    failed: 'Template belum bisa dipasang. Coba lagi atau atur di menu Akuntansi.',
    noPermission: 'Hanya pengguna dengan izin membuat akun yang bisa memasang template.'
  },
  ready: {
    title: 'Pastikan toko siap beroperasi',
    text: 'Pemeriksaan ini diambil dari data Anda saat ini.',
    outlet: 'Outlet pertama dibuat',
    methods: 'Metode pembayaran terhubung ke akun kas/bank',
    methodsFix: '{count} metode belum terhubung ke akun. Atur di Metode Pembayaran.',
    coa: 'Bagan akun terpasang',
    items: 'Barang pertama ditambahkan',
    itemsHint: 'Tambahkan barang agar kasir bisa menjual. Bisa dilakukan sekarang atau nanti.',
    addItems: 'Tambah barang',
    fixMethods: 'Buka Metode Pembayaran'
  },
  done: {
    title: 'Semua siap',
    text: 'ARUS sudah disiapkan. Anda bisa mengubah semuanya kapan saja dari menu.',
    start: 'Mulai memakai ARUS',
    saving: 'Menyimpan…',
    failed: 'Gagal menyimpan. Coba lagi.'
  }
};
