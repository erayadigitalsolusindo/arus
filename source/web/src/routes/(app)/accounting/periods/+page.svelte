<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { accounting, todayISO, type Journal, type Period } from '#lib/accounting/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let rows = $state<Period[]>([]);
  let loading = $state(true);
  let error = $state('');
  let busy = $state(false);
  let year = $state(Number(todayISO().slice(0, 4)));
  let yearDraft = $state<Journal | null>(null);
  let closing = $state<string | null>(null); // YYYY-MM yang akan ditutup
  let reopening = $state<Period | null>(null);
  let reason = $state('');

  const month = (iso: string) => iso.slice(0, 7); // YYYY-MM
  const thisMonth = todayISO().slice(0, 7);

  const byMonth = $derived(new Map(rows.map((p) => [month(p.start_date), p])));
  const closedCount = $derived(rows.filter((p) => p.status === 'closed').length);
  const lastClosed = $derived(
    rows
      .filter((p) => p.status === 'closed')
      .map((p) => month(p.start_date))
      .sort()
      .at(-1)
  );
  const years = $derived.by(() => {
    const cur = Number(thisMonth.slice(0, 4));
    const set = new Set<number>([cur, year, ...rows.map((p) => Number(p.start_date.slice(0, 4)))]);
    return [...set].sort((a, b) => b - a);
  });
  const months = $derived(Array.from({ length: 12 }, (_, i) => `${year}-${String(i + 1).padStart(2, '0')}`));
  const yearClosed = $derived(months.filter((m) => byMonth.get(m)?.status === 'closed').length);

  /** Tutup buku berikutnya yang disarankan: bulan setelah bulan tertutup terakhir, tetapi tak melewati bulan berjalan. */
  const suggested = $derived.by(() => {
    let next: string;
    if (lastClosed) {
      const [y, m] = lastClosed.split('-').map(Number);
      next = m === 12 ? `${y + 1}-01` : `${y}-${String(m + 1).padStart(2, '0')}`;
    } else {
      const open = rows.filter((p) => p.status === 'open').map((p) => month(p.start_date)).sort();
      next = open[0] ?? thisMonth;
    }
    return next <= thisMonth ? next : null;
  });

  const monthName = (m: string) => new Date(`${m}-01T00:00:00`).toLocaleDateString(undefined, { month: 'long' });
  const monthLabel = (m: string) => new Date(`${m}-01T00:00:00`).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });

  async function load() {
    loading = true;
    try {
      rows = await accounting.periods();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    document.title = t('accounting.periods.docTitle');
    void load();
  });

  async function doClose() {
    if (!closing || busy) return;
    busy = true;
    error = '';
    try {
      await accounting.closePeriod(closing);
      closing = null;
      await load();
    } catch (e) {
      closing = null;
      error = errorMessage(e);
    } finally {
      busy = false;
    }
  }

  async function reopen(e: Event) {
    e.preventDefault();
    if (!reopening || busy) return;
    busy = true;
    error = '';
    try {
      await accounting.reopenPeriod(month(reopening.start_date), reason.trim());
      reopening = null;
      reason = '';
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  async function createYearEndDraft() {
    if (busy) return;
    busy = true;
    error = '';
    yearDraft = null;
    try {
      yearDraft = await accounting.createYearEndDraft(year);
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
  const inputClass = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.periods.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.periods.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <!-- Ringkasan -->
  <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
    <div class="surface-card p-5 flex items-center gap-4">
      <span class="grid place-items-center size-10 rounded-lg badge-danger"><i class="icon-lock text-[18px]"></i></span>
      <div>
        <div class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('accounting.periods.kpi.closed')}</div>
        <div class="font-display font-bold text-[20px] leading-tight">{closedCount}</div>
      </div>
    </div>
    <div class="surface-card p-5 flex items-center gap-4">
      <span class="grid place-items-center size-10 rounded-lg badge-success"><i class="icon-calendar-check text-[18px]"></i></span>
      <div>
        <div class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('accounting.periods.kpi.lastClosed')}</div>
        <div class="font-display font-bold text-[20px] leading-tight">{lastClosed ? monthLabel(lastClosed) : '—'}</div>
      </div>
    </div>
    <div class="surface-card p-5 flex items-center gap-4">
      <span class="grid place-items-center size-10 rounded-lg bg-[var(--surface-sunken)]"><i class="icon-calendar-clock text-[18px]"></i></span>
      <div>
        <div class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('accounting.periods.kpi.next')}</div>
        <div class="font-display font-bold text-[20px] leading-tight">{suggested ? monthLabel(suggested) : '—'}</div>
      </div>
      {#if suggested && can('accounting_periods', 'update')}
        <button type="button" class="btn btn-sm btn-primary ms-auto" disabled={busy} onclick={() => (closing = suggested)}>{t('accounting.periods.close')}</button>
      {/if}
    </div>
  </div>

  <!-- Kalender periode -->
  <div class="surface-card p-5 space-y-4">
    <div class="flex flex-wrap items-center gap-3">
      <div>
        <div class="font-semibold text-[14px]">{t('accounting.periods.calendar')}</div>
        <div class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.periods.yearSummary', { closed: yearClosed, year: String(year) })}</div>
      </div>
      <div class="ms-auto flex items-center gap-1">
        <button type="button" class="btn btn-sm" aria-label={t('accounting.periods.prevYear')} onclick={() => (year -= 1)}><i class="icon-chevron-left"></i></button>
        <select class="h-8 px-2 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] font-semibold" bind:value={year} aria-label={t('accounting.periods.year')}>
          {#each years as y (y)}<option value={y}>{y}</option>{/each}
        </select>
        <button type="button" class="btn btn-sm" aria-label={t('accounting.periods.nextYear')} onclick={() => (year += 1)}><i class="icon-chevron-right"></i></button>
      </div>
      {#if year < Number(thisMonth.slice(0, 4)) && yearClosed === 12 && can('journals', 'create')}
        <button type="button" class="btn btn-sm btn-primary" disabled={busy} onclick={createYearEndDraft}><i class="icon-book-check"></i> {t('accounting.periods.createYearEnd')}</button>
      {/if}
    </div>

    {#if yearDraft}
      <div role="status" class="flex flex-wrap items-center justify-between gap-2 rounded border border-[var(--color-success-300,var(--border-subtle))] bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px]">
        <span>{t('accounting.periods.yearEndDraftReady', { year: yearDraft.date.slice(0, 4) })}</span>
        <a class="btn btn-sm" href="/accounting/journals">{t('accounting.periods.openJournals')} <i class="icon-arrow-right"></i></a>
      </div>
    {/if}

    <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
      {#each months as m (m)}
        {@const p = byMonth.get(m)}
        {@const isClosed = p?.status === 'closed'}
        {@const future = m > thisMonth}
        <div class="rounded-lg border p-4 flex flex-col gap-3 {isClosed ? 'border-[var(--color-danger-300,var(--border-default))] bg-[var(--surface-sunken)]' : 'border-[var(--border-subtle)]'} {m === thisMonth ? 'ring-1 ring-[var(--color-primary-500)]' : ''}">
          <div class="flex items-center justify-between gap-2">
            <div class="font-semibold text-[13px] capitalize">{monthName(m)}</div>
            {#if isClosed}
              <span class="badge-soft badge-danger"><i class="icon-lock text-[11px]"></i> {t('accounting.periods.closed')}</span>
            {:else if future}
              <span class="badge-soft">{t('accounting.periods.future')}</span>
            {:else if p}
              <span class="badge-soft badge-success">{t('accounting.periods.open')}</span>
            {:else}
              <span class="badge-soft">{t('accounting.periods.unused')}</span>
            {/if}
          </div>
          <div class="text-[11px] text-[var(--text-tertiary)] min-h-[1.4em]">
            {#if isClosed && p?.closed_at}
              {t('accounting.periods.closedOn', { at: formatDateTime(p.closed_at, { dateStyle: 'medium', timeStyle: 'short' }) })}
            {:else if m === thisMonth}
              {t('accounting.periods.current')}
            {/if}
          </div>
          <div class="mt-auto">
            {#if isClosed && p && can('accounting_periods', 'approve')}
              <button type="button" class="btn btn-sm w-full" disabled={busy} onclick={() => (reopening = p)}><i class="icon-lock-open"></i> {t('accounting.periods.reopen')}</button>
            {:else if !isClosed && !future && can('accounting_periods', 'update')}
              <button type="button" class="btn btn-sm w-full" disabled={busy} onclick={() => (closing = m)}><i class="icon-lock"></i> {t('accounting.periods.close')}</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
    {#if loading}<div class="text-center text-[12px] text-[var(--text-tertiary)]">…</div>{/if}
  </div>

  <!-- Penjelasan -->
  <div class="surface-card p-5">
    <div class="font-semibold text-[13px] mb-3">{t('accounting.periods.rulesTitle')}</div>
    <ul class="space-y-1.5 text-[12.5px] text-[var(--text-secondary)]">
      <li class="flex gap-2"><i class="icon-check text-[14px] mt-0.5 shrink-0"></i>{t('accounting.periods.rule1')}</li>
      <li class="flex gap-2"><i class="icon-check text-[14px] mt-0.5 shrink-0"></i>{t('accounting.periods.rule2')}</li>
      <li class="flex gap-2"><i class="icon-check text-[14px] mt-0.5 shrink-0"></i>{t('accounting.periods.rule3')}</li>
    </ul>
  </div>
</main>

{#if closing}
  <Modal title={t('accounting.periods.closeTitle', { period: monthLabel(closing) })} onclose={() => (closing = null)}>
    <div class="space-y-3 text-[13px]">
      <p>{t('accounting.periods.closeConfirm', { period: monthLabel(closing) })}</p>
      <ul class="space-y-1.5 text-[12.5px] text-[var(--text-secondary)]">
        <li class="flex gap-2"><i class="icon-triangle-alert text-[14px] mt-0.5 shrink-0"></i>{t('accounting.periods.rule1')}</li>
        <li class="flex gap-2"><i class="icon-triangle-alert text-[14px] mt-0.5 shrink-0"></i>{t('accounting.periods.rule2')}</li>
      </ul>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={() => (closing = null)} disabled={busy}>{t('accounting.periods.cancel')}</button>
        <button type="button" class="btn btn-primary" onclick={doClose} disabled={busy}><i class="icon-lock"></i> {t('accounting.periods.close')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if reopening}
  <Modal title={t('accounting.periods.reopenTitle', { period: monthLabel(month(reopening.start_date)) })} onclose={() => (reopening = null)}>
    <form class="space-y-3 text-[13px]" onsubmit={reopen}>
      <label class="block">
        <span class="font-semibold">{t('accounting.periods.reason')}</span>
        <input bind:value={reason} maxlength="200" class={inputClass} />
      </label>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={() => (reopening = null)} disabled={busy}>{t('accounting.periods.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={busy || reason.trim().length < 3}>{t('accounting.periods.reopenDo')}</button>
      </div>
    </form>
  </Modal>
{/if}
