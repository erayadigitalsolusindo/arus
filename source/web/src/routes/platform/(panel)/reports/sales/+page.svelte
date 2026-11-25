<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type SalesReportRow, type SalesMonth, type ExpiringRow, type OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  // Uang platform selalu Rupiah (nilai transaksi tenant), tanpa desimal.
  const rp = (n: number) => 'Rp ' + formatNumber(Math.round(n), { maximumFractionDigits: 0 });
  const th = 'px-4 py-2.5 font-semibold';
  const DAY = 86_400_000;

  let error = $state('');
  let loading = $state(true);
  // ---- Penjualan ----
  // Tanggal acuan WIB (operator di WIB), format YYYY-MM-DD.
  const ymd = (d: Date) => new Date(d.getTime() + 7 * 3600_000).toISOString().slice(0, 10);
  const today = () => ymd(new Date());
  let from = $state(ymd(new Date(Date.now() - 29 * DAY)));
  let to = $state(today());
  let all = $state<SalesReportRow[]>([]);
  let openId = $state<string | null>(null);
  let monthly = $state<SalesMonth[] | null>(null);

  // ---- Filter lanjutan (client-side, berlaku untuk Top 10, kartu ringkasan, dan tabel) ----
  const sel = 'h-9 w-full px-2.5 rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px]';
  let range = $state('30');
  const PRESETS = [['7', 7, 'platform.reports.d7'], ['30', 30, 'platform.reports.d30'], ['90', 90, 'platform.reports.d90'], ['month', 'month', 'platform.reports.thisMonth']] as const;
  let showPodium = $state(false);
  let replay = $state(0);
  const DOT = { up: '#16a34a', down: '#f59e0b', flat: '#94a3b8', new: '#0ea5e9', stopped: '#dc2626', none: '#cbd5e1' };
  const lbl = 'block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1';
  let q = $state('');
  let fType = $state('');
  let fLoc = $state('');
  let fStatus = $state('');
  let fMin = $state('');
  let onlyActive = $state(false);
  let onlySelling = $state(false);
  const statusCode = (r: SalesReportRow) => {
    if (r.sales_count === 0) return Number(r.prev_revenue) > 0 ? 'stopped' : 'none';
    const g = pct(Number(r.revenue), Number(r.prev_revenue));
    return g === null ? 'new' : g > 5 ? 'up' : g < -5 ? 'down' : 'flat';
  };
  const STATUSES = ['up', 'down', 'flat', 'new', 'stopped', 'none'] as const;
  const stKey = { up: 'stUp', down: 'stDown', flat: 'stFlat', new: 'stNew', stopped: 'stStopped', none: 'stNone' } as const;
  const distinct = (k: 'tenant_type' | 'location') => [...new Set(all.map((r) => r[k]).filter(Boolean))].sort((a, b) => a.localeCompare(b));
  const types = $derived(distinct('tenant_type'));
  const locs = $derived(distinct('location'));
  const stCount = $derived.by(() => {
    const c = { up: 0, down: 0, flat: 0, new: 0, stopped: 0, none: 0 };
    for (const r of all) c[statusCode(r) as keyof typeof c]++;
    return c;
  });
  const nActive = $derived([q, fType, fLoc, fStatus, fMin, onlyActive, onlySelling].filter(Boolean).length);
  const activeFilters = $derived(!!(q || fType || fLoc || fStatus || fMin || onlyActive || onlySelling));
  function resetFilters() { q = fType = fLoc = fStatus = fMin = ''; onlyActive = onlySelling = false; }
  const sales = $derived.by(() => {
    const needle = q.trim().toLowerCase(), min = Number(fMin) || 0;
    return all.filter((r) =>
      (!needle || r.name.toLowerCase().includes(needle) || r.code.toLowerCase().includes(needle)) &&
      (!fType || r.tenant_type === fType) && (!fLoc || r.location === fLoc) && (!fStatus || statusCode(r) === fStatus) &&
      Number(r.revenue) >= min && (!onlyActive || r.active) && (!onlySelling || r.sales_count > 0));
  });
  // Peringkat berdasarkan omzet; tenant tanpa omzet tidak ikut.
  const top = $derived([...sales].filter((r) => Number(r.revenue) > 0).sort((a, b) => Number(b.revenue) - Number(a.revenue)).slice(0, 10));
  const topMax = $derived(Number(top[0]?.revenue) || 1);
  const podium = $derived([top[1], top[0], top[2]].map((r, i) => r && { r, rank: [2, 1, 3][i] }).filter(Boolean) as { r: SalesReportRow; rank: number }[]);
  const MEDAL: Record<number, { ring: string; grad: string; px: number }> = {
    1: { ring: '#f5b301', grad: 'linear-gradient(180deg,#ffe27a,#f5b301)', px: 150 },
    2: { ring: '#cbd5e1', grad: 'linear-gradient(180deg,#f1f5f9,#94a3b8)', px: 112 },
    3: { ring: '#e08a3c', grad: 'linear-gradient(180deg,#f0b787,#c9772b)', px: 84 }
  };
  const CONFETTI = Array.from({ length: 34 }, (_, i) => ({ x: (i * 29) % 100, d: ((i * 7) % 10) / 10, dur: 2.6 + ((i * 3) % 8) / 5, color: ['#f5b301', '#f472b6', '#38bdf8', '#a3e635', '#fb7185', '#fff'][i % 6] }));

  const totals = $derived.by(() => {
    let rev = 0, prev = 0, count = 0, selling = 0;
    for (const r of sales) {
      rev += Number(r.revenue);
      prev += Number(r.prev_revenue);
      count += r.sales_count;
      if (r.sales_count > 0) selling++;
    }
    return { rev, prev, count, selling };
  });
  const pct = (cur: number, prev: number) => (prev > 0 ? ((cur - prev) / prev) * 100 : null);
  const g = $derived(pct(totals.rev, totals.prev));
  const fmtPct = (v: number) => (v > 0 ? '+' : '') + formatNumber(v, { maximumFractionDigits: 1 }) + '%';

  function status(r: SalesReportRow): { label: string; cls: string } {
    const cur = Number(r.revenue), prev = Number(r.prev_revenue);
    if (r.sales_count === 0) return prev > 0 ? { label: t('platform.reports.stStopped'), cls: 'badge-danger' } : { label: t('platform.reports.stNone'), cls: 'badge-neutral' };
    const g = pct(cur, prev);
    if (g === null) return { label: t('platform.reports.stNew'), cls: 'badge-info' };
    if (g > 5) return { label: t('platform.reports.stUp'), cls: 'badge-success' };
    if (g < -5) return { label: t('platform.reports.stDown'), cls: 'badge-warning' };
    return { label: t('platform.reports.stFlat'), cls: 'badge-neutral' };
  }

  async function loadSales() {
    loading = true;
    error = '';
    try {
      all = (await platformApi.salesReport(from, to)).items;
      openId = null;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  function preset(days: number | 'month') {
    range = String(days);
    const now = new Date();
    to = today();
    from = days === 'month' ? to.slice(0, 8) + '01' : ymd(new Date(now.getTime() - (days - 1) * DAY));
    void loadSales();
  }

  async function toggle(id: string) {
    if (openId === id) {
      openId = null;
      return;
    }
    openId = id;
    monthly = null;
    try {
      const m = await platformApi.salesMonthly(id);
      if (openId === id) monthly = m;
    } catch (err) {
      error = errorMessage(err);
    }
  }
  const maxMonth = $derived(Math.max(1, ...(monthly ?? []).map((m) => Number(m.revenue))));

  onMount(loadSales);
</script>

<div>
  <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.reports.tabSales')}</h1>
  <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.reports.sub')}</p>
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="h-1" style="background:linear-gradient(90deg,var(--color-primary-500),#8b5cf6,#f5b301)"></div>

    <!-- Baris 1: periode -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 pt-3.5 pb-3">
      <div class="flex items-center gap-2">
        <i class="icon-calendar-range text-[15px] text-[var(--text-tertiary)]"></i>
        <div class="w-[250px] max-w-full"><DateRange bind:from bind:to onchange={() => { range = ''; void loadSales(); }} ariaLabel={t('platform.reports.tabSales')} /></div>
      </div>
      <div class="inline-flex p-0.5 rounded-lg bg-[var(--surface-sunken)]" role="group">
        {#each PRESETS as [key, days, label] (key)}
          <button type="button" aria-pressed={range === key} onclick={() => preset(days)}
            class="h-7 px-3 rounded-md text-[12px] font-medium transition {range === key ? 'bg-[var(--surface-card)] shadow-sm text-[var(--color-primary-700)] font-semibold' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}">{t(label)}</button>
        {/each}
      </div>
      <span class="ms-auto inline-flex items-center gap-1.5 text-[12px] text-[var(--text-tertiary)]" title={t('platform.reports.compare')}>
        <i class="icon-info text-[13px]"></i>
        <span class="tabular-nums"><b class="text-[var(--text-primary)]">{sales.length}</b> / {all.length} tenant</span>
      </span>
    </div>

    <!-- Filter lanjutan -->
    <div class="flex items-center gap-2 px-4 py-2 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
      <i class="icon-sliders-horizontal text-[14px] text-[var(--color-primary-600)]"></i>
      <h2 class="text-[12.5px] font-semibold">{t('platform.reports.fAdv')}</h2>
      {#if activeFilters}<span class="badge-info rounded-full px-2 py-0.5 text-[10.5px] font-semibold">{t('platform.reports.fActiveN', { n: nActive })}</span>{/if}
      <div class="ms-auto flex flex-wrap items-center gap-x-4 gap-y-1">
        <label class="sw"><input type="checkbox" role="switch" class="sr-only" bind:checked={onlyActive} /><i></i>{t('platform.reports.fOnlyActive')}</label>
        <label class="sw"><input type="checkbox" role="switch" class="sr-only" bind:checked={onlySelling} /><i></i>{t('platform.reports.fOnlySelling')}</label>
      </div>
      {#if activeFilters}<button type="button" class="btn btn-ghost btn-sm" onclick={resetFilters}><i class="icon-rotate-ccw text-[12px]"></i>{t('platform.reports.fResetS')}</button>{/if}
    </div>

    <div class="p-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,2fr)_repeat(3,minmax(0,1fr))]">
      <label class="block">
        <span class="{lbl}">{t('platform.reports.tenant')}</span>
        <span class="relative block">
          <i class="icon-search absolute start-3 top-1/2 -translate-y-1/2 text-[13px] text-[var(--text-tertiary)] pointer-events-none"></i>
          <input type="text" class={sel} style="padding-inline-start:2.25rem" placeholder={t('platform.reports.fSearchT')} bind:value={q} />
        </span>
      </label>
      <label class="block">
        <span class="{lbl}">{t('platform.access.tenantType')}</span>
        <select class={sel} bind:value={fType}>
          <option value="">{t('platform.reports.fTypeAll')}</option>
          {#each types as x (x)}<option value={x}>{x}</option>{/each}
        </select>
      </label>
      <label class="block">
        <span class="{lbl}">{t('platform.access.tenantLocation')}</span>
        <select class={sel} bind:value={fLoc}>
          <option value="">{t('platform.reports.fLoc')}</option>
          {#each locs as x (x)}<option value={x}>{x}</option>{/each}
        </select>
      </label>
      <label class="block">
        <span class="{lbl}">{t('platform.reports.fMin')}</span>
        <span class="relative block">
          <span class="absolute start-3 top-1/2 -translate-y-1/2 text-[12px] font-semibold text-[var(--text-tertiary)] pointer-events-none">Rp</span>
          <input type="number" min="0" step="any" inputmode="numeric" class="{sel} tabular-nums" style="padding-inline-start:2.5rem" placeholder="0" bind:value={fMin} />
        </span>
      </label>
    </div>

    <div class="px-4 pb-4 flex flex-wrap items-center gap-x-5 gap-y-2.5">
      <div class="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('platform.reports.fStAll')}>
        <button type="button" class="chip {fStatus === '' ? 'on' : ''}" aria-pressed={fStatus === ''} onclick={() => (fStatus = '')}>{t('platform.reports.fStAll')} <b class="tabular-nums">{all.length}</b></button>
        {#each STATUSES as c (c)}
          <button type="button" class="chip {fStatus === c ? 'on' : ''}" aria-pressed={fStatus === c} onclick={() => (fStatus = fStatus === c ? '' : c)}>
            <span class="size-2 rounded-full" style="background:{DOT[c]}"></span>{t(`platform.reports.${stKey[c]}` as const)} <b class="tabular-nums">{stCount[c]}</b>
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
    <StatCard label={t('platform.reports.revenue')} value={rp(totals.rev)} icon="banknote" tone="success" />
    <StatCard label={t('platform.reports.count')} value={formatNumber(totals.count)} icon="receipt" tone="primary" />
    <StatCard label={t('platform.reports.tenantsSelling')} value="{formatNumber(totals.selling)} / {formatNumber(sales.length)}" icon="store" tone="info" />
    <StatCard label={t('platform.reports.change')} value={g === null ? '—' : fmtPct(g)} icon={g !== null && g < 0 ? 'trending-down' : 'trending-up'} tone={g !== null && g < 0 ? 'danger' : 'success'} hint={rp(totals.prev)} />
  </div>

  {#if !loading && top.length > 0}
    <button type="button" onclick={() => { replay++; showPodium = true; }}
      class="group w-full flex items-center gap-3 rounded-xl px-4 py-3 text-start text-white shadow-sm hover:shadow-md transition-shadow"
      style="background:linear-gradient(110deg,#1e1b4b,#4c1d95 55%,#b45309)">
      <span class="text-[26px] leading-none transition-transform group-hover:scale-110 group-hover:-rotate-6" aria-hidden="true">🏆</span>
      <span class="min-w-0 flex-1">
        <span class="block font-display font-bold text-[14px]">{t('platform.reports.top10')}</span>
        <span class="block text-[12px] text-white/75 truncate">👑 {top[0].name} · {rp(Number(top[0].revenue))}</span>
      </span>
      <span class="shrink-0 text-[12px] font-semibold rounded-full bg-white/15 px-3 py-1 group-hover:bg-white/25">{t('platform.reports.podiumOpen')} →</span>
    </button>
  {/if}

  {#if showPodium}
    <Modal title={t('platform.reports.top10')} onclose={() => (showPodium = false)} wide>
      {#key replay}
        <div class="-mx-5 -mt-1 overflow-hidden relative" style="background:radial-gradient(ellipse at 50% 0%,#4c1d95 0%,#1e1b4b 55%,#0f0a2e 100%)">
          <div class="spot" aria-hidden="true"></div>
          {#each CONFETTI as c, i (i)}<i class="confetti" style="left:{c.x}%;background:{c.color};animation-delay:{1.3 + c.d}s;animation-duration:{c.dur}s" aria-hidden="true"></i>{/each}
          <p class="relative text-center text-[12px] text-white/70 pt-3 pb-1">{t('platform.reports.top10Sub')}</p>

          <div class="relative grid items-end gap-2 sm:gap-5 max-w-xl mx-auto px-4 pt-8" style="grid-template-columns:repeat({podium.length},minmax(0,1fr))">
            {#each podium as { r, rank } (r.tenant_id)}
              {@const m = MEDAL[rank]}
              <a href="/platform/tenants/{r.tenant_id}" class="flex flex-col items-center min-w-0 text-center no-underline text-white">
                <div class="flex flex-col items-center info" style="animation-delay:{rank === 1 ? 1.1 : rank === 2 ? 0.85 : 0.6}s">
                  {#if rank === 1}<span class="crown text-[34px] leading-none -mb-1" aria-hidden="true">👑</span>{/if}
                  <div class="rounded-full grid place-items-center font-display font-bold text-white {rank === 1 ? 'size-20 text-[30px]' : 'size-14 text-[22px]'}"
                    style="background:{m.grad};box-shadow:0 0 0 3px #1e1b4b,0 0 0 5px {m.ring}{rank === 1 ? ',0 0 34px 6px ' + m.ring + '88' : ''}">
                    {r.name.slice(0, 1).toUpperCase()}
                  </div>
                  <p class="mt-2 font-semibold text-[13px] truncate max-w-full">{r.name}</p>
                  <p class="text-[12.5px] tabular-nums font-bold" style="color:{m.ring}">{rp(Number(r.revenue))}</p>
                  <p class="text-[10.5px] text-white/60 mb-2">{formatNumber(r.sales_count)} {t('platform.reports.notes').toLowerCase()}</p>
                </div>
                <div class="block-rise w-full rounded-t-xl relative overflow-hidden grid justify-items-center pt-2" style="height:{m.px}px;background:{m.grad};animation-delay:{rank === 1 ? 0.5 : rank === 2 ? 0.25 : 0}s">
                  {#if rank === 1}<span class="shine" aria-hidden="true"></span>{/if}
                  <span class="font-display font-extrabold text-[34px] leading-none text-white/90 drop-shadow">{rank}</span>
                </div>
              </a>
            {/each}
          </div>
          <div class="h-1.5 relative" style="background:linear-gradient(90deg,#c9772b,#f5b301,#9aa5b1)"></div>
        </div>

        {#if top.length > 3}
          <ol class="mt-3 divide-y divide-[var(--border-subtle)]" start="4">
            {#each top.slice(3) as r, i (r.tenant_id)}
              <li class="row flex items-center gap-3 py-2" style="animation-delay:{1.4 + i * 0.08}s">
                <span class="w-7 h-7 shrink-0 rounded-full grid place-items-center text-[12px] font-bold bg-[var(--surface-sunken)] text-[var(--text-secondary)]">{i + 4}</span>
                <a href="/platform/tenants/{r.tenant_id}" class="min-w-0 flex-1 hover:underline">
                  <span class="block font-semibold text-[12.5px] truncate">{r.name}</span>
                  <span class="block h-1.5 mt-1 rounded-full bg-[var(--surface-sunken)] overflow-hidden"><span class="bar block h-full rounded-full bg-[var(--color-primary-500)]" style="width:{Math.max(2, (Number(r.revenue) / topMax) * 100)}%;animation-delay:{1.5 + i * 0.08}s"></span></span>
                </a>
                <span class="text-[12.5px] tabular-nums font-semibold shrink-0">{rp(Number(r.revenue))}</span>
              </li>
            {/each}
          </ol>
        {/if}
      {/key}
      <div class="flex justify-end mt-3">
        <button type="button" class="btn btn-secondary btn-sm" onclick={() => replay++}><i class="icon-rotate-ccw text-[12px]"></i>{t('platform.reports.podiumReplay')}</button>
      </div>
    </Modal>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[980px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="{th} text-start" scope="col">{t('platform.reports.tenant')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.plan')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.outlets')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.notes')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.revenue')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.avg')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.growth')}</th>
            <th class="{th} text-end" scope="col">{t('platform.reports.activeDays')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.lastSale')}</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            <tr><td colspan="9" class="px-4 py-3"><div class="h-7 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td></tr>
          {:else if sales.length === 0}
            <tr><td colspan="9" class="px-4 py-10 text-center text-[var(--text-tertiary)]">{t('platform.reports.empty')}</td></tr>
          {:else}
            {#each sales as r (r.tenant_id)}
              {@const st = status(r)}
              {@const gr = pct(Number(r.revenue), Number(r.prev_revenue))}
              <tr class="border-t border-[var(--border-subtle)] cursor-pointer hover:bg-[var(--surface-sunken)]" onclick={() => toggle(r.tenant_id)} aria-expanded={openId === r.tenant_id}>
                <td class="px-4 py-2.5">
                  <div class="font-semibold">{r.name}{#if !r.active} <span class="badge-neutral rounded-full px-1.5 py-0.5 text-[10.5px]">{t('platform.reports.inactive')}</span>{/if}</div>
                  <span class="badge-soft rounded-full px-1.5 py-0.5 text-[10.5px] {st.cls}">{st.label}</span>
                </td>
                <td class="px-4 py-2.5">{r.plan_name}</td>
                <td class="px-4 py-2.5 text-end tabular-nums">{r.outlet_count}</td>
                <td class="px-4 py-2.5 text-end tabular-nums">{formatNumber(r.sales_count)}</td>
                <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{rp(Number(r.revenue))}</td>
                <td class="px-4 py-2.5 text-end tabular-nums">{r.sales_count ? rp(Number(r.revenue) / r.sales_count) : '—'}</td>
                <td class="px-4 py-2.5 text-end tabular-nums font-semibold {gr === null ? '' : gr < 0 ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{gr === null ? '—' : fmtPct(gr)}</td>
                <td class="px-4 py-2.5 text-end tabular-nums">{r.active_days}</td>
                <td class="px-4 py-2.5 text-[var(--text-tertiary)]">{r.last_sale_at ? relativeTime(r.last_sale_at) : t('platform.reports.never')}</td>
              </tr>
              {#if openId === r.tenant_id}
                <tr class="bg-[var(--surface-sunken)]">
                  <td colspan="9" class="px-4 py-4">
                    <div class="flex items-center justify-between gap-3 mb-3">
                      <p class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.reports.trend')}</p>
                      <a href="/platform/tenants/{r.tenant_id}" class="text-[12px] text-[var(--color-primary-700)] hover:underline">{r.name} →</a>
                    </div>
                    {#if monthly}
                      <div class="grid grid-cols-6 gap-2 items-end h-32">
                        {#each monthly as m (m.month)}
                          <div class="flex flex-col items-center justify-end h-full gap-1 min-w-0">
                            <span class="text-[10.5px] tabular-nums text-[var(--text-tertiary)] truncate max-w-full">{rp(Number(m.revenue))}</span>
                            <div class="w-full max-w-10 rounded-t bg-[var(--color-primary-500)]" style="height:{Math.max(2, (Number(m.revenue) / maxMonth) * 80)}%"></div>
                            <span class="text-[11px]">{formatDate(m.month + '-01', { month: 'short', year: '2-digit' })}</span>
                            <span class="text-[10.5px] text-[var(--text-tertiary)]">{formatNumber(m.sales_count)}</span>
                          </div>
                        {/each}
                      </div>
                    {:else}
                      <div class="h-32 rounded-lg bg-[var(--surface-card)] animate-pulse"></div>
                    {/if}
                  </td>
                </tr>
              {/if}
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>

<style>
  .block-rise { transform-origin: bottom; animation: rise 0.8s cubic-bezier(0.2, 0.9, 0.3, 1.15) both; }
  .info { animation: drop 0.7s cubic-bezier(0.2, 0.9, 0.3, 1.3) both; }
  .row { animation: fadein 0.5s ease both; }
  .bar { transform-origin: left; animation: grow 0.8s ease-out both; }
  .crown { animation: pop 0.6s 1.5s cubic-bezier(0.3, 1.8, 0.5, 1) both, bob 2s 2.1s ease-in-out infinite; }
  .shine { position: absolute; inset: 0; background: linear-gradient(105deg, transparent 35%, rgba(255, 255, 255, 0.55) 50%, transparent 65%); transform: translateX(-120%); animation: sweep 2.4s 1.4s ease-in-out infinite; }
  .spot { position: absolute; left: 50%; top: 20%; width: 560px; height: 560px; margin-left: -280px; opacity: 0.35; background: repeating-conic-gradient(from 0deg, rgba(255, 226, 122, 0.55) 0 8deg, transparent 8deg 24deg); -webkit-mask-image: radial-gradient(circle, #000 0%, transparent 65%); mask-image: radial-gradient(circle, #000 0%, transparent 65%); animation: spin 28s linear infinite; }
  .confetti { position: absolute; top: -12px; width: 7px; height: 12px; border-radius: 1px; opacity: 0; animation: fall linear both; }
  @keyframes rise { from { transform: scaleY(0); } to { transform: scaleY(1); } }
  @keyframes drop { from { opacity: 0; transform: translateY(-40px) scale(0.8); } to { opacity: 1; transform: none; } }
  @keyframes fadein { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: none; } }
  @keyframes grow { from { transform: scaleX(0); } to { transform: scaleX(1); } }
  @keyframes pop { from { opacity: 0; transform: scale(0) rotate(-30deg); } to { opacity: 1; transform: none; } }
  @keyframes bob { 50% { transform: translateY(-4px) rotate(4deg); } }
  @keyframes sweep { 60%, 100% { transform: translateX(120%); } }
  @keyframes spin { to { transform: rotate(360deg); } }
  @keyframes fall { 0% { opacity: 1; transform: translateY(0) rotate(0); } 100% { opacity: 0; transform: translateY(420px) rotate(540deg); } }
  @media (prefers-reduced-motion: reduce) {
    .block-rise, .info, .row, .bar, .crown, .shine, .spot, .confetti { animation: none; }
    .confetti { display: none; }
  }
  .sw { display: inline-flex; align-items: center; gap: 0.5rem; font-size: 12.5px; cursor: pointer; user-select: none; }
  .sw i { position: relative; width: 2rem; height: 1.125rem; border-radius: 9999px; background: var(--border-default); transition: background 0.2s; }
  .sw i::after { content: ''; position: absolute; top: 0.125rem; inset-inline-start: 0.125rem; width: 0.875rem; height: 0.875rem; border-radius: 9999px; background: #fff; box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3); transition: transform 0.2s; }
  .sw :global(input:checked + i) { background: var(--color-primary-500); }
  .sw :global(input:checked + i)::after { transform: translateX(0.875rem); }
  :global([dir='rtl']) .sw :global(input:checked + i)::after { transform: translateX(-0.875rem); }
  .sw :global(input:focus-visible + i) { outline: 2px solid var(--color-primary-500); outline-offset: 2px; }
  .chip { display: inline-flex; align-items: center; gap: 0.375rem; height: 1.875rem; padding: 0 0.75rem; border-radius: 9999px; font-size: 12px; border: 1px solid var(--border-default); background: var(--surface-base); color: var(--text-secondary); transition: all 0.15s; }
  .chip:hover { border-color: var(--color-primary-400); }
  .chip.on { background: var(--color-primary-50); border-color: var(--color-primary-500); color: var(--color-primary-700); font-weight: 600; box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06); }
  .chip b { font-size: 11px; padding: 0 0.375rem; border-radius: 9999px; background: var(--surface-sunken); }
  .chip.on b { background: var(--surface-card); }
</style>
