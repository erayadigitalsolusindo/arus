<script lang="ts">
  import { onMount } from 'svelte';
  import Select from '#lib/components/Select.svelte';
  import { accounting, type Account, type Mapping } from '#lib/accounting/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let accounts = $state<Account[]>([]);
  let rows = $state<Mapping[]>([]);
  let picked = $state<Record<string, string>>({});
  let loading = $state(true);
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');

  const canEdit = $derived(can('accounts', 'update'));

  function apply(items: Mapping[]) {
    rows = items;
    picked = Object.fromEntries(items.map((m) => [m.key, m.account?.id ?? '']));
  }

  async function load() {
    loading = true;
    error = '';
    try {
      const [accs, maps] = await Promise.all([accounting.accounts(), accounting.mappings()]);
      accounts = accs;
      apply(maps);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    document.title = t('accounting.mappings.docTitle');
    void load();
  });

  // Hanya akun buku aktif dengan kelas yang cocok untuk peran itu (server memeriksa ulang).
  const optionsFor = (m: Mapping) =>
    accounts.filter((a) => a.kind === 'ledger' && a.active && m.classes.includes(a.class)).map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` }));

  const dirty = $derived(rows.filter((m) => picked[m.key] && picked[m.key] !== (m.account?.id ?? '')));

  async function save() {
    if (busy || dirty.length === 0) return;
    busy = true;
    error = '';
    notice = '';
    try {
      apply(await accounting.updateMappings(Object.fromEntries(dirty.map((m) => [m.key, picked[m.key]]))));
      notice = t('accounting.mappings.saved');
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = false;
    }
  }
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-3xl mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.mappings.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.mappings.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <table class="w-full text-[12.5px]">
      <thead>
        <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
          <th class="px-4 py-3 text-start" scope="col">{t('accounting.mappings.col.role')}</th>
          <th class="px-3 py-3 text-start" scope="col">{t('accounting.mappings.col.account')}</th>
        </tr>
      </thead>
      <tbody>
        {#each rows as m (m.key)}
          <tr class="border-b border-[var(--border-subtle)] last:border-0 align-top">
            <td class="px-4 py-2.5">
              <div class="font-semibold">{t(`accounting.mappings.keys.${m.key}` as MessageKey)}</div>
              <div class="text-[11.5px] text-[var(--text-tertiary)]">{t(`accounting.mappings.hints.${m.key}` as MessageKey)}</div>
            </td>
            <td class="px-3 py-2.5 w-72">
              <Select bind:value={picked[m.key]} options={optionsFor(m)} disabled={!canEdit || busy} placeholder={t('accounting.mappings.unset')} />
            </td>
          </tr>
        {:else}
          <tr><td colspan="2" class="px-4 py-12 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.mappings.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.mappings.methodNote')}</p>

  {#if canEdit}
    <div class="flex justify-end">
      <button type="button" class="btn btn-primary" disabled={busy || dirty.length === 0} onclick={save}>{busy ? t('accounting.accounts.saving') : t('accounting.accounts.save')}</button>
    </div>
  {/if}
</main>
