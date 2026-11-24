<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type SalesReportRow, type SalesMonth, type ExpiringRow, type OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import OutletMap from '#lib/platform/OutletMap.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  // Uang platform selalu Rupiah (nilai transaksi tenant), tanpa desimal.
  const rp = (n: number) => 'Rp ' + formatNumber(Math.round(n), { maximumFractionDigits: 0 });
  const th = 'px-4 py-2.5 font-semibold';
  const DAY = 86_400_000;

  let error = $state('');
  let loading = $state(true);
  // ---- Lokasi ----
  let locs = $state<OutletLocation[]>([]);
  let q = $state('');
  let fTenant = $state('');
  let fStatus = $state(''); // '' | 'active' | 'inactive'
  let fLoc = $state(''); // '' | 'has' | 'missing'
  const tenants = $derived([...new Map(locs.map((l) => [l.tenant_id, l.tenant_name])).entries()].sort((a, b) => a[1].localeCompare(b[1])));
  const filtered = $derived(fTenant || fStatus || fLoc || q.trim());
  const shownLocs = $derived.by(() => {
    const k = q.trim().toLowerCase();
    return locs.filter((l) =>
      (!fTenant || l.tenant_id === fTenant) &&
      (!fStatus || l.active === (fStatus === 'active')) &&
      (!fLoc || (l.latitude !== null) === (fLoc === 'has')) &&
      (!k || `${l.tenant_name} ${l.outlet_name} ${l.address}`.toLowerCase().includes(k)));
  });
  // Dikelompokkan per tenant untuk accordion (urutan mengikuti API: nama tenant).
  const groups = $derived.by(() => {
    const m = new Map<string, { id: string; name: string; items: OutletLocation[] }>();
    for (const l of shownLocs) (m.get(l.tenant_id) ?? m.set(l.tenant_id, { id: l.tenant_id, name: l.tenant_name, items: [] }).get(l.tenant_id)!).items.push(l);
    return [...m.values()];
  });
  // Isian koordinat per outlet: satu kolom "lat, lng" (format salin dari Google Maps).
  let draft = $state<Record<string, string>>({});
  let busy = $state<Record<string, boolean>>({});
  let rowErr = $state<Record<string, string>>({});
  const cur = (l: OutletLocation) => (l.latitude !== null ? `${l.latitude}, ${l.longitude}` : '');
  const val = (l: OutletLocation) => draft[l.outlet_id] ?? cur(l);

  async function save(l: OutletLocation, clear = false) {
    const raw = clear ? '' : val(l).trim();
    let lat: number | null = null, lng: number | null = null;
    if (raw) {
      const m = raw.match(/^(-?\d+(?:\.\d+)?)\s*[,;\s]\s*(-?\d+(?:\.\d+)?)$/);
      lat = m ? Number(m[1]) : NaN; lng = m ? Number(m[2]) : NaN;
      if (!m || Math.abs(lat) > 90 || Math.abs(lng) > 180) { rowErr[l.outlet_id] = t('platform.reports.coordInvalid'); return; }
    }
    busy[l.outlet_id] = true; rowErr[l.outlet_id] = '';
    try {
      await platformApi.setOutletLocation(l.tenant_id, l.outlet_id, lat, lng);
      locs = locs.map((x) => (x.outlet_id === l.outlet_id ? { ...x, latitude: lat, longitude: lng } : x));
      delete draft[l.outlet_id];
    } catch (err) {
      rowErr[l.outlet_id] = errorMessage(err);
    } finally {
      busy[l.outlet_id] = false;
    }
  }
  const reset = () => { q = ''; fTenant = ''; fStatus = ''; fLoc = ''; };
  const sel = 'h-9 px-2.5 rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px]';
  const located = $derived(locs.filter((l) => l.latitude !== null).length);
  const mapUrl = (l: OutletLocation) => `https://www.openstreetmap.org/?mlat=${l.latitude}&mlon=${l.longitude}#map=17/${l.latitude}/${l.longitude}`;

  async function loadLocations() {
    loading = true;
    error = '';
    try {
      locs = await platformApi.outletLocations();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  onMount(loadLocations);
</script>

<div>
  <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.reports.tabLocations')}</h1>
  <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.reports.sub')}</p>
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

  <div class="grid grid-cols-2 gap-3 max-w-xl">
    <StatCard label={t('platform.reports.withLoc')} value="{formatNumber(located)} / {formatNumber(locs.length)}" icon="map-pin" tone="success" />
    <StatCard label={t('platform.reports.withoutLoc')} value={formatNumber(locs.length - located)} icon="circle-alert" tone="warning" />
  </div>
  <div class="flex flex-wrap items-center gap-2">
    <input type="search" class="{sel} w-full sm:w-64" placeholder={t('platform.reports.fSearch')} aria-label={t('platform.reports.fSearch')} bind:value={q} />
    <select class={sel} aria-label={t('platform.reports.tenant')} bind:value={fTenant}>
      <option value="">{t('platform.reports.fTenant')}</option>
      {#each tenants as [id, name] (id)}<option value={id}>{name}</option>{/each}
    </select>
    <select class={sel} aria-label={t('platform.reports.fStatus')} bind:value={fStatus}>
      <option value="">{t('platform.reports.fStatus')}</option>
      <option value="active">{t('platform.reports.fActive')}</option>
      <option value="inactive">{t('platform.reports.inactive')}</option>
    </select>
    <select class={sel} aria-label={t('platform.reports.fLoc')} bind:value={fLoc}>
      <option value="">{t('platform.reports.fLoc')}</option>
      <option value="has">{t('platform.reports.fHasLoc')}</option>
      <option value="missing">{t('platform.reports.withoutLoc')}</option>
    </select>
    {#if filtered}<button type="button" class="text-[12.5px] font-medium text-[var(--color-primary-700)] hover:underline" onclick={reset}>{t('platform.reports.fReset')}</button>{/if}
    <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.reports.fCount', { shown: shownLocs.length, total: locs.length })}</span>
  </div>
  <p class="text-[11.5px] -mt-2 text-[var(--text-tertiary)]">{t('platform.reports.locHint')}</p>
  <OutletMap items={shownLocs} />
  <div class="space-y-2">
    {#if loading}
      <div class="h-12 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>
    {:else if groups.length === 0}
      <div class="surface-card text-center text-[var(--text-tertiary)] py-10">{t('platform.reports.empty')}</div>
    {:else}
      {#each groups as g (g.id)}
        <details class="surface-card !p-0 group" open={!!filtered}>
          <summary class="flex items-center gap-3 px-4 py-3 cursor-pointer select-none list-none">
            <i class="icon-chevron-right text-[14px] transition-transform group-open:rotate-90"></i>
            <span class="font-semibold flex-1 min-w-0 truncate">{g.name}</span>
            <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.reports.fOutletsLoc', { n: g.items.length, m: g.items.filter((x) => x.latitude !== null).length })}</span>
            <a href="/platform/tenants/{g.id}" class="text-[12px] text-[var(--color-primary-700)] hover:underline" onclick={(e) => e.stopPropagation()}>{t('platform.reports.fOpenTenant')}</a>
          </summary>
          <div class="border-t border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)]">
            {#each g.items as l (l.outlet_id)}
              <div class="px-4 py-3 grid gap-2 md:grid-cols-[1fr_minmax(0,320px)_auto] md:items-center">
                <div class="min-w-0">
                  <div class="font-medium">{l.outlet_name}{#if !l.active} <span class="badge-neutral rounded-full px-1.5 py-0.5 text-[10.5px]">{t('platform.reports.inactive')}</span>{/if}</div>
                  <div class="text-[12px] text-[var(--text-secondary)] truncate">{l.address || '—'}</div>
                </div>
                <div>
                  <input class="{sel} w-full font-mono" placeholder="-5.147665, 119.432732" aria-label={t('platform.reports.coords')}
                    value={val(l)} oninput={(e) => (draft[l.outlet_id] = e.currentTarget.value)} onkeydown={(e) => e.key === 'Enter' && save(l)} />
                  {#if rowErr[l.outlet_id]}<p role="alert" class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{rowErr[l.outlet_id]}</p>{/if}
                </div>
                <div class="flex items-center gap-2">
                  <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy[l.outlet_id] || val(l).trim() === cur(l)} onclick={() => save(l)}>{t('platform.reports.save')}</button>
                  {#if l.latitude !== null}
                    <a href={mapUrl(l)} target="_blank" rel="noopener noreferrer" class="text-[var(--color-primary-700)]" aria-label={t('platform.reports.map')}><i class="icon-map-pin text-[15px]"></i></a>
                    <button type="button" class="text-[12px] text-[var(--text-tertiary)] hover:underline" disabled={busy[l.outlet_id]} onclick={() => save(l, true)}>{t('platform.reports.fClear')}</button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </details>
      {/each}
    {/if}
  </div>
