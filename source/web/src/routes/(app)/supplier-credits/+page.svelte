<script lang="ts">
  // Kredit pemasok: saldo dari dana kembali retur pembelian yang dijadikan kredit. Dipakai bayar hutang (metode "Kredit
  // Pemasok" di Daftar Hutang) atau dicairkan oleh pemasok di sini. Saldo dihitung server dari ledger.
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import WalletHistory from '#lib/components/WalletHistory.svelte';
  import WalletCashModal from '#lib/components/WalletCashModal.svelte';
  import { supplierCredits as api, type CreditRow, type WalletAccount } from '#lib/wallet/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const money = (v: string) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const inputClass = 'h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';

  let rows = $state<CreditRow[]>([]);
  let nextCursor = '';
  let hasMore = $state(false);
  let loading = $state(true);
  let error = $state('');
  let q = $state('');
  let positive = $state(true);

  let open = $state<WalletAccount | null>(null);
  let openLoading = $state(false);
  let openError = $state('');
  let cashOut = $state(false);
  let saved = $state('');
  const canCashOut = $derived(can('supplier_credits', 'create'));

  let seq = 0;
  async function load(more = false) {
    const mine = ++seq;
    loading = true;
    error = '';
    try {
      const res = await api.list({ q: q.trim(), positive, cursor: more ? nextCursor : '', limit: 50 });
      if (mine !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      hasMore = res.has_more;
      nextCursor = res.next_cursor;
    } catch (e) {
      if (mine === seq) error = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void q;
    void positive;
    void session.outlet?.id;
    const h = setTimeout(() => void load(), 250);
    return () => clearTimeout(h);
  });

  async function show(id: string, before = 0) {
    openLoading = true;
    openError = '';
    try {
      const acc = await api.account(id, before);
      open = before && open ? { ...acc, entries: [...open.entries, ...acc.entries] } : acc;
    } catch (e) {
      openError = errorMessage(e);
    } finally {
      openLoading = false;
    }
  }

  onMount(() => {
    document.title = t('supplierCredits.docTitle');
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="max-w-3xl">
    <h1 class="font-display font-bold text-[19px]">{t('supplierCredits.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('supplierCredits.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('supplierCredits.loadFailed')} {error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-3 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative w-full sm:w-80">
        <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.75rem"></i>
        <input type="search" class={inputClass} style="padding-inline-start:2rem" placeholder={t('supplierCredits.search')} aria-label={t('supplierCredits.search')} bind:value={q} maxlength="100" />
      </div>
      <label class="inline-flex items-center gap-2 text-[12.5px]">
        <input type="checkbox" bind:checked={positive} />{t('supplierCredits.onlyPositive')}
      </label>
    </div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[13px]">
        <thead>
          <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="px-4 py-3 text-start" scope="col">{t('supplierCredits.col.supplier')}</th>
            <th class="px-4 py-3 text-end" scope="col">{t('supplierCredits.col.balance')}</th>
            <th class="px-4 py-3" scope="col"><span class="sr-only">{t('supplierCredits.detail')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.supplier_id)}
            <tr class="border-b border-[var(--border-subtle)] last:border-0">
              <td class="px-4 py-2.5"><div class="font-medium">{r.name}</div>{#if r.code}<div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.code}</div>{/if}</td>
              <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{money(r.balance)}</td>
              <td class="px-4 py-2.5 text-end"><button type="button" class="btn btn-sm" onclick={() => { saved = ''; void show(r.supplier_id); }}>{t('supplierCredits.detail')}</button></td>
            </tr>
          {:else}
            <tr><td colspan="3" class="px-4 py-12 text-center text-[12.5px] text-[var(--text-tertiary)]">{loading ? '…' : t('supplierCredits.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if hasMore}
      <div class="p-3 text-center"><button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(true)}>{t('supplierCredits.loadMore')}</button></div>
    {/if}
  </section>
</main>

{#if open}
  <Modal title={open.name} onclose={() => (open = null)} wide>
    <div class="space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="text-[12px] text-[var(--text-tertiary)]">{t('supplierCredits.col.balance')}</p>
          <p class="font-display font-bold text-[22px] tabular-nums">{money(open.balance)}</p>
        </div>
        {#if canCashOut}
          <button type="button" class="btn btn-primary btn-sm" disabled={Number(open.balance) <= 0} onclick={() => (cashOut = true)}>{t('supplierCredits.cashOut.action')}</button>
        {/if}
      </div>
      {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}
      {#if openError}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{openError}</p>{/if}
      <WalletHistory entries={open.entries} hasMore={open.has_more} loading={openLoading} onmore={() => open && show(open.owner_id, open.next_before)} />
    </div>
  </Modal>
{/if}

{#if open && cashOut}
  {@const acc = open}
  <WalletCashModal
    title={t('supplierCredits.cashOut.title')}
    hint={t('supplierCredits.cashOut.hint')}
    balance={acc.balance}
    max={acc.balance}
    onsubmit={(input, key) => api.cashOut(acc.owner_id, input, key)}
    onclose={() => (cashOut = false)}
    ondone={(next, doc) => {
      cashOut = false;
      open = next;
      saved = t('deposits.form.saved', { doc });
      void load();
    }}
  />
{/if}
