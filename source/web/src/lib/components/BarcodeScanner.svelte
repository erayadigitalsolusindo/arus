<script lang="ts">
  import { t } from '#lib/i18n/index.ts';

  // Lebar tiap batang (px); celah antar batang 3px. Dekoratif, bukan barcode sungguhan.
  const bars = [3, 1, 2, 1, 4, 2, 1, 3, 1, 2, 4, 1, 2, 1, 3, 2, 1, 4, 1, 2, 3, 1, 2, 1, 4, 2, 1, 3, 2, 1];
  let x = 0;
  const rects = bars.map((w, i) => {
    const r = { x, w, key: i };
    x += w + 2;
    return r;
  });
  const width = x;
</script>

<div class="scanner mb-6 inline-flex items-center gap-3" aria-hidden="true">
  <span class="relative block overflow-hidden rounded-md px-2 py-1.5 border border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
    <svg viewBox="0 0 {width} 28" width={width} height="28" class="block text-[var(--text-primary,#141414)]" style="width:{width * 1.3}px;height:36px">
      {#each rects as r (r.key)}
        <rect x={r.x} y="0" width={r.w} height="28" fill="currentColor" opacity="0.85" />
      {/each}
    </svg>
    <span class="scan-line absolute inset-x-1 h-[2px] rounded-full bg-[var(--color-danger-500,#ef4444)]"></span>
  </span>
  <span class="text-[10.5px] font-semibold uppercase tracking-wide text-tertiary leading-tight">
    <span class="beep inline-flex items-center gap-1 text-[var(--color-primary-600)]"><i class="icon-check text-[11px]"></i>{t('auth.promo.scanRead')}</span>
    <span class="block font-medium normal-case tracking-normal">{t('auth.promo.scanTagline')}</span>
  </span>
</div>

<style>
  .scan-line {
    top: 4px;
    box-shadow: 0 0 8px 1px rgb(239 68 68 / 0.6);
    animation: scan 2.4s ease-in-out infinite;
  }

  .beep {
    animation: beep 2.4s ease-in-out infinite;
  }

  @keyframes scan {
    0%,
    8% {
      top: 4px;
      opacity: 0;
    }
    15% {
      opacity: 1;
    }
    50% {
      top: calc(100% - 6px);
      opacity: 1;
    }
    85% {
      top: 4px;
      opacity: 1;
    }
    100% {
      top: 4px;
      opacity: 0;
    }
  }

  @keyframes beep {
    0%,
    45% {
      opacity: 0.25;
    }
    52%,
    80% {
      opacity: 1;
    }
    100% {
      opacity: 0.25;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .scan-line {
      animation: none;
      top: 50%;
      opacity: 1;
    }
    .beep {
      animation: none;
      opacity: 1;
    }
  }
</style>
