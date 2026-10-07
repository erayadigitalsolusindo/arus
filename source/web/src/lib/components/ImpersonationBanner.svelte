<script lang="ts">
  import { session, exitImpersonation } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
</script>

{#if session.impersonating}
  <div role="status" class="sticky top-0 z-40 flex flex-wrap items-center justify-between gap-2 px-4 py-2 text-[12.5px] font-semibold text-white bg-[var(--color-danger-600)]">
    <span class="flex items-center gap-2">
      <i class="icon-shield-alert text-[15px] shrink-0"></i>
      {t('platform.banner.text', { tenant: session.tenant?.name ?? '', outlet: session.outlet?.name ?? '' })}
    </span>
    <button type="button" class="rounded-md px-2.5 py-1 bg-white/20 hover:bg-white/30" onclick={exitImpersonation}>{t('platform.banner.exit')}</button>
  </div>
{/if}
