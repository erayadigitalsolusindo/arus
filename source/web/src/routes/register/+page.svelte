<script lang="ts">
  import { goto } from '$app/navigation';
  import { api, ApiError, type AuthResponse } from '#lib/api/client.ts';
  import { startSession } from '#lib/auth/session.svelte.ts';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName, checkEmail, checkPhone, checkPassword, passwordStrength } from '#lib/validation.ts';
  import LoginBackdrop from '#lib/components/LoginBackdrop.svelte';
  import LoginFooter from '#lib/components/LoginFooter.svelte';
  import GoogleButton from '#lib/components/GoogleButton.svelte';
  import { onMount } from 'svelte';
  import { decodeCredential, googleEnabled, initGoogleForHost, PENDING_KEY, type GoogleProfile } from '#lib/auth/google.ts';

  type Field = 'business_name' | 'owner_name' | 'outlet_name' | 'email' | 'phone' | 'password' | 'confirm' | 'accept_terms';

  const steps: MessageKey[] = ['auth.register.step1', 'auth.register.step2', 'auth.register.step3', 'auth.register.step4'];

  let businessName = $state('');
  let ownerName = $state('');
  let outletName = $state('');
  let email = $state('');
  let phone = $state('');
  let password = $state('');
  let confirm = $state('');
  let showPassword = $state(false);
  let acceptTerms = $state(false);
  let loading = $state(false);
  let formError = $state('');
  let errors = $state<Partial<Record<Field, string>>>({});

  // Mode Google: nama & email dari akun Google (diverifikasi server), tanpa kata sandi.
  let googleToken = $state('');
  let googleProfile = $state<GoogleProfile | null>(null);
  let fromLogin = $state(false);

  function useGoogle(credential: string) {
    const p = decodeCredential(credential);
    if (!p) {
      formError = errorMessage(new ApiError(401, 'GOOGLE_INVALID_TOKEN', ''));
      return;
    }
    googleToken = credential;
    googleProfile = p;
    errors = {};
    formError = '';
  }

  function leaveGoogle() {
    googleToken = '';
    googleProfile = null;
    fromLogin = false;
    try {
      sessionStorage.removeItem(PENDING_KEY);
    } catch {
      /* abaikan */
    }
  }

  onMount(async () => {
    if (!(await initGoogleForHost())) return;
    try {
      const pending = sessionStorage.getItem(PENDING_KEY);
      if (pending) {
        useGoogle(pending);
        fromLogin = googleProfile !== null;
      }
    } catch {
      /* sessionStorage diblokir */
    }
  });

  const strength = $derived(passwordStrength(password));
  const strengthLabel = $derived(
    password === '' ? '' : t(`auth.register.strength.${(['weak', 'fair', 'strong'] as const)[strength]}`)
  );

  const inputClass = 'w-full px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered';

  async function submitGoogle() {
    const biz = checkName(businessName);
    const outlet = checkName(outletName);
    const tel = checkPhone(phone);
    const next: Partial<Record<Field, string>> = {};
    if (biz.code) next.business_name = fieldMessage(biz.code);
    if (outlet.code) next.outlet_name = fieldMessage(outlet.code);
    if (tel.code) next.phone = fieldMessage(tel.code);
    if (!acceptTerms) next.accept_terms = t('account.terms.required');
    if (Object.keys(next).length) {
      errors = next;
      return;
    }
    loading = true;
    try {
      const res = await api<AuthResponse>('/auth/google/register', {
        method: 'POST',
        body: JSON.stringify({ id_token: googleToken, business_name: biz.value, outlet_name: outlet.value, phone: tel.value, accept_terms: acceptTerms })
      });
      leaveGoogle();
      startSession(res);
      await goto('/dashboard');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'EMAIL_TAKEN') {
        formError = errorMessage(err);
      } else if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, code] of Object.entries(err.fields)) errors[k as Field] = fieldMessage(code);
        if (!Object.keys(errors).length) formError = errorMessage(err);
      } else {
        if (err instanceof ApiError && err.code === 'GOOGLE_INVALID_TOKEN') leaveGoogle();
        formError = errorMessage(err);
      }
    } finally {
      loading = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    formError = '';
    errors = {};

    if (googleToken) return submitGoogle();
    const biz = checkName(businessName);
    const owner = checkName(ownerName);
    const outlet = checkName(outletName);
    const mail = checkEmail(email);
    const tel = checkPhone(phone);
    const pwCode = checkPassword(password, mail.value);

    const next: Partial<Record<Field, string>> = {};
    if (biz.code) next.business_name = fieldMessage(biz.code);
    if (owner.code) next.owner_name = fieldMessage(owner.code);
    if (outlet.code) next.outlet_name = fieldMessage(outlet.code);
    if (mail.code) next.email = fieldMessage(mail.code);
    if (tel.code) next.phone = fieldMessage(tel.code);
    if (pwCode) next.password = fieldMessage(pwCode);
    else if (confirm !== password) next.confirm = t('auth.register.mismatch');
    if (!acceptTerms) next.accept_terms = t('account.terms.required');
    if (Object.keys(next).length) {
      errors = next;
      return;
    }

    loading = true;
    try {
      const res = await api<AuthResponse>('/auth/register', {
        method: 'POST',
        body: JSON.stringify({
          business_name: biz.value,
          owner_name: owner.value,
          outlet_name: outlet.value,
          email: mail.value,
          phone: tel.value,
          password,
          accept_terms: acceptTerms
        })
      });
      startSession(res);
      password = confirm = '';
      await goto('/dashboard');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'EMAIL_TAKEN') {
        errors = { email: errorMessage(err) };
      } else if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, code] of Object.entries(err.fields)) errors[k as Field] = fieldMessage(code);
        if (!Object.keys(errors).length) formError = errorMessage(err);
      } else {
        formError = errorMessage(err);
      }
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>{t('auth.register.docTitle')}</title></svelte:head>

{#snippet textField(id: Field, label: string, value: string, set: (v: string) => void, opts: { type?: string; placeholder?: string; autocomplete: AutoFill; icon?: string; max: number })}
  <div>
    <label for={id} class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">{label}</label>
    <div class="relative mt-1.5">
      {#if opts.icon}<i class="{opts.icon} absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>{/if}
      <input
        {id}
        type={opts.type ?? 'text'}
        {value}
        oninput={(e) => set(e.currentTarget.value)}
        placeholder={opts.placeholder}
        autocomplete={opts.autocomplete}
        maxlength={opts.max}
        aria-invalid={!!errors[id]}
        aria-describedby={errors[id] ? `${id}-err` : undefined}
        class="{inputClass} {opts.icon ? 'ps-9' : ''}"
      />
    </div>
    {#if errors[id]}<p id="{id}-err" class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{errors[id]}</p>{/if}
  </div>
{/snippet}

<div class="min-h-screen grid lg:grid-cols-2 bg-base">
  <!-- Left: brand panel -->
  <div class="hidden lg:flex relative items-center justify-center p-12 overflow-hidden u-background-linear-gradient-150deg-color-primary-700-color-pr">
    <div class="absolute inset-0 opacity-10 u-background-image-radial-gradient-circle-at-20-20-white-1px-t"></div>
    <div class="relative max-w-[420px] w-full">
      <h2 class="font-display font-bold text-[24px] text-white">{t('auth.register.headline')}</h2>
      <p class="text-[12px] font-semibold uppercase tracking-wide mt-6 u-color-rgb-255-255-255-0-7">{t('auth.register.stepsTitle')}</p>
      <ol class="space-y-3 mt-3">
        {#each steps as key, i (key)}
          <li class="surface-card !bg-white/95 p-4 shadow-xl flex items-center gap-3">
            <span class="grid place-items-center size-8 rounded-full shrink-0 bg-primary-soft font-display font-bold text-[13px]">{i + 1}</span>
            <span class="text-[12.5px] font-semibold">{t(key)}</span>
          </li>
        {/each}
      </ol>
    </div>
  </div>

  <!-- Right: form -->
  <div class="relative flex flex-col px-6 sm:px-10 lg:px-16 py-10 overflow-hidden">
    <LoginBackdrop />
    <div class="relative flex-1 flex flex-col justify-center w-full max-w-[420px] mx-auto">
      <h1 class="font-display font-bold text-[22px]">{t('auth.register.title')}</h1>
      <p class="text-[12.5px] mt-1.5 text-tertiary">{t('auth.register.subtitle')}</p>

      {#if $googleEnabled && !googleToken}
        <div class="mt-6">
          <GoogleButton text="signup_with" onCredential={useGoogle} />
        </div>
        <div class="flex items-center gap-3 mt-6">
          <span class="flex-1 h-px bg-border-subtle"></span>
          <span class="text-[11px] font-medium text-tertiary">{t('auth.google.orManual')}</span>
          <span class="flex-1 h-px bg-border-subtle"></span>
        </div>
      {/if}

      {#if googleProfile}
        <div class="mt-6 flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] bg-sunken-bordered">
          <i class="fa-brands fa-google text-[14px] mt-px shrink-0"></i>
          <div class="flex-1">
            <p>{fromLogin ? t('auth.google.notRegistered') : t('auth.google.signupHint', { email: googleProfile.email })}</p>
            <button type="button" class="font-semibold text-primary-600 mt-1" onclick={leaveGoogle}>{t('auth.google.useEmail')}</button>
          </div>
        </div>
      {/if}

      <form class="space-y-4 mt-6" onsubmit={submit} novalidate>
        {#if formError}
          <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
            <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i>
            <span>{formError}</span>
          </div>
        {/if}

        {@render textField('business_name', t('auth.register.businessName'), businessName, (v) => (businessName = v), { placeholder: t('auth.register.businessNamePlaceholder'), autocomplete: 'organization', icon: 'icon-store', max: 100 })}
        {@render textField('outlet_name', t('auth.register.outletName'), outletName, (v) => (outletName = v), { placeholder: t('auth.register.outletNamePlaceholder'), autocomplete: 'off', icon: 'icon-map-pin', max: 100 })}
        {#if !googleToken}
        {@render textField('owner_name', t('auth.register.ownerName'), ownerName, (v) => (ownerName = v), { placeholder: t('auth.register.ownerNamePlaceholder'), autocomplete: 'name', icon: 'icon-user', max: 100 })}
        {/if}

        <div class="grid sm:grid-cols-2 gap-3">
          {#if !googleToken}
          {@render textField('email', t('auth.register.email'), email, (v) => (email = v), { type: 'email', placeholder: t('auth.login.emailPlaceholder'), autocomplete: 'email', icon: 'icon-mail', max: 254 })}
          {/if}
          {@render textField('phone', t('auth.register.phone'), phone, (v) => (phone = v), { type: 'tel', placeholder: t('auth.register.phonePlaceholder'), autocomplete: 'tel', icon: 'icon-phone', max: 24 })}
        </div>

        {#if !googleToken}

        <div>
          <label for="password" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">{t('auth.register.password')}</label>
          <div class="relative mt-1.5">
            <i class="icon-lock absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              placeholder={t('auth.register.passwordPlaceholder')}
              autocomplete="new-password"
              maxlength="128"
              aria-invalid={!!errors.password}
              aria-describedby={errors.password ? 'password-err' : undefined}
              class="{inputClass} ps-9 pe-9"
            />
            <button type="button" class="absolute top-1/2 -translate-y-1/2 end-3 text-tertiary" aria-label={showPassword ? t('auth.login.hidePassword') : t('auth.login.showPassword')} onclick={() => (showPassword = !showPassword)}>
              <i class={showPassword ? 'icon-eye-off text-[14px]' : 'icon-eye text-[14px]'}></i>
            </button>
          </div>
          {#if errors.password}<p id="password-err" class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{errors.password}</p>{/if}
          <div class="flex items-center gap-1.5 mt-2" aria-live="polite">
            {#each [0, 1, 2] as i (i)}
              <span class="flex-1 h-1 rounded-full {password !== '' && i <= strength ? (strength === 0 ? 'bg-danger-500' : strength === 1 ? 'bg-warning-500' : 'bg-success-500') : 'bg-border-subtle'}"></span>
            {/each}
            <span class="text-[11px] font-semibold min-w-10 text-tertiary">{strengthLabel}</span>
          </div>
        </div>

        <div>
          <label for="confirm" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">{t('auth.register.confirmPassword')}</label>
          <div class="relative mt-1.5">
            <i class="icon-lock absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="confirm"
              type={showPassword ? 'text' : 'password'}
              bind:value={confirm}
              autocomplete="new-password"
              maxlength="128"
              aria-invalid={!!errors.confirm}
              class="{inputClass} ps-9"
            />
          </div>
          {#if errors.confirm}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{errors.confirm}</p>{/if}
        </div>
        {/if}

        <div>
          <label class="flex items-start gap-2 text-[12px]">
            <input type="checkbox" class="size-3.5 rounded mt-0.5" bind:checked={acceptTerms} aria-invalid={!!errors.accept_terms} />
            <span>
              {t('account.terms.accept')} <a href="/terms" target="_blank" rel="noopener" class="font-semibold text-primary-600">{t('account.terms.termsLink')}</a>
              {t('account.terms.and')} <a href="/privacy" target="_blank" rel="noopener" class="font-semibold text-primary-600">{t('account.terms.privacyLink')}</a>
            </span>
          </label>
          {#if errors.accept_terms}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{errors.accept_terms}</p>{/if}
        </div>

        <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
          {t('auth.register.submit')}<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
        </button>
      </form>

      <p class="text-center text-[12.5px] mt-6 text-tertiary">
        {t('auth.register.haveAccount')} <a href="/login" class="font-semibold text-primary-600">{t('auth.register.signIn')}</a>
      </p>
    </div>

    <div class="relative w-full max-w-[420px] mx-auto">
      <LoginFooter />
    </div>
  </div>
</div>
