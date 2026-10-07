<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { session } from '#lib/auth/session.svelte.ts';
  import Sidebar from '#lib/components/Sidebar.svelte';
  import Header from '#lib/components/Header.svelte';
  import TabBar from '#lib/components/TabBar.svelte';
  import UnsavedDialog from '#lib/components/UnsavedDialog.svelte';
  import { tabs } from '#lib/tabs/store.svelte.ts';
  import VerifyBanner from '#lib/components/VerifyBanner.svelte';
  import ImpersonationBanner from '#lib/components/ImpersonationBanner.svelte';

  let { children }: { children: Snippet } = $props();

  let collapsed = $state(false);
  let mobileOpen = $state(false);

  afterNavigate(() => (mobileOpen = false));

  // Sesi berakhir (refresh gagal, atau logout dari tab lain): kembali ke login.
  $effect(() => {
    if (session.status === 'anon') {
      untrack(() => tabs.reset());
      // untrack: returnTo hanya dibaca/dihapus di sini; jika dilacak, efek berjalan ulang dan jatuh ke /login.
      const to = untrack(() => session.returnTo) ?? '/login';
      untrack(() => (session.returnTo = null));
      void goto(to);
    }
  });
</script>

<div class="app-shell" class:is-collapsed={collapsed}>
  <Sidebar {collapsed} {mobileOpen} ontoggle={() => (collapsed = !collapsed)} onclose={() => (mobileOpen = false)} />
  <div class="app-main">
    <ImpersonationBanner />
    <Header onmenu={() => (mobileOpen = true)} />
    <TabBar />
    <UnsavedDialog />
    <VerifyBanner />
    {@render children()}
  </div>
</div>
