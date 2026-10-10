<script lang="ts">
  // Grafik batang ringan (tanpa pustaka): tinggi proporsional, batang disorot, nilai muncul saat disentuh/diarahkan.
  type Bar = { key: string; label: string; value: number; tip: string; tick?: boolean };
  let { bars, highlight, height = 150, axis }: { bars: Bar[]; highlight?: string; height?: number; axis: (v: number) => string } = $props();

  const max = $derived(Math.max(1, ...bars.map((b) => b.value)));
  let active = $state<string | null>(null);
  const shown = $derived(bars.find((b) => b.key === (active ?? highlight)));
</script>

<div>
  <p class="h-5 text-[11.5px] font-semibold u-color-text-secondary truncate">{shown ? shown.tip : ''}</p>
  <div class="relative" style="height:{height}px">
    <div class="absolute inset-x-0 top-0 border-t border-dashed border-[var(--border-subtle)]"></div>
    <div class="absolute inset-x-0 top-1/2 border-t border-dashed border-[var(--border-subtle)]"></div>
    <span class="absolute start-0 -top-0.5 text-[10px] bg-[var(--surface-card)] pe-1 u-color-text-tertiary">{axis(max)}</span>
    <div class="absolute inset-0 flex items-end gap-[3px] ps-0.5">
      {#each bars as b (b.key)}
        <button
          type="button"
          class="group flex-1 min-w-0 h-full flex items-end outline-none"
          aria-label={b.tip}
          onmouseenter={() => (active = b.key)}
          onmouseleave={() => (active = null)}
          onfocus={() => (active = b.key)}
          onblur={() => (active = null)}
        >
          <span
            class="w-full rounded-t-[4px] transition-[height,opacity] duration-300 {b.key === highlight ? 'bg-[var(--color-primary-600)]' : 'bg-[var(--color-primary-300)] group-hover:bg-[var(--color-primary-500)] group-focus-visible:bg-[var(--color-primary-500)]'}"
            style="height:{b.value > 0 ? Math.max(3, (b.value / max) * 100) : 0}%"
          ></span>
        </button>
      {/each}
    </div>
  </div>
  <div class="flex gap-[3px] ps-0.5 mt-1.5">
    {#each bars as b (b.key)}
      <span class="flex-1 min-w-0 text-center text-[10px] leading-none u-color-text-tertiary {b.tick === false ? 'invisible' : ''}">{b.label}</span>
    {/each}
  </div>
</div>
