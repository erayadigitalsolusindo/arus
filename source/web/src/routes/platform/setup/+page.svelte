<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { ApiError, request } from '#lib/api/client.ts';
  import { startPlatformSession, type PlatformAdmin } from '#lib/platform/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import AuthCard from '#lib/components/AuthCard.svelte';

  let available = $state<boolean | null>(null);
  let setupToken = $state('');
  let name = $state('');
  let email = $state('');
  let password = $state('');
  let loading = $state(false);
  let error = $state('');
  let fields = $state<Record<string, string>>({});

  onMount(async () => {
    try {
      available = (await request<{ setup_required: boolean }>('/platform/setup/status', {}, null)).setup_required;
    } catch (err) {
      available = false;
      error = errorMessage(err);
    }
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    fields = {};
    loading = true;
    try {
      const res = await request<{ access_token: string; expires_in: number; admin: PlatformAdmin }>(
        '/platform/setup',
        { method: 'POST', body: JSON.stringify({ setup_token: setupToken, name, email: email.trim(), password }) },
        null
      );
      startPlatformSession(res);
      setupToken = password = '';
      await goto('/platform/tenants');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'NOT_FOUND') available = false;
      else if (err instanceof ApiError && err.code === 'VALIDATION') fields = err.fields;
      else error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  const inputClass = 'mt-1.5 w-full px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide text-tertiary';
</script>

<svelte:head><title>{t('platform.setup.title')} | ACIRABA</title></svelte:head>

<AuthCard>
  <h1 class="font-display font-bold text-[18px]">{t('platform.setup.title')}</h1>

  {#if available === null}
    <p class="text-[12.5px] mt-3 text-tertiary">…</p>
  {:else if !available}
    <p class="text-[12.5px] mt-3 text-tertiary">{error || t('platform.setup.unavailable')}</p>
    <a href="/platform/login" class="btn btn-outline mt-4 !text-[12.5px] justify-center w-full">{t('platform.login.submit')}</a>
  {:else}
    <p class="text-[12.5px] mt-1 mb-4 text-tertiary">{t('platform.setup.subtitle')}</p>
    <form class="space-y-3.5" onsubmit={submit} novalidate>
      {#if error}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{error}</span>
        </div>
      {/if}
      {#each [{ id: 'stoken', key: 'setup_token', label: 'platform.setup.token', type: 'password', value: 'setupToken' }, { id: 'sname', key: 'name', label: 'platform.setup.name', type: 'text', value: 'name' }, { id: 'semail', key: 'email', label: 'platform.setup.email', type: 'email', value: 'email' }, { id: 'spass', key: 'password', label: 'platform.setup.password', type: 'password', value: 'password' }] as f (f.id)}
        <div>
          <label for={f.id} class={labelClass}>{t(f.label as 'platform.setup.token')}</label>
          {#if f.value === 'setupToken'}
            <input id={f.id} type={f.type} bind:value={setupToken} autocomplete="off" class={inputClass} />
          {:else if f.value === 'name'}
            <input id={f.id} type={f.type} bind:value={name} autocomplete="name" class={inputClass} />
          {:else if f.value === 'email'}
            <input id={f.id} type={f.type} bind:value={email} autocomplete="username" class={inputClass} />
          {:else}
            <input id={f.id} type={f.type} bind:value={password} autocomplete="new-password" class={inputClass} />
          {/if}
          {#if fields[f.key]}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fieldMessage(fields[f.key])}</p>{/if}
        </div>
      {/each}
      <button type="submit" disabled={loading} class="btn btn-primary w-full justify-center !text-[13px] disabled:opacity-60">
        {t('platform.setup.submit')}<i class={loading ? 'icon-loader-circle animate-spin text-[13px]' : 'icon-arrow-right text-[13px]'}></i>
      </button>
    </form>
  {/if}
</AuthCard>
