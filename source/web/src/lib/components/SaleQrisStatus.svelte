<script lang="ts">
  // Status tagihan QRIS EraPay pada detail nota: cek ke EraPay (server memutakhirkan bila masih menunggu) dan tampilkan QR
  // selama belum lunas dan belum kedaluwarsa, supaya pelanggan masih bisa membayar.
  import { onMount } from 'svelte';
  import { erapay, eraFinal, type EraPayCharge } from '#lib/pos/erapay.ts';
  import { t } from '#lib/i18n/index.ts';

  let { chargeId, amountLabel }: { chargeId: string; amountLabel: string } = $props();

  let charge = $state<EraPayCharge | null>(null);
  let qr = $state('');
  let busy = $state(false);
  let failed = $state(false);
  let timer: ReturnType<typeof setInterval> | undefined;

  const expired = $derived(!!charge?.expires_at && new Date(charge.expires_at).getTime() <= Date.now());
  const waiting = $derived(!!charge && !eraFinal(charge.status) && !expired);

  async function check() {
    if (busy) return;
    busy = true;
    failed = false;
    try {
      charge = await erapay.get(chargeId);
      const v = charge.qr_value;
      qr = v && !eraFinal(charge.status) ? await (await import('qrcode')).toDataURL(v, { margin: 1, width: 240 }) : '';
    } catch {
      failed = true;
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    void check();
    // Selama menunggu bayar, cek ulang tiap 5 detik; berhenti begitu status akhir/kedaluwarsa.
    timer = setInterval(() => {
      if (waiting) void check();
    }, 5000);
    return () => clearInterval(timer);
  });

  const tone = $derived(
    charge?.status === 'succeeded' ? 'badge-success' : charge && (charge.status === 'failed' || charge.status === 'cancelled' || expired) ? 'badge-danger' : 'badge-warning'
  );
  const label = $derived.by(() => {
    if (!charge) return '';
    if (charge.status === 'succeeded') return t('pos.erapay.paid');
    if (charge.status === 'failed') return t('sales.detail.payments.qrisFailed');
    if (charge.status === 'cancelled') return t('sales.detail.payments.qrisCancelled');
    if (charge.status === 'expired' || expired) return t('sales.detail.payments.qrisExpired');
    return t('sales.detail.payments.qrisWaiting');
  });
</script>

<div class="mt-1.5 text-[11.5px]">
  <div class="flex flex-wrap items-center gap-2">
    {#if charge}<span class="badge-soft {tone}">{label}</span>{/if}
    {#if failed}<span class="text-[var(--color-danger-600)]">{t('sales.detail.payments.qrisCheckFailed')}</span>{/if}
    <button type="button" class="text-[var(--color-primary-600)] hover:underline disabled:opacity-50" disabled={busy} onclick={check}>
      {busy ? t('sales.detail.payments.qrisChecking') : t('sales.detail.payments.qrisCheck')}
    </button>
  </div>
  {#if qr && waiting}
    <div class="mt-2 inline-block rounded-lg border border-[var(--border-subtle)] bg-white p-2 text-center">
      <img src={qr} alt={t('pos.erapay.qrAlt')} class="mx-auto size-[200px]" />
      <div class="mt-1 text-[var(--text-secondary)]">{t('pos.erapay.scan', { amount: amountLabel })}</div>
    </div>
  {/if}
</div>
