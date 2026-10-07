<script lang="ts">
  import { api } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { checkEmail } from '#lib/validation.ts';
  import { fieldMessage } from '#lib/i18n/errors.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  let email = $state('');
  let loading = $state(false);
  let sent = $state(false);
  let error = $state('');
  let fieldError = $state('');

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = fieldError = '';
    const mail = checkEmail(email);
    if (mail.code) {
      fieldError = fieldMessage(mail.code) ?? '';
      return;
    }
    loading = true;
    try {
      // Server selalu menjawab 202 (tidak membocorkan apakah email terdaftar); kita tampilkan hal yang sama.
      await api('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email: mail.value }) });
      sent = true;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>{t('account.forgot.docTitle')}</title></svelte:head>

<AuthCard>
  {#if sent}
    <div class="text-center">
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-success"><i class="icon-mail-check text-[22px]"></i></span>
      <h1 class="font-display font-bold text-[18px]">{t('account.forgot.sentTitle')}</h1>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('account.forgot.sent')}</p>
      <a href="/login" class="btn btn-outline w-full justify-center !text-[12.5px] mt-5">{t('account.forgot.back')}</a>
    </div>
  {:else}
    <h1 class="font-display font-bold text-[18px] text-center">{t('account.forgot.title')}</h1>
    <p class="text-[12.5px] mt-1.5 text-center text-tertiary">{t('account.forgot.subtitle')}</p>
    <form class="space-y-4 mt-5" onsubmit={submit} novalidate>
      {#if error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{error}</span>
        </div>
      {/if}
      <div>
        <label for="email" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('account.forgot.email')}</label>
        <div class="relative mt-1.5">
          <i class="icon-mail absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
          <input id="email" type="email" bind:value={email} autocomplete="email" placeholder={t('account.forgot.emailPlaceholder')} aria-invalid={!!fieldError} class="w-full ps-9 pe-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" />
        </div>
        {#if fieldError}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldError}</p>{/if}
      </div>
      <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
        {loading ? t('account.forgot.sending') : t('account.forgot.submit')}
      </button>
      <a href="/login" class="block text-center text-[12.5px] font-semibold text-primary-600">{t('account.forgot.back')}</a>
    </form>
  {/if}
</AuthCard>
