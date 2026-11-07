// White label per domain kustom. Di host utama tampilannya tetap ARUS; di domain kustom, nama/logo/ikon/footer diambil dari
// pengaturan domain itu (Platform Admin > Domain) lewat /public/branding. Berkas statis ARUS tetap jadi cadangan.
import { isCustomDomain, request } from '#lib/api/client.ts';
import { brandText } from '#lib/i18n/i18n.svelte.ts';

const DEFAULT_LOGO = '/logo_dengan_text-no-bg.svg';
const DEFAULT_ICON = '/logo_tanpa_text-no-bg.svg';

export const brand = $state({
  name: 'ARUS',
  /** Logo penuh (dengan teks) dan ikon; '' = belum siap (domain kustom masih memuat) agar logo ARUS tidak sempat berkedip. */
  logo: isCustomDomain ? '' : DEFAULT_LOGO,
  icon: isCustomDomain ? '' : DEFAULT_ICON,
  footer: '',
  ready: !isCustomDomain
});

let started = false;

/** Dipanggil sekali saat aplikasi dibuka. Host utama: tidak melakukan apa-apa. */
export async function initBrand(): Promise<void> {
  if (!isCustomDomain || started) return;
  started = true;
  try {
    const r = await request<{
      tenant_name?: string;
      brand?: { name: string; footer: string; logo_url: string; icon_url: string };
    }>(`/public/branding?host=${encodeURIComponent(location.hostname)}`, {}, null);
    const b = r.brand;
    // Bawaan = tampilan ARUS; hanya bagian yang diisi admin (Platform > Domain) yang berubah.
    const name = (b?.name ?? '').trim();
    if (name) {
      brand.name = name;
      brandText.name = name;
      // Judul tab diatur tiap halaman ("... | ACIRABA"); ganti ARUS/ACIRABA dengan nama merek, juga saat judul berubah.
      // Idempoten: judul yang sudah memuat nama merek tidak diganti lagi. Tanpa ini, nama yang mengandung kata "ARUS"
      // (mis. "ARUS Kotak Cantik") membuat penggantian memicu pengamat lagi tanpa henti dan browser membeku.
      const fix = () => {
        const cur = document.title;
        if (cur.includes(name) || !/\b(ARUS|ACIRABA)\b/.test(cur)) return;
        document.title = cur.replace(/\b(ARUS|ACIRABA)\b/g, () => name);
      };
      fix();
      const titleEl = document.querySelector('title');
      if (titleEl) new MutationObserver(fix).observe(titleEl, { childList: true, characterData: true });
    }
    brand.footer = b?.footer ?? '';
    brand.logo = b?.logo_url || DEFAULT_LOGO;
    brand.icon = b?.icon_url || DEFAULT_ICON;
    if (b?.icon_url) {
      for (const el of document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')) {
        el.href = b.icon_url;
        el.removeAttribute('type');
      }
    }
  } catch {
    // Gagal memuat: tetap tampilan ARUS.
    brand.logo = DEFAULT_LOGO;
    brand.icon = DEFAULT_ICON;
  } finally {
    brand.ready = true;
  }
}
