<script lang="ts">
  import type { Snippet } from 'svelte';
  import LoginBackdrop from '#lib/components/LoginBackdrop.svelte';
  import LoginFooter from '#lib/components/LoginFooter.svelte';

  // Kerangka halaman publik satu kolom (lupa/reset password, verifikasi email, syarat & privasi).
  let { wide = false, children }: { wide?: boolean; children: Snippet } = $props();
</script>

<div class="min-h-screen flex flex-col px-6 py-10 relative overflow-hidden bg-base">
  <LoginBackdrop />
  <div class="relative flex-1 flex flex-col justify-center w-full mx-auto {wide ? 'max-w-[760px]' : 'max-w-[420px]'}">
    <a href="/login" class="flex justify-center" aria-label="ACIRABA">
      <img src="/logo_dengan_text-no-bg.svg" alt="ACIRABA" class="h-16 w-auto" />
    </a>
    <div class="surface-card p-6 mt-6">
      {@render children()}
    </div>
  </div>
  <div class="relative w-full mx-auto {wide ? 'max-w-[760px]' : 'max-w-[420px]'}">
    <LoginFooter />
  </div>
</div>
