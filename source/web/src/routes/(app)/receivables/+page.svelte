<script lang="ts">
  import { onMount } from 'svelte';
  import Select from '#lib/components/Select.svelte';
  import SettleModal from '#lib/components/SettleModal.svelte';
  import ReceivableModal from '#lib/components/ReceivableModal.svelte';
  import { receivables as api, type Receivable, type ReceivableFilter, type ReceivableSummary } from '#lib/receivables/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const PAGE = 50;
  const money = (v: string | number) => formatCurrency(Number(v));
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const statusClass = { open: 'badge-info', overdue: 'badge-danger', paid: 'badge-success' } as const;

  let rows = $state<Receivable[]>([]);
  let summary = $state<ReceivableSummary | null>(null);
  let hasMore = $state(false);
  let loading = $state(true);
  let error = $state('');
  let q = $state('');
  let status = $state<ReceivableFilter>('open');
  let openId = $state<string | null>(null);
  let settling = $state(false);

  const statusOptions = $derived((['open', 'overdue', 'paid', 'all'] as const).map((s) => ({ value: s, label: t(`receivables.status.${s}`) })));
  const canPay = $derived(can('member_receivables', 'create'));

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ status, q: q.trim(), limit: PAGE, offset: more ? rows.length : 0 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      summary = res.summary;
      hasMore = res.has_more;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void status;
    void q;
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    document.title = t('receivables.docTitle');
  });
</script>

{#snippet stat(icon: string, tone: string, label: string, value: string, valueClass: string = '')}
  <div class="surface-card !p-4 flex items-start gap-3">
    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-xl {tone}"><i class="{icon} text-[18px]"></i></span>
    <div class="min-w-0">
      <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{label}</div>
      <div class="mt-0.5 truncate font-display text-[20px] font-bold leading-tight tabular-nums {valueClass}">{value}</div>
    </div>
  </div>
{/snippet}

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('receivables.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('receivables.subtitle')}</p>
    </div>
    {#if canPay}
      <button type="button" class="btn btn-primary" onclick={() => (settling = true)}><i class="icon-banknote me-1"></i>{t('settle.receivable.button')}</button>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('receivables.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if summary}
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      {@render stat('icon-wallet', 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]', t('receivables.stat.outstanding'), money(summary.outstanding))}
      {@render stat('icon-alarm-clock', 'bg-[var(--color-danger-600,#dc2626)]/10 text-[var(--color-danger-600,#dc2626)]', t('receivables.stat.overdue'), money(summary.overdue), Number(summary.overdue) > 0 ? 'text-[var(--color-danger-600,#dc2626)]' : '')}
      {@render stat('icon-receipt-text', 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]', t('receivables.stat.open'), formatNumber(summary.open_count))}
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-end gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('receivables.search')} aria-label={t('receivables.search')} bind:value={q} maxlength="100" />
      </div>
      <Select class="!w-auto min-w-44" ariaLabel={t('receivables.col.status')} bind:value={status} options={statusOptions} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[860px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('receivables.col.docNo')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('receivables.col.member')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('receivables.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('receivables.col.due')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('receivables.col.amount')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('receivables.col.paid')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('receivables.col.balance')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('receivables.col.status')}</th>
            <th class="px-3 py-3 text-end" scope="col"><span class="sr-only">{t('receivables.col.action')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] align-middle last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-3 font-mono text-[12px] font-bold whitespace-nowrap">{r.doc_no}</td>
              <td class="px-3 py-3"><div class="font-medium">{r.member_name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.member_code}</div></td>
              <td class="px-3 py-3 whitespace-nowrap">{formatDate(r.created_at)}</td>
              <td class="px-3 py-3 whitespace-nowrap">{r.due_date ? formatDate(r.due_date) : t('receivables.noDue')}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.amount)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap">{money(r.paid)}</td>
              <td class="px-3 py-3 text-end tabular-nums whitespace-nowrap font-bold {r.status === 'paid' ? '' : 'text-[var(--color-danger-600)]'}">{money(r.balance)}</td>
              <td class="px-3 py-3"><span class="badge-soft {statusClass[r.status]}">{t(`receivables.status.${r.status}`)}</span></td>
              <td class="px-3 py-3 text-end whitespace-nowrap">
                <button type="button" class="btn btn-sm {r.status !== 'paid' && canPay ? 'btn-primary' : ''}" onclick={() => (openId = r.id)}>{r.status !== 'paid' && canPay ? t('receivables.pay') : t('receivables.detail')}</button>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-16 text-center text-[var(--text-tertiary)]">
                {#if loading}…{:else if q || status !== 'open'}{t('receivables.emptySearch')}{:else}{t('receivables.empty')}{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hasMore}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('receivables.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if settling}
  <SettleModal kind="receivable" onclose={() => (settling = false)} onchanged={() => load()} />
{/if}

{#if openId}
  <ReceivableModal id={openId} onclose={() => (openId = null)} onchanged={() => load()} />
{/if}
