<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { platformApi, type TenantDetail, type TenantAuditItem } from '#lib/platform/api.ts';
  import { startImpersonation } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const id = $derived(page.params.id ?? '');

  let tenant = $state<TenantDetail | null>(null);
  let loading = $state(true);
  let error = $state('');
  let busy = $state(false);
  let audit = $state<TenantAuditItem[]>([]);
  let auditCursor = $state('');
  let auditLoading = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      tenant = await platformApi.tenant(id);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  async function loadAudit(more = false) {
    auditLoading = true;
    try {
      const res = await platformApi.tenantAudit(id, more ? auditCursor : '');
      audit = more ? [...audit, ...res.items] : res.items;
      auditCursor = res.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      auditLoading = false;
    }
  }

  onMount(() => {
    void load();
    void loadAudit();
  });

  async function enter(outletId = '') {
    busy = true;
    error = '';
    try {
      const res = await platformApi.impersonate(id, outletId);
      startImpersonation(res, id);
      await goto('/dashboard');
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function toggleActive() {
    if (!tenant) return;
    const next = !tenant.active;
    if (!confirm(t(next ? 'platform.tenant.enableConfirm' : 'platform.tenant.disableConfirm', { name: tenant.name }))) return;
    busy = true;
    error = '';
    try {
      await platformApi.setTenantActive(id, next);
      await Promise.all([load(), loadAudit()]);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;
  const activeOutlets = $derived(tenant?.outlets.filter((o) => o.active) ?? []);
</script>

<a href="/platform/tenants" class="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-[var(--color-primary-600)]"><i class="icon-arrow-left text-[13px]"></i>{t('platform.tenant.back')}</a>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

{#if tenant}
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px] flex items-center gap-2">
        {tenant.name}
        <span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {tenant.active ? 'badge-success' : 'badge-danger'}">{tenant.active ? t('platform.tenants.active') : t('platform.tenants.inactive')}</span>
      </h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">
        {t('platform.tenant.code')}: <span class="font-mono">{tenant.code}</span> · {t('platform.tenant.registered')} {formatDate(tenant.created_at, { dateStyle: 'medium' })}
      </p>
    </div>
    <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60 {tenant.active ? '!text-[var(--color-danger-600)]' : ''}" disabled={busy} onclick={toggleActive}>
      {tenant.active ? t('platform.tenant.disable') : t('platform.tenant.enable')}
    </button>
  </div>

  <section class="surface-card p-4 space-y-3">
    <div>
      <h2 class="font-display font-bold text-[14px]">{t('platform.tenant.enterAs')}</h2>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.tenant.enterAsHelp')}</p>
    </div>
    {#if activeOutlets.length === 0}
      <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.tenant.noActiveOutlet')}</p>
    {:else}
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={() => enter()}>
          <i class={busy ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-log-in text-[13px]'}></i>{busy ? t('platform.tenant.entering') : t('platform.tenant.enterAll')}
        </button>
        {#if activeOutlets.length > 1}
          {#each activeOutlets as o (o.id)}
            <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={() => enter(o.id)}>{t('platform.tenant.enterOutlet', { outlet: o.name })}</button>
          {/each}
        {/if}
      </div>
    {/if}
  </section>

  <div class="grid lg:grid-cols-3 gap-4">
    <section class="surface-card !p-0 overflow-hidden lg:col-span-1">
      <h2 class="font-display font-bold text-[14px] p-4 pb-2">{t('platform.tenant.outlets')} ({tenant.outlets.length})</h2>
      <ul>
        {#each tenant.outlets as o (o.id)}
          <li class="flex items-center justify-between gap-2 px-4 py-2.5 border-t border-[var(--border-subtle)] text-[12.5px]">
            <span><span class="font-semibold">{o.name}</span> <span class="font-mono text-[11px] text-[var(--text-tertiary)]">{o.code}</span></span>
            <span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {o.active ? 'badge-success' : 'badge-danger'}">{o.active ? t('platform.tenants.active') : t('platform.tenants.inactive')}</span>
          </li>
        {:else}
          <li class="px-4 py-3 text-[12.5px] text-[var(--text-tertiary)]">{t('platform.tenant.noOutlets')}</li>
        {/each}
      </ul>
    </section>

    <section class="surface-card !p-0 overflow-hidden lg:col-span-2">
      <h2 class="font-display font-bold text-[14px] p-4 pb-2">{t('platform.tenant.users')} ({tenant.users.length})</h2>
      <div class="overflow-x-auto scroll-thin">
        <table class="w-full text-[12.5px] min-w-[560px]">
          <thead>
            <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
              <th class="p-3 text-start" scope="col">{t('platform.tenant.userName')}</th>
              <th class="p-3 text-start" scope="col">{t('platform.tenant.role')}</th>
              <th class="p-3 text-start" scope="col">{t('platform.tenants.lastLogin')}</th>
              <th class="p-3 text-start" scope="col">{t('platform.tenants.status')}</th>
            </tr>
          </thead>
          <tbody>
            {#each tenant.users as u (u.id)}
              <tr class="border-t border-[var(--border-subtle)]">
                <td class="p-3">
                  <span class="font-semibold">{u.name}</span>
                  <span class="block text-[11px] text-[var(--text-tertiary)]">{u.email} · {u.email_verified ? t('platform.tenant.verified') : t('platform.tenant.unverified')}</span>
                </td>
                <td class="p-3">{u.role_name}</td>
                <td class="p-3 whitespace-nowrap">{u.last_login_at ? formatDateTime(u.last_login_at, { dateStyle: 'medium', timeStyle: 'short' }) : t('platform.tenants.never')}</td>
                <td class="p-3"><span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {u.active ? 'badge-success' : 'badge-danger'}">{u.active ? t('platform.tenants.active') : t('platform.tenants.inactive')}</span></td>
              </tr>
            {:else}
              <tr><td colspan="4" class="p-4 text-[var(--text-tertiary)]">{t('platform.tenant.noUsers')}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  </div>

  <section class="surface-card !p-0 overflow-hidden">
    <h2 class="font-display font-bold text-[14px] p-4 pb-2">{t('platform.tenant.auditTitle')}</h2>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[640px]">
        <tbody>
          {#each audit as it (it.id)}
            <tr class="border-t border-[var(--border-subtle)] align-top">
              <td class="p-3 whitespace-nowrap">{formatDateTime(it.created_at, { dateStyle: 'medium', timeStyle: 'medium' })}</td>
              <td class="p-3">{it.actor_name || '—'}</td>
              <td class="p-3 font-semibold">{actionLabel(it.action)}</td>
              <td class="p-3 font-mono text-[11.5px]">{it.ip}</td>
            </tr>
          {:else}
            <tr><td colspan="4" class="p-4 text-[var(--text-tertiary)]">{t('platform.tenant.auditEmpty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if auditCursor}
      <div class="text-center p-3">
        <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={auditLoading} onclick={() => loadAudit(true)}>{t('platform.tenant.loadMore')}</button>
      </div>
    {/if}
  </section>
{:else if loading}
  <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.tenants.loading')}</p>
{/if}
