<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Select from '#lib/components/Select.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { shifts, type Shift } from '#lib/shift/api.ts';
  import { signedCents } from '#lib/pos/report-receipts.ts';

  // Kas kecil kasir (uang keluar/masuk laci di luar penjualan) untuk shift terbuka milik pemanggil.
  let { shiftId, onclose, onchanged }: { shiftId: string; onclose: () => void; onchanged?: (s: Shift) => void } = $props();

  const CATS = ['operational', 'meal', 'transport', 'supplies', 'change', 'misc'] as const;
  type Cat = (typeof CATS)[number];

  let sh = $state<Shift | null>(null);
  let direction = $state<'out' | 'in'>('out');
  let amount = $state('');
  let cat = $state<Cat>('operational');
  let note = $state('');
  let approverId = $state('');
  let pin = $state('');
  let approvers = $state<Approver[] | null>(null);
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');
  let limitDraft = $state('');
  let limitBusy = $state(false);
  // Satu kunci per percobaan simpan: kirim ulang (jaringan putus) tidak mencatat dua kali; dibuat ulang setelah sukses.
  let key = 'pc-' + crypto.randomUUID();

  const fmt = (c: bigint) => formatCurrency(Number(c) / 100);
  const money = (s: string | null | undefined) => fmt(signedCents(s));
  const open = $derived(sh?.status === 'open');
  const activeEntries = $derived((sh?.petty_cash ?? []).filter((m) => !m.voided_at));
  const totalOut = $derived(activeEntries.filter((m) => m.direction === 'out').reduce((a, m) => a + signedCents(m.amount), 0n));
  const totalIn = $derived(activeEntries.filter((m) => m.direction === 'in').reduce((a, m) => a + signedCents(m.amount), 0n));
  const needApproval = $derived(direction === 'out' && !!sh && signedCents(amount || '0') > signedCents(sh.petty_cash_limit));
  const valid = $derived(
    signedCents(amount || '0') > 0n && (!needApproval || (approverId !== '' && /^\d{6}$/.test(pin)))
  );
  const canSetLimit = $derived(can('cash_shifts', 'update'));

  async function load() {
    try {
      sh = await shifts.get(shiftId);
      onchanged?.(sh);
      limitDraft = String(Number(signedCents(sh.petty_cash_limit)) / 100);
    } catch (e) {
      error = errorMessage(e);
    }
  }
  onMount(() => void load());

  $effect(() => {
    if (needApproval && approvers === null) {
      approvers = [];
      approvals
        .approvers('shift_close')
        .then((l) => {
          approvers = l;
          if (l.length === 1) approverId = l[0].id;
        })
        .catch((e) => (error = errorMessage(e)));
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy || !open) return;
    busy = true;
    error = '';
    notice = '';
    try {
      await shifts.addPettyCash(
        {
          direction,
          amount,
          category: t(`shift.petty.cat.${cat}` as MessageKey),
          note: note.trim(),
          approval: needApproval ? { user_id: approverId, pin } : undefined
        },
        key
      );
      key = 'pc-' + crypto.randomUUID();
      amount = '';
      note = '';
      pin = '';
      notice = t('shift.petty.saved');
      await load();
    } catch (err) {
      pin = '';
      error = err instanceof ApiError && err.code === 'PIN_LOCKED' ? errorMessage(err).replace('{count}', String(err.retryAfter)) : errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function voidEntry(id: string) {
    if (busy || !confirm(t('shift.petty.voidConfirm'))) return;
    busy = true;
    error = '';
    notice = '';
    try {
      await shifts.voidPettyCash(id);
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function saveLimit() {
    if (limitBusy) return;
    limitBusy = true;
    error = '';
    notice = '';
    try {
      await shifts.setPettyCashLimit(limitDraft || '0');
      notice = t('shift.petty.limitSaved');
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      limitBusy = false;
    }
  }
</script>

<Modal title={t('shift.petty.title')} wide {onclose}>
  {#if !sh}
    <p class="text-[13px] text-[var(--text-tertiary)]">{error || t('shift.closing.loading')}</p>
  {:else}
    <div class="space-y-4 text-[13px]">
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('shift.petty.body')}</p>

      <dl class="grid grid-cols-3 gap-2 text-center">
        <div class="rounded-lg bg-[var(--surface-sunken)] px-2 py-2.5"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.petty.out')}</dt><dd class="font-bold tabular-nums text-[var(--color-danger-600)]">−{fmt(totalOut)}</dd></div>
        <div class="rounded-lg bg-[var(--surface-sunken)] px-2 py-2.5"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.petty.in')}</dt><dd class="font-bold tabular-nums text-[var(--color-success-600)]">+{fmt(totalIn)}</dd></div>
        <div class="rounded-lg bg-[var(--surface-sunken)] px-2 py-2.5"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.petty.net')}</dt><dd class="font-bold tabular-nums {totalIn - totalOut < 0n ? 'text-[var(--color-danger-600)]' : ''}">{totalIn - totalOut < 0n ? '−' : ''}{fmt(totalIn - totalOut < 0n ? totalOut - totalIn : totalIn - totalOut)}</dd></div>
      </dl>

      {#if open}
        <form class="space-y-3 rounded-xl border border-[var(--border-subtle)] p-3.5" onsubmit={submit} autocomplete="off">
          <h3 class="font-semibold text-[13.5px]">{t('shift.petty.newEntry')}</h3>
          <div class="grid grid-cols-2 gap-2" role="radiogroup" aria-label={t('shift.petty.title')}>
            <button
              type="button"
              role="radio"
              aria-checked={direction === 'out'}
              class="flex items-center justify-center gap-2 h-11 rounded-lg border-2 font-semibold transition-colors {direction === 'out' ? 'border-[var(--color-danger-500)] bg-[color-mix(in_oklab,var(--color-danger-500)_12%,transparent)] text-[var(--color-danger-600)]' : 'border-[var(--border-default)] text-[var(--text-secondary)] hover:bg-[var(--surface-sunken)]'}"
              onclick={() => (direction = 'out')}
            >
              <i class="icon-minus"></i> {t('shift.petty.out')}
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={direction === 'in'}
              class="flex items-center justify-center gap-2 h-11 rounded-lg border-2 font-semibold transition-colors {direction === 'in' ? 'border-[var(--color-success-500)] bg-[color-mix(in_oklab,var(--color-success-500)_12%,transparent)] text-[var(--color-success-600)]' : 'border-[var(--border-default)] text-[var(--text-secondary)] hover:bg-[var(--surface-sunken)]'}"
              onclick={() => (direction = 'in')}
            >
              <i class="icon-plus"></i> {t('shift.petty.in')}
            </button>
          </div>

          <label class="block">
            <span class="font-semibold">{t('shift.petty.amount')}</span>
            <MoneyInput
              id="petty-amount"
              bind:value={amount}
              placeholder="0"
              class="mt-1 w-full h-12 px-3 rounded-md text-[18px] font-bold tabular-nums text-end border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
            />
          </label>

          <div>
            <span class="font-semibold">{t('shift.petty.category')}</span>
            <div class="mt-1 flex flex-wrap gap-1.5">
              {#each CATS as c (c)}
                <button type="button" aria-pressed={cat === c} class="btn btn-sm {cat === c ? 'btn-primary' : ''}" onclick={() => (cat = c)}>{t(`shift.petty.cat.${c}` as MessageKey)}</button>
              {/each}
            </div>
          </div>

          <label class="block">
            <span class="font-semibold">{t('shift.petty.note')}</span>
            <input type="text" bind:value={note} maxlength="300" placeholder={t('shift.petty.notePh')} class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)]" />
          </label>

          {#if needApproval}
            <div class="space-y-3 rounded-md border border-[color-mix(in_oklab,var(--color-warning-500)_45%,transparent)] bg-[color-mix(in_oklab,var(--color-warning-500)_8%,transparent)] p-3">
              <p class="text-[12.5px] font-semibold text-[var(--color-warning-700)]"><i class="icon-triangle-alert"></i> {t('shift.petty.needApproval', { limit: money(sh.petty_cash_limit) })}</p>
              <div class="grid sm:grid-cols-2 gap-3">
                <label class="block">
                  <span class="font-semibold">{t('shift.petty.approver')}</span>
                  <div class="mt-1"><Select bind:value={approverId} class="!min-h-10" options={[{ value: '', label: t('shift.petty.pickApprover'), disabled: true }, ...(approvers ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
                  {#if approvers && approvers.length === 0}<span class="text-[12px] text-[var(--color-warning-600)]">{t('shift.petty.noApprovers')}</span>{/if}
                </label>
                <label class="block">
                  <span class="font-semibold">{t('shift.petty.pin')}</span>
                  <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
                </label>
              </div>
            </div>
          {/if}

          {#if notice}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]">{notice}</p>{/if}
          {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

          <div class="flex justify-end gap-2">
            <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('shift.closing.cancel')}</button>
            <button type="submit" class="btn btn-primary" disabled={!valid || busy}><i class="icon-save"></i> {busy ? t('shift.petty.saving') : t('shift.petty.save')}</button>
          </div>
        </form>
      {/if}

      <div>
        <div class="flex items-center justify-between mb-1.5">
          <h3 class="font-semibold">{t('shift.petty.history')}</h3>
          <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('shift.petty.entryCount', { count: sh.petty_cash.length })}</span>
        </div>
        {#if sh.petty_cash.length === 0}
          <p class="rounded-lg border border-dashed border-[var(--border-default)] py-6 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('shift.petty.empty')}</p>
        {:else}
          <ul class="rounded-xl border border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)] max-h-72 overflow-y-auto">
            {#each sh.petty_cash as m (m.id)}
              <li class="flex items-center justify-between gap-3 px-3 py-2.5 {m.voided_at ? 'opacity-50' : ''}">
                <span class="grid place-items-center size-8 shrink-0 rounded-full {m.direction === 'out' ? 'bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)] text-[var(--color-danger-600)]' : 'bg-[color-mix(in_oklab,var(--color-success-500)_14%,transparent)] text-[var(--color-success-600)]'}"><i class={m.direction === 'out' ? 'icon-minus' : 'icon-plus'}></i></span>
                <div class="min-w-0 grow">
                  <div class="font-semibold truncate {m.voided_at ? 'line-through' : ''}">{m.category}{m.note ? ` · ${m.note}` : ''}</div>
                  <div class="text-[11.5px] text-[var(--text-tertiary)]">
                    {m.created_local}{m.approved_by_name ? ` · ${t('shift.petty.approvedBy', { name: m.approved_by_name })}` : ''}{m.voided_at ? ` · ${t('shift.petty.voided')}` : ''}
                  </div>
                </div>
                <div class="flex items-center gap-2 shrink-0">
                  <span class="tabular-nums font-bold {m.direction === 'out' ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{m.direction === 'out' ? '−' : '+'}{money(m.amount)}</span>
                  {#if open && !m.voided_at}
                    <button type="button" class="btn btn-sm" disabled={busy} onclick={() => voidEntry(m.id)}>{t('shift.petty.void')}</button>
                  {/if}
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      {#if canSetLimit}
        <div class="rounded-md bg-[var(--surface-sunken)] p-3 space-y-2">
          <label class="block">
            <span class="font-semibold">{t('shift.petty.limit')}</span>
            <div class="mt-1 flex items-stretch gap-2">
              <MoneyInput id="petty-limit" bind:value={limitDraft} placeholder="0" class="grow min-w-0 h-10 px-3 rounded-md tabular-nums text-end border border-[var(--border-default)] bg-[var(--surface-base)]" />
              <button type="button" class="btn shrink-0" disabled={limitBusy} onclick={saveLimit}>{t('shift.petty.limitSave')}</button>
            </div>
          </label>
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('shift.petty.limitHint')}</p>
        </div>
      {/if}
    </div>
  {/if}
</Modal>
