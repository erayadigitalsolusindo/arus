<script lang="ts">
  import { onMount } from 'svelte';
  import { later } from '#lib/debounce.svelte.ts';
  import {
    salesCustomers,
    salesCustomerInvoices,
    salesCustomerInvoicesFileUrl,
    type SalesCustomersResult,
    type CustInvSale,
    type CustomerInvoicesParams
  } from '#lib/reports/sales/api.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { iam } from '#lib/iam/api.ts';

  const D = 'reports.salesCustomerInvoice';
  const C = 'reports.salesCustomer';
  const S = 'reports.salesDetail';

  // Faktur satu pelanggan yang sudah dimuat (dimuat saat akordion dibuka).
  type Bucket = { sales: CustInvSale[]; next?: string; loading: boolean; error: string };

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let cashierId = $state('');
  let cashierLabel = $state('');
  let q = $state('');
  let sort = $state('total');
  let limit = $state('100');
  let withItems = $state(true);

  let loading = $state(true);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let result = $state<SalesCustomersResult | null>(null);
  let cashiers = $state<Option[]>([]);
  let openCust = $state<Record<string, boolean>>({});
  let openInv = $state<Record<string, boolean>>({});
  let buckets = $state<Record<string, Bucket>>({});
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
    { value: 'total', label: t(`${C}.sortTotal`) },
    { value: 'count', label: t(`${C}.sortCount`) },
    { value: 'name', label: t(`${C}.sortName`) },
    { value: 'last', label: t(`${C}.sortLast`) }
  ]);
  const limitOptions = ['50', '100', '300', '1000'].map((v) => ({ value: v, label: v }));

  const base = () => ({
    from,
    to,
    outlet_id: outletId || undefined,
    cashier_id: cashierId || undefined,
    q: q.trim() || undefined,
    sort,
    all_outlets: outletId === ''
  });

  async function load() {
    if (!from || !to) return;
    const my = ++seq;
    loading = true;
    error = '';
    openCust = {};
    openInv = {};
    buckets = {};
    try {
      const res = await salesCustomers({ ...base(), limit: Number(limit) });
      if (my !== seq) return;
      result = res;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      result = null;
    } finally {
      if (my === seq) loading = false;
    }
  }

  // Muat faktur satu pelanggan (halaman pertama saat dibuka; lanjutan lewat tombol).
  async function loadInvoices(memberId: string, more = false) {
    const cur = buckets[memberId];
    if (cur?.loading) return;
    const my = seq;
    const prev = more ? (cur?.sales ?? []) : [];
    buckets[memberId] = { sales: prev, next: cur?.next, loading: true, error: '' };
    try {
      const p: CustomerInvoicesParams = { ...base(), q: undefined, sort: undefined, member_id: memberId, with_items: true, cursor: more ? cur?.next : undefined };
      const res = await salesCustomerInvoices(p);
      if (my !== seq) return;
      const c = res.customers[0];
      buckets[memberId] = { sales: [...prev, ...(c?.sales ?? [])], next: res.next_cursor, loading: false, error: '' };
    } catch (err) {
      if (my !== seq) return;
      buckets[memberId] = { sales: prev, next: cur?.next, loading: false, error: errorMessage(err) };
    }
  }

  function toggleCustomer(memberId: string) {
    const open = !openCust[memberId];
    openCust[memberId] = open;
    if (open && !buckets[memberId]) void loadInvoices(memberId);
  }

  async function download(format: 'xlsx' | 'pdf') {
    if (!from || !to || exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(salesCustomerInvoicesFileUrl({ ...base(), with_items: withItems }, format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `pelanggan-perfaktur${withItems ? '-detail' : ''}_${from}_${to}.${format}`;
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
    void q;
    void sort;
    void limit;
    return later(() => void load(), 350);
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

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${S}.allOutlets`) : '');
    cashierLabel = cashiers.find((c) => c.id === cashierId)?.name ?? (cashierId === '' ? t(`${S}.allCashiers`) : '');
  });

  const rows = $derived(result?.data ?? []);
  const summary = $derived(result?.summary ?? null);

  onMount(() => {
    document.title = t(`${D}.docTitle`);
    from = ago(29);
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

<main class="p-4 lg:p-6 space-y-4 max-w-[1500px] mx-auto w-full">
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
      <div class="md:col-span-2">
        <span class={lbl}>{t(`${C}.customer`)}</span>
        <input
          type="search"
          class="w-full field-control"
          placeholder={t(`${D}.search`)}
          aria-label={t(`${D}.search`)}
          maxlength="64"
          bind:value={q}
        />
      </div>
      <div>
        <span class={lbl}>{t(`${D}.sortLabel`)}</span>
        <Select bind:value={sort} options={sortOptions} ariaLabel={t(`${D}.sortLabel`)} />
      </div>
      <div>
        <span class={lbl}>Top</span>
        <Select bind:value={limit} options={limitOptions} ariaLabel="Top" />
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
      <span class="inline-flex items-center gap-2 ms-auto">
        <label class="inline-flex items-center gap-1.5 text-[12px] text-[var(--text-secondary)]" title={t(`${D}.pdfItemsHint`)}>
          <input type="checkbox" bind:checked={withItems} />
          {t(`${D}.pdfItems`)}
        </label>
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

  {#if summary && summary.customer_count > 0}
    <section class="grid gap-3 sm:grid-cols-3">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.customerCount`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{formatNumber(summary.customer_count)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.invoiceCount`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{formatNumber(summary.invoice_count)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.revenue`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.total)}</div>
      </div>
    </section>
  {/if}

  <section class="space-y-2 transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    {#each rows as r (r.member_id)}
      {@const open = !!openCust[r.member_id]}
      {@const b = buckets[r.member_id]}
      <div class="surface-card !p-0 overflow-hidden">
        <button
          type="button"
          class="w-full flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-start transition-colors hover:bg-[var(--surface-sunken)]"
          aria-expanded={open}
          onclick={() => toggleCustomer(r.member_id)}
        >
          <i class="icon-chevron-right text-[13px] shrink-0 transition-transform {open ? 'rotate-90' : ''} rtl:rotate-180"></i>
          <span class="min-w-0 flex-1">
            <span class="font-semibold">{r.name}</span>
            <span class="ms-2 text-[12px] text-[var(--text-tertiary)]">{r.code}{r.phone ? ` · ${r.phone}` : ''}</span>
          </span>
          <span class="text-[12px] text-[var(--text-secondary)]">{t(`${D}.invoicesCount`, { n: formatNumber(r.invoice_count) })}</span>
          <span class="w-36 text-end font-display font-bold tabular-nums">{money(r.total)}</span>
        </button>
        {#if open}
          <div class="border-t border-[var(--border-subtle)] px-4 py-3 space-y-2">
            {#if b?.error}
              <p role="alert" class="text-[12.5px] badge-danger rounded px-3 py-2">{b.error}</p>
            {/if}
            {#if !b || (b.loading && b.sales.length === 0)}
              <p class="text-[12.5px] text-[var(--text-tertiary)]">{t(`${D}.loadingInvoices`)}</p>
            {:else if b.sales.length === 0 && !b.error}
              <p class="text-[12.5px] text-[var(--text-tertiary)]">{t(`${D}.noInvoices`)}</p>
            {:else}
              <div class="overflow-x-auto scroll-thin">
                <table class="w-full min-w-[640px] text-[12.5px] border-collapse">
                  <thead>
                    <tr class="text-[10.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
                      <th class="py-1.5 pe-3 text-start font-semibold" scope="col">{t(`${D}.invoice`)}</th>
                      <th class="py-1.5 pe-3 text-start font-semibold" scope="col">{t(`${D}.date`)}</th>
                      <th class="py-1.5 pe-3 text-start font-semibold" scope="col">{t(`${D}.cashier`)}</th>
                      <th class="py-1.5 pe-3 text-end font-semibold" scope="col">{t(`${D}.discount`)}</th>
                      <th class="py-1.5 text-end font-semibold" scope="col">{t(`${D}.total`)}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each b.sales as s (s.id)}
                      {@const iopen = !!openInv[s.id]}
                      <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
                        <td class="py-1.5 pe-3 font-semibold whitespace-nowrap">
                          <button
                            type="button"
                            class="inline-flex items-center gap-1.5 text-start"
                            aria-expanded={iopen}
                            onclick={() => (openInv[s.id] = !iopen)}
                          >
                            <i class="icon-chevron-right text-[11px] transition-transform {iopen ? 'rotate-90' : ''} rtl:rotate-180"></i>
                            {s.doc_no}
                          </button>
                        </td>
                        <td class="py-1.5 pe-3 whitespace-nowrap tabular-nums">
                          {formatDate(s.date)} <span class="text-[var(--text-tertiary)]">{s.time}</span>
                        </td>
                        <td class="py-1.5 pe-3">{s.cashier_name}</td>
                        <td class="py-1.5 pe-3 text-end tabular-nums">{money(s.discount)}</td>
                        <td class="py-1.5 text-end tabular-nums font-semibold">{money(s.total)}</td>
                      </tr>
                      {#if iopen}
                        <tr>
                          <td colspan="5" class="pb-2 ps-6">
                            {#if !s.lines || s.lines.length === 0}
                              <p class="text-[12px] text-[var(--text-tertiary)]">{t(`${D}.noLines`)}</p>
                            {:else}
                              <table class="w-full text-[12px] border-collapse">
                                <thead>
                                  <tr class="text-[10.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
                                    <th class="py-1 pe-3 text-start font-semibold" scope="col">{t(`${D}.sku`)}</th>
                                    <th class="py-1 pe-3 text-start font-semibold" scope="col">{t(`${D}.item`)}</th>
                                    <th class="py-1 pe-3 text-end font-semibold" scope="col">{t(`${D}.qty`)}</th>
                                    <th class="py-1 pe-3 text-start font-semibold" scope="col">{t(`${D}.unit`)}</th>
                                    <th class="py-1 pe-3 text-end font-semibold" scope="col">{t(`${D}.price`)}</th>
                                    <th class="py-1 text-end font-semibold" scope="col">{t(`${D}.lineTotal`)}</th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {#each s.lines as l (l.position)}
                                    <tr class="border-t border-[var(--border-subtle)]">
                                      <td class="py-1 pe-3 whitespace-nowrap text-[var(--text-secondary)]">{l.sku}</td>
                                      <td class="py-1 pe-3">{l.name}</td>
                                      <td class="py-1 pe-3 text-end tabular-nums">{qty(l.qty)}</td>
                                      <td class="py-1 pe-3">{l.unit_name}</td>
                                      <td class="py-1 pe-3 text-end tabular-nums">{money(l.unit_price)}</td>
                                      <td class="py-1 text-end tabular-nums font-medium">{money(l.line_total)}</td>
                                    </tr>
                                  {/each}
                                </tbody>
                              </table>
                            {/if}
                          </td>
                        </tr>
                      {/if}
                    {/each}
                  </tbody>
                </table>
              </div>
              {#if b.next}
                <div class="text-center">
                  <button type="button" class="btn-secondary text-[12.5px] px-4 py-1.5" disabled={b.loading} onclick={() => loadInvoices(r.member_id, true)}>
                    {b.loading ? '…' : t(`${D}.loadMore`)}
                  </button>
                </div>
              {/if}
            {/if}
          </div>
        {/if}
      </div>
    {:else}
      <div class="surface-card px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">
        {loading ? '…' : t(`${D}.noData`)}
      </div>
    {/each}
    {#if result?.truncated}
      <p class="text-center text-[12px] text-[var(--text-tertiary)]">
        {t(`${D}.shown`, { shown: formatNumber(rows.length), total: formatNumber(result.summary.customer_count) })}
      </p>
    {/if}
  </section>
</main>
