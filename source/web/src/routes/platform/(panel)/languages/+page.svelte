<script lang="ts">
  import { platformApi, type LanguageRow } from '#lib/platform/api.ts';
  import { t } from '#lib/i18n/index.ts';

  let languages = $state<LanguageRow[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');

  // Modal state
  let showModal = $state(false);
  let editing = $state<LanguageRow | null>(null);
  let form = $state({ code: '', name: '', short: '', intl: '', sort_order: 0, active: true });
  let saving = $state(false);

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await platformApi.languages();
      languages = res.items;
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  /** Normalisasi input kode/singkat/intl: NFKC mengubah full-width IME (ｊａ) jadi ASCII (ja), lalu lowercase. */
  function normCode(s: string) {
    return s.normalize('NFKC').trim().toLowerCase();
  }
  /** Intl distandarkan ke xx-XX (region uppercase) kendati diketik ja-jp / ja_JP / full-width. */
  function normIntl(s: string) {
    const n = s.normalize('NFKC').trim();
    const m = n.match(/^([a-z]{2})[-–—_\s]([a-z]{2})$/i);
    return m ? `${m[1].toLowerCase()}-${m[2].toUpperCase()}` : n.replace(/[-–—_\s]/g, '-');
  }

  function validate(f: typeof form): string {
    if (!/^[a-z]{2}$/.test(f.code)) return t('platform.languages.codeHelp');
    if (f.name.trim().length < 1 || f.name.trim().length > 100) return t('platform.languages.name');
    if (f.short.trim().length < 2 || f.short.trim().length > 6) return t('platform.languages.short');
    if (!/^[a-z]{2}-[A-Z]{2}$/.test(normIntl(f.intl))) return t('platform.languages.intlHelp');
    return '';
  }

  async function save() {
    saving = true;
    notice = '';
    try {
      if (editing) {
        await platformApi.updateLanguage(editing.code, { name: form.name.trim(), active: form.active, sort_order: form.sort_order });
        notice = t('platform.languages.saved');
      } else {
        const body = {
          code: normCode(form.code),
          name: form.name.trim(),
          short: form.short.trim().toUpperCase(),
          intl: normIntl(form.intl),
          sort_order: form.sort_order
        };
        const problem = validate(body as typeof form);
        if (problem) throw new Error(problem);
        await platformApi.createLanguage(body);
        notice = t('platform.languages.created');
      }
      showModal = false;
      editing = null;
      await load();
    } catch (e) {
      error = String(e);
    } finally {
      saving = false;
    }
  }

  async function remove(code: string) {
    if (!confirm(t('platform.languages.deleteConfirm'))) return;
    error = '';
    try {
      await platformApi.deleteLanguage(code);
      notice = t('platform.languages.deleted');
      await load();
    } catch (e) {
      error = String(e);
    }
  }

  function openCreate() {
    editing = null;
    form = { code: '', name: '', short: '', intl: '', sort_order: languages.length + 1, active: true };
    showModal = true;
  }

  function openEdit(lang: LanguageRow) {
    editing = lang;
    form = { code: lang.code, name: lang.name, short: lang.short, intl: lang.intl, sort_order: lang.sort_order, active: lang.active };
    showModal = true;
  }

  $effect(() => { load(); });
</script>

<svelte:head><title>{t('platform.languages.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex items-center justify-between gap-4">
    <div>
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.languages.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)]">{t('platform.languages.subtitle')}</p>
    </div>
    <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}>
      <i class="icon-plus text-[13px]"></i>{t('platform.languages.add')}
    </button>
  </div>

  {#if notice}
    <div class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]">{notice}</div>
  {/if}
  {#if error}
    <div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]">{error}</div>
  {/if}

  {#if loading}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.languages.loading')}</div>
  {:else if languages.length === 0}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('platform.languages.empty')}</div>
  {:else}
    <div class="surface-card overflow-hidden">
      <table class="w-full text-[13px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.code')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.name')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.short')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.intl')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.status')}</th>
            <th class="px-4 py-2.5 text-start font-semibold">{t('platform.languages.order')}</th>
            <th class="px-4 py-2.5 text-end font-semibold">{t('platform.languages.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {#each languages as lang (lang.code)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--surface-sunken)]">
              <td class="px-4 py-2.5 font-medium">{lang.code}</td>
              <td class="px-4 py-2.5">{lang.name}</td>
              <td class="px-4 py-2.5 text-[var(--text-secondary)]">{lang.short}</td>
              <td class="px-4 py-2.5 text-[var(--text-secondary)]">{lang.intl}</td>
              <td class="px-4 py-2.5">
                {#if lang.protected}
                  <span class="badge-info">{t('platform.languages.protected')}</span>
                {:else if lang.active}
                  <span class="badge-success">{t('platform.languages.active')}</span>
                {:else}
                  <span class="badge-warning">{t('platform.languages.inactive')}</span>
                {/if}
              </td>
              <td class="px-4 py-2.5 text-[var(--text-secondary)]">{lang.sort_order}</td>
              <td class="px-4 py-2.5 text-end">
                <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => openEdit(lang)}>{t('platform.languages.edit')}</button>
                {#if !lang.protected}
                  <button type="button" class="btn btn-outline !text-[12px] !py-1 text-[var(--color-danger-600)]" onclick={() => remove(lang.code)}>{t('platform.languages.delete')}</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

{#if showModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" role="dialog" aria-modal="true">
    <div class="surface-card w-full max-w-md rounded-xl shadow-xl">
      <div class="flex items-center justify-between border-b border-[var(--border-subtle)] px-5 py-4">
        <h2 class="font-semibold text-[15px]">{editing ? t('platform.languages.editTitle') : t('platform.languages.addTitle')}</h2>
        <button type="button" aria-label={t('platform.languages.close')} class="header-icon-btn !size-7" onclick={() => (showModal = false)}><i class="icon-x text-[14px]"></i></button>
      </div>
      <form onsubmit={(e) => { e.preventDefault(); save(); }} novalidate class="p-5 space-y-4">
        {#if !editing}
          <div>
            <label for="lang-code" class="block text-[12.5px] font-medium mb-1.5">{t('platform.languages.code')}</label>
            <input id="lang-code" type="text" bind:value={form.code} maxlength="2" class="form-input !text-[13px]" placeholder="ja" autocomplete="off" name="lang-code" />
            <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.languages.codeHelp')}</p>
          </div>
          <div>
            <label for="lang-short" class="block text-[12.5px] font-medium mb-1.5">{t('platform.languages.short')}</label>
            <input id="lang-short" type="text" bind:value={form.short} maxlength="6" class="form-input !text-[13px]" placeholder="JA" />
          </div>
          <div>
            <label for="lang-intl" class="block text-[12.5px] font-medium mb-1.5">{t('platform.languages.intl')}</label>
            <input id="lang-intl" type="text" bind:value={form.intl} maxlength="6" class="form-input !text-[13px]" placeholder="ja-JP" />
            <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.languages.intlHelp')}</p>
          </div>
        {/if}
        <div>
          <label for="lang-name" class="block text-[12.5px] font-medium mb-1.5">{t('platform.languages.name')}</label>
          <input id="lang-name" type="text" bind:value={form.name} maxlength="100" required class="form-input !text-[13px]" />
        </div>
        <div>
          <label for="lang-order" class="block text-[12.5px] font-medium mb-1.5">{t('platform.languages.order')}</label>
          <input id="lang-order" type="number" bind:value={form.sort_order} min="0" class="form-input !text-[13px]" />
        </div>
        {#if editing}
          <div class="flex items-center gap-2">
            <input type="checkbox" id="lang-active" bind:checked={form.active} class="form-checkbox" />
            <label for="lang-active" class="text-[13px]">{t('platform.languages.active')}</label>
          </div>
        {/if}
        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showModal = false)}>{t('platform.languages.cancel')}</button>
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving}>{saving ? t('platform.languages.saving') : t('platform.languages.save')}</button>
        </div>
      </form>
    </div>
  </div>
{/if}
