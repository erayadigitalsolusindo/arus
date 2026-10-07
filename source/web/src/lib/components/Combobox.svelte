<script lang="ts" module>
  export type Option = { id: string; name: string };
</script>

<script lang="ts">
  // Combobox dengan pencarian server-side (bits-ui). Daftar opsi tidak difilter di klien: setiap ketikan
  // (di-debounce) memanggil `search(q)` sehingga master besar tidak perlu dimuat seluruhnya.
  // `value` = id terpilih ('' = kosong); `label` = nama terpilih untuk ditampilkan (diisi saat memilih; saat
  // mengubah data yang sudah ada, pemanggil menyetelnya dari data item).
  import { Combobox } from 'bits-ui';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let {
    value = $bindable(''),
    label = $bindable(''),
    search,
    id,
    placeholder = '',
    disabled = false,
    invalid = false,
    clearable = true
  }: {
    value?: string;
    label?: string;
    search: (q: string) => Promise<Option[]>;
    id?: string;
    placeholder?: string;
    disabled?: boolean;
    invalid?: boolean;
    clearable?: boolean;
  } = $props();

  let open = $state(false);
  let query = $state('');
  let options = $state<Option[]>([]);
  let loading = $state(false);
  let failed = $state('');

  let seq = 0; // hanya respons permintaan terbaru yang dipakai
  async function run(q: string) {
    const mine = ++seq;
    loading = true;
    failed = '';
    try {
      const res = await search(q);
      if (mine === seq) options = res;
    } catch (err) {
      if (mine === seq) {
        options = [];
        failed = errorMessage(err);
      }
    } finally {
      if (mine === seq) loading = false;
    }
  }

  // Muat saat dibuka dan setiap `query` berubah (jeda 250 ms agar tidak membanjiri server).
  $effect(() => {
    if (!open) return;
    const q = query.trim();
    const h = setTimeout(() => void run(q), q ? 250 : 0);
    return () => clearTimeout(h);
  });

  function onOpenChange(next: boolean) {
    open = next;
    if (!next) query = '';
  }

  function onValueChange(next: string) {
    label = options.find((o) => o.id === next)?.name ?? label;
  }

  function clear() {
    value = '';
    label = '';
    query = '';
  }
</script>

<Combobox.Root type="single" bind:value bind:open {onOpenChange} {onValueChange} {disabled}>
  <div class="relative">
    <Combobox.Input
      {id}
      class="field-control w-full !pe-14"
      {placeholder}
      defaultValue={label}
      clearOnDeselect
      aria-invalid={invalid}
      oninput={(e) => {
        query = e.currentTarget.value;
        open = true; // mengetik membuka daftar (bits-ui tidak melakukannya sendiri)
      }}
      onclick={() => (open = true)}
    />
    <div class="absolute end-1.5 top-1/2 -translate-y-1/2 flex items-center">
      {#if clearable && value && !disabled}
        <button type="button" class="header-icon-btn !size-6" aria-label={t('catalog.combobox.clear')} onclick={clear}><i class="icon-x text-[12px]"></i></button>
      {/if}
      <Combobox.Trigger class="header-icon-btn !size-6" aria-label={placeholder || t('catalog.combobox.placeholder')}><i class="icon-chevron-down text-[12px]"></i></Combobox.Trigger>
    </div>
  </div>
  <Combobox.Portal>
    <Combobox.Content
      sideOffset={4}
      class="z-[1100] w-[var(--bits-combobox-anchor-width)] min-w-48 max-h-64 overflow-y-auto scroll-thin rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-raised)] p-1 shadow-lg"
    >
      {#each options as o (o.id)}
        <Combobox.Item
          value={o.id}
          label={o.name}
          class="flex items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-[12.5px] cursor-pointer data-[highlighted]:bg-[var(--surface-sunken)] data-[selected]:font-semibold"
        >
          {#snippet children({ selected })}
            <span class="truncate">{o.name}</span>
            {#if selected}<i class="icon-check text-[12px] shrink-0"></i>{/if}
          {/snippet}
        </Combobox.Item>
      {/each}
      {#if failed}
        <div class="px-2.5 py-2 text-[12px] text-[var(--color-danger-600)]">{t('catalog.combobox.loadFailed')} {failed}</div>
      {:else if loading && options.length === 0}
        <div class="px-2.5 py-2 text-[12px] text-[var(--text-tertiary)]">{t('catalog.combobox.searching')}</div>
      {:else if !loading && options.length === 0}
        <div class="px-2.5 py-2 text-[12px] text-[var(--text-tertiary)]">{t('catalog.combobox.noResults')}</div>
      {/if}
    </Combobox.Content>
  </Combobox.Portal>
</Combobox.Root>
