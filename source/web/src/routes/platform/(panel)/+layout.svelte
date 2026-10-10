<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { afterNavigate, goto } from '$app/navigation';
  import { platform, platformLogout } from '#lib/platform/session.svelte.ts';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import { initials } from '#lib/auth/initials.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import CommandHero from '#lib/platform/CommandHero.svelte';

  let { children }: { children: Snippet } = $props();

  // Shell sama dengan aplikasi tenant (app-shell / app-sidebar / app-header dari template), isi menu khusus platform.
  const groups: { titleKey: MessageKey; items: { href: string; key: MessageKey; icon: string }[] }[] = [
    {
      titleKey: 'platform.nav.groupMain',
      items: [
        { href: '/platform/tenants', key: 'platform.nav.tenants', icon: 'store' },
        { href: '/platform/admins', key: 'platform.nav.admins', icon: 'shield-check' }
      ]
    },
    {
      titleKey: 'platform.nav.groupSystem',
      items: [
        { href: '/platform/audit', key: 'platform.nav.audit', icon: 'scroll-text' },
        { href: '/platform/mobile', key: 'platform.nav.mobile', icon: 'smartphone' },
        { href: '/platform/security', key: 'platform.nav.security', icon: 'key-round' }
      ]
    }
  ];

  const isActive = (href: string) => page.url.pathname.startsWith(href);

  let collapsed = $state(false);
  let mobileOpen = $state(false);
  let menu = $state(false);
  afterNavigate(() => (mobileOpen = false));

  let theme = $state<'light' | 'dark'>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('theme', theme);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }

  let fullscreen = $state(!!document.fullscreenElement);
  function toggleFullscreen() {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void document.documentElement.requestFullscreen?.();
  }

  // Sesi berakhir (refresh gagal): kembali ke login platform.
  $effect(() => {
    if (platform.status === 'anon') void goto('/platform/login');
  });
</script>

<svelte:head><title>{t('platform.docTitle')}</title></svelte:head>

<svelte:document onfullscreenchange={() => (fullscreen = !!document.fullscreenElement)} />
<svelte:window
  onclick={() => (menu = false)}
  onkeydown={(e) => {
    if (e.key === 'Escape') menu = false;
  }}
/>

<div class="app-shell" class:is-collapsed={collapsed}>
  {#if mobileOpen}
    <button type="button" class="sidebar-backdrop lg:hidden" aria-label={t('shell.closeMenu')} onclick={() => (mobileOpen = false)}></button>
  {/if}

  <aside class="app-sidebar sidebar-pattern scroll-thin" class:is-mobile-open={mobileOpen} aria-label={t('shell.sidebar')}>
    <div class="flex items-center justify-between h-16 px-4 shrink-0 border-b border-[var(--sidebar-border)]">
      <a href="/platform/tenants" class="flex items-center gap-2.5 min-w-0 rounded-xl px-2.5 py-1.5">
        <img src="/logo_dengan_text-no-bg.svg" alt="ARUS" class="sidebar-logo-full logo-outline h-10 w-auto object-contain shrink-0" />
        <img src="/logo_tanpa_text-no-bg.svg" alt="ARUS" class="sidebar-logo-small logo-outline h-8 w-auto object-contain shrink-0 hidden" />
      </a>
      <button
        type="button"
        class="header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-8 shrink-0 max-lg:hidden"
        aria-label={collapsed ? t('shell.expandSidebar') : t('shell.collapseSidebar')}
        onclick={() => (collapsed = !collapsed)}
      >
        <i class="icon-panel-left-close text-[17px]"></i>
      </button>
    </div>

    <div class="px-4 pt-3">
      <span class="workspace-text inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[10.5px] font-bold uppercase tracking-wider bg-[rgb(255_255_255_/_0.1)] text-white">
        <i class="icon-shield-check text-[12px]"></i>{t('platform.brand')}
      </span>
    </div>

    <nav class="sidebar-inner flex-1 overflow-y-auto scroll-thin px-3 pb-4 mt-1" aria-label={t('shell.mainNav')}>
      <ul role="menu">
        {#each groups as group (group.titleKey)}
          <li class="sidebar-group-title">{t(group.titleKey)}</li>
          {#each group.items as item (item.href)}
            <li>
              <a href={item.href} class="sidebar-link" class:is-active={isActive(item.href)} aria-current={isActive(item.href) ? 'page' : undefined}>
                <i class="icon-{item.icon} text-[16px]"></i>
                <span class="sidebar-label">{t(item.key)}</span>
              </a>
            </li>
          {/each}
        {/each}
      </ul>
    </nav>

    <div class="p-3 border-t shrink-0 border-[var(--sidebar-border)]">
      <div class="w-full flex items-center gap-2.5 rounded-lg p-2 bg-[rgb(255_255_255_/_0.04)]">
        <span class="grid place-items-center size-8 rounded-full shrink-0 bg-[rgb(255_255_255_/_0.12)] text-white text-[12px] font-bold">{initials(platform.admin?.name)}</span>
        <span class="workspace-text text-left min-w-0 flex-1">
          <span class="block text-[12.5px] font-semibold text-white truncate">{platform.admin?.name}</span>
          <span class="block text-[10.5px] truncate text-[var(--sidebar-text)]">{platform.admin?.email}</span>
        </span>
        <button type="button" onclick={platformLogout} class="workspace-text header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-7 shrink-0" title={t('platform.nav.signOut')} aria-label={t('platform.nav.signOut')}>
          <i class="icon-log-out text-[14px]"></i>
        </button>
      </div>
    </div>
  </aside>

  <div class="app-main">
    <header class="app-header">
      <div class="absolute inset-x-0 bottom-0 h-px pointer-events-none [background:linear-gradient(90deg,transparent,var(--color-primary-500)_20%,var(--color-accent-500)_50%,var(--color-primary-500)_80%,transparent)] opacity-[0.55]"></div>
      <div class="flex items-center gap-3 px-4 lg:px-6 h-16">
        <button type="button" class="header-icon-btn lg:hidden" aria-label={t('shell.openMenu')} onclick={() => (mobileOpen = true)}>
          <i class="icon-menu text-[18px]"></i>
        </button>
        <a href="/platform/tenants" class="lg:hidden shrink-0" aria-label="ACIRABA">
          <img src="/logo_tanpa_text-no-bg.svg" alt="ARUS" class="h-8 w-auto object-contain" />
        </a>

        <div class="ms-auto flex items-center gap-2.5 lg:gap-3">
          <button
            type="button"
            class="grid place-items-center size-10 rounded-full transition-transform hover:scale-105 bg-[color-mix(in_oklab,var(--color-warning-500)_16%,transparent)] text-[var(--color-warning-600)]"
            aria-label={theme === 'dark' ? t('common.theme.light') : t('common.theme.dark')}
            onclick={toggleTheme}
          >
            <i class={theme === 'dark' ? 'icon-moon text-[17px]' : 'icon-sun-medium text-[17px]'}></i>
          </button>

          <div class="flex items-center gap-1 p-0.5 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] shadow-[var(--shadow-sm)]">
            <LanguageSwitcher variant="circle" />
            <button
              type="button"
              class="hidden sm:grid place-items-center size-9 rounded-full transition-transform hover:scale-105 bg-[var(--surface-sunken)] text-[var(--text-secondary)]"
              aria-label={t('pos.fullscreen')}
              title={t('pos.fullscreen')}
              onclick={toggleFullscreen}
            >
              <i class={fullscreen ? 'icon-minimize text-[16px]' : 'icon-maximize text-[16px]'}></i>
            </button>
          </div>

          <div class="relative">
            <button
              type="button"
              class="flex items-center gap-2.5 ps-1 pe-2 h-10 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-sunken)]"
              aria-haspopup="menu"
              aria-expanded={menu}
              onclick={(e) => {
                e.stopPropagation();
                menu = !menu;
              }}
            >
              <span class="grid place-items-center size-8 rounded-full bg-[var(--color-primary-600)] text-white text-[12px] font-bold">{initials(platform.admin?.name)}</span>
              <span class="hidden md:block text-start leading-tight">
                <span class="block text-[12.5px] font-semibold max-w-40 truncate">{platform.admin?.name}</span>
                <span class="block text-[10.5px] max-w-40 truncate text-[var(--text-tertiary)]">{t('platform.brand')}</span>
              </span>
              <i class="icon-chevron-down text-[10px] text-[var(--text-tertiary)]"></i>
            </button>
            {#if menu}
              <div class="absolute end-0 mt-2 w-52 surface-card p-1.5 z-50" role="menu">
                <a href="/platform/security" class="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-key-round text-[15px]"></i>{t('platform.nav.security')}</a>
                <button type="button" onclick={platformLogout} class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium text-[var(--color-danger-600)] hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-log-out text-[15px]"></i>{t('platform.nav.signOut')}</button>
              </div>
            {/if}
          </div>
        </div>
      </div>
    </header>

    <div class="p-4 lg:p-6 w-full max-w-[1400px] mx-auto space-y-5">
      {#if page.url.pathname === '/platform/tenants'}<CommandHero />{/if}
      {@render children()}
    </div>
  </div>
</div>

<style>
  /* Outline putih mengikuti kontur logo (logo biru tua di atas sidebar gelap). */
  .logo-outline {
    filter: drop-shadow(1.5px 0 0 #fff) drop-shadow(-1.5px 0 0 #fff) drop-shadow(0 1.5px 0 #fff) drop-shadow(0 -1.5px 0 #fff);
  }
</style>
