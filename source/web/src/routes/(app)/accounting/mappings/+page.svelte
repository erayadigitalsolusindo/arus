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

  const GROUPS: { id: string; icon: string; keys: string[] }[] = [
    { id: 'sales', icon: 'shopping-cart', keys: ['sales_revenue', 'other_income', 'cogs', 'receivable', 'member_deposit'] },
    { id: 'purchase', icon: 'truck', keys: ['inventory', 'payable', 'supplier_advance', 'stock_variance'] },
    { id: 'tax', icon: 'percent', keys: ['tax_out', 'tax_in', 'fee_expense'] },
    { id: 'cash', icon: 'wallet', keys: ['petty_cash_expense', 'petty_cash_in', 'cash_over', 'cash_short'] }
  ];
  const known = new Set(GROUPS.flatMap((g) => g.keys));
  const grouped = $derived(
    [
      ...GROUPS.map((g) => ({ ...g, items: g.keys.map((k) => rows.find((m) => m.key === k)).filter((m): m is Mapping => !!m) })),
      { id: 'misc', icon: 'ellipsis', keys: [], items: rows.filter((m) => !known.has(m.key)) }
    ].filter((g) => g.items.length)
  );
  const mappedCount = $derived(rows.filter((m) => m.account).length);
  const pct = $derived(rows.length ? Math.round((mappedCount / rows.length) * 100) : 0);
  const isDirty = (m: Mapping) => !!picked[m.key] && picked[m.key] !== (m.account?.id ?? '');

  const dirty = $derived(rows.filter(isDirty));

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

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('accounting.mappings.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)] max-w-2xl">{t('accounting.mappings.subtitle')}</p>
    </div>
    {#if canEdit}
      <button type="button" class="btn btn-primary" disabled={busy || dirty.length === 0} onclick={save}>
        <i class="icon-save"></i> {busy ? t('accounting.accounts.saving') : t('accounting.accounts.save')}{dirty.length ? ` (${dirty.length})` : ''}
      </button>
    {/if}
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}

  {#if rows.length}
    <div class="surface-card p-5 flex flex-wrap items-center gap-x-6 gap-y-3">
      <div class="min-w-48">
        <div class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">{t('accounting.mappings.progress')}</div>
        <div class="font-display font-bold text-[20px] leading-tight">{t('accounting.mappings.progressOf', { done: mappedCount, total: rows.length })}</div>
      </div>
      <div class="flex-1 min-w-48">
        <div class="h-2 rounded-full bg-[var(--surface-sunken)] overflow-hidden"><div class="h-full rounded-full bg-[var(--color-primary-500)] transition-[width]" style="width:{pct}%"></div></div>
        <div class="text-[11.5px] mt-1.5 text-[var(--text-tertiary)]">{mappedCount === rows.length ? t('accounting.mappings.complete') : t('accounting.mappings.incomplete', { n: rows.length - mappedCount })}</div>
      </div>
    </div>
  {/if}

  {#each grouped as g (g.id)}
    <section class="surface-card !p-0 overflow-hidden">
      <header class="flex items-center gap-2.5 px-5 py-3.5 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
        <i class="icon-{g.icon} text-[16px] text-[var(--text-secondary)]"></i>
        <h2 class="font-semibold text-[13.5px]">{t(`accounting.mappings.groups.${g.id}` as MessageKey)}</h2>
        <span class="ms-auto text-[11.5px] text-[var(--text-tertiary)]">{g.items.filter((m) => m.account).length}/{g.items.length}</span>
      </header>
      <div class="divide-y divide-[var(--border-subtle)]">
        {#each g.items as m (m.key)}
          <div class="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_minmax(0,22rem)] gap-x-6 gap-y-2 px-5 py-4 items-center">
            <div class="min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-semibold text-[13px]">{t(`accounting.mappings.keys.${m.key}` as MessageKey)}</span>
                {#if isDirty(m)}
                  <span class="badge-soft">{t('accounting.mappings.changed')}</span>
                {:else if !m.account}
                  <span class="badge-soft badge-danger">{t('accounting.mappings.missing')}</span>
                {/if}
              </div>
              <div class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`accounting.mappings.hints.${m.key}` as MessageKey)}</div>
            </div>
            <Select bind:value={picked[m.key]} options={optionsFor(m)} disabled={!canEdit || busy} placeholder={t('accounting.mappings.unset')} />
          </div>
        {/each}
      </div>
    </section>
  {:else}
    <div class="surface-card text-center py-12 text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.mappings.empty')}</div>
  {/each}

  <p class="text-[11.5px] text-[var(--text-tertiary)] flex gap-2"><i class="icon-info text-[14px] shrink-0"></i>{t('accounting.mappings.methodNote')}</p>
</main>
