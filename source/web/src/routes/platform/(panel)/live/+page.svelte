<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type LiveSale, type LiveSummary } from '#lib/platform/api.ts';
  import { pstream } from '#lib/platform/session.svelte.ts';
  import { t, formatCurrency, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  const REFRESH_GAP_MS = 2000; // sinyal beruntun menyatu: paling sering sekali per 2 dtk
  const FALLBACK_MS = 30_000;
  const WATCHDOG_MS = 60_000;

  let summary = $state<LiveSummary | null>(null);
  let rows = $state<LiveSale[]>([]);
  let next = $state<{ at: string; id: string } | null>(null);
  let loading = $state(true);
  let more = $state(false);
  let error = $state('');
  let status = $state<'connecting' | 'live' | 'offline'>('connecting');
  let fresh = $state<Set<string>>(new Set());

  const money = (s: string) => formatCurrency(Number(s), { maximumFractionDigits: 0, minimumFractionDigits: 0 });
  const clock = (iso: string) => new Date(iso).toLocaleTimeString([], { hour12: false });
  const selling = $derived((summary?.online ?? []).filter((p) => p.selling));

  let lastRefresh = 0;
  // Memuat ringkasan + halaman pertama; nota baru ditaruh di atas baris yang sudah dimuat (halaman lama tidak dibuang).
  async function refresh() {
    lastRefresh = Date.now();
    try {
      const [s, f] = await Promise.all([platformApi.liveSummary(), platformApi.liveFeed()]);
      summary = s;
      error = '';
      if (rows.length === 0) {
        rows = f.items;
        next = f.next_at && f.next_id ? { at: f.next_at, id: f.next_id } : null;
      } else {
        const known = new Set(rows.map((r) => r.id));
        const add = f.items.filter((r) => !known.has(r.id));
        if (add.length) {
          rows = [...add, ...rows];
          fresh = new Set(add.map((r) => r.id));
          setTimeout(() => (fresh = new Set()), 2500);
        }
      }
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!next || more) return;
    more = true;
    try {
      const f = await platformApi.liveFeed(next);
      const known = new Set(rows.map((r) => r.id));
      rows = [...rows, ...f.items.filter((r) => !known.has(r.id))];
      next = f.next_at && f.next_id ? { at: f.next_at, id: f.next_id } : null;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      more = false;
    }
  }

  onMount(() => {
    let alive = true;
    let ctl = new AbortController();
    let pending: ReturnType<typeof setTimeout> | undefined;
    const schedule = () => {
      if (pending) return;
      pending = setTimeout(() => {
        pending = undefined;
        void refresh();
      }, Math.max(0, REFRESH_GAP_MS - (Date.now() - lastRefresh)));
    };
    void refresh();
    const fallback = setInterval(() => void refresh(), FALLBACK_MS);

    (async () => {
      let delay = 1000;
      while (alive) {
        let dog: ReturnType<typeof setTimeout> | undefined;
        const feed = () => {
          clearTimeout(dog);
          dog = setTimeout(() => ctl.abort(), WATCHDOG_MS);
        };
        try {
          feed();
          const res = await pstream('/platform/live/stream', ctl.signal);
          const reader = res.body!.pipeThrough(new TextDecoderStream()).getReader();
          let buf = '';
          for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            feed();
            buf += value;
            let i: number;
            while ((i = buf.indexOf('\n\n')) >= 0) {
              const block = buf.slice(0, i);
              buf = buf.slice(i + 2);
              if (block.includes('event: ready')) {
                status = 'live';
                delay = 1000;
                schedule();
              } else if (block.includes('event: sales')) schedule();
            }
          }
          delay = 1000; // ditutup server (batas umur aliran): sambung lagi
        } catch {
          if (!alive) return;
          status = 'offline';
          await new Promise((r) => setTimeout(r, delay));
          delay = Math.min(delay * 2, 30_000);
        } finally {
          clearTimeout(dog);
        }
        if (alive && ctl.signal.aborted) ctl = new AbortController();
      }
    })();

    return () => {
      alive = false;
      ctl.abort();
      clearTimeout(pending);
      clearInterval(fallback);
    };
  });
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
  <div>
    <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.live.title')}</h1>
    <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.live.sub')}</p>
  </div>
  <span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11.5px] font-semibold {status === 'live' ? 'badge-success' : status === 'offline' ? 'badge-danger' : 'badge-neutral'}" role="status">
    <span class="size-1.5 rounded-full bg-current {status === 'live' ? 'animate-pulse' : ''}"></span>
    {status === 'live' ? t('platform.live.connected') : status === 'offline' ? t('platform.live.offline') : '…'}
  </span>
</div>

<div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
  <StatCard label={t('platform.live.revenue')} value={summary ? money(summary.sales_total) : '—'} icon="banknote" tone="success" />
  <StatCard label={t('platform.live.count')} value={summary ? formatNumber(summary.sales_count) : '—'} icon="receipt" tone="primary" />
  <StatCard label={t('platform.live.tenants')} value={summary ? formatNumber(summary.tenant_count) : '—'} icon="store" tone="info" />
  <StatCard label={t('platform.live.online')} value={summary ? formatNumber(summary.online.length) : '—'} icon="users" tone="warning" hint={t('platform.live.selling') + ': ' + formatNumber(selling.length)} />
</div>

{#if error}
  <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
{/if}

<div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px] items-start">
  <div class="surface-card !p-0 overflow-hidden">
    <div class="p-3 sm:p-4 border-b border-[var(--border-subtle)] font-semibold text-[13px]">{t('platform.live.feed')}</div>
    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.live.time')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.live.tenant')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.live.outlet')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.live.doc')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('platform.live.cashier')}</th>
            <th class="px-4 py-2.5 text-end font-semibold" scope="col">{t('platform.live.items')}</th>
            <th class="px-4 py-2.5 text-end font-semibold" scope="col">{t('platform.live.total')}</th>
          </tr>
        </thead>
        <tbody>
          {#if loading}
            {#each [0, 1, 2, 3, 4] as i (i)}
              <tr class="border-t border-[var(--border-subtle)]" aria-hidden="true"><td class="px-4 py-3" colspan="7"><div class="h-7 rounded-lg bg-[var(--surface-sunken)] animate-pulse"></div></td></tr>
            {/each}
          {:else if rows.length === 0}
            <tr><td colspan="7" class="px-4 py-10 text-center text-[var(--text-tertiary)]">{t('platform.live.empty')}</td></tr>
          {:else}
            {#each rows as r (r.id)}
              <tr class="border-t border-[var(--border-subtle)] transition-colors duration-700 {fresh.has(r.id) ? 'bg-[var(--color-primary-50)]' : ''}">
                <td class="px-4 py-2.5 font-mono tabular-nums text-[var(--text-tertiary)]">{clock(r.created_at)}</td>
                <td class="px-4 py-2.5 font-semibold"><a href="/platform/tenants/{r.tenant_id}" class="text-[var(--color-primary-700)] hover:underline">{r.tenant_name}</a></td>
                <td class="px-4 py-2.5">{r.outlet_name}</td>
                <td class="px-4 py-2.5 font-mono">{r.doc_no}</td>
                <td class="px-4 py-2.5">{r.cashier || '—'}</td>
                <td class="px-4 py-2.5 text-end tabular-nums">{r.line_count}</td>
                <td class="px-4 py-2.5 text-end tabular-nums font-semibold">{money(r.total)}</td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
    {#if next}
      <div class="p-3 border-t border-[var(--border-subtle)] text-center">
        <button type="button" class="btn btn-secondary btn-sm" onclick={loadMore} disabled={more}>{t('platform.live.more')}</button>
      </div>
    {/if}
  </div>

  <div class="surface-card !p-0 overflow-hidden">
    <div class="p-3 sm:p-4 border-b border-[var(--border-subtle)] font-semibold text-[13px]">{t('platform.live.people')}</div>
    {#if !summary || summary.online.length === 0}
      <p class="px-4 py-8 text-center text-[12.5px] text-[var(--text-tertiary)]">{t('platform.live.peopleEmpty')}</p>
    {:else}
      <ul class="divide-y divide-[var(--border-subtle)] max-h-[560px] overflow-y-auto scroll-thin">
        {#each summary.online as p (p.tenant_id + p.user_id)}
          <li class="px-4 py-2.5 flex items-center gap-3">
            <span class="size-2 rounded-full shrink-0 {p.selling ? 'bg-[var(--color-success-500,#22c55e)] animate-pulse' : 'bg-[var(--text-tertiary)]'}"></span>
            <div class="min-w-0 flex-1">
              <div class="font-semibold truncate">{p.user_name}</div>
              <div class="text-[11.5px] text-[var(--text-tertiary)] truncate">{p.tenant_name} · {t('platform.live.seen', { t: relativeTime(p.seen_at) })}</div>
            </div>
            <span class="text-[11px] font-semibold rounded-full px-2 py-0.5 {p.selling ? 'badge-success' : 'badge-neutral'}">{p.selling ? t('platform.live.selling') : t('platform.live.idle')}</span>
          </li>
        {/each}
      </ul>
    {/if}
    <p class="px-4 py-2.5 border-t border-[var(--border-subtle)] text-[11px] text-[var(--text-tertiary)]">{t('platform.live.window')}</p>
  </div>
</div>
