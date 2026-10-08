<script lang="ts" module>
  export type UnitRow = { unitId: string; unitLabel: string; factor: string; barcode: string; price: string };
  export const MAX_UNITS = 5;
</script>

<script lang="ts">
  // Satuan tambahan item: 1 satuan ini = `factor` satuan dasar; barcode dan harga per satuan opsional
  // (harga kosong = factor × harga satuan dasar). Server otoritatif atas semua aturan.
  import { lookup } from '#lib/catalog/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import Combobox from '#lib/components/Combobox.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';

  let { rows = $bindable([]), baseUnit = '', readOnly = false }: { rows?: UnitRow[]; baseUnit?: string; readOnly?: boolean } = $props();

  const searchUnits = (q: string) => lookup('units').search(q);
  const cell = 'w-full field-control';
  const label = 'text-[11px] font-semibold uppercase tracking-wide mb-1 block text-[var(--text-tertiary)]';

  const add = () => rows.length < MAX_UNITS && rows.push({ unitId: '', unitLabel: '', factor: '', barcode: '', price: '' });
</script>

<div class="space-y-3">
  <p class="text-[11px] text-[var(--text-tertiary)] max-w-2xl">{t('items.units.hint')}</p>

  {#if rows.length === 0}
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('items.units.empty')}</p>
  {/if}

  {#each rows as row, i (i)}
    <div class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-2.5">
      <div class="grid gap-2.5 sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <label for="u-unit-{i}" class={label}>{t('items.units.unit')}</label>
          <Combobox id="u-unit-{i}" bind:value={row.unitId} bind:label={row.unitLabel} search={searchUnits} placeholder={t('items.units.unit')} clearable={false} disabled={readOnly} />
        </div>
        <div>
          <label for="u-factor-{i}" class={label}>{t('items.units.factor')}</label>
          <MoneyInput id="u-factor-{i}" class={cell} bind:value={row.factor} decimals={6} pad={false} placeholder="12" disabled={readOnly} />
        </div>
        <div>
          <label for="u-barcode-{i}" class={label}>{t('items.units.barcode')}</label>
          <input id="u-barcode-{i}" class="{cell} font-mono" bind:value={row.barcode} maxlength="200" autocomplete="off" disabled={readOnly} />
        </div>
        <div>
          <label for="u-price-{i}" class={label}>{t('items.units.price')}</label>
          <MoneyInput id="u-price-{i}" class={cell} bind:value={row.price} placeholder={t('items.units.pricePlaceholder')} disabled={readOnly} />
        </div>
      </div>
      <div class="flex items-center justify-between gap-2 text-[11.5px] text-[var(--text-tertiary)]">
        <span>
          {#if row.unitLabel && row.factor.trim()}{t('items.units.equals', { unit: row.unitLabel, factor: row.factor.trim(), base: baseUnit || t('items.units.base') })}{/if}
          {#if !row.price.trim()} · {t('items.units.priceHint')}{/if}
        </span>
        {#if !readOnly}
          <button type="button" class="text-[11.5px] font-medium text-[var(--color-danger-600)] hover:underline" onclick={() => rows.splice(i, 1)}>{t('items.units.remove')}</button>
        {/if}
      </div>
    </div>
  {/each}

  {#if !readOnly}
    <button type="button" class="btn !text-[12px]" disabled={rows.length >= MAX_UNITS} onclick={add}><i class="icon-plus text-[12px]"></i>{t('items.units.add')}</button>
    {#if rows.length >= MAX_UNITS}<span class="text-[11px] ms-2 text-[var(--text-tertiary)]">{t('items.units.maxUnits', { max: MAX_UNITS })}</span>{/if}
  {/if}
</div>
