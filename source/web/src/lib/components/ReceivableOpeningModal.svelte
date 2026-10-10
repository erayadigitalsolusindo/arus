<script lang="ts">
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Combobox from '#lib/components/Combobox.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { members } from '#lib/members/api.ts';
  import { receivables } from '#lib/receivables/api.ts';
  import { newIdempotencyKey } from '#lib/sales/api.ts';
  import { toCents } from '#lib/pos/money.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let { onclose, onchanged }: { onclose: () => void; onchanged: () => void } = $props();

  const today = (() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  })();

  let memberId = $state('');
  let memberLabel = $state('');
  let refNo = $state('');
  let docDate = $state('');
  let amount = $state('');
  let dueDate = $state('');
  let note = $state('');
  let busy = $state(false);
  let error = $state('');
  let fields = $state<Record<string, string>>({});
  let saved = $state('');
  // Satu kunci per isian: klik ganda/jaringan putus tidak mencatat dua kali. Diganti setelah tersimpan.
  let key = newIdempotencyKey();

  const searchMember = (q: string) => members.lookup(q).then((r) => r.map((m) => ({ id: m.id, name: `${m.name} · ${m.code}` })));
  const valid = $derived(memberId !== '' && docDate !== '' && toCents(amount) > 0n && (dueDate === '' || dueDate >= docDate));

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    fields = {};
    saved = '';
    try {
      const res = await receivables.createOpening(
        {
          member_id: memberId,
          doc_date: docDate,
          amount,
          ...(refNo.trim() ? { ref_no: refNo.trim().slice(0, 64) } : {}),
          ...(dueDate ? { due_date: dueDate } : {}),
          ...(note.trim() ? { note: note.trim().slice(0, 200) } : {})
        },
        key
      );
      saved = t('receivables.opening.saved', { doc: res.doc_no });
      key = newIdempotencyKey();
      // Siap untuk member berikutnya: tanggal dipertahankan (biasanya sama untuk satu sesi input).
      memberId = '';
      memberLabel = '';
      refNo = '';
      amount = '';
      dueDate = '';
      note = '';
      onchanged();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') fields = err.fields;
      else error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const fieldError = (name: string) => (fields[name] ? (name === 'ref_no' && fields[name] === 'DUPLICATE' ? t('receivables.opening.refHint') : fieldMessage(fields[name])) : '');
  const inputClass = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]';
</script>

<Modal title={t('receivables.opening.title')} {onclose}>
  <form class="space-y-3 text-[13px]" onsubmit={submit} autocomplete="off">
    <p class="text-[12.5px] text-[var(--text-secondary)]">{t('receivables.opening.hint')}</p>

    <label class="block">
      <span class="font-semibold">{t('receivables.opening.member')}</span>
      <div class="mt-1"><Combobox bind:value={memberId} bind:label={memberLabel} search={searchMember} placeholder={t('receivables.opening.memberHint')} /></div>
      {#if fieldError('member_id')}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldError('member_id')}</span>{/if}
    </label>

    <div class="grid sm:grid-cols-2 gap-3">
      <label class="block">
        <span class="font-semibold">{t('receivables.opening.refNo')}</span>
        <input bind:value={refNo} maxlength="64" class={inputClass} />
        <span class="text-[11.5px] {fields.ref_no ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-tertiary)]'}">{fieldError('ref_no') || t('receivables.opening.refHint')}</span>
      </label>
      <label class="block">
        <span class="font-semibold">{t('receivables.opening.docDate')}</span>
        <div class="mt-1"><DatePicker bind:value={docDate} max={today} clearable={false} invalid={!!fields.doc_date} /></div>
        {#if fieldError('doc_date')}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldError('doc_date')}</span>{/if}
      </label>
      <label class="block">
        <span class="font-semibold">{t('receivables.opening.amount')}</span>
        <MoneyInput bind:value={amount} placeholder="0" class="{inputClass} text-end tabular-nums font-semibold" />
        {#if fieldError('amount')}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldError('amount')}</span>{/if}
      </label>
      <label class="block">
        <span class="font-semibold">{t('receivables.opening.dueDate')}</span>
        <div class="mt-1"><DatePicker bind:value={dueDate} min={docDate || undefined} invalid={!!fields.due_date || (dueDate !== '' && docDate !== '' && dueDate < docDate)} /></div>
        {#if fieldError('due_date')}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldError('due_date')}</span>{/if}
      </label>
    </div>

    <label class="block">
      <span class="font-semibold">{t('receivables.opening.note')}</span>
      <input bind:value={note} maxlength="200" class={inputClass} />
    </label>

    {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
    {#if saved}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{saved}</p>{/if}

    <div class="flex justify-end gap-2">
      <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('receivables.modal.close')}</button>
      <button type="submit" class="btn btn-primary" disabled={!valid || busy}>{busy ? t('receivables.opening.saving') : t('receivables.opening.save')}</button>
    </div>
  </form>
</Modal>
