<script lang="ts">
  import LegalPage from '#lib/components/LegalPage.svelte';

  const sections = ['s1', 's2', 's3', 's4', 's5', 's6', 's7', 's8'] as const;
</script>

<LegalPage doc="terms" {sections} />
