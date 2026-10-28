<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type PlanRow } from '#lib/platform/api.ts';
  import { t, tryT, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Switch from '#lib/components/Switch.svelte';

  // Harga paket selalu Rupiah (tagihan nyata), tidak mengikuti simbol mata uang bahasa.
  const rp = (n: number) => 'Rp ' + formatNumber(n, { maximumFractionDigits: 0 });

  type Form = {
    id: string | null;
    code: string;
    name: string;
    description: string;
    price: string;
    duration: string;
    maxTx: string;
    maxUsers: string;
    selfService: boolean;
    active: boolean;
    sort: string;
    isDefault: boolean;
    /** null = semua menu */
    modules: string[] | null;
    /** satu poin per baris */
    highlights: string;
  };

  let plans = $state<PlanRow[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let form = $state<Form | null>(null);
  let saving = $state(false);
  let fieldErrors = $state<Record<string, string>>({});
  let moduleIds = $state<string[]>([]);
  let moduleQuery = $state('');
  const moduleLabel = (id: string) => tryT(`iam.module.${id}`) ?? id;
  const shownModules = $derived(moduleIds.filter((id) => !moduleQuery.trim() || moduleLabel(id).toLowerCase().includes(moduleQuery.trim().toLowerCase()) || id.includes(moduleQuery.trim().toLowerCase())));

  async function load() {
    loading = true;
    error = '';
    try {
      plans = (await platformApi.plans()).items;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    platformApi.planModules().then((m) => (moduleIds = m.map((x) => x.id).filter((id) => id !== 'subscription'))).catch(() => {});
  });

  function toggleModule(id: string) {
    if (!form || form.modules === null) return;
    form.modules = form.modules.includes(id) ? form.modules.filter((m) => m !== id) : [...form.modules, id];
  }

  const num = (s: string) => (s.trim() === '' ? null : Number(s));
  const limitLabel = (n: number | null, key: 'perDay' | 'days' | null) =>
    n === null ? t('platform.plans.unlimited') : key ? t(`platform.plans.${key}`, { count: formatNumber(n) }) : formatNumber(n);

  function openNew() {
    notice = '';
    fieldErrors = {};
    form = { id: null, code: '', name: '', description: '', price: '0', duration: '', maxTx: '', maxUsers: '', selfService: false, active: true, sort: String(plans.length + 1), isDefault: false, modules: null, highlights: '' };
  }

  function openEdit(p: PlanRow) {
    notice = '';
    fieldErrors = {};
    form = {
      id: p.id, code: p.code, name: p.name, description: p.description, price: p.price,
      duration: p.duration_days === null ? '' : String(p.duration_days),
      maxTx: p.max_tx_per_day === null ? '' : String(p.max_tx_per_day),
      maxUsers: p.max_users === null ? '' : String(p.max_users),
      selfService: p.self_service, active: p.active, sort: String(p.sort_order), isDefault: p.is_default,
      modules: p.modules === null ? null : [...p.modules],
      highlights: p.highlights.join('\n')
    };
  }

  async function save() {
    if (!form || saving) return;
    saving = true;
    error = '';
    fieldErrors = {};
    const body = {
      name: form.name.trim(),
      description: form.description.trim(),
      price: form.price || '0',
      duration_days: num(form.duration),
      max_tx_per_day: num(form.maxTx),
      max_users: num(form.maxUsers),
      self_service: form.selfService,
      active: form.active,
      sort_order: Number(form.sort) || 0,
      modules: form.modules,
      highlights: form.highlights.split('\n').map((l) => l.trim()).filter(Boolean)
    };
    try {
      if (form.id) await platformApi.updatePlan(form.id, body);
      else await platformApi.createPlan({ ...body, code: form.code.trim().toLowerCase() });
      notice = t(form.id ? 'platform.plans.saved' : 'platform.plans.created');
      form = null;
      await load();
    } catch (err) {
      const e = err as { fields?: Record<string, string> };
      if (e.fields && Object.keys(e.fields).length) fieldErrors = e.fields;
      else error = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  const fe = (k: string) => (fieldErrors[k] ? fieldMessage(fieldErrors[k]) : '');
</script>

<svelte:head><title>{t('platform.plans.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex items-center justify-between gap-4">
    <div>
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.plans.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)]">{t('platform.plans.subtitle')}</p>
    </div>
    <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('platform.plans.add')}</button>
  </div>

  {#if notice}<div class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]">{notice}</div>{/if}
  {#if error}<div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]">{error}</div>{/if}

  {#if loading}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.plans.loading')}</div>
  {:else if plans.length === 0}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.plans.empty')}</div>
  {:else}
    <div class="surface-card !p-0 overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.plans.name')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.plans.price')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.plans.duration')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.plans.maxTx')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.plans.maxUsers')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.plans.status')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.plans.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each plans as p (p.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-3">
                <span class="block font-semibold">{p.name}</span>
                <span class="font-mono text-[11px] text-[var(--text-tertiary)]">{p.code}</span>
              </td>
              <td class="px-4 py-3 text-end tabular-nums">{rp(Number(p.price))}</td>
              <td class="px-4 py-3 text-end tabular-nums">{p.duration_days === null ? '—' : t('platform.plans.days', { count: formatNumber(p.duration_days) })}</td>
              <td class="px-4 py-3 text-end tabular-nums">{limitLabel(p.max_tx_per_day, 'perDay')}</td>
              <td class="px-4 py-3 text-end tabular-nums">{limitLabel(p.max_users, null)}</td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1">
                  {#if p.is_default}<span class="badge-info">{t('platform.plans.isDefault')}</span>{/if}
                  {#if p.self_service}<span class="badge-success">{t('platform.plans.sold')}</span>{:else if !p.is_default}<span class="badge-warning">{t('platform.plans.manual')}</span>{/if}
                  {#if !p.active}<span class="badge-danger">{t('platform.plans.inactive')}</span>{/if}
                </div>
              </td>
              <td class="px-4 py-3 text-end"><button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => openEdit(p)}>{t('platform.plans.edit')}</button></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

{#if form}
  {@const priceN = Number(form.price || 0)}
  {@const daysN = num(form.duration)}
  {@const txN = num(form.maxTx)}
  {@const usersN = num(form.maxUsers)}
  {@const checks = [
    { ok: priceN > 0, text: t('platform.plans.m.chkPrice') },
    { ok: daysN !== null && daysN > 0, text: t('platform.plans.m.chkDuration') },
    { ok: form.active, text: t('platform.plans.m.chkActive') }
  ]}
  {@const sellable = checks.every((c) => c.ok)}
  {@const hlLines = form.highlights.split('\n').map((l) => l.trim()).filter(Boolean)}
  <Modal title={form.id ? t('platform.plans.editTitle') : t('platform.plans.addTitle')} onclose={() => (form = null)} xl>
    <div class="mhead">
      <span class="mhead-ico"><i class="icon-{form.id ? 'pencil' : 'badge-dollar-sign'}"></i></span>
      <p>{form.id ? t('platform.plans.m.editSub') : t('platform.plans.m.createSub')}</p>
    </div>
    <form
      novalidate
      onsubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      {#if error}<div class="mnote mnote-danger mb-4" role="alert"><i class="icon-circle-alert"></i>{error}</div>{/if}
      <div class="grid gap-6 lg:grid-cols-[1fr_310px]">
        <div class="space-y-5 min-w-0">
          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">1</span>{t('platform.plans.m.secIdentity')}</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="plan-name" class="mlabel">{t('platform.plans.name')}<b class="req">*</b></label>
                <input id="plan-name" class="form-input !text-[13px]" bind:value={form.name} maxlength="60" placeholder="PRO" />
                {#if fe('name')}<p class="mhint !text-[var(--color-danger-600)]">{fe('name')}</p>{/if}
              </div>
              <div>
                <label for="plan-code" class="mlabel">{t('platform.plans.code')}{#if !form.id}<b class="req">*</b>{/if}</label>
                {#if form.id}
                  <div class="locked"><i class="icon-lock"></i><span class="font-mono">{form.code}</span></div>
                  <p class="mhint">{t('platform.plans.m.codeLocked')}</p>
                {:else}
                  <input id="plan-code" class="form-input !text-[13px] font-mono" bind:value={form.code} maxlength="32" autocomplete="off" placeholder="enterprise" />
                  <p class="mhint {fe('code') ? '!text-[var(--color-danger-600)]' : ''}">{fe('code') || t('platform.plans.codeHelp')}</p>
                {/if}
              </div>
              <div class="sm:col-span-2">
                <label for="plan-desc" class="mlabel flex justify-between">{t('platform.plans.description')}<span class="font-normal text-[11px] text-[var(--text-tertiary)] tabular-nums">{form.description.length}/300</span></label>
                <input id="plan-desc" class="form-input !text-[13px]" bind:value={form.description} maxlength="300" placeholder={t('platform.plans.m.descPlaceholder')} />
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">2</span>{t('platform.plans.m.secPricing')}</h3>
            <div class="grid gap-5 sm:grid-cols-2">
              <div>
                <label for="plan-price" class="mlabel">{t('platform.plans.price')}</label>
                <div class="affix"><span>Rp</span><MoneyInput id="plan-price" class="form-input !text-[13px] text-end" bind:value={form.price} decimals={0} /></div>
                <p class="mhint {fe('price') ? '!text-[var(--color-danger-600)]' : ''}">{fe('price') || (priceN === 0 ? t('platform.plans.m.priceFree') : daysN ? t('platform.plans.m.perDay', { price: rp(Math.round(priceN / daysN)) }) : t('platform.plans.m.priceHint'))}</p>
              </div>
              <div>
                <label for="plan-days" class="mlabel">{t('platform.plans.duration')}</label>
                <div class="affix"><input id="plan-days" class="form-input !text-[13px] text-end !pe-14" inputmode="numeric" bind:value={form.duration} placeholder="∞" /><span class="after">{t('platform.plans.m.daysUnit')}</span></div>
                <div class="mt-2 flex flex-wrap gap-1.5">
                  {#each [30, 90, 365] as d (d)}
                    <button type="button" class="mpreset {form.duration === String(d) ? 'on' : ''}" onclick={() => (form!.duration = String(d))}>{t('platform.plans.days', { count: d })}</button>
                  {/each}
                  <button type="button" class="mpreset {form.duration === '' ? 'on' : ''}" onclick={() => (form!.duration = '')}>{t('platform.plans.m.noExpiry')}</button>
                </div>
                <p class="mhint {fe('duration_days') ? '!text-[var(--color-danger-600)]' : ''}">{fe('duration_days') || t('platform.plans.durationHelp')}</p>
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">3</span>{t('platform.plans.m.secLimits')}</h3>
            <div class="grid gap-5 sm:grid-cols-2">
              {#each [{ id: 'plan-tx', key: 'maxTx', label: t('platform.plans.maxTx'), icon: 'receipt', presets: [100, 300, 1000], err: fe('max_tx_per_day') }, { id: 'plan-users', key: 'maxUsers', label: t('platform.plans.maxUsers'), icon: 'users', presets: [2, 4, 10], err: fe('max_users') }] as f (f.id)}
                {@const unlimited = form[f.key as 'maxTx' | 'maxUsers'] === ''}
                <div>
                  <label for={f.id} class="mlabel"><i class="icon-{f.icon} me-1.5 text-[var(--text-tertiary)]"></i>{f.label}</label>
                  <div class="affix">
                    <input id={f.id} class="form-input !text-[13px] text-end" inputmode="numeric" disabled={unlimited} value={form[f.key as 'maxTx' | 'maxUsers']} oninput={(e) => (form![f.key as 'maxTx' | 'maxUsers'] = e.currentTarget.value)} placeholder="∞" />
                  </div>
                  <div class="mt-2 flex flex-wrap gap-1.5">
                    {#each f.presets as n (n)}
                      <button type="button" class="mpreset {form[f.key as 'maxTx' | 'maxUsers'] === String(n) ? 'on' : ''}" onclick={() => (form![f.key as 'maxTx' | 'maxUsers'] = String(n))}>{formatNumber(n)}</button>
                    {/each}
                    <button type="button" class="mpreset {unlimited ? 'on' : ''}" onclick={() => (form![f.key as 'maxTx' | 'maxUsers'] = unlimited ? String(f.presets[0]) : '')}><i class="icon-infinity text-[12px] me-1"></i>{t('platform.plans.unlimited')}</button>
                  </div>
                  <p class="mhint {f.err ? '!text-[var(--color-danger-600)]' : ''}">{f.err || t('platform.plans.limitHelp')}</p>
                </div>
              {/each}
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">4</span>{t('platform.plans.m.secModules')}</h3>
            <div class="flex items-start gap-3">
              <Switch bind:checked={() => form!.modules !== null, (v) => (form!.modules = v ? [...moduleIds] : null)} label={t('platform.plans.m.limitModules')} />
              <div>
                <div class="text-[13px] font-semibold">{t('platform.plans.m.limitModules')}</div>
                <p class="mhint !mt-0.5 {fe('modules') ? '!text-[var(--color-danger-600)]' : ''}">{fe('modules') || (form.modules === null ? t('platform.plans.m.modulesAllHint') : t('platform.plans.m.modulesHelp'))}</p>
              </div>
            </div>
            {#if form.modules !== null}
              <div class="mt-3 flex flex-wrap items-center gap-2">
                <input class="form-input !text-[12.5px] !w-52" bind:value={moduleQuery} placeholder={t('platform.plans.m.modulesSearch')} />
                <button type="button" class="mpreset" onclick={() => (form!.modules = [...moduleIds])}>{t('platform.plans.m.pickAll')}</button>
                <button type="button" class="mpreset" onclick={() => (form!.modules = [])}>{t('platform.plans.m.pickNone')}</button>
                <span class="text-[11.5px] text-[var(--text-tertiary)] tabular-nums ms-auto">{t('platform.plans.m.modulesCount', { count: form.modules.length, total: moduleIds.length })}</span>
              </div>
              <div class="mt-2 grid gap-x-4 gap-y-1 sm:grid-cols-2 max-h-64 overflow-y-auto scroll-thin rounded-lg border border-[var(--border-subtle)] p-2">
                {#each shownModules as id (id)}
                  <label class="flex items-center gap-2 rounded px-1.5 py-1 text-[12.5px] hover:bg-[var(--surface-sunken)] cursor-pointer">
                    <input type="checkbox" checked={form.modules.includes(id)} onchange={() => toggleModule(id)} />
                    <span class="truncate">{moduleLabel(id)}</span>
                  </label>
                {/each}
              </div>
              {#if form.modules.length === 0}<div class="mnote mnote-info"><i class="icon-info"></i>{t('platform.plans.m.modulesNone')}</div>{/if}
            {/if}
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">5</span>{t('platform.plans.m.secHighlights')}</h3>
            <label for="plan-hl" class="mlabel flex justify-between">{t('platform.plans.m.highlights')}<span class="font-normal text-[11px] text-[var(--text-tertiary)] tabular-nums">{hlLines.length}/12</span></label>
            <textarea id="plan-hl" class="form-input !text-[13px] w-full" rows="4" bind:value={form.highlights} placeholder={t('platform.plans.m.highlightsPlaceholder')}></textarea>
            <p class="mhint {fe('highlights') || hlLines.length > 12 || hlLines.some((l) => l.length > 120) ? '!text-[var(--color-danger-600)]' : ''}">{fe('highlights') || t('platform.plans.m.highlightsHelp')}</p>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">6</span>{t('platform.plans.m.secSales')}</h3>
            {#if form.isDefault}
              <div class="mnote mnote-info !mt-0"><i class="icon-info"></i>{t('platform.plans.m.defaultNote')}</div>
            {:else}
              <div class="space-y-4">
                <div class="flex items-start gap-3">
                  <Switch bind:checked={form.selfService} label={t('platform.plans.selfService')} />
                  <div>
                    <div class="text-[13px] font-semibold">{t('platform.plans.selfService')}</div>
                    <p class="mhint !mt-0.5 {fe('self_service') ? '!text-[var(--color-danger-600)]' : ''}">{fe('self_service') || t('platform.plans.selfServiceHelp')}</p>
                  </div>
                </div>
                <div class="flex items-start gap-3">
                  <Switch bind:checked={form.active} label={t('platform.plans.active')} />
                  <div>
                    <div class="text-[13px] font-semibold">{t('platform.plans.active')}</div>
                    <p class="mhint !mt-0.5">{t('platform.plans.m.activeHint')}</p>
                  </div>
                </div>
              </div>
            {/if}
            <div class="mt-4 flex items-center gap-3">
              <label for="plan-sort" class="mlabel !mb-0">{t('platform.plans.sort')}</label>
              <input id="plan-sort" class="form-input !text-[13px] !w-24 text-end" inputmode="numeric" bind:value={form.sort} />
            </div>
          </section>
        </div>

        <!-- Pratinjau -->
        <aside class="space-y-4 lg:sticky lg:top-0 self-start">
          <div class="mlabel !mb-0">{t('platform.plans.m.previewTitle')}</div>
          <div class="pv">
            <div class="pv-top"></div>
            <div class="p-5 space-y-3.5">
              <div>
                <div class="font-display font-bold text-[17px]">{form.name || t('platform.plans.m.namePlaceholder')}</div>
                {#if form.description}<p class="text-[11.5px] text-[var(--text-tertiary)] mt-0.5">{form.description}</p>{/if}
              </div>
              <div class="flex items-baseline gap-1.5">
                <span class="font-display font-bold text-[26px] tabular-nums">{rp(priceN)}</span>
                {#if daysN}<span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.plans.m.perDuration', { count: formatNumber(daysN) })}</span>{/if}
              </div>
              <ul class="space-y-2 text-[12.5px]">
                <li class="flex items-center gap-2"><span class="pv-ok"><i class="icon-check"></i></span>{txN === null ? t('platform.plans.m.unlimitedTx') : t('platform.plans.m.txPerDay', { count: formatNumber(txN) })}</li>
                <li class="flex items-center gap-2"><span class="pv-ok"><i class="icon-check"></i></span>{usersN === null ? t('platform.plans.m.unlimitedUsers') : t('platform.plans.m.usersIncl', { count: formatNumber(usersN) })}</li>
                {#each hlLines.slice(0, 12) as h, i (i)}<li class="flex items-center gap-2"><span class="pv-ok"><i class="icon-sparkles"></i></span>{h}</li>{/each}
                <li class="flex items-center gap-2"><span class="pv-ok"><i class="icon-check"></i></span>{form.modules === null ? t('platform.plans.m.allModules') : t('platform.plans.m.modulesCount', { count: form.modules.length, total: moduleIds.length })}</li>
              </ul>
              <div class="pv-btn"><i class="icon-qr-code"></i>{t('platform.plans.m.payBtn')}</div>
            </div>
          </div>

          {#if !form.isDefault}
            <div class="rounded-xl border border-[var(--border-subtle)] p-4 space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-[12px] font-semibold">{t('platform.plans.m.readiness')}</span>
                <span class="rounded-full px-2 py-0.5 text-[10.5px] font-bold uppercase {form.selfService ? (sellable ? 'bg-[var(--color-success-500)]/15 text-[var(--color-success-600)]' : 'bg-[var(--color-warning-500)]/15 text-[var(--color-warning-600)]') : 'bg-[var(--surface-sunken)] text-[var(--text-tertiary)]'}">
                  {form.selfService ? (sellable ? t('platform.plans.m.readyOk') : t('platform.plans.m.readyNo')) : t('platform.plans.m.readyManual')}
                </span>
              </div>
              {#if form.selfService}
                <ul class="space-y-1.5 text-[12px]">
                  {#each checks as c (c.text)}
                    <li class="flex items-center gap-2 {c.ok ? 'text-[var(--text-secondary)]' : 'text-[var(--color-warning-600)] font-medium'}"><i class="icon-{c.ok ? 'circle-check' : 'circle-alert'} text-[14px]"></i>{c.text}</li>
                  {/each}
                </ul>
              {:else}
                <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.plans.m.manualNote')}</p>
              {/if}
            </div>
          {/if}
        </aside>
      </div>

      <div class="mfoot mt-5">
        <span class="mhint !mt-0"><b class="req">*</b> {t('platform.plans.m.required')}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (form = null)}>{t('platform.plans.cancel')}</button>
          <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={saving || !form.name.trim() || (!form.id && !form.code.trim())}>{saving ? t('platform.plans.saving') : form.id ? t('platform.plans.save') : t('platform.plans.m.create')}</button>
        </div>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .mhead { display: flex; align-items: center; gap: 0.85rem; margin-bottom: 1.1rem; padding: 0.85rem 1rem; border-radius: 0.85rem; background: var(--surface-sunken); }
  .mhead p { margin: 0; font-size: 12.5px; color: var(--text-secondary); }
  .mhead-ico { display: grid; place-items: center; flex: none; width: 2.5rem; height: 2.5rem; border-radius: 0.7rem; background: var(--color-primary-50); color: var(--color-primary-600); font-size: 18px; }
  .msec { padding: 1rem 1.1rem 1.1rem; border: 1px solid var(--border-subtle); border-radius: 0.85rem; }
  .msec-title { display: flex; align-items: center; gap: 0.6rem; margin-bottom: 0.85rem; font-size: 11.5px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-secondary); }
  .msec-num { display: grid; place-items: center; width: 1.3rem; height: 1.3rem; border-radius: 9999px; background: var(--color-primary-600); color: #fff; font-size: 10.5px; letter-spacing: 0; }
  .msec :global(input.form-input), .msec :global(textarea.form-input) { width: 100%; }
  .mlabel { display: block; margin-bottom: 0.4rem; font-size: 12.5px; font-weight: 600; }
  .req { margin-inline-start: 2px; color: var(--color-danger-500); }
  .mhint { margin-top: 0.4rem; font-size: 11.5px; color: var(--text-tertiary); }
  .mnote { display: flex; align-items: flex-start; gap: 0.6rem; margin-top: 0.6rem; padding: 0.65rem 0.8rem; border-radius: 0.65rem; font-size: 12.5px; }
  .mnote i { margin-top: 2px; }
  .mnote-info { background: color-mix(in srgb, var(--color-info-500) 10%, transparent); color: var(--color-info-600); }
  .mnote-danger { background: color-mix(in srgb, var(--color-danger-500) 10%, transparent); color: var(--color-danger-600); }
  .mfoot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0.75rem; padding-top: 0.9rem; border-top: 1px solid var(--border-subtle); }
  .mpreset { display: inline-flex; align-items: center; padding: 0.2rem 0.65rem; border-radius: 9999px; border: 1px solid var(--border-subtle); font-size: 11.5px; font-weight: 600; color: var(--text-secondary); transition: all 0.15s; }
  .mpreset:hover { border-color: var(--color-primary-400); color: var(--color-primary-600); }
  .mpreset.on { background: var(--color-primary-600); border-color: var(--color-primary-600); color: #fff; }
  .locked { display: flex; align-items: center; gap: 0.5rem; padding: 0.45rem 0.7rem; border: 1px dashed var(--border-subtle); border-radius: 0.5rem; background: var(--surface-sunken); font-size: 13px; color: var(--text-secondary); }
  .affix { position: relative; display: flex; align-items: center; }
  .affix > span { position: absolute; inset-inline-start: 0.75rem; z-index: 1; font-size: 12.5px; font-weight: 600; color: var(--text-tertiary); pointer-events: none; }
  .affix > span.after { inset-inline-start: auto; inset-inline-end: 0.75rem; }
  .affix > span + :global(input), .affix > span + :global(*) { padding-inline-start: 2.1rem !important; }
  .affix > :global(input:has(+ .after)) { padding-inline-end: 3.4rem !important; }
  .pv { overflow: hidden; border: 1px solid var(--border-subtle); border-radius: 0.9rem; background: var(--surface-base); box-shadow: 0 8px 22px -16px rgb(15 23 42 / 0.35); }
  .pv-top { height: 0.4rem; background: linear-gradient(90deg, var(--color-primary-500), var(--color-primary-800)); }
  .pv-ok { display: grid; place-items: center; width: 1.3rem; height: 1.3rem; border-radius: 9999px; background: color-mix(in srgb, var(--color-success-500) 14%, transparent); color: var(--color-success-600); font-size: 11px; }
  .pv-btn { display: flex; align-items: center; justify-content: center; gap: 0.5rem; padding: 0.55rem; border-radius: 0.6rem; background: var(--color-primary-600); color: #fff; font-size: 12.5px; font-weight: 700; }
  @media (prefers-reduced-motion: reduce) { .mpreset { transition: none; } }
</style>
