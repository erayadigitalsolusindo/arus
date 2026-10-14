<script lang="ts">
  import { onMount } from 'svelte';
  import { salesByDate, type SalesByDateResult, type DayRow } from '#lib/reports/sales/api.ts';
  import { t, formatDate, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';

  let from = $state('');
  let to = $state('');
  let hideEmpty = $state(true);

  let loading = $state(false);
  let error = $state('');
  let result = $state<SalesByDateResult | null>(null);

  const money = (v: string | number) => formatCurrency(Number(v));

  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const ago = (days: number) => {
    const d = new Date();
    d.setDate(d.getDate() - days);
    return ymd(d);
  };
  const presets = $derived.by(() => {
    const now = new Date();
    return [
      { id: 'today' as const, from: ymd(now), to: ymd(now) },
      { id: 'days7' as const, from: ago(6), to: ymd(now) },
      { id: 'days30' as const, from: ago(29), to: ymd(now) },
      { id: 'month' as const, from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: ymd(now) }
    ];
  });

  async function load() {
    if (!from || !to) return;
    loading = true;
    error = '';
    try {
      result = await salesByDate({ from, to, hide_empty: hideEmpty });
    } catch (err) {
      error = errorMessage(err);
      result = null;
    } finally {
      loading = false;
    }
  }

  function applyPreset(p: { from: string; to: string }) {
    from = p.from;
    to = p.to;
    void load();
  }

  onMount(() => {
    document.title = t('reports.docTitle');
    const now = ymd(new Date());
    if (!from) from = ago(6);
    if (!to) to = now;
    void load();
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('reports.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('reports.subtitle')}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-[minmax(0,1.5fr)]">
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      <i class="icon-calendar text-[13px] text-[var(--text-tertiary)] me-0.5"></i>
      {#each presets as p (p.id)}
        {@const on = from === p.from && to === p.to}
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors {on
            ? 'border-[var(--color-primary-600)] bg-[var(--color-primary-600)] text-white'
            : 'border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)]'}"
          aria-pressed={on}
          onclick={() => applyPreset(p)}
        >{t(`payables.history.preset.${p.id}`)}</button>
      {/each}
      <label class="inline-flex items-center gap-1.5 text-[12px] cursor-pointer ms-auto text-[var(--text-secondary)]"><input type="checkbox" class="size-3.5 rounded accent-[var(--color-primary-600)]" bind:checked={hideEmpty} onchange={() => load()} />{t('reports.hideEmpty')}</label>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[1500px] text-[12.5px]">
        <thead class="bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('reports.date')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.saleCount')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.voidCount')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.subtotal')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.discount')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.taxStore')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.taxGov')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.total')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.paid')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.receivable')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.voidTotal')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.returnCount')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.returnTotal')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.netTotal')}</th>
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('reports.methods')}</th>
          </tr>
        </thead>
        <tbody>
          {#each result?.data ?? [] as d (d.date)}
            <tr class="border-t border-[var(--border-subtle)] align-top transition-colors hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-2.5 whitespace-nowrap font-semibold">{formatDate(d.date)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{d.sale_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{d.void_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.subtotal)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.discount)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.tax_store)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.tax_gov)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums font-semibold">{money(d.total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.paid)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.receivable)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.void_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{d.return_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(d.return_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums font-semibold">{money(d.net_total)}</td>
              <td class="px-4 py-2.5 text-[11.5px]">
                {#if d.methods?.length}
                  <ul class="space-y-0.5">
                    {#each d.methods as m (m.name + (m.method_id ?? ''))}
                      <li class="flex items-center justify-between gap-2 whitespace-nowrap">
                        <span>{m.name} <span class="text-[var(--text-tertiary)]">({m.kind})</span></span>
                        <span class="tabular-nums">{money(m.amount)}</span>
                      </li>
                    {/each}
                  </ul>
                {:else}
                  <span class="text-[var(--text-tertiary)]">—</span>
                {/if}
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="15" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('reports.noData')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>
</main>