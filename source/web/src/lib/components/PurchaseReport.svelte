<script lang="ts" module>
  export type PurchaseReportView = 'invoice' | 'detail' | 'item' | 'bydate';
</script>

<script lang="ts">
  // Laporan pembelian (per faktur, per faktur detail, per barang, per tanggal): satu komponen,
  // empat rute pembungkus. Faktur memakai paginasi keyset ("Muat Lagi"); ringkasan hanya di halaman pertama.
  import { onMount } from 'svelte';
  import { later } from '#lib/debounce.svelte.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import {
    purchaseByDate,
    purchaseInvoices,
    purchaseItems,
    purchaseReportFileUrl,
    type PurchaseByDateResult,
    type PurchaseInvoice,
    type PurchaseInvoiceSummary,
    type PurchaseItemsResult,
    type PurchaseReportKind,
    type PurchaseReportParams
  } from '#lib/reports/purchases/api.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';

  let { view }: { view: PurchaseReportView } = $props();

  const P = 'reports.purchase';
  const C = `${P}.c`;
  const kind = $derived<PurchaseReportKind>(
    ({ invoice: 'invoices', detail: 'invoice-detail', item: 'items', bydate: 'by-date' } as const)[view]
  );
  const showStatus = $derived(view === 'invoice' || view === 'detail');
  const showSearch = $derived(view !== 'bydate');
  const fileBase = $derived({ invoice: 'perfaktur', detail: 'perfaktur-detail', item: 'perbarang', bydate: 'pertanggal' }[view]);

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let supplierId = $state('');
  let supplierLabel = $state('');
  let payment = $state('');
  let status = $state('');
  let q = $state('');
  let sort = $state('total');
  let hideEmpty = $state(false);

  let loading = $state(true);
  let loadingMore = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let invoices = $state<PurchaseInvoice[]>([]);
  let summary = $state<PurchaseInvoiceSummary | null>(null);
  let nextCursor = $state<string | undefined>(undefined);
  let items = $state<PurchaseItemsResult | null>(null);
  let byDate = $state<PurchaseByDateResult | null>(null);
  // Token permintaan: respons lama (filter sudah berganti) dibuang.
  let seq = 0;

  const money = (v: string | number) => formatCurrency(Number(v));
  const qtyFmt = (v: string) => formatNumber(Number(v), { maximumFractionDigits: 3 });

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
  const paymentOptions = $derived([
    { value: '', label: t(`${C}.allPayments`) },
    { value: 'cash', label: t(`${C}.cash`) },
    { value: 'credit', label: t(`${C}.credit`) }
  ]);
  const statusOptions = $derived([
    { value: '', label: t(`${C}.allStatuses`) },
    { value: 'completed', label: t(`${C}.completed`) },
    { value: 'void', label: t(`${C}.void`) }
  ]);
  const sortOptions = $derived([
    { value: 'total', label: t(`${C}.sortTotal`) },
    { value: 'qty', label: t(`${C}.sortQty`) },
    { value: 'name', label: t(`${C}.sortName`) }
  ]);

  const params = (): PurchaseReportParams => ({
    from,
    to,
    outlet_id: outletId || undefined,
    supplier_id: supplierId || undefined,
    payment_type: payment || undefined,
    status: showStatus ? status || undefined : undefined,
    q: showSearch ? q.trim() || undefined : undefined,
    sort: view === 'item' ? sort : undefined,
    hide_empty: view === 'bydate' ? hideEmpty : undefined,
    all_outlets: outletId === ''
  });

  async function load() {
    if (!from || !to) return;
    const my = ++seq;
    loading = true;
    error = '';
    nextCursor = undefined;
    try {
      if (view === 'invoice' || view === 'detail') {
        const res = await purchaseInvoices(kind as 'invoices' | 'invoice-detail', params());
        if (my !== seq) return;
        invoices = res.data;
        summary = res.has_summary ? res.summary : null;
        nextCursor = res.next_cursor;
      } else if (view === 'item') {
        const res = await purchaseItems(params());
        if (my !== seq) return;
        items = res;
      } else {
        const res = await purchaseByDate(params());
        if (my !== seq) return;
        byDate = res;
      }
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      invoices = [];
      summary = null;
      items = null;
      byDate = null;
    } finally {
      if (my === seq) loading = false;
    }
  }

  async function loadMore() {
    if (!nextCursor || loadingMore) return;
    const my = seq;
    loadingMore = true;
    error = '';
    try {
      const res = await purchaseInvoices(kind as 'invoices' | 'invoice-detail', { ...params(), cursor: nextCursor });
      if (my !== seq) return;
      invoices = [...invoices, ...res.data];
      nextCursor = res.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loadingMore = false;
    }
  }

  async function download(format: 'xlsx' | 'pdf') {
    if (!from || !to || exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(purchaseReportFileUrl(kind, params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `pembelian-${fileBase}_${from}_${to}.${format}`;
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

  // Filter/pencarian berubah → muat dari awal (ditunda agar tidak menembak tiap ketikan).
  $effect(() => {
    void outletId;
    void supplierId;
    void payment;
    void status;
    void q;
    void sort;
    void hideEmpty;
    return later(() => void load(), 350);
  });

  const searchOutlets = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${C}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };
  const searchSuppliers = async (term: string): Promise<Option[]> => [
    { id: '', name: t(`${C}.allSuppliers`) },
    ...(await lookup('suppliers').search(term))
  ];

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${C}.allOutlets`) : '');
    if (supplierId === '') supplierLabel = t(`${C}.allSuppliers`);
  });

  onMount(() => {
    document.title = t(`${P}.${view}.docTitle`);
    from = ago(6);
    to = ymd(new Date());
    void refreshOutlets();
  });

  const lbl = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const chip =
    'rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50';
  const card = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const big = 'mt-0.5 font-display text-[20px] font-bold tabular-nums';
  const thS = 'px-3 py-3 text-start font-semibold';
  const thE = 'px-3 py-3 text-end font-semibold';
  const voidBadge = 'ms-1 rounded px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide badge-danger';

  const discText = (l: { disc1: string; disc2: string; disc3: string; disc4: string }) =>
    [l.disc1, l.disc2, l.disc3, l.disc4].filter((d) => Number(d) !== 0).map((d) => String(Number(d))).join('+');
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${P}.${view}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${P}.${view}.subtitle`)}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class={lbl}>{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class={lbl}>{t(`${C}.outlet`)}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t(`${C}.outlet`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${C}.supplier`)}</span>
        <Combobox bind:value={supplierId} bind:label={supplierLabel} search={searchSuppliers} placeholder={t(`${C}.supplier`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${C}.payment`)}</span>
        <Select bind:value={payment} options={paymentOptions} ariaLabel={t(`${C}.payment`)} />
      </div>
      {#if showStatus}
        <div>
          <span class={lbl}>{t(`${C}.status`)}</span>
          <Select bind:value={status} options={statusOptions} ariaLabel={t(`${C}.status`)} />
        </div>
      {/if}
      {#if view === 'item'}
        <div>
          <span class={lbl}>{t(`${C}.sort`)}</span>
          <Select bind:value={sort} options={sortOptions} ariaLabel={t(`${C}.sort`)} />
        </div>
      {/if}
      {#if showSearch}
        <div class="md:col-span-2">
          <span class={lbl}>{view === 'item' ? t(`${C}.item`) : t(`${C}.invoice`)}</span>
          <input
            type="search"
            class="w-full field-control"
            placeholder={view === 'item' ? t(`${C}.searchItem`) : t(`${C}.search`)}
            aria-label={view === 'item' ? t(`${C}.searchItem`) : t(`${C}.search`)}
            maxlength="64"
            bind:value={q}
          />
        </div>
      {/if}
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
      {#if view === 'bydate'}
        <label class="inline-flex items-center gap-2 ms-2 text-[12px] text-[var(--text-secondary)]">
          <input type="checkbox" bind:checked={hideEmpty} />{t(`${C}.hideEmpty`)}
        </label>
      {/if}
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

  {#if (view === 'invoice' || view === 'detail') && summary && summary.purchase_count + summary.void_count > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.invoiceCount`)}</div>
        <div class={big}>{formatNumber(summary.purchase_count)}</div>
        {#if summary.void_count > 0}
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${C}.voidCount`)}: {formatNumber(summary.void_count)}</div>
        {/if}
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.total`)}</div>
        <div class={big}>{money(summary.total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.cashTotal`)}</div>
        <div class={big}>{money(summary.cash_total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.creditTotal`)}</div>
        <div class={big}>{money(summary.credit_total)}</div>
      </div>
    </section>
  {:else if view === 'item' && items && items.summary.item_count > 0}
    <section class="grid gap-3 sm:grid-cols-2">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.itemCount`)}</div>
        <div class={big}>{formatNumber(items.summary.item_count)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.total`)}</div>
        <div class={big}>{money(items.summary.total)}</div>
      </div>
    </section>
  {:else if view === 'bydate' && byDate && byDate.totals.purchase_count + byDate.totals.void_count + byDate.totals.return_count > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.invoiceCount`)}</div>
        <div class={big}>{formatNumber(byDate.totals.purchase_count)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.total`)}</div>
        <div class={big}>{money(byDate.totals.total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.returnTotal`)}</div>
        <div class={big}>{money(byDate.totals.return_total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.net`)}</div>
        <div class={big}>{money(byDate.totals.net_total)}</div>
      </div>
    </section>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full min-w-[1100px] text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            {#if view === 'invoice'}
              <th class={thS} scope="col">{t(`${C}.date`)}</th>
              <th class={thS} scope="col">{t(`${C}.invoice`)}</th>
              <th class={thS} scope="col">{t(`${C}.supplierInvoice`)}</th>
              <th class={thS} scope="col">{t(`${C}.supplier`)}</th>
              <th class={thS} scope="col">{t(`${C}.payment`)}</th>
              <th class={thS} scope="col">{t(`${C}.dueDate`)}</th>
              <th class={thE} scope="col">{t(`${C}.subtotal`)}</th>
              <th class={thE} scope="col">{t(`${C}.tax`)}</th>
              <th class={thE} scope="col">{t(`${C}.otherCost`)}</th>
              <th class={thE} scope="col">{t(`${C}.total`)}</th>
            {:else if view === 'detail'}
              <th class={thS} scope="col">{t(`${C}.date`)}</th>
              <th class={thS} scope="col">{t(`${C}.invoice`)}</th>
              <th class={thS} scope="col">{t(`${C}.supplier`)}</th>
              <th class={thS} scope="col">{t(`${C}.sku`)}</th>
              <th class={thS} scope="col">{t(`${C}.item`)}</th>
              <th class={thS} scope="col">{t(`${C}.unit`)}</th>
              <th class={thE} scope="col">{t(`${C}.qty`)}</th>
              <th class={thE} scope="col">{t(`${C}.price`)}</th>
              <th class={thE} scope="col">{t(`${C}.disc`)}</th>
              <th class={thE} scope="col">{t(`${C}.lineTotal`)}</th>
            {:else if view === 'item'}
              <th class={thS} scope="col">{t(`${C}.sku`)}</th>
              <th class={thS} scope="col">{t(`${C}.item`)}</th>
              <th class={thS} scope="col">{t(`${C}.unit`)}</th>
              <th class={thE} scope="col">{t(`${C}.purchaseCount`)}</th>
              <th class={thE} scope="col">{t(`${C}.qty`)}</th>
              <th class={thE} scope="col">{t(`${C}.avgPrice`)}</th>
              <th class={thE} scope="col">{t(`${C}.lastPrice`)}</th>
              <th class={thE} scope="col">{t(`${C}.total`)}</th>
            {:else}
              <th class={thS} scope="col">{t(`${C}.date`)}</th>
              <th class={thE} scope="col">{t(`${C}.invoiceCount`)}</th>
              <th class={thE} scope="col">{t(`${C}.voidCount`)}</th>
              <th class={thE} scope="col">{t(`${C}.subtotal`)}</th>
              <th class={thE} scope="col">{t(`${C}.tax`)}</th>
              <th class={thE} scope="col">{t(`${C}.otherCost`)}</th>
              <th class={thE} scope="col">{t(`${C}.total`)}</th>
              <th class={thE} scope="col">{t(`${C}.cashTotal`)}</th>
              <th class={thE} scope="col">{t(`${C}.creditTotal`)}</th>
              <th class={thE} scope="col">{t(`${C}.returnTotal`)}</th>
              <th class={thE} scope="col">{t(`${C}.net`)}</th>
            {/if}
          </tr>
        </thead>
        <tbody>
          {#if view === 'invoice'}
            {#each invoices as r (r.id)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                <td class="px-3 py-2 whitespace-nowrap tabular-nums">{formatDate(r.date)}</td>
                <td class="px-3 py-2 font-semibold whitespace-nowrap">
                  {r.doc_no}
                  {#if r.status === 'void'}<span class={voidBadge}>{t(`${C}.void`)}</span>{/if}
                </td>
                <td class="px-3 py-2">{r.supplier_invoice_no}</td>
                <td class="px-3 py-2">{r.supplier_name}</td>
                <td class="px-3 py-2">{r.payment_type === 'credit' ? t(`${C}.credit`) : t(`${C}.cash`)}</td>
                <td class="px-3 py-2 whitespace-nowrap tabular-nums">{r.due_date ? formatDate(r.due_date) : ''}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.subtotal)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.tax_amount)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.other_cost)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.total)}</td>
              </tr>
            {:else}
              <tr><td colspan="10" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {:else if view === 'detail'}
            {#each invoices as r (r.id)}
              {#each r.lines ?? [] as l, i (l.position)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)] {i === 0 ? 'border-t-2' : ''}">
                  <td class="px-3 py-2 whitespace-nowrap tabular-nums">{i === 0 ? formatDate(r.date) : ''}</td>
                  <td class="px-3 py-2 font-semibold whitespace-nowrap">
                    {#if i === 0}
                      {r.doc_no}
                      {#if r.status === 'void'}<span class={voidBadge}>{t(`${C}.void`)}</span>{/if}
                    {/if}
                  </td>
                  <td class="px-3 py-2">{i === 0 ? r.supplier_name : ''}</td>
                  <td class="px-3 py-2">{l.sku}</td>
                  <td class="px-3 py-2">{l.name}</td>
                  <td class="px-3 py-2">{l.unit}</td>
                  <td class="px-3 py-2 text-end tabular-nums">{qtyFmt(l.qty)}</td>
                  <td class="px-3 py-2 text-end tabular-nums">{money(l.unit_price)}</td>
                  <td class="px-3 py-2 text-end tabular-nums">{discText(l)}</td>
                  <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(l.line_total)}</td>
                </tr>
              {/each}
            {:else}
              <tr><td colspan="10" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {:else if view === 'item'}
            {#each items?.data ?? [] as r (r.item_id)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                <td class="px-3 py-2 whitespace-nowrap">{r.sku}</td>
                <td class="px-3 py-2 font-semibold">{r.name}</td>
                <td class="px-3 py-2">{r.unit}</td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(r.purchase_count)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{qtyFmt(r.qty)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.avg_price)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.last_price)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.total)}</td>
              </tr>
            {:else}
              <tr><td colspan="8" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {:else}
            {#each byDate?.data ?? [] as r (r.date)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                <td class="px-3 py-2 whitespace-nowrap tabular-nums">{formatDate(r.date)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(r.purchase_count)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(r.void_count)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.subtotal)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.tax_amount)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.other_cost)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.total)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.cash_total)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.credit_total)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.return_total)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.net_total)}</td>
              </tr>
            {:else}
              <tr><td colspan="11" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {/if}
        </tbody>
        {#if view === 'bydate' && byDate && byDate.data.length > 0}
          {@const g = byDate.totals}
          <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
            <tr class="border-t-2 border-[var(--border-subtle)]">
              <td class="px-3 py-2.5">{t(`${C}.grandTotal`)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatNumber(g.purchase_count)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{formatNumber(g.void_count)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.subtotal)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.tax_amount)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.other_cost)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.cash_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.credit_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.return_total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(g.net_total)}</td>
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
    {#if nextCursor}
      <div class="p-3 text-center border-t border-[var(--border-subtle)]">
        <button type="button" class="btn-secondary text-[12.5px] px-4 py-2" onclick={loadMore} disabled={loadingMore}>
          {loadingMore ? '…' : t(`${C}.loadMore`)}
        </button>
      </div>
    {/if}
  </section>
</main>
