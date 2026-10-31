// Panduan tambahan (alur inti lainnya). Format sama dengan guide-defs.mjs.
import { guides } from './guide-defs.mjs';
import { pick } from './helpers.mjs';

const dlgSave = '[role="dialog"] button[type="submit"]';
const openFirst = (sel) => async (page) => {
  await page.locator(sel).locator('visible=true').first().click();
};

guides.push(
  {
    id: 'faktur-pembelian',
    at: '/purchases/new',
    group: 'Pembelian',
    title: 'Cara mencatat faktur pembelian dari pemasok',
    intro: 'Faktur pembelian menambah stok dan menghitung ulang harga pokok rata-rata barang. Hutang ke pemasok tercatat bila dipilih Kredit.',
    shots: [
      {
        title: 'Isi data faktur dan tambah barang',
        width: 1900,
        goto: '/purchases/new',
        before: async (page) => {
          await pick(page, '#pb-supplier');
          await page.locator('#pb-invoice').fill('INV-2026-001');
          const scan = page.locator('#pb-scan');
          await scan.fill('DMY-004769');
          await scan.press('Enter');
          await page.waitForTimeout(1200);
        },
        marks: [
          { sel: '#pb-supplier', text: 'Pilih pemasok.' },
          { sel: '#pb-invoice', text: 'Isi nomor faktur dari pemasok (sesuai kertas faktur).' },
          { sel: 'button:has-text("Kredit")', text: 'Pilih Tunai atau Kredit. Kredit akan membuat hutang ke pemasok.' },
          { sel: '#pb-scan', text: 'Cari/pindai barang yang dibeli lalu tekan Enter; barang masuk ke tabel di bawahnya.' },
          { sel: '#pb-qty', text: 'Jumlah yang ditambahkan setiap kali Enter ditekan.' },
          { sel: 'button[type="submit"]', text: 'Klik Simpan pembelian setelah semua barang dan jumlahnya benar.' }
        ],
        tip: 'Isi jumlah beli dan harga beli pada tabel; diskon bertingkat bisa diisi per baris.'
      }
    ]
  },
  {
    id: 'tambah-pemasok',
    at: '/suppliers',
    group: 'Master Data',
    title: 'Cara menambah pemasok (suplier)',
    intro: 'Pemasok dipakai pada faktur pembelian dan pencatatan hutang.',
    shots: [
      { title: 'Buka daftar pemasok', goto: '/suppliers', marks: [{ sel: 'main button:has-text("Tambah")', text: 'Klik Tambah Suplier.' }] },
      {
        title: 'Isi data pemasok',
        before: async (page) => {
          await openFirst('main button:has-text("Tambah")')(page);
          await page.waitForTimeout(800);
          await page.locator('#s-name').fill('CV Maju Jaya');
          await page.locator('#s-contact').fill('Pak Rahmat');
          await page.locator('#s-phone').fill('0761555123');
        },
        marks: [
          { sel: '#s-name', text: 'Nama pemasok (wajib).' },
          { sel: '#s-code', text: 'Kode (opsional): kosongkan agar dibuat otomatis.' },
          { sel: '#s-contact', text: 'Nama orang yang dihubungi di pemasok.' },
          { sel: '#s-phone', text: 'Nomor telepon pemasok.' },
          { sel: dlgSave, text: 'Klik Simpan.' }
        ]
      }
    ]
  },
  {
    id: 'retur-penjualan',
    at: '/sales-returns',
    group: 'Penjualan',
    title: 'Cara membuat retur penjualan',
    intro: 'Retur dibuat dari nota penjualan yang sudah ada: barang kembali ke stok dan uang/piutang disesuaikan.',
    shots: [
      {
        title: 'Pilih nota yang diretur',
        goto: '/sales-returns/new',
        marks: [
          { sel: 'input[aria-label^="Cari no"]', text: 'Cari nota dengan nomor nota, nama member, atau kasir.' },
          { sel: 'main li button', text: 'Klik nota yang barangnya dikembalikan.' }
        ],
        after: openFirst('main li button')
      },
      {
        title: 'Isi jumlah barang yang diretur',
        height: 1500,
        marks: [
          { sel: 'button:has-text("Ganti nota")', text: 'Salah pilih nota? Klik Ganti nota.' },
          { sel: 'input[aria-label^="Qty retur"]', text: 'Isi jumlah barang yang dikembalikan (tidak boleh melebihi yang dibeli).' },
          { sel: 'button:has-text("Isi semua sisa")', text: 'Atau klik Isi semua sisa untuk meretur seluruh barang.' },
          { sel: '#sale-return-note', text: 'Tulis alasan retur dan kondisi barang.' },
          { sel: 'button:has-text("Simpan Retur")', text: 'Klik Simpan Retur.' }
        ]
      }
    ]
  },
  {
    id: 'saldo-awal-stok',
    at: '/stock-opening',
    group: 'Penyesuaian',
    title: 'Cara mengisi saldo awal stok',
    intro: 'Dipakai saat toko baru mulai memakai ARUS: isi jumlah stok yang ada di setiap tempat. Setelah dikunci, perubahan stok hanya lewat transaksi.',
    shots: [
      {
        title: 'Isi stok awal per barang',
        goto: '/stock-opening',
        marks: [
          { sel: 'input[aria-label^="Cari nama"]', text: 'Cari barang dengan nama, kode, atau barcode.' },
          { sel: 'input[aria-label$="Display"]', text: 'Isi jumlah di rak display (etalase).' },
          { sel: 'input[aria-label$="Gudang"]', text: 'Isi jumlah di gudang.' },
          { sel: 'input[aria-label$="Retur"]', text: 'Isi jumlah barang retur/rusak bila ada.' },
          { sel: 'button:has-text("Kunci")', text: 'Setelah semua terisi, klik Kunci & mulai operasional.' }
        ],
        tip: 'Kunci hanya sekali per cabang. Periksa angkanya sebelum menekan tombol ini.'
      }
    ]
  },
  {
    id: 'stok-opname',
    at: '/stock-opname',
    group: 'Penyesuaian',
    title: 'Cara melakukan stok opname',
    intro: 'Opname mencocokkan stok di sistem dengan hasil hitung fisik; selisihnya dicatat sebagai penyesuaian.',
    shots: [
      {
        title: 'Mulai sesi opname',
        goto: '/stock-opname',
        marks: [
          { sel: 'main button:has-text("Opname baru")', text: 'Opname baru: hitung dalam sesi (bisa dikerjakan beberapa orang, lalu disahkan).' },
          { sel: 'a[href="/stock-opname/langsung"]', text: 'Opname langsung: hitung dan koreksi stok saat itu juga.' }
        ],
        after: openFirst('main button:has-text("Opname baru")')
      },
      {
        title: 'Tentukan sesi',
        marks: [
          { sel: '#oc-bucket', text: 'Pilih tempat yang dihitung: Display, Gudang, atau Retur.' },
          { sel: '#oc-note', text: 'Catatan sesi (opsional).' },
          { sel: dlgSave, text: 'Klik Buat sesi, lalu isi hasil hitung tiap barang.' }
        ]
      }
    ]
  },
  {
    id: 'shift-kasir',
    at: '/shifts',
    group: 'Penjualan',
    title: 'Memantau dan menutup shift kasir',
    intro: 'Shift mencatat uang laci kasir dari buka sampai tutup. Selisih uang tercatat untuk pemeriksaan.',
    shots: [
      {
        title: 'Daftar shift',
        width: 1900,
        goto: '/shifts',
        marks: [
          { sel: 'input[aria-label^="Rentang"]', text: 'Pilih rentang tanggal shift.' },
          { sel: 'main button:has-text("Semua kasir")', text: 'Saring menurut kasir.' },
          { sel: '[role="switch"]', text: 'Tampilkan hanya shift yang uangnya selisih.' },
          { sel: 'main button:has-text("Tutup Shift")', text: 'Shift yang masih terbuka bisa ditutup di sini: hitung uang laci lalu konfirmasi.' }
        ],
        tip: 'Shift dibuka dari layar Kasir saat kasir mulai bekerja.'
      }
    ]
  },
  {
    id: 'kas-kecil',
    at: '/petty-cash',
    group: 'Penjualan',
    title: 'Mencatat kas kecil (uang masuk/keluar laci)',
    intro: 'Catat pengeluaran kecil seperti beli galon atau parkir agar uang laci tetap cocok.',
    shots: [
      {
        title: 'Buka catatan kas kecil',
        goto: '/petty-cash',
        marks: [{ sel: 'main button:has-text("Catat")', text: 'Klik Catat kas kecil.' }],
        after: openFirst('main button:has-text("Catat")')
      },
      {
        title: 'Isi catatan',
        marks: [
          { sel: 'button:has-text("Kas keluar")', text: 'Pilih Kas keluar atau Kas masuk.' },
          { sel: 'button:has-text("Operasional")', text: 'Pilih kategori.' },
          { sel: '#petty-amount', text: 'Isi jumlah uang.' },
          { sel: dlgSave, text: 'Klik Simpan.' }
        ]
      }
    ]
  },
  {
    id: 'pelunasan-piutang',
    at: '/receivables',
    group: 'Penjualan',
    title: 'Menerima pelunasan piutang member',
    intro: 'Piutang muncul dari penjualan kredit. Terima pembayaran per nota atau sekaligus per member.',
    shots: [
      {
        title: 'Daftar piutang',
        goto: '/receivables',
        marks: [
          { sel: 'input[aria-label^="Cari no"]', text: 'Cari nota atau member.' },
          { sel: 'main button:has-text("Belum lunas")', text: 'Saring: belum lunas, lunas, atau semua.' },
          { sel: 'main button:has-text("Saldo Awal")', text: 'Catat piutang lama yang dibawa dari sebelum memakai ARUS.' },
          { sel: 'main button:has-text("Terima per Member")', text: 'Terima pembayaran sekaligus untuk satu member.' }
        ],
        after: openFirst('main button:has-text("Terima per Member")')
      },
      {
        title: 'Pilih member yang membayar',
        marks: [{ sel: '[role="dialog"] input', text: 'Cari dan pilih member; lalu isi jumlah bayar dan simpan. Pembayaran melunasi nota paling lama lebih dulu.' }]
      }
    ]
  },
  {
    id: 'pelunasan-hutang',
    at: '/supplier-payables',
    group: 'Pembelian',
    title: 'Membayar hutang ke pemasok',
    intro: 'Hutang muncul dari faktur pembelian kredit. Bayar per faktur atau sekaligus per pemasok.',
    shots: [
      {
        title: 'Daftar hutang',
        goto: '/supplier-payables',
        marks: [
          { sel: 'input[aria-label^="Cari no"]', text: 'Cari nota, nomor faktur, atau pemasok.' },
          { sel: 'main button:has-text("Belum lunas")', text: 'Saring status pelunasan.' },
          { sel: 'main button:has-text("Bayar per Pemasok")', text: 'Bayar hutang sekaligus untuk satu pemasok.' }
        ]
      }
    ]
  },
  {
    id: 'laporan-penjualan',
    at: '/reports/sales/invoices',
    group: 'Laporan',
    title: 'Melihat laporan penjualan',
    intro: 'Laporan penjualan per faktur memperlihatkan semua nota pada periode tertentu, bisa disaring dan diekspor.',
    shots: [
      {
        title: 'Atur periode dan saringan',
        goto: '/reports/sales/invoices',
        marks: [
          { sel: 'input[aria-label^="Rentang"]', text: 'Pilih rentang tanggal.' },
          { sel: 'main button:has-text("7 hari")', text: 'Atau pakai pintasan periode: hari ini, 7 hari, 30 hari, bulan ini.' },
          { sel: '#bits-4', text: 'Saring menurut kasir atau cabang.' },
          { sel: 'input[aria-label^="Cari nomor"]', text: 'Cari nomor faktur tertentu.' },
          { sel: 'main button:has-text("Excel")', text: 'Ekspor ke Excel atau PDF untuk dicetak/dikirim.' }
        ]
      }
    ]
  },
  {
    id: 'tambah-pengguna',
    at: '/users',
    group: 'Sistem',
    title: 'Menambah pengguna (kasir/staf)',
    intro: 'Setiap staf punya akun sendiri dengan role yang menentukan menu apa yang boleh dibuka.',
    shots: [
      {
        title: 'Buka form pengguna',
        goto: '/users',
        marks: [{ sel: 'main button:has-text("Tambah")', text: 'Klik Tambah Pengguna.' }],
        after: openFirst('main button:has-text("Tambah")')
      },
      {
        title: 'Isi data pengguna',
        before: async (page) => {
          await page.locator('#u-name').fill('Siti Aminah');
          await page.locator('#u-email').fill('siti@contoh.test');
        },
        marks: [
          { sel: '#u-name', text: 'Nama lengkap staf.' },
          { sel: '#u-email', text: 'Email: dipakai untuk masuk ke ARUS.' },
          { sel: '#u-role', text: 'Pilih role: menentukan hak akses staf.' },
          { sel: '#u-pw', text: 'Kata sandi awal; staf bisa menggantinya setelah masuk.' },
          { sel: dlgSave, text: 'Klik Simpan.' }
        ],
        tip: 'Paket langganan membatasi jumlah pengguna aktif.'
      }
    ]
  },
  {
    id: 'tambah-role',
    at: '/roles',
    group: 'Sistem',
    title: 'Membuat role dan hak akses',
    intro: 'Role mengatur menu dan aksi (lihat, tambah, ubah, hapus) yang boleh dilakukan pengguna.',
    shots: [
      {
        title: 'Buka form role',
        goto: '/roles',
        marks: [{ sel: 'main button:has-text("Tambah")', text: 'Klik Tambah Role.' }],
        after: openFirst('main button:has-text("Tambah")')
      },
      {
        title: 'Tentukan hak akses',
        marks: [
          { sel: '#role-name', text: 'Beri nama role, misalnya Kasir atau Admin Gudang.' },
          { sel: 'button:has-text("Pilih semua")', text: 'Centang semua hak sekaligus, atau pilih satu per satu di tiap modul.' },
          { sel: 'input[aria-label$="Lihat"]', text: 'Centang Lihat agar menu tampil; Tambah/Ubah/Hapus untuk aksinya.' },
          { sel: dlgSave, text: 'Klik Simpan.' }
        ]
      }
    ]
  }
);
