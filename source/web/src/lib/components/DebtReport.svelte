<script lang="ts">
  // Laporan hutang (pemasok) & piutang (member): beredar, pembayaran, saldo per pihak, buku bantu, dan
  // saldo di pemasok — satu komponen, satu rute pembungkus per laporan. Paginasi keyset ("Muat Lagi");
  // ringkasan hanya dihitung server di halaman pertama.
  import { onMount } from 'svelte';
  import { later } from '#lib/debounce.svelte.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { members } from '#lib/members/api.ts';
  import {
    debtReport,
    debtReportFileUrl,
    type BalanceRow,
    type Buckets,
    type CreditRow,
    type CreditSummary,
    type DebtKind,
    type DebtParams,
    type DebtSummary,
    type DebtView,
    type LedgerRow,
    type LedgerSummary,
    type OutstandingRow,
    type PaymentRow,
    type PaymentSummary
  } from '#lib/reports/debt/api.ts';
  import { t, formatCurrency, formatDate, formatNumber, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';

  let { kind, view }: { kind: DebtKind; view: DebtView } = $props();

  const P = 'reports.debtReport';
  const C = `${P}.c`;
  const payable = $derived(kind === 'payable');
  const titleKey = $derived(view === 'credit' ? `${P}.credit` : `${P}.${kind}.${view}`);
  const fileBase = $derived(view === 'credit' ? 'saldo-di-pemasok' : `${payable ? 'hutang' : 'piutang'}-${view}`);

  // Filter yang dipakai tiap laporan.
  const useAsOf = $derived(view === 'outstanding' || view === 'balances');
  const useRange = $derived(view === 'payments' || view === 'ledger' || view === 'credit');
  const useParty = $derived(view === 'outstanding' || view === 'payments' || view === 'ledger');
  const useOutlet = $derived(view !== 'credit');
  const useSearch = $derived(view !== 'ledger');
  const needParty = $derived(view === 'ledger');

  let asOf = $state('');
  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let partyId = $state('');
  let partyLabel = $state('');
  let q = $state('');

  let loading = $state(true);
  let loadingMore = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let rows = $state<(OutstandingRow | BalanceRow | PaymentRow | LedgerRow | CreditRow)[]>([]);
  let summary = $state<DebtSummary | PaymentSummary | LedgerSummary | CreditSummary | null>(null);
  let nextCursor = $state<string | undefined>(undefined);
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

  const params = (): DebtParams => ({
    as_of: useAsOf ? asOf : undefined,
    from: useRange ? from : undefined,
    to: useRange ? to : undefined,
    outlet_id: useOutlet ? outletId || undefined : undefined,
    party_id: useParty ? partyId || undefined : undefined,
    q: useSearch ? q.trim() || undefined : undefined,
    all_outlets: useOutlet && outletId === ''
  });
  const ready = $derived((useAsOf ? !!asOf : true) && (useRange ? !!from && !!to : true) && (needParty ? !!partyId : true));

  async function load() {
    const my = ++seq;
    nextCursor = undefined;
    if (!ready) {
      rows = [];
      summary = null;
      loading = false;
      return;
    }
    loading = true;
    error = '';
    try {
      const res = await debtReport(kind, view, params());
      if (my !== seq) return;
      rows = res.data as typeof rows;
      summary = res.has_summary ? (res.summary as typeof summary) : null;
      nextCursor = res.next_cursor;
    } catch (err) {
      if (my !== seq) return;
      error = errorMessage(err);
      rows = [];
      summary = null;
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
      const res = await debtReport(kind, view, { ...params(), cursor: nextCursor });
      if (my !== seq) return;
      rows = [...rows, ...(res.data as typeof rows)];
      nextCursor = res.next_cursor;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loadingMore = false;
    }
  }

  async function download(format: 'xlsx' | 'pdf') {
    if (!ready || exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(debtReportFileUrl(kind, view, params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${fileBase}_${useRange ? `${from}_${to}` : asOf}.${format}`;
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

  // Filter/pencarian berubah → muat dari awal (ditunda agar tidak menembak tiap ketikan).
  $effect(() => {
    void asOf;
    void outletId;
    void partyId;
    void q;
    return later(() => void load(), 350);
  });

  const searchOutlets = async (term: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${C}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!term) return all;
    const lower = term.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };
  const searchParty = async (term: string): Promise<Option[]> => {
    const found: Option[] = payable
      ? await lookup('suppliers').search(term)
      : (await members.list({ q: term, limit: 20 })).data.map((m) => ({ id: m.id, name: m.name }));
    return needParty ? found : [{ id: '', name: t(payable ? `${C}.allSuppliers` : `${C}.allMembers`) }, ...found];
  };

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${C}.allOutlets`) : '');
    if (partyId === '') partyLabel = needParty ? '' : t(payable ? `${C}.allSuppliers` : `${C}.allMembers`);
  });

  onMount(() => {
    document.title = t(`${titleKey}.docTitle` as MessageKey);
    asOf = ymd(new Date());
    to = ymd(new Date());
    from = view === 'payments' ? ago(6) : ymd(new Date(new Date().getFullYear(), new Date().getMonth(), 1));
    void refreshOutlets();
  });

  const bucketOf = (b: Buckets) => [
    { id: 'bCurrent', v: b.current, tone: '' },
    { id: 'b1', v: b.d1_30, tone: 'text-amber-600' },
    { id: 'b2', v: b.d31_60, tone: 'text-orange-600' },
    { id: 'b3', v: b.d61_90, tone: 'text-red-600' },
    { id: 'b4', v: b.over_90, tone: 'text-red-700 font-bold' }
  ];
  const lateTone = (bucket: number) => ['', 'text-amber-600', 'text-orange-600', 'text-red-600', 'text-red-700 font-bold'][bucket] ?? '';
  const typeLabel = (ty: LedgerRow['type']) => t(`${C}.${ty === 'invoice' ? 'typeInvoice' : ty === 'payment' ? 'typePayment' : 'typeReturn'}`);

  // Penyempit tipe untuk templat.
  const asRows = <T,>(r: unknown) => r as T[];
  const debtSum = $derived(summary as DebtSummary | null);
  const paySum = $derived(summary as PaymentSummary | null);
  const ledSum = $derived(summary as LedgerSummary | null);
  const credSum = $derived(summary as CreditSummary | null);

  const lbl = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const chip =
    'rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50';
  const card = 'text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]';
  const big = 'mt-0.5 font-display text-[20px] font-bold tabular-nums';
  const thS = 'px-3 py-3 text-start font-semibold';
  const thE = 'px-3 py-3 text-end font-semibold';
  const tdS = 'px-3 py-2';
  const tdE = 'px-3 py-2 text-end tabular-nums';
  const docLabel = $derived(t(payable ? `${C}.docPayable` : `${C}.docReceivable`));
  const partyLabelName = $derived(t(payable ? `${C}.supplier` : `${C}.member`));
  const colCount = $derived(
    { outstanding: 10, balances: 12, payments: payable ? 9 : 10, ledger: 8, credit: 8 }[view]
  );
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${titleKey}.title` as MessageKey)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${titleKey}.subtitle` as MessageKey)}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      {#if useAsOf}
        <div>
          <span class={lbl}>{t(`${C}.asOf`)}</span>
          <DatePicker bind:value={asOf} clearable={false} />
        </div>
      {/if}
      {#if useRange}
        <div class="md:col-span-2">
          <span class={lbl}>{t('reports.range')}</span>
          <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
        </div>
      {/if}
      {#if useOutlet}
        <div>
          <span class={lbl}>{t(`${C}.outlet`)}</span>
          <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t(`${C}.outlet`)} />
        </div>
      {/if}
      {#if useParty}
        <div>
          <span class={lbl}>{partyLabelName}</span>
          <Combobox bind:value={partyId} bind:label={partyLabel} search={searchParty} placeholder={partyLabelName} />
        </div>
      {/if}
      {#if useSearch}
        <div class="md:col-span-2">
          <span class={lbl}>{t(`${C}.search`)}</span>
          <input
            type="search"
            class="w-full field-control"
            placeholder={t(view === 'outstanding' ? (payable ? `${C}.searchDocPayable` : `${C}.searchDocReceivable`) : view === 'payments' ? `${C}.searchPayment` : `${C}.searchParty`)}
            aria-label={t(`${C}.search`)}
            maxlength="64"
            bind:value={q}
          />
        </div>
      {/if}
    </div>
    <div class="flex flex-wrap items-center gap-1.5">
      {#if useRange}
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
      {/if}
      <span class="inline-flex items-center gap-1.5 ms-auto">
        <button type="button" class={chip} disabled={!ready || exporting !== null} onclick={() => download('xlsx')}>{exporting === 'xlsx' ? '…' : 'Excel'}</button>
        <button type="button" class={chip} disabled={!ready || exporting !== null} onclick={() => download('pdf')}>{exporting === 'pdf' ? '…' : 'PDF'}</button>
      </span>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if needParty && !partyId}
    <div class="surface-card !p-8 text-center text-[13px] text-[var(--text-tertiary)]">{t(payable ? `${C}.pickSupplier` : `${C}.pickMember`)}</div>
  {:else}
    <!-- Kartu ringkasan (seluruh hasil filter, bukan hanya halaman yang tampil). -->
    {#if debtSum && (view === 'outstanding' || view === 'balances')}
      <section class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardBalance`)}</div><div class={big}>{money(debtSum.balance)}</div></div>
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardDocs`)}</div><div class={big}>{formatNumber(debtSum.doc_count)}</div></div>
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardParties`)}</div><div class={big}>{formatNumber(debtSum.party_count ?? 0)}</div></div>
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardOverdue`)}</div><div class="{big} text-red-600">{money(debtSum.overdue)}</div></div>
        </div>
        <div class="surface-card !p-4">
          <div class="{card} mb-2">{t(`${C}.aging`)}</div>
          <div class="grid gap-3 grid-cols-2 sm:grid-cols-5">
            {#each bucketOf(debtSum.buckets) as b (b.id)}
              <div><div class="text-[11.5px] text-[var(--text-tertiary)]">{t(`${C}.${b.id}` as MessageKey)}</div><div class="font-semibold tabular-nums {b.tone}">{money(b.v)}</div></div>
            {/each}
          </div>
        </div>
      </section>
    {:else if paySum && view === 'payments'}
      <section class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-3">
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardPayments`)}</div><div class={big}>{formatNumber(paySum.count)}</div></div>
          <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardPaidTotal`)}</div><div class={big}>{money(paySum.amount)}</div></div>
          {#if !payable}
            <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardFee`)}</div><div class={big}>{money(paySum.fee)}</div></div>
          {/if}
        </div>
        {#if paySum.by_method.length > 0}
          <div class="surface-card !p-4">
            <div class="{card} mb-2">{t(`${C}.byMethod`)}</div>
            <div class="flex flex-wrap gap-x-8 gap-y-2">
              {#each paySum.by_method as m (m.name)}
                <div><div class="text-[11.5px] text-[var(--text-tertiary)]">{m.name} · {formatNumber(m.count)}</div><div class="font-semibold tabular-nums">{money(m.amount)}</div></div>
              {/each}
            </div>
          </div>
        {/if}
      </section>
    {:else if ledSum && view === 'ledger'}
      <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardOpening`)}</div><div class={big}>{money(ledSum.opening)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardIncrease`)}</div><div class={big}>{money(ledSum.increase)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardDecrease`)}</div><div class={big}>{money(ledSum.decrease)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardClosing`)}</div><div class={big}>{money(ledSum.closing)}</div></div>
      </section>
    {:else if credSum && view === 'credit'}
      <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardSuppliers`)}</div><div class={big}>{formatNumber(credSum.supplier_count)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardOpening`)}</div><div class={big}>{money(credSum.opening)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.creditAdded`)}</div><div class={big}>{money(credSum.added)}</div></div>
        <div class="surface-card !p-4"><div class={card}>{t(`${C}.cardClosing`)}</div><div class={big}>{money(credSum.closing)}</div></div>
      </section>
    {/if}

    <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
      <div class="overflow-auto scroll-thin max-h-[70vh]">
        <table class="w-full min-w-[1000px] text-[12.5px] border-collapse">
          <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
            <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
              {#if view === 'outstanding'}
                <th class={thS} scope="col">{t(`${C}.date`)}</th>
                <th class={thS} scope="col">{docLabel}</th>
                <th class={thS} scope="col">{t(`${C}.ref`)}</th>
                <th class={thS} scope="col">{partyLabelName}</th>
                <th class={thS} scope="col">{t(`${C}.dueDate`)}</th>
                <th class={thE} scope="col">{t(`${C}.amount`)}</th>
                <th class={thE} scope="col">{t(`${C}.paid`)}</th>
                <th class={thE} scope="col">{t(`${C}.returned`)}</th>
                <th class={thE} scope="col">{t(`${C}.balance`)}</th>
                <th class={thE} scope="col">{t(`${C}.late`)}</th>
              {:else if view === 'balances'}
                <th class={thS} scope="col">{t(`${C}.code`)}</th>
                <th class={thS} scope="col">{partyLabelName}</th>
                <th class={thE} scope="col">{t(`${C}.docCount`)}</th>
                <th class={thE} scope="col">{t(`${C}.balance`)}</th>
                <th class={thE} scope="col">{t(`${C}.bCurrent`)}</th>
                <th class={thE} scope="col">{t(`${C}.b1`)}</th>
                <th class={thE} scope="col">{t(`${C}.b2`)}</th>
                <th class={thE} scope="col">{t(`${C}.b3`)}</th>
                <th class={thE} scope="col">{t(`${C}.b4`)}</th>
                <th class={thE} scope="col">{t(`${C}.amount`)}</th>
                <th class={thE} scope="col">{t(`${C}.paid`)}</th>
                <th class={thS} scope="col">{t(`${C}.oldestDue`)}</th>
              {:else if view === 'payments'}
                <th class={thS} scope="col">{t(`${C}.date`)}</th>
                <th class={thS} scope="col">{t(`${C}.payNo`)}</th>
                <th class={thS} scope="col">{t(`${C}.paidDoc`)}</th>
                <th class={thS} scope="col">{partyLabelName}</th>
                <th class={thS} scope="col">{t(`${C}.method`)}</th>
                <th class={thS} scope="col">{t(`${C}.ref`)}</th>
                <th class={thE} scope="col">{t(`${C}.amount`)}</th>
                {#if !payable}<th class={thE} scope="col">{t(`${C}.fee`)}</th>{/if}
                <th class={thS} scope="col">{t(`${C}.settlement`)}</th>
                <th class={thS} scope="col">{t(payable ? `${C}.byPayable` : `${C}.byReceivable`)}</th>
              {:else if view === 'ledger'}
                <th class={thS} scope="col">{t(`${C}.date`)}</th>
                <th class={thS} scope="col">{t(`${C}.type`)}</th>
                <th class={thS} scope="col">{docLabel}</th>
                <th class={thS} scope="col">{t(`${C}.ref`)}</th>
                <th class={thS} scope="col">{t(`${C}.memo`)}</th>
                <th class={thE} scope="col">{t(`${C}.increase`)}</th>
                <th class={thE} scope="col">{t(`${C}.decrease`)}</th>
                <th class={thE} scope="col">{t(`${C}.running`)}</th>
              {:else}
                <th class={thS} scope="col">{t(`${C}.code`)}</th>
                <th class={thS} scope="col">{t(`${C}.supplier`)}</th>
                <th class={thE} scope="col">{t(`${C}.cardOpening`)}</th>
                <th class={thE} scope="col">{t(`${C}.creditAdded`)}</th>
                <th class={thE} scope="col">{t(`${C}.creditUsed`)}</th>
                <th class={thE} scope="col">{t(`${C}.creditCashedOut`)}</th>
                <th class={thE} scope="col">{t(`${C}.creditVoided`)}</th>
                <th class={thE} scope="col">{t(`${C}.cardClosing`)}</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#if view === 'outstanding'}
              {#each asRows<OutstandingRow>(rows) as r (r.id)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="{tdS} whitespace-nowrap tabular-nums">{formatDate(r.doc_date)}</td>
                  <td class="{tdS} font-semibold whitespace-nowrap">{r.doc_no}</td>
                  <td class="{tdS} whitespace-nowrap">{r.ref_no}</td>
                  <td class={tdS}>{r.party_name}</td>
                  <td class="{tdS} whitespace-nowrap tabular-nums">{r.due_date ? formatDate(r.due_date) : '–'}</td>
                  <td class={tdE}>{money(r.amount)}</td>
                  <td class={tdE}>{money(r.paid)}</td>
                  <td class={tdE}>{money(r.returned)}</td>
                  <td class="{tdE} font-semibold">{money(r.balance)}</td>
                  <td class="{tdE} {lateTone(r.bucket)}">{r.days_overdue > 0 ? t(`${C}.lateDays`, { n: r.days_overdue }) : '–'}</td>
                </tr>
              {:else}
                <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
              {/each}
            {:else if view === 'balances'}
              {#each asRows<BalanceRow>(rows) as r (r.party_id)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="{tdS} whitespace-nowrap">{r.code}</td>
                  <td class="{tdS} font-semibold">{r.name}</td>
                  <td class={tdE}>{formatNumber(r.doc_count)}</td>
                  <td class="{tdE} font-semibold">{money(r.balance)}</td>
                  <td class={tdE}>{money(r.buckets.current)}</td>
                  <td class="{tdE} text-amber-600">{money(r.buckets.d1_30)}</td>
                  <td class="{tdE} text-orange-600">{money(r.buckets.d31_60)}</td>
                  <td class="{tdE} text-red-600">{money(r.buckets.d61_90)}</td>
                  <td class="{tdE} text-red-700">{money(r.buckets.over_90)}</td>
                  <td class={tdE}>{money(r.amount)}</td>
                  <td class={tdE}>{money(r.paid)}</td>
                  <td class="{tdS} whitespace-nowrap tabular-nums">{r.oldest_due ? formatDate(r.oldest_due) : '–'}</td>
                </tr>
              {:else}
                <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
              {/each}
            {:else if view === 'payments'}
              {#each asRows<PaymentRow>(rows) as r (r.id)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="{tdS} whitespace-nowrap tabular-nums">{formatDate(r.date)} <span class="text-[var(--text-tertiary)]">{r.time}</span></td>
                  <td class="{tdS} font-semibold whitespace-nowrap">{r.doc_no}</td>
                  <td class="{tdS} whitespace-nowrap">{r.target_doc_no}</td>
                  <td class={tdS}>{r.party_name}</td>
                  <td class="{tdS} whitespace-nowrap">{r.method}</td>
                  <td class="{tdS} whitespace-nowrap">{r.ref_no}</td>
                  <td class="{tdE} font-semibold">{money(r.amount)}</td>
                  {#if !payable}<td class={tdE}>{money(r.fee)}</td>{/if}
                  <td class="{tdS} whitespace-nowrap">{r.settlement_no}</td>
                  <td class={tdS}>{r.user_name}</td>
                </tr>
              {:else}
                <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
              {/each}
            {:else if view === 'ledger'}
              {#if ledSum}
                <tr class="bg-[var(--surface-sunken)] font-semibold">
                  <td class="{tdS} tabular-nums" colspan="7">{t(`${C}.openingRow`)} · {formatDate(from)}</td>
                  <td class={tdE}>{money(ledSum?.opening ?? 0)}</td>
                </tr>
              {/if}
              {#each asRows<LedgerRow>(rows) as r, i (i)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="{tdS} whitespace-nowrap tabular-nums">{formatDate(r.date)}</td>
                  <td class="{tdS} whitespace-nowrap">{typeLabel(r.type)}</td>
                  <td class="{tdS} font-semibold whitespace-nowrap">{r.doc_no}</td>
                  <td class="{tdS} whitespace-nowrap">{r.ref_no}</td>
                  <td class={tdS}>{r.memo}</td>
                  <td class={tdE}>{Number(r.increase) ? money(r.increase) : ''}</td>
                  <td class={tdE}>{Number(r.decrease) ? money(r.decrease) : ''}</td>
                  <td class="{tdE} font-semibold">{money(r.balance)}</td>
                </tr>
              {:else}
                <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
              {/each}
            {:else}
              {#each asRows<CreditRow>(rows) as r (r.supplier_id)}
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="{tdS} whitespace-nowrap">{r.code}</td>
                  <td class="{tdS} font-semibold">{r.name}</td>
                  <td class={tdE}>{money(r.opening)}</td>
                  <td class={tdE}>{money(r.added)}</td>
                  <td class={tdE}>{money(r.used)}</td>
                  <td class={tdE}>{money(r.cashed_out)}</td>
                  <td class={tdE}>{money(r.voided)}</td>
                  <td class="{tdE} font-semibold">{money(r.closing)}</td>
                </tr>
              {:else}
                <tr><td colspan={colCount} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${C}.noData`)}</td></tr>
              {/each}
            {/if}
          </tbody>
          {#if rows.length > 0 && debtSum && view === 'outstanding'}
            <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
              <tr class="border-t-2 border-[var(--border-subtle)]">
                <td class="px-3 py-2.5" colspan="5">{t(`${C}.grandTotal`)}</td>
                <td class={tdE}>{money(debtSum.amount)}</td>
                <td class={tdE}>{money(debtSum.paid)}</td>
                <td class={tdE}>{money(debtSum.returned)}</td>
                <td class={tdE}>{money(debtSum.balance)}</td>
                <td></td>
              </tr>
            </tfoot>
          {:else if rows.length > 0 && debtSum && view === 'balances'}
            <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
              <tr class="border-t-2 border-[var(--border-subtle)]">
                <td class="px-3 py-2.5" colspan="2">{t(`${C}.grandTotal`)}</td>
                <td class={tdE}>{formatNumber(debtSum.doc_count)}</td>
                <td class={tdE}>{money(debtSum.balance)}</td>
                <td class={tdE}>{money(debtSum.buckets.current)}</td>
                <td class={tdE}>{money(debtSum.buckets.d1_30)}</td>
                <td class={tdE}>{money(debtSum.buckets.d31_60)}</td>
                <td class={tdE}>{money(debtSum.buckets.d61_90)}</td>
                <td class={tdE}>{money(debtSum.buckets.over_90)}</td>
                <td class={tdE}>{money(debtSum.amount)}</td>
                <td class={tdE}>{money(debtSum.paid)}</td>
                <td></td>
              </tr>
            </tfoot>
          {:else if rows.length > 0 && paySum && view === 'payments'}
            <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
              <tr class="border-t-2 border-[var(--border-subtle)]">
                <td class="px-3 py-2.5" colspan="6">{t(`${C}.grandTotal`)}</td>
                <td class={tdE}>{money(paySum.amount)}</td>
                {#if !payable}<td class={tdE}>{money(paySum.fee)}</td>{/if}
                <td colspan="2"></td>
              </tr>
            </tfoot>
          {:else if rows.length > 0 && ledSum && view === 'ledger'}
            <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
              <tr class="border-t-2 border-[var(--border-subtle)]">
                <td class="px-3 py-2.5" colspan="5">{t(`${C}.cardClosing`)}</td>
                <td class={tdE}>{money(ledSum.increase)}</td>
                <td class={tdE}>{money(ledSum.decrease)}</td>
                <td class={tdE}>{money(ledSum.closing)}</td>
              </tr>
            </tfoot>
          {:else if rows.length > 0 && credSum && view === 'credit'}
            <tfoot class="sticky bottom-0 bg-[var(--surface-sunken)] font-semibold">
              <tr class="border-t-2 border-[var(--border-subtle)]">
                <td class="px-3 py-2.5" colspan="2">{t(`${C}.grandTotal`)}</td>
                <td class={tdE}>{money(credSum.opening)}</td>
                <td class={tdE}>{money(credSum.added)}</td>
                <td class={tdE}>{money(credSum.used)}</td>
                <td class={tdE}>{money(credSum.cashed_out)}</td>
                <td class={tdE}>{money(credSum.voided)}</td>
                <td class={tdE}>{money(credSum.closing)}</td>
              </tr>
            </tfoot>
          {/if}
        </table>
      </div>
      {#if view === 'credit'}
        <div class="px-4 py-2 text-[11.5px] text-[var(--text-tertiary)] border-t border-[var(--border-subtle)]">{t(`${C}.creditNote`)}</div>
      {/if}
      {#if nextCursor}
        <div class="p-3 text-center border-t border-[var(--border-subtle)]">
          <button type="button" class="btn-secondary text-[12.5px] px-4 py-2" onclick={loadMore} disabled={loadingMore}>
            {loadingMore ? '…' : t(`${C}.loadMore`)}
          </button>
        </div>
      {/if}
    </section>
  {/if}
</main>
