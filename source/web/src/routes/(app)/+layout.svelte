<script lang="ts">
  import type { Snippet } from 'svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { session } from '#lib/auth/session.svelte.ts';
  import Sidebar from '#lib/components/Sidebar.svelte';
  import Header from '#lib/components/Header.svelte';
  import VerifyBanner from '#lib/components/VerifyBanner.svelte';

  let { children }: { children: Snippet } = $props();

  let collapsed = $state(false);
  let mobileOpen = $state(false);

  afterNavigate(() => (mobileOpen = false));

  // Sesi berakhir (refresh gagal, atau logout dari tab lain): kembali ke login.
  $effect(() => {
    if (session.status === 'anon') void goto('/login');
  });
</script>

<div class="app-shell" class:is-collapsed={collapsed}>
  <Sidebar {collapsed} {mobileOpen} ontoggle={() => (collapsed = !collapsed)} onclose={() => (mobileOpen = false)} />
  <div class="app-main">
    <Header onmenu={() => (mobileOpen = true)} />
    <VerifyBanner />
    {@render children()}
  </div>
</div>
