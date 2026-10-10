<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import JournalModal from '#lib/components/JournalModal.svelte';
  import { accounting, todayISO, type JournalSummary } from '#lib/accounting/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let rows = $state<JournalSummary[]>([]);
  let cursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let type = $state('');
  let status = $state('');
  let editor = $state<{ id: string; opening: boolean; type?: string } | null>(null);

  let seq = 0;
  async function load(more = false) {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.journals({ from, to, type, status, cursor: more ? cursor : '', limit: 50 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.items] : res.items;
      cursor = res.next_cursor ?? '';
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void type;
    void status;
    void session.outlet?.id;
    void load();
  });
  onMount(() => {
    document.title = t('accounting.journals.docTitle');
    // Pintasan dari halaman Kas dan Bank: ?new=KM|KK|TK membuka editor jurnal baru.
    const n = page.url.searchParams.get('new');
    if (n && ['JU', 'KM', 'KK', 'TK'].includes(n) && can('journals', 'create')) editor = { id: '', opening: false, type: n };
  });

  async function post(r: JournalSummary) {
    if (!confirm(t('accounting.journals.postConfirm'))) return;
    error = '';
    try {
      await accounting.postJournal(r.id);
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
  }
  async function remove(r: JournalSummary) {
    if (!confirm(t('accounting.journals.removeConfirm'))) return;
    error = '';
    try {
      await accounting.deleteJournal(r.id);
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  const typeBadge: Record<string, string> = { JU: 'badge-info', KM: 'badge-success', KK: 'badge-danger', TK: 'badge-warning', OPENING: 'badge-warning' };
  const sumTotal = $derived(rows.reduce((a, r) => a + Number(r.total), 0));
  const sumDraft = $derived(rows.filter((r) => r.status === 'draft').length);

  const typeOptions = $derived([
    { value: '', label: t('accounting.journals.allTypes') },
    ...(['JU', 'KM', 'KK', 'TK', 'OPENING'] as const).map((v) => ({ value: v as string, label: t(`accounting.type.${v}`) }))
  ]);
  const statusOptions = $derived([
    { value: '', label: t('accounting.journals.allStatus') },
    { value: 'draft', label: t('accounting.status.draft') },
    { value: 'posted', label: t('accounting.status.posted') }
  ]);
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('accounting.journals.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.journals.subtitle')}</p>
    </div>
    <div class="flex gap-2">
      {#if can('journals', 'approve')}<button type="button" class="btn" onclick={() => (editor = { id: '', opening: true })}><i class="icon-flag"></i> {t('accounting.journals.opening')}</button>{/if}
      {#if can('journals', 'create')}<button type="button" class="btn btn-primary" onclick={() => (editor = { id: '', opening: false })}><i class="icon-plus"></i> {t('accounting.journals.add')}</button>{/if}
    </div>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.journals.range')} />
      <Select class="!w-auto min-w-40" ariaLabel={t('accounting.journals.type')} bind:value={type} options={typeOptions} />
      <Select class="!w-auto min-w-40" ariaLabel={t('accounting.journals.status')} bind:value={status} options={statusOptions} />
    </div>

    <div class="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2 border-b border-[var(--border-subtle)] text-[12px] text-[var(--text-secondary)]">
      <span class="font-semibold text-[var(--text-primary)]">{t('accounting.journals.sumCount', { n: rows.length })}{cursor ? '+' : ''}</span>
      <span>{t('accounting.journals.sumDraft', { n: sumDraft })}</span>
      <span>{t('accounting.journals.sumTotal')}: <b class="tabular-nums text-[var(--text-primary)]">{formatCurrency(sumTotal)}</b></span>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[980px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-2.5 text-start" scope="col">{t('accounting.journals.col.doc')}</th>
            <th class="px-3 py-2.5 text-start" scope="col">{t('accounting.journals.col.date')}</th>
            <th class="px-3 py-2.5 text-start" scope="col">{t('accounting.journals.col.type')}</th>
            <th class="px-3 py-2.5 text-start" scope="col">{t('accounting.journals.col.narration')}</th>
            <th class="px-3 py-2.5 text-end" scope="col">{t('accounting.journals.col.total')}</th>
            <th class="px-3 py-2.5 text-start" scope="col">{t('accounting.journals.col.status')}</th>
            <th class="px-3 py-2.5"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5">
              <td class="px-4 py-2 whitespace-nowrap">
                <button type="button" class="font-mono text-[12px] font-bold text-[var(--color-primary-600)] hover:underline" onclick={() => (editor = { id: r.id, opening: false })}>{r.doc_no || t('accounting.journals.draftNo')}</button>
                {#if r.source}<div class="mt-0.5"><span class="badge-soft badge-info !text-[10px]"><i class="icon-zap text-[10px]"></i> {t('accounting.journals.auto')}</span></div>{/if}
              </td>
              <td class="px-3 py-2 whitespace-nowrap tabular-nums">{r.date}</td>
              <td class="px-3 py-2 whitespace-nowrap"><span class="badge-soft {typeBadge[r.type] ?? 'badge-info'}">{t(`accounting.type.${r.type}`)}</span></td>
              <td class="px-3 py-2 max-w-[26rem]">
                <div class="line-clamp-1 font-medium" title={r.narration}>{r.narration || '—'}</div>
                {#if r.debit_account || r.credit_account}
                  <div class="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11px] text-[var(--text-tertiary)]">
                    <span class="truncate max-w-[11rem]" title={r.debit_account}>{r.debit_account}</span>
                    <i class="icon-arrow-right text-[10px]"></i>
                    <span class="truncate max-w-[11rem]" title={r.credit_account}>{r.credit_account}</span>
                    <span class="rounded bg-[var(--surface-sunken)] px-1.5">{t('accounting.journals.lineCount', { n: r.lines })}</span>
                  </div>
                {/if}
              </td>
              <td class="px-3 py-2 text-end tabular-nums whitespace-nowrap font-semibold">{formatCurrency(Number(r.total))}</td>
              <td class="px-3 py-2 whitespace-nowrap">
                <span class="badge-soft {r.status === 'posted' ? 'badge-success' : 'badge-info'}">{t(`accounting.status.${r.status}`)}</span>
                {#if r.reversed}<span class="badge-soft badge-warning ms-1">{t('accounting.journals.reversed')}</span>{/if}
                {#if r.is_reversal}<span class="badge-soft badge-warning ms-1">{t('accounting.journals.reversal')}</span>{/if}
                {#if r.created_by}<div class="mt-0.5 text-[11px] text-[var(--text-tertiary)]">{r.created_by}</div>{/if}
              </td>
              <td class="px-3 py-2 text-end whitespace-nowrap">
                {#if r.status === 'draft'}
                  {#if can('journals', 'update')}<button type="button" class="btn btn-sm" onclick={() => (editor = { id: r.id, opening: false })}>{t('accounting.journals.edit')}</button>{/if}
                  {#if can('journals', 'approve')}<button type="button" class="btn btn-sm" onclick={() => post(r)}>{t('accounting.journals.post')}</button>{/if}
                  {#if can('journals', 'delete')}<button type="button" class="btn btn-sm" onclick={() => remove(r)}>{t('accounting.journals.remove')}</button>{/if}
                {:else}
                  <button type="button" class="btn btn-sm" onclick={() => (editor = { id: r.id, opening: false })}>{t('accounting.journals.view')}</button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="7" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.journals.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if cursor}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('accounting.journals.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if editor}
  <JournalModal id={editor.id} opening={editor.opening} initialType={editor.type} onclose={() => (editor = null)} onchanged={() => load()} />
{/if}
