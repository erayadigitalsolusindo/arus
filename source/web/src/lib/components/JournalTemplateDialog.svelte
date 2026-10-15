<script lang="ts">
  // Dialog template jurnal: mode 'pick' = pilih template (milik pengguna + bawaan), mode 'save' = buat template baru
  // dari akun + sisi (debit/kredit) saja, tanpa nominal. Minimal 2 baris, sedikitnya satu debit dan satu kredit.
  import Modal from '#lib/components/Modal.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import type { SavedTemplate, SavedTemplateInput } from '#lib/accounting/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  type Side = 'debit' | 'credit';
  type Builtin = { id: string; type: string; name: string; debit: string; credit: string };
  type Seed = { type: string; narration: string; lines: { account: string; label: string; side: Side; memo: string }[] };

  let {
    mode,
    saved = [],
    builtin = [],
    seed = { type: 'JU', narration: '', lines: [] },
    search,
    onpick,
    onpickBuiltin,
    onremove,
    onsave,
    onclose
  }: {
    mode: 'pick' | 'save';
    saved?: SavedTemplate[];
    builtin?: Builtin[];
    seed?: Seed;
    search: (q: string) => Promise<Option[]>;
    onpick: (tp: SavedTemplate) => void;
    onpickBuiltin: (id: string) => void;
    onremove: (tp: SavedTemplate) => Promise<void>;
    onsave: (input: SavedTemplateInput) => Promise<void>;
    onclose: () => void;
  } = $props();

  const TYPES = ['JU', 'KM', 'KK', 'TK'] as const;
  const badge: Record<string, string> = { JU: 'badge-info', KM: 'badge-success', KK: 'badge-danger', TK: 'badge-warning' };
  const inputClass = 'w-full h-9 px-2.5 rounded-md border border-[var(--border-default)] bg-[var(--surface-base)] text-[12.5px] outline-none focus:border-[var(--color-primary-500)]';

  // ---- pilih ----
  let q = $state('');
  let error = $state('');
  const needle = $derived(q.trim().toLowerCase());
  const label = (tp: SavedTemplate, side: Side) => tp.lines.filter((l) => l.side === side).map((l) => `${l.account_code} · ${l.account_name}`).join(', ');
  const mine = $derived(saved.filter((tp) => !needle || `${tp.name} ${tp.narration} ${label(tp, 'debit')} ${label(tp, 'credit')}`.toLowerCase().includes(needle)));
  const base = $derived(builtin.filter((b) => !needle || `${b.name} ${b.debit} ${b.credit}`.toLowerCase().includes(needle)));

  async function remove(tp: SavedTemplate) {
    error = '';
    try {
      await onremove(tp);
    } catch (e) {
      error = errorMessage(e);
    }
  }

  // ---- simpan ----
  let seq = 0;
  type Row = { k: number; account: string; label: string; side: Side; memo: string };
  const blank = (side: Side): Row => ({ k: ++seq, account: '', label: '', side, memo: '' });
  const start = (() => {
    const rs: Row[] = seed.lines.map((l) => ({ ...l, k: ++seq }));
    while (rs.length < 2) rs.push(blank(rs.length === 0 ? 'debit' : 'credit'));
    return rs;
  })();
  /* svelte-ignore state_referenced_locally */
  let name = $state(seed.narration.trim().slice(0, 80));
  /* svelte-ignore state_referenced_locally */
  let type = $state(seed.type);
  /* svelte-ignore state_referenced_locally */
  let narration = $state(seed.narration);
  let rows = $state<Row[]>(start);
  let busy = $state(false);

  const valid = $derived(
    name.trim() !== '' && rows.length >= 2 && rows.every((r) => r.account !== '') && rows.some((r) => r.side === 'debit') && rows.some((r) => r.side === 'credit')
  );

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy) return;
    busy = true;
    error = '';
    try {
      await onsave({
        name: name.trim(),
        type,
        narration: narration.trim(),
        lines: rows.map((r) => ({ account_id: r.account, side: r.side, memo: r.memo.trim() }))
      });
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

{#if mode === 'pick'}
  <Modal title={t('accounting.journals.tplPick')} {onclose} wide>
    <div class="space-y-3 text-[12.5px]">
      <div class="relative">
        <i class="icon-search absolute start-3 top-2.5 text-[14px] text-[var(--text-tertiary)] pointer-events-none"></i>
        <input bind:value={q} class={inputClass} style="padding-inline-start:2.1rem" placeholder={t('accounting.journals.tplSearch')} aria-label={t('accounting.journals.tplSearch')} autocomplete="off" />
      </div>
      <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('accounting.journals.tplHint')}</p>
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

      <div class="max-h-[56vh] overflow-y-auto scroll-thin space-y-4 pe-1">
        {#if mine.length}
          <section>
            <h3 class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.tplMine')}</h3>
            <ul class="grid gap-1.5 sm:grid-cols-2">
              {#each mine as tp (tp.id)}
                <li class="group relative rounded-lg border border-[var(--color-primary-500)]/40 bg-[color-mix(in_srgb,var(--color-primary-500)_5%,transparent)] hover:border-[var(--color-primary-500)]">
                  <button type="button" class="block w-full px-3 py-2 text-start" onclick={() => onpick(tp)}>
                    <span class="flex items-center gap-2"><i class="icon-bookmark text-[12px] text-[var(--color-primary-600)]"></i><span class="font-semibold truncate">{tp.name}</span><span class="badge-soft {badge[tp.type] ?? 'badge-info'} !text-[10px] ms-auto me-6">{tp.type}</span></span>
                    <span class="mt-1 block text-[11px] text-[var(--text-tertiary)] truncate"><b class="font-semibold">D</b> {label(tp, 'debit')}</span>
                    <span class="block text-[11px] text-[var(--text-tertiary)] truncate"><b class="font-semibold">K</b> {label(tp, 'credit')}</span>
                  </button>
                  <button type="button" class="absolute end-1.5 top-1.5 grid size-6 place-items-center rounded text-[var(--text-tertiary)] hover:text-[var(--color-danger-600)]" aria-label="{t('accounting.journals.tplDelete')}: {tp.name}" title={t('accounting.journals.tplDelete')} onclick={() => remove(tp)}><br><i class="icon-trash-2 text-[13px]"></i></button>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
        {#if base.length}
          <section>
            <h3 class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.tplBuiltin')}</h3>
            <ul class="grid gap-1.5 sm:grid-cols-2">
              {#each base as b (b.id)}
                <li>
                  <button type="button" class="block w-full rounded-lg border border-[var(--border-default)] px-3 py-2 text-start hover:border-[var(--color-primary-500)] hover:bg-[var(--surface-sunken)]" onclick={() => onpickBuiltin(b.id)}>
                    <span class="flex items-center gap-2"><span class="font-semibold truncate">{b.name}</span><span class="badge-soft {badge[b.type] ?? 'badge-info'} !text-[10px] ms-auto">{b.type}</span></span>
                    <span class="mt-1 block text-[11px] text-[var(--text-tertiary)] truncate"><b class="font-semibold">D</b> {b.debit}</span>
                    <span class="block text-[11px] text-[var(--text-tertiary)] truncate"><b class="font-semibold">K</b> {b.credit}</span>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
        {#if !mine.length && !base.length}
          <p class="py-8 text-center text-[var(--text-tertiary)]">{t('accounting.journals.tplEmpty')}</p>
        {/if}
      </div>
    </div>
  </Modal>
{:else}
  <Modal title={t('accounting.journals.tplSaveTitle')} {onclose} wide>
    <form class="space-y-3 text-[12.5px]" onsubmit={submit}>
      <p class="text-[12px] text-[var(--text-secondary)]">{t('accounting.journals.tplSaveHint')}</p>
      <div class="grid gap-2.5 sm:grid-cols-[1fr_auto]">
        <label class="block">
          <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.tplName')}</span>
          <input bind:value={name} maxlength="80" class="mt-1 {inputClass}" autocomplete="off" />
        </label>
        <div>
          <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.type')}</span><br>
          <div class="mt-1 inline-flex rounded-lg border border-[var(--border-default)] p-0.5" role="group" aria-label={t('accounting.journals.type')}>
            {#each TYPES as v (v)}
              <button type="button" title={t(`accounting.type.${v}`)} class="h-8 rounded-md px-3 text-[12px] font-semibold {type === v ? 'bg-[var(--color-primary)] text-white' : 'text-[var(--text-secondary)]'}" aria-pressed={type === v} onclick={() => (type = v)}>{v}</button>
            {/each}
          </div>
        </div>
      </div>
      <label class="block">
        <span class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{t('accounting.journals.narration')}</span>
        <input bind:value={narration} maxlength="500" class="mt-1 {inputClass}" autocomplete="off" />
      </label>

      <div class="rounded-lg border border-[var(--border-default)] divide-y divide-[var(--border-subtle)]">
        {#each rows as r, i (r.k)}
          <div class="grid items-center gap-2 p-2 sm:grid-cols-[1.5rem_minmax(0,1fr)_auto_11rem_auto]">
            <span class="text-[11px] text-[var(--text-tertiary)] tabular-nums">{i + 1}</span>
            <Combobox bind:value={r.account} bind:label={r.label} {search} placeholder={t('accounting.journals.accountPick')} clearable={false} />
            <div class="inline-flex rounded-lg border border-[var(--border-default)] p-0.5" role="group" aria-label={t('accounting.journals.tplSide')}>
              {#each ['debit', 'credit'] as const as s (s)}
                <button type="button" class="h-7 rounded-md px-2.5 text-[12px] font-semibold {r.side === s ? (s === 'debit' ? 'bg-[var(--color-primary)] text-white' : 'bg-[var(--color-success-600,#16a34a)] text-white') : 'text-[var(--text-secondary)]'}" aria-pressed={r.side === s} onclick={() => (r.side = s)}>{t(`accounting.journals.${s}`)}</button>
              {/each}
            </div>
            <input bind:value={r.memo} maxlength="200" class={inputClass} placeholder={t('accounting.journals.memo')} aria-label={t('accounting.journals.memo')} />
            <button type="button" class="header-icon-btn !size-7" disabled={rows.length <= 2} aria-label={t('accounting.journals.removeLine')} title={t('accounting.journals.removeLine')} onclick={() => (rows = rows.filter((x) => x.k !== r.k))}><i class="icon-trash-2 text-[13px]"></i></button>
          </div>
        {/each}
        <div class="p-2"><button type="button" class="btn btn-sm" onclick={() => (rows = [...rows, blank('credit')])}><i class="icon-plus"></i> {t('accounting.journals.addLine')}</button></div>
      </div>
      {#if !valid}<p class="text-[11.5px] text-[var(--text-tertiary)]"><i class="icon-info text-[12px]"></i> {t('accounting.journals.tplNeedRows')}</p>{/if}
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('accounting.journals.close')}</button>
        <button type="submit" class="btn btn-primary" disabled={!valid || busy}><i class="icon-bookmark-plus"></i> {t('accounting.journals.tplSaveDo')}</button>
      </div>
    </form>
  </Modal>
{/if}
