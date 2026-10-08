<script lang="ts">
  import { LOCALES, i18n, setLocale, t, type Locale } from '#lib/i18n/index.ts';

  let { placement = 'down', variant = 'text' }: { placement?: 'down' | 'up'; variant?: 'text' | 'circle' } = $props();

  let open = $state(false);

  function choose(code: Locale) {
    setLocale(code);
    open = false;
  }
</script>

<svelte:window
  onclick={() => (open = false)}
  onkeydown={(e) => {
    if (e.key === 'Escape') open = false;
  }}
/>

<div class="relative">
  <button
    type="button"
    class={variant === 'circle'
      ? 'grid place-items-center size-9 rounded-full transition-transform hover:scale-105 bg-[color-mix(in_oklab,var(--color-primary-500)_16%,transparent)] text-[var(--color-primary-600)]'
      : 'inline-flex items-center gap-1.5 h-8 px-2 rounded-full text-[12px] font-semibold hover:text-[var(--color-primary-600)]'}
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={t('common.language')}
    title={variant === 'circle' ? `${t('common.language')}: ${LOCALES[i18n.locale].short}` : undefined}
    onclick={(e) => {
      e.stopPropagation();
      open = !open;
    }}
  >
    <i class="icon-languages {variant === 'circle' ? 'text-[16px]' : 'text-[14px]'}"></i>
    {#if variant === 'text'}<span>{LOCALES[i18n.locale].short}</span>{/if}
  </button>

  {#if open}
    <div class="absolute end-0 w-44 surface-card p-1.5 z-50 {placement === 'up' ? 'bottom-full mb-2' : 'mt-2'}" role="menu">
      {#each i18n.locales as code (code)}
        <button
          type="button"
          class="w-full flex items-center justify-between gap-2 px-2.5 py-2 rounded-lg text-[13px] font-medium text-start hover:bg-[var(--surface-sunken)]"
          role="menuitemradio"
          aria-checked={code === i18n.locale}
          onclick={() => choose(code)}
        >
          <span>{LOCALES[code].name}</span>
          {#if code === i18n.locale}<i class="icon-check text-[13px] text-[var(--color-primary-600)]"></i>{/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
