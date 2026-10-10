<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import Switch from '#lib/components/Switch.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import { accounting, todayISO, type Account, type AccClass } from '#lib/accounting/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  const CLASSES: AccClass[] = ['asset', 'liability', 'equity', 'revenue', 'cogs', 'expense'];

  let rows = $state<Account[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let editing = $state<Account | 'new' | null>(null);

  async function load() {
    loading = true;
    error = '';
    try {
      rows = await accounting.accounts();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    document.title = t('accounting.accounts.docTitle');
    void load();
  });

  // Urutan kode sudah dari server; kedalaman dihitung dari rantai induk.
  const depthOf = $derived.by(() => {
    const byId = new Map(rows.map((r) => [r.id, r]));
    const memo = new Map<string, number>();
    const depth = (r: Account): number => {
      if (memo.has(r.id)) return memo.get(r.id)!;
      const d = r.parent_id && byId.has(r.parent_id) ? depth(byId.get(r.parent_id)!) + 1 : 0;
      memo.set(r.id, d);
      return d;
    };
    return (r: Account) => depth(r);
  });

  async function seed() {
    error = '';
    try {
      await accounting.seedRetail();
      notice = t('accounting.accounts.seeded');
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function remove(a: Account) {
    if (!confirm(t('accounting.accounts.removeConfirm', { name: `${a.code} ${a.name}` }))) return;
    error = '';
    try {
      await accounting.deleteAccount(a.id);
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  // ---- form
  let fCode = $state('');
  let fName = $state('');
  let fKind = $state<'group' | 'ledger'>('ledger');
  let fClass = $state<AccClass>('asset');
  let fParent = $state('');
  let fCash = $state(false);
  let fActive = $state(true);
  let busy = $state(false);
  let formError = $state('');
  let fields = $state<Record<string, string>>({});

  $effect(() => {
    if (editing === null) return;
    formError = '';
    fields = {};
    if (editing === 'new') {
      fCode = '';
      fName = '';
      fKind = 'ledger';
      fClass = 'asset';
      fParent = '';
      fCash = false;
      fActive = true;
      codeTouched = false;
    } else {
      fCode = editing.code;
      fName = editing.name;
      fKind = editing.kind;
      fClass = editing.class;
      fParent = editing.parent_id ?? '';
      fCash = editing.is_cash_bank;
      fActive = editing.active;
      codeTouched = true;
    }
  });

  const isNew = $derived(editing === 'new');
  const selfId = $derived(editing && editing !== 'new' ? editing.id : '');
  // Induk = akun grup berkelas sama (server memeriksa ulang, termasuk siklus).
  const parentOptions = $derived([
    { value: '', label: t('accounting.accounts.noParent') },
    ...rows.filter((r) => r.kind === 'group' && r.class === fClass && r.id !== selfId).map((r) => ({ value: r.id, label: `${r.code} · ${r.name}` }))
  ]);
  const classOptions = CLASSES.map((c) => ({ value: c, label: t(`accounting.class.${c}`) }));
  const isSys = $derived(editing !== null && editing !== 'new' && editing.is_system);
  const canCash = $derived(fKind === 'ledger' && fClass === 'asset');
  $effect(() => {
    if (!canCash) fCash = false;
  });

  async function save(e: Event) {
    e.preventDefault();
    if (busy || editing === null) return;
    busy = true;
    formError = '';
    fields = {};
    const body = { parent_id: fParent || null, code: fCode.trim(), name: fName.trim(), class: fClass, is_cash_bank: fCash };
    try {
      if (editing === 'new') await accounting.createAccount({ ...body, kind: fKind });
      else await accounting.updateAccount(editing.id, { ...body, active: fActive });
      editing = null;
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') fields = err.fields;
      else formError = errorMessage(err);
    } finally {
      busy = false;
    }
  }
  const inputClass = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]';

  // ---- saldo berjalan & saldo awal
  let balances = $state<Record<string, number>>({});
  let search = $state('');
  let openingMode = $state(false);
  let openingDate = $state(todayISO().slice(0, 8) + '01');
  let amounts = $state<Record<string, string>>({});
  let savingOpening = $state(false);

  const debitNormal = (c: AccClass) => c === 'asset' || c === 'cogs' || c === 'expense';

  async function loadBalances() {
    const to = todayISO();
    try {
      const tb = await accounting.trialBalance({ from: to.slice(0, 8) + '01', to });
      const m: Record<string, number> = {};
      for (const r of tb.rows) {
        const d = Number(r.closing_debit) - Number(r.closing_credit);
        m[r.account_id] = debitNormal(r.class) ? d : -d;
      }
      balances = m;
    } catch {
      balances = {};
    }
  }
  onMount(() => void loadBalances());

  // Saldo akun grup = jumlah saldo anak-anaknya (kelas sama, jadi sisi normal sama).
  const rolled = $derived.by(() => {
    const kids = new Map<string, Account[]>();
    for (const r of rows) if (r.parent_id) kids.set(r.parent_id, [...(kids.get(r.parent_id) ?? []), r]);
    const memo = new Map<string, number>();
    const sum = (r: Account): number => {
      if (memo.has(r.id)) return memo.get(r.id)!;
      const v = r.kind === 'group' ? (kids.get(r.id) ?? []).reduce((s, k) => s + sum(k), 0) : (balances[r.id] ?? 0);
      memo.set(r.id, v);
      return v;
    };
    return (r: Account) => sum(r);
  });

  const visible = $derived.by(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) => r.code.toLowerCase().includes(q) || r.name.toLowerCase().includes(q));
  });

  const cents = (s: string | undefined) => Math.round((Number(s) || 0) * 100);
  const equityPlug = $derived(rows.find((r) => r.code === '3100' && r.kind === 'ledger'));
  const totals = $derived.by(() => {
    let d = 0;
    let c = 0;
    for (const r of rows) {
      const v = cents(amounts[r.id]);
      if (!v || r.kind !== 'ledger') continue;
      if (debitNormal(r.class)) d += v;
      else c += v;
    }
    return { d, c, diff: d - c };
  });

  function startOpening() {
    notice = '';
    error = '';
    amounts = Object.fromEntries(rows.filter((r) => r.kind === 'ledger').map((r) => [r.id, '']));
    openingMode = true;
  }

  async function saveOpening() {
    if (savingOpening) return;
    const lines: { account_id: string; debit: string; credit: string; memo: string }[] = [];
    for (const r of rows) {
      const v = cents(amounts[r.id]);
      if (!v || r.kind !== 'ledger') continue;
      const s = (v / 100).toFixed(2);
      lines.push(debitNormal(r.class) ? { account_id: r.id, debit: s, credit: '0', memo: '' } : { account_id: r.id, debit: '0', credit: s, memo: '' });
    }
    if (lines.length === 0) return;
    if (totals.diff !== 0) {
      if (!equityPlug) {
        error = t('accounting.accounts.openingUnbalanced');
        return;
      }
      const s = (Math.abs(totals.diff) / 100).toFixed(2);
      lines.push(totals.diff > 0 ? { account_id: equityPlug.id, debit: '0', credit: s, memo: t('accounting.accounts.openingPlug') } : { account_id: equityPlug.id, debit: s, credit: '0', memo: t('accounting.accounts.openingPlug') });
    }
    savingOpening = true;
    error = '';
    try {
      await accounting.opening({ date: openingDate, narration: t('accounting.type.OPENING'), lines });
      openingMode = false;
      notice = t('accounting.accounts.openingSaved');
      await loadBalances();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      savingOpening = false;
    }
  }
  const money = (n: number) => (n === 0 ? '–' : formatCurrency(n));

  // ---- bantuan form: saran kode & ringkasan
  let codeTouched = $state(false);
  const fSide = $derived(debitNormal(fClass) ? 'debit' : 'credit');
  const parentLabel = $derived.by(() => {
    const p = rows.find((r) => r.id === fParent);
    return p ? `${p.code} ${p.name}` : t('accounting.accounts.form.previewRoot');
  });
  const BASE_CODE: Record<AccClass, number> = { asset: 1000, liability: 2000, equity: 3000, revenue: 4000, cogs: 5000, expense: 6000 };

  // Saran kode: anak terakhir induk + 10 (atau induk + 10 bila belum punya anak); tanpa induk: kode terakhir sekelas + 10.
  function suggestCode(): string {
    const used = new Set(rows.map((r) => r.code));
    const kids = rows.filter((r) => (fParent ? r.parent_id === fParent : r.class === fClass) && /^\d+$/.test(r.code)).map((r) => Number(r.code));
    const parent = rows.find((r) => r.id === fParent);
    let n: number;
    if (kids.length > 0) n = Math.max(...kids) + 10;
    else if (parent && /^\d+$/.test(parent.code)) n = Number(parent.code) + 10;
    else n = BASE_CODE[fClass];
    for (let i = 0; i < 500 && used.has(String(n)); i++) n += 10;
    return String(n);
  }
  $effect(() => {
    // dibaca agar efek bereaksi pada perubahan induk/kelas
    void fParent;
    void fClass;
    if (editing === 'new' && !codeTouched) fCode = suggestCode();
  });
  // Induk harus sekelas: ganti kelas -> induk lama yang tak cocok dilepas.
  $effect(() => {
    if (fParent && !rows.some((r) => r.id === fParent && r.class === fClass)) fParent = '';
  });
</script>

<main class="p-4 lg:p-6 space-y-3 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('accounting.accounts.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('accounting.accounts.subtitle')}</p>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      <div class="relative">
        <i class="icon-search absolute start-2.5 top-1/2 -translate-y-1/2 text-[13px] text-[var(--text-tertiary)]"></i>
        <input bind:value={search} placeholder={t('accounting.accounts.search')} class="h-9 w-52 ps-8 pe-2.5 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px] outline-none focus:border-[var(--color-primary-500)]" />
      </div>
      {#if can('journals', 'approve') && !openingMode && rows.length > 0}
        <button type="button" class="btn" onclick={startOpening}><i class="icon-scale"></i> {t('accounting.accounts.openingStart')}</button>
      {/if}
      {#if can('accounts', 'create')}
        <button type="button" class="btn btn-primary" onclick={() => (editing = 'new')}><i class="icon-plus"></i> {t('accounting.accounts.add')}</button>
      {/if}
    </div>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}

  {#if openingMode}
    <div class="surface-card !py-3 flex flex-wrap items-end gap-x-6 gap-y-3 text-[12.5px]">
      <div>
        <span class="font-semibold">{t('accounting.accounts.openingDate')}</span>
        <div class="mt-1 w-44"><DatePicker bind:value={openingDate} clearable={false} /></div>
      </div>
      <p class="flex-1 min-w-[16rem] text-[var(--text-secondary)]">{t('accounting.accounts.openingHint')}</p>
      <dl class="flex items-center gap-5 tabular-nums">
        <div><dt class="text-[11px] uppercase text-[var(--text-tertiary)]">{t('accounting.accounts.totalDebit')}</dt><dd class="font-semibold">{formatCurrency(totals.d / 100)}</dd></div>
        <div><dt class="text-[11px] uppercase text-[var(--text-tertiary)]">{t('accounting.accounts.totalCredit')}</dt><dd class="font-semibold">{formatCurrency(totals.c / 100)}</dd></div>
        <div>
          <dt class="text-[11px] uppercase text-[var(--text-tertiary)]">{t('accounting.accounts.diff')}</dt>
          <dd class="font-semibold {totals.diff === 0 ? 'text-[var(--color-success-600)]' : 'text-[var(--color-warning-700)]'}">{formatCurrency(Math.abs(totals.diff) / 100)}</dd>
        </div>
      </dl>
      <div class="flex gap-2">
        <button type="button" class="btn" onclick={() => (openingMode = false)} disabled={savingOpening}>{t('accounting.accounts.cancel')}</button>
        <button type="button" class="btn btn-primary" onclick={saveOpening} disabled={savingOpening || (totals.d === 0 && totals.c === 0)}><i class="icon-save"></i> {savingOpening ? t('accounting.accounts.saving') : t('accounting.accounts.openingSave')}</button>
      </div>
      {#if totals.diff !== 0}
        <p class="basis-full text-[12px] text-[var(--color-warning-700)]">{equityPlug ? t('accounting.accounts.openingPlugHint', { acc: `${equityPlug.code} ${equityPlug.name}` }) : t('accounting.accounts.openingUnbalanced')}</p>
      {/if}
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    {#if !loading && rows.length === 0}
      <div class="px-4 py-14 text-center space-y-3">
        <p class="text-[var(--text-tertiary)]">{t('accounting.accounts.seedHint')}</p>
        {#if can('accounts', 'create')}<button type="button" class="btn btn-primary" onclick={seed}><i class="icon-wand-sparkles"></i> {t('accounting.accounts.seed')}</button>{/if}
      </div>
    {:else}
      <div class="overflow-auto scroll-thin max-h-[calc(100vh-15rem)]">
        <table class="w-full min-w-[760px] text-[12px] border-collapse">
          <thead class="sticky top-0 z-10">
            <tr class="border-b border-[var(--border-default)] bg-[var(--surface-sunken)] text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
              <th class="px-3 py-2 text-start w-24" scope="col">{t('accounting.accounts.code')}</th>
              <th class="px-3 py-2 text-start" scope="col">{t('accounting.accounts.name')}</th>
              <th class="px-3 py-2 text-start w-28" scope="col">{t('accounting.accounts.class')}</th>
              <th class="px-3 py-2 text-end w-44" scope="col">{openingMode ? t('accounting.accounts.openingCol') : t('accounting.accounts.balance')}</th>
              <th class="px-2 py-2 w-20"><span class="sr-only">{t('accounting.accounts.edit')}</span></th>
            </tr>
          </thead>
          <tbody>
            {#each visible as r (r.id)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--color-primary)]/5 {r.kind === 'group' ? 'bg-[var(--surface-sunken)]/60' : ''} {r.active ? '' : 'opacity-60'}">
                <td class="px-3 py-1 font-mono whitespace-nowrap {r.kind === 'group' ? 'font-bold' : ''}">{r.code}</td>
                <td class="px-3 py-1" style="padding-inline-start: {0.75 + (search ? 0 : depthOf(r)) * 1.1}rem">
                  <span class={r.kind === 'group' ? 'font-bold' : ''}>{r.name}</span>
                  {#if r.is_system}<i class="icon-lock text-[11px] ms-1 text-[var(--text-tertiary)]" title={t('accounting.accounts.system')}></i>{/if}
                  {#if r.is_cash_bank}<span class="badge-soft badge-info ms-1 !py-0">{t('accounting.accounts.cashBank')}</span>{/if}
                  {#if !r.active}<span class="badge-soft ms-1 !py-0">{t('accounting.accounts.inactive')}</span>{/if}
                </td>
                <td class="px-3 py-1 text-[var(--text-secondary)]">{t(`accounting.class.${r.class}`)}</td>
                <td class="px-3 py-1 text-end tabular-nums">
                  {#if openingMode && r.kind === 'ledger' && r.active}
                    <MoneyInput bind:value={amounts[r.id]} placeholder="0" aria-label={r.name} class="w-full h-7 px-2 text-end rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[12px] outline-none focus:border-[var(--color-primary-500)]" />
                  {:else if !openingMode}
                    <span class={r.kind === 'group' ? 'font-bold' : ''}>{money(rolled(r))}</span>
                  {/if}
                </td>
                <td class="px-2 py-1 text-end whitespace-nowrap">
                  {#if can('accounts', 'update')}<button type="button" class="p-1 rounded hover:bg-[var(--color-primary)]/10" title={t('accounting.accounts.edit')} aria-label={t('accounting.accounts.edit')} onclick={() => (editing = r)}><i class="icon-pencil text-[13px]"></i></button>{/if}
                  {#if can('accounts', 'delete') && !r.is_system}<button type="button" class="p-1 rounded hover:bg-[var(--color-danger-600)]/10 text-[var(--color-danger-600)]" title={t('accounting.accounts.remove')} aria-label={t('accounting.accounts.remove')} onclick={() => remove(r)}><i class="icon-trash-2 text-[13px]"></i></button>{/if}
                </td>
              </tr>
            {:else}
              <tr><td colspan="5" class="px-4 py-14 text-center text-[var(--text-tertiary)]">{loading ? '…' : t('accounting.accounts.empty')}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</main>

{#if editing !== null}
  <Modal title={isNew ? t('accounting.accounts.form.titleNew') : t('accounting.accounts.form.titleEdit')} onclose={() => (editing = null)} wide>
    <form class="space-y-5 text-[13px]" onsubmit={save} autocomplete="off">
      <p class="rounded-lg bg-[var(--surface-sunken)] px-3 py-2.5 text-[12.5px] leading-relaxed text-[var(--text-secondary)]">
        <i class="icon-info text-[13px] me-1 align-[-1px]"></i>{isNew ? t('accounting.accounts.form.introNew') : t('accounting.accounts.form.introEdit')}
      </p>

      {#if isSys}
        <p class="flex gap-2 rounded-lg px-3 py-2.5 text-[12.5px] leading-relaxed badge-warning">
          <i class="icon-lock text-[14px] shrink-0 mt-0.5"></i><span>{t('accounting.accounts.form.systemNote')}</span>
        </p>
      {/if}

      <!-- 1. Jenis akun -->
      <fieldset class="space-y-2" disabled={isSys}>
        <legend class="font-semibold text-[13.5px]"><span class="step-no">1</span> {t('accounting.accounts.form.q1')}</legend>
        <div class="grid sm:grid-cols-2 lg:grid-cols-3 gap-2">
          {#each CLASSES as c (c)}
            <label class="class-card {fClass === c ? 'is-on' : ''} {isSys ? 'opacity-60' : 'cursor-pointer'}">
              <input type="radio" class="sr-only" name="acc-class" value={c} bind:group={fClass} />
              <span class="font-semibold">{t(`accounting.accounts.form.cls.${c}.title`)}</span>
              <span class="text-[11.5px] leading-snug text-[var(--text-secondary)]">{t(`accounting.accounts.form.cls.${c}.desc`)}</span>
              <span class="text-[11px] italic text-[var(--text-tertiary)]">{t(`accounting.accounts.form.cls.${c}.eg`)}</span>
            </label>
          {/each}
        </div>
        {#if fields.class}<p class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fields.class)}</p>{/if}
      </fieldset>

      <!-- 2. Bentuk akun (hanya saat membuat) -->
      {#if isNew}
        <fieldset class="space-y-2">
          <legend class="font-semibold text-[13.5px]"><span class="step-no">2</span> {t('accounting.accounts.form.q2')}</legend>
          <div class="grid sm:grid-cols-2 gap-2">
            {#each ['ledger', 'group'] as const as k (k)}
              <label class="class-card cursor-pointer {fKind === k ? 'is-on' : ''}">
                <input type="radio" class="sr-only" name="acc-kind" value={k} bind:group={fKind} />
                <span class="font-semibold">{t(`accounting.accounts.form.kind.${k}.title`)}</span>
                <span class="text-[11.5px] leading-snug text-[var(--text-secondary)]">{t(`accounting.accounts.form.kind.${k}.desc`)}</span>
              </label>
            {/each}
          </div>
        </fieldset>
      {/if}

      <!-- 3. Identitas -->
      <div class="space-y-3">
        <p class="font-semibold text-[13.5px]"><span class="step-no">{isNew ? 3 : 2}</span> {t('accounting.accounts.form.q3')}</p>
        <label class="block">
          <span class="font-semibold">{t('accounting.accounts.form.nameLabel')}</span>
          <input bind:value={fName} maxlength="120" placeholder={t('accounting.accounts.form.namePh')} class={inputClass} />
          <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.accounts.form.nameHint')}</span>
          {#if fields.name}<span class="block text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fields.name)}</span>{/if}
        </label>
        <div class="grid sm:grid-cols-2 gap-3">
          <div>
            <span class="font-semibold">{t('accounting.accounts.form.parentLabel')}</span>
            <div class="mt-1"><Select bind:value={fParent} disabled={isSys} options={parentOptions} ariaLabel={t('accounting.accounts.parent')} /></div>
            <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.accounts.form.parentHint')}</span>
          </div>
          <label class="block">
            <span class="font-semibold">{t('accounting.accounts.form.codeLabel')}</span>
            <input bind:value={fCode} oninput={() => (codeTouched = true)} disabled={isSys} maxlength="32" class="{inputClass} font-mono" />
            <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.accounts.form.codeHint')}</span>
            {#if fields.code}<span class="block text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fields.code)}</span>{/if}
          </label>
        </div>
      </div>

      {#if canCash}
        <div class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-1">
          <Switch bind:checked={fCash} disabled={isSys} label={t('accounting.accounts.form.cashLabel')} />
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.accounts.form.cashHint')}</p>
        </div>
      {/if}
      {#if !isNew}
        <div class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-1">
          <Switch bind:checked={fActive} disabled={isSys} label={t('accounting.accounts.form.activeLabel')} />
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.accounts.form.activeHint')}</p>
        </div>
      {/if}

      <!-- Ringkasan hidup -->
      <div class="rounded-lg border border-[var(--color-primary-500)]/40 bg-[var(--color-primary)]/5 p-3 space-y-1">
        <p class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.accounts.form.previewTitle')}</p>
        <p class="text-[13px]">
          <span class="font-mono font-semibold">{fCode.trim() || '—'}</span> · <span class="font-semibold">{fName.trim() || t('accounting.accounts.form.previewNoName')}</span>
        </p>
        <p class="text-[12px] text-[var(--text-secondary)]">
          {t('accounting.accounts.form.previewWhere', { cls: t(`accounting.accounts.form.cls.${fClass}.title`), parent: parentLabel })}
          {fKind === 'group' ? t('accounting.accounts.form.previewGroup') : t('accounting.accounts.form.previewLedger')}
        </p>
        <p class="text-[12px] text-[var(--text-secondary)]">
          {fSide === 'debit' ? t('accounting.accounts.form.sideDebit') : t('accounting.accounts.form.sideCredit')}
        </p>
      </div>

      {#if formError}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{formError}</p>{/if}
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={() => (editing = null)} disabled={busy}>{t('accounting.accounts.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={busy || !fCode.trim() || !fName.trim()}>{busy ? t('accounting.accounts.saving') : t('accounting.accounts.save')}</button>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .step-no {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.35rem;
    height: 1.35rem;
    margin-inline-end: 0.25rem;
    border-radius: 9999px;
    background: var(--color-primary-500);
    color: #fff;
    font-size: 11.5px;
    font-weight: 700;
  }
  .class-card {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: 0.65rem 0.8rem;
    border: 1.5px solid var(--border-default);
    border-radius: 0.6rem;
    background: var(--surface-base);
    transition: border-color 0.12s, background 0.12s;
  }
  .class-card:hover:not(.opacity-60) {
    border-color: var(--color-primary-500);
  }
  .class-card.is-on {
    border-color: var(--color-primary-500);
    background: color-mix(in srgb, var(--color-primary-500) 9%, var(--surface-base));
    box-shadow: 0 0 0 1px var(--color-primary-500);
  }
  .class-card:has(input:focus-visible) {
    outline: 2px solid var(--color-primary-500);
    outline-offset: 2px;
  }
</style>
