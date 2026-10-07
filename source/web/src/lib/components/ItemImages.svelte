<script lang="ts" module>
  export type PendingImage = { file: File; url: string };
</script>

<script lang="ts">
  // Gambar item: banyak gambar, satu utama. Dua mode:
  //   - ubah (itemId terisi): setiap aksi langsung ke server (unggah, jadikan utama, hapus);
  //   - tambah (itemId null): file dikumpulkan di `pending` (+ `mainIndex`) dan diunggah form setelah item tersimpan.
  // Validasi di sini hanya untuk umpan balik cepat; server memeriksa isi file sebenarnya dan mengubahnya menjadi JPG.
  import { onDestroy } from 'svelte';
  import { items as api, imageUrl, type ItemImage } from '#lib/items/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import AuthImage from '#lib/components/AuthImage.svelte';

  let {
    itemId = null,
    itemName = '',
    images = $bindable([]),
    pending = $bindable([]),
    mainIndex = $bindable(0),
    readOnly = false
  }: {
    itemId?: string | null;
    itemName?: string;
    images?: ItemImage[];
    pending?: PendingImage[];
    mainIndex?: number;
    readOnly?: boolean;
  } = $props();

  const MAX = 5;
  const MAX_BYTES = 10 * 1024 * 1024;
  const TYPES = ['image/jpeg', 'image/png', 'image/webp'];

  let busy = $state(false);
  let messages = $state<string[]>([]);

  const count = $derived(itemId ? images.length : pending.length);
  const full = $derived(count >= MAX);

  /** Memilah file terpilih: yang lolos pemeriksaan cepat dan pesan untuk yang ditolak. */
  function screen(files: File[]): { ok: File[]; errs: string[] } {
    const ok: File[] = [];
    const errs: string[] = [];
    for (const f of files) {
      if (count + ok.length >= MAX) {
        errs.push(t('items.images.tooMany', { max: MAX }));
        break;
      }
      if (!TYPES.includes(f.type)) errs.push(t('items.images.badType', { name: f.name }));
      else if (f.size > MAX_BYTES) errs.push(t('items.images.tooLarge', { name: f.name }));
      else ok.push(f);
    }
    return { ok, errs };
  }

  async function reload() {
    if (itemId) images = (await api.get(itemId)).images;
  }

  async function onPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = ''; // memilih file yang sama lagi tetap memicu change
    if (!files.length) return;
    const { ok, errs } = screen(files);
    messages = errs;
    if (!itemId) {
      pending = [...pending, ...ok.map((file) => ({ file, url: URL.createObjectURL(file) }))];
      return;
    }
    busy = true;
    try {
      for (const file of ok) {
        try {
          await api.uploadImage(itemId, file);
        } catch (err) {
          messages = [...messages, t('items.images.failed', { name: file.name, reason: errorMessage(err) })];
        }
      }
      await reload();
    } finally {
      busy = false;
    }
  }

  async function act(fn: () => Promise<unknown>) {
    busy = true;
    messages = [];
    try {
      await fn();
      await reload();
    } catch (err) {
      messages = [errorMessage(err)];
    } finally {
      busy = false;
    }
  }

  const makeMain = (img: ItemImage) => act(() => api.setMainImage(itemId!, img.id));
  const remove = (img: ItemImage) => {
    if (confirm(t('items.images.confirmDelete'))) void act(() => api.deleteImage(itemId!, img.id));
  };

  function removePending(i: number) {
    URL.revokeObjectURL(pending[i].url);
    pending = pending.filter((_, idx) => idx !== i);
    if (i === mainIndex) mainIndex = 0;
    else if (i < mainIndex) mainIndex -= 1;
  }

  onDestroy(() => pending.forEach((p) => URL.revokeObjectURL(p.url)));

  async function openFull(img: ItemImage) {
    if (!itemId) return;
    try {
      window.open(await imageUrl(itemId, img.id, 'full'), '_blank', 'noopener');
    } catch (err) {
      messages = [errorMessage(err)];
    }
  }
</script>

<div class="space-y-3">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <p class="text-[11px] text-[var(--text-tertiary)] max-w-xl">{t('items.images.hint', { max: MAX })}{#if !itemId && !readOnly} {t('items.images.pendingHint')}{/if}</p>
    {#if !readOnly}
      <label class="btn !text-[12.5px] {full || busy ? 'opacity-60 pointer-events-none' : 'cursor-pointer'}">
        <i class="icon-image-plus text-[14px]"></i>{t('items.images.add')}
        <input type="file" class="sr-only" accept={TYPES.join(',')} multiple disabled={full || busy} onchange={onPick} />
      </label>
    {/if}
  </div>

  {#if messages.length}
    <ul role="alert" class="rounded-lg px-3 py-2 text-[12px] badge-danger space-y-0.5">
      {#each messages as m, i (i)}<li>{m}</li>{/each}
    </ul>
  {/if}
  {#if busy}
    <p class="text-[12px] text-[var(--text-tertiary)]" role="status"><i class="icon-loader-circle animate-spin text-[13px] me-1"></i>{t('items.images.uploading')}</p>
  {/if}

  {#if count === 0}
    <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('items.images.empty')}</p>
  {:else}
    <ul class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
      {#if itemId}
        {#each images as img, i (img.id)}
          <li class="rounded-lg border p-1.5 space-y-1.5 {img.is_main ? 'border-[var(--color-primary-600)]' : 'border-[var(--border-subtle)]'}">
            <button type="button" class="block w-full" onclick={() => openFull(img)} aria-label={t('items.images.alt', { n: i + 1, name: itemName })}>
              <AuthImage {itemId} imageId={img.id} alt={t('items.images.alt', { n: i + 1, name: itemName })} class="w-full aspect-square object-cover rounded-md" />
            </button>
            <div class="flex items-center justify-between gap-1">
              {#if img.is_main}
                <span class="badge-soft badge-info"><i class="icon-star text-[10px]"></i>{t('items.images.main')}</span>
              {:else if !readOnly}
                <button type="button" class="text-[11px] font-medium text-[var(--color-primary-600)] hover:underline disabled:opacity-50" disabled={busy} onclick={() => makeMain(img)}>{t('items.images.makeMain')}</button>
              {:else}<span></span>{/if}
              {#if !readOnly}
                <button type="button" class="header-icon-btn !size-7" aria-label={t('items.images.remove')} title={t('items.images.remove')} disabled={busy} onclick={() => remove(img)}>
                  <i class="icon-trash-2 text-[12px]"></i>
                </button>
              {/if}
            </div>
          </li>
        {/each}
      {:else}
        {#each pending as p, i (p.url)}
          <li class="rounded-lg border p-1.5 space-y-1.5 {i === mainIndex ? 'border-[var(--color-primary-600)]' : 'border-[var(--border-subtle)]'}">
            <img src={p.url} alt={p.file.name} class="w-full aspect-square object-cover rounded-md" />
            <div class="flex items-center justify-between gap-1">
              <label class="flex items-center gap-1 text-[11px] font-medium cursor-pointer">
                <input type="radio" name="main-image" checked={i === mainIndex} onchange={() => (mainIndex = i)} />{t('items.images.main')}
              </label>
              <button type="button" class="header-icon-btn !size-7" aria-label={t('items.images.remove')} title={t('items.images.remove')} onclick={() => removePending(i)}>
                <i class="icon-trash-2 text-[12px]"></i>
              </button>
            </div>
          </li>
        {/each}
      {/if}
    </ul>
  {/if}
</div>
