<script lang="ts">
  // Saklar on/off (bits-ui Switch) bergaya template. `checked` dua arah; label dikirim pemanggil (sudah diterjemahkan).
  import { Switch } from 'bits-ui';

  let {
    checked = $bindable(false),
    id,
    label,
    disabled = false
  }: { checked?: boolean; id?: string; label: string; disabled?: boolean } = $props();
</script>

<Switch.Root
  bind:checked
  {id}
  {disabled}
  aria-label={label}
  class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-[var(--border-subtle)] transition-colors disabled:cursor-not-allowed disabled:opacity-60 data-[state=checked]:bg-[var(--color-primary-600)] data-[state=unchecked]:bg-[var(--surface-sunken)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-primary-600)]"
>
  <Switch.Thumb class="pointer-events-none block size-[18px] rounded-full bg-white shadow transition-transform data-[state=checked]:translate-x-[22px] data-[state=unchecked]:translate-x-[3px]" />
</Switch.Root>
