<script lang="ts">
  import { onMount } from 'svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { accounting, todayISO, type BalanceSheet, type StatementSection } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string) => formatCurrency(Number(s));

  let asOf = $state(todayISO());
  let data = $state<BalanceSheet | null>(null);
  let loading = $state(false);
  let error = $state('');

  let seq = 0;
  async function load() {
    if (!asOf) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.balanceSheet(asOf);
      if (mine === seq) data = res;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.balance.docTitle');
  });
  $effect(() => {
    void asOf;
    void session.outlet?.id;
    void load();
  });

  const diff = $derived(data ? formatCurrency(Number(data.total_assets) - Number(data.total_liabilities_equity)) : '');
</script>

{#snippet block(title: string, sec: StatementSection, extra?: { label: string; amount: string })}
  <tr class="bg-[var(--surface-sunken)] font-semibold"><td class="px-4 py-2.5" colspan="3">{title}</td></tr>
  {#each sec.lines as l (l.account_id)}
    <tr class="border-b border-[var(--border-subtle)] hover:bg-[var(--color-primary)]/5">
      <td class="ps-8 pe-3 py-2 font-mono whitespace-nowrap">
        <a class="text-[var(--color-primary-600)] hover:underline" href="/accounting/ledger?account={l.account_id}&from={asOf.slice(0, 4)}-01-01&to={asOf}">{l.code}</a>
      </td>
      <td class="px-3 py-2">{l.name}</td>
      <td class="px-4 py-2 text-end tabular-nums">{money(l.amount)}</td>
    </tr>
  {/each}
  {#if extra && Number(extra.amount)}
    <tr class="border-b border-[var(--border-subtle)]">
      <td class="ps-8 pe-3 py-2" colspan="2"><em>{extra.label}</em></td>
      <td class="px-4 py-2 text-end tabular-nums">{money(extra.amount)}</td>
    </tr>
  {/if}
  <tr class="border-b border-[var(--border-default)] font-semibold">
    <td class="ps-8 pe-3 py-2" colspan="2">{t('accounting.reports.total')} {title}</td>
    <td class="px-4 py-2 text-end tabular-nums">{money(extra ? String(Number(sec.total) + Number(extra.amount)) : sec.total)}</td>
  </tr>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.balance.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.balance.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <label class="flex items-center gap-2 text-[12.5px] font-semibold" for="bs-asof">{t('accounting.reports.asOf')}</label>
      <div class="w-44"><DatePicker id="bs-asof" bind:value={asOf} clearable={false} /></div>
      {#if data}
        <span class="badge-soft {data.balanced ? 'badge-success' : 'badge-danger'}">{data.balanced ? t('accounting.reports.balanced') : t('accounting.balance.diff', { diff })}</span>
      {/if}
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[560px] text-[12.5px]">
        <tbody>
          {#if data}
            {@render block(t('accounting.balance.assets'), data.assets)}
            <tr class="border-b border-[var(--border-default)] bg-[var(--color-primary)]/5 font-bold">
              <td class="px-4 py-3" colspan="2">{t('accounting.balance.totalAssets')}</td>
              <td class="px-4 py-3 text-end tabular-nums">{money(data.total_assets)}</td>
            </tr>
            {@render block(t('accounting.balance.liabilities'), data.liabilities)}
            {@render block(t('accounting.balance.equity'), data.equity, { label: t('accounting.balance.unclosedProfit'), amount: data.unclosed_profit })}
            <tr class="border-b border-[var(--border-default)] bg-[var(--color-primary)]/5 font-bold">
              <td class="px-4 py-3" colspan="2">{t('accounting.balance.totalLiabEquity')}</td>
              <td class="px-4 py-3 text-end tabular-nums">{money(data.total_liabilities_equity)}</td>
            </tr>
          {:else}
            <tr><td class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.reports.empty')}</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  </div>
</main>
