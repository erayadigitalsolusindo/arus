<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import OutletMap from '#lib/platform/OutletMap.svelte';

  const sel = 'h-8 px-2 rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px]';

  // ---- Data: halaman berbasis cursor; filter dijalankan di server ----
  let q = $state('');
  let fStatus = $state(''); // '' | 'active' | 'inactive'
  let fLoc = $state(''); // '' | 'has' | 'missing'
  let locs = $state<OutletLocation[]>([]);
  let next = $state('');
  let total = $state(0);
  let located = $state(0);
  let loading = $state(true);
  let more = $state(false);
  let error = $state('');
  let seq = 0; // abaikan respons filter lama yang datang terlambat
  const filtered = $derived(!!(q.trim() || fStatus || fLoc));

  async function load(append = false) {
    const my = ++seq;
    error = '';
    if (append) more = true; else loading = true;
    try {
      const r = await platformApi.outletLocations({ q: q.trim(), status: fStatus, loc: fLoc, after: append ? next : '' });
      if (my !== seq) return;
      locs = append ? [...locs, ...r.items] : r.items;
      next = r.next;
      if (r.total !== undefined) { total = r.total; located = r.located ?? 0; }
    } catch (err) {
      if (my === seq) error = errorMessage(err);
    } finally {
      if (my === seq) { loading = false; more = false; }
    }
  }

  let timer: ReturnType<typeof setTimeout>;
  function refilter() { clearTimeout(timer); timer = setTimeout(() => void load(), 300); }
  const reset = () => { q = ''; fStatus = ''; fLoc = ''; void load(); };
  onMount(() => { void load(); return () => clearTimeout(timer); });

  // Muat otomatis saat penanda di dasar daftar terlihat; tombol tetap ada sebagai cadangan.
  let sentinel = $state<HTMLElement>();
  $effect(() => {
    if (!sentinel) return;
    const io = new IntersectionObserver((e) => { if (e[0].isIntersecting && next && !more && !loading) void load(true); }, { rootMargin: '200px' });
    io.observe(sentinel);
    return () => io.disconnect();
  });

  const groups = $derived.by(() => {
    const m = new Map<string, { id: string; name: string; items: OutletLocation[] }>();
    for (const l of locs) (m.get(l.tenant_id) ?? m.set(l.tenant_id, { id: l.tenant_id, name: l.tenant_name, items: [] }).get(l.tenant_id)!).items.push(l);
    return [...m.values()];
  });

  // ---- Peta (dimuat hanya saat dibuka, memakai filter yang sama) ----
  let mapOpen = $state(false);
  let mapItems = $state<OutletLocation[]>([]);
  let mapBusy = $state(false);
  async function openMap() {
    mapOpen = true; mapBusy = true;
    try {
      mapItems = (await platformApi.outletLocations({ q: q.trim(), status: fStatus, loc: fLoc, map: true })).items;
    } catch (err) {
      error = errorMessage(err); mapOpen = false;
    } finally {
      mapBusy = false;
    }
  }

  // ---- Isi koordinat per outlet: satu kolom "lat, lng" (format salin dari Google Maps) ----
  let draft = $state<Record<string, string>>({});
  let busy = $state<Record<string, boolean>>({});
  let rowErr = $state<Record<string, string>>({});
  const cur = (l: OutletLocation) => (l.latitude !== null ? `${l.latitude}, ${l.longitude}` : '');
  const val = (l: OutletLocation) => draft[l.outlet_id] ?? cur(l);
  const mapUrl = (l: OutletLocation) => `https://www.openstreetmap.org/?mlat=${l.latitude}&mlon=${l.longitude}#map=17/${l.latitude}/${l.longitude}`;

  /** Isi kolom dari GPS perangkat ini (butuh HTTPS + izin browser); belum menyimpan. */
  function useGps(l: OutletLocation) {
    if (!navigator.geolocation) { rowErr[l.outlet_id] = t('outlets.geo.unavailable'); return; }
    busy[l.outlet_id] = true; rowErr[l.outlet_id] = '';
    navigator.geolocation.getCurrentPosition(
      (p) => { draft[l.outlet_id] = `${p.coords.latitude.toFixed(6)}, ${p.coords.longitude.toFixed(6)}`; busy[l.outlet_id] = false; },
      (err) => { rowErr[l.outlet_id] = t(err.code === err.PERMISSION_DENIED ? 'outlets.geo.denied' : 'outlets.geo.unavailable'); busy[l.outlet_id] = false; },
      { enableHighAccuracy: true, timeout: 15000, maximumAge: 0 }
    );
  }

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
      const had = l.latitude !== null;
      locs = locs.map((x) => (x.outlet_id === l.outlet_id ? { ...x, latitude: lat, longitude: lng } : x));
      located += (lat !== null ? 1 : 0) - (had ? 1 : 0);
      delete draft[l.outlet_id];
    } catch (err) {
      rowErr[l.outlet_id] = errorMessage(err);
    } finally {
      busy[l.outlet_id] = false;
    }
  }
</script>

<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
  <h1 class="font-display font-bold text-[20px] tracking-tight">{t('platform.reports.tabLocations')}</h1>
  {#if !loading}
    <span class="badge-success rounded-full px-2 py-0.5 text-[11.5px] font-semibold">{formatNumber(located)} / {formatNumber(total)} · {t('platform.reports.withLoc')}</span>
  {/if}
  <button type="button" class="btn btn-outline !text-[12.5px] ms-auto" onclick={openMap}><i class="icon-map text-[14px]"></i>{t('platform.reports.map')}</button>
</div>

<div class="flex flex-wrap items-center gap-2">
  <input type="search" class="{sel} w-full sm:w-64" placeholder={t('platform.reports.fSearch')} aria-label={t('platform.reports.fSearch')} bind:value={q} oninput={refilter} />
  <select class={sel} aria-label={t('platform.reports.fStatus')} bind:value={fStatus} onchange={() => load()}>
    <option value="">{t('platform.reports.fStatus')}</option>
    <option value="active">{t('platform.reports.fActive')}</option>
    <option value="inactive">{t('platform.reports.inactive')}</option>
  </select>
  <select class={sel} aria-label={t('platform.reports.fLoc')} bind:value={fLoc} onchange={() => load()}>
    <option value="">{t('platform.reports.fLoc')}</option>
    <option value="has">{t('platform.reports.fHasLoc')}</option>
    <option value="missing">{t('platform.reports.withoutLoc')}</option>
  </select>
  {#if filtered}<button type="button" class="text-[12.5px] font-medium text-[var(--color-primary-700)] hover:underline" onclick={reset}>{t('platform.reports.fReset')}</button>{/if}
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

<div class="space-y-1.5">
  {#if loading}
    <div class="h-10 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>
  {:else if groups.length === 0}
    <div class="surface-card text-center text-[var(--text-tertiary)] py-8">{t('platform.reports.empty')}</div>
  {:else}
    {#each groups as g (g.id)}
      <details class="surface-card !p-0 group" open={filtered}>
        <summary class="flex items-center gap-2 px-3 py-2 cursor-pointer select-none list-none text-[13px]">
          <i class="icon-chevron-right text-[13px] transition-transform group-open:rotate-90"></i>
          <span class="font-semibold flex-1 min-w-0 truncate">{g.name}</span>
          <span class="text-[11.5px] text-[var(--text-tertiary)] whitespace-nowrap">{t('platform.reports.fOutletsLoc', { n: g.items.length, m: g.items.filter((x) => x.latitude !== null).length })}</span>
          <a href="/platform/tenants/{g.id}" class="text-[11.5px] text-[var(--color-primary-700)] hover:underline" onclick={(e) => e.stopPropagation()}>{t('platform.reports.fOpenTenant')}</a>
        </summary>
        <div class="border-t border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)]">
          {#each g.items as l (l.outlet_id)}
            <div class="px-3 py-1.5 grid gap-1.5 md:grid-cols-[minmax(0,1fr)_minmax(0,260px)_auto] md:items-center text-[12.5px]">
              <div class="min-w-0 truncate">
                <span class="font-medium">{l.outlet_name}</span>{#if !l.active} <span class="badge-neutral rounded-full px-1.5 py-0.5 text-[10.5px]">{t('platform.reports.inactive')}</span>{/if}
                {#if l.address}<span class="text-[var(--text-tertiary)]"> · {l.address}</span>{/if}
              </div>
              <div>
                <input class="{sel} w-full font-mono" placeholder="-5.147665, 119.432732" aria-label={t('platform.reports.coords')}
                  value={val(l)} oninput={(e) => (draft[l.outlet_id] = e.currentTarget.value)} onkeydown={(e) => e.key === 'Enter' && save(l)} />
                {#if rowErr[l.outlet_id]}<p role="alert" class="text-[11px] mt-0.5 text-[var(--color-danger-600)]">{rowErr[l.outlet_id]}</p>{/if}
              </div>
              <div class="flex items-center gap-1.5">
                <button type="button" class="btn btn-outline !h-8 !px-2" title={t('outlets.geo.use')} aria-label={t('outlets.geo.use')} disabled={busy[l.outlet_id]} onclick={() => useGps(l)}><i class="icon-crosshair text-[14px]"></i></button>
                <button type="button" class="btn btn-primary !h-8 !px-3 !text-[12px] disabled:opacity-60" disabled={busy[l.outlet_id] || val(l).trim() === cur(l)} onclick={() => save(l)}>{t('platform.reports.save')}</button>
                {#if l.latitude !== null}
                  <a href={mapUrl(l)} target="_blank" rel="noopener noreferrer" class="text-[var(--color-primary-700)]" aria-label={t('platform.reports.map')}><i class="icon-map-pin text-[15px]"></i></a>
                  <button type="button" class="text-[11.5px] text-[var(--text-tertiary)] hover:underline" disabled={busy[l.outlet_id]} onclick={() => save(l, true)}>{t('platform.reports.fClear')}</button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </details>
    {/each}
    <div bind:this={sentinel} class="flex justify-center py-2">
      {#if next}
        <button type="button" class="btn btn-outline !text-[12.5px]" disabled={more} onclick={() => load(true)}>{more ? '…' : t('platform.reports.loadMore')}</button>
      {/if}
    </div>
  {/if}
</div>

{#if mapOpen}
  <div class="fixed inset-0 z-50 grid place-items-center p-4 bg-black/50" role="presentation" onclick={(e) => e.target === e.currentTarget && (mapOpen = false)} onkeydown={(e) => e.key === 'Escape' && (mapOpen = false)}>
    <div role="dialog" aria-modal="true" aria-label={t('platform.reports.map')} class="surface-card w-full max-w-5xl !p-3 space-y-2">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold text-[14px]">{t('platform.reports.map')}{#if !mapBusy} <span class="text-[var(--text-tertiary)] font-normal">· {formatNumber(mapItems.length)}</span>{/if}</h2>
        <button type="button" class="btn btn-outline !h-8 !px-2" aria-label={t('common.close')} onclick={() => (mapOpen = false)}><i class="icon-x text-[14px]"></i></button>
      </div>
      {#if mapBusy}<div class="h-64 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>{:else}<OutletMap items={mapItems} />{/if}
    </div>
  </div>
{/if}
