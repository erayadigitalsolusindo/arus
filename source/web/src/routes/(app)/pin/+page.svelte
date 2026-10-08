<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals } from '#lib/approval/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let hasPin = $state<boolean | null>(null);
  let password = $state('');
  let pin = $state('');
  let pin2 = $state('');
  let busy = $state(false);
  let error = $state('');
  let ok = $state(false);
  let fieldErr = $state<Record<string, string>>({});

  onMount(async () => {
    try {
      hasPin = (await approvals.pinStatus()).has_pin;
    } catch (e) {
      error = errorMessage(e);
    }
  });

  async function save(e: Event) {
    e.preventDefault();
    error = '';
    ok = false;
    fieldErr = {};
    if (pin !== pin2) {
      error = t('pos.pinPage.mismatch');
      return;
    }
    busy = true;
    try {
      await approvals.setPin(password, pin);
      hasPin = true;
      ok = true;
      password = pin = pin2 = '';
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') fieldErr = err.fields;
      else error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head><title>{t('pos.pinPage.docTitle')}</title></svelte:head>

<main class="p-4 lg:p-6 max-w-xl w-full mx-auto space-y-4">
  <header>
    <h1 class="font-display font-bold text-[20px]">{t('pos.pinPage.title')}</h1>
    <p class="text-[13px] text-[var(--text-secondary)] mt-1">{t('pos.pinPage.subtitle')}</p>
  </header>

  {#if hasPin !== null}
    <p class="text-[13px] font-semibold {hasPin ? 'text-[var(--color-success-600)]' : 'text-[var(--color-warning-600)]'}">
      {hasPin ? t('pos.pinPage.status.set') : t('pos.pinPage.status.unset')}
    </p>
  {/if}

  <form class="surface-card p-4 space-y-3" onsubmit={save} autocomplete="off">
    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.pinPage.password')}</span>
      <input type="password" bind:value={password} autocomplete="current-password" required class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)]" />
      {#if fieldErr.password}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.password)}</span>{/if}
    </label>
    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.pinPage.pin')}</span>
      <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" required class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
      {#if fieldErr.pin}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.pin)}</span>{/if}
    </label>
    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.pinPage.pin2')}</span>
      <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin2} autocomplete="new-password" required class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
    </label>
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    {#if ok}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]">{t('pos.pinPage.saved')}</p>{/if}
    <button type="submit" class="btn btn-primary" disabled={busy}>{t('pos.pinPage.save')}</button>
  </form>
</main>
