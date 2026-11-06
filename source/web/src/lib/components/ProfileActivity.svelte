<script lang="ts">
  // Linimasa aktivitas pribadi dari audit log (keyset: tombol "muat lebih banyak", bukan OFFSET).
  import { onMount } from 'svelte';
  import { t, tryT, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { profileApi, type Activity } from '#lib/profile/api.ts';

  let items = $state<Activity[]>([]);
  let next = $state('');
  let loading = $state(true);
  let error = $state('');

  async function load(more = false) {
    loading = true;
    error = '';
    try {
      const page = await profileApi.activity(more ? next : '');
      items = more ? [...items, ...page.data] : page.data;
      next = page.next;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => void load());

  const key = (a: string) => a.replace(/\./g, '_');
  const label = (a: Activity) => tryT(`profile.overview.activity.actions.${key(a.action)}`) ?? tryT(`audit.actions.${key(a.action)}`) ?? a.action;
  const icon = (action: string) =>
    action.startsWith('auth.login') ? 'log-in' : action.includes('password') ? 'key-round' : action.includes('mfa') ? 'shield-check' : action.includes('image') ? 'image' : action.startsWith('sale') ? 'shopping-cart' : action.includes('pin') ? 'fingerprint' : 'pencil';
</script>

<section class="space-y-3" aria-labelledby="act-title">
  <header>
    <h3 id="act-title" class="font-display font-bold text-[15px]">{t('profile.overview.activity.title')}</h3>
    <p class="text-[12.5px] text-[var(--text-secondary)] mt-0.5">{t('profile.overview.activity.subtitle')}</p>
  </header>

  {#if error}
    <p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{t('profile.overview.activity.failed')} {error}</p>
  {:else if !loading && items.length === 0}
    <p class="text-[13px] text-[var(--text-tertiary)] py-6 text-center">{t('profile.overview.activity.empty')}</p>
  {:else}
    <ol class="relative ms-3 border-s border-[var(--border-subtle)] space-y-4">
      {#each items as a (a.id)}
        <li class="ps-5 relative" style="padding-inline-start:1.25rem">
          <span class="absolute grid place-items-center size-6 rounded-full bg-[var(--surface-raised)] border border-[var(--border-subtle)] text-[var(--color-primary-600)]" style="inset-inline-start:-0.8rem;top:0"><i class="icon-{icon(a.action)} text-[12px]"></i></span>
          <p class="text-[13.5px] font-medium text-[var(--text-primary)]">{label(a)}</p>
          <p class="text-[12px] text-[var(--text-tertiary)]">
            {formatDateTime(a.created_at)}{#if a.ip} · {t('profile.overview.activity.from', { ip: a.ip })}{/if}
          </p>
        </li>
      {/each}
    </ol>
    {#if loading}<p class="text-[12.5px] text-[var(--text-tertiary)]" aria-live="polite">{t('profile.overview.activity.loading')}</p>{/if}
    {#if next && !loading}
      <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => load(true)}>{t('profile.overview.activity.more')}</button>
    {/if}
  {/if}
</section>
