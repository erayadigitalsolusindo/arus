<script lang="ts">
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { t, formatDate } from '#lib/i18n/index.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { onMount } from 'svelte';
  import { shifts, type Shift } from '#lib/shift/api.ts';

  let { onclose, onopened, classic = false }: { onclose: () => void; onopened: (s: Shift) => void; classic?: boolean } = $props();

  let amount = $state('');
  let busy = $state(false);
  let error = $state('');

  // Modal memfokuskan dialognya sendiri saat dibuka; fokus ke isian ditunda satu tick.
  onMount(() => {
    const id = setTimeout(() => document.getElementById('shift-open-amount')?.focus(), 0);
    return () => clearTimeout(id);
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    error = '';
    try {
      onopened(await shifts.open(amount || '0'));
    } catch (err) {
      // Shift sudah terbuka (mis. dari tab lain): ambil yang ada.
      if (err instanceof ApiError && err.code === 'SHIFT_ALREADY_OPEN') {
        const cur = await shifts.current().catch(() => null);
        if (cur) return onopened(cur);
      }
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={classic ? t('shift.open.classicTitle', { date: formatDate(new Date(), { day: '2-digit', month: '2-digit', year: 'numeric' }) }) : t('shift.open.title')} {onclose}>
  {#if classic}
    <form class="space-y-3" onsubmit={submit} autocomplete="off">
      <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[13px]">
        <dt>{t('shift.open.cashierName')}</dt><dd class="font-semibold">{session.user?.name}</dd>
        <dt>{t('shift.open.outletName')}</dt><dd class="font-semibold">{session.outlet?.code}</dd>
      </dl>
      <div class="flex items-stretch gap-2">
        <MoneyInput id="shift-open-amount" bind:value={amount} placeholder="0" class="grow min-w-0 h-10 px-2 rounded-sm text-[15px] font-bold tabular-nums text-end border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
        <button type="submit" class="btn btn-sm shrink-0" disabled={busy}><i class="icon-save"></i> {busy ? t('shift.open.busy') : t('shift.open.classicSave')}</button>
      </div>
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    </form>
  {:else}
  <form class="space-y-4" onsubmit={submit} autocomplete="off">
    <p class="text-[12.5px] text-[var(--text-secondary)]">{t('shift.open.body')}</p>
    <label class="block text-[13px]">
      <span class="font-semibold">{t('shift.open.amount')}</span>
      <div>
        <MoneyInput
          id="shift-open-amount"
          bind:value={amount}
          placeholder="0"
          class="mt-1 w-full h-12 px-3 rounded-md text-[18px] font-bold tabular-nums text-end border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
        />
      </div>
    </label>
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    <div class="flex justify-end gap-2">
      <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('shift.open.later')}</button>
      <button type="submit" class="btn btn-primary" disabled={busy}><i class="icon-lock-open"></i> {busy ? t('shift.open.busy') : t('shift.open.submit')}</button>
    </div>
  </form>
  {/if}
</Modal>
