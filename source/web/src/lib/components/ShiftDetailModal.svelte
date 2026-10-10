<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { t, formatCurrency, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { shifts, type Shift } from '#lib/shift/api.ts';
  import { signedCents, shiftLines } from '#lib/pos/report-receipts.ts';
  import { loadReceiptSettings, printErrorMessage, printLines } from '#lib/pos/receipt.ts';

  let { id, onclose }: { id: string; onclose: () => void } = $props();

  let sh = $state<Shift | null>(null);
  let error = $state('');
  let printing = $state(false);
  let printMsg = $state('');

  onMount(async () => {
    try {
      sh = await shifts.get(id);
    } catch (e) {
      error = errorMessage(e);
    }
  });

  const fmt = (c: bigint) => formatCurrency(Number(c) / 100);
  const money = (s: string | null | undefined) => fmt(signedCents(s));
  const signedFmt = (s: string | null | undefined) => {
    const c = signedCents(s);
    return (c > 0n ? '+' : '') + fmt(c);
  };
  const diffClass = (s: string | null | undefined) => {
    const c = signedCents(s);
    return c === 0n ? '' : c < 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-warning-700)]';
  };
  const flowLabel = (src: string) => t(`pos.today.flows.${src}` as MessageKey);

  async function print() {
    if (!sh || printing) return;
    printing = true;
    printMsg = '';
    try {
      const s = loadReceiptSettings();
      await printLines(shiftLines(sh, s.paper), s);
      printMsg = t('shift.closed.printed');
    } catch (e) {
      printMsg = printErrorMessage(e);
    } finally {
      printing = false;
    }
  }
</script>

<Modal title={t('shift.detail.title', { doc: sh?.doc_no ?? '' })} wide {onclose}>
  {#if !sh}
    <p class="text-[13px] text-[var(--text-tertiary)]">{error || '…'}</p>
  {:else}
    <div class="space-y-4 text-[13px]">
      <dl class="grid sm:grid-cols-2 gap-x-6 gap-y-1.5">
        <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.detail.cashier')}</dt><dd class="font-semibold">{sh.user_name}</dd></div>
        <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.detail.opened')}</dt><dd>{sh.opened_local}</dd></div>
        <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.openingCash')}</dt><dd class="tabular-nums">{money(sh.opening_cash)}</dd></div>
        {#if sh.closed_local}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.detail.closed')}</dt><dd>{sh.closed_local}</dd></div>{/if}
        {#if sh.closed_by_name}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.detail.closedBy')}</dt><dd>{sh.closed_by_name}</dd></div>{/if}
        {#if sh.approved_by_name}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.detail.approvedBy')}</dt><dd class="font-semibold">{sh.approved_by_name}</dd></div>{/if}
        <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('shift.list.col.sales')}</dt><dd class="tabular-nums">{t('shift.closing.salesCount', { count: sh.sale_count, total: money(sh.sales_total) })}</dd></div>
      </dl>

      <div class="overflow-x-auto rounded-md border border-[var(--border-subtle)]">
        <table class="w-full text-[12.5px]">
          <thead class="bg-[var(--surface-sunken)] text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
            <tr>
              <th class="text-start px-3 py-2">{t('shift.closing.method')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.opening')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.sales')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.flows')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.expected')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.counted')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.diff')}</th>
            </tr>
          </thead>
          <tbody>
            {#each sh.counts as c (c.method_id)}
              <tr class="border-t border-[var(--border-subtle)] tabular-nums">
                <td class="px-3 py-2 font-semibold">{c.name}</td>
                <td class="px-3 py-2 text-end">{money(c.opening)}</td>
                <td class="px-3 py-2 text-end">{money(c.sales)}</td>
                <td class="px-3 py-2 text-end">{signedFmt(c.flows)}</td>
                <td class="px-3 py-2 text-end">{money(c.expected)}</td>
                <td class="px-3 py-2 text-end">{c.counted === null ? '—' : money(c.counted)}</td>
                <td class="px-3 py-2 text-end font-semibold {diffClass(c.diff)}">{c.diff === null ? '—' : signedFmt(c.diff)}</td>
              </tr>
            {/each}
          </tbody>
          <tfoot class="border-t-2 border-[var(--border-default)] font-bold tabular-nums">
            <tr>
              <td class="px-3 py-2" colspan="4">{t('shift.closing.totalExpected')}</td>
              <td class="px-3 py-2 text-end">{money(sh.expected_total)}</td>
              <td class="px-3 py-2 text-end">{sh.counted_total === null ? '—' : money(sh.counted_total)}</td>
              <td class="px-3 py-2 text-end {diffClass(sh.diff_total)}">{sh.diff_total === null ? '—' : signedFmt(sh.diff_total)}</td>
            </tr>
          </tfoot>
        </table>
      </div>

      {#if sh.flows.length}
        <div>
          <h3 class="font-semibold mb-1">{t('shift.detail.flowsTitle')}</h3>
          <ul class="text-[12px] rounded-md bg-[var(--surface-sunken)] p-3 space-y-0.5">
            {#each sh.flows as f (f.source + f.method_id)}
              <li class="flex justify-between gap-3"><span>{flowLabel(f.source)} · {f.name} ({f.count})</span><span class="tabular-nums">{signedFmt(f.amount)}</span></li>
            {/each}
          </ul>
        </div>
      {/if}

      {#if sh.note}
        <div>
          <h3 class="font-semibold mb-1">{t('shift.detail.note')}</h3>
          <p class="whitespace-pre-line rounded-md bg-[var(--surface-sunken)] p-3">{sh.note}</p>
        </div>
      {/if}

      {#if printMsg}<p class="text-[12.5px] text-[var(--text-secondary)]" role="status">{printMsg}</p>{/if}
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" disabled={printing} onclick={print}><i class={printing ? 'icon-loader-circle animate-spin' : 'icon-printer'}></i> {t('shift.detail.print')}</button>
        <button type="button" class="btn btn-primary" onclick={onclose}>{t('shift.closed.done')}</button>
      </div>
    </div>
  {/if}
</Modal>
