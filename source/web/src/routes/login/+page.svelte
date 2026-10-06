<script lang="ts">
  import { z } from 'zod';
  import { goto } from '$app/navigation';
  import { api, ApiError } from '#lib/api/client.ts';
  import TransactionFeed from '#lib/components/TransactionFeed.svelte';

  const schema = z.object({
    email: z.string().trim().min(1, 'Email wajib diisi.').pipe(z.email('Format email tidak valid.')),
    password: z.string().min(1, 'Kata sandi wajib diisi.')
  });

  const points = [
    'Ekosistem dan Integrasi terlengkap untuk maksimalkan peluang usaha di level berikutnya.',
    'Aplikasi kami membantu dalam melesatkan bisnis Anda.',
    'Point of Sale dengan ragam fitur lengkap, mudah digunakan dan penyajian data akurat, pastikan strategi bisnis yang lebih tepat.',
    'Whitelabel untuk membangun brand bisnis Anda sendiri.',
    'Aciraba hadir untuk mendukung kemajuan bisnis UMKM.'
  ];

  let email = $state('');
  let password = $state('');
  let remember = $state(true);
  let showPassword = $state(false);
  let loading = $state(false);
  let formError = $state('');
  let fieldErrors = $state<{ email?: string; password?: string }>({});

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    formError = '';
    fieldErrors = {};

    const parsed = schema.safeParse({ email, password });
    if (!parsed.success) {
      for (const issue of parsed.error.issues) {
        const key = issue.path[0] as 'email' | 'password';
        fieldErrors[key] ??= issue.message;
      }
      return;
    }

    loading = true;
    try {
      // Endpoint dibuat di Fase 2.1; sampai saat itu API akan membalas NOT_FOUND.
      await api('/auth/login', { method: 'POST', body: JSON.stringify({ ...parsed.data, remember }) });
      await goto('/dashboard');
    } catch (err) {
      formError = err instanceof ApiError ? err.message : 'Terjadi kesalahan tak terduga.';
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Sign In | ACIRABA</title></svelte:head>

<div class="min-h-screen grid lg:grid-cols-2 bg-base">
  <!-- Left: form -->
  <div class="flex flex-col justify-center px-6 sm:px-10 lg:px-16 py-10">
    <div class="w-full max-w-[400px] mx-auto">
      <a href="/" class="flex items-center gap-2.5 mb-10">
        <img src="/logo-dark.svg" alt="ACIRABA logo" class="login-logo" />
      </a>

      <h1 class="font-display font-bold text-[22px]">Welcome back</h1>
      <p class="text-[12.5px] mt-1.5 text-tertiary">Sign in to continue to your workspace</p>

      <div class="grid grid-cols-2 gap-3 mt-6">
        <button type="button" class="btn btn-outline !text-[12.5px] justify-center"><i class="fa-brands fa-google text-[13px]"></i>Google</button>
        <button type="button" class="btn btn-outline !text-[12.5px] justify-center"><i class="fa-brands fa-github text-[14px]"></i>GitHub</button>
      </div>

      <div class="flex items-center gap-3 my-6">
        <span class="flex-1 h-px bg-border-subtle"></span>
        <span class="text-[11px] font-medium text-tertiary">or sign in with email</span>
        <span class="flex-1 h-px bg-border-subtle"></span>
      </div>

      <form class="space-y-4" onsubmit={submit} novalidate>
        {#if formError}
          <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
            <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i>
            <span>{formError}</span>
          </div>
        {/if}

        <div>
          <label for="email" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">Email address</label>
          <div class="relative mt-1.5">
            <i class="icon-mail absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="email"
              type="email"
              bind:value={email}
              placeholder="you@company.com"
              autocomplete="username"
              aria-invalid={!!fieldErrors.email}
              class="w-full ps-9 pe-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered"
            />
          </div>
          {#if fieldErrors.email}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.email}</p>{/if}
        </div>

        <div>
          <div class="flex items-center justify-between">
            <label for="password" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">Password</label>
            <a href="/forgot-password" class="text-[11.5px] font-semibold text-primary-600">Forgot password?</a>
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
            <button type="button" class="absolute top-1/2 -translate-y-1/2 end-3 text-tertiary" aria-label="View" onclick={() => (showPassword = !showPassword)}>
              <i class={showPassword ? 'icon-eye-off text-[14px]' : 'icon-eye text-[14px]'}></i>
            </button>
          </div>
          {#if fieldErrors.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.password}</p>{/if}
        </div>

        <label class="flex items-center gap-2 text-[12px] font-medium">
          <input type="checkbox" class="size-3.5 rounded" bind:checked={remember} />
          Keep me signed in
        </label>

        <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
          Sign In<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
        </button>
      </form>

      <p class="text-center text-[12.5px] mt-6 text-tertiary">
        Don't have an account? <a href="/register" class="font-semibold text-primary-600">Create one</a>
      </p>
    </div>
  </div>

  <!-- Right: brand panel -->
  <div class="hidden lg:flex relative items-center justify-center p-12 overflow-hidden u-background-linear-gradient-150deg-color-primary-700-color-pr">
    <div class="absolute inset-0 opacity-10 u-background-image-radial-gradient-circle-at-20-20-white-1px-t"></div>
    <div class="relative max-w-[440px] w-full">
      <h2 class="font-display font-bold text-[24px] text-white">Wirausaha Membuka Lapangan Pekerjaan!</h2>
      <div class="space-y-3 mt-5">
        {#each points as line (line)}
          <div class="flex items-start gap-2 text-[12.5px] text-white">
            <i class="icon-check-circle-2 text-[15px] shrink-0 mt-px u-color-color-success-400"></i>{line}
          </div>
        {/each}
      </div>

      <TransactionFeed />
    </div>
  </div>
</div>
