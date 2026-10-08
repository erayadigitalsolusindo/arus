<script lang="ts">
  let { ok, label }: { ok: boolean; label: string } = $props();
</script>

<span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-semibold {ok ? 'badge-success' : 'badge-danger'}">
  <span class="size-1.5 rounded-full bg-current"></span>{label}
</span>
