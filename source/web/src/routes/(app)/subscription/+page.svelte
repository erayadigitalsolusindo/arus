<script lang="ts">
  import { onMount } from 'svelte';
  import { fly, fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { subscription, payFinal, type SubscriptionStatus, type PlanInfo, type PlanPayment } from '#lib/subscription/api.ts';
  import { session } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { featureGroups, planHas } from '#lib/subscription/features.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';

  // Harga paket selalu Rupiah (tagihan nyata), tidak mengikuti simbol mata uang bahasa.
  const rp = (n: number) => 'Rp ' + formatNumber(n, { maximumFractionDigits: 0 });

  let status = $state<SubscriptionStatus | null>(null);
  let loading = $state(true);
  let error = $state('');

  // Tagihan yang sedang dibayar (dialog QRIS).
  let paying = $state<PlanPayment | null>(null);
  let qr = $state('');
  let creating = $state<string | null>(null);
  let checking = $state(false);
  let timer: ReturnType<typeof setInterval> | undefined;
  let key = '';

  const isOwner = $derived(!!session.permissions?.['*']);
  const waiting = $derived(!!paying && !payFinal(paying.status) && !(paying.expires_at && new Date(paying.expires_at).getTime() <= Date.now()));
  const cur = $derived(status?.current);

  // Animasi hitung naik untuk angka pemakaian (dimatikan bila pengguna meminta gerak berkurang).
  const reduce = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  const dur = (ms: number) => (reduce ? 0 : ms);

  let shownTx = $state(0);
  let shownUsers = $state(0);
  let meterReady = $state(false);
  function countTo(setter: (n: number) => void, to: number, ms = 900) {
    if (reduce || to === 0) return setter(to);
    const start = performance.now();
    const step = (now: number) => {
      const p = Math.min(1, (now - start) / ms);
      setter(Math.round(to * (1 - Math.pow(1 - p, 3))));
      if (p < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  async function load() {
    try {
      status = await subscription.status();
      countTo((n) => (shownTx = n), status.usage.tx_today);
      countTo((n) => (shownUsers = n), status.usage.users);
      requestAnimationFrame(() => (meterReady = true)); // lebar bar bertransisi dari 0 ke nilai akhir
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    return () => clearInterval(timer);
  });

  const pct = (used: number, max: number | null) => (max === null ? 0 : Math.min(100, Math.round((used / max) * 100)));
  const tone = (used: number, max: number | null) => {
    const p = pct(used, max);
    return p >= 100 ? 'danger' : p >= 80 ? 'warning' : 'primary';
  };
  const toneColor = { danger: 'var(--color-danger-500)', warning: 'var(--color-warning-500)', primary: 'var(--color-primary-500)' } as const;

  const meters = $derived(
    status && cur
      ? [
          { id: 'tx', icon: 'receipt', label: t('subscription.txToday'), used: status.usage.tx_today, shown: shownTx, max: cur.max_tx_per_day, unit: t('subscription.unitTx') },
          { id: 'users', icon: 'users', label: t('subscription.activeUsers'), used: status.usage.users, shown: shownUsers, max: cur.max_users, unit: t('subscription.unitUsers') }
        ]
      : []
  );

  const daysLeft = $derived(cur?.expires_at ? Math.ceil((new Date(cur.expires_at).getTime() - Date.now()) / 86_400_000) : null);
  const expiringSoon = $derived(daysLeft !== null && daysLeft <= 7);

  const limitLabel = (n: number | null, unit: 'tx' | 'users') =>
    n === null ? t('subscription.unlimited') : t(unit === 'tx' ? 'subscription.txLimit' : 'subscription.userLimit', { count: formatNumber(n) });
  // Selisih batas paket dibanding paket aktif ("naik dari 100 jadi 300"), hanya untuk paket lain.
  function gain(p: PlanInfo, unit: 'tx' | 'users') {
    const from = unit === 'tx' ? cur?.max_tx_per_day : cur?.max_users;
    const to = unit === 'tx' ? p.max_tx_per_day : p.max_users;
    if (!cur || p.id === cur.plan_id || from == null || to == null || to <= from) return '';
    return t('subscription.gain', { from: formatNumber(from), to: formatNumber(to) });
  }
  const featName = (id: string) => tryT(`subscription.features.${id}.name`) ?? id;
  const featDesc = (id: string) => tryT(`subscription.features.${id}.desc`) ?? '';
  const menuName = (id: string) => tryT(`iam.module.${id}`) ?? id;
  // Menu yang bisa dibuka per paket (tanpa modul pendukung teknis seperti PIN penyetuju), untuk daftar "lihat semua menu".
  const menusOf = (modules: string[] | null) => (modules ?? []).filter((m) => m !== 'subscription');
  let openMenus = $state<Record<string, boolean>>({});
  const recommended = $derived(status?.plans.find((p) => p.id !== cur?.plan_id)?.id ?? status?.plans[0]?.id);

  async function setPayment(p: PlanPayment) {
    paying = p;
    qr = p.qr_value && !payFinal(p.status) ? await (await import('qrcode')).toDataURL(p.qr_value, { margin: 1, width: 240 }) : '';
    if (p.status === 'succeeded' && p.applied) {
      clearInterval(timer);
      void load();
    }
  }

  async function buy(plan: PlanInfo) {
    if (creating) return;
    creating = plan.id;
    error = '';
    key = 'plan-' + crypto.randomUUID();
    try {
      await setPayment(await subscription.checkout(plan.id, key));
      clearInterval(timer);
      timer = setInterval(() => {
        if (waiting) void check();
      }, 4000);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      creating = null;
    }
  }

  async function check() {
    if (!paying || checking) return;
    checking = true;
    try {
      await setPayment(await subscription.payment(paying.id));
    } catch (err) {
      error = errorMessage(err);
    } finally {
      checking = false;
    }
  }

  function closePay() {
    clearInterval(timer);
    paying = null;
    qr = '';
    meterReady = false;
    void load();
  }

  const steps = [
    { icon: 'mouse-pointer-click', title: 'subscription.how.s1t', text: 'subscription.how.s1d' },
    { icon: 'qr-code', title: 'subscription.how.s2t', text: 'subscription.how.s2d' },
    { icon: 'circle-check', title: 'subscription.how.s3t', text: 'subscription.how.s3d' }
  ] as const;
  const notes = ['subscription.notes.n1', 'subscription.notes.n2', 'subscription.notes.n3', 'subscription.notes.n4'] as const;
</script>

<svelte:head><title>{t('subscription.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('subscription.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('subscription.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-6 max-w-[1600px] mx-auto w-full">
  <div in:fade={{ duration: dur(250) }}>
    <h1 class="font-display font-bold text-[22px] tracking-tight">{t('subscription.title')}</h1>
    <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)]">{t('subscription.subtitle')}</p>
  </div>

  {#if error}
    <div transition:fly={{ y: -8, duration: dur(200) }} class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]" role="alert">{error}</div>
  {/if}

  {#if loading}
    <div class="grid lg:grid-cols-3 gap-5" aria-busy="true">
      <div class="lg:col-span-1 h-52 rounded-2xl bg-[var(--surface-base)] animate-pulse"></div>
      <div class="lg:col-span-2 h-52 rounded-2xl bg-[var(--surface-base)] animate-pulse"></div>
    </div>
  {:else if status && cur}
    <!-- Ringkasan: paket aktif + pemakaian -->
    <section class="grid lg:grid-cols-3 gap-5" in:fly={{ y: 14, duration: dur(450), easing: cubicOut }}>
      <div class="hero rounded-2xl p-6 sm:p-7 text-white relative overflow-hidden flex flex-col justify-between gap-6">
        <span class="hero-orb hero-orb-a" aria-hidden="true"></span>
        <span class="hero-orb hero-orb-b" aria-hidden="true"></span>
        <div class="relative flex items-start justify-between gap-3">
          <div>
            <span class="block text-[11px] font-semibold uppercase tracking-[0.08em] text-white/70">{t('subscription.current')}</span>
            <span class="font-display font-bold text-[28px] leading-tight">{cur.name}</span>
          </div>
          <span class="grid size-10 place-items-center rounded-xl bg-white/15 backdrop-blur"><i class="icon-{cur.is_default ? 'sprout' : 'crown'} text-[20px]"></i></span>
        </div>
        <div class="relative space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <span class="inline-flex items-center gap-1.5 rounded-full bg-white/15 px-2.5 py-1 text-[11.5px] font-semibold">
              <span class="size-1.5 rounded-full {expiringSoon ? 'bg-amber-300' : 'bg-emerald-300'} pulse-dot"></span>
              {cur.is_default ? t('subscription.statusFree') : expiringSoon ? t('subscription.statusExpiring') : t('subscription.statusActive')}
            </span>
            {#if daysLeft !== null && daysLeft > 0}<span class="text-[12px] text-white/80">{t('subscription.daysLeft', { count: formatNumber(daysLeft) })}</span>{/if}
          </div>
          <p class="text-[12px] text-white/75">
            {#if cur.expires_at}{t('subscription.expiresOn', { date: formatDateTime(cur.expires_at, { dateStyle: 'medium', timeStyle: 'short' }) })}{:else if cur.is_default}{t('subscription.freeNote')}{:else}{t('subscription.noExpiry')}{/if}
          </p>
        </div>
      </div>

      <div class="lg:col-span-2 surface-card p-6 sm:p-7">
        <div class="flex items-center justify-between mb-4">
          <h2 class="font-display font-bold text-[15px]">{t('subscription.usage')}</h2>
          <span class="text-[11.5px] text-[var(--text-tertiary)]"><i class="icon-clock me-1"></i>{t('subscription.resetNote')}</span>
        </div>
        <div class="grid sm:grid-cols-2 gap-6">
          {#each meters as m (m.id)}
            {@const p = pct(m.used, m.max)}
            {@const tn = tone(m.used, m.max)}
            <div class="rounded-xl border border-[var(--border-subtle)] p-5">
              <div class="flex items-center gap-2 text-[12.5px] text-[var(--text-tertiary)]">
                <span class="grid size-7 place-items-center rounded-lg" style="background: color-mix(in srgb, {toneColor[tn]} 14%, transparent); color: {toneColor[tn]}"><i class="icon-{m.icon} text-[14px]"></i></span>
                {m.label}
              </div>
              <div class="mt-3 flex items-baseline gap-1.5">
                <span class="font-display font-bold text-[30px] tabular-nums leading-none">{formatNumber(m.shown)}</span>
                <span class="text-[13px] text-[var(--text-tertiary)]">/ {m.max === null ? t('subscription.unlimited') : formatNumber(m.max)}</span>
              </div>
              <div class="mt-3 h-2 rounded-full bg-[var(--surface-sunken)] overflow-hidden" role="progressbar" aria-valuenow={p} aria-valuemin="0" aria-valuemax="100" aria-label={m.label}>
                <div class="meter-fill h-full rounded-full" style="width: {meterReady ? (m.max === null ? 100 : Math.max(p, m.used > 0 ? 3 : 0)) : 0}%; background: {m.max === null ? 'var(--color-success-500)' : toneColor[tn]}; opacity: {m.max === null ? 0.35 : 1}"></div>
              </div>
              <p class="mt-2 text-[11.5px] min-h-4 {tn === 'danger' ? 'text-[var(--color-danger-600)] font-semibold' : tn === 'warning' ? 'text-[var(--color-warning-600)]' : 'text-[var(--text-tertiary)]'}">
                {#if m.max === null}{t('subscription.meterUnlimited')}
                {:else if p >= 100}<i class="icon-triangle-alert me-1"></i>{t('subscription.meterFull')}
                {:else if p >= 80}<i class="icon-circle-alert me-1"></i>{t('subscription.meterNear', { left: formatNumber(m.max - m.used), unit: m.unit })}
                {:else}{t('subscription.meterLeft', { left: formatNumber(m.max - m.used), unit: m.unit })}{/if}
              </p>
            </div>
          {/each}
        </div>
      </div>
    </section>

    <!-- Isi paket aktif -->
    <section class="surface-card p-6 sm:p-7" in:fly={{ y: 14, duration: dur(450), delay: dur(100), easing: cubicOut }}>
      <div class="mb-4">
        <h2 class="font-display font-bold text-[15px]">{t('subscription.yourFeatures')} <span class="text-[var(--color-primary-600)]">{cur.name}</span></h2>
        <p class="text-[12px] mt-1 text-[var(--text-tertiary)]">{t('subscription.yourFeaturesHint')}</p>
      </div>
      <ul class="grid sm:grid-cols-2 xl:grid-cols-3 gap-3">
        {#each featureGroups as g (g.id)}
          {@const has = planHas(cur.modules, g)}
          <li class="flex items-start gap-3 rounded-xl border border-[var(--border-subtle)] p-3.5 {has ? '' : 'opacity-55'}">
            <span class="grid size-9 shrink-0 place-items-center rounded-lg {has ? 'bg-[var(--color-success-500)]/12 text-[var(--color-success-600)]' : 'bg-[var(--surface-sunken)] text-[var(--text-tertiary)]'}"><i class="icon-{has ? g.icon : 'lock'} text-[16px]"></i></span>
            <span class="min-w-0">
              <span class="block text-[13px] font-semibold">{featName(g.id)}</span>
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{has ? featDesc(g.id) : t('subscription.notIncluded')}</span>
            </span>
          </li>
        {/each}
      </ul>
    </section>

    <!-- Pilihan paket -->
    <section class="space-y-5">
      <div class="flex flex-wrap items-end justify-between gap-2">
        <h2 class="font-display font-bold text-[16px]">{t('subscription.upgrade')}</h2>
        {#if !status.payments_enabled}
          <span class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-warning-500)]/10 px-3 py-1.5 text-[12px] text-[var(--color-warning-600)]"><i class="icon-info"></i>{t('subscription.paymentsOff')}</span>
        {:else if !isOwner}
          <span class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-info-500)]/10 px-3 py-1.5 text-[12px] text-[var(--color-info-600)]"><i class="icon-lock"></i>{t('subscription.ownerOnly')}</span>
        {/if}
      </div>

      {#if status.plans.length === 0}
        <p class="surface-card p-8 text-center text-[13px] text-[var(--text-tertiary)]">{t('subscription.noPlans')}</p>
      {:else}
        <div class="grid sm:grid-cols-2 xl:grid-cols-3 gap-6 pt-3">
          {#each status.plans as p, i (p.id)}
            {@const mine = p.id === cur.plan_id}
            {@const star = !mine && p.id === recommended}
            <article
              class="plan-card surface-card p-6 sm:p-7 flex flex-col gap-5 relative {mine ? 'is-mine' : ''} {star ? 'is-star' : ''}"
              in:fly={{ y: 18, duration: dur(450), delay: dur(120 + i * 90), easing: cubicOut }}
            >
              {#if mine || star}
                <span class="absolute -top-2.5 start-6 rounded-full px-2.5 py-0.5 text-[10.5px] font-bold uppercase tracking-wide text-white {mine ? 'bg-[var(--color-success-600)]' : 'bg-[var(--color-primary-600)]'}">
                  {mine ? t('subscription.badgeCurrent') : t('subscription.badgeRecommended')}
                </span>
              {/if}
              <div>
                <h3 class="font-display font-bold text-[18px]">{p.name}</h3>
                {#if p.description}<p class="text-[12px] mt-1 text-[var(--text-tertiary)]">{p.description}</p>{/if}
              </div>
              <div class="flex items-baseline gap-1.5">
                <span class="font-display font-bold text-[30px] tabular-nums tracking-tight">{rp(Number(p.price))}</span>
                {#if p.duration_days}<span class="text-[12.5px] text-[var(--text-tertiary)]">{t('subscription.perDuration', { count: formatNumber(p.duration_days) })}</span>{/if}
              </div>
              <ul class="text-[13px] space-y-2.5 grow">
                {#each [{ unit: 'tx', icon: 'receipt', text: limitLabel(p.max_tx_per_day, 'tx') }, { unit: 'users', icon: 'users', text: limitLabel(p.max_users, 'users') }] as f (f.unit)}
                  {@const g = gain(p, f.unit as 'tx' | 'users')}
                  <li class="flex items-start gap-2.5">
                    <span class="grid size-6 shrink-0 place-items-center rounded-full bg-[var(--color-success-500)]/12 text-[var(--color-success-600)]"><i class="icon-check text-[13px]"></i></span>
                    <span>
                      <span class="font-medium">{f.text}</span>
                      {#if g}<span class="block text-[11.5px] text-[var(--color-success-600)]"><i class="icon-trending-up me-1"></i>{g}</span>{/if}
                    </span>
                  </li>
                {/each}
                <li class="flex items-start gap-2.5 text-[var(--text-secondary)]">
                  <span class="grid size-6 shrink-0 place-items-center rounded-full bg-[var(--surface-sunken)]"><i class="icon-qr-code text-[13px]"></i></span>
                  <span>{t('subscription.featureQris')}</span>
                </li>
              </ul>
              <div class="border-t border-[var(--border-subtle)] pt-4 space-y-2.5">
                <div class="text-[11px] font-bold uppercase tracking-wide text-[var(--text-tertiary)]">{t('subscription.featuresTitle')}</div>
                <ul class="space-y-2">
                  {#each featureGroups as g (g.id)}
                    {@const has = planHas(p.modules, g)}
                    <li class="flex items-start gap-2 text-[12.5px] {has ? '' : 'text-[var(--text-tertiary)] opacity-60'}">
                      <i class="icon-{has ? 'circle-check' : 'circle-minus'} mt-0.5 text-[15px] shrink-0 {has ? 'text-[var(--color-success-600)]' : ''}" aria-hidden="true"></i>
                      <span>
                        <span class="font-medium">{featName(g.id)}</span>
                        {#if has}<span class="block text-[11.5px] text-[var(--text-tertiary)]">{featDesc(g.id)}</span>{/if}
                      </span>
                    </li>
                  {/each}
                </ul>
                {#if p.highlights.length}
                  <ul class="space-y-1.5 pt-1">
                    {#each p.highlights as h, i (i)}
                      <li class="flex items-start gap-2 text-[12.5px] font-medium"><i class="icon-sparkles mt-0.5 text-[14px] shrink-0 text-[var(--color-primary-600)]" aria-hidden="true"></i>{h}</li>
                    {/each}
                  </ul>
                {/if}
                {#if p.modules === null}
                  <p class="text-[11.5px] font-medium text-[var(--color-success-600)]"><i class="icon-infinity me-1"></i>{t('subscription.allMenus')}</p>
                {:else}
                  <button type="button" class="text-[11.5px] font-medium text-[var(--color-primary-600)]" aria-expanded={!!openMenus[p.id]} onclick={() => (openMenus[p.id] = !openMenus[p.id])}>
                    {openMenus[p.id] ? t('subscription.hideMenus') : t('subscription.seeMenus', { count: menusOf(p.modules).length })}
                  </button>
                  {#if openMenus[p.id]}
                    <div class="flex flex-wrap gap-1.5">
                      {#each menusOf(p.modules) as m (m)}<span class="rounded-full bg-[var(--surface-sunken)] px-2 py-0.5 text-[11px]">{menuName(m)}</span>{/each}
                    </div>
                  {/if}
                {/if}
              </div>
              <button
                type="button"
                class="btn {star || mine ? 'btn-primary' : 'btn-outline'} !text-[13px] !py-2.5 disabled:opacity-60 buy-btn"
                disabled={!status.payments_enabled || !isOwner || creating !== null}
                onclick={() => buy(p)}
              >
                {#if creating === p.id}
                  <span class="spinner" aria-hidden="true"></span>{t('subscription.creating')}
                {:else}
                  <i class="icon-qr-code text-[15px]"></i>{mine ? t('subscription.renew') : t('subscription.buy')}
                {/if}
              </button>
            </article>
          {/each}
        </div>
      {/if}
    </section>

    <!-- Cara kerja + catatan -->
    <section class="grid lg:grid-cols-5 gap-5" in:fly={{ y: 14, duration: dur(450), delay: dur(300), easing: cubicOut }}>
      <div class="lg:col-span-3 surface-card p-6 sm:p-7">
        <h2 class="font-display font-bold text-[15px] mb-4">{t('subscription.how.title')}</h2>
        <ol class="grid sm:grid-cols-3 gap-6">
          {#each steps as s, i (s.icon)}
            <li class="relative">
              <div class="flex items-center gap-2.5">
                <span class="grid size-9 place-items-center rounded-xl bg-[var(--color-primary-50)] text-[var(--color-primary-600)]"><i class="icon-{s.icon} text-[18px]"></i></span>
                <span class="text-[11px] font-bold text-[var(--text-tertiary)]">{i + 1}</span>
              </div>
              <h3 class="mt-2 text-[13px] font-semibold">{t(s.title)}</h3>
              <p class="mt-0.5 text-[12px] text-[var(--text-tertiary)]">{t(s.text)}</p>
            </li>
          {/each}
        </ol>
      </div>
      <div class="lg:col-span-2 surface-card p-6 sm:p-7">
        <h2 class="font-display font-bold text-[15px] mb-3">{t('subscription.notes.title')}</h2>
        <ul class="space-y-3.5">
          {#each notes as n (n)}
            <li class="flex gap-2.5 text-[12.5px] text-[var(--text-secondary)]"><i class="icon-info text-[14px] mt-0.5 text-[var(--color-primary-500)] shrink-0"></i>{t(n)}</li>
          {/each}
        </ul>
      </div>
    </section>
  {/if}
</main>

{#if paying}
  <Modal title={t('subscription.pay.title', { plan: paying.plan_name })} onclose={closePay}>
    <div class="space-y-4 text-center">
      {#if paying.status === 'succeeded' && paying.applied}
        <div in:scale={{ duration: dur(350), start: 0.7, easing: cubicOut }} class="space-y-3 py-2">
          <span class="success-ring mx-auto grid size-16 place-items-center rounded-full bg-[var(--color-success-500)]/15 text-[var(--color-success-600)]"><i class="icon-circle-check text-[34px]"></i></span>
          <p class="text-[14px] font-semibold text-[var(--color-success-600)]">
            {t('subscription.pay.paid', { plan: paying.plan_name, date: paying.subscription_ends ? formatDateTime(paying.subscription_ends, { dateStyle: 'medium', timeStyle: 'short' }) : '' })}
          </p>
        </div>
      {:else if payFinal(paying.status)}
        <div in:fade={{ duration: dur(200) }} class="space-y-2 py-2">
          <span class="mx-auto grid size-14 place-items-center rounded-full bg-[var(--color-danger-500)]/12 text-[var(--color-danger-600)]"><i class="icon-circle-x text-[30px]"></i></span>
          <p class="text-[13px] text-[var(--color-danger-600)]">{t('subscription.pay.failed')}</p>
        </div>
      {:else if qr}
        <div class="qr-wrap mx-auto relative inline-block rounded-2xl border border-[var(--border-subtle)] bg-white p-3 shadow-sm" in:scale={{ duration: dur(300), start: 0.92 }}>
          <img src={qr} alt={t('subscription.pay.qrAlt')} class="mx-auto size-[220px]" />
          <span class="scan-line" aria-hidden="true"></span>
        </div>
        <div>
          <p class="font-display font-bold text-[22px] tabular-nums">{rp(Number(paying.amount))}</p>
          <p class="mt-1 text-[12.5px] text-[var(--text-secondary)]">{t('subscription.pay.scanShort')}</p>
        </div>
        <p class="inline-flex items-center gap-2 rounded-full bg-[var(--surface-sunken)] px-3 py-1.5 text-[12px] text-[var(--text-tertiary)]"><span class="spinner spinner-dark" aria-hidden="true"></span>{t('subscription.pay.waiting')}</p>
      {/if}
      <div class="flex justify-center gap-2">
        {#if waiting}<button type="button" class="btn btn-outline !text-[12.5px] disabled:opacity-60" disabled={checking} onclick={check}>{t('subscription.pay.check')}</button>{/if}
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={closePay}>{t('subscription.pay.close')}</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .hero {
    background: linear-gradient(140deg, var(--color-primary-600), var(--color-primary-800) 70%);
    min-height: 13rem;
  }
  .hero-orb {
    position: absolute;
    border-radius: 9999px;
    background: radial-gradient(circle, rgb(255 255 255 / 0.22), transparent 70%);
    animation: float 9s ease-in-out infinite;
  }
  .hero-orb-a { width: 11rem; height: 11rem; inset-block-start: -3rem; inset-inline-end: -2rem; }
  .hero-orb-b { width: 8rem; height: 8rem; inset-block-end: -2.5rem; inset-inline-start: 25%; animation-delay: -4s; }
  @keyframes float {
    0%, 100% { transform: translate3d(0, 0, 0); }
    50% { transform: translate3d(0.6rem, -0.8rem, 0); }
  }
  .pulse-dot { animation: pulse 1.8s ease-in-out infinite; }
  @keyframes pulse {
    0%, 100% { box-shadow: 0 0 0 0 rgb(255 255 255 / 0.5); }
    50% { box-shadow: 0 0 0 5px rgb(255 255 255 / 0); }
  }
  .meter-fill { transition: width 0.9s cubic-bezier(0.22, 1, 0.36, 1), background 0.3s; }

  .plan-card { transition: transform 0.25s ease, box-shadow 0.25s ease, border-color 0.25s ease; }
  .plan-card:hover { transform: translateY(-4px); box-shadow: 0 14px 30px -14px rgb(15 23 42 / 0.28); }
  .plan-card.is-star { border-color: var(--color-primary-400); box-shadow: 0 0 0 1px var(--color-primary-400); }
  .plan-card.is-mine { border-color: var(--color-success-500); box-shadow: 0 0 0 1px var(--color-success-500); }
  .buy-btn { display: inline-flex; align-items: center; justify-content: center; gap: 0.5rem; }

  .spinner {
    width: 0.95rem; height: 0.95rem; border-radius: 9999px;
    border: 2px solid rgb(255 255 255 / 0.4); border-top-color: #fff;
    animation: spin 0.7s linear infinite;
  }
  .spinner-dark { border-color: var(--border-subtle); border-top-color: var(--color-primary-500); }
  @keyframes spin { to { transform: rotate(360deg); } }

  .scan-line {
    position: absolute; inset-inline: 0.75rem; height: 2px; border-radius: 2px;
    background: linear-gradient(90deg, transparent, var(--color-primary-500), transparent);
    animation: scan 2.2s ease-in-out infinite;
  }
  @keyframes scan { 0% { inset-block-start: 0.75rem; opacity: 0; } 15%, 85% { opacity: 1; } 100% { inset-block-start: calc(100% - 0.75rem); opacity: 0; } }
  .success-ring { animation: ring 1.4s ease-out 1; }
  @keyframes ring { 0% { box-shadow: 0 0 0 0 rgb(34 197 94 / 0.45); } 100% { box-shadow: 0 0 0 18px rgb(34 197 94 / 0); } }

  @media (prefers-reduced-motion: reduce) {
    .hero-orb, .pulse-dot, .scan-line, .success-ring, .spinner { animation: none; }
    .plan-card, .meter-fill { transition: none; }
    .plan-card:hover { transform: none; }
  }
</style>
