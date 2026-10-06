<script lang="ts">
  import { z } from 'zod';
  import { goto } from '$app/navigation';
  import { api, ApiError } from '#lib/api/client.ts';

  const schema = z.object({
    username: z.string().trim().min(1, 'Nama pengguna wajib diisi.'),
    password: z.string().min(1, 'Kata sandi wajib diisi.')
  });

  let username = $state('');
  let password = $state('');
  let showPassword = $state(false);
  let loading = $state(false);
  let formError = $state('');
  let fieldErrors = $state<{ username?: string; password?: string }>({});

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    formError = '';
    fieldErrors = {};

    const parsed = schema.safeParse({ username, password });
    if (!parsed.success) {
      for (const issue of parsed.error.issues) {
        const key = issue.path[0] as 'username' | 'password';
        fieldErrors[key] ??= issue.message;
      }
      return;
    }

    loading = true;
    try {
      // Endpoint dibuat di Fase 2.1; sampai saat itu API akan membalas NOT_FOUND.
      await api('/auth/login', { method: 'POST', body: JSON.stringify(parsed.data) });
      await goto('/dashboard');
    } catch (err) {
      formError = err instanceof ApiError ? err.message : 'Terjadi kesalahan tak terduga.';
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>Masuk · ACIRABA</title></svelte:head>

<div class="min-h-screen grid lg:grid-cols-2 bg-base">
  <div class="flex flex-col justify-center px-6 sm:px-10 lg:px-16 py-10">
    <div class="w-full max-w-[400px] mx-auto">
      <a href="/" class="flex items-center gap-2.5 mb-10">
        <img src="/logo-dark.svg" alt="ACIRABA" class="login-logo" />
      </a>

      <h1 class="font-display font-bold text-[22px]">Selamat datang kembali</h1>
      <p class="text-[12.5px] mt-1.5 text-tertiary">Masuk untuk melanjutkan ke ruang kerja Anda</p>

      <form class="space-y-4 mt-8" onsubmit={submit} novalidate>
        {#if formError}
          <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
            <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i>
            <span>{formError}</span>
          </div>
        {/if}

        <div>
          <label for="username" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">Nama pengguna</label>
          <div class="relative mt-1.5">
            <i class="icon-user absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
            <input
              id="username"
              type="text"
              bind:value={username}
              placeholder="nama.pengguna"
              autocomplete="username"
              aria-invalid={!!fieldErrors.username}
              class="w-full ps-9 pe-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered"
            />
          </div>
          {#if fieldErrors.username}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.username}</p>{/if}
        </div>

        <div>
          <label for="password" class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-tertiary">Kata sandi</label>
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
            <button
              type="button"
              class="absolute top-1/2 -translate-y-1/2 end-3 text-tertiary"
              aria-label={showPassword ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}
              onclick={() => (showPassword = !showPassword)}
            >
              <i class={showPassword ? 'icon-eye-off text-[14px]' : 'icon-eye text-[14px]'}></i>
            </button>
          </div>
          {#if fieldErrors.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldErrors.password}</p>{/if}
        </div>

        <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
          {#if loading}<i class="icon-loader-circle animate-spin text-[13px]"></i>Memproses…{:else}Masuk<i class="icon-arrow-right text-[13px]"></i>{/if}
        </button>
      </form>
    </div>
  </div>

  <div class="hidden lg:flex relative items-center justify-center p-12 overflow-hidden u-background-linear-gradient-150deg-color-primary-700-color-pr">
    <div class="absolute inset-0 opacity-10 u-background-image-radial-gradient-circle-at-20-20-white-1px-t"></div>
    <div class="relative max-w-[440px] w-full">
      <h2 class="font-display font-bold text-[24px] text-white">Seluruh operasional toko dalam satu ruang kerja.</h2>
      <p class="text-[13px] mt-2 u-color-rgb-255-255-255-0-7">Kasir, stok, pembelian, dan laporan — terpadu dan akurat.</p>
      <div class="space-y-2 mt-5">
        {#each ['Kasir cepat dengan harga dihitung server', 'Stok akurat per outlet, tercatat sebagai ledger', 'Hak akses berbasis peran untuk setiap tim'] as line (line)}
          <div class="flex items-center gap-2 text-[12.5px] text-white">
            <i class="icon-check-circle-2 text-[15px] shrink-0 u-color-color-success-400"></i>{line}
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>
