<script lang="ts">
  // Riwayat saldo titipan (deposit member / kredit pemasok): terbaru dulu, "Muat lebih banyak" memakai keyset id.
  import { t, formatCurrency, formatDateTime } from '#lib/i18n/index.ts';
  import type { WalletEntry } from '#lib/wallet/api.ts';

  let {
    entries,
    hasMore = false,
    loading = false,
    onmore
  }: { entries: WalletEntry[]; hasMore?: boolean; loading?: boolean; onmore?: () => void } = $props();

  const money = (v: string) => formatCurrency(Number(v), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
</script>

<div class="overflow-x-auto scroll-thin rounded-lg border border-[var(--border-subtle)]">
  <table class="w-full text-[12.5px] min-w-[640px]">
    <thead>
      <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)] bg-[var(--surface-sunken)]">
        <th class="p-2.5 text-start" scope="col">{t('deposits.history.time')}</th>
        <th class="p-2.5 text-start" scope="col">{t('deposits.history.kind')}</th>
        <th class="p-2.5 text-start" scope="col">{t('deposits.history.doc')}</th>
        <th class="p-2.5 text-end" scope="col">{t('deposits.history.amount')}</th>
        <th class="p-2.5 text-end" scope="col">{t('deposits.history.balance')}</th>
        <th class="p-2.5 text-start" scope="col">{t('deposits.history.by')}</th>
      </tr>
    </thead>
    <tbody>
      {#each entries as e (e.id)}
        <tr class="border-t border-[var(--border-subtle)] align-top">
          <td class="p-2.5 whitespace-nowrap">{formatDateTime(e.created_at)}</td>
          <td class="p-2.5">
            {t(`deposits.kind.${e.kind}`)}
            {#if e.method_name}<div class="text-[11px] text-[var(--text-tertiary)]">{e.method_name}{#if e.ref_no} · {e.ref_no}{/if}</div>{/if}
          </td>
          <td class="p-2.5 font-mono text-[11.5px]">
            {e.doc_no || '—'}
            {#if e.note}<div class="font-sans text-[11px] text-[var(--text-tertiary)]">{e.note}</div>{/if}
          </td>
          <td class="p-2.5 text-end font-semibold tabular-nums {Number(e.amount) < 0 ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{Number(e.amount) > 0 ? '+' : ''}{money(e.amount)}</td>
          <td class="p-2.5 text-end tabular-nums">{money(e.balance_after)}</td>
          <td class="p-2.5">
            {e.actor_name || '—'}
            {#if e.outlet_name}<div class="text-[11px] text-[var(--text-tertiary)]">{e.outlet_name}</div>{/if}
          </td>
        </tr>
      {:else}
        <tr><td colspan="6" class="p-5 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('deposits.empty')}</td></tr>
      {/each}
    </tbody>
  </table>
</div>
{#if hasMore && onmore}
  <div class="mt-2 text-center"><button type="button" class="btn !text-[12px]" disabled={loading} onclick={onmore}>{loading ? '…' : t('deposits.loadMore')}</button></div>
{/if}
