package platformadmin

// Katalog checklist QA/QC. Kunci item (Key) disimpan di DB bersama hasilnya, jadi JANGAN diganti nama setelah dipakai;
// item baru cukup ditambahkan, item yang dihapus dari sini hasilnya diabaikan.

// QAItem = satu langkah uji.
type QAItem struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// QAGroup = kelompok item pada satu aplikasi.
type QAGroup struct {
	Key   string   `json:"key"`
	Title string   `json:"title"`
	Items []QAItem `json:"items"`
}

// QAApp = satu aplikasi ARUS yang diuji.
type QAApp struct {
	Key    string    `json:"key"`
	Title  string    `json:"title"`
	Groups []QAGroup `json:"groups"`
}

var qaCatalog = []QAApp{
	{Key: "web", Title: "Web Tenant", Groups: []QAGroup{
		{Key: "auth", Title: "Masuk & akun", Items: []QAItem{
			{"web.auth.login", "Login email + kata sandi berhasil, salah sandi ditolak dengan pesan jelas"},
			{"web.auth.logout", "Keluar menghapus sesi; tombol Back tidak membuka halaman terlindungi"},
			{"web.auth.refresh", "Sesi tetap hidup lewat refresh token; token kedaluwarsa kembali ke login"},
			{"web.auth.outlet", "Ganti outlet aktif: data di semua halaman ikut berganti"},
			{"web.auth.lang", "Ganti bahasa id/en/ja: menu, tombol, dan simbol mata uang ikut berubah"},
		}},
		{Key: "pos", Title: "Kasir (/kasir dan /kasirb)", Items: []QAItem{
			{"web.pos.shift", "Buka shift dengan saldo awal; tidak bisa simpan nota tanpa shift"},
			{"web.pos.cash", "Nota tunai: total, kembalian, stok berkurang, struk tercetak benar"},
			{"web.pos.multi", "Bayar campuran (tunai + non tunai) dan jumlah kurang ditolak"},
			{"web.pos.discount", "Diskon, pajak, dan harga grosir dihitung server dan cocok dengan spec test"},
			{"web.pos.override", "Ubah harga manual wajib izin + PIN, tercatat di audit"},
			{"web.pos.member", "Pilih member: poin bertambah, piutang/kredit sesuai aturan"},
			{"web.pos.qris", "QRIS EraPay: charge terbuat, lunas otomatis, kadaluwarsa ditangani"},
			{"web.pos.force", "Simpan paksa QRIS dengan PIN penyetuju; struk bertanda perlu dicek"},
			{"web.pos.void", "Batalkan nota dan retur penjualan: stok dan uang kembali benar"},
			{"web.pos.idem", "Klik simpan dua kali / koneksi putus tidak membuat nota ganda"},
			{"web.pos.variant", "/kasir dan /kasirb menghasilkan perilaku yang sama"},
			{"web.pos.close", "Tutup shift: selisih laci dihitung dan rekap cocok dengan struk"},
		}},
		{Key: "stock", Title: "Barang & stok", Items: []QAItem{
			{"web.stock.item", "Tambah/ubah barang, satuan, harga bertingkat, barcode unik"},
			{"web.stock.import", "Import barang Excel/CSV: baris salah dilaporkan, baris benar masuk"},
			{"web.stock.opening", "Saldo awal stok membuat movement OPENING"},
			{"web.stock.opname", "Stok opname menyesuaikan saldo dan terlihat di kartu stok"},
			{"web.stock.minus", "Stok tidak bisa minus kecuali barang diizinkan minus"},
			{"web.stock.split", "Pecah satuan memindah stok dengan faktor yang benar"},
			{"web.stock.card", "Kartu stok urut, saldo berjalan cocok dengan saldo barang"},
		}},
		{Key: "buy", Title: "Pembelian & hutang", Items: []QAItem{
			{"web.buy.po", "Faktur pembelian menambah stok dan HPP per cabang"},
			{"web.buy.edit", "Edit faktur beli membuat revisi tanpa error"},
			{"web.buy.pay", "Bayar hutang parsial dan lunas; sisa hutang benar"},
			{"web.buy.return", "Retur pembelian mengurangi stok dan hutang/kredit pemasok"},
		}},
		{Key: "member", Title: "Member, piutang, kupon", Items: []QAItem{
			{"web.member.crud", "Daftar member, poin, deposit"},
			{"web.member.recv", "Nota kredit membuat piutang; pelunasan mengurangi sisa"},
			{"web.member.coupon", "Kupon valid dipakai sekali; kupon kadaluwarsa ditolak"},
		}},
		{Key: "report", Title: "Laporan", Items: []QAItem{
			{"web.report.sales", "Penjualan per tanggal, faktur, barang, kasir: total saling cocok"},
			{"web.report.export", "Ekspor XLSX dan PDF terbuka dan isinya benar"},
			{"web.report.purchase", "Laporan pembelian dan retur cocok dengan transaksi"},
			{"web.report.debt", "Laporan hutang/piutang: saldo as-of dan aging benar"},
			{"web.report.stock", "Laporan persediaan sesuai saldo stok"},
			{"web.report.range", "Rentang satu tahun dan data banyak tetap cepat (tanpa timeout)"},
		}},
		{Key: "acc", Title: "Akuntansi (SIAK)", Items: []QAItem{
			{"web.acc.journal", "Jurnal manual: debit = kredit, posting, periode tertutup ditolak"},
			{"web.acc.auto", "Jurnal otomatis dari penjualan/pembelian terbentuk dan seimbang"},
			{"web.acc.bank", "Rekonsiliasi bank manual sampai finalisasi"},
			{"web.acc.fs", "Neraca dan Laba Rugi cocok dengan buku besar"},
			{"web.acc.year", "Tutup tahun membuat draf penutup yang bisa diposting"},
		}},
		{Key: "iam", Title: "Pengguna, role, paket", Items: []QAItem{
			{"web.iam.role", "Matriks izin: menu dan tombol hilang bila izin dicabut"},
			{"web.iam.limit", "Batas pengguna paket menolak pengguna baru dengan pesan upgrade"},
			{"web.iam.txlimit", "Batas transaksi/hari paket menolak nota dengan pesan jelas"},
			{"web.iam.lock", "Menu terkunci paket tampil bergembok dan membuka dialog upgrade"},
			{"web.iam.sub", "Halaman Langganan: bayar QRIS, paket berubah setelah lunas"},
			{"web.iam.audit", "Audit log tenant mencatat aksi penting dan bisa diekspor"},
		}},
		{Key: "ui", Title: "Tampilan", Items: []QAItem{
			{"web.ui.theme", "Tema terang/gelap konsisten di semua halaman utama"},
			{"web.ui.mobile", "Halaman utama dapat dipakai di lebar ponsel"},
			{"web.ui.errors", "Halaman 404/500 tampil; error API muncul sebagai pesan, bukan layar kosong"},
		}},
	}},
	{Key: "platform", Title: "Platform Admin", Groups: []QAGroup{
		{Key: "access", Title: "Akses", Items: []QAItem{
			{"platform.access.setup", "Setup admin pertama hanya bisa sekali"},
			{"platform.access.login", "Login platform + 2FA berhasil; kode salah ditolak"},
			{"platform.access.lockout", "Percobaan salah berulang memicu pembatasan"},
			{"platform.access.admins", "Tambah, nonaktifkan admin, reset password dan reset 2FA"},
		}},
		{Key: "tenant", Title: "Tenant & paket", Items: []QAItem{
			{"platform.tenant.list", "Daftar dan pencarian tenant; detail menampilkan outlet/pengguna"},
			{"platform.tenant.plan", "Set paket tenant dan override batas; berlaku ≤10 detik"},
			{"platform.tenant.delete.schedule", "Jadwalkan hapus tenant: ditolak bila tenant masih aktif; masa tunggu (0-365 hari) diisi admin dan tercatat di audit platform"},
			{"platform.tenant.delete.cancel", "Batalkan jadwal hapus, atau aktifkan tenant kembali: jadwal hilang dan data utuh"},
			{"platform.tenant.delete.purge", "Hapus permanen: wajib ketik nama tenant, ditolak sebelum jatuh tempo; semua data dan berkas tenant hilang, tenant lain tidak terpengaruh, audit platform tetap ada"},
			{"platform.tenant.delete.auto", "Tenant yang masa tunggunya habis terhapus otomatis oleh penjadwal API (±10 menit)"},
			{"platform.plan.crud", "Buat/ubah paket, hak akses menu, poin keunggulan"},
			{"platform.plan.modules", "Modul di luar paket terkunci walau role Owner"},
		}},
		{Key: "system", Title: "Sistem", Items: []QAItem{
			{"platform.domain.add", "Tambah domain, verifikasi DNS, aktifkan/matikan, hapus"},
			{"platform.domain.tls", "Domain aktif dilayani edge dengan sertifikat TLS otomatis"},
			{"platform.api.cred", "Kredensial API: buat (secret tampil sekali), rotasi, IP allowlist"},
			{"platform.api.brand", "Profil white label portal dokumentasi tersimpan dan tampil"},
			{"platform.lang", "Tambah/ubah bahasa; kode full-width IME dinormalkan"},
			{"platform.apk", "Unggah APK mobile; versi dan sha256 benar, unduhan jalan"},
			{"platform.audit", "Audit platform mencatat semua aksi di atas, filter dan live tail jalan"},
			{"platform.security", "Halaman keamanan: ubah password dan atur 2FA sendiri"},
		}},
	}},
	{Key: "developers", Title: "Portal Developer & API Eksternal", Groups: []QAGroup{
		{Key: "gateway", Title: "Gateway HMAC", Items: []QAItem{
			{"dev.gw.sign", "Request bertanda tangan valid → 200; tanda tangan salah → 401"},
			{"dev.gw.ip", "IP di luar allowlist ditolak; allowlist kosong menolak semua"},
			{"dev.gw.nonce", "Nonce yang sama dipakai dua kali ditolak"},
			{"dev.gw.skew", "Timestamp lewat toleransi ditolak"},
			{"dev.gw.rate", "Rate limit per kredensial menjawab 429"},
			{"dev.gw.perm", "Izin = izin role akun layanan; rute /auth /platform tertolak"},
			{"dev.gw.rotate", "Rotasi secret: secret lama berlaku selama masa transisi lalu mati"},
		}},
		{Key: "portal", Title: "Portal /developers", Items: []QAItem{
			{"dev.portal.load", "Portal terbuka tanpa login; tema gelap/terang"},
			{"dev.portal.catalog", "Katalog endpoint lengkap dan pencarian berfungsi"},
			{"dev.portal.try", "Tombol Coba menandatangani di browser dan mendapat 200"},
			{"dev.portal.brand", "Domain white label menampilkan nama/logo/aksen profilnya"},
		}},
	}},
	{Key: "mobile", Title: "ARUS Mobile (Flutter)", Groups: []QAGroup{
		{Key: "core", Title: "Inti", Items: []QAItem{
			{"mobile.auth", "Login native (X-Client: mobile), token tersimpan aman, logout"},
			{"mobile.pos", "Nota tunai dari ponsel: total dan stok sama dengan web"},
			{"mobile.qris", "QRIS + simpan paksa dengan PIN penyetuju"},
			{"mobile.fee", "Preview biaya metode bayar (MDR transaksi kecil) benar"},
			{"mobile.lang", "Bahasa id/en/ja dan simbol mata uang ikut berganti"},
			{"mobile.update", "Pembaruan APK dari halaman unduhan platform terpasang"},
		}},
		{Key: "print", Title: "Cetak", Items: []QAItem{
			{"mobile.bt", "Cetak struk ke printer Bluetooth"},
		}},
	}},
	{Key: "printagent", Title: "Print-agent (PC kasir)", Groups: []QAGroup{
		{Key: "print", Title: "Cetak", Items: []QAItem{
			{"agent.install", "Agent terpasang dan terhubung ke printer lokal"},
			{"agent.receipt", "Struk ESC/POS tercetak lengkap (logo, item, total, QR)"},
			{"agent.drawer", "Laci kas terbuka saat nota tunai"},
			{"agent.offline", "Printer mati/offline menghasilkan pesan jelas, nota tetap tersimpan"},
		}},
	}},
	{Key: "cross", Title: "Lintas Aplikasi", Groups: []QAGroup{
		{Key: "security", Title: "Keamanan & tenant", Items: []QAItem{
			{"cross.tenant.iso", "Tenant A tidak bisa membaca/menulis data tenant B (API dan RLS)"},
			{"cross.tenant.body", "tenant_id/outlet_id di body request diabaikan"},
			{"cross.idem", "Idempotency-Key mencegah transaksi ganda di web dan mobile"},
			{"cross.money", "Pembulatan uang konsisten web, mobile, laporan, jurnal"},
			{"cross.tz", "Tanggal bisnis mengikuti zona waktu outlet di semua laporan"},
		}},
		{Key: "infra", Title: "Infrastruktur", Items: []QAItem{
			{"cross.backup", "Backup database terbaru bisa dipulihkan di lingkungan uji"},
			{"cross.migrate", "goose up dari DB kosong dan dari DB lama tanpa error"},
			{"cross.proxy", "Di belakang proxy: TRUST_PROXY aktif, IP klien benar di audit/allowlist"},
			{"cross.erapay", "Webhook/rekonsiliasi EraPay: nota force-save lunas atau auto-void"},
			{"cross.redis", "Redis mati: sistem gagal aman, tidak menerima transaksi ganda"},
		}},
	}},
}

// qaItemKeys = himpunan kunci item yang sah (untuk validasi input).
var qaItemKeys = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, a := range qaCatalog {
		for _, g := range a.Groups {
			for _, it := range g.Items {
				m[it.Key] = struct{}{}
			}
		}
	}
	return m
}()
