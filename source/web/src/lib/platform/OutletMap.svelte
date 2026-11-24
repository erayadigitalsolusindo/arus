<script lang="ts">
  import { onMount } from 'svelte';
  import type { OutletLocation } from '#lib/platform/api.ts';
  import 'leaflet/dist/leaflet.css';

  let { items }: { items: OutletLocation[] } = $props();
  let el: HTMLDivElement;
  let map: import('leaflet').Map | undefined;
  let layer: import('leaflet').LayerGroup | undefined;
  let L: typeof import('leaflet') | undefined;

  // circleMarker (bukan ikon gambar bawaan) agar tidak bergantung pada aset yang gagal dibundel Vite.
  function draw() {
    if (!map || !layer || !L) return;
    layer.clearLayers();
    const pts: [number, number][] = [];
    for (const o of items) {
      if (o.latitude === null || o.longitude === null) continue;
      pts.push([o.latitude, o.longitude]);
      const c = L.circleMarker([o.latitude, o.longitude], {
        radius: 8, weight: 2, color: '#fff', fillOpacity: 0.9, fillColor: o.active ? '#16a34a' : '#9ca3af'
      });
      const pop = document.createElement('div'); // textContent: nama tenant/outlet dari user, jangan jadi HTML
      const b = document.createElement('b');
      b.textContent = o.tenant_name;
      pop.append(b, document.createElement('br'), document.createTextNode(o.outlet_name));
      c.bindPopup(pop).addTo(layer);
    }
    if (pts.length) map.fitBounds(pts, { padding: [30, 30], maxZoom: 15 });
  }

  onMount(() => {
    let dead = false;
    void import('leaflet').then((m) => {
      if (dead) return;
      L = m.default ?? m;
      map = L.map(el).setView([-2.5, 118], 5);
      L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
        maxZoom: 19, attribution: '&copy; OpenStreetMap'
      }).addTo(map);
      layer = L.layerGroup().addTo(map);
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

<div bind:this={el} class="h-[min(70vh,560px)] w-full rounded-xl overflow-hidden border border-[var(--border)] z-0"></div>
