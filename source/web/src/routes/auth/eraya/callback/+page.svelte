<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, ApiError } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { ERAYA_FLOW_KEY, erayaRedirectUri, putErayaPending, type ErayaFlow } from '#lib/auth/eraya.ts';

  let error = $state('');

  // Eraya mengembalikan ?code&state (atau ?error). state harus sama dengan yang disimpan saat memulai; code ditukar lewat server.
  onMount(async () => {
    const q = new URL(location.href).searchParams;
    let flow: ErayaFlow | null = null;
    try {
      flow = JSON.parse(sessionStorage.getItem(ERAYA_FLOW_KEY) ?? 'null') as ErayaFlow | null;
      sessionStorage.removeItem(ERAYA_FLOW_KEY);
    } catch {
      /* sessionStorage diblokir */
    }
    const code = q.get('code');
    if (!flow || !code || q.get('state') !== flow.state) {
      error = errorMessage(new ApiError(401, 'ERAYA_INVALID_TOKEN', ''));
      return;
    }
    try {
      const r = await api<{ id_token: string }>('/auth/eraya/exchange', {
        method: 'POST',
        body: JSON.stringify({ code, code_verifier: flow.verifier, redirect_uri: erayaRedirectUri() })
      });
      putErayaPending({ token: r.id_token, mode: flow.mode });
      await goto(`/${flow.mode === 'register' ? 'register' : 'login'}${flow.next ? `?next=${encodeURIComponent(flow.next)}` : ''}`, { replaceState: true });
    } catch (err) {
      error = errorMessage(err);
    }
  });
</script>

<svelte:head><title>Eraya</title></svelte:head>

<div class="min-h-screen grid place-items-center bg-base px-6">
  <div class="text-center space-y-3 max-w-[360px]">
    {#if error}
      <div role="alert" class="rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">{error}</div>
      <a href="/login" class="font-semibold text-[12.5px] text-primary-600">{t('auth.eraya.backToLogin')}</a>
    {:else}
      <i class="icon-loader-circle animate-spin text-[22px] text-tertiary"></i>
      <p class="text-[12.5px] text-tertiary">{t('auth.eraya.working')}</p>
    {/if}
  </div>
</div>
