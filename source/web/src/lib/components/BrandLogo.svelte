<script lang="ts">
  import { brand } from '#lib/brand.svelte.ts';

  // Logo aplikasi: ARUS di host utama; di domain kustom memakai logo/ikon domain itu, atau nama saja bila tidak ada gambar.
  let { kind = 'full', class: cls = '' }: { kind?: 'full' | 'icon'; class?: string } = $props();
  const src = $derived(kind === 'icon' ? brand.icon : brand.logo);
</script>

{#if src}
  <img {src} alt={brand.name} class={cls} />
{:else if brand.ready}
  <span class="font-display font-bold tracking-tight leading-none {cls}" style="height:auto;width:auto;display:inline-block">{kind === 'icon' ? brand.name.slice(0, 1).toUpperCase() : brand.name}</span>
{/if}
