<script lang="ts">
  import MasterPage from '#lib/components/MasterPage.svelte';
  import { t } from '#lib/i18n/index.ts';
</script>

<MasterPage kind="categories" docTitle={t('catalog.categories.docTitle')} title={t('catalog.categories.title')} subtitle={t('catalog.categories.subtitle')} addLabel={t('catalog.categories.add')} editLabel={t('catalog.categories.edit')} />
