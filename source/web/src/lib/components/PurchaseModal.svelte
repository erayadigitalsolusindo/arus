<script lang="ts">
  // Rincian satu faktur pembelian: header, baris (qty per bucket, diskon, HPP baris dan HPP rata-rata sesudah), biaya lain, total, hutang.
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { purchases, type Purchase } from '#lib/purchases/api.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { id, onclose }: { id: string; onclose: () => void } = $props();

  let p = $state<Purchase | null>(null);
  let error = $state('');
  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { maximumFractionDigits: 2 });
  const qty = (v: string) => formatNumber(Number(v), { maximumFractionDigits: 3 });

  onMount(async () => {
    try {
      p = await purchases.get(id);
    } catch (e) {
      error = errorMessage(e);
    }
  });

  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<Modal title={p ? t('purchases.modal.title', { doc: p.doc_no }) : t('purchases.detail')} {onclose} wide>
  {#if error}
    <div role="alert" class="rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">{error}</div>
  {:else if !p}
    <p class="py-10 text-center text-[var(--text-tertiary)]">…</p>
  {:else}
    <div class="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-4 text-[13px]">
      <div><div class={label}>{t('purchases.modal.supplier')}</div><div class="font-medium">{p.supplier_name}</div></div>
      <div><div class={label}>{t('purchases.modal.invoice')}</div><div class="font-mono">{p.supplier_invoice_no || '—'}</div></div>
      <div><div class={label}>{t('purchases.modal.date')}</div><div>{formatDate(p.purchase_date)}</div></div>
      <div>
        <div class={label}>{t('purchases.col.type')}</div>
        <span class="badge-soft {p.payment_type === 'credit' ? 'badge-warning' : 'badge-success'}">{t(`purchases.type.${p.payment_type}`)}</span>
      </div>
      <div><div class={label}>{t('purchases.modal.outlet')}</div><div>{p.outlet_name}</div></div>
      <div><div class={label}>{t('purchases.modal.by')}</div><div>{p.created_by || '—'}</div></div>
      <div><div class={label}>{t('purchases.col.date')}</div><div>{formatDateTime(p.created_at)}</div></div>
      {#if p.payment_type === 'credit'}
        <div><div class={label}>{t('purchases.modal.due')}</div><div>{p.due_date ? formatDate(p.due_date) : t('purchases.modal.noDue')}</div></div>
      {/if}
    </div>

    <div class="mt-4 overflow-x-auto scroll-thin rounded-lg border border-[var(--border-subtle)]">
      <table class="w-full min-w-[760px] text-[12.5px]">
        <thead>
          <tr class="bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-3 py-2 text-start" scope="col">{t('purchases.lineCol.item')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.display')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.warehouse')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.price')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.discount')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.total')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.cost')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('purchases.lineCol.avg')}</th>
          </tr>
        </thead>
        <tbody>
          {#each p.lines as l, i (i)}
            <tr class="border-t border-[var(--border-subtle)] align-top">
              <td class="px-3 py-2"><div class="font-medium">{l.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{l.sku} · {l.unit}</div></td>
              <td class="px-3 py-2 text-end tabular-nums">{qty(l.qty_display)}</td>
              <td class="px-3 py-2 text-end tabular-nums">{qty(l.qty_warehouse)}</td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{money(l.unit_price)}</td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{l.discounts.length ? l.discounts.map((d) => `${formatNumber(Number(d))}%`).join(' + ') : '—'}</td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap font-medium">{money(l.line_total)}</td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{money(l.unit_cost)}</td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">
                <span class="text-[var(--text-tertiary)]">{money(l.avg_before)} →</span> <span class="font-semibold">{money(l.avg_after)}</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="mt-4 grid gap-4 sm:grid-cols-2">
      <div class="space-y-3 text-[13px]">
        {#if p.costs.length}
          <div>
            <div class={label}>{t('purchases.modal.costs')}</div>
            <ul class="mt-1 space-y-0.5">
              {#each p.costs as c, i (i)}<li class="flex justify-between gap-3"><span>{c.name || '—'}</span><span class="tabular-nums">{money(c.amount)}</span></li>{/each}
            </ul>
          </div>
        {/if}
        {#if p.note}
          <div><div class={label}>{t('purchases.modal.note')}</div><p class="whitespace-pre-wrap">{p.note}</p></div>
        {/if}
        {#if p.payable}
          <div class="rounded-lg bg-[var(--surface-sunken)] p-3">
            <div class={label}>{t('purchases.modal.payable')}</div>
            <div class="font-display text-[16px] font-bold tabular-nums">{money(p.payable.amount)}</div>
            <p class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('purchases.modal.payableHint')}</p>
          </div>
        {/if}
      </div>
      <dl class="space-y-1 text-[13px]">
        <div class="flex justify-between gap-3"><dt>{t('purchases.totals.subtotal')}</dt><dd class="tabular-nums">{money(p.subtotal)}</dd></div>
        {#if Number(p.tax_amount) > 0}
          <div class="flex justify-between gap-3"><dt>{t('purchases.totals.tax', { pct: formatNumber(Number(p.tax_pct)) })}</dt><dd class="tabular-nums">{money(p.tax_amount)}</dd></div>
        {/if}
        {#if Number(p.other_cost) > 0}
          <div class="flex justify-between gap-3"><dt>{t('purchases.totals.otherCosts')}</dt><dd class="tabular-nums">{money(p.other_cost)}</dd></div>
        {/if}
        <div class="flex justify-between gap-3 border-t border-[var(--border-subtle)] pt-2 text-[15px] font-bold"><dt>{t('purchases.totals.total')}</dt><dd class="tabular-nums">{money(p.total)}</dd></div>
      </dl>
    </div>

    <div class="mt-5 flex justify-end">
      <button type="button" class="btn" onclick={onclose}>{t('purchases.modal.close')}</button>
    </div>
  {/if}
</Modal>
