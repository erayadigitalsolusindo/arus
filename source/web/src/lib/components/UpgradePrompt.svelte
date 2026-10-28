<script lang="ts">
  // Ajakan upgrade untuk menu yang tidak termasuk paket langganan: dialog (dari sidebar) atau panel penuh (URL dibuka langsung).
  import { goto } from '$app/navigation';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import Modal from '#lib/components/Modal.svelte';

  let { labelKey, panel = false, onclose }: { labelKey: MessageKey; panel?: boolean; onclose?: () => void } = $props();

  function upgrade() {
    onclose?.();
    void goto('/subscription');
  }
</script>

{#snippet body()}
  <div class="grid place-items-center text-center gap-3 py-6 px-4">
    <span class="grid place-items-center size-14 rounded-full bg-[var(--color-primary-50)] text-[var(--color-primary-600)] text-[26px]"><i class="icon-lock"></i></span>
    <h2 class="font-display font-bold text-[18px]">{t('subscription.lock.title', { menu: t(labelKey) })}</h2>
    <p class="text-[13px] text-[var(--text-tertiary)] max-w-sm">{t('subscription.lock.body')}</p>
    <div class="flex gap-2 mt-2">
      {#if onclose}<button type="button" class="btn btn-outline !text-[12.5px]" onclick={onclose}>{t('subscription.lock.later')}</button>{/if}
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={upgrade}><i class="icon-rocket text-[13px]"></i>{t('subscription.lock.upgrade')}</button>
    </div>
  </div>
{/snippet}

{#if panel}
  <div class="surface-card max-w-xl mx-auto mt-10">{@render body()}</div>
{:else}
  <Modal title={t('subscription.lock.badge')} onclose={() => onclose?.()}>{@render body()}</Modal>
{/if}
