<script lang="ts">
  import { onMount } from 'svelte';
  import { masterReport, masterReportFileUrl, type MasterKind } from '#lib/reports/stock/api.ts';
  import { t, formatCurrency, formatNumber, formatDate } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { apiBlob } from '#lib/api/client.ts';
  import Select from '#lib/components/Select.svelte';
  import type { MessageKey } from '#lib/i18n/types.ts';

  // Satu komponen untuk tiga laporan master; rute hanya pembungkus (pola sama dengan PosScreen).
  let { kind }: { kind: MasterKind } = $props();

  type Row = Record<string, any>;
  type Col = { key: string; label: string; cls?: string; render?: (r: Row) => string };

  const M = 'reports.masterReport';
  const sect = $derived(kind === 'members' ? 'member' : kind === 'suppliers' ? 'supplier' : 'salesperson');

  const columns = $derived.by<Col[]>(() => {
    const c = (k: string) => t(`${M}.${k}` as MessageKey);
    const status = (r: Row) => (r.active ? c('active') : c('inactive'));
    const code = 'font-mono text-[11.5px] whitespace-nowrap';
    if (kind === 'members') {
      return [
        { key: 'code', label: c('code'), cls: code },
        { key: 'name', label: c('name') },
        { key: 'gender', label: c('gender'), render: (r) => (r.gender === 'M' ? c('male') : r.gender === 'F' ? c('female') : '') },
        { key: 'phone', label: c('phone'), cls: 'whitespace-nowrap' },
        { key: 'city', label: c('city') },
        { key: 'level', label: c('level') },
        { key: 'points', label: c('points'), cls: 'text-end tabular-nums', render: (r) => formatNumber(r.points) },
        { key: 'credit_limit', label: c('creditLimit'), cls: 'text-end tabular-nums', render: (r) => (Number(r.credit_limit) ? formatCurrency(Number(r.credit_limit)) : '-') },
        { key: 'due_days', label: c('dueDays'), cls: 'text-end tabular-nums', render: (r) => (r.due_days ? String(r.due_days) : '-') },
        { key: 'valid_until', label: c('validUntil'), cls: 'whitespace-nowrap', render: (r) => (r.valid_until ? formatDate(r.valid_until) : '-') },
        { key: 'active', label: c('statusCol'), render: status }
      ];
    }
    if (kind === 'suppliers') {
      return [
        { key: 'code', label: c('code'), cls: code },
        { key: 'name', label: c('name') },
        { key: 'contact_name', label: c('contact') },
        { key: 'phone', label: c('phone'), cls: 'whitespace-nowrap' },
        { key: 'email', label: c('email') },
        { key: 'address', label: c('address') },
        { key: 'note', label: c('note') },
        { key: 'active', label: c('statusCol'), render: status }
      ];
    }
    return [
      { key: 'code', label: c('code'), cls: code },
      { key: 'name', label: c('name') },
      { key: 'phone', label: c('phone'), cls: 'whitespace-nowrap' },
      { key: 'commission_pct', label: c('commission'), cls: 'text-end tabular-nums', render: (r) => formatNumber(Number(r.commission_pct)) },
      { key: 'note', label: c('note') },
      { key: 'active', label: c('statusCol'), render: status }
    ];
  });

  const statusOptions = $derived([
    { value: '', label: t(`${M}.all`) },
    { value: 'active', label: t(`${M}.active`) },
    { value: 'inactive', label: t(`${M}.inactive`) }
  ]);

  let q = $state('');
  let status = $state('');
  let loading = $state(false);
  let loadingMore = $state(false);
  let error = $state('');
  let rows = $state<Row[]>([]);
  let nextCursor = $state<string | undefined>(undefined);
  let seq = 0; // abaikan respons usang saat filter cepat berganti

  async function load(more = false) {
    if (more && (!nextCursor || loadingMore)) return;
    const my = ++seq;
    error = '';
    if (more) loadingMore = true;
    else {
      loadingMore = false; // reload membatalkan load-more yang sedang berjalan
      loading = true;
      rows = [];
      nextCursor = undefined;
    }
    try {
      const res = await masterReport<Row>(kind, { q: q.trim() || undefined, status: status || undefined, cursor: more ? nextCursor : undefined });
      if (my !== seq) return;
      rows = more ? [...rows, ...res.data] : res.data;
      nextCursor = res.next_cursor;
    } catch (err) {
      if (my === seq) error = errorMessage(err);
    } finally {
      if (my === seq) {
        loading = false;
        loadingMore = false;
      }
    }
  }

  let exporting = $state<'xlsx' | 'pdf' | null>(null);

  async function download(format: 'xlsx' | 'pdf') {
    if (exporting) return;
    exporting = format;
    error = '';
    try {
      const blob = await apiBlob(masterReportFileUrl(kind, { q: q.trim() || undefined, status: status || undefined }, format));
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `master-${sect}_${new Date().toISOString().slice(0, 10)}.${format}`;
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

  const chip =
    'rounded-full border px-3 py-1 text-[12px] font-medium transition-colors border-[var(--border-subtle)] text-[var(--text-secondary)] hover:border-[var(--color-primary-400)] hover:text-[var(--color-primary-600)] disabled:opacity-50';

  let timer: ReturnType<typeof setTimeout> | undefined;
  function onSearch() {
    clearTimeout(timer);
    timer = setTimeout(() => load(), 300);
  }

  onMount(() => {
    document.title = t(`${M}.${sect}.docTitle`);
    void load();
    return () => clearTimeout(timer);
  });
</script>

<main class="p-4 lg:p-6 space-y-4 max-w-[1900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t(`${M}.${sect}.title`)}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t(`${M}.${sect}.subtitle`)}</p>
  </div>

  <section class="surface-card !p-4">
    <div class="grid gap-3 md:grid-cols-4">
      <div class="md:col-span-2">
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${M}.search`)}</span>
        <input type="search" class="w-full field-control" maxlength="64" placeholder={t(`${M}.search`)} aria-label={t(`${M}.search`)} bind:value={q} oninput={onSearch} />
      </div>
      <div>
        <span class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]">{t(`${M}.status`)}</span>
        <Select bind:value={status} options={statusOptions} ariaLabel={t(`${M}.status`)} onchange={() => load()} />
      </div>
      <div class="flex items-end justify-end gap-1.5">
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('xlsx')}>{exporting === 'xlsx' ? '…' : 'Excel'}</button>
        <button type="button" class={chip} disabled={exporting !== null} onclick={() => download('pdf')}>{exporting === 'pdf' ? '…' : 'PDF'}</button>
      </div>
    </div>
  </section>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  <section class="surface-card !p-0 overflow-hidden transition-opacity {loading ? 'opacity-60' : ''}" aria-busy={loading}>
    <div class="overflow-x-auto scroll-thin max-h-[calc(100vh-300px)]">
      <table class="w-full text-[12.5px] border-collapse">
        <thead class="sticky top-0 z-10 bg-[var(--surface-sunken)]">
          <tr class="text-[11px] uppercase tracking-wide text-[var(--text-tertiary)]">
            {#each columns as c (c.key)}
              <th class="px-3 py-3 font-semibold {c.cls?.includes('text-end') ? 'text-end' : 'text-start'}" scope="col">{c.label}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] transition-colors hover:bg-[var(--surface-sunken)] {r.active ? '' : 'opacity-60'}">
              {#each columns as c (c.key)}
                <td class="px-3 py-2.5 {c.cls ?? ''}">{c.render ? c.render(r) : (r[c.key] ?? '')}</td>
              {/each}
            </tr>
          {:else}
            <tr>
              <td colspan={columns.length} class="px-4 py-14 text-center text-[13px] text-[var(--text-tertiary)]">{loading ? '…' : t(`${M}.noData`)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if nextCursor}
      <div class="p-3 text-center border-t border-[var(--border-subtle)]">
        <button type="button" class="btn-secondary text-[12.5px] px-4 py-2" onclick={() => load(true)} disabled={loadingMore}>
          {loadingMore ? '…' : t(`${M}.loadMore`)}
        </button>
      </div>
    {/if}
  </section>
</main>
