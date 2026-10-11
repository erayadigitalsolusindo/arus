<script lang="ts">
  // Editor/penampil jurnal. Mode: jurnal baru, ubah draf, saldo awal (opening), atau baca-saja (terposting) + jurnal balik.
  import { onMount, tick } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import JournalTemplateDialog from '#lib/components/JournalTemplateDialog.svelte';
  import Combobox from '#lib/components/Combobox.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Select from '#lib/components/Select.svelte';
  import { accounting, todayISO, type Account, type Journal, type LineInput, type SavedTemplate, type SavedTemplateInput } from '#lib/accounting/api.ts';
  import { journalTemplates, type JournalTemplate } from '#lib/accounting/templates.ts';
  import { toCents } from '#lib/pos/money.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import { drafts } from '#lib/tabs/drafts.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  // `page`: dirender langsung di halaman (tab "Jurnal Baru") tanpa pop up; isiannya disimpan otomatis sebagai draf
  // di localStorage (lib/tabs/drafts.ts) sehingga selamat dari penutupan browser.
  let {
    id = '',
    opening = false,
    initialType = '',
    page = false,
    onclose,
    onchanged
  }: { id?: string; opening?: boolean; initialType?: string; page?: boolean; onclose: () => void; onchanged: () => void } = $props();

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

  // ---- Draf lokal (hanya mode halaman, jurnal baru) ----
  const draftPath = '/accounting/journals/new';
  const draftOwner = session.tenant && session.outlet && session.user ? `${session.tenant.id}:${session.outlet.id}:${session.user.id}` : '';
  const useDraft = page && !id && !opening && draftOwner !== '';
  type Draft = { owner: string; type: string; date: string; narration: string; rows: Omit<Row, 'k'>[] };
  const collect = (): Draft => ({
    owner: draftOwner,
    type,
    date,
    narration,
    rows: rows.map((r) => ({ account: r.account, label: r.label, debit: r.debit, credit: r.credit, memo: r.memo }))
  });
  const hasContent = () => narration.trim() !== '' || rows.some((r) => r.account !== '' || r.debit !== '' || r.credit !== '' || r.memo !== '');
  let restored = $state(false);
  if (useDraft) {
    const d = drafts.load<Draft>(draftPath);
    const s = (v: unknown): v is string => typeof v === 'string';
    if (d && d.owner === draftOwner && s(d.type) && s(d.date) && s(d.narration) && Array.isArray(d.rows) && d.rows.length > 0 && d.rows.length <= 200) {
      type = ['JU', 'KM', 'KK', 'TK'].includes(d.type) ? d.type : 'JU';
      date = d.date || todayISO();
      narration = d.narration;
      rows = d.rows.map((r) => ({ ...blank(), account: s(r.account) ? r.account : '', label: s(r.label) ? r.label : '', debit: s(r.debit) ? r.debit : '', credit: s(r.credit) ? r.credit : '', memo: s(r.memo) ? r.memo : '' }));
      restored = true;
    }
  }
  let finished = false; // sudah disimpan/diposting atau ditutup: jangan menulis draf lagi
  function persist() {
    if (finished) return;
    if (hasContent()) drafts.save(draftPath, collect());
    else drafts.clear(draftPath);
  }
  function finish() {
    finished = true;
    if (useDraft) drafts.clear(draftPath);
  }
  // Simpan otomatis dengan jeda 500 ms.
  $effect(() => {
    if (!useDraft) return;
    void JSON.stringify(collect());
    const h = setTimeout(persist, 500);
    return () => clearTimeout(h);
  });
  $effect(() => {
    if (!useDraft) return;
    const off = guard.register(() => !finished && hasContent());
    return () => {
      off();
      if (!drafts.takeDiscard(draftPath)) persist();
    };
  });
  const discardDraft = () => {
    finish();
    type = initialType || 'JU';
    date = todayISO();
    narration = '';
    rows = [blank(), blank()];
    mirror = false;
    restored = false;
    finished = false;
  };
  const closeIt = () => (page ? guard.request(() => (finish(), onclose())) : onclose());

  const readonly = $derived(journal?.status === 'posted');
  const typeOptions = $derived((['JU', 'KM', 'KK', 'TK'] as const).map((v) => ({ value: v as string, label: `${v} · ${t(`accounting.type.${v}`)}` })));

  onMount(async () => {
    if (!id && initialType && !restored) type = initialType;
    try {
      accounts = (await accounting.accounts()).filter((a) => a.kind === 'ledger' && a.active);
      if (!id && !opening) saved = await accounting.templates().catch(() => []);
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

  let saved = $state<SavedTemplate[]>([]);
  // Template pengguna hanya dipakai bila semua akunnya masih aktif (akun yang dihapus sudah ikut menghapus barisnya).
  const usableSaved = $derived(saved.filter((tp) => tp.lines.length >= 2 && tp.lines.every((l) => l.active)));
  let tplDialog = $state<'pick' | 'save' | null>(null);
  const canTpl = $derived(!readonly && !opening && !id);

  // Isi awal dialog simpan: baris yang akunnya sudah dipilih (sisi dari nominal bila ada); nominal tidak ikut.
  const tplSeed = $derived({
    type,
    narration: narration.trim(),
    lines: filled.map((r, i) => ({
      account: r.account,
      label: r.label,
      side: (toCents(r.debit) > 0n ? 'debit' : toCents(r.credit) > 0n ? 'credit' : i === 0 ? 'debit' : 'credit') as 'debit' | 'credit',
      memo: r.memo.trim()
    }))
  });

  function applySaved(tp: SavedTemplate) {
    tplDialog = null;
    type = tp.type;
    if (!narration.trim()) narration = tp.narration || tp.name;
    rows = tp.lines.map((l) => ({ ...blank(), account: l.account_id, label: `${l.account_code} · ${l.account_name}`, memo: l.memo }));
    mirror = tp.lines.length === 2;
    notice = t('accounting.journals.tplApplied');
    void focusCell(0, tp.lines[0].side === 'debit' ? 1 : 2);
  }

  // Galat dilempar ke dialog agar tampil di dalamnya.
  async function createTpl(input: SavedTemplateInput) {
    const made = await accounting.createTemplate(input);
    saved = [...saved, made].sort((a, b) => a.name.localeCompare(b.name));
    tplDialog = null;
    notice = t('accounting.journals.tplSaved', { name: made.name });
  }

  async function removeTpl(tp: SavedTemplate) {
    if (!confirm(t('accounting.journals.tplDeleteConfirm', { name: tp.name }))) return;
    await accounting.deleteTemplate(tp.id);
    saved = saved.filter((x) => x.id !== tp.id);
  }

  const byCode = $derived(new Map(accounts.map((a) => [a.code, a])));
  const usableTemplates = $derived(journalTemplates.filter((tp) => byCode.has(tp.debit) && byCode.has(tp.credit)));

  const builtinList = $derived(
    usableTemplates.map((tp) => ({
      id: tp.id as string,
      type: tp.type as string,
      name: t(`accounting.journals.tpl.${tp.id}`),
      debit: `${tp.debit} · ${byCode.get(tp.debit)!.name}`,
      credit: `${tp.credit} · ${byCode.get(tp.credit)!.name}`
    }))
  );
  const pickBuiltin = (id: string) => {
    const tp = journalTemplates.find((x) => x.id === id);
    if (tp) applyTemplate(tp);
  };

  function applyTemplate(tp: JournalTemplate) {
    tplDialog = null;
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
      if (page) {
        // Draf sudah di server (ubah lewat daftar jurnal); tab Jurnal Baru selesai tugasnya.
        finish();
        onclose();
      } else if (post) onclose();
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

{#snippet content()}
  {#if !loaded}
    <p class="py-10 text-center text-[var(--text-tertiary)]">…</p>
  {:else}
    <div class="space-y-3 text-[12.5px]">
      {#if restored}
        <div role="status" class="flex flex-wrap items-center gap-3 rounded-lg px-3 py-2 text-[12.5px] badge-warning">
          <i class="icon-save text-[14px] shrink-0"></i>
          <span class="grow">{t('accounting.journals.draftRestored')}</span>
          <button type="button" class="btn btn-sm" onclick={discardDraft}><i class="icon-trash-2 text-[13px]"></i>{t('accounting.journals.draftDiscard')}</button>
        </div>
      {/if}
      {#if opening}<p class="text-[12px] text-[var(--text-secondary)]">{t('accounting.journals.openingHint')}</p>{/if}
      {#if !opening && accounts.length < 2 && !readonly}<p class="text-[12px] text-[var(--color-warning-700)]">{t('accounting.journals.noAccounts')}</p>{/if}

      <section class="surface-card !p-3.5 space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h2 class="font-display font-bold text-[13px] uppercase tracking-wide inline-flex items-center gap-1.5"><i class="icon-file-text text-[14px] text-[var(--text-tertiary)]"></i>{t('accounting.journals.infoTitle')}</h2>
          {#if canTpl}
            <div class="flex flex-wrap items-center gap-2">
              {#if usableTemplates.length || usableSaved.length}
                <button type="button" class="btn btn-sm" onclick={() => (tplDialog = 'pick')}><i class="icon-bookmark"></i> {t('accounting.journals.tplBtn')} <span class="rounded-full bg-[var(--surface-sunken)] px-1.5 text-[10.5px] tabular-nums text-[var(--text-tertiary)]">{usableTemplates.length + usableSaved.length}</span></button>
              {/if}
              {#if can('journals', 'create')}
                <button type="button" class="btn btn-sm" onclick={() => (tplDialog = 'save')}><i class="icon-bookmark-plus"></i> {t('accounting.journals.tplSave')}</button>
              {/if}
            </div>
          {/if}
        </div>
        <div class="grid sm:grid-cols-[auto_10.5rem_1fr] gap-3 items-end">
          <div>
            <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.type')}</span>
            {#if !opening}
              <div class="mt-1 inline-flex rounded-lg border border-[var(--border-default)] p-0.5" role="group" aria-label={t('accounting.journals.type')}>
                {#each ['JU', 'KM', 'KK', 'TK'] as const as v (v)}
                  <button type="button" disabled={readonly || !!id} title={t(`accounting.type.${v}`)} class="h-8 rounded-md px-3 text-[12px] font-semibold disabled:cursor-default {type === v ? 'bg-[var(--color-primary)] text-white' : 'text-[var(--text-secondary)] hover:bg-[var(--surface-sunken)] disabled:hover:bg-transparent'}" aria-pressed={type === v} onclick={() => (type = v)}>{v}</button>
                {/each}
              </div>
              <span class="ms-2 text-[12px] text-[var(--text-tertiary)]">{t(`accounting.type.${type as 'JU'}`)}</span>
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
      </section>

      <section class="surface-card !p-0 overflow-hidden">
      <div class="flex items-center justify-between gap-2 px-3.5 py-2.5 border-b border-[var(--border-subtle)]">
        <h2 class="font-display font-bold text-[13px] uppercase tracking-wide inline-flex items-center gap-1.5"><i class="icon-list-ordered text-[14px] text-[var(--text-tertiary)]"></i>{t('accounting.journals.linesTitle')}</h2>
        {#if !readonly}<button type="button" class="btn btn-sm" onclick={() => addRow(rows.length)}><i class="icon-plus"></i> {t('accounting.journals.addLine')}</button>{/if}
      </div>
      <div class="overflow-x-auto scroll-thin">
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
              <td colspan="2" class="py-2 ps-3.5 pe-2 text-[11px] uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.totalDebit')} / {t('accounting.journals.totalCredit')}</td>
              <td class="py-2 px-1.5 text-end tabular-nums">{readonly && journal ? money(journal.total_debit) : fmtCents(totalD)}</td>
              <td class="py-2 px-1.5 text-end tabular-nums">{readonly && journal ? money(journal.total_credit) : fmtCents(totalC)}</td>
              <td colspan="2"></td>
            </tr>
          </tfoot>
        </table>
      </div>
      </section>
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

      <div class="sticky bottom-0 z-10 surface-card !py-2.5 !px-4 flex flex-wrap items-center gap-x-4 gap-y-2 shadow-[0_-6px_16px_rgba(0,0,0,.08)]">
        {#if !readonly}
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12.5px]">
            <span class="tabular-nums text-[var(--text-secondary)]">{t('accounting.journals.debit')} <b class="text-[var(--text-primary)]">{fmtCents(totalD)}</b></span>
            <span class="tabular-nums text-[var(--text-secondary)]">{t('accounting.journals.credit')} <b class="text-[var(--text-primary)]">{fmtCents(totalC)}</b></span>
            {#if diff === 0n && totalD > 0n}
              <span class="badge-soft badge-success"><i class="icon-check"></i> {t('accounting.journals.balanced')}</span>
            {:else if diff !== 0n}
              <span class="badge-soft badge-danger">{t('accounting.journals.unbalanced', { diff: fmtCents(diff) })}</span>
            {/if}
          </div>
        {/if}
        <div class="ms-auto flex flex-wrap justify-end gap-2">
          <button type="button" class="btn" onclick={closeIt} disabled={busy}>{t('accounting.journals.close')}</button>
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
    </div>
  {/if}
{/snippet}

{#if tplDialog}
  <JournalTemplateDialog mode={tplDialog} saved={usableSaved} builtin={builtinList} seed={tplSeed} {search} onpick={applySaved} onpickBuiltin={pickBuiltin} onremove={removeTpl} onsave={createTpl} onclose={() => (tplDialog = null)} />
{/if}

{#if page}
  {@render content()}
{:else}
  <Modal {title} {onclose} xl>{@render content()}</Modal>
{/if}
