<script lang="ts">
  let { class: className = '' }: { class?: string } = $props();
</script>

<div class={className} aria-hidden="true">
  <svg class="tk-anim" viewBox="0 40 600 160">
    <rect x="0" y="170" width="600" height="24" fill="#1e293b" />
    <path d="M0 182H600" stroke="#fbbf24" stroke-width="2.5" stroke-dasharray="16 14" fill="none" />

    <rect x="8" y="78" width="120" height="92" fill="#e2e8f0" />
    <rect x="2" y="66" width="132" height="12" rx="3" fill="#1d4ed8" />
    <rect x="22" y="86" width="84" height="20" rx="4" fill="#1d4ed8" />
    <text x="64" y="100" text-anchor="middle" font-size="11" font-weight="700" fill="#fff" font-family="system-ui,sans-serif">GUDANG A</text>
    <rect x="84" y="116" width="36" height="54" fill="#1e293b" />
    <g class="tk-door tk-door-a">
      <rect x="84" y="116" width="36" height="54" fill="#cbd5e1" />
      <path d="M84 128H120M84 140H120M84 152H120M84 164H120" stroke="#94a3b8" stroke-width="1.5" />
    </g>
    <rect x="26" y="146" width="22" height="24" fill="#f59e0b" />
    <rect x="48" y="156" width="18" height="14" fill="#fbbf24" />

    <rect x="472" y="78" width="120" height="92" fill="#e2e8f0" />
    <rect x="466" y="66" width="132" height="12" rx="3" fill="#1d4ed8" />
    <rect x="494" y="86" width="84" height="20" rx="4" fill="#1d4ed8" />
    <text x="536" y="100" text-anchor="middle" font-size="11" font-weight="700" fill="#fff" font-family="system-ui,sans-serif">GUDANG B</text>
    <rect x="480" y="116" width="36" height="54" fill="#1e293b" />
    <g class="tk-door tk-door-b">
      <rect x="480" y="116" width="36" height="54" fill="#cbd5e1" />
      <path d="M480 128H516M480 140H516M480 152H516M480 164H516" stroke="#94a3b8" stroke-width="1.5" />
    </g>
    <rect x="552" y="146" width="22" height="24" fill="#f59e0b" />
    <rect x="534" y="156" width="18" height="14" fill="#fbbf24" />

    <g transform="translate(170,169)">
      <g class="tk-mv">
        <g class="tk-fl">
          <ellipse cx="0" cy="2" rx="52" ry="4" fill="#000" opacity=".3" />
          <g class="tk-bob">
            <rect x="-55" y="-62" width="72" height="50" rx="3" fill="#2563eb" />
            <rect x="-55" y="-62" width="72" height="6" rx="3" fill="#60a5fa" />
            <path d="M-37-52V-14M-19-52V-14" stroke="#1d4ed8" stroke-width="1.5" />
            <text x="-19" y="-30" text-anchor="middle" font-size="9" font-weight="700" fill="#fff" font-family="system-ui,sans-serif">LOGISTIK</text>
            <rect x="-55" y="-12" width="108" height="6" fill="#0f172a" />
            <path d="M20-50H38L54-28V-12H20Z" fill="#f8fafc" />
            <path d="M26-44H37L47-30H26Z" fill="#38bdf8" />
            <rect x="50" y="-14" width="6" height="5" rx="1" fill="#94a3b8" />
            <circle cx="53" cy="-20" r="2.5" fill="#fde047" />
            <rect x="17" y="-60" width="3" height="14" fill="#94a3b8" />
            <circle class="tk-smoke" cx="18" cy="-62" r="3" fill="#cbd5e1" />
            <circle class="tk-smoke" cx="18" cy="-62" r="3" fill="#cbd5e1" style="animation-delay:.9s" />
          </g>
          <g transform="translate(-38,-11)"><g class="tk-spin"><circle r="11" fill="#0f172a" /><circle r="5" fill="#94a3b8" /><path d="M-5 0H5M0-5V5" stroke="#0f172a" stroke-width="1.5" /></g></g>
          <g transform="translate(-16,-11)"><g class="tk-spin"><circle r="11" fill="#0f172a" /><circle r="5" fill="#94a3b8" /><path d="M-5 0H5M0-5V5" stroke="#0f172a" stroke-width="1.5" /></g></g>
          <g transform="translate(36,-11)"><g class="tk-spin"><circle r="11" fill="#0f172a" /><circle r="5" fill="#94a3b8" /><path d="M-5 0H5M0-5V5" stroke="#0f172a" stroke-width="1.5" /></g></g>
        </g>
      </g>
    </g>
  </svg>
</div>

<style>
  .tk-anim { width: 100%; height: auto; display: block; }
  .tk-mv { animation: tk-move 12s infinite ease-in-out; }
  .tk-fl { transform-box: fill-box; transform-origin: center; animation: tk-flip 12s infinite; }
  .tk-bob { animation: tk-bob 0.5s infinite alternate ease-in-out; }
  .tk-spin { transform-box: fill-box; transform-origin: center; animation: tk-spin 12s infinite ease-in-out; }
  .tk-smoke { transform-box: fill-box; transform-origin: center; animation: tk-smoke 1.8s infinite ease-out; opacity: 0; }
  .tk-door { transform-box: fill-box; transform-origin: 50% 0; }
  .tk-door-a { animation: tk-door-a 12s infinite ease-in-out; }
  .tk-door-b { animation: tk-door-b 12s infinite ease-in-out; }

  @keyframes tk-move { 0% { transform: translate(0, 0); } 40%, 50% { transform: translate(260px, 0); } 90%, 100% { transform: translate(0, 0); } }
  @keyframes tk-spin { 0% { transform: rotate(0); } 40%, 50% { transform: rotate(1440deg); } 90%, 100% { transform: rotate(2880deg); } }
  @keyframes tk-flip { 0%, 49.9% { transform: scaleX(1); } 50%, 99.9% { transform: scaleX(-1); } 100% { transform: scaleX(1); } }
  @keyframes tk-bob { from { transform: translateY(0); } to { transform: translateY(-1px); } }
  @keyframes tk-smoke { 0% { transform: translate(0, 0) scale(0.5); opacity: 0.6; } 100% { transform: translate(-12px, -24px) scale(1.7); opacity: 0; } }
  @keyframes tk-door-a { 0%, 6% { transform: scaleY(0.12); } 14%, 78% { transform: scaleY(1); } 86%, 100% { transform: scaleY(0.12); } }
  @keyframes tk-door-b { 0%, 32% { transform: scaleY(1); } 38%, 56% { transform: scaleY(0.12); } 64%, 100% { transform: scaleY(1); } }

  @media (prefers-reduced-motion: reduce) {
    .tk-mv, .tk-fl, .tk-bob, .tk-spin, .tk-smoke, .tk-door { animation: none; }
  }
</style>