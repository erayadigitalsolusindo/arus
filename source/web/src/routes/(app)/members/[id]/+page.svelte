<script lang="ts">
  import MemberForm from '#lib/components/MemberForm.svelte';
  import { can } from '#lib/auth/session.svelte.ts';
  import { page } from '$app/state';
  import { t } from '#lib/i18n/index.ts';

  let { data } = $props();
  // ?notice=cover_failed: member baru tersimpan tetapi foto cover gagal diunggah.
  const coverFailed = $derived(page.url.searchParams.get('notice') === 'cover_failed');
  const title = $derived(can('members', 'update') ? t('members.editTitle') : t('members.view'));
</script>

<svelte:head><title>{title} | ACIRABA</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{title}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <a href="/members" class="text-[var(--color-primary-600)]">{t('members.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{data.member.code}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 max-w-full mx-auto w-full space-y-3">
  {#if coverFailed}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-warning">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('members.cover.failed')}</span>
    </div>
  {/if}
  <!-- {#key}: berpindah ke member lain memasang ulang form dengan nilai awal yang baru. -->
  {#key data.member.id}
    <MemberForm member={data.member} />
  {/key}
</main>
