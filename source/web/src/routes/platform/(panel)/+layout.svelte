<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { platform, platformLogout } from '#lib/platform/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';

  let { children }: { children: Snippet } = $props();

  const links = [
    { href: '/platform/tenants', key: 'platform.nav.tenants', icon: 'store' },
    { href: '/platform/admins', key: 'platform.nav.admins', icon: 'shield-check' },
    { href: '/platform/audit', key: 'platform.nav.audit', icon: 'scroll-text' },
    { href: '/platform/security', key: 'platform.nav.security', icon: 'key-round' }
  ] as const;

  // Sesi berakhir (refresh gagal): kembali ke login platform.
  $effect(() => {
    if (platform.status === 'anon') void goto('/platform/login');
  });
</script>

<svelte:head><title>{t('platform.docTitle')}</title></svelte:head>

<div class="min-h-screen flex flex-col bg-[var(--surface-sunken)]">
  <header class="sticky top-0 z-30 flex flex-wrap items-center gap-x-6 gap-y-2 px-4 lg:px-6 py-2.5 border-b border-[var(--border-subtle)] bg-[var(--surface-card,var(--surface-base,#fff))]">
    <a href="/platform/tenants" class="flex items-center gap-2.5 shrink-0">
      <img src="/logo_dengan_text-no-bg.svg" alt="ACIRABA" class="h-9 w-auto" />
      <span class="badge-danger rounded-md px-2 py-0.5 text-[11px] font-bold uppercase tracking-wide">{t('platform.brand')}</span>
    </a>
    <nav class="flex items-center gap-1 flex-1 min-w-0 overflow-x-auto" aria-label={t('platform.brand')}>
      {#each links as l (l.href)}
        <a
          href={l.href}
          class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[12.5px] font-semibold whitespace-nowrap hover:bg-[var(--surface-sunken)] {page.url.pathname.startsWith(l.href) ? 'text-[var(--color-primary-600)] bg-[var(--surface-sunken)]' : 'text-[var(--text-secondary,inherit)]'}"
          aria-current={page.url.pathname.startsWith(l.href) ? 'page' : undefined}
        >
          <i class="icon-{l.icon} text-[14px]"></i>{t(l.key)}
        </a>
      {/each}
    </nav>
    <div class="flex items-center gap-3 shrink-0">
      <LanguageSwitcher />
      <span class="hidden sm:block text-[12px] font-semibold max-w-48 truncate">{platform.admin?.name}</span>
      <button type="button" class="header-icon-btn" title={t('platform.nav.signOut')} aria-label={t('platform.nav.signOut')} onclick={platformLogout}>
        <i class="icon-log-out text-[15px]"></i>
      </button>
    </div>
  </header>

  <main class="flex-1 p-4 lg:p-6 w-full max-w-[1400px] mx-auto space-y-4">
    {@render children()}
  </main>
</div>
