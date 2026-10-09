<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Select from '#lib/components/Select.svelte';
  import {
    purchaseReturns as api,
    type ReturnablePurchase,
    type ReturnSource,
    type ReturnQuote,
    type ReturnInput,
    type PurchaseReturn
  } from '#lib/purchases/returns.ts';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { toMilli } from '#lib/pos/money.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const q3 = (s: string) => formatNumber(Number(s), { maximumFractionDigits: 3 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const label = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  /** qty ketikan → string desimal titik untuk API ('' bila kosong/tidak valid). */
  const cleanQty = (s: string) => {
    const v = s.trim().replace(',', '.');
    return /^\d*\.?\d{0,3}$/.test(v) && toMilli(v) > 0n ? v : '';
  };

  // ---- Pilih nota ----
  let search = $state('');
  let choices = $state<ReturnablePurchase[]>([]);
  let searching = $state(false);
  let src = $state<ReturnSource | null>(null);
  let loadError = $state('');

  let pickSeq = 0;
  $effect(() => {
    if (src) return;
    const q = search.trim();
    void session.outlet?.id;
    const mine = ++pickSeq;
    searching = true;
    const h = setTimeout(async () => {
      try {
        const res = await api.purchases(q);
        if (mine === pickSeq) choices = res;
      } catch (e) {
        if (mine === pickSeq) loadError = errorMessage(e);
      } finally {
        if (mine === pickSeq) searching = false;
      }
    }, 250);
    return () => clearTimeout(h);
  });

  async function pick(id: string) {
    loadError = '';
    try {
      src = await api.source(id);
      qtys = {};
      note = '';
      quote = null;
      lineErrors = {};
      formError = '';
      saved = null;
    } catch (e) {
      loadError = errorMessage(e);
    }
  }

  function reset() {
    src = null;
    qtys = {};
    quote = null;
    saved = null;
    history.replaceState(history.state, '', '/purchase-returns/new');
  }

  // ---- Isian ----
  let qtys = $state<Record<number, string>>({});
  let note = $state('');
  let methods = $state<PaymentMethod[]>([]);
  let methodId = $state('');
  let refundRef = $state('');
  let quote = $state<ReturnQuote | null>(null);
  let quoting = $state(false);
  let lineErrors = $state<Record<number, string>>({});
  let formError = $state('');
  let busy = $state(false);
  let saved = $state<PurchaseReturn | null>(null);

  const methodOptions = $derived(methods.map((m) => ({ value: m.id, label: m.name })));
  const input = $derived.by((): ReturnInput | null => {
    if (!src) return null;
    const lines = src.lines.map((l) => ({ position: l.position, qty: cleanQty(qtys[l.position] ?? '') })).filter((l) => l.qty !== '');
    return { purchase_id: src.purchase_id, lines };
  });
  /** Galat sisi klien: qty melebihi sisa yang boleh diretur atau format salah (server tetap memeriksa). */
  const localErrors = $derived.by(() => {
    const out: Record<number, string> = {};
    for (const l of src?.lines ?? []) {
      const raw = (qtys[l.position] ?? '').trim();
      if (raw === '') continue;
      const c = cleanQty(raw);
      if (c === '') out[l.position] = fieldMessage('INVALID') ?? '';
      else if (toMilli(c) > toMilli(l.returnable)) out[l.position] = fieldMessage('TOO_HIGH') ?? '';
    }
    return out;
  });
  const issues = $derived(new Map((quote?.lines ?? []).filter((l) => l.issue).map((l) => [l.position, l.return_stock])));
  const values = $derived(new Map((quote?.lines ?? []).map((l) => [l.position, l.value])));
  const needsMethod = $derived(!!quote && Number(quote.refund) > 0);
  const canSave = $derived(
    !!input && input.lines.length > 0 && Object.keys(localErrors).length === 0 && issues.size === 0 && !!quote && !quoting && (!needsMethod || methodId !== '')
  );

  let quoteSeq = 0;
  let quoteCtl: AbortController | undefined;
  $effect(() => {
    const body = input;
    if (!body || body.lines.length === 0 || Object.keys(localErrors).length > 0) {
      quote = null;
      return;
    }
    const sig = JSON.stringify(body);
    const mine = ++quoteSeq;
    quoting = true;
    const h = setTimeout(async () => {
      quoteCtl?.abort();
      quoteCtl = new AbortController();
      try {
        const q = await api.quote(JSON.parse(sig), quoteCtl.signal);
        if (mine !== quoteSeq) return;
        quote = q;
        lineErrors = {};
        formError = '';
      } catch (err) {
        if (mine !== quoteSeq || (err instanceof DOMException && err.name === 'AbortError')) return;
        quote = null;
        applyError(err, body);
      } finally {
        if (mine === quoteSeq) quoting = false;
      }
    }, 300);
    return () => clearTimeout(h);
  });

  /** Galat per baris dari server (lines.N.qty) dipetakan kembali ke posisi baris nota. */
  function applyError(err: unknown, body: ReturnInput) {
    if (err instanceof ApiError && err.code === 'VALIDATION') {
      const le: Record<number, string> = {};
      const rest: string[] = [];
      for (const [k, v] of Object.entries(err.fields)) {
        const m = /^lines\.(\d+)\./.exec(k);
        const line = m ? body.lines[Number(m[1])] : undefined;
        if (line) le[line.position] = fieldMessage(v) ?? v;
        else rest.push(fieldMessage(v) ?? v);
      }
      lineErrors = le;
      formError = rest.join(' ');
    } else {
      formError = errorMessage(err);
    }
  }

  function fillAll() {
    if (!src) return;
    const next: Record<number, string> = {};
    for (const l of src.lines) {
      const max = toMilli(l.returnable) < toMilli(l.return_stock) ? l.returnable : l.return_stock;
      if (toMilli(max) > 0n) next[l.position] = String(Number(max));
    }
    qtys = next;
  }

  // Kunci idempotensi per isi permintaan: klik ganda memakai kunci yang sama; isi berubah → kunci baru.
  let attempt: { sig: string; key: string } | null = null;
  function keyFor(sig: string): string {
    if (!attempt || attempt.sig !== sig) attempt = { sig, key: `rb-${crypto.randomUUID()}` };
    return attempt.key;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSave || busy || !input) return;
    busy = true;
    formError = '';
    const body: ReturnInput = {
      ...input,
      ...(note.trim() ? { note: note.trim() } : {}),
      ...(needsMethod ? { refund_method_id: methodId, ...(refundRef.trim() ? { refund_ref: refundRef.trim() } : {}) } : {})
    };
    try {
      saved = await api.create(body, keyFor(JSON.stringify(body)));
      attempt = null;
      qtys = {};
      note = '';
      refundRef = '';
      quote = null;
      if (src) src = await api.source(src.purchase_id).catch(() => src);
    } catch (err) {
      applyError(err, body);
    } finally {
      busy = false;
    }
  }

  onMount(async () => {
    document.title = t('purchaseReturns.form.docTitle');
    const id = new URLSearchParams(location.search).get('purchase');
    if (id) void pick(id);
    try {
      methods = await paymentMethodsLookup.all();
      methodId = methods[0]?.id ?? '';
    } catch {
      /* metode hanya dibutuhkan bila ada dana kembali; galat ditampilkan saat simpan */
    }
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="max-w-3xl">
      <a href="/purchase-returns" class="inline-flex items-center gap-1 text-[12px] text-[var(--text-tertiary)] hover:text-[var(--color-primary)]"><i class="icon-arrow-left text-[12px]"></i>{t('purchaseReturns.form.back')}</a>
      <h1 class="mt-1 font-display font-bold text-[19px]">{t('purchaseReturns.form.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('purchaseReturns.form.subtitle')}</p>
    </div>
    {#if can('stock_transfer', 'create')}
      <a href="/stock-transfer" class="btn btn-sm"><i class="icon-arrow-left-right text-[13px]"></i>{t('purchaseReturns.form.stockLink')}</a>
    {/if}
  </div>

  {#if saved}
    <div role="status" class="flex flex-wrap items-center gap-3 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i>
      <span class="grow">{t('purchaseReturns.form.saved', { doc: saved.doc_no })}</span>
      <a href="/purchase-returns?open={saved.id}" class="btn btn-sm">{t('purchaseReturns.detail')}</a>
      <button type="button" class="btn btn-sm" onclick={reset}>{t('purchaseReturns.form.another')}</button>
    </div>
  {/if}

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{loadError}</span>
    </div>
  {/if}

  {#if !src}
    <section class="surface-card !p-0 overflow-hidden">
      <div class="p-3 border-b border-[var(--border-subtle)]">
        <h2 class="mb-2 text-[13px] font-semibold">{t('purchaseReturns.form.pickPurchase')}</h2>
        <div class="relative sm:w-96">
          <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.75rem"></i>
          <!-- svelte-ignore a11y_autofocus -->
          <input type="search" class={inputClass} style="padding-inline-start:2rem" placeholder={t('purchaseReturns.form.pickHint')} aria-label={t('purchaseReturns.form.pickHint')} bind:value={search} maxlength="100" autofocus />
        </div>
      </div>
      <ul class="divide-y divide-[var(--border-subtle)]">
        {#each choices as c (c.id)}
          <li>
            <button type="button" class="flex w-full flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-start hover:bg-[var(--color-primary)]/5" onclick={() => pick(c.id)}>
              <span class="font-mono text-[12.5px] font-bold">{c.doc_no}</span>
              <span class="font-medium">{c.supplier_name}</span>
              {#if c.supplier_invoice_no}<span class="font-mono text-[11.5px] text-[var(--text-tertiary)]">{c.supplier_invoice_no}</span>{/if}
              <span class="text-[12px] text-[var(--text-tertiary)]">{formatDate(c.purchase_date)}</span>
              <span class="badge-soft {c.payment_type === 'credit' ? 'badge-warning' : 'badge-success'}">{t(`purchases.type.${c.payment_type}`)}</span>
              <span class="ms-auto tabular-nums font-semibold">{money(c.total)}</span>
            </button>
          </li>
        {:else}
          <li class="px-4 py-12 text-center text-[12.5px] text-[var(--text-tertiary)]">{searching ? '…' : t('purchaseReturns.form.noPurchases')}</li>
        {/each}
      </ul>
    </section>
  {:else}
    <form class="space-y-4" onsubmit={submit}>
      <section class="surface-card !p-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <dl class="grid grow grid-cols-2 gap-x-6 gap-y-2 text-[13px] sm:grid-cols-5">
            <div><dt class={label}>{t('purchaseReturns.col.purchase')}</dt><dd class="font-mono text-[12.5px] font-bold">{src.doc_no}</dd></div>
            <div><dt class={label}>{t('purchaseReturns.form.supplier')}</dt><dd class="font-semibold">{src.supplier_name}</dd></div>
            <div><dt class={label}>{t('purchaseReturns.form.invoice')}</dt><dd class="font-mono text-[12px]">{src.supplier_invoice_no || '—'}</dd></div>
            <div><dt class={label}>{t('purchaseReturns.form.date')}</dt><dd>{formatDate(src.purchase_date)} · <span class="badge-soft {src.payment_type === 'credit' ? 'badge-warning' : 'badge-success'}">{t(`purchases.type.${src.payment_type}`)}</span></dd></div>
            <div>
              <dt class={label}>{t('purchaseReturns.form.payableBalance')}</dt>
              <dd class="font-semibold tabular-nums">{src.payable_balance !== null ? money(src.payable_balance) : t('purchaseReturns.form.noPayable')}</dd>
            </div>
          </dl>
          <button type="button" class="btn btn-sm" onclick={reset}><i class="icon-arrow-left-right text-[13px]"></i>{t('purchaseReturns.form.change')}</button>
        </div>
      </section>

      <section class="surface-card !p-0 overflow-hidden">
        <div class="flex flex-wrap justify-end gap-2 p-3 border-b border-[var(--border-subtle)]">
          <button type="button" class="btn btn-sm" onclick={fillAll}>{t('purchaseReturns.form.fillAll')}</button>
          <button type="button" class="btn btn-sm" onclick={() => (qtys = {})}>{t('purchaseReturns.form.clear')}</button>
        </div>
        <div class="overflow-x-auto scroll-thin">
          <table class="w-full min-w-[900px] text-[12.5px]">
            <thead>
              <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                <th class="px-4 py-3 text-start" scope="col">{t('purchaseReturns.form.col.item')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.bought')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.returned')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.returnable')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.returnStock')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.price')}</th>
                <th class="px-3 py-3 text-end w-36" scope="col">{t('purchaseReturns.form.col.qty')}</th>
                <th class="px-3 py-3 text-end" scope="col">{t('purchaseReturns.form.col.value')}</th>
              </tr>
            </thead>
            <tbody>
              {#each src.lines as l (l.position)}
                {@const err = localErrors[l.position] ?? lineErrors[l.position]}
                {@const short = issues.get(l.position)}
                {@const done = toMilli(l.returnable) <= 0n}
                <tr class="border-b border-[var(--border-subtle)] align-top last:border-0 {done ? 'opacity-50' : ''}">
                  <td class="px-4 py-2.5"><div class="font-medium">{l.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{l.sku} · {l.unit}</div></td>
                  <td class="px-3 py-2.5 text-end tabular-nums">{q3(l.qty)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums">{Number(l.returned) > 0 ? q3(l.returned) : '—'}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums font-semibold">{q3(l.returnable)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums {Number(l.return_stock) <= 0 ? 'text-[var(--color-danger-600)]' : ''}">{q3(l.return_stock)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">{money(l.net_price)}</td>
                  <td class="px-3 py-2">
                    <input
                      type="text"
                      inputmode="decimal"
                      class="{inputClass} !h-8 text-end tabular-nums {err || short ? '!border-[var(--color-danger-500)]' : ''}"
                      aria-label="{t('purchaseReturns.form.col.qty')} {l.name}"
                      aria-invalid={!!err || !!short}
                      disabled={done}
                      placeholder="0"
                      bind:value={qtys[l.position]}
                    />
                    {#if err}<p class="mt-1 text-end text-[11px] text-[var(--color-danger-600)]">{err}</p>
                    {:else if short}<p class="mt-1 text-end text-[11px] text-[var(--color-danger-600)]">{t('purchaseReturns.form.stockShort', { stock: q3(short) })}</p>{/if}
                  </td>
                  <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap font-semibold">{values.has(l.position) ? money(values.get(l.position) ?? '0') : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <div class="grid gap-4 lg:grid-cols-2">
        <section class="surface-card !p-4 space-y-3">
          <div>
            <label class="mb-1 block text-[12px] font-semibold" for="ret-note">{t('purchaseReturns.form.note')}</label>
            <textarea id="ret-note" rows="3" maxlength="500" class="w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 py-2 text-[13px] outline-none focus:border-[var(--color-primary-500)]" placeholder={t('purchaseReturns.form.notePlaceholder')} bind:value={note}></textarea>
          </div>
          {#if needsMethod}
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <span class="mb-1 block text-[12px] font-semibold">{t('purchaseReturns.form.refundMethod')}</span>
                <Select ariaLabel={t('purchaseReturns.form.refundMethod')} bind:value={methodId} options={methodOptions} />
              </div>
              <div>
                <label class="mb-1 block text-[12px] font-semibold" for="ret-ref">{t('purchaseReturns.form.refundRef')}</label>
                <input id="ret-ref" class={inputClass} maxlength="100" autocomplete="off" bind:value={refundRef} />
              </div>
            </div>
          {/if}
        </section>

        <section class="surface-card !p-4">
          <dl class="space-y-1.5 text-[13px]">
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.subtotal')}</dt><dd class="tabular-nums">{quote ? money(quote.subtotal) : '—'}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.tax', { pct: formatNumber(Number(src.tax_pct), { maximumFractionDigits: 2 }) })}</dt><dd class="tabular-nums">{quote ? money(quote.tax_amount) : '—'}</dd></div>
            <div class="flex justify-between gap-3 border-t border-[var(--border-subtle)] pt-1.5 text-[15px] font-bold"><dt>{t('purchaseReturns.form.summary.total')}</dt><dd class="tabular-nums">{quote ? money(quote.total) : '—'}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.cut')}</dt><dd class="tabular-nums">{quote ? money(quote.payable_cut) : '—'}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('purchaseReturns.form.summary.refund')}</dt><dd class="tabular-nums font-semibold {needsMethod ? 'text-[var(--color-success-600)]' : ''}">{quote ? money(quote.refund) : '—'}</dd></div>
          </dl>
          <p class="mt-2 text-[11.5px] text-[var(--text-tertiary)]">{t('purchaseReturns.form.summary.hint')}</p>
          {#if formError}<p role="alert" class="mt-2 text-[12px] text-[var(--color-danger-600)]">{formError}</p>{/if}
          {#if input && input.lines.length === 0}<p class="mt-2 text-[12px] text-[var(--text-tertiary)]">{t('purchaseReturns.form.nothing')}</p>{/if}
          <div class="mt-3 flex justify-end">
            <button type="submit" class="btn btn-primary" disabled={!canSave || busy}>
              <i class="icon-undo-2 text-[14px]"></i>{busy ? t('purchaseReturns.form.saving') : t('purchaseReturns.form.save')}
            </button>
          </div>
        </section>
      </div>
    </form>
  {/if}
</main>
