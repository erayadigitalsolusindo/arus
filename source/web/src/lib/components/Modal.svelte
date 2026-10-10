<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from '#lib/i18n/index.ts';

  let {
    title,
    onclose,
    wide = false,
    xl = false,
    children
  }: { title: string; onclose: () => void; wide?: boolean; xl?: boolean; children: Snippet } = $props();

  let dialog = $state<HTMLElement>();

  // Fokus ke dialog saat dibuka; Escape menutup; fokus dikembalikan ke pemicu saat ditutup.
  $effect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog?.focus();
    return () => previous?.focus?.();
  });

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !e.defaultPrevented) onclose();
  }
</script>

<svelte:window {onkeydown} />

<!-- Klik latar menutup; klik di dalam dialog tidak. -->
<div class="ui-modal is-open" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div
    bind:this={dialog}
    class="ui-modal-dialog p-5 max-h-[92vh] overflow-y-auto scroll-thin {xl ? '!max-w-[1040px]' : wide ? '!max-w-[860px]' : '!max-w-[460px]'}"
    role="dialog"
    aria-modal="true"
    aria-label={title}
    tabindex="-1"
  >
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-display font-bold text-[15px]">{title}</h2>
      <button type="button" class="header-icon-btn" onclick={onclose} aria-label={t('common.close')}><i class="icon-x text-[16px]"></i></button>
    </div>
    {@render children()}
  </div>
</div>
