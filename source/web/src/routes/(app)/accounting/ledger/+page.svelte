<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import Combobox from '#lib/components/Combobox.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import { accounting, todayISO, type Account, type LedgerRow } from '#lib/accounting/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (s: string | undefined) => (s === undefined ? '—' : formatCurrency(Number(s)));
  const monthStart = () => todayISO().slice(0, 8) + '01';

  let accounts = $state<Account[]>([]);
  let accountId = $state('');
  let accountLabel = $state('');
  let from = $state(monthStart());
  let to = $state(todayISO());
  let rows = $state<LedgerRow[]>([]);
  let opening = $state<string | undefined>();
  let totalDebit = $state<string | undefined>();
  let totalCredit = $state<string | undefined>();
  let cursor = $state('');
  let loading = $state(false);
  let error = $state('');

  onMount(async () => {
    document.title = t('accounting.ledger.docTitle');
    try {
      accounts = (await accounting.accounts()).filter((a) => a.kind === 'ledger');
      // Tautan dari laporan lain: ?account=<id>&from=&to= langsung memilih akun dan rentang.
      const qs = page.url.searchParams;
      const pre = accounts.find((a) => a.id === qs.get('account'));
      if (pre) {
        accountLabel = `${pre.code} · ${pre.name}`;
        from = qs.get('from') || from;
        to = qs.get('to') || to;
        accountId = pre.id;
      }
    } catch (e) {
      error = errorMessage(e);
    }
  });

  // Daftar akun kecil (COA): disaring di klien; tetap dibatasi 30 hasil.
  const search = (q: string) => {
    const s = q.trim().toLowerCase();
    const hit = accounts.filter((a) => !s || a.code.toLowerCase().includes(s) || a.name.toLowerCase().includes(s)).slice(0, 30);
    return Promise.resolve(hit.map((a) => ({ id: a.id, name: `${a.code} · ${a.name}` })));
  };

  let seq = 0;
  async function load(more = false) {
    if (!accountId || !from || !to) return;
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await accounting.ledger({ account_id: accountId, from, to, cursor: more ? cursor : '', limit: 100 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.rows] : res.rows;
      if (!more) {
        opening = res.opening_balance;
        totalDebit = res.total_debit;
        totalCredit = res.total_credit;
      }
      cursor = res.next_cursor ?? '';
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Akun, rentang, atau outlet aktif berubah → muat dari awal.
  $effect(() => {
    void accountId;
    void session.outlet?.id;
    rows = [];
    opening = totalDebit = totalCredit = undefined;
    cursor = '';
    void load();
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.ledger.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.ledger.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="w-72"><Combobox bind:value={accountId} bind:label={accountLabel} {search} placeholder={t('accounting.ledger.accountPick')} clearable={false} /></div>
      <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('accounting.ledger.range')} />
    </div>

    {#if !accountId}
      <div class="px-4 py-16 text-center text-[var(--text-tertiary)]">{t('accounting.ledger.pickFirst')}</div>
    {:else}
      <div class="overflow-x-auto scroll-thin">
        <table class="w-full min-w-[820px] text-[12.5px]">
          <thead>
            <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="px-4 py-3 text-start" scope="col">{t('accounting.ledger.col.date')}</th>
              <th class="px-3 py-3 text-start" scope="col">{t('accounting.ledger.col.doc')}</th>
              <th class="px-3 py-3 text-start" scope="col">{t('accounting.ledger.col.type')}</th>
              <th class="px-3 py-3 text-start" scope="col">{t('accounting.ledger.col.narration')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('accounting.ledger.col.debit')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('accounting.ledger.col.credit')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('accounting.ledger.col.balance')}</th>
            </tr>
          </thead>
          <tbody>
            {#if opening !== undefined}
              <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] font-semibold">
                <td class="px-4 py-2.5" colspan="6">{t('accounting.ledger.opening')} · {from}</td>
                <td class="px-3 py-2.5 text-end tabular-nums">{money(opening)}</td>
              </tr>
            {/if}
            {#each rows as r (r.line_id)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5">
                <td class="px-4 py-2.5 whitespace-nowrap">{r.date}</td>
                <td class="px-3 py-2.5 font-mono whitespace-nowrap">{r.doc_no}</td>
                <td class="px-3 py-2.5 whitespace-nowrap">{t(`accounting.type.${r.type}`)}</td>
                <td class="px-3 py-2.5 max-w-80"><div class="line-clamp-2">{r.narration}{r.memo ? ` — ${r.memo}` : ''}</div></td>
                <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">{Number(r.debit) ? money(r.debit) : ''}</td>
                <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">{Number(r.credit) ? money(r.credit) : ''}</td>
                <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap font-semibold">{money(r.balance)}</td>
              </tr>
            {:else}
              <tr><td colspan="7" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.ledger.empty')}</td></tr>
            {/each}
          </tbody>
          {#if totalDebit !== undefined}
            <tfoot>
              <tr class="border-t border-[var(--border-default)] bg-[var(--surface-sunken)] font-bold">
                <td class="px-4 py-2.5" colspan="4">{t('accounting.ledger.totalDebit')} / {t('accounting.ledger.totalCredit')}</td>
                <td class="px-3 py-2.5 text-end tabular-nums">{money(totalDebit)}</td>
                <td class="px-3 py-2.5 text-end tabular-nums">{money(totalCredit)}</td>
                <td></td>
              </tr>
            </tfoot>
          {/if}
        </table>
      </div>
      {#if cursor}
        <div class="flex justify-center p-3 border-t border-[var(--border-subtle)]">
          <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('accounting.ledger.loadMore')}</button>
        </div>
      {/if}
    {/if}
  </div>
</main>
