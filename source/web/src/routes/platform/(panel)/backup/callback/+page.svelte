<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { platformApi } from '#lib/platform/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // Halaman tujuan pengalihan OAuth Google: menukar kode lewat API lalu kembali ke halaman Backup.
  let status = $state<'work' | 'ok' | 'fail'>('work');
  let message = $state('');

  onMount(async () => {
    const q = page.url.searchParams;
    const code = q.get('code');
    const state = q.get('state');
    if (!code || !state || q.get('error')) {
      status = 'fail';
      message = q.get('error') ?? '';
      return;
    }
    try {
      await platformApi.backupConnect(code, state);
      status = 'ok';
      await goto('/platform/backup', { replaceState: true });
    } catch (err) {
      status = 'fail';
      message = errorMessage(err);
    }
  });
</script>

<svelte:head><title>{t('platform.backup.title')} · {t('platform.docTitle')}</title></svelte:head>

<div class="max-w-md surface-card p-5 space-y-3">
  {#if status === 'work'}
    <p class="text-[13px] flex items-center gap-2"><i class="icon-loader-circle animate-spin text-[14px]"></i>{t('platform.backup.cbConnecting')}</p>
  {:else if status === 'ok'}
    <p class="text-[13px]">{t('platform.backup.cbOk')}</p>
  {:else}
    <p role="alert" class="text-[13px] badge-danger rounded-lg px-3 py-2">{t('platform.backup.cbFail')} {message}</p>
    <a class="btn btn-outline !text-[12.5px]" href="/platform/backup">{t('platform.backup.cbBack')}</a>
  {/if}
</div>
