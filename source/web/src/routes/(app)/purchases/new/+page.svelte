<script lang="ts">
  import { onMount } from 'svelte';
  import { purchases, type ItemChoice, type PaymentType, type PurchaseInput, type Quote } from '#lib/purchases/api.ts';
  import { lookup, suppliers, type Supplier } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatNumber, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import PurchaseModal from '#lib/components/PurchaseModal.svelte';

  type Row = { key: string; item: ItemChoice; qd: string; qw: string; price: string; sub: string; disc: [string, string, string, string] };

  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { maximumFractionDigits: 2 });
  const num = (s: string) => (s === '' ? 0 : Number(s));
  const initialsOf = (name: string) => name.split(/s+/).filter((w) => /[p{L}p{N}]/u.test(w)).slice(0, 2).map((w) => w.replace(/^[^p{L}p{N}]+/u, '')[0]?.toUpperCase() ?? '').join('') || '?';
  const trim = (n: string) => (n.includes('.') ? n.replace(/.?0+$/, '') : n);
  const qtyOf = (r: Row) => num(r.qd) + num(r.qw);
  // Sub total (sebelum diskon) = qty × harga; kalau diketik langsung, harga = sub total ÷ qty (maks 4 desimal, sama dengan server).
  const calcSub = (r: Row) => (r.price === '' || qtyOf(r) <= 0 ? '' : trim((Math.round(qtyOf(r) * num(r.price) * 100) / 100).toFixed(2)));
  function setField(r: Row, k: 'qd' | 'qw' | 'price', v: string) {
    r[k] = v;
    r.sub = calcSub(r);
  }
  function setSub(r: Row, v: string) {
    r.sub = v;
    if (v === '') r.price = '';
    else if (qtyOf(r) > 0) r.price = trim((num(v) / qtyOf(r)).toFixed(4));
  }
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const todayStr = ymd(new Date());
  const addDays = (s: string, n: number) => {
    const [y, m, d] = s.split('-').map(Number);
    return ymd(new Date(y, m - 1, d + n));
  };

  let supplierId = $state('');
  let supplierLabel = $state('');
  let invoice = $state('');
  let date = $state(todayStr);
  let payment = $state<PaymentType>('cash');
  let due = $state('');
  let taxPct = $state('');
  let costs = $state<{ key: string; name: string; amount: string }[]>([]);
  let note = $state('');
  let rows = $state<Row[]>([]);

  let scanQty = $state('1');
  let scanText = $state('');
  let hits = $state<ItemChoice[]>([]);
  let active = $state(-1); // baris hasil cari yang disorot keyboard (-1 = belum ada)
  let scanMsg = $state('');
  let rowFilter = $state('');
  let supplier = $state<Supplier | null>(null);

  let quote = $state<Quote | null>(null);
  let quoting = $state(false);
  let busy = $state(false);
  let formError = $state('');
  let fieldErrors = $state<Record<string, string>>({});
  let doneId = $state('');
  let doneNo = $state('');
  let viewId = $state<string | null>(null);

  const canCreate = $derived(can('purchase_invoices', 'create'));
  const lineOk = (r: Row) => num(r.qd) + num(r.qw) > 0 && r.price !== '';
  const complete = $derived(rows.filter(lineOk));
  const allComplete = $derived(rows.length > 0 && complete.length === rows.length);
  const canSave = $derived(canCreate && !busy && supplierId !== '' && date !== '' && allComplete);
  const qLine = $derived(new Map(quote ? complete.map((r, i) => [r.key, quote!.lines[i]] as const) : []));

  async function searchSuppliers(q: string): Promise<Option[]> {
    return lookup('suppliers').search(q);
  }
  // Detail pemasok hanya tampil bila pemakai berhak membaca master pemasok; selain itu diabaikan tanpa galat.
  $effect(() => {
    const id = supplierId;
    supplier = null;
    if (!id) return;
    suppliers
      .list({ q: supplierLabel, limit: 20 })
      .then((r) => {
        if (supplierId === id) supplier = r.data.find((x) => x.id === id) ?? null;
      })
      .catch(() => {});
  });

  function addItem(it: ItemChoice, qty: string) {
    const q = num(qty) > 0 ? qty : '1';
    const ex = rows.find((r) => r.item.id === it.id);
    if (ex) setField(ex, 'qd', String(Math.round((num(ex.qd) + num(q)) * 1000) / 1000));
    else {
      const row: Row = { key: crypto.randomUUID(), item: it, qd: q, qw: '', price: Number(it.last_cost) > 0 ? it.last_cost : '', sub: '', disc: ['', '', '', ''] };
      row.sub = calcSub(row);
      rows.push(row);
    }
    scanText = '';
    hits = [];
    active = -1;
    scanMsg = '';
    scanQty = '1';
    document.getElementById('pb-scan')?.focus();
  }

  // Pencarian saat mengetik (debounce); Enter memasukkan hasil yang cocok persis (SKU/barcode) atau satu-satunya hasil.
  let sseq = 0;
  $effect(() => {
    const q = scanText.trim();
    scanMsg = '';
    const mine = ++sseq;
    if (q.length < 2) {
      hits = [];
      return;
    }
    const h = setTimeout(async () => {
      try {
        const res = await purchases.items(q);
        if (mine === sseq) {
          hits = res.slice(0, 8);
          active = -1;
        }
      } catch {
        if (mine === sseq) hits = [];
      }
    }, 250);
    return () => clearTimeout(h);
  });

  async function scanEnter() {
    const q = scanText.trim();
    if (!q) {
      document.getElementById('pb-qty')?.focus();
      return;
    }
    let list = hits;
    try {
      list = await purchases.items(q);
    } catch {
      return;
    }
    const lq = q.toLowerCase();
    const exact = list.filter((r) => r.barcode.toLowerCase() === lq || r.sku.toLowerCase() === lq);
    const pick = exact.length === 1 ? exact[0] : list.length === 1 ? list[0] : null;
    if (pick) return addItem(pick, scanQty);
    hits = list.slice(0, 8);
    active = hits.length ? 0 : -1;
    scanMsg = list.length === 0 ? t('purchases.form.notFound') : '';
  }

  const shown = $derived.by(() => {
    const f = rowFilter.trim().toLowerCase();
    return f ? rows.filter((r) => r.item.name.toLowerCase().includes(f) || r.item.sku.toLowerCase().includes(f) || r.item.barcode.toLowerCase().includes(f)) : rows;
  });
  // Navigasi ala Excel di sel angka tabel: Enter/↓/↑ pindah baris; ←/→ pindah kolom bila kursor di tepi teks.
  function navKey(e: KeyboardEvent) {
    const el = e.target as HTMLInputElement;
    if (!(el instanceof HTMLInputElement) || el.dataset.r === undefined) return;
    let r = Number(el.dataset.r);
    let c = Number(el.dataset.c);
    const len = el.value.length;
    const a = el.selectionStart ?? 0;
    const b = el.selectionEnd ?? 0;
    if (e.key === 'Enter' || e.key === 'ArrowDown') r++;
    else if (e.key === 'ArrowUp') r--;
    else if (e.key === 'ArrowRight' && b === len && (a === len || a === 0)) c++;
    else if (e.key === 'ArrowLeft' && a === 0 && (b === 0 || b === len)) c--;
    else return;
    e.preventDefault();
    const next = (e.currentTarget as HTMLElement).querySelector<HTMLInputElement>(`input[data-r="${r}"][data-c="${c}"]`);
    if (next) {
      next.focus();
      next.select();
    } else if (e.key === 'Enter' && r >= shown.length) document.getElementById('pb-scan')?.focus();
  }
  const removeRow = (key: string) => {
    const i = rows.findIndex((r) => r.key === key);
    if (i >= 0) rows.splice(i, 1);
  };

  function body(): Partial<PurchaseInput> {
    return {
      ...(supplierId ? { supplier_id: supplierId } : {}),
      payment_type: payment,
      tax_pct: taxPct || undefined,
      other_costs: costs.filter((c) => c.amount !== '').map((c) => ({ name: c.name, amount: c.amount })),
      lines: complete.map((r) => ({
        item_id: r.item.id,
        qty_display: r.qd || '0',
        qty_warehouse: r.qw || '0',
        unit_price: r.price,
        discounts: r.disc.filter((d) => d !== '')
      }))
    };
  }

  // Pratinjau server (nilai baris, alokasi biaya, HPP baru): aturan sama dengan saat simpan, tanpa menulis apa pun.
  let qseq = 0;
  $effect(() => {
    const payload = JSON.stringify(body());
    const mine = ++qseq;
    if (complete.length === 0 && costs.length === 0) {
      quote = null;
      return;
    }
    quoting = true;
    const ctl = new AbortController();
    const h = setTimeout(async () => {
      try {
        const res = await purchases.quote(JSON.parse(payload), ctl.signal);
        if (mine === qseq) quote = res;
      } catch {
        if (mine === qseq) quote = null; // galat ditampilkan saat simpan; pratinjau hanya petunjuk
      } finally {
        if (mine === qseq) quoting = false;
      }
    }, 300);
    return () => {
      clearTimeout(h);
      ctl.abort();
    };
  });

  // Kunci idempotensi per isi permintaan: klik ganda / jaringan putus memakai kunci yang sama; isi diubah → kunci baru.
  let attempt: { sig: string; key: string } | null = null;
  function keyFor(sig: string): string {
    if (!attempt || attempt.sig !== sig) attempt = { sig, key: `pb-${crypto.randomUUID()}` };
    return attempt.key;
  }

  function reset() {
    rows = [];
    costs = [];
    invoice = '';
    note = '';
    scanText = '';
    hits = [];
    taxPct = '';
    due = '';
    quote = null;
    attempt = null;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSave) return;
    busy = true;
    formError = '';
    fieldErrors = {};
    const input: PurchaseInput = {
      supplier_id: supplierId,
      supplier_invoice_no: invoice.trim() || undefined,
      purchase_date: date,
      payment_type: payment,
      due_date: payment === 'credit' && due ? due : undefined,
      tax_pct: taxPct || undefined,
      other_costs: costs.filter((c) => c.amount !== '').map((c) => ({ name: c.name.trim(), amount: c.amount })),
      note: note.trim() || undefined,
      lines: rows.map((r) => ({
        item_id: r.item.id,
        qty_display: r.qd || '0',
        qty_warehouse: r.qw || '0',
        unit_price: r.price,
        discounts: r.disc.filter((d) => d !== '')
      }))
    };
    try {
      const p = await purchases.create(input, keyFor(JSON.stringify(input)));
      doneId = p.id;
      doneNo = p.doc_no;
      reset();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        fieldErrors = Object.fromEntries(Object.entries(err.fields).map(([k, v]) => [k, fieldMessage(v) ?? errorMessage(err)]));
        formError = t('purchases.form.incomplete');
      } else {
        formError = errorMessage(err);
      }
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    document.title = t('purchases.form.docTitle');
  });

  const th = 'px-2 py-2 text-end whitespace-nowrap';
  const fieldLabel = 'flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wide mb-1 text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] text-[var(--color-danger-600)]';
  const cell = 'h-7 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-1.5 text-end text-[12px] tabular-nums outline-none focus:border-[var(--color-primary-500)]';
  const inputClass = 'h-8 w-full rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] px-2.5 text-[12.5px] outline-none focus:border-[var(--color-primary-500)]';
</script>

<main class="p-3 lg:p-5 space-y-3 max-w-[1680px] mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex items-center gap-3 min-w-0">
      <span class="grid place-items-center size-10 rounded-xl shrink-0 bg-[color-mix(in_srgb,var(--color-primary)_14%,transparent)] text-[var(--color-primary)]"><i class="icon-receipt text-[20px]"></i></span>
      <div class="min-w-0">
        <h1 class="font-display font-bold text-[18px] leading-tight">{t('purchases.form.title')}</h1>
        <p class="text-[12px] text-[var(--text-tertiary)] truncate">{t('purchases.form.subtitle')}</p>
      </div>
    </div>
    <a href="/purchases" class="btn btn-sm"><i class="icon-list text-[14px]"></i>{t('purchases.title')}</a>
  </div>

  {#if doneNo}
    <div role="status" class="flex flex-wrap items-center gap-3 rounded-lg px-3 py-2 text-[13px] badge-success">
      <i class="icon-circle-check text-[16px] shrink-0"></i>
      <span class="grow">{t('purchases.form.done', { doc: doneNo })}</span>
      <button type="button" class="btn btn-sm" onclick={() => (viewId = doneId)}><i class="icon-eye text-[13px]"></i>{t('purchases.form.viewIt')}</button>
      <button type="button" class="btn btn-sm" onclick={() => (doneNo = '')}><i class="icon-plus text-[13px]"></i>{t('purchases.form.newAnother')}</button>
    </div>
  {/if}
  {#if formError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{formError}</span>
    </div>
  {/if}

  <form onsubmit={submit} class="space-y-3" novalidate>
    <section class="surface-card !p-3">
      <div class="grid gap-3 lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1.25fr)_270px]">
        <!-- Pemasok -->
        <div class="rounded-lg border border-[var(--border-subtle)] p-3 flex flex-col gap-3">
          <div>
            <label class={fieldLabel} for="pb-supplier"><i class="icon-truck text-[13px]"></i>{t('purchases.form.supplier')}</label>
            <Combobox id="pb-supplier" bind:value={supplierId} bind:label={supplierLabel} search={searchSuppliers} placeholder={t('purchases.form.supplierPlaceholder')} invalid={!!fieldErrors.supplier_id} />
            {#if fieldErrors.supplier_id}<p class={errClass}>{fieldErrors.supplier_id}</p>{/if}
          </div>
          {#if supplierId}
            <div class="flex items-center gap-3">
              <span class="grid place-items-center size-11 shrink-0 rounded-full font-display font-bold text-[15px] text-white bg-gradient-to-br from-[var(--color-primary)] to-[color-mix(in_srgb,var(--color-primary)_55%,#7c3aed)]">{initialsOf(supplierLabel)}</span>
              <div class="min-w-0">
                <div class="font-display font-bold text-[14px] leading-tight truncate">{supplierLabel}</div>
                <div class="text-[11px] text-[var(--text-tertiary)]">{t('purchases.form.supplier')}</div>
              </div>
            </div>
            <dl class="grid gap-1.5 text-[12px]">
              <div class="flex items-start gap-2 rounded-md bg-[var(--surface-sunken)] px-2.5 py-1.5"><dt class="mt-px text-[var(--text-tertiary)]" title={t('purchases.form.supplierAddress')}><i class="icon-map-pin text-[14px]"></i></dt><dd class="text-[var(--text-secondary)]">{supplier?.address || '—'}</dd></div>
              <div class="flex items-start gap-2 rounded-md bg-[var(--surface-sunken)] px-2.5 py-1.5"><dt class="mt-px text-[var(--text-tertiary)]" title={t('purchases.form.supplierPhone')}><i class="icon-phone text-[14px]"></i></dt><dd class="text-[var(--text-secondary)]">{supplier?.phone || '—'}</dd></div>
            </dl>
          {:else}
            <p class="text-[12px] text-[var(--text-tertiary)] flex items-center gap-2"><i class="icon-info text-[14px]"></i>{t('purchases.form.supplierNone')}</p>
          {/if}
          <!-- Adegan truk pengantar: mengisi ruang kosong dan memberi tanda bahwa barang sedang "dikirim" -->
          <div class="delivery mt-auto relative h-16 overflow-hidden rounded-lg border border-dashed border-[var(--border-default)] bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)]" aria-hidden="true">
            <span class="road"></span>
            <i class="icon-package box box1 text-[16px]"></i>
            <i class="icon-package box box2 text-[13px]"></i>
            <i class="icon-truck truck text-[26px] text-[var(--color-primary)]"></i>
          </div>
        </div>

        <!-- Faktur -->
        <div class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-2">
          <div class="grid gap-2 sm:grid-cols-2">
            <div>
              <label class={fieldLabel} for="pb-invoice"><i class="icon-hash text-[13px]"></i>{t('purchases.form.invoice')}</label>
              <input id="pb-invoice" class={inputClass} bind:value={invoice} maxlength="60" autocomplete="off" />
              {#if fieldErrors.supplier_invoice_no}<p class={errClass}>{fieldErrors.supplier_invoice_no}</p>{/if}
            </div>
            <div>
              <label class={fieldLabel} for="pb-date"><i class="icon-calendar text-[13px]"></i>{t('purchases.form.date')}</label>
              <DatePicker id="pb-date" bind:value={date} max={todayStr} clearable={false} invalid={!!fieldErrors.purchase_date} />
              {#if fieldErrors.purchase_date}<p class={errClass}>{fieldErrors.purchase_date}</p>{/if}
            </div>
          </div>
          <div class="flex flex-wrap items-end gap-2">
            <div>
              <span class={fieldLabel}><i class="icon-wallet text-[13px]"></i>{t('purchases.form.payment')}</span>
              <div class="inline-flex rounded-lg border border-[var(--border-default)] p-0.5" role="group" aria-label={t('purchases.form.payment')}>
                {#each [['cash', 'icon-banknote'], ['credit', 'icon-credit-card']] as const as [p, ic] (p)}
                  <button type="button" class="h-7 inline-flex items-center gap-1.5 rounded-md px-3 text-[12px] font-semibold {payment === p ? 'bg-[var(--color-primary)] text-white' : 'text-[var(--text-secondary)]'}" aria-pressed={payment === p} onclick={() => (payment = p)}><i class="{ic} text-[13px]"></i>{t(`purchases.type.${p}`)}</button>
                {/each}
              </div>
            </div>
            {#if payment === 'credit'}
              <div class="w-56">
                <label class={fieldLabel} for="pb-due"><i class="icon-calendar-clock text-[13px]"></i>{t('purchases.form.due')}</label>
                <DatePicker id="pb-due" bind:value={due} min={date} invalid={!!fieldErrors.due_date} />
              </div>
              <div class="flex gap-1 pb-0.5">
                {#each [['d7', 7], ['d14', 14], ['d30', 30], ['d60', 60]] as const as [k, n] (k)}
                  <button type="button" class="btn btn-sm !h-7 !px-2 !text-[11.5px]" onclick={() => (due = addDays(date, n))}>{t(`purchases.form.dueChips.${k}`)}</button>
                {/each}
              </div>
            {/if}
          </div>
          {#if fieldErrors.due_date}<p class={errClass}>{fieldErrors.due_date}</p>{/if}
          <div>
            <label class={fieldLabel} for="pb-note"><i class="icon-message-square-text text-[13px]"></i>{t('purchases.form.note')}</label>
            <input id="pb-note" class={inputClass} maxlength="500" placeholder={t('purchases.form.noteHere')} bind:value={note} />
          </div>
        </div>

        <!-- Total ringkas -->
        <div class="rounded-lg p-3 flex flex-col gap-2 border border-[color-mix(in_srgb,var(--color-primary)_28%,transparent)] bg-[color-mix(in_srgb,var(--color-primary)_7%,transparent)]" aria-busy={quoting}>
          <div class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]"><i class="icon-calculator text-[13px]"></i>{t('purchases.form.sumInvoice')}</div>
          <div class="font-display font-bold text-[24px] leading-none tabular-nums">{quote ? money(quote.subtotal) : '—'}</div>
          <dl class="grid gap-1 text-[12px] tabular-nums border-t border-[var(--border-subtle)] pt-2">
            <div class="flex items-center justify-between gap-2"><dt class="text-[var(--text-secondary)]">{t('purchases.form.sumTax')}</dt><dd class="font-semibold">{quote ? money(quote.tax_amount) : '—'}</dd></div>
            <div class="flex items-center justify-between gap-2"><dt class="text-[var(--text-secondary)]">{t('purchases.totals.otherCosts')}</dt><dd class="font-semibold">{quote ? money(quote.other_cost) : '—'}</dd></div>
            <div class="flex items-center justify-between gap-2 border-t border-[var(--border-subtle)] pt-1.5 text-[13px]"><dt class="font-semibold">{t('purchases.totals.total')}</dt><dd class="font-bold text-[var(--color-primary)]">{quote ? money(quote.total) : '—'}</dd></div>
          </dl>
          <dl class="grid gap-1 text-[12px] border-t border-[var(--border-subtle)] pt-2">
            <div class="flex items-center justify-between gap-2"><dt class="text-[var(--text-secondary)] inline-flex items-center gap-1.5"><i class="icon-package text-[13px]"></i>{t('purchases.lineCol.item')}</dt><dd class="font-semibold tabular-nums">{rows.length} · {formatNumber(rows.reduce((x, r) => x + qtyOf(r), 0), { maximumFractionDigits: 3 })} {t('purchases.form.qtyShort')}</dd></div>
            <div class="flex items-center justify-between gap-2"><dt class="text-[var(--text-secondary)] inline-flex items-center gap-1.5"><i class="icon-wallet text-[13px]"></i>{t('purchases.form.payment')}</dt><dd><span class="rounded-full px-2 py-0.5 text-[11px] font-semibold {payment === 'credit' ? 'badge-warning' : 'badge-success'}">{t(`purchases.type.${payment}`)}</span></dd></div>
            {#if payment === 'credit'}
              <div class="flex items-center justify-between gap-2"><dt class="text-[var(--text-secondary)] inline-flex items-center gap-1.5"><i class="icon-calendar-clock text-[13px]"></i>{t('purchases.form.due')}</dt><dd class="font-semibold">{due ? formatDate(new Date(due + 'T00:00:00'), { dateStyle: 'medium' }) : '—'}</dd></div>
            {/if}
          </dl>
          <div class="flex items-center gap-2 mt-auto rounded-md bg-[var(--surface-base)] border border-[var(--border-subtle)] px-2.5 py-1.5">
            <label class="text-[12px] font-semibold text-[var(--text-secondary)] whitespace-nowrap grow" for="pb-tax">{t('purchases.form.taxPct')}</label>
            <MoneyInput id="pb-tax" class="h-7 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2 text-end text-[12px] outline-none focus:border-[var(--color-primary-500)]" style="width:4.5rem" decimals={2} pad={false} bind:value={taxPct} />
          </div>
          {#if fieldErrors.tax_pct}<p class={errClass}>{fieldErrors.tax_pct}</p>{/if}
        </div>
      </div>
    </section>

    <section class="surface-card !p-0 overflow-hidden">
      <div class="flex flex-wrap items-center gap-2 p-3 bg-[var(--surface-sunken)] border-b border-[var(--border-subtle)]">
        <div class="relative w-28">
          <i class="icon-shopping-cart absolute start-2.5 top-2 text-[14px] text-[var(--text-tertiary)] pointer-events-none"></i>
          <label class="sr-only" for="pb-qty">{t('purchases.form.qtyShort')}</label>
          <MoneyInput id="pb-qty" class="{inputClass} text-end" style="padding-inline-start:2rem" decimals={3} pad={false} bind:value={scanQty} placeholder={t('purchases.form.qtyShort')} onkeydown={(e: KeyboardEvent) => { if (e.key === 'Enter') { e.preventDefault(); document.getElementById('pb-scan')?.focus(); } }} />
        </div>
        <div class="relative grow min-w-[260px] max-w-3xl">
          <i class="icon-scan-barcode absolute start-3 top-2 text-[15px] text-[var(--color-primary)] pointer-events-none"></i>
          <input
            id="pb-scan"
            class={inputClass}
            style="padding-inline-start:2.25rem"
            bind:value={scanText}
            placeholder={t('purchases.form.scanPlaceholder')}
            autocomplete="off"
            onkeydown={(e) => {
              if (e.key === 'ArrowDown' && hits.length) {
                e.preventDefault();
                active = (active + 1) % hits.length;
              } else if (e.key === 'ArrowUp' && hits.length) {
                e.preventDefault();
                active = active <= 0 ? hits.length - 1 : active - 1;
              } else if (e.key === 'Enter') {
                e.preventDefault();
                if (active >= 0 && hits[active]) addItem(hits[active], scanQty);
                else scanEnter();
              } else if (e.key === 'Escape') {
                hits = [];
                active = -1;
              }
            }}
          />
          {#if hits.length > 0}
            <ul class="absolute z-20 mt-1 w-full rounded-lg border border-[var(--border-default)] bg-[var(--surface-base)] shadow-lg overflow-hidden" role="listbox">
              {#each hits as h, hi (h.id)}
                <li role="option" aria-selected={hi === active}>
                  <button type="button" tabindex="-1" class="w-full text-start px-3 py-1.5 text-[12.5px] flex items-center gap-3 {hi === active ? 'bg-[color-mix(in_srgb,var(--color-primary)_12%,transparent)]' : 'hover:bg-[var(--surface-sunken)]'}" onmouseenter={() => (active = hi)} onclick={() => addItem(h, scanQty)}>
                    <i class="icon-package text-[14px] text-[var(--text-tertiary)]"></i>
                    <span class="grow font-medium truncate">{h.name}</span>
                    <span class="font-mono text-[11px] text-[var(--text-tertiary)]">{h.barcode || h.sku}</span>
                    <span class="text-[11px] text-[var(--text-tertiary)] tabular-nums">{t('purchases.form.stockNow', { qty: formatNumber(Number(h.stock_total), { maximumFractionDigits: 3 }) })}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
          {#if scanMsg}<p class="mt-1 {errClass}">{scanMsg}</p>{/if}
        </div>
        <div class="ms-auto flex items-center gap-2">
          <span class="inline-flex items-center gap-1.5 rounded-full border border-[var(--border-default)] px-2.5 h-7 text-[11.5px] font-semibold text-[var(--text-secondary)]"><i class="icon-package text-[13px]"></i>{t('purchases.form.entries', { count: rows.length })}</span>
          <div class="relative">
            <i class="icon-search absolute start-2.5 top-2 text-[13px] text-[var(--text-tertiary)] pointer-events-none"></i>
            <input class="{inputClass} !w-44" style="padding-inline-start:2rem" placeholder={t('purchases.form.filterRows')} aria-label={t('purchases.form.filterRows')} bind:value={rowFilter} />
          </div>
        </div>
      </div>
      <p class="px-3 py-1.5 text-[11px] text-[var(--text-tertiary)] flex items-start gap-1.5 border-b border-[var(--border-subtle)]"><i class="icon-info text-[13px] mt-px shrink-0"></i><span>{t('purchases.form.scanHint')} {t('purchases.form.discountHint')}</span></p>
      {#if fieldErrors.lines}<p class="px-3 pt-2 {errClass}">{fieldErrors.lines}</p>{/if}

      <div class="overflow-x-auto scroll-thin">
        {#snippet head()}
          <tr class="bg-[var(--surface-sunken)] text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-3 py-2 text-start sticky start-0 z-10 bg-[var(--surface-sunken)] min-w-[250px]" scope="col">{t('purchases.lineCol.item')}</th>
            <th class={th} scope="col">{t('purchases.form.stockBefore')}</th>
            <th class={th} scope="col">{t('purchases.form.qtyTotal')}</th>
            <th class="{th} w-24" scope="col">{t('purchases.lineCol.display')}</th>
            <th class="{th} w-24" scope="col">{t('purchases.lineCol.warehouse')}</th>
            <th class="{th} w-32" scope="col">{t('purchases.form.price')}</th>
            <th class={th} scope="col">{t('purchases.form.subTotal')}</th>
            {#each [1, 2, 3, 4] as n (n)}<th class="{th} w-20" scope="col">{t('purchases.form.discountN', { n })}</th>{/each}
            <th class={th} scope="col">{t('purchases.form.afterDiscount')}</th>
            <th class={th} scope="col">{t('purchases.form.unitCostCol')}</th>
            <th class={th} scope="col">{t('purchases.form.avgAfter')}</th>
            <th class="px-2 py-2 w-9"><span class="sr-only">{t('purchases.form.remove')}</span></th>
          </tr>
        {/snippet}
        <table class="w-full min-w-[1500px] text-[12px]">
          <thead>
            <tr class="text-[10px] font-bold uppercase tracking-wider text-[var(--text-tertiary)] border-b border-[var(--border-subtle)]">
              <th class="px-3 py-1 text-start sticky start-0 z-10 bg-[var(--surface-base)]" scope="colgroup"><i class="icon-package text-[12px] me-1 align-[-1px]"></i>{t('purchases.group.item')}</th>
              <th colspan="4" class="px-2 py-1 text-center bg-[color-mix(in_srgb,var(--color-primary)_7%,transparent)]" scope="colgroup"><i class="icon-boxes text-[12px] me-1 align-[-1px]"></i>{t('purchases.group.stock')}</th>
              <th colspan="6" class="px-2 py-1 text-center" scope="colgroup"><i class="icon-tag text-[12px] me-1 align-[-1px]"></i>{t('purchases.group.price')}</th>
              <th colspan="3" class="px-2 py-1 text-center bg-[color-mix(in_srgb,var(--color-success-600,#16a34a)_9%,transparent)]" scope="colgroup"><i class="icon-calculator text-[12px] me-1 align-[-1px]"></i>{t('purchases.group.result')}</th>
              <th></th>
            </tr>
            {@render head()}
          </thead>
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <tbody onkeydown={navKey}>
            {#each shown as r, ri (r.key)}
              {@const i = rows.indexOf(r)}
              {@const ql = qLine.get(r.key)}
              {@const up = ql ? Number(ql.avg_after) > Number(r.item.avg_cost) : false}
              <tr class="border-t border-[var(--border-subtle)] align-middle even:bg-[var(--surface-sunken)] hover:bg-[color-mix(in_srgb,var(--color-primary)_5%,transparent)]">
                <td class="stickyc px-3 py-1.5 sticky start-0 z-[1]">
                  <div class="flex items-center gap-2.5">
                    <span class="grid place-items-center size-8 rounded-lg shrink-0 bg-[var(--surface-sunken)] text-[var(--text-tertiary)] border border-[var(--border-subtle)]"><i class="icon-package text-[16px]"></i></span>
                    <div class="min-w-0">
                      <div class="font-semibold leading-tight truncate max-w-[210px]" title={r.item.name}>{r.item.name}</div>
                      <div class="font-mono text-[10.5px] text-[var(--text-tertiary)] truncate max-w-[210px]">{r.item.barcode || r.item.sku} · {r.item.unit} · {t('purchases.form.avgNow', { cost: money(r.item.avg_cost) })}</div>
                    </div>
                  </div>
                  {#each ['item_id', 'qty_display', 'qty_warehouse', 'unit_price', 'discounts'] as f (f)}
                    {#if fieldErrors[`lines.${i}.${f}`]}<p class={errClass}>{fieldErrors[`lines.${i}.${f}`]}</p>{/if}
                  {/each}
                </td>
                <td class="px-2 py-1.5 text-end tabular-nums text-[var(--text-secondary)] bg-[color-mix(in_srgb,var(--color-primary)_4%,transparent)]">{formatNumber(Number(r.item.stock_total), { maximumFractionDigits: 3 })}</td>
                <td class="px-2 py-1.5 text-end bg-[color-mix(in_srgb,var(--color-primary)_4%,transparent)]"><span class="inline-block min-w-8 rounded-md px-2 py-0.5 text-center tabular-nums font-bold bg-[color-mix(in_srgb,var(--color-primary)_14%,transparent)] text-[var(--color-primary)]">{formatNumber(num(r.qd) + num(r.qw), { maximumFractionDigits: 3 })}</span></td>
                <td class="px-1.5 py-1.5 bg-[color-mix(in_srgb,var(--color-primary)_4%,transparent)]"><MoneyInput class={cell} decimals={3} pad={false} bind:value={() => r.qd, (v) => setField(r, 'qd', v)} data-r={ri} data-c={0} aria-label={t('purchases.form.qtyDisplay')} /></td>
                <td class="px-1.5 py-1.5 bg-[color-mix(in_srgb,var(--color-primary)_4%,transparent)]"><MoneyInput class={cell} decimals={3} pad={false} bind:value={() => r.qw, (v) => setField(r, 'qw', v)} data-r={ri} data-c={1} aria-label={t('purchases.form.qtyWarehouse')} /></td>
                <td class="px-1.5 py-1.5"><MoneyInput class={cell} decimals={4} pad={false} bind:value={() => r.price, (v) => setField(r, 'price', v)} data-r={ri} data-c={2} aria-label={t('purchases.form.price')} /></td>
                <td class="px-1.5 py-1.5"><MoneyInput class={cell} decimals={2} bind:value={() => r.sub, (v) => setSub(r, v)} data-r={ri} data-c={3} aria-label={t('purchases.form.subTotal')} /></td>
                {#each [0, 1, 2, 3] as j (j)}
                  <td class="px-1 py-1.5"><MoneyInput class={cell} decimals={2} pad={false} placeholder="0" bind:value={r.disc[j]} data-r={ri} data-c={4 + j} aria-label={t('purchases.form.discountN', { n: j + 1 })} /></td>
                {/each}
                <td class="px-2 py-1.5 text-end tabular-nums whitespace-nowrap font-semibold bg-[color-mix(in_srgb,var(--color-success-600,#16a34a)_6%,transparent)]">{ql ? money(ql.line_total) : '—'}</td>
                <td class="px-2 py-1.5 text-end tabular-nums whitespace-nowrap bg-[color-mix(in_srgb,var(--color-success-600,#16a34a)_6%,transparent)]">{ql ? money(ql.unit_cost) : '—'}</td>
                <td class="px-2 py-1.5 text-end tabular-nums whitespace-nowrap font-bold bg-[color-mix(in_srgb,var(--color-success-600,#16a34a)_6%,transparent)]">
                  {#if ql}
                    <span class="inline-flex items-center gap-1 {up ? 'text-[var(--color-warning-600,#d97706)]' : 'text-[var(--color-success-600,#16a34a)]'}"><i class="{up ? 'icon-trending-up' : 'icon-trending-down'} text-[13px]"></i>{money(ql.avg_after)}</span>
                  {:else}—{/if}
                </td>
                <td class="px-1 py-1.5 text-center">
                  <button type="button" class="header-icon-btn !size-7 hover:!text-[var(--color-danger-600)]" aria-label={t('purchases.form.remove')} onclick={() => removeRow(r.key)}><i class="icon-trash-2 text-[14px]"></i></button>
                </td>
              </tr>
            {:else}
              <tr>
                <td colspan="15" class="px-4 py-10 text-center text-[var(--text-tertiary)]">
                  <i class="{rows.length > 0 ? 'icon-search-x' : 'icon-scan-barcode'} text-[28px] block mb-1 opacity-60"></i>
                  {rows.length > 0 ? t('purchases.emptySearch') : t('purchases.form.noLines')}
                </td>
              </tr>
            {/each}
          </tbody>
          {#if shown.length > 8}<tfoot>{@render head()}</tfoot>{/if}
        </table>
      </div>
      {#if rows.length > 0 && !allComplete}<p class="px-3 py-1.5 text-[11.5px] flex items-center gap-1.5 text-[var(--color-warning-600,#d97706)]"><i class="icon-triangle-alert text-[13px]"></i>{t('purchases.form.needLines')}</p>{/if}
    </section>

    <section class="surface-card !p-3">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 mb-2">
        <h2 class="font-display font-bold text-[12px] uppercase tracking-wide inline-flex items-center gap-1.5"><i class="icon-coins text-[14px]"></i>{t('purchases.form.costs')}</h2>
        <p class="text-[11px] text-[var(--text-tertiary)] grow">{t('purchases.form.costsShare')}</p>
        <button type="button" class="btn btn-sm !h-7" onclick={() => costs.push({ key: crypto.randomUUID(), name: '', amount: '' })}><i class="icon-plus text-[13px]"></i>{t('purchases.form.addCost')}</button>
      </div>
      {#if costs.length > 0}
        <div class="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          {#each costs as c, i (c.key)}
            <div>
              <div class="flex items-center gap-1.5">
                <input class="{inputClass} grow" placeholder={t('purchases.form.costName')} aria-label={t('purchases.form.costName')} maxlength="80" bind:value={c.name} />
                <MoneyInput class="{inputClass} !w-32 text-end" decimals={2} bind:value={c.amount} aria-label={t('purchases.form.costAmount')} placeholder={t('purchases.form.costAmount')} />
                <button type="button" class="header-icon-btn !size-7 hover:!text-[var(--color-danger-600)]" aria-label={t('purchases.form.remove')} onclick={() => costs.splice(i, 1)}><i class="icon-trash-2 text-[14px]"></i></button>
              </div>
              {#if fieldErrors[`other_costs.${i}.amount`]}<p class={errClass}>{fieldErrors[`other_costs.${i}.amount`]}</p>{/if}
            </div>
          {/each}
        </div>
      {/if}
    </section>

    <div class="sticky bottom-0 z-10 surface-card !py-2.5 !px-4 flex flex-wrap items-center gap-x-6 gap-y-2 shadow-[0_-6px_16px_rgba(0,0,0,.08)]" aria-busy={quoting}>
      <div class="inline-flex items-center gap-2 text-[12px] text-[var(--text-secondary)]">
        <i class="icon-layers text-[15px]"></i>
        <span>{t('purchases.form.entries', { count: rows.length })} · {formatNumber(rows.reduce((a, r) => a + num(r.qd) + num(r.qw), 0), { maximumFractionDigits: 3 })} {t('purchases.form.qtyShort')}</span>
      </div>
      <dl class="ms-auto grid gap-0.5 text-[12.5px] tabular-nums" style="min-width:min(100%,300px)">
        <div class="flex items-baseline justify-between gap-6"><dt class="text-[var(--text-tertiary)]">{t('purchases.totals.subtotal')}</dt><dd class="font-medium">{quote ? money(quote.subtotal) : '—'}</dd></div>
        <div class="flex items-baseline justify-between gap-6"><dt class="text-[var(--text-tertiary)]">{t('purchases.totals.tax', { pct: formatNumber(num(taxPct)) })}</dt><dd class="font-medium">{quote ? money(quote.tax_amount) : '—'}</dd></div>
        {#if quote && Number(quote.other_cost) > 0}
          <div class="flex items-baseline justify-between gap-6"><dt class="text-[var(--text-tertiary)]">{t('purchases.totals.otherCosts')}</dt><dd class="font-medium">{money(quote.other_cost)}</dd></div>
        {/if}
        <div class="flex items-baseline justify-between gap-6 border-t border-[var(--border-subtle)] pt-1 font-display font-bold text-[18px] leading-tight"><dt class="text-[12px] uppercase tracking-wide text-[var(--text-tertiary)] font-semibold">{t('purchases.totals.total')}</dt><dd>{quote ? money(quote.total) : quoting ? t('purchases.form.totalsPending') : '—'}</dd></div>
      </dl>
      <button type="submit" class="btn btn-primary !h-10 px-6" disabled={!canSave}><i class="icon-save text-[15px]"></i>{busy ? t('purchases.form.saving') : t('purchases.form.save')}</button>
    </div>
  </form>
</main>

{#if viewId}
  <PurchaseModal id={viewId} onclose={() => (viewId = null)} />
{/if}

<style>
  /* Kolom nama menempel: latar WAJIB solid (bukan transparan) supaya isi kolom lain tidak tembus saat tabel digulir. */
  tbody tr td.stickyc { background: var(--surface-base); box-shadow: 1px 0 0 var(--border-subtle); }
  tbody tr:nth-child(even) td.stickyc { background: var(--surface-sunken); }
  tbody tr:hover td.stickyc { background: color-mix(in srgb, var(--color-primary) 6%, var(--surface-base)); }
  .delivery .road {
    position: absolute;
    inset-inline: 0;
    bottom: 10px;
    height: 2px;
    background: repeating-linear-gradient(90deg, color-mix(in srgb, var(--text-tertiary) 60%, transparent) 0 8px, transparent 8px 14px);
    animation: road-scroll 0.7s linear infinite;
  }
  .delivery .truck {
    position: absolute;
    bottom: 12px;
    inset-inline-start: 0;
    animation: truck-drive 5.5s ease-in-out infinite;
  }
  .delivery .box {
    position: absolute;
    color: var(--text-tertiary);
    animation: box-float 2.4s ease-in-out infinite;
  }
  .delivery .box1 { top: 8px; inset-inline-end: 18%; }
  .delivery .box2 { top: 20px; inset-inline-end: 8%; animation-delay: 0.7s; }
  @keyframes truck-drive {
    0% { transform: translateX(-40px); }
    55% { transform: translateX(calc(50cqw)); }
    100% { transform: translateX(calc(100cqw + 10px)); }
  }
  @keyframes road-scroll { to { background-position-x: -14px; } }
  @keyframes box-float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-4px); } }
  .delivery { container-type: inline-size; }
  @media (prefers-reduced-motion: reduce) {
    .delivery .truck, .delivery .box, .delivery .road { animation: none; }
    .delivery .truck { inset-inline-start: 40%; }
  }
</style>
