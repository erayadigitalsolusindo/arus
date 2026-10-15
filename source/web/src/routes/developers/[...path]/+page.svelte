<script lang="ts">
  import '#lib/developers/portal.css';
  import { onMount, tick } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { t } from '#lib/i18n/index.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import { loadPortal, baseUrlOf, type Brand, type SpecInfo, type Catalog, type Endpoint } from '#lib/developers/api.ts';
  import Guides, { type GuideView } from '#lib/developers/Guides.svelte';
  import EndpointCard from '#lib/developers/EndpointCard.svelte';
  import CodeBlock from '#lib/developers/CodeBlock.svelte';

  type Loaded = { brand: Brand; spec: SpecInfo; catalog: Catalog };
  let data = $state<Loaded | null>(null);
  let failed = $state(false);
  let theme = $state<'dark' | 'light'>('dark');
  let drawer = $state(false);
  let query = $state('');
  let searchEl = $state<HTMLInputElement>();
  let searchOpen = $state(false);
  let cursor = $state(0);

  // ---- routing berbasis path: /developers/<panduan> · /developers/api/<grup>[?ep=METHOD /path] ----
  type Route = { kind: 'guide'; id: GuideView } | { kind: 'group'; id: string; ep?: string };
  const GUIDES: GuideView[] = ['overview', 'quickstart', 'auth', 'playground', 'errors', 'limits', 'conventions'];
  const ROOT = '/developers';
  let route = $state<Route>({ kind: 'guide', id: 'overview' });
  let openEps = $state<Record<string, boolean>>({});

  const epKey = (e: Endpoint) => `${e.method} ${e.path}`;
  const guideHref = (g: GuideView) => (g === 'overview' ? ROOT : `${ROOT}/${g}`);
  const groupHref = (id: string) => `${ROOT}/api/${id}`;
  const epHref = (e: Endpoint) => `${groupHref(e.group)}?ep=${encodeURIComponent(epKey(e))}`;

  function applyUrl(url: { pathname: string; searchParams: { get(name: string): string | null } }) {
    const seg = url.pathname.replace(/^\/developers\/?/, '').split('/').filter(Boolean).map(decodeURIComponent);
    if (seg[0] === 'api' && seg[1]) {
      const key = url.searchParams.get('ep') ?? undefined;
      route = { kind: 'group', id: seg[1], ep: key };
      const ep = key && data ? data.catalog.endpoints.find((e) => epKey(e) === key) : undefined;
      if (ep) {
        openEps[epKey(ep)] = true;
        // Tunggu DOM grup dirender dulu, baru gulir ke kartu endpoint (scroll-margin menyisakan ruang header).
        const scrollTo = () => document.getElementById(`ep-${ep.method}-${ep.path}`)?.scrollIntoView({ block: 'start' });
        void tick().then(scrollTo);
        setTimeout(scrollTo, 120); // SvelteKit menggulir ke atas setelah navigasi; gulir ulang ke kartu
      }
    } else if ((GUIDES as string[]).includes(seg[0] ?? '')) route = { kind: 'guide', id: seg[0] as GuideView };
    else route = { kind: 'guide', id: 'overview' };
    drawer = false;
  }
  const go = (href: string) => void goto(href);

  // Sinkronkan tampilan dengan URL (juga saat tombol back/forward) setelah katalog termuat.
  $effect(() => {
    const url = page.url;
    if (data) applyUrl(url);
  });

  onMount(() => {
    try {
      const saved = localStorage.getItem('dv-theme');
      if (saved === 'light' || saved === 'dark') theme = saved;
    } catch {
      /* penyimpanan diblokir: pakai bawaan */
    }
    void load();
  });

  async function load() {
    failed = false;
    try {
      data = await loadPortal(location.hostname);
      document.title = `${data.brand.name} · ${t('extapi.portal.docTitle')}`;
    } catch {
      failed = true;
    }
  }

  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    try {
      localStorage.setItem('dv-theme', theme);
    } catch {
      /* abaikan */
    }
  }

  // ---- pencarian ----
  const results = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!data || q.length < 2) return [];
    const scored: { ep: Endpoint; score: number }[] = [];
    for (const ep of data.catalog.endpoints) {
      const hay = `${ep.method} ${ep.path} ${ep.summary} ${ep.group}`.toLowerCase();
      if (!q.split(/\s+/).every((w) => hay.includes(w))) continue;
      scored.push({ ep, score: (ep.path.toLowerCase().includes(q) ? 0 : 2) + (ep.summary.toLowerCase().includes(q) ? 0 : 1) + ep.path.length / 1000 });
    }
    return scored.sort((a, b) => a.score - b.score).slice(0, 9).map((x) => x.ep);
  });
  $effect(() => {
    void query;
    cursor = 0;
  });
  function pick(ep: Endpoint) {
    query = '';
    searchOpen = false;
    go(epHref(ep));
  }
  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      cursor = Math.min(cursor + 1, results.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      cursor = Math.max(cursor - 1, 0);
    } else if (e.key === 'Enter' && results[cursor]) pick(results[cursor]);
    else if (e.key === 'Escape') {
      query = '';
      searchEl?.blur();
    }
  }
  function onWindowKey(e: KeyboardEvent) {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test((e.target as HTMLElement)?.tagName ?? '');
    if ((e.key === '/' && !typing) || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k')) {
      e.preventDefault();
      searchEl?.focus();
    }
  }

  // ---- turunan ----
  const base = $derived(data ? baseUrlOf(data.brand) : '');
  const groupEps = $derived(route.kind === 'group' && data ? data.catalog.endpoints.filter((e) => e.group === route.id) : []);
  const group = $derived(route.kind === 'group' && data ? data.catalog.groups.find((g) => g.id === route.id) : undefined);
  const inkOn = (hex: string) => {
    const n = parseInt(hex.slice(1), 16);
    const l = (0.299 * (n >> 16) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255)) / 255;
    return l > 0.58 ? '#06121a' : '#ffffff';
  };
  const accentStyle = $derived(data ? `--dv-accent:${data.brand.accent};--dv-accent-ink:${inkOn(data.brand.accent)}` : '');

  // Buka endpoint pertama otomatis saat grup dipilih tanpa target.
  $effect(() => {
    if (route.kind === 'group' && !route.ep && groupEps.length) {
      const first = epKey(groupEps[0]);
      if (!(first in openEps)) openEps[first] = true;
    }
  });

  const heroSample = `curl -sS https://api.example.com/ext/v1/items?q=kopi \\
  -H "X-Api-Key: ak_7q2k…" \\
  -H "X-Api-Timestamp: 1760000000" \\
  -H "X-Api-Nonce: 3f9c…e1" \\
  -H "X-Api-Signature: 4bc13d…1366"`;
  const heroResponse = `HTTP/1.1 200 OK
{
  "items": [{ "id": "0b1f…", "name": "Kopi Susu", "price": "18000.00" }],
  "next_cursor": null
}`;
</script>

<svelte:window onkeydown={onWindowKey} />
<svelte:head>
  <title>{data ? `${data.brand.name} · ${t('extapi.portal.docTitle')}` : t('extapi.portal.docTitle')}</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="dv" data-theme={theme} style={accentStyle}>
  <header class="dv-top">
    <button type="button" class="dv-iconbtn dv-burger" aria-label={t('extapi.portal.menu')} onclick={() => (drawer = !drawer)}><i class="icon-menu" style="font-size:16px"></i></button>
    <a class="dv-brand" href={ROOT}>
      {#if data?.brand.logo_url}
        <img src={data.brand.logo_url} alt={data.brand.name} />
      {:else}
        <span class="dv-mark">{(data?.brand.name ?? 'A').slice(0, 1).toUpperCase()}</span>
        <span>{data?.brand.name ?? ''}</span>
      {/if}
    </a>
    <span class="dv-pill hide-sm">{t('extapi.portal.docTitle')}</span>

    <div class="dv-search">
      <i class="icon-search" style="position:absolute;left:12px;top:11px;font-size:14px;color:var(--dv-muted)"></i>
      <input
        bind:this={searchEl}
        bind:value={query}
        type="search"
        placeholder={t('extapi.portal.search')}
        aria-label={t('extapi.portal.search')}
        autocomplete="off"
        onfocus={() => (searchOpen = true)}
        onblur={() => setTimeout(() => (searchOpen = false), 120)}
        onkeydown={onSearchKey}
      />
      <span class="dv-kbd">/</span>
      {#if searchOpen && query.trim().length >= 2}
        <div class="dv-results" role="listbox">
          {#each results as ep, i (epKey(ep))}
            <button type="button" class="dv-result" role="option" aria-selected={i === cursor} onmousedown={() => pick(ep)}>
              <span class="dv-m" data-m={ep.method}>{ep.method}</span>
              <span class="mono" style="font-size:12px;word-break:break-all">{ep.path}</span>
              <span style="margin-left:auto;color:var(--dv-muted)">{ep.summary}</span>
            </button>
          {:else}
            <div class="dv-note" style="border:0">{t('extapi.portal.searchEmpty')}</div>
          {/each}
        </div>
      {/if}
    </div>

    {#if data}<span class="dv-pill hide-sm" title={t('extapi.portal.baseUrl')}><i class="icon-globe" style="font-size:12px"></i>{base}</span>{/if}
    <LanguageSwitcher />
    <button type="button" class="dv-iconbtn" aria-label={t('extapi.portal.theme')} onclick={toggleTheme}><i class="icon-{theme === 'dark' ? 'sun' : 'moon'}" style="font-size:15px"></i></button>
  </header>

  {#if failed}
    <div class="dv-main"><div class="dv-page">
      <div class="dv-warn">{t('extapi.portal.loadFailed')}</div>
      <p style="margin-top:14px"><button type="button" class="dv-btn" onclick={load}>{t('extapi.portal.retry')}</button></p>
    </div></div>
  {:else if !data}
    <div class="dv-main"><div class="dv-page" style="display:grid;gap:12px" aria-busy="true" aria-label={t('extapi.portal.loading')}>
      <div class="dv-skeleton" style="width:38%;height:34px"></div><div class="dv-skeleton" style="width:70%"></div><div class="dv-skeleton" style="width:56%"></div><div class="dv-skeleton" style="height:180px"></div>
    </div></div>
  {:else}
    {#if route.kind === 'guide' && route.id === 'overview'}
      <section class="dv-hero">
        <div class="dv-hero-grid">
          <div>
            <span class="dv-pill"><span style="width:7px;height:7px;border-radius:50%;background:var(--dv-get);box-shadow:0 0 10px var(--dv-get)"></span>{t('extapi.portal.hero.badge')}</span>
            <h1>{data.brand.name}<br /><em>{data.brand.tagline || t('extapi.portal.docTitle')}</em><span class="dv-caret"></span></h1>
            <p class="lead">{t('extapi.portal.overview.lead')}</p>
            <div style="display:flex;gap:10px;flex-wrap:wrap;margin-top:22px">
              <a class="dv-btn" style="text-decoration:none" href={guideHref('quickstart')}><i class="icon-rocket" style="font-size:14px"></i>{t('extapi.portal.nav.quickstart')}</a>
              <a class="dv-btn ghost" style="text-decoration:none" href={guideHref('auth')}>{t('extapi.portal.nav.auth')}</a>
            </div>
            <div class="dv-stats">
              <div class="dv-stat"><b>{data.catalog.endpoints.length}</b><span>{t('extapi.portal.hero.stats.endpoints')}</span></div>
              <div class="dv-stat"><b>{data.catalog.groups.length}</b><span>{t('extapi.portal.hero.stats.groups')}</span></div>
              <div class="dv-stat"><b>HMAC</b><span>{t('extapi.portal.hero.stats.signed')}</span></div>
            </div>
          </div>
          <div style="display:grid;gap:12px;min-width:0">
            <CodeBlock code={heroSample} lang="bash" title="request" />
            <CodeBlock code={heroResponse} lang="json" title="response" />
          </div>
        </div>
      </section>
    {/if}

    <div class="dv-shell">
      <nav class="dv-side" data-open={drawer} aria-label={t('extapi.portal.menu')}>
        <h6>{t('extapi.portal.nav.guides')}</h6>
        {#each GUIDES as g (g)}
          <button type="button" class="dv-nav" aria-current={route.kind === 'guide' && route.id === g ? 'page' : undefined} onclick={() => go(guideHref(g))}>{t(`extapi.portal.nav.${g}`)}</button>
        {/each}
        <h6>{t('extapi.portal.nav.reference')}</h6>
        {#each data.catalog.groups as g (g.id)}
          <button type="button" class="dv-nav" aria-current={route.kind === 'group' && route.id === g.id ? 'page' : undefined} onclick={() => go(groupHref(g.id))}>
            {g.title}<span class="dv-count">{g.count}</span>
          </button>
        {/each}
      </nav>

      <main class="dv-main">
        <div class="dv-page">
          {#if route.kind === 'guide'}
            <Guides view={route.id} spec={data.spec} {base} />
          {:else}
            <h2 class="dv-h2">{group?.title ?? route.id}</h2>
            <p class="dv-lead">{group?.description ?? ''} <span class="dv-chip">{t('extapi.portal.ref.count', { count: groupEps.length })}</span></p>
            {#each groupEps as ep (epKey(ep))}
              <EndpointCard {ep} {base} prefix={data.spec.prefix} open={!!openEps[epKey(ep)]} ontoggle={() => (openEps[epKey(ep)] = !openEps[epKey(ep)])} />
            {/each}
          {/if}

          <footer class="dv-foot">
            {#if data.brand.footer}<p>{data.brand.footer}</p>{/if}
            {#if data.brand.support_email || data.brand.support_url}
              <p>
                {t('extapi.portal.support')}:
                {#if data.brand.support_email}<a href={`mailto:${data.brand.support_email}`}>{data.brand.support_email}</a>{/if}
                {#if data.brand.support_url}<a href={data.brand.support_url} target="_blank" rel="noopener noreferrer">{data.brand.support_url}</a>{/if}
              </p>
            {/if}
            {#if data.brand.sandbox_note}<p>{data.brand.sandbox_note}</p>{/if}
          </footer>
        </div>
      </main>
    </div>
  {/if}
</div>
