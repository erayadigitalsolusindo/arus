<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Select from '#lib/components/Select.svelte';
  import { salesReturns, type SaleReturnChoice, type SaleReturnInput, type SaleReturnQuote, type SaleReturnSource } from '#lib/sales/returns.ts';
  import { paymentMethodsLookup, type PaymentMethod } from '#lib/catalog/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { toMilli } from '#lib/pos/money.ts';
  import { t, formatCurrency, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  const money = (value: string | number) => formatCurrency(Number(value), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const qtyFmt = (value: string) => formatNumber(Number(value), { maximumFractionDigits: 3 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
  const labelClass = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const cleanQty = (raw: string) => {
    const value = raw.trim().replace(',', '.');
    return /^\d*\.?\d{0,3}$/.test(value) && toMilli(value) > 0n ? value : '';
  };
  const hasPayable = (value: string) => Number(value) > 0;

  let search = $state('');
  let choices = $state<SaleReturnChoice[]>([]);
  let searching = $state(false);
  let source = $state<SaleReturnSource | null>(null);
  let loadError = $state('');
  let qtys = $state<Record<number, string>>({});
  let note = $state('');
  let methods = $state<PaymentMethod[]>([]);
  let methodId = $state('');
  let refundRef = $state('');
  let quote = $state<SaleReturnQuote | null>(null);
  let quoting = $state(false);
  let lineErrors = $state<Record<number, string>>({});
  let formError = $state('');
  let busy = $state(false);
  let saved = $state<{ id: string; doc_no: string } | null>(null);

  // Deposit member hanya untuk nota ber-member (server juga menolak).
  const usableMethods = $derived(methods.filter((method) => method.kind !== 'deposit' || !!source?.has_member));
  const methodOptions = $derived(usableMethods.map((method) => ({ value: method.id, label: method.name })));
  const input = $derived.by((): SaleReturnInput | null => {
    if (!source) return null;
    const lines = source.lines.map((line) => ({ position: line.position, qty: cleanQty(qtys[line.position] ?? '') })).filter((line) => line.qty !== '');
    return { sale_id: source.sale_id, lines };
  });
  const localErrors = $derived.by(() => {
    const errors: Record<number, string> = {};
    for (const line of source?.lines ?? []) {
      const raw = (qtys[line.position] ?? '').trim();
      if (raw === '') continue;
      const value = cleanQty(raw);
      if (!value) errors[line.position] = fieldMessage('INVALID') ?? '';
      else if (toMilli(value) > toMilli(line.returnable)) errors[line.position] = fieldMessage('TOO_HIGH') ?? '';
    }
    return errors;
  });
  const values = $derived(new Map((quote?.lines ?? []).map((line) => [line.position, line.value])));
  const needsRefund = $derived(!!quote && Number(quote.refund) > 0);
  const selectedMethod = $derived(usableMethods.find((method) => method.id === methodId));
  const needsReference = $derived(needsRefund && !!selectedMethod && selectedMethod.kind !== 'cash' && selectedMethod.kind !== 'deposit');
  // Metode terpilih tidak tersedia untuk nota ini (mis. deposit tanpa member) → kembali ke metode pertama.
  $effect(() => {
    if (methodId && !usableMethods.some((method) => method.id === methodId)) methodId = usableMethods[0]?.id ?? '';
  });
  const canSave = $derived(!!input && input.lines.length > 0 && Object.keys(localErrors).length === 0 && !!quote && !quoting && (!needsRefund || !!selectedMethod) && (!needsReference || refundRef.trim() !== ''));

  let choiceSeq = 0;
  $effect(() => {
    if (source) return;
    const query = search.trim();
    void session.outlet?.id;
    const mine = ++choiceSeq;
    searching = true;
    const timer = setTimeout(async () => {
      try {
        const result = await salesReturns.choices(query);
        if (mine === choiceSeq) choices = result;
      } catch (err) {
        if (mine === choiceSeq) loadError = errorMessage(err);
      } finally {
        if (mine === choiceSeq) searching = false;
      }
    }, 250);
    return () => clearTimeout(timer);
  });

  async function choose(saleId: string) {
    loadError = '';
    try {
      source = await salesReturns.source(saleId);
      qtys = {};
      note = '';
      refundRef = '';
      quote = null;
      lineErrors = {};
      formError = '';
      saved = null;
    } catch (err) {
      loadError = errorMessage(err);
    }
  }

  async function reset() {
    source = null;
    qtys = {};
    quote = null;
    saved = null;
    lineErrors = {};
    formError = '';
    loadError = '';
    history.replaceState(history.state, '', '/sales-returns/new');
  }

  let quoteSeq = 0;
  let quoteController: AbortController | undefined;
  $effect(() => {
    const body = input;
    if (!body || body.lines.length === 0 || Object.keys(localErrors).length > 0) {
      quote = null;
      return;
    }
    const signature = JSON.stringify(body);
    const mine = ++quoteSeq;
    quoting = true;
    const timer = setTimeout(async () => {
      quoteController?.abort();
      quoteController = new AbortController();
      try {
        const result = await salesReturns.quote(JSON.parse(signature), quoteController.signal);
        if (mine !== quoteSeq) return;
        quote = result;
        lineErrors = {};
        formError = '';
      } catch (err) {
        if (mine !== quoteSeq || (err instanceof DOMException && err.name === 'AbortError')) return;
        quote = null;
        applyError(err, body);
      } finally {
        if (mine === quoteSeq) quoting = false;
      }
    }, 250);
    return () => clearTimeout(timer);
  });

  function applyError(err: unknown, body: SaleReturnInput) {
    if (err instanceof ApiError && err.code === 'VALIDATION') {
      const perLine: Record<number, string> = {};
      const rest: string[] = [];
      for (const [key, code] of Object.entries(err.fields)) {
        const match = /^lines\.(\d+)\./.exec(key);
        const line = match ? body.lines[Number(match[1])] : undefined;
        if (line) perLine[line.position] = fieldMessage(code) ?? code;
        else rest.push(fieldMessage(code) ?? code);
      }
      lineErrors = perLine;
      formError = rest.join(' ');
    } else {
      formError = errorMessage(err);
    }
  }

  function fillAll() {
    if (!source) return;
    const next: Record<number, string> = {};
    for (const line of source.lines) if (toMilli(line.returnable) > 0n) next[line.position] = line.returnable;
    qtys = next;
  }

  let attempt: { signature: string; key: string } | null = null;
  function keyFor(signature: string) {
    if (!attempt || attempt.signature !== signature) attempt = { signature, key: `sr-${crypto.randomUUID()}` };
    return attempt.key;
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (!canSave || busy || !input) return;
    busy = true;
    formError = '';
    const body: SaleReturnInput = {
      ...input,
      ...(note.trim() ? { note: note.trim() } : {}),
      ...(needsRefund ? { refund_method_id: methodId, ...(needsReference && refundRef.trim() ? { refund_ref: refundRef.trim() } : {}) } : {})
    };
    try {
      saved = await salesReturns.create(body, keyFor(JSON.stringify(body)));
      attempt = null;
      qtys = {};
      note = '';
      refundRef = '';
      quote = null;
      if (source) source = await salesReturns.source(source.sale_id).catch(() => source);
    } catch (err) {
      applyError(err, body);
    } finally {
      busy = false;
    }
  }

  onMount(async () => {
    document.title = t('sales.returnFlow.form.docTitle');
    const saleId = new URLSearchParams(location.search).get('sale');
    if (saleId) void choose(saleId);
    try {
      methods = await paymentMethodsLookup.all('sale_return');
      methodId = methods[0]?.id ?? '';
    } catch {
      // The payment lookup is only needed when a refund is due.
    }
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="max-w-3xl">
      <a href="/sales-returns" class="inline-flex items-center gap-1 text-[12px] text-[var(--text-tertiary)] hover:text-[var(--color-primary)]"><i class="icon-arrow-left text-[12px]"></i>{t('sales.returnFlow.form.back')}</a>
      <h1 class="mt-1 font-display font-bold text-[19px]">{t('sales.returnFlow.form.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('sales.returnFlow.form.subtitle')}</p>
    </div>
    {#if source}<button type="button" class="btn btn-sm" onclick={reset}><i class="icon-receipt-text text-[13px]"></i>{t('sales.returnFlow.form.change')}</button>{/if}
  </div>

  {#if saved}
    <div role="status" class="flex flex-wrap items-center gap-3 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span class="grow">{t('sales.returnFlow.form.saved', { doc: saved.doc_no })}</span>
      <a href="/sales-returns?open={saved.id}" class="btn btn-sm">{t('sales.returnFlow.detail')}</a>
      <button type="button" class="btn btn-sm" onclick={reset}>{t('sales.returnFlow.form.another')}</button>
    </div>
  {/if}

  {#if loadError}<div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{loadError}</span></div>{/if}

  {#if !source}
    <section class="surface-card !p-0 overflow-hidden">
      <div class="p-3 border-b border-[var(--border-subtle)]">
        <h2 class="mb-2 text-[13px] font-semibold">{t('sales.returnFlow.form.pickSale')}</h2>
        <div class="relative sm:w-96">
          <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.75rem"></i>
          <input type="search" class={inputClass} style="padding-inline-start:2rem" placeholder={t('sales.returnFlow.form.pickHint')} aria-label={t('sales.returnFlow.form.pickHint')} bind:value={search} maxlength="100" />
        </div>
      </div>
      <ul class="divide-y divide-[var(--border-subtle)]">
        {#each choices as choice (choice.sale_id)}
          <li><button type="button" class="flex w-full flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-start hover:bg-[var(--color-primary)]/5" onclick={() => choose(choice.sale_id)}>
            <span class="font-mono text-[12.5px] font-bold">{choice.doc_no}</span>
            <span class="font-medium">{choice.member_name || '—'}</span>
            <span class="text-[12px] text-[var(--text-tertiary)]">{formatDateTime(choice.created_at)}</span>
            {#if hasPayable(choice.receivable)}<span class="badge-soft badge-warning">{t('sales.returnFlow.form.receivable')}: {money(choice.receivable)}</span>{/if}
            <span class="ms-auto tabular-nums font-semibold">{money(choice.total)}</span>
          </button></li>
        {:else}
          <li class="px-4 py-12 text-center text-[12.5px] text-[var(--text-tertiary)]">{searching ? '…' : t('sales.returnFlow.form.noSales')}</li>
        {/each}
      </ul>
    </section>
  {:else}
    <form class="space-y-4" onsubmit={submit}>
      <section class="surface-card !p-4">
        <dl class="grid grow grid-cols-2 gap-x-6 gap-y-2 text-[13px] sm:grid-cols-5">
          <div><dt class={labelClass}>{t('sales.returnFlow.col.sale')}</dt><dd class="font-mono text-[12.5px] font-bold">{source.doc_no}</dd></div>
          <div><dt class={labelClass}>{t('sales.returnFlow.form.member')}</dt><dd class="font-semibold">{source.member_name || '—'}</dd></div>
          <div><dt class={labelClass}>{t('sales.returnFlow.form.date')}</dt><dd>{formatDateTime(source.created_at)}</dd></div>
          <div><dt class={labelClass}>{t('sales.returnFlow.modal.outlet')}</dt><dd>{source.outlet_name}</dd></div>
          <div><dt class={labelClass}>{t('sales.returnFlow.form.total')}</dt><dd class="font-semibold tabular-nums">{money(source.total)}</dd></div>
          <div><dt class={labelClass}>{t('sales.returnFlow.form.receivable')}</dt><dd class="font-semibold tabular-nums">{hasPayable(source.receivable) ? money(source.receivable) : t('sales.returnFlow.form.noReceivable')}</dd></div>
        </dl>
      </section>

      <section class="surface-card !p-0 overflow-hidden">
        <div class="flex flex-wrap justify-end gap-2 p-3 border-b border-[var(--border-subtle)]">
          <button type="button" class="btn btn-sm" onclick={fillAll}>{t('sales.returnFlow.form.fillAll')}</button>
          <button type="button" class="btn btn-sm" onclick={() => (qtys = {})}>{t('sales.returnFlow.form.clear')}</button>
        </div>
        <div class="overflow-x-auto scroll-thin">
          <table class="w-full min-w-[900px] text-[12.5px]">
            <thead><tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="px-4 py-3 text-start" scope="col">{t('sales.returnFlow.form.col.item')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.sold')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.returned')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.returnable')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.returnsStock')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.price')}</th>
              <th class="px-3 py-3 text-end w-36" scope="col">{t('sales.returnFlow.form.col.qty')}</th>
              <th class="px-3 py-3 text-end" scope="col">{t('sales.returnFlow.form.col.value')}</th>
            </tr></thead>
            <tbody>
              {#each source.lines as line (line.position)}
                {@const fieldError = localErrors[line.position] ?? lineErrors[line.position]}
                {@const done = toMilli(line.returnable) <= 0n}
                {@const value = values.get(line.position)}
                <tr class="border-b border-[var(--border-subtle)] align-top last:border-0 {done ? 'opacity-50' : ''}">
                  <td class="px-4 py-2.5"><div class="font-medium">{line.name}</div><div class="font-mono text-[11px] text-[var(--text-tertiary)]">{line.sku} · {line.unit}</div></td>
                  <td class="px-3 py-2.5 text-end tabular-nums">{qtyFmt(line.qty)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums">{Number(line.returned) > 0 ? qtyFmt(line.returned) : '—'}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums font-semibold">{qtyFmt(line.returnable)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums">{qtyFmt(line.return_stock)}</td>
                  <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">{money(line.unit_price)}</td>
                  <td class="px-3 py-2">
                    <input type="text" inputmode="decimal" class="{inputClass} !h-8 text-end tabular-nums {fieldError ? '!border-[var(--color-danger-500)]' : ''}"
                      aria-label="{t('sales.returnFlow.form.col.qty')} {line.name}" aria-invalid={!!fieldError} disabled={done} placeholder="0" bind:value={qtys[line.position]} />
                    {#if fieldError}<p class="mt-1 text-end text-[11px] text-[var(--color-danger-600)]">{fieldError}</p>{/if}
                  </td>
                  <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap font-semibold">{value ? money(value) : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <div class="grid gap-4 lg:grid-cols-2">
        <section class="surface-card !p-4 space-y-3">
          <div><label class="mb-1 block text-[12px] font-semibold" for="sale-return-note">{t('sales.returnFlow.form.note')}</label>
            <textarea id="sale-return-note" rows="3" maxlength="500" class="w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 py-2 text-[13px] outline-none focus:border-[var(--color-primary-500)]" placeholder={t('sales.returnFlow.form.notePlaceholder')} bind:value={note}></textarea>
          </div>
          {#if needsRefund}
            <div class="grid gap-3 sm:grid-cols-2">
              <div><span class="mb-1 block text-[12px] font-semibold">{t('sales.returnFlow.form.refundMethod')}</span><Select ariaLabel={t('sales.returnFlow.form.refundMethod')} bind:value={methodId} options={methodOptions} /></div>
              {#if selectedMethod?.kind === 'deposit'}
                <p class="self-end text-[11.5px] text-[var(--text-secondary)]">{t('sales.returnFlow.depositHint')}</p>
              {:else}
                <div><label class="mb-1 block text-[12px] font-semibold" for="sale-return-ref">{t('sales.returnFlow.form.refundRef')}</label>
                  <input id="sale-return-ref" class={inputClass} maxlength="100" autocomplete="off" bind:value={refundRef} required={needsReference} />
                  {#if needsReference}<p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('sales.returnFlow.form.refundRefHint')}</p>{/if}
                </div>
              {/if}
            </div>
          {/if}
        </section>

        <section class="surface-card !p-4">
          <dl class="space-y-1.5 text-[13px]">
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.subtotal')}</dt><dd class="tabular-nums">{quote ? money(quote.subtotal) : '—'}</dd></div>
            {#if quote && Number(quote.discount) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.discount')}</dt><dd class="tabular-nums">−{money(quote.discount)}</dd></div>{/if}
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.tax')}</dt><dd class="tabular-nums">{quote ? money(quote.tax_amount) : '—'}</dd></div>
            {#if quote && Number(quote.surcharge) > 0}<div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.surcharge')}</dt><dd class="tabular-nums">{money(quote.surcharge)}</dd></div>{/if}
            <div class="flex justify-between gap-3 border-t border-[var(--border-subtle)] pt-1.5 text-[15px] font-bold"><dt>{t('sales.returnFlow.form.summary.total')}</dt><dd class="tabular-nums">{quote ? money(quote.total) : '—'}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.cut')}</dt><dd class="tabular-nums">{quote ? money(quote.receivable_cut) : '—'}</dd></div>
            <div class="flex justify-between gap-3"><dt class="text-[var(--text-secondary)]">{t('sales.returnFlow.form.summary.refund')}</dt><dd class="tabular-nums font-semibold">{quote ? money(quote.refund) : '—'}</dd></div>
          </dl>
          {#if quote && (quote.points_earned_reversed > 0 || quote.points_redeemed_restored > 0)}
            <div class="mt-3 rounded border border-[var(--border-subtle)] p-3 text-[12px]">
              {#if quote.points_earned_reversed}<div>{t('sales.returnFlow.form.points.earned')}: −{formatNumber(quote.points_earned_reversed)}</div>{/if}
              {#if quote.points_redeemed_restored}<div>{t('sales.returnFlow.form.points.redeemed')}: +{formatNumber(quote.points_redeemed_restored)}</div>{/if}
              <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('sales.returnFlow.form.points.hint')}</p>
            </div>
          {/if}
          <p class="mt-2 text-[11.5px] text-[var(--text-tertiary)]">{t('sales.returnFlow.form.summary.hint')}</p>
          {#if formError}<p role="alert" class="mt-2 text-[12px] text-[var(--color-danger-600)]">{formError}</p>{/if}
          {#if input && input.lines.length === 0}<p class="mt-2 text-[12px] text-[var(--text-tertiary)]">{t('sales.returnFlow.form.nothing')}</p>{/if}
          <div class="mt-3 flex justify-end"><button type="submit" class="btn btn-primary" disabled={!canSave || busy}>
            <i class="icon-undo-2 text-[14px]"></i>{busy ? t('sales.returnFlow.form.saving') : t('sales.returnFlow.form.save')}
          </button></div>
        </section>
      </div>
    </form>
  {/if}
</main>
