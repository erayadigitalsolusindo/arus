<script lang="ts">
  import ItemForm from '#lib/components/ItemForm.svelte';
  import PriceHistory from '#lib/components/PriceHistory.svelte';
  import { can } from '#lib/auth/session.svelte.ts';
  import { page } from '$app/state';
  import { t } from '#lib/i18n/index.ts';

  let { data } = $props();
  // ?notice=images_failed: item baru tersimpan tetapi sebagian gambarnya gagal diunggah.
  const imagesFailed = $derived(page.url.searchParams.get('notice') === 'images_failed');
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
  {#if imagesFailed}
    <div role="alert" class="mb-3 flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-warning">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('items.images.someFailed')}</span>
    </div>
  {/if}
  <h1 class="font-display font-bold text-[19px]">{data.item.name}</h1>
  <p class="text-[12px] mt-0.5 mb-4 font-mono text-[var(--text-tertiary)]">{data.item.sku}</p>
  <!-- {#key}: berpindah ke item lain memasang ulang form dengan nilai awal yang baru. -->
  {#key data.item.id}
    <ItemForm item={data.item} outlets={[]} />
  {/key}
  <div class="mt-4">
    <PriceHistory itemId={data.item.id} outlets={data.item.outlet_prices.map((p) => ({ id: p.outlet_id, name: p.outlet_name }))} />
  </div>
</main>
