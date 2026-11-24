<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type SalesReportRow, type SalesMonth, type ExpiringRow, type OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  // Uang platform selalu Rupiah (nilai transaksi tenant), tanpa desimal.
  const rp = (n: number) => 'Rp ' + formatNumber(Math.round(n), { maximumFractionDigits: 0 });
  const th = 'px-4 py-2.5 font-semibold';
  const DAY = 86_400_000;

  let tab = $state<'sales' | 'expiring' | 'locations'>('sales');
  let error = $state('');
  let loading = $state(true);

  // ---- Penjualan ----
  // Tanggal acuan WIB (operator di WIB), format YYYY-MM-DD.
  const ymd = (d: Date) => new Date(d.getTime() + 7 * 3600_000).toISOString().slice(0, 10);
  const today = () => ymd(new Date());
  let from = $state(ymd(new Date(Date.now() - 29 * DAY)));
  let to = $state(today());
  let sales = $state<SalesReportRow[]>([]);
  let openId = $state<string | null>(null);
  let monthly = $state<SalesMonth[] | null>(null);

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
      sales = (await platformApi.salesReport(from, to)).items;
      openId = null;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  function preset(days: number | 'month') {
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

  // ---- Paket berakhir ----
  let days = $state(30);
  let expiring = $state<ExpiringRow[]>([]);
  const left = (iso: string) => Math.ceil((new Date(iso).getTime() - Date.now()) / DAY);
  const groups = $derived([
    { key: 'expired', title: t('platform.reports.expired'), cls: 'badge-danger', rows: expiring.filter((r) => left(r.expires_at) <= 0) },
    { key: 'soon', title: t('platform.reports.soon'), cls: 'badge-warning', rows: expiring.filter((r) => left(r.expires_at) > 0 && left(r.expires_at) <= 7) },
    { key: 'later', title: t('platform.reports.later'), cls: 'badge-info', rows: expiring.filter((r) => left(r.expires_at) > 7) }
  ].filter((g) => g.rows.length));

  function leftText(iso: string) {
    const n = left(iso);
    return n > 0 ? t('platform.reports.daysLeft', { n }) : n === 0 ? t('platform.reports.today') : t('platform.reports.daysAgo', { n: -n });
  }

  async function loadExpiring() {
    loading = true;
    error = '';
    try {
      expiring = await platformApi.expiringReport(days);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  // ---- Lokasi ----
  let locs = $state<OutletLocation[]>([]);
  let onlyMissing = $state(false);
  const shownLocs = $derived(onlyMissing ? locs.filter((l) => l.latitude === null) : locs);
  const located = $derived(locs.filter((l) => l.latitude !== null).length);
  const mapUrl = (l: OutletLocation) => `https://www.openstreetmap.org/?mlat=${l.latitude}&mlon=${l.longitude}#map=17/${l.latitude}/${l.longitude}`;

  async function loadLocations() {
    loading = true;
    error = '';
    try {
      locs = await platformApi.outletLocations();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  function show(next: typeof tab) {
    tab = next;
    void (next === 'sales' ? loadSales() : next === 'expiring' ? loadExpiring() : loadLocations());
  }
  onMount(loadSales);
</script>

<div>
  <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.reports.title')}</h1>
  <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.reports.sub')}</p>
</div>

<div class="flex flex-wrap gap-1.5" role="tablist">
  {#each [['sales', 'tabSales', 'trending-up'], ['expiring', 'tabExpiring', 'alarm-clock'], ['locations', 'tabLocations', 'map-pin']] as [k, label, icon] (k)}
    <button type="button" role="tab" aria-selected={tab === k} class="btn btn-sm {tab === k ? 'btn-primary' : 'btn-secondary'}" onclick={() => show(k as typeof tab)}>
      <i class="icon-{icon} text-[14px]"></i>{t(`platform.reports.${label}` as 'platform.reports.tabSales')}
    </button>
  {/each}
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

{#if tab === 'sales'}
  <div class="surface-card p-3 sm:p-4 flex flex-wrap items-end gap-3">
    <label class="text-[12px] font-medium">{t('platform.reports.from')}<input type="date" class="input mt-1 block" bind:value={from} max={to} /></label>
    <label class="text-[12px] font-medium">{t('platform.reports.to')}<input type="date" class="input mt-1 block" bind:value={to} min={from} max={today()} /></label>
    <button type="button" class="btn btn-primary btn-sm" onclick={loadSales} disabled={loading}>{t('platform.reports.tabSales')}</button>
    <div class="flex flex-wrap gap-1.5 ms-auto">
      <button type="button" class="btn btn-secondary btn-sm" onclick={() => preset(7)}>{t('platform.reports.d7')}</button>
      <button type="button" class="btn btn-secondary btn-sm" onclick={() => preset(30)}>{t('platform.reports.d30')}</button>
      <button type="button" class="btn btn-secondary btn-sm" onclick={() => preset(90)}>{t('platform.reports.d90')}</button>
      <button type="button" class="btn btn-secondary btn-sm" onclick={() => preset('month')}>{t('platform.reports.thisMonth')}</button>
    </div>
  </div>
  <p class="text-[11.5px] -mt-2 text-[var(--text-tertiary)]">{t('platform.reports.compare')}</p>

  {@const g = pct(totals.rev, totals.prev)}
  <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
    <StatCard label={t('platform.reports.revenue')} value={rp(totals.rev)} icon="banknote" tone="success" />
    <StatCard label={t('platform.reports.count')} value={formatNumber(totals.count)} icon="receipt" tone="primary" />
    <StatCard label={t('platform.reports.tenantsSelling')} value="{formatNumber(totals.selling)} / {formatNumber(sales.length)}" icon="store" tone="info" />
    <StatCard label={t('platform.reports.change')} value={g === null ? '—' : fmtPct(g)} icon={g !== null && g < 0 ? 'trending-down' : 'trending-up'} tone={g !== null && g < 0 ? 'danger' : 'success'} hint={rp(totals.prev)} />
  </div>

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
{:else if tab === 'expiring'}
  <div class="surface-card p-3 sm:p-4 flex flex-wrap items-center gap-2">
    <span class="text-[12px] font-medium me-1">{t('platform.reports.window')}</span>
    {#each [7, 14, 30, 60, 90] as n (n)}
      <button type="button" class="btn btn-sm {days === n ? 'btn-primary' : 'btn-secondary'}" onclick={() => { days = n; void loadExpiring(); }}>{t('platform.reports.days', { n })}</button>
    {/each}
  </div>
  <p class="text-[11.5px] -mt-2 text-[var(--text-tertiary)]">{t('platform.reports.expHint')}</p>

  {#if loading}
    <div class="h-24 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>
  {:else if expiring.length === 0}
    <div class="surface-card p-10 text-center text-[var(--text-tertiary)]">{t('platform.reports.expEmpty')}</div>
  {:else}
    {#each groups as grp (grp.key)}
      <div class="surface-card !p-0 overflow-hidden">
        <div class="p-3 sm:p-4 border-b border-[var(--border-subtle)] flex items-center gap-2 font-semibold text-[13px]">
          <span class="rounded-full px-2 py-0.5 text-[11.5px] {grp.cls}">{grp.rows.length}</span>{grp.title}
        </div>
        <div class="overflow-x-auto scroll-thin">
          <table class="w-full text-[12.5px] min-w-[900px]">
            <thead>
              <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
                <th class="{th} text-start" scope="col">{t('platform.reports.tenant')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.plan')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.expiresOn')}</th>
                <th class="{th} text-end" scope="col">{t('platform.reports.rev30')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.lastLogin')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.owner')}</th>
              </tr>
            </thead>
            <tbody>
              {#each grp.rows as r (r.tenant_id)}
                <tr class="border-t border-[var(--border-subtle)]">
                  <td class="px-4 py-2.5">
                    <a href="/platform/tenants/{r.tenant_id}" class="font-semibold text-[var(--color-primary-700)] hover:underline">{r.name}</a>
                    <div class="text-[11.5px] text-[var(--text-tertiary)]">{r.outlet_count} {t('platform.reports.outlets').toLowerCase()} · {r.user_count} {t('platform.reports.users').toLowerCase()}{#if !r.active} · {t('platform.reports.inactive')}{/if}</div>
                  </td>
                  <td class="px-4 py-2.5">
                    <span class="font-semibold">{r.plan_name}</span>
                    <div class="text-[11.5px] text-[var(--text-tertiary)]">{Number(r.plan_price) > 0 ? rp(Number(r.plan_price)) : '—'}{#if r.note} · {r.note}{/if}</div>
                  </td>
                  <td class="px-4 py-2.5">
                    <div>{formatDateTime(r.expires_at)}</div>
                    <span class="rounded-full px-1.5 py-0.5 text-[10.5px] {grp.cls}">{leftText(r.expires_at)}</span>
                  </td>
                  <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{rp(Number(r.revenue_30d))}</td>
                  <td class="px-4 py-2.5 text-[var(--text-tertiary)]">{r.last_login_at ? relativeTime(r.last_login_at) : t('platform.reports.never')}</td>
                  <td class="px-4 py-2.5">{#if r.owner_email}<a href="mailto:{r.owner_email}" class="hover:underline">{r.owner_email}</a>{:else}—{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/each}
  {/if}
{:else}
  <div class="grid grid-cols-2 gap-3 max-w-xl">
    <StatCard label={t('platform.reports.withLoc')} value="{formatNumber(located)} / {formatNumber(locs.length)}" icon="map-pin" tone="success" />
    <StatCard label={t('platform.reports.withoutLoc')} value={formatNumber(locs.length - located)} icon="circle-alert" tone="warning" />
  </div>
  <label class="flex items-center gap-2 text-[12.5px] font-medium cursor-pointer w-fit">
    <input type="checkbox" class="size-4 rounded accent-[var(--color-primary-600)]" bind:checked={onlyMissing} />{t('platform.reports.onlyMissing')}
  </label>
  <p class="text-[11.5px] -mt-2 text-[var(--text-tertiary)]">{t('platform.reports.locHint')}</p>
  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[720px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="{th} text-start" scope="col">{t('platform.reports.tenant')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.outlet')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.address')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.coords')}</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            <tr><td colspan="4" class="px-4 py-3"><div class="h-7 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td></tr>
          {:else if shownLocs.length === 0}
            <tr><td colspan="4" class="px-4 py-10 text-center text-[var(--text-tertiary)]">{t('platform.reports.empty')}</td></tr>
          {:else}
            {#each shownLocs as l (l.outlet_id)}
              <tr class="border-t border-[var(--border-subtle)]">
                <td class="px-4 py-2.5 font-semibold"><a href="/platform/tenants/{l.tenant_id}" class="text-[var(--color-primary-700)] hover:underline">{l.tenant_name}</a></td>
                <td class="px-4 py-2.5">{l.outlet_name}{#if !l.active} <span class="badge-neutral rounded-full px-1.5 py-0.5 text-[10.5px]">{t('platform.reports.inactive')}</span>{/if}</td>
                <td class="px-4 py-2.5 text-[var(--text-secondary)]">{l.address || '—'}</td>
                <td class="px-4 py-2.5">
                  {#if l.latitude !== null && l.longitude !== null}
                    <a href={mapUrl(l)} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 font-mono text-[12px] text-[var(--color-primary-700)] hover:underline"><i class="icon-map-pin text-[13px]"></i>{l.latitude.toFixed(5)}, {l.longitude.toFixed(5)}</a>
                  {:else}
                    <span class="text-[var(--text-tertiary)]">{t('platform.reports.noLoc')}</span>
                  {/if}
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>
{/if}
