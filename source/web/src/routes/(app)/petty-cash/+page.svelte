<script lang="ts">
  import { onMount } from 'svelte';
  import PettyCashModal from '#lib/components/PettyCashModal.svelte';
  import ShiftOpenModal from '#lib/components/ShiftOpenModal.svelte';
  import { shifts, type Shift } from '#lib/shift/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { signedCents } from '#lib/pos/report-receipts.ts';

  // Menu Kas Kecil: ringkasan kas kecil pada shift terbuka milik kasir ini; pencatatan lewat PettyCashModal.
  let shift = $state<Shift | null | undefined>(undefined);
  let error = $state('');
  let recordOpen = $state(false);
  let openShiftDlg = $state(false);

  const money = (s: string | null | undefined) => formatCurrency(Number(signedCents(s)) / 100);

  async function load() {
    error = '';
    try {
      shift = await shifts.current();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  // Muat ulang saat outlet aktif berpindah (shift berlaku per outlet).
  $effect(() => {
    void session.outlet?.id;
    void load();
  });

  onMount(() => {
    document.title = `${t('shift.petty.title')} | ACIRABA`;
  });

  const active = $derived((shift?.petty_cash ?? []).filter((m) => !m.voided_at));
  const totalOut = $derived(active.filter((m) => m.direction === 'out').reduce((a, m) => a + signedCents(m.amount), 0n));
  const totalIn = $derived(active.filter((m) => m.direction === 'in').reduce((a, m) => a + signedCents(m.amount), 0n));
  const net = $derived(totalIn - totalOut);
  const fmt = (c: bigint) => formatCurrency(Number(c) / 100);
</script>

<main class="p-4 lg:p-6 space-y-5 max-w-3xl mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="flex items-start gap-3">
      <span class="grid place-items-center size-11 shrink-0 rounded-xl bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)] text-[var(--color-primary-600)]"><i class="icon-wallet text-[22px]"></i></span>
      <div>
        <h1 class="font-display font-bold text-[19px] leading-tight">{t('shift.petty.title')}</h1>
        <p class="text-[12.5px] mt-0.5 text-[var(--text-tertiary)]">{t('shift.petty.pageSubtitle')}</p>
      </div>
    </div>
    {#if shift}
      <button type="button" class="btn btn-primary" onclick={() => (recordOpen = true)}><i class="icon-plus"></i> {t('shift.petty.record')}</button>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if shift === undefined}
    <div class="space-y-3" aria-busy="true">
      <div class="h-14 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
        {#each [0, 1, 2] as i (i)}<div class="h-20 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>{/each}
      </div>
    </div>
  {:else if shift === null}
    <div class="surface-card flex flex-col items-center text-center gap-3 py-10">
      <span class="grid place-items-center size-14 rounded-full bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-lock text-[26px]"></i></span>
      <p class="text-[13px] max-w-sm text-[var(--text-secondary)]">{t('shift.petty.noShift')}</p>
      <button type="button" class="btn btn-primary" onclick={() => (openShiftDlg = true)}><i class="icon-lock-open"></i> {t('shift.open.submit')}</button>
    </div>
  {:else}
    <div class="surface-card flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5 !py-3">
      <div class="flex flex-wrap items-center gap-2 text-[12.5px] text-[var(--text-secondary)]">
        <span class="size-2 rounded-full bg-[var(--color-success-500)]"></span>
        <span class="font-mono font-bold text-[var(--text-primary)]">{shift.doc_no}</span>
        <span>· {t('shift.openAt', { time: shift.opened_local })}</span>
      </div>
      <div class="text-[12px] text-[var(--text-tertiary)]">{t('shift.petty.limitInfo', { limit: money(shift.petty_cash_limit) })}</div>
    </div>

    <dl class="grid grid-cols-1 sm:grid-cols-3 gap-3">
      <div class="surface-card !p-4 border-s-4 border-s-[var(--color-danger-500)]">
        <dt class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('shift.petty.out')}</dt>
        <dd class="mt-1 text-[20px] font-bold tabular-nums text-[var(--color-danger-600)]">−{fmt(totalOut)}</dd>
      </div>
      <div class="surface-card !p-4 border-s-4 border-s-[var(--color-success-500)]">
        <dt class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('shift.petty.in')}</dt>
        <dd class="mt-1 text-[20px] font-bold tabular-nums text-[var(--color-success-600)]">+{fmt(totalIn)}</dd>
      </div>
      <div class="surface-card !p-4 border-s-4 border-s-[var(--color-primary-500)]">
        <dt class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('shift.petty.net')}</dt>
        <dd class="mt-1 text-[20px] font-bold tabular-nums {net < 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-primary)]'}">{net < 0n ? '−' : ''}{fmt(net < 0n ? -net : net)}</dd>
      </div>
    </dl>

    <section class="surface-card !p-0 overflow-hidden">
      <header class="flex items-center justify-between gap-3 px-4 py-3 border-b border-[var(--border-subtle)]">
        <div>
          <h2 class="font-semibold text-[14px]">{t('shift.petty.history')}</h2>
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('shift.petty.historyHint')}</p>
        </div>
        <span class="text-[11.5px] font-semibold rounded-full px-2 py-0.5 bg-[var(--surface-sunken)] text-[var(--text-secondary)]">{t('shift.petty.entryCount', { count: shift.petty_cash.length })}</span>
      </header>
      {#if shift.petty_cash.length === 0}
        <div class="flex flex-col items-center gap-2 py-10 text-center">
          <span class="grid place-items-center size-12 rounded-full bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-wallet text-[22px]"></i></span>
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('shift.petty.empty')}</p>
          <button type="button" class="btn btn-sm" onclick={() => (recordOpen = true)}><i class="icon-plus"></i> {t('shift.petty.record')}</button>
        </div>
      {:else}
        <ul class="divide-y divide-[var(--border-subtle)]">
          {#each shift.petty_cash as m (m.id)}
            <li class="flex items-center gap-3 px-4 py-3 {m.voided_at ? 'opacity-55' : ''}">
              <span class="grid place-items-center size-9 shrink-0 rounded-full {m.direction === 'out' ? 'bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)] text-[var(--color-danger-600)]' : 'bg-[color-mix(in_oklab,var(--color-success-500)_14%,transparent)] text-[var(--color-success-600)]'}"><i class="{m.direction === 'out' ? 'icon-minus' : 'icon-plus'} text-[16px]"></i></span>
              <div class="min-w-0 grow">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                  <span class="font-semibold text-[13px] {m.voided_at ? 'line-through' : ''}">{m.category}</span>
                  {#if m.approved_by_name}<span class="rounded-full px-1.5 py-px text-[10.5px] font-semibold bg-[color-mix(in_oklab,var(--color-warning-500)_16%,transparent)] text-[var(--color-warning-700)]">{t('shift.petty.approvedBy', { name: m.approved_by_name })}</span>{/if}
                  {#if m.voided_at}<span class="rounded-full px-1.5 py-px text-[10.5px] font-semibold bg-[var(--surface-sunken)] text-[var(--text-secondary)]">{t('shift.petty.voided')}</span>{/if}
                </div>
                {#if m.note}<div class="text-[12px] text-[var(--text-secondary)] truncate">{m.note}</div>{/if}
                <div class="text-[11.5px] text-[var(--text-tertiary)]">{m.created_local}</div>
              </div>
              <span class="tabular-nums font-bold text-[14px] shrink-0 {m.voided_at ? 'line-through' : ''} {m.direction === 'out' ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{m.direction === 'out' ? '−' : '+'}{money(m.amount)}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</main>

{#if recordOpen && shift}
  <PettyCashModal shiftId={shift.id} onclose={() => ((recordOpen = false), void load())} />
{/if}

{#if openShiftDlg}
  <ShiftOpenModal
    onclose={() => (openShiftDlg = false)}
    onopened={(s) => {
      openShiftDlg = false;
      shift = s;
      void load();
    }}
  />
{/if}
