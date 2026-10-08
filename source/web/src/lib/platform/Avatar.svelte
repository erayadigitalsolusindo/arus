<script lang="ts">
  import { initials } from '#lib/auth/initials.ts';

  let { name, size = 36, square = false }: { name: string; size?: number; square?: boolean } = $props();

  // Warna stabil per nama (hash sederhana → hue), jadi tiap tenant mudah dikenali sekilas.
  const hue = $derived.by(() => {
    let h = 0;
    for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) % 360;
    return h;
  });
</script>

<span
  class="inline-flex shrink-0 items-center justify-center font-bold text-white select-none {square ? 'rounded-xl' : 'rounded-full'}"
  style="width:{size}px;height:{size}px;font-size:{Math.round(size * 0.38)}px;background:linear-gradient(135deg,hsl({hue} 70% 52%),hsl({(hue + 40) % 360} 70% 40%))"
  aria-hidden="true">{initials(name) || '?'}</span
>
