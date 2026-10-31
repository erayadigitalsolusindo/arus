// Definisi panduan. Tiap `shot` = satu gambar; `marks` = elemen bernomor 1, 2, 3 … (urutan array = nomor).
// `before` dijalankan sebelum memotret (mengisi form dsb.), `after` setelah memotret (klik lanjut).
import { pick } from './helpers.mjs';

export const guides = [
  {
    id: 'tambah-barang',
    at: '/items',
    group: 'Master Data',
    title: 'Cara menambah barang',
    intro: 'Barang adalah dasar semua transaksi: kasir, pembelian, dan stok memakai data ini. Ikuti langkah berikut untuk menambah barang baru.',
    shots: [
      {
        title: 'Buka daftar barang',
        goto: '/items',
        marks: [
          { sel: 'a[href="/items/new"]', text: 'Klik tombol Tambah untuk membuka formulir barang baru.' }
        ]
      },
      {
        title: 'Isi data barang',
        height: 2500,
        goto: '/items/new',
        before: async (page) => {
          await page.locator('#i-name').fill('Kopi Susu Botol 250 ml');
          await pick(page, '#i-unit', 'Pcs');
          await page.locator('#i-cost').fill('8000');
          await page.locator('#i-price').fill('15000');
        },
        marks: [
          { sel: '#i-name', text: 'Nama barang: tulis nama yang mudah dikenali kasir.' },
          { sel: '#i-barcode', text: 'Barcode (opsional): isi atau pindai agar barang cepat ditemukan di kasir.' },
          { sel: '#i-unit', text: 'Satuan: pilih satuan jual, misalnya pcs atau botol.' },
          { sel: '#i-cost', text: 'Harga beli (modal) per satuan.' },
          { sel: '#i-price', text: 'Harga jual per satuan.' },
          { sel: '#i-category', text: 'Kategori (opsional): memudahkan pengelompokan dan laporan.' },
          { sel: 'button[type="submit"]', text: 'Klik Simpan untuk menyimpan barang.' }
        ],
        tip: 'SKU dibuat otomatis bila dikosongkan.',
        after: async (page) => {
          await page.locator('button[type="submit"]').click();
        }
      },
      {
        title: 'Barang tersimpan',
        marks: []
      }
    ]
  }
];

guides.push(
  {
    id: 'tambah-member',
    at: '/members',
    group: 'Master Data',
    title: 'Cara menambah member (pelanggan)',
    intro: 'Member dipakai untuk poin, kupon, harga khusus, dan penjualan kredit (piutang). Pelanggan umum tidak perlu didaftarkan.',
    shots: [
      {
        title: 'Buka formulir member baru',
        goto: '/members',
        marks: [{ sel: 'a[href="/members/new"]', text: 'Klik tombol Tambah untuk membuka formulir member baru.' }]
      },
      {
        title: 'Isi data member',
        height: 1480,
        goto: '/members/new',
        before: async (page) => {
          await page.locator('#m-name').fill('Budi Santoso');
          await page.locator('#m-phone').fill('081234567890');
          await page.locator('#m-city').fill('Pekanbaru');
        },
        marks: [
          { sel: '#m-name', text: 'Nama member (wajib).' },
          { sel: '#m-phone', text: 'Nomor telepon: dipakai untuk mencari member di kasir.' },
          { sel: '#m-city', text: 'Alamat (opsional): kota, kecamatan, dan alamat lengkap.' },
          { sel: 'button[type="submit"]', text: 'Klik Simpan untuk menyimpan member.' }
        ],
        after: async (page) => {
          await page.locator('button[type="submit"]').click();
        }
      },
      { title: 'Member tersimpan', marks: [] }
    ]
  }
);
