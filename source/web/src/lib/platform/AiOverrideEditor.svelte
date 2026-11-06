<script lang="ts">
  // Editor pengecualian tenant bergaya matriks: tiap baris menampilkan nilai GLOBAL, pilihan tiga segmen (ikut / izinkan / blokir),
  // dan HASIL akhir. Field yang tidak diubah (null) ikut global. Panel kanan menampilkan hasil akhir untuk tenant ini.
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import { fieldMessage } from '#lib/i18n/errors.ts';
  import type { AiPolicyValues } from '#lib/platform/api.ts';
  import { FEATURES, DOMAIN_META, mergeValues, changedCount, emptyPolicy, type BoolKey } from '#lib/platform/aiPolicyMeta.ts';
  import AiEffectPanel from '#lib/platform/AiEffectPanel.svelte';

  type Props = { value: AiPolicyValues; global: AiPolicyValues; domains: string[]; domainTools?: Record<string, string[]>; freeModels: string[]; errors?: Record<string, string> };
  let { value = $bindable(), global, domains, domainTools = {}, freeModels, errors = {} }: Props = $props();

  type Tri = 'inherit' | 'allow' | 'block';
  const tri = (v: boolean | null): Tri => (v === null ? 'inherit' : v ? 'allow' : 'block');
  const fromTri = (s: Tri) => (s === 'inherit' ? null : s === 'allow');
  const SEGS: { v: Tri; icon: string; label: MessageKey }[] = [
    { v: 'inherit', icon: 'rotate-ccw', label: 'platform.ai.inherit' },
    { v: 'allow', icon: 'check', label: 'platform.ai.allow' },
    { v: 'block', icon: 'x', label: 'platform.ai.block' }
  ];
  const fe = (k: string) => (errors[k] ? fieldMessage(errors[k]) : '');
  const num = (s: string) => (s.trim() === '' ? null : Number(s));

  const merged = $derived(mergeValues(global, value));
  const changed = $derived(changedCount(value));
  const domainsInherit = $derived(value.domains === null);
  const finalDomains = $derived(new Set(merged.domains ?? []));
  const globalDomains = $derived(new Set(global.domains ?? []));

  function toggleDomain(d: string) {
    const cur = value.domains ?? [...(global.domains ?? [])];
    value.domains = cur.includes(d) ? cur.filter((x) => x !== d) : [...cur, d];
  }
  const resetAll = () => Object.assign(value, emptyPolicy());
  const rowState = (k: BoolKey) => tri(value[k]);
</script>

{#snippet pill(on: boolean | null | undefined)}
  <span class="pill" data-on={on === true ? 'y' : on === false ? 'n' : 'x'}>{on === true ? t('platform.ai.can') : on === false ? t('platform.ai.cannot') : '—'}</span>
{/snippet}

<div class="grid items-start gap-3 xl:grid-cols-[minmax(0,1fr)_15.5rem]">
  <div class="min-w-0 space-y-3">
    <!-- Kemampuan: global → pilihan → hasil -->
    <section class="matrix">
      <header class="mh">
        <h3>{t('platform.ai.ovSecCap')}</h3>
        <span class="col">{t('platform.ai.ovGlobal')}</span>
        <span class="col">{t('platform.ai.ovPick')}</span>
        <span class="col">{t('platform.ai.ovResult')}</span>
      </header>
      {#each FEATURES as f (f.key)}
        {@const st = rowState(f.key)}
        <div class="row" data-s={st}>
          <span class="ico"><i class="icon-{f.icon}"></i></span>
          <div class="min-w-0">
            <p class="lab"><span class="truncate">{t(f.label)}</span>{#if f.risky}<span class="risk"></span>{/if}</p>
            <p class="hint" title={t(f.hint)}>{t(f.hint)}</p>
            {#if fe(f.key)}<p class="err">{fe(f.key)}</p>{/if}
          </div>
          <span class="cell">{@render pill(global[f.key])}</span>
          <div class="seg cell" role="group" aria-label={t(f.label)}>
            {#each SEGS as s (s.v)}
              <button type="button" class="seg-b" data-v={s.v} aria-pressed={st === s.v} title={t(s.label)} aria-label={t(s.label)} onclick={() => (value[f.key] = fromTri(s.v))}>
                <i class="icon-{s.icon}"></i>
              </button>
            {/each}
          </div>
          <span class="cell">{@render pill(merged[f.key])}{#if st !== 'inherit'}<i class="own" title={t('platform.ai.ovOwn')}></i>{/if}</span>
        </div>
      {/each}
    </section>

    <!-- Domain data -->
    <section class="matrix">
      <header class="mh">
        <h3>{t('platform.ai.ovSecData')} <span class="font-mono normal-case tracking-normal">{finalDomains.size}/{domains.length}</span></h3>
        <label class="ms-auto flex cursor-pointer items-center gap-1.5 text-[11.5px] font-semibold normal-case tracking-normal">
          <input type="checkbox" checked={domainsInherit} onchange={(e) => (value.domains = e.currentTarget.checked ? null : [...(global.domains ?? [])])} />{t('platform.ai.domainsInherit')}
        </label>
      </header>
      <div class="flex flex-wrap gap-1.5 px-3 py-2.5" title={t('platform.ai.ovDomainTip')}>
        {#each domains as d (d)}
          {@const meta = DOMAIN_META[d]}
          <button type="button" class="chip" aria-pressed={finalDomains.has(d)} disabled={domainsInherit} data-global={globalDomains.has(d)} title={domainTools[d]?.join(', ')} onclick={() => toggleDomain(d)}>
            <i class="icon-{meta?.icon ?? 'database'}"></i>{meta ? t(meta.label) : d}
            <span class="gdot" aria-hidden="true"></span>
          </button>
        {/each}
      </div>
      {#if fe('domains')}<p class="err px-3 pb-2">{fe('domains')}</p>{/if}
    </section>

    <!-- Batas -->
    <section class="matrix">
      <header class="mh"><h3>{t('platform.ai.ovSecLimit')}</h3></header>
      <div class="grid gap-2 p-3 sm:grid-cols-2">
        {#each [
          { k: 'chat_limit', label: t('platform.ai.chatLimit'), unit: t('platform.ai.unitMin') },
          { k: 'max_range_days', label: t('platform.ai.maxRange'), unit: t('platform.ai.unitDay') },
          { k: 'free_daily_limit', label: t('platform.ai.freeLimit'), unit: t('platform.ai.unitQ') }
        ] as n (n.k)}
          {@const key = n.k as 'chat_limit' | 'max_range_days' | 'free_daily_limit'}
          <label class="num" data-own={value[key] !== null}>
            <span>{n.label}</span>
            <span class="num-f"><input inputmode="numeric" placeholder={String(global[key] ?? t('platform.ai.serverVal'))} value={value[key] ?? ''} oninput={(e) => (value[key] = num(e.currentTarget.value))} /><em>{n.unit}</em></span>
            <small>{t('platform.ai.ovGlobal')}: {global[key] ?? t('platform.ai.serverVal')}</small>
            {#if fe(key)}<b>{fe(key)}</b>{/if}
          </label>
        {/each}
        <label class="num" data-own={value.free_model !== null}>
          <span>{t('platform.ai.freeModel')}</span>
          <span class="num-f">
            <select value={value.free_model ?? ''} onchange={(e) => (value.free_model = e.currentTarget.value || null)}>
              <option value="">↺ {global.free_model ?? t('platform.ai.freeModelDefault')}</option>
              {#each freeModels as m (m)}<option value={m}>{m}</option>{/each}
            </select>
          </span>
          <small>{t('platform.ai.ovGlobal')}: {global.free_model ?? t('platform.ai.freeModelDefault')}</small>
          {#if fe('free_model')}<b>{fe('free_model')}</b>{/if}
        </label>
      </div>
    </section>
  </div>

  <div class="space-y-3 xl:sticky xl:top-0">
    <div class="rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3">
      <p class="text-[10.5px] font-bold uppercase tracking-[0.09em] text-[var(--text-tertiary)]">{t('platform.ai.ovChanged', { count: changed })}</p>
      <button type="button" class="mt-2 inline-flex items-center gap-1.5 text-[12px] font-semibold text-[var(--color-primary-600)] hover:underline disabled:opacity-40 disabled:no-underline" disabled={changed === 0} onclick={resetAll}>
        <i class="icon-rotate-ccw text-[13px]"></i>{t('platform.ai.ovResetAll')}
      </button>
    </div>
    <AiEffectPanel value={merged} {domains} title={t('platform.ai.ovPreview')} />
  </div>
</div>

<style>
  .matrix { border: 1px solid var(--border-subtle); border-radius: 0.8rem; background: var(--surface-base); overflow: hidden; }
  .mh { display: grid; grid-template-columns: minmax(0, 1fr) 4.2rem 5.9rem 4.6rem; align-items: center; gap: 0.5rem; padding: 0.4rem 0.75rem; background: var(--surface-sunken); font-size: 10.5px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-tertiary); }
  .mh:has(label) { display: flex; }
  .mh h3 { font-size: inherit; }
  .col { text-align: center; }
  .row { display: grid; grid-template-columns: 2rem minmax(0, 1fr) 4.2rem 5.9rem 4.6rem; align-items: center; gap: 0.5rem; padding: 0.45rem 0.75rem; border-top: 1px solid var(--border-subtle); transition: background-color 0.12s; }
  .row[data-s='allow'] { background: color-mix(in srgb, var(--color-success-500) 5%, transparent); }
  .row[data-s='block'] { background: color-mix(in srgb, var(--color-danger-500) 5%, transparent); }
  .ico { display: grid; place-items: center; width: 2rem; height: 2rem; border-radius: 0.5rem; background: var(--surface-sunken); color: var(--text-tertiary); font-size: 15px; }
  .row[data-s='allow'] .ico { background: color-mix(in srgb, var(--color-success-500) 16%, transparent); color: var(--color-success-600); }
  .row[data-s='block'] .ico { background: color-mix(in srgb, var(--color-danger-500) 15%, transparent); color: var(--color-danger-600); }
  .lab { display: flex; align-items: center; gap: 0.35rem; font-size: 12.5px; font-weight: 600; line-height: 1.25; }
  .hint { display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; font-size: 11px; color: var(--text-tertiary); }
  .err { font-size: 11px; color: var(--color-danger-600); }
  .risk { flex: none; width: 0.4rem; height: 0.4rem; border-radius: 9999px; background: var(--color-warning-500); }
  .cell { display: flex; align-items: center; justify-content: center; gap: 0.3rem; }

  .pill { padding: 0.05rem 0.45rem; border-radius: 0.3rem; font: 700 9.5px var(--font-mono, ui-monospace, monospace); letter-spacing: 0.06em; text-transform: uppercase; }
  .pill[data-on='y'] { background: color-mix(in srgb, var(--color-success-500) 15%, transparent); color: var(--color-success-600); }
  .pill[data-on='n'] { background: color-mix(in srgb, var(--color-danger-500) 14%, transparent); color: var(--color-danger-600); }
  .pill[data-on='x'] { background: var(--surface-sunken); color: var(--text-tertiary); }
  .own { width: 0.4rem; height: 0.4rem; border-radius: 9999px; background: var(--color-primary-500); box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary-500) 25%, transparent); }

  .seg { display: inline-flex; gap: 1px; padding: 2px; border: 1px solid var(--border-subtle); border-radius: 0.5rem; background: var(--surface-sunken); width: max-content; margin-inline: auto; }
  .seg-b { display: grid; place-items: center; width: 1.75rem; height: 1.5rem; border-radius: 0.35rem; font-size: 13px; color: var(--text-tertiary); transition: all 0.12s; }
  .seg-b:hover { color: var(--text-primary); }
  .seg-b[aria-pressed='true'][data-v='inherit'] { background: var(--surface-base); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
  .seg-b[aria-pressed='true'][data-v='allow'] { background: var(--color-success-600); color: #fff; }
  .seg-b[aria-pressed='true'][data-v='block'] { background: var(--color-danger-600); color: #fff; }

  .chip { position: relative; display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.7rem; border: 1px solid var(--border-subtle); border-radius: 9999px; font-size: 12px; font-weight: 600; color: var(--text-tertiary); transition: all 0.12s; }
  .chip :global(i) { font-size: 13px; }
  .chip[aria-pressed='true'] { border-color: color-mix(in srgb, var(--color-primary-500) 55%, transparent); background: color-mix(in srgb, var(--color-primary-500) 12%, transparent); color: var(--color-primary-600); }
  .chip:disabled { opacity: 0.6; }
  .chip:not(:disabled):hover { border-color: var(--color-primary-400); }
  .gdot { width: 0.35rem; height: 0.35rem; border-radius: 9999px; background: var(--border-default, #cbd5e1); }
  .chip[data-global='true'] .gdot { background: var(--color-success-500); }

  .num { display: block; min-width: 0; padding: 0.45rem 0.65rem; border: 1px solid var(--border-subtle); border-radius: 0.7rem; background: var(--surface-base); }
  .num[data-own='true'] { border-color: color-mix(in srgb, var(--color-primary-500) 55%, transparent); background: color-mix(in srgb, var(--color-primary-500) 5%, var(--surface-base)); }
  .num > span:first-child { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10.5px; font-weight: 600; color: var(--text-tertiary); }
  .num-f { display: flex; align-items: baseline; gap: 0.35rem; }
  .num-f input, .num-f select { width: 100%; min-width: 0; border: 0; padding: 0; background: transparent; font-family: var(--font-mono, ui-monospace, monospace); font-size: 15px; font-weight: 600; color: var(--text-primary); outline: none; }
  .num-f select { font-family: inherit; font-size: 12.5px; }
  .num-f em { flex: none; font-style: normal; font-size: 10.5px; color: var(--text-tertiary); }
  .num small { display: block; font-size: 10.5px; color: var(--text-tertiary); }
  .num b { display: block; font-size: 10.5px; font-weight: 500; color: var(--color-danger-600); }
  .num:focus-within { border-color: var(--color-primary-500); box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 18%, transparent); }

  @media (max-width: 640px) {
    .mh { grid-template-columns: minmax(0, 1fr) 3.4rem 5.4rem 3.6rem; padding-inline: 0.5rem; }
    .row { grid-template-columns: 1.7rem minmax(0, 1fr) 3.4rem 5.4rem 3.6rem; gap: 0.35rem; padding-inline: 0.5rem; }
    .hint { display: none; }
  }
  @media (prefers-reduced-motion: reduce) { .row, .seg-b, .chip { transition: none; } }
</style>
