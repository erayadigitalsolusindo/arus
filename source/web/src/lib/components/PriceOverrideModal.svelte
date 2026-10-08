<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount, untrack } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { centsToNumber, percentOf, toCents } from '#lib/pos/money.ts';

  let {
    name,
    listPrice,
    current,
    discount,
    discountTotal,
    onclose,
    onapply
  }: {
    name: string;
    /** Harga normal per satuan (string desimal dari server). */
    listPrice: string;
    /** Harga yang sedang berlaku (kosong = normal). */
    current: string;
    /** Potongan per satuan yang sedang berlaku (Rp, kosong = tidak ada). */
    discount: string;
    /** Potongan yang berlaku adalah total baris (bukan per satuan). */
    discountTotal: boolean;
    onclose: () => void;
    /** Dipanggil setelah PIN valid: harga baru + penyetuju + PIN (disimpan di memori sampai nota selesai). */
    onapply: (patch: { override?: string | null; disc?: string | null; discTotal?: boolean }, approver: Approver, pin: string) => void;
  } = $props();

  let mode = $state<'price' | 'discount'>('price');
  let price = $state(untrack(() => current));
  let unit = $state<'rp' | 'total' | 'pct'>(untrack(() => (discountTotal || !discount ? 'total' : 'rp')));
  let disc = $state(untrack(() => discount));
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
  const num = (s: string) => s.trim() !== '' && /^\d*[.,]?\d{0,2}$/.test(s.trim());
  // Potongan per satuan (sen): nominal Rp apa adanya, atau persen (maks 100) dari harga normal.
  const discCents = $derived(unit === 'pct' ? percentOf(toCents(listPrice), disc) : toCents(disc));
  const discOk = $derived(num(disc) && (unit !== 'pct' || toCents(disc) <= 10000n));
  const valid = $derived((mode === 'price' ? num(price) : discOk) && approverId !== '' && /^\d{6}$/.test(pin));

  async function apply(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      const approver = await approvals.check(approverId, pin);
      const fmt = (c: bigint) => String(c / 100n) + '.' + String(c % 100n).padStart(2, '0');
      if (mode === 'price') onapply({ override: fmt(cents) }, approver, pin);
      else onapply({ disc: discCents > 0n ? fmt(discCents) : null, discTotal: unit === 'total' }, approver, pin);
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
      <div class="flex justify-between gap-3"><dt>{t('pos.override.listPrice')}</dt><dd class="tabular-nums">{formatCurrency(centsToNumber(toCents(listPrice)), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</dd></div>
    </dl>

    <div class="grid grid-cols-2 gap-1 p-1 rounded-md bg-[var(--surface-sunken)] text-[12.5px] font-semibold" role="tablist">
      <button type="button" role="tab" aria-selected={mode === 'price'} class="h-8 rounded {mode === 'price' ? 'bg-[var(--surface-card)] shadow-sm text-[var(--color-primary-600)]' : 'text-[var(--text-secondary)]'}" onclick={() => (mode = 'price')}>{t('pos.override.tabPrice')}</button>
      <button type="button" role="tab" aria-selected={mode === 'discount'} class="h-8 rounded {mode === 'discount' ? 'bg-[var(--surface-card)] shadow-sm text-[var(--color-primary-600)]' : 'text-[var(--text-secondary)]'}" onclick={() => (mode = 'discount')}>{t('pos.override.tabDiscount')}</button>
    </div>

    {#if mode === 'price'}
    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.newPrice')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <MoneyInput bind:value={price} autofocus required class="mt-1 w-full h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
    </label>
    {:else}
    <div class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.discountLabel')}</span>
      <div class="mt-1 flex gap-2">
        <MoneyInput bind:value={disc} autofocus required class="grow min-w-0 h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
        <Select class="!w-auto min-w-24 !min-h-10" ariaLabel={t('pos.override.discountUnit')} bind:value={unit} options={[{ value: 'rp', label: t('pos.override.unitRp') }, { value: 'total', label: t('pos.override.unitTotal') }, { value: 'pct', label: '%' }]} />
      </div>
      {#if discOk}
        <span class="block mt-1 text-[12px] text-[var(--text-secondary)]">{t(unit === 'total' ? 'pos.override.discountPreviewTotal' : 'pos.override.discountPreview', { amount: formatCurrency(centsToNumber(discCents), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) })}</span>
      {/if}
    </div>
    {/if}

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.approver')}</span>
      <div class="mt-1"><Select bind:value={approverId} class="!min-h-10" options={[{ value: '', label: t('pos.override.pickApprover'), disabled: true }, ...(list ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
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
