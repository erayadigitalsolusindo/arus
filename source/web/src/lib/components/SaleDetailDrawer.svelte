<script lang="ts">
  // Panel detail nota (slide-over kanan): barang yang dibeli, harga daftar → harga jual, HPP/laba (bila berizin),
  // sumber potongan (manual/kupon/poin), pembayaran, retur (nota asli tetap utuh; retur = dokumen terpisah), dampak stok,
  // dan riwayat audit. Hanya baca.
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import VoidSaleModal from '#lib/components/VoidSaleModal.svelte';
  import SaleReturnModal from '#lib/components/SaleReturnModal.svelte';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { sales as api, PAY_METHODS, type SaleDetail } from '#lib/sales/api.ts';
  import { t, tryT, formatCurrency, formatNumber, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { saleId, onclose, onswitch, onchanged }: { saleId: string; onclose: () => void; onswitch?: (id: string) => void; onchanged?: () => void } = $props();

  type Tab = 'items' | 'discounts' | 'payments' | 'returns' | 'stock' | 'history';
  let tab = $state<Tab>('items');
  let d = $state<SaleDetail | null>(null);
  let error = $state('');
  let panel = $state<HTMLElement>();

  onMount(() => {
    const previous = document.activeElement as HTMLElement | null;
    panel?.focus();
    return () => previous?.focus?.();
  });

  // Muat ulang saat berpindah antar versi nota (revisi) dari dalam panel.
  let seq = 0;
  function load() {
    const mine = ++seq;
    d = null;
    error = '';
    api
      .detail(saleId)
      .then((res) => mine === seq && (d = res))
      .catch((err) => mine === seq && (error = errorMessage(err)));
  }
  $effect(() => {
    void saleId;
    load();
  });

  let voiding = $state(false);
  const isCompleted = $derived(d?.status === 'completed');
  const sameOutlet = $derived(!!d && d.outlet.id === session.outlet?.id);
  // Nota yang punya retur aktif tidak bisa diedit/dibatalkan (server: SALE_HAS_RETURNS); batalkan returnya dulu.
  const lockedByReturns = $derived(!!d && d.returns.some((r) => r.status === 'completed'));
  const canEdit = $derived(isCompleted && !lockedByReturns && can('sales_orders', 'update'));
  const canVoid = $derived(isCompleted && !lockedByReturns && can('sales_orders', 'delete'));
  function startEdit() {
    if (!d) return;
    void goto('/kasir?edit=' + d.id);
  }
  const statusLabel = (s: string) => (s === 'void' ? t('sales.status.void') : s === 'superseded' ? t('sales.statusSuperseded') : t('sales.status.completed'));

  const money = (v: string | number) => formatCurrency(Number(v));
  const qty = (v: string) => formatNumber(Number(v), { maximumFractionDigits: 3 });
  const num = (v: string | undefined) => Number(v ?? 0);
  const profitClass = (v: string | undefined) => (num(v) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : 'text-[var(--color-success-600,#16a34a)]');
  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;

  const voided = $derived(d?.status === 'void');
  const superseded = $derived(d?.status === 'superseded');
  const hasCost = $derived(d?.cost !== undefined);
  const margin = $derived(d && d.cost !== undefined && d.profit !== undefined && num(d.subtotal) - num(d.discount) > 0 ? (num(d.profit) / (num(d.subtotal) - num(d.discount))) * 100 : null);
  const discountTotal = $derived(d ? num(d.discount) + num(d.line_discount) : 0);
  const netPaid = $derived(d ? num(d.paid) - num(d.change) : 0);
  // Biaya ditanggung toko (mengurangi uang yang diterima toko) vs biaya yang ditagihkan ke pelanggan (tambahan di atas total).
  const feeTotal = $derived(d ? d.payments.reduce((s, p) => s + (p.fee_bearer === 'customer' ? 0 : Number(p.fee ?? 0)), 0) : 0);
  const surcharge = $derived(d ? Number(d.surcharge ?? 0) : 0);
  const overrides = $derived(d ? d.lines.filter((l) => l.price_override).length : 0);

  // Retur: baris nota tidak pernah berubah; qty yang diretur hanya ditandai.
  const returnedQty = (l: { returned_qty: string }) => num(l.returned_qty);
  const anyReturned = $derived(!!d && d.lines.some((l) => returnedQty(l) > 0));
  const allReturned = $derived(!!d && d.lines.length > 0 && d.lines.every((l) => returnedQty(l) >= num(l.qty)));
  const activeReturns = $derived(d ? d.returns.filter((r) => r.status === 'completed').length : 0);
  const canOpenReturn = $derived(can('sales_returns', 'view'));
  let openReturn = $state<string | null>(null);

  const tabs = $derived<{ id: Tab; label: string; count?: number }[]>([
    { id: 'items', label: t('sales.detail.tabs.items'), count: d?.lines.length },
    { id: 'discounts', label: t('sales.detail.tabs.discounts'), count: d ? (num(d.line_discount) > 0 ? 1 : 0) + (num(d.manual_discount) > 0 ? 1 : 0) + d.vouchers.length + (d.points_redeemed > 0 ? 1 : 0) : undefined },
    { id: 'payments', label: t('sales.detail.tabs.payments'), count: d?.payments.length },
    { id: 'returns', label: t('sales.detail.tabs.returns'), count: d?.returns.length },
    { id: 'stock', label: t('sales.detail.tabs.stock'), count: d?.stock.length },
    { id: 'history', label: t('sales.detail.tabs.history'), count: d?.events.length }
  ]);

  const BUCKET_ICON: Record<string, string> = { display: 'icon-store', warehouse: 'icon-boxes', returns: 'icon-rotate-ccw' };
  const METHOD_ICON: Record<string, string> = { cash: 'icon-banknote', debit: 'icon-credit-card', credit_card: 'icon-credit-card', ewallet: 'icon-wallet', transfer: 'icon-arrow-right' };

  // Detail audit yang dikenal: penyetuju + baris yang diubah (sku, harga normal → harga, atau potongan).
  type EvLine = { sku?: string; list_price?: string; price?: string; discount?: string; qty?: string };
  const evApprover = (details: Record<string, unknown> | null) => (typeof details?.approver === 'string' ? details.approver : '');
  const evLines = (details: Record<string, unknown> | null): EvLine[] => (Array.isArray(details?.lines) ? (details.lines as EvLine[]) : []);

  function onkeydown(e: KeyboardEvent) {
    // Modal di atas panel (rincian retur, batal nota) menutup dirinya sendiri; panel tetap terbuka.
    if (e.key === 'Escape' && !e.defaultPrevented && !openReturn && !voiding) onclose();
  }
  function tabKeys(e: KeyboardEvent) {
    const i = tabs.findIndex((x) => x.id === tab);
    const next = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : null;
    if (next === null) return;
    e.preventDefault();
    tab = tabs[(next + tabs.length) % tabs.length].id;
    (document.getElementById(`sd-tab-${tab}`) as HTMLElement | null)?.focus();
  }

  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<svelte:window {onkeydown} />

<div class="fixed inset-0 z-[60] flex justify-end bg-black/45 backdrop-blur-[2px] sd-fade" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div
    bind:this={panel}
    class="sd-slide flex h-full w-full max-w-[980px] flex-col bg-[var(--surface-canvas)] shadow-2xl outline-none"
    role="dialog"
    aria-modal="true"
    aria-label={t('sales.detail.title')}
    tabindex="-1"
  >
    <!-- Kepala: identitas nota -->
    <header class="border-b border-[var(--border-subtle)] bg-[var(--surface-raised)] px-5 pt-4 pb-3">
      <div class="flex items-start gap-3">
        <span class="inline-flex size-11 shrink-0 items-center justify-center rounded-xl bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-receipt text-[20px]"></i></span>
        <div class="min-w-0 grow">
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="font-mono text-[17px] font-bold tracking-tight">{d?.doc_no ?? '…'}</h2>
            {#if d}<span class="badge-soft {voided ? 'badge-danger' : superseded ? 'badge-warning' : 'badge-success'}">{statusLabel(d.status)}</span>{/if}
            {#if overrides > 0}<span class="badge-soft badge-warning">{t('sales.badge.override')} ×{overrides}</span>{/if}
            {#if anyReturned}<button type="button" class="badge-soft {allReturned ? 'badge-danger' : 'badge-warning'} inline-flex items-center gap-1" onclick={() => (tab = 'returns')}><i class="icon-rotate-ccw text-[11px]"></i>{allReturned ? t('sales.detail.returns.badgeFull') : t('sales.detail.returns.badgePartial')}</button>{/if}
          </div>
          {#if d}<div class="mt-0.5 text-[12px] text-[var(--text-tertiary)]">{formatDateTime(d.created_at)}</div>{/if}
        </div>
        {#if canEdit}
          <button type="button" class="btn btn-sm" disabled={!sameOutlet} title={sameOutlet ? '' : t('errors.OUTLET_MISMATCH')} onclick={startEdit}><i class="icon-pencil me-1 text-[12px]"></i>{t('sales.detail.actions.edit')}</button>
        {/if}
        {#if canVoid}
          <button type="button" class="btn btn-sm text-[var(--color-danger-600)]" onclick={() => (voiding = true)}><i class="icon-ban me-1 text-[12px]"></i>{t('sales.detail.actions.void')}</button>
        {/if}
        <button type="button" class="header-icon-btn" onclick={onclose} aria-label={t('common.close')}><i class="icon-x text-[16px]"></i></button>
      </div>

      {#if d && d.revisions.length > 1}
        <div class="mt-3 flex flex-wrap items-center gap-1.5 text-[11.5px]" aria-label={t('sales.detail.revisions.title')}>
          <span class="font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('sales.detail.revisions.title')}</span>
          {#each d.revisions as r (r.id)}
            <button type="button" class="rounded-full border px-2 py-0.5 font-medium transition-colors {r.id === d.id ? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary)]'}" title={(r.reason ? t('sales.detail.revisions.reason') + ': ' + r.reason + ' · ' : '') + statusLabel(r.status)} aria-current={r.id === d.id ? 'true' : undefined} onclick={() => onswitch?.(r.id)}>
              R{r.revision}{#if r.status === 'completed'} · {t('sales.detail.revisions.current')}{:else if r.status === 'void'} · {t('sales.detail.revisions.voided')}{/if}
            </button>
          {/each}
        </div>
      {/if}
      {#if superseded}<p class="mt-2 rounded-md bg-[var(--color-warning-500)]/10 px-2.5 py-1.5 text-[11.5px] text-[var(--color-warning-600)]">{t('sales.detail.revisions.replacedBy')}</p>{/if}

      {#if d}
        <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-[12.5px] sm:grid-cols-4">
          <div><dt class={label}>{t('sales.detail.info.outlet')}</dt><dd class="mt-0.5 font-medium">{d.outlet.name} <span class="font-mono text-[11px] font-normal text-[var(--text-tertiary)]">{d.outlet.code}</span></dd></div>
          <div><dt class={label}>{t('sales.detail.info.cashier')}</dt><dd class="mt-0.5 font-medium">{d.cashier || '—'}</dd></div>
          <div><dt class={label}>{t('sales.detail.info.member')}</dt><dd class="mt-0.5 font-medium">{d.member ? d.member.name : '—'}{#if d.member}<span class="font-mono text-[11px] font-normal text-[var(--text-tertiary)]"> {d.member.code}</span>{/if}</dd></div>
          <div><dt class={label}>{t('sales.detail.info.salesperson')}</dt><dd class="mt-0.5 font-medium">{d.salesperson?.name ?? '—'}</dd></div>
        </dl>
      {/if}
    </header>

    {#if error}
      <div class="p-5"><div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('sales.detail.loadFailed')} {error}</span></div></div>
    {:else if !d}
      <div class="space-y-3 p-5" aria-busy="true">
        <div class="grid grid-cols-3 gap-3">{#each [0, 1, 2] as i (i)}<div class="h-[68px] animate-pulse rounded-xl bg-[var(--surface-sunken)]"></div>{/each}</div>
        {#each [0, 1, 2, 3] as i (i)}<div class="h-10 animate-pulse rounded-lg bg-[var(--surface-sunken)]"></div>{/each}
      </div>
    {:else}
      <!-- Kartu ringkas -->
      <div class="grid grid-cols-2 gap-3 px-5 pt-4 {hasCost ? 'sm:grid-cols-4' : 'sm:grid-cols-3'}">
        <div class="surface-card !p-3.5 col-span-2 sm:col-span-1">
          <div class={label}>{t('sales.detail.stat.total')}</div>
          <div class="mt-1 font-display text-[22px] font-bold tabular-nums leading-tight {voided ? 'line-through opacity-60' : ''}">{money(d.total)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.items', { count: d.lines.length, qty: qty(d.base_qty_total) })}</div>
          {#if num(d.returned_total) > 0}<div class="mt-0.5 text-[11.5px] font-medium text-[var(--color-warning-600,#d97706)]">{t('sales.detail.returns.statNet', { returned: money(d.returned_total), net: money(d.net_total) })}</div>{/if}
        </div>
        <div class="surface-card !p-3.5">
          <div class={label}>{t('sales.detail.stat.paid')}</div>
          <div class="mt-1 font-display text-[17px] font-bold tabular-nums">{money(d.paid)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.change')} {money(d.change)}</div>
        </div>
        <div class="surface-card !p-3.5">
          <div class={label}>{t('sales.summary.discount')}</div>
          <div class="mt-1 font-display text-[17px] font-bold tabular-nums">{money(discountTotal)}</div>
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{d.vouchers.length ? d.vouchers.map((v) => v.code).join(', ') : '—'}</div>
        </div>
        {#if hasCost && d.profit !== undefined && d.cost !== undefined}
          <div class="surface-card !p-3.5">
            <div class={label}>{t('sales.detail.stat.profit')}</div>
            {#if activeReturns > 0 && d.profit_net !== undefined && d.returned_cost !== undefined}
              <div class="mt-1 font-display text-[17px] font-bold tabular-nums {profitClass(d.profit_net)}">{money(d.profit_net)}</div>
              <div class="mt-0.5 text-[11.5px] font-medium text-[var(--color-warning-600,#d97706)]">{t('sales.detail.returns.profitNet')}</div>
              <div class="text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.returns.profitBefore', { amount: money(d.profit), cost: money(d.returned_cost) })}</div>
            {:else}
              <div class="mt-1 font-display text-[17px] font-bold tabular-nums {profitClass(d.profit)}">{money(d.profit)}</div>
              <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.stat.cost')} {money(d.cost)}{#if margin !== null} · {formatNumber(margin, { maximumFractionDigits: 1 })}%{/if}</div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Tab -->
      <div class="mt-4 flex gap-1 overflow-x-auto border-b border-[var(--border-subtle)] px-5 scroll-thin" role="tablist" aria-label={t('sales.detail.title')} tabindex="-1" onkeydown={tabKeys}>
        {#each tabs as tb (tb.id)}
          <button
            id="sd-tab-{tb.id}"
            type="button"
            role="tab"
            aria-selected={tab === tb.id}
            aria-controls="sd-panel"
            tabindex={tab === tb.id ? 0 : -1}
            class="relative -mb-px flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 text-[12.5px] font-semibold transition-colors !bg-transparent {tab === tb.id ? 'border-[var(--color-primary)] !text-[var(--color-primary)]' : 'border-transparent !text-[var(--text-tertiary)] hover:!text-[var(--text-primary,inherit)]'}"
            onclick={() => (tab = tb.id)}
          >
            {tb.label}{#if tb.count !== undefined}<span class="rounded-full bg-[var(--surface-sunken)] px-1.5 py-px text-[10.5px] tabular-nums">{tb.count}</span>{/if}
          </button>
        {/each}
      </div>

      <div id="sd-panel" role="tabpanel" aria-labelledby="sd-tab-{tab}" class="grow overflow-y-auto p-5 scroll-thin" tabindex="0">
        {#if tab === 'items'}
          {#if anyReturned}
            <div class="mb-3 flex flex-wrap items-center gap-2 rounded-lg bg-[var(--color-warning-500)]/10 px-3 py-2.5 text-[12.5px] text-[var(--color-warning-600)]">
              <i class="icon-rotate-ccw text-[14px] shrink-0"></i>
              <span class="grow">{allReturned ? t('sales.detail.returns.bannerFull') : t('sales.detail.returns.banner')}{#if lockedByReturns && (can('sales_orders', 'update') || can('sales_orders', 'delete'))}<span class="mt-0.5 block text-[11.5px] opacity-80">{t('errors.SALE_HAS_RETURNS')}</span>{/if}</span>
              <button type="button" class="font-semibold underline-offset-2 hover:underline" onclick={() => (tab = 'returns')}>{t('sales.detail.returns.seeReturns')} ({activeReturns})</button>
            </div>
          {/if}
          <div class="surface-card !p-0 overflow-hidden">
            <div class="overflow-x-auto scroll-thin">
              <table class="w-full min-w-[600px] text-[12.5px]">
                <thead>
                  <tr class="bg-[var(--surface-sunken)] text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                    <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.items.item')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.qty')}</th>
                    {#if anyReturned}<th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.returns.col')}</th>{/if}
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.price')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.discount')}</th>
                    <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.total')}</th>
                    {#if hasCost}
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.cost')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.items.profit')}</th>
                    {/if}
                  </tr>
                </thead>
                <tbody>
                  {#each d.lines as l (l.position)}
                    {@const rq = returnedQty(l)}
                    {@const full = rq > 0 && rq >= num(l.qty)}
                    <tr class="border-t border-[var(--border-subtle)] align-top {full ? 'bg-[var(--surface-sunken)]/60' : ''}">
                      <td class="px-3 py-3 {full ? 'opacity-60' : ''}">
                        <div class="font-semibold {full ? 'line-through' : ''}">{l.name}</div>
                        <div class="font-mono text-[11px] text-[var(--text-tertiary)]">{l.sku}</div>
                        {#if Number(l.factor) !== 1}<div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{t('sales.detail.items.baseQty', { qty: qty(l.base_qty) })}</div>{/if}
                        {#if l.note}<div class="mt-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px]"><span class="font-semibold">{t('sales.detail.items.lineNote')}:</span> {l.note}</div>{/if}
                      </td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">{qty(l.qty)} <span class="text-[var(--text-tertiary)]">{l.unit}</span></td>
                      {#if anyReturned}
                        <td class="px-3 py-3 text-end whitespace-nowrap">
                          {#if rq > 0}
                            <span class="badge-soft {full ? 'badge-danger' : 'badge-warning'} inline-flex items-center gap-1 tabular-nums"><i class="icon-rotate-ccw text-[10px]"></i>{full ? t('sales.detail.returns.lineFull') : t('sales.detail.returns.lineSome', { qty: qty(l.returned_qty) })}</span>
                            {#if !full}<div class="mt-1 text-[11px] text-[var(--text-tertiary)] tabular-nums">{t('sales.detail.returns.kept', { qty: qty(String(num(l.qty) - rq)), unit: l.unit })}</div>{/if}
                          {:else}<span class="text-[var(--text-tertiary)]">—</span>{/if}
                        </td>
                      {/if}
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">
                        {#if l.price_override || num(l.list_price) !== num(l.unit_price)}<div class="text-[11px] text-[var(--text-tertiary)] line-through">{money(l.list_price)}</div>{/if}
                        <div class="font-medium">{money(l.unit_price)}</div>
                        {#if l.price_override}<span class="badge-soft badge-warning mt-1 inline-block">{t('sales.detail.items.overridden')}</span>{/if}
                      </td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums">{num(l.discount) > 0 ? '−' + money(l.discount) : '—'}</td>
                      <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-semibold">{money(l.line_total)}</td>
                      {#if hasCost && l.unit_cost !== undefined && l.profit !== undefined}
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums text-[var(--text-secondary)]">{money(l.unit_cost)}</td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-medium {profitClass(l.profit)}">{money(l.profit)}</td>
                      {/if}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </div>
          <!-- Ringkasan angka ala invoice -->
          <div class="mt-4 flex justify-end">
            <dl class="surface-card !p-4 w-full max-w-[360px] space-y-2 text-[12.5px] tabular-nums">
              <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.discounts.subtotal')}</dt><dd class="font-medium">{money(d.subtotal)}</dd></div>
              {#if num(d.discount) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.summary.discount')}</dt><dd class="text-[var(--color-warning-600,#d97706)]">−{money(d.discount)}</dd></div>{/if}
              {#if num(d.tax_store) + num(d.tax_gov) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.col.tax')}</dt><dd>+{money(num(d.tax_store) + num(d.tax_gov))}</dd></div>{/if}
              {#if num(d.other_cost) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.discounts.otherCost')}</dt><dd>+{money(d.other_cost)}</dd></div>{#each d.other_costs as c, ci (ci)}<div class="flex justify-between gap-3 text-[11.5px] text-[var(--text-tertiary)]"><dt class="ps-3">{c.name || t('sales.detail.discounts.unnamedCost')}</dt><dd>{money(c.amount)}</dd></div>{/each}{/if}
              <div class="flex items-baseline justify-between gap-3 border-t border-[var(--border-subtle)] pt-3 text-[15px] font-bold"><dt>{t('sales.detail.discounts.total')}</dt><dd>{money(d.total)}</dd></div>
              {#if num(d.returned_total) > 0}
                <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.returns.returned')} ({activeReturns})</dt><dd class="text-[var(--color-warning-600,#d97706)]">−{money(d.returned_total)}</dd></div>
                <div class="flex items-baseline justify-between gap-3 border-t border-dashed border-[var(--border-subtle)] pt-2 font-bold"><dt>{t('sales.detail.returns.net')}</dt><dd>{money(d.net_total)}</dd></div>
              {/if}
            </dl>
          </div>
          {#if d.note}<p class="mt-3 rounded-lg bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px]"><span class={label}>{t('sales.detail.info.note')}</span><br />{d.note}</p>{/if}
        {:else if tab === 'discounts'}
          <div class="grid gap-4 md:grid-cols-2">
            <section>
              <h3 class="mb-2 text-[12.5px] font-bold">{t('sales.detail.discounts.sources')}</h3>
              {#if discountTotal <= 0}
                <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-4 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.discounts.none')}</p>
              {:else}
                <ul class="space-y-2">
                  {#if num(d.line_discount) > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-tag text-[15px]"></i></span>
                      <div class="min-w-0 grow"><div class="font-semibold text-[12.5px]">{t('sales.detail.discounts.lineDiscountItems')}</div><div class="truncate text-[11.5px] text-[var(--text-tertiary)]">{d.lines.filter((l) => num(l.discount) > 0).map((l) => l.name).join(', ')}</div></div>
                      <div class="font-semibold tabular-nums">−{money(d.line_discount)}</div>
                    </li>
                  {/if}
                  {#if num(d.manual_discount) > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="icon-percent text-[15px]"></i></span>
                      <div class="grow font-semibold text-[12.5px]">{t('sales.detail.discounts.manual')}</div>
                      <div class="font-semibold tabular-nums">−{money(d.manual_discount)}</div>
                    </li>
                  {/if}
                  {#each d.vouchers as v (v.code)}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-success-600,#16a34a)]/10 text-[var(--color-success-600,#16a34a)]"><i class="icon-ticket text-[15px]"></i></span>
                      <div class="min-w-0 grow">
                        <div class="font-mono text-[12.5px] font-bold">{v.code}</div>
                        <div class="truncate text-[11.5px] text-[var(--text-tertiary)]">{v.name} · {v.kind === 'percent' ? t('sales.detail.discounts.kindPercent', { value: formatNumber(Number(v.value), { maximumFractionDigits: 2 }) }) : t('sales.detail.discounts.kindAmount')}</div>
                      </div>
                      <div class="font-semibold tabular-nums">−{money(v.amount)}</div>
                    </li>
                  {/each}
                  {#if d.points_redeemed > 0}
                    <li class="surface-card !p-3 flex items-center gap-3">
                      <span class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-[var(--color-warning-600,#d97706)]/10 text-[var(--color-warning-600,#d97706)]"><i class="icon-star text-[15px]"></i></span>
                      <div class="min-w-0 grow"><div class="font-semibold text-[12.5px]">{t('sales.detail.discounts.points')}</div><div class="text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.discounts.pointsDetail', { points: formatNumber(d.points_redeemed) })}</div></div>
                      <div class="font-semibold tabular-nums">−{money(d.redeem_amount)}</div>
                    </li>
                  {/if}
                </ul>
              {/if}
              {#if d.points_earned > 0}<p class="mt-3 flex items-center gap-2 text-[12px] text-[var(--text-secondary)]"><i class="icon-star text-[13px]"></i>{t('sales.detail.discounts.earned')}: <strong>{formatNumber(d.points_earned)}</strong></p>{/if}
            </section>

            <!-- Rincian hitung ala struk -->
            <section>
              <h3 class="mb-2 text-[12.5px] font-bold">{t('sales.detail.discounts.calc')}</h3>
              <div class="surface-card !p-4 text-[12.5px]">
                <dl class="space-y-2 tabular-nums">
                  <div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.subtotal')}</dt><dd class="font-medium">{money(d.subtotal)}</dd></div>
                  {#if num(d.line_discount) > 0}<div class="flex justify-between gap-3 text-[var(--text-tertiary)]"><dt class="ps-3 text-[11.5px]">{t('sales.detail.discounts.lineDiscount')}</dt><dd class="text-[11.5px]">{money(d.line_discount)}</dd></div>{/if}
                  {#if num(d.manual_discount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.manual')}</dt><dd>−{money(d.manual_discount)}</dd></div>{/if}
                  {#if num(d.voucher_amount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.voucher')}</dt><dd>−{money(d.voucher_amount)}</dd></div>{/if}
                  {#if num(d.redeem_amount) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.points')}</dt><dd>−{money(d.redeem_amount)}</dd></div>{/if}
                  <div class="flex justify-between gap-3 border-t border-dashed border-[var(--border-subtle)] pt-2"><dt>{t('sales.detail.discounts.afterDiscount')}</dt><dd class="font-medium">{money(num(d.subtotal) - num(d.discount))}</dd></div>
                  {#if num(d.tax_store) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.taxStore', { pct: formatNumber(Number(d.tax_store_pct), { maximumFractionDigits: 2 }) })}</dt><dd>+{money(d.tax_store)}</dd></div>{/if}
                  {#if num(d.tax_gov) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.taxGov', { pct: formatNumber(Number(d.tax_gov_pct), { maximumFractionDigits: 2 }) })}</dt><dd>+{money(d.tax_gov)}</dd></div>{/if}
                  {#if num(d.other_cost) > 0}<div class="flex justify-between gap-3"><dt>{t('sales.detail.discounts.otherCost')}</dt><dd>+{money(d.other_cost)}</dd></div>{#each d.other_costs as c, ci (ci)}<div class="flex justify-between gap-3 text-[11.5px] text-[var(--text-tertiary)]"><dt class="ps-3">{c.name || t('sales.detail.discounts.unnamedCost')}</dt><dd>{money(c.amount)}</dd></div>{/each}{/if}
                  <div class="flex items-baseline justify-between gap-3 border-t border-[var(--border-subtle)] pt-3 text-[15px] font-bold"><dt>{t('sales.detail.discounts.total')}</dt><dd>{money(d.total)}</dd></div>
                </dl>
              </div>
            </section>
          </div>
        {:else if tab === 'payments'}
          {#if num(d.receivable) > 0 && d.credit}
            <div class="surface-card !p-4 mb-3 text-[12.5px]">
              <div class="mb-2 flex items-center justify-between gap-2">
                <span class="text-[13px] font-semibold"><i class="icon-hourglass me-1.5"></i>{t('sales.detail.credit.title')}</span>
                <span class="badge-soft {d.credit.status === 'paid' ? 'badge-success' : d.credit.status === 'overdue' ? 'badge-danger' : 'badge-info'}">{t(`receivables.status.${d.credit.status}`)}</span>
              </div>
              <dl class="space-y-1.5 tabular-nums">
                <div class="flex justify-between"><dt>{t('sales.detail.credit.amount')}</dt><dd>{money(d.credit.amount)}</dd></div>
                <div class="flex justify-between"><dt>{t('sales.detail.credit.paid')}</dt><dd>{money(d.credit.paid)}</dd></div>
                <div class="flex justify-between"><dt>{t('sales.detail.credit.returned')}</dt><dd>{money(d.credit.returned)}</dd></div>
                <div class="flex justify-between border-t border-[var(--border-subtle)] pt-1.5 text-[14px] font-bold"><dt>{t('sales.detail.credit.balance')}</dt><dd class={num(d.credit.balance) > 0 ? 'text-[var(--color-danger-600)]' : ''}>{money(d.credit.balance)}</dd></div>
                <div class="flex justify-between"><dt>{t('sales.detail.credit.due')}</dt><dd>{d.credit.due_date ? formatDate(d.credit.due_date) : t('receivables.noDue')}</dd></div>
              </dl>
            </div>
          {/if}
          {#if d.payments.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.payments.empty')}</p>
          {:else}
            <ul class="space-y-2">
              {#each d.payments as p, i (i)}
                <li class="surface-card !p-3.5 flex items-center gap-3">
                  <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary-600)]/10 text-[var(--color-primary-600)]"><i class="{METHOD_ICON[p.method] ?? 'icon-wallet'} text-[17px]"></i></span>
                  <div class="min-w-0 grow">
                    <div class="font-semibold text-[13px]">{p.method_name || (PAY_METHODS.includes(p.method) ? t(`sales.method.${p.method}`) : p.method)}</div>
                    {#if p.ref_no}<div class="font-mono text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.payments.ref')}: {p.ref_no}</div>{/if}
                    {#if num(p.fee) > 0}<div class="text-[11.5px] text-[var(--color-warning-600)]">{Number(p.fee_pct) > 0 ? t('sales.detail.payments.fee', { pct: formatNumber(Number(p.fee_pct), { maximumFractionDigits: 2 }) }) + ' · ' : ''}{p.fee_bearer === 'customer' ? t('sales.detail.payments.feeCustomer') + ' +' : t('sales.detail.payments.feeFlat') + ' −'}{money(p.fee)}</div>{/if}
                  </div>
                  <div class="font-display text-[16px] font-bold tabular-nums">{money(p.amount)}</div>
                </li>
              {/each}
            </ul>
            <div class="surface-card !p-4 mt-3 text-[12.5px]">
              <dl class="space-y-2 tabular-nums">
                <div class="flex justify-between"><dt>{t('sales.detail.payments.paid')}</dt><dd class="font-medium">{money(d.paid)}</dd></div>
                <div class="flex justify-between"><dt>{t('sales.detail.payments.change')}</dt><dd>−{money(d.change)}</dd></div>
                <div class="flex justify-between border-t border-[var(--border-subtle)] pt-2 text-[14px] font-bold"><dt>{t('sales.detail.payments.net')}</dt><dd>{money(netPaid)}</dd></div>
                {#if surcharge > 0}
                  <div class="flex justify-between"><dt>{t('sales.detail.payments.surchargeTotal')}</dt><dd>+{money(surcharge)}</dd></div>
                  <div class="flex justify-between font-semibold"><dt>{t('sales.detail.payments.charged')}</dt><dd>{money(num(d.total) + surcharge)}</dd></div>
                {/if}
                {#if feeTotal > 0}
                  <div class="flex justify-between text-[var(--color-warning-600)]"><dt>{t('sales.detail.payments.feeTotal')}</dt><dd>−{money(feeTotal)}</dd></div>
                  <div class="flex justify-between font-semibold"><dt>{t('sales.detail.payments.received')}</dt><dd>{money(netPaid - feeTotal)}</dd></div>
                {/if}
              </dl>
            </div>
          {/if}
        {:else if tab === 'returns'}
          <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('sales.detail.returns.intro')}</p>
          {#if d.returns.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.returns.empty')}</p>
          {:else}
            <ul class="space-y-3">
              {#each d.returns as r (r.id)}
                {@const isVoid = r.status === 'void'}
                <li class="surface-card !p-4 {isVoid ? 'opacity-70' : ''}">
                  <div class="flex flex-wrap items-start gap-3">
                    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-lg {isVoid ? 'bg-[var(--surface-sunken)] text-[var(--text-tertiary)]' : 'bg-[var(--color-warning-600,#d97706)]/10 text-[var(--color-warning-600,#d97706)]'}"><i class="icon-rotate-ccw text-[17px]"></i></span>
                    <div class="min-w-0 grow">
                      <div class="flex flex-wrap items-center gap-2">
                        {#if canOpenReturn}
                          <button type="button" class="font-mono text-[13.5px] font-bold text-[var(--color-primary)] hover:underline {isVoid ? 'line-through' : ''}" title={t('sales.detail.returns.open')} onclick={() => (openReturn = r.id)}>{r.doc_no}</button>
                        {:else}
                          <span class="font-mono text-[13.5px] font-bold {isVoid ? 'line-through' : ''}">{r.doc_no}</span>
                        {/if}
                        <span class="badge-soft {isVoid ? 'badge-danger' : 'badge-success'}">{isVoid ? t('sales.detail.returns.voided') : t('sales.status.completed')}</span>
                      </div>
                      <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.detail.returns.date')} {formatDate(r.return_date)} · {t('sales.detail.returns.by', { at: formatDateTime(r.created_at), by: r.created_by || '—' })}</div>
                      {#if isVoid && r.void_reason}<div class="mt-1 text-[11.5px] text-[var(--color-danger-600)]">{t('sales.detail.returns.voidReason', { reason: r.void_reason })}</div>{/if}
                    </div>
                    <div class="text-end">
                      <div class="font-display text-[16px] font-bold tabular-nums {isVoid ? 'line-through' : ''}">−{money(r.total)}</div>
                    </div>
                  </div>
                  <div class="mt-3 grid gap-3 sm:grid-cols-2">
                    <div>
                      <div class={label}>{t('sales.detail.returns.items')}</div>
                      <ul class="mt-1 space-y-1 text-[12.5px]">
                        {#each r.lines as rl, j (j)}
                          <li class="flex justify-between gap-3"><span class="truncate"><span class="text-[var(--text-tertiary)] tabular-nums">#{rl.sale_position}</span> {rl.name}</span><span class="shrink-0 tabular-nums font-medium">{qty(rl.qty)} {rl.unit}</span></li>
                        {/each}
                      </ul>
                    </div>
                    <dl class="space-y-1 text-[12.5px] tabular-nums">
                      {#if num(r.receivable_cut) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.returns.cut')}</dt><dd>{money(r.receivable_cut)}</dd></div>{/if}
                      {#if num(r.refund) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.returns.refund')}{#if r.refund_method}<span class="ms-1 text-[var(--text-tertiary)]">{t('sales.detail.returns.via', { method: r.refund_method })}</span>{/if}</dt><dd>{money(r.refund)}</dd></div>{/if}
                    </dl>
                  </div>
                </li>
              {/each}
            </ul>
            <div class="mt-4 flex justify-end">
              <dl class="surface-card !p-4 w-full max-w-[360px] space-y-2 text-[12.5px] tabular-nums">
                <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.discounts.total')}</dt><dd class="font-medium">{money(d.total)}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.detail.returns.returned')} ({activeReturns})</dt><dd class="text-[var(--color-warning-600,#d97706)]">−{money(d.returned_total)}</dd></div>
                <div class="flex items-baseline justify-between gap-3 border-t border-[var(--border-subtle)] pt-3 text-[15px] font-bold"><dt>{t('sales.detail.returns.net')}</dt><dd>{money(d.net_total)}</dd></div>
                {#if d.returns.some((r) => r.status === 'void')}<p class="pt-1 text-[11px] text-[var(--text-tertiary)]">{t('sales.detail.returns.voidedNote')}</p>{/if}
              </dl>
            </div>
          {/if}
        {:else if tab === 'stock'}
          <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('sales.detail.stock.intro')}</p>
          {#if d.stock.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.stock.empty')}</p>
          {:else}
            <div class="surface-card !p-0 overflow-hidden">
              <div class="overflow-x-auto scroll-thin">
                <table class="w-full min-w-[560px] text-[12.5px]">
                  <thead>
                    <tr class="bg-[var(--surface-sunken)] text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.item')}</th>
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.type')}</th>
                      <th class="px-3 py-2.5 text-start" scope="col">{t('sales.detail.stock.bucket')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.stock.change')}</th>
                      <th class="px-3 py-2.5 text-end" scope="col">{t('sales.detail.stock.after')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each d.stock as m (m.id)}
                      <tr class="border-t border-[var(--border-subtle)]">
                        <td class="px-3 py-3"><div class="font-semibold">{m.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{m.sku}</div></td>
                        <td class="px-3 py-3"><span class="badge-soft {m.type === 'SALE' ? 'badge-info' : m.type === 'SALE_RETURN' ? 'badge-success' : 'badge-warning'}">{t(`sales.detail.stock.types.${m.type}`)}</span><div class="mt-1 text-[11px] text-[var(--text-tertiary)]">{formatDateTime(m.at)}</div></td>
                        <td class="px-3 py-3 whitespace-nowrap"><i class="{BUCKET_ICON[m.bucket]} me-1 text-[12px] text-[var(--text-tertiary)]"></i>{t(`sales.detail.stock.bucketNames.${m.bucket}`)}</td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums font-semibold {num(m.delta) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : 'text-[var(--color-success-600,#16a34a)]'}">{num(m.delta) > 0 ? '+' : ''}{qty(m.delta)} <span class="font-normal text-[var(--text-tertiary)]">{m.unit}</span></td>
                        <td class="px-3 py-3 text-end whitespace-nowrap tabular-nums {num(m.balance_after) < 0 ? 'text-[var(--color-danger-600,#dc2626)]' : ''}">{qty(m.balance_after)}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        {:else}
          <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('sales.detail.history.intro')}</p>
          {#if d.events.length === 0}
            <p class="rounded-lg border border-dashed border-[var(--border-subtle)] p-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('sales.detail.history.empty')}</p>
          {:else}
            <ol class="relative ms-3 space-y-5 border-s border-[var(--border-subtle)] ps-6">
              {#each d.events as ev, i (i)}
                <li class="relative">
                  <span class="absolute -start-[31px] top-0.5 inline-flex size-[11px] rounded-full border-2 border-[var(--surface-canvas)] bg-[var(--color-primary-600)] ring-1 ring-[var(--color-primary-600)]/40"></span>
                  <div class="flex flex-wrap items-baseline gap-x-2">
                    <span class="text-[13px] font-semibold">{actionLabel(ev.action)}</span>
                    <span class="text-[11.5px] text-[var(--text-tertiary)]">{formatDateTime(ev.at)}{#if ev.actor} · {t('sales.detail.history.by', { actor: ev.actor })}{/if}</span>
                  </div>
                  {#if evApprover(ev.details)}<span class="badge-soft badge-warning mt-1.5 inline-flex items-center gap-1"><i class="icon-shield-check text-[11px]"></i>{t('sales.detail.history.approver', { name: evApprover(ev.details) })}</span>{/if}
                  {#each evLines(ev.details) as el, j (j)}
                    <div class="mt-1.5 flex flex-wrap items-center gap-2 rounded-md bg-[var(--surface-sunken)] px-2.5 py-1.5 text-[12px]">
                      <span class="font-mono text-[11px]">{el.sku}</span>
                      {#if el.list_price !== undefined && el.price !== undefined}<span class="tabular-nums"><span class="text-[var(--text-tertiary)] line-through">{money(el.list_price)}</span> → <strong>{money(el.price)}</strong></span>{/if}
                      {#if el.discount !== undefined}<span class="tabular-nums">−{money(el.discount)}</span>{/if}
                    </div>
                  {/each}
                </li>
              {/each}
            </ol>
          {/if}
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if openReturn}
  <SaleReturnModal id={openReturn} onclose={() => (openReturn = null)} onchanged={() => { onchanged?.(); load(); }} />
{/if}

{#if voiding && d}
  <VoidSaleModal saleId={d.id} docNo={d.doc_no} onclose={() => (voiding = false)} ondone={() => { voiding = false; onchanged?.(); load(); }} />
{/if}

<style>
  .sd-fade {
    animation: sd-fade 160ms ease-out;
  }
  .sd-slide {
    animation: sd-slide 220ms cubic-bezier(0.22, 1, 0.36, 1);
  }
  @keyframes sd-fade {
    from {
      opacity: 0;
    }
  }
  @keyframes sd-slide {
    from {
      transform: translateX(32px);
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .sd-fade,
    .sd-slide {
      animation: none;
    }
  }
</style>
