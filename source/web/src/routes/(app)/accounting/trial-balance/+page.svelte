<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type TrialBalance } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string) => (Number(s) ? formatCurrency(Number(s)) : '');

  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let data = $state<TrialBalance | null>(null);
  let loading = $state(false);
  let error = $state('');

  let seq = 0;
  async function load() {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.trialBalance({ from, to });
      if (mine === seq) data = res;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.trial.docTitle');
  });
  $effect(() => {
    void session.outlet?.id;
    void load();
  });

  const th = 'px-3 py-2 text-end';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.trial.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.trial.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.reports.range')} />
      {#if data}
        <span class="badge-soft {data.balanced ? 'badge-success' : 'badge-danger'}">{data.balanced ? t('accounting.reports.balanced') : t('accounting.reports.unbalanced')}</span>
      {/if}
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[980px] text-[12.5px]">
        <thead>
          <tr class="bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 pt-3 text-start" rowspan="2" scope="col">{t('accounting.reports.code')}</th>
            <th class="px-3 pt-3 text-start" rowspan="2" scope="col">{t('accounting.reports.name')}</th>
            <th class="px-3 pt-3 text-center border-s border-[var(--border-subtle)]" colspan="2" scope="colgroup">{t('accounting.trial.opening')}</th>
            <th class="px-3 pt-3 text-center border-s border-[var(--border-subtle)]" colspan="2" scope="colgroup">{t('accounting.trial.movement')}</th>
            <th class="px-3 pt-3 text-center border-s border-[var(--border-subtle)]" colspan="2" scope="colgroup">{t('accounting.trial.closing')}</th>
          </tr>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="{th} border-s border-[var(--border-subtle)]" scope="col">{t('accounting.trial.debit')}</th>
            <th class={th} scope="col">{t('accounting.trial.credit')}</th>
            <th class="{th} border-s border-[var(--border-subtle)]" scope="col">{t('accounting.trial.debit')}</th>
            <th class={th} scope="col">{t('accounting.trial.credit')}</th>
            <th class="{th} border-s border-[var(--border-subtle)]" scope="col">{t('accounting.trial.debit')}</th>
            <th class={th} scope="col">{t('accounting.trial.credit')}</th>
          </tr>
        </thead>
        <tbody>
          {#each data?.rows ?? [] as r (r.account_id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-2.5 font-mono whitespace-nowrap">
                <a class="text-[var(--color-primary-600)] hover:underline" href="/accounting/ledger?account={r.account_id}&from={from}&to={to}">{r.code}</a>
              </td>
              <td class="px-3 py-2.5">{r.name}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{money(r.opening_debit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(r.opening_credit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{money(r.debit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(r.credit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{money(r.closing_debit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(r.closing_credit)}</td>
            </tr>
          {:else}
            <tr><td colspan="8" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.reports.empty')}</td></tr>
          {/each}
        </tbody>
        {#if data && data.rows.length}
          <tfoot>
            <tr class="border-t border-[var(--border-default)] bg-[var(--surface-sunken)] font-bold">
              <td class="px-4 py-2.5" colspan="2">{t('accounting.reports.total')}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{formatCurrency(Number(data.totals.opening_debit))}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatCurrency(Number(data.totals.opening_credit))}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{formatCurrency(Number(data.totals.debit))}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatCurrency(Number(data.totals.credit))}</td>
              <td class="px-3 py-2.5 text-end tabular-nums border-s border-[var(--border-subtle)]">{formatCurrency(Number(data.totals.closing_debit))}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatCurrency(Number(data.totals.closing_credit))}</td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
  </div>
</main>
