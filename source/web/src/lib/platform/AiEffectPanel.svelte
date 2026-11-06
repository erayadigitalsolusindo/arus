<script lang="ts">
  // Ringkasan efektif: postur + apa yang bisa/tidak bisa dilakukan AI untuk satu set nilai kebijakan (sudah final, tanpa null).
  import { t } from '#lib/i18n/index.ts';
  import type { AiPolicyValues } from '#lib/platform/api.ts';
  import { FEATURES, posture } from '#lib/platform/aiPolicyMeta.ts';

  let { value, domains, title = '' }: { value: AiPolicyValues; domains: string[]; title?: string } = $props();

  const post = $derived(posture(value, domains));
  const open = $derived(domains.filter((d) => (value.domains ?? []).includes(d)).length);
</script>

<aside class="surface-card !p-3" aria-label={title || t('platform.ai.canTitle')}>
  <div class="flex items-center justify-between gap-2">
    <h3 class="text-[10.5px] font-bold uppercase tracking-[0.09em] text-[var(--text-tertiary)]">{title || t('platform.ai.canTitle')}</h3>
    <span class="meter" data-level={post.level} title={t('platform.ai.postureHint', { blocked: post.blocked, total: post.total })}><i></i><i></i><i></i></span>
  </div>
  <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.ai.postureHint', { blocked: post.blocked, total: post.total })}</p>
  <ul class="mt-2 space-y-1">
    {#each FEATURES as f (f.key)}
      {@const on = value[f.key] === true}
      <li class="can" data-on={on}>
        <i class="icon-{on ? 'circle-check' : 'circle-x'}"></i>
        <span class="min-w-0 flex-1 truncate" title={t(f.can)}>{t(f.can)}</span>
        <b>{on ? t('platform.ai.can') : t('platform.ai.cannot')}</b>
      </li>
    {/each}
    <li class="can" data-on={open > 0}>
      <i class="icon-{open > 0 ? 'circle-check' : 'circle-x'}"></i>
      <span class="min-w-0 flex-1 truncate">{t('platform.ai.cData', { count: open, total: domains.length })}</span>
    </li>
  </ul>
</aside>

<style>
  .meter { display: inline-flex; gap: 3px; }
  .meter i { width: 0.9rem; height: 0.35rem; border-radius: 2px; background: var(--border-default, #cbd5e1); }
  .meter[data-level='open'] i:nth-child(1) { background: var(--color-success-500); }
  .meter[data-level='balanced'] i:nth-child(-n + 2) { background: var(--color-warning-500); }
  .meter[data-level='strict'] i { background: var(--color-danger-500); }
  .can { display: flex; align-items: center; gap: 0.45rem; font-size: 12px; }
  .can :global(i) { flex: none; font-size: 14px; color: var(--color-danger-500); }
  .can[data-on='true'] :global(i) { color: var(--color-success-600); }
  .can[data-on='false'] { color: var(--text-tertiary); }
  .can b { flex: none; font: 700 9.5px var(--font-mono, ui-monospace, monospace); letter-spacing: 0.06em; text-transform: uppercase; color: var(--color-danger-600); }
  .can[data-on='true'] b { color: var(--color-success-600); }
</style>
