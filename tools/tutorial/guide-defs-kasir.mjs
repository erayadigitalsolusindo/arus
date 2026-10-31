// Panduan lengkap layar Kasir: tampilan biasa (/kasir) dan Classic (/kasirb). Format sama dengan guide-defs.mjs.
// Tidak ada langkah yang menulis data ke server: keranjang & nota pending hanya tersimpan di browser pemotret,
// dialog hanya dibuka/diisi (tidak disimpan). Dua keadaan di-stub di peramban pemotret saja: "belum ada shift"
// (agar dialog Buka Shift tampil) dan "transaksi berhasil" (agar layar sukses tampil tanpa menyimpan nota).
import { guides } from './guide-defs.mjs';

const GROUP = 'Penjualan';
const SEARCH = 'input[placeholder^="Ketikkan kode item"]';
const DEMO = 'DMY-004769';
const dlg = '[role="dialog"]';
const click = (sel) => async (page) => {
  await page.locator(sel).locator('visible=true').first().click();
};

// Keadaan awal bersih: keranjang, nota pending, dan pilihan tampilan dihapus; stub sebelumnya dilepas.
async function fresh(page, path) {
  await page.unrouteAll({ behavior: 'ignoreErrors' });
  await page.goto(`http://localhost:5200${path}`, { waitUntil: 'domcontentloaded' });
  await page.evaluate(() => {
    for (const k of Object.keys(localStorage)) if (/^pos\.(cart|pending)\.v1|^pos\.(sideCollapsed|classicExpanded)$/.test(k)) localStorage.removeItem(k);
    localStorage.setItem('aciraba.receipt.settings', JSON.stringify({ auto: false }));
  });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1800);
}
async function addDemo(page, qty = '') {
  if (qty) await page.locator('#pos-qty').fill(qty);
  const q = page.locator(SEARCH);
  await q.fill(DEMO);
  await q.press('Enter');
  await page.waitForTimeout(1500);
}
// Peramban pemotret seolah-olah belum punya shift terbuka (hanya respons yang diubah; tidak ada data ditulis).
async function noShift(page) {
  await page.route('**/shifts/current', async (r) => r.fulfill({ response: await r.fetch(), json: { shift: null } }));
}
// Nota "berhasil disimpan" palsu hanya untuk memotret layar sukses; tidak ada nota yang dibuat.
async function fakeSaved(page, extra = {}) {
  // Semua tulis ke /sales dicegat: POST nota dijawab palsu, selain itu dibatalkan (tidak pernah sampai ke server).
  await page.route(
    (u) => /^\/sales(\/|$)/.test(u.pathname) && !/\/(quote)$/.test(u.pathname),
    async (r) => {
      const m = r.request().method();
      if (m === 'GET' || m === 'HEAD') return r.fallback();
      if (m === 'OPTIONS') return r.fallback();
      if (m !== 'POST' || !/^\/sales\/?$/.test(new URL(r.request().url()).pathname)) return r.abort();
      await r.fulfill({
        status: 201,
        headers: { 'access-control-allow-origin': 'http://localhost:5200', 'access-control-allow-credentials': 'true', 'content-type': 'application/json' },
        body: JSON.stringify({ id: '00000000-0000-0000-0000-000000000000', doc_no: 'POS-2026-000123', total: '62000.00', paid: '100000.00', change: '38000.00', surcharge: '0', receivable: '0', credit: null, member: null, points_earned: 0, points_redeemed: 0, erapay_pending: false, ...extra })
      });
    }
  );
}

const modern = (id, title, intro, shots) => ({ id: `kasir-${id}`, at: '/kasir', group: GROUP, title: `Kasir: ${title}`, intro, shots });
const classic = (id, title, intro, shots) => ({ id: `kasirb-${id}`, at: '/kasirb', group: GROUP, title: `Kasir Classic: ${title}`, intro, shots });

guides.push(
  // ============================== KASIR BIASA ==============================
  modern(
    'layar',
    'Mengenal layar Kasir',
    'Layar Kasir (tampilan biasa) dibagi tiga kolom: informasi outlet di kiri, daftar barang di tengah, dan keranjang di kanan. Semua pekerjaan kasir bisa dilakukan dari satu layar ini.',
    [
      {
        title: 'Bilah atas',
        goto: '/kasir',
        before: (page) => fresh(page, '/kasir'),
        marks: [
          { sel: 'header a[href="/dashboard"]', text: 'Panah kembali: keluar dari Kasir ke Dashboard. Kasir yang hanya boleh memakai layar ini melihat tombol Keluar (logout) di tempat yang sama.' },
          { sel: 'header button[aria-label="Shortcut keyboard"]', text: 'Daftar tombol pintas (shortcut) keyboard. Bisa juga dibuka dengan tombol F1.' },
          { sel: 'header button[aria-label="Bahasa"]', text: 'Ganti bahasa tampilan.' },
          { sel: 'header button[aria-label="Layar penuh"]', text: 'Layar penuh: menyembunyikan bilah browser agar area kerja lebih luas.' },
          { sel: 'header button[aria-label="Ganti tema"]', text: 'Ganti tema terang/gelap. Pilih yang paling nyaman di mata Anda.' },
          { sel: 'header span.rounded-full', text: 'Nama kasir yang sedang masuk. Nota yang Anda simpan tercatat atas nama ini.' }
        ]
      },
      {
        title: 'Panel informasi outlet (kolom kiri)',
        before: click('button[aria-label="Bentangkan panel outlet"]'),
        marks: [
          { sel: 'button[aria-label="Ciutkan panel outlet"]', text: 'Panel ini awalnya tertutup. Klik panah ini untuk membuka atau menutupnya; pilihan Anda diingat.' },
          { sel: 'aside >> text=Nota baru', text: 'Nomor nota baru, jam, nama kasir, dan nomor shift yang sedang berjalan.' },
          { sel: 'aside button[title="Pilih salesman"]', text: 'Salesman nota: klik untuk memilih. Kosongkan untuk "Salesman Umum".' },
          { sel: 'aside button:has-text("Nota Pending")', text: 'Daftar nota yang diparkir sementara.' },
          { sel: 'aside button:has-text("Tutup Shift")', text: 'Tutup shift saat selesai bekerja (hitung uang laci).' },
          { sel: 'aside button:has-text("Penjualan Hari Ini")', text: 'Lihat semua nota yang tersimpan hari ini.' },
          { sel: 'aside button:has-text("Tampilan Kasir B")', text: 'Pindah ke tampilan Classic (bergaya aplikasi desktop). Pilihan diingat untuk login berikutnya.' },
          { sel: 'aside button:has-text("Kas Kecil")', text: 'Catat uang keluar/masuk laci di luar penjualan.' }
        ],
        tip: 'Bila toko Anda punya lebih dari satu outlet, pilihan outlet ada di bagian atas panel ini. Pindah outlet butuh PIN Owner/Supervisor dan mengosongkan keranjang.'
      },
      {
        title: 'Kolom tengah dan kanan',
        before: click('button[aria-label="Ciutkan panel outlet"]'),
        marks: [
          { sel: '#pos-qty', text: 'QTY: jumlah untuk barang berikutnya (kosong = 1).' },
          { sel: SEARCH, text: 'Kolom cari / pindai barcode barang.' },
          { sel: 'main .grid > div:first-child', text: 'Kartu barang: foto, nama, kode, stok, dan harga. Stok merah berarti habis.' },
          { sel: 'button[aria-label="Pilih barang cepat (tanpa pindai)"]', text: 'Barang cepat: 16 tombol barang yang paling sering dijual.' },
          { sel: 'input[placeholder^="Ketikkan keterangan"]', text: 'Keterangan nota (opsional).' },
          { sel: 'section [aria-live="polite"]', text: 'Total belanja yang harus dibayar pelanggan, selalu tampil besar.' },
          { sel: 'button:has-text("BAYAR :")', text: 'Tombol BAYAR. Menyala hijau setelah ada barang di keranjang.' }
        ]
      },
      {
        title: 'Di HP atau tablet kecil',
        width: 800,
        height: 1000,
        marks: [
          { sel: 'nav[aria-label="Bagian kasir"] button:nth-child(1)', text: 'Main: informasi outlet.' },
          { sel: 'nav[aria-label="Bagian kasir"] button:nth-child(2)', text: 'Barang: cari dan pilih barang.' },
          { sel: 'nav[aria-label="Bagian kasir"] button:nth-child(3)', text: 'Keranjang: periksa belanjaan lalu bayar.' }
        ],
        tip: 'Pada layar sempit, tiga kolom berubah menjadi tiga tab di bawah layar.'
      }
    ]
  ),

  modern('buka-shift', 'Membuka shift (modal awal laci)', 'Sebelum menyimpan nota pertama, kasir harus membuka shift: catat uang kembalian yang ada di laci. Semua nota dan uang yang diterima sampai Tutup Shift tercatat ke shift ini.', [
    {
      title: 'Isi modal awal',
      goto: '/kasir',
      before: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
        await noShift(page);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page.waitForTimeout(2200);
        await page.locator('#shift-open-amount').fill('200000');
      },
      marks: [
        { sel: '#shift-open-amount', text: 'Hitung uang di laci lalu isi jumlahnya sebagai modal awal.' },
        { sel: `${dlg} button:has-text("Nanti")`, text: 'Nanti: tutup dialog dulu. Anda tetap bisa memilih barang, tetapi belum bisa menyimpan nota.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Klik Buka Shift. Dialog ini juga muncul otomatis saat Anda menekan BAYAR tanpa shift.' }
      ],
      after: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
        await page.reload({ waitUntil: 'domcontentloaded' });
      }
    },
    {
      title: 'Shift yang sedang berjalan',
      goto: '/kasir',
      before: click('button[aria-label="Bentangkan panel outlet"]'),
      marks: [
        { sel: 'aside >> text=/Shift\\s*:/', text: 'Setelah shift dibuka, nomor shift tampil di panel informasi.' },
        { sel: 'aside button:has-text("Tutup Shift")', text: 'Tombol Tutup Shift muncul di sini; lihat panduan Tutup Shift.' }
      ],
      tip: 'Satu kasir hanya punya satu shift terbuka per outlet. Bila lupa menutup, shift berikutnya tidak bisa dibuka sampai shift lama ditutup.'
    }
  ]),

  modern('cari-barang', 'Mencari dan menambah barang', 'Barang bisa dimasukkan ke keranjang dengan memindai barcode, mengetik kode/nama, mengklik kartu barang, atau memakai tombol Barang cepat.', [
    {
      title: 'Kolom pencarian dan QTY',
      goto: '/kasir',
      before: (page) => fresh(page, '/kasir'),
      marks: [
        { sel: '#pos-qty', text: 'QTY (F3): isi jumlah lebih dulu bila membeli lebih dari satu, mis. 3. Kosong berarti 1.' },
        { sel: SEARCH, text: 'Cari barang (F2): ketik kode atau nama, atau pindai barcode dengan scanner. Hasil menyaring otomatis.' },
        { sel: 'button[aria-label="Lihat keranjang sebagai tabel"]', text: 'Lihat isi keranjang sebagai tabel lengkap.' },
        { sel: 'button[aria-label="Muat ulang"]', text: 'Muat ulang daftar barang dan stok terbaru.' },
        { sel: 'button[aria-label="Pindai barcode"]', text: 'Menaruh kursor di kolom cari agar siap memindai.' }
      ]
    },
    {
      title: 'Memindai atau mengetik kode barang',
      before: async (page) => {
        await page.locator('#pos-qty').fill('3');
        await page.locator(SEARCH).fill(DEMO);
        await page.waitForTimeout(900);
      },
      marks: [
        { sel: '#pos-qty', text: 'Jumlah 3 sudah diisi.' },
        { sel: SEARCH, text: 'Kode barang diketik atau dipindai, lalu tekan Enter. Barang cocok langsung masuk keranjang sebanyak QTY.' },
        { sel: 'main .grid > div:first-child', text: 'Daftar di bawah menyaring sesuai ketikan Anda.' }
      ],
      tip: 'Bila satu barcode dipakai beberapa barang, muncul daftar pilihan; klik barang yang benar.',
      after: async (page) => {
        await page.locator(SEARCH).press('Enter');
        await page.waitForTimeout(1500);
      }
    },
    {
      title: 'Barang masuk keranjang',
      marks: [
        { sel: '#cart-line-0', text: 'Barang yang baru ditambahkan selalu naik ke baris paling atas dan diberi sorotan sebentar.' },
        { sel: '#cart-line-0 input[data-cart-qty]', text: 'Jumlah 3 sesuai QTY tadi. Memindai barang yang sama lagi menambah jumlahnya, bukan baris baru.' },
        { sel: 'section [aria-live="polite"]', text: 'Total langsung dihitung oleh sistem.' }
      ]
    },
    {
      title: 'Menambah dengan klik kartu barang',
      before: async (page) => {
        await page.locator('#pos-qty').fill('');
        await page.locator(SEARCH).fill('');
        await page.waitForTimeout(1200);
      },
      marks: [{ sel: 'main button:has-text("Tambah Keranjang")', text: 'Klik Tambah Keranjang di kartu barang mana pun untuk memasukkannya (jumlah 1).' }],
      tip: 'Gulir daftar ke bawah lalu klik Muat lebih banyak untuk melihat barang lainnya.'
    },
    {
      title: 'Barang cepat (tanpa pindai)',
      before: click('button[aria-label="Pilih barang cepat (tanpa pindai)"]'),
      marks: [
        { sel: `${dlg} .grid > button:first-child`, text: 'Satu slot = satu barang favorit. Klik slot terisi untuk langsung memasukkan barangnya ke keranjang.' },
        { sel: `${dlg} p:has-text("Klik barang")`, text: 'Klik kanan (atau tekan lama di layar sentuh) pada slot terisi untuk mengganti atau mengosongkan. Klik slot kosong (+) untuk mengisinya.' }
      ],
      tip: 'Cocok untuk barang tanpa barcode, mis. gorengan atau plastik. Slot tersimpan per kasir.',
      after: click(`${dlg} .grid > button:last-child`)
    },
    {
      title: 'Mengisi slot Barang cepat',
      before: async (page) => {
        await page.waitForTimeout(1800);
      },
      marks: [
        { sel: '[role="dialog"][aria-label^="Pilih barang untuk slot"] input[type="search"]', text: 'Cari barang yang akan ditaruh di slot ini.' },
        { sel: '[role="dialog"][aria-label^="Pilih barang untuk slot"] .grid > button:nth-child(1)', text: 'Klik barangnya: tersimpan di slot sampai Anda menggantinya.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
        await page.waitForTimeout(300);
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Melihat keranjang sebagai tabel',
      before: click('button[aria-label="Lihat keranjang sebagai tabel"]'),
      marks: [
        { sel: `${dlg} input[type="search"]`, text: 'Cari barang di dalam keranjang.' },
        { sel: `${dlg} tbody tr:first-child`, text: 'Nomor, nama, kode, jumlah, harga, potongan, dan jumlah per baris.' },
        { sel: `${dlg} tfoot tr:last-child`, text: 'Ringkasan: subtotal, potongan, biaya lain, pajak, lalu total.' }
      ],
      tip: 'Tabel ini hanya untuk dilihat. Mengubah jumlah dilakukan di keranjang.'
    }
  ]),

  modern('keranjang', 'Mengatur keranjang belanja', 'Ubah jumlah, ubah harga atau beri potongan, hapus barang, atau kosongkan keranjang. Stok dan harga diperiksa sistem setiap ada perubahan.', [
    {
      title: 'Tombol di setiap baris',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await addDemo(page, '2');
      },
      marks: [
        { sel: '#cart-line-0 button[aria-label="Kurangi"]', text: 'Kurangi jumlah 1 (shortcut Ctrl dan −).' },
        { sel: '#cart-line-0 input[data-cart-qty]', text: 'Atau ketik jumlah langsung, boleh desimal untuk barang timbang (mis. 0,5). Enter pindah ke baris berikutnya.' },
        { sel: '#cart-line-0 button[aria-label="Tambah"]', text: 'Tambah jumlah 1 (Ctrl dan +).' },
        { sel: '#cart-line-0 button[aria-label="Ubah harga / potongan"]', text: 'Ubah harga atau beri potongan (Ctrl+E). Butuh persetujuan.' },
        { sel: '#cart-line-0 button[aria-label="Hapus baris"]', text: 'Hapus barang ini dari keranjang (Ctrl+Delete).' },
        { sel: '#cart-line-0 >> text=SUB TOTAL', text: 'Sub total dan potongan baris ini.' }
      ]
    },
    {
      title: 'Bila stok tidak cukup',
      before: async (page) => {
        await page.locator('#cart-line-0 input[data-cart-qty]').fill('99');
        await page.locator('#cart-line-0 input[data-cart-qty]').press('Tab');
        await page.waitForTimeout(1500);
      },
      marks: [
        { sel: '#cart-line-0 [role="alert"]', text: 'Baris memerah dan muncul pesan stok. Jumlah melebihi stok tidak bisa dibayar.' },
        { sel: '#cart-line-0 input[data-cart-qty]', text: 'Kecilkan jumlahnya sesuai stok tersedia.' },
        { sel: 'button:has-text("BAYAR :")', text: 'Tombol BAYAR nonaktif selama ada baris bermasalah.' }
      ],
      after: async (page) => {
        await page.locator('#cart-line-0 input[data-cart-qty]').fill('2');
        await page.locator('#cart-line-0 input[data-cart-qty]').press('Tab');
        await page.waitForTimeout(1200);
      }
    },
    {
      title: 'Ubah harga atau beri potongan',
      before: click('#cart-line-0 button[aria-label="Ubah harga / potongan"]'),
      marks: [
        { sel: `${dlg} [role="tab"]:has-text("Ubah harga")`, text: 'Tab Ubah harga: ganti harga satuan barang ini hanya untuk nota ini.' },
        { sel: `${dlg} [role="tab"]:has-text("Potongan")`, text: 'Tab Potongan: beri potongan dalam rupiah per satuan atau untuk seluruh baris.' },
        { sel: `${dlg} input[type="password"]`, text: 'Perubahan butuh persetujuan: pilih Owner/Supervisor lalu masukkan PIN 6 digit miliknya.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Klik Setujui & Terapkan. Baris diberi tanda "Harga diubah" beserta nama penyetujunya.' }
      ],
      tip: 'Tanda "Kembalikan harga normal" atau "Hapus potongan" muncul di baris untuk membatalkan perubahan.',
      after: click(`${dlg} button:has-text("Batal")`)
    },
    {
      title: 'Hapus barang atau kosongkan keranjang',
      marks: [
        { sel: '#cart-line-0 button[aria-label="Hapus baris"]', text: 'Hapus satu barang.' },
        { sel: 'section button[aria-label="Kosongkan keranjang"]', text: 'Kosongkan seluruh keranjang sekaligus (pelanggan batal belanja).' }
      ],
      tip: 'Keranjang tersimpan di browser per kasir. Jika browser tertutup tidak sengaja, isinya dipulihkan saat Kasir dibuka lagi (harga yang diubah kembali normal).'
    }
  ]),

  modern('member-kupon', 'Member, tukar poin, kupon, dan salesman', 'Pilih member agar poin terkumpul, tukarkan poin sebagai potongan, pakai kupon belanja, dan catat salesman penjual.', [
    {
      title: 'Memilih member',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await addDemo(page);
        await page.locator('button[aria-label="Pilih pelanggan"]').click();
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: `${dlg} input[type="search"]`, text: 'Cari member dengan nama, kode, atau nomor telepon (F4 membuka dialog ini).' },
        { sel: `${dlg} ul li:first-child button`, text: 'Klik member yang dimaksud. Tampil poin dan levelnya.' }
      ],
      tip: 'Tanpa memilih member, nota dicatat sebagai "Member Umum". Penjualan kredit (piutang) wajib memilih member.',
      after: click(`${dlg} ul li:first-child button`)
    },
    {
      title: 'Member terpilih',
      before: async (page) => {
        await page.waitForTimeout(1800);
      },
      marks: [
        { sel: 'section button[aria-label="Ganti member"]', text: 'Foto member. Klik untuk mengganti atau melepas member.' },
        { sel: 'section >> text=Nama Pelanggan Terpilih', text: 'Nama, kode, level, saldo poin, dan poin yang akan didapat dari nota ini.' },
        { sel: 'button[aria-label="Tukar poin"]', text: 'Ikon hadiah: tukar poin menjadi potongan.' },
        { sel: 'button[aria-label="Kupon belanja"]', text: 'Ikon tiket: pakai kupon belanja.' }
      ]
    },
    {
      title: 'Menukar poin',
      before: click('button[aria-label="Tukar poin"]'),
      marks: [
        { sel: `${dlg} input[inputmode="numeric"]`, text: 'Isi jumlah poin yang ditukar. Nilai 1 poin dan batas maksimal ditampilkan di atasnya.' },
        { sel: `${dlg} button:has-text("Maks")`, text: 'Maks.: tukar poin sebanyak mungkin yang diizinkan.' },
        { sel: `${dlg} button:has-text("Selesai")`, text: 'Klik Selesai. Potongan langsung mengurangi total.' }
      ],
      tip: 'Level member tertentu mungkin tidak boleh menukar poin; sistem akan memberi tahu.',
      after: click(`${dlg} button:has-text("Selesai")`)
    },
    {
      title: 'Memakai kupon belanja',
      before: click('button[aria-label="Kupon belanja"]'),
      marks: [
        { sel: '#pos-voucher', text: 'Ketik atau pindai kode kupon.' },
        { sel: `${dlg} button:has-text("Pakai")`, text: 'Klik Pakai. Sistem memeriksa kupon (masa berlaku, minimal belanja, dsb.).' },
        { sel: `${dlg} button:has-text("Selesai")`, text: 'Selesai. Potongan kupon dihitung sebelum pajak; boleh memakai lebih dari satu kupon.' }
      ],
      after: click(`${dlg} button:has-text("Selesai")`)
    },
    {
      title: 'Memilih salesman',
      before: async (page) => {
        await page.keyboard.press('F6');
        await page.waitForTimeout(700);
      },
      marks: [
        { sel: `${dlg} input`, text: 'Pilih salesman penjual nota ini (F6). Kosongkan untuk "Salesman Umum".' },
        { sel: `${dlg} button:has-text("Selesai")`, text: 'Klik Selesai. Salesman dipakai untuk laporan dan komisi.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  modern('biaya-pajak', 'Biaya lain-lain, pajak, dan keterangan', 'Tambahkan ongkos kirim atau biaya bungkus, hitung pajak bila toko memungut pajak, dan tulis keterangan nota.', [
    {
      title: 'Isian di bawah keranjang',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await addDemo(page);
        await page.locator('input[aria-label="Biaya Lain-Lain"]').fill('5000');
        await page.locator('input[placeholder^="Ketikkan keterangan"]').fill('Titip kirim ke rumah');
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: 'input[aria-label="Biaya Lain-Lain"]', text: 'Isi biaya tambahan sekali jalan (mis. 5.000). Biaya ikut menambah total.' },
        { sel: 'button[aria-label="Rincian biaya lain-lain"]', text: 'Untuk lebih dari satu jenis biaya, klik tombol rincian.' },
        { sel: 'section output:near(:text("Pajak Toko"))', text: 'Pajak Toko dan Pajak Negara (persennya tampil di bawah namanya). Baru dihitung setelah Anda menekan HITUNG PAJAK.' },
        { sel: 'button:has-text("HITUNG PAJAK")', text: 'HITUNG PAJAK: terapkan pajak pada nota ini.' },
        { sel: 'button:has-text("BATAL HITUNG")', text: 'BATAL HITUNG: lepas pajak dari nota ini.' },
        { sel: 'input[placeholder^="Ketikkan keterangan"]', text: 'Keterangan nota, mis. nama penerima. Ikut tercetak di struk.' }
      ]
    },
    {
      title: 'Setelah HITUNG PAJAK',
      before: click('button:has-text("HITUNG PAJAK")'),
      marks: [
        { sel: 'section output:near(:text("Pajak Toko"))', text: 'Nilai pajak muncul.' },
        { sel: 'section [aria-live="polite"]', text: 'Total ikut bertambah.' }
      ],
      tip: 'Pajak hanya muncul bila outlet memiliki persentase pajak di pengaturan outlet.',
      after: click('button:has-text("BATAL HITUNG")')
    },
    {
      title: 'Rincian biaya lain-lain',
      before: click('button[aria-label="Rincian biaya lain-lain"]'),
      marks: [
        { sel: `${dlg} input[placeholder^="mis."]`, text: 'Nama biaya, mis. Ongkos kirim.' },
        { sel: `${dlg} input[placeholder="0"]`, text: 'Jumlah biayanya.' },
        { sel: `${dlg} button:has-text("Tambah biaya")`, text: 'Tambah baris biaya lain (maksimal 20).' },
        { sel: `${dlg} button:has-text("Simpan rincian")`, text: 'Klik Simpan rincian. Total semua biaya masuk ke kolom Biaya Lain-Lain.' }
      ]
    }
  ]),

  modern('pending', 'Menyimpan nota pending', 'Pending memarkir belanjaan pelanggan yang belum selesai, supaya Anda bisa melayani pelanggan lain, lalu membukanya lagi kapan saja.', [
    {
      title: 'Parkir keranjang',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await addDemo(page);
      },
      marks: [{ sel: 'button:text-is("PENDING")', text: 'Klik PENDING (F7) saat pelanggan belum jadi membayar.' }],
      after: click('button:text-is("PENDING")')
    },
    {
      title: 'Beri keterangan',
      before: async (page) => {
        await page.locator(`${dlg} input`).fill('Bu Ani, baju merah');
      },
      marks: [
        { sel: `${dlg} input`, text: 'Tulis keterangan agar mudah dikenali, mis. nama pelanggan. Boleh dikosongkan.' },
        { sel: `${dlg} button:has-text("Simpan Pending")`, text: 'Klik Simpan Pending (atau Enter). Keranjang langsung kosong dan siap untuk pelanggan berikutnya.' }
      ],
      after: click(`${dlg} button:has-text("Simpan Pending")`)
    },
    {
      title: 'Membuka daftar nota pending',
      marks: [{ sel: 'section button[aria-label="Nota Pending"]', text: 'Ikon daftar (F8). Angka kuning = jumlah nota pending. Maksimal ada batasnya.' }],
      after: click('section button[aria-label="Nota Pending"]')
    },
    {
      title: 'Membuka, mengganti nama, atau menghapus',
      marks: [
        { sel: `${dlg} li:first-child input`, text: 'Ubah keterangan kapan saja.' },
        { sel: `${dlg} li:first-child button:has-text("Buka")`, text: 'Buka: isi nota kembali ke keranjang. Bila keranjang sedang berisi, otomatis diparkir dulu.' },
        { sel: `${dlg} li:first-child button[aria-label="Hapus"]`, text: 'Hapus nota pending (isinya hilang).' }
      ],
      tip: 'Harga dan stok dihitung ulang saat nota dibuka. Nota pending tersimpan di browser komputer kasir ini, jadi tidak muncul di komputer lain.'
    }
  ]),

  modern('bayar', 'Membayar dan menyimpan transaksi', 'Pilih jenis transaksi (Tunai, Kredit, Nontunai, Split), isi uang diterima, lalu simpan. Stok berkurang dan struk tercetak otomatis.', [
    {
      title: 'Klik BAYAR',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await addDemo(page);
      },
      marks: [
        { sel: 'section [aria-live="polite"]', text: 'Periksa total sekali lagi bersama pelanggan.' },
        { sel: 'button:has-text("BAYAR :")', text: 'Klik BAYAR (F10 atau Ctrl+Enter). Bila shift belum dibuka, Anda diminta membukanya dulu.' }
      ],
      after: click('button:has-text("BAYAR :")')
    },
    {
      title: 'Pembayaran tunai (F1)',
      before: async (page) => {
        await page.locator('input[data-pay]').first().fill('100000');
        await page.waitForTimeout(500);
      },
      marks: [
        { sel: `${dlg} [role="radiogroup"]`, text: 'Jenis transaksi: F1 TUNAI, F2 KREDIT, F3 NONTUNAI, F4 SPLITCASH. Tekan tombol F-nya untuk berpindah.' },
        { sel: `${dlg} input[data-pay]`, text: 'Isi uang yang diterima dari pelanggan.' },
        { sel: `${dlg} input[aria-label="Total Bayar:"]`, text: 'Total yang sudah dibayar.' },
        { sel: `${dlg} input[aria-label="Kembalian:"]`, text: 'Kembalian dihitung otomatis. Bila uang kurang, tulisan berubah menjadi Kurang.' },
        { sel: `${dlg} button:has-text("Batal")`, text: 'Batal: kembali ke keranjang tanpa menyimpan.' },
        { sel: `${dlg} button:has-text("Simpan Transaksi")`, text: 'Simpan Transaksi (tombol End).' }
      ],
      tip: 'Enter di kolom isian memindahkan ke kolom berikutnya, terakhir ke tombol Simpan.'
    },
    {
      title: 'Nontunai (F3): transfer, kartu, e-wallet, QRIS',
      before: async (page) => {
        await page.locator(`${dlg} label:has-text("NONTUNAI")`).click();
        await page.waitForTimeout(900);
      },
      marks: [
        { sel: `${dlg} [role="radiogroup"]:has-text("jenis") , ${dlg} [role="radiogroup"] >> nth=1`, text: 'Pilih jenis (transfer, kartu debit/kredit, e-wallet, deposit member, QRIS).' },
        { sel: `${dlg} input[aria-label="No. referensi"]`, text: 'Isi nomor referensi / ID transaksi dari struk EDC atau bukti transfer.' }
      ],
      tip: 'Untuk QRIS, klik Buat QR QRIS lalu minta pelanggan memindai. Nota tersimpan otomatis setelah pembayaran diterima. Bila ada biaya metode yang ditanggung pelanggan, biaya itu ditagihkan terpisah.'
    },
    {
      title: 'Split cash (F4): beberapa cara bayar',
      before: async (page) => {
        await page.locator(`${dlg} label:has-text("SPLITCASH")`).click();
        await page.waitForTimeout(900);
      },
      marks: [
        { sel: `${dlg} input[data-pay]`, text: 'Isi jumlah per metode, mis. sebagian tunai.' },
        { sel: `${dlg} button:has-text("Tambah metode")`, text: 'Tambah metode lain, mis. sisanya transfer.' }
      ],
      tip: 'Jumlah semua metode harus menutup total belanja.'
    },
    {
      title: 'Kredit (F2): jual ke member dengan piutang',
      before: async (page) => {
        await page.locator(`${dlg} label:has-text("KREDIT")`).click();
        await page.waitForTimeout(900);
      },
      marks: [{ sel: `${dlg} [role="alert"]`, text: 'Kredit hanya untuk member. Tutup dialog, pilih member di panel pelanggan, lalu bayar lagi.' }],
      tip: 'Dengan member terpilih, dialog menampilkan jatuh tempo, limit piutang, dan piutang berjalan. Boleh membayar di muka (DP); sisanya menjadi piutang. Melewati limit butuh persetujuan Owner/Supervisor. Pelunasannya dicatat di menu Piutang.',
      after: click(`${dlg} button:has-text("Batal")`)
    },
    {
      title: 'Transaksi berhasil',
      before: async (page) => {
        await fakeSaved(page);
        await page.locator('button:has-text("BAYAR :")').click();
        await page.waitForTimeout(800);
        await page.locator('input[data-pay]').first().fill('100000');
        await page.locator(`${dlg} button:has-text("Simpan Transaksi")`).click();
        await page.waitForTimeout(1000);
      },
      marks: [
        { sel: `${dlg} .font-mono.font-bold`, text: 'Nomor nota yang tersimpan.' },
        { sel: `${dlg} dl`, text: 'Ringkasan: total, uang diterima, dan kembalian (atau piutang untuk transaksi kredit).' },
        { sel: `${dlg} button:has-text("Transaksi Baru")`, text: 'Transaksi Baru: kosongkan layar untuk pelanggan berikutnya. Enter juga bisa.' },
        { sel: `${dlg} button:has-text("Cetak")`, text: 'Cetak struk (atau cetak ulang bila perlu).' },
        { sel: `${dlg} summary`, text: 'Pengaturan struk di komputer ini: cetak otomatis, lebar kertas 58/80 mm, cara cetak, dan tes printer.' }
      ],
      tip: 'Gambar ini memakai nomor nota contoh. Struk dicetak langsung ke printer lewat print-agent, atau lewat browser bila print-agent tidak aktif.',
      after: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
      }
    }
  ]),

  modern('hari-ini-shift', 'Penjualan hari ini, kas kecil, dan tutup shift', 'Pantau nota hari ini, catat uang keluar/masuk laci, dan tutup shift di akhir hari.', [
    {
      title: 'Penjualan hari ini (F9)',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await page.keyboard.press('F9');
        await page.waitForTimeout(2000);
      },
      marks: [
        { sel: `${dlg} input[aria-label="Masukkan No Transaksi"]`, text: 'Cari nota dengan nomor transaksi.' },
        { sel: `${dlg} button:has-text("Cetak Rekap")`, text: 'Cetak rekap penjualan Anda (per metode, uang lain, isi laci) ke printer struk.' },
        { sel: `${dlg} tbody tr:first-child button[aria-label="Lihat detail"]`, text: 'Lihat detail isi nota.' },
        { sel: `${dlg} tbody tr:first-child button[aria-label="Cetak ulang"]`, text: 'Cetak ulang struk (tercatat sebagai salinan).' }
      ],
      tip: 'Dialog ini juga menampilkan total per metode dan uang yang seharusnya ada di laci.',
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Kas kecil',
      before: async (page) => {
        await page.locator('button[aria-label="Bentangkan panel outlet"]').click();
        await page.locator('aside button:has-text("Kas Kecil")').click();
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: `${dlg} button:has-text("Kas keluar")`, text: 'Pilih Kas keluar (uang keluar laci) atau Kas masuk (menambah uang laci).' },
        { sel: `${dlg} button:has-text("Operasional")`, text: 'Pilih kategori.' },
        { sel: '#petty-amount', text: 'Jumlah uang.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Simpan. Kas keluar di atas batas tertentu butuh PIN Owner/Supervisor.' }
      ],
      tip: 'Catatan kas kecil ikut dihitung saat Tutup Shift supaya uang laci cocok.',
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Tutup shift',
      height: 1800,
      before: async (page) => {
        await page.locator('aside button:has-text("Tutup Shift")').click();
        await page.waitForTimeout(2000);
      },
      marks: [
        { sel: `${dlg} input[aria-label^="Fisik / bukti"]`, text: 'Isi hasil hitung fisik per metode: uang tunai di laci, dan jumlah menurut struk EDC / mutasi QRIS / transfer.' },
        { sel: `${dlg} button:has-text("Isi sama dengan seharusnya")`, text: 'Bila yakin cocok, klik ini untuk mengisi sama dengan angka sistem.' },
        { sel: `${dlg} button:has-text("Rincian seharusnya")`, text: 'Lihat dari mana angka "Seharusnya" berasal.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Tutup Shift. Bila ada selisih, wajib isi catatan dan minta persetujuan PIN.' }
      ],
      tip: 'Setelah ditutup, Anda bisa mencetak struk tutup shift atau langsung membuka shift baru.',
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  modern('shortcut', 'Tombol pintas (shortcut) keyboard', 'Kasir berpengalaman bisa bekerja tanpa mouse. Tekan F1 kapan saja untuk melihat daftar ini.', [
    {
      title: 'Daftar shortcut',
      goto: '/kasir',
      before: async (page) => {
        await fresh(page, '/kasir');
        await page.keyboard.press('F1');
        await page.waitForTimeout(600);
      },
      marks: [
        { sel: `${dlg} h3:has-text("Layar kasir")`, text: 'Layar kasir: F2 cari, F3 QTY, F4 member, F6 salesman, Ctrl+K kupon, F7 pending, F8 daftar pending, F9 penjualan hari ini, F10 atau Ctrl+Enter bayar, Esc kosongkan pencarian.' },
        { sel: `${dlg} h3:has-text("Baris keranjang")`, text: 'Baris keranjang: Ctrl+↑/↓ pilih baris, Ctrl+ dan − ubah jumlah, Ctrl+E ubah harga, Ctrl+Delete hapus.' },
        { sel: `${dlg} h3:has-text("Dialog bayar")`, text: 'Dialog bayar: F1 Tunai, F2 Kredit, F3 Nontunai, F4 Split, End simpan transaksi.' }
      ],
      tip: 'Shortcut tidak berlaku saat dialog terbuka; dialog punya tombolnya sendiri.'
    }
  ]),

  // ============================== KASIR CLASSIC ==============================
  classic(
    'layar',
    'Mengenal layar Kasir Classic',
    'Tampilan Classic bergaya aplikasi desktop: isian nota di atas, tabel keranjang di tengah, dan tombol aksi di bagian bawah. Cocok untuk kasir yang terbiasa memakai keyboard.',
    [
      {
        title: 'Kepala dan isian nota',
        goto: '/kasirb',
        before: (page) => fresh(page, '/kasirb'),
        marks: [
          { sel: 'header h1', text: 'Nama toko, outlet, dan kode outlet.' },
          { sel: 'header .font-mono', text: 'Jam dan tanggal berjalan.' },
          { sel: 'header button[aria-label="Bahasa"]', text: 'Ganti bahasa.' },
          { sel: 'header button[aria-label="Ganti tema"]', text: 'Ganti tema terang/gelap.' },
          { sel: 'button.cl-field:has-text("Member Umum")', text: 'Pelanggan: klik (F4) untuk memilih member. Kosong = Member Umum.' },
          { sel: 'button.cl-field:has-text("Salesman Umum")', text: 'Salesman nota (F6).' },
          { sel: 'output:has-text("Nota baru")', text: 'Nomor nota; baru terisi setelah nota disimpan.' }
        ]
      },
      {
        title: 'Kode item, total, dan tabel keranjang',
        marks: [
          { sel: '#pos-qty', text: 'QTY (F3): jumlah untuk barang berikutnya.' },
          { sel: SEARCH, text: 'Kode item (F2): ketik, atau pindai barcode, lalu Enter.' },
          { sel: 'button:has-text("Daftar Barang")', text: 'Buka daftar barang hasil pencarian.' },
          { sel: 'button[aria-label="Pilih barang cepat (tanpa pindai)"]', text: 'Barang cepat: 16 tombol barang favorit.' },
          { sel: 'button:has-text("Tampilan Kasir")', text: 'Pindah ke tampilan Kasir biasa.' },
          { sel: '.cl-stat >> nth=0', text: 'Poin yang akan didapat member, jumlah baris barang, dan jumlah seluruh QTY.' },
          { sel: 'div.font-display:has-text("TOTAL") ~ div .font-mono, span.font-mono.font-extrabold', text: 'Total belanja berukuran besar.' }
        ]
      },
      {
        title: 'Bagian bawah layar',
        marks: [
          { sel: 'input[aria-label="Biaya Lain-Lain"]', text: 'Biaya lain-lain, pajak toko, dan pajak negara.' },
          { sel: 'button:has-text("HITUNG PAJAK")', text: 'HITUNG PAJAK dan BATAL HITUNG.' },
          { sel: 'input[placeholder^="Ketikkan keterangan"]', text: 'Keterangan nota.' },
          { sel: 'footer button:has-text("Ctrl + S")', text: 'Simpan (Ctrl+S) untuk membayar.' },
          { sel: 'footer button:has-text("F7")', text: 'Pending (F7).' },
          { sel: 'footer button:has-text("F8")', text: 'Daftar nota pending (F8).' },
          { sel: 'footer button:has-text("F9")', text: 'Penjualan hari ini (F9).' },
          { sel: 'footer button:has-text("F5")', text: 'Segarkan daftar barang & stok (F5).' },
          { sel: 'footer button:has-text("Kas Kecil")', text: 'Kas kecil.' },
          { sel: 'footer button:has-text("Tutup Shift")', text: 'Tutup shift.' },
          { sel: 'footer a[href="/dashboard"], footer button:has-text("Keluar")', text: 'Keluar dari kasir.' }
        ],
        tip: 'Bilah bawah selalu menempel di dasar layar sehingga tombol Simpan tidak perlu dicari.'
      }
    ]
  ),

  classic('buka-shift', 'Membuka shift (saldo awal kasir)', 'Seperti tampilan biasa, shift harus dibuka sebelum nota disimpan. Di Classic dialognya bernama Kas Saldo Awal Kasir.', [
    {
      title: 'Kas saldo awal',
      goto: '/kasirb',
      before: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
        await noShift(page);
        await page.reload({ waitUntil: 'domcontentloaded' });
        await page.waitForTimeout(2200);
        await page.locator('#shift-open-amount').fill('200000');
      },
      marks: [
        { sel: `${dlg} dl`, text: 'Nama kasir dan lokasi outlet yang membuka shift.' },
        { sel: '#shift-open-amount', text: 'Isi uang kembalian yang ada di laci (saldo awal).' },
        { sel: `${dlg} button[type="submit"]`, text: 'Klik Simpan Saldo Awal. Shift terbuka dan Anda bisa menyimpan nota.' }
      ],
      after: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
        await page.reload({ waitUntil: 'domcontentloaded' });
      }
    }
  ]),

  classic('cari-barang', 'Mencari dan menambah barang', 'Ketik kode atau nama barang, pilih dari daftar dengan panah keyboard, lalu Enter. Pindai barcode juga langsung memasukkan barangnya.', [
    {
      title: 'Ketik di kolom Kode item',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await page.locator('#pos-qty').fill('3');
        await page.locator(SEARCH).fill('air mineral');
        await page.waitForTimeout(1500);
      },
      marks: [
        { sel: '#pos-qty', text: 'Isi QTY bila lebih dari satu (kosong = 1).' },
        { sel: SEARCH, text: 'Ketik sebagian nama atau kode barang. Daftar muncul di bawahnya.' },
        { sel: '#cl-suggest li:first-child', text: 'Gunakan panah atas/bawah untuk menyorot barang, lalu Enter untuk memasukkannya. Terlihat stok dan harga setiap barang. Esc menutup daftar.' }
      ],
      tip: 'Memindai barcode tidak perlu memilih apa pun: barang langsung masuk saat scanner mengirim Enter.'
    },
    {
      title: 'Barang masuk tabel keranjang',
      before: async (page) => {
        await page.locator(SEARCH).fill(DEMO);
        await page.waitForTimeout(1200);
        await page.locator(SEARCH).press('Enter');
        await page.waitForTimeout(1500);
      },
      marks: [
        { sel: '#cart-line-0', text: 'Barang masuk baris pertama (sejumlah QTY). Barang yang sama ditambahkan jumlahnya, bukan baris baru.' },
        { sel: 'div.font-display:has-text("TOTAL") ~ div .font-mono, span.font-mono.font-extrabold', text: 'Total langsung diperbarui.' },
        { sel: '.cl-stat >> nth=1', text: 'Jumlah item (baris) dan jumlah QTY ikut bertambah.' }
      ]
    },
    {
      title: 'Daftar Barang dan Barang cepat',
      before: click('button:has-text("Daftar Barang")'),
      marks: [
        { sel: '#cl-suggest li:first-child', text: 'Tombol Daftar Barang (F2) membuka daftar barang tanpa mengetik. Klik barang untuk memasukkannya.' },
        { sel: 'button[aria-label="Pilih barang cepat (tanpa pindai)"]', text: 'Barang cepat: klik slot terisi untuk memasukkan barang; klik kanan untuk mengganti atau mengosongkan; slot kosong (+) untuk mengisi.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  classic('keranjang', 'Mengatur tabel keranjang', 'Tabel keranjang menampilkan kode, nama, QTY, satuan, harga, diskon, dan sub total. Jumlah bisa diketik langsung di barisnya.', [
    {
      title: 'Kolom dan tombol di setiap baris',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await addDemo(page, '2');
      },
      marks: [
        { sel: '#cart-line-0 input[data-cart-qty]', text: 'Ubah QTY langsung di tabel. Enter pindah ke baris berikutnya; panah atas/bawah naik-turun baris.' },
        { sel: '#cart-line-0 td:nth-child(5)', text: 'Harga per satuan (harga normal atau hasil perubahan).' },
        { sel: '#cart-line-0 td:nth-child(7)', text: 'Sub total baris.' },
        { sel: '#cart-line-0 button[aria-label="Ubah harga / potongan"]', text: 'Ubah harga atau beri potongan (Ctrl+E). Butuh PIN Owner/Supervisor.' },
        { sel: '#cart-line-0 button[aria-label="Hapus baris"]', text: 'Hapus baris (Ctrl+Delete).' }
      ],
      tip: 'Ctrl+↑ / Ctrl+↓ memilih baris, Ctrl dan + / − mengubah jumlah baris terpilih.'
    },
    {
      title: 'Cari di keranjang dan Perluas',
      marks: [
        { sel: '#cl-cart-search', text: 'Cari barang di dalam keranjang (Ctrl+F). Esc atau Enter kembali ke kolom Kode item.' },
        { sel: 'button:has-text("Isi keranjang")', text: 'Tampilkan keranjang sebagai tabel ringkasan lengkap beserta total, biaya, dan pajak.' },
        { sel: 'button:has-text("Perluas")', text: 'Perluas: sembunyikan kepala dan isian nota agar tabel lebih lega. Klik Ciutkan untuk kembali. Layar pendek otomatis diperluas.' }
      ]
    },
    {
      title: 'Stok kurang dan hapus barang',
      before: async (page) => {
        await page.locator('#cart-line-0 input[data-cart-qty]').fill('99');
        await page.locator('#cart-line-0 input[data-cart-qty]').press('Tab');
        await page.waitForTimeout(1500);
      },
      marks: [
        { sel: '#cart-line-0 [role="alert"]', text: 'Jumlah melebihi stok: baris memerah dan nota tidak bisa disimpan sampai jumlahnya dikecilkan.' },
        { sel: 'footer button.cl-icon', text: 'Ikon tong sampah: kosongkan seluruh keranjang.' },
        { sel: 'footer button:has-text("Ctrl + S")', text: 'Tombol Simpan nonaktif selama ada baris bermasalah.' }
      ],
      after: async (page) => {
        await page.locator('#cart-line-0 input[data-cart-qty]').fill('2');
        await page.locator('#cart-line-0 input[data-cart-qty]').press('Tab');
        await page.waitForTimeout(1200);
      }
    },
    {
      title: 'Ubah harga atau potongan',
      before: click('#cart-line-0 button[aria-label="Ubah harga / potongan"]'),
      marks: [
        { sel: `${dlg} [role="tab"]:has-text("Ubah harga")`, text: 'Ubah harga satuan untuk nota ini.' },
        { sel: `${dlg} [role="tab"]:has-text("Potongan")`, text: 'Atau beri potongan per satuan / seluruh baris.' },
        { sel: `${dlg} input[type="password"]`, text: 'Pilih penyetuju lalu masukkan PIN 6 digitnya.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Setujui & Terapkan.' }
      ],
      after: click(`${dlg} button:has-text("Batal")`)
    }
  ]),

  classic('member-kupon', 'Member, tukar poin, kupon, dan salesman', 'Di Classic, member dan salesman dipilih dari kolom isian di bagian atas.', [
    {
      title: 'Memilih member (F4)',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await addDemo(page);
        await page.locator('button.cl-field:has-text("Member Umum")').click();
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: `${dlg} input[type="search"]`, text: 'Cari dengan nama, kode, atau telepon member.' },
        { sel: `${dlg} ul li:first-child button`, text: 'Klik member yang dimaksud.' }
      ],
      after: click(`${dlg} ul li:first-child button`)
    },
    {
      title: 'Member, tukar poin, dan kupon',
      before: async (page) => {
        await page.waitForTimeout(1800);
      },
      marks: [
        { sel: 'button.cl-field.uppercase', text: 'Member terpilih. Level dan saldo poinnya tampil di kanan bawah isian.' },
        { sel: 'button[aria-label="Tukar poin"]', text: 'Tukar poin menjadi potongan.' },
        { sel: 'button[aria-label="Kupon belanja"]', text: 'Kupon belanja (Ctrl+K).' },
        { sel: '.cl-stat >> nth=0', text: 'Poin yang akan didapat dari nota ini.' }
      ],
      tip: 'Cara memakai dialog tukar poin dan kupon sama dengan tampilan Kasir biasa.'
    },
    {
      title: 'Kupon belanja',
      before: click('button[aria-label="Kupon belanja"]'),
      marks: [
        { sel: '#pos-voucher', text: 'Ketik atau pindai kode kupon.' },
        { sel: `${dlg} button:has-text("Pakai")`, text: 'Klik Pakai untuk memeriksa dan menerapkan.' }
      ],
      after: click(`${dlg} button:has-text("Selesai")`)
    },
    {
      title: 'Memilih salesman (F6)',
      before: click('button.cl-field:has-text("Salesman Umum")'),
      marks: [
        { sel: `${dlg} input`, text: 'Pilih salesman, atau kosongkan untuk Salesman Umum.' },
        { sel: `${dlg} button:has-text("Selesai")`, text: 'Klik Selesai.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  classic('biaya-pajak', 'Biaya lain-lain, pajak, dan keterangan', 'Isian ini ada di bagian bawah layar, di atas tombol aksi.', [
    {
      title: 'Biaya, pajak, dan keterangan',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await addDemo(page);
        await page.locator('input[aria-label="Biaya Lain-Lain"]').fill('5000');
        await page.locator('input[placeholder^="Ketikkan keterangan"]').fill('Titip kirim ke rumah');
        await page.locator('button:has-text("HITUNG PAJAK")').click();
        await page.waitForTimeout(1500);
      },
      marks: [
        { sel: 'input[aria-label="Biaya Lain-Lain"]', text: 'Biaya tambahan (ongkir, bungkus). Ikut menambah total.' },
        { sel: 'button[aria-label="Rincian biaya lain-lain"]', text: 'Rincian untuk beberapa jenis biaya (nama + jumlah, maksimal 20 baris).' },
        { sel: 'div:has(> .cl-label:has-text("Pajak Toko")) output', text: 'Pajak Toko (persen ditampilkan di judulnya).' },
        { sel: 'div:has(> .cl-label:has-text("Pajak Negara")) output', text: 'Pajak Negara.' },
        { sel: 'button:has-text("HITUNG PAJAK")', text: 'HITUNG PAJAK: terapkan pajak. Nilainya muncul di dua kolom di kiri.' },
        { sel: 'button:has-text("BATAL HITUNG")', text: 'BATAL HITUNG: lepas pajak.' },
        { sel: 'input[placeholder^="Ketikkan keterangan"]', text: 'Keterangan nota; ikut tercetak di struk.' }
      ],
      after: click('button:has-text("BATAL HITUNG")')
    },
    {
      title: 'Rincian biaya lain-lain',
      before: click('button[aria-label="Rincian biaya lain-lain"]'),
      marks: [
        { sel: `${dlg} input[placeholder^="mis."]`, text: 'Nama biaya.' },
        { sel: `${dlg} input[placeholder="0"]`, text: 'Jumlahnya.' },
        { sel: `${dlg} button:has-text("Tambah biaya")`, text: 'Tambah baris biaya lain.' },
        { sel: `${dlg} button:has-text("Simpan rincian")`, text: 'Simpan rincian.' }
      ]
    }
  ]),

  classic('pending', 'Menyimpan nota pending', 'Parkir keranjang yang belum dibayar, layani pelanggan lain, lalu buka lagi.', [
    {
      title: 'Parkir dengan F7',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await addDemo(page);
      },
      marks: [{ sel: 'footer button:has-text("F7 - PENDING")', text: 'Klik F7 - PENDING (atau tekan F7).' }],
      after: click('footer button:has-text("F7 - PENDING")')
    },
    {
      title: 'Beri keterangan',
      before: async (page) => {
        await page.locator(`${dlg} input`).fill('Bu Ani, baju merah');
      },
      marks: [
        { sel: `${dlg} input`, text: 'Keterangan agar mudah dikenali (boleh kosong).' },
        { sel: `${dlg} button:has-text("Simpan Pending")`, text: 'Simpan Pending. Keranjang langsung kosong.' }
      ],
      after: click(`${dlg} button:has-text("Simpan Pending")`)
    },
    {
      title: 'Daftar nota pending (F8)',
      before: click('footer button:has-text("F8")'),
      marks: [
        { sel: `${dlg} li:first-child input`, text: 'Ubah keterangan.' },
        { sel: `${dlg} li:first-child button:has-text("Buka")`, text: 'Buka: isi nota kembali ke keranjang (keranjang yang sedang berisi otomatis diparkir).' },
        { sel: `${dlg} li:first-child button[aria-label="Hapus"]`, text: 'Hapus nota pending.' }
      ],
      tip: 'Angka kuning di tombol F8 = jumlah nota pending. Nota pending tersimpan di komputer kasir ini.',
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  classic('bayar', 'Membayar dan menyimpan transaksi (Ctrl+S)', 'Simpan membuka dialog pembayaran yang sama dengan tampilan biasa: pilih Tunai, Kredit, Nontunai, atau Split, isi uang diterima, lalu simpan.', [
    {
      title: 'Tombol Simpan (Ctrl+S)',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await addDemo(page);
      },
      marks: [
        { sel: 'div.font-display:has-text("TOTAL") ~ div .font-mono, span.font-mono.font-extrabold', text: 'Periksa total bersama pelanggan.' },
        { sel: 'footer button:has-text("Ctrl + S")', text: 'Klik Ctrl + S - Simpan (atau tekan Ctrl+S / F10). Bila shift belum dibuka, Anda diminta membukanya dulu.' }
      ],
      after: click('footer button:has-text("Ctrl + S")')
    },
    {
      title: 'Dialog pembayaran',
      before: async (page) => {
        await page.locator('input[data-pay]').first().fill('100000');
        await page.waitForTimeout(500);
      },
      marks: [
        { sel: `${dlg} [role="radiogroup"]`, text: 'F1 TUNAI, F2 KREDIT, F3 NONTUNAI, F4 SPLITCASH.' },
        { sel: `${dlg} input[data-pay]`, text: 'Isi uang yang diterima.' },
        { sel: `${dlg} input[aria-label="Kembalian:"]`, text: 'Kembalian dihitung otomatis.' },
        { sel: `${dlg} button:has-text("Simpan Transaksi")`, text: 'Simpan Transaksi (tombol End).' }
      ],
      tip: 'Penjelasan lengkap Nontunai (transfer, kartu, QRIS), Split, dan Kredit ada di panduan "Kasir: Membayar dan menyimpan transaksi"; dialognya sama persis.'
    },
    {
      title: 'Transaksi berhasil',
      before: async (page) => {
        await fakeSaved(page);
        await page.locator(`${dlg} button:has-text("Simpan Transaksi")`).click();
        await page.waitForTimeout(1000);
      },
      marks: [
        { sel: `${dlg} .font-mono.font-bold`, text: 'Nomor nota.' },
        { sel: `${dlg} dl`, text: 'Total, uang diterima, dan kembalian.' },
        { sel: `${dlg} button:has-text("Transaksi Baru")`, text: 'Transaksi Baru: siap untuk pelanggan berikutnya.' },
        { sel: `${dlg} button:has-text("Cetak")`, text: 'Cetak struk atau cetak ulang.' }
      ],
      tip: 'Gambar memakai nomor nota contoh.',
      after: async (page) => {
        await page.unrouteAll({ behavior: 'ignoreErrors' });
      }
    }
  ]),

  classic('hari-ini-shift', 'Penjualan hari ini, kas kecil, dan tutup shift', 'Ketiganya ada di tombol bawah layar.', [
    {
      title: 'Penjualan hari ini (F9)',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await page.locator('footer button:has-text("F9")').click();
        await page.waitForTimeout(2000);
      },
      marks: [
        { sel: `${dlg} input[aria-label="Masukkan No Transaksi"]`, text: 'Cari nota dengan nomor transaksi.' },
        { sel: `${dlg} button:has-text("Cetak Rekap")`, text: 'Cetak rekap penjualan Anda.' },
        { sel: `${dlg} tbody tr:first-child button[aria-label="Lihat detail"]`, text: 'Lihat detail nota.' },
        { sel: `${dlg} tbody tr:first-child button[aria-label="Cetak ulang"]`, text: 'Cetak ulang struk.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Kas kecil',
      before: async (page) => {
        await page.locator('footer button:has-text("Kas Kecil")').click();
        await page.waitForTimeout(1200);
      },
      marks: [
        { sel: `${dlg} button:has-text("Kas keluar")`, text: 'Kas keluar atau Kas masuk.' },
        { sel: '#petty-amount', text: 'Jumlah uang.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Simpan. Kas keluar di atas batas butuh PIN.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Tutup shift',
      height: 1800,
      before: async (page) => {
        await page.locator('footer button:has-text("Tutup Shift")').click();
        await page.waitForTimeout(2000);
      },
      marks: [
        { sel: `${dlg} input[aria-label^="Fisik / bukti"]`, text: 'Isi hasil hitung fisik per metode (tunai dan bukti non-tunai).' },
        { sel: `${dlg} button:has-text("Isi sama dengan seharusnya")`, text: 'Isi sama dengan angka sistem bila cocok.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Tutup Shift. Selisih wajib dicatat dan disetujui PIN.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]),

  classic('shortcut', 'Tombol pintas dan mode Perluas', 'Classic dirancang untuk keyboard. Tombol di bagian bawah layar menampilkan shortcut-nya.', [
    {
      title: 'Daftar shortcut (F1)',
      goto: '/kasirb',
      before: async (page) => {
        await fresh(page, '/kasirb');
        await page.keyboard.press('F1');
        await page.waitForTimeout(600);
      },
      marks: [
        { sel: `${dlg} h3:has-text("Layar kasir")`, text: 'F2 cari, F3 QTY, F4 member, F6 salesman, Ctrl+K kupon, F7 pending, F8 daftar pending, F9 hari ini, F5 segarkan, Ctrl+F cari di keranjang, Ctrl+S atau F10 simpan.' },
        { sel: `${dlg} h3:has-text("Baris keranjang")`, text: 'Ctrl+↑/↓ pilih baris, Ctrl dan + / − ubah jumlah, Ctrl+E ubah harga, Ctrl+Delete hapus.' },
        { sel: `${dlg} h3:has-text("Dialog bayar")`, text: 'F1 Tunai, F2 Kredit, F3 Nontunai, F4 Split, End simpan.' }
      ],
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    },
    {
      title: 'Mode Perluas',
      before: async (page) => {
        await addDemo(page);
        await page.locator('button:has-text("Perluas")').click();
        await page.waitForTimeout(800);
      },
      marks: [
        { sel: 'button:has-text("Ciutkan")', text: 'Mode Perluas: kepala dan isian nota disembunyikan, tabel keranjang lebih tinggi. Klik Ciutkan untuk kembali. Pilihan diingat.' },
        { sel: 'button:has-text("Tampilan Kasir")', text: 'Tombol pindah ke Tampilan Kasir tetap ada di baris Kode item. Tombol Keluar ada di bilah bawah.' }
      ],
      after: click('button:has-text("Ciutkan")')
    }
  ])
);

// Panduan tambahan: membatalkan nota. Dialog hanya dibuka dan diisi alasannya; PIN tidak diisi dan tidak dikirim.
guides.push({
  id: 'kasir-batalkan-nota',
  at: '/sales',
  group: GROUP,
  title: 'Kasir: Membatalkan nota yang sudah tersimpan',
  intro: 'Nota yang salah bisa dibatalkan oleh Owner/Supervisor. Stok, poin, dan kupon nota dikembalikan. Nota tetap tercatat berstatus Dibatalkan dan tidak bisa dipulihkan. Untuk memperbaiki isi nota, gunakan Edit nota.',
  shots: [
    {
      title: 'Buka nota dari daftar penjualan',
      width: 1900,
      goto: '/sales',
      marks: [
        { sel: 'input[type="search"]', text: 'Cari nota dengan nomor nota.' },
        { sel: 'main button.font-mono', text: 'Klik nomor nota yang akan dibatalkan.' }
      ],
      after: click('main button.font-mono')
    },
    {
      title: 'Klik Batalkan nota',
      width: 1900,
      marks: [
        { sel: 'button:has-text("Edit nota")', text: 'Edit nota: untuk membetulkan isi nota (qty, harga) tanpa membatalkannya.' },
        { sel: 'button:has-text("Batalkan nota")', text: 'Batalkan nota: membatalkan seluruh nota.' }
      ],
      after: click('button:has-text("Batalkan nota")')
    },
    {
      title: 'Isi alasan dan persetujuan',
      width: 1900,
      before: async (page) => {
        await page.locator('#void-reason').fill('Salah input barang');
      },
      marks: [
        { sel: '#void-reason', text: 'Tulis alasan pembatalan (wajib).' },
        { sel: `${dlg} [aria-label="Penyetuju"]`, text: 'Pilih Owner/Supervisor yang berizin menyetujui pembatalan.' },
        { sel: '#void-pin', text: 'Masukkan PIN 6 digit milik penyetuju.' },
        { sel: `${dlg} button[type="submit"]`, text: 'Klik tombol Batalkan nota. Tombol aktif setelah alasan, penyetuju, dan PIN terisi.' }
      ],
      tip: 'Penyetuju dan PIN diatur di menu PIN Penyetuju. Pembatalan tercatat di audit.',
      after: async (page) => {
        await page.keyboard.press('Escape');
      }
    }
  ]
});
