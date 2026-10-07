<script lang="ts">
  import { t, formatDate, type MessageKey } from '#lib/i18n/index.ts';

  const kpis: { icon: string; tone: string; label: MessageKey; value: string; rows: [MessageKey, string][] }[] = [
    { icon: 'banknote', tone: 'primary', label: 'dashboard.kpi.salesToday', value: 'Rp —', rows: [['dashboard.kpi.monthlyTarget', '—'], ['dashboard.kpi.profitShare', '—']] },
    { icon: 'receipt', tone: 'info', label: 'dashboard.kpi.receipts', value: '—', rows: [['dashboard.kpi.dailyAverage', '—'], ['dashboard.kpi.peakHour', '—']] },
    { icon: 'boxes', tone: 'warning', label: 'dashboard.kpi.lowStock', value: '—', rows: [['dashboard.kpi.activeItems', '—'], ['dashboard.kpi.negativeStock', '—']] },
    { icon: 'wallet', tone: 'danger', label: 'dashboard.kpi.overdue', value: 'Rp —', rows: [['dashboard.kpi.totalReceivable', '—'], ['dashboard.kpi.due7Days', '—']] }
  ];

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

<main class="p-4 lg:p-6 space-y-6 max-w-full mx-auto w-full">
  <!-- Hero -->
  <div class="relative overflow-hidden rounded-2xl p-5 lg:p-7 text-white animate-rise-in bg-[linear-gradient(135deg,var(--color-primary-700),var(--color-primary-950)_65%)] shadow-[var(--shadow-lg)]">
    <span class="absolute -top-16 -end-10 size-56 rounded-full pointer-events-none bg-[rgb(255_255_255_/_0.08)] blur-[4px]"></span>
    <span class="absolute -bottom-20 -start-10 size-48 rounded-full pointer-events-none bg-[var(--color-accent-500)] opacity-[0.16] blur-[10px]"></span>
    <svg class="absolute inset-0 w-full h-full opacity-[0.06] pointer-events-none" preserveAspectRatio="none" aria-hidden="true">
      <pattern id="heroGrid" width="28" height="28" patternUnits="userSpaceOnUse"><path d="M28 0H0V28" fill="none" stroke="white" stroke-width="1"></path></pattern>
      <rect width="100%" height="100%" fill="url(#heroGrid)"></rect>
    </svg>

    <div class="relative flex flex-wrap items-start justify-between gap-5">
      <div class="min-w-0">
        <span class="inline-flex items-center gap-1.5 px-2.5 py-1 mb-2.5 rounded-full text-[10.5px] font-semibold bg-[rgb(255_255_255_/_0.14)] border border-[rgb(255_255_255_/_0.18)]">
          <i class="icon-sparkle text-[10px]"></i>{t('dashboard.badge')}
        </span>
        <h1 class="font-display font-extrabold text-[24px] lg:text-[28px] leading-tight text-[#fff]">{t('dashboard.welcome')}</h1>
        <p class="text-[13px] mt-1.5 max-w-lg text-[rgb(255_255_255_/_0.78)]">
          {t('dashboard.intro')}
        </p>
      </div>
      <div class="flex flex-col items-end gap-3 shrink-0">
        <div class="flex flex-wrap items-center gap-2 justify-end">
          <button type="button" class="btn !text-[12.5px] !border-0 bg-[rgb(255_255_255_/_0.14)] text-[#fff]"><i class="icon-download text-[13px]"></i>{t('dashboard.exportReport')}</button>
          <button type="button" class="btn !text-[12.5px] !border-0 bg-[#fff] text-[var(--color-primary-800)]"><i class="icon-refresh-cw text-[13px]"></i>{t('dashboard.reload')}</button>
        </div>
        <p class="text-[11px] text-end text-[rgb(255_255_255_/_0.65)]">{today}</p>
      </div>
    </div>
  </div>

  <!-- KPI -->
  <div class="bento-grid">
    {#each kpis as k (k.icon)}
      <article class="surface-card is-interactive col-span-12 sm:col-span-6 lg:col-span-3 p-4 animate-rise-in">
        <div class="flex items-start justify-between gap-2">
          <span class="grid place-items-center size-9 rounded-lg badge-solid-{k.tone} shrink-0"><i class="icon-{k.icon} text-[16px]"></i></span>
        </div>
        <h2 class="text-[11.5px] mt-2.5 u-color-text-tertiary">{t(k.label)}</h2>
        <h3 class="font-display font-extrabold text-[24px] leading-none mt-0.5">{k.value}</h3>
        <ul class="space-y-1 mt-3 text-[11px] u-color-text-tertiary">
          {#each k.rows as [name, val] (name)}
            <li class="flex items-center justify-between"><span>{t(name)}</span><span class="font-semibold u-color-text-secondary">{val}</span></li>
          {/each}
        </ul>
      </article>
    {/each}
  </div>
</main>

<footer class="px-4 lg:px-6 py-4 flex flex-col sm:flex-row items-center justify-between gap-2 text-[12px] border-t text-[var(--text-tertiary)] bg-[var(--surface-card)] border-[var(--border-subtle)]">
  <span>© {new Date().getFullYear()} ACIRABA NewGen</span>
  <span>{t('dashboard.devVersion')}</span>
</footer>
