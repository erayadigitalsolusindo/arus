<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { session } from '#lib/auth/session.svelte.ts';

  let { children }: { children: Snippet } = $props();

  // Sesi berakhir (refresh gagal / logout dari tab lain): kembali ke login.
  $effect(() => {
    if (session.status === 'anon') {
      const to = untrack(() => session.returnTo) ?? '/login';
      untrack(() => (session.returnTo = null));
      void goto(to);
    }
  });
</script>

{@render children()}
