<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import Combobox from '#lib/components/Combobox.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { accounting, todayISO, type Account, type BankReconciliation } from '#lib/accounting/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (value: string) => formatCurrency(Number(value));
  const inputClass = 'h-10 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)] disabled:opacity-60';
  const labelClass = 'block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const moneyInputClass = 'money-field w-full min-w-0 bg-transparent px-3.5 text-end text-[14px] font-semibold tabular-nums outline-none disabled:cursor-not-allowed';

  let accounts = $state<Account[]>([]);
  let accountId = $state('');
  let accountLabel = $state('');
  let month = $state(todayISO().slice(0, 7));
  let statementOpening = $state('');
  let statementClosing = $state('');
  let reconciliation = $state<BankReconciliation | null>(null);
  let loadedKey = $state('');
  let selected = $state<Record<string, boolean>>({});
  let busy = $state(false);
  let loading = $state(false);
  let notFound = $state(false);
  let error = $state('');

  const canEdit = $derived(reconciliation?.status === 'draft' && can('journals', 'update'));
  const canStart = $derived(can('journals', 'create'));
  const canComplete = $derived(reconciliation?.status === 'draft' && can('journals', 'approve'));
  const selectedAccount = $derived(accounts.find((account) => account.id === accountId));
  const matchedCount = $derived(reconciliation?.lines.filter((line) => line.matched).length ?? 0);
  const hasOutstanding = $derived(!!reconciliation && (Number(reconciliation.outstanding_debit) > 0 || Number(reconciliation.outstanding_credit) > 0));

  const searchAccounts = (query: string) => {
    const text = query.trim().toLowerCase();
    return Promise.resolve(accounts
      .filter((account) => account.active)
      .filter((account) => !text || account.code.toLowerCase().includes(text) || account.name.toLowerCase().includes(text))
      .slice(0, 30)
      .map((account) => ({ id: account.id, name: `${account.code} · ${account.name}` })));
  };

  function applyReconciliation(value: BankReconciliation) {
    reconciliation = value;
    loadedKey = `${accountId}|${month}`;
    statementOpening = value.statement_opening;
    statementClosing = value.statement_closing;
    selected = Object.fromEntries(value.lines.map((line) => [line.line_id, line.matched]));
    notFound = false;
  }

  $effect(() => {
    const key = `${accountId}|${month}`;
    if (loadedKey && loadedKey !== key) {
      reconciliation = null;
      selected = {};
      statementOpening = '';
      statementClosing = '';
      notFound = false;
      loadedKey = key;
    }
  });

  onMount(async () => {
    document.title = t('accounting.bankReconciliation.docTitle');
    try {
      accounts = (await accounting.accounts()).filter((account) => account.kind === 'ledger' && account.is_cash_bank);
      const params = page.url.searchParams;
      month = params.get('month') || month;
      const preset = accounts.find((account) => account.id === params.get('account_id'));
      if (preset) {
        accountId = preset.id;
        accountLabel = `${preset.code} · ${preset.name}`;
        await loadExisting();
      }
    } catch (cause) {
      error = errorMessage(cause);
    }
  });

  async function loadExisting() {
    if (!accountId || !month) return;
    loading = true;
    error = '';
    notFound = false;
    try {
      applyReconciliation(await accounting.findBankReconciliation({ account_id: accountId, month }));
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 404) {
        reconciliation = null;
        selected = {};
        loadedKey = `${accountId}|${month}`;
        notFound = true;
      } else {
        error = errorMessage(cause);
      }
    } finally {
      loading = false;
    }
  }

  async function startReconciliation() {
    if (busy || !accountId || !month || !canStart || !selectedAccount?.active || !statementOpening.trim() || !statementClosing.trim()) return;
    busy = true;
    error = '';
    try {
      applyReconciliation(await accounting.startBankReconciliation({
        account_id: accountId,
        month,
        statement_opening: statementOpening,
        statement_closing: statementClosing
      }));
      notFound = false;
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }

  async function saveMatches() {
    if (!reconciliation || !canEdit || busy) return false;
    busy = true;
    error = '';
    try {
      applyReconciliation(await accounting.updateBankReconciliation(reconciliation.id, {
        statement_opening: statementOpening,
        statement_closing: statementClosing,
        matched_line_ids: Object.entries(selected).filter(([, checked]) => checked).map(([id]) => id)
      }));
      return true;
    } catch (cause) {
      error = errorMessage(cause);
      return false;
    } finally {
      busy = false;
    }
  }

  async function complete() {
    if (!reconciliation || !canComplete || !reconciliation.balanced || busy) return;
    if (canEdit && (!(await saveMatches()) || !reconciliation?.balanced)) return;
    if (!reconciliation) return;
    busy = true;
    error = '';
    try {
      applyReconciliation(await accounting.completeBankReconciliation(reconciliation.id));
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
</script>

<main class="p-4 lg:p-6 space-y-5 max-w-full mx-auto w-full" aria-busy={loading}>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="flex items-start gap-3 min-w-0">
      <span class="grid place-items-center size-11 rounded-xl shrink-0" style="background: color-mix(in srgb, var(--color-primary) 14%, transparent); color: var(--color-primary)"><i class="icon-landmark text-[22px]"></i></span>
      <div class="min-w-0">
        <h1 class="font-display font-bold text-[20px] leading-tight">{t('accounting.bankReconciliation.title')}</h1>
        <p class="text-[12.5px] mt-1 max-w-2xl text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.subtitle')}</p>
      </div>
    </div>
    <a class="btn" href="/accounting/cash-bank"><i class="icon-wallet"></i> {t('accounting.cashBank.title')}</a>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <!-- Pilih akun + bulan -->
  <section class="surface-card !p-0 overflow-hidden">
    <div class="grid gap-x-6 gap-y-3.5 p-4 lg:p-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,240px)_auto] lg:items-end">
      <label class="block min-w-0 space-y-1.5">
        <span class={labelClass}>{t('accounting.bankReconciliation.account')}</span>
        <Combobox bind:value={accountId} bind:label={accountLabel} search={searchAccounts} placeholder={t('accounting.bankReconciliation.account')} clearable={false} />
      </label>
      <label class="block min-w-0 space-y-1.5">
        <span class={labelClass}>{t('accounting.bankReconciliation.month')}</span>
        <input class={inputClass} type="month" bind:value={month} />
      </label>
      <button class="btn btn-primary h-10 w-full lg:w-auto" type="button" disabled={busy || loading || !accountId || !month} onclick={loadExisting}>
        <i class="icon-search"></i> {t('accounting.bankReconciliation.reload')}
      </button>
    </div>
  </section>

  {#if notFound}
    {#if canStart && selectedAccount?.active}
      <section class="surface-card !p-0 overflow-hidden">
        <header class="flex flex-wrap items-center gap-3 px-4 py-4 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
          <span class="grid place-items-center size-10 rounded-lg shrink-0" style="background: color-mix(in srgb, var(--color-warning-600, #d97706) 14%, transparent); color: var(--color-warning-600, #d97706)"><i class="icon-file-plus text-[20px]"></i></span>
          <div class="min-w-0 flex-1">
            <h2 class="font-semibold text-[14px]">{t('accounting.bankReconciliation.emptyRecord')}</h2>
            <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.startHint')}</p>
          </div>
        </header>

        <div class="p-4 lg:p-6">
          <div class="grid gap-6 sm:grid-cols-2 max-w-3xl">
            <label class="block min-w-0 space-y-1.5">
              <span class="block text-[12px] font-semibold text-[var(--text-secondary)]">{t('accounting.bankReconciliation.statementOpening')}</span>
              <span class="flex h-11 items-stretch overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] transition-colors focus-within:border-[var(--color-primary-500)]">
                <span class="grid place-items-center border-r border-[var(--border-default)] bg-[var(--surface-sunken)] px-3.5 text-[12.5px] font-medium text-[var(--text-tertiary)]">Rp</span>
                <input class={moneyInputClass} type="number" min="0" step="0.01" inputmode="decimal" placeholder="0" bind:value={statementOpening} />
              </span>
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.openingHint')}</span>
            </label>
            <label class="block min-w-0 space-y-1.5">
              <span class="block text-[12px] font-semibold text-[var(--text-secondary)]">{t('accounting.bankReconciliation.statementClosing')}</span>
              <span class="flex h-11 items-stretch overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] transition-colors focus-within:border-[var(--color-primary-500)]">
                <span class="grid place-items-center border-r border-[var(--border-default)] bg-[var(--surface-sunken)] px-3.5 text-[12.5px] font-medium text-[var(--text-tertiary)]">Rp</span>
                <input class={moneyInputClass} type="number" min="0" step="0.01" inputmode="decimal" placeholder="0" bind:value={statementClosing} />
              </span>
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.closingHint')}</span>
            </label>
          </div>
        </div>

        <footer class="flex flex-wrap items-center justify-end gap-2 px-4 py-4 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
          <button class="btn btn-primary h-10" type="button" disabled={busy || !statementOpening.trim() || !statementClosing.trim()} onclick={startReconciliation}>
            <i class="icon-play"></i> {t('accounting.bankReconciliation.createDraft')}
          </button>
        </footer>
      </section>
    {:else}
      <p class="text-[12px] text-[var(--text-tertiary)]">{canStart ? t('accounting.bankReconciliation.inactiveAccount') : t('accounting.bankReconciliation.permission')}</p>
    {/if}
  {/if}

  {#if reconciliation}
    <section class="surface-card !p-0 overflow-hidden">
      <header class="flex flex-wrap items-center justify-between gap-3 px-4 py-4 border-b border-[var(--border-subtle)]">
        <div class="flex items-center gap-3 min-w-0">
          <span class="grid place-items-center size-10 rounded-lg shrink-0" style="background: color-mix(in srgb, var(--color-primary) 14%, transparent); color: var(--color-primary)"><i class="icon-vault text-[19px]"></i></span>
          <div class="min-w-0">
            <h2 class="font-semibold text-[15px] leading-tight truncate">{reconciliation.account_code} · {reconciliation.account_name}</h2>
            <p class="text-[11.5px] mt-0.5 text-[var(--text-tertiary)]">{reconciliation.month}</p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-[12px] text-[var(--text-secondary)]">{t('accounting.bankReconciliation.matchedCount', { matched: String(matchedCount), total: String(reconciliation.lines.length) })}</span>
          <span class="badge-soft {reconciliation.status === 'completed' ? 'badge-success' : 'badge-warning'}">{t(`accounting.bankReconciliation.${reconciliation.status}`)}</span>
        </div>
      </header>

      <div class="grid divide-y lg:divide-y-0 lg:divide-x divide-[var(--border-subtle)] lg:grid-cols-2">
        <div class="p-4 lg:p-5 space-y-4">
          <div class="flex items-center gap-2 text-[12.5px] font-semibold text-[var(--text-secondary)]"><i class="icon-landmark text-[15px]"></i>{t('accounting.bankReconciliation.statementSection')}</div>
          <div class="grid gap-6 sm:grid-cols-2">
            <label class="block min-w-0 space-y-1.5">
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.statementOpening')}</span>
              <span class="flex h-11 items-stretch overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] transition-colors focus-within:border-[var(--color-primary-500)]" class:opacity-60={!canEdit}>
                <span class="grid place-items-center border-r border-[var(--border-default)] bg-[var(--surface-sunken)] px-3.5 text-[12.5px] font-medium text-[var(--text-tertiary)]">Rp</span>
                <input class={moneyInputClass} type="number" min="0" step="0.01" inputmode="decimal" placeholder="0" bind:value={statementOpening} disabled={!canEdit} />
              </span>
            </label>
            <label class="block min-w-0 space-y-1.5">
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.statementClosing')}</span>
              <span class="flex h-11 items-stretch overflow-hidden rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] transition-colors focus-within:border-[var(--color-primary-500)]" class:opacity-60={!canEdit}>
                <span class="grid place-items-center border-r border-[var(--border-default)] bg-[var(--surface-sunken)] px-3.5 text-[12.5px] font-medium text-[var(--text-tertiary)]">Rp</span>
                <input class={moneyInputClass} type="number" min="0" step="0.01" inputmode="decimal" placeholder="0" bind:value={statementClosing} disabled={!canEdit} />
              </span>
            </label>
          </div>
        </div>

        <div class="p-4 lg:p-5 space-y-4">
          <div class="flex items-center gap-2 text-[12.5px] font-semibold text-[var(--text-secondary)]"><i class="icon-book-open text-[15px]"></i>{t('accounting.bankReconciliation.bookSection')}</div>
          <div class="grid gap-6 sm:grid-cols-2">
            <div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3.5">
              <div class="text-[11px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.bookOpening')}</div>
              <div class="font-display font-bold text-[17px] leading-tight tabular-nums mt-1">{money(reconciliation.book_opening)}</div>
            </div>
            <div class="rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3.5">
              <div class="text-[11px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.bookClosing')}</div>
              <div class="font-display font-bold text-[17px] leading-tight tabular-nums mt-1">{money(reconciliation.book_closing)}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="p-4 lg:p-5 space-y-4 border-t border-[var(--border-subtle)]">
        <div class="flex items-center gap-2 text-[12.5px] font-semibold text-[var(--text-secondary)]"><i class="icon-scale text-[15px]"></i>{t('accounting.bankReconciliation.summary')}</div>
        <div class="grid gap-3 grid-cols-2 xl:grid-cols-4">
          {#each [
            { icon: 'icon-banknote', tone: 'var(--color-info-600, #0284c7)', label: t('accounting.bankReconciliation.adjustedBank'), value: money(reconciliation.adjusted_bank) },
            { icon: 'icon-git-compare', tone: reconciliation.balanced ? 'var(--color-success-600, #16a34a)' : 'var(--color-danger-600, #dc2626)', label: t('accounting.bankReconciliation.difference'), value: money(reconciliation.difference) },
            { icon: 'icon-arrow-down-to-line', tone: 'var(--color-primary)', label: t('accounting.bankReconciliation.outstandingDebit'), value: money(reconciliation.outstanding_debit) },
            { icon: 'icon-arrow-up-from-line', tone: 'var(--color-warning-600, #d97706)', label: t('accounting.bankReconciliation.outstandingCredit'), value: money(reconciliation.outstanding_credit) }
          ] as k (k.label)}
            <div class="surface-card !p-3.5 flex items-center gap-3 min-w-0">
              <span class="grid place-items-center size-10 rounded-lg shrink-0" style="background: color-mix(in srgb, {k.tone} 14%, transparent); color: {k.tone}"><i class="{k.icon} text-[19px]"></i></span>
              <div class="min-w-0">
                <div class="text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] truncate">{k.label}</div>
                <div class="font-display font-bold text-[17px] leading-tight tabular-nums truncate">{k.value}</div>
              </div>
            </div>
          {/each}
        </div>

        <div role="status" class="flex items-start gap-2 rounded-lg px-3.5 py-3 text-[12.5px] {!reconciliation.balanced ? 'badge-danger' : hasOutstanding ? 'badge-warning' : 'badge-success'}">
          <i class={!reconciliation.balanced ? 'icon-triangle-alert mt-0.5' : hasOutstanding ? 'icon-circle-alert mt-0.5' : 'icon-circle-check mt-0.5'}></i>
          <span>
            {#if !reconciliation.balanced}
              {t('accounting.bankReconciliation.unbalancedHint')}
            {:else if hasOutstanding}
              {t('accounting.bankReconciliation.balancedOutstandingHint', { debit: money(reconciliation.outstanding_debit), credit: money(reconciliation.outstanding_credit) })}
            {:else}
              {t('accounting.bankReconciliation.balancedHint')}
            {/if}
          </span>
        </div>
      </div>

      <div class="overflow-x-auto scroll-thin border-t border-[var(--border-subtle)]">
        <table class="w-full min-w-[680px] text-[12.5px]">
          <thead>
            <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="px-4 py-3 text-start" scope="col">{t('accounting.bankReconciliation.date')} / {t('accounting.bankReconciliation.document')}</th>
              <th class="px-4 py-3 text-start" scope="col">{t('accounting.bankReconciliation.description')}</th>
              <th class="px-4 py-3 text-end" scope="col">{t('accounting.bankReconciliation.debit')}</th>
              <th class="px-4 py-3 text-end" scope="col">{t('accounting.bankReconciliation.credit')}</th>
              <th class="px-4 py-3 text-center" scope="col">{t('accounting.bankReconciliation.matched')}</th>
            </tr>
          </thead>
          <tbody>
            {#each reconciliation.lines as line (line.line_id)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--surface-sunken)]">
                <td class="px-4 py-3 whitespace-nowrap align-top"><div>{line.date}</div><div class="font-mono text-[10.5px] text-[var(--text-tertiary)]">{line.doc_no}</div></td>
                <td class="px-4 py-3 max-w-[300px] align-top"><div class="line-clamp-2">{line.narration}{line.memo ? ` — ${line.memo}` : ''}</div></td>
                <td class="px-4 py-3 text-end tabular-nums whitespace-nowrap align-top">{Number(line.debit) ? money(line.debit) : ''}</td>
                <td class="px-4 py-3 text-end tabular-nums whitespace-nowrap align-top">{Number(line.credit) ? money(line.credit) : ''}</td>
                <td class="px-4 py-3 text-center align-top"><input type="checkbox" aria-label={`${t('accounting.bankReconciliation.matched')} ${line.doc_no}`} bind:checked={selected[line.line_id]} disabled={!canEdit} /></td>
              </tr>
            {:else}
              <tr><td colspan="5" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.empty')}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if canEdit || canComplete}
        <footer class="flex flex-wrap justify-end gap-2 px-4 py-4 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
          {#if canEdit}
            <button class="btn" type="button" disabled={busy} onclick={saveMatches}><i class="icon-save"></i> {t('accounting.bankReconciliation.save')}</button>
          {/if}
          {#if canComplete}
            <button class="btn btn-primary" type="button" disabled={busy || !reconciliation.balanced} onclick={complete}><i class="icon-check"></i> {t(hasOutstanding ? 'accounting.bankReconciliation.completeWithOutstanding' : 'accounting.bankReconciliation.complete')}</button>
          {/if}
        </footer>
      {:else if reconciliation.status === 'draft'}
        <p class="px-4 py-4 text-[12px] text-[var(--text-tertiary)]">{t('accounting.bankReconciliation.permission')}</p>
      {/if}
    </section>
  {/if}
</main>
