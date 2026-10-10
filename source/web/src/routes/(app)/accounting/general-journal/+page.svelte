<script lang="ts">
  import { onMount } from 'svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type GeneralJournalEntry } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string) => (Number(s) ? formatCurrency(Number(s)) : '');

  let from = $state(todayISO().slice(0, 8) + '01');
  let to = $state(todayISO());
  let rows = $state<GeneralJournalEntry[]>([]);
  let cursor = $state('');
  let loading = $state(false);
  let error = $state('');

  let seq = 0;
  async function load(more = false) {
    if (!from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.generalJournal({ from, to, cursor: more ? cursor : '', limit: 30 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.items] : res.items;
      cursor = res.next_cursor ?? '';
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(() => {
    document.title = t('accounting.gj.docTitle');
  });
  $effect(() => {
    void session.outlet?.id;
    void load();
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.gj.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.gj.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.reports.range')} />
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[760px] text-[12.5px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('accounting.journals.col.date')}</th>
            <th class="px-3 py-3 text-start" scope="col">{t('accounting.reports.code')} / {t('accounting.reports.name')}</th>
            <th class="px-3 py-3 text-end" scope="col">{t('accounting.journals.debit')}</th>
            <th class="px-4 py-3 text-end" scope="col">{t('accounting.journals.credit')}</th>
          </tr>
        </thead>
        {#each rows as e (e.id)}
          <tbody class="border-b border-[var(--border-default)]">
            <tr class="bg-[var(--surface-sunken)]/60">
              <td class="px-4 py-2 whitespace-nowrap font-semibold">{e.date}</td>
              <td class="px-3 py-2" colspan="3">
                <span class="font-mono font-bold">{e.doc_no}</span>
                <span class="ms-2 text-[var(--text-tertiary)]">{t(`accounting.type.${e.type}`)}</span>
                {#if e.narration}<span class="ms-2">— {e.narration}</span>{/if}
              </td>
            </tr>
            {#each e.lines as l (l.line_no)}
              <tr>
                <td></td>
                <td class="py-1.5 {Number(l.credit) ? 'ps-10 pe-3' : 'px-3'}">
                  <span class="font-mono">{l.account_code}</span> {l.account_name}{l.memo ? ` — ${l.memo}` : ''}
                </td>
                <td class="px-3 py-1.5 text-end tabular-nums">{money(l.debit)}</td>
                <td class="px-4 py-1.5 text-end tabular-nums">{money(l.credit)}</td>
              </tr>
            {/each}
          </tbody>
        {:else}
          <tbody><tr><td colspan="4" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.gj.empty')}</td></tr></tbody>
        {/each}
      </table>
    </div>
    {#if cursor}
      <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('accounting.reports.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>
