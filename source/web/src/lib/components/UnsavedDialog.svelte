<script lang="ts">
  import { beforeNavigate } from '$app/navigation';
  import { t } from '#lib/i18n/index.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import Modal from '#lib/components/Modal.svelte';

  // Pindah halaman/tab di dalam aplikasi tidak dicegat: form menyimpan drafnya (lib/tabs/drafts.ts) dan memulihkannya.
  // Yang dicegat hanya reload/tutup browser (dialog bawaan browser); dialog di bawah dipakai saat menutup tab.
  beforeNavigate((nav) => {
    if (guard.dirty && (nav.type === 'leave' || !nav.to)) nav.cancel();
  });
</script>

{#if guard.pending}
  <Modal title={t('shell.unsaved.title')} onclose={() => guard.cancel()}>
    <p class="text-[13px] mb-5">{t('shell.unsaved.body')}</p>
    <div class="flex justify-end gap-2">
      <button type="button" class="btn !text-[12.5px]" onclick={() => guard.cancel()}>{t('shell.unsaved.stay')}</button>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => guard.confirm()}>{t('shell.unsaved.discard')}</button>
    </div>
  </Modal>
{/if}
