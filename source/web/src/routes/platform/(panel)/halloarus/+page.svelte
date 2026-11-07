<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type AiPolicies, type AiPolicyRow, type AiPolicyValues } from '#lib/platform/api.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import AiOrb from '#lib/components/AiOrb.svelte';
  import AiPolicyForm from '#lib/platform/AiPolicyForm.svelte';
  import AiOverrideEditor from '#lib/platform/AiOverrideEditor.svelte';
  import AiEffectPanel from '#lib/platform/AiEffectPanel.svelte';
  import Avatar from '#lib/platform/Avatar.svelte';
  import AiRulesPanel from '#lib/platform/AiRulesPanel.svelte';
  import AiProviderPanel from '#lib/platform/AiProviderPanel.svelte';
  import { FEATURES, pick, emptyPolicy, posture, changedCount } from '#lib/platform/aiPolicyMeta.ts';

  type Tab = 'policy' | 'overrides' | 'rules' | 'provider';
  let tab = $state<Tab>('policy');

  let data = $state<AiPolicies | null>(null);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');

  let gForm = $state<AiPolicyValues>(emptyPolicy());
  let gNote = $state('');
  let gSnap = $state('');
  let gErrors = $state<Record<string, string>>({});
  let gSaving = $state(false);
  const dirty = $derived(data !== null && JSON.stringify([gForm, gNote]) !== gSnap);

  // Info aturan markdown untuk ringkasan di header.
  let rulesVer = $state(0);
  let rulesChars = $state(0);
  let rulesTenants = $state(0);

  async function loadRulesInfo() {
    try {
      const [g, tn] = await Promise.all([platformApi.aiRules(), platformApi.aiRuleTenants()]);
      rulesVer = g.items[0]?.version ?? 0;
      rulesChars = g.items[0]?.body.trim().length ?? 0;
      rulesTenants = tn.items.length;
    } catch {
      /* ringkasan saja: abaikan */
    }
  }

  function sync(d: AiPolicies) {
    gForm = pick(d.global);
    gNote = d.global.note;
    gSnap = JSON.stringify([gForm, gNote]);
  }

  async function load() {
    loading = true;
    error = '';
    try {
      data = await platformApi.aiPolicies();
      sync(data);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    void loadRulesInfo();
  });

  async function saveGlobal() {
    gSaving = true;
    error = notice = '';
    gErrors = {};
    try {
      await platformApi.saveAiGlobalPolicy({ ...gForm, note: gNote });
      notice = t('platform.ai.saved');
      await load();
    } catch (e) {
      const f = (e as { fields?: Record<string, string> }).fields;
      if (f && Object.keys(f).length) gErrors = f;
      else error = errorMessage(e);
    } finally {
      gSaving = false;
    }
  }

  const post = $derived(data ? posture(gForm, data.domains) : { total: 0, blocked: 0, level: 'open' as const });
  const domainsOpen = $derived(gForm.domains?.length ?? 0);
  const featuresOn = $derived(FEATURES.filter((f) => gForm[f.key] === true).length);
  const globalOn = $derived(gForm.enabled === true);

  // ---- pengecualian tenant ----
  let q = $state('');
  const shown = $derived((data?.tenants ?? []).filter((r) => !q.trim() || `${r.tenant_name} ${r.tenant_code} ${r.note}`.toLowerCase().includes(q.trim().toLowerCase())));

  let modal = $state<{ tenantId: string; tenantName: string; code: string; isNew: boolean } | null>(null);
  let oForm = $state<AiPolicyValues>(emptyPolicy());
  let oNote = $state('');
  let oErrors = $state<Record<string, string>>({});
  let oError = $state('');
  let oSaving = $state(false);
  let lookup = $state<{ state: 'idle' | 'checking' | 'found' | 'missing'; id: string; name: string }>({ state: 'idle', id: '', name: '' });

  function openNew() {
    modal = { tenantId: '', tenantName: '', code: '', isNew: true };
    oForm = emptyPolicy();
    oNote = '';
    oErrors = {};
    oError = '';
    lookup = { state: 'idle', id: '', name: '' };
  }
  function openEdit(r: AiPolicyRow) {
    modal = { tenantId: r.tenant_id!, tenantName: r.tenant_name ?? '', code: r.tenant_code ?? '', isNew: false };
    oForm = pick(r);
    oNote = r.note;
    oErrors = {};
    oError = '';
  }

  // Cari tenant berdasarkan kode (debounce), agar salah ketik terlihat sebelum disimpan.
  let seq = 0;
  $effect(() => {
    if (!modal?.isNew) return;
    const code = modal.code.trim().toLowerCase();
    if (!code) {
      lookup = { state: 'idle', id: '', name: '' };
      return;
    }
    lookup = { state: 'checking', id: '', name: '' };
    const my = ++seq;
    const timer = setTimeout(async () => {
      try {
        const f = (await platformApi.tenants(code, 10, 0)).items.find((x) => x.code === code);
        if (my === seq) lookup = f ? { state: 'found', id: f.id, name: f.name } : { state: 'missing', id: '', name: '' };
      } catch {
        if (my === seq) lookup = { state: 'idle', id: '', name: '' };
      }
    }, 350);
    return () => clearTimeout(timer);
  });

  const canSaveOverride = $derived(!!modal && (modal.isNew ? lookup.state === 'found' : true));

  async function saveOverride() {
    if (!modal || !canSaveOverride) return;
    oSaving = true;
    oError = '';
    oErrors = {};
    try {
      await platformApi.saveAiTenantPolicy(modal.isNew ? lookup.id : modal.tenantId, { ...oForm, note: oNote });
      notice = t('platform.ai.ovSaved');
      modal = null;
      await load();
    } catch (e) {
      const f = (e as { fields?: Record<string, string> }).fields;
      if (f && Object.keys(f).length) oErrors = f;
      else oError = errorMessage(e);
    } finally {
      oSaving = false;
    }
  }

  async function removeOverride(r: AiPolicyRow) {
    if (!confirm(t('platform.ai.ovRemoveConfirm', { name: r.tenant_name ?? r.tenant_code ?? '' }))) return;
    error = notice = '';
    try {
      await platformApi.deleteAiTenantPolicy(r.tenant_id!);
      notice = t('platform.ai.ovRemoved');
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  /** Beda dari global, untuk chip di daftar pengecualian. */
  function diffChips(r: AiPolicyRow) {
    const chips: { text: string; tone: 'allow' | 'block' | 'val' }[] = [];
    for (const f of FEATURES) {
      const v = r[f.key];
      if (v !== null && v !== undefined) chips.push({ text: t(f.short), tone: v ? 'allow' : 'block' });
    }
    if (r.domains) chips.push({ text: `${t('platform.ai.shDomains')} ${r.domains.length}/${data?.domains.length ?? 0}`, tone: 'val' });
    if (r.chat_limit != null) chips.push({ text: `${t('platform.ai.shChat')} ${r.chat_limit}`, tone: 'val' });
    if (r.max_range_days != null) chips.push({ text: `${t('platform.ai.shRange')} ${r.max_range_days}`, tone: 'val' });
    if (r.free_daily_limit != null) chips.push({ text: `${t('platform.ai.shFreeLimit')} ${r.free_daily_limit}`, tone: 'val' });
    if (r.free_model) chips.push({ text: `${t('platform.ai.shFreeModel')} ${r.free_model}`, tone: 'val' });
    return chips;
  }

  const tabBtn = (on: boolean) =>
    `relative flex shrink-0 items-center gap-1.5 whitespace-nowrap px-3 py-2.5 text-[12.5px] font-semibold transition-colors ${on ? 'text-[var(--color-primary-600)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-primary)]'}`;
</script>

<svelte:head><title>{t('platform.ai.title')} | ACIRABA</title></svelte:head>

<div class="space-y-3">
  <!-- Header control-plane -->
  <header class="hero">
    <div class="flex min-w-0 items-center gap-3">
      <span class="orb-wrap"><AiOrb size={52} busy={false} /></span>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="title">{t('platform.ai.title')}</h1>
          <span class="tag">{t('platform.ai.tag')}</span>
          <span class="live" data-on={globalOn}><span class="dot"></span>{globalOn ? t('platform.ai.online') : t('platform.ai.offline')}</span>
        </div>
        <p class="mt-1 truncate text-[12px] text-sky-100/75">{t('platform.ai.heroSub')}</p>
      </div>
    </div>
    <div class="posture" title={t('platform.ai.postureHint', { blocked: post.blocked, total: post.total })}>
      <span class="text-[10px] font-bold uppercase tracking-[0.14em] text-sky-200/70">{t('platform.ai.postureTitle')}</span>
      <span class="meter" data-level={post.level}><i></i><i></i><i></i></span>
      <span class="font-mono text-[12px] font-semibold">{post.level === 'open' ? t('platform.ai.postureOpen') : post.level === 'balanced' ? t('platform.ai.postureBalanced') : t('platform.ai.postureStrict')}</span>
    </div>
  </header>

  <!-- Stat ringkas -->
  <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-7">
    {#each [
      { icon: 'database', label: t('platform.ai.stDomains'), value: `${domainsOpen}/${data?.domains.length ?? 0}`, tone: domainsOpen === (data?.domains.length ?? 0) ? 'ok' : 'warn' },
      { icon: 'zap', label: t('platform.ai.stFeatures'), value: `${featuresOn}/${FEATURES.length}`, tone: 'ok' },
      { icon: 'gift', label: t('platform.ai.stFree'), value: gForm.free_enabled === false ? '—' : gForm.free_daily_limit == null ? t('platform.ai.serverVal') : `${formatNumber(gForm.free_daily_limit)}${t('platform.ai.perDay')}`, tone: 'ok' },
      { icon: 'gauge', label: t('platform.ai.stRate'), value: String(gForm.chat_limit ?? '—'), tone: 'ok' },
      { icon: 'calendar-range', label: t('platform.ai.stRange'), value: `${gForm.max_range_days ?? '—'} ${t('platform.ai.days')}`, tone: 'ok' },
      { icon: 'scroll-text', label: t('platform.ai.stRules'), value: rulesVer ? `${t('platform.ai.rulesVer', { n: rulesVer })} · ${formatNumber(rulesChars)}c${rulesTenants ? ` +${rulesTenants}` : ''}` : t('platform.ai.noRules'), tone: rulesChars ? 'ok' : 'mute' },
      { icon: 'git-compare', label: t('platform.ai.stOverrides'), value: String(data?.tenants.length ?? 0), tone: (data?.tenants.length ?? 0) > 0 ? 'warn' : 'ok' }
    ] as s (s.label)}
      <div class="stat" data-tone={s.tone} title="{s.label}: {s.value}">
        <i class="icon-{s.icon}"></i>
        <div class="min-w-0">
          <p class="stat-l">{s.label}</p>
          <p class="stat-v">{s.value}</p>
        </div>
      </div>
    {/each}
  </div>

  <!-- Tab -->
  <div class="flex overflow-x-auto border-b border-[var(--border-subtle)] scroll-thin" role="tablist">
    <button type="button" role="tab" aria-selected={tab === 'policy'} class={tabBtn(tab === 'policy')} onclick={() => (tab = 'policy')}>
      <i class="icon-shield-check text-[14px]"></i>{t('platform.ai.tabPolicy')}
      {#if tab === 'policy'}<span class="uline"></span>{/if}
    </button>
    <button type="button" role="tab" aria-selected={tab === 'overrides'} class={tabBtn(tab === 'overrides')} onclick={() => (tab = 'overrides')}>
      <i class="icon-git-compare text-[14px]"></i>{t('platform.ai.tabOverrides')}
      <span class="cnt">{data?.tenants.length ?? 0}</span>
      {#if tab === 'overrides'}<span class="uline"></span>{/if}
    </button>
    <button type="button" role="tab" aria-selected={tab === 'rules'} class={tabBtn(tab === 'rules')} onclick={() => (tab = 'rules')}>
      <i class="icon-scroll-text text-[14px]"></i>{t('platform.ai.tabRules')}
      {#if rulesVer}<span class="cnt">{t('platform.ai.rulesVer', { n: rulesVer })}</span>{/if}
      {#if tab === 'rules'}<span class="uline"></span>{/if}
    </button>
    <button type="button" role="tab" aria-selected={tab === 'provider'} class={tabBtn(tab === 'provider')} onclick={() => (tab = 'provider')}>
      <i class="icon-plug-zap text-[14px]"></i>{t('platform.ai.tabProvider')}
      {#if tab === 'provider'}<span class="uline"></span>{/if}
    </button>
  </div>

  {#if notice}<div class="rounded-lg border border-[var(--color-success-500)]/20 bg-[var(--color-success-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-success-600)]" role="status">{notice}</div>{/if}
  {#if error}<div class="rounded-lg border border-[var(--color-danger-500)]/20 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]" role="alert">{error}</div>{/if}

  {#if tab === 'provider'}
    <AiProviderPanel />
  {:else if tab === 'rules'}
    <AiRulesPanel onsaved={loadRulesInfo} />
  {:else if loading && !data}
    <div class="h-48 animate-pulse rounded-xl bg-[var(--surface-sunken)]"></div>
  {:else if data && tab === 'policy'}
    <div class="grid items-start gap-3 xl:grid-cols-[minmax(0,1fr)_17rem]">
      <section class="surface-card !p-3.5">
        <AiPolicyForm bind:value={gForm} domains={data.domains} domainTools={data.domain_tools} freeModels={data.free_models} errors={gErrors} />
        <div class="sticky bottom-0 -mx-3.5 -mb-3.5 mt-3.5 flex flex-wrap items-center gap-2 rounded-b-xl border-t border-[var(--border-subtle)] bg-[var(--surface-base)]/90 px-3.5 py-2.5 backdrop-blur">
          <input class="form-input !h-8 !w-auto min-w-40 flex-1 !text-[12.5px]" maxlength="300" placeholder={t('platform.ai.note')} aria-label={t('platform.ai.note')} bind:value={gNote} />
          <span class="text-[11.5px] {dirty ? 'font-semibold text-[var(--color-warning-600)]' : 'text-[var(--text-tertiary)]'}">{dirty ? t('platform.ai.dirty') : t('platform.ai.changeNote')}</span>
          <button type="button" class="btn btn-outline !py-1 !text-[12px]" disabled={!dirty || gSaving} onclick={() => data && sync(data)}>{t('platform.ai.reset')}</button>
          <button type="button" class="btn btn-primary !py-1 !text-[12px] disabled:opacity-60" disabled={!dirty || gSaving} onclick={saveGlobal}>{gSaving ? t('platform.ai.saving') : t('platform.ai.save')}</button>
        </div>
      </section>

      <div class="space-y-3 xl:sticky xl:top-2">
        <AiEffectPanel value={gForm} domains={data.domains} />
        {@render privacy()}
      </div>
    </div>
  {:else if data && tab === 'overrides'}
    <section class="surface-card !p-0 overflow-hidden">
      <div class="flex flex-wrap items-center gap-2 border-b border-[var(--border-subtle)] px-3.5 py-2.5">
        <div class="relative min-w-44 flex-1 sm:max-w-xs">
          <i class="icon-search absolute top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)]" style="inset-inline-start:0.8rem"></i>
          <input class="field-control w-full" style="padding-inline-start:2.4rem" type="search" placeholder={t('platform.ai.ovSearch')} aria-label={t('platform.ai.ovSearch')} bind:value={q} />
        </div>
        <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.ovCount', { count: shown.length })}</span>
        <span class="ms-auto hidden text-[11px] text-[var(--text-tertiary)] lg:inline">{t('platform.ai.ovLegend')}</span>
        <button type="button" class="btn btn-primary !py-1 !text-[12px]" onclick={openNew}><i class="icon-plus text-[13px]"></i>{t('platform.ai.ovAdd')}</button>
      </div>
      {#if data.tenants.length === 0}
        <div class="grid place-items-center gap-1 px-4 py-10 text-center">
          <span class="grid size-10 place-items-center rounded-xl bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-git-compare text-[18px]"></i></span>
          <p class="text-[13px] font-semibold">{t('platform.ai.ovEmpty')}</p>
          <p class="max-w-sm text-[12px] text-[var(--text-tertiary)]">{t('platform.ai.ovHint')}</p>
        </div>
      {:else if shown.length === 0}
        <p class="px-4 py-6 text-[12.5px] text-[var(--text-tertiary)]">{t('platform.ai.ovNone')}</p>
      {:else}
        <ul>
          {#each shown as r (r.tenant_id)}
            {@const chips = diffChips(r)}
            <li class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-t border-[var(--border-subtle)] px-3.5 py-2 first:border-t-0 hover:bg-[var(--surface-sunken)] md:grid-cols-[14rem_minmax(0,1fr)_auto]">
              <div class="min-w-0">
                <p class="truncate text-[12.5px] font-semibold">{r.tenant_name}</p>
                <p class="truncate font-mono text-[10.5px] text-[var(--text-tertiary)]">{r.tenant_code} · {formatDateTime(r.updated_at)}</p>
              </div>
              <div class="order-3 col-span-2 flex flex-wrap gap-1 md:order-none md:col-span-1">
                {#each chips as c, i (i)}<span class="dchip" data-tone={c.tone}>{c.text}</span>{/each}
                {#if chips.length === 0}<span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.unchanged')}</span>{/if}
                {#if r.note}<span class="truncate text-[11.5px] text-[var(--text-tertiary)]">· {r.note}</span>{/if}
              </div>
              <div class="flex gap-1">
                <button type="button" class="icon-btn" title={t('platform.ai.ovEdit')} aria-label={t('platform.ai.ovEdit')} onclick={() => openEdit(r)}><i class="icon-pencil-line"></i></button>
                <button type="button" class="icon-btn danger" title={t('platform.ai.ovRemove')} aria-label={t('platform.ai.ovRemove')} onclick={() => removeOverride(r)}><i class="icon-trash-2"></i></button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

{#snippet privacy()}
  <aside class="surface-card !p-3" aria-label={t('platform.ai.privTitle')}>
    <h3 class="flex items-center gap-1.5 text-[10.5px] font-bold uppercase tracking-[0.09em] text-[var(--text-tertiary)]"><i class="icon-shield-check text-[13px] text-[var(--color-success-600)]"></i>{t('platform.ai.privTitle')}</h3>
    <p class="mt-1 text-[11.5px] leading-snug text-[var(--text-tertiary)]">{t('platform.ai.privIntro')}</p>
    <ul class="mt-2 space-y-2">
      {#each [
        { icon: 'eye-off', tone: 'mask', title: t('platform.ai.privMask'), text: t('platform.ai.privMaskV') },
        { icon: 'ban', tone: 'never', title: t('platform.ai.privNever'), text: t('platform.ai.privNeverV') },
        { icon: 'lock', tone: 'gated', title: t('platform.ai.privGated'), text: t('platform.ai.privGatedV') },
        { icon: 'users', tone: 'shown', title: t('platform.ai.privShown'), text: t('platform.ai.privShownV') },
        { icon: 'image-plus', tone: 'shown', title: t('platform.ai.privPhoto'), text: t('platform.ai.privPhotoV') }
      ] as r (r.title)}
        <li class="priv" data-tone={r.tone}>
          <span class="priv-ico"><i class="icon-{r.icon}"></i></span>
          <div class="min-w-0">
            <p class="text-[11.5px] font-bold">{r.title}</p>
            <p class="text-[11.5px] leading-snug text-[var(--text-secondary)]">{r.text}</p>
          </div>
        </li>
      {/each}
    </ul>
  </aside>
{/snippet}

{#if modal && data}
  {@const gVals = pick(data.global)}
  {@const nChanged = changedCount(oForm)}
  <Modal title={modal.isNew ? t('platform.ai.ovAddTitle') : t('platform.ai.ovEditTitle')} onclose={() => (modal = null)} xl>
    <form
      novalidate
      class="space-y-3"
      onsubmit={(e) => {
        e.preventDefault();
        void saveOverride();
      }}
    >
      {#if oError}<div class="rounded-lg badge-danger px-3 py-2 text-[12.5px]" role="alert">{oError}</div>{/if}

      <!-- Kepala: identitas tenant -->
      <div class="tband">
        {#if modal.isNew}
          <span class="tband-ico"><i class="icon-store"></i></span>
          <div class="min-w-0 flex-1">
            <label for="ai-tcode" class="text-[10.5px] font-bold uppercase tracking-[0.09em] text-[var(--text-tertiary)]">{t('platform.ai.tenantCode')}</label>
            <input id="ai-tcode" class="field-control mt-1 w-full max-w-xs font-mono" autocomplete="off" spellcheck="false" placeholder="kotakcantik" bind:value={modal.code} />
          </div>
          <p class="min-h-4 max-w-xs text-[11.5px] {lookup.state === 'found' ? 'font-semibold text-[var(--color-success-600)]' : lookup.state === 'missing' ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-tertiary)]'}">
            {#if lookup.state === 'found'}<i class="icon-circle-check"></i> {t('platform.ai.tenantFound', { name: lookup.name })}
            {:else if lookup.state === 'missing'}<i class="icon-circle-x"></i> {t('platform.ai.tenantMissing')}
            {:else if lookup.state === 'checking'}{t('platform.ai.tenantChecking')}
            {:else}{t('platform.ai.tenantHelp')}{/if}
          </p>
        {:else}
          <Avatar name={modal.tenantName} size={40} square />
          <div class="min-w-0 flex-1">
            <p class="truncate text-[14px] font-semibold">{modal.tenantName}</p>
            <p class="font-mono text-[11px] text-[var(--text-tertiary)]">{modal.code}</p>
          </div>
        {/if}
        <span class="tband-chip" data-on={nChanged > 0}>{t('platform.ai.ovChanged', { count: nChanged })}</span>
      </div>

      <AiOverrideEditor bind:value={oForm} global={gVals} domains={data.domains} domainTools={data.domain_tools} freeModels={data.free_models} errors={oErrors} />

      <div class="flex flex-wrap items-center gap-2 border-t border-[var(--border-subtle)] pt-3">
        <input id="ai-onote" class="field-control min-w-48 flex-1" maxlength="300" aria-label={t('platform.ai.ovNoteLabel')} placeholder={t('platform.ai.notePlaceholder')} bind:value={oNote} />
        <button type="button" class="btn btn-outline !py-1 !text-[12px]" onclick={() => (modal = null)}>{t('platform.ai.cancel')}</button>
        <button type="submit" class="btn btn-primary !py-1 !text-[12px] disabled:opacity-60" disabled={oSaving || !canSaveOverride}>{oSaving ? t('platform.ai.saving') : t('platform.ai.save')}</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .hero {
    --ha-a: var(--color-primary-500, #3b82f6);
    --ha-b: #22d3ee;
    --ha-c: #a78bfa;
    position: relative;
    isolation: isolate;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem 1.25rem;
    overflow: hidden;
    padding: 0.7rem 1rem;
    border: 1px solid color-mix(in srgb, var(--ha-b) 22%, #0d2b63);
    border-radius: 0.9rem;
    color: #fff;
    background:
      radial-gradient(420px 120px at 0% 0%, color-mix(in srgb, var(--ha-a) 35%, transparent), transparent 70%),
      radial-gradient(360px 120px at 100% 100%, color-mix(in srgb, var(--ha-c) 26%, transparent), transparent 70%),
      linear-gradient(135deg, #06122b, #0a1f4a 60%, #0d2b63);
  }
  /* kisi + garis pindai tipis */
  .hero::before {
    content: '';
    position: absolute;
    inset: 0;
    z-index: -1;
    background-image: linear-gradient(rgb(255 255 255 / 0.045) 1px, transparent 1px), linear-gradient(90deg, rgb(255 255 255 / 0.045) 1px, transparent 1px);
    background-size: 22px 22px;
    mask-image: linear-gradient(90deg, #000, transparent 85%);
  }
  .hero::after {
    content: '';
    position: absolute;
    inset-block: 0;
    width: 90px;
    z-index: -1;
    background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--ha-b) 14%, transparent), transparent);
    animation: scan 6s linear infinite;
  }
  @keyframes scan { from { inset-inline-start: -90px; } to { inset-inline-start: 100%; } }
  /* judul selalu putih: warna heading global mengikuti tema terang dan tak terbaca di latar gelap */
  .title { font: 700 20px/1 var(--font-display, inherit); letter-spacing: -0.01em; color: #fff; text-shadow: 0 0 18px color-mix(in srgb, var(--ha-b) 45%, transparent); }
  .orb-wrap { display: grid; place-items: center; flex: none; width: 3.25rem; height: 3.25rem; margin-inline-start: -0.2rem; }
  .tag { padding: 0.1rem 0.45rem; border: 1px solid rgb(255 255 255 / 0.2); border-radius: 0.3rem; font: 600 9.5px var(--font-mono, ui-monospace, monospace); letter-spacing: 0.14em; color: #bae6fd; }
  .live { display: inline-flex; align-items: center; gap: 0.35rem; font: 700 10px var(--font-mono, ui-monospace, monospace); letter-spacing: 0.1em; color: #6ee7b7; }
  .live[data-on='false'] { color: #fca5a5; }
  .dot { position: relative; width: 0.45rem; height: 0.45rem; border-radius: 9999px; background: currentColor; box-shadow: 0 0 8px currentColor; }
  .posture { display: flex; align-items: center; gap: 0.6rem; padding: 0.35rem 0.7rem; border: 1px solid rgb(255 255 255 / 0.14); border-radius: 0.6rem; background: rgb(255 255 255 / 0.06); }
  .meter { display: inline-flex; gap: 3px; }
  .meter i { width: 1.1rem; height: 0.4rem; border-radius: 2px; background: rgb(255 255 255 / 0.18); transition: background-color 0.2s; }
  .meter[data-level='open'] i:nth-child(1) { background: #34d399; }
  .meter[data-level='balanced'] i:nth-child(-n + 2) { background: #fbbf24; }
  .meter[data-level='strict'] i { background: #f87171; }

  .stat { display: flex; align-items: center; gap: 0.55rem; padding: 0.45rem 0.65rem; border: 1px solid var(--border-subtle); border-radius: 0.65rem; background: var(--surface-base); }
  .stat > i { display: grid; place-items: center; flex: none; width: 1.7rem; height: 1.7rem; border-radius: 0.45rem; background: color-mix(in srgb, var(--color-primary-500) 12%, transparent); color: var(--color-primary-600); font-size: 14px; }
  .stat[data-tone='warn'] > i { background: color-mix(in srgb, var(--color-warning-500) 16%, transparent); color: var(--color-warning-600); }
  .stat[data-tone='mute'] > i { background: var(--surface-sunken); color: var(--text-tertiary); }
  .stat-l { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; font-weight: 700; letter-spacing: 0.07em; text-transform: uppercase; color: var(--text-tertiary); }
  .stat-v { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 14px/1.2 var(--font-mono, ui-monospace, monospace); font-variant-numeric: tabular-nums; }

  .uline { position: absolute; inset-inline: 0.5rem; bottom: -1px; height: 2px; border-radius: 2px; background: var(--color-primary-600); }
  .cnt { padding: 0 0.4rem; border-radius: 9999px; background: var(--surface-sunken); font: 600 10.5px var(--font-mono, ui-monospace, monospace); color: var(--text-secondary); }

  .dchip { padding: 0.05rem 0.45rem; border-radius: 0.3rem; font: 600 10.5px var(--font-mono, ui-monospace, monospace); background: var(--surface-sunken); color: var(--text-secondary); }
  .dchip[data-tone='allow'] { background: color-mix(in srgb, var(--color-success-500) 15%, transparent); color: var(--color-success-600); }
  .dchip[data-tone='block'] { background: color-mix(in srgb, var(--color-danger-500) 14%, transparent); color: var(--color-danger-600); }

  .priv { display: flex; gap: 0.55rem; }
  .priv-ico { display: grid; place-items: center; flex: none; width: 1.6rem; height: 1.6rem; border-radius: 0.45rem; font-size: 13px; background: var(--surface-sunken); color: var(--text-tertiary); }
  .priv[data-tone='mask'] .priv-ico { background: color-mix(in srgb, var(--color-info-500) 15%, transparent); color: var(--color-info-600); }
  .priv[data-tone='never'] .priv-ico { background: color-mix(in srgb, var(--color-danger-500) 14%, transparent); color: var(--color-danger-600); }
  .priv[data-tone='gated'] .priv-ico { background: color-mix(in srgb, var(--color-warning-500) 16%, transparent); color: var(--color-warning-600); }
  .priv[data-tone='shown'] .priv-ico { background: color-mix(in srgb, var(--color-success-500) 14%, transparent); color: var(--color-success-600); }

  .tband { display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; padding: 0.65rem 0.85rem; border: 1px solid var(--border-subtle); border-radius: 0.8rem; background: linear-gradient(100deg, color-mix(in srgb, var(--color-primary-500) 9%, var(--surface-base)), var(--surface-base)); }
  .tband-ico { display: grid; place-items: center; flex: none; width: 2.5rem; height: 2.5rem; border-radius: 0.7rem; background: color-mix(in srgb, var(--color-primary-500) 14%, transparent); color: var(--color-primary-600); font-size: 18px; }
  .tband-chip { padding: 0.15rem 0.6rem; border-radius: 9999px; background: var(--surface-sunken); font: 700 10.5px var(--font-mono, ui-monospace, monospace); color: var(--text-tertiary); }
  .tband-chip[data-on='true'] { background: color-mix(in srgb, var(--color-primary-500) 14%, transparent); color: var(--color-primary-600); }
  .icon-btn { display: grid; place-items: center; width: 1.8rem; height: 1.8rem; border-radius: 0.45rem; color: var(--text-tertiary); font-size: 14px; transition: all 0.12s; }
  .icon-btn:hover { background: var(--surface-sunken); color: var(--text-primary); }
  .icon-btn.danger:hover { background: color-mix(in srgb, var(--color-danger-500) 14%, transparent); color: var(--color-danger-600); }

  @media (prefers-reduced-motion: reduce) { .hero::after { animation: none; display: none; } .meter i, .icon-btn { transition: none; } }
</style>
