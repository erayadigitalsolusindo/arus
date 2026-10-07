<script lang="ts">
  import { beforeNavigate, goto } from '$app/navigation';
  import { t } from '#lib/i18n/index.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import Modal from '#lib/components/Modal.svelte';

  // Navigasi di dalam aplikasi (sidebar, tab, link) dicegat dan dikonfirmasi; reload/tutup browser memakai dialog bawaan browser.
  beforeNavigate((nav) => {
    if (!guard.dirty) return;
    if (nav.type === 'leave' || !nav.to) {
      nav.cancel();
      return;
    }
    const to = nav.to.url;
    nav.cancel();
    guard.request(() => void goto(to.pathname + to.search + to.hash));
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
