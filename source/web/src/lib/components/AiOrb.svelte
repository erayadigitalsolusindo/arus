<script lang="ts">
  // Inti AI animasi (hiasan, tanpa makna): cincin giroskop berputar, inti berdenyut, simpul bercahaya. Murni SVG + CSS;
  // gerak dimatikan oleh prefers-reduced-motion. `size` = sisi dalam px, `busy` = mempercepat putaran saat AI bekerja.
  let { size = 200, busy = false }: { size?: number; busy?: boolean } = $props();
</script>

<div class="orb" class:busy style="width:{size}px;height:{size}px" aria-hidden="true">
  <svg viewBox="0 0 240 240" width={size} height={size}>
    <defs>
      <radialGradient id="orb-core" cx="50%" cy="45%" r="60%">
        <stop offset="0%" stop-color="#ffffff" stop-opacity="0.95" />
        <stop offset="30%" style="stop-color:var(--ha-b)" stop-opacity="0.9" />
        <stop offset="100%" style="stop-color:var(--ha-a)" stop-opacity="0" />
      </radialGradient>
      <linearGradient id="orb-ring" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" style="stop-color:var(--ha-b)" />
        <stop offset="55%" style="stop-color:var(--ha-a)" />
        <stop offset="100%" style="stop-color:var(--ha-c)" />
      </linearGradient>
      <filter id="orb-blur" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="9" /></filter>
    </defs>

    <circle class="halo" cx="120" cy="120" r="86" fill="url(#orb-core)" filter="url(#orb-blur)" />

    <g class="spin s1"><g transform="rotate(0 120 120)"><ellipse cx="120" cy="120" rx="104" ry="40" fill="none" stroke="url(#orb-ring)" stroke-width="1.4" opacity="0.8" /><circle class="node" cx="224" cy="120" r="4.2" /><circle class="node n2" cx="16" cy="120" r="2.6" /></g></g>
    <g class="spin s2"><g transform="rotate(60 120 120)"><ellipse cx="120" cy="120" rx="104" ry="40" fill="none" stroke="url(#orb-ring)" stroke-width="1.2" opacity="0.6" /><circle class="node" cx="224" cy="120" r="3.4" /></g></g>
    <g class="spin s3"><g transform="rotate(120 120 120)"><ellipse cx="120" cy="120" rx="104" ry="40" fill="none" stroke="url(#orb-ring)" stroke-width="1" opacity="0.45" /><circle class="node n2" cx="16" cy="120" r="3" /></g></g>

    <g class="core">
      <circle cx="120" cy="120" r="30" fill="url(#orb-core)" />
      <circle cx="120" cy="120" r="30" fill="none" style="stroke:var(--ha-b)" stroke-opacity="0.55" stroke-width="1" />
      <path d="M120 100 L123.6 116.4 L140 120 L123.6 123.6 L120 140 L116.4 123.6 L100 120 L116.4 116.4 Z" fill="#fff" fill-opacity="0.95" />
    </g>
  </svg>
</div>

<style>
  .orb {
    display: inline-block;
    filter: drop-shadow(0 0 22px color-mix(in oklab, var(--ha-a) 45%, transparent));
  }
  svg {
    overflow: visible;
  }
  .spin {
    transform-box: view-box;
    transform-origin: 120px 120px;
    animation: orb-spin 18s linear infinite;
  }
  .s2 {
    animation-duration: 26s;
    animation-direction: reverse;
  }
  .s3 {
    animation-duration: 34s;
  }
  .busy .spin {
    animation-duration: 3.2s;
  }
  .busy .s2 {
    animation-duration: 4.6s;
  }
  .busy .s3 {
    animation-duration: 6s;
  }
  .node {
    fill: var(--ha-b);
    filter: drop-shadow(0 0 5px var(--ha-b));
    animation: orb-blink 2.6s ease-in-out infinite;
  }
  .n2 {
    fill: var(--ha-c);
    animation-delay: -1.3s;
  }
  .core,
  .halo {
    transform-box: fill-box;
    transform-origin: center;
    animation: orb-pulse 3.4s ease-in-out infinite;
  }
  .halo {
    animation-duration: 4.6s;
  }
  .busy .core {
    animation-duration: 1.1s;
  }
  @keyframes orb-spin {
    to {
      transform: rotate(360deg);
    }
  }
  @keyframes orb-pulse {
    0%,
    100% {
      transform: scale(0.94);
      opacity: 0.92;
    }
    50% {
      transform: scale(1.07);
      opacity: 1;
    }
  }
  @keyframes orb-blink {
    0%,
    100% {
      opacity: 0.45;
    }
    50% {
      opacity: 1;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin,
    .core,
    .halo,
    .node {
      animation: none;
    }
  }
</style>
