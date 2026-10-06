<script lang="ts">
  import { onMount } from 'svelte';
  import { fly } from 'svelte/transition';
  import { flip } from 'svelte/animate';
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';

  // Ilustrasi dekoratif untuk halaman login — bukan data nyata.
  type Sale = { id: number; no: string; item: string; method: string; icon: string; amount: number };

  const samples: Omit<Sale, 'id'>[] = [
    { no: 'TRX-0001', item: 'Beras Premium 5 kg × 2', method: 'Tunai', icon: 'banknote', amount: 142000 },
    { no: 'TRX-0002', item: 'Minyak Goreng 2 L × 3', method: 'QRIS', icon: 'qr-code', amount: 78500 },
    { no: 'TRX-0003', item: 'Kopi Sachet × 12', method: 'Debit', icon: 'credit-card', amount: 36000 },
    { no: 'TRX-0004', item: 'Gula Pasir 1 kg × 5', method: 'Tunai', icon: 'banknote', amount: 82500 },
    { no: 'TRX-0005', item: 'Susu UHT 1 L × 6', method: 'E-Wallet', icon: 'wallet', amount: 114000 },
    { no: 'TRX-0006', item: 'Mie Instan × 20', method: 'Tunai', icon: 'banknote', amount: 64000 }
  ];

  const visible = 4;
  let counter = 0;
  const take = (): Sale => ({ id: counter, ...samples[counter++ % samples.length] });

  const initial = [take(), take(), take()].reverse();
  let feed = $state<Sale[]>(initial);
  const total = new Tween(initial.reduce((s, x) => s + x.amount, 0), { duration: 700, easing: cubicOut });
  let count = $state(initial.length);

  const rupiah = (n: number) => 'Rp ' + Math.round(n).toLocaleString('id-ID');

  onMount(() => {
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reduce) return;

    const timer = setInterval(() => {
      const sale = take();
      feed = [sale, ...feed].slice(0, visible);
      count += 1;
      total.target += sale.amount;
    }, 2400);
    return () => clearInterval(timer);
  });
</script>

<div class="surface-card !bg-white/10 !border-white/15 p-4 mt-8 text-white" aria-hidden="true">
  <div class="flex items-center justify-between">
    <div>
      <p class="text-[10.5px] uppercase tracking-wide font-semibold text-[rgb(255_255_255_/_0.6)]">Penjualan hari ini</p>
      <p class="font-display font-extrabold text-[22px] leading-tight mt-0.5">{rupiah(total.current)}</p>
    </div>
    <div class="text-end">
      <span class="inline-flex items-center gap-1.5 px-2 py-1 rounded-full text-[10.5px] font-semibold bg-[rgb(255_255_255_/_0.14)]">
        <span class="live-dot size-1.5 rounded-full bg-[var(--color-success-400,#4ADE80)]"></span>Live
      </span>
      <p class="text-[11px] mt-1 text-[rgb(255_255_255_/_0.65)]">{count} transaksi</p>
    </div>
  </div>

  <ul class="mt-3 space-y-2 h-[212px] overflow-hidden">
    {#each feed as sale (sale.id)}
      <li
        class="flex items-center gap-3 rounded-lg px-3 py-2.5 bg-[rgb(255_255_255_/_0.1)] border border-[rgb(255_255_255_/_0.12)]"
        in:fly={{ y: -24, duration: 450, easing: cubicOut }}
        animate:flip={{ duration: 450 }}
      >
        <span class="grid place-items-center size-8 rounded-lg shrink-0 bg-[rgb(255_255_255_/_0.16)]">
          <i class="icon-{sale.icon} text-[14px]"></i>
        </span>
        <span class="min-w-0 flex-1">
          <span class="block text-[12px] font-semibold truncate">{sale.item}</span>
          <span class="block text-[10.5px] text-[rgb(255_255_255_/_0.65)]">{sale.no} · {sale.method}</span>
        </span>
        <span class="text-end shrink-0">
          <span class="block text-[12px] font-bold">{rupiah(sale.amount)}</span>
          <span class="inline-flex items-center gap-1 text-[10px] font-semibold text-[var(--color-success-400,#4ADE80)]">
            <i class="icon-check text-[10px]"></i>Lunas
          </span>
        </span>
      </li>
    {/each}
  </ul>
</div>

<style>
  .live-dot {
    animation: live-pulse 1.6s ease-in-out infinite;
  }

  @keyframes live-pulse {
    0%,
    100% {
      opacity: 1;
      box-shadow: 0 0 0 0 rgb(74 222 128 / 0.55);
    }
    50% {
      opacity: 0.6;
      box-shadow: 0 0 0 5px rgb(74 222 128 / 0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .live-dot {
      animation: none;
    }
  }
</style>
