<script lang="ts">
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import { session, logout } from '#lib/auth/session.svelte.ts';
  import { initials } from '#lib/auth/initials.ts';

  let { onmenu }: { onmenu: () => void } = $props();

  type Theme = 'light' | 'dark';
  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  let menu = $state<'' | 'quick' | 'notif' | 'user'>('');

  const quickActions: { icon: string; labelKey: MessageKey }[] = [
    { icon: 'shopping-cart', labelKey: 'shell.quick.newSale' },
    { icon: 'package', labelKey: 'shell.quick.newItem' },
    { icon: 'user-plus', labelKey: 'shell.quick.newCustomer' },
    { icon: 'truck', labelKey: 'shell.quick.newPurchase' }
  ];

  function toggle(name: 'quick' | 'notif' | 'user') {
    menu = menu === name ? '' : name;
  }

  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('theme', theme);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }
</script>

<svelte:window
  onclick={() => (menu = '')}
  onkeydown={(e) => {
    if (e.key === 'Escape') menu = '';
  }}
/>

<header class="app-header">
  <div class="absolute inset-x-0 bottom-0 h-px pointer-events-none [background:linear-gradient(90deg,transparent,var(--color-primary-500)_20%,var(--color-accent-500)_50%,var(--color-primary-500)_80%,transparent)] opacity-[0.55]"></div>

  <div class="flex items-center gap-3 px-4 lg:px-6 h-16">
    <button type="button" class="header-icon-btn lg:hidden" aria-label={t('shell.openMenu')} onclick={onmenu}>
      <i class="icon-menu text-[18px]"></i>
    </button>

    <a href="/dashboard" class="lg:hidden shrink-0" aria-label="ACIRABA">
      <img src="/logo_tanpa_text-no-bg.svg" alt="ARUS" class="h-8 w-auto object-contain" />
    </a>

    <!-- Pencarian global (palet perintah menyusul) -->
    <button
      type="button"
      class="group hidden md:flex items-center gap-2.5 w-full max-w-[240px] xl:max-w-[300px] h-10 pe-3.5 ps-1.5 rounded-full transition-all hover:shadow-[var(--shadow-sm)] bg-[var(--surface-sunken)] border border-[var(--border-subtle)]"
    >
      <span class="grid place-items-center size-7 rounded-full shrink-0 bg-[color-mix(in_oklab,var(--color-primary-500)_16%,transparent)] text-[var(--color-primary-600)]">
        <i class="icon-search text-[13px]"></i>
      </span>
      <span class="text-[13px] flex-1 text-start truncate font-medium text-[var(--text-tertiary)]">{t('shell.search')}</span>
      <kbd class="text-[10px] font-semibold px-1.5 py-0.5 rounded-md shrink-0 bg-[var(--surface-raised)] text-[var(--text-tertiary)] border border-[var(--border-default)]">Ctrl K</kbd>
    </button>

    <div class="ms-auto flex items-center gap-2.5 lg:gap-3">
      <!-- Quick Create -->
      <div class="relative">
        <button
          type="button"
          class="hidden sm:inline-flex items-center gap-2 h-10 ps-3.5 pe-3 rounded-full text-[12.5px] font-semibold text-white transition-all hover:-translate-y-px bg-[var(--color-primary-600)] shadow-[0_6px_14px_-6px_color-mix(in_oklab,var(--color-primary-600)_60%,transparent)]"
          aria-haspopup="menu"
          aria-expanded={menu === 'quick'}
          onclick={(e) => {
            e.stopPropagation();
            toggle('quick');
          }}
        >
          <i class="icon-plus text-[13px]"></i>
          <span>{t('shell.quickCreate')}</span>
          <i class="icon-chevron-down text-[9px] opacity-80"></i>
        </button>
        <button
          type="button"
          class="header-icon-btn sm:hidden"
          aria-label={t('shell.quickCreate')}
          onclick={(e) => {
            e.stopPropagation();
            toggle('quick');
          }}
        >
          <i class="icon-plus text-[17px]"></i>
        </button>
        {#if menu === 'quick'}
          <div class="absolute end-0 mt-2 w-56 surface-card p-1.5 z-50" role="menu">
            {#each quickActions as a (a.labelKey)}
              <a href="#top" class="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem">
                <i class="icon-{a.icon} text-[15px] text-[var(--color-primary-600)]"></i>{t(a.labelKey)}
              </a>
            {/each}
          </div>
        {/if}
      </div>

      <LanguageSwitcher />

      <!-- Tema -->
      <button
        type="button"
        class="grid place-items-center size-7 rounded-full transition-transform hover:scale-105 bg-[color-mix(in_oklab,var(--color-warning-500)_16%,transparent)] text-[var(--color-warning-600)]"
        aria-label={theme === 'dark' ? t('common.theme.light') : t('common.theme.dark')}
        onclick={toggleTheme}
      >
        <i class={theme === 'dark' ? 'icon-moon text-[13.5px]' : 'icon-sun-medium text-[13.5px]'}></i>
      </button>

      <!-- Notifikasi -->
      <div class="relative">
        <button
          type="button"
          class="header-icon-btn relative"
          aria-label={t('shell.notifications')}
          aria-haspopup="menu"
          aria-expanded={menu === 'notif'}
          onclick={(e) => {
            e.stopPropagation();
            toggle('notif');
          }}
        >
          <i class="icon-bell text-[17px]"></i>
        </button>
        {#if menu === 'notif'}
          <div class="absolute end-0 mt-2 w-72 surface-card p-0 z-50 overflow-hidden" role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
            <div class="px-4 py-3 border-b border-[var(--border-subtle)]">
              <p class="font-display font-bold text-[14px]">{t('shell.notifications')}</p>
            </div>
            <div class="px-4 py-8 text-center text-[12.5px] text-[var(--text-tertiary)]">
              <i class="icon-bell-off text-[22px] block mb-2"></i>{t('shell.noNotifications')}
            </div>
          </div>
        {/if}
      </div>

      <!-- Pengguna -->
      <div class="relative">
        <button
          type="button"
          class="flex items-center gap-2.5 ps-1 pe-2 h-10 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-sunken)]"
          aria-haspopup="menu"
          aria-expanded={menu === 'user'}
          onclick={(e) => {
            e.stopPropagation();
            toggle('user');
          }}
        >
          <span class="grid place-items-center size-8 rounded-full bg-[var(--color-primary-600)] text-white text-[12px] font-bold">{initials(session.user?.name)}</span>
          <span class="hidden md:block text-start leading-tight">
            <span class="block text-[12.5px] font-semibold max-w-40 truncate">{session.user?.name}</span>
            <span class="block text-[10.5px] max-w-40 truncate text-[var(--text-tertiary)]">{session.tenant?.name}</span>
          </span>
          <i class="icon-chevron-down text-[10px] text-[var(--text-tertiary)]"></i>
        </button>
        {#if menu === 'user'}
          <div class="absolute end-0 mt-2 w-52 surface-card p-1.5 z-50" role="menu">
            <a href="#top" class="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-user text-[15px]"></i>{t('shell.profile')}</a>
            <a href="#top" class="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-settings text-[15px]"></i>{t('shell.settings')}</a>
            <button type="button" onclick={logout} class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium text-[var(--color-danger-600)] hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-log-out text-[15px]"></i>{t('shell.signOut')}</button>
          </div>
        {/if}
      </div>
    </div>
  </div>
</header>
