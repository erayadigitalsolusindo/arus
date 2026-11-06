<script lang="ts">
  // Aturan operator HalloArus (markdown): dokumen global + dokumen per tenant, berversi, plus "uji tanya" untuk draf.
  // Hanya Platform Admin.
  import { onMount } from 'svelte';
  import { platformApi, type AiRuleTenant, type AiRuleTest, type AiRuleVersion } from '#lib/platform/api.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import MarkdownEditor from '#lib/components/MarkdownEditor.svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';

  let { onsaved }: { onsaved?: () => void } = $props();

  type Scope = { tenantId: string; name: string };
  const GLOBAL: Scope = { tenantId: '', name: '' };

  let maxChars = $state(6000);
  let tenants = $state<AiRuleTenant[]>([]);
  let scope = $state<Scope>(GLOBAL);
  let versions = $state<AiRuleVersion[]>([]);
  let body = $state('');
  let baseline = $state('');
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let notice = $state('');
  let fieldErr = $state('');

  const dirty = $derived(body.trim() !== baseline.trim());
  const pct = $derived(Math.min(100, Math.round((body.length / maxChars) * 100)));
  const current = $derived(versions[0]);

  async function loadTenants() {
    const r = await platformApi.aiRuleTenants();
    tenants = r.items;
    maxChars = r.max_chars;
  }

  async function open(next: Scope) {
    if (dirty && !confirm(t('platform.ai.rules.unsaved'))) return;
    scope = next;
    loading = true;
    error = notice = fieldErr = '';
    testResult = null;
    testError = '';
    try {
      versions = (await platformApi.aiRules(next.tenantId)).items;
      baseline = body = versions[0]?.body ?? '';
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    try {
      await loadTenants();
    } catch (e) {
      error = errorMessage(e);
    }
    await open(GLOBAL);
  });

  async function save() {
    if (saving) return;
    saving = true;
    error = notice = fieldErr = '';
    try {
      const r = await platformApi.saveAiRules(scope.tenantId, body);
      notice = t('platform.ai.rules.saved', { n: r.version });
      versions = (await platformApi.aiRules(scope.tenantId)).items;
      baseline = body = versions[0]?.body ?? '';
      await loadTenants();
      onsaved?.();
    } catch (e) {
      const f = (e as { fields?: Record<string, string> }).fields;
      if (f?.body) fieldErr = fieldMessage(f.body) ?? '';
      else error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  function insertTemplate() {
    if (body.trim() && !confirm(t('platform.ai.rules.templateConfirm'))) return;
    body = t('platform.ai.rules.tpl');
  }

  function loadVersion(v: AiRuleVersion) {
    body = v.body;
    notice = t('platform.ai.rules.loaded', { n: v.version });
  }

  // ---- uji tanya (draf di editor, tanpa data tenant) ----
  const samples = $derived([t('platform.ai.rules.test.s1'), t('platform.ai.rules.test.s2'), t('platform.ai.rules.test.s3'), t('platform.ai.rules.test.s4'), t('platform.ai.rules.test.s5')]);
  let question = $state('');
  let testing = $state(false);
  let testError = $state('');
  let testResult = $state<AiRuleTest | null>(null);

  async function runTest() {
    if (testing || !question.trim()) return;
    testing = true;
    testError = '';
    testResult = null;
    try {
      testResult = await platformApi.testAiRules({ tenant_id: scope.tenantId, draft: body, message: question.trim() });
    } catch (e) {
      testError = errorMessage(e);
    } finally {
      testing = false;
    }
  }

  // ---- tambah dokumen tenant (cari kode, debounce) ----
  let code = $state('');
  let lookup = $state<{ state: 'idle' | 'checking' | 'found' | 'missing'; id: string; name: string }>({ state: 'idle', id: '', name: '' });
  let seq = 0;
  $effect(() => {
    const c = code.trim().toLowerCase();
    if (!c) {
      lookup = { state: 'idle', id: '', name: '' };
      return;
    }
    lookup = { state: 'checking', id: '', name: '' };
    const my = ++seq;
    const timer = setTimeout(async () => {
      try {
        const f = (await platformApi.tenants(c, 10, 0)).items.find((x) => x.code === c);
        if (my === seq) lookup = f ? { state: 'found', id: f.id, name: f.name } : { state: 'missing', id: '', name: '' };
      } catch {
        if (my === seq) lookup = { state: 'idle', id: '', name: '' };
      }
    }, 350);
    return () => clearTimeout(timer);
  });

  async function openFound() {
    if (lookup.state !== 'found') return;
    await open({ tenantId: lookup.id, name: lookup.name });
    code = '';
  }

  const flow = $derived([
    { icon: 'lock', label: t('platform.ai.flowCore'), kind: t('platform.ai.flowHard'), hard: true, active: true },
    { icon: 'shield-check', label: t('platform.ai.flowPolicy'), kind: t('platform.ai.flowHard'), hard: true, active: true },
    { icon: 'globe', label: t('platform.ai.flowGlobal'), kind: t('platform.ai.flowSoft'), hard: false, active: scope.tenantId === '' },
    { icon: 'store', label: t('platform.ai.flowTenant'), kind: t('platform.ai.flowSoft'), hard: false, active: scope.tenantId !== '' }
  ]);
</script>

<div class="space-y-3">
  <!-- Urutan prioritas + penjelasan -->
  <div class="flowbar" aria-label={t('platform.ai.flowTitle')}>
    <span class="flow-t">{t('platform.ai.flowTitle')}</span>
    {#each flow as f, i (f.label)}
      <span class="step" data-hard={f.hard} data-active={f.active}><i class="icon-{f.icon}"></i>{f.label}<em>{f.kind}</em></span>
      {#if i < flow.length - 1}<i class="icon-chevron-right arrow"></i>{/if}
    {/each}
  </div>
  <p class="text-[12px] leading-relaxed text-[var(--text-secondary)]">{t('platform.ai.rules.intro')}</p>

  {#if notice}<div class="rounded-lg border border-[var(--color-success-500)]/20 bg-[var(--color-success-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-success-600)]" role="status">{notice}</div>{/if}
  {#if error}<div class="rounded-lg border border-[var(--color-danger-500)]/20 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]" role="alert">{error}</div>{/if}

  <div class="grid items-start gap-3 lg:grid-cols-[15rem_minmax(0,1fr)]">
    <!-- Daftar dokumen -->
    <aside class="surface-card side !p-2.5 lg:sticky lg:top-2">
      <button type="button" class="item" class:on={scope.tenantId === ''} onclick={() => open(GLOBAL)}>
        <i class="icon-globe"></i><span class="min-w-0 flex-1 truncate font-semibold">{t('platform.ai.rules.scopeGlobal')}</span>
      </button>

      <h3 class="side-h">{t('platform.ai.rules.scopeTenants')} <b>{tenants.length}</b></h3>
      <div class="list scroll-thin">
        {#each tenants as r (r.tenant_id)}
          <button type="button" class="item" class:on={scope.tenantId === r.tenant_id} onclick={() => open({ tenantId: r.tenant_id, name: r.tenant_name })}>
            <i class="icon-store"></i>
            <span class="min-w-0 flex-1 truncate">{r.tenant_name} <span class="font-mono text-[10px] opacity-70">{r.tenant_code}</span></span>
            <span class="shrink-0 font-mono text-[10px] opacity-80">{t('platform.ai.rules.ver', { n: r.version })}</span>
          </button>
        {:else}
          <p class="px-1 py-1 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.rules.scopeEmpty')}</p>
        {/each}
      </div>

      <form class="add" onsubmit={(e) => { e.preventDefault(); void openFound(); }}>
        <div class="add-row">
          <input id="air-code" class="field-control" autocomplete="off" spellcheck="false" aria-label={t('platform.ai.rules.scopeAdd')} placeholder={t('platform.ai.rules.tenantCode')} bind:value={code} />
          <button type="submit" class="add-btn" disabled={lookup.state !== 'found'} title={t('platform.ai.rules.scopeAdd')} aria-label={t('platform.ai.rules.scopeAdd')}><i class="icon-plus"></i></button>
        </div>
        <p class="add-h" data-state={lookup.state}>
          {#if lookup.state === 'found'}{t('platform.ai.rules.tenantFound', { name: lookup.name })}
          {:else if lookup.state === 'missing'}{t('platform.ai.rules.tenantMissing')}
          {:else if lookup.state === 'checking'}{t('platform.ai.rules.tenantChecking')}
          {:else}{t('platform.ai.rules.tenantHelp')}{/if}
        </p>
      </form>
    </aside>

    <section class="min-w-0 space-y-3">
      <!-- Editor -->
      <div class="surface-card !p-0 overflow-hidden">
        <div class="bar">
          <div class="min-w-0 flex-1">
            <h2 class="truncate text-[14px] font-semibold">{scope.tenantId ? t('platform.ai.rules.editing', { name: scope.name }) : t('platform.ai.rules.scopeGlobal')}</h2>
            <p class="mt-0.5 flex flex-wrap items-center gap-x-2 text-[11px] text-[var(--text-tertiary)]">
              {#if current}<span class="font-mono font-semibold">{t('platform.ai.rules.ver', { n: current.version })}</span><span>{current.author} · {formatDateTime(current.created_at)}</span>{:else}<span>{t('platform.ai.rules.historyEmpty')}</span>{/if}
              {#if scope.tenantId}<span>· {t('platform.ai.rules.scopeNote')}</span>{/if}
            </p>
          </div>
          <button type="button" class="btn btn-outline !py-1 !text-[12px]" onclick={insertTemplate}><i class="icon-file-text text-[13px]"></i>{t('platform.ai.rules.template')}</button>
          <button type="button" class="btn btn-outline !py-1 !text-[12px]" disabled={!body} onclick={() => (body = '')}>{t('platform.ai.rules.clear')}</button>
        </div>

        <div class="p-3">
          {#if loading}
            <div class="h-44 animate-pulse rounded-lg bg-[var(--surface-sunken)]"></div>
          {:else}
            <MarkdownEditor id="air-body" bind:value={body} rows={11} maxlength={maxChars} invalid={!!fieldErr} placeholder={t('platform.ai.rules.placeholder')} />
            {#if fieldErr}<p class="mt-1 text-[11.5px] text-[var(--color-danger-600)]">{fieldErr}</p>{/if}
          {/if}
        </div>

        <div class="foot">
          <div class="meter-wrap" title="{formatNumber(body.length)} / {formatNumber(maxChars)}">
            <div class="meter"><div class="fill" class:warn={pct > 70} class:crit={pct > 90} style="width:{pct}%"></div></div>
            <p>{pct}% · {t('platform.ai.tokens', { count: formatNumber(Math.ceil(body.length / 4)) })}</p>
          </div>
          <span class="dirty">{dirty ? t('platform.ai.dirty') : ''}</span>
          <button type="button" class="btn btn-outline !py-1 !text-[12px]" disabled={!dirty || saving} onclick={() => (body = baseline)}>{t('platform.ai.reset')}</button>
          <button type="button" class="btn btn-primary !py-1 !text-[12px] disabled:opacity-60" disabled={saving || loading || !dirty} onclick={save}>{saving ? t('platform.ai.rules.saving') : t('platform.ai.rules.save')}</button>
        </div>
      </div>

      <!-- Uji tanya -->
      <div class="surface-card !p-0 overflow-hidden">
        <div class="bar">
          <span class="bar-ico"><i class="icon-flask-conical"></i></span>
          <div class="min-w-0 flex-1">
            <h3 class="text-[13px] font-semibold">{t('platform.ai.rules.test.title')}</h3>
            <p class="text-[11px] text-[var(--text-tertiary)]">{t('platform.ai.rules.test.hint')}</p>
          </div>
        </div>
        <form class="space-y-2.5 p-3" onsubmit={(e) => { e.preventDefault(); void runTest(); }}>
          <div class="flex flex-wrap items-center gap-1.5">
            <span class="text-[10.5px] font-bold uppercase tracking-[0.08em] text-[var(--text-tertiary)]">{t('platform.ai.rules.test.samples')}</span>
            {#each samples as sm (sm)}
              <button type="button" class="sample" onclick={() => (question = sm)}>{sm}</button>
            {/each}
          </div>
          <div class="flex flex-wrap gap-2">
            <input class="field-control min-w-56 flex-1" maxlength="600" bind:value={question} placeholder={t('platform.ai.rules.test.placeholder')} aria-label={t('platform.ai.rules.test.title')} />
            <button type="submit" class="btn btn-primary !py-1 !text-[12px] disabled:opacity-60" disabled={testing || !question.trim()}>
              <i class="icon-{testing ? 'loader' : 'zap'} text-[13px] {testing ? 'animate-spin' : ''}"></i>{testing ? t('platform.ai.rules.test.running') : t('platform.ai.rules.test.run')}
            </button>
          </div>
          <p class="text-[10.5px] text-[var(--text-tertiary)]"><i class="icon-info"></i> {t('platform.ai.rules.test.usesDraft')} · {t('platform.ai.rules.test.limit')}</p>

          {#if testError}<div class="rounded-lg border border-[var(--color-danger-500)]/20 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]" role="alert">{testError}</div>{/if}
          {#if testing}
            <div class="space-y-1.5" aria-busy="true"><div class="h-3 w-2/3 animate-pulse rounded bg-[var(--surface-sunken)]"></div><div class="h-3 w-5/6 animate-pulse rounded bg-[var(--surface-sunken)]"></div><div class="h-3 w-1/2 animate-pulse rounded bg-[var(--surface-sunken)]"></div></div>
          {:else if testResult}
            <div class="answer">
              <p class="answer-h"><i class="icon-sparkles"></i>{t('platform.ai.rules.test.reply')}</p>
              <MarkdownView source={testResult.reply} class="text-[12.5px] leading-relaxed" />
              <p class="answer-m">{t('platform.ai.rules.test.meta', { model: testResult.model, ms: formatNumber(testResult.elapsed_ms), in: formatNumber(testResult.input_tokens), out: formatNumber(testResult.output_tokens) })}</p>
            </div>
          {/if}
        </form>
      </div>

      <!-- Riwayat versi -->
      <div class="surface-card !p-0 overflow-hidden">
        <h3 class="side-h !m-0 border-b border-[var(--border-subtle)] !px-3.5 !py-2">{t('platform.ai.rules.history')} <b>{versions.length}</b></h3>
        {#if versions.length === 0}
          <p class="px-3.5 py-3 text-[12px] text-[var(--text-tertiary)]">{t('platform.ai.rules.historyEmpty')}</p>
        {:else}
          <ul class="max-h-52 overflow-y-auto scroll-thin">
            {#each versions as v (v.version)}
              <li class="vrow">
                <span class="w-8 shrink-0 font-mono font-semibold">{t('platform.ai.rules.ver', { n: v.version })}</span>
                <span class="min-w-0 flex-1 truncate text-[var(--text-secondary)]">{v.body.trim() ? v.body.trim().split('\n')[0] : t('platform.ai.rules.emptyVersion')}</span>
                <span class="hidden shrink-0 font-mono text-[10.5px] text-[var(--text-tertiary)] sm:inline">{formatNumber(v.body.length)} · {v.author} · {formatDateTime(v.created_at)}</span>
                <button type="button" class="btn btn-outline !px-2 !py-0.5 !text-[11px]" onclick={() => loadVersion(v)}>{t('platform.ai.rules.load')}</button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </section>
  </div>
</div>

<style>
  .flowbar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.35rem 0.5rem; padding: 0.45rem 0.7rem; border: 1px solid var(--border-subtle); border-radius: 0.65rem; background: var(--surface-base); }
  .flow-t { font-size: 10px; font-weight: 700; letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-tertiary); }
  .step { display: inline-flex; align-items: center; gap: 0.35rem; padding: 0.15rem 0.55rem; border: 1px dashed var(--border-subtle); border-radius: 0.4rem; font-size: 11.5px; font-weight: 600; color: var(--text-tertiary); }
  .step em { font: 600 9px var(--font-mono, ui-monospace, monospace); font-style: normal; letter-spacing: 0.08em; text-transform: uppercase; opacity: 0.8; }
  .step[data-hard='true'] { border-style: solid; border-color: color-mix(in srgb, var(--color-success-500) 40%, transparent); background: color-mix(in srgb, var(--color-success-500) 8%, transparent); color: var(--color-success-600); }
  .step[data-hard='false'][data-active='true'] { border-style: solid; border-color: color-mix(in srgb, var(--color-primary-500) 55%, transparent); background: color-mix(in srgb, var(--color-primary-500) 10%, transparent); color: var(--color-primary-600); }
  .arrow { font-size: 12px; color: var(--text-tertiary); }

  /* daftar dokumen */
  .side { display: flex; flex-direction: column; gap: 0.5rem; }
  .side-h { display: flex; align-items: center; justify-content: space-between; margin-top: 0.25rem; padding-inline: 0.25rem; font-size: 10px; font-weight: 700; letter-spacing: 0.09em; text-transform: uppercase; color: var(--text-tertiary); }
  .side-h b { padding: 0 0.4rem; border-radius: 9999px; background: var(--surface-sunken); font: 600 10px var(--font-mono, ui-monospace, monospace); letter-spacing: 0; }
  .list { display: flex; flex-direction: column; gap: 2px; max-height: 15rem; overflow-y: auto; }
  .item { display: flex; align-items: center; gap: 0.45rem; width: 100%; padding: 0.4rem 0.55rem; border-radius: 0.5rem; font-size: 12px; text-align: start; color: var(--text-secondary); transition: background-color 0.12s; }
  .item :global(i) { flex: none; font-size: 13px; }
  .item:hover { background: var(--surface-sunken); }
  .item.on { background: var(--color-primary-600); color: #fff; }
  .add { display: flex; flex-direction: column; gap: 0.3rem; padding-top: 0.6rem; border-top: 1px solid var(--border-subtle); }
  .add-row { display: flex; gap: 0.35rem; min-width: 0; }
  .add-row :global(input) { flex: 1 1 0; min-width: 0; width: 100%; font-family: var(--font-mono, ui-monospace, monospace); font-size: 12px; }
  .add-btn { display: grid; place-items: center; flex: none; width: 2.25rem; border: 1px solid var(--border-default); border-radius: 0.5rem; background: var(--surface-base); color: var(--color-primary-600); font-size: 14px; }
  .add-btn:not(:disabled):hover { background: var(--color-primary-600); color: #fff; }
  .add-btn:disabled { opacity: 0.45; }
  .add-h { min-height: 1rem; font-size: 10.5px; color: var(--text-tertiary); }
  .add-h[data-state='found'] { font-weight: 600; color: var(--color-success-600); }
  .add-h[data-state='missing'] { color: var(--color-danger-600); }

  /* kartu editor / uji / riwayat */
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem; padding: 0.6rem 0.8rem; border-bottom: 1px solid var(--border-subtle); background: var(--surface-sunken); }
  .bar-ico { display: grid; place-items: center; flex: none; width: 1.9rem; height: 1.9rem; border-radius: 0.5rem; background: color-mix(in srgb, var(--color-primary-500) 14%, transparent); color: var(--color-primary-600); font-size: 15px; }
  .foot { display: flex; flex-wrap: wrap; align-items: center; gap: 0.6rem; padding: 0.6rem 0.8rem; border-top: 1px solid var(--border-subtle); }
  .meter-wrap { flex: 1 1 10rem; min-width: 8rem; }
  .meter { height: 4px; overflow: hidden; border-radius: 9999px; background: var(--surface-sunken); }
  .fill { height: 100%; border-radius: 9999px; background: var(--color-primary-500); transition: width 0.2s; }
  .fill.warn { background: var(--color-warning-500); }
  .fill.crit { background: var(--color-danger-500); }
  .meter-wrap p { margin-top: 0.25rem; font: 500 10.5px var(--font-mono, ui-monospace, monospace); color: var(--text-tertiary); }
  .dirty { font-size: 11.5px; font-weight: 600; color: var(--color-warning-600); }

  .sample { max-width: 100%; padding: 0.15rem 0.6rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; border: 1px solid var(--border-subtle); border-radius: 9999px; font-size: 11.5px; color: var(--text-secondary); transition: all 0.12s; }
  .sample:hover { border-color: var(--color-primary-400); color: var(--color-primary-600); }
  .answer { padding: 0.7rem 0.85rem; border: 1px solid color-mix(in srgb, var(--color-primary-500) 35%, var(--border-subtle)); border-radius: 0.75rem; background: color-mix(in srgb, var(--color-primary-500) 5%, var(--surface-base)); }
  .answer-h { display: flex; align-items: center; gap: 0.4rem; margin-bottom: 0.4rem; font-size: 10.5px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--color-primary-600); }
  .answer-m { margin-top: 0.5rem; font: 500 10.5px var(--font-mono, ui-monospace, monospace); color: var(--text-tertiary); }

  .vrow { display: flex; align-items: center; gap: 0.6rem; padding: 0.4rem 0.9rem; border-top: 1px solid var(--border-subtle); font-size: 12px; }
  .vrow:first-child { border-top: 0; }
  .vrow:hover { background: var(--surface-sunken); }
  @media (prefers-reduced-motion: reduce) { .item, .fill, .sample { transition: none; } }
</style>
