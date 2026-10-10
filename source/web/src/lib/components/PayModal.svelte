<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { t, formatCurrency, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { newIdempotencyKey, sales, type Quote, type Sale, type SaleInput } from '#lib/sales/api.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { centsToNumber, toCents } from '#lib/pos/money.ts';
  import { focusOnMount } from '#lib/focus.ts';
  import { loadReceiptSettings, printErrorMessage, printReceipt } from '#lib/pos/receipt.ts';
  import ReceiptSettings from '#lib/components/ReceiptSettings.svelte';

  let {
    total,
    build,
    lineName,
    onclose,
    ondone,
    edit,
    member = null,
    credit = null
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
    /** Mode edit nota: simpan sebagai revisi nota `id` (alasan sudah diisi di layar kasir; penyetuju ikut di build()). */
    edit?: { id: string; reason: string };
    /** Member yang dipilih di kasir (kredit hanya untuk member) dan syarat kreditnya dari quote. */
    member?: { id: string; name: string; deposit?: string } | null;
    credit?: Quote['credit'] | null;
  } = $props();

  // Jenis transaksi mengikuti layar legacy: F1 Tunai, F2 Kredit, F3 Non-tunai, F4 Split.
  type Mode = 'cash' | 'credit' | 'noncash' | 'split';
  type Field = { amount: string; ref: string };
  const MODES: { id: Mode; key: string; label: string; enabled: boolean }[] = [
    { id: 'cash', key: 'F1', label: 'pos.mode.cash', enabled: true },
    { id: 'credit', key: 'F2', label: 'pos.mode.credit', enabled: true },
    { id: 'noncash', key: 'F3', label: 'pos.mode.noncash', enabled: true },
    { id: 'split', key: 'F4', label: 'pos.mode.split', enabled: true }
  ];
  /** Metode yang tampil untuk tiap jenis transaksi (dari master Metode Pembayaran). Hanya yang tampil yang ikut dikirim. */
  const showFor = (list: PaymentMethod[], mode: Mode, pickedId = '', splitIds: string[] = []): PaymentMethod[] => {
    switch (mode) {
      case 'cash':
        return list.filter((m) => m.kind === 'cash');
      case 'noncash':
        return list.filter((m) => m.id === pickedId); // satu metode dipilih kasir (QRIS, DANA, Debit BCA, ...)
      case 'split':
      case 'credit': // kredit: baris pembayaran di muka (DP) yang ditambahkan kasir; boleh kosong
        return splitIds.flatMap((id) => list.filter((m) => m.id === id)); // baris yang ditambahkan kasir, berurutan
      default:
        return [];
    }
  };
  const hasRef = (m: PaymentMethod) => m.kind !== 'cash' && m.kind !== 'deposit';

  /** sen → string desimal untuk input/API ("12500" atau "12500.50"), tanpa float. */
  const dec = (c: bigint) => (c % 100n === 0n ? String(c / 100n) : `${c / 100n}.${String(c % 100n).padStart(2, '0')}`);
  const money = (c: bigint) => formatCurrency(centsToNumber(c), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  /** Isian kosong untuk tiap metode; tunai langsung berisi uang pas bila diminta. */
  const blank = (list: PaymentMethod[], exactCash = false): Record<string, Field> =>
    Object.fromEntries(list.map((m) => [m.id, { amount: exactCash && m.kind === 'cash' ? dec(untrack(() => total)) : '', ref: '' }]));

  let methods = $state<PaymentMethod[]>([]);
  let methodsReady = $state(false);
  let mode = $state<Mode>('cash');
  let fields = $state<Record<string, Field>>({}); // default: uang pas (diisi setelah metode termuat)
  let submitting = $state(false);
  let error = $state('');
  let done = $state<Sale | null>(null);
  let body = $state<HTMLElement>();
  // Satu kunci selama jendela ini terbuka: klik ganda / ulang setelah jaringan putus mengembalikan nota yang sama.
  const key = newIdempotencyKey();

  let pickedId = $state('');
  const nonCashMethods = $derived(methods.filter((m) => m.kind !== 'cash'));
  const KIND_ORDER = ['cash', 'debit', 'credit_card', 'ewallet', 'transfer', 'deposit'];
  const kindName = (k: string) => t(`sales.method.${k}` as 'sales.method.cash');
  /** Jenis yang punya metode aktif, urut tetap (Tunai dulu). */
  const kindsOf = (list: PaymentMethod[]) => KIND_ORDER.filter((k) => list.some((m) => m.kind === k));
  const nonCashKinds = $derived(kindsOf(nonCashMethods));
  const pickedKind = $derived(methods.find((m) => m.id === pickedId)?.kind ?? '');
  /** Jenis -> metode pertama yang bebas (dipakai saat kasir mengganti jenis). */
  const firstOfKind = (k: string, taken: string[] = []) => methods.find((m) => m.kind === k && !taken.includes(m.id));
  /** Baris MDR di bawah isian: tarif, nominal biaya, dan siapa yang menanggung. Kosong bila metode tanpa biaya atau jumlah belum diisi. */
  const feeLine = (m: PaymentMethod) => {
    const amt = amountOf(m);
    if (amt <= 0n || !feeLabel(m)) return '';
    return t('pos.feeLine', { rate: feeLabel(m), fee: money(feeCents(m, amt)), who: t(m.fee_bearer === 'customer' ? 'catalog.paymentMethods.bearerCustomer' : 'catalog.paymentMethods.bearerStore') });
  };
  let splitIds = $state<string[]>([]);
  const shown = $derived(showFor(methods, mode, pickedId, splitIds));
  const splitFree = $derived(methods.filter((m) => !splitIds.includes(m.id)));
  const multi = $derived(mode === 'split' || mode === 'credit'); // beberapa baris metode
  const isCredit = $derived(mode === 'credit');
  /** Split: tambah satu baris metode; jumlah awalnya = sisa yang belum terbayar. */
  async function addSplit(id?: string) {
    const m = id ? methods.find((x) => x.id === id) : splitFree[0];
    if (!m || splitIds.includes(m.id)) return;
    const rest = total - paid;
    splitIds = [...splitIds, m.id];
    fields[m.id] = { amount: rest > 0n && mode !== 'credit' ? dec(rest) : '', ref: '' };
    await tick();
    const el = [...(body?.querySelectorAll<HTMLInputElement>('input[data-pay]') ?? [])].find((x) => x.getAttribute('aria-label') === m.name);
    el?.focus();
    el?.select();
  }
  /** Ganti jenis pada baris split: pindah ke metode bebas pertama dari jenis itu. */
  function changeKind(oldId: string, kind: string) {
    const next = firstOfKind(kind, splitIds);
    if (next) swapSplit(oldId, next.id);
  }
  function removeSplit(id: string) {
    splitIds = splitIds.filter((x) => x !== id);
    fields[id] = { amount: '', ref: '' };
  }
  /** Ganti metode pada baris split (jumlah dan referensi ikut pindah). */
  function swapSplit(oldId: string, newId: string) {
    if (oldId === newId || splitIds.includes(newId)) return;
    fields[newId] = { ...fields[oldId] };
    fields[oldId] = { amount: '', ref: '' };
    splitIds = splitIds.map((x) => (x === oldId ? newId : x));
  }
  /** "0,7%" / "Rp1.000" / "0,7% + Rp1.000" untuk label chip; kosong bila tanpa biaya. */
  const feeLabel = (m: PaymentMethod) => {
    const pct = Number(m.fee_pct ?? 0);
    const flat = toCents(m.fee_flat ?? '');
    return [pct > 0 ? `${pct}%` : '', flat > 0n ? money(flat) : ''].filter(Boolean).join(' + ');
  };
  /** Pilih metode non-tunai: isian lain dikosongkan, jumlah langsung pas dengan total. */
  async function pick(id: string) {
    pickedId = id;
    fields = blank(methods);
    if (fields[id]) fields[id].amount = dec(total);
    await tick();
    focusFirst();
  }
  const amountOf = (m: PaymentMethod) => toCents(fields[m.id]?.amount ?? '');
  const entered = $derived(shown.filter((m) => amountOf(m) > 0n));
  const paid = $derived(entered.reduce((s, m) => s + amountOf(m), 0n));
  const cashPaid = $derived(entered.filter((m) => m.kind === 'cash').reduce((s, m) => s + amountOf(m), 0n));
  const nonCashOver = $derived(paid - cashPaid > total);
  const diff = $derived(paid - total); // negatif = masih kurang
  /** Pratinjau biaya metode (rumus sama dengan server: jumlah × persen + tetap, dibulatkan sen). Server yang menentukan. */
  const feeCents = (m: PaymentMethod, amount: bigint): bigint => {
    const pctH = BigInt(Math.round(Number(m.fee_pct ?? 0) * 100));
    const flat = toCents(m.fee_flat ?? '');
    return pctH === 0n && flat === 0n ? 0n : (amount * pctH + 5000n) / 10000n + flat;
  };
  const customerFees = $derived(entered.filter((m) => m.fee_bearer === 'customer').map((m) => ({ m, amount: amountOf(m), fee: feeCents(m, amountOf(m)) })));
  const surcharge = $derived(customerFees.reduce((s, x) => s + x.fee, 0n));
  // Kredit: sisa yang belum dibayar menjadi piutang member. Pratinjau sen BigInt; server yang menentukan dan memeriksa limit.
  const receivable = $derived(isCredit && paid < total ? total - paid : 0n);
  const limitC = $derived(toCents(credit?.limit ?? '0'));
  const outstandingC = $derived(toCents(credit?.outstanding ?? '0'));
  let serverOver = $state(false); // server menolak karena limit walau pratinjau belum tahu
  const overLimit = $derived(isCredit && receivable > 0n && ((limitC > 0n && outstandingC + receivable > limitC) || serverOver));
  const baseApproval = $derived(isCredit ? build().approval : undefined); // sudah ada penyetuju (ubah harga / edit nota)
  let creditApprovers = $state<Approver[] | null>(null);
  let creditApproverId = $state('');
  let creditPin = $state('');
  $effect(() => {
    if (overLimit && !baseApproval && creditApprovers === null) {
      creditApprovers = [];
      void approvals
        .approvers('credit_limit')
        .then((l) => {
          creditApprovers = l;
          if (l.length === 1) creditApproverId = l[0].id;
        })
        .catch((e) => (error = errorMessage(e)));
    }
  });
  const approvalOk = $derived(!overLimit || !!baseApproval || (creditApproverId !== '' && /^d{6}$/.test(creditPin)));
  /** Saldo deposit member (dari quote); pemakaian deposit tidak boleh melebihinya (server tetap memeriksa saat simpan). */
  const depositC = $derived(toCents(member?.deposit ?? '0'));
  const depositUsed = $derived(entered.filter((m) => m.kind === 'deposit').reduce((s, m) => s + amountOf(m), 0n));
  const depositOver = $derived(depositUsed > depositC);
  const canSubmit = $derived(
    methodsReady && !submitting && !nonCashOver && !depositOver && (isCredit ? !!member && receivable > 0n && approvalOk : entered.length > 0 && paid >= total)
  );

  function focusFirst() {
    const el = body?.querySelector<HTMLInputElement>('input[data-pay]');
    el?.focus();
    el?.select();
  }

  async function setMode(next: Mode) {
    if (!MODES.find((m) => m.id === next)?.enabled || next === mode) return;
    mode = next;
    fields = blank(methods, next === 'cash');
    error = '';
    splitIds = [];
    serverOver = false;
    if (next === 'split') {
      await addSplit(methods.find((m) => m.kind === 'cash')?.id);
      return;
    }
    if (next === 'credit') return; // tanpa DP dulu; kasir menambah pembayaran di muka bila ada
    if (next === 'noncash') {
      await pick(methods.some((m) => m.id === pickedId && m.kind !== 'cash') ? pickedId : (nonCashMethods[0]?.id ?? ''));
      return;
    }
    await tick();
    focusFirst();
  }

  onMount(() => {
    void paymentMethodsLookup
      .all('sale')
      .then(async (list) => {
        // Deposit member hanya bisa dipakai bila nota punya member.
        methods = list.filter((m) => m.kind !== 'deposit' || !!member);
        pickedId = list.find((m) => m.kind !== 'cash')?.id ?? '';
        fields = blank(list, true);
        methodsReady = true;
        await tick();
        focusFirst();
      })
      .catch((e) => {
        error = errorMessage(e);
      });
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
    if (err instanceof ApiError && err.code === 'CREDIT_LIMIT_EXCEEDED') serverOver = true; // minta persetujuan penyetuju
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
      const base = build();
      const payload = {
          ...base,
          ...(isCredit ? { credit: true } : {}),
          ...(overLimit && !baseApproval ? { approval: { user_id: creditApproverId, pin: creditPin } } : {}),
          payments: entered.map((m) => {
            const f = fields[m.id];
            const ref = f.ref.trim().slice(0, 100);
            return { method_id: m.id, amount: dec(toCents(f.amount)), ...(ref ? { ref_no: ref } : {}) };
          })
        };
      done = edit ? await sales.edit(edit.id, { ...payload, reason: edit.reason }, key) : await sales.create(payload, key);
    } catch (e) {
      error = describe(e);
    } finally {
      submitting = false;
    }
    if (done && receipt.auto) void print();
  }

  // Struk: cetak pertama setelah nota tersimpan; cetakan berikutnya dari layar ini = cetak ulang (tercatat di audit).
  let receipt = $state(loadReceiptSettings());
  let printed = $state(false);
  let printing = $state(false);
  let printError = $state('');

  async function print() {
    if (!done || printing) return;
    printing = true;
    printError = '';
    try {
      await printReceipt(done.id, { reprint: printed });
      printed = true;
    } catch (e) {
      printError = printErrorMessage(e);
    } finally {
      printing = false;
    }
  }


</script>

<svelte:window onkeydown={onWindowKeydown} />

<Modal title={done ? t('pos.paidTitle') : edit ? t('pos.edit.payTitle') : t('pos.payTitle')} onclose={done ? ondone : onclose} wide={!done}>
  {#if done}
    <div class="text-center space-y-3 py-2">
      <span class="grid place-items-center size-14 mx-auto rounded-full bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-600)]"><i class="icon-check text-[28px]"></i></span>
      <div class="text-[12px] text-[var(--text-tertiary)]">{edit ? t('pos.edit.newDoc') : t('pos.paidDoc')}</div>
      <div class="font-mono text-[20px] font-bold">{done.doc_no}</div>
      <dl class="text-[13px] space-y-1 text-start max-w-xs mx-auto">
        <div class="flex justify-between"><dt>{t('pos.payTotal')}</dt><dd class="font-semibold tabular-nums">{money(toCents(done.total))}</dd></div>
        <div class="flex justify-between"><dt>{t('pos.payPaid')}</dt><dd class="tabular-nums">{money(toCents(done.paid))}</dd></div>
        {#if Number(done.surcharge) > 0}
          <div class="flex justify-between"><dt>{t('pos.surcharge')}</dt><dd class="tabular-nums">+{money(toCents(done.surcharge))}</dd></div>
          <div class="flex justify-between font-semibold"><dt>{t('pos.charged')}</dt><dd class="tabular-nums">{money(toCents(done.total) + toCents(done.surcharge))}</dd></div>
        {/if}
        {#if Number(done.receivable) > 0}
          <div class="flex justify-between text-[16px] font-extrabold text-[var(--color-danger-600)]"><dt>{t('pos.credit.receivable')}</dt><dd class="tabular-nums">{money(toCents(done.receivable))}</dd></div>
          {#if done.credit?.due_date}
            <div class="flex justify-between text-[12.5px] text-[var(--text-secondary)]"><dt>{t('pos.credit.dueOn')}</dt><dd>{formatDate(done.credit.due_date)}</dd></div>
          {/if}
        {:else}
          <div class="flex justify-between text-[16px] font-extrabold text-[var(--color-success-600)]"><dt>{t('pos.paidChange')}</dt><dd class="tabular-nums">{money(toCents(done.change))}</dd></div>
        {/if}
      </dl>
      {#if done.member}
        <p class="text-[12.5px] text-[var(--text-secondary)]">
          <i class="icon-user-round me-1"></i>{done.member.name}
          {#if done.points_earned > 0}<span class="ms-1.5 font-semibold text-[var(--color-success-600)]">{t('members.pos.earned', { points: done.points_earned })}</span>{/if}
          {#if done.points_redeemed > 0}<span class="ms-1.5 font-semibold text-[var(--color-warning-600)]">{t('members.pos.redeemed', { points: done.points_redeemed })}</span>{/if}
        </p>
      {/if}
      {#if printError}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{printError}</p>{/if}
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="flex gap-2"
        role="group"
        onkeydown={(e) => {
          if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
          const btns = [...e.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')];
          const i = btns.indexOf(document.activeElement as HTMLButtonElement);
          const next = btns[i + (e.key === 'ArrowRight' ? 1 : -1)];
          if (next) (e.preventDefault(), next.focus());
        }}
      >
        <!-- Fokus awal di "Transaksi baru" (Enter = lanjut); cetak ada di kanannya (→ lalu Enter). -->
        <button type="button" class="btn btn-primary flex-1" onclick={ondone} use:focusOnMount>{t('pos.newSale')}</button>
        <button type="button" class="btn btn-outline flex-1 disabled:opacity-60" onclick={print} disabled={printing}>
          <i class="icon-printer text-[14px]"></i>{printing ? t('pos.receipt.printing') : printed ? t('pos.receipt.reprint') : t('pos.receipt.print')}
        </button>
      </div>
      <details class="text-start text-[12px] text-[var(--text-secondary)]">
        <summary class="cursor-pointer select-none">{t('pos.receipt.settings')}</summary>
        <div class="mt-2"><ReceiptSettings onchange={(v) => (receipt = v)} /></div>
      </details>
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

      {#if !methodsReady && !error}
        <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('pos.methodsLoading')}</p>
      {/if}
      {#if mode === 'noncash' && methodsReady}
        {#if nonCashMethods.length === 0}
          <p class="text-[12.5px] text-[var(--color-danger-600)]">{t('pos.noNonCash')}</p>
        {:else}
          <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-start">
            <div class="text-[16px] font-semibold pt-1">{t('pos.pickKind')}:</div>
            <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('pos.pickKind')}>
              {#each nonCashKinds as k (k)}
                <label class="cursor-pointer select-none rounded border border-[var(--border-default)] px-3 py-1.5 text-[13px] font-semibold has-[:checked]:border-[var(--color-primary-500)] has-[:checked]:bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)]">
                  <input type="radio" name="paykind" class="sr-only" value={k} checked={pickedKind === k} onchange={() => { const n = firstOfKind(k); if (n) void pick(n.id); }} />
                  {kindName(k)}
                </label>
              {/each}
            </div>
            <div class="text-[16px] font-semibold pt-1">{t('pos.pickMethod')}:</div>
            <div class="flex flex-wrap gap-1.5" role="radiogroup" aria-label={t('pos.pickMethod')}>
              {#each nonCashMethods.filter((x) => x.kind === pickedKind) as m (m.id)}
                <label class="cursor-pointer select-none rounded border border-[var(--border-default)] px-3 py-1.5 text-[13px] font-semibold has-[:checked]:border-[var(--color-primary-500)] has-[:checked]:bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)]">
                  <input type="radio" name="paymethod" class="sr-only" value={m.id} checked={pickedId === m.id} onchange={() => pick(m.id)} />
                  {m.name}
                  {#if feeLabel(m)}<span class="ms-1 text-[11px] font-normal text-[var(--text-tertiary)]">{feeLabel(m)}{m.fee_bearer === 'customer' ? ' ↗' : ''}</span>{/if}
                </label>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
      {#each shown as m (m.id)}
      <div class={multi ? 'rounded border border-[var(--border-subtle)] p-2.5 space-y-2' : ''}>
        {#if multi}
          <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
            <div class="text-[16px] font-semibold">{t('pos.pickKind')}:</div>
            <div class="flex items-center gap-1">
              <div class="min-w-0 grow">
                <Select
                  ariaLabel={t('pos.pickKind')}
                  value={m.kind}
                  onchange={(k) => changeKind(m.id, k)}
                  options={KIND_ORDER.filter((k) => k === m.kind || methods.some((x) => x.kind === k && !splitIds.includes(x.id))).map((k) => ({ value: k, label: kindName(k) }))}
                />
              </div>
              {#if splitIds.length > 1 || isCredit}
                <button type="button" class="header-icon-btn shrink-0" aria-label={t('pos.splitRemove')} title={t('pos.splitRemove')} onclick={() => removeSplit(m.id)}><i class="icon-x text-[15px]"></i></button>
              {/if}
            </div>
          </div>
          <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
            <div class="text-[16px] font-semibold">{t('pos.pickMethod')}:</div>
            <Select
              ariaLabel={t('pos.pickMethod')}
              value={m.id}
              onchange={(v) => swapSplit(m.id, v)}
              options={methods.filter((x) => x.kind === m.kind && (x.id === m.id || !splitIds.includes(x.id))).map((x) => ({ value: x.id, label: x.name }))}
            />
          </div>
        {/if}
        <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-start">
          <div class="text-[16px] font-semibold pt-1 break-words">{isCredit ? t('pos.credit.dpAmount') : mode === 'split' ? t('pos.splitAmount') : m.name}:</div>
          <div class="space-y-1.5">
            <MoneyInput
              data-pay
              bind:value={fields[m.id].amount}
              placeholder="0"
              aria-label={m.name}
              class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
            />
            {#if feeLine(m)}<p class="text-[12px] text-[var(--color-warning-600)]">{feeLine(m)}</p>{/if}
            {#if m.kind === 'deposit'}
              <p class="text-[12px] {depositOver ? 'text-[var(--color-danger-600)] font-semibold' : 'text-[var(--text-tertiary)]'}">{t('pos.depositBalance', { amount: money(depositC) })}{#if depositOver} · {t('pos.depositOver')}{/if}</p>
            {/if}
            {#if hasRef(m)}
              <input data-pay bind:value={fields[m.id].ref} maxlength="100" autocomplete="off" placeholder={t(('pos.payRefFor.' + (m.kind === 'cash' ? 'transfer' : m.kind)) as 'pos.payRefFor.transfer')} aria-label={t('pos.payRef')} class="w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
            {/if}
          </div>
        </div>
      </div>
      {/each}

      {#if multi && splitFree.length > 0}
        <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2">
          <span></span>
          <button type="button" class="btn btn-sm justify-self-start" onclick={() => addSplit()}><i class="icon-plus me-1"></i>{isCredit ? t('pos.credit.dpAdd') : t('pos.splitAdd')}</button>
        </div>
      {/if}

      {#if isCredit}
        <div class="rounded border border-[var(--border-default)] p-2.5 space-y-1.5 text-[13px]">
          {#if !member}
            <p role="alert" class="text-[var(--color-danger-600)] font-semibold"><i class="icon-user-round me-1"></i>{t('pos.credit.noMember')}</p>
          {:else}
            <div class="flex justify-between gap-3"><span>{t('pos.credit.member')}</span><span class="font-semibold text-end">{member.name}</span></div>
            <div class="flex justify-between gap-3">
              <span>{t('pos.credit.due')}</span>
              <span class="font-semibold text-end">{credit && credit.due_days > 0 ? t('pos.credit.dueDays', { days: credit.due_days }) : t('pos.credit.noDue')}</span>
            </div>
            {#if credit}
              <div class="flex justify-between gap-3">
                <span>{t('pos.credit.limit')}</span>
                <span class="font-semibold text-end">{limitC > 0n ? money(limitC) : t('pos.credit.noLimit')}</span>
              </div>
              <div class="flex justify-between gap-3"><span>{t('pos.credit.outstanding')}</span><span class="tabular-nums text-end">{money(outstandingC)}</span></div>
              <div class="flex justify-between gap-3"><span>{t('pos.credit.after')}</span><span class="tabular-nums font-semibold text-end">{money(outstandingC + receivable)}</span></div>
            {/if}
          {/if}
          {#if member && receivable <= 0n && paid > 0n}
            <p class="text-[12px] text-[var(--color-warning-600)]">{fieldMessage('CREDIT_NOT_NEEDED')}</p>
          {/if}
        </div>
        {#if overLimit}
          <div class="rounded border border-[var(--color-warning-500)]/50 bg-[color-mix(in_oklab,var(--color-warning-500)_10%,transparent)] p-2.5 space-y-2 text-[13px]">
            <p class="font-semibold text-[var(--color-warning-600)]"><i class="icon-shield-alert me-1"></i>{t('pos.credit.overLimit')}</p>
            {#if baseApproval}
              <p class="text-[12px] text-[var(--text-secondary)]">{t('pos.credit.sameApprover')}</p>
            {:else}
              <label class="block">
                <span class="font-semibold">{t('pos.override.approver')}</span>
                <div class="mt-1"><Select bind:value={creditApproverId} options={[{ value: '', label: t('pos.override.pickApprover'), disabled: true }, ...(creditApprovers ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
                {#if creditApprovers && creditApprovers.length === 0}<span class="text-[12px] text-[var(--color-danger-600)]">{t('pos.credit.noApprovers')}</span>{/if}
              </label>
              <label class="block">
                <span class="font-semibold">{t('pos.override.pin')}</span>
                <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={creditPin} autocomplete="new-password" class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
              </label>
            {/if}
          </div>
        {/if}
      {/if}

      {#if surcharge > 0n}
        <div class="rounded border border-[var(--color-warning-500)]/40 bg-[color-mix(in_oklab,var(--color-warning-500)_10%,transparent)] p-2.5 space-y-1 text-[13px]">
          {#each customerFees as x (x.m.id)}
            <div class="flex justify-between gap-3"><span>{x.m.name}: {t('pos.feeCustomerHint', { amount: money(x.amount + x.fee), fee: money(x.fee) })}</span></div>
          {/each}
          <div class="flex justify-between gap-3 font-semibold"><span>{t('pos.surcharge')}</span><span class="tabular-nums">+{money(surcharge)}</span></div>
          <div class="flex justify-between gap-3 text-[15px] font-bold text-[var(--color-danger-600)]"><span>{t('pos.charged')}</span><span class="tabular-nums">{money(total + surcharge)}</span></div>
        </div>
      {/if}

      <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
        <div class="text-[16px] font-semibold text-[var(--color-primary-600)]">{t('pos.payTotalPaid')}</div>
        <input readonly tabindex="-1" value={money(paid)} aria-label={t('pos.payTotalPaid')} class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[color-mix(in_oklab,var(--color-danger-500)_16%,transparent)]" />
      </div>
      {#if isCredit}
        <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
          <div class="text-[16px] font-semibold text-[var(--color-danger-600)]">{t('pos.credit.receivable')}:</div>
          <input readonly tabindex="-1" value={money(receivable)} aria-label={t('pos.credit.receivable')} class="w-full h-11 px-3 text-end text-[22px] font-bold tabular-nums rounded border border-[var(--border-default)] bg-[color-mix(in_oklab,var(--color-danger-500)_16%,transparent)]" />
        </div>
      {:else}
        <div class="grid sm:grid-cols-[170px_minmax(0,1fr)] gap-2 items-center">
          <div class="text-[16px] font-semibold text-[var(--color-primary-600)]">{diff < 0n ? t('pos.payShort') : t('pos.payChange')}</div>
          <input readonly tabindex="-1" value={money(diff)} aria-label={t('pos.payChange')} class="w-full h-11 px-3 text-end text-[22px] tabular-nums rounded border border-[var(--border-default)] bg-[color-mix(in_oklab,var(--color-success-500)_22%,transparent)]" />
        </div>
      {/if}

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
