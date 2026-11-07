<script lang="ts">
  import '../app.css';
  import { onMount, type Snippet } from 'svelte';
  import { initBrand } from '#lib/brand.svelte.ts';

  let { children }: { children: Snippet } = $props();
  onMount(() => void initBrand());
</script>

{@render children()}
