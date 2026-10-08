<script lang="ts">
  import { onMount } from 'svelte';
  import { conversions, type Conversion, type OpeningRow } from '#lib/stock/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime, formatNumber, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import MarkdownEditor from '#lib/components/MarkdownEditor.svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';

  const PAGE = 20;

  // Hasil pencarian terakhir per id: dipakai menampilkan satuan & stok barang yang dipilih.
  let known = $state<Record<string, OpeningRow>>({});
  let fromId = $state('');
  let fromLabel = $state('');
  let toId = $state('');
  let toLabel = $state('');
  let fromQty = $state('');
  let toQty = $state('');
  let note = $state('');

  let busy = $state(false);
  let formError = $state('');
  let fieldErrors = $state<Record<string, string>>({});
  let notice = $state('');
  let loadError = $state('');

  let rows = $state<Conversion[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);

  const from = $derived(fromId ? known[fromId] : undefined);
  const to = $derived(toId ? known[toId] : undefined);
  const num = (s: string) => Number(s.replace(',', '.'));
  const ratio = $derived(num(fromQty) > 0 && num(toQty) > 0 ? num(toQty) / num(fromQty) : 0);
  const canCreate = $derived(can('stock_conversion', 'create'));
  const qty = (s: string) => formatNumber(num(s), { maximumFractionDigits: 3 });

  async function search(q: string): Promise<Option[]> {
    const res = await conversions.items(q.trim());
    for (const r of res.data) known[r.id] = r;
    return res.data.map((r) => ({ id: r.id, name: `${r.sku} — ${r.name}` }));
  }

  async function loadHistory() {
    loading = true;
    loadError = '';
    try {
      const res = await conversions.list(PAGE, offset);
      rows = res.data;
      total = res.total;
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  onMount(loadHistory);

  function go(next: number) {
    offset = Math.max(0, next);
    void loadHistory();
  }

  // Kunci idempotensi per isi permintaan: klik ganda / jaringan putus memakai kunci yang sama (tidak membuat dokumen ganda);
  // isi diubah → kunci baru.
  let attempt: { sig: string; key: string } | null = null;
  function keyFor(sig: string): string {
    if (!attempt || attempt.sig !== sig) attempt = { sig, key: `ps-${crypto.randomUUID()}` };
    return attempt.key;
  }

  // Muat ulang stok kedua barang setelah dokumen dibuat (daftar `known` bisa basi).
  async function refreshStock() {
    for (const r of [from, to]) {
      if (!r) continue;
      try {
        const res = await conversions.items(r.sku, 5);
        for (const x of res.data) known[x.id] = x;
      } catch {
        /* stok di layar hanya petunjuk; server tetap otoritatif */
      }
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    formError = '';
    fieldErrors = {};
    const input = { from_item_id: fromId, from_qty: fromQty.trim().replace(',', '.'), to_item_id: toId, to_qty: toQty.trim().replace(',', '.'), ...(note.trim() ? { note } : {}) };
    try {
      const doc = await conversions.create(input, keyFor(JSON.stringify(input)));
      notice = t('stock.conversion.form.done', { doc: doc.doc_no });
      fromQty = '';
      toQty = '';
      note = '';
      attempt = null;
      offset = 0;
      await Promise.all([loadHistory(), refreshStock()]);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        fieldErrors = Object.fromEntries(Object.entries(err.fields).map(([k, v]) => [k, fieldMessage(v) ?? errorMessage(err)]));
      } else {
        formError = errorMessage(err);
      }
    } finally {
      busy = false;
    }
  }

  const label = 'text-[11.5px] font-semibold uppercase tracking-wide block text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] text-[var(--color-danger-600)]';
</script>

<svelte:head><title>{t('stock.conversion.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.conversion.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.conversion.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-5 max-w-[1200px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('stock.conversion.title')}</h1>
    <p class="text-[12.5px] mt-1 leading-relaxed text-[var(--text-tertiary)]">{t('stock.conversion.subtitle')}</p>
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}

  {#if canCreate}
    <form class="surface-card !p-4 lg:!p-5 space-y-5" onsubmit={submit} novalidate>
      <h3 class="font-display font-bold text-[14px]">{t('stock.conversion.form.title')}</h3>

      <div class="grid gap-3 md:grid-cols-[1fr_auto_1fr] md:items-stretch">
        {#each ['from', 'to'] as side (side)}
          {@const item = side === 'from' ? from : to}
          {@const itemErr = fieldErrors[`${side}_item_id`]}
          {@const qtyErr = fieldErrors[`${side}_qty`]}
          {#if side === 'to'}
            <div class="flex items-center justify-center text-[var(--text-tertiary)]" aria-hidden="true">
              <i class="icon-arrow-down md:hidden text-[18px]"></i><i class="icon-arrow-right hidden md:inline text-[20px]"></i>
            </div>
          {/if}
          <section class="rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-4 space-y-3 min-w-0">
            <label for="conv-{side}" class="{label} flex items-center gap-1.5">
              <i class="{side === 'from' ? 'icon-circle-minus text-[var(--color-danger-600)]' : 'icon-circle-plus text-[var(--color-success-600,#16a34a)]'} text-[14px]"></i>
              {side === 'from' ? t('stock.conversion.form.from') : t('stock.conversion.form.to')}
            </label>
            {#if side === 'from'}
              <Combobox id="conv-from" bind:value={fromId} bind:label={fromLabel} {search} placeholder={t('stock.conversion.form.pick')} invalid={!!itemErr} />
            {:else}
              <Combobox id="conv-to" bind:value={toId} bind:label={toLabel} {search} placeholder={t('stock.conversion.form.pick')} invalid={!!itemErr} />
            {/if}
            {#if itemErr}<p class={errClass} role="alert">{itemErr}</p>{/if}

            <div class="flex items-center gap-2">
              {#if side === 'from'}
                <MoneyInput class="field-control !text-end grow min-w-0 {qtyErr ? '!border-[var(--color-danger-600)]' : ''}" decimals={3} pad={false} aria-label="{t('stock.conversion.form.from')} – {t('stock.conversion.form.qty')}" placeholder={t('stock.conversion.form.qty')} bind:value={fromQty} />
              {:else}
                <MoneyInput class="field-control !text-end grow min-w-0 {qtyErr ? '!border-[var(--color-danger-600)]' : ''}" decimals={3} pad={false} aria-label="{t('stock.conversion.form.to')} – {t('stock.conversion.form.qty')}" placeholder={t('stock.conversion.form.qty')} bind:value={toQty} />
              {/if}
              <span class="w-14 shrink-0 text-[12.5px] font-semibold truncate" title={item?.unit}>{item?.unit ?? '—'}</span>
            </div>
            {#if qtyErr}<p class={errClass} role="alert">{qtyErr}</p>{/if}

            <p class="text-[11.5px] min-h-[1.1rem] text-[var(--text-tertiary)]">
              {#if item}
                {t('stock.conversion.form.stock', { qty: qty(item.display), unit: item.unit })}
              {/if}
            </p>
          </section>
        {/each}
      </div>

      {#if from && to && ratio > 0}
        {@const fromAfter = num(from.display) - num(fromQty)}
        {@const toAfter = num(to.display) + num(toQty)}
        {@const n3 = (v: number) => formatNumber(v, { maximumFractionDigits: 3 })}
        <section class="rounded-xl border border-[var(--border-subtle)] p-4 space-y-3 text-[12.5px] leading-relaxed" aria-label={t('stock.conversion.form.summary.title')}>
          <h4 class="font-display font-bold text-[13px]">{t('stock.conversion.form.summary.title')}</h4>
          <p>
            {t('stock.conversion.form.summary.meaning', {
              fromQty: n3(num(fromQty)), fromUnit: from.unit, fromName: from.name,
              toQty: n3(num(toQty)), toUnit: to.unit, toName: to.name,
              ratio: formatNumber(ratio, { maximumFractionDigits: 4 })
            })}
          </p>
          <ul class="space-y-1.5">
            <li class="flex flex-wrap items-baseline justify-between gap-x-3 rounded-lg px-3 py-2 bg-[var(--surface-sunken)]">
              <span><span class="font-semibold text-[var(--color-danger-600)]">−</span> {t('stock.conversion.form.summary.down', { name: from.name, qty: n3(num(fromQty)), unit: from.unit })}</span>
              <span class="font-semibold tabular-nums">{t('stock.conversion.form.summary.change', { before: n3(num(from.display)), after: n3(fromAfter), unit: from.unit })}</span>
            </li>
            <li class="flex flex-wrap items-baseline justify-between gap-x-3 rounded-lg px-3 py-2 bg-[var(--surface-sunken)]">
              <span><span class="font-semibold text-[var(--color-success-600,#16a34a)]">+</span> {t('stock.conversion.form.summary.up', { name: to.name, qty: n3(num(toQty)), unit: to.unit })}</span>
              <span class="font-semibold tabular-nums">{t('stock.conversion.form.summary.change', { before: n3(num(to.display)), after: n3(toAfter), unit: to.unit })}</span>
            </li>
          </ul>
          <div class="space-y-1.5 text-[12px]">
            {#if fromAfter < 0}
              <p class="flex items-start gap-2 rounded-lg px-3 py-2 badge-warning"><i class="icon-triangle-alert text-[14px] mt-px shrink-0"></i><span>{t('stock.conversion.form.summary.minus', { name: from.name })}</span></p>
            {/if}
            <p class="text-[var(--text-tertiary)]">
              {num(to.display) <= 0 ? t('stock.conversion.form.summary.costNew', { name: to.name }) : t('stock.conversion.form.summary.costKept', { name: to.name })}
            </p>
            <p class="text-[var(--text-tertiary)]">{t('stock.conversion.form.summary.final')}</p>
          </div>
        </section>
      {/if}
      <p class="text-[11.5px] leading-relaxed text-[var(--text-tertiary)]">{t('stock.conversion.form.hint')}</p>

      <div class="space-y-1.5">
        <label for="conv-note" class={label}>{t('stock.conversion.form.note')}</label>
        <MarkdownEditor id="conv-note" bind:value={note} rows={4} maxlength={1000} invalid={!!fieldErrors.note} />
        {#if fieldErrors.note}<p class={errClass} role="alert">{fieldErrors.note}</p>{/if}
      </div>

      {#if formError}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{formError}</span>
        </div>
      {/if}

      <div class="flex justify-end pt-1">
        <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy || !fromId || !toId || !fromQty || !toQty}>
          <i class="icon-split text-[13px]"></i>{busy ? t('stock.conversion.form.submitting') : t('stock.conversion.form.submit')}
        </button>
      </div>
    </form>
  {/if}

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.conversion.form.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <h3 class="font-display font-bold text-[14px] px-4 py-3 border-b border-[var(--border-subtle)]">{t('stock.conversion.history.title')}</h3>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[720px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.conversion.history.doc')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.conversion.history.move')}</th>
            <th class="px-4 py-2.5 text-end font-semibold" scope="col">{t('stock.conversion.history.cost')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.conversion.history.by')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] align-top">
              <td class="px-4 py-3 whitespace-nowrap">
                <div class="font-mono text-[12px] font-semibold">{r.doc_no}</div>
                <div class="text-[11.5px] text-[var(--text-tertiary)]">{formatDateTime(new Date(r.created_at))}</div>
              </td>
              <td class="px-4 py-3 min-w-0">
                <div class="flex items-baseline gap-1.5"><span class="text-[var(--color-danger-600)] font-semibold">−{qty(r.from.qty)} {r.from.unit}</span><span class="truncate">{r.from.name}</span></div>
                <div class="flex items-baseline gap-1.5"><span class="text-[var(--color-success-600,#16a34a)] font-semibold">+{qty(r.to.qty)} {r.to.unit}</span><span class="truncate">{r.to.name}</span></div>
                {#if r.note}
                  <MarkdownView source={r.note} class="mt-1.5 text-[12px] text-[var(--text-tertiary)] border-s-2 border-[var(--border-subtle)] ps-2.5" />
                {/if}
              </td>
              <td class="px-4 py-3 text-end whitespace-nowrap">
                <div class="font-semibold">{formatCurrency(num(r.to_unit_cost))} <span class="font-normal text-[var(--text-tertiary)]">/ {r.to.unit}</span></div>
                <div class="text-[11px] text-[var(--text-tertiary)]">{r.cost_applied ? t('stock.conversion.history.costApplied') : t('stock.conversion.history.costKept')}</div>
              </td>
              <td class="px-4 py-3">{r.actor}</td>
            </tr>
          {:else}
            <tr><td colspan="4" class="px-4 py-8 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('stock.conversion.history.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if total > PAGE}
      <div class="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
        <span>{t('stock.opening.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('stock.opening.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('stock.opening.next')}</button>
        </div>
      </div>
    {/if}
  </div>
</main>
