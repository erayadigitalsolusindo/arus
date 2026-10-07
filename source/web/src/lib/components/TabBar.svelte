<script lang="ts">
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { session, can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
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
      const next = tabs.close(path, active);
      if (next) void goto(next);
    };
    // Hanya tab aktif yang punya form hidup; tab lain tidak perlu dikonfirmasi.
    if (path === active) guard.request(run);
    else run();
  }

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
    <div class="tab" class:is-active={path === active} role="presentation" onauxclick={(e) => onAux(e, path)}>
      <a href={path} role="tab" aria-selected={path === active} class="tab-link">{t(meta.titleKey)}</a>
      {#if path !== HOME}
        <button type="button" class="tab-close" aria-label={t('shell.closeTab')} onclick={() => close(path)}>
          <i class="icon-x text-[12px]"></i>
        </button>
      {/if}
    </div>
  {/each}
</div>

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
  .tab:not(:has(.tab-close)) .tab-link {
    padding-right: 12px;
  }
</style>
