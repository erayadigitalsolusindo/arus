<script lang="ts">
  // Pemilih bulan bergaya DatePicker (bits-ui Popover), pengganti <input type="month"> bawaan browser.
  // Nilai kanonik = string "YYYY-MM" (kosong = belum dipilih), sama dengan nilai input month.
  import { Popover } from 'bits-ui';
  import { i18n, t } from '#lib/i18n/index.ts';

  let {
    value = $bindable(''),
    id,
    disabled = false,
    placeholder = ''
  }: { value?: string; id?: string; disabled?: boolean; placeholder?: string } = $props();

  let open = $state(false);
  let year = $state(new Date().getFullYear());

  const parsed = $derived.by(() => {
    const m = /^(\d{4})-(\d{2})$/.exec(value);
    return m ? { y: Number(m[1]), m: Number(m[2]) } : null;
  });
  const names = $derived(Array.from({ length: 12 }, (_, i) => new Intl.DateTimeFormat(i18n.intl, { month: 'short' }).format(new Date(2024, i, 1))));
  const label = $derived(parsed ? new Intl.DateTimeFormat(i18n.intl, { month: 'long', year: 'numeric' }).format(new Date(parsed.y, parsed.m - 1, 1)) : '');

  $effect(() => {
    if (open) year = parsed?.y ?? new Date().getFullYear();
  });

  function pick(y: number, m: number) {
    value = `${y}-${String(m).padStart(2, '0')}`;
    open = false;
  }
  const now = () => new Date();
</script>

<Popover.Root bind:open>
  <Popover.Trigger {id} {disabled} type="button" class="field-control relative w-full flex items-center text-start disabled:opacity-60" style="padding-inline-end:2.25rem">
    <span class="min-w-0 truncate capitalize {label ? '' : 'text-[var(--text-tertiary)]'}">{label || placeholder || t('common.monthPicker.choose')}</span>
    <i class="icon-calendar-days absolute top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)]" style="inset-inline-end:0.75rem"></i>
  </Popover.Trigger>

  <Popover.Portal>
    <Popover.Content sideOffset={6} align="start" collisionPadding={8} class="z-[100] w-[17.5rem] rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-3 shadow-[var(--shadow-xl)]">
      <div class="mb-2 flex items-center justify-between">
        <button type="button" class="grid size-8 place-items-center rounded-lg hover:bg-[var(--surface-sunken)]" aria-label={t('common.monthPicker.prev')} onclick={() => year--}>
          <i class="icon-chevron-left text-[15px]"></i>
        </button>
        <span class="font-display text-[13.5px] font-bold tabular-nums">{year}</span>
        <button type="button" class="grid size-8 place-items-center rounded-lg hover:bg-[var(--surface-sunken)]" aria-label={t('common.monthPicker.next')} onclick={() => year++}>
          <i class="icon-chevron-right text-[15px]"></i>
        </button>
      </div>
      <div class="grid grid-cols-3 gap-1.5">
        {#each names as n, i (i)}
          {@const on = parsed?.y === year && parsed?.m === i + 1}
          {@const cur = now().getFullYear() === year && now().getMonth() === i}
          <button
            type="button"
            class="mp-month h-9 rounded-lg text-[12.5px] capitalize transition-colors {on
              ? 'mp-on bg-[var(--color-primary-600)] font-bold'
              : 'hover:bg-[var(--surface-sunken)]'} {cur && !on ? 'font-bold text-[var(--color-primary-600)] ring-1 ring-[var(--color-primary-600)]' : ''}"
            aria-pressed={on}
            onclick={() => pick(year, i + 1)}
          >{n}</button>
        {/each}
      </div>
      <div class="mt-2 border-t border-[var(--border-subtle)] pt-2">
        <button type="button" class="text-[12px] font-semibold text-[var(--color-primary-600)]" onclick={() => pick(now().getFullYear(), now().getMonth() + 1)}>{t('common.monthPicker.thisMonth')}</button>
      </div>
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>

<style>
  .mp-on {
    color: #fff !important;
  }
</style>
