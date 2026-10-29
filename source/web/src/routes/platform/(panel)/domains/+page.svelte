<script lang="ts">
  import { platformApi, type DomainRow } from '#lib/platform/api.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let rows = $state<DomainRow[]>([]);
  let target = $state('');
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let busy = $state(''); // id domain yang sedang diproses

  let showModal = $state(false);
  let form = $state({ tenantCode: '', host: '' });
  let saving = $state(false);

  async function load() {
    loading = true;
    try {
      const res = await platformApi.domains();
      rows = res.items;
      target = res.target;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  function openCreate() {
    form = { tenantCode: '', host: '' };
    error = '';
    showModal = true;
  }

  async function save() {
    saving = true;
    error = '';
    notice = '';
    try {
      const code = form.tenantCode.trim().toLowerCase();
      const found = (await platformApi.tenants(code, 10, 0)).items.find((x) => x.code === code);
      if (!found) throw new Error(t('platform.domains.tenantNotFound'));
      await platformApi.createDomain({ tenant_id: found.id, host: form.host.trim() });
      notice = t('platform.domains.created');
      showModal = false;
      await load();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function run(id: string, fn: () => Promise<string | void>) {
    busy = id;
    error = '';
    notice = '';
    try {
      const msg = await fn();
      if (msg) notice = msg;
      await load();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = '';
    }
  }

  const verify = (d: DomainRow) =>
    run(d.id, async () => {
      const res = await platformApi.verifyDomain(d.id);
      return res.status === 'active' ? t('platform.domains.verified') : t('platform.domains.notVerified');
    });
  const toggle = (d: DomainRow) =>
    run(d.id, async () => {
      const disable = d.status !== 'disabled';
      await platformApi.setDomainDisabled(d.id, disable);
      return disable ? t('platform.domains.disabledMsg') : t('platform.domains.enabledMsg');
    });
  const remove = (d: DomainRow) => {
    if (!confirm(t('platform.domains.deleteConfirm'))) return;
    return run(d.id, async () => {
      await platformApi.deleteDomain(d.id);
      return t('platform.domains.deleted');
    });
  };

  const badge = (s: DomainRow['status']) => (s === 'active' ? 'badge-success' : s === 'pending' ? 'badge-warning' : 'badge-info');

  $effect(() => {
    load();
  });
</script>

<svelte:head><title>{t('platform.domains.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex items-center justify-between gap-4">
    <div>
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.domains.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)]">{t('platform.domains.subtitle')}</p>
      <p class="text-[12.5px] mt-1 text-[var(--text-secondary)]">
        {#if target}{t('platform.domains.target', { target })}{:else if !loading}{t('platform.domains.noTarget')}{/if}
      </p>
    </div>
    <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}>
      <i class="icon-plus text-[13px]"></i>{t('platform.domains.add')}
    </button>
  </div>

  {#if notice}
    <div class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]">{notice}</div>
  {/if}
  {#if error && !showModal}
    <div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]">{error}</div>
  {/if}

  {#if loading}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.domains.loading')}</div>
  {:else if rows.length === 0}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.domains.empty')}</div>
  {:else}
    <div class="surface-card overflow-x-auto">
      <table class="w-full text-[13px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.domains.host')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.domains.tenant')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.domains.status')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.domains.lastCheck')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.domains.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as d (d.id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-2.5 font-medium">{d.host}</td>
              <td class="px-4 py-2.5">{d.tenant_name} <span class="text-[var(--text-tertiary)]">({d.tenant_code})</span></td>
              <td class="px-4 py-2.5"><span class={badge(d.status)}>{t(`platform.domains.${d.status}`)}</span></td>
              <td class="px-4 py-2.5 text-[var(--text-secondary)]">
                {d.last_check_at ? formatDateTime(d.last_check_at) : t('platform.domains.never')}
                {#if d.last_check_error}<div class="text-[11.5px] text-[var(--color-danger-600)]">{d.last_check_error}</div>{/if}
              </td>
              <td class="px-4 py-2.5 text-end whitespace-nowrap">
                {#if d.status !== 'disabled'}
                  <button type="button" class="btn btn-outline !text-[12px] !py-1" disabled={busy === d.id} onclick={() => verify(d)}>
                    {busy === d.id ? t('platform.domains.verifying') : t('platform.domains.verify')}
                  </button>
                {/if}
                <button type="button" class="btn btn-outline !text-[12px] !py-1" disabled={busy === d.id} onclick={() => toggle(d)}>
                  {d.status === 'disabled' ? t('platform.domains.enable') : t('platform.domains.disable')}
                </button>
                <button type="button" class="btn btn-outline !text-[12px] !py-1 text-[var(--color-danger-600)]" disabled={busy === d.id} onclick={() => remove(d)}>
                  {t('platform.domains.delete')}
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

{#if showModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
    <div class="surface-card w-full max-w-md rounded-xl shadow-xl">
      <div class="flex items-center justify-between border-b border-[var(--border-subtle)] px-5 py-4">
        <h2 class="font-semibold text-[15px]">{t('platform.domains.addTitle')}</h2>
        <button type="button" aria-label={t('platform.domains.close')} class="header-icon-btn !size-7" onclick={() => (showModal = false)}><i class="icon-x text-[14px]"></i></button>
      </div>
      <form onsubmit={(e) => { e.preventDefault(); save(); }} novalidate class="p-5 space-y-4">
        {#if error}
          <div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]">{error}</div>
        {/if}
        <div>
          <label for="dom-tenant" class="block text-[12.5px] font-medium mb-1.5">{t('platform.domains.tenantCode')}</label>
          <input id="dom-tenant" type="text" bind:value={form.tenantCode} class="form-input !text-[13px]" autocomplete="off" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.domains.tenantCodeHelp')}</p>
        </div>
        <div>
          <label for="dom-host" class="block text-[12.5px] font-medium mb-1.5">{t('platform.domains.host')}</label>
          <input id="dom-host" type="text" bind:value={form.host} class="form-input !text-[13px]" placeholder="arus.kotakcantik.com" autocomplete="off" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.domains.hostHelp')}</p>
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showModal = false)}>{t('platform.domains.cancel')}</button>
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving || !form.tenantCode.trim() || !form.host.trim()}>{saving ? t('platform.domains.saving') : t('platform.domains.save')}</button>
        </div>
      </form>
    </div>
  </div>
{/if}
