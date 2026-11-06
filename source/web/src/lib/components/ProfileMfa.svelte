<script lang="ts">
  // Pengaturan 2FA TOTP di halaman profil: aktifkan (QR + kode), kode pemulihan, buat ulang, nonaktifkan.
  // Rahasia hanya ada di respons `begin`; QR dibuat di klien (tidak dikirim ke layanan luar).
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';
  import { ApiError } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { profileApi, type MfaSetup, type MfaStatus } from '#lib/profile/api.ts';

  let { onchange }: { onchange: () => void } = $props();

  type Phase = 'idle' | 'setup' | 'codes' | 'disable' | 'regen';
  let status = $state<MfaStatus | null>(null);
  let loadError = $state('');
  let phase = $state<Phase>('idle');
  let setup = $state<MfaSetup | null>(null);
  let qr = $state('');
  let code = $state('');
  let password = $state('');
  let codes = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');
  let fieldErr = $state<Record<string, string>>({});
  let copied = $state(false);

  async function refresh() {
    try {
      status = await profileApi.mfa.status();
      loadError = '';
    } catch (e) {
      loadError = errorMessage(e);
    }
  }
  onMount(refresh);

  function reset(to: Phase = 'idle') {
    phase = to;
    code = password = error = '';
    fieldErr = {};
    setup = null;
    qr = '';
  }

  /** Menjalankan aksi server dengan penanganan galat seragam; kembalikan true bila berhasil. */
  async function run(fn: () => Promise<void>): Promise<boolean> {
    busy = true;
    error = '';
    fieldErr = {};
    try {
      await fn();
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.code === 'VALIDATION') fieldErr = e.fields;
      else error = errorMessage(e);
      return false;
    } finally {
      busy = false;
    }
  }

  async function begin() {
    reset('setup');
    await run(async () => {
      setup = await profileApi.mfa.begin();
      qr = await QRCode.toDataURL(setup.url, { margin: 1, width: 200 });
    });
  }

  async function enable(e: Event) {
    e.preventDefault();
    if (await run(async () => void (codes = (await profileApi.mfa.enable(code.trim())).recovery_codes))) {
      reset('codes');
      await refresh();
      onchange();
    }
  }

  async function regenerate(e: Event) {
    e.preventDefault();
    let fresh: string[] = [];
    if (await run(async () => void (fresh = (await profileApi.mfa.recovery(code.trim())).recovery_codes))) {
      codes = fresh;
      reset('codes');
      await refresh();
    }
  }

  async function disable(e: Event) {
    e.preventDefault();
    if (await run(() => profileApi.mfa.disable(password, code.trim()))) {
      reset();
      await refresh();
      onchange();
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(codes.join('\n'));
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      /* clipboard diblokir: pengguna masih bisa mengunduh */
    }
  }

  function download() {
    const url = URL.createObjectURL(new Blob([codes.join('\n') + '\n'], { type: 'text/plain' }));
    const a = Object.assign(document.createElement('a'), { href: url, download: 'aciraba-recovery-codes.txt' });
    a.click();
    URL.revokeObjectURL(url);
  }

  const group = (s: string) => s.replace(/(.{4})/g, '$1 ').trim();
  const inputCls = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[var(--text-primary)] text-[13.5px] focus-visible:outline-2 focus-visible:outline-[var(--color-primary-600)]';
</script>

<section class="space-y-4" aria-labelledby="mfa-title">
  <header class="flex flex-wrap items-start gap-3">
    <span class="grid place-items-center size-10 rounded-xl bg-[color-mix(in_oklab,var(--color-primary-600)_12%,transparent)] text-[var(--color-primary-600)]"><i class="icon-smartphone text-[18px]"></i></span>
    <div class="min-w-0 flex-1">
      <h3 id="mfa-title" class="font-display font-bold text-[15px] flex items-center gap-2">
        {t('profile.mfa.title')}
        {#if status}
          <span class="text-[11px] px-2 h-5 inline-flex items-center rounded-full font-semibold {status.enabled ? 'text-[var(--color-success-600)] bg-[color-mix(in_oklab,var(--color-success-600)_12%,transparent)]' : 'text-[var(--text-tertiary)] bg-[var(--surface-sunken)]'}">{status.enabled ? t('profile.mfa.on') : t('profile.mfa.off')}</span>
        {/if}
      </h3>
      <p class="text-[12.5px] text-[var(--text-secondary)] mt-0.5">{t('profile.mfa.subtitle')}</p>
    </div>
  </header>

  {#if loadError}
    <p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{t('profile.mfa.loadFailed')} {loadError}</p>
  {:else if status}
    {#if phase === 'idle'}
      {#if status.enabled}
        <div class="flex flex-wrap items-center gap-2.5">
          <span class="text-[12.5px] text-[var(--text-secondary)]">{t('profile.mfa.remaining', { count: status.recovery_remaining })}</span>
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => reset('regen')}><i class="icon-refresh-cw text-[13px]"></i>{t('profile.mfa.regenerate')}</button>
          <button type="button" class="btn btn-outline !text-[12.5px] text-[var(--color-danger-600)]" onclick={() => reset('disable')}>{t('profile.mfa.disable')}</button>
        </div>
        <p class="text-[12px] text-[var(--text-tertiary)]">{t('profile.mfa.lostDevice')}</p>
      {:else}
        <button type="button" class="btn btn-primary" onclick={begin}><i class="icon-shield-check text-[14px]"></i>{t('profile.mfa.enable')}</button>
      {/if}
    {:else if phase === 'setup'}
      <form class="grid gap-5 sm:grid-cols-[auto_minmax(0,1fr)] p-4 rounded-xl border border-[var(--border-subtle)]" onsubmit={enable} novalidate>
        <div class="space-y-2">
          <p class="text-[12.5px] font-semibold sm:max-w-[200px]">{t('profile.mfa.stepScan')}</p>
          {#if qr}
            <img src={qr} alt={t('profile.mfa.qrAlt')} width="200" height="200" class="rounded-lg bg-white p-1.5" />
          {:else}
            <div class="size-[200px] rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div>
          {/if}
        </div>
        <div class="space-y-3 min-w-0">
          {#if setup}
            <div class="text-[12px] text-[var(--text-secondary)]">
              {t('profile.mfa.manual')}
              <code class="block mt-1 px-2.5 py-2 rounded-lg bg-[var(--surface-sunken)] font-mono text-[13px] tracking-wider select-all wrap-anywhere">{group(setup.secret)}</code>
            </div>
          {/if}
          <label class="block text-[13px]">
            <span class="font-semibold">{t('profile.mfa.stepCode')}</span>
            <input class="{inputCls} font-mono tracking-[0.3em] text-center sm:text-start" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123456" aria-invalid={!!fieldErr.code} />
            {#if fieldErr.code}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.code)}</span>{/if}
          </label>
          {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
          <div class="flex gap-2">
            <button type="submit" class="btn btn-primary disabled:opacity-60" disabled={busy || !setup || code.trim().length < 6}>{t('profile.mfa.verify')}</button>
            <button type="button" class="btn btn-outline" onclick={() => reset()}>{t('profile.mfa.cancel')}</button>
          </div>
        </div>
      </form>
    {:else if phase === 'codes'}
      <div class="p-4 rounded-xl border border-[var(--color-warning-600)] bg-[color-mix(in_oklab,var(--color-warning-600)_7%,transparent)] space-y-3">
        <p class="text-[13.5px] font-semibold flex items-center gap-2"><i class="icon-key-round text-[15px]"></i>{t('profile.mfa.recoveryTitle')}</p>
        <p class="text-[12.5px] text-[var(--text-secondary)]">{t('profile.mfa.recoveryNote')}</p>
        <ul class="grid grid-cols-2 gap-2 font-mono text-[13.5px]">
          {#each codes as c (c)}<li class="px-3 py-1.5 rounded-lg bg-[var(--surface-raised)] border border-[var(--border-subtle)] text-center select-all">{c}</li>{/each}
        </ul>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={copy}><i class="{copied ? 'icon-check' : 'icon-copy'} text-[13px]"></i>{copied ? t('profile.mfa.copied') : t('profile.mfa.copy')}</button>
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={download}><i class="icon-download text-[13px]"></i>{t('profile.mfa.download')}</button>
          <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => { codes = []; reset(); }}>{t('profile.mfa.saved')}</button>
        </div>
      </div>
    {:else if phase === 'regen'}
      <form class="p-4 rounded-xl border border-[var(--border-subtle)] space-y-3 max-w-md" onsubmit={regenerate} novalidate>
        <p class="text-[12.5px] text-[var(--text-secondary)]">{t('profile.mfa.regenerateNote')}</p>
        <label class="block text-[13px]">
          <span class="font-semibold">{t('profile.mfa.code')}</span>
          <input class="{inputCls} font-mono tracking-[0.3em]" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123456" />
          {#if fieldErr.code}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.code)}</span>{/if}
        </label>
        {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
        <div class="flex gap-2">
          <button type="submit" class="btn btn-primary disabled:opacity-60" disabled={busy || code.trim().length < 6}>{t('profile.mfa.regenerate')}</button>
          <button type="button" class="btn btn-outline" onclick={() => reset()}>{t('profile.mfa.cancel')}</button>
        </div>
      </form>
    {:else if phase === 'disable'}
      <form class="p-4 rounded-xl border border-[var(--border-subtle)] space-y-3 max-w-md" onsubmit={disable} novalidate>
        <p class="text-[12.5px] text-[var(--text-secondary)]">{t('profile.mfa.disableNote')}</p>
        <label class="block text-[13px]">
          <span class="font-semibold">{t('profile.mfa.password')}</span>
          <input class={inputCls} type="password" bind:value={password} autocomplete="current-password" aria-invalid={!!fieldErr.password} />
          {#if fieldErr.password}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.password)}</span>{/if}
        </label>
        <label class="block text-[13px]">
          <span class="font-semibold">{t('profile.mfa.recoveryCodeOrTotp')}</span>
          <input class="{inputCls} font-mono" bind:value={code} autocomplete="one-time-code" maxlength="14" />
          {#if fieldErr.code}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.code)}</span>{/if}
        </label>
        {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
        <div class="flex gap-2">
          <button type="submit" class="btn btn-primary disabled:opacity-60" disabled={busy || !password || !code.trim()}>{t('profile.mfa.confirmDisable')}</button>
          <button type="button" class="btn btn-outline" onclick={() => reset()}>{t('profile.mfa.cancel')}</button>
        </div>
      </form>
    {/if}
  {:else}
    <div class="h-10 w-40 rounded bg-[var(--surface-sunken)] animate-pulse"></div>
  {/if}
</section>
