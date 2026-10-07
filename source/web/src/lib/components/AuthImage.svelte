<script lang="ts">
  // Gambar item yang butuh autentikasi: diunduh lewat fetch (header Authorization) lalu ditampilkan dari URL objek.
  import { imageUrl } from '#lib/items/api.ts';

  let {
    itemId,
    imageId,
    size = 'thumb',
    alt = '',
    class: cls = ''
  }: { itemId: string; imageId: string; size?: 'thumb' | 'full'; alt?: string; class?: string } = $props();

  let src = $state('');
  let failed = $state(false);

  $effect(() => {
    let live = true;
    src = '';
    failed = false;
    imageUrl(itemId, imageId, size).then(
      (u) => live && (src = u),
      () => live && (failed = true)
    );
    return () => (live = false);
  });
</script>

{#if src}
  <img {src} {alt} class={cls} loading="lazy" decoding="async" />
{:else}
  <span class="inline-flex items-center justify-center bg-[var(--surface-sunken)] text-[var(--text-tertiary)] {cls}" role="img" aria-label={alt}>
    <i class="{failed ? 'icon-image-off' : 'icon-image'} text-[16px]"></i>
  </span>
{/if}
