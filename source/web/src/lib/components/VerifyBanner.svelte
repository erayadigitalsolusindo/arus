<script lang="ts">
  import { api, CSRF_HEADERS } from '#lib/api/client.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  type Phase = 'idle' | 'sending' | 'sent' | 'error';
  let phase = $state<Phase>('idle');
  let error = $state('');

  async function resend() {
    phase = 'sending';
    error = '';
    try {
      await api('/auth/resend-verification', { method: 'POST', headers: CSRF_HEADERS });
      phase = 'sent';
    } catch (err) {
      error = errorMessage(err);
      phase = 'error';
    }
  }
</script>

{#if !session.emailVerified}
  <div role="status" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 lg:px-6 py-2 text-[12.5px] badge-warning">
    <i class="icon-mail-warning text-[14px] shrink-0"></i>
    <span class="flex-1 min-w-[200px]">{phase === 'sent' ? t('account.verifyBanner.sent') : t('account.verifyBanner.text')}</span>
    {#if phase === 'error'}<span role="alert" class="font-medium">{error}</span>{/if}
    {#if phase !== 'sent'}
      <button type="button" class="font-semibold underline disabled:opacity-60" disabled={phase === 'sending'} onclick={resend}>
        {phase === 'sending' ? t('account.verifyBanner.sending') : t('account.verifyBanner.resend')}
      </button>
    {/if}
  </div>
{/if}
