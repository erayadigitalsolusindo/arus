<script lang="ts" module>
  /** Satu slide: gambar server (itemId + imageId, butuh token) atau URL lokal (file yang belum diunggah). */
  export type Slide = { itemId?: string; imageId?: string; url?: string; alt: string };
</script>

<script lang="ts">
  // Penampil gambar layar penuh: slide show (panah/tombol/strip thumbnail) dan zoom (roda, tombol, klik ganda, geser).
  import { imageUrl } from '#lib/items/api.ts';
  import { t } from '#lib/i18n/index.ts';

  let { slides, index = 0, onclose }: { slides: Slide[]; index?: number; onclose: () => void } = $props();

  const MIN = 1;
  const MAX = 6;

  let current = $state(0);
  let src = $state('');
  let failed = $state(false);
  let scale = $state(1);
  let tx = $state(0);
  let ty = $state(0);
  let dragging = $state(false);
  let drag: { x: number; y: number; tx: number; ty: number } | null = null;
  let closeBtn: HTMLButtonElement | undefined;

  const slide = $derived(slides[current]);

  $effect(() => {
    current = Math.min(Math.max(index, 0), Math.max(slides.length - 1, 0));
  });

  // Muat gambar penuh setiap pindah slide.
  $effect(() => {
    const s = slide;
    scale = 1;
    tx = 0;
    ty = 0;
    src = '';
    failed = false;
    if (!s) return;
    if (s.url) {
      src = s.url;
      return;
    }
    let live = true;
    imageUrl(s.itemId!, s.imageId!, 'full').then(
      (u) => live && (src = u),
      () => live && (failed = true)
    );
    return () => (live = false);
  });

  // Pratinjau thumbnail di strip bawah.
  function thumb(s: Slide): Promise<string> | string {
    return s.url ?? imageUrl(s.itemId!, s.imageId!, 'thumb');
  }

  $effect(() => {
    closeBtn?.focus();
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => (document.body.style.overflow = prev);
  });

  function go(delta: number) {
    if (slides.length < 2) return;
    current = (current + delta + slides.length) % slides.length;
  }

  function setScale(next: number) {
    scale = Math.min(MAX, Math.max(MIN, next));
    if (scale === 1) {
      tx = 0;
      ty = 0;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose();
    else if (e.key === 'ArrowLeft') go(-1);
    else if (e.key === 'ArrowRight') go(1);
    else if (e.key === '+' || e.key === '=') setScale(scale * 1.4);
    else if (e.key === '-') setScale(scale / 1.4);
    else if (e.key === '0') setScale(1);
    else return;
    e.preventDefault();
  }

  // Pendengar non-pasif agar preventDefault berlaku (onwheel Svelte bersifat pasif).
  function wheelZoom(node: HTMLElement) {
    const h = (e: WheelEvent) => {
      e.preventDefault();
      setScale(scale * (e.deltaY < 0 ? 1.2 : 1 / 1.2));
    };
    node.addEventListener('wheel', h, { passive: false });
    return { destroy: () => node.removeEventListener('wheel', h) };
  }

  function onDown(e: PointerEvent) {
    if (scale <= 1) return;
    drag = { x: e.clientX, y: e.clientY, tx, ty };
    dragging = true;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onMove(e: PointerEvent) {
    if (!drag) return;
    tx = drag.tx + (e.clientX - drag.x);
    ty = drag.ty + (e.clientY - drag.y);
  }
  function onUp() {
    drag = null;
    dragging = false;
  }

  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }

  const btn = 'inline-flex size-10 items-center justify-center rounded-full bg-black/50 text-white hover:bg-black/70 disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-white';
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="lb-root" use:portal
  role="dialog"
  aria-modal="true"
  tabindex="-1"
  aria-label={slide?.alt}
  onclick={(e) => e.target === e.currentTarget && onclose()}
>
  <div class="lb-bar" onclick={(e) => e.target === e.currentTarget && onclose()}>
    <span class="text-[13px] tabular-nums px-2">{t('items.images.counter', { n: current + 1, total: slides.length })}</span>
    <div class="flex items-center gap-2">
      <button type="button" class={btn} onclick={() => setScale(scale / 1.4)} disabled={scale <= MIN} aria-label={t('items.images.zoomOut')} title={t('items.images.zoomOut')}><i class="icon-zoom-out text-[18px]"></i></button>
      <button type="button" class="{btn} !w-auto px-3 text-[12px] tabular-nums" onclick={() => setScale(1)} disabled={scale === 1} aria-label={t('items.images.zoomReset')} title={t('items.images.zoomReset')}>{Math.round(scale * 100)}%</button>
      <button type="button" class={btn} onclick={() => setScale(scale * 1.4)} disabled={scale >= MAX} aria-label={t('items.images.zoomIn')} title={t('items.images.zoomIn')}><i class="icon-zoom-in text-[18px]"></i></button>
      <button type="button" bind:this={closeBtn} class={btn} onclick={onclose} aria-label={t('items.images.close')} title={t('items.images.close')}><i class="icon-x text-[18px]"></i></button>
    </div>
  </div>

  <div class="lb-stage" onclick={(e) => e.target === e.currentTarget && onclose()} use:wheelZoom>
    {#if src}
      <img
        {src}
        alt={slide?.alt}
        draggable="false"
        class="max-h-full max-w-full object-contain select-none touch-none {scale > 1 ? (dragging ? 'cursor-grabbing' : 'cursor-grab') : 'cursor-zoom-in'}"
        style="transform: translate({tx}px, {ty}px) scale({scale}); transition: {dragging ? 'none' : 'transform 120ms ease-out'};"
        onpointerdown={onDown}
        onpointermove={onMove}
        onpointerup={onUp}
        onpointercancel={onUp}
        ondblclick={() => setScale(scale > 1 ? 1 : 2.5)}
      />
    {:else}
      <i class="{failed ? 'icon-image-off' : 'icon-loader-circle animate-spin'} text-[32px] text-white/70"></i>
    {/if}

    {#if slides.length > 1}
      <button type="button" class="{btn} absolute start-3 top-1/2 -translate-y-1/2" onclick={() => go(-1)} aria-label={t('items.images.prev')} title={t('items.images.prev')}><i class="icon-chevron-left text-[20px]"></i></button>
      <button type="button" class="{btn} absolute end-3 top-1/2 -translate-y-1/2" onclick={() => go(1)} aria-label={t('items.images.next')} title={t('items.images.next')}><i class="icon-chevron-right text-[20px]"></i></button>
    {/if}
  </div>

  {#if slides.length > 1}
    <ul class="lb-strip">
      {#each slides as s, i (i)}
        <li>
          <button type="button" class="block rounded-md overflow-hidden border-2 {i === current ? 'border-white' : 'border-transparent opacity-60 hover:opacity-100'}" onclick={() => (current = i)} aria-label={s.alt} aria-current={i === current}>
            {#await thumb(s)}
              <span class="block size-14 bg-white/10"></span>
            {:then u}
              <img src={u} alt="" class="size-14 object-cover" />
            {:catch}
              <span class="block size-14 bg-white/10"></span>
            {/await}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .lb-root { position: fixed; inset: 0; z-index: 1000; display: flex; flex-direction: column; background: rgb(0 0 0 / 0.55); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
  .lb-bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 12px; color: #fff; }
  .lb-stage { position: relative; flex: 1; min-height: 0; display: flex; align-items: center; justify-content: center; overflow: hidden; }
  .lb-stage img { max-width: 100%; max-height: 100%; }
  .lb-strip { display: flex; justify-content: center; gap: 8px; padding: 12px; overflow-x: auto; list-style: none; margin: 0; }
</style>
