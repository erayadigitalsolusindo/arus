// Panduan Data Pendukung (satuan, kategori, brand, principal, level member, metode pembayaran), Salesman, Kupon,
// Kartu Stok, dan Pecah Satuan. Semua dialog/formulir hanya diisi lalu dipotret; tidak ada yang disimpan.
import { guides } from './guide-defs.mjs';
import { pick } from './helpers.mjs';

const dlg = '[role="dialog"]';
const addBtn = 'main button.btn-primary:has(i.icon-plus)';
const openAdd = async (page) => {
  await page.locator(addBtn).first().click();
  await page.waitForTimeout(900);
};
const rowEdit = 'tbody tr:first-child button.header-icon-btn >> nth=0';
const rowArchive = 'tbody tr:first-child button.header-icon-btn >> nth=1';

/** Halaman master sederhana (nama saja) yang memakai MasterPage. */
function simple(id, href, group, noun, what, example) {
  return {
    id: `master-${id}`,
    at: href,
    group,
    title: `Mengelola ${noun}`,
    intro: `${what} Data ini dipilih saat mengisi barang. Contoh ini hanya diisi, tidak disimpan.`,
    shots: [
      {
        title: `Daftar ${noun}`,
        goto: href,
        marks: [
          { sel: 'input[type="search"]', text: 'Cari berdasarkan nama.' },
          { sel: addBtn, text: `Klik tombol Tambah untuk membuat ${noun} baru.` },
          { sel: rowEdit, text: 'Ikon pensil: ubah nama.' },
          { sel: rowArchive, text: 'Ikon arsip: nonaktifkan agar tidak muncul di pilihan. Data lama tidak dihapus, dan bisa diaktifkan lagi.' }
        ],
        after: openAdd
      },
      {
        title: `Isi nama ${noun}`,
        before: async (page) => {
          await page.locator('#m-name').fill(example);
        },
        marks: [
          { sel: '#m-name', text: `Tulis nama ${noun}, mis. "${example}".` },
          { sel: `${dlg} button[type="submit"]`, text: 'Klik Simpan.' }
        ],
        tip: 'Nama tidak boleh sama dengan yang sudah ada.'
      }
    ]
  };
}

guides.push(
  simple('satuan', '/units', 'Master Data', 'satuan', 'Satuan adalah ukuran jual barang: Pcs, Botol, Kg, Dus, dan sebagainya.', 'Botol'),
  simple('kategori', '/categories', 'Master Data', 'kategori barang', 'Kategori mengelompokkan barang untuk pencarian dan laporan.', 'Minuman'),
  simple('brand', '/brands', 'Master Data', 'brand', 'Brand adalah merek barang.', 'Kopi Nusantara'),
  simple('principal', '/principals', 'Master Data', 'principal', 'Principal adalah pemilik merek atau distributor utama barang.', 'PT Sumber Rasa'),

  {
    id: 'master-level-member',
    at: '/member-levels',
    group: 'Master Data',
    title: 'Mengatur level member dan poin',
    intro: 'Level member menentukan cara member mengumpulkan dan menukar poin. Contoh ini hanya diisi, tidak disimpan.',
    shots: [
      {
        title: 'Daftar level',
        goto: '/member-levels',
        marks: [
          { sel: addBtn, text: 'Klik tombol Tambah untuk membuat level baru.' },
          { sel: rowEdit, text: 'Ubah level.' }
        ],
        after: openAdd
      },
      {
        title: 'Isi aturan level',
        before: async (page) => {
          await page.locator('#lv-name').fill('Gold');
          await page.locator('#lv-min').fill('500');
          await page.locator('#lv-spend').fill('10000');
          await page.locator('#lv-value').fill('100');
        },
        marks: [
          { sel: '#lv-name', text: 'Nama level, mis. Gold.' },
          { sel: '#lv-min', text: 'Poin minimum agar member naik ke level ini. Level dasar selalu mulai dari 0.' },
          { sel: '#lv-spend', text: 'Setiap belanja sebesar ini member mendapat 1 poin (mis. Rp 10.000 = 1 poin). Isi 0 bila level ini tidak mengumpulkan poin.' },
          { sel: '#lv-value', text: 'Nilai tukar 1 poin dalam rupiah saat dipakai sebagai potongan di kasir (mis. 1 poin = Rp 100). Isi 0 bila poin tidak bisa ditukar.' },
          { sel: `${dlg} button[type="submit"]`, text: 'Klik Simpan.' }
        ]
      }
    ]
  },

  {
    id: 'master-metode-bayar',
    at: '/payment-methods',
    group: 'Master Data',
    title: 'Mengatur metode pembayaran',
    intro: 'Metode pembayaran muncul di dialog bayar kasir (Nontunai). Atur biaya (MDR) bila bank atau e-wallet memotong sebagian transaksi. Contoh ini hanya diisi, tidak disimpan.',
    shots: [
      {
        title: 'Daftar metode',
        width: 1900,
        goto: '/payment-methods',
        marks: [
          { sel: 'input[type="search"]', text: 'Cari metode pembayaran.' },
          { sel: addBtn, text: 'Klik Tambah untuk membuat metode baru.' },
          { sel: rowEdit, text: 'Ubah metode: nama, biaya, dan akun.' }
        ],
        tip: 'Metode Tunai adalah metode sistem dan tidak bisa dihapus.',
        after: openAdd
      },
      {
        title: 'Isi metode dan biaya',
        height: 1300,
        before: async (page) => {
          await page.locator(`${dlg} button:has-text("Transfer BCA")`).click();
          await page.locator('#pm-fee-pct').fill('0.7');
        },
        marks: [
          { sel: `${dlg} button:has-text("Transfer BCA")`, text: 'Pilihan cepat: klik nama bank atau e-wallet (di sini Transfer BCA) untuk mengisi nama dan jenis otomatis.' },
          { sel: '#pm-name', text: 'Nama metode seperti yang ingin tampil di kasir dan laporan.' },
          { sel: '#pm-kind', text: 'Jenis: Transfer, Debit, Kartu kredit, atau E-money. Menentukan kelompok di dialog bayar.' },
          { sel: '#pm-fee-pct', text: 'Biaya persen (MDR) yang dipotong dari transaksi. Isi 0 bila tidak ada.' },
          { sel: '#pm-fee-flat', text: 'Biaya tetap per transaksi dalam rupiah (opsional).' },
          { sel: '#pm-bearer', text: 'Penanggung biaya: Toko (mengurangi pendapatan) atau Pelanggan (ditagihkan tambahan ke pelanggan di kasir).' }
        ]
      }
    ]
  },

  {
    id: 'master-salesman',
    at: '/salespeople',
    group: 'Master Data',
    title: 'Menambah salesman',
    intro: 'Salesman adalah penjual yang dicatat pada nota (opsional) untuk laporan dan komisi. Contoh ini hanya diisi, tidak disimpan.',
    shots: [
      {
        title: 'Daftar salesman',
        goto: '/salespeople',
        marks: [
          { sel: 'input[type="search"]', text: 'Cari salesman.' },
          { sel: addBtn, text: 'Klik Tambah.' }
        ],
        after: openAdd
      },
      {
        title: 'Isi data salesman',
        before: async (page) => {
          await page.locator('#s-name').fill('Rina Wulandari');
          await page.locator('#s-phone').fill('081234567890');
          await page.locator('#s-comm').fill('2');
        },
        marks: [
          { sel: '#s-name', text: 'Nama salesman (wajib).' },
          { sel: '#s-code', text: 'Kode (opsional): kosongkan agar dibuat otomatis.' },
          { sel: '#s-phone', text: 'Nomor telepon.' },
          { sel: '#s-comm', text: 'Persen komisi dari penjualan.' },
          { sel: '#s-note', text: 'Catatan bebas.' },
          { sel: `${dlg} button[type="submit"]`, text: 'Klik Simpan. Salesman lalu bisa dipilih di layar Kasir (F6).' }
        ]
      }
    ]
  },

  {
    id: 'master-kupon',
    at: '/vouchers',
    group: 'Master Data',
    title: 'Membuat kupon belanja',
    intro: 'Kupon memberi potongan persen atau nominal yang dipakai kasir dengan mengetik kodenya (Ctrl+K). Contoh ini hanya diisi, tidak disimpan.',
    shots: [
      {
        title: 'Daftar kupon',
        width: 1900,
        goto: '/vouchers',
        marks: [
          { sel: 'input[type="search"]', text: 'Cari kupon berdasarkan kode atau nama.' },
          { sel: addBtn, text: 'Klik Tambah untuk membuat kupon.' },
          { sel: rowEdit, text: 'Ubah kupon (kode tidak bisa diubah setelah pernah dipakai).' },
          { sel: rowArchive, text: 'Nonaktifkan kupon yang sudah tidak berlaku.' }
        ],
        after: openAdd
      },
      {
        title: 'Isi aturan kupon',
        height: 1200,
        before: async (page) => {
          await page.locator('#v-code').fill('HEMAT10');
          await page.locator('#v-name').fill('Diskon 10% akhir pekan');
          await page.locator('#v-value').fill('10');
          await page.locator('#v-cap').fill('50000');
          await page.locator('#v-min').fill('100000');
          await page.locator('#v-uses').fill('100');
        },
        marks: [
          { sel: '#v-code', text: 'Kode kupon yang diketik atau dipindai kasir.' },
          { sel: '#v-name', text: 'Nama kupon untuk Anda sendiri.' },
          { sel: `${dlg} [aria-label="Jenis potongan"]`, text: 'Jenis potongan: persen atau nominal rupiah.' },
          { sel: '#v-value', text: 'Besar potongan (angka persen atau rupiah sesuai jenis).' },
          { sel: '#v-cap', text: 'Batas maksimal potongan (khusus kupon persen).' },
          { sel: '#v-min', text: 'Minimal belanja agar kupon bisa dipakai.' },
          { sel: '#v-uses', text: 'Batas berapa kali kupon boleh dipakai. Kosongkan bila tanpa batas.' },
          { sel: `${dlg} button[type="submit"]`, text: 'Klik Simpan. Masa berlaku juga bisa diisi di kolom Periode.' }
        ],
        tip: 'Potongan kupon dihitung dari belanja sebelum pajak. Satu nota boleh memakai lebih dari satu kupon.'
      }
    ]
  },

  {
    id: 'stok-kartu-stok',
    at: '/stock-card',
    group: 'Master Data',
    title: 'Melihat kartu stok barang',
    intro: 'Kartu stok menampilkan semua pergerakan stok satu barang: masuk, keluar, dan saldo, sehingga selisih mudah dilacak.',
    shots: [
      {
        title: 'Pilih barang dan periode',
        goto: '/stock-card',
        marks: [
          { sel: '#card-item', text: 'Pilih barang (ketik nama atau kode).' },
          { sel: '[aria-label="Periode"]', text: 'Pilih periode tanggal.' },
          { sel: '[aria-label="Bucket"]', text: 'Filter tempat penyimpanan (bucket): Display, Gudang, atau Retur.' }
        ],
        before: async (page) => {
          await pick(page, '#card-item', 'AYAM BAWANG 100 ML');
          await page.waitForTimeout(2500);
        }
      },
      {
        title: 'Membaca kartu stok',
        height: 1400,
        marks: [
          { sel: 'button[aria-pressed]', text: 'Tombol cepat periode (hari ini, bulan ini, dan seterusnya).' },
          { sel: 'div.grid.grid-cols-2', text: 'Ringkasan: saldo awal, total masuk, total keluar, dan saldo akhir pada periode.' },
          { sel: 'table', text: 'Tabel pergerakan: tanggal, keterangan transaksi, jumlah masuk/keluar, dan saldo berjalan.' }
        ],
        tip: 'Stok tidak pernah diubah langsung: setiap perubahan tercatat sebagai pergerakan di sini.'
      }
    ]
  },

  {
    id: 'stok-pecah-satuan',
    at: '/stock-conversion',
    group: 'Penyesuaian',
    title: 'Pecah satuan (konversi stok)',
    intro: 'Pecah Satuan memindahkan stok dari satu barang ke barang lain, mis. 1 Dus dipecah menjadi 12 Pcs ketika keduanya dicatat sebagai barang berbeda. Contoh ini hanya diisi, tidak disimpan.',
    shots: [
      {
        title: 'Pilih barang asal dan tujuan',
        height: 1700,
        goto: '/stock-conversion',
        before: async (page) => {
          await pick(page, '#conv-from', 'AYAM BAWANG 100 ML');
          await pick(page, '#conv-to', 'BIRU 10 PCS');
          const qty = page.locator('section input[inputmode]:not([role="combobox"])');
          await qty.nth(0).fill('1');
          await qty.nth(1).fill('12');
          await page.waitForTimeout(800);
        },
        marks: [
          { sel: '#conv-from', text: 'Barang asal: stoknya akan dikurangi.' },
          { sel: '#conv-to', text: 'Barang tujuan: stoknya akan bertambah.' },
          { sel: 'section:has(#conv-from) input[inputmode]:not([role="combobox"])', text: 'Jumlah barang asal yang dipecah.' },
          { sel: 'section:has(#conv-to) input[inputmode]:not([role="combobox"])', text: 'Jumlah barang tujuan yang dihasilkan.' },
          { sel: 'section[aria-label]', text: 'Ringkasan: artinya, perubahan stok sebelum dan sesudah. Periksa lagi sebelum menyimpan.' },
          { sel: '#conv-note', text: 'Catatan alasan pecah satuan (opsional).' },
          { sel: 'form button[type="submit"]', text: 'Klik tombol simpan untuk mencatat konversi.' }
        ],
        tip: 'Bila stok tujuan masih kosong, harga pokoknya mengikuti barang asal; bila sudah ada stok, harga pokok lama dipertahankan. Hasil konversi tampil di Riwayat di bagian bawah halaman.'
      }
    ]
  }
);
