<script lang="ts">
  // Laporan retur (penjualan & pembelian) — per faktur dan per faktur detail: satu komponen, empat rute
  // pembungkus. Per faktur = satu baris per dokumen retur; detail = kartu per dokumen (kop retur + baris
  // barang). Paginasi keyset ("Muat Lagi"); ringkasan & rasio retur hanya di halaman pertama.
  import { onMount } from 'svelte';
  import { later } from '#lib/debounce.svelte.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { members } from '#lib/members/api.ts';
  import {
    returnReport,
    returnReportFileUrl,
    type ReturnKind,
    type ReturnParams,
    type ReturnRow,
    type ReturnSummary,
    type ReturnView
  } from '#lib/reports/returns/api.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';

  let { kind, view }: { kind: ReturnKind; view: ReturnView } = $props();

  const P = 'reports.returnReport';
  const C = `${P}.c`;
  const sale = $derived(kind === 'sale');
  const fileBase = $derived(`${sale ? 'retur-penjualan' : 'retur-pembelian'}-${view === 'detail' ? 'perfaktur-detail' : 'perfaktur'}`);

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let partyId = $state('');
  let partyLabel = $state('');
  let status = $state('');
  let settle = $state('');
  let q = $state('');

  let loading = $state(true);
  let loadingMore = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let rows = $state<ReturnRow[]>([]);
  let summary = $state<ReturnSummary | null>(null);
  let nextCursor = $state<string | undefined>(undefined);
  // Token permintaan: respons lama (filter sudah berganti) dibuang.
  let seq = 0;

  const money = (v: string | number) => formatCurrency(Number(v));
  const qtyFmt = (v: string) => formatNumber(Number(v), { maximumFractionDigits: 3 });
  const pct = (v: string) => `${formatNumber(Number(v), { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`;

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
  const statusOptions = $derived([
    { value: '', label: t(`${C}.allStatuses`) },
    { value: 'completed', label: t(`${C}.completed`) },
    { value: 'void', label: t(`${C}.void`) }
  ]);
  const settleOptions = $derived([
    { value: '', label: t(`${C}.allSettles`) },
    { value: 'offset', label: t(sale ? `${C}.settleOffsetSale` : `${C}.settleOffsetPurchase`) },
    { value: 'refund', label: t(`${C}.settleRefund`) }
  ]);

  const params = (): ReturnParams => ({
    from,
    to,
    outlet_id: outletId || undefined,
    party_id: partyId || undefined,
    status: status || undefined,
    settle: settle || undefined,
    q: q.trim() || undefined,
    all_outlets: outletId === ''
  });

  async function load() {
    if (!from || !to) return;
    const my = ++seq;
    loading = true;
    error = '';
    nextCursor = undefined;
    try {
      const res = await returnReport(kind, view, params());
      if (my !== seq) return;
      rows = res.data;
      summary = res.has_summary ? res.summary : null;
      nextCursor = res.next_cursor;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      rows = [];
      summary = null;
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
      const res = await returnReport(kind, view, { ...params(), cursor: nextCursor });
      if (my !== seq) return;
      rows = [...rows, ...res.data];
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
      const blob = await apiBlob(returnReportFileUrl(kind, view, params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${fileBase}_${from}_${to}.${format}`;
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
    void partyId;
    void status;
    void settle;
    void q;
    return later(() => void load(), 350);
  });

  const searchOutlets = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${C}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };
  const searchParty = async (term: string): Promise<Option[]> => [
    { id: '', name: t(sale ? `${C}.allMembers` : `${C}.allSuppliers`) },
    ...(sale
      ? (await members.list({ q: term, limit: 20 })).data.map((m) => ({ id: m.id, name: m.name }))
      : await lookup('suppliers').search(term))
  ];

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${C}.allOutlets`) : '');
    if (partyId === '') partyLabel = t(sale ? `${C}.allMembers` : `${C}.allSuppliers`);
  });

  onMount(() => {
    document.title = t(`${P}.${kind === 'sale' ? 'sale' : 'purchase'}.${view === 'detail' ? 'detail' : 'invoice'}.docTitle`);
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
  const colCount = $derived(view === 'invoice' ? (sale ? 14 : 11) : sale ? 8 : 6);
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${P}.${sale ? 'sale' : 'purchase'}.${view === 'detail' ? 'detail' : 'invoice'}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${P}.${sale ? 'sale' : 'purchase'}.${view === 'detail' ? 'detail' : 'invoice'}.subtitle`)}</p>
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
        <span class={lbl}>{t(sale ? `${C}.member` : `${C}.supplier`)}</span>
        <Combobox bind:value={partyId} bind:label={partyLabel} search={searchParty} placeholder={t(sale ? `${C}.member` : `${C}.supplier`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${C}.status`)}</span>
        <Select bind:value={status} options={statusOptions} ariaLabel={t(`${C}.status`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${C}.settle`)}</span>
        <Select bind:value={settle} options={settleOptions} ariaLabel={t(`${C}.settle`)} />
      </div>
      <div class="md:col-span-2">
        <span class={lbl}>{t(`${C}.returnNo`)}</span>
        <input
          type="search"
          class="w-full field-control"
          placeholder={t(sale ? `${C}.searchSale` : `${C}.searchPurchase`)}
          aria-label={t(sale ? `${C}.searchSale` : `${C}.searchPurchase`)}
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

  {#if summary && summary.return_count + summary.void_count > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.returnCount`)}</div>
        <div class={big}>{formatNumber(summary.return_count)}</div>
        {#if summary.void_count > 0}
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${C}.voidCount`)}: {formatNumber(summary.void_count)}</div>
        {/if}
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.total`)}</div>
        <div class={big}>{money(summary.total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(sale ? `${C}.settleOffsetSale` : `${C}.settleOffsetPurchase`)}</div>
        <div class={big}>{money(summary.offset)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.settleRefund`)}</div>
        <div class={big}>{money(summary.refund)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${C}.ratio`)}</div>
        <div class={big}>{pct(summary.ratio_pct)}</div>
        <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">
          {t(sale ? `${C}.ratioSale` : `${C}.ratioPurchase`, { amount: money(summary.base_total) })}
        </div>
      </div>
    </section>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full {view === 'invoice' ? 'min-w-[1300px]' : 'min-w-[820px]'} text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            {#if view === 'invoice'}
              <th class={thS} scope="col">{t(`${C}.date`)}</th>
              <th class={thS} scope="col">{t(`${C}.returnNo`)}</th>
              <th class={thS} scope="col">{t(sale ? `${C}.refSale` : `${C}.refPurchase`)}</th>
              <th class={thS} scope="col">{t(sale ? `${C}.member` : `${C}.supplier`)}</th>
              {#if sale}<th class={thS} scope="col">{t(`${C}.cashier`)}</th>{/if}
              <th class={thE} scope="col">{t(`${C}.subtotal`)}</th>
              {#if sale}<th class={thE} scope="col">{t(`${C}.discount`)}</th>{/if}
              <th class={thE} scope="col">{t(`${C}.tax`)}</th>
              {#if sale}<th class={thE} scope="col">{t(`${C}.surcharge`)}</th>{/if}
              <th class={thE} scope="col">{t(`${C}.total`)}</th>
              <th class={thE} scope="col">{t(sale ? `${C}.settleOffsetSale` : `${C}.settleOffsetPurchase`)}</th>
              <th class={thE} scope="col">{t(`${C}.refund`)}</th>
              <th class={thS} scope="col">{t(`${C}.method`)}</th>
            {:else}
              <th class={thS} scope="col">{t(`${C}.sku`)}</th>
              <th class={thS} scope="col">{t(`${C}.item`)}</th>
              <th class={thS} scope="col">{t(`${C}.unit`)}</th>
              <th class={thE} scope="col">{t(`${C}.qty`)}</th>
              {#if sale}
                <th class={thE} scope="col">{t(`${C}.value`)}</th>
                <th class={thE} scope="col">{t(`${C}.discount`)}</th>
                <th class={thE} scope="col">{t(`${C}.tax`)}</th>
              {:else}
                <th class={thE} scope="col">{t(`${C}.price`)}</th>
              {/if}
              <th class={thE} scope="col">{t(`${C}.lineTotal`)}</th>
            {/if}
          </tr>
        </thead>
        <tbody>
          {#if view === 'invoice'}
            {#each rows as r (r.id)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                <td class="px-3 py-2 whitespace-nowrap tabular-nums">{formatDate(r.date)}</td>
                <td class="px-3 py-2 font-semibold whitespace-nowrap">
                  {r.doc_no}
                  {#if r.status === 'void'}<span class={voidBadge} title={r.void_reason}>{t(`${C}.void`)}</span>{/if}
                </td>
                <td class="px-3 py-2 whitespace-nowrap">{r.ref_doc_no}</td>
                <td class="px-3 py-2">{r.party_name}</td>
                {#if sale}<td class="px-3 py-2">{r.user_name}</td>{/if}
                <td class="px-3 py-2 text-end tabular-nums">{money(r.subtotal)}</td>
                {#if sale}<td class="px-3 py-2 text-end tabular-nums">{money(r.discount)}</td>{/if}
                <td class="px-3 py-2 text-end tabular-nums">{money(r.tax_amount)}</td>
                {#if sale}<td class="px-3 py-2 text-end tabular-nums">{money(r.surcharge)}</td>{/if}
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.total)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.offset)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.refund)}</td>
                <td class="px-3 py-2 whitespace-nowrap">{r.refund_method}</td>
              </tr>
            {:else}
              <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {:else}
            {#each rows as r (r.id)}
              <!-- Kop dokumen retur: identitas di kiri, rincian penyelesaian di kanan. -->
              <tr class="border-t-2 border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
                <td colspan={colCount} class="px-3 py-2.5">
                  <div class="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
                    <div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
                      <span class="font-semibold">{r.doc_no}</span>
                      {#if r.status === 'void'}<span class={voidBadge} title={r.void_reason}>{t(`${C}.void`)}</span>{/if}
                      <span class="tabular-nums text-[var(--text-secondary)]">{formatDate(r.date)}</span>
                      <span class="text-[var(--text-secondary)]">{t(sale ? `${C}.refSale` : `${C}.refPurchase`)}: <b class="font-semibold text-[var(--text-primary)]">{r.ref_doc_no}</b></span>
                      {#if r.party_name}<span class="text-[var(--text-secondary)]">{r.party_name}</span>{/if}
                      {#if r.user_name}<span class="text-[var(--text-tertiary)]">· {r.user_name}</span>{/if}
                    </div>
                    <div class="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 tabular-nums">
                      {#if Number(r.offset) > 0}
                        <span class="text-[var(--text-secondary)]">{t(sale ? `${C}.settleOffsetSale` : `${C}.settleOffsetPurchase`)}: {money(r.offset)}</span>
                      {/if}
                      {#if Number(r.refund) > 0}
                        <span class="text-[var(--text-secondary)]">{t(`${C}.refund`)}: {money(r.refund)}{r.refund_method ? ` (${r.refund_method})` : ''}</span>
                      {/if}
                      <span class="font-semibold">{t(`${C}.total`)}: {money(r.total)}</span>
                    </div>
                  </div>
                  {#if r.note}<div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{r.note}</div>{/if}
                  {#if r.status === 'void' && r.void_reason}
                    <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${C}.voidReason`)}: {r.void_reason}</div>
                  {/if}
                </td>
              </tr>
              {#each r.lines ?? [] as l (l.position)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="ps-6 pe-3 py-2 whitespace-nowrap">{l.sku}</td>
                  <td class="px-3 py-2">{l.name}</td>
                  <td class="px-3 py-2">{l.unit}</td>
                  <td class="px-3 py-2 text-end tabular-nums">{qtyFmt(l.qty)}</td>
                  {#if sale}
                    <td class="px-3 py-2 text-end tabular-nums">{money(l.value)}</td>
                    <td class="px-3 py-2 text-end tabular-nums">{money(l.discount)}</td>
                    <td class="px-3 py-2 text-end tabular-nums">{money(l.tax_amount)}</td>
                  {:else}
                    <td class="px-3 py-2 text-end tabular-nums">{money(l.unit_price)}</td>
                  {/if}
                  <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(l.net)}</td>
                </tr>
              {/each}
            {:else}
              <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
            {/each}
          {/if}
        </tbody>
        {#if view === 'invoice' && summary && rows.length > 0}
          <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
            <tr class="border-t-2 border-[var(--border-subtle)]">
              <td class="px-3 py-2.5" colspan={sale ? 5 : 4}>{t(`${C}.grandTotal`)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(summary.subtotal)}</td>
              {#if sale}<td class="px-3 py-2.5 text-end tabular-nums">{money(summary.discount)}</td>{/if}
              <td class="px-3 py-2.5 text-end tabular-nums">{money(summary.tax_amount)}</td>
              {#if sale}<td class="px-3 py-2.5 text-end tabular-nums">{money(summary.surcharge)}</td>{/if}
              <td class="px-3 py-2.5 text-end tabular-nums">{money(summary.total)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(summary.offset)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums">{money(summary.refund)}</td>
              <td></td>
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
