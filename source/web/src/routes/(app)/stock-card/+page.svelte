<script lang="ts">
  import { card, BUCKETS, type Bucket, type CardItem, type CardResult, type CardRow } from '#lib/stock/api.ts';
  import { persistPage } from '#lib/tabs/persist.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';

  let itemId = $state('');
  let itemLabel = $state('');
  let from = $state('');
  let to = $state('');
  let bucket = $state<Bucket | ''>('');

  // Filter tetap ada saat pindah tab lalu kembali (lib/tabs/persist.ts).
  persistPage(
    () => ({ itemId, itemLabel, from, to, bucket }),
    (s) => ({ itemId, itemLabel, from, to, bucket } = s)
  );

  let res = $state<CardResult | null>(null);
  let rows = $state<CardRow[]>([]);
  let loading = $state(false);
  let error = $state('');
  let seq = 0; // abaikan respons basi bila filter berubah saat memuat

  const num = (s: string) => Number(s);
  const qty = (s: string | number) => formatNumber(Number(s), { maximumFractionDigits: 3 });
  const bucketOptions = $derived([
    { value: '' as const, label: t('stock.card.bucketAll') },
    ...BUCKETS.map((b) => ({ value: b, label: t(`stock.card.bucket.${b}`) }))
  ]);

  async function search(q: string): Promise<Option[]> {
    const r = await card.items(q.trim());
    return r.data.map((i: CardItem) => ({ id: i.id, name: `${i.sku} — ${i.name}` }));
  }

  // Muat dari awal (filter berubah) atau lanjutkan dari kursor (muat lebih banyak).
  async function load(more = false) {
    if (!itemId) {
      res = null;
      rows = [];
      return;
    }
    const my = ++seq;
    loading = true;
    error = '';
    try {
      const r = await card.get({ itemId, from, to, bucket, cursor: more ? res?.next_cursor : null });
      if (my !== seq) return;
      res = r;
      rows = more ? [...rows, ...r.rows] : r.rows;
      // Tampilkan periode bawaan server di pemilih tanggal tanpa memuat ulang.
      if (!from) from = r.from;
      if (!to) to = r.to;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      if (!more) {
        res = null;
        rows = [];
      }
    } finally {
      if (my === seq) loading = false;
    }
  }

  // Ganti barang/bucket/outlet aktif → muat ulang. (Tanggal dimuat lewat onchange DateRange atau preset.)
  $effect(() => {
    void itemId;
    void bucket;
    void session.outlet?.id;
    void load();
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

  // ---- Tampilan jenis mutasi ----
  const TYPE_STYLE: Record<string, { icon: string; tone: string }> = {
    OPENING: { icon: 'icon-package-plus', tone: 'badge-info' },
    SALE: { icon: 'icon-shopping-cart', tone: 'badge-primary' },
    SALE_VOID: { icon: 'icon-ban', tone: 'badge-neutral' },
    SALE_RETURN: { icon: 'icon-undo-2', tone: 'badge-warning' },
    PURCHASE: { icon: 'icon-truck', tone: 'badge-success' },
    PURCHASE_RETURN: { icon: 'icon-undo-2', tone: 'badge-warning' },
    OPNAME: { icon: 'icon-clipboard-check', tone: 'badge-accent' },
    TRANSFER_OUT: { icon: 'icon-arrow-left-right', tone: 'badge-info' },
    TRANSFER_IN: { icon: 'icon-arrow-left-right', tone: 'badge-info' },
    UNIT_CONVERSION: { icon: 'icon-split', tone: 'badge-accent' },
    ADJUSTMENT: { icon: 'icon-sliders-horizontal', tone: 'badge-warning' }
  };
  const style = (r: CardRow) => TYPE_STYLE[r.ref_type] ?? { icon: 'icon-package', tone: 'badge-neutral' };
  const typeLabel = (r: CardRow) => tryT(`stock.card.type.${r.ref_type}`) ?? r.ref_type;
  const day = (r: CardRow) => formatDate(new Date(r.at), { day: '2-digit', month: 'short', year: 'numeric' });
  const time = (r: CardRow) => new Date(r.at).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });

  const periodText = $derived(res ? `${formatDate(new Date(`${res.from}T00:00:00`))} – ${formatDate(new Date(`${res.to}T00:00:00`))}` : '');
</script>

<svelte:head><title>{t('stock.card.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.card.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.card.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-[1600px] mx-auto w-full">
  <div class="max-w-3xl">
    <h1 class="font-display font-bold text-[19px]">{t('stock.card.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('stock.card.subtitle')}</p>
  </div>

  <!-- Filter -->
  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-[minmax(0,2fr)_minmax(0,1.5fr)_minmax(0,1fr)]">
      <div>
        <label class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5" for="card-item">{t('stock.card.item')}</label>
        <Combobox id="card-item" bind:value={itemId} bind:label={itemLabel} {search} placeholder={t('stock.card.pickItem')} />
      </div>
      <div>
        <span class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5">{t('stock.card.period')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('stock.card.period')} />
      </div>
      <div>
        <span class="block text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1.5">{t('stock.card.col.bucket')}</span>
        <Select bind:value={bucket} options={bucketOptions} ariaLabel={t('stock.card.col.bucket')} />
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
          disabled={!itemId}
          onclick={() => applyPreset(p)}
        >{t(`stock.card.preset.${p.id}`)}</button>
      {/each}
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.card.loadFailed')} {error}</span>
    </div>
  {/if}

  {#if !itemId}
    <div class="surface-card flex flex-col items-center text-center" style="padding: 3.5rem 1rem">
      <span class="grid place-items-center size-14 rounded-2xl bg-[var(--color-primary-50)] text-[var(--color-primary-600)] mb-3">
        <i class="icon-book-open text-[26px]"></i>
      </span>
      <p class="font-display font-semibold text-[15px]">{t('stock.card.title')}</p>
      <p class="text-[12.5px] mt-1 max-w-sm text-[var(--text-tertiary)]">{t('stock.card.prompt')}</p>
    </div>
  {:else if !res}
    <!-- Kerangka saat pertama memuat -->
    <div class="grid gap-3 grid-cols-2 lg:grid-cols-4" aria-hidden="true">
      {#each [0, 1, 2, 3] as i (i)}<div class="surface-card h-[88px] animate-pulse"></div>{/each}
    </div>
    <div class="surface-card h-64 animate-pulse" aria-hidden="true"></div>
  {:else}
    <!-- Barang -->
    <section class="surface-card !p-4 flex flex-wrap items-center gap-x-4 gap-y-2">
      <span class="grid place-items-center size-11 rounded-xl bg-[var(--color-primary-50)] text-[var(--color-primary-600)] shrink-0">
        <i class="icon-package text-[20px]"></i>
      </span>
      <div class="min-w-0 grow">
        <h2 class="font-display font-bold text-[16px] leading-tight truncate">{res.item.name}</h2>
        <div class="flex flex-wrap items-center gap-1.5 mt-1.5 text-[11.5px]">
          <span class="badge-soft badge-neutral font-mono">{res.item.sku}</span>
          <span class="badge-soft badge-info">{t('stock.card.inUnit', { unit: res.item.unit })}</span>
          {#if session.outlet}<span class="badge-soft badge-neutral"><i class="icon-store text-[11px]"></i> {session.outlet.name}</span>{/if}
        </div>
      </div>
      <div class="text-end text-[12px] text-[var(--text-tertiary)]">
        <div class="font-semibold text-[var(--text-secondary)]">{periodText}</div>
        {#if !res.next_cursor}<div>{t('stock.card.count', { count: rows.length })}</div>{/if}
      </div>
    </section>

    <!-- Ringkasan -->
    <section class="grid gap-3 grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4 flex items-center gap-3">
        <span class="grid place-items-center size-10 rounded-xl bg-[var(--surface-sunken)] text-[var(--text-secondary)] shrink-0"><i class="icon-history text-[18px]"></i></span>
        <div class="min-w-0">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('stock.card.summary.opening')}</div>
          <div class="font-display font-bold text-[20px] leading-tight tabular-nums">{qty(res.opening)}</div>
        </div>
      </div>
      <div class="surface-card !p-4 flex items-center gap-3">
        <span class="grid place-items-center size-10 rounded-xl bg-[var(--color-success-50,#f0fdf4)] text-[var(--color-success-600,#16a34a)] shrink-0"><i class="icon-arrow-down-to-line text-[18px]"></i></span>
        <div class="min-w-0">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('stock.card.summary.in')}</div>
          <div class="font-display font-bold text-[20px] leading-tight tabular-nums text-[var(--color-success-600,#16a34a)]">+{qty(res.in)}</div>
        </div>
      </div>
      <div class="surface-card !p-4 flex items-center gap-3">
        <span class="grid place-items-center size-10 rounded-xl bg-[var(--color-danger-50)] text-[var(--color-danger-600)] shrink-0"><i class="icon-arrow-up-from-line text-[18px]"></i></span>
        <div class="min-w-0">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('stock.card.summary.out')}</div>
          <div class="font-display font-bold text-[20px] leading-tight tabular-nums text-[var(--color-danger-600)]">−{qty(res.out)}</div>
        </div>
      </div>
      <div class="surface-card !p-4 flex items-center gap-3 ring-1 ring-inset ring-[var(--color-primary-200)] bg-[var(--color-primary-50)]/50">
        <span class="grid place-items-center size-10 rounded-xl bg-[var(--color-primary-600)] text-white shrink-0"><i class="icon-boxes text-[18px]"></i></span>
        <div class="min-w-0">
          <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('stock.card.summary.closing')}</div>
          <div class="font-display font-bold text-[20px] leading-tight tabular-nums {num(res.closing) < 0 ? 'text-[var(--color-danger-600)]' : ''}">{qty(res.closing)}</div>
        </div>
      </div>
    </section>

    <!-- Tabel mutasi -->
    <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
      <div class="hidden md:block overflow-x-auto scroll-thin">
        <table class="w-full text-[12.5px] min-w-[900px]">
          <thead class="bg-[var(--surface-sunken)]">
            <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
              <th class="px-4 py-3 text-start font-semibold" scope="col">{t('stock.card.col.time')}</th>
              <th class="px-3 py-3 text-start font-semibold" scope="col">{t('stock.card.col.type')}</th>
              <th class="px-3 py-3 text-start font-semibold" scope="col">{t('stock.card.col.doc')}</th>
              <th class="px-3 py-3 text-start font-semibold" scope="col">{t('stock.card.col.bucket')}</th>
              <th class="px-3 py-3 text-end font-semibold" scope="col">{t('stock.card.col.in')}</th>
              <th class="px-3 py-3 text-end font-semibold" scope="col">{t('stock.card.col.out')}</th>
              <th class="px-3 py-3 text-end font-semibold" scope="col">{t('stock.card.col.balance')}</th>
              <th class="px-4 py-3 text-start font-semibold" scope="col">{t('stock.card.col.by')}</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as r (r.id)}
              {@const d = num(r.qty_delta)}
              {@const st = style(r)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                <td class="px-4 py-3 whitespace-nowrap">
                  <div class="font-semibold leading-tight">{day(r)}</div>
                  <div class="text-[11.5px] text-[var(--text-tertiary)] tabular-nums">{time(r)}</div>
                </td>
                <td class="px-3 py-3">
                  <span class="badge-soft {st.tone} inline-flex items-center gap-1.5 whitespace-nowrap"><i class="{st.icon} text-[12px]"></i>{typeLabel(r)}</span>
                </td>
                <td class="px-3 py-3">
                  {#if r.note}<span class="font-mono text-[11.5px] rounded-md bg-[var(--surface-sunken)] px-1.5 py-0.5">{r.note}</span>{:else}<span class="text-[var(--text-tertiary)]">—</span>{/if}
                </td>
                <td class="px-3 py-3 text-[var(--text-secondary)]">{t(`stock.card.bucket.${r.bucket}`)}</td>
                <td class="px-3 py-3 text-end tabular-nums font-semibold text-[var(--color-success-600,#16a34a)]">{#if d > 0}+{qty(d)}{:else}<span class="text-[var(--text-tertiary)] font-normal">·</span>{/if}</td>
                <td class="px-3 py-3 text-end tabular-nums font-semibold text-[var(--color-danger-600)]">{#if d < 0}−{qty(-d)}{:else}<span class="text-[var(--text-tertiary)] font-normal">·</span>{/if}</td>
                <td class="px-3 py-3 text-end tabular-nums font-bold {num(r.balance) < 0 ? 'text-[var(--color-danger-600)]' : ''}">{qty(r.balance)}</td>
                <td class="px-4 py-3 text-[var(--text-secondary)] whitespace-nowrap">{r.actor || '—'}</td>
              </tr>
            {:else}
              <tr>
                <td colspan="8" class="px-4 py-14 text-center">
                  <span class="grid place-items-center size-12 rounded-2xl bg-[var(--surface-sunken)] text-[var(--text-tertiary)] mx-auto mb-2.5"><i class="icon-package text-[22px]"></i></span>
                  <p class="text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('stock.card.empty')}</p>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <!-- Ponsel: satu kartu per mutasi -->
      <ul class="md:hidden divide-y divide-[var(--border-subtle)]">
        {#each rows as r (r.id)}
          {@const d = num(r.qty_delta)}
          {@const st = style(r)}
          <li class="p-3.5 space-y-2">
            <div class="flex items-start justify-between gap-3">
              <span class="badge-soft {st.tone} inline-flex items-center gap-1.5"><i class="{st.icon} text-[12px]"></i>{typeLabel(r)}</span>
              <span class="font-display font-bold text-[16px] tabular-nums {d > 0 ? 'text-[var(--color-success-600,#16a34a)]' : 'text-[var(--color-danger-600)]'}">{d > 0 ? '+' : '−'}{qty(Math.abs(d))}</span>
            </div>
            <div class="flex items-center justify-between gap-3 text-[12px]">
              <span class="text-[var(--text-secondary)]">{day(r)} · <span class="tabular-nums">{time(r)}</span></span>
              <span class="text-[var(--text-tertiary)]">{t(`stock.card.bucket.${r.bucket}`)}</span>
            </div>
            <div class="flex items-center justify-between gap-3 text-[12px]">
              {#if r.note}<span class="font-mono text-[11.5px] rounded-md bg-[var(--surface-sunken)] px-1.5 py-0.5 truncate">{r.note}</span>{:else}<span></span>{/if}
              <span class="shrink-0"><span class="text-[var(--text-tertiary)]">{t('stock.card.col.balance')}</span> <b class="tabular-nums {num(r.balance) < 0 ? 'text-[var(--color-danger-600)]' : ''}">{qty(r.balance)}</b></span>
            </div>
            {#if r.actor}<div class="text-[11.5px] text-[var(--text-tertiary)]">{t('stock.card.col.by')}: {r.actor}</div>{/if}
          </li>
        {:else}
          <li class="px-4 py-12 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t('stock.card.empty')}</li>
        {/each}
      </ul>
      {#if res.next_cursor}
        <div class="p-3 border-t border-[var(--border-subtle)] text-center bg-[var(--surface-sunken)]">
          <button type="button" class="btn !text-[12px]" disabled={loading} onclick={() => load(true)}>
            <i class="icon-chevrons-down text-[13px]"></i>{t('stock.card.loadMore')}
          </button>
        </div>
      {/if}
    </section>
    <p class="text-[11.5px] text-center text-[var(--text-tertiary)]">{t('stock.card.flowHint')}</p>
  {/if}
</main>
