<script lang="ts" module>
  export type TierRow = { min: string; price: string };
  export type OutletTierSet = { outletId: string; name: string; tiers: TierRow[] };
  export const MAX_TIERS = 10;
</script>

<script lang="ts">
  // Editor harga grosir: satu set default (semua cabang) + set khusus cabang. Aturan (server otoritatif):
  // harga tier berlaku untuk SELURUH jumlah; batas atas tier = tier berikutnya; tier terakhir berlaku sampai stok habis;
  // set cabang menggantikan default sepenuhnya; jumlah dalam satuan dasar.
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import MoneyInput from '#lib/components/MoneyInput.svelte';

  let {
    defaultTiers = $bindable([]),
    outletSets = $bindable([]),
    outlets = [],
    baseUnit = '',
    readOnly = false
  }: {
    defaultTiers?: TierRow[];
    outletSets?: OutletTierSet[];
    /** Cabang yang boleh diatur pemanggil. */
    outlets?: { id: string; name: string }[];
    baseUnit?: string;
    readOnly?: boolean;
  } = $props();

  const free = $derived(outlets.filter((o) => !outletSets.some((s) => s.outletId === o.id)));
  let pickOutlet = $state('');

  const num = (s: string) => {
    const n = Number(s);
    return s.trim() !== '' && Number.isFinite(n) ? n : null;
  };

  /** Label rentang tiap baris dari urutan jumlah awal: "2–5", "6–10", "≥ 11". */
  function ranges(rows: TierRow[]): string[] {
    const mins = rows.map((r) => num(r.min));
    const sorted = mins.filter((m): m is number => m !== null && m > 0).sort((a, b) => a - b);
    return mins.map((m) => {
      if (m === null || m <= 0) return '';
      const next = sorted.find((x) => x > m);
      if (next === undefined) return t('items.wholesale.rangeFrom', { from: formatNumber(m) });
      const upper = Number.isInteger(m) && Number.isInteger(next) ? formatNumber(next - 1) : `<${formatNumber(next)}`;
      return t('items.wholesale.rangeBetween', { from: formatNumber(m), to: upper });
    });
  }

  function addOutletSet() {
    const o = outlets.find((x) => x.id === pickOutlet);
    if (!o) return;
    // Mulai dari salinan default supaya pengguna tinggal mengubah angkanya.
    outletSets = [...outletSets, { outletId: o.id, name: o.name, tiers: defaultTiers.map((r) => ({ ...r })) }];
    pickOutlet = '';
  }

  const addRow = (rows: TierRow[]) => rows.length < MAX_TIERS && rows.push({ min: '', price: '' });
  const removeRow = (rows: TierRow[], i: number) => rows.splice(i, 1);

  const cell = 'w-full field-control';
</script>

{#snippet editor(rows: TierRow[], idPrefix: string)}
  {@const labels = ranges(rows)}
  {#if rows.length}
    <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_5.5rem_2rem] gap-2 items-end text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">
      <span>{t('items.wholesale.minQty')}</span><span>{t('items.wholesale.price')}</span><span>{t('items.wholesale.range')}</span><span></span>
    </div>
    {#each rows as row, i (i)}
      <div class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_5.5rem_2rem] gap-2 items-center">
        <MoneyInput id="{idPrefix}-min-{i}" class={cell} bind:value={row.min} decimals={3} pad={false} aria-label={t('items.wholesale.minQty')} disabled={readOnly} />
        <MoneyInput id="{idPrefix}-price-{i}" class={cell} bind:value={row.price} aria-label={t('items.wholesale.price')} disabled={readOnly} />
        <span class="text-[12px] text-[var(--text-secondary)] whitespace-nowrap">{labels[i]}</span>
        {#if !readOnly}
          <button type="button" class="header-icon-btn !size-8" aria-label={t('items.wholesale.remove')} title={t('items.wholesale.remove')} onclick={() => removeRow(rows, i)}>
            <i class="icon-x text-[13px]"></i>
          </button>
        {:else}<span></span>{/if}
      </div>
    {/each}
  {:else}
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('items.wholesale.empty')}</p>
  {/if}
  {#if !readOnly}
    <button type="button" class="btn !text-[12px]" disabled={rows.length >= MAX_TIERS} onclick={() => addRow(rows)}>
      <i class="icon-plus text-[12px]"></i>{t('items.wholesale.add')}
    </button>
    {#if rows.length >= MAX_TIERS}<span class="text-[11px] ms-2 text-[var(--text-tertiary)]">{t('items.wholesale.maxTiers', { max: MAX_TIERS })}</span>{/if}
  {/if}
{/snippet}

<div class="space-y-4">
  <p class="text-[11px] text-[var(--text-tertiary)] max-w-2xl">
    {t('items.wholesale.hint')}
    {#if baseUnit}{t('items.wholesale.unitNote', { unit: baseUnit })}{/if}
  </p>

  <div class="space-y-2">
    <h4 class="text-[12.5px] font-semibold">{t('items.wholesale.defaultSet')}</h4>
    {@render editor(defaultTiers, 'ws-default')}
  </div>

  {#each outletSets as set, si (set.outletId)}
    <div class="space-y-2 rounded-lg border border-[var(--border-subtle)] p-3">
      <div class="flex items-center justify-between gap-2">
        <h4 class="text-[12.5px] font-semibold">{t('items.wholesale.outletSet', { name: set.name })}</h4>
        {#if !readOnly}
          <button type="button" class="text-[11.5px] font-medium text-[var(--color-primary-600)] hover:underline" onclick={() => (outletSets = outletSets.filter((_, i) => i !== si))}>
            {t('items.wholesale.removeOutlet')}
          </button>
        {/if}
      </div>
      {@render editor(set.tiers, `ws-${set.outletId}`)}
    </div>
  {/each}

  {#if !readOnly && free.length}
    <div class="flex flex-wrap items-center gap-2">
      <select class="field-control" aria-label={t('items.wholesale.addOutlet')} bind:value={pickOutlet} onchange={addOutletSet}>
        <option value="">{t('items.wholesale.addOutlet')}</option>
        {#each free as o (o.id)}<option value={o.id}>{o.name}</option>{/each}
      </select>
      <span class="text-[11px] text-[var(--text-tertiary)]">{t('items.wholesale.outletHint')}</span>
    </div>
  {/if}
</div>
