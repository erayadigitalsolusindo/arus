<script lang="ts">
  // Pemilih tanggal bergaya template (bits-ui Popover + Calendar), pengganti <input type="date"> bawaan browser.
  // Nilai kanonik = string "YYYY-MM-DD" (kosong = belum dipilih), sama dengan nilai input date, jadi API tidak berubah.
  import { Popover, Calendar } from 'bits-ui';
  import { CalendarDate, parseDate, type DateValue } from '@internationalized/date';
  import { i18n, t, formatDate } from '#lib/i18n/index.ts';

  let {
    value = $bindable(''),
    id,
    min,
    max,
    disabled = false,
    invalid = false,
    clearable = true,
    placeholder = ''
  }: { value?: string; id?: string; min?: string; max?: string; disabled?: boolean; invalid?: boolean; clearable?: boolean; placeholder?: string } = $props();

  let open = $state(false);

  const toDate = (s?: string) => {
    try {
      return s ? parseDate(s) : undefined;
    } catch {
      return undefined;
    }
  };
  const picked = $derived(toDate(value));
  const today = $derived.by(() => {
    const n = new Date();
    return new CalendarDate(n.getFullYear(), n.getMonth() + 1, n.getDate());
  });

  function onSelect(v: DateValue | undefined) {
    value = v ? v.toString() : '';
    open = false;
  }

  // Nama hari singkat menurut bahasa aktif, mulai Senin (minggu dibuka dari Senin).
  const dayNames = $derived(Array.from({ length: 7 }, (_, i) => new Intl.DateTimeFormat(i18n.intl, { weekday: 'short' }).format(new Date(2024, 0, 1 + i)).slice(0, 3)));

  const label = $derived(picked ? formatDate(new Date(picked.year, picked.month - 1, picked.day), { dateStyle: 'long' }) : '');
</script>

<Popover.Root bind:open>
  <div class="relative">
    <Popover.Trigger
      {id}
      {disabled}
      type="button"
      aria-invalid={invalid}
      class="field-control w-full flex items-center justify-between gap-2 text-start disabled:opacity-60 {invalid ? '!border-[var(--color-danger-600)]' : ''}"
    >
      <span class={label ? '' : 'text-[var(--text-tertiary)]'}>{label || placeholder || t('common.datePicker.choose')}</span>
      <i class="icon-calendar-days text-[14px] text-[var(--text-tertiary)]"></i>
    </Popover.Trigger>
    {#if clearable && value && !disabled}
      <button
        type="button"
        class="absolute end-9 top-1/2 -translate-y-1/2 grid size-6 place-items-center rounded text-[var(--text-tertiary)] hover:text-[inherit]"
        aria-label={t('common.datePicker.clear')}
        title={t('common.datePicker.clear')}
        onclick={() => (value = '')}
      >
        <i class="icon-x text-[13px]"></i>
      </button>
    {/if}
  </div>

  <Popover.Portal>
    <Popover.Content
      sideOffset={6}
      align="start"
      collisionPadding={8}
      class="z-[100] rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-3 shadow-[var(--shadow-xl)]"
    >
      <Calendar.Root
        type="single"
        value={picked}
        onValueChange={onSelect}
        locale={i18n.intl}
        weekStartsOn={1}
        fixedWeeks
        preventDeselect
        minValue={toDate(min)}
        maxValue={toDate(max)}
        class="w-[17.5rem] select-none"
      >
        {#snippet children({ months, weekdays })}
          <Calendar.Header class="mb-2 flex items-center justify-between">
            <Calendar.PrevButton class="grid size-8 place-items-center rounded-lg hover:bg-[var(--surface-sunken)] disabled:opacity-40" aria-label={t('common.datePicker.prev')}>
              <i class="icon-chevron-left text-[15px]"></i>
            </Calendar.PrevButton>
            <Calendar.Heading class="font-display text-[13.5px] font-bold capitalize" />
            <Calendar.NextButton class="grid size-8 place-items-center rounded-lg hover:bg-[var(--surface-sunken)] disabled:opacity-40" aria-label={t('common.datePicker.next')}>
              <i class="icon-chevron-right text-[15px]"></i>
            </Calendar.NextButton>
          </Calendar.Header>
          {#each months as month (month.value.toString())}
            <Calendar.Grid class="w-full border-collapse">
              <Calendar.GridHead>
                <Calendar.GridRow class="grid grid-cols-7">
                  {#each weekdays as d, di (di)}
                    <Calendar.HeadCell class="py-1 text-center text-[11px] font-semibold uppercase text-[var(--text-tertiary)]">{dayNames[di]}</Calendar.HeadCell>
                  {/each}
                </Calendar.GridRow>
              </Calendar.GridHead>
              <Calendar.GridBody>
                {#each month.weeks as week, wi (wi)}
                  <Calendar.GridRow class="grid grid-cols-7">
                    {#each week as date (date.toString())}
                      <Calendar.Cell {date} month={month.value} class="p-0.5 text-center">
                        <Calendar.Day
                          class="grid size-9 w-full place-items-center rounded-lg text-[12.5px] tabular-nums transition-colors
                            hover:bg-[var(--surface-sunken)]
                            data-[outside-month]:opacity-35
                            data-[disabled]:pointer-events-none data-[disabled]:opacity-30
                            data-[unavailable]:line-through
                            data-[today]:font-bold data-[today]:text-[var(--color-primary-600)] data-[today]:ring-1 data-[today]:ring-[var(--color-primary-600)]
                            data-[selected]:!bg-[var(--color-primary-600)] data-[selected]:!text-white data-[selected]:font-bold data-[selected]:ring-0"
                        />
                      </Calendar.Cell>
                    {/each}
                  </Calendar.GridRow>
                {/each}
              </Calendar.GridBody>
            </Calendar.Grid>
          {/each}
        {/snippet}
      </Calendar.Root>
      <div class="mt-2 flex justify-between border-t border-[var(--border-subtle)] pt-2">
        <button type="button" class="text-[12px] font-semibold text-[var(--color-primary-600)] disabled:opacity-40" disabled={(!!min && today.toString() < min) || (!!max && today.toString() > max)} onclick={() => onSelect(today)}>
          {t('common.datePicker.today')}
        </button>
        {#if clearable && value}
          <button type="button" class="text-[12px] text-[var(--text-tertiary)] hover:text-[inherit]" onclick={() => onSelect(undefined)}>{t('common.datePicker.clear')}</button>
        {/if}
      </div>
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>
