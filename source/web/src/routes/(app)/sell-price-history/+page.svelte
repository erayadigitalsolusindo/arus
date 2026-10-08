<script lang="ts">
  import { items, type PriceReportEvent } from '#lib/items/api.ts';
  import { changeValue, diffClass, orderChanges, scopeName } from '#lib/items/pricefmt.ts';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { persistPage } from '#lib/tabs/persist.ts';
  import { t, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';

  let from = $state('');
  let to = $state('');
  let q = $state('');
  let outlet = $state('');

  // Filter tetap ada saat pindah tab lalu kembali (lib/tabs/persist.ts).
  persistPage(
    () => ({ from, to, q, outlet }),
    (s) => ({ from, to, q, outlet } = s)
  );

  let rows = $state<PriceReportEvent[]>([]);
  let next = $state('');
  let count = $state(0);
  let loading = $state(true);
  let error = $state('');
  let seq = 0; // abaikan respons basi bila filter berubah saat memuat

  const names = $derived(new Map(accessibleOutlets.items.map((o) => [o.id, o.name])));
  const outletOptions = $derived([
    { value: '', label: t('items.priceReport.outletAll') },
    { value: 'default', label: t('items.priceReport.outletDefault') },
    ...accessibleOutlets.items.map((o) => ({ value: o.id, label: o.name }))
  ]);

  async function load(more = false) {
    const my = ++seq;
    loading = true;
    error = '';
    try {
      const r = await items.priceReport({ from, to, q: q.trim(), outlet, cursor: more ? next : '' });
      if (my !== seq) return;
      rows = more ? [...rows, ...r.items] : r.items;
      count = more ? count + r.items.length : r.items.length;
      next = r.next_cursor;
      // Tampilkan periode bawaan server di pemilih tanggal tanpa memuat ulang.
      if (!from) from = r.from;
      if (!to) to = r.to;
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

  if (accessibleOutlets.items.length === 0) void refreshOutlets();

  // Cabang berubah → langsung muat; pencarian → tunggu pengetikan berhenti. (Tanggal lewat onchange DateRange/preset.)
  let first = true;
  $effect(() => {
    void outlet;
    const query = q;
    void query;
    const delay = first ? 0 : 350;
    first = false;
    const h = setTimeout(() => void load(), delay);
    return () => clearTimeout(h);
  });

  // ---- Preset periode (hari kalender lokal; server menafsirkan menurut zona waktu outlet) ----
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
  const kindKey = (k: string) => (k === 'wholesale' ? 'kindWholesale' : 'kindInitial');
</script>

<svelte:head><title>{t('items.priceReport.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('items.priceReport.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('items.priceReport.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-[1600px] mx-auto w-full">
  <div class="max-w-3xl">
    <h1 class="font-display font-bold text-[19px]">{t('items.priceReport.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('items.priceReport.subtitle')}</p>
  </div>

  <!-- Filter -->
  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-[minmax(0,2fr)_minmax(0,1.5fr)_minmax(0,1.2fr)]">
      <div>
        <label class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5" for="pr-q">{t('items.priceReport.search')}</label>
        <input id="pr-q" class="form-input w-full" type="search" bind:value={q} placeholder={t('items.priceReport.searchPlaceholder')} maxlength="100" />
      </div>
      <div>
        <span class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5">{t('items.priceReport.period')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('items.priceReport.period')} />
      </div>
      <div>
        <span class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5">{t('items.priceReport.outlet')}</span>
        <Select bind:value={outlet} options={outletOptions} ariaLabel={t('items.priceReport.outlet')} />
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
        >{t(`items.priceReport.preset.${p.id}`)}</button>
      {/each}
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('items.priceReport.loadFailed')} {error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="hidden md:block overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[860px]">
        <thead class="bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('items.priceHistory.time')}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t('items.priceReport.item')}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t('items.priceHistory.scope')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('items.priceHistory.before')}</th>
            <th class="px-3 py-3 text-end font-semibold" scope="col">{t('items.priceHistory.after')}</th>
            <th class="px-4 py-3 text-start font-semibold" scope="col">{t('items.priceHistory.by')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as ev (ev.id)}
            {#each orderChanges(ev.changes, names) as c, i (c.outlet_id ?? 'default')}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)] align-top">
                <td class="px-4 py-2.5 whitespace-nowrap">{i === 0 ? formatDateTime(ev.at) : ''}</td>
                <td class="px-3 py-2.5">
                  {#if i === 0}
                    <a href="/items/{ev.item_id}" class="font-semibold text-[var(--color-primary-600)]">{ev.name}</a>
                    <div class="font-mono text-[11px] text-[var(--text-tertiary)]">{ev.sku}</div>
                  {/if}
                </td>
                <td class="px-3 py-2.5">
                  {scopeName(c, names)}
                  {#if ev.kind !== 'price'}<span class="badge-soft badge-info ms-1">{t(`items.priceHistory.${kindKey(ev.kind)}`)}</span>{/if}
                </td>
                <td class="px-3 py-2.5 text-end text-[var(--text-secondary)]">{changeValue(ev, c.before, ev.kind === 'initial' ? t('items.priceHistory.none') : t('items.priceHistory.created'))}</td>
                <td class="px-3 py-2.5 text-end font-medium {diffClass(c)}">{changeValue(ev, c.after, t('items.priceHistory.removed'))}</td>
                <td class="px-4 py-2.5 text-[var(--text-secondary)] whitespace-nowrap">{i === 0 ? ev.actor_name || '—' : ''}</td>
              </tr>
            {/each}
          {:else}
            <tr>
              <td colspan="6" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('items.priceReport.empty')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Ponsel: satu kartu per kejadian -->
    <ul class="md:hidden divide-y divide-[var(--border-subtle)]">
      {#each rows as ev (ev.id)}
        <li class="p-3.5 space-y-1.5">
          <div class="flex items-start justify-between gap-3">
            <a href="/items/{ev.item_id}" class="font-semibold text-[var(--color-primary-600)]">{ev.name}</a>
            <span class="font-mono text-[11px] text-[var(--text-tertiary)] shrink-0">{ev.sku}</span>
          </div>
          {#each orderChanges(ev.changes, names) as c (c.outlet_id ?? 'default')}
            <div class="text-[12.5px]">
              <div class="text-[var(--text-secondary)]">
                {scopeName(c, names)}
                {#if ev.kind !== 'price'}<span class="badge-soft badge-info ms-1">{t(`items.priceHistory.${kindKey(ev.kind)}`)}</span>{/if}
              </div>
              <div class="tabular-nums">
                <span class="text-[var(--text-tertiary)]">{changeValue(ev, c.before, ev.kind === 'initial' ? t('items.priceHistory.none') : t('items.priceHistory.created'))}</span>
                → <b class={diffClass(c)}>{changeValue(ev, c.after, t('items.priceHistory.removed'))}</b>
              </div>
            </div>
          {/each}
          <div class="text-[11.5px] text-[var(--text-tertiary)]">{formatDateTime(ev.at)} · {ev.actor_name || '—'}</div>
        </li>
      {:else}
        <li class="px-4 py-12 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('items.priceReport.empty')}</li>
      {/each}
    </ul>

    {#if next}
      <div class="p-3 border-t border-[var(--border-subtle)] text-center bg-[var(--surface-sunken)]">
        <button type="button" class="btn !text-[12px]" disabled={loading} onclick={() => load(true)}>
          <i class="icon-chevrons-down text-[13px]"></i>{t('items.priceReport.loadMore')}
        </button>
      </div>
    {/if}
  </section>
  {#if rows.length > 0}
    <p class="text-[11.5px] text-center text-[var(--text-tertiary)]">
      {t('items.priceReport.count', { count })}{#if from && to}{' · '}{formatDate(new Date(`${from}T00:00:00`))} – {formatDate(new Date(`${to}T00:00:00`))}{/if}
    </p>
  {/if}
</main>
