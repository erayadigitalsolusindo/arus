<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { purchaseReturns, type PurchaseReturn } from '#lib/purchases/returns.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { id, onclose, onchanged }: { id: string; onclose: () => void; onchanged?: () => void } = $props();

  const money = (v: string) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const q3 = (s: string) => formatNumber(Number(s), { maximumFractionDigits: 3 });
  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';

  let r = $state<PurchaseReturn | null>(null);
  let error = $state('');
  let voiding = $state(false);
  let reason = $state('');
  let busy = $state(false);
  let voidError = $state('');

  const canVoid = $derived(can('purchase_returns', 'approve'));

  onMount(async () => {
    try {
      r = await purchaseReturns.get(id);
    } catch (e) {
      error = errorMessage(e);
    }
  });

  async function doVoid(e: Event) {
    e.preventDefault();
    if (reason.trim().length < 3 || busy) return;
    busy = true;
    voidError = '';
    try {
      r = await purchaseReturns.void(id, reason.trim());
      voiding = false;
      reason = '';
      onchanged?.();
    } catch (err) {
      voidError = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={r ? t('purchaseReturns.modal.title', { doc: r.doc_no }) : t('purchaseReturns.detail')} {onclose} wide>
  {#if !r}
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{:else}<p class="text-[12.5px] text-[var(--text-tertiary)]">…</p>{/if}
  {:else}
    <div class="space-y-4">
      {#if r.status === 'void'}
        <div role="status" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-ban text-[14px] mt-0.5 shrink-0"></i>
          <span>{t('purchaseReturns.modal.voidedBanner', { at: formatDateTime(r.voided_at ?? r.created_at), by: r.voided_by || '—', reason: r.void_reason })}</span>
        </div>
      {/if}

      <dl class="grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-4">
        <div><dt class={label}>{t('purchaseReturns.modal.purchase')}</dt><dd class="font-mono text-[12.5px] font-semibold">{r.purchase_doc_no}</dd></div>
        <div><dt class={label}>{t('purchaseReturns.col.supplier')}</dt><dd class="font-semibold">{r.supplier_name}{#if r.supplier_invoice_no}<div class="font-mono text-[11px] font-normal text-[var(--text-tertiary)]">{r.supplier_invoice_no}</div>{/if}</dd></div>
        <div><dt class={label}>{t('purchaseReturns.col.date')}</dt><dd>{formatDate(r.return_date)}</dd></div>
        <div><dt class={label}>{t('purchaseReturns.modal.outlet')}</dt><dd>{r.outlet_name} <span class="text-[11px] text-[var(--text-tertiary)]">({r.outlet_code})</span></dd></div>
        <div class="col-span-2"><dt class={label}>{t('purchaseReturns.modal.createdBy')}</dt><dd>{r.created_by || '—'} · {formatDateTime(r.created_at)}</dd></div>
      </dl>

      <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
        <table class="w-full min-w-[560px] text-[12.5px]">
          <thead>
            <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="px-3 py-2 text-start" scope="col">{t('purchaseReturns.form.col.item')}</th>
              <th class="px-3 py-2 text-end" scope="col">{t('purchaseReturns.form.col.qty')}</th>
              <th class="px-3 py-2 text-end" scope="col">{t('purchaseReturns.form.col.value')}</th>
            </tr>
          </thead>
          <tbody>
            {#each r.lines as l (l.position)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0">
                <td class="px-3 py-2"><div class="font-medium">{l.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{l.sku}</div></td>
                <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{q3(l.qty)} {l.unit}</td>
                <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{money(l.value)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          {#if r.note}
            <div class={label}>{t('purchaseReturns.modal.note')}</div>
            <p class="mt-1 whitespace-pre-line text-[12.5px]">{r.note}</p>
          {/if}
        </div>
        <dl class="space-y-1.5 text-[13px]">
          <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.subtotal')}</dt><dd class="tabular-nums">{money(r.subtotal)}</dd></div>
          {#if Number(r.tax_amount) > 0}
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.taxPlain')}</dt><dd class="tabular-nums">{money(r.tax_amount)}</dd></div>
          {/if}
          <div class="flex justify-between gap-3 border-t border-[var(--border-subtle)] pt-1.5 font-bold"><dt>{t('purchaseReturns.form.summary.total')}</dt><dd class="tabular-nums">{money(r.total)}</dd></div>
          <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.cut')}</dt><dd class="tabular-nums">{money(r.payable_cut)}</dd></div>
          <div class="flex justify-between gap-3">
            <dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.refund')}</dt>
            <dd class="text-end tabular-nums">{money(r.refund)}{#if Number(r.refund) > 0}<div class="text-[11px] text-[var(--text-tertiary)]">{t('purchaseReturns.modal.refundVia', { method: r.refund_method_name })}{#if r.refund_ref} · {r.refund_ref}{/if}</div>{/if}</dd>
          </div>
        </dl>
      </div>

      {#if voiding}
        <form class="space-y-2 rounded-lg border border-[var(--color-danger-500)]/40 p-3" onsubmit={doVoid}>
          <p class="text-[12px] text-[var(--text-secondary)]">{t('purchaseReturns.modal.voidHint')}</p>
          <label class="block text-[12px] font-semibold" for="ret-void-reason">{t('purchaseReturns.modal.voidReason')}</label>
          <input id="ret-void-reason" bind:value={reason} maxlength="200" autocomplete="off"
            class="h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]" />
          {#if voidError}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{voidError}</p>{/if}
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-sm" disabled={busy} onclick={() => (voiding = false)}>{t('purchaseReturns.modal.voidBack')}</button>
            <button type="submit" class="btn btn-sm btn-danger" disabled={busy || reason.trim().length < 3}><i class="icon-ban text-[13px]"></i>{t('purchaseReturns.modal.voidConfirm')}</button>
          </div>
        </form>
      {/if}
    </div>
    <div class="mt-5 flex flex-wrap justify-end gap-2">
      {#if r.status === 'completed' && canVoid && !voiding && Number(r.refund) > 0 && r.refund_method !== 'supplier_credit'}
        <p class="me-auto self-center text-[11.5px] text-[var(--text-tertiary)]">{t('purchaseReturns.modal.voidRefunded')}</p>
      {:else if r.status === 'completed' && canVoid && !voiding}
        <button type="button" class="btn btn-sm" onclick={() => (voiding = true)}><i class="icon-ban text-[13px]"></i>{t('purchaseReturns.modal.voidAction')}</button>
      {/if}
      <button type="button" class="btn" onclick={onclose}>{t('purchaseReturns.modal.close')}</button>
    </div>
  {/if}
</Modal>
