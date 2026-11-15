<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, CSRF_HEADERS } from '#lib/api/client.ts';
  import { accounting } from '#lib/accounting/api.ts';
  import { onboarding, type OnboardingStatus } from '#lib/onboarding/api.ts';
  import { session, can, markEmailVerified } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  const stepKeys = ['email', 'coa', 'ready', 'done'] as const;
  type Step = (typeof stepKeys)[number];

  let step = $state<Step>('email');
  let st = $state<OnboardingStatus | null>(null);
  let busy = $state(false);
  let error = $state('');
  let resend = $state<'idle' | 'sending' | 'sent'>('idle');

  const idx = $derived(stepKeys.indexOf(step));
  const email = $derived(session.user?.email ?? '');
  const canInstall = $derived(can('accounts', 'create'));

  async function refresh() {
    try {
      st = await onboarding.status();
      if (st.email_verified) markEmailVerified();
    } catch {
      /* jaringan putus: pertahankan status terakhir */
    }
  }

  onMount(() => {
    void refresh().then(() => {
      if (st?.completed) void goto('/dashboard');
      // Email sudah terverifikasi (daftar via Google/Eraya): lompat ke langkah berikutnya.
      else if (st?.email_verified) step = 'coa';
    });
    // Selama menunggu konfirmasi, cek berkala (tautan bisa dibuka di perangkat lain).
    const timer = setInterval(() => {
      if (step === 'email' && !st?.email_verified && document.visibilityState === 'visible') void refresh();
    }, 4000);
    return () => clearInterval(timer);
  });

  async function resendEmail() {
    resend = 'sending';
    error = '';
    try {
      await api('/auth/resend-verification', { method: 'POST', headers: CSRF_HEADERS });
      resend = 'sent';
    } catch (e) {
      error = errorMessage(e);
      resend = 'idle';
    }
  }

  async function install() {
    busy = true;
    error = '';
    try {
      await accounting.seedRetail();
    } catch {
      // Sudah terpasang (mis. dari tab lain) bukan kegagalan: status di bawah yang menentukan.
    }
    await refresh();
    if (!st?.account_count) error = t('onboarding.coa.failed');
    busy = false;
  }

  async function finish(skipped: boolean) {
    busy = true;
    error = '';
    try {
      await onboarding.complete(skipped);
      await goto('/dashboard');
    } catch {
      error = t('onboarding.done.failed');
      busy = false;
    }
  }

  function go(to: Step) {
    error = '';
    step = to;
    if (to === 'ready' || to === 'done') void refresh();
  }

  const ok = $derived({
    outlet: (st?.outlet_count ?? 0) > 0,
    coa: (st?.account_count ?? 0) > 0,
    methods: (st?.account_count ?? 0) > 0 && (st?.methods_unlinked ?? 0) === 0,
    items: (st?.item_count ?? 0) > 0
  });
</script>

<svelte:head><title>{t('onboarding.docTitle')}</title></svelte:head>

<AuthCard wide>
  <div class="text-center">
    <h1 class="font-display font-bold text-[18px]">{t('onboarding.title')}</h1>
    <p class="text-[12.5px] mt-1 text-tertiary">{t('onboarding.subtitle')}</p>
  </div>

  <ol class="flex items-center gap-2 mt-5" aria-label={t('onboarding.stepOf', { n: idx + 1, total: stepKeys.length })}>
    {#each stepKeys as k, i (k)}
      <li class="flex-1 min-w-0" aria-current={i === idx ? 'step' : undefined}>
        <div class="h-1.5 rounded-full {i <= idx ? 'bg-[var(--color-primary-600)]' : 'bg-[var(--border-subtle,rgba(128,128,128,.25))]'}"></div>
        <div class="mt-1 text-[11px] truncate {i === idx ? 'font-semibold' : 'text-tertiary'}">{t(`onboarding.steps.${k}`)}</div>
      </li>
    {/each}
  </ol>

  <div class="mt-6 min-h-[220px]" aria-live="polite">
    {#if step === 'email'}
      <h2 class="font-semibold text-[14px]">{t('onboarding.email.title')}</h2>
      {#if st?.email_verified}
        <p class="mt-3 flex items-center gap-2 text-[12.5px]"><i class="icon-circle-check text-[18px]"></i>{t('onboarding.email.verified', { email })}</p>
      {:else}
        <p class="text-[12.5px] mt-2 text-tertiary">{t('onboarding.email.text', { email })}</p>
        <p class="mt-3 flex items-center gap-2 text-[12.5px] text-tertiary"><i class="icon-loader-circle animate-spin"></i>{t('onboarding.email.waiting')}</p>
        <div class="mt-4 flex flex-wrap items-center gap-3">
          <button type="button" class="btn btn-outline !text-[12.5px]" disabled={resend !== 'idle'} onclick={resendEmail}>
            {resend === 'sending' ? t('onboarding.email.sending') : t('onboarding.email.resend')}
          </button>
          {#if resend === 'sent'}<span class="text-[12.5px]">{t('onboarding.email.sent')}</span>{/if}
        </div>
      {/if}
    {:else if step === 'coa'}
      <h2 class="font-semibold text-[14px]">{t('onboarding.coa.title')}</h2>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('onboarding.coa.text')}</p>
      <ul class="mt-3 space-y-1.5 text-[12.5px]">
        {#each ['point1', 'point2', 'point3'] as const as p (p)}
          <li class="flex items-start gap-2"><i class="icon-check mt-0.5"></i>{t(`onboarding.coa.${p}`)}</li>
        {/each}
      </ul>
      <div class="mt-4">
        {#if ok.coa}
          <p class="flex items-center gap-2 text-[12.5px] badge-success rounded-lg px-3 py-2"><i class="icon-circle-check text-[16px]"></i>{t('onboarding.coa.installed', { count: st?.account_count ?? 0 })}</p>
        {:else if canInstall}
          <button type="button" class="btn btn-primary !text-[13px]" disabled={busy} onclick={install}>
            <i class="icon-wand-sparkles"></i>{busy ? t('onboarding.coa.installing') : t('onboarding.coa.install')}
          </button>
        {:else}
          <p class="text-[12.5px] text-tertiary">{t('onboarding.coa.noPermission')}</p>
        {/if}
      </div>
    {:else if step === 'ready'}
      <h2 class="font-semibold text-[14px]">{t('onboarding.ready.title')}</h2>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('onboarding.ready.text')}</p>
      <ul class="mt-3 space-y-2 text-[12.5px]">
        {#each [['outlet', ok.outlet], ['coa', ok.coa], ['methods', ok.methods], ['items', ok.items]] as const as [key, done] (key)}
          <li class="flex items-start gap-2">
            <i class="mt-0.5 {done ? 'icon-circle-check' : 'icon-circle-alert'}"></i>
            <div class="flex-1">
              <div>{t(`onboarding.ready.${key}`)}</div>
              {#if !done && key === 'methods' && ok.coa}
                <div class="text-tertiary">{t('onboarding.ready.methodsFix', { count: st?.methods_unlinked ?? 0 })} <a class="underline" href="/payment-methods">{t('onboarding.ready.fixMethods')}</a></div>
              {:else if !done && key === 'items'}
                <div class="text-tertiary">{t('onboarding.ready.itemsHint')} <a class="underline" href="/items">{t('onboarding.ready.addItems')}</a></div>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="text-center py-4">
        <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-success"><i class="icon-party-popper text-[22px]"></i></span>
        <h2 class="font-display font-bold text-[16px]">{t('onboarding.done.title')}</h2>
        <p class="text-[12.5px] mt-2 text-tertiary">{t('onboarding.done.text')}</p>
        <button type="button" class="btn btn-primary !text-[13px] mt-5" disabled={busy} onclick={() => finish(false)}>
          {busy ? t('onboarding.done.saving') : t('onboarding.done.start')}
        </button>
      </div>
    {/if}
    {#if error}<p role="alert" class="text-[12.5px] mt-3 badge-danger rounded-lg px-3 py-2">{error}</p>{/if}
  </div>

  <div class="mt-4 pt-4 flex items-center justify-between gap-3 border-t border-[var(--border-subtle,rgba(128,128,128,.25))]">
    <button type="button" class="text-[12px] text-tertiary underline disabled:opacity-60" disabled={busy} onclick={() => finish(true)}>{t('onboarding.skip')}</button>
    <div class="flex items-center gap-2">
      {#if idx > 0 && step !== 'done'}
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => go(stepKeys[idx - 1])}>{t('onboarding.back')}</button>
      {/if}
      {#if step === 'email'}
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => go('coa')}>{st?.email_verified ? t('onboarding.next') : t('onboarding.email.later')}</button>
      {:else if step !== 'done'}
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => go(stepKeys[idx + 1])}>{t('onboarding.next')}</button>
      {/if}
    </div>
  </div>
</AuthCard>
