<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { paymentMethods as api, type FeeBearer, type PaymentKind, type PaymentMethod } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import { focusOnMount } from '#lib/focus.ts';

  const PAGE = 20;
  // Jenis dasar yang boleh dipilih untuk metode baru. Tunai hanya satu (bawaan), jadi tidak ditawarkan.
  const NEW_KINDS: PaymentKind[] = ['transfer', 'debit', 'credit_card', 'ewallet'];
  const kindLabel = (k: PaymentKind) => t(`sales.method.${k}` as 'sales.method.cash');

  type Field = 'name' | 'kind' | 'fee_pct' | 'fee_flat' | 'fee_bearer';
  type Editor = {
    id: string | null;
    name: string;
    kind: PaymentKind;
    feePct: string;
    feeFlat: string;
    bearer: FeeBearer;
    system: boolean;
    saving: boolean;
    error: string;
    errors: Partial<Record<Field, string>>;
  };

  let rows = $state<PaymentMethod[]>([]);
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

  const canWrite = (m: PaymentMethod | null) => (m ? can('payment_methods', 'update') : can('payment_methods', 'create'));
  const openNew = () => (editor = { id: null, name: '', kind: 'ewallet', feePct: '', feeFlat: '', bearer: 'store', system: false, saving: false, error: '', errors: {} });
  const openMethod = (m: PaymentMethod) => (editor = { id: m.id, name: m.name, kind: m.kind, feePct: Number(m.fee_pct) ? m.fee_pct : '', feeFlat: Number(m.fee_flat) ? m.fee_flat : '', bearer: m.fee_bearer ?? 'store', system: m.is_system, saving: false, error: '', errors: {} });

  /** Ringkasan biaya untuk tabel, mis. "0,7% + Rp100"; "—" bila tanpa biaya. */
  function feeText(m: PaymentMethod): string {
    const parts: string[] = [];
    if (Number(m.fee_pct) > 0) parts.push(`${formatNumber(Number(m.fee_pct), { maximumFractionDigits: 2 })}%`);
    if (Number(m.fee_flat) > 0) parts.push(formatCurrency(Number(m.fee_flat), 'IDR', { maximumFractionDigits: 2 }));
    if (!parts.length) return t('catalog.paymentMethods.noFee');
    return `${parts.join(' + ')} · ${t(m.fee_bearer === 'customer' ? 'catalog.paymentMethods.bearerCustomer' : 'catalog.paymentMethods.bearerStore')}`;
  }

  const DEC_RE = /^\d+([.,]\d{1,2})?$/;

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = '';
    const next: Partial<Record<Field, string>> = {};
    const name = checkName(ed.name);
    if (name.code) next.name = fieldMessage(name.code);
    const pct = ed.feePct.trim().replace(',', '.');
    const flat = ed.feeFlat.trim().replace(',', '.');
    if (pct && (!DEC_RE.test(ed.feePct.trim()) || Number(pct) > 100)) next.fee_pct = fieldMessage('INVALID');
    if (flat && !DEC_RE.test(ed.feeFlat.trim())) next.fee_flat = fieldMessage('INVALID');
    ed.errors = next;
    if (Object.keys(next).length) return;

    // Kolom biaya yang kosong TIDAK dikirim: API membaca angka sebagai json.Number dan menolak teks kosong ("").
    const body = { name: name.value, kind: ed.kind, fee_bearer: ed.bearer, ...(pct ? { fee_pct: pct } : {}), ...(flat ? { fee_flat: flat } : {}) };
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
      } else {
        ed.error = errorMessage(err);
      }
    } finally {
      ed.saving = false;
    }
  }

  async function toggle(m: PaymentMethod) {
    busyId = m.id;
    notice = loadError = '';
    try {
      await api.setActive(m.id, !m.active);
      notice = t(m.active ? 'catalog.archived' : 'catalog.restored', { name: m.name });
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

<svelte:head><title>{t('catalog.paymentMethods.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('catalog.paymentMethods.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('catalog.paymentMethods.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('catalog.paymentMethods.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('catalog.paymentMethods.subtitle')}</p>
    </div>
    {#if can('payment_methods', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('catalog.paymentMethods.add')}</button>
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
        <input type="search" class="{inputClass} !ps-8" placeholder={t('catalog.paymentMethods.search')} aria-label={t('catalog.paymentMethods.search')} bind:value={q} maxlength="60" />
      </div>
      <Select class="!w-auto min-w-40" ariaLabel={t('catalog.status')} bind:value={filter} options={[{ value: 'all', label: t('catalog.filter.all') }, { value: 'active', label: t('catalog.filter.active') }, { value: 'inactive', label: t('catalog.filter.inactive') }]} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[600px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('catalog.name')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.paymentMethods.kind')}</th>
            <th class="p-3 text-end" scope="col">{t('catalog.paymentMethods.fee')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as m (m.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-semibold">
                {m.name}
                {#if m.is_system}<span class="badge-soft badge-info ms-1.5">{t('catalog.paymentMethods.system')}</span>{/if}
              </td>
              <td class="p-3">{kindLabel(m.kind)}</td>
              <td class="p-3 text-end tabular-nums">{feeText(m)}</td>
              <td class="p-3"><span class="badge-soft {m.active ? 'badge-success' : 'badge-danger'}">{m.active ? t('catalog.active') : t('catalog.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={canWrite(m) ? t('catalog.edit') : t('catalog.view')} onclick={() => openMethod(m)}>
                  <i class="{canWrite(m) ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
                {#if can('payment_methods', 'update') && !m.is_system}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={m.active ? t('catalog.archive') : t('catalog.restore')}
                    title={m.active ? t('catalog.archive') : t('catalog.restore')}
                    disabled={busyId === m.id}
                    onclick={() => toggle(m)}
                  >
                    <i class="{m.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="5" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' ? t('catalog.emptySearch') : t('catalog.empty')}</td></tr>
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
  {@const ro = ed.id !== null && !can('payment_methods', 'update')}
  <Modal title={ed.id ? t('catalog.paymentMethods.edit') : t('catalog.paymentMethods.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}

      <div>
        <label for="pm-name" class={labelClass}>{t('catalog.name')}</label>
        <input id="pm-name" class={inputClass} bind:value={ed.name} maxlength="60" disabled={ro} aria-invalid={!!ed.errors.name} use:focusOnMount />
        {#if ed.errors.name}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.name}</p>{:else}<p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('catalog.paymentMethods.nameHint')}</p>{/if}
      </div>
      <div>
        <label for="pm-kind" class={labelClass}>{t('catalog.paymentMethods.kind')}</label>
        {#if ed.id && ed.kind === 'cash'}
          <input id="pm-kind" class={inputClass} value={kindLabel(ed.kind)} disabled />
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('catalog.paymentMethods.systemHint')}</p>
        {:else}
          <Select id="pm-kind" ariaLabel={t('catalog.paymentMethods.kind')} bind:value={ed.kind} options={NEW_KINDS.map((k) => ({ value: k, label: kindLabel(k) }))} />
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('catalog.paymentMethods.kindHint')} {t('catalog.paymentMethods.kindCashHint')}</p>
        {/if}
        {#if ed.errors.kind}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.kind}</p>{/if}
      </div>

      {#if ed.kind !== 'cash'}
        <div>
          <span class={labelClass}>{t('catalog.paymentMethods.fee')}</span>
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label for="pm-fee-pct" class="text-[11.5px] mb-1 block text-[var(--text-secondary)]">{t('catalog.paymentMethods.feePct')}</label>
              <input id="pm-fee-pct" inputmode="decimal" class={inputClass} bind:value={ed.feePct} maxlength="6" placeholder="0" disabled={ro} aria-invalid={!!ed.errors.fee_pct} />
              {#if ed.errors.fee_pct}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.fee_pct}</p>{/if}
            </div>
            <div>
              <label for="pm-fee-flat" class="text-[11.5px] mb-1 block text-[var(--text-secondary)]">{t('catalog.paymentMethods.feeFlat')}</label>
              <input id="pm-fee-flat" inputmode="decimal" class={inputClass} bind:value={ed.feeFlat} maxlength="12" placeholder="0" disabled={ro} aria-invalid={!!ed.errors.fee_flat} />
              {#if ed.errors.fee_flat}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.fee_flat}</p>{/if}
            </div>
          </div>
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('catalog.paymentMethods.feeHint')}</p>
          <div class="mt-3">
            <label for="pm-bearer" class="text-[11.5px] mb-1 block text-[var(--text-secondary)]">{t('catalog.paymentMethods.bearer')}</label>
            <Select id="pm-bearer" ariaLabel={t('catalog.paymentMethods.bearer')} bind:value={ed.bearer} disabled={ro} options={[{ value: 'store', label: t('catalog.paymentMethods.bearerStore') }, { value: 'customer', label: t('catalog.paymentMethods.bearerCustomer') }]} />
            <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t(ed.bearer === 'customer' ? 'catalog.paymentMethods.bearerCustomerHint' : 'catalog.paymentMethods.bearerStoreHint')}</p>
            {#if ed.errors.fee_bearer}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{ed.errors.fee_bearer}</p>{/if}
          </div>
        </div>
      {:else}
        <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('catalog.paymentMethods.feeCash')}</p>
      {/if}

      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (editor = null)}>{ro ? t('catalog.close') : t('catalog.cancel')}</button>
        {#if !ro}
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={ed.saving}>{ed.saving ? t('catalog.saving') : t('catalog.save')}</button>
        {/if}
      </div>
    </form>
  </Modal>
{/if}
