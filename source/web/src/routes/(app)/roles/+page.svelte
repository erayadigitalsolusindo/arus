<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { iam, type ModuleDef, type Role } from '#lib/iam/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, tryT } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';

  const ACTIONS = ['view', 'create', 'update', 'delete', 'approve'] as const;

  type Grants = Record<string, string[]>;
  type Editor = { id: string | null; name: string; grants: Grants; system: boolean; saving: boolean; error: string; nameError: string };

  let roles = $state<Role[]>([]);
  let modules = $state<ModuleDef[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let editor = $state<Editor | null>(null);
  let removing = $state<{ role: Role; busy: boolean; error: string } | null>(null);

  const isOwner = $derived(session.permissions['*'] === true);
  const readonly = $derived(!!editor && (editor.system || (editor.id !== null && !can('roles', 'update'))));

  async function load() {
    loading = true;
    loadError = '';
    try {
      [roles, modules] = await Promise.all([iam.roles(), iam.registry()]);
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  const moduleLabel = (id: string) => tryT(`iam.module.${id}`) ?? id;
  const actionLabel = (a: string) => tryT(`iam.actions.${a}`) ?? a;

  function grantsOf(role: Role): Grants {
    const out: Grants = {};
    for (const [k, v] of Object.entries(role.permissions)) if (k !== '*' && Array.isArray(v)) out[k] = [...v];
    return out;
  }

  function permissionCount(role: Role): number {
    return Object.values(grantsOf(role)).reduce((n, a) => n + a.length, 0);
  }

  function openNew() {
    editor = { id: null, name: '', grants: {}, system: false, saving: false, error: '', nameError: '' };
  }

  function openRole(role: Role) {
    editor = { id: role.id, name: role.name, grants: grantsOf(role), system: role.is_system, saving: false, error: '', nameError: '' };
  }

  // Pemilik boleh memberi apa saja; yang lain hanya izin yang ia miliki (server menegakkan hal yang sama).
  const grantable = (m: string, a: string) => isOwner || can(m, a);
  const has = (m: string, a: string) => !!editor?.grants[m]?.includes(a);

  // "Hanya Kasir" = paket tetap (sama dengan authz.PosOnlyGrants di server): mengisi paketnya dan mengunci baris lain.
  const POS_ONLY: Grants = { pos_only: ['view'], sales_orders: ['view', 'create'], items: ['view'] };
  const posLocked = $derived(!!editor?.grants.pos_only?.includes('view'));
  const locked = (m: string) => posLocked && m !== 'pos_only';

  function toggle(m: string, a: string) {
    if (!editor || readonly || !grantable(m, a)) return;
    if (m === 'pos_only' && !has(m, a)) {
      editor.grants = { ...POS_ONLY };
      return;
    }
    if (locked(m)) return;
    const cur = new Set(editor.grants[m] ?? []);
    if (cur.has(a)) {
      cur.delete(a);
      if (a === 'view') cur.clear(); // tanpa izin lihat, aksi lain tidak berarti
    } else {
      cur.add(a);
      if (a !== 'view' && grantable(m, 'view')) cur.add('view');
    }
    const next = { ...editor.grants };
    if (cur.size) next[m] = [...cur];
    else delete next[m];
    editor.grants = next;
  }

  function rowAll(m: ModuleDef): boolean {
    return m.actions.filter((a) => grantable(m.id, a)).every((a) => has(m.id, a));
  }

  function toggleRow(m: ModuleDef) {
    if (!editor || readonly || locked(m.id)) return;
    if (m.id === 'pos_only' && !rowAll(m)) {
      editor.grants = { ...POS_ONLY };
      return;
    }
    const allowed = m.actions.filter((a) => grantable(m.id, a));
    if (!allowed.length) return;
    const next = { ...editor.grants };
    if (rowAll(m)) delete next[m.id];
    else next[m.id] = allowed;
    editor.grants = next;
  }

  function setAll(on: boolean) {
    if (!editor || readonly || posLocked) return;
    const next: Grants = {};
    if (on) for (const m of modules) {
      const allowed = m.actions.filter((a) => grantable(m.id, a));
      if (allowed.length) next[m.id] = allowed;
    }
    editor.grants = next;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!editor || readonly) return;
    editor.saving = true;
    editor.error = editor.nameError = '';
    try {
      if (editor.id) await iam.updateRole(editor.id, editor.name, editor.grants);
      else await iam.createRole(editor.name, editor.grants);
      editor = null;
      notice = t('iam.roles.saved');
      await load();
    } catch (err) {
      if (!editor) return;
      if (err instanceof ApiError && err.code === 'VALIDATION' && err.fields.name) editor.nameError = fieldMessage(err.fields.name) ?? '';
      else if (err instanceof ApiError && err.code === 'NAME_TAKEN') editor.nameError = errorMessage(err);
      else editor.error = errorMessage(err);
    } finally {
      if (editor) editor.saving = false;
    }
  }

  async function confirmRemove() {
    if (!removing) return;
    removing.busy = true;
    removing.error = '';
    try {
      await iam.deleteRole(removing.role.id);
      removing = null;
      notice = t('iam.roles.deleted');
      await load();
    } catch (err) {
      if (removing) {
        removing.error = errorMessage(err);
        removing.busy = false;
      }
    }
  }
</script>

<svelte:head><title>{t('iam.roles.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('iam.roles.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('iam.roles.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('iam.roles.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('iam.roles.subtitle')}</p>
    </div>
    {#if can('roles', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('iam.roles.add')}</button>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('iam.roles.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('iam.roles.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[560px]">
        <thead>
          <tr class="text-start text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('iam.roles.name')}</th>
            <th class="p-3 text-start" scope="col">{t('nav.users')}</th>
            <th class="p-3 text-start" scope="col">{t('iam.roles.matrix')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each roles as role (role.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3">
                <button type="button" class="font-semibold text-start" onclick={() => openRole(role)}>{role.name}</button>
                {#if role.is_system}<span class="badge-soft badge-primary ms-2"><i class="icon-shield-check text-[9px]"></i>{t('iam.roles.system')}</span>{/if}
              </td>
              <td class="p-3">{t('iam.roles.users', { count: role.user_count })}</td>
              <td class="p-3">{role.permissions['*'] === true ? t('iam.roles.allAccess') : t('iam.roles.permissions', { count: permissionCount(role) })}</td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={role.is_system || !can('roles', 'update') ? t('iam.roles.view') : t('iam.roles.edit')} onclick={() => openRole(role)}>
                  <i class="{role.is_system || !can('roles', 'update') ? 'icon-eye' : 'icon-pencil'} text-[13px]"></i>
                </button>
                {#if !role.is_system && can('roles', 'delete')}
                  <button type="button" class="header-icon-btn !size-8 !text-[var(--color-danger-600)]" aria-label={t('iam.roles.delete')} onclick={() => (removing = { role, busy: false, error: '' })}>
                    <i class="icon-trash-2 text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="4" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('iam.roles.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</main>

{#if editor}
  <Modal title={readonly ? t('iam.roles.view') : editor.id ? t('iam.roles.edit') : t('iam.roles.add')} onclose={() => (editor = null)} wide>
    <form class="space-y-4" onsubmit={save} novalidate>
      {#if editor.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{editor.error}</span>
        </div>
      {/if}

      <div>
        <label for="role-name" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('iam.roles.name')}</label>
        <input id="role-name" class="w-full field-control" bind:value={editor.name} maxlength="50" placeholder={t('iam.roles.namePlaceholder')} disabled={readonly} aria-invalid={!!editor.nameError} />
        {#if editor.nameError}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{editor.nameError}</p>{/if}
      </div>

      {#if editor.system}
        <p class="text-[12.5px] rounded-lg px-3 py-2.5 bg-[var(--surface-sunken)]"><i class="icon-shield-check me-1.5"></i>{t('iam.roles.systemHint')}</p>
      {/if}

      {#if !editor.system}
        <div>
          <div class="flex flex-wrap items-center justify-between gap-2 mb-2">
            <h3 class="font-display font-bold text-[13px]">{t('iam.roles.matrix')}</h3>
            {#if !readonly}
              <div class="flex gap-2">
                <button type="button" class="btn btn-outline !text-[11.5px] !py-1" onclick={() => setAll(true)}>{t('iam.roles.selectAll')}</button>
                <button type="button" class="btn btn-outline !text-[11.5px] !py-1" onclick={() => setAll(false)}>{t('iam.roles.clearAll')}</button>
              </div>
            {/if}
          </div>
          {#if posLocked}
            <p class="text-[12px] rounded-md px-3 py-2 badge-info">{t('iam.roles.posOnlyHint')}</p>
          {/if}
          <div class="overflow-auto scroll-thin max-h-[50vh] rounded-lg border border-[var(--border-subtle)]">
            <table class="w-full text-[12px] min-w-[560px]">
              <thead class="sticky top-0 bg-[var(--surface-sunken)]">
                <tr>
                  <th class="p-2 text-start" scope="col">{t('iam.roles.module')}</th>
                  {#each ACTIONS as a (a)}<th class="p-2 text-center" scope="col">{actionLabel(a)}</th>{/each}
                  <th class="p-2 text-center" scope="col"><span class="sr-only">{t('iam.roles.selectAll')}</span></th>
                </tr>
              </thead>
              <tbody>
                {#each modules as m (m.id)}
                  <tr class="border-t border-[var(--border-subtle)]">
                    <td class="p-2 font-semibold">{moduleLabel(m.id)}</td>
                    {#each ACTIONS as a (a)}
                      <td class="p-2 text-center">
                        {#if m.actions.includes(a)}
                          <input
                            type="checkbox"
                            class="size-4 rounded accent-[var(--color-primary-600)] disabled:opacity-40"
                            checked={has(m.id, a)}
                            disabled={readonly || !grantable(m.id, a) || locked(m.id)}
                            title={!readonly && !grantable(m.id, a) ? t('iam.roles.notYours') : undefined}
                            aria-label="{moduleLabel(m.id)}: {actionLabel(a)}"
                            onchange={() => toggle(m.id, a)}
                          />
                        {:else}
                          <span class="text-[var(--text-tertiary)]">—</span>
                        {/if}
                      </td>
                    {/each}
                    <td class="p-2 text-center">
                      {#if !readonly && !locked(m.id)}
                        <button type="button" class="text-[11px] font-semibold text-[var(--color-primary-600)]" onclick={() => toggleRow(m)}>{rowAll(m) ? t('iam.roles.clearAll') : t('iam.roles.selectAll')}</button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      {/if}

      <div class="flex items-center gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (editor = null)}>{readonly ? t('iam.roles.close') : t('iam.roles.cancel')}</button>
        {#if !readonly}
          <button type="submit" class="btn btn-primary !text-[12.5px] flex-1 disabled:opacity-60" disabled={editor.saving}>{editor.saving ? t('iam.roles.saving') : t('iam.roles.save')}</button>
        {/if}
      </div>
    </form>
  </Modal>
{/if}

{#if removing}
  <Modal title={t('iam.roles.deleteTitle')} onclose={() => (removing = null)}>
    <p class="text-[12.5px]">{t('iam.roles.deleteConfirm', { name: removing.role.name })}</p>
    {#if removing.error}
      <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 mt-3 text-[12.5px] badge-danger">
        <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{removing.error}</span>
      </div>
    {/if}
    <div class="flex items-center gap-2 mt-5">
      <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (removing = null)}>{t('iam.roles.cancel')}</button>
      <button type="button" class="btn btn-primary !text-[12.5px] flex-1 !bg-[var(--color-danger-600)] disabled:opacity-60" disabled={removing.busy} onclick={confirmRemove}>{t('iam.roles.delete')}</button>
    </div>
  </Modal>
{/if}
