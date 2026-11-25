<script lang="ts">
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { platformApi, type TenantPage, type PlanRow, type TenantFilter, type AdminRow } from '#lib/platform/api.ts';
  import { canPlatform } from '#lib/platform/session.svelte.ts';
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

  // Filter lanjutan
  let showFilter = $state(false);
  let fStatus = $state('');
  let fPlan = $state('');
  let fLogin = $state('');
  let fFrom = $state('');
  let fTo = $state('');
  let fType = $state('');
  let fLoc = $state('');
  let types = $state<{ type: string; count: number }[]>([]);
  let locations = $state<{ location: string; count: number }[]>([]);
  const filterCount = $derived([fStatus, fPlan, fLogin, fFrom, fTo, fType, fLoc].filter(Boolean).length);
  const filters = (): TenantFilter => ({ status: fStatus, plan: fPlan, login: fLogin, from: fFrom, to: fTo, type: fType, location: fLoc });
  // DatePicker tidak punya onchange: terapkan filter saat rentang tanggal berubah (tanpa memicu pada muat awal).
  let lastRange = '|';
  $effect(() => {
    const k = `${fFrom}|${fTo}`;
    if (k === lastRange) return;
    lastRange = k;
    untrack(applyFilter);
  });
  function applyFilter() {
    offset = 0;
    selected = new Set();
    void load();
  }
  function resetFilter() {
    fStatus = fPlan = fLogin = fFrom = fTo = fType = fLoc = '';
    applyFilter();
  }

  // Ubah paket massal
  let plans = $state<PlanRow[]>([]);
  let selected = $state(new Set<string>());
  let showPlan = $state(false);
  let bulkPlan = $state(''); // '' = paket default
  let bulkExpires = $state('');
  let bulkBusy = $state(false);
  let bulkMsg = $state('');
  function toggle(id: string) {
    const n = new Set(selected);
    if (!n.delete(id)) n.add(id);
    selected = n;
  }
  function toggleAll() {
    selected = allChecked ? new Set() : new Set(rows.map((r) => r.id));
  }
  async function applyPlan() {
    bulkBusy = true;
    bulkMsg = '';
    error = '';
    try {
      const res = await platformApi.bulkSetPlan([...selected], bulkPlan || null, bulkPlan && bulkExpires ? new Date(bulkExpires + 'T23:59:59+07:00').toISOString() : null);
      bulkMsg = t('platform.tenants.bulkPlan.done', { ok: res.updated, failed: res.failed.length });
      selected = new Set(res.failed);
      showPlan = res.failed.length > 0;
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      bulkBusy = false;
    }
  }

  // Atur akses admin: tenant terpilih ditambahkan/dicabut dari cakupan satu admin (admin berbatas hanya melihat tenant itu).
  let admins = $state<AdminRow[]>([]);
  let showScope = $state(false);
  let scopeAdmin = $state('');
  let scopeMode = $state<'add' | 'remove'>('add');
  function openScope() {
    showScope = !showScope;
    if (showScope && admins.length === 0) platformApi.admins().then((r) => (admins = r.items.filter((a) => a.active))).catch(() => {});
  }
  async function applyScope() {
    bulkBusy = true;
    bulkMsg = error = '';
    try {
      await platformApi.setAdminScope(scopeAdmin, scopeMode, [...selected]);
      bulkMsg = t('platform.tenants.bulkScope.done');
      showScope = false;
      admins = [];
    } catch (err) {
      error = errorMessage(err);
    } finally {
      bulkBusy = false;
    }
  }

  // Ubah jenis & lokasi massal: '' = tidak diubah, '__clear' = kosongkan, lainnya = nilai baru (boleh ketik baru).
  let showProfile = $state(false);
  let profType = $state('');
  let profLoc = $state('');
  const pv = (v: string) => (v === '' ? undefined : v === '__clear' ? '' : v.trim());
  async function applyProfile() {
    bulkBusy = true;
    bulkMsg = error = '';
    try {
      const res = await platformApi.bulkSetProfile([...selected], pv(profType), pv(profLoc));
      bulkMsg = t('platform.tenants.bulkProfile.done', { ok: res.updated, failed: res.failed.length });
      selected = new Set(res.failed);
      showProfile = res.failed.length > 0;
      await load();
      platformApi.tenantTypes().then((r) => (types = r)).catch(() => {});
      platformApi.tenantLocations().then((r) => (locations = r)).catch(() => {});
    } catch (err) {
      error = errorMessage(err);
    } finally {
      bulkBusy = false;
    }
  }

  async function load() {
    const my = ++seq; // abaikan respons usang saat pengguna mengetik cepat
    loading = true;
    error = '';
    try {
      const res = await platformApi.tenants(q.trim(), LIMIT, offset, filters());
      if (my === seq) data = res;
    } catch (err) {
      if (my === seq) error = errorMessage(err);
    } finally {
      if (my === seq) loading = false;
    }
  }
  onMount(() => {
    void load();
    platformApi.plans().then((r) => (plans = r.items)).catch(() => {});
    platformApi.tenantTypes().then((r) => (types = r)).catch(() => {});
    platformApi.tenantLocations().then((r) => (locations = r)).catch(() => {});
  });

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
  const allChecked = $derived(rows.length > 0 && rows.every((r) => selected.has(r.id)));
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
  {#if canPlatform('tenants', 'create')}
    <a href="/platform/tenants/bulk" class="btn btn-outline !text-[12.5px]"><i class="icon-users text-[13px]"></i>{t('platform.tenants.bulk.open')}</a>
  {/if}
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
    <button type="button" class="btn btn-outline !text-[12.5px]" aria-expanded={showFilter} onclick={() => (showFilter = !showFilter)}><i class="icon-list-filter text-[13px]"></i>{t('platform.tenants.filter.title')}{filterCount ? ` (${filterCount})` : ''}</button>
    <span class="text-[12px] font-medium text-[var(--text-tertiary)]">{t('platform.tenants.total', { count: total })}</span>
  </div>

  {#if showFilter}
    <div class="p-3 sm:p-4 grid grid-cols-2 lg:grid-cols-4 gap-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]/50">
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.status')}
        <select class="field-control w-full mt-1 font-normal" bind:value={fStatus} onchange={applyFilter}>
          <option value="">{t('platform.tenants.filter.all')}</option>
          <option value="active">{t('platform.tenants.active')}</option>
          <option value="inactive">{t('platform.tenants.inactive')}</option>
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.plan')}
        <select class="field-control w-full mt-1 font-normal" bind:value={fPlan} onchange={applyFilter}>
          <option value="">{t('platform.tenants.filter.all')}</option>
          {#each plans as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.access.tenantType')}
        <select class="field-control w-full mt-1 font-normal" bind:value={fType} onchange={applyFilter}>
          <option value="">{t('platform.tenants.filter.all')}</option>
          {#each types as ty (ty.type)}<option value={ty.type}>{ty.type} ({ty.count})</option>{/each}
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.access.tenantLocation')}
        <select class="field-control w-full mt-1 font-normal" bind:value={fLoc} onchange={applyFilter}>
          <option value="">{t('platform.tenants.filter.all')}</option>
          {#each locations as l (l.location)}<option value={l.location}>{l.location} ({l.count})</option>{/each}
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.lastLogin')}
        <select class="field-control w-full mt-1 font-normal" bind:value={fLogin} onchange={applyFilter}>
          <option value="">{t('platform.tenants.filter.all')}</option>
          <option value="recent7">{t('platform.tenants.filter.recent7')}</option>
          <option value="stale30">{t('platform.tenants.filter.stale30')}</option>
          <option value="never">{t('platform.tenants.never')}</option>
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.filter.from')}
        <div class="mt-1 font-normal"><DatePicker bind:value={fFrom} max={fTo || undefined} placeholder={t('platform.tenants.filter.from')} /></div>
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.filter.to')}
        <div class="mt-1 font-normal"><DatePicker bind:value={fTo} min={fFrom || undefined} placeholder={t('platform.tenants.filter.to')} /></div>
      </label>
      {#if filterCount}
        <button type="button" class="col-span-full justify-self-start text-[12px] font-semibold text-[var(--color-primary-700)] hover:underline" onclick={resetFilter}>{t('platform.tenants.filter.reset')}</button>
      {/if}
    </div>
  {/if}

  {#if selected.size > 0 || bulkMsg}
    <div class="px-3 sm:px-4 py-2.5 flex flex-wrap items-center gap-3 border-b border-[var(--border-subtle)] bg-[var(--color-primary-50)]/60 text-[12.5px]">
      {#if selected.size > 0}
        <span class="font-semibold">{t('platform.tenants.bulkPlan.selected', { count: selected.size })}</span>
        {#if canPlatform('tenants', 'update')}
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showPlan = !showPlan)}><i class="icon-package text-[13px]"></i>{t('platform.tenants.bulkPlan.open')}</button>
        {/if}
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showProfile = !showProfile)}><i class="icon-map-pin text-[13px]"></i>{t('platform.tenants.bulkProfile.open')}</button>
        {#if canPlatform('admins', 'update')}<button type="button" class="btn btn-outline !text-[12.5px]" onclick={openScope}><i class="icon-user-check text-[13px]"></i>{t('platform.tenants.bulkScope.open')}</button>{/if}
        <button type="button" class="text-[12px] text-[var(--text-tertiary)] hover:underline" onclick={() => (selected = new Set())}>{t('platform.tenants.bulkPlan.clear')}</button>
      {/if}
      {#if bulkMsg}<span class="text-[var(--text-secondary,inherit)]">{bulkMsg}</span>{/if}
    </div>
    {#if showPlan && selected.size > 0}
      <div class="px-3 sm:px-4 py-3 flex flex-wrap items-end gap-3 border-b border-[var(--border-subtle)]">
        <label class="text-[11.5px] font-semibold">{t('platform.tenants.plan')}
          <select class="field-control mt-1 font-normal min-w-44" bind:value={bulkPlan}>
            <option value="">{t('platform.tenants.bulkPlan.default')}</option>
            {#each plans as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </label>
        {#if bulkPlan}
          <label class="text-[11.5px] font-semibold">{t('platform.tenants.bulkPlan.expires')}
            <div class="mt-1 font-normal"><DatePicker bind:value={bulkExpires} /></div>
          </label>
        {/if}
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={bulkBusy} onclick={() => confirm(t('platform.tenants.bulkPlan.confirm', { count: selected.size })) && applyPlan()}>{bulkBusy ? t('platform.tenants.loading') : t('platform.tenants.bulkPlan.apply')}</button>
        <p class="basis-full text-[11.5px] text-[var(--text-tertiary)]">{t('platform.tenants.bulkPlan.hint')}</p>
      </div>
    {/if}
  {/if}

  {#if showProfile && selected.size > 0}
    <div class="px-3 sm:px-4 py-3 flex flex-wrap items-end gap-3 border-b border-[var(--border-subtle)]">
      <label class="text-[11.5px] font-semibold">{t('platform.access.tenantType')}
        <input class="field-control mt-1 font-normal block min-w-44" list="bulk-types" maxlength="50" placeholder={t('platform.tenants.bulkProfile.keep')} bind:value={profType} />
      </label>
      <label class="text-[11.5px] font-semibold">{t('platform.access.tenantLocation')}
        <input class="field-control mt-1 font-normal block min-w-44" list="bulk-locs" maxlength="100" placeholder={t('platform.tenants.bulkProfile.keep')} bind:value={profLoc} />
      </label>
      <datalist id="bulk-types">{#each types as ty (ty.type)}<option value={ty.type}></option>{/each}</datalist>
      <datalist id="bulk-locs">{#each locations as l (l.location)}<option value={l.location}></option>{/each}</datalist>
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => { profType = profType === '__clear' ? '' : '__clear'; }}>{t('platform.access.tenantType')}: {t('platform.tenants.bulkProfile.clear')}</button>
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => { profLoc = profLoc === '__clear' ? '' : '__clear'; }}>{t('platform.access.tenantLocation')}: {t('platform.tenants.bulkProfile.clear')}</button>
      <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={bulkBusy || (!profType && !profLoc)} onclick={() => confirm(t('platform.tenants.bulkProfile.confirm', { count: selected.size })) && applyProfile()}>{t('platform.tenants.bulkProfile.apply')}</button>
      <p class="basis-full text-[11.5px] text-[var(--text-tertiary)]">{t('platform.tenants.bulkProfile.hint')}</p>
    </div>
  {/if}

  {#if showScope && selected.size > 0}
    <div class="px-3 sm:px-4 py-3 flex flex-wrap items-end gap-3 border-b border-[var(--border-subtle)]">
      <label class="text-[11.5px] font-semibold">{t('platform.tenants.bulkScope.admin')}
        <select class="field-control mt-1 font-normal min-w-44" bind:value={scopeAdmin}>
          <option value="" disabled>—</option>
          {#each admins as a (a.id)}<option value={a.id}>{a.name} ({a.email})</option>{/each}
        </select>
      </label>
      <label class="text-[11.5px] font-semibold">&nbsp;
        <select class="field-control mt-1 font-normal block" bind:value={scopeMode}>
          <option value="add">{t('platform.tenants.bulkScope.add')}</option>
          <option value="remove">{t('platform.tenants.bulkScope.remove')}</option>
        </select>
      </label>
      <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={bulkBusy || !scopeAdmin} onclick={() => confirm(t('platform.tenants.bulkScope.confirm', { count: selected.size })) && applyScope()}>{t('platform.tenants.bulkScope.apply')}</button>
      <p class="basis-full text-[11.5px] text-[var(--text-tertiary)]">{t('platform.tenants.bulkScope.hint')}</p>
    </div>
  {/if}

  {#if error}
    <div role="alert" class="m-3 flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[1050px]">
      <thead>
        <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
          <th class="ps-4 py-2.5 w-8" scope="col"><input type="checkbox" checked={allChecked} onchange={toggleAll} aria-label={t('platform.tenants.bulkPlan.selectAll')} /></th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.name')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.access.tenantType')} / {t('platform.access.tenantLocation')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.owner')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.outlets')} / {t('platform.tenants.users')}</th>
          <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.tenants.plan')}</th>
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
              <td class="px-4 py-3.5" colspan="10"><div class="h-9 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td>
            </tr>
          {/each}
        {:else}
          {#each rows as r (r.id)}
            <tr
              class="group border-t border-[var(--border-subtle)] cursor-pointer transition-colors hover:bg-[var(--color-primary-50)]/60 {loading ? 'opacity-60' : ''}"
              onclick={() => goto(`/platform/tenants/${r.id}`)}
            >
              <td class="ps-4 py-3" onclick={(e) => e.stopPropagation()}><input type="checkbox" checked={selected.has(r.id)} onchange={() => toggle(r.id)} aria-label={r.name} /></td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-3">
                  <Avatar name={r.name} size={38} square />
                  <div class="min-w-0">
                    <a href="/platform/tenants/{r.id}" class="block font-semibold truncate max-w-64 text-[var(--color-primary-700)] hover:underline" onclick={(e) => e.stopPropagation()}>{r.name}</a>
                    <span class="block text-[11px] font-mono text-[var(--text-tertiary)]">{r.code}</span>
                  </div>
                </div>
              </td>
              <td class="px-4 py-3">
                {#if r.tenant_type}<span class="rounded-full px-2.5 py-0.5 text-[11px] font-semibold badge-info">{r.tenant_type}</span>{/if}
                {#if r.location}<span class="block text-[11px] mt-0.5 text-[var(--text-tertiary)]">{r.location}</span>{/if}
                {#if !r.tenant_type && !r.location}<span class="text-[var(--text-tertiary)]">—</span>{/if}
              </td>
              <td class="px-4 py-3 text-[var(--text-secondary,inherit)]">{r.owner_email || '—'}</td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-1.5">
                  <span class="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px] font-semibold tabular-nums" title={t('platform.tenants.outlets')}><i class="icon-map-pin text-[12px] text-[var(--text-tertiary)]"></i>{r.outlet_count}</span>
                  <span class="inline-flex items-center gap-1 rounded-md bg-[var(--surface-sunken)] px-2 py-1 text-[11.5px] font-semibold tabular-nums" title={t('platform.tenants.users')}><i class="icon-users text-[12px] text-[var(--text-tertiary)]"></i>{r.user_count}</span>
                </div>
              </td>
              <td class="px-4 py-3 whitespace-nowrap">
                <span class="font-semibold">{r.plan_name}</span>
                {#if r.plan_expires_at}<span class="block text-[11px] text-[var(--text-tertiary)]">{t('platform.tenants.bulkPlan.until', { date: formatDate(r.plan_expires_at, { dateStyle: 'medium' }) })}</span>{/if}
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
              <td colspan="10" class="px-4 py-14 text-center">
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
