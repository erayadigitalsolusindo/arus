<script lang="ts">
  import { onMount } from 'svelte';
  import { later } from '#lib/debounce.svelte.ts';
  import {
    salesCashiers,
    salesCashiersFileUrl,
    type SalesCashiersResult,
    type CashierShift,
    type SalesCashiersParams
  } from '#lib/reports/sales/api.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { iam } from '#lib/iam/api.ts';

  const D = 'reports.salesCashier';
  const S = 'reports.salesDetail';

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let cashierId = $state('');
  let cashierLabel = $state('');
  let status = $state('');
  let diffOnly = $state(false);

  let loading = $state(true);
  let loadingMore = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let result = $state<SalesCashiersResult | null>(null);
  let rows = $state<CashierShift[]>([]);
  let nextCursor = $state<string | undefined>(undefined);
  let cashiers = $state<Option[]>([]);
  // Token permintaan: respons lama (filter sudah berganti) dibuang.
  let seq = 0;

  const money = (v: string | number) => formatCurrency(Number(v));

  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const ago = (daysBack: number) => {
    const d = new Date();
    d.setDate(d.getDate() - daysBack);
    return ymd(d);
  };
  const presets = $derived.by(() => {
    const now = new Date();
    return [
      { id: 'today' as const, from: ymd(now), to: ymd(now) },
      { id: 'days7' as const, from: ago(6), to: ymd(now) },
      { id: 'days30' as const, from: ago(29), to: ymd(now) },
      { id: 'month' as const, from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: ymd(now) }
    ];
  });
  const statusOptions = $derived([
    { value: '', label: t(`${D}.statusAll`) },
    { value: 'open', label: t(`${D}.statusOpen`) },
    { value: 'closed', label: t(`${D}.statusClosed`) }
  ]);

  const params = (): SalesCashiersParams => ({
    from,
    to,
    outlet_id: outletId || undefined,
    cashier_id: cashierId || undefined,
    status: status || undefined,
    diff_only: diffOnly || undefined,
    all_outlets: outletId === ''
  });

  async function load() {
    if (!from || !to) return;
    const my = ++seq;
    loading = true;
    error = '';
    nextCursor = undefined;
    try {
      const res = await salesCashiers(params());
      if (my !== seq) return;
      result = res;
      rows = res.data;
      nextCursor = res.next_cursor;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      result = null;
      rows = [];
    } finally {
      if (my === seq) loading = false;
    }
  }

  async function loadMore() {
    if (!nextCursor || loadingMore) return;
    const my = seq;
    loadingMore = true;
    error = '';
    try {
      const res = await salesCashiers({ ...params(), cursor: nextCursor });
      if (my !== seq) return;
      rows = [...rows, ...res.data];
      nextCursor = res.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loadingMore = false;
    }
  }

  async function download(format: 'xlsx' | 'pdf') {
    if (!from || !to || exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(salesCashiersFileUrl(params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `penjualan-perkasir_${from}_${to}.${format}`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 5000);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      exporting = null;
    }
  }

  function applyPreset(p: { from: string; to: string }) {
    from = p.from;
    to = p.to;
    void load();
  }

  // Klik baris kasir pada ringkasan = saring daftar shift ke kasir itu.
  function pickCashier(id?: string, name?: string) {
    if (!id) return;
    cashierId = cashierId === id ? '' : id;
    cashierLabel = cashierId ? (name ?? '') : t(`${S}.allCashiers`);
  }

  $effect(() => {
    void outletId;
    void cashierId;
    void status;
    void diffOnly;
    return later(() => void load(), 350);
  });

  const searchOutlets = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${S}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };
  const searchCashiers = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${S}.allCashiers`) }, ...cashiers];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((c) => c.name.toLowerCase().includes(lower));
  };

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${S}.allOutlets`) : '');
    cashierLabel = cashiers.find((c) => c.id === cashierId)?.name ?? (cashierId === '' ? t(`${S}.allCashiers`) : '');
  });

  const summary = $derived(result?.has_summary ? result.summary : null);
  const cashierRows = $derived(result?.cashiers ?? []);

  // Selisih: merah bila kurang, kuning bila lebih, netral bila 0.
  const diffClass = (v: string | number) => {
    const n = Number(v);
    return n < 0 ? 'text-[var(--color-danger-600,#dc2626)] font-semibold' : n > 0 ? 'text-[var(--color-warning-600,#d97706)] font-semibold' : 'text-[var(--text-tertiary)]';
  };
  const diffLabel = (v: string | number) => {
    const n = Number(v);
    return n < 0 ? t(`${D}.short`) : n > 0 ? t(`${D}.over`) : t(`${D}.balanced`);
  };

  onMount(() => {
    document.title = t(`${D}.docTitle`);
    from = ago(6);
    to = ymd(new Date());
    void refreshOutlets();
    iam
      .users()
      .then((users) => (cashiers = users.filter((u) => u.active).map((u) => ({ id: u.id, name: u.name }))))
      .catch(() => undefined);
  });

  const lbl = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const chip =
    'rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50';
  const card = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const th = 'px-3 py-3 font-semibold';
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${D}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${D}.subtitle`)}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class={lbl}>{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class={lbl}>{t(`${S}.outlet`)}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t(`${S}.outlet`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${S}.cashier`)}</span>
        <Combobox bind:value={cashierId} bind:label={cashierLabel} search={searchCashiers} placeholder={t(`${S}.cashier`)} />
      </div>
      <div>
        <span class={lbl}>{t(`${D}.statusCol`)}</span>
        <Select bind:value={status} options={statusOptions} ariaLabel={t(`${D}.statusCol`)} />
      </div>
      <label class="flex items-center gap-2 text-[12.5px] md:col-span-2 self-end pb-2">
        <input type="checkbox" bind:checked={diffOnly} />
        <span>{t(`${D}.diffOnly`)}</span>
      </label>
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      {#each presets as p (p.id)}
        {@const on = from === p.from && to === p.to}
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors {on
            ? 'border-[var(--color-primary-600)] bg-[var(--color-primary-600)] text-white'
            : 'border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)]'}"
          aria-pressed={on}
          onclick={() => applyPreset(p)}
        >{t(`payables.history.preset.${p.id}`)}</button>
      {/each}
      <span class="inline-flex items-center gap-1.5 ms-auto">
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('xlsx')}>{exporting === 'xlsx' ? '…' : 'Excel'}</button>
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('pdf')}>{exporting === 'pdf' ? '…' : 'PDF'}</button>
      </span>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if summary && summary.shifts > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.cashExpected`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.cash_expected)}</div>
        <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${D}.cashHint`)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.cashCounted`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.cash_counted)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.cashDiff`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums {diffClass(summary.cash_diff)}">
          {money(summary.cash_diff)}
          <span class="text-[12px] font-medium">{diffLabel(summary.cash_diff)}</span>
        </div>
        {#if summary.diff_shifts > 0}
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${D}.diffShifts`)}: {formatNumber(summary.diff_shifts)}</div>
        {/if}
      </div>
      <div class="surface-card !p-4">
        <div class={card}>{t(`${D}.sales`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.sales_total)}</div>
        <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">
          {formatNumber(summary.shifts)} {t(`${D}.shiftsLabel`)}
          {#if summary.open_shifts > 0}({formatNumber(summary.open_shifts)} {t(`${D}.openLabel`)}){/if}
          · {formatNumber(summary.sale_count)} {t(`${D}.notes`)}
        </div>
      </div>
    </section>
  {/if}

  {#if summary && summary.open_shifts > 0}
    <p class="text-[12px] text-[var(--text-tertiary)]">{t(`${D}.openNote`)}</p>
  {/if}

  {#if cashierRows.length > 0}
    <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}">
      <h2 class="px-4 pt-3 pb-2 font-display font-semibold text-[14px]">{t(`${D}.perCashier`)}</h2>
      <div class="overflow-auto scroll-thin">
        <table class="w-full min-w-[900px] text-[12.5px] border-collapse">
          <thead class="bg-[var(--surface-sunken)]">
            <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
              <th class="{th} text-start" scope="col">{t(`${D}.cashier`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.shifts`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.notes`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.voids`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.sales`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.cashExpected`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.cashCounted`)}</th>
              <th class="{th} text-end" scope="col">{t(`${D}.cashDiff`)}</th>
            </tr>
          </thead>
          <tbody>
            {#each cashierRows as c (c.user_id)}
              <tr
                class="border-t border-[var(--border-subtle)] cursor-pointer transition-colors hover:bg-[var(--surface-sunken)] {cashierId === c.user_id ? 'bg-[var(--surface-sunken)]' : ''}"
                onclick={() => pickCashier(c.user_id, c.name)}
              >
                <td class="px-3 py-2 font-semibold">
                  {c.name}
                  {#if c.methods.length > 0}
                    <div class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-[11px] font-normal text-[var(--text-tertiary)] tabular-nums">
                      {#each c.methods as m (m.name + m.kind)}
                        <span class="uppercase">{m.name} {money(m.expected)}{#if Number(m.diff) !== 0} <span class={diffClass(m.diff)}>({money(m.diff)})</span>{/if}</span>
                      {/each}
                    </div>
                  {/if}
                </td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(c.shifts)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(c.sale_count)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{formatNumber(c.void_count)}</td>
                <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(c.sales_total)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(c.cash_expected)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(c.cash_counted)}</td>
                <td class="px-3 py-2 text-end tabular-nums {diffClass(c.cash_diff)}">{money(c.cash_diff)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <h2 class="px-4 pt-3 pb-2 font-display font-semibold text-[14px]">{t(`${D}.perShift`)}</h2>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full min-w-[1100px] text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="{th} text-start" scope="col">{t(`${D}.date`)}</th>
            <th class="{th} text-start" scope="col">{t(`${D}.shiftNo`)}</th>
            <th class="{th} text-start" scope="col">{t(`${D}.cashier`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.notes`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.opening`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.sales`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.cashExpected`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.cashCounted`)}</th>
            <th class="{th} text-end" scope="col">{t(`${D}.cashDiff`)}</th>
            <th class="{th} text-start" scope="col">{t(`${D}.note`)}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
              <td class="px-3 py-2 whitespace-nowrap tabular-nums">
                {formatDate(r.opened_date)} <span class="text-[var(--text-tertiary)]">{r.opened_time}{#if r.closed_time}–{r.closed_time}{/if}</span>
              </td>
              <td class="px-3 py-2 font-semibold whitespace-nowrap">
                {r.doc_no}
                {#if r.status === 'open'}
                  <span class="ms-1 rounded px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide badge-warning">{t(`${D}.running`)}</span>
                {/if}
              </td>
              <td class="px-3 py-2">{r.cashier_name}</td>
              <td class="px-3 py-2 text-end tabular-nums">{formatNumber(r.sale_count)}</td>
              <td class="px-3 py-2 text-end tabular-nums">{money(r.opening_cash)}</td>
              <td class="px-3 py-2 text-end tabular-nums font-semibold">{money(r.sales_total)}</td>
              {#if r.status === 'open'}
                <td class="px-3 py-2 text-end text-[var(--text-tertiary)]" colspan="3">—</td>
              {:else}
                <td class="px-3 py-2 text-end tabular-nums">{money(r.cash_expected)}</td>
                <td class="px-3 py-2 text-end tabular-nums">{money(r.cash_counted)}</td>
                <td class="px-3 py-2 text-end tabular-nums {diffClass(r.cash_diff)}">{money(r.cash_diff)}</td>
              {/if}
              <td class="px-3 py-2 text-[11.5px] text-[var(--text-secondary)]">
                {r.note}
                {#if r.approved_by_name}<span class="text-[var(--text-tertiary)]"> · {t(`${D}.approvedBy`)}: {r.approved_by_name}</span>{/if}
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="10" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">
                {loading ? '…' : t(`${D}.noData`)}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if nextCursor}
      <div class="p-3 text-center border-t border-[var(--border-subtle)]">
        <button type="button" class="btn-secondary text-[12.5px] px-4 py-2" onclick={loadMore} disabled={loadingMore}>
          {loadingMore ? '…' : t(`${D}.loadMore`)}
        </button>
      </div>
    {/if}
  </section>
</main>
