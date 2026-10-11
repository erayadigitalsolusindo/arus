export default {
  docTitle: 'HalloArus | ACIRABA',
  title: 'HalloArus',
  subtitle: 'Asisten AI untuk membaca data usaha Anda, menarik kesimpulan, dan memberi saran.',
  readOnly: 'Hanya membaca',
  readOnlyHint: 'HalloArus tidak bisa menambah, mengubah, atau menghapus data apa pun.',
  privacy: 'Ringkasan angka dikirim ke penyedia AI yang Anda pilih (Claude, ChatGPT, Gemini, atau Qwen) memakai API key milik usaha Anda (atau akun gratis ARUS). Data pelanggan (nama, telepon, alamat) tidak pernah dikirim; pelanggan hanya tampil sebagai "Pelanggan 1, 2, …".',
  settings: 'Pengaturan',
  setup: {
    title: 'Hubungkan AI',
    intro: 'HalloArus memakai API key milik usaha Anda sendiri; biaya pemakaian ditagih langsung oleh penyedia AI ke akun Anda. Buat key di {console} lalu tempel di bawah. Key tiap penyedia disimpan terpisah; hanya satu yang aktif.',
    providerLabel: 'Penyedia AI',
    keyLabel: 'API key {provider}',
    keyReplaceHint: 'Kosongkan bila tidak ingin mengganti key.',
    keySaved: 'Key tersimpan (berakhiran {hint}). Disimpan terenkripsi dan tidak bisa dilihat lagi.',
    keyNone: 'Belum ada key untuk penyedia ini.',
    modelLabel: 'Model',
    save: 'Simpan & aktifkan',
    saving: 'Memeriksa & menyimpan…',
    saved: 'Pengaturan tersimpan.',
    removeKey: 'Hapus key {provider}',
    removeConfirm: 'Hapus API key {provider}? Bila ini penyedia aktif, HalloArus beralih ke penyedia lain yang punya key (atau berhenti sampai key dipasang).',
    needKey: 'Isi API key {provider} lebih dulu.',
    notConfigured: 'HalloArus belum aktif. Minta pemilik usaha memasang API key AI di halaman ini.',
    notConfiguredManage: 'Pasang API key AI (Claude, ChatGPT, Gemini, atau Qwen) untuk mengaktifkan HalloArus.'
  },
  providers: {
    anthropic: { name: 'Claude (Anthropic)', placeholder: 'sk-ant-…', console: 'console.anthropic.com' },
    openai: { name: 'ChatGPT (OpenAI)', placeholder: 'sk-…', console: 'platform.openai.com' },
    gemini: { name: 'Gemini (Google)', placeholder: 'AIza…', console: 'aistudio.google.com' },
    qwen: { name: 'Qwen (Alibaba)', placeholder: 'sk-…', console: 'modelstudio.console.alibabacloud.com (wilayah Singapura)' }
  },
  models: {
    'claude-sonnet-5-5': 'Claude Sonnet 5.5 (seimbang, disarankan)',
    'claude-opus-5-5': 'Claude Opus 5.5 (analisis lebih dalam)',
    'claude-fable-5-1': 'Claude Fable 5.1 (paling mampu)',
    'claude-haiku-5-5': 'Claude Haiku 5.5 (cepat & hemat)',
    'gpt-5': 'GPT-5 (seimbang)',
    'gpt-5-mini': 'GPT-5 mini (cepat & hemat)',
    'gpt-5.6-sol': 'GPT-5.6 Sol (terbaru)',
    'gemini-2.5-pro': 'Gemini 2.5 Pro (analisis lebih dalam)',
    'gemini-2.5-flash': 'Gemini 2.5 Flash (cepat & hemat)',
    'gemini-3-flash-preview': 'Gemini 3 Flash (pratinjau)',
    'qwen3-max': 'Qwen3 Max (paling mampu)',
    'qwen-plus': 'Qwen Plus (seimbang)',
    'qwen-flash': 'Qwen Flash (cepat & hemat)'
  },
  free: {
    banner: 'Akun gratis: sisa {remaining} dari {limit} pertanyaan hari ini. Pasang API key sendiri untuk tanpa batas.',
    exhausted: 'Kuota akun gratis hari ini habis (kembali besok). Pasang API key sendiri untuk lanjut tanpa batas.',
    useOwn: 'Pasang key sendiri'
  },
  connector: {
    button: 'Connector AI',
    title: 'Sambungkan ke aplikasi AI',
    intro: 'Pakai langganan Claude, ChatGPT, Gemini, atau Qwen Anda sendiri (tanpa API key): tambahkan ARUS sebagai connector MCP, lalu tanyakan data usaha langsung di aplikasi itu. Connector hanya bisa membaca, dengan batasan yang sama seperti HalloArus.',
    appsTitle: 'Pilih aplikasi Anda',
    appsNote: 'Dukungan connector berbeda-beda per aplikasi dan paket. Bila tidak tersedia, pakai chat HalloArus dengan API key (pengaturan di atas).',
    cmdLabel: 'Perintah',
    apps: {
      claude: {
        name: 'Claude',
        s1: 'Buka Settings → Connectors → Add custom connector.',
        s2: 'Isi nama (mis. ARUS) dan tempel alamat connector di atas, lalu simpan.',
        s3: 'Klik Connect; Anda diarahkan ke ARUS untuk menyetujui akses baca.'
      },
      chatgpt: {
        name: 'ChatGPT',
        s1: 'Buka Settings → Apps & Connectors → Advanced settings, aktifkan Developer mode (hanya di paket tertentu).',
        s2: 'Kembali ke Apps & Connectors → Create. Isi nama (mis. ARUS), tempel alamat connector di atas sebagai MCP Server URL, pilih autentikasi OAuth, lalu Create.',
        s3: 'Di percakapan baru, pilih ARUS dari menu + → Developer mode, lalu setujui akses baca saat diarahkan ke ARUS.'
      },
      gemini: {
        name: 'Gemini (Gemini CLI)',
        s1: 'Pasang Gemini CLI, lalu jalankan perintah di bawah.',
        s2: 'Jalankan gemini, ketik /mcp auth arus, lalu setujui akses baca di browser yang terbuka.',
        s3: 'Tanyakan data usaha langsung di Gemini CLI. Aplikasi web Gemini belum mendukung connector kustom.'
      },
      qwen: {
        name: 'Qwen (Qwen Code)',
        s1: 'Pasang Qwen Code, lalu jalankan perintah di bawah.',
        s2: 'Jalankan qwen, lalu selesaikan otorisasi yang muncul (/mcp) dan setujui akses baca di browser.',
        s3: 'Tanyakan data usaha langsung di Qwen Code.'
      }
    },
    urlLabel: 'Alamat connector',
    copy: 'Salin',
    copied: 'Tersalin',
    listTitle: 'Connector yang aktif',
    none: 'Belum ada connector yang tersambung.',
    by: 'oleh {name}',
    lastUsed: 'terakhir dipakai {when}',
    never: 'belum pernah dipakai',
    revoke: 'Cabut',
    revokeConfirm: 'Cabut akses connector ini? Claude tidak bisa lagi membaca data ARUS lewat izin ini.',
    revoked: 'Akses connector dicabut.'
  },
  connect: {
    docTitle: 'Izinkan Claude | ACIRABA',
    title: 'Izinkan {name} membaca data ARUS?',
    lead: 'Anda masuk sebagai {user}. {name} (kembali ke {host}) meminta izin untuk:',
    c1: 'Membaca ringkasan penjualan, stok, piutang/hutang, kas, dan akuntansi sesuai hak akses Anda.',
    c2: 'Melihat pelanggan hanya sebagai "Pelanggan 1, 2, …" (nama, telepon, alamat tidak pernah dikirim).',
    c3: 'Tidak bisa menambah, mengubah, atau menghapus data apa pun.',
    allow: 'Izinkan',
    deny: 'Tolak',
    working: 'Mengalihkan…',
    invalid: 'Permintaan tidak valid atau kedaluwarsa. Ulangi dari aplikasi Claude.',
    revokeHint: 'Anda bisa mencabut akses kapan saja di HalloArus → Connector Claude.'
  },
  empty: {
    title: 'Tanya apa saja tentang data usaha Anda',
    hint: 'HalloArus membaca penjualan, stok, piutang/hutang, kas, dan akuntansi sesuai hak akses Anda, lalu menyimpulkan dan memberi saran.'
  },
  suggestions: {
    s1: 'Bagaimana penjualan 30 hari terakhir dibanding 30 hari sebelumnya?',
    s2: 'Barang apa yang paling laku dan paling untung bulan ini?',
    s3: 'Barang apa yang stoknya menipis dan perlu segera dibeli?',
    s4: 'Ada piutang yang berisiko macet? Siapa yang harus ditagih dulu?',
    s5: 'Apakah ada masalah selisih kas atau banyak pembatalan nota di kasir?',
    s6: 'Barang apa yang mengendap dan sebaiknya diobral?'
  },
  input: {
    placeholder: 'Tulis pertanyaan… (Enter kirim, Shift+Enter baris baru)',
    send: 'Kirim',
    clear: 'Percakapan baru',
    stop: 'Batalkan'
  },
  thinking: 'Membaca data & menganalisis…',
  you: 'Anda',
  assistant: 'HalloArus',
  dataRead: 'Data yang dibaca',
  usage: '{input} token masuk · {output} token keluar',
  disclaimer: 'Jawaban AI bisa keliru; periksa angka penting di laporan. Bukan nasihat keuangan, pajak, atau hukum.',
  tools: {
    list_outlets: 'Daftar outlet',
    sales_summary: 'Ringkasan penjualan',
    top_items: 'Barang terlaris / terlemah',
    sales_by_category: 'Penjualan per kategori',
    returns_summary: 'Ringkasan retur',
    member_overview: 'Ringkasan pelanggan (anonim)',
    stock_overview: 'Kondisi persediaan',
    stock_items: 'Barang perlu perhatian',
    slow_moving_items: 'Barang mengendap',
    stock_opname_summary: 'Hasil opname stok',
    receivables_summary: 'Piutang & umur piutang',
    payables_summary: 'Hutang pemasok',
    purchases_summary: 'Ringkasan pembelian',
    cash_shifts_summary: 'Kontrol shift kasir',
    petty_cash_summary: 'Kas kecil',
    accounting_balances: 'Saldo akun (buku besar)'
  }
};
