<script lang="ts">
  let { label, value, icon, tone = 'primary', hint = '' }: { label: string; value: string; icon: string; tone?: 'primary' | 'success' | 'warning' | 'info' | 'danger'; hint?: string } = $props();
</script>

<div class="surface-card p-4 flex items-center gap-3.5">
  <span class="grid size-11 shrink-0 place-items-center rounded-xl" style="background:var(--color-{tone}-50);color:var(--color-{tone}-600)">
    <i class="icon-{icon} text-[20px]"></i>
  </span>
  <div class="min-w-0">
    <p class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] truncate">{label}</p>
    <p class="font-display font-bold text-[20px] leading-tight tabular-nums truncate">{value}</p>
    {#if hint}<p class="text-[11.5px] text-[var(--text-tertiary)] truncate">{hint}</p>{/if}
  </div>
</div>
