<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Switch from '#lib/components/Switch.svelte';
  import ShiftDetailModal from '#lib/components/ShiftDetailModal.svelte';
  import ShiftCloseModal from '#lib/components/ShiftCloseModal.svelte';
  import { shifts as api, type ShiftListRow } from '#lib/shift/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { signedCents } from '#lib/pos/report-receipts.ts';

  const money = (s: string | null | undefined) => (s === null || s === undefined ? '—' : formatCurrency(Number(signedCents(s)) / 100));
  const signedFmt = (s: string | null | undefined) => {
    if (s === null || s === undefined) return '—';
    const c = signedCents(s);
    return (c > 0n ? '+' : '') + formatCurrency(Number(c) / 100);
  };
  const dt = (iso?: string) => (iso ? formatDateTime(iso, { dateStyle: 'short', timeStyle: 'short' }) : '—');

  let rows = $state<ShiftListRow[]>([]);
  let cashiers = $state<{ id: string; name: string }[]>([]);
  let cursor = $state('');
  let hasMore = $state(false);
  let loading = $state(true);
  let error = $state('');
  let from = $state('');
  let to = $state('');
  let userId = $state('');
  let status = $state('');
  let diffOnly = $state(false);
  let openId = $state<string | null>(null);
  let closeId = $state<string | null>(null);
  const canClose = $derived(can('cash_shifts', 'update'));

  let seq = 0;
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ from, to, user_id: userId, status, diff: diffOnly, cursor: more ? cursor : '', limit: 50 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      cashiers = res.cashiers;
      cursor = res.next_cursor ?? '';
      hasMore = res.has_more;
      from = res.from;
      to = res.to;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Filter berubah (dan outlet aktif berpindah) → muat dari awal.
  $effect(() => {
    void userId;
    void status;
    void diffOnly;
    void session.outlet?.id;
    void load();
  });

  onMount(() => {
    document.title = `${t('shift.list.title')} | ACIRABA`;
  });

  const cashierOptions = $derived([{ value: '', label: t('shift.list.allCashiers') }, ...cashiers.map((c) => ({ value: c.id, label: c.name }))]);
  const statusOptions = $derived([
    { value: '', label: t('shift.list.allStatus') },
    { value: 'open', label: t('shift.list.statusOpen') },
    { value: 'closed', label: t('shift.list.statusClosed') }
  ]);
  const diffClass = (s: string | null) => {
    const c = signedCents(s);
    return c === 0n ? '' : c < 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-warning-700)]';
  };
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('shift.list.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('shift.list.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('shift.list.range')} />
      <Select class="!w-auto min-w-44" ariaLabel={t('shift.list.cashier')} bind:value={userId} options={cashierOptions} />
      <Select class="!w-auto min-w-40" ariaLabel={t('shift.list.status')} bind:value={status} options={statusOptions} />
      <Switch bind:checked={diffOnly} label={t('shift.list.diffOnly')} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[980px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('shift.list.col.doc')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('shift.list.col.cashier')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('shift.list.col.opened')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('shift.list.col.closed')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('shift.list.col.sales')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('shift.list.col.expected')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('shift.list.col.counted')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('shift.list.col.diff')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('shift.list.col.note')}</th>
            <th class="px-3 py-3"><span class="sr-only">{t('shift.list.detail')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-3 whitespace-nowrap">
                <button type="button" class="font-mono text-[12px] font-bold text-[var(--color-primary-600)] hover:underline" onclick={() => (openId = r.id)}>{r.doc_no}</button>
                {#if r.status === 'open'}<span class="badge-soft badge-info ms-1">{t('shift.list.statusOpen')}</span>{/if}
              </td>
              <td class="px-3 py-3">{r.user_name}</td>
              <td class="px-3 py-3 whitespace-nowrap">{dt(r.opened_at)}</td>
              <td class="px-3 py-3 whitespace-nowrap">{dt(r.closed_at)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{r.sales_total === null ? '—' : money(r.sales_total)}{#if r.sale_count !== null}<div class="text-[11px] text-[var(--text-tertiary)]">{r.sale_count}×</div>{/if}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.expected_total)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.counted_total)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold {diffClass(r.diff_total)}">
                {signedFmt(r.diff_total)}
                {#if r.diff_abs !== null && signedCents(r.diff_abs) !== signedCents(r.diff_total) && signedCents(r.diff_abs) !== -signedCents(r.diff_total)}<div class="text-[11px] font-normal text-[var(--text-tertiary)]">Σ|±| {money(r.diff_abs)}</div>{/if}
              </td>
              <td class="px-3 py-3 max-w-64">
                {#if r.note}<div class="line-clamp-2" title={r.note}>{r.note}</div>{/if}
                {#if r.approved_by_name}<div class="text-[11px] text-[var(--text-tertiary)]">{t('shift.list.approvedBy', { name: r.approved_by_name })}</div>{/if}
              </td>
              <td class="px-3 py-3 text-end whitespace-nowrap">
                {#if r.status === 'open' && canClose}
                  <button type="button" class="btn btn-sm" onclick={() => (closeId = r.id)}><i class="icon-lock"></i> {t('shift.close')}</button>
                {:else}
                  <button type="button" class="btn btn-sm" onclick={() => (openId = r.id)}>{t('shift.list.detail')}</button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="10" class="px-4 py-16 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('shift.list.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasMore}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('shift.list.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if openId}
  <ShiftDetailModal id={openId} onclose={() => (openId = null)} />
{/if}

{#if closeId}
  <ShiftCloseModal shiftId={closeId} onclose={() => (closeId = null)} onclosed={() => load()} />
{/if}
