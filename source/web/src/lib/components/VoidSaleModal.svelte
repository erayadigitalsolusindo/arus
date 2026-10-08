<script lang="ts">
  // Batalkan nota: alasan + penyetuju (izin "Setujui Edit/Batal Nota") + PIN-nya. Server membalik stok, poin, dan kupon.
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import { sales } from '#lib/sales/api.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let { saleId, docNo, onclose, ondone }: { saleId: string; docNo: string; onclose: () => void; ondone: () => void } = $props();

  let reason = $state('');
  let approverId = $state('');
  let pin = $state('');
  let approvers = $state<Approver[]>([]);
  let loaded = $state(false);
  let working = $state(false);
  let error = $state('');

  const ready = $derived(reason.trim().length >= 3 && approverId !== '' && /^[0-9]{6}$/.test(pin));
  const options = $derived([{ value: '', label: t('sales.detail.void.approverPlaceholder'), disabled: true }, ...approvers.map((a) => ({ value: a.id, label: a.name }))]);

  onMount(() => {
    approvals
      .approvers('sale_edit')
      .then((r) => (approvers = r))
      .catch(() => {})
      .finally(() => (loaded = true));
  });

  async function submit() {
    if (!ready || working) return;
    working = true;
    error = '';
    try {
      await sales.void(saleId, { reason: reason.trim(), approval: { user_id: approverId, pin } });
      ondone();
    } catch (e) {
      error = errorMessage(e);
      pin = '';
    } finally {
      working = false;
    }
  }

  const label = 'mb-1 block text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const input = 'h-10 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-[13px] outline-none focus:border-[var(--color-primary-500)]';
</script>

<Modal title={t('sales.detail.void.title', { docNo })} {onclose}>
  <form
    class="space-y-3"
    onsubmit={(e) => {
      e.preventDefault();
      void submit();
    }}
  >
    <p class="rounded-lg bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px] text-[var(--text-secondary)]">{t('sales.detail.void.hint')}</p>
    <div>
      <label class={label} for="void-reason">{t('sales.detail.void.reason')}</label>
      <input id="void-reason" class={input} bind:value={reason} maxlength="200" autocomplete="off" placeholder={t('sales.detail.void.reasonPlaceholder')} />
    </div>
    {#if loaded && approvers.length === 0}
      <p class="text-[12.5px] text-[var(--color-danger-600)]">{t('sales.detail.void.noApprover')}</p>
    {:else}
      <div class="grid grid-cols-[minmax(0,1fr)_130px] gap-2">
        <div>
          <span class={label}>{t('sales.detail.void.approver')}</span>
          <Select bind:value={approverId} {options} ariaLabel={t('sales.detail.void.approver')} />
        </div>
        <div>
          <label class={label} for="void-pin">{t('sales.detail.void.pin')}</label>
          <input id="void-pin" class="{input} text-center tracking-widest" type="password" inputmode="numeric" autocomplete="off" maxlength="6" bind:value={pin} />
        </div>
      </div>
    {/if}
    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    <div class="flex justify-end gap-2 pt-1">
      <button type="button" class="btn" onclick={onclose} disabled={working}>{t('sales.detail.void.cancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={!ready || working}>
        <i class="icon-ban me-1.5"></i>{working ? t('sales.detail.void.working') : t('sales.detail.void.confirm')}
      </button>
    </div>
  </form>
</Modal>
