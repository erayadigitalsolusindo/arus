<script lang="ts">
  import { t, tryT, formatNumber, formatCurrency, formatDate, formatDateTime, type MessageKey } from '#lib/i18n/index.ts';
  import { session, can } from '#lib/auth/session.svelte.ts';
  import { outletScope, supportAllOutlets } from '#lib/outlets/store.svelte.ts';
  import { dashboard, type Overview, type Level, type Check } from '#lib/dashboard/api.ts';
  import { ApiError } from '#lib/api/client.ts';
  import BarChart from '#lib/components/BarChart.svelte';

  supportAllOutlets();

  let data = $state<Overview | null>(null);
  let failed = $state(false);
  let loading = $state(true);
  let seq = 0;
  let range = $state<7 | 14 | 28>(14);

  async function load(quiet = false) {
    const mine = ++seq;
    if (!quiet) loading = true;
    try {
      const res = await dashboard.overview(outletScope.all);
      if (mine !== seq) return;
      data = res;
      failed = false;
    } catch (e) {
      if (mine !== seq) return;
      if (!quiet || !data) failed = true;
      if (e instanceof ApiError && e.status === 401) failed = true;
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Muat ulang saat cabang aktif / mode semua cabang berubah; segarkan diam-diam tiap menit selama halaman terbuka.
  $effect(() => {
    void outletScope.all;
    void session.outlet?.id;
    void load();
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') void load(true);
    }, 60_000);
    return () => {
      clearInterval(timer);
      seq++;
    };
  });

  // ---- format ----
  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 0, maximumFractionDigits: 0 });
  const compact = (v: number) => formatNumber(v, { notation: 'compact', maximumFractionDigits: 1 });
  const pct = (p: number) => formatNumber(p / 100, { style: 'percent', maximumFractionDigits: 0, signDisplay: 'exceptZero' });
  const localDate = (s: string) => {
    const [y, m, d] = s.split('-').map(Number);
    return new Date(y, m - 1, d);
  };

  // ---- ringkasan kesehatan ----
  const attention = $derived(data ? data.checks.filter((c) => c.level !== 'ok').length : 0);
  const hero = {
    ok: { icon: 'shield-check', bg: 'linear-gradient(135deg,var(--color-success-600),var(--color-success-900))' },
    warn: { icon: 'triangle-alert', bg: 'linear-gradient(135deg,var(--color-warning-600),var(--color-warning-900))' },
    bad: { icon: 'octagon-alert', bg: 'linear-gradient(135deg,var(--color-danger-600),var(--color-danger-900))' }
  } satisfies Record<Level, { icon: string; bg: string }>;
  const tone = { ok: 'success', warn: 'warning', bad: 'danger' } satisfies Record<Level, string>;
  const toneIcon = { ok: 'circle-check', warn: 'circle-alert', bad: 'circle-x' } satisfies Record<Level, string>;

  const checkLinks: Record<string, string> = {
    sales_pace: '/sales',
    voids: '/sales',
    shift_open_long: '/shifts',
    shift_diff: '/shifts',
    stock_negative: '/items',
    stock_low: '/items',
    receivable_overdue: '/receivables',
    payable_overdue: '/supplier-payables'
  };

  function checkText(c: Check): string {
    const p: Record<string, string | number> = {};
    const src = c.params ?? {};
    if (typeof src.pct === 'number') p.pct = pct(src.pct);
    if (typeof src.count === 'number') p.count = formatNumber(src.count);
    if (typeof src.amount === 'string') p.amount = money(src.amount);
    return tryT(`dashboard.check.${c.code}.${c.level}`, p) ?? tryT(`dashboard.check.${c.code}.warn`, p) ?? c.code;
  }

  // ---- penjualan ----
  const s = $derived(data?.sales);
  const trendBars = $derived.by(() => {
    if (!s) return [];
    const rows = s.trend.slice(-range);
    const step = range === 28 ? 4 : range === 14 ? 2 : 1;
    return rows.map((d, i) => {
      const dt = localDate(d.date);
      return {
        key: d.date,
        label: String(dt.getDate()),
        tick: (rows.length - 1 - i) % step === 0,
        value: Number(d.total),
        tip: `${formatDate(dt, { weekday: 'long', day: 'numeric', month: 'short' })} · ${money(d.total)} · ${t('dashboard.chart.receiptsCount', { count: d.count })}`
      };
    });
  });
  const trendEmpty = $derived(trendBars.every((b) => b.value === 0));
  const trendSum = $derived(trendBars.reduce((a, b) => a + b.value, 0));

  const hourBars = $derived.by(() => {
    if (!s) return [];
    const active = s.hourly.filter((h) => Number(h.total) > 0).map((h) => h.hour);
    const from = Math.min(7, ...(active.length ? active : [7]));
    const to = Math.max(21, ...(active.length ? active : [21]));
    return s.hourly
      .filter((h) => h.hour >= from && h.hour <= to)
      .map((h) => ({
        key: String(h.hour),
        label: String(h.hour).padStart(2, '0'),
        tick: h.hour % 3 === 0,
        value: Number(h.total),
        tip: `${String(h.hour).padStart(2, '0')}.00 · ${money(h.total)} · ${t('dashboard.chart.receiptsCount', { count: h.count })}`
      }));
  });
  const hourEmpty = $derived(hourBars.every((b) => b.value === 0));
  const nowHour = $derived(String(new Date().getHours()));

  const vs = $derived(s?.compare.pct);
  const topMax = $derived(Math.max(1, ...(s?.top_items ?? []).map((i) => Number(i.revenue))));

  const scopeLabel = $derived(
    data?.scope === 'all' ? t('dashboard.scope.all', { count: data.outlets }) : t('dashboard.scope.outlet', { name: session.outlet?.name ?? '' })
  );
  const updated = $derived(data ? formatDateTime(data.generated_at, { timeStyle: 'short' }) : '');
  const hasAnything = $derived(!!(data && (data.sales || data.stock || data.receivable || data.payable || data.shifts)));
  const today = $derived(formatDate(new Date(), { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }));
</script>

<svelte:head><title>{t('dashboard.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('dashboard.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('dashboard.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-5 max-w-[1600px] mx-auto w-full">
  {#if failed && !data}
    <div class="surface-card p-8 text-center space-y-3">
      <span class="grid place-items-center size-12 mx-auto rounded-full badge-solid-danger"><i class="icon-circle-alert text-[22px]"></i></span>
      <p class="text-[13px] u-color-text-secondary">{t('dashboard.loadError')}</p>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => load()}><i class="icon-refresh-cw text-[13px]"></i>{t('dashboard.retry')}</button>
    </div>
  {:else if !data}
    <div class="space-y-5 animate-pulse" aria-busy="true">
      <div class="h-40 rounded-2xl bg-[var(--surface-sunken)]"></div>
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {#each [0, 1, 2, 3] as i (i)}<div class="h-32 rounded-xl bg-[var(--surface-sunken)]"></div>{/each}
      </div>
      <div class="h-64 rounded-xl bg-[var(--surface-sunken)]"></div>
    </div>
  {:else if !hasAnything}
    <div class="surface-card p-8 text-center text-[13px] u-color-text-secondary">{t('dashboard.noAccess')}</div>
  {:else}
    <!-- Kesimpulan: apakah toko baik-baik saja -->
    <section
      class="relative overflow-hidden rounded-2xl p-5 lg:p-7 text-white animate-rise-in shadow-[var(--shadow-lg)]"
      style="background:{hero[data.verdict].bg}"
      aria-live="polite"
    >
      <span class="absolute -top-16 -end-10 size-56 rounded-full pointer-events-none bg-[rgb(255_255_255_/_0.08)]"></span>
      <span class="absolute -bottom-20 -start-10 size-48 rounded-full pointer-events-none bg-[rgb(255_255_255_/_0.06)]"></span>
      <div class="relative flex flex-wrap items-start justify-between gap-4">
        <div class="flex items-start gap-4 min-w-0">
          <span class="grid place-items-center size-14 shrink-0 rounded-2xl bg-[rgb(255_255_255_/_0.16)] border border-[rgb(255_255_255_/_0.2)]">
            <i class="icon-{hero[data.verdict].icon} text-[28px]"></i>
          </span>
          <div class="min-w-0">
            <h1 class="font-display font-extrabold text-[22px] lg:text-[28px] leading-tight text-[#fff]">{t(`dashboard.verdict.${data.verdict}.title` as MessageKey)}</h1>
            <p class="text-[13px] mt-1.5 max-w-xl text-[rgb(255_255_255_/_0.85)]">{t(`dashboard.verdict.${data.verdict}.text` as MessageKey, { count: attention })}</p>
          </div>
        </div>
        <div class="flex flex-col items-start sm:items-end gap-2 shrink-0 text-[11.5px] text-[rgb(255_255_255_/_0.8)]">
          <button type="button" class="btn !text-[12.5px] !border-0 bg-[#fff] text-[#1e293b]" disabled={loading} onclick={() => load()}>
            <i class="icon-refresh-cw text-[13px] {loading ? 'animate-spin' : ''}"></i>{t('dashboard.refresh')}
          </button>
          <span class="inline-flex items-center gap-1.5"><i class="icon-store text-[12px]"></i>{scopeLabel}</span>
          <span>{today} · {t('dashboard.updatedAt', { time: updated })}</span>
        </div>
      </div>
    </section>

    <!-- Daftar pemeriksaan -->
    {#if data.checks.length}
      <section class="surface-card p-4 lg:p-5 animate-rise-in">
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 mb-3">
          <h2 class="font-display font-bold text-[14px]">{t('dashboard.checksTitle')}</h2>
          <p class="text-[11.5px] u-color-text-tertiary">{t('dashboard.checksSub')}</p>
        </div>
        <ul class="grid grid-cols-1 lg:grid-cols-2 gap-x-6 gap-y-1">
          {#each data.checks as c (c.code)}
            <li class="flex items-start gap-2.5 py-2 border-b border-[var(--border-subtle)] last:border-0 lg:[&:nth-last-child(2):nth-child(odd)]:border-0">
              <span class="grid place-items-center size-6 mt-0.5 rounded-full shrink-0 badge-solid-{tone[c.level]}"><i class="icon-{toneIcon[c.level]} text-[13px]"></i></span>
              <p class="flex-1 min-w-0 text-[12.5px] leading-snug {c.level === 'ok' ? 'u-color-text-secondary' : 'font-medium'}">{checkText(c)}</p>
              {#if c.level !== 'ok' && checkLinks[c.code]}
                <a href={checkLinks[c.code]} class="shrink-0 inline-flex items-center gap-0.5 text-[11.5px] font-semibold text-[var(--color-primary-600)]">
                  {t('dashboard.open')}<i class="icon-chevron-right text-[11px]"></i>
                </a>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if s}
      <!-- Angka utama -->
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <article class="surface-card p-4 animate-rise-in">
          <div class="flex items-center gap-2">
            <span class="grid place-items-center size-9 rounded-lg badge-solid-primary shrink-0"><i class="icon-banknote text-[16px]"></i></span>
            <h2 class="text-[12px] u-color-text-tertiary">{t('dashboard.kpi.salesToday')}</h2>
          </div>
          <p class="font-display font-extrabold text-[26px] leading-none mt-3">{money(s.today.total)}</p>
          <p class="mt-2 text-[11.5px] flex flex-wrap items-center gap-x-2 gap-y-1">
            {#if vs != null}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md font-semibold {vs >= 0 ? 'badge badge-success' : vs <= -40 ? 'badge badge-danger' : 'badge badge-warning'}"
                title={t('dashboard.kpi.vsUsualHint')}
              >
                <i class="icon-{vs >= 0 ? 'trending-up' : 'trending-down'} text-[11px]"></i>{t('dashboard.kpi.vsUsual', { pct: pct(vs) })}
              </span>
            {:else}
              <span class="u-color-text-tertiary" title={t('dashboard.kpi.vsUsualHint')}>{t('dashboard.kpi.vsUsualNone')}</span>
            {/if}
          </p>
          <p class="mt-1.5 text-[11px] u-color-text-tertiary">{t('dashboard.kpi.yesterday', { amount: money(s.yesterday.total) })}</p>
        </article>

        <article class="surface-card p-4 animate-rise-in">
          <div class="flex items-center gap-2">
            <span class="grid place-items-center size-9 rounded-lg badge-solid-info shrink-0"><i class="icon-receipt text-[16px]"></i></span>
            <h2 class="text-[12px] u-color-text-tertiary">{t('dashboard.kpi.receipts')}</h2>
          </div>
          <p class="font-display font-extrabold text-[26px] leading-none mt-3">{formatNumber(s.today.count)}</p>
          <p class="mt-2 text-[11.5px] u-color-text-secondary">{t('dashboard.kpi.average', { amount: money(s.today.average) })}</p>
          <p class="mt-1.5 text-[11px] u-color-text-tertiary">
            {t('dashboard.kpi.cancelled', { count: s.today.voids })}
            {#if s.today.returns_count > 0}· {t('dashboard.kpi.returns', { count: s.today.returns_count, amount: money(s.today.returns_total) })}{/if}
          </p>
        </article>

        {#if s.today.profit != null}
          <article class="surface-card p-4 animate-rise-in">
            <div class="flex items-center gap-2">
              <span class="grid place-items-center size-9 rounded-lg badge-solid-success shrink-0"><i class="icon-hand-coins text-[16px]"></i></span>
              <h2 class="text-[12px] u-color-text-tertiary">{t('dashboard.kpi.profit')}</h2>
            </div>
            <p class="font-display font-extrabold text-[26px] leading-none mt-3 {Number(s.today.profit) < 0 ? 'text-[var(--color-danger-600)]' : ''}">{money(s.today.profit)}</p>
            <p class="mt-2 text-[11px] u-color-text-tertiary">{t('dashboard.kpi.profitHint')}</p>
          </article>
        {/if}

        <article class="surface-card p-4 animate-rise-in">
          <div class="flex items-center gap-2">
            <span class="grid place-items-center size-9 rounded-lg badge-solid-accent shrink-0"><i class="icon-layers text-[16px]"></i></span>
            <h2 class="text-[12px] u-color-text-tertiary">{t('dashboard.kpi.week')}</h2>
          </div>
          <p class="font-display font-extrabold text-[26px] leading-none mt-3">{money(s.week_total)}</p>
          <p class="mt-2 text-[11.5px] u-color-text-secondary flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>{t('dashboard.kpi.month')}: <span class="font-semibold">{money(s.month_total)}</span></span>
            {#if s.month_compare.pct != null}
              <span
                class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-md font-semibold {s.month_compare.pct >= 0 ? 'badge badge-success' : 'badge badge-warning'}"
                title={t('dashboard.kpi.monthVsHint', { amount: money(s.month_compare.last) })}
              >
                <i class="icon-{s.month_compare.pct >= 0 ? 'trending-up' : 'trending-down'} text-[11px]"></i>{t('dashboard.kpi.monthVs', { pct: pct(s.month_compare.pct) })}
              </span>
            {:else}
              <span class="u-color-text-tertiary">{t('dashboard.kpi.monthVsNone')}</span>
            {/if}
          </p>
        </article>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-3 gap-4">
        <!-- Tren harian -->
        <section class="surface-card p-4 lg:p-5 xl:col-span-2 animate-rise-in">
          <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
            <div>
              <h2 class="font-display font-bold text-[14px]">{t('dashboard.chart.trend')}</h2>
              <p class="text-[11.5px] u-color-text-tertiary">{money(trendSum)}</p>
            </div>
            <div class="inline-flex rounded-lg p-0.5 bg-[var(--surface-sunken)]" role="group">
              {#each [7, 14, 28] as n (n)}
                <button
                  type="button"
                  class="px-2.5 py-1 rounded-md text-[11.5px] font-semibold transition-colors {range === n ? 'bg-[var(--surface-card)] shadow-sm text-[var(--color-primary-700)]' : 'u-color-text-tertiary'}"
                  aria-pressed={range === n}
                  onclick={() => (range = n as 7 | 14 | 28)}>{t('dashboard.chart.days', { count: n })}</button>
              {/each}
            </div>
          </div>
          {#if trendEmpty}
            <p class="py-12 text-center text-[12.5px] u-color-text-tertiary">{t('dashboard.chart.noSales')}</p>
          {:else}
            <BarChart bars={trendBars} highlight={s.trend[s.trend.length - 1].date} axis={compact} />
          {/if}
        </section>

        <!-- Barang terlaris -->
        <section class="surface-card p-4 lg:p-5 animate-rise-in">
          <h2 class="font-display font-bold text-[14px]">{t('dashboard.top.title')}</h2>
          <p class="text-[11.5px] u-color-text-tertiary mb-3">{t('dashboard.top.sub')}</p>
          {#if s.top_items.length === 0}
            <p class="py-8 text-center text-[12.5px] u-color-text-tertiary">{t('dashboard.top.empty')}</p>
          {:else}
            <ol class="space-y-3">
              {#each s.top_items as it, i (it.name + i)}
                <li>
                  <div class="flex items-baseline justify-between gap-2 text-[12.5px]">
                    <span class="font-medium truncate"><span class="u-color-text-tertiary me-1.5">{i + 1}</span>{it.name}</span>
                    <span class="font-semibold shrink-0">{money(it.revenue)}</span>
                  </div>
                  <div class="mt-1 h-1.5 rounded-full bg-[var(--surface-sunken)] overflow-hidden">
                    <div class="h-full rounded-full bg-[var(--color-primary-500)]" style="width:{(Number(it.revenue) / topMax) * 100}%"></div>
                  </div>
                  <p class="mt-0.5 text-[10.5px] u-color-text-tertiary">{t('dashboard.top.qty', { qty: formatNumber(Number(it.qty), { maximumFractionDigits: 2 }) })}</p>
                </li>
              {/each}
            </ol>
          {/if}
        </section>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-3 gap-4">
        <!-- Per jam -->
        <section class="surface-card p-4 lg:p-5 xl:col-span-2 animate-rise-in">
          <div class="flex flex-wrap items-baseline justify-between gap-2 mb-3">
            <h2 class="font-display font-bold text-[14px]">{t('dashboard.chart.hourly')}</h2>
            {#if s.today.peak_hour != null}
              <p class="text-[11.5px] u-color-text-tertiary">{t('dashboard.chart.peak', { hour: `${String(s.today.peak_hour).padStart(2, '0')}.00` })}</p>
            {/if}
          </div>
          {#if hourEmpty}
            <p class="py-10 text-center text-[12.5px] u-color-text-tertiary">{t('dashboard.chart.noSales')}</p>
          {:else}
            <BarChart bars={hourBars} highlight={nowHour} height={120} axis={compact} />
          {/if}
        </section>

        <!-- Kas & shift -->
        {#if data.shifts}
          {@const sh = data.shifts}
          <section class="surface-card p-4 lg:p-5 animate-rise-in">
            <div class="flex items-center justify-between gap-2 mb-3">
              <h2 class="font-display font-bold text-[14px]">{t('dashboard.shifts.title')}</h2>
              {#if can('cash_shifts', 'view')}<a href="/shifts" class="text-[11.5px] font-semibold text-[var(--color-primary-600)]">{t('dashboard.shifts.all')}</a>{/if}
            </div>
            <p class="text-[11.5px] font-semibold u-color-text-tertiary mb-1.5">{t('dashboard.shifts.running')}</p>
            {#if sh.open.length === 0}
              <p class="text-[12.5px] u-color-text-secondary">{t('dashboard.shifts.none')}</p>
            {:else}
              <ul class="space-y-1.5">
                {#each sh.open as o, i (i)}
                  <li class="flex items-center gap-2 text-[12.5px]">
                    <span class="size-2 rounded-full shrink-0 {o.long ? 'bg-[var(--color-warning-500)]' : 'bg-[var(--color-success-500)]'}"></span>
                    <span class="font-medium truncate">{o.user}</span>
                    <span class="u-color-text-tertiary truncate">{t('dashboard.shifts.since', { time: formatDateTime(o.opened_at, { dateStyle: 'short', timeStyle: 'short' }) })}</span>
                    {#if o.long}<span class="badge badge-warning px-1.5 rounded text-[10.5px] font-semibold shrink-0">{t('dashboard.shifts.long')}</span>{/if}
                  </li>
                {/each}
              </ul>
            {/if}
            <div class="mt-3 pt-3 border-t border-[var(--border-subtle)] text-[12.5px]">
              <p class="text-[11.5px] font-semibold u-color-text-tertiary mb-1">{t('dashboard.shifts.diff')}</p>
              {#if sh.diff_count === 0}
                <p class="inline-flex items-center gap-1.5 text-[var(--color-success-700)] font-medium"><i class="icon-circle-check text-[14px]"></i>{t('dashboard.shifts.noDiff')}</p>
              {:else}
                <p class="font-semibold text-[var(--color-warning-700)]">{money(sh.diff_abs)} · {t('dashboard.shifts.closed', { count: sh.diff_count })}</p>
              {/if}
            </div>
          </section>
        {/if}
      </div>
    {/if}

    <!-- Perbandingan cabang (mode semua cabang) -->
    {#if data.by_outlet && data.by_outlet.length > 0}
      {@const topToday = Math.max(1, ...data.by_outlet.map((x) => Number(x.today_total)))}
      <section class="surface-card p-4 lg:p-5 animate-rise-in">
        <h2 class="font-display font-bold text-[14px]">{t('dashboard.branches.title')}</h2>
        <p class="text-[11.5px] u-color-text-tertiary mb-3">{t('dashboard.branches.sub')}</p>
        <div class="overflow-x-auto -mx-1 px-1">
          <table class="w-full text-[12.5px] min-w-[620px]">
            <thead>
              <tr class="text-[11px] u-color-text-tertiary">
                <th class="text-start font-semibold py-1.5 pe-3">{t('dashboard.branches.outlet')}</th>
                <th class="text-end font-semibold py-1.5 px-3">{t('dashboard.branches.today')}</th>
                <th class="text-end font-semibold py-1.5 px-3">{t('dashboard.branches.receipts')}</th>
                <th class="text-end font-semibold py-1.5 px-3">{t('dashboard.branches.yesterday')}</th>
                <th class="text-end font-semibold py-1.5 px-3">{t('dashboard.branches.week')}</th>
                {#if data.stock}<th class="text-start font-semibold py-1.5 px-3">{t('dashboard.branches.stock')}</th>{/if}
                {#if data.shifts}<th class="text-end font-semibold py-1.5 ps-3">{t('dashboard.branches.shifts')}</th>{/if}
              </tr>
            </thead>
            <tbody>
              {#each data.by_outlet as o (o.id)}
                <tr class="border-t border-[var(--border-subtle)]">
                  <td class="py-2 pe-3 font-medium">{o.name}<span class="u-color-text-tertiary"> · {o.code}</span></td>
                  <td class="py-2 px-3 text-end">
                    <span class="font-semibold">{money(o.today_total)}</span>
                    <span class="block h-1 mt-1 rounded-full bg-[var(--surface-sunken)] overflow-hidden"><span class="block h-full rounded-full bg-[var(--color-primary-500)]" style="width:{(Number(o.today_total) / topToday) * 100}%"></span></span>
                  </td>
                  <td class="py-2 px-3 text-end">{formatNumber(o.today_count)}{#if o.voids > 0}<span class="text-[var(--color-danger-600)]" title={t('dashboard.kpi.cancelled', { count: o.voids })}> ·{o.voids}</span>{/if}</td>
                  <td class="py-2 px-3 text-end u-color-text-secondary">{money(o.yesterday_total)}</td>
                  <td class="py-2 px-3 text-end u-color-text-secondary">{money(o.week_total)}</td>
                  {#if data.stock}
                    <td class="py-2 px-3">
                      {#if (o.stock_negative ?? 0) + (o.stock_low ?? 0) + (o.stock_empty ?? 0) === 0}
                        <span class="inline-flex items-center gap-1 text-[var(--color-success-700)]"><i class="icon-circle-check text-[13px]"></i>{t('dashboard.branches.stockOk')}</span>
                      {:else}
                        <span class="flex flex-wrap gap-1">
                          {#if o.stock_negative}<span class="badge badge-danger">{t('dashboard.branches.negative', { count: o.stock_negative })}</span>{/if}
                          {#if o.stock_low}<span class="badge badge-warning">{t('dashboard.branches.low', { count: o.stock_low })}</span>{/if}
                          {#if o.stock_empty}<span class="badge badge-neutral">{t('dashboard.branches.empty', { count: o.stock_empty })}</span>{/if}
                        </span>
                      {/if}
                    </td>
                  {/if}
                  {#if data.shifts}<td class="py-2 ps-3 text-end">{formatNumber(o.open_shifts ?? 0)}</td>{/if}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>
    {/if}

    <!-- Stok, piutang, hutang -->
    {#if data.stock || data.receivable || data.payable}
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {#if data.stock}
          {@const st = data.stock}
          <section class="surface-card p-4 lg:p-5 animate-rise-in">
            <div class="flex items-center gap-2 mb-3">
              <span class="grid place-items-center size-9 rounded-lg badge-solid-warning shrink-0"><i class="icon-boxes text-[16px]"></i></span>
              <h2 class="font-display font-bold text-[14px] flex-1">{t('dashboard.stock.title')}</h2>
            </div>
            <dl class="grid grid-cols-2 gap-2 text-center">
              <div class="rounded-lg p-2 bg-[var(--surface-sunken)]">
                <dd class="font-display font-extrabold text-[20px] leading-none">{formatNumber(st.tracked)}</dd>
                <dt class="text-[10.5px] mt-1 u-color-text-tertiary">{t('dashboard.stock.tracked')}</dt>
              </div>
              <div class="rounded-lg p-2 bg-[var(--surface-sunken)]">
                <dd class="font-display font-extrabold text-[20px] leading-none">{formatNumber(st.empty)}</dd>
                <dt class="text-[10.5px] mt-1 u-color-text-tertiary">{t('dashboard.stock.empty')}</dt>
              </div>
              <div class="rounded-lg p-2 {st.low > 0 ? 'bg-[var(--color-warning-50)]' : 'bg-[var(--surface-sunken)]'}">
                <dd class="font-display font-extrabold text-[20px] leading-none {st.low > 0 ? 'text-[var(--color-warning-700)]' : ''}">{formatNumber(st.low)}</dd>
                <dt class="text-[10.5px] mt-1 u-color-text-tertiary">{t('dashboard.stock.low')}</dt>
              </div>
              <div class="rounded-lg p-2 {st.negative > 0 ? 'bg-[var(--color-danger-50)]' : 'bg-[var(--surface-sunken)]'}">
                <dd class="font-display font-extrabold text-[20px] leading-none {st.negative > 0 ? 'text-[var(--color-danger-600)]' : ''}">{formatNumber(st.negative)}</dd>
                <dt class="text-[10.5px] mt-1 u-color-text-tertiary">{t('dashboard.stock.negative')}</dt>
              </div>
            </dl>
            {#if st.value != null}
              <p class="mt-3 text-[12.5px]"><span class="u-color-text-tertiary">{t('dashboard.stock.value')}</span> <span class="font-bold">{money(st.value)}</span></p>
              <p class="text-[10.5px] u-color-text-tertiary">{t('dashboard.stock.valueHint')}</p>
            {/if}
            {#if st.monitored === 0}
              <p class="mt-3 text-[11.5px] u-color-text-tertiary">{t('dashboard.stock.lowUnset')}</p>
            {/if}
            {#if st.lows.length}
              <p class="mt-3 text-[11.5px] font-semibold u-color-text-tertiary">{t('dashboard.stock.lowList')}</p>
              <ul class="mt-1 space-y-0.5 text-[12px]">
                {#each st.lows as n, i (n.name + i)}
                  <li class="flex justify-between gap-2">
                    <span class="truncate">{n.name}{#if n.outlet}<span class="u-color-text-tertiary"> · {n.outlet}</span>{/if}</span>
                    <span class="font-semibold text-[var(--color-warning-700)] shrink-0">{formatNumber(Number(n.qty), { maximumFractionDigits: 2 })} / {formatNumber(Number(n.min ?? 0), { maximumFractionDigits: 2 })}</span>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if st.negatives.length}
              <p class="mt-3 text-[11.5px] font-semibold u-color-text-tertiary">{t('dashboard.stock.negativeList')}</p>
              <ul class="mt-1 space-y-0.5 text-[12px]">
                {#each st.negatives as n, i (n.name + i)}
                  <li class="flex justify-between gap-2"><span class="truncate">{n.name}{#if n.outlet}<span class="u-color-text-tertiary"> · {n.outlet}</span>{/if}</span><span class="font-semibold text-[var(--color-danger-600)] shrink-0">{formatNumber(Number(n.qty), { maximumFractionDigits: 2 })}</span></li>
                {/each}
              </ul>
            {/if}
            {#if can('items', 'view')}<a href="/items" class="mt-3 inline-block text-[11.5px] font-semibold text-[var(--color-primary-600)]">{t('dashboard.stock.all')}</a>{/if}
          </section>
        {/if}

        {#each [data.receivable ? { key: 'receivable', icon: 'wallet', href: '/receivables', mod: 'member_receivables', d: data.receivable } : null, data.payable ? { key: 'payable', icon: 'truck', href: '/supplier-payables', mod: 'supplier_payables', d: data.payable } : null] as card (card?.key)}
          {#if card}
            {@const over = Number(card.d.overdue)}
            <section class="surface-card p-4 lg:p-5 animate-rise-in">
              <div class="flex items-center gap-2 mb-3">
                <span class="grid place-items-center size-9 rounded-lg shrink-0 {over > 0 ? 'badge-solid-danger' : 'badge-solid-success'}"><i class="icon-{card.icon} text-[16px]"></i></span>
                <h2 class="font-display font-bold text-[14px] flex-1">{t(`dashboard.${card.key}.title` as MessageKey)}</h2>
              </div>
              <p class="text-[11.5px] u-color-text-tertiary">{t(`dashboard.${card.key}.outstanding` as MessageKey)}</p>
              <p class="font-display font-extrabold text-[24px] leading-none mt-0.5">{money(card.d.outstanding)}</p>
              <p class="text-[11px] mt-1 u-color-text-tertiary">{t(`dashboard.${card.key}.open` as MessageKey, { count: card.d.open_count })}</p>
              <div class="mt-3 rounded-lg p-2.5 flex items-center justify-between gap-2 {over > 0 ? 'bg-[var(--color-danger-50)]' : 'bg-[var(--surface-sunken)]'}">
                <span class="text-[11.5px] {over > 0 ? 'text-[var(--color-danger-700)] font-semibold' : 'u-color-text-tertiary'}">{t(`dashboard.${card.key}.overdue` as MessageKey)}</span>
                <span class="font-bold text-[14px] {over > 0 ? 'text-[var(--color-danger-700)]' : ''}">{money(card.d.overdue)}</span>
              </div>
              {#if can(card.mod, 'view')}<a href={card.href} class="mt-3 inline-block text-[11.5px] font-semibold text-[var(--color-primary-600)]">{t(`dashboard.${card.key}.all` as MessageKey)}</a>{/if}
            </section>
          {/if}
        {/each}
      </div>
      <p class="text-[11px] u-color-text-tertiary">{t('dashboard.wide')}</p>
    {/if}
  {/if}
</main>
