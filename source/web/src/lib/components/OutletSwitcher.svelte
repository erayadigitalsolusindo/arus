<script lang="ts">
  import { onMount } from 'svelte';
  import { invalidateAll } from '$app/navigation';
  import { session, switchOutlet } from '#lib/auth/session.svelte.ts';
  import type { Outlet } from '#lib/outlets/api.ts';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let busy = $state(false);
  let error = $state('');

  onMount(refreshOutlets);
  const list = $derived(accessibleOutlets.items);

  // Pilihan diambil dari sesi; daftar dimuat ulang bila outlet berubah (mis. outlet baru dibuat di tab lain).
  const options = $derived(list.length ? list : session.outlet ? [{ id: session.outlet.id, name: session.outlet.name } as Outlet] : []);

  async function change(e: Event & { currentTarget: HTMLSelectElement }) {
    const el = e.currentTarget;
    const id = el.value;
    if (!id || id === session.outlet?.id) return;
    busy = true;
    error = '';
    try {
      await switchOutlet(id);
      await invalidateAll(); // data halaman bergantung pada outlet aktif
    } catch (err) {
      error = errorMessage(err);
      el.value = session.outlet?.id ?? ''; // kembalikan pilihan yang valid
    } finally {
      busy = false;
    }
  }
</script>

<div class="sidebar-outlet workspace-text px-4 pt-3 pb-1 shrink-0">
  <span class="block text-[11px] font-semibold uppercase tracking-wide text-[var(--sidebar-text)]">
    {t('shell.currentOutlet')} [{session.outlet?.code}]
  </span>
  <!-- Latar gelap lewat style inline !important: template memaksa `select { background: white !important }`. -->
  <select
    class="mt-1.5 w-full rounded-lg px-2.5 py-2 text-[12.5px] font-medium outline-none cursor-pointer disabled:opacity-60"
    style="background-color: rgb(255 255 255 / 0.08) !important; color: #fff; border: 1px solid var(--sidebar-border); color-scheme: dark"
    aria-label={t('shell.switchOutlet')}
    disabled={busy || options.length < 2}
    value={session.outlet?.id}
    onchange={change}
  >
    {#each options as o (o.id)}<option class="text-black" value={o.id}>{o.name}</option>{/each}
  </select>
  {#if error}<p role="alert" class="text-[11px] mt-1 text-[var(--color-danger-400)]">{error}</p>{/if}
</div>
