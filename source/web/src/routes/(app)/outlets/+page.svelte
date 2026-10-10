<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { outlets as api, type Outlet } from '#lib/outlets/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { refreshOutlets } from '#lib/outlets/store.svelte.ts';

  // Zona waktu Indonesia; zona lain dapat ditambah di sini tanpa mengubah backend (divalidasi lewat tzdata).
  const ZONES = ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura'] as const;
  const CODE_RE = /^[a-z0-9][a-z0-9_-]{1,19}$/;

  type Field = 'code' | 'name' | 'timezone' | 'tax_store_pct' | 'tax_gov_pct' | 'address' | 'phone' | 'receipt_header' | 'receipt_footer';
  type Editor = {
    id: string | null;
    code: string;
    name: string;
    timezone: string;
    taxStore: string;
    taxGov: string;
    active: boolean;
    address: string;
    phone: string;
    header: string;
    footer: string;
    saving: boolean;
    error: string;
    errors: Partial<Record<Field, string>>;
  };

  let list = $state<Outlet[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let editor = $state<Editor | null>(null);

  async function load() {
    loading = true;
    loadError = '';
    try {
      list = await api.list();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  const canWrite = (o: Outlet | null) => (o ? can('outlets', 'update') : can('outlets', 'create'));

  function openNew() {
    editor = { id: null, code: '', name: '', timezone: ZONES[0], taxStore: '0', taxGov: '0', active: true, address: '', phone: '', header: '', footer: '', saving: false, error: '', errors: {} };
  }

  function openOutlet(o: Outlet) {
    editor = { id: o.id, code: o.code, name: o.name, timezone: o.timezone, taxStore: o.tax_store_pct, taxGov: o.tax_gov_pct, active: o.active,
      address: o.address, phone: o.phone, header: o.receipt_header, footer: o.receipt_footer, saving: false, error: '', errors: {} };
  }

  /** Persen 0..100, maks 2 desimal; kosong dianggap 0. */
  function parsePct(s: string): number | null {
    const v = s.trim() === '' ? 0 : Number(s);
    if (!Number.isFinite(v) || v < 0 || v > 100 || Math.round(v * 100) / 100 !== v) return null;
    return v;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = '';
    const next: Partial<Record<Field, string>> = {};
    const name = checkName(ed.name);
    if (name.code) next.name = fieldMessage(name.code);
    if (!ed.id && !CODE_RE.test(ed.code.trim().toLowerCase())) next.code = fieldMessage('INVALID');
    const store = parsePct(ed.taxStore);
    const gov = parsePct(ed.taxGov);
    if (store === null) next.tax_store_pct = fieldMessage('INVALID');
    if (gov === null) next.tax_gov_pct = fieldMessage('INVALID');
    ed.errors = next;
    if (Object.keys(next).length) return;

    ed.saving = true;
    try {
      const body = {
        name: name.value, timezone: ed.timezone, tax_store_pct: store!, tax_gov_pct: gov!,
        address: ed.address, phone: ed.phone, receipt_header: ed.header, receipt_footer: ed.footer
      };
      if (ed.id) {
        await api.update(ed.id, { ...body, active: ed.active });
        notice = t('outlets.saved');
      } else {
        await api.create({ ...body, code: ed.code.trim().toLowerCase() });
        notice = t('outlets.created');
      }
      editor = null;
      await load();
      void refreshOutlets(); // pemilih outlet di sidebar
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, code] of Object.entries(err.fields)) ed.errors[k as Field] = fieldMessage(code);
      } else if (err instanceof ApiError && err.code === 'OUTLET_CODE_TAKEN') {
        ed.errors.code = errorMessage(err);
      } else {
        ed.error = errorMessage(err);
      }
    } finally {
      ed.saving = false;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('outlets.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('outlets.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('outlets.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('outlets.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('outlets.subtitle')}</p>
    </div>
    {#if can('outlets', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('outlets.add')}</button>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('outlets.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('outlets.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[640px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('outlets.name')}</th>
            <th class="p-3 text-start" scope="col">{t('outlets.code')}</th>
            <th class="p-3 text-start" scope="col">{t('outlets.timezone')}</th>
            <th class="p-3 text-start" scope="col">{t('outlets.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each list as o (o.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-semibold">
                {o.name}
                {#if o.id === session.outlet?.id}<span class="badge-soft badge-info ms-2">{t('outlets.current')}</span>{/if}
              </td>
              <td class="p-3 font-mono text-[12px]">{o.code}</td>
              <td class="p-3">{o.timezone}</td>
              <td class="p-3"><span class="badge-soft {o.active ? 'badge-success' : 'badge-danger'}">{o.active ? t('outlets.active') : t('outlets.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={t('outlets.edit')} onclick={() => openOutlet(o)}>
                  <i class="{canWrite(o) ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
              </td>
            </tr>
          {:else}
            <tr><td colspan="5" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('outlets.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</main>

{#if editor}
  {@const ed = editor}
  {@const ro = ed.id !== null && !can('outlets', 'update')}
  <Modal title={ed.id ? t('outlets.edit') : t('outlets.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}

      <div>
        <label for="o-name" class={labelClass}>{t('outlets.name')}</label>
        <input id="o-name" class={inputClass} bind:value={ed.name} maxlength="100" disabled={ro} aria-invalid={!!ed.errors.name} />
        {#if ed.errors.name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.name}</p>{/if}
      </div>

      <div>
        <label for="o-code" class={labelClass}>{t('outlets.code')}</label>
        <input id="o-code" class="{inputClass} font-mono" bind:value={ed.code} maxlength="20" disabled={ed.id !== null} autocomplete="off" aria-invalid={!!ed.errors.code} />
        {#if ed.errors.code}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.code}</p>{/if}
        {#if ed.id === null}<p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('outlets.codeHint')}</p>{/if}
      </div>

      <div>
        <label for="o-tz" class={labelClass}>{t('outlets.timezone')}</label>
        <Select id="o-tz" bind:value={ed.timezone} disabled={ro} options={[...ZONES, ...(ZONES.includes(ed.timezone as (typeof ZONES)[number]) ? [] : [ed.timezone])].map((z) => ({ value: z as string, label: z }))} />
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="o-ts" class={labelClass}>{t('outlets.taxStore')}</label>
          <MoneyInput id="o-ts" pad={false} class={inputClass} bind:value={ed.taxStore} disabled={ro} aria-invalid={!!ed.errors.tax_store_pct} />
          {#if ed.errors.tax_store_pct}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.tax_store_pct}</p>{/if}
        </div>
        <div>
          <label for="o-tg" class={labelClass}>{t('outlets.taxGov')}</label>
          <MoneyInput id="o-tg" pad={false} class={inputClass} bind:value={ed.taxGov} disabled={ro} aria-invalid={!!ed.errors.tax_gov_pct} />
          {#if ed.errors.tax_gov_pct}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.tax_gov_pct}</p>{/if}
        </div>
      </div>
      <p class="text-[11px] -mt-2 text-[var(--text-tertiary)]">{t('outlets.taxHint')}</p>

      <fieldset class="space-y-3 rounded-lg border border-[var(--border-subtle)] p-3">
        <legend class="px-1 text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]"><i class="icon-receipt text-[12px] me-1"></i>{t('outlets.receipt.title')}</legend>
        <p class="text-[11px] -mt-1 text-[var(--text-tertiary)]">{t('outlets.receipt.hint')}</p>
        {#each [['address', 'o-addr', 200, 2], ['receipt_header', 'o-rh', 300, 2], ['receipt_footer', 'o-rf', 300, 3]] as [field, id, max, rows] (field)}
          {@const key = field === 'address' ? 'address' : field === 'receipt_header' ? 'header' : 'footer'}
          <div>
            <label for={id as string} class={labelClass}>{t(`outlets.receipt.${key}`)}</label>
            <textarea id={id as string} class="{inputClass} !h-auto py-2 font-mono text-[12px]" rows={rows as number} maxlength={max as number} bind:value={ed[key]} disabled={ro} aria-invalid={!!ed.errors[field as Field]}></textarea>
            {#if ed.errors[field as Field]}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors[field as Field]}</p>{/if}
          </div>
        {/each}
        <div>
          <label for="o-phone" class={labelClass}>{t('outlets.receipt.phone')}</label>
          <input id="o-phone" class={inputClass} bind:value={ed.phone} maxlength="30" disabled={ro} aria-invalid={!!ed.errors.phone} />
          {#if ed.errors.phone}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.phone}</p>{/if}
        </div>
      </fieldset>

      {#if ed.id !== null}
        <label class="flex items-center gap-2 text-[12.5px] font-medium cursor-pointer">
          <input type="checkbox" class="size-4 rounded accent-[var(--color-primary-600)]" bind:checked={ed.active} disabled={ro} />
          {t('outlets.active')}
        </label>
      {/if}

      <div class="flex items-center gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px] flex-1" onclick={() => (editor = null)}>{ro ? t('outlets.close') : t('outlets.cancel')}</button>
        {#if !ro}
          <button type="submit" class="btn btn-primary !text-[12.5px] flex-1 disabled:opacity-60" disabled={ed.saving}>{ed.saving ? t('outlets.saving') : t('outlets.save')}</button>
        {/if}
      </div>
    </form>
  </Modal>
{/if}
