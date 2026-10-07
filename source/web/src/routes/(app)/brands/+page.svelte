<script lang="ts">
  import MasterPage from '#lib/components/MasterPage.svelte';
  import { t } from '#lib/i18n/index.ts';
</script>

<MasterPage kind="brands" docTitle={t('catalog.brands.docTitle')} title={t('catalog.brands.title')} subtitle={t('catalog.brands.subtitle')} addLabel={t('catalog.brands.add')} editLabel={t('catalog.brands.edit')} />
