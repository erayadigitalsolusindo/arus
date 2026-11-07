<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { ai, type PendingAction } from '#lib/ai/api.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t, formatCurrency, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import type { MessageKey } from '#lib/i18n/types.ts';

  // Kartu usulan aksi dari HalloArus. AI hanya mengusulkan; data baru berubah setelah pengguna menekan Setujui di sini.
  // Ringkasan (`fields`) dibuat server. PIN penyetuju diketik di kartu ini dan dikirim langsung ke server, tidak lewat chat.
  let { action, initialStatus = 'pending', onresolved }: { action: PendingAction; initialStatus?: 'pending' | 'done' | 'cancelled' | 'expired'; onresolved?: (id: string, status: 'done' | 'cancelled' | 'expired') => void } = $props();

  // initialStatus dipakai saat kartu dimuat dari riwayat (sudah disetujui/dibatalkan/kedaluwarsa).
  // svelte-ignore state_referenced_locally
  let status = $state<'pending' | 'done' | 'cancelled' | 'expired'>(initialStatus);
  let busy = $state(false);
  let error = $state('');
  let approverId = $state('');
  let pin = $state('');
  let totp = $state('');
  let approvers = $state<Approver[] | null>(null);

  const isVoid = $derived(action.kind === 'petty_cash_void');
  const isMaster = $derived(action.kind === 'master_change');
  const changes = $derived(Object.entries(action.fields).filter(([k]) => k.startsWith('chg.')).map(([k, v]) => [k.slice(4), v] as const));
  const fieldLabel = (k: string) => (t(`ai.action.field.${k}` as MessageKey) as string) || k;
  const valid = $derived(approverId !== '' && /^\d{6}$/.test(pin) && /^\d{6}$/.test(totp));

  $effect(() => {
    if (status === 'pending' && approvers === null) {
      approvers = [];
      approvals
        .approvers('ai_write')
        .then((l) => {
          approvers = l;
          if (l.length === 1) approverId = l[0].id;
        })
        .catch((e) => (error = errorMessage(e)));
    }
  });

  function finish(s: 'done' | 'cancelled' | 'expired') {
    status = s;
    pin = '';
    totp = '';
    onresolved?.(action.id, s);
  }

  async function approve() {
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      await ai.confirmAction(action.id, { user_id: approverId, pin }, totp);
      finish('done');
    } catch (e) {
      pin = '';
      totp = '';
      if (e instanceof ApiError && e.code === 'AI_ACTION_NOT_FOUND') finish('expired');
      else error = e instanceof ApiError && (e.code === 'PIN_LOCKED' || e.code === 'TOTP_LOCKED') ? errorMessage(e).replace('{count}', String(e.retryAfter)) : errorMessage(e);
    } finally {
      busy = false;
    }
  }

  async function cancel() {
    if (busy) return;
    busy = true;
    try {
      await ai.cancelAction(action.id);
    } catch {
      /* usulan kedaluwarsa sendiri di server; yang penting kartu ditutup */
    } finally {
      busy = false;
      finish('cancelled');
    }
  }

  const money = (s: string) => formatCurrency(Number(s));
</script>

<div class="mt-2 rounded-xl border border-[var(--border-default)] bg-[var(--surface-sunken)] p-3.5 text-[13px]" class:opacity-70={status !== 'pending'}>
  <div class="flex flex-wrap items-center justify-between gap-2">
    <p class="font-semibold flex items-center gap-1.5">
      <i class={isVoid ? 'icon-trash-2 text-[var(--color-danger-600,#dc2626)]' : isMaster ? 'icon-database text-[var(--color-primary-600)]' : 'icon-wallet text-[var(--color-primary-600)]'}></i>
      {t(`ai.action.kind.${action.kind}`)}
    </p>
    {#if status === 'pending'}
      <span class="badge badge-soft badge-warning badge-small">{t('ai.action.awaiting')}</span>
    {:else}
      <span class="badge badge-soft badge-small {status === 'done' ? 'badge-success' : 'badge-neutral'}">{t(`ai.action.${status}`)}</span>
    {/if}
  </div>

  {#if isMaster}
    <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
      <dt class="text-[var(--text-tertiary)]">{t(`ai.action.ent.${action.fields.entity}` as MessageKey)}</dt>
      <dd class="font-semibold">{t(`ai.action.op.${action.fields.op}` as MessageKey)}</dd>
      {#if action.fields.target}
        <dt class="text-[var(--text-tertiary)]">{t('ai.action.target')}</dt>
        <dd class="min-w-0 break-words">{action.fields.target}</dd>
      {/if}
      {#each changes as [k, v] (k)}
        <dt class="text-[var(--text-tertiary)]">{fieldLabel(k)}</dt>
        <dd class="min-w-0 whitespace-pre-wrap break-words tabular-nums">{v}</dd>
      {/each}
    </dl>
  {:else}
  <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
      <dt class="text-[var(--text-tertiary)]">{t('ai.action.direction')}</dt>
      <dd>{t(`ai.action.dir.${action.fields.direction === 'in' ? 'in' : 'out'}`)}</dd>
      <dt class="text-[var(--text-tertiary)]">{t('ai.action.amount')}</dt>
      <dd class="font-semibold tabular-nums">{money(action.fields.amount ?? '0')}</dd>
      <dt class="text-[var(--text-tertiary)]">{t('ai.action.category')}</dt>
      <dd>{action.fields.category}</dd>
      {#if action.fields.note}
        <dt class="text-[var(--text-tertiary)]">{t('ai.action.note')}</dt>
        <dd class="whitespace-pre-wrap break-words min-w-0">{action.fields.note}</dd>
      {/if}
      {#if action.fields.created}
        <dt class="text-[var(--text-tertiary)]">{t('ai.action.created')}</dt>
        <dd>{formatDateTime(action.fields.created)}</dd>
      {/if}
    </dl>
  {/if}

  {#if status === 'pending'}
    {#if isVoid}
      <p class="mt-2 text-[12px] text-[var(--text-secondary)]">{t('ai.action.voidWarn')}</p>
    {:else if isMaster}
      <p class="mt-2 text-[12px] text-[var(--text-secondary)]">{t('ai.action.masterWarn')}</p>
    {/if}
    <div class="mt-3 grid gap-2 sm:grid-cols-3">
      <label class="block text-[12.5px]">
        <span class="font-semibold">{t('shift.petty.approver')}</span>
        <div class="mt-1"><Select bind:value={approverId} class="!min-h-10" options={[{ value: '', label: t('shift.petty.pickApprover'), disabled: true }, ...(approvers ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
      </label>
      <label class="block text-[12.5px]">
        <span class="font-semibold">{t('shift.petty.pin')}</span>
        <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" onkeydown={(e) => e.key === 'Enter' && approve()}
          class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
      </label>
      <label class="block text-[12.5px]">
        <span class="font-semibold">{t('ai.action.totp')}</span>
        <input type="text" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={totp} autocomplete="one-time-code" onkeydown={(e) => e.key === 'Enter' && approve()}
          class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
      </label>
    </div>
    <p class="mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('ai.action.pinHint')}</p>
    {#if error}
      <p role="alert" class="mt-2 text-[12.5px] text-[var(--color-danger-600,#dc2626)]">{error}</p>
    {/if}
    <div class="mt-3 flex flex-wrap gap-2">
      <button type="button" class="btn btn-sm btn-primary" disabled={!valid || busy} onclick={approve}>
        <i class={busy ? 'icon-loader-circle animate-spin' : 'icon-check'}></i> {busy ? t('ai.action.working') : t('ai.action.approve')}
      </button>
      <button type="button" class="btn btn-sm" disabled={busy} onclick={cancel}><i class="icon-x"></i> {t('ai.action.cancel')}</button>
    </div>
  {/if}
</div>
