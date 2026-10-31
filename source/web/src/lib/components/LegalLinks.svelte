<script lang="ts">
  import { t } from '#lib/i18n/index.ts';

  // Tautan ke Syarat Layanan dan Kebijakan Privasi; warna mengikuti teks induk (footer terang, sidebar gelap).
  // `newTab` dipakai di dalam aplikasi supaya tab kerja dan draf formulir tidak ditinggalkan.
  let { newTab = false, help = false }: { newTab?: boolean; help?: boolean } = $props();
  const rel = $derived(newTab ? 'noopener noreferrer' : undefined);
  const target = $derived(newTab ? '_blank' : undefined);
</script>

<nav class="legal-links" aria-label={t('legal.nav')}>
  <a href="/terms" {target} {rel}>{t('legal.terms.title')}</a>
  <span aria-hidden="true">·</span>
  <a href="/privacy" {target} {rel}>{t('legal.privacy.title')}</a>
  {#if help}
    <span aria-hidden="true">·</span>
    <a href="/help" {target} {rel}>{t('common.help')}</a>
  {/if}
</nav>

<style>
  .legal-links { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 0.1rem 0.5rem; }
  .legal-links a { color: inherit; text-decoration: none; opacity: 0.85; }
  .legal-links a:hover, .legal-links a:focus-visible { opacity: 1; text-decoration: underline; text-underline-offset: 2px; }
</style>
