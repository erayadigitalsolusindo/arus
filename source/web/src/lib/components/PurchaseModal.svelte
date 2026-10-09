<script lang="ts">
  // Rincian satu faktur pembelian: header, baris (qty per bucket, diskon, HPP baris dan HPP rata-rata sesudah), biaya lain, total, hutang.
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { purchases, type Purchase } from '#lib/purchases/api.ts';
  import { t, tryT, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { can } from '#lib/auth/session.svelte.ts';

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

  const canUpdate = $derived(can('purchase_invoices', 'update'));
  const canDelete = $derived(can('purchase_invoices', 'delete'));
  let voiding = $state(false);
  let voidReason = $state('');
  let voidBusy = $state(false);
  let voidError = $state('');

  // Pembatalan: nota tetap tercatat (status void); tampilan langsung diganti dengan respons server.
  async function doVoid(e: SubmitEvent) {
    e.preventDefault();
    if (voidReason.trim().length < 3) return;
    voidBusy = true;
    voidError = '';
    try {
      p = await purchases.void(id, voidReason.trim());
      voiding = false;
      voidReason = '';
    } catch (err) {
      voidError = errorMessage(err);
    } finally {
      voidBusy = false;
    }
  }

  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;
</script>

<Modal title={p ? t('purchases.modal.title', { doc: p.doc_no }) : t('purchases.detail')} {onclose} wide>
  {#if error}
    <div role="alert" class="rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">{error}</div>
  {:else if !p}
    <p class="py-10 text-center text-[var(--text-tertiary)]">…</p>
  {:else}
    {#if p.status === 'void'}
      <div role="status" class="mb-3 flex items-start gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger">
        <i class="icon-ban text-[14px] shrink-0 mt-0.5"></i>
        <span>{t('purchases.modal.voidedBanner', { at: formatDateTime(p.voided_at ?? p.created_at), reason: p.void_reason })}</span>
      </div>
    {:else if p.status === 'superseded'}
      <div role="status" class="mb-3 flex items-start gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-warning">
        <i class="icon-history text-[14px] shrink-0 mt-0.5"></i>
        <span>{t('purchases.modal.supersededBanner')}</span>
      </div>
    {/if}
    {#if p.revision > 1}
      <div class="mb-3 flex items-start gap-2 rounded-lg bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px]">
        <i class="icon-git-branch text-[14px] shrink-0 mt-0.5 text-[var(--text-tertiary)]"></i>
        <span>{t('purchases.modal.revisionBanner', { n: p.revision, reason: p.revision_reason })}</span>
      </div>
    {/if}
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
            {#if Number(p.payable.returned) > 0}
              <p class="mt-1 text-[11.5px] text-[var(--text-secondary)]">{t('purchases.modal.payableReturned', { amount: money(p.payable.returned), balance: money(p.payable.balance) })}</p>
            {/if}
          </div>
        {/if}
        {#if p.returns.length}
          <div>
            <div class={label}>{t('purchases.modal.returns')}</div>
            <ul class="mt-1 space-y-1 text-[12.5px]">
              {#each p.returns as r (r.id)}
                <li class="flex items-center gap-2">
                  <a href="/purchase-returns?open={r.id}" class="font-mono font-semibold hover:underline {r.status === 'void' ? 'line-through opacity-60' : ''}">{r.doc_no}</a>
                  <span class="text-[var(--text-tertiary)]">{formatDateTime(r.created_at)}</span>
                  <span class="ms-auto tabular-nums {r.status === 'void' ? 'line-through opacity-60' : ''}">{money(r.total)}</span>
                </li>
              {/each}
            </ul>
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

    <section class="mt-5">
      <h3 class={label}>{t('purchases.modal.history')}</h3>
      <p class="mt-1 text-[12px] text-[var(--text-tertiary)]">{t('purchases.modal.historyIntro')}</p>
      {#if p.events.length === 0}
        <p class="mt-3 rounded-lg border border-dashed border-[var(--border-subtle)] p-4 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('purchases.modal.historyEmpty')}</p>
      {:else}
        <ol class="mt-3 space-y-3 border-s border-[var(--border-subtle)] ps-5">
          {#each p.events as ev, i (i)}
            <li class="relative">
              <span class="absolute -start-[25px] top-1 inline-flex size-[10px] rounded-full bg-[var(--color-primary-600)]"></span>
              <div class="flex flex-wrap items-baseline gap-x-2 text-[13px]">
                <span class="font-semibold">{actionLabel(ev.action)}</span>
                <span class="text-[11.5px] text-[var(--text-tertiary)]">{formatDateTime(ev.at)}{#if ev.actor} · {t('purchases.modal.historyBy', { actor: ev.actor })}{/if}</span>
              </div>
            </li>
          {/each}
        </ol>
      {/if}
    </section>

    {#if voiding}
      <form onsubmit={doVoid} class="mt-5 space-y-2 rounded-lg border border-[var(--color-danger-600)]/40 p-3" novalidate>
        <label class="block text-[12px] font-semibold" for="void-reason">{t('purchases.modal.voidReason')}</label>
        <input id="void-reason" bind:value={voidReason} maxlength="200" autocomplete="off"
          class="h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]" />
        <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('purchases.modal.reasonHint')}</p>
        {#if voidError}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{voidError}</p>{/if}
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-sm" disabled={voidBusy} onclick={() => (voiding = false)}>{t('purchases.modal.voidBack')}</button>
          <button type="submit" class="btn btn-sm btn-danger" disabled={voidBusy || voidReason.trim().length < 3}>
            <i class="icon-ban text-[13px]"></i>{t('purchases.modal.voidConfirm')}
          </button>
        </div>
      </form>
    {/if}
    <div class="mt-5 flex flex-wrap items-center justify-end gap-2">
      {#if p.status === 'completed' && can('purchase_returns', 'create')}
        <a href="/purchase-returns/new?purchase={p.id}" class="btn btn-sm"><i class="icon-undo-2 text-[13px]"></i>{t('purchases.modal.returnAction')}</a>
      {/if}
      {#if p.status === 'completed' && canUpdate}
        <a href="/purchases/new?edit={p.id}" class="btn btn-sm"><i class="icon-pencil text-[13px]"></i>{t('purchases.modal.revise')}</a>
      {/if}
      {#if p.status === 'completed' && canDelete && !voiding}
        <button type="button" class="btn btn-sm" onclick={() => (voiding = true)}><i class="icon-ban text-[13px]"></i>{t('purchases.modal.voidAction')}</button>
      {/if}
      <button type="button" class="btn" onclick={onclose}>{t('purchases.modal.close')}</button>
    </div>
  {/if}
</Modal>
