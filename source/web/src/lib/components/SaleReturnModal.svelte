<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { salesReturns, type SaleReturn } from '#lib/sales/returns.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { id, onclose, onchanged }: { id: string; onclose: () => void; onchanged?: () => void } = $props();
  const money = (value: string) => formatCurrency(Number(value), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const qty = (value: string) => formatNumber(Number(value), { maximumFractionDigits: 3 });
  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';

  let record = $state<SaleReturn | null>(null);
  let error = $state('');
  let voiding = $state(false);
  let reason = $state('');
  let busy = $state(false);
  let voidError = $state('');
  const canVoid = $derived(can('sales_returns', 'approve'));
  /** Dana kembali yang sudah diserahkan (bukan ke deposit) membuat retur tidak bisa dibatalkan (aturan server). */
  const refunded = $derived(!!record && Number(record.refund) > 0 && record.refund_method !== 'deposit');

  async function doVoid(e: Event) {
    e.preventDefault();
    if (reason.trim().length < 3 || busy) return;
    busy = true;
    voidError = '';
    try {
      record = await salesReturns.void(id, reason.trim());
      voiding = false;
      reason = '';
      onchanged?.();
    } catch (err) {
      voidError = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  onMount(async () => {
    try {
      record = await salesReturns.get(id);
    } catch (err) {
      error = errorMessage(err);
    }
  });
</script>

<Modal title={record ? t('sales.returnFlow.modal.title', { doc: record.doc_no }) : t('sales.returnFlow.detail')} {onclose} wide>
  {#if !record}
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{:else}<p class="text-[12.5px] text-[var(--text-tertiary)]">…</p>{/if}
  {:else}
    <div class="space-y-4">
      {#if record.status === 'void'}
        <div role="status" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-ban text-[14px] mt-0.5 shrink-0"></i>
          <span>{t('sales.returnFlow.modal.voidedBanner', { at: formatDateTime(record.voided_at ?? record.created_at), by: record.voided_by || '—', reason: record.void_reason })}</span>
        </div>
      {/if}
      <dl class="grid grid-cols-2 gap-x-6 gap-y-3 text-[13px] sm:grid-cols-4">
        <div><dt class={label}>{t('sales.returnFlow.modal.sale')}</dt><dd class="font-mono text-[12.5px] font-semibold">{record.sale_doc_no}</dd></div>
        <div><dt class={label}>{t('sales.returnFlow.modal.member')}</dt><dd class="font-semibold">{record.member_name || '—'}</dd></div>
        <div><dt class={label}>{t('sales.returnFlow.col.date')}</dt><dd>{formatDate(record.return_date)}</dd></div>
        <div><dt class={label}>{t('sales.returnFlow.modal.outlet')}</dt><dd>{record.outlet_name}</dd></div>
        <div class="col-span-2"><dt class={label}>{t('sales.returnFlow.modal.createdBy')}</dt><dd>{record.created_by || '—'} · {formatDateTime(record.created_at)}</dd></div>
      </dl>

      <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
        <table class="w-full min-w-[560px] text-[12.5px]">
          <thead><tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-3 py-2 text-start" scope="col">{t('sales.returnFlow.form.col.item')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('sales.returnFlow.form.col.qty')}</th>
            <th class="px-3 py-2 text-end" scope="col">{t('sales.returnFlow.form.col.value')}</th>
          </tr></thead>
          <tbody>
            {#each record.lines as line (line.position)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0">
                <td class="px-3 py-2"><div class="font-medium">{line.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{line.sku} · {line.unit}</div></td>
                <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{qty(line.qty)} {line.unit}</td>
                <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap">{money(line.value)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          {#if record.note}<div class={label}>{t('sales.returnFlow.modal.note')}</div><p class="mt-1 whitespace-pre-line text-[12.5px]">{record.note}</p>{/if}
          {#if record.points_earned_reversed || record.points_redeemed_restored}
            <div class="mt-3 rounded border border-[var(--border-subtle)] p-3 text-[12px]">
              {#if record.points_earned_reversed}<div>{t('sales.returnFlow.modal.pointsEarned')}: −{formatNumber(record.points_earned_reversed)}</div>{/if}
              {#if record.points_redeemed_restored}<div>{t('sales.returnFlow.modal.pointsRedeemed')}: +{formatNumber(record.points_redeemed_restored)}</div>{/if}
            </div>
          {/if}
        </div>
        <dl class="space-y-1.5 text-[13px]">
          <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.subtotal')}</dt><dd class="tabular-nums">{money(record.subtotal)}</dd></div>
          {#if Number(record.discount) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.discount')}</dt><dd class="tabular-nums">−{money(record.discount)}</dd></div>{/if}
          {#if Number(record.tax_amount) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.tax')}</dt><dd class="tabular-nums">{money(record.tax_amount)}</dd></div>{/if}
          {#if Number(record.surcharge) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.surcharge')}</dt><dd class="tabular-nums">{money(record.surcharge)}</dd></div>{/if}
          <div class="flex justify-between gap-3 border-t border-[var(--border-subtle)] pt-1.5 font-bold"><dt>{t('sales.returnFlow.form.summary.total')}</dt><dd class="tabular-nums">{money(record.total)}</dd></div>
          <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.cut')}</dt><dd class="tabular-nums">{money(record.receivable_cut)}</dd></div>
          <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.refund')}</dt>
            <dd class="text-end tabular-nums">{money(record.refund)}{#if Number(record.refund) > 0}<div class="text-[11px] text-[var(--text-tertiary)]">{t('sales.returnFlow.modal.refundVia', { method: record.refund_method_name })}{#if record.refund_ref} · {record.refund_ref}{/if}</div>{/if}</dd>
          </div>
        </dl>
      </div>
      {#if voiding}
        <form class="space-y-2 rounded-lg border border-[var(--color-danger-500)]/40 p-3" onsubmit={doVoid}>
          <p class="text-[12px] text-[var(--text-secondary)]">{t('sales.returnFlow.modal.voidHint')}</p>
          <label class="block text-[12px] font-semibold" for="sret-void-reason">{t('sales.returnFlow.modal.voidReason')}</label>
          <input id="sret-void-reason" bind:value={reason} maxlength="200" autocomplete="off"
            class="h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]" />
          {#if voidError}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{voidError}</p>{/if}
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-sm" disabled={busy} onclick={() => (voiding = false)}>{t('sales.returnFlow.modal.voidBack')}</button>
            <button type="submit" class="btn btn-sm btn-danger" disabled={busy || reason.trim().length < 3}><i class="icon-ban text-[13px]"></i>{t('sales.returnFlow.modal.voidConfirm')}</button>
          </div>
        </form>
      {/if}
      <div class="flex flex-wrap justify-end gap-2">
        {#if record.status === 'completed' && canVoid && !voiding && refunded}
          <p class="me-auto self-center text-[11.5px] text-[var(--text-tertiary)]">{t('sales.returnFlow.modal.voidRefunded')}</p>
        {:else if record.status === 'completed' && canVoid && !voiding}
          <button type="button" class="btn btn-sm" onclick={() => (voiding = true)}><i class="icon-ban text-[13px]"></i>{t('sales.returnFlow.modal.voidAction')}</button>
        {/if}
        <button type="button" class="btn" onclick={onclose}>{t('sales.returnFlow.modal.close')}</button>
      </div>
    </div>
  {/if}
</Modal>
