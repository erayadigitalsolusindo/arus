<script lang="ts">
  import { z } from 'zod';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, API_URL, ApiError, request, type AuthResponse } from '#lib/api/client.ts';
  import { homePath, startSession } from '#lib/auth/session.svelte.ts';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import TransactionFeed from '#lib/components/TransactionFeed.svelte';
  import LoginBackdrop from '#lib/components/LoginBackdrop.svelte';
  import BarcodeScanner from '#lib/components/BarcodeScanner.svelte';
  import LoginFooter from '#lib/components/LoginFooter.svelte';

  // APK Kasir yang diunggah Platform Admin; tombol hanya tampil bila sudah ada.
  const APK_URL = `${API_URL}/public/mobile/apk`;
  let apk = $state<{ available: boolean; version?: string } | null>(null);
  onMount(() => {
    request<{ available: boolean; version?: string }>('/public/mobile/apk/info', {}, null)
      .then((r) => (apk = r))
      .catch(() => (apk = null));
  });

  // Dibuat per submit agar pesan validasi mengikuti bahasa aktif.
  const makeSchema = () =>
    z.object({
      email: z.string().trim().min(1, t('auth.login.emailRequired')).pipe(z.email(t('auth.login.emailInvalid'))),
      password: z.string().min(1, t('auth.login.passwordRequired'))
    });

  const points: MessageKey[] = [
    'auth.promo.point1',
    'auth.promo.point2',
    'auth.promo.point3',
    'auth.promo.point4',
    'auth.promo.point5'
  ];

  let email = $state('');
  let password = $state('');
  let remember = $state(true);
  let showPassword = $state(false);
  let loading = $state(false);
  let formError = $state('');
  let attemptsLeft = $state(0); // >0 hanya bila pesan kesalahan memuat sisa percobaan
  let fieldErrors = $state<{ email?: string; password?: string }>({});
  let lockedUntil = $state(0);
  let now = $state(Date.now());
  const lockedSeconds = $derived(Math.max(0, Math.ceil((lockedUntil - now) / 1000)));
  const lockedText = $derived(lockedSeconds > 0 ? t('errors.ACCOUNT_LOCKED', { count: Math.ceil(lockedSeconds / 60) }) : '');

  // Hitung mundur sampai kunci berakhir; tombol masuk dinonaktifkan selama itu.
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
    formError = '';
    fieldErrors = {};
    attemptsLeft = 0;
    if (lockedSeconds > 0) return;

    const parsed = makeSchema().safeParse({ email, password });
    if (!parsed.success) {
      for (const issue of parsed.error.issues) {
        const key = issue.path[0] as 'email' | 'password';
        fieldErrors[key] ??= issue.message;
      }
      return;
    }

    loading = true;
    try {
      const res = await api<AuthResponse>('/auth/login', { method: 'POST', body: JSON.stringify({ ...parsed.data, remember }) });
      startSession(res);
      password = '';
      await goto(homePath());
    } catch (err) {
      if (err instanceof ApiError && err.code === 'ACCOUNT_LOCKED' && err.retryAfter > 0) {
        now = Date.now();
        lockedUntil = now + err.retryAfter * 1000;
        return;
      }
      if (err instanceof ApiError && err.code === 'INVALID_CREDENTIALS' && err.attemptsLeft > 0) {
        // Beri tahu sisa percobaan sebelum akun dikunci; tinggal satu = peringatan.
        attemptsLeft = err.attemptsLeft;
        formError = t('errors.INVALID_CREDENTIALS_LEFT', { count: err.attemptsLeft });
        return;
      }
      formError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>{t('auth.login.docTitle')}</title></svelte:head>

<div class="min-h-screen grid lg:grid-cols-2 bg-base">
  <!-- Left: form -->
  <div class="relative flex flex-col px-6 sm:px-10 lg:px-16 py-10 overflow-hidden">
    <LoginBackdrop />
    <div class="relative flex-1 flex flex-col justify-center w-full max-w-[400px] mx-auto">
      <BarcodeScanner />
      <h1 class="flex flex-col justify-center items-center text-center font-display font-bold text-[22px]">
        {t('auth.login.titleLine1')}
        <img src="/logo_dengan_text-no-bg.svg" alt={t('auth.login.titleLine2')} class="mt-1 h-20 w-auto" />
      </h1>
      <p class="flex justify-center items-center text-[12.5px] mt-1.5 text-tertiary">{t('auth.login.subtitle')}</p>

      <div class="grid grid-cols-2 gap-3 mt-6">
        <button type="button" class="btn btn-outline !text-[12.5px] justify-center"><i class="fa-brands fa-google text-[13px]"></i>Google</button>
        <button type="button" class="btn btn-outline !text-[12.5px] justify-center"><i class="fa-brands fa-github text-[14px]"></i>GitHub</button>
      </div>

      <div class="flex items-center gap-3 my-6">
        <span class="flex-1 h-px bg-border-subtle"></span>
        <span class="text-[11px] font-medium text-tertiary">{t('auth.login.orEmail')}</span>
        <span class="flex-1 h-px bg-border-subtle"></span>
      </div>

      <form class="space-y-4" onsubmit={submit} novalidate>
        {#if lockedText || formError}
          <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger {attemptsLeft === 1 ? 'font-semibold' : ''}">
            <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i>
            <span>{lockedText || formError}</span>
          </div>
        {/if}

        <div>
          <label for="email" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">{t('auth.login.email')}</label>
          <div class="relative mt-1.5">
            <i class="icon-mail absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="email"
              type="email"
              bind:value={email}
              placeholder={t('auth.login.emailPlaceholder')}
              autocomplete="username"
              aria-invalid={!!fieldErrors.email}
              class="w-full ps-9 pe-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered"
            />
          </div>
          {#if fieldErrors.email}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.email}</p>{/if}
        </div>

        <div>
          <div class="flex items-center justify-between">
            <label for="password" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">{t('auth.login.password')}</label>
            <a href="/forgot-password" class="text-[11.5px] font-semibold text-primary-600">{t('auth.login.forgot')}</a>
          </div>
          <div class="relative mt-1.5">
            <i class="icon-lock absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={password}
              placeholder="••••••••"
              autocomplete="current-password"
              aria-invalid={!!fieldErrors.password}
              class="w-full ps-9 pe-9 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered"
            />
            <button type="button" class="absolute top-1/2 -translate-y-1/2 end-3 text-tertiary" aria-label={showPassword ? t('auth.login.hidePassword') : t('auth.login.showPassword')} onclick={() => (showPassword = !showPassword)}>
              <i class={showPassword ? 'icon-eye-off text-[14px]' : 'icon-eye text-[14px]'}></i>
            </button>
          </div>
          {#if fieldErrors.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.password}</p>{/if}
        </div>

        <label class="flex items-center gap-2 text-[12px] font-medium">
          <input type="checkbox" class="size-3.5 rounded" bind:checked={remember} />
          {t('auth.login.remember')}
        </label>

        <button type="submit" disabled={loading || lockedSeconds > 0} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
          {t('auth.login.submit')}<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
        </button>
      </form>

      <p class="text-center text-[12.5px] mt-6 text-tertiary">
        {t('auth.login.noAccount')} <a href="/register" class="font-semibold text-primary-600">{t('auth.login.createOne')}</a>
      </p>

      {#if apk?.available}
      <a
        href={APK_URL}
        download
        class="btn btn-outline w-full justify-center !text-[12.5px] mt-4 font-bold tracking-wide"
      >
        <i class="icon-smartphone text-[14px]"></i>{t('auth.login.downloadApk')}
      </a>
      {/if}
    </div>

    <div class="relative w-full max-w-[400px] mx-auto">
      <LoginFooter />
    </div>
  </div>

  <!-- Right: brand panel -->
  <div class="hidden lg:flex relative items-center justify-center p-12 overflow-hidden u-background-linear-gradient-150deg-color-primary-700-color-pr">
    <div class="absolute inset-0 opacity-10 u-background-image-radial-gradient-circle-at-20-20-white-1px-t"></div>
    <div class="relative max-w-[440px] w-full">
      <h2 class="font-display font-bold text-[24px] text-white">{t('auth.promo.headline')}</h2>
      <div class="space-y-3 mt-5">
        {#each points as key (key)}
          <div class="flex items-start gap-2 text-[12.5px] text-white">
            <i class="icon-check-circle-2 text-[15px] shrink-0 mt-px u-color-color-success-400"></i>{t(key)}
          </div>
        {/each}
      </div>

      <TransactionFeed />
    </div>
  </div>
</div>
