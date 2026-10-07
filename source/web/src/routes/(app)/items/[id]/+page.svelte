<script lang="ts">
  import ItemForm from '#lib/components/ItemForm.svelte';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';

  let { data } = $props();
  const title = $derived(can('items', 'update') ? t('items.editTitle') : t('items.view'));
</script>

<svelte:head><title>{title} | ACIRABA</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{title}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <a href="/items" class="text-[var(--color-primary-600)]">{t('items.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{data.item.sku}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 max-w-full mx-auto w-full">
  <h1 class="font-display font-bold text-[19px]">{data.item.name}</h1>
  <p class="text-[12px] mt-0.5 mb-4 font-mono text-[var(--text-tertiary)]">{data.item.sku}</p>
  <!-- {#key}: berpindah ke item lain memasang ulang form dengan nilai awal yang baru. -->
  {#key data.item.id}
    <ItemForm item={data.item} outlets={[]} />
  {/key}
</main>
