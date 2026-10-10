<script lang="ts">
  import { t } from '#lib/i18n/index.ts';
  import { api } from '#lib/api/client.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { session, logout } from '#lib/auth/session.svelte.ts';
  import { initials } from '#lib/auth/initials.ts';

  let { onmenu }: { onmenu: () => void } = $props();

  type Theme = 'light' | 'dark';
  type Shortcut = { id: string; title: string; url: string; icon: string };
  let theme = $state<Theme>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  let menu = $state<'' | 'features' | 'notif' | 'user'>('');
  let shortcuts = $state<Shortcut[]>([]);
  let loadedKey = $state('');
  let loadingShortcuts = $state(false);
  let savingShortcuts = $state(false);
  let shortcutsError = $state('');
  let editorOpen = $state(false);
  let editingId = $state<string | null>(null);
  let title = $state('');
  let url = $state('');
  let icon = $state('');
  let formError = $state('');
  let iconInput = $state<HTMLInputElement>();
  const storageKey = $derived(`aciraba.feature-shortcuts.v1:${session.tenant?.id ?? ''}:${session.user?.id ?? ''}`);

  function readShortcuts(key: string): Shortcut[] {
    try {
      const value: unknown = JSON.parse(localStorage.getItem(key) ?? '[]');
      if (!Array.isArray(value)) return [];
      return value.filter((item): item is Shortcut =>
        item && typeof item.id === 'string' && typeof item.title === 'string' &&
        typeof item.url === 'string' && typeof item.icon === 'string'
      ).slice(0, 8);
    } catch {
      return [];
    }
  }

  $effect(() => {
    const key = storageKey;
    if (!session.user?.id || !session.tenant?.id || loadedKey === key) return;
    void loadShortcuts(key);
  });

  async function loadShortcuts(key: string) {
    loadingShortcuts = true;
    shortcutsError = '';
    try {
      let result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts');
      if (storageKey !== key) return;
      if (result.shortcuts.length === 0) {
        const legacy = readShortcuts(key).filter((item) => validUrl(item.url));
        if (legacy.length) result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts', { method: 'PUT', body: JSON.stringify({ shortcuts: legacy }) });
      }
      if (storageKey !== key) return;
      shortcuts = result.shortcuts;
      loadedKey = key;
      try { localStorage.removeItem(key); } catch { /* Cache lokal lama opsional. */ }
    } catch {
      if (storageKey === key) {
        shortcuts = readShortcuts(key).filter((item) => validUrl(item.url));
        shortcutsError = t('shell.shortcuts.loadFailed');
        loadedKey = key;
      }
    } finally {
      if (storageKey === key) loadingShortcuts = false;
    }
  }

  async function saveShortcutList(next: Shortcut[]): Promise<boolean> {
    savingShortcuts = true;
    shortcutsError = '';
    try {
      const result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts', { method: 'PUT', body: JSON.stringify({ shortcuts: next }) });
      shortcuts = result.shortcuts;
      return true;
    } catch {
      shortcutsError = t('shell.shortcuts.saveFailed');
      return false;
    } finally {
      savingShortcuts = false;
    }
  }

  function retryLoadShortcuts() {
    loadedKey = '';
  }

  function toggle(name: 'features' | 'notif' | 'user') {
    menu = menu === name ? '' : name;
  }

  function openEditor(item?: Shortcut) {
    editingId = item?.id ?? null;
    title = item?.title ?? '';
    url = item?.url ?? '';
    icon = item?.icon ?? '';
    formError = '';
    editorOpen = true;
    menu = '';
  }

  function validUrl(value: string): boolean {
    const trimmed = value.trim();
    if (trimmed.startsWith('/') && !trimmed.startsWith('//')) {
      try {
        return new URL(trimmed, window.location.origin).origin === window.location.origin;
      } catch {
        return false;
      }
    }
    try {
      const parsed = new URL(trimmed);
      return parsed.protocol === 'https:' || parsed.protocol === 'http:';
    } catch {
      return false;
    }
  }

  async function saveShortcut(event: SubmitEvent) {
    event.preventDefault();
    const cleanTitle = title.trim();
    const cleanUrl = url.trim();
    if (!cleanTitle || !cleanUrl || !validUrl(cleanUrl)) {
      formError = t('shell.shortcuts.urlInvalid');
      return;
    }
    if (!editingId && shortcuts.length >= 8) {
      formError = t('shell.shortcuts.limit');
      return;
    }
    const item = { id: editingId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`, title: cleanTitle, url: cleanUrl, icon };
    const next = editingId ? shortcuts.map((shortcut) => shortcut.id === editingId ? item : shortcut) : [...shortcuts, item];
    if (await saveShortcutList(next)) editorOpen = false;
  }

  function readIcon(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 128 * 1024) {
      formError = t('shell.shortcuts.iconInvalid');
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      icon = typeof reader.result === 'string' ? reader.result : '';
      formError = '';
    };
    reader.onerror = () => (formError = t('shell.shortcuts.iconInvalid'));
    reader.readAsDataURL(file);
  }

  async function removeShortcut(id: string) {
    await saveShortcutList(shortcuts.filter((shortcut) => shortcut.id !== id));
  }

  function external(url: string): boolean {
    return /^https?:\/\//i.test(url);
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
      <!-- Pintasan fitur -->
      <div class="relative me-auto">
        <button
          type="button"
          class="hidden sm:inline-flex items-center gap-2 h-10 ps-3.5 pe-3 rounded-full text-[12.5px] font-semibold text-[var(--text-primary)] transition-all hover:bg-[var(--surface-sunken)] border border-[var(--border-subtle)] bg-[var(--surface-raised)]"
          aria-haspopup="menu"
          aria-expanded={menu === 'features'}
          onclick={(e) => {
            e.stopPropagation();
            toggle('features');
          }}
        >
          <i class="icon-layout-grid text-[15px] text-[var(--color-primary-600)]"></i>
          <span class="whitespace-nowrap">{t('shell.shortcuts.title')}</span>
          <i class="icon-chevron-down text-[9px] opacity-80"></i>
        </button>
        <button
          type="button"
          class="header-icon-btn sm:hidden"
          aria-label={t('shell.shortcuts.title')}
          onclick={(e) => {
            e.stopPropagation();
            toggle('features');
          }}
        >
          <i class="icon-layout-grid text-[17px]"></i>
        </button>
        {#if menu === 'features'}
          <div class="absolute start-0 mt-2 w-[min(540px,calc(100vw-2rem))] overflow-hidden rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-raised)] shadow-[var(--shadow-lg)] z-50" role="menu" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
            <div class="border-b border-[var(--border-subtle)] px-3 py-2 text-[11px] font-semibold text-[var(--text-secondary)]">
              {t('shell.shortcuts.outlet', { outlet: session.outlet?.name ?? session.tenant?.name ?? '' })}
            </div>
            <div class="grid sm:grid-cols-2">
              <section class="flex min-h-48 flex-col justify-center bg-[var(--color-primary-600)] p-5 text-white">
                <p class="font-display text-[21px] font-bold">{t('shell.shortcuts.welcome')}</p>
                <p class="mt-2 max-w-56 text-[12px] leading-relaxed text-white/90">{t('shell.shortcuts.welcomeBody')}</p>
              </section>
              <section class="p-4">
                <h2 class="mb-3 text-[13px] font-semibold">{t('shell.shortcuts.features')}</h2>
                {#if loadingShortcuts}
                  <p class="py-4 text-center text-[11px] text-[var(--text-tertiary)]">{t('shell.shortcuts.loading')}</p>
                {:else if shortcutsError}
                  <p class="text-[11px] text-[var(--color-danger-600)]" role="alert">{shortcutsError}</p>
                  <button type="button" class="mt-2 text-[11px] font-semibold text-[var(--color-primary-600)]" onclick={retryLoadShortcuts}>{t('shell.shortcuts.retry')}</button>
                {/if}
                <div class="grid grid-cols-3 gap-1">
                  {#each shortcuts as item (item.id)}
                    <div class="group relative min-w-0">
                      <a href={item.url} target={external(item.url) ? '_blank' : undefined} rel={external(item.url) ? 'noopener noreferrer' : undefined} role="menuitem" class="flex min-h-[86px] flex-col items-center justify-center gap-2 rounded-md px-1 py-2 text-center text-[11px] text-[var(--text-secondary)] hover:bg-[var(--surface-sunken)]">
                        {#if item.icon}
                          <img src={item.icon} alt="" class="size-7 rounded object-contain" />
                        {:else}
                          <i class="icon-link-2 text-[25px] text-[var(--text-tertiary)]"></i>
                        {/if}
                        <span class="max-w-full truncate">{item.title}</span>
                      </a>
                      <button type="button" disabled={savingShortcuts || !!shortcutsError} class="absolute end-0 top-0 hidden size-6 place-items-center rounded bg-[var(--surface-raised)] text-[var(--text-tertiary)] shadow-sm group-hover:grid focus:grid disabled:!hidden" title={t('shell.shortcuts.edit')} aria-label={t('shell.shortcuts.edit')} onclick={() => openEditor(item)}><i class="icon-pencil text-[11px]"></i></button>
                      <button type="button" disabled={savingShortcuts || !!shortcutsError} class="absolute start-0 top-0 hidden size-6 place-items-center rounded bg-[var(--surface-raised)] text-[var(--color-danger-600)] shadow-sm group-hover:grid focus:grid disabled:!hidden" title={t('shell.shortcuts.remove')} aria-label={t('shell.shortcuts.remove')} onclick={() => removeShortcut(item.id)}><i class="icon-x text-[11px]"></i></button>
                    </div>
                  {:else}
                    <p class="col-span-3 py-4 text-center text-[11px] text-[var(--text-tertiary)]">{t('shell.shortcuts.empty')}</p>
                  {/each}
                  {#if shortcuts.length < 8}
                    <button type="button" disabled={loadingShortcuts || savingShortcuts || !!shortcutsError || loadedKey !== storageKey} class="flex min-h-[86px] flex-col items-center justify-center gap-2 rounded-md border border-dashed border-[var(--border-default)] px-1 py-2 text-center text-[11px] text-[var(--text-tertiary)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-primary-600)] disabled:cursor-not-allowed disabled:opacity-50" onclick={() => openEditor()}>
                      <i class="icon-plus text-[24px]"></i><span>{t('shell.shortcuts.add')}</span>
                    </button>
                  {/if}
                </div>
              </section>
            </div>
          </div>
        {/if}
      </div>

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
            <a href="#top" class="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-settings text-[15px]"></i>{t('shell.settings')}</a>
            <button type="button" onclick={logout} class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] font-medium text-[var(--color-danger-600)] hover:bg-[var(--surface-sunken)]" role="menuitem"><i class="icon-log-out text-[15px]"></i>{t('shell.signOut')}</button>
          </div>
        {/if}
      </div>
    </div>
  </div>
</header>

{#if editorOpen}
  <Modal title={editingId ? t('shell.shortcuts.edit') : t('shell.shortcuts.add')} onclose={() => (editorOpen = false)}>
    <form class="space-y-4" onsubmit={saveShortcut}>
      <label class="block space-y-1.5">
        <span class="text-[12px] font-semibold">{t('shell.shortcuts.name')}</span>
        <input class="form-control w-full" bind:value={title} maxlength="40" required />
      </label>
      <label class="block space-y-1.5">
        <span class="text-[12px] font-semibold">{t('shell.shortcuts.url')}</span>
        <input class="form-control w-full" bind:value={url} placeholder="/kasir" maxlength="2048" required />
      </label>
      <div class="space-y-2">
        <span class="block text-[12px] font-semibold">{t('shell.shortcuts.icon')}</span>
        <div class="flex items-center gap-3">
          {#if icon}<img src={icon} alt="" class="size-10 rounded border border-[var(--border-subtle)] object-contain" />{:else}<span class="grid size-10 place-items-center rounded border border-[var(--border-subtle)] text-[var(--text-tertiary)]"><i class="icon-link-2 text-[20px]"></i></span>{/if}
          <input bind:this={iconInput} class="sr-only" type="file" accept="image/png,image/jpeg,image/webp" onchange={readIcon} />
          <button type="button" class="btn btn-secondary btn-sm" onclick={() => iconInput?.click()}><i class="icon-upload text-[13px]"></i>{t('shell.shortcuts.upload')}</button>
          {#if icon}<button type="button" class="header-icon-btn !size-8" aria-label={t('shell.shortcuts.removeIcon')} onclick={() => (icon = '')}><i class="icon-trash-2 text-[13px]"></i></button>{/if}
        </div>
        <p class="text-[11px] text-[var(--text-tertiary)]">{t('shell.shortcuts.iconHint')}</p>
      </div>
      {#if formError}<p class="text-[12px] text-[var(--color-danger-600)]" role="alert">{formError}</p>{/if}
      <div class="flex justify-end gap-2 border-t border-[var(--border-subtle)] pt-4">
        <button type="button" class="btn btn-secondary" onclick={() => (editorOpen = false)}>{t('catalog.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={savingShortcuts}><i class="icon-check text-[14px]"></i>{t('catalog.save')}</button>
      </div>
    </form>
  </Modal>
{/if}
