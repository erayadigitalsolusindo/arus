<script lang="ts">
  // Pelunasan kolektif: bayar hutang per pemasok / terima piutang per member. Dua cara: "otomatis" (server melunasi nota
  // terlama dulu dari total uang) atau "pilih nota" (centang nota + jumlah per nota). Pratinjau pembagian selalu dari server.
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox from '#lib/components/Combobox.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { payables } from '#lib/payables/api.ts';
  import { receivables } from '#lib/receivables/api.ts';
  import { lookup, paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { members } from '#lib/members/api.ts';
  import { memberDeposits, supplierCredits } from '#lib/wallet/api.ts';
  import { settle, type SettleKind, type SettleMode, type SettleInput, type SettlePlan, type SettleResult } from '#lib/settle/api.ts';
  import { newIdempotencyKey } from '#lib/sales/api.ts';
  import { centsToNumber, toCents } from '#lib/pos/money.ts';
  import { t, formatCurrency, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let {
    kind,
    onclose,
    onchanged
  }: {
    kind: SettleKind;
    onclose: () => void;
    /** Dipanggil setelah pelunasan tersimpan agar daftar memuat ulang. */
    onchanged: () => void;
  } = $props();

  type Note = { id: string; doc: string; sub: string; date: string; due?: string; balance: string };

  const money = (v: string | bigint) => formatCurrency(typeof v === 'bigint' ? centsToNumber(v) : Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  /** sen → string desimal untuk API, tanpa float. */
  const dec = (c: bigint) => (c % 100n === 0n ? String(c / 100n) : `${c / 100n}.${String(c % 100n).padStart(2, '0')}`);
  const inputClass = 'w-full h-9 px-2 text-[13px] rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]';

  let partyId = $state('');
  let partyLabel = $state('');
  let notes = $state<Note[]>([]);
  let loadingNotes = $state(false);
  let methods = $state<PaymentMethod[]>([]);
  let methodId = $state('');
  let mode = $state<SettleMode>('auto');
  let amount = $state('');
  let picks = $state<Record<string, string>>({}); // id nota → jumlah (string desimal); tidak ada = tidak dicentang
  let ref = $state('');
  let note = $state('');
  let plan = $state<SettlePlan | null>(null);
  let planError = $state('');
  let error = $state('');
  let busy = $state(false);
  let result = $state<SettleResult | null>(null);

  const searchParty = (q: string) =>
    kind === 'payable' ? lookup('suppliers').search(q) : members.lookup(q).then((r) => r.map((m) => ({ id: m.id, name: m.name })));

  // Metode bayar dimuat sekali.
  $effect(() => {
    paymentMethodsLookup
      .all(kind === 'payable' ? 'payable' : 'receivable')
      .then((l) => {
        methods = l;
        methodId = methodId || (l[0]?.id ?? '');
      })
      .catch((e) => (error = errorMessage(e)));
  });

  // Nota terbuka milik pihak terpilih, urut dari yang terlama.
  $effect(() => {
    const id = partyId;
    notes = [];
    picks = {};
    plan = null;
    amount = '';
    if (!id) return;
    loadingNotes = true;
    error = '';
    const load =
      kind === 'payable'
        ? payables.list({ supplier_id: id, status: 'open', limit: 200 }).then((r) =>
            r.data.map((p): Note => ({ id: p.id, doc: p.doc_no, sub: p.supplier_invoice_no, date: p.purchase_date, due: p.due_date, balance: p.balance }))
          )
        : receivables.list({ member_id: id, status: 'open', limit: 200 }).then((r) =>
            r.data.map((p): Note => ({ id: p.id, doc: p.doc_no, sub: '', date: p.created_at.slice(0, 10), due: p.due_date, balance: p.balance }))
          );
    load
      .then((l) => {
        if (id === partyId) notes = l.sort((a, b) => (a.date === b.date ? a.doc.localeCompare(b.doc) : a.date.localeCompare(b.date)));
      })
      .catch((e) => (error = errorMessage(e)))
      .finally(() => (loadingNotes = false));
  });

  const outstanding = $derived(notes.reduce((s, n) => s + toCents(n.balance), 0n));
  const amountC = $derived(toCents(amount));
  const pickedIds = $derived(notes.filter((n) => n.id in picks).map((n) => n.id));
  const manualTotal = $derived(pickedIds.reduce((s, id) => s + toCents(picks[id] ?? ''), 0n));
  const total = $derived(mode === 'auto' ? amountC : manualTotal);
  const manualValid = $derived(pickedIds.length > 0 && pickedIds.every((id) => { const c = toCents(picks[id] ?? ''); const n = notes.find((x) => x.id === id); return c > 0n && !!n && c <= toCents(n.balance); }));
  const picked = $derived(methods.find((m) => m.id === methodId));
  /** Saldo deposit member / kredit pemasok pihak terpilih (bila metode saldo titipan dipilih). Server tetap memeriksa. */
  let walletBal = $state<string | null>(null);
  $effect(() => {
    const id = partyId;
    const k = picked?.kind;
    if (!id || (k !== 'deposit' && k !== 'supplier_credit')) {
      walletBal = null;
      return;
    }
    void (k === 'deposit' ? memberDeposits.account(id) : supplierCredits.account(id)).then((acc) => (walletBal = acc.balance)).catch((e) => (error = errorMessage(e)));
  });

  const payload = $derived<SettleInput>({
    party_id: partyId,
    mode,
    method_id: methodId || undefined,
    amount: mode === 'auto' ? dec(amountC) : undefined,
    allocations: mode === 'manual' ? pickedIds.map((id) => ({ id, amount: dec(toCents(picks[id] ?? '')) })) : undefined,
    ref_no: picked && picked.kind !== 'cash' && picked.kind !== 'deposit' && picked.kind !== 'supplier_credit' ? ref.trim().slice(0, 100) : undefined,
    note: note.trim().slice(0, 200) || undefined
  });
  const ready = $derived(!!partyId && !!methodId && total > 0n && (mode === 'auto' ? amountC <= outstanding : manualValid));

  // Satu kunci per isi permintaan: ulang setelah jaringan putus aman, ubah isian = kunci baru.
  let key = $state(newIdempotencyKey());
  let lastBody = '';
  $effect(() => {
    const b = JSON.stringify(payload);
    if (b !== lastBody) {
      lastBody = b;
      key = newIdempotencyKey();
    }
  });

  // Pratinjau pembagian dari server (debounce).
  let qseq = 0;
  $effect(() => {
    const p = payload;
    const valid = !!partyId && (mode === 'auto' ? amountC > 0n && amountC <= outstanding : manualValid);
    planError = '';
    if (!valid) {
      plan = null;
      return;
    }
    const mine = ++qseq;
    const h = setTimeout(() => {
      settle
        .quote(kind, p)
        .then((r) => mine === qseq && (plan = r))
        .catch((e) => {
          if (mine !== qseq) return;
          plan = null;
          planError = describe(e);
        });
    }, 250);
    return () => clearTimeout(h);
  });

  function describe(e: unknown) {
    return e instanceof ApiError && e.code === 'VALIDATION' ? Object.values(e.fields).map((c) => fieldMessage(c)).join(' ') : errorMessage(e);
  }

  function toggle(n: Note, on: boolean) {
    if (on) picks = { ...picks, [n.id]: n.balance };
    else {
      const { [n.id]: _drop, ...rest } = picks;
      picks = rest;
    }
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!ready || busy) return;
    busy = true;
    error = '';
    try {
      result = await settle.create(kind, payload, key);
      onchanged();
    } catch (err) {
      error = describe(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t(`settle.${kind}.title`)} {onclose} wide>
  {#if result}
    <div class="space-y-3">
      <p role="status" class="text-[14px] font-semibold text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{t('settle.done', { doc: result.doc_no })}</p>
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('settle.doneDetail', { count: result.allocations.length, amount: money(result.total) })}</p>
      <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
        <table class="w-full text-[12.5px]">
          <tbody>
            {#each result.allocations as a (a.id)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0">
                <td class="px-3 py-2 font-mono text-[12px] font-semibold">{a.doc}</td>
                <td class="px-3 py-2 text-[var(--text-tertiary)]">{formatDate(a.date)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(a.amount)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="flex justify-end"><button type="button" class="btn btn-primary" onclick={onclose}>{t('settle.close')}</button></div>
    </div>
  {:else}
    <form class="space-y-4" onsubmit={submit} autocomplete="off">
      <label class="block text-[13px]">
        <span class="font-semibold">{t(`settle.${kind}.party`)}</span>
        <div class="mt-1"><Combobox bind:value={partyId} bind:label={partyLabel} search={searchParty} placeholder={t(`settle.${kind}.partyHint`)} /></div>
      </label>

      {#if partyId}
        {#if loadingNotes}
          <p class="text-[12.5px] text-[var(--text-tertiary)]">…</p>
        {:else if notes.length === 0}
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{t(`settle.${kind}.empty`)}</p>
        {:else}
          <p class="text-[12.5px] text-[var(--text-secondary)]">{t('settle.outstanding', { amount: money(outstanding), count: notes.length })}</p>

          <div class="inline-flex rounded border border-[var(--border-default)] p-0.5 text-[12.5px]" role="tablist">
            {#each ['auto', 'manual'] as const as m (m)}
              <button type="button" role="tab" aria-selected={mode === m} class="rounded px-3 py-1.5 font-semibold {mode === m ? 'bg-[var(--color-primary-500)] text-white' : 'text-[var(--text-secondary)]'}" onclick={() => (mode = m)}>{t(`settle.mode.${m}`)}</button>
            {/each}
          </div>
          <p class="text-[12px] text-[var(--text-tertiary)]">{mode === 'auto' ? t('settle.autoHelp') : t('settle.manualHelp')}</p>

          {#if mode === 'auto'}
            <label class="block text-[13px]">
              <span class="font-semibold">{t('settle.amount')}</span>
              <div class="mt-1 flex gap-1.5">
                <MoneyInput bind:value={amount} placeholder="0" aria-label={t('settle.amount')} class="w-full h-10 px-3 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
                <button type="button" class="btn btn-sm shrink-0" onclick={() => (amount = dec(outstanding))}>{t('settle.payAll')}</button>
              </div>
            </label>
            {#if amountC > outstanding}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage('OVERPAID')}</p>{/if}
          {:else}
            <div class="flex gap-2 text-[12px]">
              <button type="button" class="btn btn-sm" onclick={() => (picks = Object.fromEntries(notes.map((n) => [n.id, n.balance])))}>{t('settle.selectAll')}</button>
              <button type="button" class="btn btn-sm" onclick={() => (picks = {})}>{t('settle.clear')}</button>
            </div>
            <div class="max-h-72 overflow-auto rounded border border-[var(--border-subtle)]">
              <table class="w-full text-[12.5px]">
                <thead>
                  <tr class="sticky top-0 bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                    <th class="px-3 py-2 text-start w-10">{t('settle.col.pick')}</th>
                    <th class="px-3 py-2 text-start">{t(`settle.${kind}.doc`)}</th>
                    <th class="px-3 py-2 text-start">{t('settle.col.date')}</th>
                    <th class="px-3 py-2 text-start">{t('settle.col.due')}</th>
                    <th class="px-3 py-2 text-end">{t('settle.col.balance')}</th>
                    <th class="px-3 py-2 text-end w-44">{t('settle.col.pay')}</th>
                  </tr>
                </thead>
                <tbody>
                  {#each notes as n (n.id)}
                    {@const on = n.id in picks}
                    {@const over = on && toCents(picks[n.id] ?? '') > toCents(n.balance)}
                    <tr class="border-b border-[var(--border-subtle)] last:border-0 {on ? 'bg-[var(--color-primary-500)]/5' : ''}">
                      <td class="px-3 py-1.5"><input type="checkbox" checked={on} onchange={(e) => toggle(n, e.currentTarget.checked)} aria-label={n.doc} /></td>
                      <td class="px-3 py-1.5"><div class="font-mono text-[12px] font-semibold">{n.doc}</div>{#if n.sub}<div class="font-mono text-[11px] text-[var(--text-tertiary)]">{n.sub}</div>{/if}</td>
                      <td class="px-3 py-1.5 whitespace-nowrap">{formatDate(n.date)}</td>
                      <td class="px-3 py-1.5 whitespace-nowrap">{n.due ? formatDate(n.due) : '—'}</td>
                      <td class="px-3 py-1.5 text-end tabular-nums whitespace-nowrap">{money(n.balance)}</td>
                      <td class="px-3 py-1.5">
                        {#if on}
                          <MoneyInput bind:value={picks[n.id]} aria-label={t('settle.col.pay')} class="w-full h-8 px-2 text-end tabular-nums rounded border {over ? 'border-[var(--color-danger-600)]' : 'border-[var(--border-default)]'} bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <section>
            <h3 class="mb-1.5 text-[12px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('settle.preview')}</h3>
            {#if plan && plan.allocations.length > 0}
              <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
                <table class="w-full text-[12.5px]">
                  <tbody>
                    {#each plan.allocations as a (a.id)}
                      <tr class="border-b border-[var(--border-subtle)] last:border-0">
                        <td class="px-3 py-1.5 font-mono text-[12px] font-semibold">{a.doc}</td>
                        <td class="px-3 py-1.5 text-[var(--text-tertiary)]">{formatDate(a.date)}</td>
                        <td class="px-3 py-1.5 text-end tabular-nums font-semibold">{money(a.amount)}</td>
                        <td class="px-3 py-1.5 text-end text-[11.5px] text-[var(--text-tertiary)] whitespace-nowrap">{t('settle.col.after')}: {money(a.balance_after ?? '0')}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {:else if planError}
              <p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{planError}</p>
            {:else}
              <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('settle.previewEmpty')}</p>
            {/if}
          </section>

          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-[13px]">
              <span class="font-semibold">{t('settle.method')}</span>
              <div class="mt-1"><Select bind:value={methodId} ariaLabel={t('settle.method')} options={methods.map((m) => ({ value: m.id, label: m.name }))} /></div>
            </label>
            {#if walletBal !== null}
              <p class="self-end text-[12px] text-[var(--text-secondary)]">{t(kind === 'payable' ? 'supplierCredits.balanceLine' : 'pos.depositBalance', { amount: money(walletBal) })}</p>
            {/if}
            {#if picked && picked.kind !== 'cash' && picked.kind !== 'deposit' && picked.kind !== 'supplier_credit'}
              <label class="block text-[13px]">
                <span class="font-semibold">{t('settle.ref')}</span>
                <input bind:value={ref} maxlength="100" class="{inputClass} mt-1" />
              </label>
            {/if}
          </div>
          <input bind:value={note} maxlength="200" placeholder={t('settle.note')} aria-label={t('settle.note')} class={inputClass} />

          {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
          <div class="flex items-center justify-between gap-2">
            <span class="text-[14px] font-extrabold tabular-nums">{t('settle.total', { amount: money(total) })}</span>
            <div class="flex gap-2">
              <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('settle.close')}</button>
              <button type="submit" class="btn btn-primary" disabled={!ready || busy}>{busy ? t('settle.saving') : t('settle.save')}</button>
            </div>
          </div>
        {/if}
      {/if}
      {#if error && !partyId}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    </form>
  {/if}
</Modal>
