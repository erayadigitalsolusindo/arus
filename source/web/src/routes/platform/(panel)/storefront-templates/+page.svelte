<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type TemplateRow, type TemplateInput } from '#lib/platform/api.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { t, tryT, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Switch from '#lib/components/Switch.svelte';

  type Form = { id: string | null; code: string; name: string; description: string; category: string; html: string; css: string; active: boolean; sort: string };

  let items = $state<TemplateRow[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let form = $state<Form | null>(null);
  let saving = $state(false);
  let fieldErrors = $state<Record<string, string>>({});
  let warnings = $state<string[]>([]);
  let tab = $state<'html' | 'css' | 'preview'>('html');
  let device = $state<'desktop' | 'mobile'>('desktop');
  let previewDoc = $state('');
  let previewError = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      items = await platformApi.storefrontTemplates();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => void load());

  function openNew() {
    form = { id: null, code: '', name: '', description: '', category: '', html: '<h1 class="text-3xl font-bold">{namatoko}</h1>', css: '', active: true, sort: String((items.at(-1)?.sort_order ?? 0) + 10) };
    tab = 'html';
    fieldErrors = {};
    warnings = [];
  }

  async function openEdit(row: TemplateRow) {
    error = '';
    try {
      const d = await platformApi.storefrontTemplate(row.id);
      form = { id: d.id, code: d.code, name: d.name, description: d.description, category: d.category, html: d.html, css: d.css, active: d.active, sort: String(d.sort_order) };
      tab = 'html';
      fieldErrors = {};
      warnings = [];
    } catch (err) {
      error = errorMessage(err);
    }
  }

  async function renderPreview() {
    if (!form) return;
    try {
      previewDoc = (await platformApi.previewStorefrontTemplate({ html: form.html, css: form.css })).html;
      previewError = '';
    } catch (err) {
      previewError = err instanceof ApiError && Object.keys(err.fields).length ? Object.keys(err.fields).join(', ') + ': ' + errorMessage(err) : errorMessage(err);
    }
  }
  function show(k: 'html' | 'css' | 'preview') {
    tab = k;
    if (k === 'preview') void renderPreview();
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!form) return;
    saving = true;
    error = '';
    fieldErrors = {};
    const body: TemplateInput = { name: form.name.trim(), description: form.description.trim(), category: form.category.trim(), html: form.html, css: form.css, active: form.active, sort_order: Number(form.sort) || 0 };
    try {
      const r = form.id ? await platformApi.updateStorefrontTemplate(form.id, body) : await platformApi.createStorefrontTemplate({ ...body, code: form.code.trim().toLowerCase() });
      warnings = r.warnings ?? [];
      notice = t('storefront.tpl.saved');
      if (warnings.length) {
        form.id = r.id; // tetap terbuka agar peringatan terbaca
      } else form = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) fieldErrors = err.fields;
      else error = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  async function remove(row: TemplateRow) {
    if (!confirm(t('storefront.tpl.deleteConfirm', { name: row.name }))) return;
    try {
      await platformApi.deleteStorefrontTemplate(row.id);
      notice = t('storefront.tpl.deleted');
      await load();
    } catch (err) {
      error = errorMessage(err);
    }
  }

  const fe = (k: string) => (fieldErrors[k] ? fieldMessage(fieldErrors[k]) : '');
  const kb = (n: number) => Math.max(1, Math.round(n / 1024)) + ' KB';
  const lbl = 'mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('storefront.tpl.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div class="max-w-3xl">
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('storefront.tpl.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)]">{t('storefront.tpl.subtitle')}</p>
    </div>
    <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('storefront.tpl.add')}</button>
  </div>

  {#if notice}<div class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]" role="status">{notice}</div>{/if}
  {#if error}<div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]" role="alert">{error}</div>{/if}

  <div class="surface-card overflow-x-auto">
    <table class="w-full text-[13px]">
      <thead class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
        <tr>
          <th class="px-4 py-2.5 text-start font-semibold">{t('storefront.tpl.name')}</th>
          <th class="px-4 py-2.5 text-start font-semibold">{t('storefront.tpl.category')}</th>
          <th class="px-4 py-2.5 text-end font-semibold">{t('storefront.tpl.size')}</th>
          <th class="px-4 py-2.5 text-start font-semibold">{t('storefront.tpl.updated')}</th>
          <th class="px-4 py-2.5 text-start font-semibold">{t('storefront.tpl.status')}</th>
          <th class="px-4 py-2.5 text-end font-semibold">{t('storefront.tpl.actions')}</th>
        </tr>
      </thead>
      <tbody>
        {#each items as row (row.id)}
          <tr class="border-t border-[var(--border-subtle)]">
            <td class="px-4 py-3"><span class="block font-semibold">{row.name}</span><span class="font-mono text-[11px] text-[var(--text-tertiary)]">{row.code}</span>{#if row.description}<span class="block max-w-md text-[12px] text-[var(--text-tertiary)]">{row.description}</span>{/if}</td>
            <td class="px-4 py-3">{row.category || '—'}</td>
            <td class="px-4 py-3 text-end tabular-nums">{kb(row.bytes)}</td>
            <td class="px-4 py-3 whitespace-nowrap">{formatDateTime(row.updated_at)}</td>
            <td class="px-4 py-3"><span class={row.active ? 'badge-success' : 'badge-warning'}>{row.active ? t('storefront.tpl.live') : t('storefront.tpl.hidden')}</span></td>
            <td class="px-4 py-3 text-end whitespace-nowrap">
              <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px]" onclick={() => openEdit(row)}><i class="icon-pencil text-[12px]"></i>{t('storefront.tpl.edit')}</button>
              <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px] text-[var(--color-danger-600)]" onclick={() => remove(row)}><i class="icon-trash-2 text-[12px]"></i>{t('storefront.tpl.del')}</button>
            </td>
          </tr>
        {:else}
          <tr><td colspan="6" class="px-4 py-8 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('storefront.tpl.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

{#if form}
  <Modal title={form.id ? t('storefront.tpl.edit') : t('storefront.tpl.add')} onclose={() => (form = null)} xl>
    <form class="space-y-4" onsubmit={save}>
      {#if error}<div class="rounded-lg bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]" role="alert">{error}</div>{/if}
      <div class="grid gap-3 md:grid-cols-3">
        <div>
          <label class={lbl} for="tp-name">{t('storefront.tpl.name')}</label>
          <input id="tp-name" class="form-input !text-[13px] w-full" bind:value={form.name} maxlength="60" required />
          {#if fe('name')}<p class="mt-1 text-[11.5px] text-[var(--color-danger-600)]">{fe('name')}</p>{/if}
        </div>
        <div>
          <label class={lbl} for="tp-code">{t('storefront.tpl.code')}</label>
          <input id="tp-code" class="form-input !text-[13px] w-full font-mono" bind:value={form.code} disabled={!!form.id} maxlength="40" placeholder="kopi-ekspor" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('storefront.tpl.codeHelp')}</p>
          {#if fe('code')}<p class="text-[11.5px] text-[var(--color-danger-600)]">{fe('code')}</p>{/if}
        </div>
        <div>
          <label class={lbl} for="tp-cat">{t('storefront.tpl.category')}</label>
          <input id="tp-cat" class="form-input !text-[13px] w-full" bind:value={form.category} maxlength="40" />
        </div>
        <div class="md:col-span-2">
          <label class={lbl} for="tp-desc">{t('storefront.tpl.description')}</label>
          <input id="tp-desc" class="form-input !text-[13px] w-full" bind:value={form.description} maxlength="300" />
        </div>
        <div class="flex items-end gap-4">
          <div class="w-24">
            <label class={lbl} for="tp-sort">{t('storefront.tpl.sort')}</label>
            <input id="tp-sort" class="form-input !text-[13px] w-full text-end" inputmode="numeric" bind:value={form.sort} />
          </div>
          <label class="flex items-center gap-2 pb-2 text-[12.5px]"><Switch bind:checked={form.active} label={t('storefront.tpl.active')} />{t('storefront.tpl.active')}</label>
        </div>
      </div>

      <div class="rounded-xl border border-[var(--border-default)] overflow-hidden">
        <div class="flex items-center gap-1 px-2 py-1.5 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[12.5px] font-semibold">
          {#each [['html', 'index.html'], ['css', 'style.css'], ['preview', t('storefront.preview')]] as const as [k, label] (k)}
            <button type="button" class="rounded-lg px-3 py-1.5 {tab === k ? 'bg-[var(--surface-card)] text-[var(--color-primary-600)] shadow-sm' : 'text-[var(--text-secondary)]'}" onclick={() => show(k)}>{label}</button>
          {/each}
          {#if tab === 'preview'}
            <span class="ms-auto inline-flex rounded-lg border border-[var(--border-default)] overflow-hidden text-[12px] font-normal">
              <button type="button" class="px-2.5 py-1 {device === 'desktop' ? 'bg-[var(--color-primary-600)] text-white' : ''}" aria-label={t('storefront.desktop')} onclick={() => (device = 'desktop')}><i class="icon-monitor text-[13px]"></i></button>
              <button type="button" class="px-2.5 py-1 {device === 'mobile' ? 'bg-[var(--color-primary-600)] text-white' : ''}" aria-label={t('storefront.mobile')} onclick={() => (device = 'mobile')}><i class="icon-smartphone text-[13px]"></i></button>
            </span>
            <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px]" onclick={renderPreview}><i class="icon-rotate-ccw text-[12px]"></i>{t('storefront.refresh')}</button>
          {/if}
        </div>
        {#if tab === 'html'}
          <textarea class="tcode" style="background-color:#0d1424 !important;color:#e2e8f0 !important" spellcheck="false" aria-label="HTML" bind:value={form.html}></textarea>
        {:else if tab === 'css'}
          <textarea class="tcode" style="background-color:#0d1424 !important;color:#e2e8f0 !important" spellcheck="false" aria-label="CSS" bind:value={form.css}></textarea>
        {:else}
          {#if previewError}<p class="px-3 py-2 text-[12px] text-[var(--color-danger-600)]" role="alert">{previewError}</p>{/if}
          <div class="flex justify-center bg-[var(--surface-sunken)] p-3" style="height:min(60vh,560px)">
            <iframe title={t('storefront.preview')} sandbox="allow-popups allow-popups-to-escape-sandbox" srcdoc={previewDoc} class="h-full max-w-full rounded-lg border border-[var(--border-default)] bg-white" style="width:{device === 'mobile' ? '390px' : '100%'}"></iframe>
          </div>
        {/if}
        <p class="px-3 py-1.5 text-[11px] text-[var(--text-tertiary)] border-t border-[var(--border-subtle)]">{t('storefront.tpl.previewNote')} · {form.html.length.toLocaleString()} / 60.000 · CSS {form.css.length.toLocaleString()} / 20.000</p>
      </div>
      {#if fe('html') || fe('css')}<p class="text-[12px] text-[var(--color-danger-600)]">{fe('html') ? tryT('storefront.fe.html') : ''} {fe('css') ? tryT('storefront.fe.css') : ''}</p>{/if}
      {#if warnings.length}
        <div class="rounded-lg border border-[var(--color-warning-500)]/40 bg-[var(--color-warning-500)]/10 px-3 py-2 text-[12.5px]"><p class="font-semibold">{t('storefront.tpl.warnings')}</p><ul class="mt-1 list-disc ps-5">{#each warnings as w (w)}<li>{w}</li>{/each}</ul></div>
      {/if}
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-ghost !text-[12.5px]" onclick={() => (form = null)}>{t('common.close')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={saving}>{saving ? t('storefront.tpl.saving') : t('storefront.tpl.save')}</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .tcode {
    display: block;
    width: 100%;
    height: min(60vh, 560px);
    resize: none;
    padding: 12px 14px;
    font: 12.5px/1.6 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace !important;
    border: 0 !important;
    border-radius: 0 !important;
    box-shadow: none !important;
    outline: none;
    tab-size: 2;
    white-space: pre;
    overflow: auto;
  }
</style>
