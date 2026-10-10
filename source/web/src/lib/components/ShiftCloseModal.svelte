<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '#lib/components/Modal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Select from '#lib/components/Select.svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { approvals, type Approver } from '#lib/approval/api.ts';
  import { t, formatCurrency, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { shifts, type Shift } from '#lib/shift/api.ts';
  import { signedCents, shiftLines } from '#lib/pos/report-receipts.ts';
  import { loadReceiptSettings, printErrorMessage, printLines } from '#lib/pos/receipt.ts';

  let {
    shiftId,
    onclose,
    onclosed,
    onopennew
  }: { shiftId: string; onclose: () => void; onclosed: (s: Shift) => void; onopennew?: () => void } = $props();

  let recap = $state<Shift | null>(null);
  let counted = $state<Record<string, string>>({});
  let note = $state('');
  let approverId = $state('');
  let pin = $state('');
  let approvers = $state<Approver[] | null>(null);
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');
  let showDetail = $state(false);
  let done = $state<Shift | null>(null);
  let printing = $state(false);
  let printMsg = $state('');
  // Satu kunci per jendela tutup shift: kirim ulang (jaringan putus) tidak menutup dua kali.
  const key = 'shift-' + crypto.randomUUID();

  async function load(keep = false) {
    error = '';
    try {
      const r = await shifts.get(shiftId);
      // Setiap metode wajib punya kunci (MoneyInput tidak menerima bind ke undefined); isian lama dipertahankan saat muat ulang.
      const next: Record<string, string> = {};
      for (const c of r.counts) next[c.method_id] = keep ? (counted[c.method_id] ?? '') : '';
      counted = next;
      recap = r;
    } catch (e) {
      error = errorMessage(e);
    }
  }
  onMount(() => void load());

  const fmt = (c: bigint) => formatCurrency(Number(c) / 100);
  const money = (s: string | null | undefined) => fmt(signedCents(s));
  const filled = (id: string) => (counted[id] ?? '') !== '';
  const diffOf = (id: string, expected: string) => (filled(id) ? signedCents(counted[id]) - signedCents(expected) : null);

  const rows = $derived(recap?.counts ?? []);
  const allFilled = $derived(rows.length > 0 && rows.every((c) => filled(c.method_id)));
  const totals = $derived.by(() => {
    let exp = 0n, cnt = 0n, diff = 0n, abs = 0n;
    for (const c of rows) {
      exp += signedCents(c.expected);
      const d = diffOf(c.method_id, c.expected);
      if (d !== null) {
        cnt += signedCents(counted[c.method_id]);
        diff += d;
        abs += d < 0n ? -d : d;
      }
    }
    return { exp, cnt, diff, abs };
  });
  const needApproval = $derived(allFilled && totals.abs !== 0n);
  const valid = $derived(allFilled && (!needApproval || (note.trim().length >= 3 && approverId !== '' && /^\d{6}$/.test(pin))));

  // Daftar penyetuju baru dimuat saat dibutuhkan (ada selisih).
  $effect(() => {
    if (needApproval && approvers === null) {
      approvers = [];
      approvals
        .approvers('shift_close')
        .then((l) => {
          approvers = l;
          if (l.length === 1) approverId = l[0].id;
        })
        .catch((e) => (error = errorMessage(e)));
    }
  });

  function fillExpected() {
    for (const c of rows) counted[c.method_id] = signedCents(c.expected) < 0n ? '0' : (Number(signedCents(c.expected)) / 100).toFixed(2);
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!valid || busy || !recap) return;
    busy = true;
    error = '';
    notice = '';
    try {
      const res = await shifts.close(
        recap.id,
        {
          counts: rows.map((c) => ({ method_id: c.method_id, counted: counted[c.method_id] })),
          note: note.trim(),
          approval: needApproval ? { user_id: approverId, pin } : undefined
        },
        key
      );
      done = res;
      onclosed(res);
      if (loadReceiptSettings().auto) void print();
    } catch (err) {
      pin = '';
      if (err instanceof ApiError && err.code === 'SHIFT_RECAP_CHANGED') {
        await load(true);
        notice = t('shift.closing.changed');
      } else if (err instanceof ApiError && err.code === 'PIN_LOCKED') {
        error = errorMessage(err).replace('{count}', String(err.retryAfter));
      } else error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function print() {
    if (!done || printing) return;
    printing = true;
    printMsg = '';
    try {
      const s = loadReceiptSettings();
      await printLines(shiftLines(done, s.paper), s);
      printMsg = t('shift.closed.printed');
    } catch (e) {
      printMsg = printErrorMessage(e);
    } finally {
      printing = false;
    }
  }

  const flowLabel = (src: string) => t(`pos.today.flows.${src}` as MessageKey);
  const diffClass = (d: bigint | null) =>
    d === null || d === 0n ? 'text-[var(--text-tertiary)]' : d < 0n ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-warning-700)]';
  const signedFmt = (d: bigint) => (d > 0n ? '+' : '') + fmt(d);
</script>

<Modal title={done ? t('shift.closed.title', { doc: done.doc_no }) : t('shift.closing.title', { doc: recap?.doc_no ?? '' })} wide {onclose}>
  {#if done}
    <div class="space-y-4 text-[13px]">
      <p class="text-[var(--text-secondary)]">{t('shift.closed.body')}</p>
      <dl class="grid grid-cols-3 gap-2">
        <div class="rounded-md bg-[var(--surface-sunken)] p-3"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.closing.totalExpected')}</dt><dd class="font-bold tabular-nums">{money(done.expected_total)}</dd></div>
        <div class="rounded-md bg-[var(--surface-sunken)] p-3"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.closing.totalCounted')}</dt><dd class="font-bold tabular-nums">{money(done.counted_total)}</dd></div>
        <div class="rounded-md bg-[var(--surface-sunken)] p-3"><dt class="text-[11px] text-[var(--text-tertiary)]">{t('shift.closing.totalDiff')}</dt><dd class="font-bold tabular-nums {diffClass(signedCents(done.diff_total))}">{signedFmt(signedCents(done.diff_total))}</dd></div>
      </dl>
      {#if printMsg}<p class="text-[12.5px] text-[var(--text-secondary)]" role="status">{printMsg}</p>{/if}
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class="btn" disabled={printing} onclick={print}><i class={printing ? 'icon-loader-circle animate-spin' : 'icon-printer'}></i> {printing ? t('shift.closed.printing') : t('shift.closed.print')}</button>
        {#if onopennew}<button type="button" class="btn" onclick={onopennew}><i class="icon-lock-open"></i> {t('shift.closed.openNew')}</button>{/if}
        <button type="button" class="btn btn-primary" onclick={onclose}>{t('shift.closed.done')}</button>
      </div>
    </div>
  {:else if !recap}
    <p class="text-[13px] text-[var(--text-tertiary)]">{error || t('shift.closing.loading')}</p>
  {:else}
    <form class="space-y-4 text-[13px]" onsubmit={submit} autocomplete="off">
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('shift.closing.body')}</p>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-[var(--text-secondary)]">
        <span>{t('shift.openAt', { time: recap.opened_local })}</span>
        <span>{t('shift.closing.salesCount', { count: recap.sale_count, total: money(recap.sales_total) })}</span>
        {#if recap.void_count}<span>{t('shift.closing.voidCount', { count: recap.void_count })}</span>{/if}
        {#if signedCents(recap.receivable_total) > 0n}<span>{t('shift.closing.receivable', { amount: money(recap.receivable_total) })}</span>{/if}
      </div>

      <div class="overflow-x-auto rounded-md border border-[var(--border-subtle)]">
        <table class="w-full text-[13px]">
          <thead class="bg-[var(--surface-sunken)] text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)]">
            <tr>
              <th class="text-start px-3 py-2">{t('shift.closing.method')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.expected')}</th>
              <th class="text-end px-3 py-2 w-48">{t('shift.closing.counted')}</th>
              <th class="text-end px-3 py-2">{t('shift.closing.diff')}</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as c, i (c.method_id)}
              {@const d = diffOf(c.method_id, c.expected)}
              <tr class="border-t border-[var(--border-subtle)]">
                <td class="px-3 py-2">
                  <div class="font-semibold">{c.name}</div>
                  {#if showDetail}
                    <div class="text-[11.5px] text-[var(--text-tertiary)] space-x-2">
                      {#if signedCents(c.opening)}<span>{t('shift.closing.opening')} {money(c.opening)}</span>{/if}
                      <span>{t('shift.closing.sales')} {money(c.sales)}</span>
                      {#if signedCents(c.flows)}<span>{t('shift.closing.flows')} {signedFmt(signedCents(c.flows))}</span>{/if}
                    </div>
                  {/if}
                </td>
                <td class="px-3 py-2 text-end tabular-nums">{money(c.expected)}</td>
                <td class="px-3 py-1.5">
                  <MoneyInput
                    id={`shift-count-${i}`}
                    bind:value={counted[c.method_id]}
                    placeholder="0"
                    aria-label={`${t('shift.closing.counted')} ${c.name}`}
                    class="w-full h-9 px-2 rounded-md text-end tabular-nums font-semibold border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
                  />
                </td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold {diffClass(d)}">{d === null ? '—' : signedFmt(d)}</td>
              </tr>
            {/each}
          </tbody>
          <tfoot class="border-t-2 border-[var(--border-default)] font-bold">
            <tr>
              <td class="px-3 py-2">{t('shift.closing.totalExpected')}</td>
              <td class="px-3 py-2 text-end tabular-nums">{fmt(totals.exp)}</td>
              <td class="px-3 py-2 text-end tabular-nums">{allFilled ? fmt(totals.cnt) : '—'}</td>
              <td class="px-3 py-2 text-end tabular-nums {diffClass(allFilled ? totals.diff : null)}">{allFilled ? signedFmt(totals.diff) : '—'}</td>
            </tr>
          </tfoot>
        </table>
      </div>

      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-sm" onclick={fillExpected}><i class="icon-copy-check"></i> {t('shift.closing.fillExpected')}</button>
        <button type="button" class="btn btn-sm" onclick={() => (showDetail = !showDetail)}><i class="icon-list"></i> {t('shift.closing.detail')}</button>
        <button type="button" class="btn btn-sm" onclick={() => load(true)}><i class="icon-refresh-cw"></i> {t('shift.closing.reload')}</button>
      </div>

      {#if showDetail && recap.flows.length}
        <ul class="text-[12px] rounded-md bg-[var(--surface-sunken)] p-3 space-y-0.5">
          {#each recap.flows as f (f.source + f.method_id)}
            <li class="flex justify-between gap-3"><span>{flowLabel(f.source)} · {f.name} ({f.count})</span><span class="tabular-nums">{signedFmt(signedCents(f.amount))}</span></li>
          {/each}
        </ul>
      {/if}

      {#if needApproval}
        <div class="space-y-3 rounded-md border border-[color-mix(in_oklab,var(--color-warning-500)_45%,transparent)] bg-[color-mix(in_oklab,var(--color-warning-500)_8%,transparent)] p-3">
          <p class="text-[12.5px] font-semibold text-[var(--color-warning-700)]"><i class="icon-triangle-alert"></i> {t('shift.closing.diffWarn')}</p>
          <label class="block">
            <span class="font-semibold">{t('shift.closing.note')}</span>
            <textarea bind:value={note} maxlength="500" rows="2" placeholder={t('shift.closing.notePh')} class="mt-1 w-full px-3 py-2 rounded border border-[var(--border-default)] bg-[var(--surface-base)]"></textarea>
          </label>
          <div class="grid sm:grid-cols-2 gap-3">
            <label class="block">
              <span class="font-semibold">{t('shift.closing.approver')}</span>
              <div class="mt-1"><Select bind:value={approverId} class="!min-h-10" options={[{ value: '', label: t('shift.closing.pickApprover'), disabled: true }, ...(approvers ?? []).map((a) => ({ value: a.id, label: a.name }))]} /></div>
              {#if approvers && approvers.length === 0}<span class="text-[12px] text-[var(--color-warning-600)]">{t('shift.closing.noApprovers')}</span>{/if}
            </label>
            <label class="block">
              <span class="font-semibold">{t('shift.closing.pin')}</span>
              <input type="password" inputmode="numeric" pattern="[0-9]*" maxlength="6" bind:value={pin} autocomplete="new-password" class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] tracking-[0.4em]" />
            </label>
          </div>
        </div>
      {/if}

      {#if notice}<p role="status" class="text-[12.5px] text-[var(--color-warning-700)]">{notice}</p>{/if}
      {#if error}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{error}</p>{/if}

      <div class="flex justify-end gap-2">
        <button type="button" class="btn" onclick={onclose} disabled={busy}>{t('shift.closing.cancel')}</button>
        <button type="submit" class="btn btn-primary" disabled={!valid || busy}><i class="icon-lock"></i> {busy ? t('shift.closing.busy') : t('shift.closing.submit')}</button>
      </div>
    </form>
  {/if}
</Modal>
