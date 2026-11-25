// Menu panel platform. `module` = id modul izin (backend/internal/platformadmin/perms.go): menu hanya tampil bila
// admin punya izin "view" modul itu. Menu tanpa `module` (Keamanan) terbuka untuk semua admin.
import type { MessageKey } from '#lib/i18n/index.ts';
import { canPlatform } from './session.svelte.ts';

export type NavItem = { href: string; key: MessageKey; icon: string; module?: string };
export type NavGroup = { titleKey: MessageKey; items: NavItem[] };

export const navGroups: NavGroup[] = [
  {
    titleKey: 'platform.nav.groupMain',
    items: [
      { href: '/platform/live', key: 'platform.nav.live', icon: 'activity', module: 'live' },
      { href: '/platform/tenants', key: 'platform.nav.tenants', icon: 'store', module: 'tenants' },
      { href: '/platform/admins', key: 'platform.nav.admins', icon: 'shield-check', module: 'admins' },
      { href: '/platform/roles', key: 'platform.access.navRoles', icon: 'key-round', module: 'roles' },
      { href: '/platform/plans', key: 'platform.nav.plans', icon: 'badge-dollar-sign', module: 'plans' },
      { href: '/platform/storefront-templates', key: 'platform.nav.templates', icon: 'layout-template', module: 'storefront_templates' },
      { href: '/platform/halloarus', key: 'platform.nav.halloarus', icon: 'sparkles', module: 'ai' }
    ]
  },
  {
    titleKey: 'platform.nav.groupReports',
    items: [
      { href: '/platform/reports/sales', key: 'platform.nav.reportSales', icon: 'trending-up', module: 'reports_sales' },
      { href: '/platform/reports/expiring', key: 'platform.nav.reportExpiring', icon: 'alarm-clock', module: 'reports_expiring' },
      { href: '/platform/reports/locations', key: 'platform.nav.reportLocations', icon: 'map-pin', module: 'reports_locations' }
    ]
  },
  {
    titleKey: 'platform.nav.groupSystem',
    items: [
      { href: '/platform/audit', key: 'platform.nav.audit', icon: 'scroll-text', module: 'audit' },
      { href: '/platform/qa', key: 'platform.nav.qa', icon: 'list-checks', module: 'qa' },
      { href: '/platform/backup', key: 'platform.nav.backup', icon: 'cloud-backup', module: 'backup' },
      { href: '/platform/mobile', key: 'platform.nav.mobile', icon: 'smartphone', module: 'mobile_apk' },
      { href: '/platform/api-access', key: 'platform.nav.apiAccess', icon: 'plug', module: 'api_access' },
      { href: '/platform/domains', key: 'platform.nav.domains', icon: 'globe', module: 'domains' },
      { href: '/platform/languages', key: 'platform.nav.languages', icon: 'languages', module: 'languages' },
      { href: '/platform/security', key: 'platform.nav.security', icon: 'key-round' }
    ]
  }
];

const items = navGroups.flatMap((g) => g.items);
export const allowed = (item: NavItem) => !item.module || canPlatform(item.module, 'view');

/** Halaman pertama yang boleh dibuka admin ini (tujuan pengalihan saat membuka halaman di luar izinnya). */
export function firstAllowedHref(): string {
  return (items.find((i) => i.module && allowed(i)) ?? items[items.length - 1]).href;
}

/** Apakah path (mis. /platform/reports/sales/xyz) boleh dibuka; path di luar menu dianggap boleh (server tetap menolak). */
export function pathAllowed(pathname: string): boolean {
  const hit = items.filter((i) => pathname === i.href || pathname.startsWith(i.href + '/')).sort((a, b) => b.href.length - a.href.length)[0];
  return !hit || allowed(hit);
}
