<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type TenantPage } from '#lib/platform/api.ts';
  import { t, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

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
</script>

<div>
  <h1 class="font-display font-bold text-[19px]">{t('platform.tenants.title')}</h1>
  <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.tenants.subtitle')}</p>
</div>

<div class="surface-card p-3 flex flex-wrap items-center gap-3">
  <div class="relative flex-1 min-w-60">
    <i class="icon-search absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-[var(--text-tertiary)]"></i>
    <input type="search" bind:value={q} oninput={onSearch} placeholder={t('platform.tenants.search')} aria-label={t('platform.tenants.search')} class="w-full ps-9 pe-3 py-2 field-control" />
  </div>
  <span class="text-[12px] text-[var(--text-tertiary)]">{t('platform.tenants.total', { count: total })}</span>
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

<div class="surface-card !p-0 overflow-hidden">
  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[860px]">
      <thead>
        <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
          <th class="p-3 text-start" scope="col">{t('platform.tenants.name')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.tenants.owner')}</th>
          <th class="p-3 text-end" scope="col">{t('platform.tenants.outlets')}</th>
          <th class="p-3 text-end" scope="col">{t('platform.tenants.users')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.tenants.lastLogin')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.tenants.registered')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.tenants.status')}</th>
        </tr>
      </thead>
      <tbody>
        {#each data?.items ?? [] as r (r.id)}
          <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
            <td class="p-3">
              <a href="/platform/tenants/{r.id}" class="font-semibold text-[var(--color-primary-600)]">{r.name}</a>
              <span class="block text-[11px] font-mono text-[var(--text-tertiary)]">{r.code}</span>
            </td>
            <td class="p-3">{r.owner_email || '—'}</td>
            <td class="p-3 text-end">{r.outlet_count}</td>
            <td class="p-3 text-end">{r.user_count}</td>
            <td class="p-3 whitespace-nowrap">{r.last_login_at ? formatDateTime(r.last_login_at, { dateStyle: 'medium', timeStyle: 'short' }) : t('platform.tenants.never')}</td>
            <td class="p-3 whitespace-nowrap">{formatDate(r.created_at, { dateStyle: 'medium' })}</td>
            <td class="p-3"><span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {r.active ? 'badge-success' : 'badge-danger'}">{r.active ? t('platform.tenants.active') : t('platform.tenants.inactive')}</span></td>
          </tr>
        {:else}
          <tr><td colspan="7" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? t('platform.tenants.loading') : t('platform.tenants.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

{#if total > LIMIT}
  <div class="flex items-center justify-between">
    <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading || offset === 0} onclick={() => go(-LIMIT)}>{t('platform.tenants.prev')}</button>
    <span class="text-[12px] text-[var(--text-tertiary)]">{offset + 1}–{Math.min(offset + LIMIT, total)} / {total}</span>
    <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading || offset + LIMIT >= total} onclick={() => go(LIMIT)}>{t('platform.tenants.next')}</button>
  </div>
{/if}
