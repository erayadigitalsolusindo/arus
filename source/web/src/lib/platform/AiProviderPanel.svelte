<script lang="ts">
  // "API ARUS": penyedia AI milik platform untuk akun gratis HalloArus. Hanya Platform Admin.
  // Key tidak pernah dikembalikan server (hanya 4 karakter terakhir); mengganti base URL mewajibkan key baru.
  import { onMount } from 'svelte';
  import { platformApi, type AiProvider } from '#lib/platform/api.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let info = $state<AiProvider | null>(null);
  let baseUrl = $state('');
  let model = $state('');
  let apiKey = $state('');
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let notice = $state('');

  const configured = $derived(info?.configured === true);
  const baseChanged = $derived(configured && baseUrl.trim().replace(/\/+$/, '') !== info?.base_url);
  const canSave = $derived(!saving && baseUrl.trim() !== '' && model.trim() !== '' && (apiKey.trim() !== '' || (configured && !baseChanged)));

  function fill(p: AiProvider) {
    info = p;
    baseUrl = p.base_url;
    model = p.model;
    apiKey = '';
  }

  async function load() {
    loading = true;
    try {
      fill(await platformApi.aiProvider());
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  async function save() {
    if (!canSave) return;
    saving = true;
    error = notice = '';
    try {
      await platformApi.saveAiProvider({ base_url: baseUrl.trim(), model: model.trim(), api_key: apiKey.trim() });
      notice = t('platform.ai.provider.saved');
      fill(await platformApi.aiProvider());
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (saving || !confirm(t('platform.ai.provider.removeConfirm'))) return;
    saving = true;
    error = notice = '';
    try {
      await platformApi.deleteAiProvider();
      notice = t('platform.ai.provider.removed');
      fill(await platformApi.aiProvider());
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }
</script>

<section class="surface-card !p-4 grid max-w-2xl gap-3">
  <div>
    <h2 class="text-[14px] font-semibold">{t('platform.ai.provider.title')}</h2>
    <p class="mt-1 text-[12.5px] text-[var(--text-secondary)]">{t('platform.ai.provider.hint')}</p>
  </div>

  {#if notice}<div class="rounded-lg border border-[var(--color-success-500)]/20 bg-[var(--color-success-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-success-600)]" role="status">{notice}</div>{/if}
  {#if error}<div class="rounded-lg border border-[var(--color-danger-500)]/20 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]" role="alert">{error}</div>{/if}

  {#if loading}
    <div class="h-40 animate-pulse rounded-xl bg-[var(--surface-sunken)]"></div>
  {:else}
    <p class="text-[12.5px] font-medium {configured ? 'text-[var(--color-success-600)]' : 'text-[var(--text-tertiary)]'}">
      <i class={configured ? 'icon-circle-check' : 'icon-circle-dashed'}></i>
      {configured ? t('platform.ai.provider.on') : t('platform.ai.provider.off')}
    </p>

    <form class="grid gap-3" onsubmit={(e) => { e.preventDefault(); void save(); }}>
      <label class="grid gap-1 text-[12.5px] font-medium">{t('platform.ai.provider.base')}
        <input bind:value={baseUrl} maxlength="300" autocomplete="off" spellcheck="false" inputmode="url" placeholder="https://openrouter.ai/api/v1" />
      </label>
      <label class="grid gap-1 text-[12.5px] font-medium">{t('platform.ai.provider.model')}
        <input bind:value={model} maxlength="100" autocomplete="off" spellcheck="false" placeholder="meta-llama/llama-3.3-70b-instruct:free" />
      </label>
      <label class="grid gap-1 text-[12.5px] font-medium">{t('platform.ai.provider.key')}
        <input type="password" bind:value={apiKey} maxlength="300" autocomplete="new-password" spellcheck="false"
          placeholder={configured ? t('platform.ai.provider.keyHolder') : t('platform.ai.provider.keyHolderNew')} />
        {#if configured}<small class="font-normal text-[var(--text-tertiary)]">{t('platform.ai.provider.keyKept', { hint: info?.key_hint ?? '' })}</small>{/if}
      </label>

      <div class="flex flex-wrap items-center gap-2">
        <button type="submit" class="btn btn-primary btn-sm" disabled={!canSave}>
          <i class="icon-plug-zap"></i>{t('platform.ai.provider.save')}
        </button>
        {#if configured}
          <button type="button" class="btn btn-sm" disabled={saving} onclick={remove}><i class="icon-trash-2"></i>{t('platform.ai.provider.remove')}</button>
          <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.provider.by', { who: info?.author || '—', when: info ? formatDateTime(info.updated_at) : '' })}</span>
        {/if}
      </div>
    </form>
    <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.ai.provider.quota')}</p>
  {/if}
</section>
