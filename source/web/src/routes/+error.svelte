<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { t } from '#lib/i18n/index.ts';

  const notFound = $derived(page.status === 404);
  const bubbles = Array.from({ length: 18 }, (_, i) => ({
    left: (i * 37 + 11) % 100,
    size: 6 + ((i * 7) % 16),
    delay: -((i * 1.3) % 12),
    dur: 8 + ((i * 3) % 9)
  }));

  const EMOJI = ['🐟', '🐠', '🐡', '🐟', '🐠', '🐟'];
  const rand = (a: number, b: number) => a + Math.random() * (b - a);

  // 404: 4 ikan berenang acak (target baru tiap beberapa detik, menghadap arah gerak).
  let wanderers = $state(
    Array.from({ length: 4 }, (_, i) => ({ e: EMOJI[i], size: i < 2 ? 64 : 32, x: rand(5, 90), y: rand(15, 80), dur: 4, flip: false }))
  );
  onMount(() => {
    if (!notFound || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    const timers = wanderers.map((_, i) => {
      const move = () => {
        const w = wanderers[i];
        const nx = rand(3, 92);
        w.flip = nx > w.x;
        w.dur = rand(3, 6);
        w.x = nx;
        w.y = rand(12, 82);
      };
      const id = setInterval(move, rand(3000, 5500));
      setTimeout(move, i * 250);
      return id;
    });
    return () => timers.forEach(clearInterval);
  });
</script>

<svelte:head><title>{page.status} · ACIRABA</title></svelte:head>

<main class="sea">
  <div class="rays" aria-hidden="true"></div>

  {#each bubbles as b, i (i)}
    <span class="bubble" aria-hidden="true" style="left:{b.left}%;width:{b.size}px;height:{b.size}px;animation-delay:{b.delay}s;animation-duration:{b.dur}s"></span>
  {/each}

  {#if notFound}
    {#each wanderers as w, i (i)}
      <div class="wander" aria-hidden="true" style="left:{w.x}%;top:{w.y}%;font-size:{w.size}px;transition-duration:{w.dur}s">
        <span style="display:inline-block;transform:scaleX({w.flip ? -1 : 1})">{w.e}</span>
      </div>
    {/each}
  {/if}
  <div class="shark" aria-hidden="true">
    <svg viewBox="0 0 220 90" width="220" height="90">
      <path d="M8 46 C40 14 110 10 160 36 L205 14 L192 46 L208 78 L160 56 C110 82 40 78 8 46Z" fill="#5b7a99" />
      <path d="M8 46 C40 66 110 70 160 56 L192 46 C130 52 60 52 8 46Z" fill="#cfe0ee" opacity=".85" />
      <path d="M92 22 L112 -4 L128 26Z" fill="#4a6783" />
      <path d="M100 62 L86 84 L124 64Z" fill="#4a6783" />
      <circle cx="46" cy="42" r="4" fill="#0b1f33" /><circle cx="47.2" cy="40.8" r="1.2" fill="#fff" />
      <path d="M62 52 l4 6 l4 -6 l4 6 l4 -6" stroke="#fff" stroke-width="2" fill="none" stroke-linejoin="round" />
    </svg>
  </div>
  {#if !notFound}
  {#each [5, 17, 29, 41, 53, 65] as top, i (i)}
    <div class="fish" aria-hidden="true" style="top:{top + 10}%;animation-delay:-{i * 3.7}s;animation-duration:{14 + (i % 3) * 3}s">{EMOJI[i]}</div>
  {/each}
  {/if}

  <section class="card">
    <div class="code">{page.status}</div>
    <h1>{notFound ? t('common.errorPage.notFoundTitle') : t('common.errorPage.serverTitle')}</h1>
    <p>{notFound ? t('common.errorPage.notFoundBody') : (page.error?.message && page.status !== 500 ? page.error.message : t('common.errorPage.serverBody'))}</p>
    <div class="actions">
      <a class="btn btn-primary" href="/">{t('common.errorPage.home')}</a>
      {#if !notFound}<button type="button" class="btn btn-outline ghost" onclick={() => location.reload()}>{t('common.errorPage.retry')}</button>{/if}
    </div>
  </section>

  <svg class="wave w1" viewBox="0 0 1440 160" preserveAspectRatio="none" aria-hidden="true">
    <path d="M0 80 C180 20 360 140 540 80 C720 20 900 140 1080 80 C1260 20 1350 100 1440 70 L1440 160 L0 160Z" fill="#0a3b78" opacity=".55" />
  </svg>
  <svg class="wave w2" viewBox="0 0 1440 160" preserveAspectRatio="none" aria-hidden="true">
    <path d="M0 90 C200 150 380 30 560 90 C740 150 920 30 1100 90 C1280 150 1360 60 1440 100 L1440 160 L0 160Z" fill="#071d3d" opacity=".8" />
  </svg>
</main>

<style>
  .sea {
    position: relative;
    min-height: 100vh;
    overflow: hidden;
    display: grid;
    place-items: center;
    padding: 24px 16px;
    color: #e8f3ff;
    background: linear-gradient(180deg, #1a8fd1 0%, #0a5aa8 28%, #0a3b78 60%, #050f24 100%);
  }
  .rays {
    position: absolute;
    inset: -20% -10% 30%;
    background: repeating-linear-gradient(100deg, rgb(255 255 255 / 0.1) 0 40px, transparent 40px 140px);
    filter: blur(18px);
    transform-origin: top;
    animation: sway 9s ease-in-out infinite alternate;
    mask-image: linear-gradient(180deg, #000, transparent 85%);
  }
  .bubble {
    position: absolute;
    bottom: -30px;
    border-radius: 50%;
    border: 1.5px solid rgb(255 255 255 / 0.6);
    background: radial-gradient(circle at 30% 30%, rgb(255 255 255 / 0.55), rgb(255 255 255 / 0.05));
    animation: rise linear infinite;
  }
  .shark {
    position: absolute;
    top: 18%;
    left: 0;
    animation: swim 22s linear infinite;
    filter: drop-shadow(0 8px 12px rgb(0 0 0 / 0.35));
  }
  .fish {
    position: absolute;
    font-size: 28px;
    animation: swim 16s linear infinite;
  }
  .wander {
    position: absolute;
    z-index: 1;
    transition-property: left, top;
    transition-timing-function: ease-in-out;
  }
  .card {
    position: relative;
    z-index: 2;
    max-width: 30rem;
    width: 100%;
    text-align: center;
    padding: 32px 28px;
    border-radius: 24px;
    background: rgb(5 20 45 / 0.45);
    border: 1px solid rgb(255 255 255 / 0.18);
    backdrop-filter: blur(10px);
    box-shadow: 0 20px 60px -15px rgb(0 0 0 / 0.6);
  }
  .code {
    font-size: clamp(72px, 18vw, 120px);
    font-weight: 800;
    line-height: 1;
    letter-spacing: -0.04em;
    background: linear-gradient(180deg, #fff, #7cc4ff);
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    animation: bob 4s ease-in-out infinite;
  }
  .card h1 { font-size: 22px; font-weight: 700; margin-top: 8px; color: #fff !important; }
  p { margin-top: 8px; font-size: 14px; color: #bcd9f2; }
  .actions { display: flex; gap: 10px; justify-content: center; margin-top: 22px; flex-wrap: wrap; }
  .ghost { color: #e8f3ff; border-color: rgb(255 255 255 / 0.35); background: transparent; }
  .wave { position: absolute; left: 0; width: 200%; height: 140px; bottom: 0; z-index: 1; }
  .w1 { animation: drift 14s linear infinite; }
  .w2 { height: 110px; animation: drift 9s linear infinite reverse; }

  @keyframes rise {
    from { transform: translateY(0) translateX(0); opacity: 0; }
    10% { opacity: 1; }
    to { transform: translateY(-110vh) translateX(24px); opacity: 0; }
  }
  @keyframes swim {
    from { transform: translateX(-260px); }
    to { transform: translateX(calc(100vw + 60px)); }
  }
  @keyframes swim-back {
    from { transform: translateX(calc(100vw + 60px)); }
    to { transform: translateX(-260px); }
  }
  @keyframes drift { to { transform: translateX(-50%); } }
  @keyframes bob { 50% { transform: translateY(-8px); } }
  @keyframes sway { to { transform: rotate(4deg) scaleX(1.1); } }
  @media (prefers-reduced-motion: reduce) {
    .rays, .bubble, .shark, .fish, .wave, .code { animation: none; }
    .wander { transition: none; }
    .shark { left: 8%; }
  }
</style>
