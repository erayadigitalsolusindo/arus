<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { platformApi, type TenantDetail, type TenantAuditItem } from '#lib/platform/api.ts';
  import { startImpersonation } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatDate, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Avatar from '#lib/platform/Avatar.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import StatusPill from '#lib/platform/StatusPill.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  const id = $derived(page.params.id ?? '');

  let tenant = $state<TenantDetail | null>(null);
  let loading = $state(true);
  let error = $state('');
  let busy = $state(false);
  let audit = $state<TenantAuditItem[]>([]);
  let auditCursor = $state('');
  let auditLoading = $state(false);
  let windowDays = $state('0');
  let savingWindow = $state(false);
  let windowSaved = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      tenant = await platformApi.tenant(id);
      windowDays = String(tenant.sale_edit_window_days);
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

  const windowValid = $derived(/^[0-9]{1,4}$/.test(windowDays.trim()) && Number(windowDays) <= 3650);
  const windowDirty = $derived(!!tenant && windowValid && Number(windowDays) !== tenant.sale_edit_window_days);
  async function saveWindow() {
    if (!tenant || !windowValid || savingWindow) return;
    savingWindow = true;
    error = '';
    windowSaved = false;
    try {
      await platformApi.setSaleEditWindow(id, Number(windowDays));
      await Promise.all([load(), loadAudit()]);
      windowSaved = true;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      savingWindow = false;
    }
  }

  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;
  const activeOutlets = $derived(tenant?.outlets.filter((o) => o.active) ?? []);
  const owner = $derived(tenant?.users.find((u) => u.role_name === 'Owner') ?? tenant?.users[0]);
  const lastLogin = $derived(
    tenant?.users.reduce<string | null>((best, u) => (u.last_login_at && (!best || u.last_login_at > best) ? u.last_login_at : best), null) ?? null
  );
</script>

<a href="/platform/tenants" class="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-[var(--text-tertiary)] hover:text-[var(--color-primary-600)]"><i class="icon-arrow-left text-[13px]"></i>{t('platform.tenant.back')}</a>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

{#if tenant}
  <!-- Hero -->
  <section class="surface-card !p-0 overflow-hidden">
    <div class="h-20 bg-gradient-to-r from-[var(--color-primary-600)] to-[var(--color-primary-400)]"></div>
    <div class="px-5 pb-5 -mt-9 flex flex-wrap items-end justify-between gap-4">
      <div class="flex items-end gap-4 min-w-0">
        <div class="rounded-2xl ring-4 ring-[var(--surface-base)]"><Avatar name={tenant.name} size={72} square /></div>
        <div class="min-w-0 pb-1">
          <h1 class="font-display font-bold text-[21px] tracking-tight flex flex-wrap items-center gap-2.5">
            <span class="truncate">{tenant.name}</span>
            <StatusPill ok={tenant.active} label={tenant.active ? t('platform.tenants.active') : t('platform.tenants.inactive')} />
          </h1>
          <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">
            {t('platform.tenant.code')}: <span class="font-mono">{tenant.code}</span> · {t('platform.tenant.registered')} {formatDate(tenant.created_at, { dateStyle: 'medium' })}{#if owner} · {owner.email}{/if}
          </p>
        </div>
      </div>
      <div class="flex flex-wrap gap-2 pb-1">
        {#if activeOutlets.length > 0}
          <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={() => enter()}>
            <i class={busy ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-log-in text-[13px]'}></i>{busy ? t('platform.tenant.entering') : t('platform.tenant.enterAll')}
          </button>
        {/if}
        <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60 {tenant.active ? '!text-[var(--color-danger-600)]' : ''}" disabled={busy} onclick={toggleActive}>
          <i class="{tenant.active ? 'icon-ban' : 'icon-circle-check'} text-[13px]"></i>{tenant.active ? t('platform.tenant.disable') : t('platform.tenant.enable')}
        </button>
      </div>
    </div>
  </section>

  <!-- KPI -->
  <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
    <StatCard label={t('platform.tenant.outlets')} value={formatNumber(tenant.outlets.length)} icon="map-pin" tone="info" hint="{activeOutlets.length} {t('platform.tenants.active').toLowerCase()}" />
    <StatCard label={t('platform.tenant.users')} value={formatNumber(tenant.users.length)} icon="users" tone="primary" hint="{tenant.users.filter((u) => u.active).length} {t('platform.tenants.active').toLowerCase()}" />
    <StatCard label={t('platform.tenants.lastLogin')} value={lastLogin ? relativeTime(lastLogin) : t('platform.tenants.never')} icon="clock" tone="success" />
    <StatCard label={t('platform.tenant.editWindow')} value={tenant.sale_edit_window_days === 0 ? t('platform.tenant.sameDayShort') : t('platform.tenant.daysShort', { days: tenant.sale_edit_window_days })} icon="file-pen-line" tone="warning" />
  </div>

  <div class="grid lg:grid-cols-3 gap-4 items-start">
    <div class="lg:col-span-2 space-y-4">
      <!-- Pengguna -->
      <section class="surface-card !p-0 overflow-hidden">
        <h2 class="font-display font-bold text-[14px] px-4 py-3.5 flex items-center gap-2"><i class="icon-users text-[16px] text-[var(--text-tertiary)]"></i>{t('platform.tenant.users')} <span class="rounded-full bg-[var(--surface-sunken)] px-2 py-0.5 text-[11px] font-semibold">{tenant.users.length}</span></h2>
        <div class="overflow-x-auto scroll-thin">
          <table class="w-full text-[12.5px] min-w-[560px]">
            <thead>
              <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
                <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenant.userName')}</th>
                <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenant.role')}</th>
                <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.lastLogin')}</th>
                <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.status')}</th>
              </tr>
            </thead>
            <tbody>
              {#each tenant.users as u (u.id)}
                <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
                  <td class="px-4 py-3">
                    <div class="flex items-center gap-3">
                      <Avatar name={u.name} size={32} />
                      <div class="min-w-0">
                        <span class="block font-semibold truncate">{u.name}</span>
                        <span class="flex items-center gap-1.5 text-[11px] text-[var(--text-tertiary)]">
                          <span class="truncate">{u.email}</span>
                          <i class="{u.email_verified ? 'icon-badge-check text-[var(--color-success-600)]' : 'icon-circle-alert text-[var(--color-warning-600)]'} text-[13px] shrink-0" title={u.email_verified ? t('platform.tenant.verified') : t('platform.tenant.unverified')}></i>
                        </span>
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-3"><span class="rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px] font-semibold">{u.role_name}</span></td>
                  <td class="px-4 py-3 whitespace-nowrap">
                    {#if u.last_login_at}<span title={formatDateTime(u.last_login_at, { dateStyle: 'medium', timeStyle: 'short' })}>{relativeTime(u.last_login_at)}</span>{:else}<span class="text-[var(--text-tertiary)]">{t('platform.tenants.never')}</span>{/if}
                  </td>
                  <td class="px-4 py-3"><StatusPill ok={u.active} label={u.active ? t('platform.tenants.active') : t('platform.tenants.inactive')} /></td>
                </tr>
              {:else}
                <tr><td colspan="4" class="p-6 text-center text-[var(--text-tertiary)]">{t('platform.tenant.noUsers')}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <!-- Aktivitas -->
      <section class="surface-card !p-0 overflow-hidden">
        <h2 class="font-display font-bold text-[14px] px-4 py-3.5 flex items-center gap-2"><i class="icon-history text-[16px] text-[var(--text-tertiary)]"></i>{t('platform.tenant.auditTitle')}</h2>
        {#if audit.length === 0}
          <p class="px-4 pb-6 pt-2 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('platform.tenant.auditEmpty')}</p>
        {:else}
          <ol class="px-4 pb-2">
            {#each audit as it, i (it.id)}
              <li class="relative flex gap-3 pb-4">
                {#if i < audit.length - 1}<span class="absolute start-[13px] top-7 bottom-0 w-px bg-[var(--border-subtle)]"></span>{/if}
                <span class="relative z-10 grid size-7 shrink-0 place-items-center rounded-full bg-[var(--color-primary-50)] text-[var(--color-primary-600)]"><i class="icon-activity text-[13px]"></i></span>
                <div class="min-w-0 flex-1">
                  <p class="text-[12.5px]"><span class="font-semibold">{actionLabel(it.action)}</span></p>
                  <p class="text-[11.5px] text-[var(--text-tertiary)]">{it.actor_name || '—'} · {formatDateTime(it.created_at, { dateStyle: 'medium', timeStyle: 'medium' })} · <span class="font-mono">{it.ip}</span></p>
                </div>
              </li>
            {/each}
          </ol>
        {/if}
        {#if auditCursor}
          <div class="text-center p-3 border-t border-[var(--border-subtle)]">
            <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={auditLoading} onclick={() => loadAudit(true)}>{t('platform.tenant.loadMore')}</button>
          </div>
        {/if}
      </section>
    </div>

    <div class="space-y-4">
      <!-- Outlet -->
      <section class="surface-card !p-0 overflow-hidden">
        <h2 class="font-display font-bold text-[14px] px-4 py-3.5 flex items-center gap-2"><i class="icon-map-pin text-[16px] text-[var(--text-tertiary)]"></i>{t('platform.tenant.outlets')} <span class="rounded-full bg-[var(--surface-sunken)] px-2 py-0.5 text-[11px] font-semibold">{tenant.outlets.length}</span></h2>
        <ul>
          {#each tenant.outlets as o (o.id)}
            <li class="flex items-center justify-between gap-2 px-4 py-2.5 border-t border-[var(--border-subtle)] text-[12.5px]">
              <div class="min-w-0">
                <span class="block font-semibold truncate">{o.name}</span>
                <span class="font-mono text-[11px] text-[var(--text-tertiary)]">{o.code}</span>
              </div>
              <div class="flex items-center gap-2 shrink-0">
                <StatusPill ok={o.active} label={o.active ? t('platform.tenants.active') : t('platform.tenants.inactive')} />
                {#if o.active && activeOutlets.length > 1}
                  <button type="button" class="header-icon-btn !size-8 disabled:opacity-60" disabled={busy} title={t('platform.tenant.enterOutlet', { outlet: o.name })} aria-label={t('platform.tenant.enterOutlet', { outlet: o.name })} onclick={() => enter(o.id)}>
                    <i class="icon-log-in text-[14px]"></i>
                  </button>
                {/if}
              </div>
            </li>
          {:else}
            <li class="px-4 py-4 border-t border-[var(--border-subtle)] text-[12.5px] text-[var(--text-tertiary)]">{t('platform.tenant.noOutlets')}</li>
          {/each}
        </ul>
        {#if activeOutlets.length === 0 && tenant.outlets.length > 0}
          <p class="px-4 py-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">{t('platform.tenant.noActiveOutlet')}</p>
        {/if}
      </section>

      <!-- Batas edit nota -->
      <section class="surface-card p-4 space-y-3">
        <div>
          <h2 class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-file-pen-line text-[16px] text-[var(--text-tertiary)]"></i>{t('platform.tenant.editWindow')}</h2>
          <p class="text-[12px] mt-1.5 text-[var(--text-tertiary)]">{t('platform.tenant.editWindowHelp')}</p>
        </div>
        <form
          class="flex flex-wrap items-end gap-2"
          onsubmit={(e) => {
            e.preventDefault();
            void saveWindow();
          }}
        >
          <label class="block">
            <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenant.editWindowDays')}</span>
            <input class="field-control w-32 text-end tabular-nums" inputmode="numeric" maxlength="4" bind:value={windowDays} oninput={() => (windowSaved = false)} aria-invalid={!windowValid} />
          </label>
          <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={!windowDirty || savingWindow}>{savingWindow ? t('platform.tenant.editWindowSaving') : t('platform.tenant.editWindowSave')}</button>
        </form>
        {#if windowSaved}<p class="text-[12px] font-semibold text-[var(--color-success-600)]"><i class="icon-circle-check me-1"></i>{t('platform.tenant.editWindowSaved')}</p>{/if}
        {#if !windowValid}<p class="text-[12px] text-[var(--color-danger-600)]">{t('platform.tenant.editWindowInvalid')}</p>{/if}
        <p class="text-[11.5px] text-[var(--text-tertiary)]">{Number(windowDays) === 0 ? t('platform.tenant.editWindowSameDay') : t('platform.tenant.editWindowDaysHint', { days: windowDays })}</p>
      </section>

      <p class="text-[11.5px] text-[var(--text-tertiary)] px-1"><i class="icon-info me-1"></i>{t('platform.tenant.enterAsHelp')}</p>
    </div>
  </div>
{:else if loading}
  <div class="space-y-4" aria-busy="true">
    <div class="h-40 rounded-2xl bg-[var(--surface-base)] animate-pulse"></div>
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">{#each [0, 1, 2, 3] as i (i)}<div class="h-20 rounded-2xl bg-[var(--surface-base)] animate-pulse"></div>{/each}</div>
  </div>
{/if}
