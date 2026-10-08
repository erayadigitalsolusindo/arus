<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { newIdempotencyKey, sales, type PayMethod, type Sale, type SaleInput } from '#lib/sales/api.ts';
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

  // Jenis transaksi mengikuti layar legacy: F1 Tunai, F2 Kredit, F3 Non-tunai, F4 Split.
  type Mode = 'cash' | 'credit' | 'noncash' | 'split';
  type Field = { amount: string; bank: string; ref: string };
  const MODES: { id: Mode; key: string; label: string; enabled: boolean }[] = [
    { id: 'cash', key: 'F1', label: 'pos.mode.cash', enabled: true },
    { id: 'credit', key: 'F2', label: 'pos.mode.credit', enabled: false }, // menunggu modul piutang
    { id: 'noncash', key: 'F3', label: 'pos.mode.noncash', enabled: true },
    { id: 'split', key: 'F4', label: 'pos.mode.split', enabled: true }
  ];
  /** Metode yang tampil (berurutan) untuk tiap jenis. Hanya yang tampil yang ikut dikirim. */
  const SHOWN: Record<Mode, PayMethod[]> = {
    cash: ['cash'],
    credit: [],
    noncash: ['transfer', 'debit', 'credit_card', 'ewallet'],
    split: ['cash', 'transfer']
  };
  const needsBank = (m: PayMethod) => m !== 'cash';

  /** sen → string desimal untuk input/API ("12500" atau "12500.50"), tanpa float. */
  const dec = (c: bigint) => (c % 100n === 0n ? String(c / 100n) : `${c / 100n}.${String(c % 100n).padStart(2, '0')}`);
  const money = (c: bigint) => formatCurrency(centsToNumber(c), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const blank = (): Record<PayMethod, Field> => ({
    cash: { amount: '', bank: '', ref: '' },
    transfer: { amount: '', bank: '', ref: '' },
    debit: { amount: '', bank: '', ref: '' },
    credit_card: { amount: '', bank: '', ref: '' },
    ewallet: { amount: '', bank: '', ref: '' }
  });

  let mode = $state<Mode>('cash');
  let fields = $state(untrack(() => ({ ...blank(), cash: { amount: dec(total), bank: '', ref: '' } }))); // default: uang pas
  let submitting = $state(false);
  let error = $state('');
  let done = $state<Sale | null>(null);
  let body = $state<HTMLElement>();
  // Satu kunci selama jendela ini terbuka: klik ganda / ulang setelah jaringan putus mengembalikan nota yang sama.
  const key = newIdempotencyKey();

  const shown = $derived(SHOWN[mode]);
  const entered = $derived(shown.filter((m) => toCents(fields[m].amount) > 0n));
  const paid = $derived(entered.reduce((s, m) => s + toCents(fields[m].amount), 0n));
  const cashPaid = $derived(entered.includes('cash') ? toCents(fields.cash.amount) : 0n);
  const nonCashOver = $derived(paid - cashPaid > total);
  const diff = $derived(paid - total); // negatif = masih kurang
  const canSubmit = $derived(!submitting && entered.length > 0 && paid >= total && !nonCashOver);

  function focusFirst() {
    const el = body?.querySelector<HTMLInputElement>('input[data-pay]');
    el?.focus();
    el?.select();
  }

  async function setMode(next: Mode) {
    if (!MODES.find((m) => m.id === next)?.enabled || next === mode) return;
    mode = next;
    fields = blank();
    if (next === 'cash') fields.cash.amount = dec(total);
    error = '';
    await tick();
    focusFirst();
  }

  onMount(() => {
    // Modal merebut fokus ke dialog saat dibuka; tunggu satu putaran lalu pindah ke kolom utama.
    const id = setTimeout(focusFirst, 0);
    return () => clearTimeout(id);
  });

  /** Enter di kolom isian = pindah ke kolom berikutnya, terakhir ke tombol Simpan (UX kasir legacy). */
  function onBodyKeydown(e: KeyboardEvent) {
    if (e.key !== 'Enter' || !(e.target instanceof HTMLInputElement) || e.target.type === 'radio') return;
    const list = [...(body?.querySelectorAll<HTMLElement>('[data-pay]') ?? [])].filter((el) => !(el as HTMLInputElement).disabled);
    const next = list[list.indexOf(e.target) + 1];
    if (!next) return;
    e.preventDefault();
    next.focus();
    if (next instanceof HTMLInputElement) next.select();
  }

  function onWindowKeydown(e: KeyboardEvent) {
    if (done) return;
    const hit = MODES.find((m) => m.key === e.key);
    if (hit) {
      e.preventDefault(); // F1 = bantuan peramban
      void setMode(hit.id);
    } else if (e.key === 'End' && canSubmit) {
      e.preventDefault();
      void submit();
    }
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
        {
          ...build(),
          payments: entered.map((m) => {
            const f = fields[m];
            const ref = [f.bank.trim(), f.ref.trim()].filter(Boolean).join(' · ').slice(0, 100);
            return { method: m, amount: dec(toCents(f.amount)), ...(ref ? { ref_no: ref } : {}) };
          })
        },
        key
      );
    } catch (e) {
      error = describe(e);
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<Modal title={done ? t('pos.paidTitle') : t('pos.payTitle')} onclose={done ? ondone : onclose} wide={!done}>
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
      {#if done.member}
        <p class="text-[12.5px] text-[var(--text-secondary)]">
          <i class="icon-user-round me-1"></i>{done.member.name}
          {#if done.points_earned > 0}<span class="ms-1.5 font-semibold text-[var(--color-success-600)]">{t('members.pos.earned', { points: done.points_earned })}</span>{/if}
          {#if done.points_redeemed > 0}<span class="ms-1.5 font-semibold text-[var(--color-warning-600)]">{t('members.pos.redeemed', { points: done.points_redeemed })}</span>{/if}
        </p>
      {/if}
      <button type="button" class="btn btn-primary w-full" onclick={ondone}>{t('pos.newSale')}</button>
    </div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div bind:this={body} class="space-y-3" onkeydown={onBodyKeydown}>
      <div class="flex items-baseline justify-between gap-3">
        <span class="text-[22px] sm:text-[28px] font-bold text-[var(--color-danger-600)]">{t('pos.payTotalLabel')}</span>
        <span class="font-mono text-[30px] sm:text-[40px] font-extrabold tabular-nums text-[var(--color-danger-600)]">{money(total)}</span>
      </div>

      <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] items-center gap-2">
        <div class="text-[16px] font-semibold">{t('pos.modeLabel')}</div>
        <div class="flex flex-nowrap gap-1 overflow-x-auto scroll-thin" role="radiogroup" aria-label={t('pos.modeLabel')}>
          {#each MODES as m (m.id)}
            <label
              class="cursor-pointer select-none whitespace-nowrap rounded border px-2 py-1.5 text-[12px] font-semibold has-[:checked]:border-[var(--color-primary-500)] has-[:checked]:bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)] has-[:disabled]:opacity-50 has-[:disabled]:cursor-not-allowed border-[var(--border-default)]"
              title={m.enabled ? '' : t('pos.modeSoon')}
            >
              <input type="radio" name="paymode" class="sr-only" value={m.id} checked={mode === m.id} disabled={!m.enabled} onchange={() => setMode(m.id)} />
              {m.key} · {t(m.label as 'pos.mode.cash')}{#if !m.enabled}<span class="ms-1 text-[10.5px] font-normal">({t('pos.soon')})</span>{/if}
            </label>
          {/each}
        </div>
      </div>

      {#each shown as m (m)}
        <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-start">
          <div class="text-[16px] font-semibold pt-1">{t(`pos.fieldLabel.${m}` as 'pos.fieldLabel.cash')}</div>
          <div class="space-y-1.5">
            <MoneyInput
              data-pay
              bind:value={fields[m].amount}
              placeholder="0"
              aria-label={t(`pos.fieldLabel.${m}` as 'pos.fieldLabel.cash')}
              class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
            />
            {#if needsBank(m)}
              <div class="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-1.5">
                <input data-pay bind:value={fields[m].bank} maxlength="40" autocomplete="off" placeholder={t('pos.bank')} aria-label={t('pos.bank')} class="h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
                <input data-pay bind:value={fields[m].ref} maxlength="50" autocomplete="off" placeholder={t('pos.payRefFor.' + m as 'pos.payRefFor.transfer')} aria-label={t('pos.payRef')} class="h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
              </div>
            {/if}
          </div>
        </div>
      {/each}

      <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
        <div class="text-[16px] font-semibold text-[var(--color-primary-600)]">{t('pos.payTotalPaid')}</div>
        <input readonly tabindex="-1" value={money(paid)} aria-label={t('pos.payTotalPaid')} class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[color-mix(in_oklab,var(--color-danger-500)_16%,transparent)]" />
      </div>
      <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
        <div class="text-[16px] font-semibold text-[var(--color-primary-600)]">{diff < 0n ? t('pos.payShort') : t('pos.payChange')}</div>
        <input readonly tabindex="-1" value={money(diff)} aria-label={t('pos.payChange')} class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[color-mix(in_oklab,var(--color-success-500)_22%,transparent)]" />
      </div>

      {#if nonCashOver}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage('NON_CASH_OVER')}</p>{/if}
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

      <div class="flex gap-2 justify-end pt-1">
        <button type="button" class="btn" onclick={onclose} disabled={submitting}>{t('pos.payCancel')}</button>
        <button data-pay type="button" class="btn btn-primary btn-lg" onclick={submit} disabled={!canSubmit}>
          <i class="icon-printer me-1.5"></i>{submitting ? t('pos.paying') : t('pos.paySave')}
        </button>
      </div>
    </div>
  {/if}
</Modal>
