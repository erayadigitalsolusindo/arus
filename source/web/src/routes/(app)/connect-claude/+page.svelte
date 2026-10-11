<script lang="ts">
  import { onMount } from 'svelte';
  import { ai } from '#lib/ai/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // Persetujuan connector Claude (OAuth). Parameter berasal dari klien lewat /oauth/authorize; server memeriksa ulang
  // klien + redirect_uri persis saat menyetujui, jadi halaman ini tidak pernah mengalihkan ke alamat sembarang.
  const KEYS = ['client_id', 'redirect_uri', 'state', 'code_challenge', 'code_challenge_method'] as const;

  let info = $state<{ name: string; host: string } | null | undefined>();
  let busy = $state(false);
  let error = $state('');
  const params = $state<Record<string, string>>({});

  onMount(async () => {
    document.title = t('ai.connect.docTitle');
    const q = new URLSearchParams(location.search);
    for (const k of KEYS) params[k] = q.get(k) ?? '';
    try {
      info = await ai.clientInfo(new URLSearchParams({ client_id: params.client_id, redirect_uri: params.redirect_uri }));
    } catch {
      info = null;
    }
  });

  async function decide(decision: 'allow' | 'deny') {
    if (busy) return;
    busy = true;
    error = '';
    try {
      const res = await ai.approve({ ...params, decision });
      window.location.href = res.redirect_url;
    } catch (e) {
      error = errorMessage(e);
      busy = false;
    }
  }
</script>

<main class="p-4 lg:p-6 max-w-xl mx-auto w-full min-w-0">
  {#if info === undefined}
    <div class="h-56 rounded-xl bg-[var(--surface-sunken)] animate-pulse" aria-busy="true"></div>
  {:else if info === null}
    <div class="surface-card p-5 flex items-start gap-3" role="alert">
      <i class="icon-circle-alert text-[20px] text-[var(--text-tertiary)]"></i>
      <p class="text-[13px] text-[var(--text-secondary)]">{t('ai.connect.invalid')}</p>
    </div>
  {:else}
    <div class="surface-card p-5 space-y-4">
      <div class="flex items-start gap-3">
        <span class="grid place-items-center size-11 shrink-0 rounded-xl bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)] text-[var(--color-primary-600)]"><i class="icon-sparkles text-[22px]"></i></span>
        <div class="min-w-0">
          <h1 class="font-display font-bold text-[18px] leading-tight">{t('ai.connect.title', { name: info.name })}</h1>
          <p class="text-[12.5px] mt-1.5 leading-relaxed text-[var(--text-secondary)]">{t('ai.connect.lead', { user: session.user?.name ?? '', name: info.name, host: info.host })}</p>
        </div>
      </div>

      <ul class="space-y-2 text-[13px]">
        <li class="flex items-start gap-2"><i class="icon-circle-check text-[16px] mt-0.5 shrink-0 text-[var(--color-success-600,#16a34a)]"></i><span>{t('ai.connect.c1')}</span></li>
        <li class="flex items-start gap-2"><i class="icon-circle-check text-[16px] mt-0.5 shrink-0 text-[var(--color-success-600,#16a34a)]"></i><span>{t('ai.connect.c2')}</span></li>
        <li class="flex items-start gap-2"><i class="icon-shield-check text-[16px] mt-0.5 shrink-0 text-[var(--color-primary-600)]"></i><span class="font-medium">{t('ai.connect.c3')}</span></li>
      </ul>

      {#if error}
        <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
        </div>
      {/if}

      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="btn btn-primary" disabled={busy} onclick={() => decide('allow')}>{busy ? t('ai.connect.working') : t('ai.connect.allow')}</button>
        <button type="button" class="btn" disabled={busy} onclick={() => decide('deny')}>{t('ai.connect.deny')}</button>
      </div>
      <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('ai.connect.revokeHint')}</p>
    </div>
  {/if}
</main>
