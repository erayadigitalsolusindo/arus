<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type IncomeStatement, type StatementSection } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string) => formatCurrency(Number(s));

  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let data = $state<IncomeStatement | null>(null);
  let loading = $state(false);
  let error = $state('');

  let seq = 0;
  async function load() {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.incomeStatement({ from, to });
      if (mine === seq) data = res;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.income.docTitle');
  });
  $effect(() => {
    void session.outlet?.id;
    void load();
  });
</script>

{#snippet block(title: string, sec: StatementSection)}
  <tr class="bg-[var(--surface-sunken)] font-semibold"><td class="px-4 py-2.5" colspan="3">{title}</td></tr>
  {#each sec.lines as l (l.account_id)}
    <tr class="border-b border-[var(--border-subtle)] hover:bg-[var(--color-primary)]/5">
      <td class="ps-8 pe-3 py-2 font-mono whitespace-nowrap">
        <a class="text-[var(--color-primary-600)] hover:underline" href="/accounting/ledger?account={l.account_id}&from={from}&to={to}">{l.code}</a>
      </td>
      <td class="px-3 py-2">{l.name}</td>
      <td class="px-4 py-2 text-end tabular-nums">{money(l.amount)}</td>
    </tr>
  {/each}
  <tr class="border-b border-[var(--border-default)] font-semibold">
    <td class="ps-8 pe-3 py-2" colspan="2">{t('accounting.reports.total')} {title}</td>
    <td class="px-4 py-2 text-end tabular-nums">{money(sec.total)}</td>
  </tr>
{/snippet}

{#snippet result(title: string, value: string)}
  <tr class="border-b border-[var(--border-default)] bg-[var(--color-primary)]/5 font-bold">
    <td class="px-4 py-3" colspan="2">{title}</td>
    <td class="px-4 py-3 text-end tabular-nums {Number(value) < 0 ? 'text-[var(--color-danger,#dc2626)]' : ''}">{money(value)}</td>
  </tr>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.income.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.income.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.reports.range')} />
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[560px] text-[12.5px]">
        <tbody>
          {#if data}
            {@render block(t('accounting.income.revenue'), data.revenue)}
            {@render block(t('accounting.income.cogs'), data.cogs)}
            {@render result(t('accounting.income.grossProfit'), data.gross_profit)}
            {@render block(t('accounting.income.expenses'), data.expenses)}
            {@render result(t('accounting.income.netProfit'), data.net_profit)}
          {:else}
            <tr><td class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.reports.empty')}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  </div>
</main>
