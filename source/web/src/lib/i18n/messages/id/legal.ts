export default {
  updated: 'Berlaku sejak Oktober 2026',
  back: 'Kembali',
  nav: 'Dokumen hukum',
  search: {
    placeholder: 'cari di kebijakan ini, mis. cookie, kamera, hapus…',
    placeholderTerms: 'cari di syarat ini, mis. pembayaran, ilegal, hapus…',
    placeholderHelp: 'cari bantuan, mis. shift, printer, stok, QRIS…',
    hint: 'Tekan / untuk mencari',
    count: '{count} kecocokan di {sections} bagian',
    none: '0 kecocokan',
    empty: 'Tidak ada hasil untuk "{q}". Coba kata lain, mis. "data", "cookie", atau "hapus".',
    clear: 'Hapus pencarian'
  },
  terms: {
    docTitle: 'Syarat Layanan | ARUS',
    title: 'Syarat Layanan',
    s1: {
      title: '1. Ringkasan singkat',
      body: `- ARUS adalah layanan kasir, stok, pembelian, laporan, dan akuntansi untuk usaha ritel dan makanan-minuman. Dengan mendaftar atau memakainya, Anda menyetujui syarat ini.
- Data usaha Anda tetap milik Anda. Kami hanya memprosesnya untuk menyediakan layanan (lihat Kebijakan Privasi).
- ARUS hanya boleh dipakai untuk transaksi yang sah dan sesuai hukum yang berlaku di Indonesia dan di negara tempat usaha Anda beroperasi. Transaksi ilegal dilarang keras (bagian 7).
- Kami dapat menangguhkan atau menghentikan akun yang melanggar, dan bekerja sama dengan otoritas yang berwenang.
- Anda bertanggung jawab atas akun, pegawai, transaksi, pembukuan, dan pajak usaha Anda sendiri.`
    },
    s2: {
      title: '2. Penerimaan dan definisi',
      body: `Syarat ini adalah perjanjian antara Anda dan Eraya Digital Solusindo ("kami"). Dengan membuat akun, masuk, atau memakai ARUS, Anda menyatakan telah membaca dan menyetujuinya. Bila Anda mendaftar atas nama usaha, Anda menyatakan berwenang mengikat usaha tersebut.

| Istilah | Arti
| Layanan | ARUS Web, ARUS Mobile (Android), API eksternal, portal developer, dan print-agent beserta pembaruannya
| Tenant atau usaha | Satu akun usaha beserta seluruh outlet, pengguna, dan datanya
| Pengguna | Setiap orang yang diberi akses ke tenant, termasuk pemilik, admin, kasir, dan akun layanan API
| Data Anda | Data usaha, pelanggan, transaksi, stok, dan pembukuan yang Anda masukkan ke layanan
| Platform Admin | Petugas kami yang mengelola layanan secara keseluruhan`
    },
    s3: {
      title: '3. Layanan yang kami sediakan',
      body: `- Kasir (web dan Android), shift, kas kecil, retur, kredit dan piutang, member, serta cetak struk.
- Stok dan master data: ledger stok, opname, mutasi antar cabang, pembelian, hutang, dan harga per cabang.
- Laporan, dasbor, dan akuntansi (SIAK), dengan ekspor Excel dan PDF.
- Fitur opsional: asisten AI HalloArus, API eksternal, pembayaran QRIS lewat EraPay, domain kustom.
- Paket langganan dengan batas dan modul yang berbeda (bagian 5).

Layanan terus dikembangkan. Kami dapat menambah, mengubah, atau menghentikan fitur; perubahan yang berdampak besar kami umumkan lebih dulu lewat aplikasi atau email. Beberapa fitur masih dalam pengembangan dan dapat berubah.`
    },
    s4: {
      title: '4. Akun dan keamanan',
      body: `- Anda harus berusia minimal 18 tahun dan cakap hukum untuk membuat akun.
- Informasi pendaftaran harus benar dan Anda wajib memperbaruinya bila berubah. Satu tenant mewakili satu usaha.
- Pemilik tenant bertanggung jawab atas semua aktivitas di tenantnya, termasuk akun pegawai yang dibuat, peran dan izin yang diberikan, dan PIN penyetuju.
- Jaga kerahasiaan password, PIN, kode 2FA, dan kredensial API. Jangan membagikan akun; buat pengguna terpisah untuk tiap orang.
- Nonaktifkan akun pegawai yang tidak lagi bekerja.
- Segera hubungi kami bila Anda menduga akun disalahgunakan.
- Kami tidak bertanggung jawab atas kerugian akibat kelalaian Anda menjaga kredensial.`
    },
    s5: {
      title: '5. Paket langganan dan pembayaran',
      body: `- Layanan tersedia dalam beberapa paket yang berbeda dalam batas transaksi per hari, jumlah pengguna, dan modul yang dapat diakses. Rincian dan harga terkini tercantum di halaman Langganan.
- Melewati batas paket membuat transaksi atau penambahan pengguna baru ditolak sampai Anda menaikkan paket atau batasnya kembali tersedia. Modul di luar paket Anda tampil terkunci.
- Pembayaran paket dilakukan lewat QRIS atau metode lain yang tersedia. Masa berlaku dan perpanjangan mengikuti keterangan di halaman Langganan.
- Harga dan isi paket dapat berubah. Perubahan tidak memengaruhi periode yang sudah Anda bayar.
- Biaya yang telah dibayar tidak dapat dikembalikan, kecuali diwajibkan hukum atau disetujui tertulis oleh kami.
- Anda bertanggung jawab atas biaya pihak ketiga yang timbul dari pemakaian Anda, misalnya MDR penyedia pembayaran, biaya API AI, dan biaya internet.`
    },
    s6: {
      title: '6. Penggunaan yang diizinkan dan dilarang',
      body: `Anda boleh memakai ARUS untuk mengelola usaha Anda sendiri. Anda tidak boleh:

- mencoba mengakses data tenant lain, melewati batas paket, atau menembus kontrol keamanan, izin, dan Row Level Security;
- mengganggu atau membebani layanan secara tidak wajar, termasuk uji beban atau pemindaian tanpa izin tertulis;
- memakai bot, skrip, atau API di luar yang diizinkan, atau membagikan kredensial API;
- menyalin, membongkar, merekayasa balik, menjual ulang, menyewakan, atau memberi merek lain pada layanan tanpa izin tertulis dari kami;
- mengunggah malware, konten melanggar hak pihak lain, atau konten yang melanggar hukum;
- memasukkan data pribadi pihak lain tanpa dasar hukum yang sah;
- memakai layanan untuk kegiatan yang bertentangan dengan bagian 7.`
    },
    s7: {
      title: '7. Transaksi harus sah dan sesuai hukum',
      body: `ARUS hanya boleh dipakai untuk mencatat dan memproses transaksi yang sah. Anda menyatakan dan menjamin bahwa semua transaksi, barang, jasa, dan pembukuan yang Anda proses lewat ARUS mematuhi hukum Republik Indonesia dan hukum negara setempat tempat usaha Anda beroperasi, termasuk perizinan usaha, perpajakan, perlindungan konsumen, dan perlindungan data pribadi. Bila hukum setempat berbeda, berlaku aturan yang lebih ketat.

Dilarang keras memakai ARUS untuk:

| Larangan | Contoh
| Barang atau jasa terlarang | Narkotika dan zat terlarang, senjata atau bahan peledak ilegal, barang curian atau palsu, satwa dan tumbuhan dilindungi, serta barang yang dilarang diperdagangkan
| Perjudian dan penipuan | Perjudian ilegal, skema piramida atau ponzi, penipuan penjualan, dan pemalsuan transaksi
| Pencucian uang dan pendanaan terorisme | Menyamarkan asal uang, memecah transaksi untuk menghindari pelaporan, atau mendanai kegiatan terlarang (UU No. 8 Tahun 2010 dan UU No. 9 Tahun 2013)
| Pemalsuan pembukuan dan penghindaran pajak | Membuat nota atau jurnal fiktif, menyembunyikan penjualan, atau memanipulasi laporan untuk menghindari kewajiban pajak
| Pelanggaran hak konsumen | Menjual barang kedaluwarsa atau berbahaya, harga menyesatkan, atau menahan hak konsumen (UU No. 8 Tahun 1999)
| Pelanggaran data pribadi dan sistem elektronik | Memakai data pelanggan tanpa dasar sah, akses ilegal, atau penyalahgunaan sistem elektronik (UU No. 27 Tahun 2022 dan UU ITE)
| Pelanggaran sanksi dan perizinan | Bertransaksi dengan pihak yang terkena sanksi atau menjual barang yang memerlukan izin tanpa izin tersebut

Konsekuensi pelanggaran:

- Kami tidak memantau isi setiap transaksi, dan keabsahan transaksi Anda sepenuhnya menjadi tanggung jawab Anda.
- Bila kami mengetahui atau patut menduga adanya pelanggaran, kami dapat langsung membatasi, menangguhkan, atau menghentikan akun tanpa pemberitahuan lebih dulu, serta menahan layanan terkait.
- Kami dapat menyimpan bukti (termasuk audit log) dan memberikannya kepada otoritas yang berwenang, serta bekerja sama dengan aparat penegak hukum, regulator, dan penyedia pembayaran sesuai peraturan.
- Pelanggaran tidak menghapus kewajiban pembayaran Anda dan tidak membatasi hak kami menempuh jalur hukum.
- Anda menanggung seluruh akibat hukum atas transaksi yang Anda lakukan, dan membebaskan kami dari klaim pihak ketiga atau otoritas yang timbul darinya (bagian 16).

Bila Anda melihat penyalahgunaan ARUS, laporkan ke info@erayadigital.co.id.`
    },
    s8: {
      title: '8. Data Anda dan privasi',
      body: `- Data Anda tetap milik Anda. Anda memberi kami izin terbatas untuk menyimpan, memproses, dan mencadangkannya semata-mata untuk menyediakan layanan.
- Untuk data pribadi pelanggan dan pegawai yang Anda masukkan, Anda adalah pengendali data dan kami pemroses data atas nama Anda. Anda wajib memiliki dasar hukum yang sah dan memberi tahu orang yang datanya Anda proses.
- Data tiap tenant dipisahkan di server dan database. Cara kami memproses dan melindungi data dijelaskan di Kebijakan Privasi (/privacy), yang menjadi bagian dari syarat ini.
- Anda dapat mengekspor laporan kapan saja (Excel, PDF, dan NDJSON untuk audit log). Kami menganjurkan ekspor berkala sebagai cadangan Anda sendiri.`
    },
    s9: {
      title: '9. Pembayaran lewat pihak ketiga (QRIS dan EraPay)',
      body: `- Pembayaran non-tunai seperti QRIS diproses oleh penyedia pembayaran pihak ketiga (misalnya EraPay). Kami bukan bank, lembaga keuangan, atau penyelenggara pembayaran, dan tidak memegang dana Anda.
- Penggunaan EraPay tunduk pada syarat penyedia tersebut, termasuk verifikasi merchant (KYC), biaya MDR, dan jadwal pencairan.
- Anda bertanggung jawab memastikan pengaturan metode bayar, MDR, dan nominal di ARUS sesuai perjanjian Anda dengan penyedia pembayaran.
- Status pembayaran bergantung pada penyedia. Fitur "simpan paksa" hanya untuk keadaan gateway tak terjangkau, wajib memakai PIN penyetuju, dan nota ditandai untuk dicek ulang. Anda wajib merekonsiliasinya dengan penyedia.
- Sengketa, pengembalian dana, dan chargeback dengan pelanggan atau penyedia adalah urusan Anda dengan pihak tersebut.`
    },
    s10: {
      title: '10. Fitur AI, laporan, akuntansi, dan pajak',
      body: `- Jawaban asisten AI (HalloArus atau connector MCP) dibuat otomatis dan dapat keliru atau tidak lengkap. Periksa kembali sebelum dipakai mengambil keputusan.
- Hasil AI, dasbor, laporan, dan jurnal otomatis adalah alat bantu. Semuanya bukan nasihat hukum, pajak, keuangan, atau akuntansi.
- Anda dan akuntan Anda bertanggung jawab atas kebenaran pembukuan, laporan keuangan, dan kewajiban pajak usaha Anda, termasuk penerbitan faktur atau bukti pajak yang diwajibkan.
- Struk dan nota dari ARUS adalah bukti transaksi usaha Anda; kami tidak menerbitkannya atas nama kami.`
    },
    s11: {
      title: '11. Kekayaan intelektual dan lisensi',
      body: `- Layanan, kode, desain, merek ARUS, dan dokumentasinya adalah milik kami atau pemberi lisensi kami. Penggunaan kode sumber diatur oleh ARUS Source-Available License pada berkas LICENSE: kode boleh dibaca dan dipakai untuk usaha sendiri, tetapi dilarang dijual ulang tanpa izin.
- Kami memberi Anda hak terbatas, non-eksklusif, tidak dapat dipindahtangankan, dan dapat dicabut untuk memakai layanan selama syarat ini berlaku.
- Anda tetap memiliki data dan konten Anda (nama usaha, logo, gambar barang). Anda menjamin berhak memakainya dan tidak melanggar hak pihak lain.
- Masukan atau saran dari Anda boleh kami pakai untuk memperbaiki layanan tanpa kewajiban apa pun kepada Anda.
- Fitur white label atau domain kustom hanya boleh dipakai sebagaimana diatur Platform Admin, dan tidak memberi Anda kepemilikan atas layanan.`
    },
    s12: {
      title: '12. API eksternal dan integrasi',
      body: `- Kredensial API (kunci dan secret) bersifat rahasia dan tampil hanya sekali. Anda bertanggung jawab menyimpannya dengan aman dan merotasinya bila bocor.
- Akses API dibatasi oleh allowlist IP, tanda tangan HMAC, nonce, dan batas laju. Dilarang menghindari batas ini.
- Setiap permintaan API berjalan sebagai akun layanan dengan izin role-nya; Anda bertanggung jawab atas izin yang diberikan dan atas semua aktivitas yang dilakukan lewat kredensial Anda.
- Kami dapat mengubah, membatasi, atau mencabut akses API, termasuk bila ada penyalahgunaan atau ancaman keamanan.
- Integrasi dengan pihak ketiga yang Anda bangun adalah tanggung jawab Anda.`
    },
    s13: {
      title: '13. Ketersediaan, pemeliharaan, dan pencadangan',
      body: `- Kami berupaya menjaga layanan tetap tersedia, tetapi tidak menjamin layanan bebas gangguan, galat, atau selalu dapat diakses. Belum ada perjanjian tingkat layanan (SLA) kecuali disepakati tertulis.
- Pemeliharaan dan pembaruan dapat membuat layanan terhenti sementara. Kami berusaha melakukannya di waktu sepi dan memberi tahu bila berdampak besar.
- Kami mencadangkan database secara berkala dan sebelum pembaruan sistem. Cadangan ditujukan untuk pemulihan layanan, bukan sebagai arsip pribadi Anda.
- Aplikasi kasir belum mendukung mode offline. Siapkan prosedur cadangan (misalnya pencatatan manual) bila koneksi terputus.
- Printer, jaringan, perangkat, dan listrik di tempat usaha Anda di luar kendali kami.`
    },
    s14: {
      title: '14. Penghentian dan penghapusan akun',
      body: `- Anda dapat berhenti memakai layanan kapan saja. Hubungi kami bila ingin menonaktifkan atau menghapus tenant.
- Kami dapat menangguhkan atau menghentikan akun bila Anda melanggar syarat ini (terutama bagian 6 dan 7), tidak membayar biaya yang jatuh tempo, membahayakan keamanan layanan, atau diwajibkan oleh hukum.
- Tenant yang dinonaktifkan dapat dijadwalkan penghapusan permanen dengan masa tunggu 0 sampai 365 hari. Setelah itu seluruh data tenant dihapus permanen dan tidak dapat dipulihkan dari aplikasi. Rincian ada di Kebijakan Privasi.
- Ekspor data yang Anda perlukan sebelum penghentian, termasuk untuk kewajiban pembukuan dan pajak.
- Ketentuan yang menurut sifatnya tetap berlaku setelah penghentian (misalnya bagian 7, 11, 15, 16, dan 18) tetap mengikat.`
    },
    s15: {
      title: '15. Jaminan dan pembatasan tanggung jawab',
      body: `- Layanan disediakan "sebagaimana adanya" dan "sebagaimana tersedia". Sejauh diizinkan hukum, kami tidak memberi jaminan tersurat maupun tersirat, termasuk kesesuaian untuk tujuan tertentu dan bebas dari galat.
- Sejauh diizinkan hukum, tanggung jawab total kami kepada Anda dibatasi pada biaya layanan yang telah Anda bayarkan dalam 12 bulan sebelum kejadian yang menimbulkan klaim.
- Kami tidak bertanggung jawab atas kerugian tidak langsung, kehilangan keuntungan, kehilangan peluang usaha, atau kehilangan data yang timbul dari penggunaan atau ketidakmampuan memakai layanan, kelalaian Anda, atau tindakan pihak ketiga (termasuk penyedia pembayaran, hosting, dan AI).
- Kami tidak bertanggung jawab atas keabsahan transaksi, pembukuan, atau kepatuhan pajak usaha Anda.
- Tidak ada ketentuan di sini yang membatasi tanggung jawab yang menurut hukum tidak boleh dibatasi.`
    },
    s16: {
      title: '16. Ganti rugi dan keadaan memaksa',
      body: `Anda setuju membebaskan dan mengganti kerugian kami, afiliasi, dan petugas kami atas klaim, denda, kerugian, dan biaya (termasuk biaya hukum yang wajar) yang timbul dari: pelanggaran Anda terhadap syarat ini, transaksi atau kegiatan ilegal yang dilakukan lewat tenant Anda, pelanggaran hak pihak lain, atau pelanggaran terhadap hukum yang berlaku.

Kami tidak bertanggung jawab atas keterlambatan atau kegagalan yang disebabkan keadaan di luar kendali yang wajar, seperti bencana alam, perang, kerusuhan, pemadaman listrik atau jaringan, gangguan penyedia infrastruktur, serangan siber berskala besar, atau perubahan peraturan atau perintah otoritas.`
    },
    s17: {
      title: '17. Perubahan syarat',
      body: 'Kami dapat memperbarui syarat ini. Perubahan penting diumumkan lewat aplikasi atau email, dan tanggal berlaku di bagian atas halaman diperbarui. Penggunaan layanan setelah tanggal berlaku berarti Anda menyetujui syarat yang diperbarui. Bila Anda tidak setuju, hentikan penggunaan layanan.'
    },
    s18: {
      title: '18. Hukum yang berlaku, sengketa, dan kontak',
      body: `- Syarat ini tunduk pada hukum Republik Indonesia. Untuk usaha yang beroperasi di negara lain, hukum setempat yang bersifat wajib tetap berlaku bagi kegiatan usaha Anda di sana.
- Perselisihan diupayakan diselesaikan secara musyawarah lebih dulu. Bila tidak berhasil, perselisihan diselesaikan melalui pengadilan yang berwenang di Indonesia.
- Syarat ini bersama Kebijakan Privasi merupakan keseluruhan perjanjian kita terkait layanan. Bila ada ketentuan yang tidak dapat diberlakukan, ketentuan lain tetap berlaku.
- Kami boleh mengalihkan hak dan kewajiban kami kepada pihak lain dalam rangka reorganisasi usaha, dengan tetap menjaga perlindungan data Anda. Anda tidak boleh mengalihkan akun tanpa persetujuan tertulis kami.
- Bila ada perbedaan antara terjemahan dan versi Bahasa Indonesia, versi Bahasa Indonesia yang berlaku.

Pertanyaan tentang syarat ini: Eraya Digital Solusindo, info@erayadigital.co.id.`
    }
  },
  privacy: {
    docTitle: 'Kebijakan Privasi | ARUS',
    title: 'Kebijakan Privasi',
    s1: {
      title: '1. Ringkasan singkat',
      body: `- Kami hanya memproses data yang diperlukan untuk menjalankan ARUS (kasir, stok, pembelian, laporan, akuntansi) bagi usaha Anda.
- Kami tidak menjual data, tidak menampilkan iklan, dan tidak memakai pelacak atau analitik pihak ketiga di aplikasi web maupun Android.
- Data tiap usaha (tenant) dipisahkan di server dan di database (Row Level Security), sehingga satu usaha tidak bisa melihat usaha lain.
- Password disimpan sebagai hash argon2id, tidak pernah dalam bentuk aslinya.
- Fitur AI (HalloArus) bersifat opsional, dan data pelanggan tidak pernah dikirim ke penyedia AI.
- Anda dapat meminta akses, koreksi, atau penghapusan data lewat info@erayadigital.co.id.`
    },
    s2: {
      title: '2. Siapa kami dan peran kami',
      body: `ARUS dikembangkan dan dioperasikan oleh Eraya Digital Solusindo ("kami"). Peran kami berbeda menurut jenis datanya.

| Jenis data | Peran kami | Artinya
| Akun dan data usaha Anda (pendaftaran, pengguna, langganan) | Pengendali data | Kami menentukan tujuan dan cara pemrosesannya.
| Data pelanggan, transaksi, dan stok yang Anda masukkan | Pemroses data atas nama usaha Anda | Usaha Anda yang menentukan; kami hanya memprosesnya untuk menyediakan layanan, sesuai instruksi Anda.

Bila usaha Anda menyimpan data pribadi pelanggan atau pegawai, usaha Anda bertanggung jawab memberi tahu mereka dan memiliki dasar hukum yang sah untuk memprosesnya.`
    },
    s3: {
      title: '3. Data yang kami kumpulkan',
      body: `| Kategori | Contoh data | Asal
| Akun dan pendaftaran | Nama usaha, nama pemilik, email, nomor HP, nama outlet, password (hash argon2id), peran dan izin | Anda, saat mendaftar
| Pegawai (pengguna) | Nama, email, peran, outlet yang boleh diakses, PIN penyetuju (tersandi), status aktif | Pemilik atau admin usaha
| Keamanan dan jejak | Alamat IP, waktu, aksi yang dilakukan, ID permintaan, percobaan login gagal | Otomatis oleh server
| Teknis | Jenis klien (web atau mobile), bahasa, zona waktu outlet | Otomatis
| Langganan dan pembayaran | Paket, status, riwayat tagihan, ID transaksi QRIS (bukan data kartu) | Sistem dan penyedia pembayaran
| Dukungan | Isi pesan dan alamat email Anda | Anda, bila menghubungi kami`
    },
    s4: {
      title: '4. Data pelanggan dan transaksi usaha Anda',
      body: `Data yang Anda masukkan ke ARUS adalah milik usaha Anda. Kami tidak memakainya untuk tujuan lain selain menyediakan layanan.

| Modul | Contoh data
| Penjualan dan kasir | Nota, baris barang, harga, pembayaran per metode, kasir, shift, kas kecil
| Member | Nama, jenis kelamin, telepon, email, alamat, poin, deposit, level
| Piutang dan hutang | Pihak, jumlah, cicilan, jatuh tempo, pelunasan
| Stok dan pembelian | Barang, gambar, pemasok, pembelian, opname, mutasi
| Akuntansi (SIAK) | Bagan akun, jurnal, rekonsiliasi bank

ARUS tidak dirancang untuk menyimpan NIK, data biometrik, atau nomor kartu pembayaran. Mohon tidak memasukkannya ke kolom catatan.`
    },
    s5: {
      title: '5. Tujuan dan dasar pemrosesan',
      body: `- Menyediakan fitur ARUS: kasir, stok, pembelian, laporan, dan akuntansi.
- Mengautentikasi pengguna dan mencegah penyalahgunaan (batas percobaan login, pembatasan laju, 2FA untuk admin platform).
- Mencatat audit perubahan penting: siapa, melakukan apa, kapan, dan dari alamat IP mana.
- Mengirim email transaksional seperti verifikasi email dan reset password.
- Mengelola langganan dan pembayaran paket.
- Menjaga dan memperbaiki layanan, misalnya dari log galat teknis.
- Memenuhi kewajiban hukum, pembukuan, dan penyelesaian sengketa.

Dasar hukumnya adalah pelaksanaan perjanjian layanan dengan Anda, persetujuan Anda, kepentingan sah kami menjaga keamanan layanan, dan kewajiban hukum. Kami tidak menjual data dan tidak membuat profil untuk iklan.`
    },
    s6: {
      title: '6. Cookie dan penyimpanan di browser',
      body: `Web ARUS hanya memakai satu cookie esensial dan sedikit penyimpanan lokal. Tidak ada cookie iklan atau analitik, sehingga tidak ada banner persetujuan cookie.

| Nama atau fungsi | Jenis | Isi dan masa berlaku
| Cookie pembaruan sesi | Cookie esensial: HttpOnly, Secure, SameSite=Lax, jalur /auth | Token pembaruan sesi. Berlaku 12 jam, atau 30 hari bila Anda memilih tetap masuk. Tidak dapat dibaca oleh JavaScript.
| Token akses | Memori tab | Berumur 15 menit dan hilang saat tab ditutup.
| Preferensi tampilan | Penyimpanan lokal browser | Tema terang atau gelap, pilihan tampilan kasir (modern atau klasik), dan bahasa.
| Draf formulir dan filter | Penyimpanan browser | Isian yang belum disimpan dan filter halaman, agar tidak hilang saat berpindah tab. Hanya ada di perangkat Anda.

Anda dapat menghapus semuanya lewat pengaturan browser; Anda hanya perlu masuk kembali.`
    },
    s7: {
      title: '7. Aplikasi seluler (ARUS Mobile)',
      body: `| Izin Android | Kegunaan | Bila ditolak
| Kamera | Memindai barcode barang. Gambar kamera tidak direkam, disimpan, atau dikirim. | Scan kamera tidak tersedia; ketik kode atau pakai pemindai HID.
| Perangkat di sekitar (Bluetooth) | Menyambung ke printer struk yang sudah dipasangkan di pengaturan Android. | Struk tidak bisa dicetak lewat Bluetooth.
| Internet | Berkomunikasi dengan server ARUS lewat HTTPS. | Aplikasi tidak dapat dipakai (belum ada mode offline).

Yang tersimpan di perangkat:

- Token pembaruan sesi di penyimpanan aman Android (Keystore); dihapus saat Anda keluar.
- Preferensi tampilan, bahasa, dan pengaturan printer.
- Keranjang tersimpan dan nota pending per pengguna dan cabang (hanya barang dan jumlah; total selalu dihitung ulang server).
- Semua data lokal terhapus saat aplikasi dicopot.

Pembayaran QRIS diproses oleh penyedia pembayaran yang Anda hubungkan; kami tidak menyimpan data kartu. Kami tidak memakai SDK iklan atau analitik dan tidak melacak Anda lintas aplikasi. APK unduhan langsung memeriksa nomor versi terbaru ke server ARUS dan mengunduh pembaruan dari server yang sama; versi Google Play diperbarui lewat Google Play.`
    },
    s8: {
      title: '8. Berbagi data dan pihak ketiga',
      body: `Data hanya dibagikan seperlunya, di bawah kewajiban kerahasiaan, atau bila diwajibkan hukum. Data tidak dijual.

| Penerima | Data | Tujuan
| Penyedia hosting, server, dan jaringan | Lalu lintas terenkripsi dan data yang tersimpan | Menjalankan layanan
| Penyedia email (SMTP) | Alamat email dan isi email transaksional (tautan verifikasi atau reset) | Mengirim email
| EraPay (QRIS) | Nominal, referensi transaksi, dan data merchant untuk aktivasi; tanpa data kartu | Pembayaran QRIS dan upgrade paket
| Penyedia AI pilihan Anda (opsional) | Ringkasan angka yang dianonimkan | Fitur HalloArus (lihat bagian 9)
| Otoritas yang berwenang | Sesuai perintah resmi | Kewajiban hukum

Admin Platform ARUS dapat membuka akun usaha untuk keperluan dukungan lewat fitur "masuk sebagai". Akses ini hanya untuk admin berwenang dengan 2FA, dan tercatat di audit platform.`
    },
    s9: {
      title: '9. Fitur AI (HalloArus dan connector MCP)',
      body: `- Fitur ini opsional dan hanya tersedia bagi pengguna yang diberi izin.
- Yang dikirim ke penyedia AI hanya ringkasan angka (misalnya omzet, stok, dan tren). Pelanggan tampil sebagai "Pelanggan 1, 2, …"; nama, telepon, dan alamat tidak pernah dikirim.
- Anda memilih penyedia (Claude, ChatGPT, Gemini, atau Qwen) dan memakai API key milik usaha Anda atau akun gratis ARUS. Kebijakan privasi penyedia yang dipilih berlaku untuk data yang mereka terima.
- Connector MCP memungkinkan asisten dari langganan Anda membaca data ARUS dengan izin yang sama dengan akun Anda, hanya baca. Token akses berlaku 1 jam dan token pembaruan 30 hari.`
    },
    s10: {
      title: '10. Keamanan',
      body: `- Koneksi memakai HTTPS; produksi mewajibkan alamat https.
- Password di-hash dengan argon2id. Token akses berumur 15 menit, token pembaruan berotasi tiap dipakai, dan login dikunci bertahap setelah percobaan gagal.
- Data tiap usaha dipisahkan dengan Row Level Security di PostgreSQL. Tenant, outlet, dan pengguna selalu diambil dari token di server, bukan dari isian klien.
- Izin per peran dan per outlet, PIN penyetuju untuk tindakan sensitif, dan 2FA (TOTP) wajib untuk Admin Platform.
- Audit log bersifat tambah-saja (append-only) dan tidak bisa diubah.
- Database dicadangkan otomatis sebelum setiap pembaruan sistem.

Tidak ada sistem yang sepenuhnya kebal. Jaga kerahasiaan password dan PIN Anda, dan segera laporkan bila menduga ada penyalahgunaan. Bila terjadi kebocoran data pribadi, kami memberi tahu pihak terdampak dan otoritas sesuai batas waktu yang ditetapkan peraturan perlindungan data pribadi.`
    },
    s11: {
      title: '11. Retensi dan penghapusan data',
      body: `| Data | Disimpan selama | Setelah itu
| Akun dan data usaha | Selama akun dan langganan aktif | Dihapus bila tenant dihapus
| Sesi login | Pembaruan 12 jam atau 30 hari; akses 15 menit | Kedaluwarsa otomatis, atau dicabut saat keluar
| Audit log usaha | Selama tenant aktif (tidak bisa diubah) | Ikut terhapus bersama tenant
| Audit log platform | Dipertahankan untuk akuntabilitas | Tetap disimpan
| Cadangan database | Sekitar 30 hari pada konfigurasi standar | Terhapus otomatis
| Tenant dinonaktifkan | Admin menjadwalkan hapus dengan masa tunggu 0 sampai 365 hari | Seluruh data tenant dihapus permanen, termasuk gambar

Penghapusan permanen tidak dapat dibatalkan dari aplikasi, dan salinannya bisa tersisa di cadangan sampai tertimpa. Sebelum menghapus usaha, ekspor laporan yang Anda perlukan untuk kewajiban pembukuan dan pajak.`
    },
    s12: {
      title: '12. Hak Anda',
      body: `Sesuai Undang-Undang Nomor 27 Tahun 2022 tentang Pelindungan Data Pribadi, Anda berhak untuk:

- mengakses dan meminta salinan data pribadi Anda;
- memperbaiki data yang keliru atau tidak lengkap;
- meminta penghapusan atau pemusnahan data;
- menarik persetujuan, serta membatasi atau menolak pemrosesan tertentu;
- memperoleh data Anda dalam format yang dapat dipakai ulang (laporan dapat diekspor ke Excel atau PDF, dan audit log ke NDJSON);
- mengajukan keberatan atau pengaduan kepada kami dan kepada otoritas yang berwenang.

Kirim permintaan ke info@erayadigital.co.id dari alamat email akun Anda. Kami memverifikasi identitas Anda lebih dulu dan menjawab dalam waktu yang wajar sesuai peraturan. Untuk data pelanggan sebuah toko, hubungi pemilik usaha tersebut sebagai pengendali datanya; kami akan membantu mereka memenuhi permintaan Anda.`
    },
    s13: {
      title: '13. Anak-anak',
      body: 'ARUS ditujukan bagi pelaku usaha dan tidak diperuntukkan bagi anak di bawah 18 tahun. Bila kami mengetahui data anak terdaftar tanpa kewenangan yang sah, kami akan menghapusnya.'
    },
    s14: {
      title: '14. Lokasi data dan transfer lintas negara',
      body: 'Data disimpan di server yang dipilih operator layanan. Bila penyedia pihak ketiga yang Anda gunakan (misalnya email atau AI) memproses data di luar Indonesia, transfer itu mengikuti peraturan perlindungan data pribadi yang berlaku dan dilindungi kewajiban kontraktual.'
    },
    s15: {
      title: '15. Perubahan kebijakan dan kontak',
      body: `Perubahan penting pada kebijakan ini diumumkan lewat aplikasi atau email, dan tanggal berlaku di bagian atas halaman diperbarui. Penggunaan berkelanjutan berarti Anda menerima kebijakan yang diperbarui.

Pertanyaan atau permintaan terkait privasi: Eraya Digital Solusindo, info@erayadigital.co.id.`
    }
  },
  help: {
    docTitle: 'Pusat Bantuan | ARUS',
    title: 'Pusat Bantuan',
    s1: {
      title: '1. Mulai cepat',
      body: `Urutan paling singkat dari daftar sampai nota pertama:

- Daftar di halaman Register: isi nama usaha, nama pemilik, email, nomor HP, nama outlet, dan password. Anda otomatis menjadi Owner.
- Verifikasi email lewat tautan yang dikirim ke email Anda.
- Siapkan data dasar di Master Data: satuan, kategori, dan metode pembayaran. Tunai sudah ada bawaan.
- Tambahkan barang di Daftar Item (kode, barcode, harga, satuan). Import massal dari Excel atau CSV belum tersedia.
- Isi stok awal lewat menu Saldo Awal Stok. Ini dicatat sebagai saldo awal dan hanya bisa dilakukan sekali; berikutnya pakai Stok Opname.
- Buka layar kasir (/kasir), buka shift dengan modal awal, lalu mulai berjualan.
- Tambahkan kasir di menu Pengguna dengan role yang sesuai, dan atur PIN penyetuju di menu PIN Penyetuju.`
    },
    s2: {
      title: '2. Peta menu',
      body: `| Menu | Untuk apa
| Dashboard | Ringkasan "toko baik-baik saja?": omzet, tren, barang terlaris, stok menipis, perbandingan cabang
| Penjualan Langsung | Penjualan hari ini yang terus diperbarui secara real-time
| Daftar Item dan Kartu Stok | Master barang dan riwayat keluar-masuk stok dengan saldo berjalan
| Data Personal | Member, suplier, dan sales
| Data Pendukung | Satuan, kategori, level member, metode pembayaran, brand, principal
| Penjualan | Daftar penjualan, retur penjualan, dan kupon belanja
| Pembelian dan hutang | Faktur beli, retur beli, dan pembayaran hutang pemasok
| Laporan | Penjualan, pembelian, retur, hutang, piutang, dan persediaan, dengan ekspor Excel dan PDF
| SIAK | Akuntansi: kode akun, jurnal, buku besar, neraca, laba rugi, kas dan bank
| HalloArus | Asisten AI hanya-baca untuk bertanya soal angka toko
| Pengguna, Role dan Hak Akses, Outlet | Mengatur orang, izin, dan cabang
| Audit Log | Jejak siapa mengubah apa dan kapan
| Langganan | Paket, batas pemakaian, dan upgrade

Menu yang tidak muncul biasanya karena role Anda belum diberi izin, atau modulnya tidak termasuk di paket Anda (tampil bergembok).`
    },
    s3: {
      title: '3. Masuk dan akun',
      body: `- Masuk dengan email dan password. Centang "Tetap masuk" agar sesi berlaku 30 hari; tanpa itu sesi berlaku 12 jam.
- Lupa password: pakai tautan "Lupa kata sandi?" di halaman masuk. Tautan reset dikirim ke email akun.
- Verifikasi email: periksa folder spam bila belum menerima. Beberapa fitur, seperti mengaktifkan EraPay, membutuhkan email yang sudah terverifikasi.
- Akun dikunci sementara setelah beberapa kali salah password (bawaan 5 kali; kunci 1, 5, 15, lalu 60 menit). Tunggu waktu yang tertera lalu coba lagi.
- Pegawai ditambahkan oleh Owner atau admin di menu Pengguna. Nonaktifkan akun pegawai yang sudah tidak bekerja.
- Jangan membagikan akun. Buat satu pengguna untuk tiap orang agar audit log akurat.`
    },
    s4: {
      title: '4. Kasir web',
      body: `Alur dasar: cari atau pindai barang, periksa keranjang, tekan Bayar, pilih cara bayar, lalu cetak struk.

- Dua tampilan: /kasir (modern) dan /kasirb (klasik gaya desktop). Logikanya sama; pilihan Anda diingat.
- Pindai barcode dengan pemindai USB (langsung masuk keranjang bila kodenya persis), atau ketik nama atau kode di kotak cari.
- Cara bayar: tunai, kredit (DP boleh kosong), non-tunai, dan split (campur beberapa metode). Harga, pajak, dan total dihitung server.
- Nota bisa disimpan sebagai pending lalu dibuka lagi.
- Member dan poin, salesman, kupon, dan biaya lain dapat ditambahkan di nota.

Pintasan keyboard (tekan F1 di kasir untuk melihat daftarnya):

| Tombol | Fungsi
| F1 | Tampilkan daftar pintasan
| F2 | Cari atau pindai barang
| F3 | Isi QTY sebelum pindai
| F4 | Pilih member
| F6 | Pilih salesman
| F7 | Simpan sebagai nota pending
| F8 | Daftar nota pending
| F9 | Penjualan hari ini
| F10 | Bayar
| Esc | Kosongkan pencarian atau tutup dialog

Di dialog bayar: F1 tunai, F2 kredit, F3 non-tunai, F4 split.`
    },
    s5: {
      title: '5. Shift dan kas kecil',
      body: `- Nota hanya bisa disimpan saat shift terbuka. Bila muncul pesan "Buka shift kasir dulu", buka shift dengan mengisi modal awal.
- Saat menutup shift, hitung uang fisik per metode pembayaran. Bila ada selisih, catatan wajib diisi dan perlu PIN penyetuju.
- Struk tutup shift dan rekap harian dapat dicetak untuk serah terima.
- Kas kecil (kas masuk dan keluar) dicatat per shift; nominal di atas batas tertentu memerlukan persetujuan dengan PIN.
- Satu kasir sebaiknya satu shift pada satu waktu agar laci kas mudah direkonsiliasi. Laporan "Per Kasir" menampilkan selisih tiap shift.`
    },
    s6: {
      title: '6. Harga, PIN penyetuju, dan hak akses',
      body: `- Harga dihitung server dari master harga. Kasir tidak bisa mengubah harga sembarangan.
- Mengubah harga atau memberi potongan di baris nota memerlukan Owner atau Supervisor sebagai penyetuju, dengan PIN-nya. Pemakaian PIN tercatat di audit log.
- Atur PIN Anda di menu PIN Penyetuju. Terlalu banyak PIN salah mengunci PIN sementara.
- Izin diatur per role di Role dan Hak Akses (lihat, tambah, ubah, hapus, setujui) dan dapat dibatasi per outlet.
- Pembatalan atau perubahan nota dilakukan lewat revisi, sehingga riwayat nota asli tetap tersimpan.`
    },
    s7: {
      title: '7. Cetak struk',
      body: `| Cara | Kapan dipakai | Kebutuhan
| Langsung ke printer (ESC/POS) | Disarankan: huruf tajam, cepat, kertas terpotong otomatis | print-agent terpasang di PC kasir
| Lewat browser | Cadangan bila agent tidak ada | Chrome dengan opsi --kiosk-printing
| Bluetooth (Android) | Aplikasi ARUS Mobile | Printer thermal Bluetooth yang sudah dipasangkan di pengaturan Android

Memasang print-agent di PC kasir:

- Pasang driver printer, jadikan printer default, dan uji "Print Test Page".
- Ekstrak arus-print-agent-windows.zip lalu klik dua kali pasang.cmd. Tidak ada jendela yang terbuka, itu normal.
- Klik dua kali uji-cetak.cmd; printer harus mencetak halaman uji. Bila tidak, periksa berkas print-agent.log di %LOCALAPPDATA%\\ArusPrintAgent.
- Di aplikasi, buka Pengaturan struk dan tekan Tes printer. Pilih cara cetak Otomatis agar beralih ke browser bila agent mati.

Panduan lengkapnya ada di berkas PC-KASIR.md di repositori.`
    },
    s8: {
      title: '8. Stok',
      body: `- Stok adalah ledger: setiap perubahan tercatat sebagai gerakan dan saldo selalu bisa ditelusuri di Kartu Stok.
- Ada tiga bucket per cabang: Display, Gudang, dan Retur.
- Stok awal: isi sekali lewat Saldo Awal Stok; setelah dikunci, koreksi dilakukan dengan Stok Opname.
- Trx Opname Barang: cocokkan hitungan fisik dengan sistem (per sesi atau langsung). Selisih tercatat sebagai penyesuaian.
- Mutasi Stok Barang: pindahkan barang antar cabang atau bucket (kirim, lalu terima). Selisih penerimaan tercatat.
- Pecah Satuan: ubah satuan besar ke satuan kecil, misalnya dus ke pcs.
- Pesan "Stok tidak mencukupi" berarti saldo bucket kurang. Isi stok lewat pembelian, mutasi, atau opname.
- Retur pembelian mengambil barang dari bucket Retur, jadi mutasikan barang ke bucket itu lebih dulu.`
    },
    s9: {
      title: '9. Pembelian, hutang, dan retur',
      body: `- Faktur pembelian bisa tunai atau kredit, dengan diskon baris bertingkat dan biaya lain. HPP dihitung rata-rata tertimbang per cabang.
- Hutang pemasok dapat dicicil atau dilunasi kolektif, dengan aging hutang di laporan.
- Retur penjualan dibuat dari nota asli; stok dan piutang ikut menyesuaikan. Retur tidak bisa dibatalkan setelah hutang atau piutang notanya dibayar.
- Mengubah atau membatalkan faktur dilakukan lewat revisi.
- Kredit pemasok (dana kembali dari retur) dapat dipakai membayar hutang.`
    },
    s10: {
      title: '10. Member, piutang, dan kupon',
      body: `- Member punya level berdasarkan poin, poin dapat ditukar saat bayar, dan deposit dapat dipakai sebagai metode pembayaran.
- Penjualan kredit membuat piutang. Piutang bisa dicicil atau dilunasi kolektif dan batas kreditnya diatur per member; melewati batas butuh persetujuan penyetuju.
- Kupon belanja dibuat di menu Kupon; kode yang sudah pernah dipakai tidak bisa diubah.
- Laporan piutang menampilkan piutang beredar, pembayaran, saldo per member, dan buku bantu.`
    },
    s11: {
      title: '11. Pembayaran QRIS (EraPay)',
      body: `- Pemilik mengaktifkan EraPay di menu akun lewat Ekosistem. Email Owner harus sudah terverifikasi, dan merchant baru menunggu peninjauan EraPay.
- Pasang API key EraPay di web (per tenant). Sistem memeriksa ulang kuncinya saat disimpan.
- Di kasir pilih metode QRIS lalu Buat QR. Pelanggan memindai; layar memeriksa status tiap beberapa detik dan nota tersimpan otomatis saat lunas.
- Bila EraPay tidak terjangkau, tersedia "Simpan paksa" yang membutuhkan PIN penyetuju. Nota ditandai untuk dicek ulang dan sistem merekonsiliasinya otomatis.
- Biaya MDR metode bayar diatur di menu Data Metode Pembayaran, termasuk aturan transaksi kecil.`
    },
    s12: {
      title: '12. Laporan, dasbor, dan akuntansi',
      body: `- Laporan Penjualan: per faktur, per faktur detail, per barang, per kasir, omset pelanggan, pelanggan per faktur, dan per tanggal.
- Ada juga Laporan Pembelian, Retur, Hutang, Piutang, Master, dan nilai Persediaan.
- Semua laporan bisa diekspor ke Excel dan PDF. Rentang tanggal maksimal 366 hari; pakai filter outlet, kasir, atau kategori untuk mempersempit.
- Dasbor menampilkan ringkasan dan peringatan, misalnya stok menipis.
- SIAK: jurnal dibuat sebagai draf lalu diposting oleh akuntan (tidak bisa diubah setelah posting). Penjualan dapat dijurnalkan otomatis. Tersedia buku besar, neraca saldo, laba rugi, neraca, kas dan bank, rekonsiliasi bank, dan tutup tahun.
- HalloArus menjawab pertanyaan tentang angka toko. Data pelanggan tidak dikirim ke penyedia AI.`
    },
    s13: {
      title: '13. Aplikasi Android (ARUS Mobile)',
      body: `- Unduh APK dari halaman masuk web, atau pasang dari Google Play bila sudah tersedia. Masuk dengan akun yang sama dengan web.
- Dialog server di halaman masuk memungkinkan memilih Produksi atau alamat lain.
- Izinkan Kamera untuk memindai barcode, dan Perangkat di sekitar untuk printer Bluetooth.
- Pasangkan printer thermal di pengaturan Android lebih dulu, lalu pilih di Pengaturan printer aplikasi (58 atau 80 mm).
- APK unduhan langsung memeriksa versi baru dan menawarkan pembaruan otomatis. Versi Google Play diperbarui lewat Google Play.
- Fitur lengkap: kasir, shift, kas kecil, member, QRIS, kredit, nota pending, dan Penjualan hari ini.
- Belum tersedia: retur penjualan, mode offline, dan printer yang hanya mendukung BLE.`
    },
    s14: {
      title: '14. Langganan dan batas paket',
      body: `- Paket menentukan batas transaksi per hari, jumlah pengguna, dan modul yang bisa dibuka. Lihat rinciannya di menu Langganan.
- Bila muncul "Batas transaksi harian paket Anda tercapai" atau "Batas jumlah pengguna tercapai", naikkan paket atau nonaktifkan pengguna lain.
- Menu bergembok berarti modul itu tidak ada di paket Anda; klik untuk melihat cara upgrade.
- Upgrade dibayar lewat QRIS EraPay dan paket aktif otomatis setelah pembayaran lunas.`
    },
    s15: {
      title: '15. Pemecahan masalah',
      body: `| Gejala | Penyebab umum | Yang bisa dilakukan
| "Email atau password salah" | Salah ketik, atau email belum benar | Cek huruf besar kecil, atau pakai Lupa kata sandi
| "Terlalu banyak percobaan gagal" | Akun dikunci sementara | Tunggu waktu yang tertera, lalu coba lagi
| "Buka shift kasir dulu" | Shift belum dibuka | Buka shift dengan modal awal
| "Stok tidak mencukupi" | Saldo bucket kurang | Isi stok lewat pembelian, mutasi, atau opname
| Diminta PIN penyetuju | Mengubah harga atau melewati batas | Minta Owner atau Supervisor memasukkan PIN
| "PIN terkunci" | Terlalu banyak PIN salah | Tunggu hitung mundur, lalu coba lagi
| "Batas paket tercapai" | Transaksi harian atau pengguna penuh | Upgrade paket di menu Langganan
| Menu tidak muncul | Role tanpa izin, atau modul di luar paket | Minta Owner mengatur Role dan Hak Akses atau upgrade
| Struk tidak tercetak | Print-agent mati, atau printer bukan default | Jalankan uji-cetak.cmd, lihat bagian 7
| "Server unreachable" | Koneksi putus atau server sedang bermasalah | Periksa internet; titik status server di footer menunjukkan kondisinya
| Login berputar di domain sendiri | Konfigurasi domain atau SSL belum tepat | Hubungi administrator server Anda`
    },
    s16: {
      title: '16. Pertanyaan umum',
      body: `- Apakah data toko saya terlihat oleh toko lain? Tidak. Data tiap usaha dipisahkan di server dan database. Rincian di Kebijakan Privasi.
- Apakah bisa untuk banyak cabang? Ya. Satu tenant bisa punya banyak outlet dengan harga, stok, dan izin per cabang.
- Apakah bisa dipakai tanpa internet? Belum. Aplikasi kasir web dan Android belum punya mode offline.
- Apakah data dari aplikasi lama bisa dipindahkan? Tidak ada migrasi otomatis. Toko mulai lewat onboarding: barang, saldo awal stok, dan saldo awal piutang atau hutang.
- Bagaimana mengekspor data? Semua laporan bisa diekspor ke Excel dan PDF, dan audit log ke NDJSON.
- Bagaimana menghapus usaha saya? Hubungi kami. Tenant dinonaktifkan lebih dulu lalu dihapus permanen setelah masa tunggu yang ditentukan.
- Bahasa apa saja yang tersedia? Indonesia, English, dan 日本語. Ganti lewat ikon bahasa di pojok halaman.
- Bolehkah dipakai untuk transaksi apa saja? Hanya yang sah menurut hukum. Lihat Syarat Layanan bagian 7.`
    },
    s17: {
      title: '17. Hubungi kami',
      body: `Butuh bantuan lebih lanjut? Kirim email ke info@erayadigital.co.id. Agar cepat ditangani, sertakan:

- nama usaha dan email akun Anda (jangan kirim password atau PIN);
- halaman atau menu tempat masalah terjadi dan langkah yang Anda lakukan;
- tangkapan layar, serta kode atau pesan galat yang tampil;
- perangkat dan versi yang dipakai (web dan browser, atau ARUS Mobile).

Developer yang ingin mengintegrasikan sistem lewat API dapat membaca dokumentasi di /developers. Aturan penggunaan ada di Syarat Layanan (/terms) dan perlindungan data di Kebijakan Privasi (/privacy).`
    }
  }
};
