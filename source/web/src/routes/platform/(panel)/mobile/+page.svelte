<script lang="ts">
  import { onMount } from 'svelte';
  import { API_URL } from '#lib/api/client.ts';
  import { platformApi, type ApkInfo } from '#lib/platform/api.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const MAX_BYTES = 150 * 1024 * 1024;

  let info = $state<ApkInfo | null>(null);
  let error = $state('');
  let notice = $state('');
  let busy = $state(false);
  let file = $state<File | null>(null);
  let version = $state('');
  let input = $state<HTMLInputElement>();

  const size = (n: number) => (n >= 1024 * 1024 ? `${(n / 1024 / 1024).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB`);

  async function load() {
    try {
      info = await platformApi.apkInfo();
    } catch (err) {
      error = errorMessage(err);
    }
  }
  onMount(load);

  function pick(e: Event) {
    const f = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    error = notice = '';
    if (f && f.size > MAX_BYTES) {
      error = t('errors.FILE_TOO_LARGE');
      file = null;
      if (input) input.value = '';
      return;
    }
    file = f;
  }

  async function upload() {
    if (!file) return;
    busy = true;
    error = notice = '';
    try {
      info = await platformApi.uploadApk(file, version.trim());
      notice = t('platform.mobile.uploaded');
      file = null;
      version = '';
      if (input) input.value = '';
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!confirm(t('platform.mobile.removeConfirm'))) return;
    busy = true;
    error = notice = '';
    try {
      await platformApi.deleteApk();
      info = { available: false };
      notice = t('platform.mobile.removed');
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>{t('platform.mobile.title')} · {t('platform.docTitle')}</title></svelte:head>

<div class="space-y-5 max-w-3xl">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('platform.mobile.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.mobile.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}

  <section class="surface-card p-4 space-y-3">
    <h2 class="font-display font-bold text-[14px]">{t('platform.mobile.current')}</h2>
    {#if info?.available}
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[13px]">
        <dt class="text-[var(--text-tertiary)]">{t('platform.mobile.version')}</dt>
        <dd class="font-semibold">{info.version || '–'}</dd>
        <dt class="text-[var(--text-tertiary)]">{t('platform.mobile.size')}</dt>
        <dd class="font-semibold">{size(info.size ?? 0)}</dd>
        <dt class="text-[var(--text-tertiary)]">{t('platform.mobile.uploadedAt')}</dt>
        <dd class="font-semibold">{info.uploaded_at ? formatDateTime(info.uploaded_at) : '–'}</dd>
        <dt class="text-[var(--text-tertiary)]">{t('platform.mobile.checksum')}</dt>
        <dd class="font-mono text-[11.5px] break-all">{info.sha256}</dd>
      </dl>
      <div class="flex flex-wrap gap-2">
        <a class="btn btn-outline !text-[12.5px]" href="{API_URL}/public/mobile/apk" download><i class="icon-download text-[14px]"></i>{t('platform.mobile.download')}</a>
        <button type="button" class="btn btn-outline !text-[12.5px] !text-[var(--danger)]" disabled={busy} onclick={remove}><i class="icon-trash-2 text-[14px]"></i>{t('platform.mobile.remove')}</button>
      </div>
    {:else if info}
      <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.mobile.none')}</p>
    {/if}
  </section>

  <section class="surface-card p-4 space-y-3">
    <h2 class="font-display font-bold text-[14px]">{t('platform.mobile.uploadTitle')}</h2>
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.mobile.uploadHelp')}</p>

    <label class="block text-[12.5px] font-semibold">
      {t('platform.mobile.versionLabel')}
      <input class="block w-full max-w-[220px] mt-1 px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" maxlength="32" placeholder={t('platform.mobile.versionHint')} bind:value={version} disabled={busy} />
    </label>

    <label class="block text-[12.5px] font-semibold">
      {t('platform.mobile.choose')}
      <input
        bind:this={input}
        type="file"
        accept=".apk,application/vnd.android.package-archive"
        class="block w-full mt-1 px-3 py-2 rounded-lg text-[12.5px] bg-sunken-bordered"
        disabled={busy}
        onchange={pick}
      />
    </label>
    {#if file}<p class="text-[12px] text-[var(--text-tertiary)]">{file.name} · {size(file.size)}</p>{/if}

    <button type="button" class="btn btn-primary !text-[13px]" disabled={busy || !file} onclick={upload}>
      <i class={busy ? 'icon-loader-circle animate-spin text-[14px]' : 'icon-upload text-[14px]'}></i>{busy ? t('platform.mobile.uploading') : t('platform.mobile.upload')}
    </button>
  </section>
</div>
