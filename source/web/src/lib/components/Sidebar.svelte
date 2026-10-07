<script lang="ts">
  import { session, logout } from '#lib/auth/session.svelte.ts';
  import { page } from '$app/state';
  import { nav } from '#lib/nav.ts';
  import { t } from '#lib/i18n/index.ts';

  let {
    collapsed,
    mobileOpen,
    ontoggle,
    onclose
  }: { collapsed: boolean; mobileOpen: boolean; ontoggle: () => void; onclose: () => void } = $props();

  // Semua submenu tertutup saat halaman dimuat/di-reload.
  let open = $state<Record<string, boolean>>({});
</script>

{#if mobileOpen}
  <button type="button" class="sidebar-backdrop lg:hidden" aria-label={t('shell.closeMenu')} onclick={onclose}></button>
{/if}

<aside class="app-sidebar sidebar-pattern scroll-thin" class:is-mobile-open={mobileOpen} aria-label={t('shell.sidebar')}>
  <div class="flex items-center justify-between h-16 px-4 shrink-0 border-b border-[var(--sidebar-border)]">
    <a href="/dashboard" class="flex items-center gap-2.5 min-w-0 rounded-xl px-2.5 py-1.5">
      <img src="/logo_dengan_text-no-bg.svg" alt="ARUS" class="sidebar-logo-full logo-outline h-10 w-auto object-contain shrink-0" />
      <img src="/logo_tanpa_text-no-bg.svg" alt="ARUS" class="sidebar-logo-small logo-outline h-8 w-auto object-contain shrink-0 hidden" />
    </a>
    <button
      type="button"
      class="header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-8 shrink-0 max-lg:hidden"
      aria-label={collapsed ? t('shell.expandSidebar') : t('shell.collapseSidebar')}
      onclick={ontoggle}
    >
      <i class="icon-panel-left-close text-[17px]"></i>
    </button>
  </div>

  <div class="sidebar-outlet workspace-text px-4 pt-3 pb-1 shrink-0">
    <span class="block text-[11px] font-semibold uppercase tracking-wide text-[var(--sidebar-text)]">
      {t('shell.currentOutlet')} [{session.outlet?.code}]
    </span>
    <!-- Pindah outlet butuh endpoint penerbit token dengan outlet baru; sementara hanya outlet aktif yang tampil. -->
    <select
      class="mt-1.5 w-full rounded-lg px-2.5 py-2 text-[12.5px] font-medium outline-none cursor-pointer"
      style="background-color: rgb(255 255 255 / 0.08) !important; color: #fff; border: 1px solid var(--sidebar-border); color-scheme: dark"
      aria-label={t('shell.switchOutlet')}
    >
      {#if session.outlet}<option class="text-black" value={session.outlet.id}>{session.outlet.name}</option>{/if}
    </select>
  </div>

  <nav class="sidebar-inner flex-1 overflow-y-auto scroll-thin px-3 pb-4 mt-1" aria-label={t('shell.mainNav')}>
    <ul role="menu">
      {#each nav as group (group.titleKey)}
        <li class="sidebar-group-title">{t(group.titleKey)}</li>
        {#each group.items as item (item.id)}
          {#if item.children}
            <li class="has-submenu" class:is-open={open[item.id]}>
              <button
                type="button"
                class="sidebar-link w-full"
                aria-expanded={!!open[item.id]}
                onclick={() => (open[item.id] = !open[item.id])}
              >
                <i class="icon-{item.icon} text-[16px]"></i>
                <span class="sidebar-label">{t(item.labelKey)}</span>
                <i class="icon-chevron-right sidebar-chevron text-[11px]"></i>
              </button>
              <ul class="sidebar-submenu">
                {#each item.children as child (child.labelKey)}
                  <li>
                    <a href={child.href ?? '#'} class="sidebar-link !text-[12.5px]">
                      <span class="sidebar-label">{t(child.labelKey)}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            </li>
          {:else}
            <li>
              <a
                href={item.href ?? '#'}
                class="sidebar-link"
                class:is-active={item.href && page.url.pathname === item.href}
                aria-current={item.href && page.url.pathname === item.href ? 'page' : undefined}
              >
                <i class="icon-{item.icon} text-[16px]"></i>
                <span class="sidebar-label">{t(item.labelKey)}</span>
              </a>
            </li>
          {/if}
        {/each}
      {/each}
    </ul>
  </nav>

  <div class="p-3 border-t shrink-0 border-[var(--sidebar-border)]">
    <div class="w-full flex items-center gap-2.5 rounded-lg p-2 bg-[rgb(255_255_255_/_0.04)]">
      <span class="grid place-items-center size-8 rounded-full shrink-0 bg-[rgb(255_255_255_/_0.12)] text-white">
        <i class="icon-user text-[15px]"></i>
      </span>
      <span class="workspace-text text-left min-w-0 flex-1">
        <span class="block text-[12.5px] font-semibold text-white truncate">{session.user?.name}</span>
        <span class="block text-[10.5px] truncate text-[var(--sidebar-text)]">{session.outlet?.name}</span>
      </span>
      <button type="button" onclick={logout} class="workspace-text header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-7 shrink-0" title={t('shell.signOut')} aria-label={t('shell.signOut')}>
        <i class="icon-log-out text-[14px]"></i>
      </button>
    </div>
  </div>
</aside>

<style>
  /* Submenu tertutup tetap membawa margin dari template, sehingga baris bersubmenu lebih tinggi dari baris biasa. */
  .has-submenu:not(.is-open) > :global(.sidebar-submenu) {
    margin-block: 0;
  }

  /* Outline putih mengikuti kontur logo (logo biru tua di atas sidebar gelap). */
  .logo-outline {
    filter: drop-shadow(1.5px 0 0 #fff) drop-shadow(-1.5px 0 0 #fff) drop-shadow(0 1.5px 0 #fff) drop-shadow(0 -1.5px 0 #fff);
  }
</style>
