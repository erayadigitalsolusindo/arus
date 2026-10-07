<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api/client.ts';
  import { session, markEmailVerified, bootstrap } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  type Phase = 'working' | 'ok' | 'failed';
  let phase = $state<Phase>('working');

  onMount(async () => {
    // Token di fragmen URL (#token=...): tidak terkirim ke server lewat navigasi/Referer. Dibuang dari bilah alamat.
    const token = new URLSearchParams(location.hash.slice(1)).get('token') ?? '';
    if (token) history.replaceState(null, '', location.pathname);
    if (!token) {
      phase = 'failed';
      return;
    }
    try {
      await api('/auth/verify-email', { method: 'POST', body: JSON.stringify({ token }) });
      await bootstrap(); // tahu apakah pengguna sedang masuk (untuk tautan tujuan di bawah)
      if (session.status === 'authed') markEmailVerified();
      phase = 'ok';
    } catch {
      phase = 'failed';
    }
  });
</script>

<svelte:head><title>{t('account.verify.docTitle')}</title></svelte:head>

<AuthCard>
  <div class="text-center" aria-live="polite">
    {#if phase === 'working'}
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-info"><i class="icon-loader-circle animate-spin text-[22px]"></i></span>
      <p class="text-[12.5px] text-tertiary">{t('account.verify.working')}</p>
    {:else if phase === 'ok'}
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-success"><i class="icon-circle-check text-[22px]"></i></span>
      <h1 class="font-display font-bold text-[18px]">{t('account.verify.successTitle')}</h1>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('account.verify.success')}</p>
      <a href={session.status === 'authed' ? '/dashboard' : '/login'} class="btn btn-primary w-full justify-center !text-[13px] mt-5">
        {session.status === 'authed' ? t('account.verify.toDashboard') : t('account.verify.toLogin')}
      </a>
    {:else}
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-danger"><i class="icon-link-2-off text-[22px]"></i></span>
      <h1 class="font-display font-bold text-[18px]">{t('account.verify.failedTitle')}</h1>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('account.verify.failed')}</p>
      <a href="/login" class="btn btn-outline w-full justify-center !text-[12.5px] mt-5">{t('account.verify.toLogin')}</a>
    {/if}
  </div>
</AuthCard>
