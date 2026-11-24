<script lang="ts">
  import { onMount } from 'svelte';
  import type { OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import 'leaflet/dist/leaflet.css';
  import 'leaflet.markercluster/dist/MarkerCluster.css';
  import 'leaflet.markercluster/dist/MarkerCluster.Default.css';

  let { items }: { items: OutletLocation[] } = $props();
  let el: HTMLDivElement;
  let map: import('leaflet').Map | undefined;
  let layer: import('leaflet').MarkerClusterGroup | undefined;
  let L: typeof import('leaflet') | undefined;
  const markers = new Map<string, import('leaflet').CircleMarker>();
  let inView = $state<OutletLocation[]>([]); // outlet yang tampak di peta saat ini (daftar samping)
  const VIEW_CAP = 60;
  let q = $state('');
  // Cari di semua titik (tenant/outlet/alamat), bukan hanya yang tampak; kosong = daftar area peta.
  const matches = $derived.by(() => {
    const k = q.trim().toLowerCase();
    return k ? items.filter((o) => o.latitude !== null && `${o.tenant_name} ${o.outlet_name} ${o.address}`.toLowerCase().includes(k)) : [];
  });
  const shown = $derived(q.trim() ? matches : inView);
  function fitMatches() {
    if (map && matches.length) map.fitBounds(matches.map((o) => [o.latitude!, o.longitude!] as [number, number]), { padding: [30, 30], maxZoom: 16 });
  }
  const activeCount = $derived(items.filter((o) => o.active).length);

  const COLOR = { on: '#16a34a', off: '#9ca3af' };

  // Isi popup dibangun dengan textContent: nama tenant/outlet/alamat dari user, jangan jadi HTML.
  function popup(o: OutletLocation) {
    const root = document.createElement('div');
    root.style.cssText = 'min-width:200px;font:12.5px/1.45 system-ui,sans-serif';
    const row = (txt: string, css = '') => { const d = document.createElement('div'); d.textContent = txt; d.style.cssText = css; root.append(d); return d; };
    row(o.outlet_name, 'font-weight:700;font-size:14px');
    row(o.tenant_name, 'color:#6b7280');
    const st = row(o.active ? t('platform.reports.fActive') : t('platform.reports.inactive'),
      `display:inline-block;margin:4px 0;padding:1px 8px;border-radius:99px;font-size:11px;font-weight:600;color:#fff;background:${o.active ? COLOR.on : COLOR.off}`);
    st.style.width = 'fit-content';
    if (o.address) row(o.address);
    row(`${o.latitude!.toFixed(6)}, ${o.longitude!.toFixed(6)}`, 'font-family:ui-monospace,monospace;color:#6b7280;margin-top:2px');
    const links = document.createElement('div');
    links.style.cssText = 'display:flex;gap:10px;margin-top:6px;font-weight:600';
    for (const [label, href] of [
      [t('platform.reports.mapDirections'), `https://www.google.com/maps/search/?api=1&query=${o.latitude},${o.longitude}`],
      [t('platform.reports.fOpenTenant'), `/platform/tenants/${o.tenant_id}`]
    ]) {
      const a = document.createElement('a');
      a.textContent = label; a.href = href; a.target = '_blank'; a.rel = 'noopener noreferrer';
      links.append(a);
    }
    root.append(links);
    return root;
  }

  function refreshView() {
    if (!map) return;
    const b = map.getBounds();
    inView = items.filter((o) => o.latitude !== null && b.contains([o.latitude, o.longitude!]));
  }

  // circleMarker (bukan ikon gambar bawaan) agar tidak bergantung pada aset yang gagal dibundel Vite.
  function draw(fit = true) {
    if (!map || !layer || !L) return;
    layer.clearLayers();
    markers.clear();
    const pts: [number, number][] = [];
    for (const o of items) {
      if (o.latitude === null || o.longitude === null) continue;
      pts.push([o.latitude, o.longitude]);
      const c = L.circleMarker([o.latitude, o.longitude], {
        radius: 9, weight: 2.5, color: '#fff', fillOpacity: 0.95, fillColor: o.active ? COLOR.on : COLOR.off
      });
      c.bindTooltip(`${o.outlet_name} · ${o.tenant_name}`, { direction: 'top', offset: [0, -6] });
      c.bindPopup(() => popup(o));
      markers.set(o.outlet_id, c);
      c.addTo(layer);
    }
    if (fit && pts.length) map.fitBounds(pts, { padding: [30, 30], maxZoom: 15 });
    refreshView();
  }

  const fitAll = () => draw(true);

  function focus(o: OutletLocation) {
    const m = markers.get(o.outlet_id);
    if (!m || !layer || !map) return;
    // Buka gugus yang menyembunyikan titik ini, lalu popup-nya.
    layer.zoomToShowLayer(m, () => m.openPopup());
  }

  onMount(() => {
    let dead = false;
    void import('leaflet').then(async (m) => {
      if (dead) return;
      L = m.default ?? m;
      (window as unknown as { L: typeof L }).L = L; // plugin cluster membaca L global
      await import('leaflet.markercluster');
      if (dead) return;
      map = L.map(el).setView([-2.5, 118], 5);
      const street = L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', { maxZoom: 19, attribution: '&copy; OpenStreetMap' });
      const sat = L.tileLayer('https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}', { maxZoom: 19, attribution: 'Tiles &copy; Esri' });
      street.addTo(map);
      L.control.layers({ [t('platform.reports.mapStreet')]: street, [t('platform.reports.mapSat')]: sat }, {}, { position: 'topright' }).addTo(map);
      L.control.scale({ imperial: false }).addTo(map);
      // Titik berdekatan digabung; klik gugus = zoom ke area itu, sampai terpisah.
      layer = L.markerClusterGroup({ maxClusterRadius: 50, showCoverageOnHover: false }).addTo(map);
      map.on('moveend', refreshView);
      draw();
    });
    return () => {
      dead = true;
      map?.remove();
    };
  });

  $effect(() => {
    void items;
    draw();
  });
</script>

<div class="space-y-2">
  <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px]">
    <span class="inline-flex items-center gap-1.5"><i class="inline-block size-2.5 rounded-full" style="background:{COLOR.on}"></i>{t('platform.reports.fActive')} <b>{formatNumber(activeCount)}</b></span>
    <span class="inline-flex items-center gap-1.5"><i class="inline-block size-2.5 rounded-full" style="background:{COLOR.off}"></i>{t('platform.reports.inactive')} <b>{formatNumber(items.length - activeCount)}</b></span>
    <button type="button" class="ms-auto text-[var(--color-primary-700)] font-medium hover:underline" onclick={fitAll}>{t('platform.reports.mapFit')}</button>
  </div>
  <div class="grid gap-2 md:grid-cols-[minmax(0,1fr)_250px]">
    <div bind:this={el} class="h-[min(70vh,560px)] w-full rounded-xl overflow-hidden border border-[var(--border)] z-0"></div>
    <div class="rounded-xl border border-[var(--border)] overflow-hidden flex flex-col h-[min(30vh,220px)] md:h-[min(70vh,560px)]">
      <div class="p-2 border-b border-[var(--border-subtle)]">
        <input type="search" bind:value={q} onkeydown={(e) => e.key === 'Enter' && fitMatches()} placeholder={t('platform.reports.fSearch')} aria-label={t('platform.reports.fSearch')}
          class="w-full h-8 px-2 rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px]" />
      </div>
      <div class="px-3 py-1.5 text-[11.5px] font-semibold uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
        {q.trim() ? t('platform.reports.mapResults', { n: formatNumber(matches.length) }) : t('platform.reports.mapInView', { n: formatNumber(inView.length) })}
      </div>
      <ul class="overflow-y-auto scroll-thin divide-y divide-[var(--border-subtle)] flex-1">
        {#each shown.slice(0, VIEW_CAP) as o (o.outlet_id)}
          <li>
            <button type="button" class="w-full text-start px-3 py-2 hover:bg-[var(--surface-sunken)]" onclick={() => focus(o)}>
              <div class="flex items-center gap-1.5 text-[12.5px] font-medium"><i class="inline-block size-2 rounded-full shrink-0" style="background:{o.active ? COLOR.on : COLOR.off}"></i><span class="truncate">{o.outlet_name}</span></div>
              <div class="text-[11.5px] text-[var(--text-tertiary)] truncate">{o.tenant_name}{o.address ? ' · ' + o.address : ''}</div>
            </button>
          </li>
        {/each}
        {#if shown.length > VIEW_CAP}<li class="px-3 py-2 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.reports.mapMore', { n: formatNumber(shown.length - VIEW_CAP) })}</li>{/if}
        {#if q.trim() && matches.length === 0}<li class="px-3 py-3 text-[12px] text-[var(--text-tertiary)]">{t('platform.reports.empty')}</li>{/if}
      </ul>
    </div>
  </div>
</div>
