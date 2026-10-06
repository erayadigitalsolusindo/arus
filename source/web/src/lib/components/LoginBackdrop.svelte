<script lang="ts">
  // Ikon retail samar yang melayang di belakang form. Posisi dalam persen; delay/durasi bervariasi agar tidak seragam.
  const icons = [
    { n: 'receipt', l: 8, t: 12, s: 34, d: 0, dur: 14 },
    { n: 'barcode', l: 78, t: 8, s: 38, d: 2, dur: 17 },
    { n: 'shopping-cart', l: 88, t: 38, s: 32, d: 4, dur: 15 },
    { n: 'package', l: 6, t: 46, s: 36, d: 1, dur: 16 },
    { n: 'banknote', l: 80, t: 68, s: 34, d: 3, dur: 13 },
    { n: 'tags', l: 14, t: 78, s: 30, d: 5, dur: 18 },
    { n: 'store', l: 52, t: 90, s: 32, d: 2.5, dur: 15 },
    { n: 'boxes', l: 60, t: 4, s: 30, d: 6, dur: 19 }
  ];
</script>

<div class="backdrop absolute inset-0 pointer-events-none overflow-hidden" aria-hidden="true">
  <div class="absolute inset-0 dots"></div>
  <span class="blob blob-a"></span>
  <span class="blob blob-b"></span>
  {#each icons as ic (ic.n)}
    <i
      class="icon-{ic.n} float absolute text-[var(--color-primary-600)]"
      style="left:{ic.l}%;top:{ic.t}%;font-size:{ic.s}px;animation-delay:-{ic.d}s;animation-duration:{ic.dur}s"
    ></i>
  {/each}
</div>

<style>
  .dots {
    background-image: radial-gradient(circle, color-mix(in oklab, var(--color-primary-600) 22%, transparent) 1px, transparent 1.4px);
    background-size: 22px 22px;
    mask-image: radial-gradient(ellipse at 50% 50%, transparent 25%, #000 85%);
    -webkit-mask-image: radial-gradient(ellipse at 50% 50%, transparent 25%, #000 85%);
    opacity: 0.55;
  }

  .blob {
    position: absolute;
    border-radius: 9999px;
    filter: blur(60px);
    opacity: 0.18;
    background: var(--color-primary-500);
    animation: drift 18s ease-in-out infinite;
  }

  .blob-a {
    width: 280px;
    height: 280px;
    top: -90px;
    left: -80px;
  }

  .blob-b {
    width: 240px;
    height: 240px;
    bottom: -80px;
    right: -60px;
    background: var(--color-accent-500, var(--color-primary-400));
    animation-delay: -9s;
  }

  .float {
    opacity: 0.09;
    animation-name: float;
    animation-timing-function: ease-in-out;
    animation-iteration-count: infinite;
  }

  @keyframes float {
    0%,
    100% {
      transform: translate3d(0, 0, 0) rotate(-6deg);
    }
    50% {
      transform: translate3d(8px, -16px, 0) rotate(6deg);
    }
  }

  @keyframes drift {
    0%,
    100% {
      transform: translate3d(0, 0, 0) scale(1);
    }
    50% {
      transform: translate3d(30px, 24px, 0) scale(1.12);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .float,
    .blob {
      animation: none;
    }
  }
</style>
