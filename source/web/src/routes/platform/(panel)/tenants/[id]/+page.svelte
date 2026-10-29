<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { platformApi, type DeleteSchedule, type TenantDetail, type TenantAuditItem, type TenantPlan, type PlanRow } from '#lib/platform/api.ts';
  import Select from '#lib/components/Select.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
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

  // ---- Paket langganan ----
  let tplan = $state<TenantPlan | null>(null);
  let planList = $state<PlanRow[]>([]);
  let planChoice = $state(''); // '' = paket default (tanpa langganan manual)
  let planExpires = $state(''); // datetime-local; kosong = tanpa kedaluwarsa
  let planTx = $state('');
  let planUsers = $state('');
  let planNote = $state('');
  let savingPlan = $state(false);
  let planSaved = $state(false);

  const pad2 = (n: number) => String(n).padStart(2, '0');
  const ymd = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
  const toLocal = (iso: string | null) => (iso ? ymd(new Date(iso)) : '');
  const inDays = (n: number) => ymd(new Date(Date.now() + n * 86_400_000));
  async function loadPlan() {
    try {
      const [tp, pl] = await Promise.all([platformApi.tenantPlan(id), platformApi.plans()]);
      tplan = tp;
      planList = pl.items;
      const sub = tp.subscription;
      planChoice = sub ? sub.plan_id : '';
      planExpires = toLocal(sub?.expires_at ?? null);
      planTx = sub?.override_max_tx_per_day == null ? '' : String(sub.override_max_tx_per_day);
      planUsers = sub?.override_max_users == null ? '' : String(sub.override_max_users);
      planNote = sub?.note ?? '';
    } catch (err) {
      error = errorMessage(err);
    }
  }
  const planOptions = $derived([
    { value: '', label: t('platform.tenantPlan.useDefault') },
    ...planList.map((p) => ({ value: p.id, label: p.name + (p.active ? '' : ' (' + t('platform.plans.inactive') + ')') }))
  ]);
  const limitText = (n: number | null) => (n === null ? t('platform.tenantPlan.unlimited') : formatNumber(n));
  async function savePlan() {
    if (savingPlan) return;
    savingPlan = true;
    error = '';
    planSaved = false;
    const num = (v: string) => (v.trim() === '' ? null : Number(v));
    try {
      await platformApi.setTenantPlan(id, {
        plan_id: planChoice || null,
        expires_at: planChoice && planExpires ? new Date(`${planExpires}T23:59:59`).toISOString() : null,
        override_max_tx_per_day: planChoice ? num(planTx) : null,
        override_max_users: planChoice ? num(planUsers) : null,
        note: planChoice ? planNote.trim() : ''
      });
      await Promise.all([loadPlan(), loadAudit()]);
      planSaved = true;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      savingPlan = false;
    }
  }

  onMount(() => {
    void load();
    void loadPlan();
    void loadAudit();
    void loadDeletion();
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
      await Promise.all([load(), loadAudit(), loadDeletion()]);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  // ---- Hapus tenant permanen ----
  let delSchedule = $state<DeleteSchedule | null>(null);
  let delDays = $state('30');
  let delConfirm = $state('');
  let delBusy = $state(false);

  async function loadDeletion() {
    try {
      delSchedule = (await platformApi.deleteSchedule(id)).schedule;
    } catch (err) {
      error = errorMessage(err);
    }
  }
  const delDaysValid = $derived(/^[0-9]{1,3}$/.test(delDays.trim()) && Number(delDays) <= 365);
  async function scheduleDelete() {
    if (!delDaysValid || delBusy) return;
    delBusy = true;
    error = '';
    try {
      await platformApi.scheduleDelete(id, Number(delDays));
      await Promise.all([loadDeletion(), loadAudit()]);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      delBusy = false;
    }
  }
  async function cancelDelete() {
    if (delBusy) return;
    delBusy = true;
    error = '';
    try {
      await platformApi.cancelDelete(id);
      await Promise.all([loadDeletion(), loadAudit()]);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      delBusy = false;
    }
  }
  async function purgeNow() {
    if (!tenant || delBusy || delConfirm.trim() !== tenant.name) return;
    delBusy = true;
    error = '';
    try {
      await platformApi.purgeTenant(id, delConfirm.trim());
      await goto('/platform/tenants');
    } catch (err) {
      error = errorMessage(err);
      delBusy = false;
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

      <!-- Paket langganan -->
      <section class="surface-card p-4 space-y-3">
        <div>
          <h2 class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-badge-dollar-sign text-[16px] text-[var(--text-tertiary)]"></i>{t('platform.tenantPlan.title')}</h2>
          <p class="text-[12px] mt-1.5 text-[var(--text-tertiary)]">{t('platform.tenantPlan.subtitle')}</p>
        </div>
        {#if tplan}
          <div class="rounded-lg bg-[var(--surface-sunken)] p-3 text-[12.5px] space-y-1">
            <div class="flex items-center justify-between gap-2">
              <span class="text-[var(--text-tertiary)]">{t('platform.tenantPlan.effective')}</span>
              <span class="font-semibold">{tplan.effective.name}</span>
            </div>
            <div class="flex items-center justify-between gap-2">
              <span class="text-[var(--text-tertiary)]">{t('platform.tenantPlan.txToday')}</span>
              <span class="tabular-nums">{t('platform.tenantPlan.usedOf', { used: formatNumber(tplan.usage.tx_today), limit: limitText(tplan.effective.max_tx_per_day) })}</span>
            </div>
            <div class="flex items-center justify-between gap-2">
              <span class="text-[var(--text-tertiary)]">{t('platform.tenantPlan.activeUsers')}</span>
              <span class="tabular-nums">{t('platform.tenantPlan.usedOf', { used: formatNumber(tplan.usage.users), limit: limitText(tplan.effective.max_users) })}</span>
            </div>
            <p class="text-[11.5px] text-[var(--text-tertiary)]">
              {#if tplan.subscription?.expired}{t('platform.tenantPlan.expiredNote')}
              {:else if tplan.effective.is_default}{t('platform.tenantPlan.defaultNote')}
              {:else if tplan.effective.expires_at}{t('platform.tenantPlan.expiresOn', { date: formatDateTime(tplan.effective.expires_at, { dateStyle: 'medium', timeStyle: 'short' }) })}
              {:else}{t('platform.tenantPlan.noExpiry')}{/if}
            </p>
          </div>
          <form
            class="space-y-3"
            onsubmit={(e) => {
              e.preventDefault();
              void savePlan();
            }}
          >
            <div>
              <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenantPlan.plan')}</span>
              <Select bind:value={planChoice} options={planOptions} ariaLabel={t('platform.tenantPlan.plan')} onchange={() => (planSaved = false)} />
            </div>
            {#if planChoice}
              <label class="block">
                <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenantPlan.expires')}</span>
                <DatePicker bind:value={planExpires} min={inDays(1)} placeholder={t('platform.tenantPlan.noExpiry')} />
                <div class="mt-2 flex flex-wrap gap-1.5">
                  {#each [30, 90, 365] as d (d)}
                    <button type="button" class="rounded-full border border-[var(--border-subtle)] px-2.5 py-0.5 text-[11.5px] font-semibold text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] {planExpires === inDays(d) ? '!bg-[var(--color-primary-600)] !border-[var(--color-primary-600)] !text-white' : ''}" onclick={() => { planExpires = inDays(d); planSaved = false; }}>{t('platform.plans.days', { count: d })}</button>
                  {/each}
                </div>
                <span class="mt-1 block text-[11px] text-[var(--text-tertiary)]">{t('platform.tenantPlan.expiresHelp')}</span>
              </label>
              <div class="grid grid-cols-2 gap-2">
                <label class="block">
                  <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenantPlan.overrideTx')}</span>
                  <input class="field-control w-full text-end tabular-nums" inputmode="numeric" bind:value={planTx} oninput={() => (planSaved = false)} />
                </label>
                <label class="block">
                  <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenantPlan.overrideUsers')}</span>
                  <input class="field-control w-full text-end tabular-nums" inputmode="numeric" bind:value={planUsers} oninput={() => (planSaved = false)} />
                </label>
              </div>
              <p class="-mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.tenantPlan.overrideHelp')}</p>
              <label class="block">
                <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenantPlan.note')}</span>
                <input class="field-control w-full" maxlength="300" bind:value={planNote} oninput={() => (planSaved = false)} />
              </label>
            {/if}
            <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={savingPlan}>{savingPlan ? t('platform.tenantPlan.saving') : t('platform.tenantPlan.save')}</button>
            {#if planSaved}<p class="text-[12px] font-semibold text-[var(--color-success-600)]"><i class="icon-circle-check me-1"></i>{t('platform.tenantPlan.saved')}</p>{/if}
          </form>
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

      <!-- Hapus tenant permanen -->
      <section class="surface-card space-y-3 border border-[var(--color-danger-200,var(--border-subtle))]">
        <h2 class="font-display font-bold text-[14px] flex items-center gap-2 text-[var(--color-danger-600)]"><i class="icon-trash-2 text-[16px]"></i>{t('platform.tenant.deleteTitle')}</h2>
        <p class="text-[12px] text-[var(--text-secondary)]">{t('platform.tenant.deleteHelp')}</p>
        {#if tenant.active}
          <p class="text-[12px] font-semibold text-[var(--text-tertiary)]"><i class="icon-info me-1"></i>{t('platform.tenant.deleteNeedInactive')}</p>
        {:else}
          {#if delSchedule}
            <p class="text-[12.5px] font-semibold text-[var(--color-danger-600)]">
              {delSchedule.due ? t('platform.tenant.deleteDueNow') : t('platform.tenant.deleteScheduledOn', { date: formatDateTime(delSchedule.delete_at, { dateStyle: 'medium', timeStyle: 'short' }) })}
            </p>
          {/if}
          <form
            class="flex flex-wrap items-end gap-2"
            onsubmit={(e) => {
              e.preventDefault();
              void scheduleDelete();
            }}
          >
            <label class="block">
              <span class="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('platform.tenant.deleteDaysLabel')}</span>
              <input class="field-control w-28 text-end tabular-nums" inputmode="numeric" maxlength="3" bind:value={delDays} aria-invalid={!delDaysValid} />
            </label>
            <button type="submit" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={!delDaysValid || delBusy}>{delSchedule ? t('platform.tenant.deleteReschedule') : t('platform.tenant.deleteSchedule')}</button>
            {#if delSchedule}
              <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={delBusy} onclick={cancelDelete}>{t('platform.tenant.deleteCancel')}</button>
            {/if}
          </form>
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.tenant.deleteDaysHint')}</p>
          {#if delSchedule?.due}
            <form
              class="space-y-2 border-t border-[var(--border-subtle)] pt-3"
              onsubmit={(e) => {
                e.preventDefault();
                void purgeNow();
              }}
            >
              <label class="block">
                <span class="mb-1 block text-[12px] font-semibold">{t('platform.tenant.deleteConfirmName', { name: tenant.name })}</span>
                <input class="field-control w-full" bind:value={delConfirm} autocomplete="off" />
              </label>
              <button type="submit" class="btn btn-primary !bg-[var(--color-danger-600)] !text-[12.5px] disabled:opacity-60" disabled={delBusy || delConfirm.trim() !== tenant.name}>
                <i class="icon-trash-2 text-[13px]"></i>{t('platform.tenant.deleteNow')}
              </button>
            </form>
          {/if}
        {/if}
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
