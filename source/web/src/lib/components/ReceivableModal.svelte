<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { receivables, type ReceivableDetail } from '#lib/receivables/api.ts';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { memberDeposits } from '#lib/wallet/api.ts';
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

  let d = $state<ReceivableDetail | null>(null);
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

  const canPay = $derived(can('member_receivables', 'create'));
  const canVoid = $derived(can('receivable_opening', 'delete'));
  let voidOpen = $state(false);
  let voidReason = $state('');
  let voiding = $state(false);
  async function voidOpening(e: Event) {
    e.preventDefault();
    if (!d || voiding || voidReason.trim().length < 3) return;
    voiding = true;
    error = '';
    try {
      d = await receivables.voidOpening(d.id, voidReason.trim().slice(0, 200));
      voidOpen = false;
      onchanged();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      voiding = false;
    }
  }
  const balance = $derived(d ? toCents(d.balance) : 0n);
  const amountC = $derived(toCents(amount));
  const picked = $derived(methods.find((m) => m.id === methodId));
  /** Saldo deposit member (hanya dimuat bila metode Deposit Member dipilih). */
  let walletBal = $state<string | null>(null);
  $effect(() => {
    const memberId = d?.member_id;
    if (picked?.kind !== 'deposit' || !memberId) {
      walletBal = null;
      return;
    }
    void memberDeposits.account(memberId).then((a) => (walletBal = a.balance)).catch((e) => (error = errorMessage(e)));
  });
  const walletOver = $derived(walletBal !== null && amountC > toCents(walletBal));
  const valid = $derived(!!picked && amountC > 0n && amountC <= balance && !walletOver && (picked.kind !== 'deposit' || walletBal !== null));
  const feeLine = $derived.by(() => {
    if (!picked || amountC <= 0n) return '';
    const pctH = BigInt(Math.round(Number(picked.fee_pct ?? 0) * 100));
    const flat = toCents(picked.fee_flat ?? '');
    if (pctH === 0n && flat === 0n) return '';
    const fee = (amountC * pctH + 5000n) / 10000n + flat;
    const rate = [Number(picked.fee_pct ?? 0) > 0 ? `${Number(picked.fee_pct)}%` : '', flat > 0n ? money(flat) : ''].filter(Boolean).join(' + ');
    return t('pos.feeLine', { rate, fee: money(fee), who: t(picked.fee_bearer === 'customer' ? 'catalog.paymentMethods.bearerCustomer' : 'catalog.paymentMethods.bearerStore') });
  });

  onMount(async () => {
    try {
      const [detail, list] = await Promise.all([receivables.get(id), canPay ? paymentMethodsLookup.all('receivable') : Promise.resolve([])]);
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
      const res = await receivables.pay(d.id, { method_id: methodId, amount: dec(amountC), ...(r ? { ref_no: r } : {}), ...(n ? { note: n } : {}) }, key);
      d = res;
      saved = t('receivables.modal.saved', { doc: res.payments.at(-1)?.doc_no ?? '' });
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

  const statusClass = { open: 'badge-info', overdue: 'badge-danger', paid: 'badge-success', void: 'badge-neutral' } as const;
</script>

<Modal title={d ? t('receivables.modal.title', { doc: d.doc_no }) : t('receivables.detail')} {onclose} wide>
  {#if !d}
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{:else}<p class="text-[12.5px] text-[var(--text-tertiary)]">…</p>{/if}
  {:else}
    <div class="space-y-4">
      <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-[13px] sm:grid-cols-4">
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.member')}</dt><dd class="font-semibold">{d.member_name}<span class="ms-1 font-mono text-[11px] font-normal text-[var(--text-tertiary)]">{d.member_code}</span></dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.date')}</dt><dd>{formatDate(d.sale_at)}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.due')}</dt><dd>{d.due_date ? formatDate(d.due_date) : t('receivables.noDue')}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.amount')}</dt><dd class="tabular-nums">{money(d.amount)}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.paid')}</dt><dd class="tabular-nums">{money(d.paid)}</dd></div>
        <div><dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.returned')}</dt><dd class="tabular-nums">{money(d.returned)}</dd></div>
        <div>
          <dt class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('receivables.col.balance')}</dt>
          <dd class="flex items-center gap-2"><span class="text-[16px] font-extrabold tabular-nums {balance > 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{money(d.balance)}</span><span class="badge-soft {statusClass[d.status]}">{t(`receivables.status.${d.status}`)}</span></dd>
        </div>
      </dl>

      {#if d.kind === 'opening'}
        <div class="rounded-md bg-[var(--surface-sunken)] p-3 text-[12.5px] space-y-0.5">
          <div><span class="badge-soft badge-warning me-1">{t('receivables.opening.badge')}</span>{#if d.ref_no}{t('receivables.opening.refLabel')}: <b>{d.ref_no}</b>{/if}</div>
          {#if d.note}<div>{d.note}</div>{/if}
          {#if d.created_by}<div class="text-[var(--text-tertiary)]">{t('receivables.opening.createdBy')} {d.created_by}</div>{/if}
          {#if d.voided_at}<div class="font-semibold text-[var(--color-danger-600)]">{t('receivables.opening.voided', { time: formatDateTime(d.voided_at), by: d.voided_by ?? '', reason: d.void_reason ?? '' })}</div>{/if}
        </div>
      {/if}

      <section>
        <h3 class="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('receivables.modal.history')}</h3>
        {#if d.payments.length === 0}
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('receivables.modal.noPayments')}</p>
        {:else}
          <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
            <table class="w-full text-[12.5px]">
              <tbody>
                {#each d.payments as p (p.id)}
                  <tr class="border-b border-[var(--border-subtle)] last:border-0">
                    <td class="px-3 py-2 whitespace-nowrap"><div class="font-mono text-[12px] font-semibold">{p.doc_no}</div><div class="text-[11px] text-[var(--text-tertiary)]">{formatDateTime(p.created_at)}</div></td>
                    <td class="px-3 py-2">{p.method_name}{#if p.ref_no}<div class="text-[11px] text-[var(--text-tertiary)]">{p.ref_no}</div>{/if}{#if p.note}<div class="text-[11px] text-[var(--text-tertiary)]">{p.note}</div>{/if}</td>
                    <td class="px-3 py-2 text-[11.5px] text-[var(--text-tertiary)]">{t('receivables.modal.receivedBy')} {p.received_by || '—'}</td>
                    <td class="px-3 py-2 text-end tabular-nums font-semibold whitespace-nowrap">{money(p.amount)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

      {#if d.kind === 'opening' && !d.voided_at && d.payments.length === 0 && canVoid}
        {#if voidOpen}
          <form class="space-y-2 rounded border border-[color-mix(in_oklab,var(--color-danger-500)_40%,transparent)] p-3" onsubmit={voidOpening}>
            <p class="text-[12px] text-[var(--text-secondary)]">{t('receivables.opening.voidHint')}</p>
            <input bind:value={voidReason} maxlength="200" placeholder={t('receivables.opening.voidReason')} aria-label={t('receivables.opening.voidReason')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)]" />
            <div class="flex justify-end gap-2">
              <button type="button" class="btn btn-sm" onclick={() => (voidOpen = false)} disabled={voiding}>{t('receivables.modal.close')}</button>
              <button type="submit" class="btn btn-sm btn-danger" disabled={voiding || voidReason.trim().length < 3}>{voiding ? t('receivables.opening.voiding') : t('receivables.opening.voidConfirm')}</button>
            </div>
          </form>
        {:else}
          <button type="button" class="btn btn-sm" onclick={() => (voidOpen = true)}><i class="icon-ban"></i> {t('receivables.opening.void')}</button>
        {/if}
      {/if}

      {#if d.voided_at}
        {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
        <div class="flex justify-end"><button type="button" class="btn" onclick={onclose}>{t('receivables.modal.close')}</button></div>
      {:else if canPay && balance > 0n}
        <form class="space-y-3 rounded border border-[var(--border-default)] p-3" onsubmit={submit} autocomplete="off">
          <h3 class="text-[12px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('receivables.modal.payTitle')}</h3>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-[13px]">
              <span class="font-semibold">{t('receivables.modal.method')}</span>
              <div class="mt-1"><Select bind:value={methodId} ariaLabel={t('receivables.modal.method')} options={methods.map((m) => ({ value: m.id, label: m.name }))} /></div>
            </label>
            <label class="block text-[13px]">
              <span class="font-semibold">{t('receivables.modal.amount')}</span>
              <div class="mt-1 flex gap-1.5">
                <MoneyInput bind:value={amount} placeholder="0" aria-label={t('receivables.modal.amount')} class="w-full h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
                <button type="button" class="btn btn-sm shrink-0" onclick={() => (amount = dec(balance))}>{t('receivables.modal.full')}</button>
              </div>
            </label>
          </div>
          {#if amountC > balance}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage('OVERPAID')}</p>{/if}
          {#if feeLine}<p class="text-[12px] text-[var(--color-warning-600)]">{feeLine}</p>{/if}
          {#if amountC > 0n && amountC <= balance}<p class="text-[12px] text-[var(--text-secondary)]">{t('receivables.modal.afterPay', { balance: money(balance - amountC) })}</p>{/if}
          {#if walletBal !== null}<p class="text-[12px] {walletOver ? 'text-[var(--color-danger-600)] font-semibold' : 'text-[var(--text-secondary)]'}">{t('pos.depositBalance', { amount: money(walletBal) })}{#if walletOver} · {t('pos.depositOver')}{/if}</p>{/if}
          {#if picked && picked.kind !== 'cash' && picked.kind !== 'deposit'}
            <input bind:value={ref} maxlength="100" placeholder={t('receivables.modal.ref')} aria-label={t('receivables.modal.ref')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
          {/if}
          <input bind:value={note} maxlength="200" placeholder={t('receivables.modal.note')} aria-label={t('receivables.modal.note')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
          {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
          {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}
          <div class="flex justify-end gap-2">
            <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('receivables.modal.close')}</button>
            <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('receivables.modal.saving') : t('receivables.modal.save')}</button>
          </div>
        </form>
      {:else}
        {#if balance <= 0n}<p class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{t('receivables.modal.settled')}</p>{/if}
        {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}
        <div class="flex justify-end"><button type="button" class="btn" onclick={onclose}>{t('receivables.modal.close')}</button></div>
      {/if}
    </div>
  {/if}
</Modal>
