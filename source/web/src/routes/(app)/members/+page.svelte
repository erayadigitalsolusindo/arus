<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { members as api, type Row } from '#lib/members/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import MemberCover from '#lib/components/MemberCover.svelte';

  const PAGE = 20;

  let rows = $state<Row[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let loading = $state(true);
  let loadError = $state('');
  let notice = $state('');
  let q = $state('');
  let filter = $state<'all' | 'active' | 'inactive'>('all');
  let busyId = $state<string | null>(null);

  // Hari ini menurut zona waktu browser (hanya untuk lencana "kedaluwarsa" di daftar; server yang menegakkan).
  const today = new Date().toLocaleDateString('en-CA');
  const isExpired = (r: Row) => !!r.valid_until && r.valid_until < today;

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function load() {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await api.list({ q: q.trim(), active: filter === 'all' ? undefined : filter === 'active', limit: PAGE, offset });
      if (mine !== seq) return;
      rows = res.data;
      total = res.total;
    } catch (err) {
      if (mine === seq) loadError = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  $effect(() => {
    void q;
    void filter;
    const h = setTimeout(() => {
      offset = 0;
      void load();
    }, 250);
    return () => clearTimeout(h);
  });

  onMount(() => {
    const n = page.url.searchParams.get('notice');
    if (n === 'saved' || n === 'created') {
      notice = t(n === 'saved' ? 'members.saved' : 'members.created');
      void goto('/members', { replaceState: true });
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
      notice = t(r.active ? 'members.archived' : 'members.restored', { name: r.name });
      await load();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      busyId = null;
    }
  }

  const inputClass = 'w-full field-control';
</script>

<svelte:head><title>{t('members.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('members.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('members.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-full mx-auto w-full">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[19px]">{t('members.title')}</h1>
      <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('members.subtitle')}</p>
    </div>
    {#if can('members', 'create')}
      <a href="/members/new" class="btn btn-primary !text-[12.5px]"><i class="icon-plus text-[13px]"></i>{t('members.add')}</a>
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
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('members.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <div class="surface-card !p-0 overflow-hidden">
    <div class="flex flex-wrap items-center gap-2 p-3 border-b border-[var(--border-subtle)]">
      <div class="relative grow sm:grow-0 sm:w-80">
        <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
        <input type="search" class="{inputClass} !ps-8" placeholder={t('members.search')} aria-label={t('members.search')} bind:value={q} maxlength="200" />
      </div>
      <select class="field-control" aria-label={t('members.col.status')} bind:value={filter}>
        <option value="all">{t('members.filter.all')}</option>
        <option value="active">{t('members.filter.active')}</option>
        <option value="inactive">{t('members.filter.inactive')}</option>
      </select>
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[860px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)]">
            <th class="p-3 text-start" scope="col">{t('members.col.code')}</th>
            <th class="p-3 text-start" scope="col">{t('members.col.name')}</th>
            <th class="p-3 text-start" scope="col">{t('members.col.phone')}</th>
            <th class="p-3 text-start" scope="col">{t('members.col.city')}</th>
            <th class="p-3 text-start" scope="col">{t('members.col.level')}</th>
            <th class="p-3 text-end" scope="col">{t('members.col.points')}</th>
            <th class="p-3 text-start" scope="col">{t('members.col.status')}</th>
            <th class="p-3 text-end" scope="col"></th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr class="border-t border-[var(--border-subtle)] odd:bg-[var(--surface-sunken)] hover:bg-[var(--surface-hover,var(--surface-sunken))]">
              <td class="p-3 font-mono text-[12px]">{r.code}</td>
              <td class="p-3">
                <a href="/members/{r.id}" class="flex items-center gap-2.5 font-semibold hover:underline">
                  <MemberCover memberId={r.id} coverId={r.cover_image_id} name={r.name} class="size-9 shrink-0 rounded-full object-cover text-[12px]" />
                  <span class="min-w-0 truncate">{r.name}</span>
                </a>
              </td>
              <td class="p-3 whitespace-nowrap">{r.phone}</td>
              <td class="p-3">{r.city}</td>
              <td class="p-3">{#if r.level}<span class="badge-soft badge-info">{r.level}</span>{/if}</td>
              <td class="p-3 text-end font-semibold tabular-nums">{formatNumber(r.points)}</td>
              <td class="p-3 whitespace-nowrap">
                <span class="badge-soft {r.active && !isExpired(r) ? 'badge-success' : 'badge-danger'}">
                  {!r.active ? t('members.inactive') : isExpired(r) ? t('members.expired') : t('members.active')}
                </span>
              </td>
              <td class="p-3 text-end whitespace-nowrap">
                <a href="/members/{r.id}" class="header-icon-btn !size-8" aria-label={can('members', 'update') ? t('members.edit') : t('members.view')}>
                  <i class="{can('members', 'update') ? 'icon-pencil' : 'icon-eye'} text-[13px]"></i>
                </a>
                {#if can('members', 'update')}
                  <button
                    type="button"
                    class="header-icon-btn !size-8"
                    aria-label={r.active ? t('members.archive') : t('members.restore')}
                    title={r.active ? t('members.archive') : t('members.restore')}
                    disabled={busyId === r.id}
                    onclick={() => toggle(r)}
                  >
                    <i class="{r.active ? 'icon-archive' : 'icon-rotate-ccw'} text-[13px]"></i>
                  </button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="8" class="p-6 text-center text-[var(--text-tertiary)]">{loading ? '…' : q.trim() || filter !== 'all' ? t('members.emptySearch') : t('members.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if total > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 border-t border-[var(--border-subtle)] text-[12px] text-[var(--text-tertiary)]">
        <span>{t('members.range', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
        <div class="flex gap-1.5">
          <button type="button" class="btn !text-[12px]" disabled={offset === 0 || loading} onclick={() => go(offset - PAGE)}>{t('members.prev')}</button>
          <button type="button" class="btn !text-[12px]" disabled={offset + PAGE >= total || loading} onclick={() => go(offset + PAGE)}>{t('members.next')}</button>
        </div>
      </div>
    {/if}
  </div>
</main>
