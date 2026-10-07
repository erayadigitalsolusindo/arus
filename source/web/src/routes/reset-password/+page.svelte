<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkPassword } from '#lib/validation.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  // Token ada di fragmen URL (#token=...): tidak ikut terkirim ke server atau header Referer. Disimpan di memori
  // lalu fragmen dihapus dari bilah alamat/riwayat.
  let token = $state('');
  let ready = $state(false);

  let password = $state('');
  let confirm = $state('');
  let show = $state(false);
  let loading = $state(false);
  let done = $state(false);
  let error = $state('');
  let fieldError = $state('');
  let confirmError = $state('');
  let tokenBad = $state(false);

  onMount(() => {
    token = new URLSearchParams(location.hash.slice(1)).get('token') ?? '';
    if (token) history.replaceState(null, '', location.pathname);
    ready = true;
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = fieldError = confirmError = '';
    const code = checkPassword(password, '');
    if (code) {
      fieldError = fieldMessage(code) ?? '';
      return;
    }
    if (confirm !== password) {
      confirmError = t('account.reset.mismatch');
      return;
    }
    loading = true;
    try {
      await api('/auth/reset-password', { method: 'POST', body: JSON.stringify({ token, password }) });
      password = confirm = '';
      done = true;
    } catch (err) {
      if (err instanceof ApiError && err.code === 'INVALID_TOKEN') tokenBad = true;
      else if (err instanceof ApiError && err.code === 'VALIDATION' && err.fields.password) fieldError = fieldMessage(err.fields.password) ?? '';
      else error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head><title>{t('account.reset.docTitle')}</title></svelte:head>

<AuthCard>
  {#if !ready}
    <p class="text-center text-[12.5px] text-tertiary">…</p>
  {:else if done}
    <div class="text-center">
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-success"><i class="icon-circle-check text-[22px]"></i></span>
      <h1 class="font-display font-bold text-[18px]">{t('account.reset.successTitle')}</h1>
      <p class="text-[12.5px] mt-2 text-tertiary">{t('account.reset.success')}</p>
      <a href="/login" class="btn btn-primary w-full justify-center !text-[13px] mt-5">{t('account.reset.login')}</a>
    </div>
  {:else if !token || tokenBad}
    <div class="text-center">
      <span class="grid place-items-center size-12 rounded-full mx-auto mb-3 badge-danger"><i class="icon-link-2-off text-[22px]"></i></span>
      <p class="text-[12.5px] text-tertiary">{tokenBad ? t('errors.INVALID_TOKEN') : t('account.reset.missing')}</p>
      <a href="/forgot-password" class="btn btn-primary w-full justify-center !text-[13px] mt-5">{t('account.reset.requestNew')}</a>
    </div>
  {:else}
    <h1 class="font-display font-bold text-[18px] text-center">{t('account.reset.title')}</h1>
    <p class="text-[12.5px] mt-1.5 text-center text-tertiary">{t('account.reset.subtitle')}</p>
    <form class="space-y-4 mt-5" onsubmit={submit} novalidate>
      {#if error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{error}</span>
        </div>
      {/if}
      <div>
        <label for="pw" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('account.reset.password')}</label>
        <div class="relative mt-1.5">
          <i class="icon-lock absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
          <input id="pw" type={show ? 'text' : 'password'} bind:value={password} autocomplete="new-password" maxlength="128" aria-invalid={!!fieldError} class="w-full ps-9 pe-9 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" />
          <button type="button" class="absolute top-1/2 -translate-y-1/2 end-3 text-tertiary" aria-label={show ? t('auth.login.hidePassword') : t('auth.login.showPassword')} onclick={() => (show = !show)}>
            <i class={show ? 'icon-eye-off text-[14px]' : 'icon-eye text-[14px]'}></i>
          </button>
        </div>
        {#if fieldError}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldError}</p>{/if}
      </div>
      <div>
        <label for="pw2" class="text-[11.5px] font-semibold uppercase tracking-wide text-tertiary">{t('account.reset.confirm')}</label>
        <div class="relative mt-1.5">
          <i class="icon-lock absolute top-1/2 -translate-y-1/2 start-3 text-[14px] text-tertiary"></i>
          <input id="pw2" type={show ? 'text' : 'password'} bind:value={confirm} autocomplete="new-password" maxlength="128" aria-invalid={!!confirmError} class="w-full ps-9 pe-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered" />
        </div>
        {#if confirmError}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{confirmError}</p>{/if}
      </div>
      <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
        {loading ? t('account.reset.saving') : t('account.reset.submit')}
      </button>
    </form>
  {/if}
</AuthCard>
