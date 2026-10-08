<script lang="ts">
  import MemberForm from '#lib/components/MemberForm.svelte';
  import { t } from '#lib/i18n/index.ts';
</script>

<svelte:head><title>{t('members.newTitle')} | ACIRABA</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('members.newTitle')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <a href="/members" class="text-[var(--color-primary-600)]">{t('members.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('members.add')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 max-w-full mx-auto w-full">
  <MemberForm member={null} />
</main>
