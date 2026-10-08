<script lang="ts">
  // Pemilih member di kasir: cari nama/kode/telepon (hanya member aktif yang belum kedaluwarsa), klik untuk memilih.
  import { members, type Lookup } from '#lib/members/api.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  let { onpick, onclose, current = '', onclear }: { onpick: (m: Lookup) => void; onclose: () => void; current?: string; onclear?: () => void } = $props();

  let q = $state('');
  let results = $state<Lookup[]>([]);
  let loading = $state(true);
  let error = $state('');
  let active = $state(0);
  let seq = 0;

  $effect(() => {
    const term = q.trim();
    const mine = ++seq;
    loading = true;
    const ctrl = new AbortController();
    const h = setTimeout(async () => {
      try {
        const r = await members.lookup(term, ctrl.signal);
        if (mine !== seq) return;
        results = r;
        active = 0;
        error = '';
      } catch (e) {
        if (mine === seq && !ctrl.signal.aborted) error = errorMessage(e);
      } finally {
        if (mine === seq) loading = false;
      }
    }, 200);
    return () => {
      clearTimeout(h);
      ctrl.abort();
    };
  });

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      active = Math.min(active + 1, results.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      active = Math.max(active - 1, 0);
    } else if (e.key === 'Enter' && results[active]) {
      e.preventDefault();
      onpick(results[active]);
    }
  }
</script>

<Modal title={t('members.pos.choose')} {onclose}>
  <div class="space-y-3">
    <div class="relative">
      <i class="icon-search text-[13px] absolute start-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]"></i>
      <input
        type="search"
        class="w-full field-control !ps-8"
        placeholder={t('members.pos.search')}
        aria-label={t('members.pos.search')}
        bind:value={q}
        maxlength="200"
        autocomplete="off"
        {onkeydown}
        use:focusOnMount
      />
    </div>
    {#if onclear}
      <button type="button" class="flex w-full items-center justify-between gap-2 rounded-lg border border-[var(--border-subtle)] px-3 py-2 text-[12.5px] hover:bg-[var(--surface-sunken)]" onclick={onclear}>
        <span class="truncate text-[var(--text-secondary)]">{current}</span>
        <span class="inline-flex shrink-0 items-center gap-1.5 font-semibold text-[var(--color-danger-600)]"><i class="icon-x text-[13px]"></i>{t('members.pos.remove')}</span>
      </button>
    {/if}
    {#if error}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{error}</p>{/if}
    <ul class="max-h-72 overflow-y-auto scroll-thin divide-y divide-[var(--border-subtle)] rounded-lg border border-[var(--border-subtle)]">
      {#each results as m, i (m.id)}
        <li>
          <button
            type="button"
            class="flex w-full items-center gap-3 px-3 py-2.5 text-start hover:bg-[var(--surface-sunken)] {i === active ? 'bg-[var(--surface-sunken)]' : ''}"
            onclick={() => onpick(m)}
            onmouseenter={() => (active = i)}
          >
            <span class="grid size-9 shrink-0 place-items-center rounded-full bg-[color-mix(in_oklab,var(--color-primary-600)_14%,transparent)] text-[var(--color-primary-600)]"><i class="icon-user-round text-[15px]"></i></span>
            <span class="min-w-0 grow leading-tight">
              <span class="block truncate text-[13px] font-semibold">{m.name}</span>
              <span class="block truncate text-[11.5px] text-[var(--text-tertiary)]"><span class="font-mono">{m.code}</span>{m.phone ? ` · ${m.phone}` : ''}</span>
            </span>
            <span class="shrink-0 text-end leading-tight">
              <span class="block text-[12px] font-semibold tabular-nums">{t('members.pos.points', { points: formatNumber(m.points) })}</span>
              {#if m.level}<span class="badge-soft badge-info">{m.level}</span>{/if}
            </span>
          </button>
        </li>
      {:else}
        <li class="p-5 text-center text-[12.5px] text-[var(--text-tertiary)]">{loading ? '…' : t('members.pos.searchEmpty')}</li>
      {/each}
    </ul>
  </div>
</Modal>
