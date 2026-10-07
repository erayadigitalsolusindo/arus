<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { platformApi, type AdminRow } from '#lib/platform/api.ts';
  import { platform, platformLogout } from '#lib/platform/session.svelte.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';

  let list = $state<AdminRow[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let busyId = $state('');

  type Form = { mode: 'create' | 'password'; target?: AdminRow; name: string; email: string; password: string; saving: boolean; error: string; fields: Record<string, string> };
  let form = $state<Form | null>(null);

  async function load() {
    loading = true;
    try {
      list = (await platformApi.admins()).items;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  const openCreate = () => (form = { mode: 'create', name: '', email: '', password: '', saving: false, error: '', fields: {} });
  const openPassword = (target: AdminRow) => (form = { mode: 'password', target, name: '', email: '', password: '', saving: false, error: '', fields: {} });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!form) return;
    form.saving = true;
    form.error = '';
    form.fields = {};
    try {
      if (form.mode === 'create') {
        await platformApi.createAdmin({ name: form.name, email: form.email.trim(), password: form.password });
        notice = t('platform.admins.created');
      } else if (form.target) {
        const self = form.target.id === platform.admin?.id;
        await platformApi.setAdminPassword(form.target.id, form.password);
        notice = t('platform.admins.passwordChanged');
        if (self) {
          form = null;
          await platformLogout(); // semua sesi (termasuk ini) sudah dicabut
          return;
        }
      }
      form = null;
      await load();
    } catch (err) {
      if (!form) return;
      if (err instanceof ApiError && err.code === 'VALIDATION') form.fields = err.fields;
      else form.error = errorMessage(err);
    } finally {
      if (form) form.saving = false;
    }
  }

  async function toggle(a: AdminRow) {
    if (a.active && !confirm(t('platform.admins.disableConfirm', { name: a.name }))) return;
    busyId = a.id;
    error = notice = '';
    try {
      await platformApi.setAdminActive(a.id, !a.active);
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busyId = '';
    }
  }

  const inputClass = 'mt-1.5 w-full px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<div class="flex flex-wrap items-start justify-between gap-3">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('platform.admins.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.admins.subtitle')}</p>
  </div>
  <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}><i class="icon-plus text-[13px]"></i>{t('platform.admins.add')}</button>
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}
{#if notice}
  <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
{/if}

<div class="surface-card !p-0 overflow-hidden">
  <div class="overflow-x-auto scroll-thin">
    <table class="w-full text-[12.5px] min-w-[700px]">
      <thead>
        <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
          <th class="p-3 text-start" scope="col">{t('platform.admins.name')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.admins.lastLogin')}</th>
          <th class="p-3 text-start" scope="col">{t('platform.admins.status')}</th>
          <th class="p-3 text-end" scope="col"><span class="sr-only">…</span></th>
        </tr>
      </thead>
      <tbody>
        {#each list as a (a.id)}
          <tr class="border-t border-[var(--border-subtle)]">
            <td class="p-3">
              <span class="font-semibold">{a.name}</span>
              {#if a.id === platform.admin?.id}<span class="ms-1.5 rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold badge-info">{t('platform.admins.you')}</span>{/if}
              <span class="block text-[11px] text-[var(--text-tertiary)]">{a.email}</span>
            </td>
            <td class="p-3 whitespace-nowrap">{a.last_login_at ? formatDateTime(a.last_login_at, { dateStyle: 'medium', timeStyle: 'short' }) : t('platform.tenants.never')}</td>
            <td class="p-3"><span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {a.active ? 'badge-success' : 'badge-danger'}">{a.active ? t('platform.tenants.active') : t('platform.tenants.inactive')}</span></td>
            <td class="p-3 text-end whitespace-nowrap">
              <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => openPassword(a)}>{t('platform.admins.resetPassword')}</button>
              {#if a.id !== platform.admin?.id}
                <button type="button" class="btn btn-outline !text-[12px] !py-1 disabled:opacity-60 {a.active ? '!text-[var(--color-danger-600)]' : ''}" disabled={busyId === a.id} onclick={() => toggle(a)}>
                  {a.active ? t('platform.admins.disable') : t('platform.admins.enable')}
                </button>
              {/if}
            </td>
          </tr>
        {:else}
          <tr><td colspan="4" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? t('platform.tenants.loading') : t('platform.tenants.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

{#if form}
  <Modal title={form.mode === 'create' ? t('platform.admins.add') : `${t('platform.admins.resetPassword')}: ${form.target?.name}`} onclose={() => (form = null)}>
    <form class="space-y-3.5" onsubmit={submit} novalidate>
      {#if form.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{form.error}</span></div>
      {/if}
      {#if form.mode === 'create'}
        <div>
          <label for="an" class={labelClass}>{t('platform.admins.name')}</label>
          <input id="an" bind:value={form.name} class={inputClass} autocomplete="off" />
          {#if form.fields.name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldMessage(form.fields.name)}</p>{/if}
        </div>
        <div>
          <label for="ae" class={labelClass}>{t('platform.admins.email')}</label>
          <input id="ae" type="email" bind:value={form.email} class={inputClass} autocomplete="off" />
          {#if form.fields.email}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldMessage(form.fields.email)}</p>{/if}
        </div>
      {:else if form.target?.id === platform.admin?.id}
        <p class="text-[12px] text-[var(--text-tertiary)]">{t('platform.admins.selfPasswordNote')}</p>
      {/if}
      <div>
        <label for="ap" class={labelClass}>{form.mode === 'create' ? t('platform.admins.password') : t('platform.admins.newPassword')}</label>
        <input id="ap" type="password" bind:value={form.password} class={inputClass} autocomplete="new-password" />
        {#if form.fields.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldMessage(form.fields.password)}</p>{/if}
      </div>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (form = null)}>{t('platform.admins.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={form.saving}>{t('platform.admins.save')}</button>
      </div>
    </form>
  </Modal>
{/if}
