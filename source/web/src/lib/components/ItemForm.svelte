<script lang="ts">
  // Form tambah/ubah item (satu komponen untuk halaman /items/new dan /items/[id]).
  // Validasi di sini hanya untuk umpan balik cepat; server memvalidasi ulang semuanya (backend/internal/item).
  import { goto } from '$app/navigation';
  import { ApiError } from '#lib/api/client.ts';
  import { items as api, type Item, type ItemInput, type ItemKind } from '#lib/items/api.ts';
  import { lookup, type LookupKind } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import { renderMarkdown } from '#lib/markdown.ts';
  import Combobox from '#lib/components/Combobox.svelte';
  import Switch from '#lib/components/Switch.svelte';

  type Outlet = { id: string; name: string };
  let { item, outlets }: { item: Item | null; outlets: Outlet[] } = $props();

  type Field =
    | 'sku' | 'barcode' | 'name' | 'weight_grams' | 'cost' | 'sell_price' | 'unit_id'
    | 'category_id' | 'brand_id' | 'principal_id' | 'supplier_id' | 'kind' | 'description' | 'outlet_prices';

  const SKU_RE = /^[A-Za-z0-9][A-Za-z0-9._/-]{0,39}$/;
  const MONEY_RE = /^\d{1,12}(\.\d{1,2})?$/;
  const GRAMS_RE = /^\d{1,8}(\.\d{1,3})?$/;

  // Nilai awal diambil sekali dari `item` (komponen dipasang ulang lewat {#key} bila item berganti).
  /* svelte-ignore state_referenced_locally */
  const init = item;
  const canWrite = init ? can('items', 'update') : can('items', 'create');

  let sku = $state(init?.sku ?? '');
  let barcode = $state(init?.barcode ?? '');
  let name = $state(init?.name ?? '');
  let weight = $state(init ? trimZeros(init.weight_grams) : '');
  let cost = $state('');
  let sellPrice = $state(init ? trimZeros(init.sell_price) : '');
  let kind = $state<ItemKind>(init?.kind ?? 'goods');
  let active = $state(init?.active ?? true);
  let allowNegative = $state(init?.allow_negative_stock ?? false);
  let belowCost = $state(init?.sell_below_cost ?? false);
  let description = $state(init?.description ?? '');
  let tab = $state<'write' | 'preview'>('write');

  let unitId = $state(init?.unit.id ?? '');
  let unitLabel = $state(init?.unit.name ?? '');
  let categoryId = $state(init?.category?.id ?? '');
  let categoryLabel = $state(init?.category?.name ?? '');
  let brandId = $state(init?.brand?.id ?? '');
  let brandLabel = $state(init?.brand?.name ?? '');
  let principalId = $state(init?.principal?.id ?? '');
  let principalLabel = $state(init?.principal?.name ?? '');
  let supplierId = $state(init?.supplier?.id ?? '');
  let supplierLabel = $state(init?.supplier?.name ?? '');

  // Harga per cabang: satu baris per outlet yang boleh diatur; kosong = pakai harga default.
  /* svelte-ignore state_referenced_locally */
  let outletPrices = $state(
    (init ? init.outlet_prices.map((p) => ({ id: p.outlet_id, name: p.outlet_name, value: p.sell_price === null ? '' : trimZeros(String(p.sell_price)) })) : outlets.map((o) => ({ id: o.id, name: o.name, value: '' })))
  );

  let saving = $state(false);
  let error = $state('');
  let errors = $state<Partial<Record<Field, string>>>({});

  /** "1500.00" → "1500", "12.50" → "12.5" (tampilan input; server menerima kedua bentuk). */
  function trimZeros(s: string): string {
    return s.includes('.') ? s.replace(/0+$/, '').replace(/\.$/, '') : s;
  }

  const search = (kindName: LookupKind) => (q: string) => lookup(kindName).search(q);
  const searchUnits = search('units');
  const searchCategories = search('categories');
  const searchBrands = search('brands');
  const searchPrincipals = search('principals');
  const searchSuppliers = search('suppliers');

  const CONTROL = /[\p{Cc}\p{Cf}\p{Co}�]/u;

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!canWrite) return;
    error = '';
    const next: Partial<Record<Field, string>> = {};

    const n = checkName(name, 200);
    if (n.code) next.name = fieldMessage(n.code);
    const code = sku.trim();
    if (code && !SKU_RE.test(code)) next.sku = fieldMessage('INVALID');
    if (init && !code) next.sku = fieldMessage('REQUIRED');
    const bc = barcode.trim();
    if (bc && (CONTROL.test(bc) || [...bc].length > 200)) next.barcode = fieldMessage(CONTROL.test(bc) ? 'INVALID' : 'TOO_LONG');
    if (weight.trim() && !GRAMS_RE.test(weight.trim())) next.weight_grams = fieldMessage('INVALID');
    if (sellPrice.trim() && !MONEY_RE.test(sellPrice.trim())) next.sell_price = fieldMessage('INVALID');
    if (!init && cost.trim() && !MONEY_RE.test(cost.trim())) next.cost = fieldMessage('INVALID');
    if (!unitId) next.unit_id = fieldMessage('REQUIRED');

    const desc = description.replace(/\r\n?/g, '\n');
    if (CONTROL.test(desc.replace(/[\n\t]/g, ''))) next.description = fieldMessage('INVALID');
    else if ([...desc].length > 5000) next.description = fieldMessage('TOO_LONG');

    for (const p of outletPrices) {
      if (p.value.trim() && !MONEY_RE.test(p.value.trim())) {
        next.outlet_prices = fieldMessage('INVALID');
        break;
      }
    }
    errors = next;
    if (Object.values(next).some(Boolean)) return;

    const filled = outletPrices.filter((p) => p.value.trim()).map((p) => ({ outlet_id: p.id, sell_price: p.value.trim() }));
    const body: ItemInput = {
      sku: code,
      barcode: bc,
      name: n.value,
      weight_grams: weight.trim(),
      sell_price: sellPrice.trim(),
      unit_id: unitId,
      category_id: categoryId,
      brand_id: brandId,
      principal_id: principalId,
      supplier_id: supplierId,
      kind,
      allow_negative_stock: allowNegative,
      sell_below_cost: belowCost,
      description: desc
    };
    saving = true;
    try {
      if (init) {
        await api.update(init.id, { ...body, outlet_prices: filled });
        if (active !== init.active) await api.setActive(init.id, active);
        await goto('/items?notice=saved');
      } else {
        const created = await api.create({ ...body, cost: cost.trim(), ...(filled.length ? { outlet_prices: filled } : {}) });
        if (!active) await api.setActive(created.id, false);
        await goto('/items?notice=created');
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, c] of Object.entries(err.fields)) errors[k as Field] = fieldMessage(c);
      } else if (err instanceof ApiError && err.code === 'CODE_TAKEN') {
        errors.sku = errorMessage(err);
      } else if (err instanceof ApiError && err.code === 'BARCODE_TAKEN') {
        errors.barcode = errorMessage(err);
      } else {
        error = errorMessage(err);
      }
    } finally {
      saving = false;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] mt-1 text-[var(--color-danger-600)]';
  const hintClass = 'text-[11px] mt-1 text-[var(--text-tertiary)]';
  const money = (s: string) => (s.trim() ? formatCurrency(Number(s)) : '—');
</script>

<form class="space-y-4" onsubmit={save} novalidate>
  {#if error}
    <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{error}</span>
    </div>
  {/if}
  {#if !canWrite}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-info">
      <i class="icon-eye text-[14px] shrink-0"></i><span>{t('items.readOnly')}</span>
    </div>
  {/if}

  <div class="grid gap-4 xl:grid-cols-3">
    <div class="space-y-4 xl:col-span-2">
      <section class="surface-card !p-4 space-y-3.5">
        <h3 class="font-display font-bold text-[14px]">{t('items.section.basic')}</h3>
        <div class="grid gap-3.5 sm:grid-cols-2">
          <div class="sm:col-span-2">
            <label for="i-name" class={labelClass}>{t('items.field.name')}</label>
            <input id="i-name" class={inputClass} bind:value={name} maxlength="200" disabled={!canWrite} aria-invalid={!!errors.name} />
            {#if errors.name}<p class={errClass}>{errors.name}</p>{/if}
          </div>
          <div>
            <label for="i-sku" class={labelClass}>{t('items.field.sku')}</label>
            <input id="i-sku" class="{inputClass} font-mono" bind:value={sku} maxlength="40" placeholder={init ? '' : 'ITM-000001'} disabled={!canWrite} aria-invalid={!!errors.sku} />
            {#if errors.sku}<p class={errClass}>{errors.sku}</p>{:else if !init}<p class={hintClass}>{t('items.field.skuHint')}</p>{/if}
          </div>
          <div>
            <label for="i-barcode" class={labelClass}>{t('items.field.barcode')}</label>
            <input id="i-barcode" class="{inputClass} font-mono" bind:value={barcode} maxlength="200" disabled={!canWrite} aria-invalid={!!errors.barcode} autocomplete="off" />
            {#if errors.barcode}<p class={errClass}>{errors.barcode}</p>{:else}<p class={hintClass}>{t('items.field.barcodeHint')}</p>{/if}
          </div>
          <div>
            <label for="i-weight" class={labelClass}>{t('items.field.weight')}</label>
            <input id="i-weight" class={inputClass} bind:value={weight} inputmode="decimal" placeholder="0" disabled={!canWrite} aria-invalid={!!errors.weight_grams} />
            {#if errors.weight_grams}<p class={errClass}>{errors.weight_grams}</p>{/if}
          </div>
          <div>
            <label for="i-unit" class={labelClass}>{t('items.field.unit')}</label>
            <Combobox id="i-unit" bind:value={unitId} bind:label={unitLabel} search={searchUnits} placeholder={t('items.field.unit')} clearable={false} disabled={!canWrite} invalid={!!errors.unit_id} />
            {#if errors.unit_id}<p class={errClass}>{errors.unit_id}</p>{/if}
          </div>
        </div>
      </section>

      <section class="surface-card !p-4 space-y-3.5">
        <h3 class="font-display font-bold text-[14px]">{t('items.section.pricing')}</h3>
        <div class="grid gap-3.5 sm:grid-cols-2">
          {#if init}
            <div>
              <span class={labelClass}>{t('items.field.lastCost')}</span>
              <p class="field-control !bg-[var(--surface-sunken)]">{money(init.last_cost)}</p>
            </div>
            <div>
              <span class={labelClass}>{t('items.field.avgCost')}</span>
              <p class="field-control !bg-[var(--surface-sunken)]">{money(init.avg_cost)}</p>
            </div>
          {:else}
            <div class="sm:col-span-2">
              <label for="i-cost" class={labelClass}>{t('items.field.cost')}</label>
              <input id="i-cost" class={inputClass} bind:value={cost} inputmode="decimal" placeholder="0" aria-invalid={!!errors.cost} />
              {#if errors.cost}<p class={errClass}>{errors.cost}</p>{:else}<p class={hintClass}>{t('items.field.costHint')}</p>{/if}
            </div>
          {/if}
          <div class="sm:col-span-2">
            <label for="i-price" class={labelClass}>{t('items.field.sellPrice')}</label>
            <input id="i-price" class={inputClass} bind:value={sellPrice} inputmode="decimal" placeholder="0" disabled={!canWrite} aria-invalid={!!errors.sell_price} />
            {#if errors.sell_price}<p class={errClass}>{errors.sell_price}</p>{:else}<p class={hintClass}>{t('items.field.sellPriceHint')}</p>{/if}
          </div>
        </div>

        <div class="border-t border-[var(--border-subtle)] pt-3.5">
          <h4 class="text-[12.5px] font-semibold">{t('items.outletPrices.title')}</h4>
          <p class={hintClass}>{t('items.outletPrices.hint')}</p>
          {#if outletPrices.length === 0}
            <p class="text-[12px] mt-2 text-[var(--text-tertiary)]">{t('items.outletPrices.empty')}</p>
          {:else}
            <div class="mt-2.5 grid gap-2.5 sm:grid-cols-2">
              {#each outletPrices as p (p.id)}
                <div>
                  <label for="op-{p.id}" class="text-[12px] font-medium mb-1 block">{p.name}</label>
                  <input id="op-{p.id}" class={inputClass} bind:value={p.value} inputmode="decimal" placeholder={t('items.outletPrices.placeholder')} disabled={!canWrite} />
                </div>
              {/each}
            </div>
            {#if errors.outlet_prices}<p class={errClass}>{errors.outlet_prices}</p>{/if}
          {/if}
        </div>
      </section>

      <section class="surface-card !p-4 space-y-2.5">
        <div class="flex items-center justify-between">
          <h3 class="font-display font-bold text-[14px]">{t('items.section.description')}</h3>
          <div class="inline-flex rounded-md border border-[var(--border-subtle)] p-0.5 text-[12px]" role="tablist">
            <button type="button" role="tab" aria-selected={tab === 'write'} class="px-2.5 py-1 rounded {tab === 'write' ? 'bg-[var(--surface-sunken)] font-semibold' : ''}" onclick={() => (tab = 'write')}>{t('items.field.write')}</button>
            <button type="button" role="tab" aria-selected={tab === 'preview'} class="px-2.5 py-1 rounded {tab === 'preview' ? 'bg-[var(--surface-sunken)] font-semibold' : ''}" onclick={() => (tab = 'preview')}>{t('items.field.preview')}</button>
          </div>
        </div>
        {#if tab === 'write'}
          <textarea id="i-desc" rows="8" class="{inputClass} font-mono" bind:value={description} maxlength="5000" disabled={!canWrite} aria-label={t('items.field.description')} aria-invalid={!!errors.description}></textarea>
          {#if errors.description}<p class={errClass}>{errors.description}</p>{:else}<p class={hintClass}>{t('items.field.descriptionHint')}</p>{/if}
        {:else}
          <div class="md min-h-[8rem] field-control">
            {#if description.trim()}
              <!-- Aman: renderMarkdown mematikan HTML mentah, gambar, dan URL berbahaya (lihat #lib/markdown.ts). -->
              {@html renderMarkdown(description)}
            {:else}
              <span class="text-[var(--text-tertiary)]">{t('items.field.previewEmpty')}</span>
            {/if}
          </div>
        {/if}
      </section>
    </div>

    <div class="space-y-4">
      <section class="surface-card !p-4 space-y-3">
        <h3 class="font-display font-bold text-[14px]">{t('items.field.status')}</h3>
        <div class="flex items-center gap-3">
          <Switch bind:checked={active} id="i-active" label={t('items.field.status')} disabled={!canWrite} />
          <label for="i-active" class="text-[12.5px]">{active ? t('items.field.statusOn') : t('items.field.statusOff')}</label>
        </div>
      </section>

      <section class="surface-card !p-4 space-y-3.5">
        <h3 class="font-display font-bold text-[14px]">{t('items.section.classification')}</h3>
        <div>
          <label for="i-category" class={labelClass}>{t('items.field.category')}</label>
          <Combobox id="i-category" bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t('items.field.category')} disabled={!canWrite} invalid={!!errors.category_id} />
          {#if errors.category_id}<p class={errClass}>{errors.category_id}</p>{/if}
        </div>
        <div>
          <label for="i-brand" class={labelClass}>{t('items.field.brand')}</label>
          <Combobox id="i-brand" bind:value={brandId} bind:label={brandLabel} search={searchBrands} placeholder={t('items.field.brand')} disabled={!canWrite} invalid={!!errors.brand_id} />
          {#if errors.brand_id}<p class={errClass}>{errors.brand_id}</p>{/if}
        </div>
        <div>
          <label for="i-principal" class={labelClass}>{t('items.field.principal')}</label>
          <Combobox id="i-principal" bind:value={principalId} bind:label={principalLabel} search={searchPrincipals} placeholder={t('items.field.principal')} disabled={!canWrite} invalid={!!errors.principal_id} />
          {#if errors.principal_id}<p class={errClass}>{errors.principal_id}</p>{/if}
        </div>
        <div>
          <label for="i-supplier" class={labelClass}>{t('items.field.supplier')}</label>
          <Combobox id="i-supplier" bind:value={supplierId} bind:label={supplierLabel} search={searchSuppliers} placeholder={t('items.field.supplier')} disabled={!canWrite} invalid={!!errors.supplier_id} />
          {#if errors.supplier_id}<p class={errClass}>{errors.supplier_id}</p>{/if}
        </div>
      </section>

      <section class="surface-card !p-4 space-y-3">
        <h3 class="font-display font-bold text-[14px]">{t('items.section.settings')}</h3>
        <div>
          <label for="i-kind" class={labelClass}>{t('items.field.kind')}</label>
          <select id="i-kind" class={inputClass} bind:value={kind} disabled={!canWrite}>
            <option value="goods">{t('items.field.kindGoods')}</option>
            <option value="service">{t('items.field.kindService')}</option>
          </select>
        </div>
        <label class="flex items-start gap-2 text-[12.5px]">
          <input type="checkbox" class="mt-0.5" bind:checked={allowNegative} disabled={!canWrite} />
          <span>{t('items.field.allowNegativeStock')}</span>
        </label>
        <label class="flex items-start gap-2 text-[12.5px]">
          <input type="checkbox" class="mt-0.5" bind:checked={belowCost} disabled={!canWrite} />
          <span>{t('items.field.sellBelowCost')}</span>
        </label>
      </section>
    </div>
  </div>

  <div class="flex justify-end gap-2">
    <a href="/items" class="btn !text-[12.5px]">{canWrite ? t('items.cancel') : t('items.back')}</a>
    {#if canWrite}
      <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving}>{saving ? t('items.saving') : t('items.save')}</button>
    {/if}
  </div>
</form>

<style>
  .md :global(h1) { font-size: 1.25rem; font-weight: 700; margin: 0.5rem 0; }
  .md :global(h2) { font-size: 1.1rem; font-weight: 700; margin: 0.5rem 0; }
  .md :global(h3) { font-size: 1rem; font-weight: 600; margin: 0.5rem 0; }
  .md :global(p) { margin: 0.4rem 0; }
  .md :global(ul) { list-style: disc; padding-inline-start: 1.25rem; margin: 0.4rem 0; }
  .md :global(ol) { list-style: decimal; padding-inline-start: 1.25rem; margin: 0.4rem 0; }
  .md :global(a) { color: var(--color-primary-600); text-decoration: underline; }
  .md :global(code) { font-family: ui-monospace, monospace; background: var(--surface-sunken); padding: 0.05rem 0.3rem; border-radius: 4px; }
  .md :global(pre) { background: var(--surface-sunken); padding: 0.6rem; border-radius: 6px; overflow-x: auto; }
  .md :global(blockquote) { border-inline-start: 3px solid var(--border-subtle); padding-inline-start: 0.75rem; color: var(--text-tertiary); }
</style>
