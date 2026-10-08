<script lang="ts">
  // Kolom teks markdown dengan tab Tulis/Pratinjau. Kanonik = string markdown mentah (disaring server).
  import { t } from '#lib/i18n/index.ts';
  import MarkdownView from './MarkdownView.svelte';

  let {
    value = $bindable(''),
    id,
    rows = 4,
    maxlength = 1000,
    disabled = false,
    invalid = false,
    placeholder = ''
  }: { value?: string; id?: string; rows?: number; maxlength?: number; disabled?: boolean; invalid?: boolean; placeholder?: string } = $props();

  let tab = $state<'write' | 'preview'>('write');
  const tabClass = (on: boolean) => `px-2.5 py-1 rounded ${on ? 'bg-[var(--surface-sunken)] font-semibold' : 'text-[var(--text-tertiary)]'}`;
</script>

<div class="space-y-1.5">
  <div class="flex items-center justify-between gap-2">
    <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('common.markdown.hint')}</p>
    <div class="inline-flex shrink-0 rounded-md border border-[var(--border-subtle)] p-0.5 text-[12px]" role="tablist">
      <button type="button" role="tab" aria-selected={tab === 'write'} class={tabClass(tab === 'write')} onclick={() => (tab = 'write')}>{t('common.markdown.write')}</button>
      <button type="button" role="tab" aria-selected={tab === 'preview'} class={tabClass(tab === 'preview')} onclick={() => (tab = 'preview')}>{t('common.markdown.preview')}</button>
    </div>
  </div>
  {#if tab === 'write'}
    <textarea {id} {rows} {maxlength} {disabled} {placeholder} class="w-full field-control font-mono resize-y {invalid ? '!border-[var(--color-danger-600)]' : ''}" aria-invalid={invalid} bind:value></textarea>
    <p class="text-end text-[11px] text-[var(--text-tertiary)]">{value.length} / {maxlength}</p>
  {:else}
    <div class="field-control min-h-[6rem] !h-auto">
      {#if value.trim()}
        <MarkdownView source={value} />
      {:else}
        <span class="text-[var(--text-tertiary)]">{t('common.markdown.empty')}</span>
      {/if}
    </div>
  {/if}
</div>
