<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type SalesReportRow, type SalesMonth, type ExpiringRow, type OutletLocation } from '#lib/platform/api.ts';
  import { t, formatNumber, formatDate, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  // Uang platform selalu Rupiah (nilai transaksi tenant), tanpa desimal.
  const rp = (n: number) => 'Rp ' + formatNumber(Math.round(n), { maximumFractionDigits: 0 });
  const th = 'px-4 py-2.5 font-semibold';
  const DAY = 86_400_000;

  let error = $state('');
  let loading = $state(true);
  // ---- Paket berakhir ----
  let days = $state(30);
  let expiring = $state<ExpiringRow[]>([]);
  const left = (iso: string) => Math.ceil((new Date(iso).getTime() - Date.now()) / DAY);
  const groups = $derived([
    { key: 'expired', title: t('platform.reports.expired'), cls: 'badge-danger', rows: expiring.filter((r) => left(r.expires_at) <= 0) },
    { key: 'soon', title: t('platform.reports.soon'), cls: 'badge-warning', rows: expiring.filter((r) => left(r.expires_at) > 0 && left(r.expires_at) <= 7) },
    { key: 'later', title: t('platform.reports.later'), cls: 'badge-info', rows: expiring.filter((r) => left(r.expires_at) > 7) }
  ].filter((g) => g.rows.length));

  function leftText(iso: string) {
    const n = left(iso);
    return n > 0 ? t('platform.reports.daysLeft', { n }) : n === 0 ? t('platform.reports.today') : t('platform.reports.daysAgo', { n: -n });
  }

  async function loadExpiring() {
    loading = true;
    error = '';
    try {
      expiring = await platformApi.expiringReport(days);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  onMount(loadExpiring);
</script>

<div>
  <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.reports.tabExpiring')}</h1>
  <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.reports.sub')}</p>
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

  <div class="surface-card p-3 sm:p-4 flex flex-wrap items-center gap-2">
    <span class="text-[12px] font-medium me-1">{t('platform.reports.window')}</span>
    {#each [7, 14, 30, 60, 90] as n (n)}
      <button type="button" class="btn btn-sm {days === n ? 'btn-primary' : 'btn-secondary'}" onclick={() => { days = n; void loadExpiring(); }}>{t('platform.reports.days', { n })}</button>
    {/each}
  </div>
  <p class="text-[11.5px] -mt-2 text-[var(--text-tertiary)]">{t('platform.reports.expHint')}</p>

  {#if loading}
    <div class="h-24 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>
  {:else if expiring.length === 0}
    <div class="surface-card p-10 text-center text-[var(--text-tertiary)]">{t('platform.reports.expEmpty')}</div>
  {:else}
    {#each groups as grp (grp.key)}
      <div class="surface-card !p-0 overflow-hidden">
        <div class="p-3 sm:p-4 border-b border-[var(--border-subtle)] flex items-center gap-2 font-semibold text-[13px]">
          <span class="rounded-full px-2 py-0.5 text-[11.5px] {grp.cls}">{grp.rows.length}</span>{grp.title}
        </div>
        <div class="overflow-x-auto scroll-thin">
          <table class="w-full text-[12.5px] min-w-[900px]">
            <thead>
              <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
                <th class="{th} text-start" scope="col">{t('platform.reports.tenant')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.plan')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.expiresOn')}</th>
                <th class="{th} text-end" scope="col">{t('platform.reports.rev30')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.lastLogin')}</th>
                <th class="{th} text-start" scope="col">{t('platform.reports.owner')}</th>
              </tr>
            </thead>
            <tbody>
              {#each grp.rows as r (r.tenant_id)}
                <tr class="border-t border-[var(--border-subtle)]">
                  <td class="px-4 py-2.5">
                    <a href="/platform/tenants/{r.tenant_id}" class="font-semibold text-[var(--color-primary-700)] hover:underline">{r.name}</a>
                    <div class="text-[11.5px] text-[var(--text-tertiary)]">{r.outlet_count} {t('platform.reports.outlets').toLowerCase()} · {r.user_count} {t('platform.reports.users').toLowerCase()}{#if !r.active} · {t('platform.reports.inactive')}{/if}</div>
                  </td>
                  <td class="px-4 py-2.5">
                    <span class="font-semibold">{r.plan_name}</span>
                    <div class="text-[11.5px] text-[var(--text-tertiary)]">{Number(r.plan_price) > 0 ? rp(Number(r.plan_price)) : '—'}{#if r.note} · {r.note}{/if}</div>
                  </td>
                  <td class="px-4 py-2.5">
                    <div>{formatDateTime(r.expires_at)}</div>
                    <span class="rounded-full px-1.5 py-0.5 text-[10.5px] {grp.cls}">{leftText(r.expires_at)}</span>
                  </td>
                  <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{rp(Number(r.revenue_30d))}</td>
                  <td class="px-4 py-2.5 text-[var(--text-tertiary)]">{r.last_login_at ? relativeTime(r.last_login_at) : t('platform.reports.never')}</td>
                  <td class="px-4 py-2.5">{#if r.owner_email}<a href="mailto:{r.owner_email}" class="hover:underline">{r.owner_email}</a>{:else}—{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/each}
  {/if}
