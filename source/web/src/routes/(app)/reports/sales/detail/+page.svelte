<script lang="ts">
  import { onMount } from 'svelte';
  import {
    salesDetail,
    salesDetailFileUrl,
    type SalesDetailResult,
    type DetailDay,
    type DetailSale,
    type SalesDetailParams
  } from '#lib/reports/sales/api.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import { t, formatCurrency, formatDate, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import DateRange from '#lib/components/DateRange.svelte';
  import Select from '#lib/components/Select.svelte';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { iam } from '#lib/iam/api.ts';

  const D = 'reports.salesDetail';

  let from = $state('');
  let to = $state('');
  let outletId = $state('');
  let outletLabel = $state('');
  let cashierId = $state('');
  let cashierLabel = $state('');
  let categoryId = $state('');
  let categoryLabel = $state('');
  let status = $state('');

  let loading = $state(true);
  let loadingMore = $state(false);
  let exporting = $state<'xlsx' | 'pdf' | null>(null);
  let error = $state('');
  let result = $state<SalesDetailResult | null>(null);
  let days = $state<DetailDay[]>([]);
  let nextCursor = $state<string | undefined>(undefined);
  let cashiers = $state<Option[]>([]);
  // Nota yang footer rinciannya sedang dibuka (accordion per nota).
  let expanded = $state(new Set<string>());
  // Nota/hari yang ringkasan hariannya ditampilkan di pop up.
  let dayModal = $state<DetailDay | null>(null);

  const money = (v: string | number) => formatCurrency(Number(v));
  const qty = (v: string) => formatNumber(Number(v));

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
    { value: '', label: t(`${D}.allStatuses`) },
    { value: 'completed', label: t(`${D}.statusCompleted`) },
    { value: 'void', label: t(`${D}.statusVoid`) }
  ]);

  const params = (): SalesDetailParams => ({
    from,
    to,
    outlet_id: outletId || undefined,
    cashier_id: cashierId || undefined,
    category_id: categoryId || undefined,
    status: status || undefined,
    all_outlets: outletId === ''
  });

  // mergeDays menempelkan halaman baru ke daftar hari; batas hari bisa terpotong
  // antar halaman karena urutan keyset menurun per nota, bukan per hari.
  function mergeDays(base: DetailDay[], more: DetailDay[]): DetailDay[] {
    const out = base.map((d) => ({ ...d, sales: [...d.sales] }));
    for (const day of more) {
      const last = out[out.length - 1];
      if (last && last.date === day.date) {
        last.sales.push(...day.sales);
      } else {
        out.push({ ...day, sales: [...day.sales] });
      }
    }
    return out;
  }

  // toggleSale membuka/menutup footer rincian sebuah nota.
  function toggleSale(id: string) {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expanded = next;
  }

  async function load() {
    loading = true;
    error = '';
    nextCursor = undefined;
    expanded = new Set();
    dayModal = null;
    try {
      const res = await salesDetail(params());
      result = res;
      days = res.data;
      nextCursor = res.next_cursor;
    } catch (err) {
      error = errorMessage(err);
      result = null;
      days = [];
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!nextCursor || loadingMore) return;
    loadingMore = true;
    error = '';
    try {
      const res = await salesDetail({ ...params(), cursor: nextCursor });
      result = res;
      days = mergeDays(days, res.data);
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
      const blob = await apiBlob(salesDetailFileUrl(params(), format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `penjualan-detail_${from}_${to}.${format}`;
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

  // Filter lain_changed → muat dari awal (pencariankasir/kategori ditunda).
  $effect(() => {
    void outletId;
    void cashierId;
    void categoryId;
    void status;
    const h = setTimeout(() => void load(), 300);
    return () => clearTimeout(h);
  });

  const searchOutlets = async (q: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${D}.allOutlets`) }, ...accessibleOutlets.items.map((o) => ({ id: o.id, name: o.name }))];
    if (!q) return all;
    const lower = q.toLowerCase();
    return all.filter((o) => o.name.toLowerCase().includes(lower));
  };

  const searchCashiers = async (q: string): Promise<Option[]> => {
    const all = [{ id: '', name: t(`${D}.allCashiers`) }, ...cashiers];
    if (!q) return all;
    const lower = q.toLowerCase();
    return all.filter((c) => c.name.toLowerCase().includes(lower));
  };

  const searchCategories = async (q: string): Promise<Option[]> => {
    try {
      const cats = await lookup('categories').search(q, 50);
      return [{ id: '', name: t(`${D}.allCategories`) }, ...cats.map((c) => ({ id: c.id, name: c.name }))];
    } catch {
      return [{ id: '', name: t(`${D}.allCategories`) }];
    }
  };

  $effect(() => {
    const o = accessibleOutlets.items.find((x) => x.id === outletId);
    outletLabel = o?.name ?? (outletId === '' ? t(`${D}.allOutlets`) : '');
    cashierLabel = cashiers.find((c) => c.id === cashierId)?.name ?? (cashierId === '' ? t(`${D}.allCashiers`) : '');
  });

  const summary = $derived(result?.summary ?? null);

  onMount(() => {
    document.title = t(`${D}.docTitle`);
    from = ago(6);
    to = ymd(new Date());
    void refreshOutlets();
    void load();
    // Daftar kasir hanya PELATIH: laporan tetap bisa dibuka tanpa izin users.
    iam
      .users()
      .then((users) => (cashiers = users.filter((u) => u.active).map((u) => ({ id: u.id, name: u.name }))))
      .catch(() => undefined);
  });

  function payText(s: DetailSale): string {
    return (s.methods ?? []).map((m) => `${m.name} ${money(m.amount)}`).join(', ');
  }
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${D}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${D}.subtitle`)}</p>
  </div>

  <section class="surface-card !p-4 space-y-3">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t('reports.range')}</span>
        <DateRange bind:from bind:to onchange={() => load()} ariaLabel={t('reports.range')} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${D}.outlet`)}</span>
        <Combobox bind:value={outletId} bind:label={outletLabel} search={searchOutlets} placeholder={t(`${D}.outlet`)} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${D}.cashier`)}</span>
        <Combobox bind:value={cashierId} bind:label={cashierLabel} search={searchCashiers} placeholder={t(`${D}.cashier`)} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${D}.category`)}</span>
        <Combobox bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t(`${D}.category`)} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${D}.status`)}</span>
        <Select bind:value={status} options={statusOptions} ariaLabel={t(`${D}.status`)} />
      </div>
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
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50"
          disabled={exporting !== null}
          onclick={() => download('xlsx')}
        >{exporting === 'xlsx' ? '…' : 'Excel'}</button>
        <button
          type="button"
          class="rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50"
          disabled={exporting !== null}
          onclick={() => download('pdf')}
        >{exporting === 'pdf' ? '…' : 'PDF'}</button>
      </span>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if result?.truncated}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-warning">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t(`${D}.truncated`)}</span>
    </div>
  {/if}

  {#if summary && summary.sale_count + summary.void_count > 0}
    <section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <div class="surface-card !p-4">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t(`${D}.saleCount`)}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{formatNumber(summary.sale_count)}</div>
        {#if summary.void_count > 0}
          <div class="mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t(`${D}.voidCount`)}: {formatNumber(summary.void_count)}</div>
        {/if}
      </div>
      <div class="surface-card !p-4">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('reports.total')}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.total)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('reports.paid')}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.paid)}</div>
      </div>
      <div class="surface-card !p-4">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t('reports.receivable')}</div>
        <div class="mt-0.5 font-display text-[20px] font-bold tabular-nums">{money(summary.receivable)}</div>
      </div>
    </section>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-auto scroll-thin max-h-[70vh]">
      <table class="w-full min-w-[1150px] text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-3 py-3 text-end font-semibold w-[44px]" scope="col">{t(`${D}.lineNo`)}</th>
            <th class="px-3 py-3 text-start font-semibold w-[64px]" scope="col">{t(`${D}.time`)}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.note`)}</th>
            <th class="px-3 py-3 text-start font-semibold w-[140px]" scope="col">{t(`${D}.cashier`)}</th>
            <th class="px-3 py-3 text-start font-semibold w-[110px]" scope="col">{t(`${D}.sku`)}</th>
            <th class="px-3 py-3 text-start font-semibold" scope="col">{t(`${D}.itemName`)}</th>
            <th class="px-3 py-3 text-end font-semibold w-[80px]" scope="col">{t(`${D}.qty`)}</th>
            <th class="px-3 py-3 text-start font-semibold w-[80px]" scope="col">{t(`${D}.unit`)}</th>
            <th class="px-3 py-3 text-end font-semibold w-[130px]" scope="col">{t(`${D}.price`)}</th>
            <th class="px-3 py-3 text-end font-semibold w-[120px]" scope="col">{t(`${D}.lineDiscount`)}</th>
            <th class="px-3 py-3 text-end font-semibold w-[140px]" scope="col">{t(`${D}.lineTotal`)}</th>
          </tr>
        </thead>
        <tbody>
          {#each days as day (day.date)}
            <!-- Baris tanggal: tanggal + jumlah nota; ringkasan nilai lewat tombol pop up. -->
            <tr class="bg-[var(--surface-sunken)] border-y-2 border-[var(--border-subtle)]">
              <td class="px-3 py-2" colspan="11">
                <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 font-semibold">
                  <span>{formatDate(day.date)}</span>
                  <span class="text-[11.5px] font-normal text-[var(--text-tertiary)]">
                    {t('reports.saleCount')}: {formatNumber(day.sale_count)}{#if day.void_count > 0} · {t('reports.voidCount')}: {formatNumber(day.void_count)}{/if}
                  </span>
                  <button
                    type="button"
                    class="ms-auto inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-[11.5px] font-normal transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)]"
                    aria-haspopup="dialog"
                    onclick={() => (dayModal = day)}
                  >
                    <i class="icon-chart-pie text-[13px]"></i>{t(`${D}.daySummary`)}
                  </button>
                </div>
              </td>
            </tr>
            {#each day.sales as s (s.id)}
              <!-- Baris judul nota: jam, nomor, kasir. Angka menyusul di bawah barang. -->
              <tr class="bg-[var(--surface-sunken)]/40 border-t border-[var(--border-subtle)]">
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2 tabular-nums text-[var(--text-tertiary)]">{s.time}</td>
                <td class="px-3 py-2 font-semibold whitespace-nowrap">
                  {s.doc_no}
                  {#if s.status === 'void'}
                    <span class="ms-1 rounded px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide badge-danger">{t(`${D}.statusVoid`)}</span>
                  {/if}
                </td>
                <td class="px-3 py-2 text-[11.5px] truncate">
                  {s.cashier_name}{#if s.member_name}<span class="text-[var(--text-tertiary)]"> / {s.member_name}</span>{/if}
                </td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
                <td class="px-3 py-2"></td>
              </tr>
              {#each s.lines as l, i (i)}
                <!-- Baris barang. -->
                <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)]">
                  <td class="px-3 py-1.5 text-end tabular-nums text-[var(--text-tertiary)]">{i + 1}</td>
                  <td class="px-3 py-1.5"></td>
                  <td class="px-3 py-1.5"></td>
                  <td class="px-3 py-1.5"></td>
                  <td class="px-3 py-1.5 font-mono text-[11.5px]">{l.sku}</td>
                  <td class="px-3 py-1.5">
                    {l.name}
                    {#if l.category_name}<span class="ms-1 text-[11px] text-[var(--text-tertiary)]">({l.category_name})</span>{/if}
                  </td>
                  <td class="px-3 py-1.5 text-end tabular-nums">{qty(l.qty)}</td>
                  <td class="px-3 py-1.5 text-[11.5px] text-[var(--text-tertiary)]">{l.unit_name}</td>
                  <td class="px-3 py-1.5 text-end tabular-nums">{money(l.unit_price)}</td>
                  <td class="px-3 py-1.5 text-end tabular-nums">{money(l.discount)}</td>
                  <td class="px-3 py-1.5 text-end tabular-nums font-semibold">{money(l.line_total)}</td>
                </tr>
              {/each}
              <!-- Footer nota: metode pembayaran + tombol accordion rincian nilai. -->
              <tr class="border-t-2 border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
                <td class="px-3 py-1.5" colspan="11">
                  <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
                    {#if payText(s)}
                      <div class="text-[11.5px]">
                        <span class="text-[var(--text-tertiary)]">{t(`${D}.payment`)}:</span>
                        <span class="font-medium text-[var(--text-secondary)]">{payText(s)}</span>
                      </div>
                    {/if}
                    <button
                      type="button"
                      class="ms-auto inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-[11.5px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)]"
                      aria-expanded={expanded.has(s.id)}
                      onclick={() => toggleSale(s.id)}
                    >
                      {t(`${D}.breakdown`)}
                      <i class="{expanded.has(s.id) ? 'icon-chevron-up' : 'icon-chevron-down'} text-[13px]"></i>
                    </button>
                  </div>
                </td>
              </tr>
              {#if expanded.has(s.id)}
                <!-- Isi accordion: rincian nilai tersusun ke bawah. -->
                <tr class="border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
                  <td class="px-3 py-2" colspan="11">
                    <div class="ms-auto w-full max-w-[320px] text-[11.5px]">
                      <div class="flex items-baseline justify-between gap-6 py-0.5">
                        <span class="text-[var(--text-tertiary)]">{t('reports.subtotal')}</span>
                        <span class="tabular-nums">{money(s.subtotal)}</span>
                      </div>
                      <div class="flex items-baseline justify-between gap-6 py-0.5">
                        <span class="text-[var(--text-tertiary)]">{t('reports.discount')}</span>
                        <span class="tabular-nums">{money(s.discount)}</span>
                      </div>
                      <div class="flex items-baseline justify-between gap-6 py-0.5">
                        <span class="text-[var(--text-tertiary)]">{t('reports.taxStore')}</span>
                        <span class="tabular-nums">{money(s.tax_store)}</span>
                      </div>
                      <div class="flex items-baseline justify-between gap-6 py-0.5">
                        <span class="text-[var(--text-tertiary)]">{t('reports.taxGov')}</span>
                        <span class="tabular-nums">{money(s.tax_gov)}</span>
                      </div>
                      <div class="mt-0.5 flex items-baseline justify-between gap-6 border-t border-[var(--border-subtle)] pt-1 text-[13px] font-semibold text-[var(--text-primary)]">
                        <span>{t('reports.total')}</span>
                        <span class="tabular-nums">{money(s.total)}</span>
                      </div>
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
          {:else}
            <tr>
              <td colspan="11" class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">
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

  {#if dayModal}
    <Modal title={t(`${D}.daySummary`)} onclose={() => (dayModal = null)}>
      <div>
        <div class="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11.5px] text-[var(--text-tertiary)]">
          <span class="font-semibold text-[var(--text-primary)]">{formatDate(dayModal.date)}</span>
          <span>{t('reports.saleCount')}: {formatNumber(dayModal.sale_count)}</span>
          {#if dayModal.void_count > 0}
            <span>{t('reports.voidCount')}: {formatNumber(dayModal.void_count)}</span>
          {/if}
        </div>
        <div class="text-[12.5px]">
          <div class="flex items-baseline justify-between gap-6 py-1">
            <span class="text-[var(--text-tertiary)]">{t('reports.subtotal')}</span>
            <span class="tabular-nums">{money(dayModal.subtotal)}</span>
          </div>
          <div class="flex items-baseline justify-between gap-6 py-1">
            <span class="text-[var(--text-tertiary)]">{t('reports.discount')}</span>
            <span class="tabular-nums">{money(dayModal.discount)}</span>
          </div>
          <div class="flex items-baseline justify-between gap-6 py-1">
            <span class="text-[var(--text-tertiary)]">{t('reports.taxStore')}</span>
            <span class="tabular-nums">{money(dayModal.tax_store)}</span>
          </div>
          <div class="flex items-baseline justify-between gap-6 py-1">
            <span class="text-[var(--text-tertiary)]">{t('reports.taxGov')}</span>
            <span class="tabular-nums">{money(dayModal.tax_gov)}</span>
          </div>
          <div class="mt-1 flex items-baseline justify-between gap-6 border-t border-[var(--border-subtle)] pt-2 text-[14px] font-semibold text-[var(--text-primary)]">
            <span>{t('reports.total')}</span>
            <span class="tabular-nums">{money(dayModal.total)}</span>
          </div>
        </div>
      </div>
    </Modal>
  {/if}
</main>