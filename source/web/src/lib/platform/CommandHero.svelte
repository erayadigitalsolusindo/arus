<script lang="ts">
  import { onMount } from 'svelte';
  import { platform } from '#lib/platform/session.svelte.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { API_URL } from '#lib/api/client.ts';
  import MatrixRain from '#lib/platform/MatrixRain.svelte';

  let now = $state(new Date());
  let db = $state<'checking' | 'ok' | 'down'>('checking');
  async function check() {
    try {
      db = (await fetch(`${API_URL}/healthz`, { signal: AbortSignal.timeout(4000) })).ok ? 'ok' : 'down';
    } catch {
      db = 'down';
    }
  }
  onMount(() => {
    const id = setInterval(() => (now = new Date()), 1000);
    check();
    const hc = setInterval(check, 30000);
    return () => { clearInterval(id); clearInterval(hc); };
  });
  const dot = $derived(db === 'ok' ? 'bg-emerald-400' : db === 'down' ? 'bg-red-500' : 'bg-slate-400');

  const hour = $derived(now.getHours());
  const greeting = $derived(hour < 11 ? t('platform.hero.morning') : hour < 15 ? t('platform.hero.noon') : hour < 19 ? t('platform.hero.afternoon') : t('platform.hero.evening'));
</script>

<section class="relative isolate overflow-hidden rounded-2xl border border-[var(--border-subtle)] bg-gradient-to-br from-[#06122b] via-[#0a1f4a] to-[#0d2b63] text-white shadow-[var(--shadow-sm)]">
  <div class="absolute inset-0 -z-10 opacity-70"><MatrixRain /></div>
  <!-- vinyet agar teks tetap terbaca di atas hujan karakter -->
  <div class="absolute inset-0 -z-10 bg-[linear-gradient(90deg,rgb(6_18_43/0.92)_0%,rgb(6_18_43/0.55)_45%,rgb(6_18_43/0.1)_100%)]"></div>

  <div class="flex flex-wrap items-center justify-between gap-4 px-5 py-5 lg:px-7 lg:py-6">
    <div class="min-w-0">
      <span class="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/15 px-2.5 py-1 text-[10.5px] font-bold uppercase tracking-[0.14em] text-sky-200">
        <span class="relative flex size-2">
          {#if db === 'ok'}<span class="absolute inline-flex size-full animate-ping rounded-full {dot} opacity-75"></span>{/if}
          <span class="relative inline-flex size-2 rounded-full {dot}"></span>
        </span>
        {db === 'down' ? t('platform.hero.offline') : t('platform.hero.online')}
      </span>
      <h2 class="mt-2.5 font-display font-bold !text-white text-[22px] lg:text-[26px] leading-tight tracking-tight">{greeting}, {platform.admin?.name}</h2>
      <p class="mt-1 text-[13px] text-sky-100 max-w-xl">{t('platform.hero.subtitle')}</p>
    </div>
    <div class="text-end shrink-0">
      <p class="font-mono text-[28px] lg:text-[34px] font-semibold tabular-nums leading-none tracking-wider text-sky-100 [text-shadow:0_1px_2px_rgb(0_0_0/0.5)]">{formatDateTime(now, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}</p>
      <p class="mt-1.5 text-[11.5px] uppercase tracking-widest text-sky-200">{formatDateTime(now, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })}</p>
    </div>
  </div>
</section>
