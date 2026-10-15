<script lang="ts">
  import { onMount } from 'svelte';
  import { salesByDate, type SalesByDateResult } from '#lib/reports/sales/api.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { t, formatDate, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { paymentMethodsLookup } from '#lib/catalog/api.ts';
  import { iam } from '#lib/iam/api.ts';

  let from = $state('');
  let to = $state('');
  let hideEmpty = $state(true);

  // Filter state
  let outletId = $state('');
  let outletLabel = $state('');
  let cashierId = $state('');
  let cashierLabel = $state('');
  let categoryId = $state('');
  let categoryLabel = $state('');
  let paymentMethodId = $state('');
  let paymentMethodLabel = $state('');

  let cashiers = $state<Option[]>([]);
  let paymentMethods = $state<{ id: string; name: string; kind: string }[]>([]);

  let loading = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let result = $state<SalesByDateResult | null>(null);

  const money = (v: string | number) => formatCurrency(Number(v));

  // Grand total dihitung dari baris yang tampil (tepat per sen, tanpa float).
  const toCents = (v: string | number) => {
    const [rp = '0', cn = '00'] = String(v).split('.');
    return Number(rp) * 100 + Number((cn + '00').slice(0, 2));
  };
  const grand = $derived.by(() => {
    const g = { sale_count: 0, void_count: 0, return_count: 0, subtotal: 0, discount: 0, tax_store: 0, tax_gov: 0, total: 0, paid: 0, receivable: 0, void_total: 0, return_total: 0, net_total: 0 };
    for (const d of result?.data ?? []) {
      g.sale_count += d.sale_count;
      g.void_count += d.void_count;
      g.return_count += d.return_count;
      g.subtotal += toCents(d.subtotal);
      g.discount += toCents(d.discount);
      g.tax_store += toCents(d.tax_store);
      g.tax_gov += toCents(d.tax_gov);
      g.total += toCents(d.total);
      g.paid += toCents(d.paid);
      g.receivable += toCents(d.receivable);
      g.void_total += toCents(d.void_total);
      g.return_total += toCents(d.return_total);
      g.net_total += toCents(d.net_total);
    }
    return g;
  });
  const grandMoney = (cents: number) => money(cents / 100);

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
      result = await salesByDate({ from, to, hide_empty: hideEmpty, outlet_id: outletId || undefined, cashier_id: cashierId || undefined, category_id: categoryId || undefined, payment_method_id: paymentMethodId || undefined, all_outlets: !outletId });
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

  async function download(format: 'xlsx' | 'pdf') {
    if (!from || !to || exporting) return;
    exporting = format;
    error = '';
    try {
      const qs = new URLSearchParams({ from, to, format });
      if (hideEmpty) qs.set('hide_empty', 'true');
      if (outletId) qs.set('outlet_id', outletId);
      if (cashierId) qs.set('cashier_id', cashierId);
      if (categoryId) qs.set('category_id', categoryId);
      if (paymentMethodId) qs.set('payment_method_id', paymentMethodId);
      const blob = await apiBlob(`/reports/sales/by-date?${qs}`);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `penjualan-per-tanggal_${from}_${to}.${format}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 5000);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      exporting = null;
    }
  }

  function resetFilters() {
    outletId = '';
    outletLabel = '';
    cashierId = '';
    cashierLabel = '';
    categoryId = '';
    categoryLabel = '';
    paymentMethodId = '';
    paymentMethodLabel = '';
  }

  // Pemilihan filter → muat ulang (debounce sederhana agar pemindahan Combobox tidak menembak berkali-kali).
  let filterTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void outletId;
    void cashierId;
    void categoryId;
    void paymentMethodId;
    clearTimeout(filterTimer);
    filterTimer = setTimeout(() => void load(), 250);
  });

  const searchOutlets = async (q: string): Promise<Option[]> => {
    const all = [{ id: '', name: t('reports.filterAllOutlets') }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!q) return all;
    const lower = q.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };

  const searchCashiers = async (q: string): Promise<Option[]> => {
    const all = [{ id: '', name: t('reports.filterAllCashiers') }, ...cashiers];
    if (!q) return all;
    const lower = q.toLowerCase();
    return all.filter((c) => c.name.toLowerCase().includes(lower));
  };

  const searchCategories = async (q: string): Promise<Option[]> => {
    try {
      const cats = await lookup('categories').search(q, 50);
      return [{ id: '', name: t('reports.filterAllCategories') }, ...cats.map((c) => ({ id: c.id, name: c.name }))];
    } catch {
      return [{ id: '', name: t('reports.filterAllCategories') }];
    }
  };

  const searchPaymentMethods = async (q: string): Promise<Option[]> => {
    const lower = q.toLowerCase();
    const all = paymentMethods.filter((pm) => !q || pm.name.toLowerCase().includes(lower) || pm.kind.toLowerCase().includes(lower));
    return [{ id: '', name: t('reports.filterAllPaymentMethods') }, ...all.map((pm) => ({ id: pm.id, name: `${pm.name} (${pm.kind})` }))];
  };

  onMount(() => {
    document.title = t('reports.docTitle');
    const now = ymd(new Date());
    if (!from) from = ago(6);
    if (!to) to = now;
    void refreshOutlets();
    void load();
    // Daftar kasir & metode bayar bersifat ringan (best-effort): laporan tetap
    // bisa dibuka tanpa izin users/payment_methods.
    iam
      .users()
      .then((users) => (cashiers = users.filter((u) => u.active).map((u) => ({ id: u.id, name: u.name }))))
      .catch(() => undefined);
    paymentMethodsLookup
      .all()
      .then((pms) => (paymentMethods = pms.map((pm) => ({ id: pm.id, name: pm.name, kind: pm.kind }))))
      .catch(() => undefined);
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('reports.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('reports.subtitle')}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-5">
      <div class="md:col-span-1">
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.filterOutlet')}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t('reports.filterOutlet')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.filterCashier')}</span>
        <Combobox bind:value={cashierId} bind:label={cashierLabel} search={searchCashiers} placeholder={t('reports.filterCashier')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.filterCategory')}</span>
        <Combobox bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t('reports.filterCategory')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.filterPaymentMethod')}</span>
        <Combobox bind:value={paymentMethodId} bind:label={paymentMethodLabel} search={searchPaymentMethods} placeholder={t('reports.filterPaymentMethod')} />
      </div>
    </div>
    {#if outletId || cashierId || categoryId || paymentMethodId}
      <div>
        <button type="button" class="text-[12px] text-[var(--color-primary-600)] hover:underline cursor-pointer" onclick={resetFilters}>{t('reports.resetFilters')}</button>
      </div>
    {/if}
  </section>

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
    <span class="inline-flex items-center gap-1.5 ms-2">
      <button
        type="button"
        class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50"
        disabled={exporting !== null || !result?.data?.length}
        onclick={() => download('xlsx')}
      >{exporting === 'xlsx' ? '…' : 'Excel'}</button>
      <button
        type="button"
        class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50"
        disabled={exporting !== null || !result?.data?.length}
        onclick={() => download('pdf')}
      >{exporting === 'pdf' ? '…' : 'PDF'}</button>
    </span>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full min-w-[1500px] text-[12.5px]">
        <thead class="bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-3 text-start font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.date')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.saleCount')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.voidCount')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.subtotal')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.discount')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.taxStore')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.taxGov')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.total')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.paid')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.receivable')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.voidTotal')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.returnCount')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.returnTotal')}</th>
            <th class="px-3 py-3 text-end font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.netTotal')}</th>
            <th class="px-4 py-3 text-start font-semibold sticky top-0 bg-[var(--surface-sunken)]" scope="col">{t('reports.methods')}</th>
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
              <td colspan="16" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('reports.noData')}</td>
            </tr>
          {/each}
        </tbody>
        {#if (result?.data ?? []).length > 0}
          <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)]">
            <tr class="border-t-2 border-[var(--border-subtle)] font-semibold">
              <td class="px-4 py-2.5 whitespace-nowrap">{t('reports.grandTotal')}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grand.sale_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grand.void_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.subtotal)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.discount)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.tax_store)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.tax_gov)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.paid)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.receivable)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.void_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grand.return_count}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.return_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{grandMoney(grand.net_total)}</td>
              <td class="px-4 py-2.5"></td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
  </section>
</main>
