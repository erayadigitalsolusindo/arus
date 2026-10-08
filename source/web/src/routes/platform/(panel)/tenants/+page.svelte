<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { platformApi, type TenantPage } from '#lib/platform/api.ts';
  import { t, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Avatar from '#lib/platform/Avatar.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import StatusPill from '#lib/platform/StatusPill.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  const LIMIT = 25;
  let q = $state('');
  let offset = $state(0);
  let data = $state<TenantPage | null>(null);
  let loading = $state(true);
  let error = $state('');
  let seq = 0;

  async function load() {
    const my = ++seq; // abaikan respons usang saat pengguna mengetik cepat
    loading = true;
    error = '';
    try {
      const res = await platformApi.tenants(q.trim(), LIMIT, offset);
      if (my === seq) data = res;
    } catch (err) {
      if (my === seq) error = errorMessage(err);
    } finally {
      if (my === seq) loading = false;
    }
  }
  onMount(load);

  let debounce: ReturnType<typeof setTimeout>;
  function onSearch() {
    clearTimeout(debounce);
    debounce = setTimeout(() => {
      offset = 0;
      void load();
    }, 300);
  }

  function go(delta: number) {
    offset = Math.max(0, offset + delta);
    void load();
  }

  const total = $derived(data?.total ?? 0);
  const rows = $derived(data?.items ?? []);
  // Ringkasan dihitung dari baris yang sedang tampil (halaman ini), bukan seluruh tenant — lihat hint kartu.
  const activeCount = $derived(rows.filter((r) => r.active).length);
  const outletSum = $derived(rows.reduce((s, r) => s + r.outlet_count, 0));
  const userSum = $derived(rows.reduce((s, r) => s + r.user_count, 0));
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
  <div>
    <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.tenants.title')}</h1>
    <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.tenants.subtitle')}</p>
  </div>
</div>

<div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
  <StatCard label={t('platform.tenants.statTotal')} value={formatNumber(total)} icon="store" tone="primary" />
  <StatCard label={t('platform.tenants.active')} value={formatNumber(activeCount)} icon="circle-check" tone="success" hint={t('platform.tenants.thisPage')} />
  <StatCard label={t('platform.tenants.outlets')} value={formatNumber(outletSum)} icon="map-pin" tone="info" hint={t('platform.tenants.thisPage')} />
  <StatCard label={t('platform.tenants.users')} value={formatNumber(userSum)} icon="users" tone="warning" hint={t('platform.tenants.thisPage')} />
</div>

<div class="surface-card !p-0 overflow-hidden">
  <div class="p-3 sm:p-4 flex flex-wrap items-center gap-3 border-b border-[var(--border-subtle)]">
    <div class="relative flex-1 min-w-60">
      <i class="icon-search absolute top-1/2 -translate-y-1/2 start-3 text-[13px] text-[var(--text-tertiary)]"></i>
      <input type="search" bind:value={q} oninput={onSearch} placeholder={t('platform.tenants.search')} aria-label={t('platform.tenants.search')} class="w-full field-control !ps-8" />
    </div>
    <span class="text-[12px] font-medium text-[var(--text-tertiary)]">{t('platform.tenants.total', { count: total })}</span>
  </div>

  {#if error}
    <div role="alert" class="m-3 flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[900px]">
      <thead>
        <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.name')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.owner')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.outlets')} / {t('platform.tenants.users')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.lastLogin')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.registered')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.status')}</th>
          <th class="w-8" scope="col"><span class="sr-only">{t('platform.tenants.open')}</span></th>
        </tr>
      </thead>
      <tbody>
        {#if loading && rows.length === 0}
          {#each [0, 1, 2, 3, 4] as i (i)}
            <tr class="border-t border-[var(--border-subtle)]" aria-hidden="true">
              <td class="px-4 py-3.5" colspan="7"><div class="h-9 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td>
            </tr>
          {/each}
        {:else}
          {#each rows as r (r.id)}
            <tr
              class="group border-t border-[var(--border-subtle)] cursor-pointer transition-colors hover:bg-[var(--color-primary-50)]/60 {loading ? 'opacity-60' : ''}"
              onclick={() => goto(`/platform/tenants/${r.id}`)}
            >
              <td class="px-4 py-3">
                <div class="flex items-center gap-3">
                  <Avatar name={r.name} size={38} square />
                  <div class="min-w-0">
                    <a href="/platform/tenants/{r.id}" class="block font-semibold truncate max-w-64 text-[var(--color-primary-700)] hover:underline" onclick={(e) => e.stopPropagation()}>{r.name}</a>
                    <span class="block text-[11px] font-mono text-[var(--text-tertiary)]">{r.code}</span>
                  </div>
                </div>
              </td>
              <td class="px-4 py-3 text-[var(--text-secondary,inherit)]">{r.owner_email || '—'}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-1.5">
                  <span class="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px] font-semibold tabular-nums" title={t('platform.tenants.outlets')}><i class="icon-map-pin text-[12px] text-[var(--text-tertiary)]"></i>{r.outlet_count}</span>
                  <span class="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px] font-semibold tabular-nums" title={t('platform.tenants.users')}><i class="icon-users text-[12px] text-[var(--text-tertiary)]"></i>{r.user_count}</span>
                </div>
              </td>
              <td class="px-4 py-3 whitespace-nowrap">
                {#if r.last_login_at}
                  <span title={formatDateTime(r.last_login_at, { dateStyle: 'medium', timeStyle: 'short' })}>{relativeTime(r.last_login_at)}</span>
                {:else}
                  <span class="text-[var(--text-tertiary)]">{t('platform.tenants.never')}</span>
                {/if}
              </td>
              <td class="px-4 py-3 whitespace-nowrap text-[var(--text-secondary,inherit)]">{formatDate(r.created_at, { dateStyle: 'medium' })}</td>
              <td class="px-4 py-3"><StatusPill ok={r.active} label={r.active ? t('platform.tenants.active') : t('platform.tenants.inactive')} /></td>
              <td class="pe-3 text-[var(--text-tertiary)] group-hover:text-[var(--color-primary-600)]"><i class="icon-chevron-right text-[15px]"></i></td>
            </tr>
          {:else}
            <tr>
              <td colspan="7" class="px-4 py-14 text-center">
                <span class="mx-auto mb-3 grid size-12 place-items-center rounded-full bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-store text-[22px]"></i></span>
                <p class="font-semibold">{t('platform.tenants.empty')}</p>
              </td>
            </tr>
          {/each}
        {/if}
      </tbody>
    </table>
  </div>

  {#if total > LIMIT}
    <div class="flex items-center justify-between gap-3 p-3 border-t border-[var(--border-subtle)]">
      <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading || offset === 0} onclick={() => go(-LIMIT)}><i class="icon-chevron-left text-[13px]"></i>{t('platform.tenants.prev')}</button>
      <span class="text-[12px] text-[var(--text-tertiary)] tabular-nums">{offset + 1}–{Math.min(offset + LIMIT, total)} / {total}</span>
      <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading || offset + LIMIT >= total} onclick={() => go(LIMIT)}>{t('platform.tenants.next')}<i class="icon-chevron-right text-[13px]"></i></button>
    </div>
  {/if}
</div>
