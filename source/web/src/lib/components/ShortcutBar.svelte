<script lang="ts">
  // Jalan pintas fitur: tombol di header yang membuka panel berisi kotak-kotak besar, tersimpan di server per akun.
  // Tujuan dipilih dari menu yang boleh diakses pengguna; alamat lain tetap bisa diketik.
  import { t } from '#lib/i18n/index.ts';
  import { api } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select, { type SelectOption } from '#lib/components/Select.svelte';
  import { session, can } from '#lib/auth/session.svelte.ts';
  import { visibleNav } from '#lib/nav.ts';

  type Shortcut = { id: string; title: string; url: string; icon: string };
  const CUSTOM = '__custom';
  const MAX = 8;

  let shortcuts = $state<Shortcut[]>([]);
  let loadedKey = $state('');
  let loading = $state(false);
  let saving = $state(false);
  let error = $state('');
  let open = $state(false);
  let managing = $state(false);
  let editorOpen = $state(false);
  let editingId = $state<string | null>(null);
  let title = $state('');
  let autoTitle = '';
  let url = $state('');
  let target = $state('');
  let icon = $state('');
  let formError = $state('');
  let iconInput = $state<HTMLInputElement>();
  const storageKey = $derived(`aciraba.feature-shortcuts.v1:${session.tenant?.id ?? ''}:${session.user?.id ?? ''}`);

  // Menu yang boleh dibuka pengguna → pilihan tujuan + ikon bawaan tiap kotak.
  const menu = $derived.by(() => {
    const options: SelectOption[] = [];
    const meta = new Map<string, { icon: string; label: string }>();
    for (const group of visibleNav((m) => can(m) || (m === 'price_override' && can('outlet_switch')))) {
      for (const item of group.items) {
        if (item.href && !meta.has(item.href)) {
          meta.set(item.href, { icon: item.icon, label: t(item.labelKey) });
          options.push({ value: item.href, label: t(item.labelKey), hint: t(group.titleKey) });
        }
        for (const c of item.children ?? []) {
          if (c.href && !meta.has(c.href)) {
            meta.set(c.href, { icon: item.icon, label: t(c.labelKey) });
            options.push({ value: c.href, label: t(c.labelKey), hint: t(item.labelKey) });
          }
        }
      }
    }
    options.push({ value: CUSTOM, label: t('shell.shortcuts.customUrl') });
    return { options, meta };
  });

  function readLegacy(key: string): Shortcut[] {
    try {
      const value: unknown = JSON.parse(localStorage.getItem(key) ?? '[]');
      if (!Array.isArray(value)) return [];
      return value
        .filter((i): i is Shortcut => i && typeof i.id === 'string' && typeof i.title === 'string' && typeof i.url === 'string' && typeof i.icon === 'string')
        .slice(0, MAX);
    } catch {
      return [];
    }
  }

  $effect(() => {
    const key = storageKey;
    if (!session.user?.id || !session.tenant?.id || loadedKey === key) return;
    void load(key);
  });

  async function load(key: string) {
    loading = true;
    error = '';
    try {
      let result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts');
      if (storageKey !== key) return;
      if (result.shortcuts.length === 0) {
        const legacy = readLegacy(key).filter((i) => validUrl(i.url));
        if (legacy.length) result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts', { method: 'PUT', body: JSON.stringify({ shortcuts: legacy }) });
      }
      if (storageKey !== key) return;
      shortcuts = result.shortcuts;
      loadedKey = key;
      try { localStorage.removeItem(key); } catch { /* cache lokal lama opsional */ }
    } catch {
      if (storageKey === key) {
        shortcuts = readLegacy(key).filter((i) => validUrl(i.url));
        error = t('shell.shortcuts.loadFailed');
        loadedKey = key;
      }
    } finally {
      if (storageKey === key) loading = false;
    }
  }

  async function saveList(next: Shortcut[]): Promise<boolean> {
    saving = true;
    error = '';
    try {
      const result = await api<{ shortcuts: Shortcut[] }>('/auth/feature-shortcuts', { method: 'PUT', body: JSON.stringify({ shortcuts: next }) });
      shortcuts = result.shortcuts;
      return true;
    } catch {
      error = t('shell.shortcuts.saveFailed');
      return false;
    } finally {
      saving = false;
    }
  }

  function validUrl(value: string): boolean {
    const v = value.trim();
    if (v.startsWith('/') && !v.startsWith('//')) {
      try {
        return new URL(v, window.location.origin).origin === window.location.origin;
      } catch {
        return false;
      }
    }
    try {
      const p = new URL(v);
      return p.protocol === 'https:' || p.protocol === 'http:';
    } catch {
      return false;
    }
  }

  const external = (u: string) => /^https?:\/\//i.test(u);
  const pathOf = (u: string) => u.split(/[?#]/)[0];

  function openEditor(item?: Shortcut) {
    open = false;
    editingId = item?.id ?? null;
    title = item?.title ?? '';
    url = item?.url ?? '';
    icon = item?.icon ?? '';
    target = item ? (menu.meta.has(pathOf(item.url)) && pathOf(item.url) === item.url ? item.url : CUSTOM) : '';
    autoTitle = '';
    formError = '';
    editorOpen = true;
  }

  function pick(value: string) {
    if (value === CUSTOM) return;
    url = value;
    const label = menu.meta.get(value)?.label ?? '';
    if (label && (!title.trim() || title === autoTitle)) title = label;
    autoTitle = label;
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const cleanTitle = title.trim();
    const cleanUrl = url.trim();
    if (!cleanTitle || !cleanUrl || !validUrl(cleanUrl)) {
      formError = t('shell.shortcuts.urlInvalid');
      return;
    }
    if (!editingId && shortcuts.length >= MAX) {
      formError = t('shell.shortcuts.limit');
      return;
    }
    const item = { id: editingId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`, title: cleanTitle, url: cleanUrl, icon };
    const next = editingId ? shortcuts.map((s) => (s.id === editingId ? item : s)) : [...shortcuts, item];
    if (await saveList(next)) editorOpen = false;
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

  const tile = 'relative flex flex-col items-center justify-center gap-1.5 aspect-square w-full min-h-[104px] rounded-xl border px-1.5 text-center';
</script>

<div class="relative me-auto">
  <button type="button" class="hidden sm:inline-flex items-center gap-2 h-10 ps-3.5 pe-3 rounded-full text-[12.5px] font-semibold text-[var(--text-primary)] transition-all hover:bg-[var(--surface-sunken)] border border-[var(--border-subtle)] bg-[var(--surface-raised)]" aria-haspopup="dialog" aria-expanded={open} onclick={(e) => { e.stopPropagation(); open = !open; }}>
    <i class="icon-layout-grid text-[15px] text-[var(--color-primary-600)]"></i>
    <span class="whitespace-nowrap">{t('shell.shortcuts.title')}</span>
    <i class="icon-chevron-down text-[9px] opacity-80"></i>
  </button>
  <button type="button" class="header-icon-btn sm:hidden" aria-label={t('shell.shortcuts.title')} onclick={(e) => { e.stopPropagation(); open = !open; }}>
    <i class="icon-layout-grid text-[17px]"></i>
  </button>
</div>

{#if open}
  <Modal title={t('shell.shortcuts.title')} wide onclose={() => (open = false)}>
      {#if shortcuts.length}
        <div class="mb-3 flex justify-end">
          <button type="button" class="btn btn-sm" aria-pressed={managing} onclick={() => (managing = !managing)}>
            <i class="{managing ? 'icon-check' : 'icon-pencil'} me-1 text-[12px]"></i>{managing ? t('shell.shortcuts.done') : t('shell.shortcuts.manage')}
          </button>
        </div>
      {/if}
      <div class="grid grid-cols-3 gap-3 sm:grid-cols-4" role="list">
        {#each shortcuts as item (item.id)}
        {@const m = menu.meta.get(pathOf(item.url))}
        <div class="relative min-w-0" role="listitem">
          <a
            href={item.url}
            target={external(item.url) ? '_blank' : undefined}
            rel={external(item.url) ? 'noopener noreferrer' : undefined}
            class="{tile} overflow-hidden border-[var(--border-subtle)] bg-[var(--surface-raised)] text-[var(--text-secondary)] transition-colors hover:border-[var(--color-primary-500)] hover:bg-[var(--surface-sunken)]"
            tabindex={managing ? -1 : undefined}
            onclick={(e) => (managing ? e.preventDefault() : (open = false))}
          >
            {#if item.icon}
              <img src={item.icon} alt="" class="absolute inset-0 size-full object-cover" />
              <span class="absolute inset-x-0 bottom-0 truncate bg-[var(--color-primary-600)] px-2 py-2 text-center text-[14px] font-bold leading-tight text-white shadow-[0_-2px_6px_rgba(0,0,0,0.25)]">{item.title}</span>
            {:else}
              <i class="icon-{m?.icon ?? 'link-2'} text-[56px] text-[var(--color-primary-600)]"></i>
              <span class="w-full truncate text-[14px] font-bold leading-tight">{item.title}</span>
            {/if}
          </a>
          {#if managing}
            <button type="button" disabled={saving || !!error} class="absolute -end-1.5 -top-1.5 grid size-7 place-items-center rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] text-[var(--text-secondary)] shadow-sm disabled:opacity-50" title={t('shell.shortcuts.edit')} aria-label={t('shell.shortcuts.edit')} onclick={() => openEditor(item)}><i class="icon-pencil text-[12px]"></i></button>
            <button type="button" disabled={saving || !!error} class="absolute -start-1.5 -top-1.5 grid size-7 place-items-center rounded-full border border-[var(--border-subtle)] bg-[var(--surface-raised)] text-[var(--color-danger-600)] shadow-sm disabled:opacity-50" title={t('shell.shortcuts.remove')} aria-label={t('shell.shortcuts.remove')} onclick={() => saveList(shortcuts.filter((s) => s.id !== item.id))}><i class="icon-x text-[13px]"></i></button>
          {/if}
        </div>
      {/each}
      {#if shortcuts.length < MAX}
        <button type="button" disabled={loading || saving || !!error || loadedKey !== storageKey} class="{tile} border-dashed border-[var(--border-default)] text-[var(--text-tertiary)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-primary-600)] disabled:cursor-not-allowed disabled:opacity-50" onclick={() => openEditor()}>
          <i class="icon-plus text-[40px]"></i><span class="text-[12px] font-medium leading-tight">{t('shell.shortcuts.add')}</span>
        </button>
      {/if}

      </div>
        {#if loading}
        <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('shell.shortcuts.loading')}</p>
      {:else if error}
        <p class="mt-1 text-[11px] text-[var(--color-danger-600)]" role="alert">
          {error}
          <button type="button" class="ms-1 font-semibold text-[var(--color-primary-600)]" onclick={() => (loadedKey = '')}>{t('shell.shortcuts.retry')}</button>
        </p>
      {/if}

  </Modal>
{/if}

{#if editorOpen}
  <Modal title={editingId ? t('shell.shortcuts.edit') : t('shell.shortcuts.add')} onclose={() => (editorOpen = false)}>
    <form class="space-y-4" onsubmit={submit}>
      <div class="space-y-1.5">
        <label for="sc-target" class="text-[12px] font-semibold">{t('shell.shortcuts.target')}</label>
        <Select id="sc-target" bind:value={target} options={menu.options} placeholder={t('shell.shortcuts.targetPh')} onchange={pick} />
      </div>
      {#if target === CUSTOM}
        <label class="block space-y-1.5">
          <span class="text-[12px] font-semibold">{t('shell.shortcuts.url')}</span>
          <input class="field-control w-full" bind:value={url} placeholder="/kasir" maxlength="2048" required />
        </label>
      {/if}
      <label class="block space-y-1.5">
        <span class="text-[12px] font-semibold">{t('shell.shortcuts.name')}</span>
        <input class="field-control w-full" bind:value={title} maxlength="40" required />
      </label>
      <div class="space-y-2">
        <span class="block text-[12px] font-semibold">{t('shell.shortcuts.icon')}</span>
        <div class="flex items-center gap-3">
          {#if icon}<img src={icon} alt="" class="size-10 rounded border border-[var(--border-subtle)] object-contain" />{:else}<span class="grid size-10 place-items-center rounded border border-[var(--border-subtle)] text-[var(--text-tertiary)]"><i class="icon-{menu.meta.get(url)?.icon ?? 'link-2'} text-[20px]"></i></span>{/if}
          <input bind:this={iconInput} class="sr-only" type="file" accept="image/png,image/jpeg,image/webp" onchange={readIcon} />
          <button type="button" class="btn btn-secondary btn-sm" onclick={() => iconInput?.click()}><i class="icon-upload text-[13px]"></i>{t('shell.shortcuts.upload')}</button>
          {#if icon}<button type="button" class="header-icon-btn !size-8" aria-label={t('shell.shortcuts.removeIcon')} onclick={() => (icon = '')}><i class="icon-trash-2 text-[13px]"></i></button>{/if}
        </div>
        <p class="text-[11px] text-[var(--text-tertiary)]">{t('shell.shortcuts.iconHint')}</p>
      </div>
      {#if formError}<p class="text-[12px] text-[var(--color-danger-600)]" role="alert">{formError}</p>{/if}
      <div class="flex justify-end gap-2 border-t border-[var(--border-subtle)] pt-4">
        <button type="button" class="btn btn-secondary" onclick={() => (editorOpen = false)}>{t('catalog.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={saving}><i class="icon-check text-[14px]"></i>{t('catalog.save')}</button>
      </div>
    </form>
  </Modal>
{/if}
