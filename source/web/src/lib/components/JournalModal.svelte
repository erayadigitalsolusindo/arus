<script lang="ts">
  // Editor/penampil jurnal. Mode: jurnal baru, ubah draf, saldo awal (opening), atau baca-saja (terposting) + jurnal balik.
  import { onMount, tick } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Combobox from '#lib/components/Combobox.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Select from '#lib/components/Select.svelte';
  import { accounting, todayISO, type Account, type Journal, type LineInput } from '#lib/accounting/api.ts';
  import { journalTemplates, type JournalTemplate } from '#lib/accounting/templates.ts';
  import { toCents } from '#lib/pos/money.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let {
    id = '',
    opening = false,
    initialType = '',
    onclose,
    onchanged
  }: { id?: string; opening?: boolean; initialType?: string; onclose: () => void; onchanged: () => void } = $props();

  type Row = { k: number; account: string; label: string; debit: string; credit: string; memo: string };
  let seq = 0;
  const blank = (): Row => ({ k: ++seq, account: '', label: '', debit: '', credit: '', memo: '' });
  const money = (s: string) => formatCurrency(Number(s));

  let accounts = $state<Account[]>([]);
  let loaded = $state(false);
  let journal = $state<Journal | null>(null);
  let type = $state('JU');
  let date = $state(todayISO());
  $effect.pre(() => {
    if (opening && !id) date = todayISO().slice(0, 8) + '01'; // saldo awal: tanggal 1
  });
  let narration = $state('');
  let rows = $state<Row[]>([blank(), blank()]);
  let busy = $state(false);
  let error = $state('');
  let fields = $state<Record<string, string>>({});
  let notice = $state('');
  let reversing = $state(false);
  let revDate = $state(todayISO());
  let revNote = $state('');

  const readonly = $derived(journal?.status === 'posted');
  const typeOptions = $derived((['JU', 'KM', 'KK', 'TK'] as const).map((v) => ({ value: v as string, label: `${v} · ${t(`accounting.type.${v}`)}` })));

  onMount(async () => {
    if (!id && initialType) type = initialType;
    try {
      accounts = (await accounting.accounts()).filter((a) => a.kind === 'ledger' && a.active);
      if (id) {
        const j = await accounting.journal(id);
        journal = j;
        type = j.type;
        date = j.date;
        narration = j.narration;
        rows = j.lines.map((l) => ({
          k: ++seq,
          account: l.account_id,
          label: `${l.account_code} · ${l.account_name}`,
          debit: Number(l.debit) ? l.debit : '',
          credit: Number(l.credit) ? l.credit : '',
          memo: l.memo
        }));
      }
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loaded = true;
    }
  });

  // COA kecil: disaring di klien, dibatasi 30 hasil.
  const search = (q: string) => {
    const s = q.trim().toLowerCase();
    const hit = accounts.filter((a) => !s || a.code.toLowerCase().includes(s) || a.name.toLowerCase().includes(s)).slice(0, 30);
    return Promise.resolve(hit.map((a) => ({ id: a.id, name: `${a.code} · ${a.name}` })));
  };

  const totalD = $derived(rows.reduce((s, r) => s + toCents(r.debit), 0n));
  const totalC = $derived(rows.reduce((s, r) => s + toCents(r.credit), 0n));
  const diff = $derived(totalD - totalC);
  const filled = $derived(rows.filter((r) => r.account !== ''));
  const rowsOk = $derived(filled.length >= 2 && filled.every((r) => (toCents(r.debit) > 0n) !== (toCents(r.credit) > 0n)));
  const valid = $derived(date !== '' && rowsOk && (opening ? diff === 0n : true));
  const fmtCents = (c: bigint) => formatCurrency(Number(c < 0n ? -c : c) / 100);

  // Template dua baris: nominal yang diketik di satu baris otomatis disalin ke sisi lawan baris satunya.
  let mirror = $state(false);

  // Mengisi satu sisi mengosongkan sisi lawannya (tepat satu sisi > 0 per baris).
  function side(r: Row, which: 'debit' | 'credit') {
    if (which === 'debit' && toCents(r.debit) > 0n) r.credit = '';
    if (which === 'credit' && toCents(r.credit) > 0n) r.debit = '';
    if (mirror && rows.length === 2) {
      const other = rows[0] === r ? rows[1] : rows[0];
      const v = which === 'debit' ? r.debit : r.credit;
      if (which === 'debit') {
        other.credit = v;
        other.debit = '';
      } else {
        other.debit = v;
        other.credit = '';
      }
    }
  }

  const byCode = $derived(new Map(accounts.map((a) => [a.code, a])));
  const usableTemplates = $derived(journalTemplates.filter((tp) => byCode.has(tp.debit) && byCode.has(tp.credit)));

  function applyTemplate(tp: JournalTemplate) {
    const row = (code: string): Row => {
      const a = byCode.get(code)!;
      return { ...blank(), account: a.id, label: `${a.code} · ${a.name}` };
    };
    type = tp.type;
    if (!narration.trim()) narration = t(`accounting.journals.tpl.${tp.id}`);
    rows = [row(tp.debit), row(tp.credit)];
    mirror = true;
    notice = t('accounting.journals.tplApplied');
    void focusCell(0, 1);
  }

  const centsStr = (c: bigint) => `${c / 100n}.${(c % 100n).toString().padStart(2, '0')}`;

  /** Baris baru langsung terisi selisih di sisi lawan agar jurnal cepat seimbang. */
  function newRow(): Row {
    const r = blank();
    if (diff > 0n) r.credit = centsStr(diff);
    else if (diff < 0n) r.debit = centsStr(-diff);
    return r;
  }

  const cellsOf = (i: number) => [...document.querySelectorAll<HTMLInputElement>(`tr[data-row="${i}"] input`)];
  async function focusCell(i: number, c: number) {
    await tick();
    const el = cellsOf(i)[c];
    el?.focus();
    el?.select();
  }

  function addRow(at: number) {
    mirror = false;
    rows = [...rows.slice(0, at), newRow(), ...rows.slice(at)];
    void focusCell(at, 0);
  }

  function removeRow(i: number, c: number) {
    mirror = false;
    if (rows.length > 2) {
      rows = rows.filter((_, k) => k !== i);
      void focusCell(Math.min(i, rows.length - 1), c);
    } else {
      rows[i] = blank();
      void focusCell(i, 0);
    }
  }

  /** "=" di kolom debit/kredit: isi dengan selisih yang tersisa (tidak termasuk baris ini). */
  function fillBalance(r: Row, which: 'debit' | 'credit') {
    const need = which === 'debit' ? totalC - (totalD - toCents(r.debit)) : totalD - (totalC - toCents(r.credit));
    if (need <= 0n) return;
    if (which === 'debit') {
      r.debit = centsStr(need);
      r.credit = '';
    } else {
      r.credit = centsStr(need);
      r.debit = '';
    }
  }

  /** Navigasi grid ala kasir: Enter ke sel berikut, ↑↓ antar baris, Insert tambah baris, Ctrl+Del hapus baris. */
  function gridKey(e: KeyboardEvent) {
    const el = e.target as HTMLElement;
    const tr = el.closest<HTMLElement>('tr[data-row]');
    if (!tr || el.tagName !== 'INPUT') return;
    const i = Number(tr.dataset.row);
    const c = cellsOf(i).indexOf(el as HTMLInputElement);
    const listOpen = c === 0 && el.getAttribute('aria-expanded') === 'true';
    const mod = e.ctrlKey || e.metaKey;

    if (e.key === 'Enter' && !mod) {
      if (listOpen) return void setTimeout(() => void focusCell(i, 1)); // bits-ui memilih opsi; lanjut ke debit
      e.preventDefault();
      if (c < 3) void focusCell(i, c + 1);
      else if (i === rows.length - 1) addRow(i + 1);
      else void focusCell(i + 1, 0);
    } else if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && !listOpen && !e.altKey) {
      e.preventDefault();
      const to = i + (e.key === 'ArrowDown' ? 1 : -1);
      if (to >= 0 && to < rows.length) void focusCell(to, c);
    } else if (e.key === 'Insert' || (e.altKey && e.key.toLowerCase() === 'n')) {
      e.preventDefault();
      addRow(i + 1);
    } else if ((mod && e.key === 'Delete') || (e.altKey && e.key === 'Backspace')) {
      e.preventDefault();
      removeRow(i, c);
    } else if (e.key === '=' && (c === 1 || c === 2)) {
      e.preventDefault();
      fillBalance(rows[i], c === 1 ? 'debit' : 'credit');
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (readonly || busy || !(e.ctrlKey || e.metaKey)) return;
    if (e.key === 'Enter' && opening && valid) {
      e.preventDefault();
      void saveOpening();
    } else if (e.key === 'Enter' && !opening && can('journals', 'approve') && valid && diff === 0n) {
      e.preventDefault();
      void saveDraft(true);
    } else if (e.key.toLowerCase() === 's' && !opening && rowsOk) {
      e.preventDefault();
      void saveDraft(false);
    }
  }

  const payloadLines = (): LineInput[] => filled.map((r) => ({ account_id: r.account, debit: r.debit || '0', credit: r.credit || '0', memo: r.memo.trim() }));

  async function run(fn: () => Promise<void>) {
    if (busy) return;
    busy = true;
    error = '';
    fields = {};
    notice = '';
    try {
      await fn();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') fields = err.fields;
      else error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function saveDraft(post: boolean) {
    await run(async () => {
      const body = { date, type, narration: narration.trim(), lines: payloadLines() };
      let j = journal && journal.status === 'draft' ? await accounting.updateJournal(journal.id, body) : await accounting.createJournal(body);
      if (post) {
        j = await accounting.postJournal(j.id);
        notice = t('accounting.journals.posted', { doc: j.doc_no ?? '' });
      } else notice = t('accounting.journals.draftSaved');
      journal = j;
      onchanged();
      if (post) onclose();
    });
  }

  async function saveOpening() {
    await run(async () => {
      const j = await accounting.opening({ date, narration: narration.trim(), lines: payloadLines() });
      onchanged();
      notice = t('accounting.journals.posted', { doc: j.doc_no ?? '' });
      onclose();
    });
  }

  async function reverse(e: Event) {
    e.preventDefault();
    if (!journal) return;
    await run(async () => {
      await accounting.reverseJournal(journal!.id, revDate, revNote.trim());
      onchanged();
      onclose();
    });
  }

  const title = $derived(opening ? t('accounting.journals.titleOpening') : readonly ? t('accounting.journals.titleView', { doc: journal?.doc_no ?? '' }) : id ? t('accounting.journals.titleEdit') : t('accounting.journals.titleNew'));
  const inputClass = 'w-full h-8 px-2 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px] outline-none focus:border-[var(--color-primary-500)] focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-primary-500)_18%,transparent)]';
  const kbd = 'rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-1.5 py-px font-mono text-[10.5px] text-[var(--text-secondary)]';
</script>

<svelte:window {onkeydown} />

<Modal {title} {onclose} xl>
  {#if !loaded}
    <p class="py-10 text-center text-[var(--text-tertiary)]">…</p>
  {:else}
    <div class="space-y-2.5 text-[12.5px]">
      {#if opening}<p class="text-[12px] text-[var(--text-secondary)]">{t('accounting.journals.openingHint')}</p>{/if}
      {#if !opening && accounts.length < 2 && !readonly}<p class="text-[12px] text-[var(--color-warning-700)]">{t('accounting.journals.noAccounts')}</p>{/if}

      {#if !readonly && !opening && !id && usableTemplates.length}
        <div>
          <div class="flex items-baseline gap-2">
            <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.tplTitle')}</span>
            <span class="text-[11px] text-[var(--text-tertiary)]">{t('accounting.journals.tplHint')}</span>
          </div>
          <div class="mt-1.5 flex flex-wrap gap-1.5">
            {#each usableTemplates as tp (tp.id)}
              <button type="button" class="rounded-full border border-[var(--border-default)] bg-[var(--surface-base)] px-2.5 py-1 text-[12px] hover:border-[var(--color-primary-500)] hover:text-[var(--color-primary-600)]" onclick={() => applyTemplate(tp)}>
                <span class="font-mono text-[10.5px] text-[var(--text-tertiary)]">{tp.type}</span> {t(`accounting.journals.tpl.${tp.id}`)}
              </button>
            {/each}
          </div>
        </div>
      {/if}

      <div class="grid sm:grid-cols-[13rem_10.5rem_1fr] gap-2.5 items-end">
        <div>
          <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.type')}</span>
          {#if !opening}
            <div class="mt-1"><Select bind:value={type} options={typeOptions} disabled={readonly || !!id} ariaLabel={t('accounting.journals.type')} /></div>
          {:else}
            <div class="mt-1 h-9 flex items-center font-medium">{t('accounting.type.OPENING')}</div>
          {/if}
        </div>
        <div>
          <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.date')}</span>
          <div class="mt-1"><DatePicker bind:value={date} clearable={false} disabled={readonly} invalid={!!fields.date} /></div>
        </div>
        <label class="block">
          <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.narration')}</span>
          <input bind:value={narration} maxlength="500" disabled={readonly} class="mt-1 {inputClass} !h-9" />
        </label>
      </div>

      <div class="overflow-x-auto scroll-thin rounded-lg border border-[var(--border-default)]">
        <table class="w-full min-w-[720px] text-[12.5px]">
          <thead>
            <tr class="bg-[var(--surface-sunken)] text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="py-1.5 ps-2.5 w-8 text-start" scope="col">#</th>
              <th class="py-1.5 pe-2 text-start" scope="col">{t('accounting.journals.account')}</th>
              <th class="py-1.5 px-1.5 text-end w-40" scope="col">{t('accounting.journals.debit')}</th>
              <th class="py-1.5 px-1.5 text-end w-40" scope="col">{t('accounting.journals.credit')}</th>
              <th class="py-1.5 px-1.5 text-start w-48" scope="col">{t('accounting.journals.memo')}</th>
              {#if !readonly}<th class="w-16"></th>{/if}
            </tr>
          </thead>
          <tbody onkeydowncapture={gridKey}>
            {#if readonly && journal}
              {#each journal.lines as l (l.line_no)}
                <tr class="border-t border-[var(--border-subtle)]">
                  <td class="py-1.5 ps-2.5 text-[var(--text-tertiary)] tabular-nums">{l.line_no}</td>
                  <td class="py-1.5 pe-2"><span class="font-mono">{l.account_code}</span> {l.account_name}</td>
                  <td class="py-1.5 px-1.5 text-end tabular-nums">{Number(l.debit) ? money(l.debit) : ''}</td>
                  <td class="py-1.5 px-1.5 text-end tabular-nums">{Number(l.credit) ? money(l.credit) : ''}</td>
                  <td class="py-1.5 px-1.5">{l.memo}</td>
                </tr>
              {/each}
            {:else}
              {#each rows as r, i (r.k)}
                <tr data-row={i} class="group border-t border-[var(--border-subtle)] align-middle focus-within:bg-[color-mix(in_srgb,var(--color-primary-500)_5%,transparent)]">
                  <td class="ps-2.5 text-[11px] text-[var(--text-tertiary)] tabular-nums">{i + 1}</td>
                  <td class="py-0.5 pe-1.5 [&_.picker-trigger]:!min-h-8 [&_.picker-trigger]:!h-8 [&_.picker-trigger]:!text-[12.5px]"><Combobox bind:value={r.account} bind:label={r.label} {search} placeholder={t('accounting.journals.accountPick')} clearable={false} /></td>
                  <td class="py-0.5 px-1.5"><MoneyInput bind:value={r.debit} placeholder="0" class="{inputClass} text-end tabular-nums" oninput={() => side(r, 'debit')} onfocus={(e) => e.currentTarget.select()} /></td>
                  <td class="py-0.5 px-1.5"><MoneyInput bind:value={r.credit} placeholder="0" class="{inputClass} text-end tabular-nums" oninput={() => side(r, 'credit')} onfocus={(e) => e.currentTarget.select()} /></td>
                  <td class="py-0.5 px-1.5"><input bind:value={r.memo} maxlength="200" class={inputClass} onfocus={(e) => e.currentTarget.select()} /></td>
                  <td class="py-0.5 pe-1.5">
                    <div class="flex justify-end gap-0.5 opacity-40 group-hover:opacity-100 group-focus-within:opacity-100">
                      <button type="button" tabindex="-1" class="header-icon-btn !size-7" title="{t('accounting.journals.addLine')} (Ins)" aria-label={t('accounting.journals.addLine')} onclick={() => addRow(i + 1)}><i class="icon-plus text-[13px]"></i></button>
                      <button type="button" tabindex="-1" class="header-icon-btn !size-7" title="{t('accounting.journals.removeLine')} (Ctrl+Del)" aria-label={t('accounting.journals.removeLine')} onclick={() => removeRow(i, 0)}><i class="icon-trash-2 text-[13px]"></i></button>
                    </div>
                  </td>
                </tr>
              {/each}
            {/if}
          </tbody>
          <tfoot>
            <tr class="border-t border-[var(--border-default)] bg-[var(--surface-sunken)] font-bold">
              <td colspan="2" class="py-1.5 ps-2.5 pe-2">
                {#if !readonly}<button type="button" class="btn btn-sm" onclick={() => addRow(rows.length)}><i class="icon-plus"></i> {t('accounting.journals.addLine')}</button>{/if}
              </td>
              <td class="py-1.5 px-1.5 text-end tabular-nums">{readonly && journal ? money(journal.total_debit) : fmtCents(totalD)}</td>
              <td class="py-1.5 px-1.5 text-end tabular-nums">{readonly && journal ? money(journal.total_credit) : fmtCents(totalC)}</td>
              <td colspan="2" class="py-1.5 px-1.5 text-[12px] font-semibold">
                {#if !readonly}
                  {#if diff === 0n && totalD > 0n}<span class="text-[var(--color-success-600)]"><i class="icon-check"></i> {t('accounting.journals.balanced')}</span>
                  {:else if diff !== 0n}<span class="text-[var(--color-danger-600)]">{t('accounting.journals.unbalanced', { diff: fmtCents(diff) })}</span>{/if}
                {/if}
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
      {#if !readonly}
        <p class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-[var(--text-tertiary)]">
          <span><kbd class={kbd}>↑</kbd> <kbd class={kbd}>↓</kbd> {t('accounting.journals.keyMove')}</span>
          <span><kbd class={kbd}>Enter</kbd> {t('accounting.journals.keyNext')}</span>
          <span><kbd class={kbd}>Ins</kbd> {t('accounting.journals.keyAdd')}</span>
          <span><kbd class={kbd}>Ctrl+Del</kbd> {t('accounting.journals.keyRemove')}</span>
          <span><kbd class={kbd}>=</kbd> {t('accounting.journals.keyBalance')}</span>
          {#if !opening}<span><kbd class={kbd}>Ctrl+S</kbd> {t('accounting.journals.keyDraft')}</span>{/if}
          <span><kbd class={kbd}>Ctrl+Enter</kbd> {opening ? t('accounting.journals.saveOpening') : t('accounting.journals.keyPost')}</span>
        </p>
      {/if}
      {#each Object.entries(fields) as [k, code] (k)}<p class="text-[12px] text-[var(--color-danger-600)]">{k}: {fieldMessage(code)}</p>{/each}

      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
      {#if notice}<p role="status" class="text-[12.5px] text-[var(--color-success-600)]"><i class="icon-check me-1"></i>{notice}</p>{/if}

      {#if reversing && journal}
        <form class="rounded-lg border border-[var(--border-default)] p-3 space-y-2" onsubmit={reverse}>
          <div class="font-semibold">{t('accounting.journals.reverseTitle')}</div>
          <p class="text-[12px] text-[var(--text-tertiary)]">{t('accounting.journals.reverseHint')}</p>
          <div class="grid sm:grid-cols-[11rem_1fr] gap-3">
            <div><span class="font-semibold">{t('accounting.journals.reverseDate')}</span><div class="mt-1"><DatePicker bind:value={revDate} clearable={false} /></div></div>
            <label class="block"><span class="font-semibold">{t('accounting.journals.reverseNote')}</span><input bind:value={revNote} maxlength="400" class="mt-1 {inputClass} !h-10" /></label>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn" onclick={() => (reversing = false)} disabled={busy}>{t('accounting.journals.close')}</button>
            <button type="submit" class="btn btn-primary" disabled={busy || !revDate}>{t('accounting.journals.reverseDo')}</button>
          </div>
        </form>
      {/if}

      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('accounting.journals.close')}</button>
        {#if readonly}
          {#if can('journals', 'approve') && !reversing && !journal?.reverses_id}
            <button type="button" class="btn" onclick={() => (reversing = true)}><i class="icon-undo-2"></i> {t('accounting.journals.reverse')}</button>
          {/if}
        {:else if opening}
          <button type="button" class="btn btn-primary" disabled={!valid || busy} onclick={saveOpening}>{busy ? t('accounting.journals.saving') : t('accounting.journals.saveOpening')}</button>
        {:else}
          <button type="button" class="btn" disabled={!rowsOk || busy} onclick={() => saveDraft(false)}>{busy ? t('accounting.journals.saving') : t('accounting.journals.saveDraft')}</button>
          {#if can('journals', 'approve')}
            <button type="button" class="btn btn-primary" disabled={!valid || diff !== 0n || busy} onclick={() => saveDraft(true)}>{t('accounting.journals.savePost')}</button>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</Modal>
