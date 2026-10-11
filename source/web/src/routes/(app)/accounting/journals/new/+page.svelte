<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import JournalModal from '#lib/components/JournalModal.svelte';
  import { can } from '#lib/auth/session.svelte.ts';
  import { tabs } from '#lib/tabs/store.svelte.ts';
  import { t } from '#lib/i18n/index.ts';

  // Tab "Jurnal Baru": editor jurnal tanpa pop up; drafnya disimpan di localStorage (lihat JournalModal).
  const type = page.url.searchParams.get('type') ?? '';
  const initialType = ['JU', 'KM', 'KK', 'TK'].includes(type) ? type : '';

  function close() {
    const next = tabs.close(page.url.pathname, page.url.pathname);
    void goto(next ?? '/accounting/journals');
  }
</script>

<svelte:head><title>{t('accounting.journals.titleNew')} | ACIRABA</title></svelte:head>

<main class="p-4 lg:p-6 space-y-4 max-w-[1100px] mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <h1 class="font-display font-bold text-[19px]">{t('accounting.journals.titleNew')}</h1>
    <a href="/accounting/journals" class="btn btn-sm"><i class="icon-list text-[14px]"></i>{t('accounting.journals.title')}</a>
  </div>
  {#if can('journals', 'create')}
    <div class="surface-card">
      <JournalModal page {initialType} onclose={close} onchanged={() => {}} />
    </div>
  {/if}
</main>
