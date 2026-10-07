<script lang="ts">
  import MasterPage from '#lib/components/MasterPage.svelte';
  import { t } from '#lib/i18n/index.ts';
</script>

<MasterPage kind="units" docTitle={t('catalog.units.docTitle')} title={t('catalog.units.title')} subtitle={t('catalog.units.subtitle')} addLabel={t('catalog.units.add')} editLabel={t('catalog.units.edit')} />
