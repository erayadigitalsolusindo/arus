<script lang="ts">
  import { session, logout, can } from '#lib/auth/session.svelte.ts';
  import { page } from '$app/state';
  import { visibleNav, siakNav, SIAK_PREFIX, type NavItem } from '#lib/nav.ts';
  import { t } from '#lib/i18n/index.ts';
  import OutletSwitcher from '#lib/components/OutletSwitcher.svelte';

  let {
    collapsed,
    mobileOpen,
    ontoggle,
    onclose
  }: { collapsed: boolean; mobileOpen: boolean; ontoggle: () => void; onclose: () => void } = $props();

  // Menu disaring menurut izin pengguna (penegakan sebenarnya di server).
  const allowed = (m: string) => can(m) || (m === 'price_override' && can('outlet_switch'));

  // Mode SIAK: di halaman akuntansi, menu ARUS diganti menu SIAK (dengan tautan kembali ke ARUS).
  const siakMode = $derived(page.url.pathname === SIAK_PREFIX || page.url.pathname.startsWith(SIAK_PREFIX + '/'));
  const siakGroups = $derived(visibleNav(allowed, siakNav));
  const siakEntry = $derived(siakGroups[0]?.items[0]?.href ?? '/accounting/accounts');
  const groups = $derived(
    siakMode
      ? siakGroups
      : visibleNav(allowed).map((g) => ({ ...g, items: g.items.map((i) => (i.id === 'siak' ? { ...i, href: siakEntry } : i)) }))
  );

  // Semua submenu tertutup saat halaman dimuat/di-reload.
  let open = $state<Record<string, boolean>>({});
  let bubbleItem = $state<NavItem | null>(null);
  let bubbleTop = $state(12);
  let bubbleTimer: ReturnType<typeof setTimeout> | undefined;

  function showBubble(item: NavItem, target: HTMLElement) {
    if (!collapsed) return;
    if (bubbleTimer) clearTimeout(bubbleTimer);
    bubbleItem = item;
    const top = target.getBoundingClientRect().top;
    bubbleTop = Math.max(12, Math.min(top, window.innerHeight - 290));
  }

  function hideBubble() {
    if (bubbleTimer) clearTimeout(bubbleTimer);
    bubbleTimer = setTimeout(() => (bubbleItem = null), 180);
  }

  function keepBubble() {
    if (bubbleTimer) clearTimeout(bubbleTimer);
  }

  $effect(() => {
    if (!collapsed) bubbleItem = null;
  });

  // Mode SIAK: submenu yang memuat halaman aktif terbuka otomatis.
  $effect(() => {
    if (!siakMode) return;
    for (const g of siakGroups)
      for (const i of g.items) if (i.children?.some((c) => c.href === page.url.pathname)) open[i.id] = true;
  });
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

  {#if !session.impersonating}<OutletSwitcher />{/if}

  <nav class="sidebar-inner flex-1 overflow-y-auto scroll-thin px-3 pb-4 mt-1" aria-label={t('shell.mainNav')}>
    <ul role="menu">
      {#if siakMode}
        <li>
          <a href="/dashboard" class="sidebar-link" aria-label={t('nav.backToArus')}>
            <i class="icon-arrow-left text-[16px]"></i>
            <span class="sidebar-label">{t('nav.backToArus')}</span>
          </a>
        </li>
      {/if}
      {#each groups as group (group.titleKey)}
        <li class="sidebar-group-title">{t(group.titleKey)}</li>
        {#each group.items as item (item.id)}
          {#if item.children}
            <li class="has-submenu" class:is-open={open[item.id]}>
              <button
                type="button"
                class="sidebar-link w-full"
                aria-expanded={!!open[item.id]}
                aria-label={t(item.labelKey)}
                onmouseenter={(event) => showBubble(item, event.currentTarget)}
                onmouseleave={hideBubble}
                onfocus={(event) => showBubble(item, event.currentTarget)}
                onclick={(event) => {
                  open[item.id] = !open[item.id];
                  if (collapsed) {
                    if (open[item.id]) showBubble(item, event.currentTarget);
                    else bubbleItem = null;
                  }
                }}
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
                aria-label={t(item.labelKey)}
                onmouseenter={(event) => showBubble(item, event.currentTarget)}
                onmouseleave={hideBubble}
                onfocus={(event) => showBubble(item, event.currentTarget)}
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

  {#if collapsed && bubbleItem}
    <div
      class="sidebar-nav-bubble"
      style:top="{bubbleTop}px"
      onmouseenter={keepBubble}
      onmouseleave={hideBubble}
      onfocusin={keepBubble}
      onfocusout={hideBubble}
    >
      <div class="sidebar-nav-bubble-title">{t(bubbleItem.labelKey)}</div>
      {#if bubbleItem.children?.length}
        <ul>
          {#each bubbleItem.children as child (child.labelKey)}
            <li>
              <a
                href={child.href ?? '#'}
                class:is-active={child.href && page.url.pathname === child.href}
                aria-current={child.href && page.url.pathname === child.href ? 'page' : undefined}
                onclick={() => (bubbleItem = null)}
              >
                {t(child.labelKey)}
                <i class="icon-chevron-right sidebar-bubble-arrow" aria-hidden="true"></i>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

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

  .sidebar-nav-bubble {
    position: fixed;
    inset-inline-start: 92px;
    z-index: 60;
    width: 224px;
    max-height: calc(100dvh - 24px);
    overflow-y: auto;
    padding: 8px;
    border: 1px solid rgb(255 255 255 / 14%);
    border-radius: 14px;
    background: var(--sidebar-bg);
    box-shadow: 0 14px 36px rgb(0 0 0 / 28%);
    color: var(--sidebar-text-active);
    animation: nav-bubble-in 140ms ease-out;
  }

  .sidebar-nav-bubble::before {
    content: '';
    position: absolute;
    inset-inline-start: -8px;
    top: 14px;
    width: 10px;
    height: 18px;
    clip-path: polygon(100% 0, 100% 100%, 0 50%);
    background: var(--sidebar-bg);
    filter: drop-shadow(-1px 0 0 rgb(255 255 255 / 20%));
  }

  .sidebar-nav-bubble:dir(rtl)::before {
    clip-path: polygon(0 0, 0 100%, 100% 50%);
    filter: drop-shadow(1px 0 0 rgb(255 255 255 / 20%));
  }

  .sidebar-nav-bubble-title {
    padding: 5px 8px 4px;
    color: rgb(255 255 255 / 58%);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: .04em;
    text-transform: uppercase;
  }

  .sidebar-nav-bubble ul {
    display: grid;
    gap: 0;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .sidebar-nav-bubble li {
    margin: 0 !important;
    padding: 0;
  }

  .sidebar-nav-bubble a {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    min-height: 32px;
    padding: 5px 8px;
    border-radius: 9px;
    color: var(--sidebar-text-active);
    font-size: 12px;
    line-height: 1.25;
    transition: background-color 120ms ease, color 120ms ease;
  }

  .sidebar-nav-bubble a:hover,
  .sidebar-nav-bubble a:focus-visible {
    outline: none;
    background: rgb(255 255 255 / 10%);
    color: white;
  }

  .sidebar-nav-bubble a.is-active {
    background: rgb(255 255 255 / 10%);
    color: white;
  }

  .sidebar-bubble-arrow {
    flex: none;
    opacity: 0;
    transform: translateX(-3px);
    transition: opacity 120ms ease, transform 120ms ease;
  }

  .sidebar-nav-bubble a:hover .sidebar-bubble-arrow,
  .sidebar-nav-bubble a:focus-visible .sidebar-bubble-arrow,
  .sidebar-nav-bubble a.is-active .sidebar-bubble-arrow {
    opacity: 1;
    transform: translateX(0);
  }

  @keyframes nav-bubble-in {
    from { opacity: 0; transform: translateX(-4px) scale(.98); }
    to { opacity: 1; transform: translateX(0) scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    .sidebar-nav-bubble { animation: none; }
  }
</style>
