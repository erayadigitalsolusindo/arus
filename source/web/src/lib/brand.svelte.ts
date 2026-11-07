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
    // Tanpa nama khusus memakai nama tenant, bukan "ARUS" (white label penuh).
    const name = (b?.name || r.tenant_name || '').trim();
    if (name) {
      brand.name = name;
      brandText.name = name;
      // Judul tab diatur tiap halaman ("... | ACIRABA"); ganti ARUS/ACIRABA dengan nama merek, juga saat judul berubah.
      const fix = () => {
        const next = document.title.replace(/\b(ARUS|ACIRABA)\b/g, () => name);
        if (next !== document.title) document.title = next;
      };
      fix();
      new MutationObserver(fix).observe(document.querySelector('title') ?? document.head, { childList: true, subtree: true, characterData: true });
    }
    brand.footer = b?.footer ?? '';
    brand.logo = b?.logo_url || '';
    brand.icon = b?.icon_url || b?.logo_url || '';
    if (brand.icon) {
      for (const el of document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')) {
        el.href = brand.icon;
        el.removeAttribute('type');
      }
    }
  } catch {
    /* gagal memuat: tampilan polos tanpa logo ARUS */
  } finally {
    brand.ready = true;
  }
}
