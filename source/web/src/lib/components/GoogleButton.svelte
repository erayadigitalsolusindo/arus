<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { i18n, t } from '#lib/i18n/index.ts';
  import { googleClientId, loadGoogle } from '#lib/auth/google.ts';

  let { text = 'signin_with', onCredential }: { text?: 'signin_with' | 'signup_with'; onCredential: (credential: string) => void } = $props();

  let host: HTMLDivElement;
  let failed = $state(false);
  let ready = $state(false);

  onMount(() => {
    loadGoogle()
      .then((gid) => {
        gid.initialize({ client_id: get(googleClientId), callback: (r) => r.credential && onCredential(r.credential) });
        ready = true;
      })
      .catch(() => (failed = true));
  });

  // Teks tombol dibuat Google; `locale` mengikuti bahasa aplikasi dan digambar ulang saat bahasa diganti.
  $effect(() => {
    const locale = i18n.locale;
    if (!ready || !window.google) return;
    const dark = document.documentElement.dataset.theme === 'dark';
    host.replaceChildren();
    window.google.accounts.id.renderButton(host, { type: 'standard', theme: dark ? 'filled_black' : 'outline', size: 'large', text, shape: 'rectangular', logo_alignment: 'center', locale, width: Math.min(400, Math.max(200, host.clientWidth)) });
  });
</script>

<div bind:this={host} class="flex justify-center min-h-10 w-full"></div>
{#if failed}<p class="text-[11.5px] mt-1 text-center text-[var(--color-danger-600)]">{t('auth.google.loadFailed')}</p>{/if}
