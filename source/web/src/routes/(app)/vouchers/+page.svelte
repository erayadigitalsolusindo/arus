<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import DateRange from '#lib/components/DateRange.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { vouchers as api, voucherState, type Voucher, type VoucherKind, type VoucherState } from '#lib/vouchers/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { cleanText } from '#lib/validation.ts';
  import { focusOnMount } from '#lib/focus.ts';

  const PAGE = 20;
  const CODE_RE = /^[A-Z0-9][A-Z0-9_-]{2,31}$/;

  type Field = 'code' | 'name' | 'value' | 'max_discount' | 'min_spend' | 'starts_on' | 'ends_on' | 'max_uses';
  type Editor = {
    id: string | null;
    used: number;
    code: string;
    name: string;
    kind: VoucherKind;
    value: string;
    maxDiscount: string;
    minSpend: string;
    from: string;
    to: string;
    maxUses: string;
    saving: boolean;
    error: string;
    errors: Partial<Record<Field, string>>;
  };

  let rows = $state<Voucher[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let filter = $state<'all' | 'active' | 'inactive'>('all');
  let editor = $state<Editor | null>(null);
  let busyId = $state<string | null>(null);

  // Hari ini menurut jam perangkat; hanya untuk label status (server yang memutuskan saat kupon dipakai).
  const d0 = new Date();
  const today = `${d0.getFullYear()}-${String(d0.getMonth() + 1).padStart(2, '0')}-${String(d0.getDate()).padStart(2, '0')}`;

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

  const money = (s: string) => formatCurrency(Number(s || 0));
  const day = (s: string) => {
    const [y, m, d] = s.split('-').map(Number);
    return formatDate(new Date(y, m - 1, d));
  };
  const trim0 = (s: string) => (s.includes('.') ? s.replace(/\.?0+$/, '') : s);
  const valueText = (v: Voucher) => (v.kind === 'percent' ? `${trim0(v.value)}%` : money(v.value));
  const validityText = (v: Voucher) => {
    if (v.starts_on && v.ends_on) return t('vouchers.between', { from: day(v.starts_on), to: day(v.ends_on) });
    if (v.starts_on) return t('vouchers.from', { date: day(v.starts_on) });
    if (v.ends_on) return t('vouchers.until', { date: day(v.ends_on) });
    return t('vouchers.always');
  };
  const usageText = (v: Voucher) => (v.max_uses === null ? t('vouchers.usageUnlimited', { used: v.used_count }) : t('vouchers.usage', { used: v.used_count, max: v.max_uses }));
  const badge: Record<VoucherState, string> = { running: 'badge-success', upcoming: 'badge-info', expired: 'badge-neutral', exhausted: 'badge-warning', inactive: 'badge-danger' };

  const canWrite = (v: Voucher | null) => (v ? can('coupons', 'update') : can('coupons', 'create'));
  const blank = (): Editor => ({ id: null, used: 0, code: '', name: '', kind: 'percent', value: '', maxDiscount: '', minSpend: '', from: '', to: '', maxUses: '', saving: false, error: '', errors: {} });
  const openNew = () => (editor = blank());
  const openVoucher = (v: Voucher) =>
    (editor = {
      ...blank(),
      id: v.id,
      used: v.used_count,
      code: v.code,
      name: v.name,
      kind: v.kind,
      value: trim0(v.value),
      maxDiscount: v.max_discount ? trim0(v.max_discount) : '',
      minSpend: Number(v.min_spend) ? trim0(v.min_spend) : '',
      from: v.starts_on,
      to: v.ends_on,
      maxUses: v.max_uses === null ? '' : String(v.max_uses)
    });

  const positive = (s: string) => /^\d+(\.\d{1,2})?$/.test(s) && Number(s) > 0;

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!editor) return;
    const ed = editor;
    ed.error = '';
    const next: Partial<Record<Field, string>> = {};

    const code = ed.code.trim().toUpperCase();
    if (!code) next.code = fieldMessage('REQUIRED');
    else if (!CODE_RE.test(code)) next.code = fieldMessage('INVALID');
    const name = cleanText(ed.name);
    if (!name) next.name = fieldMessage(name === null ? 'INVALID' : 'REQUIRED');
    else if ([...name].length > 100) next.name = fieldMessage('TOO_LONG');
    if (!positive(ed.value)) next.value = fieldMessage(ed.value ? 'INVALID' : 'REQUIRED');
    else if (ed.kind === 'percent' && Number(ed.value) > 100) next.value = fieldMessage('INVALID');
    if (ed.kind === 'percent' && ed.maxDiscount && !positive(ed.maxDiscount)) next.max_discount = fieldMessage('INVALID');
    if (ed.minSpend && !/^\d+(\.\d{1,2})?$/.test(ed.minSpend)) next.min_spend = fieldMessage('INVALID');
    if (ed.maxUses && !/^[1-9]\d{0,8}$/.test(ed.maxUses)) next.max_uses = fieldMessage('INVALID');
    ed.errors = next;
    if (Object.keys(next).length) return;

    const body = {
      code,
      name: name ?? '',
      kind: ed.kind,
      value: ed.value,
      ...(ed.kind === 'percent' && ed.maxDiscount ? { max_discount: ed.maxDiscount } : {}),
      ...(ed.minSpend ? { min_spend: ed.minSpend } : {}),
      ...(ed.from ? { starts_on: ed.from, ends_on: ed.to || ed.from } : {}),
      ...(ed.maxUses ? { max_uses: Number(ed.maxUses) } : {})
    };
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
      } else if (err instanceof ApiError && err.code === 'CODE_TAKEN') {
        ed.errors.code = errorMessage(err);
      } else {
        ed.error = errorMessage(err);
      }
    } finally {
      ed.saving = false;
    }
  }

  async function toggle(v: Voucher) {
    busyId = v.id;
    notice = loadError = '';
    try {
      await api.setActive(v.id, !v.active);
      notice = t(v.active ? 'catalog.archived' : 'catalog.restored', { name: v.code });
      await load();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      busyId = null;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] mt-1 text-[var(--color-danger-600)]';
  const hintClass = 'text-[11px] mt-1 text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('vouchers.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('vouchers.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('vouchers.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('vouchers.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('vouchers.subtitle')}</p>
    </div>
    {#if can('coupons', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('vouchers.add')}</button>
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
        <input type="search" class="{inputClass} !ps-8" placeholder={t('vouchers.search')} aria-label={t('vouchers.search')} bind:value={q} maxlength="100" />
      </div>
      <Select class="!w-auto min-w-40" ariaLabel={t('catalog.status')} bind:value={filter} options={[{ value: 'all', label: t('catalog.filter.all') }, { value: 'active', label: t('catalog.filter.active') }, { value: 'inactive', label: t('catalog.filter.inactive') }]} />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[860px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('vouchers.code')}</th>
            <th class="p-3 text-start" scope="col">{t('vouchers.name')}</th>
            <th class="p-3 text-end" scope="col">{t('vouchers.discount')}</th>
            <th class="p-3 text-end" scope="col">{t('vouchers.minSpend')}</th>
            <th class="p-3 text-start" scope="col">{t('vouchers.validity')}</th>
            <th class="p-3 text-end" scope="col">{t('vouchers.used')}</th>
            <th class="p-3 text-start" scope="col">{t('catalog.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as v (v.id)}
            {@const st = voucherState(v, today)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-mono font-semibold text-[12px]">{v.code}</td>
              <td class="p-3">{v.name}</td>
              <td class="p-3 text-end tabular-nums font-semibold">
                {valueText(v)}
                {#if v.max_discount}<div class="text-[10.5px] font-normal text-[var(--text-tertiary)]">{t('vouchers.upTo', { amount: money(v.max_discount) })}</div>{/if}
              </td>
              <td class="p-3 text-end tabular-nums">{Number(v.min_spend) ? money(v.min_spend) : '–'}</td>
              <td class="p-3 whitespace-nowrap">{validityText(v)}</td>
              <td class="p-3 text-end tabular-nums">{usageText(v)}</td>
              <td class="p-3"><span class="badge-soft {badge[st]}">{t(`vouchers.status.${st}`)}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={canWrite(v) ? t('catalog.edit') : t('catalog.view')} onclick={() => openVoucher(v)}>
                  <i class="{canWrite(v) ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
                {#if can('coupons', 'update')}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={v.active ? t('catalog.archive') : t('catalog.restore')}
                    title={v.active ? t('catalog.archive') : t('catalog.restore')}
                    disabled={busyId === v.id}
                    onclick={() => toggle(v)}
                  >
                    <i class="{v.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="8" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' ? t('catalog.emptySearch') : t('catalog.empty')}</td></tr>
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
  {@const ro = ed.id !== null && !can('coupons', 'update')}
  <Modal wide title={ed.id ? t('vouchers.edit') : t('vouchers.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}

      <div class="grid gap-3.5 sm:grid-cols-2">
        <div>
          <label for="v-code" class={labelClass}>{t('vouchers.code')}</label>
          <input id="v-code" class="{inputClass} font-mono uppercase" bind:value={ed.code} maxlength="32" disabled={ro || ed.used > 0} aria-invalid={!!ed.errors.code} autocomplete="off" use:focusOnMount />
          {#if ed.errors.code}<p class={errClass}>{ed.errors.code}</p>{:else}<p class={hintClass}>{t('vouchers.codeHint')}</p>{/if}
        </div>
        <div>
          <label for="v-name" class={labelClass}>{t('vouchers.name')}</label>
          <input id="v-name" class={inputClass} bind:value={ed.name} maxlength="100" disabled={ro} aria-invalid={!!ed.errors.name} />
          {#if ed.errors.name}<p class={errClass}>{ed.errors.name}</p>{/if}
        </div>

        <div>
          <span class={labelClass}>{t('vouchers.kind')}</span>
          <Select
            bind:value={ed.kind}
            ariaLabel={t('vouchers.kind')}
            disabled={ro}
            options={[
              { value: 'percent', label: t('vouchers.kindPercent') },
              { value: 'amount', label: t('vouchers.kindAmount') }
            ]}
          />
        </div>
        <div>
          <label for="v-value" class={labelClass}>{t('vouchers.value')}</label>
          <MoneyInput id="v-value" bind:value={ed.value} decimals={2} pad={false} disabled={ro} aria-invalid={!!ed.errors.value} class={inputClass} />
          {#if ed.errors.value}<p class={errClass}>{ed.errors.value}</p>{:else}<p class={hintClass}>{ed.kind === 'percent' ? t('vouchers.valuePercentHint') : t('vouchers.valueAmountHint')}</p>{/if}
        </div>

        {#if ed.kind === 'percent'}
          <div>
            <label for="v-cap" class={labelClass}>{t('vouchers.maxDiscount')}</label>
            <MoneyInput id="v-cap" bind:value={ed.maxDiscount} decimals={2} pad={false} disabled={ro} aria-invalid={!!ed.errors.max_discount} class={inputClass} />
            {#if ed.errors.max_discount}<p class={errClass}>{ed.errors.max_discount}</p>{:else}<p class={hintClass}>{t('vouchers.maxDiscountHint')}</p>{/if}
          </div>
        {/if}
        <div>
          <label for="v-min" class={labelClass}>{t('vouchers.minSpend')}</label>
          <MoneyInput id="v-min" bind:value={ed.minSpend} decimals={2} pad={false} disabled={ro} aria-invalid={!!ed.errors.min_spend} class={inputClass} />
          {#if ed.errors.min_spend}<p class={errClass}>{ed.errors.min_spend}</p>{:else}<p class={hintClass}>{t('vouchers.minSpendHint')}</p>{/if}
        </div>

        <div>
          <span class={labelClass}>{t('vouchers.period')}</span>
          <div class="flex items-center gap-1.5">
            {#if ro}
              <output class="{inputClass} flex items-center">{ed.from ? `${day(ed.from)} – ${day(ed.to || ed.from)}` : t('vouchers.always')}</output>
            {:else}
              <DateRange bind:from={ed.from} bind:to={ed.to} ariaLabel={t('vouchers.period')} class="grow" />
              {#if ed.from}
                <button type="button" class="header-icon-btn !size-9 shrink-0" aria-label={t('common.datePicker.clear')} title={t('common.datePicker.clear')} onclick={() => ((ed.from = ''), (ed.to = ''))}><i class="icon-x text-[13px]"></i></button>
              {/if}
            {/if}
          </div>
          {#if ed.errors.starts_on || ed.errors.ends_on}<p class={errClass}>{ed.errors.starts_on ?? ed.errors.ends_on}</p>{:else}<p class={hintClass}>{t('vouchers.periodHint')}</p>{/if}
        </div>
        <div>
          <label for="v-uses" class={labelClass}>{t('vouchers.maxUses')}</label>
          <input id="v-uses" inputmode="numeric" class={inputClass} value={ed.maxUses} oninput={(e) => (ed.maxUses = e.currentTarget.value.replace(/\D/g, ''))} maxlength="9" disabled={ro} aria-invalid={!!ed.errors.max_uses} />
          {#if ed.errors.max_uses}<p class={errClass}>{ed.errors.max_uses}</p>{:else}<p class={hintClass}>{t('vouchers.maxUsesHint')}{ed.id ? ` · ${t('vouchers.used')}: ${ed.used}` : ''}</p>{/if}
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
