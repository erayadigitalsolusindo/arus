<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import { sales, type SaleList, type Sale } from '#lib/sales/api.ts';
  import { t, formatCurrency, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { loadReceiptSettings, printErrorMessage, printLines, printReceipt } from '#lib/pos/receipt.ts';
  import { dailyRecapLines } from '#lib/pos/report-receipts.ts';
  import { session } from '#lib/auth/session.svelte.ts';

  let { onclose }: { onclose: () => void } = $props();

  let q = $state('');
  let from = $state('');
  let to = $state('');
  let size = $state('100');
  let page = $state(0);
  let data = $state<SaleList>();
  let loading = $state(true);
  let error = $state('');
  let open = $state<Record<string, Sale | 'loading' | undefined>>({});
  let seq = 0;
  let timer: ReturnType<typeof setTimeout>;

  // Tanggal kosong = hari ini menurut zona waktu outlet (server yang menentukan); balasannya mengisi kolom tanggal.
  async function load() {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await sales.list({ from, to, q: q.trim() });
      if (mine !== seq) return;
      data = res;
      from = res.from;
      to = res.to;
      page = 0;
    } catch (err) {
      if (mine === seq) error = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }
  onMount(load);

  function onSearch() {
    clearTimeout(timer);
    timer = setTimeout(load, 300);
  }

  // Cetak ulang dari daftar: selalu tercatat sebagai salinan (audit sale.reprint).
  let printingId = $state('');
  async function reprint(id: string) {
    if (printingId) return;
    printingId = id;
    error = '';
    try {
      await printReceipt(id, { reprint: true });
    } catch (err) {
      error = printErrorMessage(err);
    } finally {
      printingId = '';
    }
  }

  // Cetak rekap rentang yang dipilih (per kasir yang login). Pencarian no. nota diabaikan: rekap selalu seluruh rentang.
  let printingRecap = $state(false);
  async function printRecap() {
    if (printingRecap || !data) return;
    printingRecap = true;
    error = '';
    try {
      const list = q.trim() ? await sales.list({ from: data.from, to: data.to }) : data;
      const settings = loadReceiptSettings();
      const meta = { tenant: session.tenant?.name ?? '', outlet: session.outlet?.name ?? '', cashier: session.user?.name ?? '' };
      await printLines(dailyRecapLines(list, meta, settings.paper), settings);
    } catch (err) {
      error = printErrorMessage(err);
    } finally {
      printingRecap = false;
    }
  }

  async function toggle(id: string) {
    if (open[id]) {
      open[id] = undefined;
      return;
    }
    open[id] = 'loading';
    try {
      open[id] = await sales.get(id);
    } catch (err) {
      open[id] = undefined;
      error = errorMessage(err);
    }
  }

  const money = (s?: string) => formatCurrency(Number(s ?? 0));
  const rows = $derived(data?.data ?? []);
  const per = $derived(Number(size));
  const pages = $derived(Math.max(1, Math.ceil(rows.length / per)));
  const shown = $derived(rows.slice(page * per, page * per + per));
  const methodLabel = (m: string) => t(`pos.today.sum.${m}` as 'pos.today.sum.cash');
  const title = $derived(
    data && data.from !== data.to
      ? t('pos.today.titleRange', { from: data.from, to: data.to, total: money(data.total) })
      : t('pos.today.title', { total: money(data?.total) })
  );
  const dt = (iso: string) => formatDateTime(iso, { dateStyle: 'short', timeStyle: 'medium' });
</script>

<Modal {title} wide {onclose}>
  <div class="space-y-3 text-[13px]">
    <input
      type="search"
      bind:value={q}
      oninput={onSearch}
      placeholder={t('pos.today.search')}
      aria-label={t('pos.today.search')}
      class="w-full h-10 px-3 rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
    />
    <div class="flex items-center gap-2">
      <DateRange bind:from bind:to onchange={load} ariaLabel={t('pos.today.range')} class="grow" />
      <button type="button" class="btn btn-sm btn-primary shrink-0 h-10" disabled={!data || loading || printingRecap} title={t('pos.today.printHint')} onclick={printRecap}>
        <i class={printingRecap ? 'icon-loader-circle animate-spin' : 'icon-printer'}></i> {t('pos.today.print')}
      </button>
    </div>

    <div class="flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
      {t('pos.today.show')}
      <div class="w-20">
        <Select ariaLabel={t('pos.today.show')} bind:value={size} onchange={() => (page = 0)} options={['10', '25', '50', '100'].map((v) => ({ value: v, label: v }))} />
      </div>
      {t('pos.today.entries')}
    </div>

    {#if error}<p class="text-[var(--color-danger-600)]" role="alert">{error}</p>{/if}

    <div class="rounded-md border border-[var(--border-subtle)] overflow-x-auto">
      <table class="w-full text-[12.5px]">
        <thead>
          <tr class="bg-[var(--surface-sunken)] text-[11.5px] font-bold">
            <th class="px-3 py-2 text-center">{t('pos.today.col.action')}</th>
            <th class="px-3 py-2 text-center">{t('pos.today.col.docNo')}</th>
            <th class="px-3 py-2 text-end">{t('pos.today.col.total')}</th>
            <th class="px-3 py-2 text-center">{t('pos.today.col.time')}</th>
            <th class="px-3 py-2 text-center">{t('pos.today.col.paidTime')}</th>
            <th class="px-3 py-2 text-center">{t('pos.today.col.method')}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[var(--border-subtle)]">
          {#each shown as r (r.id)}
            {@const d = open[r.id]}
            <tr>
              <td class="px-3 py-1.5 text-center">
                <button type="button" class="header-icon-btn" aria-label={d ? t('pos.today.hideDetail') : t('pos.today.detail')} title={d ? t('pos.today.hideDetail') : t('pos.today.detail')} onclick={() => toggle(r.id)}>
                  <i class="{d ? 'icon-chevron-up' : 'icon-eye'} text-[15px]"></i>
                </button>
                <button type="button" class="header-icon-btn disabled:opacity-50" aria-label={t('pos.receipt.reprint')} title={t('pos.receipt.reprint')} disabled={!!printingId} onclick={() => reprint(r.id)}>
                  <i class="{printingId === r.id ? 'icon-loader-circle animate-spin' : 'icon-printer'} text-[15px]"></i>
                </button>
              </td>
              <td class="px-3 py-1.5 text-center font-semibold whitespace-nowrap">{r.doc_no}</td>
              <td class="px-3 py-1.5 text-end tabular-nums whitespace-nowrap">{money(r.total)}{#if Number(r.surcharge)}<span class="block text-[10.5px] text-[var(--text-tertiary)]">{t('pos.today.surchargeBadge', { amount: money(r.surcharge) })}</span>{/if}{#if Number(r.returned)}<span class="block text-[10.5px] font-medium text-[var(--color-warning-600,#d97706)]">{t('pos.today.returnedBadge', { amount: money(r.returned) })}</span>{/if}</td>
              <td class="px-3 py-1.5 text-center tabular-nums whitespace-nowrap">{dt(r.created_at)}</td>
              <td class="px-3 py-1.5 text-center tabular-nums whitespace-nowrap">{dt(r.created_at)}</td>
              <td class="px-3 py-1.5 text-center">{[...r.pays.map((p) => p.name), ...(Number(r.receivable) > 0 ? [methodLabel('credit')] : [])].join(', ')}</td>
            </tr>
            {#if d}
              <tr class="bg-[var(--surface-sunken)]">
                <td colspan="6" class="px-3 py-3 text-[12px]">
                  {#if d === 'loading'}
                    …
                  {:else}
                    <div class="mb-1.5 text-[var(--text-secondary)]">{d.cashier}{#if d.member} · {d.member.name}{/if}</div>
                    <div class="rounded-md border border-[var(--border-subtle)] bg-[var(--surface-card)] overflow-x-auto">
                      <table class="w-full text-[12px]">
                        <thead>
                          <tr class="bg-[var(--surface-sunken)] text-[11px] font-bold">
                            <th class="px-2.5 py-1.5 text-start">{t('pos.today.detailCol.item')}</th>
                            <th class="px-2.5 py-1.5 text-end">{t('pos.today.detailCol.qty')}</th>
                            <th class="px-2.5 py-1.5 text-end">{t('pos.today.detailCol.price')}</th>
                            <th class="px-2.5 py-1.5 text-end">{t('pos.today.detailCol.discount')}</th>
                            <th class="px-2.5 py-1.5 text-end">{t('pos.today.detailCol.subtotal')}</th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-[var(--border-subtle)]">
                          {#each d.lines as l}
                            <tr>
                              <td class="px-2.5 py-1.5"><span class="block font-medium">{l.name}</span><span class="block text-[10.5px] text-[var(--text-tertiary)]">{l.sku}</span></td>
                              <td class="px-2.5 py-1.5 text-end tabular-nums whitespace-nowrap">{Number(l.qty)} {l.unit}</td>
                              <td class="px-2.5 py-1.5 text-end tabular-nums whitespace-nowrap">{money(l.unit_price)}</td>
                              <td class="px-2.5 py-1.5 text-end tabular-nums whitespace-nowrap">{Number(l.discount) ? money(l.discount) : "-"}</td>
                              <td class="px-2.5 py-1.5 text-end tabular-nums whitespace-nowrap font-semibold">{money(l.line_total)}</td>
                            </tr>
                          {/each}
                        </tbody>
                        <tfoot class="border-t border-[var(--border-default)] text-[12px]">
                          {#each [
                            ['subtotal', d.subtotal, true],
                            ['discount', d.discount, Number(d.discount) > 0],
                            ['taxStore', d.tax_store, Number(d.tax_store) > 0],
                            ['taxGov', d.tax_gov, Number(d.tax_gov) > 0],
                            ['otherCost', d.other_cost, Number(d.other_cost) > 0]
                          ] as [k, v, show]}
                            {#if show}
                              <tr><td colspan="4" class="px-2.5 py-1 text-end text-[var(--text-secondary)]">{t(`pos.today.sumRow.${k}` as 'pos.today.sumRow.subtotal')}</td><td class="px-2.5 py-1 text-end tabular-nums">{money(v as string)}</td></tr>
                            {/if}
                          {/each}
                          <tr class="font-bold"><td colspan="4" class="px-2.5 py-1.5 text-end">{t('pos.today.sumRow.total')}</td><td class="px-2.5 py-1.5 text-end tabular-nums">{money(d.total)}</td></tr>
                          {#each d.payments as p}
                            <tr><td colspan="4" class="px-2.5 py-1 text-end text-[var(--text-secondary)]">{p.method_name || methodLabel(p.method)}{#if p.ref_no} · {p.ref_no}{/if}</td><td class="px-2.5 py-1 text-end tabular-nums">{money(p.amount)}</td></tr>
                          {/each}
                          {#if Number(d.change)}
                            <tr><td colspan="4" class="px-2.5 py-1 text-end text-[var(--text-secondary)]">{t('pos.today.sumRow.change')}</td><td class="px-2.5 py-1 text-end tabular-nums">{money(d.change)}</td></tr>
                          {/if}
                        </tfoot>
                      </table>
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {:else}
            <tr><td colspan="6" class="px-3 py-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('pos.today.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between gap-2 text-[12px] text-[var(--text-secondary)]">
      <span>
        {rows.length ? t('pos.today.showing', { from: page * per + 1, to: Math.min(rows.length, page * per + per), total: rows.length }) : t('pos.today.showingNone')}
      </span>
      <span class="flex gap-1">
        <button type="button" class="btn btn-sm" disabled={page === 0} onclick={() => page--}>{t('pos.today.prev')}</button>
        <button type="button" class="btn btn-sm" disabled={page + 1 >= pages} onclick={() => page++}>{t('pos.today.next')}</button>
      </span>
    </div>
    {#if data?.truncated}<p class="text-[11.5px] text-[var(--text-tertiary)]">{t('pos.today.truncated')}</p>{/if}

    <div class="pt-1">
      <table class="w-full text-[13px] tabular-nums rounded-md border border-[var(--border-subtle)] overflow-hidden">
        <tbody class="divide-y divide-[var(--border-subtle)]">
          {#each data?.by_method ?? [] as m (m.method_id)}
            <tr>
              <th scope="row" class="px-3 py-1.5 text-start font-semibold">{m.name}</th>
              <td class="px-3 py-1.5 text-end">{money(m.amount)}</td>
            </tr>
          {/each}
          <tr>
            <th scope="row" class="px-3 py-1.5 text-start font-semibold">{methodLabel('credit')}</th>
            <td class="px-3 py-1.5 text-end">{money(data?.totals['credit' as keyof typeof data.totals])}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr class="bg-[var(--surface-sunken)] font-bold border-t border-[var(--border-default)]">
            <th scope="row" class="px-3 py-1.5 text-start">{t('pos.today.sumRow.total')}</th>
            <td class="px-3 py-1.5 text-end">{money(data?.total)}</td>
          </tr>
          {#if data?.returns && data.returns.count > 0}
            <tr class="text-[var(--color-warning-600,#d97706)]">
              <th scope="row" class="px-3 py-1.5 text-start font-normal">{t('pos.today.returnsRow', { count: data.returns.count })}</th>
              <td class="px-3 py-1.5 text-end">−{money(data.returns.total)}</td>
            </tr>
            <tr class="bg-[var(--surface-sunken)] font-bold">
              <th scope="row" class="px-3 py-1.5 text-start">{t('pos.today.netTotal')}</th>
              <td class="px-3 py-1.5 text-end">{money(data.net_total)}</td>
            </tr>
          {/if}
          {#if Number(data?.surcharge)}
            <tr class="text-[var(--text-secondary)]">
              <th scope="row" class="px-3 py-1.5 text-start font-normal">{t('pos.today.surcharge')}</th>
              <td class="px-3 py-1.5 text-end">{money(data?.surcharge)}</td>
            </tr>
            <tr class="bg-[var(--surface-sunken)] font-bold">
              <th scope="row" class="px-3 py-1.5 text-start">{t('pos.today.received')}</th>
              <td class="px-3 py-1.5 text-end">{money(data?.received)}</td>
            </tr>
          {/if}
        </tfoot>
      </table>
    </div>

    {#if (data?.flows ?? []).length > 0}
      <div class="pt-1">
        <h3 class="mb-1 text-[12px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('pos.today.flows.title')}</h3>
        <table class="w-full text-[13px] tabular-nums rounded-md border border-[var(--border-subtle)] overflow-hidden">
          <tbody class="divide-y divide-[var(--border-subtle)]">
            {#each data?.flows ?? [] as f (f.source + f.method_id)}
              <tr>
                <th scope="row" class="px-3 py-1.5 text-start font-normal">{t(`pos.today.flows.${f.source}` as 'pos.today.flows.sale_return')} · {f.name} <span class="text-[var(--text-tertiary)]">({f.count})</span></th>
                <td class="px-3 py-1.5 text-end {Number(f.amount) < 0 ? 'text-[var(--color-danger-600)]' : ''}">{money(f.amount)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
    {#if (data?.drawer ?? []).length > 0}
      <div class="pt-1">
        <h3 class="mb-1 text-[12px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('pos.today.drawer')}</h3>
        <table class="w-full text-[13px] tabular-nums rounded-md border border-[var(--border-default)] overflow-hidden">
          <tbody class="divide-y divide-[var(--border-subtle)]">
            {#each data?.drawer ?? [] as m (m.method_id)}
              <tr class="font-semibold">
                <th scope="row" class="px-3 py-1.5 text-start">{m.name}</th>
                <td class="px-3 py-1.5 text-end">{money(m.amount)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('pos.today.drawerHint')}</p>
      </div>
    {/if}
  </div>
</Modal>
