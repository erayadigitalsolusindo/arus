<script lang="ts">
  import { onMount } from 'svelte';
  import {
    salesItems,
    salesItemsFileUrl,
    type SalesItemsResult,
    type ItemSaleRow,
    type SalesItemsParams
  } from '#lib/reports/sales/api.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { iam } from '#lib/iam/api.ts';
  import { lookup } from '#lib/catalog/api.ts';

  const D = 'reports.salesItem';
  const S = 'reports.salesDetail';

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let cashierId = $state('');
  let cashierLabel = $state('');
  let categoryId = $state('');
  let categoryLabel = $state('');
  let sort = $state('total');
  let limit = $state('100');
  let q = $state('');

  let loading = $state(true);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let result = $state<SalesItemsResult | null>(null);
  let rows = $state<ItemSaleRow[]>([]);
  let cashiers = $state<Option[]>([]);
  // Token permintaan: respons lama (filter sudah berganti) dibuang.
  let seq = 0;

  const money = (v: string | number) => formatCurrency(Number(v));
  const qty = (v: string) => formatNumber(Number(v));

  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const ago = (daysBack: number) => {
    const d = new Date();
    d.setDate(d.getDate() - daysBack);
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
  const sortOptions = $derived([
    { value: 'total', label: t(`${D}.sortTotal`) },
    { value: 'qty', label: t(`${D}.sortQty`) },
    { value: 'name', label: t(`${D}.sortName`) }
  ]);
  const limitOptions = ['100', '300', '1000'].map((v) => ({ value: v, label: v }));

  const params = (): SalesItemsParams => ({
    from,
    to,
    outlet_id: outletId || undefined,
    cashier_id: cashierId || undefined,
    category_id: categoryId || undefined,
    q: q.trim() || undefined,
    sort,
    limit: Number(limit),
    all_outlets: outletId === ''
  });

  async function load() {
    if (!from || !to) return;
    const my = ++seq;
    loading = true;
    error = '';
    try {
      const res = await salesItems(params());
      if (my !== seq) return;
      result = res;
      rows = res.data;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      result = null;
      rows = [];
    } finally {
      if (my === seq) loading = false;
    }
  }

  async function download(format: 'xlsx' | 'pdf') {
    if (!from || !to || exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(salesItemsFileUrl(params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `penjualan-perbarang_${from}_${to}.${format}`;
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

  function applyPreset(p: { from: string; to: string }) {
    from = p.from;
    to = p.to;
    void load();
  }

  // Filter/pencarian berubah → muat ulang (ditunda agar tidak menembak tiap ketikan).
  $effect(() => {
    void outletId;
    void cashierId;
    void categoryId;
    void sort;
    void limit;
    void q;
    const h = setTimeout(() => void load(), 350);
    return () => clearTimeout(h);
  });

  const searchOutlets = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${S}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };
  const searchCashiers = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${S}.allCashiers`) }, ...cashiers];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((c) => c.name.toLowerCase().includes(lower));
  };
  const searchCategories = async (term: string): Promise<Option[]> => {
    try {
      const cats = await lookup('categories').search(term, 50);
      return [{ id: '', name: t(`${S}.allCategories`) }, ...cats.map((c) => ({ id: c.id, name: c.name }))];
    } catch {
      return [{ id: '', name: t(`${S}.allCategories`) }];
    }
  };

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${S}.allOutlets`) : '');
    cashierLabel = cashiers.find((c) => c.id === cashierId)?.name ?? (cashierId === '' ? t(`${S}.allCashiers`) : '');
    if (categoryId === '') categoryLabel = t(`${S}.allCategories`);
  });

  const summary = $derived(result?.summary ?? null);

  onMount(() => {
    document.title = t(`${D}.docTitle`);
    from = ago(6);
    to = ymd(new Date());
    void refreshOutlets();
    iam
      .users()
      .then((users) => (cashiers = users.filter((u) => u.active).map((u) => ({ id: u.id, name: u.name }))))
      .catch(() => undefined);
  });

  const lbl = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const chip =
    'rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50';
  const card = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${D}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${D}.subtitle`)}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class={lbl}>{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class={lbl}>{t(`${S}.outlet`)}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t(`${S}.outlet`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${S}.cashier`)}</span>
        <Combobox bind:value={cashierId} bind:label={cashierLabel} search={searchCashiers} placeholder={t(`${S}.cashier`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${S}.category`)}</span>
        <Combobox bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t(`${S}.category`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${D}.sortLabel`)}</span>
        <Select bind:value={sort} options={sortOptions} ariaLabel={t(`${D}.sortLabel`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${D}.limitLabel`)}</span>
        <Select bind:value={limit} options={limitOptions} ariaLabel={t(`${D}.limitLabel`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${D}.item`)}</span>
        <input
          type="search"
          class="w-full field-control"
          placeholder={t(`${D}.search`)}
          aria-label={t(`${D}.search`)}
          maxlength="64"
          bind:value={q}
        />
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
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
      <span class="inline-flex items-center gap-1.5 ms-auto">
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('xlsx')}>{exporting === 'xlsx' ? '…' : 'Excel'}</button>
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('pdf')}>{exporting === 'pdf' ? '…' : 'PDF'}</button>
      </span>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if summary && summary.item_count > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.itemCount`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{formatNumber(summary.item_count)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.gross`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.gross)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.discount`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.discount)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.net`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.net)}</div>
      </div>
    </section>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full min-w-[900px] text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.sku`)}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.item`)}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.category`)}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t(`${D}.qty`)}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.unit`)}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t(`${D}.frequency`)}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t(`${D}.gross`)}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t(`${D}.discount`)}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t(`${D}.net`)}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.item_id)}
            <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
              <td class="px-3 py-2 whitespace-nowrap text-[var(--text-secondary)]">{r.sku}</td>
              <td class="px-3 py-2 font-semibold">{r.name}</td>
              <td class="px-3 py-2">{r.category_name}</td>
              <td class="px-3 py-2 text-end tabular-nums">{qty(r.qty)}</td>
              <td class="px-3 py-2">{r.unit_name}</td>
              <td class="px-3 py-2 text-end tabular-nums">{formatNumber(r.line_count)}</td>
              <td class="px-3 py-2 text-end tabular-nums">{money(r.gross)}</td>
              <td class="px-3 py-2 text-end tabular-nums">{money(r.discount)}</td>
              <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.net)}</td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">
                {loading ? '…' : t(`${D}.noData`)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if result?.truncated}
      <div class="p-3 text-center text-[12px] border-t border-[var(--border-subtle)] text-[var(--text-tertiary)]">
        {t(`${D}.shown`, { shown: formatNumber(rows.length), total: formatNumber(result.summary.item_count) })}
      </div>
    {/if}
  </section>
</main>
