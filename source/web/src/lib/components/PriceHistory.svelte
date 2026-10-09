<script lang="ts">
  import { items, type PriceChange, type PriceEvent } from '#lib/items/api.ts';
  import { changeValue, diffClass, orderChanges, scopeName } from '#lib/items/pricefmt.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';

  // outlets: nama cabang yang boleh dilihat pemanggil (id → nama); cabang tak dikenal tampil sebagai "—".
  let { itemId, outlets }: { itemId: string; outlets: { id: string; name: string }[] } = $props();

  let events = $state<PriceEvent[]>([]);
  let next = $state('');
  let loading = $state(true);
  let failed = $state(false);

  const names = $derived(new Map(outlets.map((o) => [o.id, o.name])));

  async function load(cursor = '') {
    loading = true;
    failed = false;
    try {
      const r = await items.priceHistory(itemId, cursor);
      events = cursor ? [...events, ...r.items] : r.items;
      next = r.next_cursor;
    } catch {
      failed = true;
    } finally {
      loading = false;
    }
  }

  // Dimuat ulang bila item berganti.
  $effect(() => {
    itemId;
    events = [];
    next = '';
    load();
  });

  const scope = (c: PriceChange) => scopeName(c, names);
  const ordered = (cs: PriceChange[]) => orderChanges(cs, names);
  const value = changeValue;
</script>

<section class="surface-card !p-4 space-y-3">
  <div>
    <h3 class="font-display font-bold text-[14px]">{t('items.priceHistory.title')}</h3>
    <p class="text-[12px] text-[var(--text-tertiary)]">{t('items.priceHistory.hint')}</p>
  </div>

  {#if failed}
    <p role="alert" class="text-[12.5px] badge-warning rounded-lg px-3 py-2">{t('items.priceHistory.failed')}</p>
  {:else if loading && events.length === 0}
    <div class="h-16 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>
  {:else if events.length === 0}
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('items.priceHistory.empty')}</p>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full text-[12.5px]">
        <thead>
          <tr class="text-start text-[var(--text-tertiary)] border-b border-[var(--border-subtle)]">
            <th class="py-1.5 pe-3 text-start font-medium whitespace-nowrap">{t('items.priceHistory.time')}</th>
            <th class="py-1.5 pe-3 text-start font-medium">{t('items.priceHistory.scope')}</th>
            <th class="py-1.5 pe-3 text-end font-medium">{t('items.priceHistory.before')}</th>
            <th class="py-1.5 pe-3 text-end font-medium">{t('items.priceHistory.after')}</th>
            <th class="py-1.5 text-start font-medium">{t('items.priceHistory.by')}</th>
          </tr>
        </thead>
        <tbody>
          {#each events as ev (ev.id)}
            {#each ordered(ev.changes) as c, i (c.outlet_id ?? 'default')}
              <tr class="border-b border-[var(--border-subtle)] last:border-0 align-top">
                <td class="py-1.5 pe-3 whitespace-nowrap">{i === 0 ? formatDateTime(ev.at) : ''}</td>
                <td class="py-1.5 pe-3">
                  {scope(c)}
                  {#if ev.kind === 'wholesale'}<span class="badge-soft badge-info ms-1">{t('items.priceHistory.kindWholesale')}</span>{/if}
                  {#if ev.kind === 'initial'}<span class="badge-soft ms-1">{t('items.priceHistory.kindInitial')}</span>{/if}
                </td>
                <td class="py-1.5 pe-3 text-end text-[var(--text-secondary)]">{value(ev, c.before, ev.kind === 'initial' ? t('items.priceHistory.none') : t('items.priceHistory.created'))}</td>
                <td class="py-1.5 pe-3 text-end font-medium {diffClass(c)}">{value(ev, c.after, t('items.priceHistory.removed'))}</td>
                <td class="py-1.5">{i === 0 ? ev.actor_name : ''}</td>
              </tr>
            {/each}
          {/each}
        </tbody>
      </table>
    </div>
    {#if next}
      <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(next)}>{t('items.priceHistory.more')}</button>
    {/if}
  {/if}
</section>
