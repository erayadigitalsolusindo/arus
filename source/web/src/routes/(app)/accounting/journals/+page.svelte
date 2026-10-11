<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import JournalModal from '#lib/components/JournalModal.svelte';
  import { accounting, todayISO, type JournalSummary } from '#lib/accounting/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let rows = $state<JournalSummary[]>([]);
  let cursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let type = $state('');
  let status = $state('');
  let q = $state('');
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
    if (n && ['JU', 'KM', 'KK', 'TK'].includes(n) && can('journals', 'create')) void goto(`/accounting/journals/new?type=${n}`, { replaceState: true });
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
  const sumPosted = $derived(rows.length - sumDraft);
  const more = $derived(cursor ? '+' : '');
  // Pencarian teks hanya menyaring baris yang sudah dimuat; jenis, status, dan tanggal disaring server.
  const needle = $derived(q.trim().toLowerCase());
  const shown = $derived(needle ? rows.filter((r) => `${r.doc_no} ${r.narration} ${r.debit_account} ${r.credit_account}`.toLowerCase().includes(needle)) : rows);
  const hasFilter = $derived(!!(type || status || q));
  function resetFilter() {
    type = '';
    status = '';
    q = '';
  }

  const typeOptions = $derived([
    { value: '', label: t('accounting.journals.allTypes') },
    ...(['JU', 'KM', 'KK', 'TK', 'OPENING'] as const).map((v) => ({ value: v as string, label: t(`accounting.type.${v}`) }))
  ]);
  const statusOptions = $derived([
    { value: '', label: t('accounting.journals.allStatus') },
    { value: 'draft', label: t('accounting.status.draft') },
    { value: 'posted', label: t('accounting.status.posted') }
  ]);

  const act = 'header-icon-btn !size-8';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="flex items-start gap-3 min-w-0">
      <span class="grid place-items-center size-11 rounded-xl shrink-0 bg-[color-mix(in_srgb,var(--color-primary)_14%,transparent)] text-[var(--color-primary)]"><i class="icon-book-open text-[22px]"></i></span>
      <div class="min-w-0">
        <h1 class="font-display font-bold text-[20px] leading-tight">{t('accounting.journals.title')}</h1>
        <p class="text-[12.5px] mt-0.5 max-w-2xl text-[var(--text-tertiary)]">{t('accounting.journals.subtitle')}</p>
      </div>
    </div>
    <div class="flex flex-wrap gap-2">
      {#if can('journals', 'approve')}<button type="button" class="btn" onclick={() => (editor = { id: '', opening: true })}><i class="icon-flag"></i> {t('accounting.journals.opening')}</button>{/if}
      {#if can('journals', 'create')}<a href="/accounting/journals/new" class="btn btn-primary"><i class="icon-plus"></i> {t('accounting.journals.add')}</a>{/if}
    </div>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <!-- Ringkasan dari jurnal yang dimuat -->
  <div class="grid gap-3 grid-cols-2 xl:grid-cols-4">
    {#each [
      { icon: 'icon-files', tone: 'var(--color-primary)', label: t('accounting.journals.kpiCount'), value: `${rows.length}${more}`, text: false },
      { icon: 'icon-banknote', tone: 'var(--color-info-600, #0284c7)', label: t('accounting.journals.kpiTotal'), value: formatCurrency(sumTotal), text: true },
      { icon: 'icon-pencil-line', tone: 'var(--color-warning-600, #d97706)', label: t('accounting.journals.kpiDraft'), value: String(sumDraft), text: false },
      { icon: 'icon-circle-check', tone: 'var(--color-success-600, #16a34a)', label: t('accounting.journals.kpiPosted'), value: String(sumPosted), text: false }
    ] as k (k.label)}
      <div class="surface-card !p-3.5 flex items-center gap-3 min-w-0" aria-busy={loading}>
        <span class="grid place-items-center size-10 rounded-lg shrink-0" style="background: color-mix(in srgb, {k.tone} 14%, transparent); color: {k.tone}"><i class="{k.icon} text-[19px]"></i></span>
        <div class="min-w-0">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] truncate">{k.label}</div>
          <div class="font-display font-bold leading-tight tabular-nums truncate {k.text ? 'text-[17px]' : 'text-[22px]'}">{loading && rows.length === 0 ? '—' : k.value}</div>
        </div>
      </div>
    {/each}
  </div>

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2.5 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.journals.range')} />
      <Select class="!w-auto min-w-40" ariaLabel={t('accounting.journals.type')} bind:value={type} options={typeOptions} />
      <Select class="!w-auto min-w-40" ariaLabel={t('accounting.journals.status')} bind:value={status} options={statusOptions} />
      <div class="relative grow min-w-[14rem] max-w-md">
        <i class="icon-search absolute start-3 top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)] pointer-events-none"></i>
        <input bind:value={q} placeholder={t('accounting.journals.search')} aria-label={t('accounting.journals.search')} autocomplete="off" class="field-control w-full text-[12.5px]" style="padding-inline-start:2.25rem" />
      </div>
      {#if hasFilter}
        <button type="button" class="btn btn-sm ms-auto" onclick={resetFilter}><i class="icon-rotate-ccw"></i> {t('accounting.journals.resetFilter')}</button>
      {/if}
    </div>

    {#if shown.length > 0}
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[900px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-2.5 text-start whitespace-nowrap" scope="col">{t('accounting.journals.col.date')}</th>
            <th class="px-3 py-2.5 text-start whitespace-nowrap" scope="col">{t('accounting.journals.col.doc')}</th>
            <th class="px-3 py-2.5 text-start" scope="col">{t('accounting.journals.col.narration')}</th>
            <th class="px-3 py-2.5 text-end whitespace-nowrap" scope="col">{t('accounting.journals.col.total')}</th>
            <th class="px-3 py-2.5 text-start whitespace-nowrap" scope="col">{t('accounting.journals.col.status')}</th>
            <th class="px-3 py-2.5 w-px"><span class="sr-only">{t('accounting.journals.edit')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as r (r.id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)] {r.status === 'draft' ? 'shadow-[inset_3px_0_0_var(--color-warning-600,#d97706)]' : ''}">
              <td class="px-4 py-2.5 whitespace-nowrap tabular-nums align-top">
                <div class="font-semibold">{formatDate(r.date, { day: '2-digit', month: 'short', year: 'numeric' })}</div>
                {#if r.created_by}<div class="text-[11px] text-[var(--text-tertiary)]">{r.created_by}</div>{/if}
              </td>
              <td class="px-3 py-2.5 whitespace-nowrap align-top">
                <button type="button" class="font-mono text-[12px] font-bold text-[var(--color-primary-600)] hover:underline" onclick={() => (editor = { id: r.id, opening: false })}>{r.doc_no || t('accounting.journals.draftNo')}</button>
                <div class="mt-1 flex items-center gap-1">
                  <span class="badge-soft {typeBadge[r.type] ?? 'badge-info'} !text-[10px]">{t(`accounting.type.${r.type}`)}</span>
                  {#if r.source}<span class="badge-soft badge-info !text-[10px]"><i class="icon-zap text-[10px]"></i> {t('accounting.journals.auto')}</span>{/if}
                </div>
              </td>
              <td class="px-3 py-2.5 max-w-[30rem] align-top">
                <div class="line-clamp-1 font-medium" title={r.narration}>{r.narration || '—'}</div>
                {#if r.debit_account || r.credit_account}
                  <div class="mt-1 flex items-center gap-1.5 text-[11px] text-[var(--text-tertiary)]">
                    <span class="truncate max-w-[12rem]" title={r.debit_account}>{r.debit_account}</span>
                    <i class="icon-arrow-right text-[10px] shrink-0"></i>
                    <span class="truncate max-w-[12rem]" title={r.credit_account}>{r.credit_account}</span>
                    <span class="shrink-0 rounded bg-[var(--surface-sunken)] px-1.5">{t('accounting.journals.lineCount', { n: r.lines })}</span>
                  </div>
                {/if}
              </td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap font-bold align-top">{formatCurrency(Number(r.total))}</td>
              <td class="px-3 py-2.5 whitespace-nowrap align-top">
                <span class="inline-flex items-center gap-1.5 font-medium">
                  <span class="size-2 rounded-full {r.status === 'posted' ? 'bg-[var(--color-success-600,#16a34a)]' : 'bg-[var(--color-warning-600,#d97706)]'}"></span>{t(`accounting.status.${r.status}`)}
                </span>
                {#if r.reversed || r.is_reversal}
                  <div class="mt-1 flex gap-1">
                    {#if r.reversed}<span class="badge-soft badge-warning !text-[10px]">{t('accounting.journals.reversed')}</span>{/if}
                    {#if r.is_reversal}<span class="badge-soft badge-warning !text-[10px]">{t('accounting.journals.reversal')}</span>{/if}
                  </div>
                {/if}
              </td>
              <td class="px-3 py-2 text-end whitespace-nowrap align-top">
                <div class="inline-flex gap-0.5">
                  {#if r.status === 'draft'}
                    {#if can('journals', 'update')}<button type="button" class={act} title={t('accounting.journals.edit')} aria-label={t('accounting.journals.edit')} onclick={() => (editor = { id: r.id, opening: false })}><i class="icon-pencil text-[15px]"></i></button>{/if}
                    {#if can('journals', 'approve')}<button type="button" class="{act} hover:!text-[var(--color-success-600)]" title={t('accounting.journals.post')} aria-label={t('accounting.journals.post')} onclick={() => post(r)}><i class="icon-circle-check text-[15px]"></i></button>{/if}
                    {#if can('journals', 'delete')}<button type="button" class="{act} hover:!text-[var(--color-danger-600)]" title={t('accounting.journals.remove')} aria-label={t('accounting.journals.remove')} onclick={() => remove(r)}><i class="icon-trash-2 text-[15px]"></i></button>{/if}
                  {:else}
                    <button type="button" class={act} title={t('accounting.journals.view')} aria-label={t('accounting.journals.view')} onclick={() => (editor = { id: r.id, opening: false })}><i class="icon-eye text-[15px]"></i></button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {:else if loading}
      <div class="space-y-2.5 px-6 py-10 max-w-2xl mx-auto" aria-hidden="true">
        {#each [0, 1, 2] as n (n)}<div class="h-10 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>{/each}
      </div>
    {:else}
      <div class="flex flex-col items-center px-6 py-16 text-center">
        <span class="grid size-16 place-items-center rounded-full bg-[color-mix(in_srgb,var(--color-primary)_10%,transparent)] text-[var(--color-primary)]"><i class="icon-book-open" style="font-size:28px"></i></span>
        <h2 class="mt-4 font-display font-bold text-[16px]">{needle && rows.length ? t('accounting.journals.noMatch') : t('accounting.journals.emptyTitle')}</h2>
        {#if !(needle && rows.length)}<p class="mt-1 max-w-sm text-[12.5px] leading-relaxed text-[var(--text-tertiary)]">{t('accounting.journals.emptyHint')}</p>{/if}
        <div class="mt-5 flex flex-wrap justify-center gap-2">
          {#if hasFilter}<button type="button" class="btn btn-sm" onclick={resetFilter}><i class="icon-rotate-ccw"></i> {t('accounting.journals.resetFilter')}</button>{/if}
          {#if can('journals', 'create')}<a href="/accounting/journals/new" class="btn btn-sm btn-primary"><i class="icon-plus"></i> {t('accounting.journals.add')}</a>{/if}
        </div>
      </div>
    {/if}
    {#if shown.length > 0}
    <div class="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
      <span>{t('accounting.journals.showing', { n: shown.length })}{more}</span>
      {#if cursor}
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('accounting.journals.loadMore')}</button>
      {/if}
    </div>
    {/if}
  </div>
</main>

{#if editor}
  <JournalModal id={editor.id} opening={editor.opening} initialType={editor.type} onclose={() => (editor = null)} onchanged={() => load()} />
{/if}
