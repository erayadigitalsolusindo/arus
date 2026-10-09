<script lang="ts">
  import { tick } from 'svelte';
  import { counts, BUCKETS, type Bucket, type OpeningRow, type QuickMode } from '#lib/stock/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import Select from '#lib/components/Select.svelte';

  type Row = { item: OpeningRow; draft: string };

  let mode = $state<QuickMode>('replace');
  let bucket = $state<Bucket>('display');
  let note = $state('');
  let rows = $state<Row[]>([]);
  let busy = $state(false);
  let error = $state('');
  let doneDoc = $state<{ id: string; doc_no: string } | null>(null);
  let notice = $state('');

  const canApply = $derived(can('stock_opname', 'approve'));

  // ---- Pemilih barang ----
  const cache = new Map<string, OpeningRow>();
  let pickId = $state('');
  let pickLabel = $state('');

  async function searchItems(term: string): Promise<Option[]> {
    const res = await counts.items(term.trim());
    return res.data.map((r) => {
      cache.set(r.id, r);
      return { id: r.id, name: `${r.sku} — ${r.name}` };
    });
  }

  $effect(() => {
    if (!pickId) return;
    const id = pickId;
    pickId = '';
    pickLabel = '';
    const item = cache.get(id);
    if (!item) return;
    if (rows.some((r) => r.item.id === id)) {
      notice = t('stock.count.quick.duplicate');
      return;
    }
    notice = '';
    doneDoc = null;
    rows = [...rows, { item, draft: '' }];
    void tick().then(() => document.getElementById(`qc-${id}`)?.focus());
  });

  // ---- Perhitungan pratinjau (server tetap otoritatif) ----
  const round3 = (n: number) => Math.round(n * 1000) / 1000;
  const sysOf = (r: Row) => Number(r.item[bucket]);
  const valid = (s: string) => (mode === 'replace' ? /^\d+([.,]\d{1,3})?$/ : /^[+-]?\d+([.,]\d{1,3})?$/).test(s.trim());
  const valueOf = (r: Row) => Number(r.draft.trim().replace(',', '.'));
  const calc = (r: Row) => {
    if (!valid(r.draft)) return null;
    const v = valueOf(r);
    if (mode === 'adjust' && v === 0) return null;
    const sys = sysOf(r);
    const result = mode === 'replace' ? v : round3(sys + v);
    return { sys, result, delta: round3(result - sys) };
  };
  const qty = (n: number) => formatNumber(n, { maximumFractionDigits: 3 });
  const sign = (n: number) => (n > 0 ? '+' : '');

  const stats = $derived.by(() => {
    let ok = 0, changed = 0;
    for (const r of rows) {
      const c = calc(r);
      if (c) {
        ok++;
        if (c.delta !== 0) changed++;
      }
    }
    return { total: rows.length, ok, changed, ready: rows.length > 0 && ok === rows.length };
  });

  async function apply() {
    if (busy || !stats.ready || !canApply) return;
    busy = true;
    error = '';
    notice = '';
    try {
      const d = await counts.quick({
        bucket,
        mode,
        note: note.trim(),
        items: rows.map((r) => ({ item_id: r.item.id, qty: r.draft.trim().replace(',', '.').replace(/^\+/, '') }))
      });
      doneDoc = { id: d.id, doc_no: d.doc_no };
      rows = [];
      note = '';
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  function remove(id: string) {
    rows = rows.filter((r) => r.item.id !== id);
  }

  function onEnter(e: KeyboardEvent, idx: number) {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const next = rows[idx + 1];
    if (next) document.getElementById(`qc-${next.item.id}`)?.focus();
    else document.getElementById('qc-search')?.focus();
  }

  const label = 'text-[11px] font-semibold uppercase tracking-wide block text-[var(--text-tertiary)]';
  const modes = [
    { k: 'replace', icon: 'icon-replace' },
    { k: 'adjust', icon: 'icon-diff' }
  ] as const;
</script>

<svelte:head><title>{t('stock.count.quick.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.count.quick.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <a href="/stock-opname" class="text-[var(--color-primary-600)]">{t('stock.count.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.count.quick.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-3 max-w-[1400px] mx-auto w-full pb-4">
  {#if !canApply}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-warning"><i class="icon-lock text-[14px] shrink-0"></i><span>{t('stock.count.quick.needApprove')}</span></div>
  {/if}
  {#if doneDoc}
    <div role="status" class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg px-3 py-2 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i>
      <span>{t('stock.count.quick.done', { doc: doneDoc.doc_no })}</span>
      <a href="/stock-opname/{doneDoc.id}" class="font-semibold underline ms-auto">{t('stock.count.quick.viewDoc')}</a>
    </div>
  {/if}
  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (error = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}

  <!-- Pengaturan: satu baris -->
  <section class="surface-card !p-3.5">
    <div class="flex flex-wrap items-end gap-x-4 gap-y-3">
      <div>
        <p class={label}>{t('stock.count.quick.modeTitle')}</p>
        <div class="mt-1 inline-flex rounded-lg border border-[var(--border-subtle)] p-0.5 bg-[var(--surface-sunken)]" role="radiogroup" aria-label={t('stock.count.quick.modeTitle')}>
          {#each modes as m (m.k)}
            {@const active = mode === m.k}
            <button
              type="button"
              role="radio"
              aria-checked={active}
              disabled={rows.length > 0 && !active}
              title={rows.length > 0 && !active ? t('stock.count.quick.modeLock') : undefined}
              class="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-[12.5px] font-semibold transition disabled:opacity-50 disabled:cursor-not-allowed {active ? 'bg-[var(--color-primary-600)] text-white shadow-sm' : 'text-[var(--text-tertiary)] hover:text-[inherit]'}"
              onclick={() => (mode = m.k)}
            >
              <i class="{m.icon} text-[13px]"></i>{t(`stock.count.quick.${m.k}Title`)}
            </button>
          {/each}
        </div>
      </div>
      <div class="w-40">
        <label for="qc-bucket" class={label}>{t('stock.count.quick.bucket')}</label>
        <div class="mt-1"><Select id="qc-bucket" bind:value={bucket} options={BUCKETS.map((b) => ({ value: b, label: t(`stock.count.bucket.${b}`) }))} /></div>
      </div>
      <div class="grow basis-64 min-w-0">
        <label for="qc-note" class={label}>{t('stock.count.quick.note')}</label>
        <input id="qc-note" class="mt-1 field-control w-full" maxlength="1000" placeholder={t('stock.count.quick.notePlaceholder')} bind:value={note} />
      </div>
      {#if session.outlet}<span class="badge-soft badge-neutral inline-flex items-center gap-1 mb-1.5"><i class="icon-store text-[11px]"></i>{session.outlet.name}</span>{/if}
    </div>
    <p class="mt-2.5 flex items-start gap-1.5 text-[11.5px] text-[var(--text-tertiary)]"><i class="icon-info text-[12px] mt-px shrink-0"></i>{t(`stock.count.quick.${mode}Desc`)}</p>
  </section>

  <!-- Barang -->
  <section class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2 px-3.5 py-2.5 border-b border-[var(--border-subtle)]">
      <div class="grow basis-72 max-w-xl"><Combobox id="qc-search" bind:value={pickId} bind:label={pickLabel} search={searchItems} placeholder={t('stock.count.quick.addItem')} /></div>
      {#if notice}<span class="text-[12px] text-[var(--color-warning-600,#b45309)]">{notice}</span>{/if}
      {#if rows.length > 0}
        <button type="button" class="ms-auto text-[12px] text-[var(--color-danger-600)] inline-flex items-center gap-1" onclick={() => (rows = [])}><i class="icon-trash-2 text-[12px]"></i>{t('stock.count.quick.clearAll')}</button>
      {/if}
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[700px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="px-3.5 py-2 text-start font-semibold" scope="col">{t('stock.count.quick.colItem')}</th>
            <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.count.quick.colSystem')}</th>
            <th class="px-3 py-2 text-end font-semibold" scope="col">{mode === 'replace' ? t('stock.count.quick.inputReplace') : t('stock.count.quick.inputAdjust')}</th>
            <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.count.quick.colResult')}</th>
            <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.count.quick.colChange')}</th>
            <th class="px-2 py-2 w-8" scope="col"><span class="sr-only">{t('stock.count.quick.remove')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r, i (r.item.id)}
            {@const c = calc(r)}
            {@const bad = r.draft.trim() !== '' && c == null}
            <tr class="border-t border-[var(--border-subtle)] align-middle">
              <td class="px-3.5 py-1.5">
                <span class="font-semibold">{r.item.name}</span>
                <span class="ms-2 font-mono text-[11px] text-[var(--text-tertiary)]">{r.item.sku} · {r.item.unit}</span>
              </td>
              <td class="px-3 py-1.5 text-end tabular-nums whitespace-nowrap">{qty(sysOf(r))}</td>
              <td class="px-3 py-1 text-end">
                <input
                  id="qc-{r.item.id}"
                  class="field-control !text-end !py-1 w-24 font-semibold {bad ? '!border-[var(--color-danger-600)]' : ''}"
                  inputmode={mode === 'adjust' ? 'text' : 'decimal'}
                  autocomplete="off"
                  aria-label="{r.item.name} – {mode === 'replace' ? t('stock.count.quick.inputReplace') : t('stock.count.quick.inputAdjust')}"
                  aria-invalid={bad}
                  title={bad ? t('stock.count.quick.invalid') : undefined}
                  placeholder={mode === 'adjust' ? '+0' : '0'}
                  bind:value={r.draft}
                  onfocus={(e) => e.currentTarget.select()}
                  onkeydown={(e) => onEnter(e, i)}
                />
              </td>
              <td class="px-3 py-1.5 text-end tabular-nums whitespace-nowrap font-semibold {c && c.result < 0 ? 'text-[var(--color-danger-600)]' : ''}" title={c && c.result < 0 ? t('stock.count.quick.negative') : undefined}>
                {c ? qty(c.result) : '—'}{#if c && c.result < 0} <i class="icon-triangle-alert text-[12px]"></i>{/if}
              </td>
              <td class="px-3 py-1.5 text-end whitespace-nowrap">
                {#if !c}
                  <span class="text-[var(--text-tertiary)]">—</span>
                {:else if c.delta === 0}
                  <span class="badge-soft badge-success"><i class="icon-check text-[11px]"></i>{t('stock.count.match')}</span>
                {:else}
                  <span class="badge-soft {c.delta < 0 ? 'badge-danger' : 'badge-success'} tabular-nums"><i class="{c.delta < 0 ? 'icon-arrow-down' : 'icon-arrow-up'} text-[11px]"></i>{sign(c.delta)}{qty(c.delta)}</span>
                {/if}
              </td>
              <td class="px-2 py-1.5 text-end">
                <button type="button" class="header-icon-btn" aria-label="{t('stock.count.quick.remove')}: {r.item.name}" title={t('stock.count.quick.remove')} onclick={() => remove(r.item.id)}><i class="icon-trash-2 text-[14px]"></i></button>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="6" class="px-4 py-8 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('stock.count.quick.empty')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <!-- Bilah terapkan: menempel di bawah area konten; tanpa dialog konfirmasi -->
  <div class="sticky bottom-0 z-20 -mx-4 lg:-mx-6 -mb-4 lg:-mb-6 mt-2 border-t border-[var(--border-subtle)] bg-[var(--surface-card,var(--surface))]">
    <div class="px-4 lg:px-6 py-2.5 flex flex-wrap items-center gap-x-4 gap-y-2">
      <p class="text-[13px] font-semibold">{t('stock.count.quick.summary', { count: stats.total, changed: stats.changed })}</p>
      <p class="text-[11.5px] text-[var(--text-tertiary)] grow min-w-0 hidden md:block">{t('stock.count.quick.applyHint')}</p>
      <button type="button" class="btn btn-primary !text-[13px] ms-auto disabled:opacity-60" disabled={!stats.ready || busy || !canApply} onclick={apply}>
        <i class="icon-zap text-[14px]"></i>{busy ? t('stock.count.quick.applying') : t('stock.count.quick.apply')}
      </button>
    </div>
  </div>
</main>
