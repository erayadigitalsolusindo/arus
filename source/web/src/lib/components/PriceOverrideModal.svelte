<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { centsToNumber, toCents } from '#lib/pos/money.ts';

  let {
    name,
    listPrice,
    current,
    onclose,
    onapply
  }: {
    name: string;
    /** Harga normal per satuan (string desimal dari server). */
    listPrice: string;
    /** Harga yang sedang berlaku (kosong = normal). */
    current: string;
    onclose: () => void;
    /** Dipanggil setelah PIN valid: harga baru + penyetuju + PIN (disimpan di memori sampai nota selesai). */
    onapply: (price: string, approver: Approver, pin: string) => void;
  } = $props();

  let price = $state(untrack(() => current));
  let approverId = $state('');
  let pin = $state('');
  let list = $state<Approver[] | null>(null);
  let busy = $state(false);
  let error = $state('');

  onMount(async () => {
    try {
      list = await approvals.approvers();
      if (list.length === 1) approverId = list[0].id;
    } catch (e) {
      error = errorMessage(e);
      list = [];
    }
  });

  const cents = $derived(toCents(price));
  const valid = $derived(price.trim() !== '' && /^\d*[.,]?\d{0,2}$/.test(price.trim()) && approverId !== '' && /^\d{6}$/.test(pin));

  async function apply(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      const approver = await approvals.check(approverId, pin);
      onapply(String(cents / 100n) + '.' + String(cents % 100n).padStart(2, '0'), approver, pin);
    } catch (err) {
      pin = '';
      error = err instanceof ApiError && err.code === 'PIN_LOCKED' ? errorMessage(err).replace('{count}', String(err.retryAfter)) : errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('pos.override.title')} {onclose}>
  <form class="space-y-3" onsubmit={apply} autocomplete="off">
    <p class="text-[12.5px] text-[var(--text-secondary)]">{t('pos.override.body')}</p>
    <dl class="text-[13px] rounded-md bg-[var(--surface-sunken)] p-3 space-y-1">
      <div class="flex justify-between gap-3"><dt>{t('pos.override.item')}</dt><dd class="font-semibold text-end">{name}</dd></div>
      <div class="flex justify-between gap-3"><dt>{t('pos.override.listPrice')}</dt><dd class="tabular-nums">{formatCurrency(centsToNumber(toCents(listPrice)))}</dd></div>
    </dl>

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.newPrice')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input bind:value={price} inputmode="decimal" autofocus required class="mt-1 w-full h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
    </label>

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.approver')}</span>
      <select bind:value={approverId} required class="mt-1 w-full h-10 px-2 rounded border border-[var(--border-default)] bg-[var(--surface-base)]">
        <option value="" disabled>{t('pos.override.pickApprover')}</option>
        {#each list ?? [] as a (a.id)}<option value={a.id}>{a.name}</option>{/each}
      </select>
      {#if list && list.length === 0}<span class="text-[12px] text-[var(--color-warning-600)]">{t('pos.override.noApprovers')}</span>{/if}
    </label>

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.pin')}</span>
      <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" required class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
    </label>

    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

    <div class="flex justify-end gap-2 pt-1">
      <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('pos.payCancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('pos.override.checking') : t('pos.override.apply')}</button>
    </div>
  </form>
</Modal>
