# AGENTS.md — web (SvelteKit)

> Berlaku untuk `source/web/`. Aturan global ada di `/AGENTS.md`; pemetaan template di `docs/ARCHITECTURE.md` (§5b).
> Kasir: `/kasir` dan `/kasirb` hanya membungkus `lib/components/PosScreen.svelte` (prop `variant`); ubah logika di sana, bukan di rute. `/dashboard` dan `/live-sales` adalah halaman terpisah; fitur baru = menu/halaman baru, jangan menimpa yang ada.

- **Catatan SvelteKit 3 (terpasang 3.0.1, bukan 2):** konfigurasi adapter ada di `vite.config.ts` (tidak ada `svelte.config.js`); alias `$lib` dihapus → pakai `#lib/...` **dengan ekstensi** (`#lib/nav.ts`, `#lib/components/X.svelte`) lewat `imports` di `source/web/package.json`; butuh `typescript@6`.
- **UI:** `source/web/src/lib/styles/dreams/dreams-core.css` = salinan build CSS template (token, komponen `.btn`/`.surface-card`/`.app-sidebar`, ikon lucide `icon-*` & phosphor `ph-*`) + font; 6 URL gambar demo diganti GIF 1px. Template menyembunyikan `<html>` sampai `data-theme` terpasang → di-set sinkron di `web/src/app.html`. Tailwind v4 (`@tailwindcss/vite`) hanya menambah utilitas baru. Logo ACIRABA = placeholder SVG di `web/static/`.
- Teks UI dan pesan error untuk pengguna akhir: **multi-bahasa (id = sumber kebenaran + fallback, en)**. **Dilarang menulis teks UI langsung di komponen** — pakai `t('domain.kunci')` dari `#lib/i18n/index.ts`. Kamus di `web/src/lib/i18n/messages/{id,en}/<domain>.ts`; `id` menentukan bentuk, bahasa lain bertipe `Messages` (kunci kurang/typo gagal di `npm run check`). Modul baru = satu file domain per bahasa + daftarkan di `index.ts`. Plural: `{ one, other }` + param `count`. Angka/uang/tanggal lewat `formatNumber/formatCurrency/formatDate/formatDateTime` (bukan `toLocaleString` manual). Error API: terjemahan lewat `errors.<CODE>` (`errorMessage()`), server cukup mengirim `code` stabil; klien mengirim `Accept-Language`. Bahasa baru: tambah di `locales.ts` + folder kamus.
- **Rentang tanggal ("between", dari–sampai) WAJIB memakai `lib/components/DateRange.svelte`** (flatpickr mode range, 2 bulan berdampingan, gaya template — lihat `reference/template/sales-report.html`, input `data-daterange`); jangan dua `<input type="date">`. CSS `.flatpickr-*` sudah ada di `dreams-core.css`; nilai `from`/`to` = `YYYY-MM-DD`, `onchange` terpanggil saat kedua ujung terpilih. Filter tanggal tunggal: pakai flatpickr yang sama (mode single), bukan input bawaan browser.

- Cara menjalankan web & test: lihat `docs/DEV-ENV.md`.

## Pelajaran UI (dari log sesi)
- Dropdown selalu `Select.svelte` / `Combobox.svelte` (bukan `<select>`). Pemicu bits-ui Select terbuka lewat `pointerdown`; uji otomatis pakai klik sungguhan.
- Utilitas Tailwind berawalan `!`, `end-*`, `ps-*` tidak dapat diandalkan di berkas baru: pakai `style`. Tailwind dev server tidak memindai berkas baru → `touch src/app.css`.
- Sel `sticky` wajib berlatar solid (bukan `inherit`/transparan). Penanda daftar markdown digambar lewat `::before` (template memaksa `list-style: none`). Template menimpa `button[aria-selected=true]` → tab pakai `!bg-transparent`.
- Token akses hanya di memori; gambar berotorisasi lewat `apiBlob()`/`AuthImage`. `{@html}` hanya untuk markdown yang sudah disaring `lib/markdown.ts`.
- Draf form per tab: `lib/tabs/drafts.ts`; filter halaman: `persistPage`. Uji UI jangan membuat/mengubah data di tenant dev pengguna.
