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
  let query = $state(''); // teks pencarian yang dikirim ke server
  let inputValue = $state(label); // teks yang tampil di kotak: nama terpilih saat tertutup, ketikan saat mencari
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

  // Saat tertutup, kotak selalu menampilkan nama terpilih (ketikan yang tidak dipilih dibuang; label yang diubah
  // dari luar, mis. data item baru dimuat, ikut tampil).
  $effect(() => {
    const l = label;
    if (!open) inputValue = l;
  });

  function onOpenChange(next: boolean) {
    open = next;
    if (!next) query = '';
  }

  function onValueChange(next: string) {
    label = options.find((o) => o.id === next)?.name ?? label;
    inputValue = label;
  }

  function clear() {
    value = '';
    label = '';
    query = '';
    inputValue = '';
  }
</script>

<Combobox.Root type="single" bind:value bind:open {inputValue} {onOpenChange} {onValueChange} {disabled}>
  <div class="relative">
    <!-- onfocus: teks terpilih semua sehingga mengetik langsung menggantikan nama terpilih. -->
    <Combobox.Input
      {id}
      class="field-control picker-trigger !pe-14 {invalid ? '!border-[var(--color-danger-600)]' : ''}"
      {placeholder}
      aria-invalid={invalid}
      autocomplete="off"
      oninput={(e) => {
        query = inputValue = e.currentTarget.value; // cermin teks kotak (inputValue hanya prop satu arah ke bits-ui)
        open = true; // mengetik membuka daftar (bits-ui tidak melakukannya sendiri)
      }}
      onclick={() => (open = true)}
      onfocus={(e) => e.currentTarget.select()}
    />
    <div class="absolute end-1.5 top-1/2 -translate-y-1/2 flex items-center">
      {#if clearable && value && !disabled}
        <button type="button" class="header-icon-btn !size-6" aria-label={t('catalog.combobox.clear')} onclick={clear}><i class="icon-x text-[12px]"></i></button>
      {/if}
      {#if loading && open}<i class="icon-loader-circle animate-spin text-[13px] mx-1 text-[var(--text-tertiary)]" role="status" aria-label={t('catalog.combobox.searching')}></i>{/if}
      <Combobox.Trigger class="header-icon-btn !size-6 {open ? '[&_i]:rotate-180' : ''}" aria-label={placeholder || t('catalog.combobox.placeholder')}><i class="icon-chevron-down text-[12px] transition-transform"></i></Combobox.Trigger>
    </div>
  </div>
  <Combobox.Portal>
    <Combobox.Content
      sideOffset={4}
      class="picker-panel scroll-thin w-[var(--bits-combobox-anchor-width)]"
    >
      {#each options as o (o.id)}
        <Combobox.Item
          value={o.id}
          label={o.name}
          class="picker-item"
        >
          {#snippet children({ selected })}
            <span class="truncate">{o.name}</span>
            {#if selected}<i class="icon-check text-[12px] shrink-0" aria-hidden="true"></i>{/if}
          {/snippet}
        </Combobox.Item>
      {/each}
      {#if failed}
        <div class="picker-empty !text-[var(--color-danger-600)]" role="alert"><i class="icon-circle-alert text-[13px] shrink-0"></i><span>{t('catalog.combobox.loadFailed')} {failed}</span></div>
      {:else if loading && options.length === 0}
        <div class="picker-empty"><i class="icon-loader-circle animate-spin text-[13px]"></i><span>{t('catalog.combobox.searching')}</span></div>
      {:else if !loading && options.length === 0}
        <div class="picker-empty"><i class="icon-search-x text-[13px]"></i><span>{t('catalog.combobox.noResults')}</span></div>
      {/if}
    </Combobox.Content>
  </Combobox.Portal>
</Combobox.Root>
