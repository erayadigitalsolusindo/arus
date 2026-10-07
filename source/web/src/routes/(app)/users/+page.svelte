<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { iam, type Role, type User } from '#lib/iam/api.ts';
  import { outlets as outletsApi, type Outlet } from '#lib/outlets/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName, checkEmail, checkPhone, checkPassword } from '#lib/validation.ts';
  import Modal from '#lib/components/Modal.svelte';

  type Field = 'name' | 'email' | 'phone' | 'password' | 'role_id' | 'outlet_ids';
  type Editor = {
    id: string | null; // null = pengguna baru
    name: string;
    email: string;
    phone: string;
    password: string;
    roleId: string;
    active: boolean;
    outletIds: string[];
    saving: boolean;
    error: string;
    errors: Partial<Record<Field, string>>;
  };

  let users = $state<User[]>([]);
  let roles = $state<Role[]>([]); // hanya role yang boleh diberikan pemanggil
  let outlets = $state<Outlet[]>([]); // outlet yang boleh ditugaskan pemanggil (yang ia akses sendiri)
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let editor = $state<Editor | null>(null);
  let reset = $state<{ user: User; password: string; busy: boolean; error: string } | null>(null);

  async function load() {
    loading = true;
    loadError = '';
    try {
      [users, roles, outlets] = await Promise.all([iam.users(), iam.assignableRoles(), outletsApi.accessible().then((r) => r.outlets)]);
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  const isSelf = (u: { id: string }) => u.id === session.user?.id;

  function openNew() {
    editor = { id: null, name: '', email: '', phone: '', password: '', roleId: '', active: true, outletIds: session.outlet ? [session.outlet.id] : [], saving: false, error: '', errors: {} };
  }

  function openUser(u: User) {
    editor = { id: u.id, name: u.name, email: u.email, phone: u.phone, password: '', roleId: u.role_id, active: u.active, outletIds: [...u.outlet_ids], saving: false, error: '', errors: {} };
  }

  // Role pengguna yang sedang diubah bisa di luar daftar yang boleh diberikan (mis. Owner): tetap tampil agar tidak kosong.
  const roleOptions = $derived.by(() => {
    const cur = editor && users.find((u) => u.id === editor!.id);
    return cur && !roles.some((r) => r.id === cur.role_id) ? [{ id: cur.role_id, name: cur.role_name, is_system: cur.role_is_system, permissions: cur.all_outlets ? { '*': true } : {} } as Role, ...roles] : roles;
  });
  const roleLocked = $derived(!!editor && editor.id !== null && (isSelf({ id: editor.id }) || !roles.some((r) => r.id === users.find((u) => u.id === editor!.id)?.role_id)));
  // Owner (role berizin "*") otomatis mengakses semua outlet: pilihan outlet disembunyikan.
  const roleAllAccess = $derived(!!editor && roleOptions.find((r) => r.id === editor!.roleId)?.permissions['*'] === true);
  const accessible = $derived(new Set(outlets.map((o) => o.id)));
  const outletName = (id: string) => outlets.find((o) => o.id === id)?.name;

  function toggleOutlet(id: string) {
    if (!editor) return;
    editor.outletIds = editor.outletIds.includes(id) ? editor.outletIds.filter((x) => x !== id) : [...editor.outletIds, id];
  }

  /** Hanya outlet yang boleh diatur pemanggil yang dikirim; penugasan di luar jangkauannya dipertahankan server. */
  const outletPayload = (ed: Editor) => (roleAllAccess ? [] : ed.outletIds.filter((id) => accessible.has(id)));

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = '';
    const next: Partial<Record<Field, string>> = {};
    const name = checkName(ed.name);
    if (name.code) next.name = fieldMessage(name.code);
    if (!ed.roleId) next.role_id = fieldMessage('REQUIRED');
    if (!roleAllAccess && outletPayload(ed).length === 0 && !ed.outletIds.some((id) => !accessible.has(id))) next.outlet_ids = fieldMessage('REQUIRED');
    const phone = ed.phone.trim() ? checkPhone(ed.phone) : { value: '', code: '' };
    if (phone.code) next.phone = fieldMessage(phone.code);
    if (!ed.id) {
      const mail = checkEmail(ed.email);
      if (mail.code) next.email = fieldMessage(mail.code);
      const pw = checkPassword(ed.password, mail.value);
      if (pw) next.password = fieldMessage(pw);
    }
    ed.errors = next;
    if (Object.keys(next).length) return;

    ed.saving = true;
    try {
      if (ed.id) {
        await iam.updateUser(ed.id, { name: name.value, phone: phone.value, role_id: ed.roleId, active: ed.active, outlet_ids: outletPayload(ed) });
        notice = t('iam.users.saved');
      } else {
        await iam.createUser({ name: name.value, email: checkEmail(ed.email).value, phone: phone.value, password: ed.password, role_id: ed.roleId, outlet_ids: outletPayload(ed) });
        notice = t('iam.users.created');
      }
      editor = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, code] of Object.entries(err.fields)) ed.errors[k as Field] = fieldMessage(code);
      } else if (err instanceof ApiError && err.code === 'EMAIL_TAKEN') {
        ed.errors.email = errorMessage(err);
      } else {
        ed.error = errorMessage(err);
      }
    } finally {
      ed.saving = false;
    }
  }

  async function savePassword(e: SubmitEvent) {
    e.preventDefault();
    if (!reset) return;
    const r = reset;
    const code = checkPassword(r.password, r.user.email);
    if (code) {
      r.error = fieldMessage(code) ?? '';
      return;
    }
    r.busy = true;
    r.error = '';
    try {
      await iam.resetPassword(r.user.id, r.password);
      reset = null;
      notice = t('iam.users.passwordSaved');
    } catch (err) {
      r.error = err instanceof ApiError && err.code === 'VALIDATION' ? (fieldMessage(err.fields.password) ?? errorMessage(err)) : errorMessage(err);
    } finally {
      r.busy = false;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('iam.users.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('iam.users.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('iam.users.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('iam.users.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('iam.users.subtitle', { count: users.length })}</p>
    </div>
    {#if can('users', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-user-plus text-[13px]"></i>{t('iam.users.add')}</button>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('iam.users.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('iam.users.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[720px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('iam.users.name')}</th>
            <th class="p-3 text-start" scope="col">{t('iam.users.role')}</th>
            <th class="p-3 text-start" scope="col">{t('iam.users.outlets')}</th>
            <th class="p-3 text-start" scope="col">{t('iam.users.status')}</th>
            <th class="p-3 text-start" scope="col">{t('iam.users.lastLogin')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each users as u (u.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3">
                <p class="font-semibold">{u.name}{#if isSelf(u)} <span class="badge-soft badge-info ms-1">{t('iam.users.you')}</span>{/if}</p>
                <p class="text-[11.5px] text-[var(--text-tertiary)]">{u.email} <span class="badge-soft {u.email_verified ? 'badge-success' : 'badge-warning'} ms-1">{u.email_verified ? t('iam.users.verified') : t('iam.users.unverified')}</span></p>
              </td>
              <td class="p-3"><span class="badge-soft badge-primary">{u.role_name}</span></td>
              <td class="p-3 text-[12px]">
                {#if u.all_outlets}{t('iam.users.allOutlets')}{:else}{u.outlet_ids.map(outletName).filter(Boolean).join(', ') || '—'}{/if}
              </td>
              <td class="p-3"><span class="badge-soft {u.active ? 'badge-success' : 'badge-danger'}">{u.active ? t('iam.users.active') : t('iam.users.inactive')}</span></td>
              <td class="p-3 text-[var(--text-tertiary)]">{u.last_login_at ? formatDateTime(u.last_login_at) : t('iam.users.never')}</td>
              <td class="p-3 text-end whitespace-nowrap">
                {#if can('users', 'update')}
                  <button type="button" class="header-icon-btn !size-8" aria-label={t('iam.users.edit')} onclick={() => openUser(u)}><i class="icon-pencil text-[13px]"></i></button>
                  <button type="button" class="header-icon-btn !size-8" aria-label={t('iam.users.resetPassword')} onclick={() => (reset = { user: u, password: '', busy: false, error: '' })}><i class="icon-key text-[13px]"></i></button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="6" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('iam.users.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</main>

{#if editor}
  {@const ed = editor}
  <Modal title={ed.id ? t('iam.users.edit') : t('iam.users.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}

      <div>
        <label for="u-name" class={labelClass}>{t('iam.users.name')}</label>
        <input id="u-name" class={inputClass} bind:value={ed.name} maxlength="100" autocomplete="off" aria-invalid={!!ed.errors.name} />
        {#if ed.errors.name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.name}</p>{/if}
      </div>

      <div>
        <label for="u-email" class={labelClass}>{t('iam.users.email')}</label>
        <input id="u-email" type="email" class={inputClass} bind:value={ed.email} disabled={ed.id !== null} autocomplete="off" aria-invalid={!!ed.errors.email} />
        {#if ed.errors.email}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.email}</p>{/if}
      </div>

      <div>
        <label for="u-phone" class={labelClass}>{t('iam.users.phone')} <span class="normal-case font-normal">({t('iam.users.phoneHint')})</span></label>
        <input id="u-phone" type="tel" class={inputClass} bind:value={ed.phone} autocomplete="off" aria-invalid={!!ed.errors.phone} />
        {#if ed.errors.phone}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.phone}</p>{/if}
      </div>

      <div>
        <label for="u-role" class={labelClass}>{t('iam.users.role')}</label>
        <select id="u-role" class={inputClass} bind:value={ed.roleId} disabled={roleLocked} aria-invalid={!!ed.errors.role_id}>
          <option value="" disabled>{t('iam.users.selectRole')}</option>
          {#each roleOptions as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
        </select>
        {#if ed.errors.role_id}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.role_id}</p>{/if}
        {#if ed.id && isSelf({ id: ed.id })}<p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('iam.users.selfHint')}</p>{/if}
      </div>

      {#if !roleAllAccess}
        <fieldset>
          <legend class={labelClass}>{t('iam.users.outlets')}</legend>
          <div class="space-y-1.5 max-h-40 overflow-y-auto scroll-thin rounded-lg border border-[var(--border-subtle)] p-2.5">
            {#each outlets as o (o.id)}
              <label class="flex items-center gap-2 text-[12.5px] cursor-pointer">
                <input type="checkbox" class="size-4 rounded accent-[var(--color-primary-600)]" checked={ed.outletIds.includes(o.id)} onchange={() => toggleOutlet(o.id)} />
                {o.name} <span class="text-[11px] text-[var(--text-tertiary)]">[{o.code}]</span>
              </label>
            {/each}
          </div>
          {#if ed.errors.outlet_ids}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.outlet_ids}</p>{/if}
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('iam.users.outletsHint')}</p>
        </fieldset>
      {/if}

      {#if ed.id === null}
        <div>
          <label for="u-pw" class={labelClass}>{t('iam.users.password')}</label>
          <input id="u-pw" type="password" class={inputClass} bind:value={ed.password} autocomplete="new-password" aria-invalid={!!ed.errors.password} />
          {#if ed.errors.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.password}</p>{/if}
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('iam.users.passwordHint')}</p>
        </div>
      {:else}
        <label class="flex items-center gap-2 text-[12.5px] font-medium {isSelf({ id: ed.id }) ? 'opacity-60' : 'cursor-pointer'}">
          <input type="checkbox" class="size-4 rounded accent-[var(--color-primary-600)]" bind:checked={ed.active} disabled={isSelf({ id: ed.id })} />
          {t('iam.users.active')}
        </label>
      {/if}

      <div class="flex items-center gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (editor = null)}>{t('iam.users.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px] flex-1 disabled:opacity-60" disabled={ed.saving}>{ed.saving ? t('iam.users.saving') : t('iam.users.save')}</button>
      </div>
    </form>
  </Modal>
{/if}

{#if reset}
  {@const r = reset}
  <Modal title="{t('iam.users.resetPassword')} · {r.user.name}" onclose={() => (reset = null)}>
    <form class="space-y-3.5" onsubmit={savePassword} novalidate>
      {#if r.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{r.error}</span>
        </div>
      {/if}
      <div>
        <label for="u-newpw" class={labelClass}>{t('iam.users.newPassword')}</label>
        <input id="u-newpw" type="password" class={inputClass} bind:value={r.password} autocomplete="new-password" />
        <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('iam.users.passwordHint')}</p>
      </div>
      <div class="flex items-center gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (reset = null)}>{t('iam.users.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px] flex-1 disabled:opacity-60" disabled={r.busy}>{r.busy ? t('iam.users.saving') : t('iam.users.save')}</button>
      </div>
    </form>
  </Modal>
{/if}
