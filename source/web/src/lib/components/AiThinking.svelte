<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '#lib/i18n/index.ts';

  // Indikator "AI sedang bekerja": tiga titik memantul, bilah pindai berjalan, dan teks langkah yang bergantian.
  // Langkah hanya hiasan (bukan status sebenarnya dari server).
  const steps = ['s1', 's2', 's3', 's4'] as const;
  let i = $state(0);

  onMount(() => {
    const timer = setInterval(() => (i = (i + 1) % steps.length), 2200);
    return () => clearInterval(timer);
  });
</script>

<div class="think" role="status">
  <span class="dots" aria-hidden="true"><i></i><i></i><i></i></span>
  <span class="label">
    {#key i}<span class="step">{t(`ai.thinkingSteps.${steps[i]}`)}</span>{/key}
  </span>
  <span class="scan" aria-hidden="true"></span>
</div>

<style>
  .think {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.7rem 0.9rem;
    border-radius: 0.9rem;
    overflow: hidden;
    font-size: 12.5px;
    color: var(--text-secondary);
    border: 1px solid color-mix(in oklab, var(--ha-a) 30%, var(--border-default));
    background: color-mix(in oklab, var(--ha-a) 7%, var(--surface-base));
  }
  .dots {
    display: inline-flex;
    gap: 4px;
  }
  .dots i {
    width: 6px;
    height: 6px;
    border-radius: 999px;
    background: linear-gradient(135deg, var(--ha-b), var(--ha-a));
    animation: think-bounce 1.1s ease-in-out infinite;
  }
  .dots i:nth-child(2) {
    animation-delay: 0.15s;
  }
  .dots i:nth-child(3) {
    animation-delay: 0.3s;
  }
  .step {
    display: inline-block;
    animation: think-in 0.45s ease both;
  }
  .scan {
    position: absolute;
    inset: 0;
    background: linear-gradient(100deg, transparent 30%, color-mix(in oklab, var(--ha-b) 22%, transparent) 50%, transparent 70%);
    transform: translateX(-100%);
    animation: think-scan 2.4s ease-in-out infinite;
    pointer-events: none;
  }
  @keyframes think-bounce {
    0%,
    80%,
    100% {
      transform: translateY(0);
      opacity: 0.45;
    }
    40% {
      transform: translateY(-5px);
      opacity: 1;
    }
  }
  @keyframes think-in {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  @keyframes think-scan {
    to {
      transform: translateX(100%);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .dots i,
    .step,
    .scan {
      animation: none;
    }
    .scan {
      display: none;
    }
  }
</style>
