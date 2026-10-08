<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { session, can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import { drafts } from '#lib/tabs/drafts.ts';
  import { tabs, tabMeta, HOME } from '#lib/tabs/store.svelte.ts';

  const active = $derived(page.url.pathname);

  // Tab milik pengguna ini; dimuat sekali saat identitas sesi diketahui.
  $effect(() => {
    if (session.user && session.tenant) tabs.load(`tabs:${session.tenant.id}:${session.user.id}`);
  });

  // Setiap navigasi ke halaman yang dikenal membuka (atau memakai ulang) tabnya.
  $effect(() => {
    const path = active;
    untrack(() => tabs.open(path)); // hanya navigasi yang membuka tab, bukan perubahan daftar tab
  });

  // Tab yang izinnya sudah dicabut tidak ditampilkan.
  const visible = $derived(
    tabs.paths.filter((p) => {
      const m = tabMeta(p);
      return m && (!m.module || p === HOME || can(m.module));
    })
  );

  function close(path: string) {
    const run = () => {
      // Menutup tab = membuang drafnya; form aktif juga diberi tahu agar tidak menyimpannya lagi saat dilepas.
      if (path === active) drafts.discard(path);
      else drafts.clear(path);
      const next = tabs.close(path, active);
      if (next) void goto(next);
    };
    // Hanya tab aktif yang punya form hidup; tab lain tidak perlu dikonfirmasi.
    if (path === active) guard.request(run);
    else run();
  }

  // ---- Menu klik kanan: tutup tab / lainnya / di kanan / semua ----
  // Draf di halaman form (item/member) berisi isian pengguna; halaman lain hanya menyimpan filter dan dibuang tanpa tanya.
  const FORM_PATH = /^\/(items|members)\/[^/]+$/;
  let menu = $state<{ path: string; x: number; y: number } | null>(null);

  function openMenu(e: MouseEvent, path: string) {
    e.preventDefault();
    menu = { path, x: Math.min(e.clientX, window.innerWidth - 220), y: e.clientY };
  }

  const closable = (list: string[]) => list.filter((p) => p !== HOME);

  function closeGroup(list: string[]) {
    menu = null;
    const targets = closable(list);
    if (!targets.length) return;
    const run = () => {
      for (const p of targets) {
        if (p === active) drafts.discard(p);
        else drafts.clear(p);
      }
      const next = tabs.closeMany(targets, active);
      if (next) void goto(next);
    };
    const hasWork = targets.some((p) => (p === active ? guard.dirty : FORM_PATH.test(p) && drafts.has(p)));
    if (hasWork) guard.ask(run);
    else run();
  }

  const menuItems = $derived.by(() => {
    if (!menu) return [];
    const i = visible.indexOf(menu.path);
    return [
      { key: 'close', disabled: menu.path === HOME, run: () => closeGroup([menu!.path]) },
      { key: 'closeOthers', disabled: !closable(visible.filter((p) => p !== menu!.path)).length, run: () => closeGroup(visible.filter((p) => p !== menu!.path)) },
      { key: 'closeRight', disabled: !closable(visible.slice(i + 1)).length, run: () => closeGroup(visible.slice(i + 1)) },
      { key: 'closeAll', disabled: !closable(visible).length, run: () => closeGroup(visible) }
    ] as const;
  });

  function onAux(e: MouseEvent, path: string) {
    if (e.button === 1 && path !== HOME) {
      e.preventDefault();
      close(path);
    }
  }
</script>

<div class="tabbar scroll-thin" role="tablist" aria-label={t('shell.tabs')}>
  {#each visible as path (path)}
    {@const meta = tabMeta(path)!}
    <div class="tab" class:is-active={path === active} role="presentation" onauxclick={(e) => onAux(e, path)} oncontextmenu={(e) => openMenu(e, path)}>
      <a href={path} role="tab" aria-selected={path === active} class="tab-link">{t(meta.titleKey)}</a>
      {#if path !== HOME}
        <button type="button" class="tab-close" aria-label={t('shell.closeTab')} onclick={() => close(path)}>
          <i class="icon-x text-[12px]"></i>
        </button>
      {/if}
    </div>
  {/each}
</div>

<svelte:window onkeydown={(e) => e.key === 'Escape' && (menu = null)} onresize={() => (menu = null)} />
{#if menu}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-[90]" onclick={() => (menu = null)} oncontextmenu={(e) => { e.preventDefault(); menu = null; }}></div>
  <div class="tab-menu" role="menu" style="left:{menu.x}px; top:{menu.y}px">
    {#each menuItems as m (m.key)}
      <button type="button" role="menuitem" class="tab-menu-item" disabled={m.disabled} onclick={m.run}>{t(`shell.tabMenu.${m.key}`)}</button>
    {/each}
  </div>
{/if}

<style>
  .tabbar {
    display: flex;
    gap: 2px;
    overflow-x: auto;
    padding: 6px 12px 0;
    border-bottom: 1px solid var(--color-border, rgb(0 0 0 / 0.1));
  }
  .tab {
    display: flex;
    align-items: center;
    flex: 0 0 auto;
    max-width: 14rem;
    border: 1px solid transparent;
    border-bottom: 0;
    border-radius: 8px 8px 0 0;
    font-size: 12.5px;
    opacity: 0.75;
  }
  .tab:hover {
    opacity: 1;
    background: rgb(127 127 127 / 0.1);
  }
  .tab.is-active {
    opacity: 1;
    font-weight: 600;
    border-color: rgb(127 127 127 / 0.25);
    background: rgb(127 127 127 / 0.18);
    margin-bottom: -1px;
  }
  .tab-link {
    padding: 7px 4px 7px 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tab-close {
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    margin: 0 6px 0 2px;
    border-radius: 4px;
  }
  .tab-close:hover {
    background: rgb(127 127 127 / 0.25);
  }
  .tab-menu {
    position: fixed;
    z-index: 91;
    min-width: 210px;
    padding: 4px;
    border-radius: 8px;
    border: 1px solid rgb(127 127 127 / 0.3);
    background: var(--surface-raised, var(--surface-sunken, #fff));
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.25);
  }
  .tab-menu-item {
    display: block;
    width: 100%;
    padding: 7px 10px;
    border-radius: 5px;
    text-align: left;
    font-size: 12.5px;
  }
  .tab-menu-item:hover:not(:disabled) {
    background: rgb(127 127 127 / 0.2);
  }
  .tab-menu-item:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .tab:not(:has(.tab-close)) .tab-link {
    padding-right: 12px;
  }
</style>
