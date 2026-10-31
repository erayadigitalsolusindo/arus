// Panduan lengkap Daftar Item: data barang, harga cabang, grosir, satuan, gambar, keterangan, riwayat harga, daftar.
// Semua formulir hanya diisi lalu dipotret; tidak ada yang disimpan ke server.
import { guides } from './guide-defs.mjs';
import { pick } from './helpers.mjs';

const FOTO = 'C:/Users/SeiSkuy/AppData/Local/Temp/claude/D--Coding-apps-aciraba-newgen/eddd7068-a34a-465b-a2fb-9441b29d31dc/scratchpad/contoh-foto.png';
const sec = (name) => `section:has(> h3:has-text("${name}"))`;
const base = (id, title, intro, shots) => ({ id: `item-${id}`, at: '/items', group: 'Master Data', title, intro, shots });

guides.push(
  base('data-dasar', 'Item: mengisi data barang dengan lengkap', 'Selain nama, satuan, dan harga, form barang punya isian lain yang berguna: kode, barcode, pembeda, berat, batas stok, klasifikasi, dan jenis barang. Contoh ini hanya diisi, tidak disimpan.', [
    {
      title: 'Data barang',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await page.locator('#i-name').fill('Kopi Susu Botol 250 ml');
        await page.locator('#i-sku').fill('KOPI-250');
        await page.locator('#i-barcode').fill('8991234567890');
        await page.locator('#i-origin').fill('Lokal');
        await page.locator('#i-weight').fill('280');
        await page.locator('#i-minstock').fill('12');
        await pick(page, '#i-unit', 'Pcs');
      },
      marks: [
        { sel: '#i-name', text: 'Nama barang (wajib). Tulis yang mudah dikenali kasir.' },
        { sel: '#i-sku', text: 'Kode barang. Kosongkan agar dibuat otomatis (ITM-000001, dst.). Boleh huruf, angka, titik, garis.' },
        { sel: '#i-barcode', text: 'Barcode / QR: arahkan pemindai ke kolom ini lalu pindai barangnya.' },
        { sel: '#i-origin', text: 'Pembeda: isi bila ada barang lain berbarcode sama (mis. negara asal) supaya kasir bisa membedakan.' },
        { sel: '#i-weight', text: 'Berat dalam gram (opsional).' },
        { sel: '#i-minstock', text: 'Batas stok minimum: Dashboard memberi peringatan bila stok ≤ angka ini.' },
        { sel: '#i-unit', text: 'Satuan dasar (wajib), mis. Pcs, Botol, Kg. Jumlah stok dan harga grosir dihitung dalam satuan ini.' }
      ],
      tip: 'Barcode yang sama dengan barang lain tetap boleh. Form akan memperingatkan dan meminta Anda mencentang konfirmasi "barcode ini memang kembar".'
    },
    {
      title: 'Harga dan status',
      height: 2200,
      before: async (page) => {
        await page.locator('#i-cost').fill('8000');
        await page.locator('#i-price').fill('15000');
      },
      marks: [
        { sel: '#i-cost', text: 'Harga pokok awal (modal). Setelah itu berubah otomatis oleh pembelian (rata-rata dan terakhir). Saat mengubah barang, kolom ini berubah menjadi keterangan saja.' },
        { sel: '#i-price', text: 'Harga jual satuan: harga default semua cabang.' },
        { sel: '#i-active', text: 'Status barang: Aktif bisa dijual; Nonaktif disembunyikan dari pilihan.' }
      ]
    },
    {
      title: 'Klasifikasi',
      height: 2200,
      marks: [
        { sel: '#i-category', text: 'Kategori: pengelompokan utama untuk filter dan laporan.' },
        { sel: '#i-brand', text: 'Brand / merek.' },
        { sel: '#i-principal', text: 'Principal: pemilik merek atau distributor utama.' },
        { sel: '#i-supplier', text: 'Suplier utama barang ini.' }
      ],
      tip: 'Isian ini dikelola di menu Data Pendukung (Kategori, Brand, Principal) dan Suplier. Ketik untuk mencari lalu pilih.'
    },
    {
      title: 'Pengaturan barang',
      height: 2200,
      marks: [
        { sel: '#i-kind', text: 'Jenis: Barang (punya stok) atau Jasa (tanpa stok, mis. ongkos servis).' },
        { sel: 'label:has-text("stok minus")', text: 'Boleh dijual walau stok kosong: stok akan menjadi minus. Pakai hanya bila stok fisik sering belum tercatat.' },
        { sel: 'label:has-text("di bawah harga pokok")', text: 'Boleh dijual di bawah harga pokok (rugi). Bila tidak dicentang, kasir diperingatkan dan nota tidak bisa dibayar.' },
        { sel: 'button[type="submit"]', text: 'Klik Simpan bila semua sudah benar.' }
      ]
    }
  ]),

  base('harga-cabang', 'Item: harga jual khusus cabang', 'Harga jual satuan berlaku untuk semua cabang. Bila satu cabang perlu harga berbeda, isi harga khusus untuk cabang itu saja.', [
    {
      title: 'Harga per cabang',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await page.locator('#i-price').fill('15000');
      },
      marks: [
        { sel: '#i-price', text: 'Harga default semua cabang.' },
        { sel: 'h4:has-text("Harga per cabang")', text: 'Bagian Harga per cabang. Kolom kosong = cabang itu memakai harga default.' },
        { sel: '[id^="op-"]', text: 'Isi harga khusus untuk cabang ini. Hanya cabang yang boleh Anda atur yang tampil.' }
      ],
      tip: 'Di Daftar Item, barang dengan harga khusus diberi ikon toko kecil di samping harganya. Setiap perubahan harga tercatat di Riwayat Harga.'
    }
  ]),

  base('harga-grosir', 'Item: mengatur harga grosir', 'Harga grosir otomatis dipakai kasir saat jumlah beli mencapai batas tertentu. Contoh: beli 6 ke atas harganya lebih murah. Contoh ini hanya diisi, tidak disimpan.', [
    {
      title: 'Tambah tier grosir',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await pick(page, '#i-unit', 'Pcs');
        await page.locator('#i-price').fill('15000');
        const add = page.locator('button:has-text("Tambah tier")').first();
        await add.click();
        await add.click();
        await page.locator('#ws-default-min-0').fill('6');
        await page.locator('#ws-default-price-0').fill('14000');
        await page.locator('#ws-default-min-1').fill('12');
        await page.locator('#ws-default-price-1').fill('13000');
      },
      marks: [
        { sel: 'h4:has-text("Semua cabang")', text: 'Set default: berlaku untuk semua cabang.' },
        { sel: '#ws-default-min-0', text: 'Mulai jumlah: batas bawah tier, dalam satuan dasar. Di bawah tier pertama berlaku harga jual biasa.' },
        { sel: '#ws-default-price-0', text: 'Harga per satuan untuk SELURUH jumlah beli pada tier ini (bukan hanya selisihnya).' },
        { sel: 'span:has-text("6–11")', text: 'Rentang dihitung otomatis: 6–11 memakai tier ini, 12 ke atas memakai tier berikutnya.' },
        { sel: 'button[aria-label="Hapus tier"]', text: 'Hapus tier yang tidak perlu.' },
        { sel: 'button:has-text("Tambah tier")', text: 'Tambah tier lagi (maksimal 10 per set).' }
      ],
      tip: 'Harga harus makin murah untuk jumlah yang makin besar, dan jumlah awal tidak boleh sama. Bila tidak, form menolak saat disimpan. Tier terakhir berlaku sampai berapa pun jumlahnya.'
    },
    {
      title: 'Harga grosir khusus cabang',
      height: 2200,
      marks: [
        { sel: '[aria-label="Atur harga grosir khusus cabang…"]', text: 'Pilih cabang untuk membuat set grosir sendiri. Awalnya berisi salinan set default; tinggal ubah angkanya. Cabang yang punya set sendiri memakai set itu sepenuhnya, bukan gabungan dengan default. Klik Pakai default untuk mengembalikannya.' }
      ],
      tip: 'Di kasir, pelanggan otomatis mendapat harga grosir; kasir cukup mengisi jumlah. Perubahan harga grosir tercatat di Riwayat Harga dengan label Grosir.'
    }
  ]),

  base('satuan-tambahan', 'Item: satuan tambahan (konversi satuan)', 'Satu barang bisa dijual dengan beberapa satuan, mis. 1 Dus = 12 Pcs, lengkap dengan barcode dan harga sendiri. Stok tetap dihitung dalam satuan dasar. Contoh ini hanya diisi, tidak disimpan.', [
    {
      title: 'Tambah satuan Dus',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await pick(page, '#i-unit', 'Pcs');
        await page.locator('#i-price').fill('15000');
        await page.locator('button:has-text("Tambah satuan")').click();
        await pick(page, '#u-unit-0', 'Dus');
        await page.locator('#u-factor-0').fill('12');
        await page.locator('#u-barcode-0').fill('18991234567897');
      },
      marks: [
        { sel: '#u-unit-0', text: 'Satuan tambahan, mis. Dus. Tidak boleh sama dengan satuan dasar.' },
        { sel: '#u-factor-0', text: 'Isi: berapa satuan dasar dalam satu satuan ini. 12 berarti 1 Dus = 12 Pcs.' },
        { sel: '#u-barcode-0', text: 'Barcode satuan (opsional): memindai barcode ini di kasir langsung memasukkan 1 Dus.' },
        { sel: '#u-price-0', text: 'Harga jual satuan ini. Kosong = isi × harga satuan dasar (otomatis).' },
        { sel: 'button:has-text("Hapus satuan")', text: 'Hapus satuan tambahan.' },
        { sel: 'button:has-text("Tambah satuan")', text: 'Tambah satuan lain (maksimal 5).' }
      ],
      tip: 'Di kasir, stok berkurang sesuai isi: menjual 1 Dus mengurangi 12 Pcs. Satuan dan barcode tidak boleh ganda.'
    }
  ]),

  base('gambar-barang', 'Item: menambah gambar barang', 'Gambar membantu kasir mengenali barang di layar Kasir. Satu barang bisa punya beberapa gambar; salah satunya menjadi gambar utama.', [
    {
      title: 'Pilih gambar',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await page.locator('input[type="file"]').first().setInputFiles([FOTO]);
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: 'label:has(input[type="file"])', text: 'Klik Tambah gambar lalu pilih file JPG, PNG, atau WebP (maksimal 10 MB per gambar, boleh beberapa sekaligus).' },
        { sel: 'input[name="main-image"]', text: 'Pilih gambar utama: ini yang tampil di kartu barang kasir dan daftar item.' },
        { sel: 'button[aria-label="Hapus gambar"]', text: 'Hapus gambar yang salah pilih.' }
      ],
      tip: 'Pada barang baru, gambar diunggah setelah barang disimpan. Pada barang yang sudah ada, gambar langsung diunggah dan bisa dihapus atau dijadikan utama kapan saja. Klik gambar untuk memperbesar.'
    }
  ]),

  base('keterangan', 'Item: keterangan barang dengan format', 'Keterangan menyimpan deskripsi barang: komposisi, aturan pakai, dan sebagainya. Mendukung format sederhana (markdown).', [
    {
      title: 'Tulis keterangan',
      height: 2200,
      goto: '/items/new',
      before: async (page) => {
        await page.locator('#i-desc').fill('**Kopi susu** siap minum.\n\n- Isi 250 ml\n- Simpan di tempat sejuk');
      },
      marks: [
        { sel: '#i-desc', text: 'Tulis keterangan. **tebal**, *miring*, daftar dengan tanda -, dan tautan didukung.' },
        { sel: '[role="tab"]:has-text("Pratinjau")', text: 'Klik Pratinjau untuk melihat tampilannya.' }
      ],
      after: async (page) => {
        await page.locator('[role="tab"]:has-text("Pratinjau")').click();
      }
    },
    {
      title: 'Pratinjau',
      height: 2200,
      marks: [{ sel: '[role="tab"]:has-text("Tulis")', text: 'Kembali ke Tulis untuk mengubah.' }]
    }
  ]),

  base('riwayat-harga', 'Item: melihat riwayat perubahan harga', 'Setiap perubahan harga jual dan harga grosir dicatat: kapan, oleh siapa, dan dari berapa menjadi berapa. Harga pada nota lama tidak berubah.', [
    {
      title: 'Buka barang',
      goto: '/items',
      marks: [{ sel: 'tbody a[href^="/items/"]:has(i.icon-pencil)', text: 'Klik ikon pensil pada barang.' }],
      after: async (page) => {
        await page.locator('tbody a[href^="/items/"]:has(i.icon-pencil)').first().click();
        await page.waitForTimeout(2500);
      }
    },
    {
      title: 'Riwayat Harga',
      height: 2300,
      marks: [
        { sel: 'h3:has-text("Riwayat Harga")', text: 'Riwayat Harga ada di bagian bawah halaman ubah barang. Kolomnya: waktu, berlaku untuk (semua cabang atau cabang tertentu), harga sebelum dan sesudah, dan siapa yang mengubah.' }
      ],
      tip: 'Label Grosir menandai perubahan tier grosir, dan Harga awal menandai harga saat barang dibuat. Menu Riwayat Harga Jual juga menampilkan perubahan semua barang sekaligus.'
    }
  ]),

  base('daftar-fitur', 'Item: filter dan kolom di daftar item', 'Selain mencari dan memfilter status, Daftar Item punya filter kategori, gambar yang bisa diperbesar, dan penomoran halaman.', [
    {
      title: 'Filter dan kolom',
      width: 1900,
      goto: '/items',
      marks: [
        { sel: 'input[type="search"]', text: 'Cari nama, kode, atau barcode.' },
        { sel: '[aria-label="Status"]', text: 'Filter status barang.' },
        { sel: 'input[placeholder="Semua kategori"]', text: 'Filter kategori.' },
        { sel: 'tbody tr:first-child a[href^="/items/"]:has(i.icon-pencil)', text: 'Ubah barang. Barang yang tidak boleh Anda ubah hanya bisa dilihat.' }
      ]
    }
  ])
);
