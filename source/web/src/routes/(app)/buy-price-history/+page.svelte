<script lang="ts">
  import { onMount } from 'svelte';
  import { buyPriceHistory, type BuyPriceRow } from '#lib/purchases/api.ts';
  import { persistPage } from '#lib/tabs/persist.ts';
  import { t, formatDate, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';

  let from = $state('');
  let to = $state('');
  let q = $state('');

  // Filter tetap ada saat pindah tab lalu kembali (lib/tabs/persist.ts).
  persistPage(
    () => ({ from, to, q }),
    (s) => ({ from, to, q } = s)
  );

  let rows = $state<BuyPriceRow[]>([]);
  let next = $state('');
  let loading = $state(true);
  let error = $state('');
  let seq = 0; // abaikan respons basi bila filter berubah saat memuat

  const money = (v: string | number) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 0, maximumFractionDigits: 4 });

  async function load(more = false) {
    const my = ++seq;
    loading = true;
    error = '';
    try {
      const r = await buyPriceHistory.list({ from, to, q: q.trim(), cursor: more ? next : '', limit: 50 });
      if (my !== seq) return;
      rows = more ? [...rows, ...r.data] : r.data;
      next = r.next_cursor;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      if (!more) {
        rows = [];
        next = '';
      }
    } finally {
      if (my === seq) loading = false;
    }
  }

  // Pencarian menunggu pengetikan berhenti. (Tanggal lewat onchange DateRange/preset.)
  let first = true;
  $effect(() => {
    const query = q;
    void query;
    const delay = first ? 0 : 350;
    first = false;
    const h = setTimeout(() => void load(), delay);
    return () => clearTimeout(h);
  });

  onMount(() => {
    document.title = t('payables.history.docTitle');
  });

  // ---- Preset periode (hari kalender lokal) ----
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const ago = (days: number) => {
    const d = new Date();
    d.setDate(d.getDate() - days);
    return ymd(d);
  };
  const presets = $derived.by(() => {
    const now = new Date();
    return [
      { id: 'today' as const, from: ymd(now), to: ymd(now) },
      { id: 'days7' as const, from: ago(6), to: ymd(now) },
      { id: 'days30' as const, from: ago(29), to: ymd(now) },
      { id: 'month' as const, from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: ymd(now) }
    ];
  });
  function applyPreset(p: { from: string; to: string }) {
    from = p.from;
    to = p.to;
    void load();
  }

  // Selisih harga terhadap pembelian sebelumnya: naik = merah (lebih mahal), turun = hijau.
  function diff(r: BuyPriceRow): { pct: number; cls: string } | null {
    if (r.prev_price === undefined) return null;
    const prev = Number(r.prev_price);
    const cur = Number(r.unit_price);
    if (!prev || prev === cur) return { pct: 0, cls: 'text-[var(--text-tertiary)]' };
    return { pct: ((cur - prev) / prev) * 100, cls: cur > prev ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]' };
  }
  const pctText = (p: number) => `${p > 0 ? '+' : ''}${p.toFixed(1)}%`;
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1600px] mx-auto w-full">
  <div class="max-w-3xl">
    <h1 class="font-display font-bold text-[19px]">{t('payables.history.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('payables.history.subtitle')}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-[minmax(0,2fr)_minmax(0,1.5fr)]">
      <div class="relative">
        <i class="icon-search text-[13px] absolute start-3 bottom-[13px] text-[var(--text-tertiary)]"></i>
        <input class="form-input w-full" style="padding-inline-start:2rem" type="search" bind:value={q} placeholder={t('payables.history.search')} aria-label={t('payables.history.search')} maxlength="100" />
      </div>
      <div>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('payables.history.period')} />
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      <i class="icon-calendar text-[13px] text-[var(--text-tertiary)] me-0.5"></i>
      {#each presets as p (p.id)}
        {@const on = from === p.from && to === p.to}
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors {on
            ? 'border-[var(--color-primary-600)] bg-[var(--color-primary-600)] text-white'
            : 'border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)]'}"
          aria-pressed={on}
          onclick={() => applyPreset(p)}
        >{t(`payables.history.preset.${p.id}`)}</button>
      {/each}
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('payables.history.loadFailed')} {error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full min-w-[980px] text-[12.5px]">
        <thead class="bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('payables.history.col.date')}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t('payables.history.col.item')}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t('payables.history.col.supplier')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('payables.history.col.qty')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('payables.history.col.price')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('payables.history.col.change')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('payables.history.col.discount')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('payables.history.col.cost')}</th>
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('payables.history.col.doc')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.purchase_id + ':' + r.position)}
            {@const d = diff(r)}
            <tr class="border-t border-[var(--border-subtle)] align-top transition-colors hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-2.5 whitespace-nowrap">{formatDate(r.purchase_date)}</td>
              <td class="px-3 py-2.5">
                <a href="/items/{r.item_id}" class="font-semibold text-[var(--color-primary-600)]">{r.name}</a>
                <div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.sku}</div>
              </td>
              <td class="px-3 py-2.5">{r.supplier_name}<div class="text-[11px] text-[var(--text-tertiary)]">{r.outlet_name}</div></td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">{r.qty} {r.unit}</td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap font-semibold">{money(r.unit_price)}</td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap">
                {#if d}
                  <span class="font-medium {d.cls}">{pctText(d.pct)}</span>
                  <div class="text-[11px] text-[var(--text-tertiary)]">{money(r.prev_price ?? 0)}</div>
                {:else}
                  <span class="badge-soft badge-info">{t('payables.history.first')}</span>
                {/if}
              </td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap text-[var(--text-secondary)]">{r.discounts.length ? r.discounts.map((x) => (Number(x) >= 100 ? money(x) : `${Number(x)}%`)).join(' + ') : '—'}</td>
              <td class="px-3 py-2.5 text-end tabular-nums whitespace-nowrap text-[var(--text-secondary)]">{money(r.unit_cost)}</td>
              <td class="px-4 py-2.5 font-mono text-[12px] whitespace-nowrap">{r.doc_no}</td>
            </tr>
          {:else}
            <tr>
              <td colspan="9" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('payables.history.empty')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if next}
      <div class="p-3 border-t border-[var(--border-subtle)] text-center bg-[var(--surface-sunken)]">
        <button type="button" class="btn !text-[12px]" disabled={loading} onclick={() => load(true)}>
          <i class="icon-chevrons-down text-[13px]"></i>{t('payables.history.loadMore')}
        </button>
      </div>
    {/if}
  </section>
  {#if rows.length > 0}
    <p class="text-[11.5px] text-center text-[var(--text-tertiary)]">{t('payables.history.count', { count: rows.length })}</p>
  {/if}
</main>
