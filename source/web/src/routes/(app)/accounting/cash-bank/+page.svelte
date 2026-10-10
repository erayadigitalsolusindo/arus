<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type CashBank } from '#lib/accounting/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string) => formatCurrency(Number(s));

  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let data = $state<CashBank | null>(null);
  let loading = $state(false);
  let error = $state('');

  let seq = 0;
  async function load() {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.cashBank({ from, to });
      if (mine === seq) data = res;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.cashBank.docTitle');
  });
  $effect(() => {
    void session.outlet?.id;
    void load();
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('accounting.cashBank.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.cashBank.subtitle')}</p>
    </div>
    {#if can('journals', 'create')}
      <div class="flex flex-wrap gap-2">
        <a class="btn" href="/accounting/journals?new=KM"><i class="icon-arrow-down-to-line"></i> {t('accounting.cashBank.cashIn')}</a>
        <a class="btn" href="/accounting/journals?new=KK"><i class="icon-arrow-up-from-line"></i> {t('accounting.cashBank.cashOut')}</a>
        <a class="btn" href="/accounting/journals?new=TK"><i class="icon-arrow-left-right"></i> {t('accounting.cashBank.transfer')}</a>
      </div>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.reports.range')} />
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[760px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('accounting.reports.code')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('accounting.reports.name')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.cashBank.opening')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.cashBank.in')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.cashBank.out')}</th>
            <th class="px-4 py-3 text-end" scope="col">{t('accounting.cashBank.closing')}</th>
          </tr>
        </thead>
        <tbody>
          {#each data?.rows ?? [] as r (r.account_id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-2.5 font-mono whitespace-nowrap">
                <a class="text-[var(--color-primary-600)] hover:underline" href="/accounting/ledger?account={r.account_id}&from={from}&to={to}">{r.code}</a>
              </td>
              <td class="px-3 py-2.5">{r.name}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(r.opening)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{Number(r.debit) ? money(r.debit) : ''}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{Number(r.credit) ? money(r.credit) : ''}</td>
              <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{money(r.closing)}</td>
            </tr>
          {:else}
            <tr><td colspan="6" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.cashBank.empty')}</td></tr>
          {/each}
        </tbody>
        {#if data && data.rows.length}
          <tfoot>
            <tr class="border-t border-[var(--border-default)] bg-[var(--surface-sunken)] font-bold">
              <td class="px-4 py-2.5" colspan="2">{t('accounting.reports.total')}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(data.total.opening)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(data.total.debit)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(data.total.credit)}</td>
              <td class="px-4 py-2.5 text-end tabular-nums">{money(data.total.closing)}</td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
  </div>
</main>
