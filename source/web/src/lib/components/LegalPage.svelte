<script lang="ts">
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  // Satu komponen untuk Syarat Layanan dan Kebijakan Privasi; isi ada di kamus `legal.*` (id/en).
  let { doc, sections }: { doc: 'terms' | 'privacy'; sections: readonly string[] } = $props();
  const key = (s: string, part: 'title' | 'body') => `legal.${doc}.${s}.${part}` as MessageKey;
</script>

<svelte:head><title>{t(`legal.${doc}.docTitle` as MessageKey)}</title></svelte:head>

<AuthCard wide>
  <h1 class="font-display font-bold text-[20px]">{t(`legal.${doc}.title` as MessageKey)}</h1>
  <p class="text-[11.5px] mt-1 text-tertiary">{t('legal.updated')}</p>
  <p role="note" class="text-[12px] mt-3 rounded-lg px-3 py-2 badge-warning">{t('legal.draft')}</p>

  <div class="space-y-4 mt-5">
    {#each sections as s (s)}
      <section>
        <h2 class="font-display font-semibold text-[13.5px]">{t(key(s, 'title'))}</h2>
        <p class="text-[12.5px] mt-1 leading-relaxed text-secondary">{t(key(s, 'body'))}</p>
      </section>
    {/each}
  </div>

  <button type="button" class="btn btn-outline !text-[12.5px] mt-6" onclick={() => history.back()}>{t('legal.back')}</button>
</AuthCard>
