<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { accounting, todayISO, type Period } from '#lib/accounting/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let rows = $state<Period[]>([]);
  let loading = $state(true);
  let error = $state('');
  let pick = $state(todayISO());
  let busy = $state(false);
  let reopening = $state<Period | null>(null);
  let reason = $state('');

  const month = (iso: string) => iso.slice(0, 7); // YYYY-MM

  async function load() {
    loading = true;
    try {
      rows = await accounting.periods();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    document.title = t('accounting.periods.docTitle');
    void load();
  });

  async function close(m: string) {
    if (busy || !confirm(t('accounting.periods.closeConfirm', { period: m }))) return;
    busy = true;
    error = '';
    try {
      await accounting.closePeriod(m);
      await load();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = false;
    }
  }

  async function reopen(e: Event) {
    e.preventDefault();
    if (!reopening || busy) return;
    busy = true;
    error = '';
    try {
      await accounting.reopenPeriod(month(reopening.start_date), reason.trim());
      reopening = null;
      reason = '';
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  const inputClass = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-3xl mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('accounting.periods.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.periods.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  {#if can('accounting_periods', 'update')}
    <div class="surface-card flex flex-wrap items-end gap-3">
      <div>
        <div class="font-semibold text-[13px]">{t('accounting.periods.closeTitle')}</div>
        <div class="mt-1 w-44"><DatePicker bind:value={pick} clearable={false} /></div>
      </div>
      <button type="button" class="btn btn-primary" disabled={busy || !pick} onclick={() => close(month(pick))}><i class="icon-lock"></i> {t('accounting.periods.close')} {pick ? month(pick) : ''}</button>
      <p class="text-[11.5px] text-[var(--text-tertiary)] w-full">{t('accounting.periods.closePick')}</p>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <table class="w-full text-[12.5px]">
      <thead>
        <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
          <th class="px-4 py-3 text-start" scope="col">{t('accounting.periods.col.period')}</th>
          <th class="px-3 py-3 text-start" scope="col">{t('accounting.periods.col.status')}</th>
          <th class="px-3 py-3 text-start" scope="col">{t('accounting.periods.col.closedAt')}</th>
          <th class="px-3 py-3"></th>
        </tr>
      </thead>
      <tbody>
        {#each rows as p (p.id)}
          <tr class="border-b border-[var(--border-subtle)] last:border-0">
            <td class="px-4 py-2.5 font-mono">{month(p.start_date)}</td>
            <td class="px-3 py-2.5">
              <span class="badge-soft {p.status === 'closed' ? 'badge-danger' : 'badge-success'}">{p.status === 'closed' ? t('accounting.periods.closed') : t('accounting.periods.open')}</span>
            </td>
            <td class="px-3 py-2.5">{p.closed_at ? formatDateTime(p.closed_at, { dateStyle: 'short', timeStyle: 'short' }) : '—'}</td>
            <td class="px-3 py-2.5 text-end">
              {#if p.status === 'closed' && can('accounting_periods', 'approve')}
                <button type="button" class="btn btn-sm" onclick={() => (reopening = p)}>{t('accounting.periods.reopen')}</button>
              {:else if p.status === 'open' && can('accounting_periods', 'update')}
                <button type="button" class="btn btn-sm" disabled={busy} onclick={() => close(month(p.start_date))}>{t('accounting.periods.close')}</button>
              {/if}
            </td>
          </tr>
        {:else}
          <tr><td colspan="4" class="px-4 py-12 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.periods.empty')}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</main>

{#if reopening}
  <Modal title={t('accounting.periods.reopenTitle', { period: month(reopening.start_date) })} onclose={() => (reopening = null)}>
    <form class="space-y-3 text-[13px]" onsubmit={reopen}>
      <label class="block">
        <span class="font-semibold">{t('accounting.periods.reason')}</span>
        <input bind:value={reason} maxlength="200" class={inputClass} />
      </label>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={() => (reopening = null)} disabled={busy}>{t('accounting.periods.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={busy || reason.trim().length < 3}>{t('accounting.periods.reopenDo')}</button>
      </div>
    </form>
  </Modal>
{/if}
