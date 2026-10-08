<script lang="ts">
  import { ApiError } from '#lib/api/client.ts';
  import { levels as api, type Level } from '#lib/members/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  type Field = 'name' | 'min_points' | 'spend_per_point' | 'point_value';
  type Editor = { id: string | null; base: boolean; name: string; min: string; spend: string; value: string; saving: boolean; error: string; errors: Partial<Record<Field, string>> };

  let rows = $state<Level[]>([]);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let editor = $state<Editor | null>(null);
  let busyId = $state<string | null>(null);

  async function load() {
    loading = true;
    loadError = '';
    try {
      rows = await api.list();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  void load();

  // Rentang poin tiap level aktif = [min, min level aktif berikutnya − 1]; level terakhir tanpa batas atas.
  const ranges = $derived.by(() => {
    const act = rows.filter((r) => r.active).sort((a, b) => a.min_points - b.min_points);
    const m = new Map<string, string>();
    act.forEach((r, i) => {
      const next = act[i + 1];
      m.set(r.id, next ? t('members.levels.range', { from: formatNumber(r.min_points), to: formatNumber(next.min_points - 1) }) : t('members.levels.rangeOpen', { from: formatNumber(r.min_points) }));
    });
    return m;
  });

  const canEdit = $derived(can('member_categories', 'update'));

  function openNew() {
    editor = { id: null, base: false, name: '', min: '', spend: '', value: '', saving: false, error: '', errors: {} };
  }
  function openEdit(l: Level) {
    editor = {
      id: l.id,
      base: l.min_points === 0,
      name: l.name,
      min: String(l.min_points),
      spend: trim(l.spend_per_point),
      value: trim(l.point_value),
      saving: false,
      error: '',
      errors: {}
    };
  }
  const trim = (s: string) => (s.includes('.') ? s.replace(/0+$/, '').replace(/\.$/, '') : s);

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    const ed = editor;
    if (!ed) return;
    const errs: Partial<Record<Field, string>> = {};
    const n = checkName(ed.name, 60);
    if (n.code) errs.name = fieldMessage(n.code);
    if (!/^\d{1,9}$/.test(ed.min.trim())) errs.min_points = fieldMessage(ed.min.trim() ? 'INVALID' : 'REQUIRED');
    const money = /^\d{1,12}(\.\d{1,2})?$/;
    if (ed.spend.trim() && !money.test(ed.spend.trim())) errs.spend_per_point = fieldMessage('INVALID');
    if (ed.value.trim() && !money.test(ed.value.trim())) errs.point_value = fieldMessage('INVALID');
    ed.errors = errs;
    ed.error = '';
    if (Object.values(errs).some(Boolean)) return;
    const body = { name: n.value, min_points: ed.min.trim(), spend_per_point: ed.spend.trim(), point_value: ed.value.trim() };
    ed.saving = true;
    try {
      if (ed.id) await api.update(ed.id, body);
      else await api.create(body);
      notice = t(ed.id ? 'members.levels.saved' : 'members.levels.created');
      editor = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, c] of Object.entries(err.fields)) ed.errors[k as Field] = fieldMessage(c);
      } else ed.error = errorMessage(err);
      ed.saving = false;
    }
  }

  async function toggle(l: Level) {
    busyId = l.id;
    notice = loadError = '';
    try {
      await api.setActive(l.id, !l.active);
      notice = t(l.active ? 'members.levels.archived' : 'members.levels.restored', { name: l.name });
      await load();
    } catch (err) {
      loadError = err instanceof ApiError && err.code === 'VALIDATION' ? fieldMessage(Object.values(err.fields)[0]) ?? errorMessage(err) : errorMessage(err);
    } finally {
      busyId = null;
    }
  }

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] mt-1 text-[var(--color-danger-600)]';
  const hintClass = 'text-[11px] mt-1 text-[var(--text-tertiary)]';
</script>

<svelte:head><title>{t('members.levels.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('members.levels.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('members.levels.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('members.levels.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)] max-w-2xl">{t('members.levels.subtitle')}</p>
    </div>
    {#if can('member_categories', 'create')}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('members.levels.add')}</button>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('members.levels.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="p-3 text-start" scope="col">{t('members.levels.col.name')}</th>
            <th class="p-3 text-start" scope="col">{t('members.levels.col.min')}</th>
            <th class="p-3 text-end" scope="col">{t('members.levels.col.spend')}</th>
            <th class="p-3 text-end" scope="col">{t('members.levels.col.value')}</th>
            <th class="p-3 text-end" scope="col">{t('members.levels.col.members')}</th>
            <th class="p-3 text-start" scope="col">{t('members.levels.col.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as l (l.id)}
            <tr class="border-t border-[var(--border-subtle)] odd:bg-[var(--surface-sunken)] {l.active ? '' : 'opacity-60'}">
              <td class="p-3 font-semibold">
                <span class="inline-flex items-center gap-2"><i class="icon-award text-[14px] text-[var(--color-primary-600)]"></i>{l.name}</span>
                {#if l.min_points === 0}<span class="badge-soft badge-info ms-2">{t('members.levels.base')}</span>{/if}
              </td>
              <td class="p-3 whitespace-nowrap">{l.active ? ranges.get(l.id) : formatNumber(l.min_points)}</td>
              <td class="p-3 text-end whitespace-nowrap">{Number(l.spend_per_point) > 0 ? formatCurrency(Number(l.spend_per_point)) : '—'}</td>
              <td class="p-3 text-end whitespace-nowrap">{Number(l.point_value) > 0 ? formatCurrency(Number(l.point_value)) : '—'}</td>
              <td class="p-3 text-end tabular-nums">{formatNumber(l.member_count)}</td>
              <td class="p-3"><span class="badge-soft {l.active ? 'badge-success' : 'badge-danger'}">{l.active ? t('members.active') : t('members.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <button type="button" class="header-icon-btn !size-8" aria-label={canEdit ? t('members.levels.edit') : t('members.view')} onclick={() => openEdit(l)}>
                  <i class="{canEdit ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </button>
                {#if canEdit && l.min_points !== 0}
                  <button type="button" class="header-icon-btn !size-8" aria-label={l.active ? t('members.levels.archive') : t('members.levels.restore')} title={l.active ? t('members.levels.archive') : t('members.levels.restore')} disabled={busyId === l.id} onclick={() => toggle(l)}>
                    <i class="{l.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="7" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('members.levels.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</main>

{#if editor}
  {@const ed = editor}
  {@const ro = !canEdit && ed.id !== null}
  <Modal title={ed.id ? t('members.levels.edit') : t('members.levels.add')} onclose={() => (editor = null)}>
    <form class="space-y-3.5" onsubmit={save} novalidate>
      {#if ed.error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{ed.error}</span>
        </div>
      {/if}
      <div>
        <label for="lv-name" class={labelClass}>{t('members.levels.field.name')}</label>
        <input id="lv-name" class={inputClass} bind:value={ed.name} maxlength="60" disabled={ro} aria-invalid={!!ed.errors.name} use:focusOnMount />
        {#if ed.errors.name}<p class={errClass}>{ed.errors.name}</p>{/if}
      </div>
      <div>
        <label for="lv-min" class={labelClass}>{t('members.levels.field.min')}</label>
        <input id="lv-min" class={inputClass} bind:value={ed.min} inputmode="numeric" maxlength="9" disabled={ro || ed.base} aria-invalid={!!ed.errors.min_points} />
        {#if ed.errors.min_points}<p class={errClass}>{ed.errors.min_points}</p>{:else}<p class={hintClass}>{ed.base ? t('members.levels.baseHint') : t('members.levels.field.minHint')}</p>{/if}
      </div>
      <div>
        <label for="lv-spend" class={labelClass}>{t('members.levels.field.spend')}</label>
        <MoneyInput id="lv-spend" class={inputClass} bind:value={ed.spend} placeholder="0" disabled={ro} aria-invalid={!!ed.errors.spend_per_point} />
        {#if ed.errors.spend_per_point}<p class={errClass}>{ed.errors.spend_per_point}</p>{:else}<p class={hintClass}>{t('members.levels.field.spendHint')}</p>{/if}
      </div>
      <div>
        <label for="lv-value" class={labelClass}>{t('members.levels.field.value')}</label>
        <MoneyInput id="lv-value" class={inputClass} bind:value={ed.value} placeholder="0" disabled={ro} aria-invalid={!!ed.errors.point_value} />
        {#if ed.errors.point_value}<p class={errClass}>{ed.errors.point_value}</p>{:else}<p class={hintClass}>{t('members.levels.field.valueHint')}</p>{/if}
      </div>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (editor = null)}>{ro ? t('common.close') : t('members.cancel')}</button>
        {#if !ro}
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={ed.saving}>{ed.saving ? t('members.saving') : t('catalog.save')}</button>
        {/if}
      </div>
    </form>
  </Modal>
{/if}
