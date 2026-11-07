<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type DomainRow, type TenantRow } from '#lib/platform/api.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { API_URL } from '#lib/api/client.ts';
  import Avatar from '#lib/platform/Avatar.svelte';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';

  type StepState = 'done' | 'current' | 'error' | 'off';
  type Filter = 'all' | DomainRow['status'];

  let rows = $state<DomainRow[]>([]);
  let target = $state('');
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let busy = $state(''); // id domain yang sedang diproses

  let q = $state('');
  let filter = $state<Filter>('all');

  let showModal = $state(false);
  let form = $state({ host: '', landing: false });
  let saving = $state(false);
  let modalError = $state('');

  // combobox tenant
  let picked = $state<TenantRow | null>(null);
  let tq = $state('');
  let options = $state<TenantRow[]>([]);
  let optState = $state<'idle' | 'loading' | 'done'>('idle');
  let listOpen = $state(false);
  let activeIdx = $state(0);

  let copiedKey = $state('');

  async function load() {
    try {
      const res = await platformApi.domains();
      rows = res.items;
      target = res.target;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  // ---- ringkasan & filter ----
  const total = $derived(rows.length);
  const count = $derived({
    active: rows.filter((r) => r.status === 'active').length,
    pending: rows.filter((r) => r.status === 'pending').length,
    disabled: rows.filter((r) => r.status === 'disabled').length
  });
  const shown = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    return rows.filter(
      (r) =>
        (filter === 'all' || r.status === filter) &&
        (!needle || r.host.includes(needle) || r.tenant_name.toLowerCase().includes(needle) || r.tenant_code.includes(needle))
    );
  });
  const chips = $derived<{ key: Filter; label: string; n: number }[]>([
    { key: 'all', label: t('platform.domains.filterAll'), n: total },
    { key: 'pending', label: t('platform.domains.pending'), n: count.pending },
    { key: 'active', label: t('platform.domains.active'), n: count.active },
    { key: 'disabled', label: t('platform.domains.disabled'), n: count.disabled }
  ]);

  // ---- helper tampilan ----
  const label = (host: string) => host.split('.')[0];
  const dnsRecord = (d: { host: string }) => `${label(d.host)}  CNAME  ${target}`;
  const digCmd = (host: string) => `dig +short CNAME ${host}`;
  const instruction = (host: string) => t('platform.domains.clientInstruction', { name: label(host), host, target });
  const hostLooksValid = (h: string) => /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$/.test(h.trim().toLowerCase());

  function steps(d: DomainRow): { label: string; state: StepState }[] {
    const dns: StepState = d.verified_at ? 'done' : d.status === 'disabled' ? 'off' : d.last_check_error ? 'error' : 'current';
    return [
      { label: t('platform.domains.stepRegistered'), state: 'done' },
      { label: t('platform.domains.stepDns'), state: dns },
      { label: t('platform.domains.stepLive'), state: d.status === 'active' ? 'done' : 'off' }
    ];
  }
  const stepIcon: Record<StepState, string> = { done: 'check', current: 'clock', error: 'circle-alert', off: 'circle-dashed' };
  const stepColor: Record<StepState, string> = {
    done: 'background:var(--color-success-500);color:#fff',
    current: 'background:var(--color-warning-50);color:var(--color-warning-600);box-shadow:0 0 0 2px var(--color-warning-500) inset',
    error: 'background:var(--color-danger-500);color:#fff',
    off: 'background:var(--surface-sunken);color:var(--text-tertiary)'
  };
  const statusBadge = { active: 'badge-success', pending: 'badge-warning', disabled: 'badge-info' } as const;
  const statusIcon = { active: 'shield-check', pending: 'clock', disabled: 'ban' } as const;

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text);
      copiedKey = key;
      setTimeout(() => {
        if (copiedKey === key) copiedKey = '';
      }, 1500);
    } catch {
      /* clipboard diblokir: teks tetap bisa dipilih manual */
    }
  }

  // ---- tambah domain ----
  function openCreate() {
    form = { host: '', landing: false };
    picked = null;
    tq = '';
    options = [];
    optState = 'idle';
    listOpen = false;
    modalError = '';
    showModal = true;
  }

  // Saran tenant: cari berdasarkan nama/kode (debounce); kolom kosong menampilkan tenant terbaru.
  let seq = 0;
  $effect(() => {
    const q = tq.trim().toLowerCase();
    if (!showModal || !listOpen || picked) return;
    optState = 'loading';
    const my = ++seq;
    const timer = setTimeout(async () => {
      try {
        const res = await platformApi.tenants(q, 8, 0);
        if (my !== seq) return;
        options = res.items;
        activeIdx = 0;
      } catch {
        if (my === seq) options = [];
      } finally {
        if (my === seq) optState = 'done';
      }
    }, q ? 200 : 0);
    return () => clearTimeout(timer);
  });

  function pick(row: TenantRow) {
    picked = row;
    tq = '';
    listOpen = false;
  }
  function clearPick() {
    picked = null;
    tq = '';
    listOpen = true;
  }
  function comboKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      listOpen = true;
      if (options.length) activeIdx = (activeIdx + 1) % options.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (options.length) activeIdx = (activeIdx - 1 + options.length) % options.length;
    } else if (e.key === 'Enter' && listOpen && options[activeIdx]) {
      e.preventDefault();
      pick(options[activeIdx]);
    } else if (e.key === 'Escape' && listOpen) {
      e.stopPropagation();
      listOpen = false;
    }
  }
  // tutup daftar saat fokus keluar dari combobox
  function comboBlur(e: FocusEvent) {
    const next = e.relatedTarget as Node | null;
    if (!next || !(e.currentTarget as HTMLElement).contains(next)) listOpen = false;
  }

  const hostNorm = $derived(form.host.trim().toLowerCase());
  const hostOk = $derived(hostLooksValid(hostNorm));
  const hostPlaceholder = $derived('arus.namabisnismu.id');
  const canSave = $derived(!!picked && hostOk && !saving);

  async function save() {
    if (!picked || !hostOk) return;
    saving = true;
    modalError = '';
    notice = '';
    error = '';
    try {
      const created = await platformApi.createDomain({ tenant_id: picked.id, host: hostNorm });
      if (form.landing) await platformApi.setDomainLanding(created.id, true);
      notice = t('platform.domains.created');
      showModal = false;
      await load();
    } catch (e) {
      modalError = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  // ---- aksi per domain ----
  async function run(id: string, fn: () => Promise<string | void>) {
    busy = id;
    error = '';
    notice = '';
    try {
      const msg = await fn();
      if (msg) notice = msg;
      await load();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = '';
    }
  }

  const verify = (d: DomainRow) =>
    run(d.id, async () => {
      const res = await platformApi.verifyDomain(d.id);
      return res.status === 'active' ? t('platform.domains.verified') : t('platform.domains.notVerified');
    });
  const toggle = (d: DomainRow) =>
    run(d.id, async () => {
      const disable = d.status !== 'disabled';
      await platformApi.setDomainDisabled(d.id, disable);
      return disable ? t('platform.domains.disabledMsg') : t('platform.domains.enabledMsg');
    });
  const setLanding = (d: DomainRow, on: boolean) =>
    run(d.id, async () => {
      await platformApi.setDomainLanding(d.id, on);
      return on ? t('platform.domains.landingOnMsg') : t('platform.domains.landingOffMsg');
    });
  // Client ID Google per domain: draf yang sedang diketik (kunci = id domain); tanpa draf = nilai tersimpan.
  let googleDraft = $state<Record<string, string>>({});
  const googleValue = (d: DomainRow) => googleDraft[d.id] ?? d.google_client_id;
  const saveGoogle = (d: DomainRow) =>
    run(d.id, async () => {
      const id = googleValue(d).trim();
      await platformApi.setDomainGoogle(d.id, id);
      delete googleDraft[d.id];
      return id ? t('platform.domains.googleOnMsg') : t('platform.domains.googleOffMsg');
    });
  // White label: draf nama/footer yang sedang diketik per domain; logo/ikon diunggah langsung saat dipilih.
  let brandDraft = $state<Record<string, { name?: string; footer?: string }>>({});
  const brandName = (d: DomainRow) => brandDraft[d.id]?.name ?? d.brand_name;
  const brandFooter = (d: DomainRow) => brandDraft[d.id]?.footer ?? d.brand_footer;
  const brandDirty = (d: DomainRow) => brandName(d).trim() !== d.brand_name || brandFooter(d).trim() !== d.brand_footer;
  const saveBrand = (d: DomainRow) =>
    run(d.id, async () => {
      await platformApi.setDomainBrand(d.id, { brand_name: brandName(d).trim(), brand_footer: brandFooter(d).trim() });
      delete brandDraft[d.id];
      return t('platform.domains.brandSaved');
    });
  const pickAsset = (d: DomainRow, kind: 'logo' | 'icon', e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    return run(d.id, async () => {
      await platformApi.uploadDomainAsset(d.id, kind, file);
      return t('platform.domains.brandImgSaved');
    });
  };
  const removeAsset = (d: DomainRow, kind: 'logo' | 'icon') =>
    run(d.id, async () => {
      await platformApi.deleteDomainAsset(d.id, kind);
      return t('platform.domains.brandImgRemoved');
    });
  const assetUrl = (d: DomainRow, kind: 'logo' | 'icon') => `${API_URL}/public/brand-asset?kind=${kind}&host=${encodeURIComponent(d.host)}&v=${d.brand_ver}`;
  const remove = (d: DomainRow) => {
    if (!confirm(t('platform.domains.deleteConfirm'))) return;
    return run(d.id, async () => {
      await platformApi.deleteDomain(d.id);
      return t('platform.domains.deleted');
    });
  };
</script>

<svelte:head><title>{t('platform.domains.title')} | ACIRABA</title></svelte:head>

{#snippet copyBtn(text: string, key: string)}
  <button type="button" class="inline-flex items-center gap-1 rounded-md border border-white/15 bg-white/10 px-2 py-1 text-[11px] font-semibold text-sky-100 hover:bg-white/20" onclick={() => copy(text, key)}>
    <i class="icon-{copiedKey === key ? 'check' : 'copy'} text-[12px]"></i>{copiedKey === key ? t('platform.domains.copied') : t('platform.domains.copy')}
  </button>
{/snippet}

<div class="space-y-5">
  <div class="flex flex-wrap items-end justify-between gap-3">
    <div>
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('platform.domains.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('platform.domains.subtitle')}</p>
    </div>
    <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}>
      <i class="icon-plus text-[13px]"></i>{t('platform.domains.add')}
    </button>
  </div>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}
  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
    <StatCard label={t('platform.domains.statTotal')} value={formatNumber(total)} icon="globe" tone="primary" />
    <StatCard label={t('platform.domains.statActive')} value={formatNumber(count.active)} icon="shield-check" tone="success" />
    <StatCard label={t('platform.domains.statPending')} value={formatNumber(count.pending)} icon="clock" tone="warning" />
    <StatCard label={t('platform.domains.statDisabled')} value={formatNumber(count.disabled)} icon="ban" tone="info" />
  </div>

  <!-- Tujuan DNS + alur singkat -->
  <section class="overflow-hidden rounded-2xl border border-[var(--border-subtle)] bg-gradient-to-br from-[#06122b] via-[#0a1f4a] to-[#0d2b63] text-white">
    <div class="p-5 lg:p-6">
      <p class="flex items-center gap-2 text-[10.5px] font-bold uppercase tracking-[0.14em] text-sky-200"><i class="icon-terminal text-[13px]"></i>{t('platform.domains.targetTitle')}</p>
      {#if target}
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <code class="rounded-lg border border-white/10 bg-black/25 px-3.5 py-2 font-mono text-[17px] lg:text-[19px] font-semibold text-sky-100 break-all"><span class="text-emerald-300">$</span> {target}</code>
          {@render copyBtn(target, 'target')}
        </div>
        <p class="mt-3 max-w-3xl text-[12.5px] leading-relaxed text-sky-100/75">{t('platform.domains.targetHelp')} {t('platform.domains.cloudflareNote')}</p>
      {:else if !loading}
        <p class="mt-3 flex items-center gap-2 text-[14px] font-semibold text-amber-300"><i class="icon-circle-alert text-[16px]"></i>{t('platform.domains.noTarget')}</p>
        <p class="mt-1 text-[12px] text-sky-100/75 max-w-xl">{t('platform.domains.noTargetHelp')}</p>
      {/if}
    </div>
    <ol class="grid gap-px border-t border-white/10 bg-white/10 sm:grid-cols-2 xl:grid-cols-4" aria-label={t('platform.domains.howTitle')}>
      {#each [t('platform.domains.how1'), t('platform.domains.how2'), t('platform.domains.how3'), t('platform.domains.how4')] as step, i (i)}
        <li class="flex items-center gap-2.5 bg-[#08193c] px-5 py-3 text-[12.5px] font-medium text-sky-50">
          <span class="grid size-5 shrink-0 place-items-center rounded-full text-[11px] font-bold" style="background:#7dd3fc;color:#06122b">{i + 1}</span>{step}
        </li>
      {/each}
    </ol>
  </section>

  <!-- Filter -->
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="relative w-full sm:max-w-sm">
      <i class="icon-search absolute top-1/2 -translate-y-1/2 start-3 text-[13px] text-[var(--text-tertiary)]"></i>
      <input type="search" bind:value={q} placeholder={t('platform.domains.search')} aria-label={t('platform.domains.search')} class="w-full field-control !ps-8" />
    </div>
    <div class="flex flex-wrap gap-1.5" role="group" aria-label={t('platform.domains.status')}>
      {#each chips as c (c.key)}
        <button
          type="button"
          aria-pressed={filter === c.key}
          class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-semibold transition-colors {filter === c.key
            ? 'border-[var(--color-primary-500)] bg-[var(--color-primary-50)] text-[var(--color-primary-700)]'
            : 'border-[var(--border-subtle)] text-[var(--text-secondary,inherit)] hover:bg-[var(--surface-sunken)]'}"
          onclick={() => (filter = c.key)}
        >
          {c.label}<span class="tabular-nums text-[11px] opacity-70">{c.n}</span>
        </button>
      {/each}
    </div>
  </div>

  <!-- Daftar domain -->
  {#if loading}
    <div class="space-y-3" aria-hidden="true">
      {#each [0, 1, 2] as i (i)}<div class="h-40 rounded-2xl bg-[var(--surface-sunken)] animate-pulse"></div>{/each}
    </div>
  {:else if rows.length === 0}
    <div class="surface-card empty-hero relative overflow-hidden text-center">
      <div class="empty-grid" aria-hidden="true"></div>

      <!-- ilustrasi: domain klien → CNAME → server ARUS -->
      <div class="relative mx-auto mb-8 flex max-w-xl items-center justify-center gap-0" aria-hidden="true">
        <div class="node float-a">
          <span class="node-icon"><i class="icon-globe text-[24px]"></i></span>
          <span class="node-label">toko-klien.com</span>
        </div>
        <div class="link"><span class="packet"></span><span class="packet p2"></span></div>
        <div class="node float-b">
          <span class="node-icon node-core"><span class="ring"></span><span class="ring r2"></span><i class="icon-server text-[24px]"></i></span>
          <span class="node-label">{target || 'ARUS'}</span>
        </div>
        <div class="link"><span class="packet p3"></span></div>
        <div class="node float-a">
          <span class="node-icon"><i class="icon-shield-check text-[24px]"></i></span>
          <span class="node-label">HTTPS</span>
        </div>
      </div>

      <h2 class="relative text-[18px] font-bold tracking-tight">{t('platform.domains.emptyTitle')}</h2>
      <p class="relative mx-auto mt-2 max-w-md text-[13px] leading-relaxed text-[var(--text-tertiary)]">{t('platform.domains.emptyHint')}</p>
    </div>
  {:else if shown.length === 0}
    <div class="surface-card px-4 py-10 text-center text-[13px] text-[var(--text-tertiary)]">{t('platform.domains.noMatch')}</div>
  {:else}
    <div class="space-y-3">
      {#each shown as d (d.id)}
        <article class="surface-card !p-0 overflow-hidden {busy === d.id ? 'opacity-70' : ''}">
          <!-- kepala: domain + tenant + status -->
          <div class="flex flex-wrap items-start justify-between gap-3 p-4">
            <div class="flex items-center gap-3 min-w-0">
              <Avatar name={d.tenant_name} size={40} square />
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <code class="font-mono text-[15px] font-semibold truncate">{d.host}</code>
                  {#if d.status === 'active'}
                    <a href="https://{d.host}" target="_blank" rel="noopener noreferrer" class="text-[var(--text-tertiary)] hover:text-[var(--color-primary-600)]" title={t('platform.domains.open')} aria-label={t('platform.domains.open')}><i class="icon-external-link text-[14px]"></i></a>
                  {/if}
                </div>
                <p class="text-[12px] text-[var(--text-tertiary)] truncate">{d.tenant_name} · <span class="font-mono">{d.tenant_code}</span></p>
              </div>
            </div>
            <span class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold {statusBadge[d.status]}">
              <i class="icon-{statusIcon[d.status]} text-[12px]"></i>{t(`platform.domains.${d.status}`)}
            </span>
          </div>

          <!-- pipeline -->
          <ol class="flex items-center px-4 pb-4" aria-label={t('platform.domains.status')}>
            {#each steps(d) as s, i (i)}
              {#if i > 0}
                <li class="h-0.5 flex-1 mx-2 rounded-full {s.state === 'off' ? 'bg-[var(--border-subtle)]' : s.state === 'error' ? 'bg-[var(--color-danger-500)]' : 'bg-[var(--color-success-500)]'}" aria-hidden="true"></li>
              {/if}
              <li class="flex items-center gap-2">
                <span class="grid size-7 place-items-center rounded-full {s.state === 'current' ? 'animate-pulse' : ''}" style={stepColor[s.state]}><i class="icon-{stepIcon[s.state]} text-[14px]"></i></span>
                <span class="text-[12px] font-semibold {s.state === 'off' ? 'text-[var(--text-tertiary)]' : ''}">{s.label}</span>
              </li>
            {/each}
          </ol>

          <!-- isi: petunjuk / record DNS / hasil terakhir -->
          <div class="grid gap-3 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]/50 p-4 lg:grid-cols-[1fr_auto]">
            <div class="space-y-3 min-w-0">
              <p class="text-[12.5px] text-[var(--text-secondary,inherit)]">
                {d.status === 'pending' ? t('platform.domains.hintPending') : d.status === 'active' ? t('platform.domains.hintActive') : t('platform.domains.hintDisabled')}
              </p>

              {#if d.status === 'pending' && target}
                <div class="rounded-xl bg-[#0b1220] p-3 text-slate-200">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <p class="text-[10.5px] font-bold uppercase tracking-wider text-sky-300">{t('platform.domains.dnsRecord')}</p>
                    <div class="flex gap-1.5">
                      {@render copyBtn(instruction(d.host), `ins-${d.id}`)}
                    </div>
                  </div>
                  <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 font-mono text-[12.5px]">
                    <dt class="text-slate-400">{t('platform.domains.recordType')}</dt><dd class="text-emerald-300">CNAME</dd>
                    <dt class="text-slate-400">{t('platform.domains.recordName')}</dt><dd class="break-all">{label(d.host)} <span class="text-slate-500">({d.host})</span></dd>
                    <dt class="text-slate-400">{t('platform.domains.recordValue')}</dt><dd class="break-all text-sky-200">{target}</dd>
                  </dl>
                  <div class="mt-2 flex flex-wrap items-center gap-2 border-t border-white/10 pt-2">
                    <span class="text-[11px] text-slate-400">{t('platform.domains.checkCmd')}</span>
                    <code class="font-mono text-[12px] text-amber-200">$ {digCmd(d.host)}</code>
                    {@render copyBtn(digCmd(d.host), `dig-${d.id}`)}
                  </div>
                </div>
              {/if}

              {#if d.last_check_error}
                <div role="alert" class="rounded-xl bg-[#1a0f12] p-3 font-mono text-[12px] text-rose-200">
                  <p class="mb-1 text-[10.5px] font-bold uppercase tracking-wider text-rose-400">{t('platform.domains.lastResult')}</p>
                  <p><span class="text-rose-400">✗</span> {d.last_check_error}</p>
                </div>
              {/if}
            </div>

            <dl class="grid grid-cols-[auto_1fr] content-start gap-x-4 gap-y-1.5 text-[12px] lg:min-w-64">
              <dt class="text-[var(--text-tertiary)]">{t('platform.domains.createdAt')}</dt>
              <dd>{formatDateTime(d.created_at, { dateStyle: 'medium', timeStyle: 'short' })}</dd>
              <dt class="text-[var(--text-tertiary)]">{t('platform.domains.verifiedAt')}</dt>
              <dd>{d.verified_at ? formatDateTime(d.verified_at, { dateStyle: 'medium', timeStyle: 'short' }) : '—'}</dd>
              <dt class="text-[var(--text-tertiary)]">{t('platform.domains.lastCheck')}</dt>
              <dd title={d.last_check_at ? formatDateTime(d.last_check_at, { dateStyle: 'medium', timeStyle: 'medium' }) : ''}>{d.last_check_at ? relativeTime(d.last_check_at) : t('platform.domains.never')}</dd>
            </dl>
          </div>

          <!-- landing page di root domain -->
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-subtle)] px-4 py-3">
            <div class="flex min-w-0 items-start gap-3">
              <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-[var(--color-primary-50)] text-[var(--color-primary-600)]"><i class="icon-layout-template text-[16px]"></i></span>
              <div class="min-w-0">
                <p class="text-[12.5px] font-semibold">{t('platform.domains.landingTitle')}</p>
                <p class="text-[12px] text-[var(--text-tertiary)]">
                  {d.status === 'disabled' ? t('platform.domains.landingNeedActive') : d.landing && d.status === 'pending' ? t('platform.domains.landingPending') : d.landing ? t('platform.domains.landingHelpOn', { host: d.host }) : t('platform.domains.landingHelpOff', { host: d.host })}
                </p>
              </div>
            </div>
            <div class="flex items-center gap-2">
              {#if d.landing && d.status === 'active'}
                <a href="https://{d.host}/" target="_blank" rel="noopener noreferrer" class="btn btn-outline !text-[12px] !py-1"><i class="icon-external-link text-[13px]"></i>{t('platform.domains.open')}</a>
              {/if}
              <button
                type="button"
                role="switch"
                aria-checked={d.landing}
                aria-label={t('platform.domains.landingTitle')}
                disabled={busy === d.id || (d.status === 'disabled' && !d.landing)}
                class="relative h-6 w-11 shrink-0 rounded-full transition-colors disabled:opacity-50"
                style="background:{d.landing ? 'var(--color-success-500)' : 'var(--border-subtle)'}"
                onclick={() => setLanding(d, !d.landing)}
              >
                <span class="absolute top-0.5 size-5 rounded-full bg-white shadow transition-all" style="inset-inline-start:{d.landing ? '22px' : '2px'}"></span>
              </button>
            </div>
          </div>

          <!-- white label: nama, logo, ikon, footer -->
          <div class="space-y-3 border-t border-[var(--border-subtle)] px-4 py-3">
            <div class="flex items-start gap-3">
              <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-[var(--color-primary-50)] text-[var(--color-primary-600)]"><i class="icon-palette text-[16px]"></i></span>
              <div class="min-w-0">
                <p class="text-[12.5px] font-semibold">{t('platform.domains.brandTitle')}</p>
                <p class="text-[12px] text-[var(--text-tertiary)]">{t('platform.domains.brandHelp')}</p>
              </div>
            </div>
            <div class="grid gap-3 ps-11 sm:grid-cols-2">
              <label class="block text-[12px] font-medium">{t('platform.domains.brandName')}
                <input type="text" maxlength="60" value={brandName(d)} oninput={(e) => (brandDraft[d.id] = { ...brandDraft[d.id], name: e.currentTarget.value })} placeholder="ARUS" class="field-control mt-1 w-full !text-[12.5px]" />
              </label>
              <label class="block text-[12px] font-medium">{t('platform.domains.brandFooter')}
                <input type="text" maxlength="200" value={brandFooter(d)} oninput={(e) => (brandDraft[d.id] = { ...brandDraft[d.id], footer: e.currentTarget.value })} placeholder={t('platform.domains.brandFooterPh', { name: d.tenant_name })} class="field-control mt-1 w-full !text-[12.5px]" />
              </label>
              {#each [{ kind: 'logo' as const, has: d.has_logo, label: t('platform.domains.brandLogo') }, { kind: 'icon' as const, has: d.has_icon, label: t('platform.domains.brandIcon') }] as a (a.kind)}
                <div class="rounded-xl border border-[var(--border-subtle)] p-3">
                  <p class="text-[12px] font-medium">{a.label}</p>
                  <div class="mt-2 flex items-center gap-3">
                    <div class="grid h-12 w-24 shrink-0 place-items-center overflow-hidden rounded-lg bg-[var(--surface-sunken)]">
                      {#if a.has && d.status === 'active'}
                        <img src={assetUrl(d, a.kind)} alt={a.label} class="max-h-12 max-w-24 object-contain" />
                      {:else if a.has}
                        <i class="icon-check text-[16px] text-[var(--color-success-600)]"></i>
                      {:else}
                        <i class="icon-image text-[18px] text-[var(--text-tertiary)]"></i>
                      {/if}
                    </div>
                    <div class="flex flex-wrap gap-1.5">
                      <label class="btn btn-outline !text-[11.5px] !py-1 cursor-pointer">
                        <i class="icon-upload text-[12px]"></i>{a.has ? t('platform.domains.brandReplace') : t('platform.domains.brandUpload')}
                        <input type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml" class="sr-only" disabled={busy === d.id} onchange={(e) => pickAsset(d, a.kind, e)} />
                      </label>
                      {#if a.has}
                        <button type="button" class="btn btn-outline !text-[11.5px] !py-1 text-[var(--color-danger-600)]" disabled={busy === d.id} onclick={() => removeAsset(d, a.kind)}>{t('platform.domains.brandRemove')}</button>
                      {/if}
                    </div>
                  </div>
                </div>
              {/each}
            </div>
            <div class="flex flex-wrap items-center justify-between gap-2 ps-11">
              <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('platform.domains.brandImgHelp')}</p>
              <button type="button" class="btn btn-primary !text-[12px] !py-1" disabled={busy === d.id || !brandDirty(d)} onclick={() => saveBrand(d)}>{t('platform.domains.save')}</button>
            </div>
          </div>

          <!-- login Google: client ID milik domain ini -->
          <div class="space-y-2 border-t border-[var(--border-subtle)] px-4 py-3">
            <div class="flex items-start gap-3">
              <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-[var(--color-primary-50)] text-[var(--color-primary-600)]"><i class="fa-brands fa-google text-[14px]"></i></span>
              <div class="min-w-0">
                <p class="text-[12.5px] font-semibold">{t('platform.domains.googleTitle')}</p>
                <p class="text-[12px] text-[var(--text-tertiary)]">{d.google_client_id ? t('platform.domains.googleHelpOn') : t('platform.domains.googleHelpOff', { origin: `https://${d.host}` })}</p>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2 ps-11">
              <input
                type="text"
                value={googleValue(d)}
                oninput={(e) => (googleDraft[d.id] = e.currentTarget.value)}
                placeholder="1234567890-abc123.apps.googleusercontent.com"
                aria-label={t('platform.domains.googleTitle')}
                class="field-control min-w-0 flex-1 font-mono !text-[12px]"
                autocomplete="off"
                spellcheck="false"
              />
              <button type="button" class="btn btn-outline !text-[12px] !py-1" disabled={busy === d.id || googleValue(d).trim() === d.google_client_id} onclick={() => saveGoogle(d)}>{t('platform.domains.save')}</button>
            </div>
          </div>

          <!-- aksi -->
          <div class="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--border-subtle)] px-4 py-2.5">
            <button type="button" class="btn btn-outline !text-[12px] !py-1 text-[var(--color-danger-600)]" disabled={busy === d.id} onclick={() => remove(d)}>
              <i class="icon-trash-2 text-[13px]"></i>{t('platform.domains.delete')}
            </button>
            <button type="button" class="btn btn-outline !text-[12px] !py-1" disabled={busy === d.id} onclick={() => toggle(d)}>
              <i class="icon-{d.status === 'disabled' ? 'play' : 'power'} text-[13px]"></i>{d.status === 'disabled' ? t('platform.domains.enable') : t('platform.domains.disable')}
            </button>
            {#if d.status !== 'disabled'}
              <button type="button" class="btn {d.status === 'pending' ? 'btn-primary' : 'btn-outline'} !text-[12px] !py-1" disabled={busy === d.id || !target} onclick={() => verify(d)}>
                <i class="icon-refresh-cw text-[13px] {busy === d.id ? 'animate-spin' : ''}"></i>{busy === d.id ? t('platform.domains.verifying') : t('platform.domains.verify')}
              </button>
            {/if}
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>

{#if showModal}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-[2px]" role="dialog" tabindex="-1" aria-modal="true" aria-label={t('platform.domains.addTitle')} onkeydown={(e) => e.key === 'Escape' && (showModal = false)}>
    <div class="surface-card w-full max-w-xl rounded-2xl shadow-2xl !p-0 overflow-visible">
      <div class="flex items-start justify-between gap-3 rounded-t-2xl bg-gradient-to-br from-[#06122b] via-[#0a1f4a] to-[#0d2b63] px-5 py-4 text-white">
        <div class="flex items-center gap-3">
          <span class="grid size-10 place-items-center rounded-xl bg-white/10 text-sky-200"><i class="icon-globe text-[20px]"></i></span>
          <div>
            <h2 class="font-semibold text-[15px] leading-tight" style="color:#fff">{t('platform.domains.addTitle')}</h2>
            <p class="mt-0.5 text-[12px] text-sky-100/75">{t('platform.domains.addSubtitle')}</p>
          </div>
        </div>
        <button type="button" aria-label={t('platform.domains.close')} class="grid size-7 place-items-center rounded-md text-sky-100 hover:bg-white/15" onclick={() => (showModal = false)}><i class="icon-x text-[14px]"></i></button>
      </div>

      <form onsubmit={(e) => { e.preventDefault(); save(); }} novalidate class="space-y-5 p-5">
        {#if modalError}
          <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{modalError}</span></div>
        {/if}

        <!-- 1. tenant -->
        <div>
          <p class="mb-1.5 flex items-center gap-2 text-[12.5px] font-semibold">
            <span class="grid size-5 place-items-center rounded-full bg-[var(--color-primary-600)] text-[11px] font-bold text-white">1</span>
            <label for="dom-tenant">{t('platform.domains.tenantPick')}</label>
          </p>

          {#if picked}
            <div class="flex items-center gap-3 rounded-xl border border-[var(--color-success-500)] p-3">
              <Avatar name={picked.name} size={40} square />
              <div class="min-w-0 flex-1">
                <p class="truncate text-[13.5px] font-semibold">{picked.name}</p>
                <p class="truncate text-[12px] text-[var(--text-tertiary)]"><span class="font-mono">{picked.code}</span>{picked.owner_email ? ` · ${picked.owner_email}` : ''}</p>
              </div>
              <i class="icon-circle-check text-[18px] text-[var(--color-success-600)]"></i>
              <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={clearPick}>{t('platform.domains.tenantChange')}</button>
            </div>
          {:else}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="relative" onfocusout={comboBlur}>
              <div class="flex items-center gap-2 rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-card,#fff)] px-3 focus-within:border-[var(--color-primary-500)] focus-within:ring-2 focus-within:ring-[var(--color-primary-500)]/20" style="height:42px">
              <i class="icon-search shrink-0 text-[14px] text-[var(--text-tertiary)]"></i>
              <input
                id="dom-tenant"
                type="text"
                role="combobox"
                aria-expanded={listOpen}
                aria-controls="dom-tenant-list"
                aria-autocomplete="list"
                bind:value={tq}
                style="flex:1;min-width:0;border:0;outline:0;background:transparent;box-shadow:none;padding:0;font-size:13px;color:inherit"
                autocomplete="off"
                spellcheck="false"
                placeholder={t('platform.domains.tenantSearch')}
                onfocus={() => (listOpen = true)}
                oninput={() => (listOpen = true)}
                onkeydown={comboKey}
              />
              </div>
              {#if listOpen}
                <ul id="dom-tenant-list" role="listbox" class="surface-card absolute inset-x-0 top-full z-10 mt-1 max-h-64 overflow-y-auto rounded-xl !p-1 shadow-xl">
                  {#if optState === 'loading' && options.length === 0}
                    <li class="px-3 py-3 text-[12.5px] text-[var(--text-tertiary)]">{t('platform.domains.tenantChecking')}</li>
                  {:else if options.length === 0}
                    <li class="flex items-center gap-2 px-3 py-3 text-[12.5px] text-[var(--text-tertiary)]"><i class="icon-circle-x text-[14px]"></i>{t('platform.domains.tenantNotFound')}</li>
                  {:else}
                    {#each options as o, i (o.id)}
                      <li role="option" aria-selected={i === activeIdx}>
                        <button
                          type="button"
                          tabindex="-1"
                          class="flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-start {i === activeIdx ? 'bg-[var(--color-primary-50)]' : 'hover:bg-[var(--surface-sunken)]'}"
                          onmousemove={() => (activeIdx = i)}
                          onmousedown={(e) => { e.preventDefault(); pick(o); }}
                        >
                          <Avatar name={o.name} size={32} square />
                          <span class="min-w-0 flex-1">
                            <span class="block truncate text-[13px] font-semibold">{o.name}</span>
                            <span class="block truncate text-[11.5px] text-[var(--text-tertiary)]">{o.owner_email || '—'}</span>
                          </span>
                          <code class="shrink-0 rounded-md bg-[var(--surface-sunken)] px-1.5 py-0.5 font-mono text-[11.5px]">{o.code}</code>
                          {#if !o.active}<span class="badge-info shrink-0 rounded-full px-2 py-0.5 text-[10.5px] font-semibold">{t('platform.domains.tenantInactive')}</span>{/if}
                        </button>
                      </li>
                    {/each}
                  {/if}
                </ul>
              {/if}
            </div>
            <p class="mt-1.5 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.domains.tenantCodeHelp')}</p>
          {/if}
        </div>

        <!-- 2. domain -->
        <div>
          <p class="mb-1.5 flex items-center gap-2 text-[12.5px] font-semibold">
            <span class="grid size-5 place-items-center rounded-full bg-[var(--color-primary-600)] text-[11px] font-bold text-white">2</span>
            <label for="dom-host">{t('platform.domains.host')}</label>
          </p>
          <div class="flex items-center gap-2 rounded-lg border border-[var(--border-subtle)] bg-[var(--surface-card,#fff)] px-3 focus-within:border-[var(--color-primary-500)] focus-within:ring-2 focus-within:ring-[var(--color-primary-500)]/20" style="height:42px">
            <span class="shrink-0 font-mono text-[12px] text-[var(--text-tertiary)]">https://</span>
            <input id="dom-host" type="text" bind:value={form.host} class="font-mono" style="flex:1;min-width:0;border:0;outline:0;background:transparent;box-shadow:none;padding:0;font-size:13px;color:inherit" placeholder={hostPlaceholder} autocomplete="off" spellcheck="false" />
          </div>
          <p class="mt-1.5 min-h-4 text-[11.5px] {form.host && !hostOk ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-tertiary)]'}">
            {#if form.host && !hostOk}<i class="icon-circle-alert text-[12px]"></i> {t('platform.domains.hostInvalid')}{:else}{t('platform.domains.hostHelp')}{/if}
          </p>
        </div>

        <label class="flex cursor-pointer items-start gap-3 rounded-xl border border-[var(--border-subtle)] p-3 {form.landing ? 'bg-[var(--color-primary-50)] border-[var(--color-primary-500)]' : ''}">
          <input type="checkbox" bind:checked={form.landing} class="mt-0.5 size-4 accent-[var(--color-primary-600)]" />
          <span class="min-w-0">
            <span class="block text-[12.5px] font-semibold">{t('platform.domains.landingAddLabel')}</span>
            <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t('platform.domains.landingAddHelp')}</span>
          </span>
        </label>

        <!-- 3. DNS -->
        <div>
          <p class="mb-1.5 flex items-center gap-2 text-[12.5px] font-semibold">
            <span class="grid size-5 place-items-center rounded-full bg-[var(--color-primary-600)] text-[11px] font-bold text-white">3</span>
            {t('platform.domains.dnsRecord')}
          </p>
          {#if hostOk && target}
            <div class="rounded-xl bg-[#0b1220] p-3 text-slate-200">
              <div class="flex items-center justify-between gap-2">
                <p class="text-[10.5px] font-bold uppercase tracking-wider text-sky-300">{t('platform.domains.dnsAtProvider')}</p>
                {@render copyBtn(instruction(hostNorm), 'ins-new')}
              </div>
              <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 font-mono text-[12.5px]">
                <dt class="text-slate-400">{t('platform.domains.recordType')}</dt><dd class="text-emerald-300">CNAME</dd>
                <dt class="text-slate-400">{t('platform.domains.recordName')}</dt><dd class="break-all">{label(hostNorm)}</dd>
                <dt class="text-slate-400">{t('platform.domains.recordValue')}</dt><dd class="break-all text-sky-200">{target}</dd>
              </dl>
            </div>
          {:else}
            <p class="rounded-xl border border-dashed border-[var(--border-subtle)] px-3 py-3 text-[12px] text-[var(--text-tertiary)]">{target ? t('platform.domains.dnsWaiting') : t('platform.domains.noTarget')}</p>
          {/if}
        </div>

        <div class="flex items-center justify-between gap-2 border-t border-[var(--border-subtle)] pt-4">
          <p class="hidden text-[11.5px] text-[var(--text-tertiary)] sm:block">{t('platform.domains.afterSave')}</p>
          <div class="ms-auto flex gap-2">
            <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showModal = false)}>{t('platform.domains.cancel')}</button>
            <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={!canSave}>{saving ? t('platform.domains.saving') : t('platform.domains.save')}</button>
          </div>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .empty-hero {
    padding: 44px 24px 48px !important;
  }
  @media (min-width: 1024px) {
    .empty-hero {
      padding: 56px 32px 60px !important;
    }
  }
  .empty-grid {
    position: absolute;
    inset: 0;
    background-image: radial-gradient(circle, color-mix(in srgb, var(--color-primary-500) 22%, transparent) 1px, transparent 1.5px);
    background-size: 22px 22px;
    mask-image: radial-gradient(ellipse at center, #000 0%, transparent 70%);
    -webkit-mask-image: radial-gradient(ellipse at center, #000 0%, transparent 70%);
    opacity: 0.6;
  }
  .node {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    min-width: 96px;
  }
  .node-icon {
    position: relative;
    display: grid;
    place-items: center;
    width: 60px;
    height: 60px;
    border-radius: 18px;
    background: var(--surface-card, #fff);
    border: 1px solid var(--border-subtle);
    color: var(--color-primary-600);
    box-shadow: 0 8px 24px -10px color-mix(in srgb, var(--color-primary-500) 55%, transparent);
  }
  .node-core {
    background: linear-gradient(135deg, var(--color-primary-600), var(--color-primary-800, var(--color-primary-700)));
    color: #fff;
    border: 0;
  }
  .node-label {
    font-family: ui-monospace, monospace;
    font-size: 11px;
    color: var(--text-tertiary);
    max-width: 130px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ring {
    position: absolute;
    inset: 0;
    border-radius: 18px;
    border: 2px solid var(--color-primary-500);
    animation: ring-out 2.6s ease-out infinite;
  }
  .ring.r2 {
    animation-delay: 1.3s;
  }
  .link {
    position: relative;
    flex: 1;
    min-width: 48px;
    height: 2px;
    margin: 0 4px 26px;
    background-image: linear-gradient(90deg, var(--border-subtle) 50%, transparent 50%);
    background-size: 10px 2px;
  }
  .packet {
    position: absolute;
    top: -3px;
    left: 0;
    width: 8px;
    height: 8px;
    border-radius: 99px;
    background: var(--color-primary-500);
    box-shadow: 0 0 10px var(--color-primary-500);
    animation: packet 2.2s linear infinite;
  }
  .packet.p2 {
    animation-delay: 1.1s;
  }
  .packet.p3 {
    animation-delay: 0.6s;
  }
  .float-a {
    animation: float 4s ease-in-out infinite;
  }
  .float-b {
    animation: float 4s ease-in-out infinite 1s;
  }
  @keyframes ring-out {
    from { transform: scale(1); opacity: 0.55; }
    to { transform: scale(1.7); opacity: 0; }
  }
  @keyframes packet {
    from { left: 0; opacity: 0; }
    15% { opacity: 1; }
    85% { opacity: 1; }
    to { left: calc(100% - 8px); opacity: 0; }
  }
  @keyframes float {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-6px); }
  }
  @media (prefers-reduced-motion: reduce) {
    .ring, .packet, .float-a, .float-b { animation: none; }
  }
</style>
