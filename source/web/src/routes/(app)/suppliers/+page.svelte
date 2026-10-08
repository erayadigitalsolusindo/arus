<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { suppliers as api, type Supplier } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName, checkEmail, checkPhone, cleanText } from '#lib/validation.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  const PAGE = 20;
  const CODE_RE = /^[A-Za-z0-9][A-Za-z0-9._/-]{0,29}$/;

  type Field = 'code' | 'name' | 'contact_name' | 'phone' | 'email' | 'address' | 'note';
  type Editor = {
    id: string | null;
    code: string;
    name: string;
    contact: string;
    phone: string;
    email: string;
    address: string;
    note: string;
    saving: boolean;
    error: string;
    errors: Partial<Record<Field, string>>;
  };

  let rows = $state<Supplier[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let filter = $state<'all' | 'active' | 'inactive'>('all');
  let editor = $state<Editor | null>(null);
  let busyId = $state<string | null>(null);

  let seq = 0;
  async function load() {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await api.list({ q: q.trim(), active: filter === 'all' ? undefined : filter === 'active', limit: PAGE, offset });
      if (mine !== seq) return;
      rows = res.data;
      total = res.total;
    } catch (err) {
      if (mine === seq) loadError = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void q;
    void filter;
    const h = setTimeout(() => {
      offset = 0;
      void load();
    }, 250);
    return () => clearTimeout(h);
  });

  function go(next: number) {
    offset = Math.max(0, next);
    void load();
  }

  const canWrite = (s: Supplier | null) => (s ? can('suppliers', 'update') : can('suppliers', 'create'));
  const blank = (): Editor => ({ id: null, code: '', name: '', contact: '', phone: '', email: '', address: '', note: '', saving: false, error: '', errors: {} });
  const openNew = () => (editor = blank());
  const openSupplier = (s: Supplier) =>
    (editor = { ...blank(), id: s.id, code: s.code, name: s.name, contact: s.contact_name, phone: s.phone, email: s.email, address: s.address, note: s.note });

  /** Teks opsional: kosong boleh; selain itu bersih dan tidak melebihi batas. */
  function optional(raw: string, max: number, noMarkup = false): { value: string; code?: string } {
    const v = cleanText(raw);
    if (v === null || (noMarkup && /[<>]/.test(v))) return { value: '', code: 'INVALID' };
    if ([...v].length > max) return { value: v, code: 'TOO_LONG' };
    return { value: v };
  }

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = '';
    const next: Partial<Record<Field, string>> = {};

    const name = checkName(ed.name);
    if (name.code) next.name = fieldMessage(name.code);
    const code = ed.code.trim();
    if (code && !CODE_RE.test(code)) next.code = fieldMessage('INVALID');
    const contact = optional(ed.contact, 100, true);
    if (contact.code) next.contact_name = fieldMessage(contact.code);
    let phone = '';
    if (ed.phone.trim()) {
      const p = checkPhone(ed.phone);
      if (p.code) next.phone = fieldMessage(p.code);
      else phone = p.value;
    }
    let email = '';
    if (ed.email.trim()) {
      const m = checkEmail(ed.email);
      if (m.code) next.email = fieldMessage(m.code);
      else email = m.value;
    }
    const address = optional(ed.address, 500);
    if (address.code) next.address = fieldMessage(address.code);
    const note = optional(ed.note, 500);
    if (note.code) next.note = fieldMessage(note.code);
    ed.errors = next;
    if (Object.keys(next).length) return;

    const body = { code, name: name.value, contact_name: contact.value, phone, email, address: address.value, note: note.value };
    ed.saving = true;
    try {
      if (ed.id) {
        await api.update(ed.id, body);
        notice = t('catalog.saved');
      } else {
        await api.create(body);
        notice = t('catalog.created');
      }
      editor = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, c] of Object.entries(err.fields)) ed.errors[k as Field] = fieldMessage(c);
      } else if (err instanceof ApiError && err.code === 'NAME_TAKEN') {
        ed.errors.name = errorMessage(err);
      } else if (err instanceof ApiError && err.code === 'CODE_TAKEN') {
        ed.errors.code = errorMessage(err);
      } else {
        ed.error = errorMessage(err);
      }
    } finally {
      ed.saving = false;
    }
  }

  async function toggle(s: Supplier) {
    busyId = s.id;
    notice = loadError = '';
    try {
      await api.setActive(s.id, !s.active);
      notice = t(s.active ? 'catalog.archived' : 'catalog.restored', { name: s.name });
      await load();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      busyId = null;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('catalog.suppliers.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('catalog.suppliers.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('catalog.suppliers.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('catalog.suppliers.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('catalog.suppliers.subtitle')}</p>
    </div>
    {#if can('suppliers', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('catalog.suppliers.add')}</button>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('catalog.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('catalog.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-72">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('catalog.suppliers.search')} aria-label={t('catalog.suppliers.search')} bind:value={q} maxlength="100" />
      </div>
      <Select class="!w-auto min-w-40" ariaLabel={t('catalog.status')} bind:value={filter} options={[{ value: 'all', label: t('catalog.filter.all') }, { value: 'active', label: t('catalog.filter.active') }, { value: 'inactive', label: t('catalog.filter.inactive') }]} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[720px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('catalog.name')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.suppliers.code')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.suppliers.contact')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.suppliers.phone')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as s (s.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-semibold">{s.name}</td>
              <td class="p-3 font-mono text-[12px]">{s.code}</td>
              <td class="p-3">{s.contact_name}</td>
              <td class="p-3">{s.phone}</td>
              <td class="p-3"><span class="badge-soft {s.active ? 'badge-success' : 'badge-danger'}">{s.active ? t('catalog.active') : t('catalog.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={canWrite(s) ? t('catalog.edit') : t('catalog.view')} onclick={() => openSupplier(s)}>
                  <i class="{canWrite(s) ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
                {#if can('suppliers', 'update')}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={s.active ? t('catalog.archive') : t('catalog.restore')}
                    title={s.active ? t('catalog.archive') : t('catalog.restore')}
                    disabled={busyId === s.id}
                    onclick={() => toggle(s)}
                  >
                    <i class="{s.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="6" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' ? t('catalog.emptySearch') : t('catalog.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if total > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
        <span>{t('catalog.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('catalog.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('catalog.next')}</button>
        </div>
      </div>
    {/if}
  </div>
  <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('catalog.archiveHint')}</p>
</main>

{#if editor}
  {@const ed = editor}
  {@const ro = ed.id !== null && !can('suppliers', 'update')}
  <Modal wide title={ed.id ? t('catalog.suppliers.edit') : t('catalog.suppliers.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}

      <div class="grid gap-3.5 sm:grid-cols-2">
        <div class="sm:col-span-2">
          <label for="s-name" class={labelClass}>{t('catalog.name')}</label>
          <input id="s-name" class={inputClass} bind:value={ed.name} maxlength="100" disabled={ro} aria-invalid={!!ed.errors.name} use:focusOnMount />
          {#if ed.errors.name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.name}</p>{/if}
        </div>
        <div>
          <label for="s-code" class={labelClass}>{t('catalog.suppliers.code')}</label>
          <input id="s-code" class={inputClass} bind:value={ed.code} maxlength="30" disabled={ro} aria-invalid={!!ed.errors.code} />
          {#if ed.errors.code}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.code}</p>{:else}<p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('catalog.suppliers.codeHint')}</p>{/if}
        </div>
        <div>
          <label for="s-contact" class={labelClass}>{t('catalog.suppliers.contact')}</label>
          <input id="s-contact" class={inputClass} bind:value={ed.contact} maxlength="100" disabled={ro} aria-invalid={!!ed.errors.contact_name} />
          {#if ed.errors.contact_name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.contact_name}</p>{/if}
        </div>
        <div>
          <label for="s-phone" class={labelClass}>{t('catalog.suppliers.phone')}</label>
          <input id="s-phone" type="tel" class={inputClass} bind:value={ed.phone} maxlength="30" disabled={ro} aria-invalid={!!ed.errors.phone} />
          {#if ed.errors.phone}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.phone}</p>{/if}
        </div>
        <div>
          <label for="s-email" class={labelClass}>{t('catalog.suppliers.email')}</label>
          <input id="s-email" type="email" class={inputClass} bind:value={ed.email} maxlength="254" disabled={ro} aria-invalid={!!ed.errors.email} />
          {#if ed.errors.email}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.email}</p>{/if}
        </div>
        <div class="sm:col-span-2">
          <label for="s-address" class={labelClass}>{t('catalog.suppliers.address')}</label>
          <textarea id="s-address" rows="2" class={inputClass} bind:value={ed.address} maxlength="500" disabled={ro} aria-invalid={!!ed.errors.address}></textarea>
          {#if ed.errors.address}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.address}</p>{/if}
        </div>
        <div class="sm:col-span-2">
          <label for="s-note" class={labelClass}>{t('catalog.suppliers.note')}</label>
          <textarea id="s-note" rows="2" class={inputClass} bind:value={ed.note} maxlength="500" disabled={ro} aria-invalid={!!ed.errors.note}></textarea>
          {#if ed.errors.note}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.note}</p>{/if}
        </div>
      </div>

      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (editor = null)}>{ro ? t('catalog.close') : t('catalog.cancel')}</button>
        {#if !ro}
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={ed.saving}>{ed.saving ? t('catalog.saving') : t('catalog.save')}</button>
        {/if}
      </div>
    </form>
  </Modal>
{/if}
