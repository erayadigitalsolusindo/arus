<script lang="ts">
  import { untrack } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { t, formatCurrency, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { PAY_METHODS, newIdempotencyKey, sales, type PayMethod, type Sale, type SaleInput } from '#lib/sales/api.ts';
  import { centsToNumber, toCents } from '#lib/pos/money.ts';

  let {
    total,
    build,
    lineName,
    onclose,
    ondone
  }: {
    /** Total pratinjau (sen). Total resmi dihitung server; bila beda, server yang benar dan pembayaran divalidasi ulang. */
    total: bigint;
    /** Isi nota tanpa pembayaran. */
    build: () => Omit<SaleInput, 'payments'>;
    /** Nama baris keranjang ke-n (untuk pesan galat per baris). */
    lineName: (index: number) => string;
    onclose: () => void;
    /** Dipanggil saat pengguna menutup layar sukses ("Transaksi baru"). */
    ondone: () => void;
  } = $props();

  type Row = { method: PayMethod; amount: string; ref: string };
  /** sen → string desimal untuk input/API ("12500" atau "12500.50"), tanpa float. */
  const dec = (c: bigint) => (c % 100n === 0n ? String(c / 100n) : `${c / 100n}.${String(c % 100n).padStart(2, '0')}`);
  const money = (c: bigint) => formatCurrency(centsToNumber(c));

  let rows = $state<Row[]>([{ method: 'cash', amount: dec(untrack(() => total)), ref: '' }]);
  let submitting = $state(false);
  let error = $state('');
  let done = $state<Sale | null>(null);
  // Satu kunci selama jendela ini terbuka: klik ganda / ulang setelah jaringan putus mengembalikan nota yang sama.
  const key = newIdempotencyKey();

  const paid = $derived(rows.reduce((s, r) => s + toCents(r.amount), 0n));
  const cashPaid = $derived(rows.filter((r) => r.method === 'cash').reduce((s, r) => s + toCents(r.amount), 0n));
  const nonCashOver = $derived(paid - cashPaid > total);
  const remaining = $derived(total > paid ? total - paid : 0n);
  const change = $derived(paid > total ? paid - total : 0n);
  const canSubmit = $derived(!submitting && paid >= total && !nonCashOver && rows.every((r) => toCents(r.amount) > 0n));

  function addRow() {
    const used = new Set(rows.map((r) => r.method));
    const method = PAY_METHODS.find((m) => !used.has(m)) ?? 'cash';
    rows.push({ method, amount: remaining > 0n ? dec(remaining) : '', ref: '' });
  }
  const removeRow = (i: number) => {
    if (rows.length > 1) rows.splice(i, 1);
  };
  function exact(i: number) {
    const others = rows.reduce((s, r, j) => (j === i ? s : s + toCents(r.amount)), 0n);
    rows[i].amount = dec(total > others ? total - others : 0n);
  }

  function describe(err: unknown): string {
    if (err instanceof ApiError && err.code === 'STOCK_INSUFFICIENT') return err.message; // memuat nama barang
    if (err instanceof ApiError && err.code === 'VALIDATION') {
      return Object.entries(err.fields)
        .map(([f, c]) => {
          const m = /^lines\.(\d+)\./.exec(f);
          const text = fieldMessage(c) ?? '';
          return m ? `${t('pos.lineError', { n: Number(m[1]) + 1, name: lineName(Number(m[1])) })}: ${text}` : text;
        })
        .join(' ');
    }
    return errorMessage(err);
  }

  async function submit() {
    if (!canSubmit) return;
    submitting = true;
    error = '';
    try {
      done = await sales.create(
        { ...build(), payments: rows.map((r) => ({ method: r.method, amount: dec(toCents(r.amount)), ...(r.ref.trim() ? { ref_no: r.ref.trim() } : {}) })) },
        key
      );
    } catch (e) {
      error = describe(e);
    } finally {
      submitting = false;
    }
  }
</script>

<Modal title={done ? t('pos.paidTitle') : t('pos.payTitle')} onclose={done ? ondone : onclose}>
  {#if done}
    <div class="text-center space-y-3 py-2">
      <span class="grid place-items-center size-14 mx-auto rounded-full bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-600)]"><i class="icon-check text-[28px]"></i></span>
      <div class="text-[12px] text-[var(--text-tertiary)]">{t('pos.paidDoc')}</div>
      <div class="font-mono text-[20px] font-bold">{done.doc_no}</div>
      <dl class="text-[13px] space-y-1 text-start max-w-xs mx-auto">
        <div class="flex justify-between"><dt>{t('pos.payTotal')}</dt><dd class="font-semibold tabular-nums">{money(toCents(done.total))}</dd></div>
        <div class="flex justify-between"><dt>{t('pos.payPaid')}</dt><dd class="tabular-nums">{money(toCents(done.paid))}</dd></div>
        <div class="flex justify-between text-[16px] font-extrabold text-[var(--color-success-600)]"><dt>{t('pos.paidChange')}</dt><dd class="tabular-nums">{money(toCents(done.change))}</dd></div>
      </dl>
      <button type="button" class="btn btn-primary w-full" onclick={ondone}>{t('pos.newSale')}</button>
    </div>
  {:else}
    <div class="space-y-3">
      <div class="flex items-baseline justify-between rounded-md p-3 bg-[var(--surface-sunken)]">
        <span class="text-[12px] font-semibold uppercase">{t('pos.payTotal')}</span>
        <span class="font-mono text-[26px] font-extrabold tabular-nums text-[var(--color-danger-600)]">{money(total)}</span>
      </div>

      {#each rows as r, i (i)}
        <div class="rounded-md border border-[var(--border-subtle)] p-2.5 space-y-2">
          <div class="flex gap-2">
            <select bind:value={r.method} class="h-9 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2 text-[13px]" aria-label={t('pos.payTitle')}>
              {#each PAY_METHODS as m (m)}<option value={m}>{t(`pos.payMethods.${m}` as MessageKey)}</option>{/each}
            </select>
            <input
              bind:value={r.amount}
              inputmode="decimal"
              aria-label={t('pos.payAmount')}
              placeholder={t('pos.payAmount')}
              class="grow min-w-0 h-9 px-2 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
            />
            <button type="button" class="btn btn-sm" onclick={() => exact(i)}>{t('pos.payExact')}</button>
            {#if rows.length > 1}
              <button type="button" class="header-icon-btn" aria-label={t('pos.payRemove')} title={t('pos.payRemove')} onclick={() => removeRow(i)}><i class="icon-trash-2 text-[14px]"></i></button>
            {/if}
          </div>
          {#if r.method !== 'cash'}
            <input bind:value={r.ref} maxlength="100" placeholder={t('pos.payRef')} aria-label={t('pos.payRef')} class="w-full h-8 px-2 text-[12.5px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none" />
          {/if}
        </div>
      {/each}

      {#if rows.length < PAY_METHODS.length}
        <button type="button" class="btn btn-sm" onclick={addRow}><i class="icon-plus me-1 text-[12px]"></i>{t('pos.payAdd')}</button>
      {/if}

      <dl class="text-[13px] space-y-1 border-t border-[var(--border-subtle)] pt-2">
        <div class="flex justify-between"><dt>{t('pos.payPaid')}</dt><dd class="tabular-nums">{money(paid)}</dd></div>
        {#if remaining > 0n}<div class="flex justify-between font-bold text-[var(--color-danger-600)]"><dt>{t('pos.payRemaining')}</dt><dd class="tabular-nums">{money(remaining)}</dd></div>{/if}
        {#if change > 0n}<div class="flex justify-between font-bold text-[var(--color-success-600)]"><dt>{t('pos.payChange')}</dt><dd class="tabular-nums">{money(change)}</dd></div>{/if}
      </dl>
      {#if nonCashOver}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage('NON_CASH_OVER')}</p>{/if}
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

      <div class="flex gap-2 justify-end pt-1">
        <button type="button" class="btn" onclick={onclose} disabled={submitting}>{t('pos.payCancel')}</button>
        <button type="button" class="btn btn-primary" onclick={submit} disabled={!canSubmit}>{submitting ? t('pos.paying') : t('pos.payConfirm')}</button>
      </div>
    </div>
  {/if}
</Modal>
