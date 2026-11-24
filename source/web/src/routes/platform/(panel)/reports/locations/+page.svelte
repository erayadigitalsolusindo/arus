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
  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[720px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="{th} text-start" scope="col">{t('platform.reports.tenant')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.outlet')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.address')}</th>
            <th class="{th} text-start" scope="col">{t('platform.reports.coords')}</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            <tr><td colspan="4" class="px-4 py-3"><div class="h-7 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td></tr>
          {:else if shownLocs.length === 0}
            <tr><td colspan="4" class="px-4 py-10 text-center text-[var(--text-tertiary)]">{t('platform.reports.empty')}</td></tr>
          {:else}
            {#each shownLocs as l (l.outlet_id)}
              <tr class="border-t border-[var(--border-subtle)]">
                <td class="px-4 py-2.5 font-semibold"><a href="/platform/tenants/{l.tenant_id}" class="text-[var(--color-primary-700)] hover:underline">{l.tenant_name}</a></td>
                <td class="px-4 py-2.5">{l.outlet_name}{#if !l.active} <span class="badge-neutral rounded-full px-1.5 py-0.5 text-[10.5px]">{t('platform.reports.inactive')}</span>{/if}</td>
                <td class="px-4 py-2.5 text-[var(--text-secondary)]">{l.address || '—'}</td>
                <td class="px-4 py-2.5">
                  {#if l.latitude !== null && l.longitude !== null}
                    <a href={mapUrl(l)} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 font-mono text-[12px] text-[var(--color-primary-700)] hover:underline"><i class="icon-map-pin text-[13px]"></i>{l.latitude.toFixed(5)}, {l.longitude.toFixed(5)}</a>
                  {:else}
                    <span class="text-[var(--text-tertiary)]">{t('platform.reports.noLoc')}</span>
                  {/if}
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>
