<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type PettyCashReport } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (value: string) => formatCurrency(Number(value));

  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let data = $state<PettyCashReport | null>(null);
  let loading = $state(false);
  let error = $state('');
  let seq = 0;

  async function load() {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const result = await accounting.pettyCashReport({ from, to });
      if (mine === seq) data = result;
    } catch (cause) {
      if (mine === seq) error = errorMessage(cause);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.pettyCashReport.docTitle');
  });
  $effect(() => {
    void session.outlet?.id;
    void load();
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.pettyCashReport.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.pettyCashReport.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.reports.range')} />
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[680px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('accounting.pettyCashReport.category')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.pettyCashReport.entries')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.pettyCashReport.in')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.pettyCashReport.out')}</th>
            <th class="px-4 py-3 text-end" scope="col">{t('accounting.pettyCashReport.net')}</th>
          </tr>
        </thead>
        <tbody>
          {#each data?.rows ?? [] as row (row.category)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-2.5">{row.category}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatNumber(row.entries)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(row.in)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(row.out)}</td>
              <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{money(row.net)}</td>
            </tr>
          {:else}
            <tr><td colspan="5" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.pettyCashReport.empty')}</td></tr>
          {/each}
        </tbody>
        {#if data && data.rows.length}
          <tfoot>
            <tr class="border-t border-[var(--border-default)] bg-[var(--surface-sunken)] font-bold">
              <td class="px-4 py-2.5">{t('accounting.reports.total')}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatNumber(data.total.entries)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(data.total.in)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(data.total.out)}</td>
              <td class="px-4 py-2.5 text-end tabular-nums">{money(data.total.net)}</td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
  </div>
</main>