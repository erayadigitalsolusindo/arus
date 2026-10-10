<script lang="ts">
  // Pengaturan struk per perangkat (PC kasir): cetak otomatis, lebar kertas, cara cetak, potong kertas, tes printer.
  import { onMount } from 'svelte';
  import { t } from '#lib/i18n/index.ts';
  import { agentStatus, loadReceiptSettings, printErrorMessage, printLines, saveReceiptSettings, testLines, type Paper, type PrintMethod, type ReceiptSettings } from '#lib/pos/receipt.ts';

  let { onchange }: { onchange?: (s: ReceiptSettings) => void } = $props();

  let s = $state(loadReceiptSettings());
  let agent = $state<{ version: string; printer: string } | null | undefined>(undefined);
  let testing = $state(false);
  let message = $state('');
  let failed = $state(false);

  async function check() {
    agent = undefined;
    agent = await agentStatus(s.agentUrl);
  }
  onMount(check);

  function set(next: Partial<ReceiptSettings>) {
    s = { ...s, ...next };
    saveReceiptSettings(s);
    onchange?.(s);
  }

  async function test() {
    testing = true;
    message = '';
    try {
      const via = await printLines(testLines(s.paper), s);
      failed = false;
      message = t('pos.receipt.testSent', { via: via === 'agent' ? t('pos.receipt.viaAgent') : t('pos.receipt.viaBrowser') });
    } catch (e) {
      failed = true;
      message = printErrorMessage(e);
    } finally {
      testing = false;
      void check();
    }
  }

  const methods: PrintMethod[] = ['auto', 'agent', 'browser'];
  const methodLabel = (m: PrintMethod) => (m === 'auto' ? t('pos.receipt.methodAuto') : m === 'agent' ? t('pos.receipt.methodAgent') : t('pos.receipt.methodBrowser'));
</script>

<div class="space-y-2.5 text-start text-[12px] text-[var(--text-secondary)]">
  <label class="flex items-center gap-2 cursor-pointer">
    <input type="checkbox" class="size-4 accent-[var(--color-primary-600)]" checked={s.auto} onchange={(e) => set({ auto: e.currentTarget.checked })} />
    {t('pos.receipt.auto')}
  </label>

  <fieldset class="space-y-1">
    <legend class="font-semibold mb-1">{t('pos.receipt.method')}</legend>
    {#each methods as m (m)}
      <label class="flex items-center gap-2 cursor-pointer">
        <input type="radio" name="receipt-method" class="accent-[var(--color-primary-600)]" checked={s.method === m} onchange={() => set({ method: m })} />
        {methodLabel(m)}
      </label>
    {/each}
  </fieldset>

  <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
    <span class="font-semibold">{t('pos.receipt.paper')}</span>
    {#each [58, 80] as p (p)}
      <label class="flex items-center gap-1 cursor-pointer">
        <input type="radio" name="receipt-paper" class="accent-[var(--color-primary-600)]" checked={s.paper === p} onchange={() => set({ paper: p as Paper })} />
        {p === 58 ? t('pos.receipt.paper58') : t('pos.receipt.paper80')}
      </label>
    {/each}
  </div>

  {#if s.method !== 'browser'}
    <label class="flex items-center gap-2 cursor-pointer">
      <input type="checkbox" class="size-4 accent-[var(--color-primary-600)]" checked={s.cut} onchange={(e) => set({ cut: e.currentTarget.checked })} />
      {t('pos.receipt.cut')}
    </label>
    <p class="flex items-center gap-1.5 {agent ? 'text-[var(--color-success-600)]' : 'text-[var(--text-tertiary)]'}">
      <i class="{agent ? 'icon-printer-check' : agent === null ? 'icon-printer-x' : 'icon-loader-circle animate-spin'} text-[13px]"></i>
      {agent ? t('pos.receipt.agentFound', { version: agent.version, printer: agent.printer }) : agent === null ? t('pos.receipt.agentNotFound') : t('pos.receipt.agentChecking')}
    </p>
  {/if}

  <button type="button" class="btn btn-outline btn-sm disabled:opacity-60" onclick={test} disabled={testing}>
    <i class="icon-printer text-[13px]"></i>{testing ? t('pos.receipt.printing') : t('pos.receipt.test')}
  </button>
  {#if message}<p role="status" class={failed ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}>{message}</p>{/if}
</div>
