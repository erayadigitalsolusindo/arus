<script lang="ts">
  // Formulir kebijakan GLOBAL HalloArus (padat): kartu kemampuan bersaklar, chip domain data, dan batas pemakaian.
  // Pengecualian per tenant memakai AiOverrideEditor.
  import Switch from '#lib/components/Switch.svelte';
  import { t } from '#lib/i18n/index.ts';
  import { fieldMessage } from '#lib/i18n/errors.ts';
  import type { AiPolicyValues } from '#lib/platform/api.ts';
  import { FEATURES, DOMAIN_META } from '#lib/platform/aiPolicyMeta.ts';

  type Props = { value: AiPolicyValues; domains: string[]; domainTools?: Record<string, string[]>; freeModels: string[]; errors?: Record<string, string> };
  let { value = $bindable(), domains, domainTools = {}, freeModels, errors = {} }: Props = $props();

  const num = (s: string) => (s.trim() === '' ? null : Number(s));
  const fe = (k: string) => (errors[k] ? fieldMessage(errors[k]) : '');
  const domainSet = $derived(new Set(value.domains ?? []));
  const toggleDomain = (d: string) => {
    const cur = value.domains ?? [];
    value.domains = cur.includes(d) ? cur.filter((x) => x !== d) : [...cur, d];
  };
  const setAll = (on: boolean) => (value.domains = on ? [...domains] : []);
  const toolsTip = (d: string) => (domainTools[d]?.length ? t('platform.ai.domainHint', { tools: domainTools[d].join(', ') }) : '');
</script>

<div class="space-y-4">
  <!-- Kemampuan -->
  <section>
    <h3 class="sec">{t('platform.ai.sFeatures')}</h3>
    <div class="mt-2 grid gap-2 sm:grid-cols-2 2xl:grid-cols-3">
      {#each FEATURES as f (f.key)}
        {@const on = value[f.key] === true}
        <div class="tile" class:master={f.key === 'enabled'} data-s={on ? 'allow' : 'block'}>
          <span class="tile-ico"><i class="icon-{f.icon}"></i></span>
          <div class="min-w-0 flex-1">
            <p class="tile-t"><span class="truncate">{t(f.label)}</span>{#if f.risky}<span class="risk" title="!"></span>{/if}</p>
            <p class="tile-h" title={t(f.hint)}>{t(f.hint)}</p>
            {#if fe(f.key)}<p class="text-[11px] text-[var(--color-danger-600)]">{fe(f.key)}</p>{/if}
          </div>
          <Switch bind:checked={() => value[f.key] === true, (v) => (value[f.key] = v)} label={t(f.label)} />
        </div>
      {/each}
    </div>
  </section>

  <!-- Domain data -->
  <section>
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="sec">{t('platform.ai.sData')} <span class="ms-1 font-mono normal-case tracking-normal">{domainSet.size}/{domains.length}</span></h3>
      <div class="flex items-center gap-3 text-[11.5px] font-semibold">
        <button type="button" class="link" onclick={() => setAll(true)}>{t('platform.ai.allOn')}</button>
        <button type="button" class="link" onclick={() => setAll(false)}>{t('platform.ai.allOff')}</button>
      </div>
    </div>
    <p class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.domainsHint')}</p>
    <div class="mt-2 flex flex-wrap gap-1.5">
      {#each domains as d (d)}
        {@const meta = DOMAIN_META[d]}
        <button type="button" class="chip" aria-pressed={domainSet.has(d)} title={toolsTip(d)} onclick={() => toggleDomain(d)}>
          <i class="icon-{meta?.icon ?? 'database'}"></i>{meta ? t(meta.label) : d}
          {#if domainTools[d]}<em>{domainTools[d].length}</em>{/if}
        </button>
      {/each}
    </div>
    {#if fe('domains')}<p class="mt-1 text-[11px] text-[var(--color-danger-600)]">{fe('domains')}</p>{/if}
  </section>

  <!-- Batas -->
  <section>
    <h3 class="sec">{t('platform.ai.sLimits')}</h3>
    <div class="mt-2 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
      <label class="num" title={t('platform.ai.chatLimitHint')}>
        <span>{t('platform.ai.chatLimit')}</span>
        <span class="num-f"><input inputmode="numeric" value={value.chat_limit ?? ''} oninput={(e) => (value.chat_limit = num(e.currentTarget.value))} /><em>{t('platform.ai.unitMin')}</em></span>
        {#if fe('chat_limit')}<b>{fe('chat_limit')}</b>{/if}
      </label>
      <label class="num" title={t('platform.ai.maxRangeHint')}>
        <span>{t('platform.ai.maxRange')}</span>
        <span class="num-f"><input inputmode="numeric" value={value.max_range_days ?? ''} oninput={(e) => (value.max_range_days = num(e.currentTarget.value))} /><em>{t('platform.ai.unitDay')}</em></span>
        {#if fe('max_range_days')}<b>{fe('max_range_days')}</b>{/if}
      </label>
      <label class="num" title={t('platform.ai.freeLimitHint')}>
        <span>{t('platform.ai.freeLimit')}</span>
        <span class="num-f"><input inputmode="numeric" placeholder={t('platform.ai.serverVal')} value={value.free_daily_limit ?? ''} oninput={(e) => (value.free_daily_limit = num(e.currentTarget.value))} /><em>{t('platform.ai.unitQ')}</em></span>
        {#if fe('free_daily_limit')}<b>{fe('free_daily_limit')}</b>{/if}
      </label>
      <label class="num">
        <span>{t('platform.ai.freeModel')}</span>
        <span class="num-f">
          <select value={value.free_model ?? ''} onchange={(e) => (value.free_model = e.currentTarget.value || null)}>
            <option value="">{t('platform.ai.freeModelDefault')}</option>
            {#each freeModels as m (m)}<option value={m}>{m}</option>{/each}
          </select>
        </span>
        {#if fe('free_model')}<b>{fe('free_model')}</b>{/if}
      </label>
    </div>
  </section>
</div>

<style>
  .sec { font-size: 10.5px; font-weight: 700; letter-spacing: 0.09em; text-transform: uppercase; color: var(--text-tertiary); }
  .link { color: var(--color-primary-600); }
  .link:hover { text-decoration: underline; }

  .tile { display: flex; align-items: center; gap: 0.65rem; min-width: 0; padding: 0.55rem 0.7rem; border: 1px solid var(--border-subtle); border-radius: 0.7rem; background: var(--surface-base); transition: border-color 0.15s, background-color 0.15s; }
  .tile.master { grid-column: 1 / -1; padding-block: 0.7rem; }
  .tile-ico { display: grid; place-items: center; flex: none; width: 2rem; height: 2rem; border-radius: 0.55rem; background: var(--surface-sunken); color: var(--text-tertiary); font-size: 15px; transition: all 0.15s; }
  .tile-t { display: flex; align-items: center; gap: 0.35rem; min-width: 0; font-size: 12.5px; font-weight: 600; line-height: 1.25; }
  .tile-h { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; margin-top: 1px; font-size: 11px; line-height: 1.35; color: var(--text-tertiary); }
  .risk { flex: none; width: 0.4rem; height: 0.4rem; border-radius: 9999px; background: var(--color-warning-500); }
  .tile[data-s='allow'] { border-color: color-mix(in srgb, var(--color-success-500) 38%, var(--border-subtle)); background: color-mix(in srgb, var(--color-success-500) 5%, var(--surface-base)); }
  .tile[data-s='allow'] .tile-ico { background: color-mix(in srgb, var(--color-success-500) 16%, transparent); color: var(--color-success-600); }
  .tile[data-s='block'] { border-color: color-mix(in srgb, var(--color-danger-500) 38%, var(--border-subtle)); background: color-mix(in srgb, var(--color-danger-500) 5%, var(--surface-base)); }
  .tile[data-s='block'] .tile-ico { background: color-mix(in srgb, var(--color-danger-500) 15%, transparent); color: var(--color-danger-600); }

  .chip { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.7rem; border: 1px solid var(--border-subtle); border-radius: 9999px; font-size: 12px; font-weight: 600; color: var(--text-tertiary); transition: all 0.12s; }
  .chip :global(i) { font-size: 13px; }
  .chip em { padding: 0 0.35rem; border-radius: 9999px; background: var(--surface-sunken); font: 600 10px var(--font-mono, ui-monospace, monospace); font-style: normal; }
  .chip[aria-pressed='true'] { border-color: color-mix(in srgb, var(--color-primary-500) 55%, transparent); background: color-mix(in srgb, var(--color-primary-500) 12%, transparent); color: var(--color-primary-600); }
  .chip:hover { border-color: var(--color-primary-400); }

  .num { display: block; min-width: 0; padding: 0.45rem 0.65rem; border: 1px solid var(--border-subtle); border-radius: 0.7rem; background: var(--surface-base); }
  .num > span:first-child { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10.5px; font-weight: 600; color: var(--text-tertiary); }
  .num-f { display: flex; align-items: baseline; gap: 0.35rem; }
  .num-f input, .num-f select { width: 100%; min-width: 0; border: 0; padding: 0; background: transparent; font-family: var(--font-mono, ui-monospace, monospace); font-size: 15px; font-weight: 600; color: var(--text-primary); outline: none; }
  .num-f select { font-family: inherit; font-size: 12.5px; }
  .num-f em { flex: none; font-style: normal; font-size: 10.5px; color: var(--text-tertiary); }
  .num:focus-within { border-color: var(--color-primary-500); box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 18%, transparent); }
  .num b { display: block; font-size: 10.5px; font-weight: 500; color: var(--color-danger-600); }
  @media (prefers-reduced-motion: reduce) { .tile, .tile-ico, .chip { transition: none; } }
</style>
