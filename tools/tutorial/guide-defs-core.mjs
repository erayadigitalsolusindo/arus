// Panduan dasar: masuk, daftar, dan CRUD Daftar Item. Format sama dengan guide-defs.mjs.
// `anon: true` = dipotret tanpa login. Tidak ada panduan di sini yang menyimpan data (formulir hanya diisi, tidak dikirim).
import { guides } from './guide-defs.mjs';

guides.push(
  {
    id: 'masuk',
    at: '/login',
    anon: true,
    group: 'Mulai',
    title: 'Cara masuk (login)',
    intro: 'Masuk ke ARUS memakai email dan kata sandi yang didaftarkan. Setelah berhasil Anda diarahkan ke Dashboard.',
    shots: [
      {
        title: 'Isi email dan kata sandi',
        goto: '/login',
        before: async (page) => {
          await page.locator('#email').fill('anda@tokoanda.com');
          await page.locator('#password').fill('contoh-kata-sandi');
        },
        marks: [
          { sel: '#email', text: 'Isi alamat email akun Anda.' },
          { sel: '#password', text: 'Isi kata sandi. Ikon mata di ujung kolom untuk menampilkan atau menyembunyikan sandi.' },
          { sel: 'label:has(input[type="checkbox"])', text: 'Centang "Tetap masuk" agar tidak perlu login ulang di perangkat ini.' },
          { sel: 'button[type="submit"]', text: 'Klik Masuk. Anda akan diarahkan ke Dashboard.' }
        ]
      },
      {
        title: 'Lupa sandi atau belum punya akun',
        marks: [
          { sel: 'a[href="/forgot-password"]', text: 'Lupa kata sandi? Klik di sini untuk mengatur ulang lewat email.' },
          { sel: 'a[href="/register"]', text: 'Belum punya akun? Klik Buat akun.' }
        ]
      }
    ]
  },
  {
    id: 'daftar',
    at: '/login',
    anon: true,
    group: 'Mulai',
    title: 'Cara mendaftar akun baru',
    intro: 'Satu pendaftaran membuat akun pemilik dan outlet pertama sekaligus. Setelah itu Anda langsung masuk ke Dashboard.',
    shots: [
      {
        title: 'Isi data bisnis dan pemilik',
        goto: '/register',
        height: 1000,
        before: async (page) => {
          await page.locator('#business_name').fill('Toko Maju Jaya');
          await page.locator('#outlet_name').fill('Cabang Pusat');
          await page.locator('#owner_name').fill('Budi Santoso');
          await page.locator('#email').fill('budi@tokomaju.com');
          await page.locator('#phone').fill('081234567890');
          await page.locator('#password').fill('contoh12345');
          await page.locator('#confirm').fill('contoh12345');
          await page.locator('input[type="checkbox"]').check();
        },
        marks: [
          { sel: '#business_name', text: 'Nama bisnis Anda.' },
          { sel: '#outlet_name', text: 'Nama outlet pertama, misalnya Cabang Pusat.' },
          { sel: '#owner_name', text: 'Nama pemilik.' },
          { sel: '#email', text: 'Email: dipakai untuk masuk.' },
          { sel: '#phone', text: 'Nomor HP aktif.' },
          { sel: '#password', text: 'Kata sandi minimal 10 karakter, huruf dan angka. Indikator di bawahnya menunjukkan kekuatan sandi.' },
          { sel: '#confirm', text: 'Ulangi kata sandi yang sama.' },
          { sel: 'input[type="checkbox"]', text: 'Centang persetujuan Syarat Layanan dan Kebijakan Privasi.' },
          { sel: 'button[type="submit"]', text: 'Klik Buat akun.' }
        ],
        tip: 'Contoh ini hanya diisi, tidak dikirim. Email yang sudah terdaftar akan ditolak.'
      }
    ]
  },
  {
    id: 'cari-barang',
    at: '/items',
    group: 'Master Data',
    title: 'Mencari dan memfilter daftar item',
    intro: 'Daftar Item menampilkan semua barang beserta harga dan stok. Gunakan pencarian dan filter untuk menemukan barang dengan cepat.',
    shots: [
      {
        title: 'Cari dan filter',
        goto: '/items',
        marks: [
          { sel: 'input[type="search"]', text: 'Cari berdasarkan nama, kode, atau barcode.' },
          { sel: '[aria-label="Status"]', text: 'Filter status: Semua, Aktif, atau Nonaktif.' },
          { sel: 'table thead', text: 'Tabel menampilkan kode, nama, harga jual, stok, satuan, kategori, dan status.' },
          { sel: 'a[href="/items/new"]', text: 'Tambah Item untuk membuat barang baru.' }
        ]
      }
    ]
  },
  {
    id: 'ubah-barang',
    at: '/items',
    group: 'Master Data',
    title: 'Mengubah data barang',
    intro: 'Ubah nama, harga, atau pengaturan barang kapan saja. Perubahan harga jual tercatat di Riwayat Harga dan tidak mengubah nota lama.',
    shots: [
      {
        title: 'Pilih barang',
        goto: '/items',
        marks: [{ sel: 'tbody a[href^="/items/"]:has(i.icon-pencil)', text: 'Klik ikon pensil pada barang yang ingin diubah.' }],
        after: async (page) => {
          await page.locator('tbody a[href^="/items/"]:has(i.icon-pencil)').first().click();
        }
      },
      {
        title: 'Ubah lalu simpan',
        height: 2500,
        marks: [
          { sel: '#i-name', text: 'Ubah nama barang.' },
          { sel: '#i-price', text: 'Ubah harga jual satuan.' },
          { sel: 'button[type="submit"]', text: 'Klik Simpan. Muncul notifikasi "Item disimpan."' }
        ],
        tip: 'Contoh ini hanya membuka form, tidak menyimpan perubahan.'
      }
    ]
  },
  {
    id: 'nonaktifkan-barang',
    at: '/items',
    group: 'Master Data',
    title: 'Menonaktifkan dan mengaktifkan kembali barang',
    intro: 'ARUS tidak menghapus barang permanen supaya riwayat nota dan stok tetap utuh. Barang yang tidak dijual lagi cukup dinonaktifkan.',
    shots: [
      {
        title: 'Nonaktifkan barang',
        goto: '/items',
        marks: [
          { sel: 'tbody button:has(i.icon-archive)', text: 'Klik ikon arsip untuk menonaktifkan barang. Barang disembunyikan dari pilihan dan tidak bisa dijual.' }
        ]
      },
      {
        title: 'Aktifkan kembali',
        goto: '/items',
        before: async (page) => {
          await page.locator('[aria-label="Status"]').first().click();
          await page.locator('[role="option"]', { hasText: 'Nonaktif' }).first().click();
        },
        marks: [
          { sel: '[aria-label="Status"]', text: 'Ubah filter status menjadi Nonaktif untuk melihat barang yang diarsipkan.' }
        ],
        tip: 'Pada barang nonaktif, klik ikon putar balik di kolom aksi untuk mengaktifkannya kembali.'
      }
    ]
  }
);
