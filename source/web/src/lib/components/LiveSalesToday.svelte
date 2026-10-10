<script lang="ts">
  import { Tween } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';
  import { t, formatNumber, formatCurrency } from '#lib/i18n/index.ts';
  import { formatDateTime } from '#lib/i18n/index.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { LiveSales } from '#lib/live/sales.svelte.ts';

  const live = new LiveSales();

  // Outlet aktif berubah (atau komponen dibuka) → sambung ulang.
  $effect(() => {
    void session.outlet?.id;
    live.stop();
    live.start();
    return () => live.stop();
  });

  const rp = (n: number) => formatCurrency(n, 'IDR', { minimumFractionDigits: 0, maximumFractionDigits: 0 });
  const num = (s: string | undefined) => Number(s ?? 0);

  const d = $derived(live.data);
  const sales = new Tween(0, { duration: 700, easing: cubicOut });
  const receipts = new Tween(0, { duration: 500, easing: cubicOut });
  $effect(() => {
    sales.target = num(d?.sales.total);
    receipts.target = d?.sales.count ?? 0;
  });

  // Pembanding: kemarin sampai jam yang sama.
  const delta = $derived.by(() => {
    if (!d) return null;
    const y = num(d.yesterday.total);
    if (y <= 0) return { none: true as const };
    const pct = ((num(d.sales.total) - y) / y) * 100;
    return { none: false as const, pct, up: pct >= 0 };
  });

  // Grafik per jam: tampilkan jam kerja wajar, melebar bila ada penjualan di luar itu.
  const hourNow = $derived.by(() => {
    if (!d) return -1;
    return Number(new Intl.DateTimeFormat('en-GB', { hour: '2-digit', hour12: false, timeZone: d.timezone }).format(new Date(d.as_of))) % 24;
  });
  const bars = $derived.by(() => {
    if (!d) return [];
    const active = d.hours.filter((h) => h.count > 0).map((h) => h.hour);
    const from = Math.min(7, ...active);
    const to = Math.max(21, hourNow, ...active);
    const max = Math.max(1, ...d.hours.map((h) => num(h.total)));
    return d.hours.slice(from, to + 1).map((h) => ({ ...h, pct: (num(h.total) / max) * 100 }));
  });

  // Nota yang baru muncul (setelah muat pertama) berkedip sebentar.
  let seen = new Set<string>();
  let primed = false;
  let fresh = $state(new Set<string>());
  $effect(() => {
    if (!d) return;
    const incoming = d.recent.map((r) => r.id).filter((id) => !seen.has(id));
    d.recent.forEach((r) => seen.add(r.id));
    if (!primed) {
      primed = true;
      return;
    }
    if (incoming.length) {
      fresh = new Set(incoming);
      const timer = setTimeout(() => (fresh = new Set()), 2500);
      return () => clearTimeout(timer);
    }
  });

  const time = (iso: string) => formatDateTime(iso, { hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: d?.timezone });
  const statusKey = $derived(live.status === 'live' ? 'dashboard.live.statusLive' : live.status === 'offline' ? 'dashboard.live.statusOffline' : 'dashboard.live.statusConnecting');
</script>

<section class="space-y-4" aria-label={t('dashboard.live.title')}>
  <div class="flex flex-wrap items-center justify-between gap-2">
    <div class="flex items-center gap-2.5">
      <h2 class="font-display font-bold text-[15px]">{t('dashboard.live.title')}</h2>
      <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10.5px] font-bold tracking-wide {live.status === 'live' ? 'badge-solid-success' : live.status === 'offline' ? 'badge-solid-danger' : 'badge-solid-warning'}" role="status">
        <span class="size-1.5 rounded-full bg-current {live.status === 'live' ? 'live-dot' : ''}"></span>{t(statusKey)}
      </span>
    </div>
    <p class="text-[11.5px] u-color-text-tertiary">
      {t('dashboard.live.subtitle', { outlet: session.outlet?.name ?? '' })}
      {#if d}· {t('dashboard.live.updated', { time: time(d.as_of) })}{/if}
    </p>
  </div>

  {#if live.failed}
    <p class="text-[12px] px-3 py-2 rounded-lg bg-[var(--surface-sunken)] border border-[var(--border-subtle)]" role="alert">{t('dashboard.live.failed')}</p>
  {/if}

  <div class="bento-grid">
    <article class="surface-card col-span-12 lg:col-span-6 p-4">
      <h3 class="text-[11.5px] u-color-text-tertiary">{t('dashboard.live.sales')}</h3>
      <p class="font-display font-extrabold text-[32px] leading-none mt-1 tabular-nums">{rp(sales.current)}</p>
      <p class="text-[11.5px] mt-2 font-semibold {delta === null || delta.none ? 'u-color-text-tertiary' : delta.up ? 'text-[var(--color-success-600,#16a34a)]' : 'text-[var(--color-danger-600,#dc2626)]'}">
        {#if delta === null}&nbsp;
        {:else if delta.none}{t('dashboard.live.noBaseline')}
        {:else}{delta.up ? '▲' : '▼'} {formatNumber(Math.abs(delta.pct), { maximumFractionDigits: 1 })}% {t('dashboard.live.vsYesterday')}{/if}
      </p>
    </article>
    <article class="surface-card col-span-6 lg:col-span-2 p-4">
      <h3 class="text-[11.5px] u-color-text-tertiary">{t('dashboard.live.receipts')}</h3>
      <p class="font-display font-extrabold text-[24px] leading-none mt-1 tabular-nums">{formatNumber(Math.round(receipts.current))}</p>
    </article>
    <article class="surface-card col-span-6 lg:col-span-2 p-4">
      <h3 class="text-[11.5px] u-color-text-tertiary">{t('dashboard.live.average')}</h3>
      <p class="font-display font-extrabold text-[20px] leading-none mt-1 tabular-nums">{rp(num(d?.average))}</p>
    </article>
    <article class="surface-card col-span-12 lg:col-span-2 p-4">
      <h3 class="text-[11.5px] u-color-text-tertiary">{t('dashboard.live.returns')}</h3>
      <p class="font-display font-extrabold text-[20px] leading-none mt-1 tabular-nums">{rp(num(d?.returns.total))}</p>
      {#if d && d.returns.count > 0}<p class="text-[11px] mt-2 u-color-text-tertiary">{t('dashboard.live.net', { amount: rp(num(d.net)) })}</p>{/if}
    </article>

    <article class="surface-card col-span-12 lg:col-span-7 p-4">
      <h3 class="text-[12.5px] font-semibold mb-3">{t('dashboard.live.perHour')}</h3>
      <div class="flex items-end gap-1 h-40" role="img" aria-label={t('dashboard.live.perHour')}>
        {#each bars as b (b.hour)}
          <div class="flex-1 min-w-0 h-full flex flex-col justify-end items-center gap-1 group" title="{String(b.hour).padStart(2, '0')}:00 · {rp(num(b.total))} · {t('dashboard.live.receipts')} {b.count}">
            <div class="w-full rounded-t-sm transition-[height] duration-500 {b.hour === hourNow ? 'bg-[var(--color-primary-500)]' : 'bg-[var(--color-primary-300,#93c5fd)]'} {b.count === 0 ? 'opacity-25' : ''}" style="height:{Math.max(b.count > 0 ? 3 : 1, b.pct)}%"></div>
            <span class="text-[9.5px] tabular-nums {b.hour === hourNow ? 'font-bold' : 'u-color-text-tertiary'}">{String(b.hour).padStart(2, '0')}</span>
          </div>
        {/each}
      </div>
    </article>

    <article class="surface-card col-span-12 lg:col-span-5 p-4">
      <h3 class="text-[12.5px] font-semibold mb-3">{t('dashboard.live.recent')}</h3>
      {#if !d || d.recent.length === 0}
        <p class="text-[12.5px] u-color-text-tertiary py-6 text-center">{d ? t('dashboard.live.empty') : ''}</p>
      {:else}
        <ul class="divide-y divide-[var(--border-subtle)]">
          {#each d.recent as r (r.id)}
            <li class="flex items-center justify-between gap-3 py-2 px-1 rounded {fresh.has(r.id) ? 'live-fresh' : ''}">
              <div class="min-w-0">
                <p class="text-[12.5px] font-semibold truncate">{r.doc_no}</p>
                <p class="text-[11px] u-color-text-tertiary truncate">{time(r.created_at)} · {r.cashier} · {r.member || t('dashboard.live.walkIn')} · {t('dashboard.live.lines', { count: r.line_count })}</p>
              </div>
              <span class="text-[13px] font-bold tabular-nums shrink-0">{rp(num(r.total))}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </article>
  </div>
</section>

<style>
  .live-dot {
    animation: live-pulse 1.6s ease-in-out infinite;
  }
  .live-fresh {
    animation: live-flash 2.4s ease-out;
  }
  @keyframes live-pulse {
    50% {
      opacity: 0.3;
    }
  }
  @keyframes live-flash {
    from {
      background: color-mix(in srgb, var(--color-primary-500) 22%, transparent);
    }
    to {
      background: transparent;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .live-dot,
    .live-fresh {
      animation: none;
    }
  }
</style>
