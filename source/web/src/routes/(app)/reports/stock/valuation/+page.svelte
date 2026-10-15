<script lang="ts">
  import { onMount } from 'svelte';
  import { stockValuation, type ValuationResult, type ValuationRow, type ValuationTotals } from '#lib/reports/stock/api.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let categoryId = $state('');
  let categoryLabel = $state('');

  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let rows = $state<ValuationRow[]>([]);
  let totals = $state<ValuationTotals | null>(null);
  let nextCursor = $state<string | undefined>(undefined);

  const money = (v: string | number) => formatCurrency(Number(v));
  const qty = (v: string) => formatNumber(Number(v));

  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  async function load() {
    loading = true;
    error = '';
    rows = [];
    totals = null;
    nextCursor = undefined;
    try {
      const result = await stockValuation({
        from: from || undefined,
        to: to || undefined,
        outlet_id: outletId || undefined,
        category_id: categoryId || undefined,
      });
      rows = result.data;
      totals = result.totals;
      nextCursor = result.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!nextCursor || loadingMore) return;
    loadingMore = true;
    error = '';
    try {
      const result = await stockValuation({
        from: from || undefined,
        to: to || undefined,
        outlet_id: outletId || undefined,
        category_id: categoryId || undefined,
        cursor: nextCursor,
      });
      rows = [...rows, ...result.data];
      nextCursor = result.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loadingMore = false;
    }
  }

  const searchOutlets = async (q: string): Promise<Option[]> => {
    const all = [{ id: '', name: t('reports.stockValuation.allOutlets') }, ...accessibleOutlets.items.map(o => ({ id: o.id, name: o.name }))];
    if (!q) return all;
    const lower = q.toLowerCase();
    return all.filter(o => o.name.toLowerCase().includes(lower));
  };

  const searchCategories = async (q: string): Promise<Option[]> => {
    try {
      const cats = await lookup('categories').search(q, 50);
      return [{ id: '', name: t('reports.stockValuation.allCategories') }, ...cats.map(c => ({ id: c.id, name: c.name }))];
    } catch {
      return [{ id: '', name: t('reports.stockValuation.allCategories') }];
    }
  };

  $effect(() => {
    const o = accessibleOutlets.items.find(o => o.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t('reports.stockValuation.allOutlets') : '');
  });

  onMount(() => {
    document.title = t('reports.stockValuation.docTitle');
    void refreshOutlets();
  });

  const totalDisplay = $derived(totals ? Number(totals.display_qty) : 0);
  const totalWarehouse = $derived(totals ? Number(totals.warehouse_qty) : 0);
  const totalReturns = $derived(totals ? Number(totals.returns_qty) : 0);
  const totalQty = $derived(totals ? Number(totals.total_qty) : 0);
  const totalStockValue = $derived(totals ? totals.stock_value : '0');
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('reports.stockValuation.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('reports.stockValuation.subtitle')}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.stockValuation.outlet')}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t('reports.stockValuation.outlet')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.stockValuation.category')}</span>
        <Combobox bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t('reports.stockValuation.category')} />
      </div>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-x-auto scroll-thin max-h-[calc(100vh-320px)]">
      <table class="w-full min-w-[1400px] text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-3 text-start font-semibold" scope="col">SKU</th>
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('reports.stockValuation.title')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.displayQty')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.warehouseQty')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.returnsQty')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.totalQty')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.avgCost')}</th>
            <th class="px-4 py-3 text-end font-semibold" scope="col">{t('reports.stockValuation.stockValue')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-2.5 whitespace-nowrap font-mono text-[11.5px]">{r.sku}</td>
              <td class="px-4 py-2.5">{r.name}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{qty(r.display_qty)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{qty(r.warehouse_qty)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{qty(r.returns_qty)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums font-semibold">{qty(r.total_qty)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(r.avg_cost)}</td>
              <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{money(r.stock_value)}</td>
            </tr>
          {:else}
            <tr>
              <td colspan="8" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('reports.noData')}</td>
            </tr>
          {/each}
        </tbody>
        {#if rows.length}
          <tfoot class="sticky bottom-0 z-10 bg-[var(--surface-sunken)]">
            <tr class="border-t-2 border-[var(--border-subtle)] font-semibold">
              <td class="px-4 py-3 text-end text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]" colspan="2">{t('reports.stockValuation.totalValue')}</td>
              <td class="px-3 py-3 text-end tabular-nums">{qty(totalDisplay.toFixed(3))}</td>
              <td class="px-3 py-3 text-end tabular-nums">{qty(totalWarehouse.toFixed(3))}</td>
              <td class="px-3 py-3 text-end tabular-nums">{qty(totalReturns.toFixed(3))}</td>
              <td class="px-3 py-3 text-end tabular-nums">{qty(totalQty.toFixed(3))}</td>
              <td class="px-3 py-3 text-end text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]"></td>
              <td class="px-4 py-3 text-end tabular-nums">{money(totalStockValue)}</td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
    {#if nextCursor}
      <div class="p-3 text-center border-t border-[var(--border-subtle)]">
        <button
          type="button"
          class="btn-secondary text-[12.5px] px-4 py-2"
          onclick={loadMore}
          disabled={loadingMore}
        >
          {loadingMore ? '…' : t('reports.stockValuation.loadMore')}
        </button>
      </div>
    {/if}
  </section>
</main>
