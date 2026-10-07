<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, request } from '#lib/api/client.ts';
  import { startPlatformSession, type PlatformAuth } from '#lib/platform/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  let email = $state('');
  let password = $state('');
  let remember = $state(false);
  let loading = $state(false);
  let error = $state('');
  // Langkah 2 (2FA): token tantangan dari server setelah password benar.
  let mfaToken = $state('');
  let code = $state('');
  let lockedUntil = $state(0);
  let now = $state(Date.now());
  const lockedSeconds = $derived(Math.max(0, Math.ceil((lockedUntil - now) / 1000)));

  $effect(() => {
    if (lockedUntil <= Date.now()) return;
    const id = setInterval(() => {
      now = Date.now();
      if (now >= lockedUntil) clearInterval(id);
    }, 1000);
    return () => clearInterval(id);
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (mfaToken) return void submitCode();
    if (!email.trim()) return void (error = t('platform.login.emailRequired'));
    if (!password) return void (error = t('platform.login.passwordRequired'));
    loading = true;
    try {
      const res = await request<PlatformAuth & { mfa_required?: boolean; mfa_token?: string }>(
        '/platform/auth/login',
        { method: 'POST', body: JSON.stringify({ email: email.trim(), password, remember }) },
        null
      );
      password = '';
      if (res.mfa_required && res.mfa_token) {
        mfaToken = res.mfa_token;
        return;
      }
      startPlatformSession(res);
      await goto(res.admin.mfa_enabled ? '/platform/tenants' : '/platform/security');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'ACCOUNT_LOCKED' && err.retryAfter > 0) {
        now = Date.now();
        lockedUntil = now + err.retryAfter * 1000;
        return;
      }
      error = err instanceof ApiError && err.code === 'INVALID_CREDENTIALS' && err.attemptsLeft > 0 ? t('errors.INVALID_CREDENTIALS_LEFT', { count: err.attemptsLeft }) : errorMessage(err);
    } finally {
      loading = false;
    }
  }
  async function submitCode() {
    loading = true;
    try {
      const res = await request<PlatformAuth>('/platform/auth/login/mfa', { method: 'POST', body: JSON.stringify({ mfa_token: mfaToken, code: code.trim() }) }, null);
      startPlatformSession(res);
      code = '';
      await goto('/platform/tenants');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'ACCOUNT_LOCKED' && err.retryAfter > 0) {
        now = Date.now();
        lockedUntil = now + err.retryAfter * 1000;
        return;
      }
      if (err instanceof ApiError && err.code === 'MFA_TOKEN_INVALID') mfaToken = ''; // tantangan habis: ulangi dari password
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  function backToPassword() {
    mfaToken = code = error = '';
  }
</script>

<svelte:head><title>{t('platform.login.title')} | ACIRABA</title></svelte:head>

<AuthCard>
  <h1 class="font-display font-bold text-[18px]">{t('platform.login.title')}</h1>
  <p class="text-[12.5px] mt-1 mb-4 text-tertiary">{t('platform.login.subtitle')}</p>

  <form class="space-y-4" onsubmit={submit} novalidate>
    {#if lockedSeconds > 0 || error}
      <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
        <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i>
        <span>{lockedSeconds > 0 ? t('errors.ACCOUNT_LOCKED', { count: Math.ceil(lockedSeconds / 60) }) : error}</span>
      </div>
    {/if}
    {#if mfaToken}
      <p class="text-[12.5px] text-tertiary">{t('platform.login.mfaPrompt')}</p>
      <div>
        <label for="pcode" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('platform.login.code')}</label>
        <input id="pcode" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="16" class="mt-1.5 w-full px-3 py-2.5 rounded-lg text-[15px] tracking-[0.25em] font-mono outline-none bg-sunken-bordered" />
        <p class="text-[11.5px] mt-1 text-tertiary">{t('platform.login.codeHelp')}</p>
      </div>
      <button type="submit" disabled={loading || lockedSeconds > 0 || !code.trim()} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
        {t('platform.login.verify')}<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
      </button>
      <button type="button" class="w-full text-[12px] font-semibold text-primary-600" onclick={backToPassword}>{t('platform.login.back')}</button>
    {:else}
    <div>
      <label for="pemail" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('platform.login.email')}</label>
      <input id="pemail" type="email" bind:value={email} autocomplete="username" class="mt-1.5 w-full px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" />
    </div>
    <div>
      <label for="ppass" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('platform.login.password')}</label>
      <input id="ppass" type="password" bind:value={password} autocomplete="current-password" class="mt-1.5 w-full px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" />
    </div>
    <label class="flex items-center gap-2 text-[12px] font-medium">
      <input type="checkbox" class="size-3.5 rounded" bind:checked={remember} />
      {t('platform.login.remember')}
    </label>
    <button type="submit" disabled={loading || lockedSeconds > 0} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
      {t('platform.login.submit')}<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
    </button>
    {/if}
  </form>
</AuthCard>
