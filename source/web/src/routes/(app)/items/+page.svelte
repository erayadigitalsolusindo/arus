<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { items as api, type Row } from '#lib/items/api.ts';
  import { lookup } from '#lib/catalog/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Combobox from '#lib/components/Combobox.svelte';
  import AuthImage from '#lib/components/AuthImage.svelte';

  const PAGE = 20;

  let rows = $state<Row[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let filter = $state<'all' | 'active' | 'inactive'>('all');
  let categoryId = $state('');
  let categoryLabel = $state('');
  let busyId = $state<string | null>(null);

  const searchCategories = (s: string) => lookup('categories').search(s);

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load() {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await api.list({
        q: q.trim(),
        active: filter === 'all' ? undefined : filter === 'active',
        category_id: categoryId || undefined,
        limit: PAGE,
        offset
      });
      if (mine !== seq) return;
      rows = res.data;
      total = res.total;
    } catch (err) {
      if (mine === seq) loadError = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Cari/ubah filter → kembali ke halaman 1 setelah jeda singkat (debounce).
  $effect(() => {
    void q;
    void filter;
    void categoryId;
    const h = setTimeout(() => {
      offset = 0;
      void load();
    }, 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    // Pemberitahuan dari halaman form (?notice=saved|created), lalu bersihkan URL.
    const n = page.url.searchParams.get('notice');
    if (n === 'saved' || n === 'created') {
      notice = t(n === 'saved' ? 'items.saved' : 'items.created');
      void goto('/items', { replaceState: true });
    }
  });

  function go(next: number) {
    offset = Math.max(0, next);
    void load();
  }

  async function toggle(r: Row) {
    busyId = r.id;
    notice = loadError = '';
    try {
      await api.setActive(r.id, !r.active);
      notice = t(r.active ? 'items.archived' : 'items.restored', { name: r.name });
      await load();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      busyId = null;
    }
  }

  const inputClass = 'w-full field-control';
</script>

<svelte:head><title>{t('items.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('items.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('items.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('items.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('items.subtitle')}</p>
    </div>
    {#if can('items', 'create')}
      <a href="/items/new" class="btn btn-primary !text-[12.5px]"><i class="icon-plus text-[13px]"></i>{t('items.add')}</a>
    {/if}
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('items.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('items.search')} aria-label={t('items.search')} bind:value={q} maxlength="200" />
      </div>
      <select class="field-control" aria-label={t('items.col.status')} bind:value={filter}>
        <option value="all">{t('items.filter.all')}</option>
        <option value="active">{t('items.filter.active')}</option>
        <option value="inactive">{t('items.filter.inactive')}</option>
      </select>
      <div class="w-56">
        <Combobox bind:value={categoryId} bind:label={categoryLabel} search={searchCategories} placeholder={t('items.filter.allCategories')} />
      </div>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[860px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="p-3 text-start" scope="col">{t('items.col.code')}</th>
            <th class="p-3 text-start" scope="col">{t('items.col.name')}</th>
            <th class="p-3 text-start" scope="col">{t('items.col.unit')}</th>
            <th class="p-3 text-start" scope="col">{t('items.col.category')}</th>
            <th class="p-3 text-start" scope="col">{t('items.col.brand')}</th>
            <th class="p-3 text-end" scope="col">{t('items.col.price')}</th>
            <th class="p-3 text-start" scope="col">{t('items.col.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]">
              <td class="p-3 font-mono text-[12px]">{r.sku}</td>
              <td class="p-3">
                <div class="flex items-center gap-2.5">
                  {#if r.main_image_id}
                    <AuthImage itemId={r.id} imageId={r.main_image_id} alt={r.name} class="size-10 shrink-0 rounded-md object-cover" />
                  {:else}
                    <span class="inline-flex size-10 shrink-0 items-center justify-center rounded-md bg-[var(--surface-sunken)] text-[var(--text-tertiary)]" aria-hidden="true"><i class="icon-image text-[15px]"></i></span>
                  {/if}
                  <div class="min-w-0">
                    <a href="/items/{r.id}" class="font-semibold hover:underline">{r.name}</a>{#if r.origin}<span class="badge-soft badge-info ms-1.5 align-middle">{r.origin}</span>{/if}
                    {#if r.barcode}<div class="font-mono text-[11px] text-[var(--text-tertiary)]">{r.barcode}</div>{/if}
                  </div>
                </div>
              </td>
              <td class="p-3">{r.unit}</td>
              <td class="p-3">{r.category}</td>
              <td class="p-3">{r.brand}</td>
              <td class="p-3 text-end whitespace-nowrap">
                {formatCurrency(Number(r.price))}
                {#if r.price_override}<i class="icon-store text-[11px] ms-1 text-[var(--color-primary-600)]" title={t('items.priceOverride')} aria-label={t('items.priceOverride')}></i>{/if}
              </td>
              <td class="p-3"><span class="badge-soft {r.active ? 'badge-success' : 'badge-danger'}">{r.active ? t('items.active') : t('items.inactive')}</span></td>
              <td class="p-3 text-end whitespace-nowrap">
                <a href="/items/{r.id}" class="header-icon-btn !size-8" aria-label={can('items', 'update') ? t('items.edit') : t('items.view')}>
                  <i class="{can('items', 'update') ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </a>
                {#if can('items', 'update')}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={r.active ? t('items.archive') : t('items.restore')}
                    title={r.active ? t('items.archive') : t('items.restore')}
                    disabled={busyId === r.id}
                    onclick={() => toggle(r)}
                  >
                    <i class="{r.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="8" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' || categoryId ? t('items.emptySearch') : t('items.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if total > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
        <span>{t('items.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('items.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('items.next')}</button>
        </div>
      </div>
    {/if}
  </div>
</main>
