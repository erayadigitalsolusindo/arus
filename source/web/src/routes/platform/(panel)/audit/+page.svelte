<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount } from 'svelte';
  import { platformApi, type PlatformAuditItem } from '#lib/platform/api.ts';
  import { t, tryT, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const ACTIONS = ['platform.login', 'platform.impersonate', 'platform.tenant_status', 'platform.admin_create', 'platform.admin_status', 'platform.admin_password', 'platform.mfa_enable', 'platform.mfa_disable', 'platform.mfa_reset', 'platform.mfa_recovery', 'platform.apk_upload', 'platform.apk_delete', 'platform.setup'] as const;

  let action = $state('');
  let items = $state<PlatformAuditItem[]>([]);
  let nextBefore = $state(0);
  let loading = $state(true);
  let error = $state('');
  let open = $state<Record<number, boolean>>({});

  async function load(more = false) {
    loading = true;
    error = '';
    try {
      const res = await platformApi.audit({ action, before: more ? nextBefore : undefined });
      items = more ? [...items, ...res.items] : res.items;
      nextBefore = res.next_before ?? 0;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => load());

  const label = (a: string) => tryT(`platform.audit.actions.${a.replace('.', '_')}`) ?? a;
  const iconFor = (a: string) =>
    a.includes('impersonate') ? 'log-in' : a.includes('mfa') ? 'shield-check' : a.includes('login') ? 'key-round' : a.includes('admin') ? 'user-cog' : a.includes('tenant') ? 'store' : 'activity';
  const toneFor = (a: string) => (a.includes('impersonate') ? 'warning' : a.includes('mfa') ? 'success' : a.includes('status') || a.includes('password') ? 'danger' : 'primary');
  const hasDetails = (o: Record<string, unknown>) => Object.keys(o).length > 0;
</script>

<div>
  <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.audit.title')}</h1>
  <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.audit.subtitle')}</p>
</div>

<div class="surface-card p-3 sm:p-4 flex items-center gap-3">
  <Select class="!w-auto min-w-56" ariaLabel={t('platform.audit.action')} bind:value={action} onchange={() => load()} options={[{ value: '', label: t('platform.audit.all') }, ...ACTIONS.map((a) => ({ value: a as string, label: label(a) }))]} />
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

<div class="surface-card !p-0 overflow-hidden">
  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[760px]">
      <thead>
        <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
          <th class="p-3 text-start" scope="col">{t('platform.audit.time')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.audit.admin')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.audit.action')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.audit.tenant')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.audit.ip')}</th>
          <th class="p-3" scope="col"></th>
        </tr>
      </thead>
      <tbody>
        {#each items as it (it.id)}
          <tr class="border-t border-[var(--border-subtle)] align-top hover:bg-[var(--surface-sunken)]">
            <td class="p-3 whitespace-nowrap">{formatDateTime(it.created_at, { dateStyle: 'medium', timeStyle: 'medium' })}</td>
            <td class="p-3">{it.admin_name || '—'}</td>
            <td class="p-3">
              <span class="inline-flex items-center gap-2 font-semibold">
                <span class="grid size-7 place-items-center rounded-lg" style="background:var(--color-{toneFor(it.action)}-50);color:var(--color-{toneFor(it.action)}-600)"><i class="icon-{iconFor(it.action)} text-[14px]"></i></span>{label(it.action)}
              </span>
            </td>
            <td class="p-3">{#if it.tenant_id}<a class="text-[var(--color-primary-600)]" href="/platform/tenants/{it.tenant_id}">{it.tenant_name || it.tenant_id}</a>{:else}—{/if}</td>
            <td class="p-3 font-mono text-[11.5px]">{it.ip}</td>
            <td class="p-3 text-end">
              {#if hasDetails(it.details)}
                <button type="button" class="header-icon-btn !size-8" aria-expanded={!!open[it.id]} aria-label="details" onclick={() => (open[it.id] = !open[it.id])}>
                  <i class="{open[it.id] ? 'icon-chevron-up' : 'icon-chevron-down'} text-[13px]"></i>
                </button>
              {/if}
            </td>
          </tr>
          {#if open[it.id]}
            <tr class="bg-[var(--surface-sunken)]"><td colspan="6" class="px-3 pb-3"><pre class="text-[11.5px] overflow-x-auto scroll-thin whitespace-pre-wrap break-words">{JSON.stringify(it.details, null, 2)}</pre></td></tr>
          {/if}
        {:else}
          <tr><td colspan="6" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? t('platform.tenants.loading') : t('platform.audit.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

{#if nextBefore}
  <div class="text-center">
    <button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={loading} onclick={() => load(true)}>{t('platform.audit.loadMore')}</button>
  </div>
{/if}
