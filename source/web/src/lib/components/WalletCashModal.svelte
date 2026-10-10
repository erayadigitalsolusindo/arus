<script lang="ts">
  // Uang yang mengubah saldo titipan lewat metode bayar biasa: top-up/tarik deposit member, pencairan kredit pemasok.
  // Server menentukan saldo dan menolak bila tidak cukup; kunci idempotensi ikut isi permintaan (klik ganda tidak mencatat dua kali).
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import type { CashInput, WalletAccount } from '#lib/wallet/api.ts';
  import { toCents, centsToNumber } from '#lib/pos/money.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let {
    title,
    hint,
    balance = null,
    max = null,
    onsubmit,
    onclose,
    ondone
  }: {
    title: string;
    hint: string;
    /** Saldo sekarang (ditampilkan). */
    balance?: string | null;
    /** Batas jumlah di sisi klien (mis. tarik ≤ saldo); server tetap memeriksa. */
    max?: string | null;
    onsubmit: (input: CashInput, key: string) => Promise<WalletAccount>;
    onclose: () => void;
    ondone: (acc: WalletAccount, docNo: string) => void;
  } = $props();

  const money = (v: string | bigint) => formatCurrency(typeof v === 'bigint' ? centsToNumber(v) : Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const inputClass = 'w-full h-10 px-3 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]';

  let methods = $state<PaymentMethod[]>([]);
  let methodId = $state('');
  let amount = $state('');
  let ref = $state('');
  let note = $state('');
  let busy = $state(false);
  let error = $state('');
  let fields = $state<Record<string, string>>({});
  let attempt: { sig: string; key: string } | null = null;

  const picked = $derived(methods.find((m) => m.id === methodId));
  const needsRef = $derived(!!picked && picked.kind !== 'cash');
  const amountC = $derived(toCents(amount));
  const over = $derived(max !== null && amountC > toCents(max));
  const valid = $derived(!!picked && amountC > 0n && !over && (!needsRef || ref.trim() !== ''));

  onMount(async () => {
    try {
      methods = await paymentMethodsLookup.all('wallet');
      methodId = methods[0]?.id ?? '';
    } catch (e) {
      error = errorMessage(e);
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    const input: CashInput = {
      amount: amount.trim().replace(',', '.'),
      method_id: methodId,
      ...(needsRef ? { ref_no: ref.trim() } : {}),
      ...(note.trim() ? { note: note.trim() } : {})
    };
    const sig = JSON.stringify(input);
    if (!attempt || attempt.sig !== sig) attempt = { sig, key: `wl-${crypto.randomUUID()}` };
    busy = true;
    error = '';
    fields = {};
    try {
      const acc = await onsubmit(input, attempt.key);
      ondone(acc, acc.entries[0]?.doc_no ?? '');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        fields = Object.fromEntries(Object.entries(err.fields).map(([k, c]) => [k, fieldMessage(c) ?? c]));
      } else {
        error = errorMessage(err);
      }
    } finally {
      busy = false;
    }
  }
</script>

<Modal {title} {onclose}>
  <form class="space-y-3" onsubmit={submit} autocomplete="off">
    <p class="text-[12.5px] text-[var(--text-secondary)]">{hint}</p>
    {#if balance !== null}<p class="text-[13px]">{t('deposits.balance')}: <span class="font-semibold tabular-nums">{money(balance)}</span></p>{/if}
    <label class="block text-[13px]">
      <span class="font-semibold">{t('deposits.form.amount')}</span>
      <MoneyInput bind:value={amount} placeholder="0" aria-label={t('deposits.form.amount')} class="{inputClass} mt-1 text-end tabular-nums" />
      {#if over}<p class="mt-1 text-[12px] text-[var(--color-danger-600)]">{fieldMessage('BALANCE_INSUFFICIENT')}</p>{/if}
      {#if fields.amount}<p class="mt-1 text-[12px] text-[var(--color-danger-600)]">{fields.amount}</p>{/if}
    </label>
    <div class="block text-[13px]">
      <span class="font-semibold">{t('deposits.form.method')}</span>
      <div class="mt-1"><Select bind:value={methodId} ariaLabel={t('deposits.form.method')} options={methods.map((m) => ({ value: m.id, label: m.name }))} /></div>
      {#if fields.method_id}<p class="mt-1 text-[12px] text-[var(--color-danger-600)]">{fields.method_id}</p>{/if}
    </div>
    {#if needsRef}
      <label class="block text-[13px]">
        <span class="font-semibold">{t('deposits.form.ref')}</span>
        <input bind:value={ref} maxlength="100" required class="{inputClass} mt-1" />
        <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('deposits.form.refHint')}</p>
        {#if fields.ref_no}<p class="mt-1 text-[12px] text-[var(--color-danger-600)]">{fields.ref_no}</p>{/if}
      </label>
    {/if}
    <label class="block text-[13px]">
      <span class="font-semibold">{t('deposits.form.note')}</span>
      <input bind:value={note} maxlength="200" class="{inputClass} mt-1" />
    </label>
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    <div class="flex justify-end gap-2">
      <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('deposits.form.cancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('deposits.form.saving') : t('deposits.form.save')}</button>
    </div>
  </form>
</Modal>
