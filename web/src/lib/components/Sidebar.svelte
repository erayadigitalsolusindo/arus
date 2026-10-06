<script lang="ts">
  import { page } from '$app/state';
  import { nav } from '#lib/nav.ts';

  let {
    collapsed,
    mobileOpen,
    ontoggle,
    onclose
  }: { collapsed: boolean; mobileOpen: boolean; ontoggle: () => void; onclose: () => void } = $props();

  let open = $state<Record<string, boolean>>({ 'Master Data': true });
</script>

{#if mobileOpen}
  <button type="button" class="sidebar-backdrop lg:hidden" aria-label="Tutup menu" onclick={onclose}></button>
{/if}

<aside class="app-sidebar sidebar-pattern scroll-thin" class:is-mobile-open={mobileOpen} aria-label="Sidebar">
  <div class="flex items-center justify-between h-16 px-4 shrink-0 border-b border-[var(--sidebar-border)]">
    <a href="/dashboard" class="flex items-center gap-2.5 min-w-0 rounded-xl px-2.5 py-1.5">
      <img src="/logo-light.svg" alt="ACIRABA" class="sidebar-logo-full max-w-[150px] object-contain shrink-0" />
      <img src="/mark.svg" alt="ACIRABA" class="sidebar-logo-small size-8 object-contain shrink-0 hidden" />
    </a>
    <button
      type="button"
      class="header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-8 shrink-0 max-lg:hidden"
      aria-label={collapsed ? 'Perluas sidebar' : 'Ciutkan sidebar'}
      onclick={ontoggle}
    >
      <i class="icon-panel-left-close text-[17px]"></i>
    </button>
  </div>

  <nav class="sidebar-inner flex-1 overflow-y-auto scroll-thin px-3 pb-4 mt-1" aria-label="Navigasi utama">
    <ul role="menu">
      {#each nav as group (group.title)}
        <li class="sidebar-group-title">{group.title}</li>
        {#each group.items as item (item.label)}
          {#if item.children}
            <li class="has-submenu" class:is-open={open[item.label]}>
              <button
                type="button"
                class="sidebar-link w-full"
                aria-expanded={!!open[item.label]}
                onclick={() => (open[item.label] = !open[item.label])}
              >
                <i class="icon-{item.icon} text-[16px]"></i>
                <span class="sidebar-label">{item.label}</span>
                <i class="icon-chevron-right sidebar-chevron text-[11px]"></i>
              </button>
              <ul class="sidebar-submenu">
                {#each item.children as child (child.label)}
                  <li>
                    <a href={child.href ?? '#'} class="sidebar-link !text-[12.5px]">
                      <span class="sidebar-label">{child.label}</span>
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
                <span class="sidebar-label">{item.label}</span>
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
        <span class="block text-[12.5px] font-semibold text-white truncate">Belum masuk</span>
        <span class="block text-[10.5px] truncate text-[var(--sidebar-text)]">Auth: Fase 2</span>
      </span>
      <a href="/login" class="workspace-text header-icon-btn !text-white/60 hover:!text-white hover:!bg-white/10 !size-7 shrink-0" title="Keluar" aria-label="Keluar">
        <i class="icon-log-out text-[14px]"></i>
      </a>
    </div>
  </div>
</aside>
