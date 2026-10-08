<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import QRCode from 'qrcode';
  import { ApiError } from '#lib/api/client.ts';
  import { endPlatformSession, papi, platform, refresh } from '#lib/platform/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  type Status = { enabled: boolean; pending: boolean; recovery_remaining: number };
  const post = <T,>(path: string, body: unknown = {}) => papi<T>(path, { method: 'POST', body: JSON.stringify(body) });

  let status = $state<Status | null>(null);
  let error = $state('');
  let notice = $state('');
  let busy = $state(false);

  // Pendaftaran
  let setup = $state<{ secret: string; url: string } | null>(null);
  let qr = $state('');
  let code = $state('');
  // Kode pemulihan yang baru dibuat (tampil sekali)
  let recovery = $state<string[]>([]);
  // Aksi lanjutan (ganti kode pemulihan / nonaktifkan)
  let action = $state<'' | 'regen' | 'disable'>('');
  let actionCode = $state('');
  let password = $state('');

  async function load() {
    try {
      status = await papi<Status>('/platform/security');
    } catch (err) {
      error = errorMessage(err);
    }
  }
  onMount(load);

  async function run(fn: () => Promise<void>) {
    busy = true;
    error = notice = '';
    try {
      await fn();
    } catch (err) {
      error = err instanceof ApiError && err.code === 'INVALID_CODE' ? t('platform.security.invalidCode') : errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const begin = () =>
    run(async () => {
      setup = await post<{ secret: string; url: string }>('/platform/security/totp/begin');
      qr = await QRCode.toDataURL(setup.url, { margin: 1, width: 220 });
      code = '';
    });

  const enable = () =>
    run(async () => {
      recovery = (await post<{ recovery_codes: string[] }>('/platform/security/totp/enable', { code })).recovery_codes;
      setup = null;
      qr = code = '';
      await refresh(); // profil admin kini mfa_enabled = true
      await load();
    });

  const regenerate = () =>
    run(async () => {
      recovery = (await post<{ recovery_codes: string[] }>('/platform/security/recovery-codes', { code: actionCode })).recovery_codes;
      action = '';
      actionCode = '';
      await load();
    });

  const disable = () =>
    run(async () => {
      await post('/platform/security/totp/disable', { password, code: actionCode });
      // Semua sesi dicabut server: kembali ke login.
      endPlatformSession();
      await goto('/platform/login');
    });

  async function copy() {
    try {
      await navigator.clipboard.writeText(recovery.join('\n'));
      notice = t('platform.security.copied');
    } catch {
      /* clipboard diblokir: pengguna dapat menyalin manual */
    }
  }

  function download() {
    const blob = new Blob([`ACIRABA Platform: ${platform.admin?.email}\n\n${recovery.join('\n')}\n`], { type: 'text/plain' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'aciraba-platform-recovery-codes.txt';
    a.click();
    URL.revokeObjectURL(a.href);
  }

  const inputClass = 'w-full field-control';
  const codeClass = `${inputClass} font-mono tracking-[0.2em]`;
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
</script>

<div>
  <h1 class="font-display font-bold text-[19px]">{t('platform.security.title')}</h1>
  <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.security.subtitle')}</p>
</div>

{#if status && !status.enabled && !platform.admin?.mfa_enabled}
  <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-warning">
    <i class="icon-shield-alert text-[14px] mt-px shrink-0"></i><span>{t('platform.security.required')}</span>
  </div>
{/if}
{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}
{#if notice}
  <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
{/if}

{#if recovery.length}
  <section class="surface-card p-4 space-y-3">
    <h2 class="font-display font-bold text-[14px]">{t('platform.security.recoveryTitle')}</h2>
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.security.recoveryHelp')}</p>
    <ul class="grid grid-cols-2 sm:grid-cols-5 gap-2 font-mono text-[13px]">
      {#each recovery as c (c)}<li class="rounded-lg px-3 py-2 bg-[var(--surface-sunken)] text-center">{c}</li>{/each}
    </ul>
    <div class="flex flex-wrap gap-2">
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={copy}><i class="icon-copy text-[13px]"></i>{t('platform.security.copy')}</button>
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={download}><i class="icon-download text-[13px]"></i>{t('platform.security.download')}</button>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={async () => { recovery = []; if (platform.admin?.mfa_enabled) await goto('/platform/tenants'); }}>{t('platform.security.savedIt')}</button>
    </div>
  </section>
{:else if status}
  <section class="surface-card p-4 space-y-4 max-w-[560px]">
    <div class="flex items-center justify-between gap-3">
      <h2 class="font-display font-bold text-[14px]">{t('platform.security.totpTitle')}</h2>
      <span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {status.enabled ? 'badge-success' : 'badge-danger'}">{status.enabled ? t('platform.security.on') : t('platform.security.off')}</span>
    </div>

    {#if !status.enabled}
      {#if !setup}
        <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.security.enableHelp')}</p>
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy} onclick={begin}>{t('platform.security.start')}</button>
      {:else}
        <ol class="list-decimal ps-5 space-y-1 text-[12.5px] text-[var(--text-tertiary)]">
          <li>{t('platform.security.step1')}</li>
          <li>{t('platform.security.step2')}</li>
        </ol>
        <div class="flex flex-wrap items-center gap-4">
          {#if qr}<img src={qr} alt={t('platform.security.qrAlt')} width="220" height="220" class="rounded-lg bg-white p-1" />{/if}
          <div class="min-w-0">
            <p class={labelClass}>{t('platform.security.manualKey')}</p>
            <p class="font-mono text-[13px] break-all select-all">{setup.secret}</p>
          </div>
        </div>
        <form onsubmit={(e) => (e.preventDefault(), enable())}>
          <label for="totp-code" class={labelClass}>{t('platform.security.code')}</label>
          <input id="totp-code" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="6" class={codeClass} />
          <button type="submit" class="btn btn-primary !text-[12.5px] mt-3 disabled:opacity-60" disabled={busy || code.trim().length !== 6}>{t('platform.security.confirm')}</button>
        </form>
      {/if}
    {:else}
      <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.security.recoveryLeft', { count: status.recovery_remaining })}</p>
      {#if action === ''}
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (action = 'regen')}>{t('platform.security.regen')}</button>
          <button type="button" class="btn btn-outline !text-[12.5px] !text-[var(--color-danger-600)]" onclick={() => (action = 'disable')}>{t('platform.security.disable')}</button>
        </div>
      {:else}
        <form class="space-y-3" onsubmit={(e) => (e.preventDefault(), action === 'regen' ? regenerate() : disable())}>
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{action === 'regen' ? t('platform.security.regenHelp') : t('platform.security.disableHelp')}</p>
          {#if action === 'disable'}
            <div>
              <label for="sec-pass" class={labelClass}>{t('platform.login.password')}</label>
              <input id="sec-pass" type="password" bind:value={password} autocomplete="current-password" class={inputClass} />
            </div>
          {/if}
          <div>
            <label for="sec-code" class={labelClass}>{t('platform.security.code')}</label>
            <input id="sec-code" bind:value={actionCode} inputmode="numeric" autocomplete="one-time-code" maxlength="16" class={codeClass} />
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (action = '')}>{t('platform.admins.cancel')}</button>
            <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={busy || !actionCode.trim() || (action === 'disable' && !password)}>{t('platform.admins.save')}</button>
          </div>
        </form>
      {/if}
    {/if}
  </section>
{/if}
