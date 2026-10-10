<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { payables, type PayableDetail } from '#lib/payables/api.ts';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { supplierCredits } from '#lib/wallet/api.ts';
  import { newIdempotencyKey } from '#lib/sales/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { centsToNumber, toCents } from '#lib/pos/money.ts';
  import { t, formatCurrency, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let {
    id,
    onclose,
    onchanged
  }: {
    id: string;
    onclose: () => void;
    /** Dipanggil setelah pembayaran tersimpan agar daftar memuat ulang. */
    onchanged: () => void;
  } = $props();

  const money = (v: string | bigint) => formatCurrency(typeof v === 'bigint' ? centsToNumber(v) : Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  /** sen → string desimal untuk API, tanpa float. */
  const dec = (c: bigint) => (c % 100n === 0n ? String(c / 100n) : `${c / 100n}.${String(c % 100n).padStart(2, '0')}`);

  let d = $state<PayableDetail | null>(null);
  let methods = $state<PaymentMethod[]>([]);
  let methodId = $state('');
  let amount = $state('');
  let ref = $state('');
  let note = $state('');
  let error = $state('');
  let saved = $state('');
  let busy = $state(false);
  // Satu kunci per isian pembayaran: klik ganda / ulang setelah jaringan putus tidak mencatat dua kali. Diganti setelah tersimpan.
  let key = newIdempotencyKey();

  const canPay = $derived(can('supplier_payables', 'create'));
  const balance = $derived(d ? toCents(d.balance) : 0n);
  const amountC = $derived(toCents(amount));
  const picked = $derived(methods.find((m) => m.id === methodId));
  /** Saldo kredit pemasok (hanya dimuat bila metode Kredit Pemasok dipilih). */
  let walletBal = $state<string | null>(null);
  $effect(() => {
    const supplierId = d?.supplier_id;
    if (picked?.kind !== 'supplier_credit' || !supplierId) {
      walletBal = null;
      return;
    }
    void supplierCredits.account(supplierId).then((a) => (walletBal = a.balance)).catch((e) => (error = errorMessage(e)));
  });
  const walletOver = $derived(walletBal !== null && amountC > toCents(walletBal));
  const valid = $derived(!!picked && amountC > 0n && amountC <= balance && !walletOver && (picked.kind !== 'supplier_credit' || walletBal !== null));
  onMount(async () => {
    try {
      const [detail, list] = await Promise.all([payables.get(id), canPay ? paymentMethodsLookup.all('payable') : Promise.resolve([])]);
      d = detail;
      methods = list;
      methodId = list[0]?.id ?? '';
    } catch (e) {
      error = errorMessage(e);
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy || !d) return;
    busy = true;
    error = '';
    saved = '';
    try {
      const r = ref.trim().slice(0, 100);
      const n = note.trim().slice(0, 200);
      const res = await payables.pay(d.id, { method_id: methodId, amount: dec(amountC), ...(r ? { ref_no: r } : {}), ...(n ? { note: n } : {}) }, key);
      d = res;
      saved = t('payables.modal.saved', { doc: res.payments.at(-1)?.doc_no ?? '' });
      amount = '';
      ref = '';
      note = '';
      key = newIdempotencyKey();
      onchanged();
    } catch (err) {
      error = err instanceof ApiError && err.code === 'VALIDATION' ? Object.values(err.fields).map((c) => fieldMessage(c)).join(' ') : errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const statusClass = { open: 'badge-info', overdue: 'badge-danger', paid: 'badge-success' } as const;
</script>

<Modal title={d ? t('payables.modal.title', { doc: d.doc_no }) : t('payables.detail')} {onclose} wide>
  {#if !d}
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{:else}<p class="text-[12.5px] text-[var(--text-tertiary)]">…</p>{/if}
  {:else}
    <div class="space-y-4">
      <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-[13px] sm:grid-cols-3">
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.supplier')}</dt><dd class="font-semibold">{d.supplier_name}{#if d.supplier_invoice_no}<span class="ms-1 font-mono text-[11px] font-normal text-[var(--text-tertiary)]">{d.supplier_invoice_no}</span>{/if}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.date')}</dt><dd>{formatDate(d.purchase_date)}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.due')}</dt><dd>{d.due_date ? formatDate(d.due_date) : t('payables.noDue')}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.amount')}</dt><dd class="tabular-nums">{money(d.amount)}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.paid')}</dt><dd class="tabular-nums">{money(d.paid)}{#if Number(d.returned) > 0}<div class="text-[11px] text-[var(--text-tertiary)]">{t('payables.col.returned')} {money(d.returned)}</div>{/if}</dd></div>
        <div>
          <dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('payables.col.balance')}</dt>
          <dd class="flex items-center gap-2"><span class="text-[16px] font-extrabold tabular-nums {balance > 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{money(d.balance)}</span><span class="badge-soft {statusClass[d.status]}">{t(`payables.status.${d.status}`)}</span></dd>
        </div>
      </dl>

      <section>
        <h3 class="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('payables.modal.history')}</h3>
        {#if d.payments.length === 0}
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('payables.modal.noPayments')}</p>
        {:else}
          <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
            <table class="w-full text-[12.5px]">
              <tbody>
                {#each d.payments as p (p.id)}
                  <tr class="border-b border-[var(--border-subtle)] last:border-0">
                    <td class="px-3 py-2 whitespace-nowrap"><div class="font-mono text-[12px] font-semibold">{p.doc_no}</div><div class="text-[11px] text-[var(--text-tertiary)]">{formatDateTime(p.created_at)}</div></td>
                    <td class="px-3 py-2">{p.method_name}{#if p.ref_no}<div class="text-[11px] text-[var(--text-tertiary)]">{p.ref_no}</div>{/if}{#if p.note}<div class="text-[11px] text-[var(--text-tertiary)]">{p.note}</div>{/if}</td>
                    <td class="px-3 py-2 text-[11.5px] text-[var(--text-tertiary)]">{t('payables.modal.paidBy')} {p.paid_by || '—'}</td>
                    <td class="px-3 py-2 text-end tabular-nums font-semibold whitespace-nowrap">{money(p.amount)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

      {#if canPay && balance > 0n}
        <form class="space-y-3 rounded border border-[var(--border-default)] p-3" onsubmit={submit} autocomplete="off">
          <h3 class="text-[12px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('payables.modal.payTitle')}</h3>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-[13px]">
              <span class="font-semibold">{t('payables.modal.method')}</span>
              <div class="mt-1"><Select bind:value={methodId} ariaLabel={t('payables.modal.method')} options={methods.map((m) => ({ value: m.id, label: m.name }))} /></div>
            </label>
            <label class="block text-[13px]">
              <span class="font-semibold">{t('payables.modal.amount')}</span>
              <div class="mt-1 flex gap-1.5">
                <MoneyInput bind:value={amount} placeholder="0" aria-label={t('payables.modal.amount')} class="w-full h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
                <button type="button" class="btn btn-sm shrink-0" onclick={() => (amount = dec(balance))}>{t('payables.modal.full')}</button>
              </div>
            </label>
          </div>
          {#if amountC > balance}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage('OVERPAID')}</p>{/if}
          {#if amountC > 0n && amountC <= balance}<p class="text-[12px] text-[var(--text-secondary)]">{t('payables.modal.afterPay', { balance: money(balance - amountC) })}</p>{/if}
          {#if walletBal !== null}<p class="text-[12px] {walletOver ? 'text-[var(--color-danger-600)] font-semibold' : 'text-[var(--text-secondary)]'}">{t('supplierCredits.balanceLine', { amount: money(walletBal) })}{#if walletOver} · {t('supplierCredits.over')}{/if}</p>{/if}
          {#if picked && picked.kind !== 'cash' && picked.kind !== 'supplier_credit'}
            <input bind:value={ref} maxlength="100" placeholder={t('payables.modal.ref')} aria-label={t('payables.modal.ref')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
          {/if}
          <input bind:value={note} maxlength="200" placeholder={t('payables.modal.note')} aria-label={t('payables.modal.note')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
          {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
          {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}
          <div class="flex justify-end gap-2">
            <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('payables.modal.close')}</button>
            <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('payables.modal.saving') : t('payables.modal.save')}</button>
          </div>
        </form>
      {:else}
        {#if balance <= 0n}<p class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{t('payables.modal.settled')}</p>{/if}
        {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}
        <div class="flex justify-end"><button type="button" class="btn" onclick={onclose}>{t('payables.modal.close')}</button></div>
      {/if}
    </div>
  {/if}
</Modal>
