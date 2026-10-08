<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';

  let {
    outletId,
    outletName,
    onclose,
    onapprove
  }: {
    outletId: string;
    outletName: string;
    onclose: () => void;
    /** Dipanggil dengan penyetuju + PIN; pemanggil yang mengirimnya ke server (server memverifikasi di sana). */
    onapprove: (approver: Approver, pin: string) => Promise<void>;
  } = $props();

  let approverId = $state('');
  let pin = $state('');
  let list = $state<Approver[] | null>(null);
  let busy = $state(false);
  let error = $state('');

  onMount(async () => {
    try {
      list = await approvals.approvers('outlet_switch', outletId);
      if (list.length === 1) approverId = list[0].id;
    } catch (e) {
      error = errorMessage(e);
      list = [];
    }
  });

  const valid = $derived(approverId !== '' && /^\d{6}$/.test(pin));

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      await onapprove(list!.find((a) => a.id === approverId)!, pin);
    } catch (err) {
      pin = '';
      error = err instanceof ApiError && err.code === 'PIN_LOCKED' ? errorMessage(err).replace('{count}', String(err.retryAfter)) : errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('pos.outletApproval.title')} {onclose}>
  <form class="space-y-3" onsubmit={submit} autocomplete="off">
    <p class="text-[12.5px] text-[var(--text-secondary)]">{t('pos.outletApproval.body')}</p>
    <dl class="text-[13px] rounded-md bg-[var(--surface-sunken)] p-3">
      <div class="flex justify-between gap-3"><dt>{t('pos.outletApproval.target')}</dt><dd class="font-semibold text-end">{outletName}</dd></div>
    </dl>

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.approver')}</span>
      <div class="mt-1"><Select bind:value={approverId} class="!min-h-10" options={[{ value: '', label: t('pos.override.pickApprover'), disabled: true }, ...(list ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
      {#if list && list.length === 0}<span class="text-[12px] text-[var(--color-warning-600)]">{t('pos.outletApproval.noApprovers')}</span>{/if}
    </label>

    <label class="block text-[13px]">
      <span class="font-semibold">{t('pos.override.pin')}</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" required class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
    </label>

    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

    <div class="flex justify-end gap-2 pt-1">
      <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('pos.payCancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('pos.override.checking') : t('pos.outletApproval.apply')}</button>
    </div>
  </form>
</Modal>
