<script lang="ts">
  import { onMount } from 'svelte';
  import { audit as api, type AuditItem } from '#lib/audit/api.ts';
  import { t, tryT, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const ENTITIES = ['user', 'role', 'outlet', 'session', 'tenant'] as const;
  const ACTIONS = [
    'auth.register', 'auth.login', 'auth.password_reset', 'auth.password_change', 'auth.email_verified', 'auth.outlet_switch',
    'user.create', 'user.update', 'user.password_reset', 'role.create', 'role.update', 'role.delete', 'outlet.create', 'outlet.update', 'platform.impersonate', 'platform.tenant_status'
  ] as const;

  let entity = $state('');
  let action = $state('');
  let from = $state('');
  let to = $state('');

  let items = $state<AuditItem[]>([]);
  let cursor = $state('');
  let loading = $state(true);
  let error = $state('');
  let open = $state<Record<number, boolean>>({});

  // Tanggal (YYYY-MM-DD, zona waktu pengguna) → batas RFC3339. "Sampai" inklusif: awal hari berikutnya.
  const startOfDay = (d: string) => (d ? new Date(`${d}T00:00:00`).toISOString() : '');
  const endExclusive = (d: string) => {
    if (!d) return '';
    const x = new Date(`${d}T00:00:00`);
    x.setDate(x.getDate() + 1);
    return x.toISOString();
  };

  async function load(more = false) {
    loading = true;
    error = '';
    try {
      const page = await api.list({ entity, action, from: startOfDay(from), to: endExclusive(to), cursor: more ? cursor : '', limit: 50 });
      items = more ? [...items, ...page.items] : page.items;
      cursor = page.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => load());

  function resetFilters() {
    entity = action = from = to = '';
    void load();
  }

  const actionLabel = (a: string) => tryT(`audit.actions.${a.replace('.', '_')}`) ?? a;
  const entityLabel = (e: string) => tryT(`audit.entities.${e}`) ?? e;
  const pretty = (o: Record<string, unknown>) => JSON.stringify(o, null, 2);
  const hasDetails = (o: Record<string, unknown>) => Object.keys(o).length > 0;

  const fieldClass = 'field-control';
  const labelClass = 'text-[11px] font-semibold uppercase tracking-wide mb-1 block text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('audit.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('audit.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('audit.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('audit.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('audit.subtitle')}</p>
  </div>

  <form class="surface-card p-4 grid grid-cols-2 lg:grid-cols-6 gap-3 items-end" onsubmit={(e) => (e.preventDefault(), load())}>
    <div>
      <label for="f-entity" class={labelClass}>{t('audit.entity')}</label>
      <select id="f-entity" class="w-full {fieldClass}" bind:value={entity}>
        <option value="">{t('audit.all')}</option>
        {#each ENTITIES as e (e)}<option value={e}>{entityLabel(e)}</option>{/each}
      </select>
    </div>
    <div class="lg:col-span-2">
      <label for="f-action" class={labelClass}>{t('audit.action')}</label>
      <select id="f-action" class="w-full {fieldClass}" bind:value={action}>
        <option value="">{t('audit.all')}</option>
        {#each ACTIONS as a (a)}<option value={a}>{actionLabel(a)}</option>{/each}
      </select>
    </div>
    <div>
      <label for="f-from" class={labelClass}>{t('audit.from')}</label>
      <input id="f-from" type="date" class="w-full {fieldClass}" bind:value={from} max={to || undefined} />
    </div>
    <div>
      <label for="f-to" class={labelClass}>{t('audit.to')}</label>
      <input id="f-to" type="date" class="w-full {fieldClass}" bind:value={to} min={from || undefined} />
    </div>
    <div class="flex gap-2">
      <button type="submit" class="btn btn-primary !text-[12.5px] flex-1" disabled={loading}>{t('audit.apply')}</button>
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={resetFilters}>{t('audit.reset')}</button>
    </div>
  </form>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('audit.loadFailed')} {error}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('audit.time')}</th>
            <th class="p-3 text-start" scope="col">{t('audit.actor')}</th>
            <th class="p-3 text-start" scope="col">{t('audit.action')}</th>
            <th class="p-3 text-start" scope="col">{t('audit.entity')}</th>
            <th class="p-3 text-start" scope="col">{t('audit.ip')}</th>
            <th class="p-3 text-end" scope="col"><span class="sr-only">{t('audit.details')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each items as it (it.id)}
            <tr class="border-t border-[var(--border-subtle)] align-top hover:bg-[var(--surface-sunken)]">
              <td class="p-3 whitespace-nowrap">{formatDateTime(it.created_at, { dateStyle: 'medium', timeStyle: 'medium' })}</td>
              <td class="p-3">{it.actor_name || t('audit.system')}</td>
              <td class="p-3 font-semibold">{actionLabel(it.action)}</td>
              <td class="p-3">
                {entityLabel(it.entity)}
                {#if it.entity_id}<span class="block text-[11px] font-mono text-[var(--text-tertiary)] max-w-40 truncate" title={it.entity_id}>{it.entity_id}</span>{/if}
              </td>
              <td class="p-3 font-mono text-[11.5px]">{it.ip}</td>
              <td class="p-3 text-end">
                {#if hasDetails(it.details)}
                  <button type="button" class="header-icon-btn !size-8" aria-expanded={!!open[it.id]} aria-label={t('audit.details')} onclick={() => (open[it.id] = !open[it.id])}>
                    <i class="{open[it.id] ? 'icon-chevron-up' : 'icon-chevron-down'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
            {#if open[it.id]}
              <tr class="bg-[var(--surface-sunken)]">
                <td colspan="6" class="px-3 pb-3"><pre class="text-[11.5px] overflow-x-auto scroll-thin whitespace-pre-wrap break-words">{pretty(it.details)}</pre></td>
              </tr>
            {/if}
          {:else}
            <tr><td colspan="6" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? t('audit.loading') : t('audit.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>

  {#if cursor}
    <div class="text-center">
      <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading} onclick={() => load(true)}>{loading ? t('audit.loading') : t('audit.loadMore')}</button>
    </div>
  {/if}
</main>
