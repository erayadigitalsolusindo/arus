<script lang="ts" module>
  export type SelectOption<T extends string = string> = { value: T; label: string; disabled?: boolean; hint?: string };
</script>

<script lang="ts" generics="T extends string">
  // Pilihan tunggal tanpa mengetik (pengganti <select> bawaan browser): tampilan seragam dengan Combobox,
  // navigasi papan ketik penuh (panah, Home/End, ketik huruf awal, Enter, Esc) lewat bits-ui.
  // `value` = nilai terpilih; opsi dengan value '' dan disabled dipakai sebagai teks placeholder.
  import { Select } from 'bits-ui';

  let {
    value = $bindable(),
    options,
    id,
    placeholder = '',
    ariaLabel,
    disabled = false,
    invalid = false,
    variant = 'default',
    class: cls = '',
    onchange
  }: {
    value?: T;
    options: SelectOption<T>[];
    id?: string;
    placeholder?: string;
    ariaLabel?: string;
    disabled?: boolean;
    invalid?: boolean;
    variant?: 'default' | 'sidebar';
    class?: string;
    onchange?: (value: T) => void;
  } = $props();

  const current = $derived(options.find((o) => o.value === value));
  // Placeholder: tidak ada opsi cocok, atau opsi kosong yang dinonaktifkan (berfungsi sebagai petunjuk).
  const muted = $derived(!current || (current.value === '' && !!current.disabled));
  const text = $derived(current?.label ?? placeholder);
  // Opsi kosong yang dinonaktifkan hanya teks petunjuk di pemicu, bukan pilihan di daftar.
  const choices = $derived(options.filter((o) => !(o.value === '' && o.disabled)));

  function pick(next: string) {
    value = next as T;
    onchange?.(next as T);
  }
</script>

<Select.Root type="single" value={value ?? ''} onValueChange={pick} {disabled} items={options}>
  <Select.Trigger
    {id}
    aria-label={ariaLabel}
    aria-invalid={invalid}
    class="picker-trigger {variant === 'sidebar' ? 'picker-trigger-sidebar' : 'field-control'} {cls}"
  >
    <span class="truncate {muted && variant === 'default' ? 'text-[var(--text-tertiary)]' : ''}">{text}</span>
    <i class="icon-chevron-down picker-chevron text-[12px] shrink-0 {variant === 'default' ? 'text-[var(--text-tertiary)]' : 'opacity-70'}" aria-hidden="true"></i>
  </Select.Trigger>
  <Select.Portal>
    <Select.Content sideOffset={4} class="picker-panel w-[var(--bits-select-anchor-width)]">
      <Select.Viewport>
        {#each choices as o (o.value)}
          <Select.Item value={o.value} label={o.label} disabled={o.disabled} class="picker-item">
            {#snippet children({ selected })}
              <span class="min-w-0">
                <span class="block truncate">{o.label}</span>
                {#if o.hint}<span class="picker-hint">{o.hint}</span>{/if}
              </span>
              {#if selected && !(o.value === '' && o.disabled)}<i class="icon-check text-[12px] shrink-0" aria-hidden="true"></i>{/if}
            {/snippet}
          </Select.Item>
        {/each}
      </Select.Viewport>
    </Select.Content>
  </Select.Portal>
</Select.Root>
