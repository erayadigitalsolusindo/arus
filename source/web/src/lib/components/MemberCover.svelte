<script lang="ts">
  // Foto cover member yang butuh autentikasi (diunduh lewat fetch lalu ditampilkan dari URL objek).
  // Tanpa foto: lingkaran/blok berisi inisial nama.
  import { coverUrl } from '#lib/members/api.ts';
  import { initials } from '#lib/auth/initials.ts';

  let {
    memberId,
    coverId,
    name,
    size = 'thumb',
    alt = '',
    class: cls = ''
  }: { memberId: string; coverId: string | null; name: string; size?: 'thumb' | 'full'; alt?: string; class?: string } = $props();

  let src = $state('');

  $effect(() => {
    let live = true;
    src = '';
    if (coverId) coverUrl(memberId, coverId, size).then((u) => live && (src = u), () => {});
    return () => (live = false);
  });
</script>

{#if src}
  <img {src} {alt} class={cls} loading="lazy" decoding="async" />
{:else}
  <span class="inline-flex items-center justify-center bg-[color-mix(in_oklab,var(--color-primary-600)_14%,transparent)] font-display font-bold text-[var(--color-primary-600)] {cls}" role="img" aria-label={alt || name}>{initials(name)}</span>
{/if}
