<script lang="ts">
  import { onMount } from 'svelte';

  // Hujan karakter ala matrix di <canvas>. Hanya dekorasi: aria-hidden, berhenti saat tab tersembunyi / di luar layar,
  // dan statis bila pengguna memilih prefers-reduced-motion.
  let { speed = 1, opacity = 0.9 }: { speed?: number; opacity?: number } = $props();

  const GLYPHS = 'アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン0123456789ARUS<>{}=+*#';
  const SIZE = 15;

  let canvas: HTMLCanvasElement;

  onMount(() => {
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;

    let w = 0;
    let h = 0;
    let drops: number[] = [];
    let raf = 0;
    let last = 0;
    let visible = true;
    let color = '96 165 250';

    function readColor() {
      const v = getComputedStyle(document.documentElement).getPropertyValue('--color-primary-400').trim();
      const m = v.match(/#([0-9a-f]{6})/i);
      if (m) {
        const n = parseInt(m[1], 16);
        color = `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`;
      }
    }

    function resize() {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = canvas.clientWidth;
      h = canvas.clientHeight;
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx!.font = `${SIZE}px ui-monospace, monospace`;
      const cols = Math.ceil(w / SIZE);
      drops = Array.from({ length: cols }, () => Math.random() * -40);
      ctx!.clearRect(0, 0, w, h);
    }

    function frame(now: number) {
      raf = requestAnimationFrame(frame);
      if (!visible || now - last < 55 / speed) return;
      last = now;
      // Jejak memudar: timpa dengan lapisan transparan (destination-out) agar latar kartu tetap terlihat.
      ctx!.globalCompositeOperation = 'destination-out';
      ctx!.fillStyle = 'rgba(0,0,0,0.12)';
      ctx!.fillRect(0, 0, w, h);
      ctx!.globalCompositeOperation = 'source-over';
      for (let i = 0; i < drops.length; i++) {
        const y = drops[i] * SIZE;
        const ch = GLYPHS[Math.floor(Math.random() * GLYPHS.length)];
        // kepala terang, badan berwarna
        ctx!.fillStyle = `rgb(235 245 255 / ${opacity})`;
        ctx!.fillText(ch, i * SIZE, y);
        ctx!.fillStyle = `rgb(${color} / ${opacity * 0.75})`;
        ctx!.fillText(GLYPHS[Math.floor(Math.random() * GLYPHS.length)], i * SIZE, y - SIZE);
        if (y > h && Math.random() > 0.975) drops[i] = 0;
        drops[i] += 1;
      }
    }

    readColor();
    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(canvas);
    const io = new IntersectionObserver(([e]) => (visible = e.isIntersecting));
    io.observe(canvas);
    const onVis = () => (visible = !document.hidden);
    document.addEventListener('visibilitychange', onVis);

    if (reduce) {
      // Satu bingkai statis.
      for (let k = 0; k < 30; k++) {
        for (let i = 0; i < drops.length; i++) {
          ctx.fillStyle = `rgb(${color} / ${Math.random() * 0.5})`;
          ctx.fillText(GLYPHS[Math.floor(Math.random() * GLYPHS.length)], i * SIZE, Math.random() * h);
        }
      }
    } else {
      raf = requestAnimationFrame(frame);
    }

    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
      io.disconnect();
      document.removeEventListener('visibilitychange', onVis);
    };
  });
</script>

<canvas bind:this={canvas} class="absolute inset-0 size-full" aria-hidden="true"></canvas>
