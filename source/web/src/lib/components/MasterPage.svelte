<script lang="ts">
  // Halaman master sederhana (nama + status): satuan, kategori, brand, principal. Teks judul dikirim dari route
  // (pemanggil memakai kunci i18n literal agar tetap dicek tipe); sisanya memakai kamus `catalog.*`.
  import { ApiError } from '#lib/api/client.ts';
  import { simple, type Entry, type SimpleKind } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  let {
    kind,
    docTitle,
    title,
    subtitle,
    addLabel,
    editLabel
  }: { kind: SimpleKind; docTitle: string; title: string; subtitle: string; addLabel: string; editLabel: string } = $props();

  const PAGE = 20;
  // kind tetap selama halaman hidup (satu route = satu kind).
  // svelte-ignore state_referenced_locally
  const api = simple(kind);

  type Editor = { id: string | null; name: string; saving: boolean; error: string; fieldError: string };

  let rows = $state<Entry[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let filter = $state<'all' | 'active' | 'inactive'>('all');
  let editor = $state<Editor | null>(null);
  let busyId = $state<string | null>(null);

  let seq = 0; // hanya respons permintaan terbaru yang dipakai (pencarian cepat tidak menimpa hasil baru)
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

  // Cari/ubah filter → kembali ke halaman 1 setelah jeda singkat (debounce).
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

  const canWrite = (e: Entry | null) => (e ? can(kind, 'update') : can(kind, 'create'));
  const openNew = () => (editor = { id: null, name: '', saving: false, error: '', fieldError: '' });
  const openEntry = (e: Entry) => (editor = { id: e.id, name: e.name, saving: false, error: '', fieldError: '' });

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = ed.fieldError = '';
    const name = checkName(ed.name);
    if (name.code) {
      ed.fieldError = fieldMessage(name.code) ?? '';
      return;
    }
    ed.saving = true;
    try {
      if (ed.id) {
        await api.rename(ed.id, name.value);
        notice = t('catalog.saved');
      } else {
        await api.create(name.value);
        notice = t('catalog.created');
      }
      editor = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') ed.fieldError = fieldMessage(err.fields.name) ?? errorMessage(err);
      else if (err instanceof ApiError && err.code === 'NAME_TAKEN') ed.fieldError = errorMessage(err);
      else ed.error = errorMessage(err);
    } finally {
      ed.saving = false;
    }
  }

  async function toggle(e: Entry) {
    busyId = e.id;
    notice = loadError = '';
    try {
      await api.setActive(e.id, !e.active);
      notice = t(e.active ? 'catalog.archived' : 'catalog.restored', { name: e.name });
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

<svelte:head><title>{docTitle}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{title}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{title}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{title}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{subtitle}</p>
    </div>
    {#if can(kind, 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{addLabel}</button>
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
        <input type="search" class="{inputClass} !ps-8" placeholder={t('catalog.search')} aria-label={t('catalog.search')} bind:value={q} maxlength="100" />
      </div>
      <select class="field-control" aria-label={t('catalog.status')} bind:value={filter}>
        <option value="all">{t('catalog.filter.all')}</option>
        <option value="active">{t('catalog.filter.active')}</option>
        <option value="inactive">{t('catalog.filter.inactive')}</option>
      </select>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[420px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('catalog.name')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as e (e.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-semibold">{e.name}</td>
              <td class="p-3"><span class="badge-soft {e.active ? 'badge-success' : 'badge-danger'}">{e.active ? t('catalog.active') : t('catalog.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={canWrite(e) ? t('catalog.edit') : t('catalog.view')} onclick={() => openEntry(e)}>
                  <i class="{canWrite(e) ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
                {#if can(kind, 'update')}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={e.active ? t('catalog.archive') : t('catalog.restore')}
                    title={e.active ? t('catalog.archive') : t('catalog.restore')}
                    disabled={busyId === e.id}
                    onclick={() => toggle(e)}
                  >
                    <i class="{e.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="3" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' ? t('catalog.emptySearch') : t('catalog.empty')}</td></tr>
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
  {@const ro = ed.id !== null && !can(kind, 'update')}
  <Modal title={ed.id ? editLabel : addLabel} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}
      <div>
        <label for="m-name" class={labelClass}>{t('catalog.name')}</label>
        <input id="m-name" class={inputClass} bind:value={ed.name} maxlength="100" disabled={ro} aria-invalid={!!ed.fieldError} use:focusOnMount />
        {#if ed.fieldError}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.fieldError}</p>{/if}
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
