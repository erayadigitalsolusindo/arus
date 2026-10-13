<script lang="ts">
  import { t } from '#lib/i18n/index.ts';
  import ShortcutBar from '#lib/components/ShortcutBar.svelte';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import { session, logout } from '#lib/auth/session.svelte.ts';
  import { initials } from '#lib/auth/initials.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { api } from '#lib/api/client.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { onmenu }: { onmenu: () => void } = $props();

  type Theme = 'light' | 'dark';
  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  let menu = $state<'' | 'notif' | 'user'>('');
  type EcosystemStatus = { active: boolean; merchant_status?: string; owner_role?: string };
  let ecosystemOpen = $state(false);
  let ecosystemLoading = $state(false);
  let ecosystemActivating = $state(false);
  let ecosystemStatus = $state<EcosystemStatus | null>(null);
  let ecosystemError = $state('');
  let acceptedErapayTerms = $state(false);
  function toggle(name: 'notif' | 'user') {
    menu = menu === name ? '' : name;
  }

  let fullscreen = $state(!!document.fullscreenElement);

  function toggleFullscreen() {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void document.documentElement.requestFullscreen?.();
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

  async function refreshEcosystem() {
    ecosystemLoading = true;
    ecosystemStatus = null;
    ecosystemError = '';
    try {
      ecosystemStatus = await api<EcosystemStatus>('/ecosystem/erapay/status');
    } catch (error) {
      ecosystemError = errorMessage(error);
    } finally {
      ecosystemLoading = false;
    }
  }

  function openEcosystem() {
    menu = '';
    acceptedErapayTerms = false;
    ecosystemOpen = true;
    void refreshEcosystem();
  }

  async function activateErapay() {
    ecosystemActivating = true;
    ecosystemError = '';
    try {
      ecosystemStatus = await api<EcosystemStatus>('/ecosystem/erapay/activate', {
        method: 'POST',
        body: JSON.stringify({ accepted_terms: acceptedErapayTerms })
      });
    } catch (error) {
      ecosystemError = errorMessage(error);
    } finally {
      ecosystemActivating = false;
    }
  }
</script>

<svelte:document onfullscreenchange={() => (fullscreen = !!document.fullscreenElement)} />

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

    <div class="flex w-full items-center gap-2.5 lg:gap-3">
      <ShortcutBar />

      <!-- Tema -->
      <button
        type="button"
        class="grid place-items-center size-10 rounded-full transition-transform hover:scale-105 bg-[color-mix(in_oklab,var(--color-warning-500)_16%,transparent)] text-[var(--color-warning-600)]"
        aria-label={theme === 'dark' ? t('common.theme.light') : t('common.theme.dark')}
        onclick={toggleTheme}
      >
        <i class={theme === 'dark' ? 'icon-moon text-[17px]' : 'icon-sun-medium text-[17px]'}></i>
      </button>

      <!-- Kapsul aksi: bahasa, layar penuh, notifikasi -->
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

        <!-- Notifikasi -->
        <div class="relative">
        <button
          type="button"
          class="grid place-items-center size-9 rounded-full transition-transform hover:scale-105 bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)] text-[var(--color-danger-600)]"
          aria-label={t('shell.notifications')}
          aria-haspopup="menu"
          aria-expanded={menu === 'notif'}
          onclick={(e) => {
            e.stopPropagation();
            toggle('notif');
          }}
        >
          <i class="icon-bell text-[16px]"></i>
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
            <button type="button" onclick={(e) => { e.stopPropagation(); openEcosystem(); }} class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-settings text-[15px]"></i>{t('shell.settings')}</button>
            <button type="button" onclick={logout} class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium text-[var(--color-danger-600)] hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-log-out text-[15px]"></i>{t('shell.signOut')}</button>
          </div>
        {/if}
      </div>
    </div>
  </div>
</header>

{#if ecosystemOpen}
  <Modal title={t('shell.ecosystem.title')} onclose={() => (ecosystemOpen = false)}>
    <div class="surface-card p-4 sm:p-5">
      <div class="flex items-center justify-between gap-4">
        <img src="/erapay-logo.svg" alt="EraPay" class="h-12 w-44 object-contain object-left" />
        {#if ecosystemLoading}
          <span class="badge">{t('shell.ecosystem.checking')}</span>
        {:else if ecosystemStatus}
          <span class="badge {ecosystemStatus.active ? 'badge-success' : 'badge-muted'}">
            {ecosystemStatus.active ? t('shell.ecosystem.active') : t('shell.ecosystem.inactive')}
          </span>
        {:else}
          <span class="badge badge-muted">{t('shell.ecosystem.unknown')}</span>
        {/if}
      </div>

      {#if ecosystemLoading}
        <p class="mt-4 text-sm text-[var(--text-secondary)]">{t('shell.ecosystem.checkingDetails')}</p>
      {:else if ecosystemError}
        <div class="mt-4" role="alert">
          <p class="text-sm text-[var(--color-danger-600)]">{ecosystemError}</p>
          <button type="button" class="btn btn-secondary mt-3" onclick={() => void refreshEcosystem()}>
            <i class="icon-refresh-cw me-1.5"></i>{t('shell.ecosystem.retry')}
          </button>
        </div>
      {:else if ecosystemStatus?.active}
        <p class="mt-4 text-sm font-semibold">{t('shell.ecosystem.ownerConnected')}</p>
        {#if ecosystemStatus.merchant_status !== 'approved'}
          <p class="mt-2 text-xs text-[var(--text-secondary)]">{t('shell.ecosystem.reviewPending')}</p>
        {/if}
      {:else if ecosystemStatus}
        <p class="mt-4 text-sm text-[var(--text-secondary)]">{t('shell.ecosystem.intro')}</p>
        <label class="mt-4 flex items-start gap-2.5 text-xs leading-5">
          <input type="checkbox" bind:checked={acceptedErapayTerms} class="mt-0.5 size-4 accent-[var(--color-primary-600)]" />
          <span>{t('shell.ecosystem.acceptTerms')}</span>
        </label>
        <p class="mt-2 text-xs text-[var(--text-tertiary)]">{t('shell.ecosystem.ownerInvite')}</p>
        <button type="button" class="btn btn-primary mt-4 w-full" disabled={!acceptedErapayTerms || ecosystemActivating} onclick={() => void activateErapay()}>
          {#if ecosystemActivating}<i class="icon-loader-circle me-1.5 animate-spin"></i>{t('shell.ecosystem.activating')}{:else}<i class="icon-link me-1.5"></i>{t('shell.ecosystem.activate')}{/if}
        </button>
      {/if}
    </div>
  </Modal>
{/if}
