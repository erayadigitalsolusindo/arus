<script lang="ts">
  import { onMount } from 'svelte';
  import { invalidateAll } from '$app/navigation';
  import { session, switchOutlet } from '#lib/auth/session.svelte.ts';
  import type { Outlet } from '#lib/outlets/api.ts';
  import { accessibleOutlets, outletScope, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import Select from '#lib/components/Select.svelte';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let busy = $state(false);
  let error = $state('');

  onMount(refreshOutlets);
  const list = $derived(accessibleOutlets.items);

  // Pilihan diambil dari sesi; daftar dimuat ulang bila outlet berubah (mis. outlet baru dibuat di tab lain).
  const options = $derived(list.length ? list : session.outlet ? [{ id: session.outlet.id, name: session.outlet.name } as Outlet] : []);

  const ALL = '__all__';
  const showAll = $derived(outletScope.supported && options.length > 1);
  // Pilihan tampil mengikuti sesi/mode; di-reset ke sini bila perpindahan gagal.
  let selected = $state(outletScope.all ? ALL : (session.outlet?.id ?? ''));
  $effect(() => {
    selected = outletScope.all ? ALL : (session.outlet?.id ?? '');
  });
  const selectOptions = $derived([...(showAll ? [{ value: ALL, label: t('shell.allOutlets') }] : []), ...options.map((o) => ({ value: o.id, label: o.name }))]);

  async function change(id: string) {
    if (id === ALL) {
      outletScope.setAll(true);
      return;
    }
    if (!id) return;
    outletScope.setAll(false);
    if (id === session.outlet?.id) return;
    busy = true;
    error = '';
    try {
      await switchOutlet(id);
      await invalidateAll(); // data halaman bergantung pada outlet aktif
    } catch (err) {
      error = errorMessage(err);
      selected = outletScope.all ? ALL : (session.outlet?.id ?? ''); // kembalikan pilihan yang valid
    } finally {
      busy = false;
    }
  }
</script>

<div class="sidebar-outlet workspace-text px-4 pt-3 pb-1 shrink-0">
  <span class="block text-[11px] font-semibold uppercase tracking-wide text-[var(--sidebar-text)]">
    {t('shell.currentOutlet')} [{outletScope.all ? t('shell.allOutletsShort') : session.outlet?.code}]
  </span>
  <div class="mt-1.5">
    <Select variant="sidebar" ariaLabel={t('shell.switchOutlet')} disabled={busy || options.length < 2} bind:value={selected} onchange={change} options={selectOptions} />
  </div>
  {#if error}<p role="alert" class="text-[11px] mt-1 text-[var(--color-danger-400)]">{error}</p>{/if}
</div>
