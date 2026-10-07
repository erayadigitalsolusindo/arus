<script lang="ts">
  import MasterPage from '#lib/components/MasterPage.svelte';
  import { t } from '#lib/i18n/index.ts';
</script>

<MasterPage kind="principals" docTitle={t('catalog.principals.docTitle')} title={t('catalog.principals.title')} subtitle={t('catalog.principals.subtitle')} addLabel={t('catalog.principals.add')} editLabel={t('catalog.principals.edit')} />
