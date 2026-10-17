<script lang="ts">
  import { platformApi, type ExtCredential, type ExtBrand, type ExtBrandInput, type ExtActors, type ExtGroup, type ExtCreated } from '#lib/platform/api.ts';
  import { fly, fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import StatCard from '#lib/platform/StatCard.svelte';
  import StatusPill from '#lib/platform/StatusPill.svelte';
  import Avatar from '#lib/platform/Avatar.svelte';
  import { relativeTime } from '#lib/platform/relative.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import Switch from '#lib/components/Switch.svelte';

  let tab = $state<'credentials' | 'branding'>('credentials');
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');

  // ---- kredensial ----
  let creds = $state<ExtCredential[]>([]);
  let tenants = $state<{ id: string; name: string }[]>([]);
  let filterTenant = $state('');

  async function loadCreds() {
    creds = (await platformApi.extCredentials(filterTenant)).items;
  }
  async function loadTenants() {
    // Pilihan tenant untuk form & filter; dibatasi 200 teratas (pencarian lengkap ada di halaman Tenant).
    const page = await platformApi.tenants('', 200, 0);
    tenants = page.items.map((x) => ({ id: x.id, name: `${x.name} (${x.code})` }));
  }
  async function loadAll() {
    loading = true;
    error = '';
    try {
      await Promise.all([loadTenants(), loadCreds(), loadBrands()]);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  type CredForm = { id: string; tenant_id: string; user_id: string; outlet_id: string; label: string; ips: string; rate: number; expires: string; active: boolean };
  let credForm = $state<CredForm | null>(null);
  let actors = $state<ExtActors>({ users: [], outlets: [] });
  let saving = $state(false);
  let formError = $state('');

  const blankCred = (): CredForm => ({ id: '', tenant_id: '', user_id: '', outlet_id: '', label: '', ips: '', rate: 120, expires: '', active: true });

  async function pickTenant(id: string) {
    actors = { users: [], outlets: [] };
    if (credForm) {
      credForm.tenant_id = id;
      credForm.user_id = '';
      credForm.outlet_id = '';
    }
    if (!id) return;
    try {
      actors = await platformApi.extActors(id);
      if (credForm && actors.outlets.length === 1) credForm.outlet_id = actors.outlets[0].id;
    } catch (e) {
      formError = errorMessage(e);
    }
  }

  function openCreate() {
    ipDraft = '';
    ipDraftBadNow = false;
    formError = '';
    credForm = blankCred();
    actors = { users: [], outlets: [] };
  }
  function openEdit(c: ExtCredential) {
    ipDraft = '';
    ipDraftBadNow = false;
    formError = '';
    credForm = {
      id: c.id, tenant_id: c.tenant_id, user_id: c.user_id, outlet_id: c.outlet_id, label: c.label, ips: c.ip_allowlist.join('\n'),
      rate: c.rate_limit_min, expires: c.expires_at ? c.expires_at.slice(0, 10) : '', active: c.active
    };
  }

  const parseIps = (s: string) => s.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean);
  const endOfDay = (d: string) => (d ? new Date(`${d}T23:59:59`).toISOString() : null);

  let created = $state<ExtCreated | null>(null);
  let copied = $state(false);

  async function saveCred() {
    if (!credForm) return;
    saving = true;
    formError = '';
    try {
      const ips = parseIps(credForm.ips);
      if (credForm.id) {
        await platformApi.extUpdateCredential(credForm.id, { label: credForm.label, ip_allowlist: ips, rate_limit_min: credForm.rate, active: credForm.active, expires_at: endOfDay(credForm.expires) });
        notice = t('extapi.admin.saved');
      } else {
        created = await platformApi.extCreateCredential({
          tenant_id: credForm.tenant_id, user_id: credForm.user_id, outlet_id: credForm.outlet_id, label: credForm.label,
          ip_allowlist: ips, rate_limit_min: credForm.rate, expires_at: endOfDay(credForm.expires)
        });
      }
      credForm = null;
      await loadCreds();
    } catch (e) {
      formError = e instanceof ApiError && e.status === 422 ? t('extapi.admin.cred.invalid') : errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function removeCred(c: ExtCredential) {
    confirmDlg = {
      title: t('extapi.admin.m.confirm.credTitle'),
      text: t('extapi.admin.m.confirm.credText', { label: c.label }),
      run: async () => {
        await platformApi.extDeleteCredential(c.id);
        await loadCreds();
      }
    };
  }

  let rotating = $state<ExtCredential | null>(null);
  let grace = $state('1440');
  async function doRotate() {
    if (!rotating) return;
    saving = true;
    try {
      created = await platformApi.extRotateCredential(rotating.id, Number(grace));
      rotating = null;
      await loadCreds();
    } catch (e) {
      error = errorMessage(e);
      rotating = null;
    } finally {
      saving = false;
    }
  }

  async function copySecret() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.secret);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch {
      /* clipboard diblokir: pengguna menyalin manual */
    }
  }

  // ---- white label ----
  let brands = $state<ExtBrand[]>([]);
  let groups = $state<ExtGroup[]>([]);
  async function loadBrands() {
    const r = await platformApi.extBrands();
    brands = r.items;
    groups = r.groups;
  }

  type BrandForm = ExtBrandInput & { id: string; domainText: string };
  let brandForm = $state<BrandForm | null>(null);
  let brandFields = $state<Record<string, string>>({});

  const blankBrand = (): BrandForm => ({
    id: '', slug: '', domain: null, domainText: '', is_default: false, active: true, name: '', tagline: '', logo_url: '', accent: '#06b6d4',
    support_email: '', support_url: '', footer: '', base_url: '', hidden_modules: [], sandbox_note: ''
  });
  function openBrand(b?: ExtBrand) {
    formError = '';
    brandFields = {};
    brandForm = b ? { ...b, domainText: b.domain ?? '' } : blankBrand();
  }
  function toggleHidden(id: string, on: boolean) {
    if (!brandForm) return;
    const set = new Set(brandForm.hidden_modules);
    if (on) set.add(id);
    else set.delete(id);
    brandForm.hidden_modules = [...set];
  }
  async function saveBrand() {
    if (!brandForm) return;
    saving = true;
    formError = '';
    brandFields = {};
    const { id, domainText, ...rest } = brandForm;
    const body: ExtBrandInput = { ...rest, domain: domainText.trim() || null };
    try {
      if (id) await platformApi.extUpdateBrand(id, body);
      else await platformApi.extCreateBrand(body);
      notice = t('extapi.admin.saved');
      brandForm = null;
      await loadBrands();
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        brandFields = e.fields;
        formError = t('extapi.admin.brand.invalid');
      } else if (e instanceof ApiError && e.status === 409) {
        formError = t('extapi.admin.brand.taken');
      } else {
        formError = errorMessage(e);
      }
    } finally {
      saving = false;
    }
  }
  async function removeBrand(b: ExtBrand) {
    confirmDlg = {
      title: t('extapi.admin.m.confirm.brandTitle'),
      text: t('extapi.admin.m.confirm.brandText', { name: b.name }),
      run: async () => {
        await platformApi.extDeleteBrand(b.id);
        await loadBrands();
      }
    };
  }

  $effect(() => {
    void loadAll();
  });
  $effect(() => {
    void filterTenant;
    if (!loading) loadCreds().catch((e) => (error = errorMessage(e)));
  });

  // ---- tampilan ----
  const reduce = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  const dur = (ms: number) => (reduce ? 0 : ms);
  const DAY = 86_400_000;
  type CredState = { ok: boolean; label: string; expired: boolean; warn: boolean };
  function credState(c: ExtCredential): CredState {
    const exp = c.expires_at ? new Date(c.expires_at).getTime() : null;
    const expired = exp !== null && exp <= Date.now();
    const soon = exp !== null && !expired && exp - Date.now() <= 14 * DAY;
    if (expired) return { ok: false, label: t('extapi.admin.cred.expired'), expired, warn: true };
    if (!c.active) return { ok: false, label: t('extapi.admin.inactive'), expired, warn: false };
    return { ok: true, label: t('extapi.admin.active'), expired, warn: soon };
  }
  const stats = $derived.by(() => {
    let active = 0, rotatingN = 0, expiring = 0, idle = 0;
    const tn = new Set<string>();
    for (const c of creds) {
      const st = credState(c);
      tn.add(c.tenant_id);
      if (st.ok) active++;
      if (c.rotating) rotatingN++;
      if (st.ok && st.warn) expiring++;
      if (st.ok && !c.last_used_at) idle++;
    }
    return { active, rotating: rotatingN, expiring, idle, tenants: tn.size };
  });
  let copiedKey = $state('');
  async function copyKeyId(k: string) {
    try {
      await navigator.clipboard.writeText(k);
      copiedKey = k;
      setTimeout(() => (copiedKey = ''), 1400);
    } catch {
      /* clipboard diblokir */
    }
  }
  const introSteps = [
    { icon: 'key-round', title: 'extapi.admin.intro.s1t', text: 'extapi.admin.intro.s1d' },
    { icon: 'shield-check', title: 'extapi.admin.intro.s2t', text: 'extapi.admin.intro.s2d' },
    { icon: 'signature', title: 'extapi.admin.intro.s3t', text: 'extapi.admin.intro.s3d' },
    { icon: 'activity', title: 'extapi.admin.intro.s4t', text: 'extapi.admin.intro.s4d' }
  ] as const;

  // ---- popup ----
  const validIp = (v: string) => {
    const [addr, mask, extra] = v.split('/');
    if (extra !== undefined) return false;
    const v4 = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/.test(addr);
    const v6 = addr.includes(':') && /^[0-9a-fA-F:.]+$/.test(addr) && addr.length >= 2;
    if (!v4 && !v6) return false;
    if (mask === undefined) return true;
    const n = Number(mask);
    return /^\d{1,3}$/.test(mask) && n >= 0 && n <= (v4 ? 32 : 128);
  };
  const pad2 = (n: number) => String(n).padStart(2, '0');
  const inDays = (d: number) => {
    if (d === 0) return '';
    const x = new Date(Date.now() + d * DAY);
    return `${x.getFullYear()}-${pad2(x.getMonth() + 1)}-${pad2(x.getDate())}`;
  };
  let ack = $state(false);
  let showSecret = $state(false);
  let copiedWhat = $state('');
  async function copy(what: string, text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copiedWhat = what;
      setTimeout(() => (copiedWhat = ''), 1400);
    } catch {
      /* clipboard diblokir: pengguna menyalin manual */
    }
  }
  function closeCreated() {
    created = null;
    ack = false;
    showSecret = false;
    copiedWhat = '';
  }
  const graceOptions = [
    { v: '0', label: 'extapi.admin.m.rotate.none', desc: 'extapi.admin.m.rotate.noneD', rec: false },
    { v: '60', label: 'extapi.admin.m.rotate.h1', desc: 'extapi.admin.m.rotate.h1D', rec: false },
    { v: '1440', label: 'extapi.admin.m.rotate.h24', desc: 'extapi.admin.m.rotate.h24D', rec: true },
    { v: '10080', label: 'extapi.admin.m.rotate.d7', desc: 'extapi.admin.m.rotate.d7D', rec: false }
  ] as const;
  let ipDraft = $state('');
  function addIps() {
    if (!credForm) return;
    const incoming = parseIps(ipDraft);
    if (incoming.length === 0) return;
    const bad = incoming.find((x) => !validIp(x));
    if (bad) {
      ipDraftBadNow = true;
      return;
    }
    ipDraftBadNow = false;
    const merged = [...new Set([...parseIps(credForm.ips), ...incoming])];
    credForm.ips = merged.join('\n');
    ipDraft = '';
  }
  let ipDraftBadNow = $state(false);
  function removeIp(ip: string) {
    if (credForm) credForm.ips = parseIps(credForm.ips).filter((x) => x !== ip).join('\n');
  }
  let confirmDlg = $state<{ title: string; text: string; run: () => Promise<void> } | null>(null);
  async function runConfirm() {
    if (!confirmDlg || saving) return;
    saving = true;
    try {
      await confirmDlg.run();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
      confirmDlg = null;
    }
  }

  const fieldCls = (f: string) => (brandFields[f] ? '!border-[var(--color-danger-500)]' : '');
</script>

<svelte:head><title>{t('extapi.admin.title')} | ACIRABA</title></svelte:head>

<div class="space-y-6 api-page">
  <!-- Judul -->
  <div class="flex flex-wrap items-start justify-between gap-4" in:fly={{ y: 10, duration: dur(350), easing: cubicOut }}>
    <div class="flex items-start gap-3.5">
      <span class="grid size-12 shrink-0 place-items-center rounded-2xl text-white shadow-sm" style="background: linear-gradient(140deg, var(--color-primary-500), var(--color-primary-800))"><i class="icon-plug text-[22px]"></i></span>
      <div>
        <h1 class="font-display font-bold text-[22px] tracking-tight">{t('extapi.admin.title')}</h1>
        <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('extapi.admin.subtitle')}</p>
      </div>
    </div>
    <a href="/developers" target="_blank" rel="noopener" class="btn btn-outline !text-[12.5px]"><i class="icon-external-link text-[13px]"></i>{t('extapi.admin.portalLink')}</a>
  </div>

  <!-- Ringkasan -->
  <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
    {#each [
      { label: t('extapi.admin.kpi.total'), value: formatNumber(creds.length), icon: 'key-round', tone: 'primary', hint: t('extapi.admin.kpi.totalHint', { count: formatNumber(stats.tenants) }) },
      { label: t('extapi.admin.kpi.active'), value: formatNumber(stats.active), icon: 'shield-check', tone: 'success', hint: t('extapi.admin.kpi.activeHint', { count: formatNumber(stats.rotating) }) },
      { label: t('extapi.admin.kpi.attention'), value: formatNumber(stats.expiring + stats.idle), icon: 'triangle-alert', tone: stats.expiring + stats.idle > 0 ? 'warning' : 'info', hint: t('extapi.admin.kpi.attentionHint', { expiring: formatNumber(stats.expiring), idle: formatNumber(stats.idle) }) },
      { label: t('extapi.admin.kpi.brands'), value: formatNumber(brands.length), icon: 'palette', tone: 'info', hint: t('extapi.admin.kpi.brandsHint', { count: formatNumber(brands.filter((b) => b.active).length) }) }
    ] as k, i (k.icon)}
      <div in:fly={{ y: 12, duration: dur(380), delay: dur(60 + i * 70), easing: cubicOut }}>
        <StatCard label={k.label} value={k.value} icon={k.icon} tone={k.tone as 'primary' | 'success' | 'warning' | 'info'} hint={k.hint} />
      </div>
    {/each}
  </div>

  <!-- Cara kerja -->
  <section class="surface-card p-5 sm:p-6" in:fly={{ y: 12, duration: dur(400), delay: dur(280), easing: cubicOut }}>
    <h2 class="font-display font-bold text-[14px] mb-4 flex items-center gap-2"><i class="icon-route text-[16px] text-[var(--text-tertiary)]"></i>{t('extapi.admin.intro.title')}</h2>
    <ol class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
      {#each introSteps as s, i (s.icon)}
        <li class="flex gap-3">
          <span class="relative grid size-10 shrink-0 place-items-center rounded-xl bg-[var(--color-primary-50)] text-[var(--color-primary-600)]">
            <i class="icon-{s.icon} text-[18px]"></i>
            <span class="absolute -top-1.5 -end-1.5 grid size-4 place-items-center rounded-full bg-[var(--color-primary-600)] text-[9.5px] font-bold text-white">{i + 1}</span>
          </span>
          <div>
            <h3 class="text-[13px] font-semibold">{t(s.title)}</h3>
            <p class="mt-0.5 text-[12px] text-[var(--text-tertiary)]">{t(s.text)}</p>
          </div>
        </li>
      {/each}
    </ol>
  </section>

  <!-- Tab -->
  <div class="flex gap-1 border-b border-[var(--border-subtle)]" role="tablist">
    {#each [['credentials', 'extapi.admin.tabs.credentials', creds.length], ['branding', 'extapi.admin.tabs.branding', brands.length]] as const as [id, key, n] (id)}
      <button
        type="button"
        role="tab"
        aria-selected={tab === id}
        class="px-4 py-2.5 text-[13px] font-semibold -mb-px border-b-2 transition-colors flex items-center gap-2 {tab === id
          ? 'border-[var(--color-primary-600)] text-[var(--color-primary-600)]'
          : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-primary)]'}"
        onclick={() => (tab = id)}
      >
        {t(key)}
        <span class="rounded-full bg-[var(--surface-sunken)] px-2 py-0.5 text-[10.5px] font-bold tabular-nums">{n}</span>
      </button>
    {/each}
  </div>

  {#if notice}
    <div transition:fly={{ y: -6, duration: dur(200) }} class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]">{notice}</div>
  {/if}
  {#if error}
    <div transition:fly={{ y: -6, duration: dur(200) }} class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]" role="alert">{error}</div>
  {/if}

  {#if loading}
    <div class="space-y-3" aria-busy="true">
      {#each [0, 1, 2] as i (i)}<div class="h-16 rounded-xl bg-[var(--surface-base)] animate-pulse"></div>{/each}
    </div>
  {:else if tab === 'credentials'}
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="w-72 max-w-full">
        <Select
          bind:value={filterTenant}
          ariaLabel={t('extapi.admin.cred.tenant')}
          options={[{ value: '', label: t('extapi.admin.cred.filterTenant') }, ...tenants.map((x) => ({ value: x.id, label: x.name }))]}
        />
      </div>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}><i class="icon-plus text-[13px]"></i>{t('extapi.admin.cred.add')}</button>
    </div>

    {#if creds.length === 0}
      <div class="surface-card p-10 text-center" in:fade={{ duration: dur(250) }}>
        <span class="mx-auto grid size-14 place-items-center rounded-2xl bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-key-round text-[26px]"></i></span>
        <p class="mt-3 text-[13.5px] font-semibold">{t('extapi.admin.cred.empty')}</p>
        <p class="mt-1 text-[12px] text-[var(--text-tertiary)]">{t('extapi.admin.cred.emptyHint')}</p>
      </div>
    {:else}
      <div class="surface-card !p-0 overflow-x-auto scroll-thin">
        <table class="w-full text-[13px] min-w-[860px]">
          <thead>
            <tr class="text-[11px] uppercase tracking-wider text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
              <th class="px-5 py-3 text-start font-semibold">{t('extapi.admin.cred.label')}</th>
              <th class="px-5 py-3 text-start font-semibold">{t('extapi.admin.cred.keyId')}</th>
              <th class="px-5 py-3 text-start font-semibold">{t('extapi.admin.cred.ips')}</th>
              <th class="px-5 py-3 text-start font-semibold">{t('extapi.admin.cred.lastUsed')}</th>
              <th class="px-5 py-3 text-start font-semibold">{t('extapi.admin.cred.status')}</th>
              <th class="px-5 py-3"></th>
            </tr>
          </thead>
          <tbody>
            {#each creds as c, i (c.id)}
              {@const st = credState(c)}
              <tr class="cred-row border-t border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)] align-top" in:fly={{ y: 8, duration: dur(300), delay: dur(Math.min(i, 8) * 40), easing: cubicOut }}>
                <td class="px-5 py-4">
                  <div class="flex items-start gap-3">
                    <Avatar name={c.tenant_name} size={34} />
                    <div class="min-w-0">
                      <div class="font-semibold">{c.label}</div>
                      <div class="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11.5px] text-[var(--text-tertiary)]">
                        <span class="inline-flex items-center gap-1"><i class="icon-store text-[11px]"></i>{c.tenant_name}</span>
                        <span aria-hidden="true">·</span>
                        <span class="inline-flex items-center gap-1"><i class="icon-user text-[11px]"></i>{c.user_name}</span>
                        <span aria-hidden="true">·</span>
                        <span class="inline-flex items-center gap-1"><i class="icon-map-pin text-[11px]"></i>{c.outlet_name}</span>
                      </div>
                    </div>
                  </div>
                </td>
                <td class="px-5 py-4">
                  <button type="button" class="key-chip font-mono text-[11.5px] rounded-md border border-[var(--border-subtle)] bg-[var(--surface-sunken)] px-2 py-1 inline-flex items-center gap-1.5" title={t('extapi.admin.cred.copyKeyId')} onclick={() => copyKeyId(c.key_id)}>
                    {c.key_id}<i class="{copiedKey === c.key_id ? 'icon-check text-[var(--color-success-600)]' : 'icon-copy text-[var(--text-tertiary)]'} text-[12px]"></i>
                  </button>
                </td>
                <td class="px-5 py-4">
                  <div class="flex flex-wrap gap-1 max-w-[260px]">
                    {#each c.ip_allowlist as ip (ip)}<span class="font-mono text-[11px] px-1.5 py-0.5 rounded bg-[var(--surface-sunken)] border border-[var(--border-subtle)]">{ip}</span>{/each}
                  </div>
                  <div class="text-[11px] text-[var(--text-tertiary)] mt-1.5 inline-flex items-center gap-1"><i class="icon-gauge text-[12px]"></i>{t('extapi.admin.cred.perMin', { count: formatNumber(c.rate_limit_min) })}</div>
                </td>
                <td class="px-5 py-4 text-[var(--text-secondary)]">
                  {#if c.last_used_at}
                    <span title={formatDateTime(c.last_used_at)}>{relativeTime(c.last_used_at)}</span>
                    <div class="text-[11px] font-mono text-[var(--text-tertiary)]">{c.last_used_ip}</div>
                  {:else}<span class="text-[var(--text-tertiary)]">{t('extapi.admin.cred.never')}</span>{/if}
                </td>
                <td class="px-5 py-4">
                  <div class="flex flex-wrap items-center gap-1">
                    <StatusPill ok={st.ok} label={st.label} />
                    {#if c.rotating}<span class="badge-info">{t('extapi.admin.cred.rotating')}</span>{/if}
                  </div>
                  <div class="mt-1.5 text-[11px] {st.warn ? 'text-[var(--color-warning-600)] font-semibold' : 'text-[var(--text-tertiary)]'}">
                    {#if c.expires_at}{st.expired ? t('extapi.admin.cred.expired') : t('extapi.admin.cred.expiresIn', { when: relativeTime(c.expires_at) })}{:else}{t('extapi.admin.cred.noExpiry')}{/if}
                  </div>
                </td>
                <td class="px-5 py-4 text-end whitespace-nowrap">
                  <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => openEdit(c)}>{t('extapi.admin.edit')}</button>
                  <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => ((grace = '1440'), (rotating = c))}>{t('extapi.admin.cred.rotate')}</button>
                  <button type="button" class="btn btn-outline !text-[12px] !py-1 text-[var(--color-danger-600)]" onclick={() => removeCred(c)} aria-label={t('extapi.admin.delete')}><i class="icon-trash-2 text-[13px]"></i></button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {:else}
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="text-[12.5px] text-[var(--text-tertiary)] max-w-xl">{t('extapi.admin.brand.intro')}</p>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => openBrand()}><i class="icon-plus text-[13px]"></i>{t('extapi.admin.brand.add')}</button>
    </div>
    {#if brands.length === 0}
      <div class="surface-card p-10 text-center text-[13px] text-[var(--text-tertiary)]">{t('extapi.admin.brand.empty')}</div>
    {:else}
      <div class="grid gap-5 md:grid-cols-2">
        {#each brands as b, i (b.id)}
          <div class="brand-card surface-card overflow-hidden" in:fly={{ y: 14, duration: dur(380), delay: dur(i * 80), easing: cubicOut }}>
            <div class="h-1.5" style="background:{b.accent}"></div>
            <div class="p-5 space-y-3.5">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-3 min-w-0">
                  {#if b.logo_url}
                    <img src={b.logo_url} alt="" class="size-10 rounded-lg object-contain bg-[var(--surface-sunken)]" />
                  {:else}
                    <span class="grid place-items-center size-10 rounded-lg font-bold text-white text-[15px]" style="background:{b.accent}">{b.name.slice(0, 1).toUpperCase()}</span>
                  {/if}
                  <div class="min-w-0">
                    <div class="font-semibold truncate">{b.name}</div>
                    <div class="text-[11.5px] text-[var(--text-tertiary)] truncate font-mono">{b.domain ?? b.slug}</div>
                  </div>
                </div>
                <div class="flex gap-1 shrink-0">
                  {#if b.is_default}<span class="badge-info">{t('extapi.admin.brand.default')}</span>{/if}
                  {#if !b.active}<span class="badge-warning">{t('extapi.admin.inactive')}</span>{/if}
                </div>
              </div>
              {#if b.tagline}<p class="text-[12.5px] text-[var(--text-secondary)]">{b.tagline}</p>{/if}
              <dl class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11.5px]">
                <dt class="text-[var(--text-tertiary)]">{t('extapi.admin.brand.accentLabel')}</dt>
                <dd class="font-mono flex items-center gap-1.5"><span class="size-3 rounded-full border border-black/10" style="background:{b.accent}"></span>{b.accent}</dd>
                {#if b.support_email}
                  <dt class="text-[var(--text-tertiary)]">{t('extapi.admin.brand.supportLabel')}</dt>
                  <dd class="truncate">{b.support_email}</dd>
                {/if}
              </dl>
              {#if b.hidden_modules.length}
                <div>
                  <p class="text-[11px] text-[var(--text-tertiary)] mb-1">{t('extapi.admin.brand.hiddenLabel', { count: formatNumber(b.hidden_modules.length) })}</p>
                  <div class="flex flex-wrap gap-1">
                    {#each b.hidden_modules as m (m)}<span class="font-mono text-[10.5px] px-1.5 py-0.5 rounded bg-[var(--surface-sunken)] line-through text-[var(--text-tertiary)]">{m}</span>{/each}
                  </div>
                </div>
              {/if}
              <div class="flex justify-end gap-2 pt-1">
                <button type="button" class="btn btn-outline !text-[12px] !py-1" onclick={() => openBrand(b)}>{t('extapi.admin.edit')}</button>
                {#if !b.is_default}
                  <button type="button" class="btn btn-outline !text-[12px] !py-1 text-[var(--color-danger-600)]" onclick={() => removeBrand(b)}>{t('extapi.admin.delete')}</button>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>

{#if credForm}
  {@const ipList = parseIps(credForm.ips)}
  {@const badIps = ipList.filter((x) => !validIp(x))}
  {@const sel = actors.users.find((u) => u.id === credForm!.user_id)}
  {@const selOutlet = actors.outlets.find((o) => o.id === credForm!.outlet_id)}
  <Modal title={credForm.id ? t('extapi.admin.cred.editTitle') : t('extapi.admin.cred.createTitle')} onclose={() => (credForm = null)} wide>
    <div class="mhead">
      <span class="mhead-ico"><i class="icon-{credForm.id ? 'pencil' : 'key-round'}"></i></span>
      <p>{credForm.id ? t('extapi.admin.m.cred.editSub') : t('extapi.admin.m.cred.createSub')}</p>
    </div>
    <form
      class="space-y-5"
      novalidate
      onsubmit={(e) => {
        e.preventDefault();
        void saveCred();
      }}
    >
      {#if formError}<div class="mnote mnote-danger" role="alert"><i class="icon-circle-alert"></i>{formError}</div>{/if}

      <section class="msec">
        <h3 class="msec-title"><span class="msec-num">1</span>{t('extapi.admin.m.cred.secIdentity')}</h3>
        <label for="c-label" class="mlabel">{t('extapi.admin.cred.label')}<b class="req">*</b></label>
        <input id="c-label" class="form-input !text-[13px] !w-full !max-w-none" bind:value={credForm.label} maxlength="100" placeholder={t('extapi.admin.cred.labelHint')} />
        <p class="mhint">{t('extapi.admin.m.cred.identityHint')}</p>
      </section>

      <section class="msec">
        <h3 class="msec-title"><span class="msec-num">2</span>{t('extapi.admin.m.cred.secAccess')}</h3>
        {#if credForm.id}
          {@const cur = creds.find((c) => c.id === credForm!.id)}
          {#if cur}
            <div class="mcard">
              <Avatar name={cur.tenant_name} size={36} />
              <div class="min-w-0 text-[12.5px]">
                <div class="font-semibold">{cur.tenant_name}</div>
                <div class="text-[var(--text-tertiary)]">{cur.user_name} · {cur.outlet_name}</div>
              </div>
              <span class="ms-auto font-mono text-[11.5px] rounded-md border border-[var(--border-subtle)] bg-[var(--surface-sunken)] px-2 py-1">{cur.key_id}</span>
            </div>
          {/if}
        {:else}
          <div class="grid gap-4 sm:grid-cols-3">
            <div>
              <span class="mlabel">{t('extapi.admin.cred.tenant')}<b class="req">*</b></span>
              <Select value={credForm.tenant_id} placeholder={t('extapi.admin.cred.chooseTenant')} onchange={(v) => void pickTenant(v)} options={tenants.map((x) => ({ value: x.id, label: x.name }))} />
            </div>
            <div>
              <span class="mlabel">{t('extapi.admin.cred.serviceUser')}<b class="req">*</b></span>
              <Select bind:value={credForm.user_id} placeholder={t('extapi.admin.cred.choose')} disabled={!credForm.tenant_id} options={actors.users.map((u) => ({ value: u.id, label: `${u.name} · ${u.role}`, hint: u.email }))} />
            </div>
            <div>
              <span class="mlabel">{t('extapi.admin.cred.outlet')}<b class="req">*</b></span>
              <Select bind:value={credForm.outlet_id} placeholder={t('extapi.admin.cred.choose')} disabled={!credForm.tenant_id} options={actors.outlets.map((o) => ({ value: o.id, label: o.name }))} />
            </div>
          </div>
          {#if sel && selOutlet}
            <div class="mnote mnote-info" transition:fly={{ y: -4, duration: dur(180) }}><i class="icon-shield-check"></i>{t('extapi.admin.m.cred.acting', { user: sel.name, role: sel.role, outlet: selOutlet.name })}</div>
          {:else}
            <p class="mhint">{t('extapi.admin.cred.serviceUserHint')}</p>
          {/if}
        {/if}
      </section>

      <section class="msec">
        <h3 class="msec-title"><span class="msec-num">3</span>{t('extapi.admin.m.cred.secSecurity')}</h3>
        <label for="c-ip-draft" class="mlabel">{t('extapi.admin.cred.ips')}<b class="req">*</b></label>
        <div class="ipbox {ipDraftBadNow ? 'bad' : ''}">
          <div class="ipbox-list">
            {#each ipList as ip (ip)}
              <span class="ipchip {validIp(ip) ? '' : 'bad'}" transition:scale={{ duration: dur(140), start: 0.85 }}>
                <i class="icon-{validIp(ip) ? 'shield-check' : 'triangle-alert'}"></i>{ip}
                <button type="button" class="ipchip-x" aria-label={t('extapi.admin.m.cred.removeIp', { ip })} onclick={() => removeIp(ip)}><i class="icon-x"></i></button>
              </span>
            {/each}
            {#if ipList.length === 0}<span class="ipbox-empty"><i class="icon-network"></i>{t('extapi.admin.m.cred.ipNone')}</span>{/if}
          </div>
          <div class="ipbox-add">
            <input
              id="c-ip-draft"
              class="ipbox-input"
              bind:value={ipDraft}
              placeholder={t('extapi.admin.m.cred.ipPlaceholder')}
              spellcheck="false"
              autocomplete="off"
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ',' || e.key === ' ') {
                  e.preventDefault();
                  addIps();
                }
              }}
              onpaste={(e) => {
                const txt = e.clipboardData?.getData('text') ?? '';
                if (/[\s,]/.test(txt.trim())) {
                  e.preventDefault();
                  ipDraft = txt;
                  addIps();
                }
              }}
            />
            <button type="button" class="btn btn-outline !text-[12px] !py-1.5 shrink-0" disabled={!ipDraft.trim()} onclick={addIps}><i class="icon-plus text-[13px]"></i>{t('extapi.admin.m.cred.addIp')}</button>
          </div>
        </div>
        {#if ipDraftBadNow}
          <p class="mhint !text-[var(--color-danger-600)]"><i class="icon-triangle-alert me-1"></i>{t('extapi.admin.m.cred.ipInvalid', { ip: ipDraft.trim() })}</p>
        {:else if badIps.length}
          <p class="mhint !text-[var(--color-danger-600)]"><i class="icon-triangle-alert me-1"></i>{t('extapi.admin.m.cred.ipInvalid', { ip: badIps[0] })}</p>
        {:else}
          <p class="mhint">{t('extapi.admin.m.cred.ipHelp')}{ipList.length ? ' · ' + t('extapi.admin.m.cred.ipCount', { count: formatNumber(ipList.length) }) : ''}</p>
        {/if}

        <div class="grid gap-5 sm:grid-cols-2 mt-4">
          <div>
            <label for="c-rate" class="mlabel">{t('extapi.admin.cred.rate')}</label>
            <input id="c-rate" type="number" min="1" max="100000" class="form-input !text-[13px]" bind:value={credForm.rate} />
            <div class="mt-2 flex flex-wrap gap-1.5">
              {#each [60, 120, 600, 1200] as r (r)}
                <button type="button" class="mpreset {credForm.rate === r ? 'on' : ''}" onclick={() => (credForm!.rate = r)}>{formatNumber(r)}</button>
              {/each}
            </div>
            <p class="mhint">{t('extapi.admin.m.cred.rateHint')}</p>
          </div>
          <div>
            <label for="c-exp" class="mlabel">{t('extapi.admin.cred.expires')}</label>
            <DatePicker id="c-exp" bind:value={credForm.expires} min={inDays(1)} placeholder={t('extapi.admin.m.cred.none')} />
            <div class="mt-2 flex flex-wrap gap-1.5">
              {#each [[30, 'extapi.admin.m.cred.d30'], [90, 'extapi.admin.m.cred.d90'], [365, 'extapi.admin.m.cred.y1'], [0, 'extapi.admin.m.cred.none']] as const as [d, k] (d)}
                <button type="button" class="mpreset {credForm.expires === inDays(d) ? 'on' : ''}" onclick={() => (credForm!.expires = inDays(d))}>{t(k)}</button>
              {/each}
            </div>
            <p class="mhint">{credForm.expires ? t('extapi.admin.m.cred.expHint', { date: formatDateTime(`${credForm.expires}T23:59:59`, { dateStyle: 'medium' }) }) : t('extapi.admin.m.cred.expNone')}</p>
          </div>
        </div>
      </section>

      {#if credForm.id}
        <section class="msec">
          <h3 class="msec-title"><span class="msec-num">4</span>{t('extapi.admin.m.cred.secStatus')}</h3>
          <div class="flex items-start gap-3">
            <Switch bind:checked={credForm.active} label={t('extapi.admin.active')} />
            <div>
              <div class="text-[13px] font-semibold">{credForm.active ? t('extapi.admin.active') : t('extapi.admin.inactive')}</div>
              <p class="mhint !mt-0.5">{t('extapi.admin.m.cred.activeHint')}</p>
            </div>
          </div>
        </section>
      {/if}

      <div class="mfoot">
        <span class="mhint !mt-0"><b class="req">*</b> {t('extapi.admin.m.required')}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (credForm = null)}>{t('extapi.admin.cancel')}</button>
          <button
            type="submit"
            class="btn btn-primary !text-[12.5px]"
            disabled={saving || !credForm.label.trim() || ipList.length === 0 || badIps.length > 0 || (!credForm.id && (!credForm.tenant_id || !credForm.user_id || !credForm.outlet_id))}
          >
            {saving ? t('extapi.admin.saving') : credForm.id ? t('extapi.admin.save') : t('extapi.admin.m.cred.issue')}
          </button>
        </div>
      </div>
    </form>
  </Modal>
{/if}

{#if rotating}
  <Modal title={t('extapi.admin.cred.rotateTitle')} onclose={() => (rotating = null)}>
    <div class="mhead">
      <span class="mhead-ico warn"><i class="icon-refresh-cw"></i></span>
      <p>{t('extapi.admin.m.rotate.sub', { label: rotating.label })}</p>
    </div>
    <div class="space-y-4">
      <div class="mcard text-[12.5px]">
        <span class="font-mono text-[11.5px] rounded-md border border-[var(--border-subtle)] bg-[var(--surface-sunken)] px-2 py-1">{rotating.key_id}</span>
        <span class="text-[var(--text-tertiary)] truncate">{rotating.tenant_name}</span>
      </div>
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('extapi.admin.cred.rotateText')}</p>
      <div class="mlabel !mb-0">{t('extapi.admin.m.rotate.graceTitle')}</div>
      <div class="grid gap-2" role="radiogroup" aria-label={t('extapi.admin.m.rotate.graceTitle')}>
        {#each graceOptions as g (g.v)}
          <button type="button" role="radio" aria-checked={grace === g.v} class="gopt {grace === g.v ? 'on' : ''}" onclick={() => (grace = g.v)}>
            <span class="gopt-dot"></span>
            <span class="min-w-0 text-start">
              <span class="block text-[13px] font-semibold">{t(g.label)}{#if g.rec}<span class="ms-2 rounded-full bg-[var(--color-success-500)]/15 px-2 py-0.5 text-[10px] font-bold uppercase text-[var(--color-success-600)]">{t('extapi.admin.m.rotate.recommended')}</span>{/if}</span>
              <span class="block text-[11.5px] text-[var(--text-tertiary)]">{t(g.desc)}</span>
            </span>
          </button>
        {/each}
      </div>
      {#if grace === '0'}
        <div class="mnote mnote-warn" transition:fly={{ y: -4, duration: dur(180) }}><i class="icon-triangle-alert"></i>{t('extapi.admin.m.rotate.warnNone')}</div>
      {/if}
      <div class="mfoot">
        <span class="mhint !mt-0">{t('extapi.admin.m.rotate.afterNote')}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (rotating = null)}>{t('extapi.admin.cancel')}</button>
          <button type="button" class="btn btn-primary !text-[12.5px]" disabled={saving} onclick={doRotate}><i class="icon-refresh-cw text-[13px]"></i>{saving ? t('extapi.admin.saving') : t('extapi.admin.cred.rotate')}</button>
        </div>
      </div>
    </div>
  </Modal>
{/if}

{#if created}
  <Modal title={t('extapi.admin.cred.revealTitle')} onclose={() => ack && closeCreated()}>
    <div class="space-y-4 text-center">
      <span class="success-ring mx-auto grid size-14 place-items-center rounded-full bg-[var(--color-success-500)]/15 text-[var(--color-success-600)]" in:scale={{ duration: dur(350), start: 0.7, easing: cubicOut }}><i class="icon-circle-check text-[30px]"></i></span>
      <div>
        <p class="text-[14px] font-semibold">{t('extapi.admin.m.reveal.sub')}</p>
        <p class="text-[12px] text-[var(--text-tertiary)] mt-0.5">{t('extapi.admin.m.reveal.hint')}</p>
      </div>
    </div>
    <div class="mt-4 space-y-4">
      <div class="mnote mnote-warn"><i class="icon-triangle-alert"></i><span><b>{t('extapi.admin.m.reveal.onceTitle')}</b> {t('extapi.admin.cred.revealText')}</span></div>

      <div>
        <div class="mlabel">{t('extapi.admin.cred.keyId')}</div>
        <div class="secret-row">
          <code>{created.key_id}</code>
          <button type="button" class="btn btn-outline !text-[12px]" onclick={() => copy('key', created!.key_id)}><i class="icon-{copiedWhat === 'key' ? 'check' : 'copy'} text-[13px]"></i>{copiedWhat === 'key' ? t('extapi.admin.cred.copied') : t('extapi.admin.cred.copy')}</button>
        </div>
      </div>
      <div>
        <div class="mlabel">{t('extapi.admin.cred.secret')}</div>
        <div class="secret-row secret-dark">
          <code>{showSecret ? created.secret : '•'.repeat(Math.min(created.secret.length, 40))}</code>
          <button type="button" class="btn btn-outline !text-[12px]" onclick={() => (showSecret = !showSecret)} aria-label={showSecret ? t('extapi.admin.m.reveal.hide') : t('extapi.admin.m.reveal.show')}><i class="icon-{showSecret ? 'eye-off' : 'eye'} text-[13px]"></i></button>
          <button type="button" class="btn btn-outline !text-[12px]" onclick={() => copy('secret', created!.secret)}><i class="icon-{copiedWhat === 'secret' ? 'check' : 'copy'} text-[13px]"></i>{copiedWhat === 'secret' ? t('extapi.admin.cred.copied') : t('extapi.admin.cred.copy')}</button>
        </div>
      </div>
      <button type="button" class="w-full btn btn-outline !text-[12.5px]" onclick={() => copy('env', `ARUS_API_KEY_ID=${created!.key_id}\nARUS_API_SECRET=${created!.secret}`)}>
        <i class="icon-{copiedWhat === 'env' ? 'check' : 'file-code'} text-[14px]"></i>{copiedWhat === 'env' ? t('extapi.admin.cred.copied') : t('extapi.admin.m.reveal.copyEnv')}
      </button>

      <label class="flex items-start gap-2.5 rounded-lg border border-[var(--border-subtle)] p-3 text-[12.5px] cursor-pointer {ack ? 'bg-[var(--color-success-500)]/8' : ''}">
        <input type="checkbox" class="form-checkbox mt-0.5" bind:checked={ack} />
        <span>{t('extapi.admin.m.reveal.ack')}</span>
      </label>
      <div class="flex justify-end"><button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-50" disabled={!ack} onclick={closeCreated}>{t('extapi.admin.cred.done')}</button></div>
    </div>
  </Modal>
{/if}

{#if confirmDlg}
  <Modal title={confirmDlg.title} onclose={() => (confirmDlg = null)}>
    <div class="space-y-4">
      <div class="mhead !mb-0">
        <span class="mhead-ico danger"><i class="icon-trash-2"></i></span>
        <p class="!text-[var(--text-primary)]">{confirmDlg.text}</p>
      </div>
      <div class="mnote mnote-danger"><i class="icon-triangle-alert"></i>{t('extapi.admin.m.confirm.irreversible')}</div>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (confirmDlg = null)}>{t('extapi.admin.cancel')}</button>
        <button type="button" class="btn !text-[12.5px] btn-danger-solid" disabled={saving} onclick={runConfirm}><i class="icon-trash-2 text-[13px]"></i>{t('extapi.admin.m.confirm.yes')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if brandForm}
  {@const hiddenN = brandForm.hidden_modules.length}
  <Modal title={brandForm.id ? t('extapi.admin.brand.editTitle') : t('extapi.admin.brand.createTitle')} onclose={() => (brandForm = null)} xl>
    <div class="mhead">
      <span class="mhead-ico"><i class="icon-palette"></i></span>
      <p>{brandForm.id ? t('extapi.admin.m.brand.editSub') : t('extapi.admin.m.brand.createSub')}</p>
    </div>
    <form
      novalidate
      onsubmit={(e) => {
        e.preventDefault();
        void saveBrand();
      }}
    >
      {#if formError}<div class="mnote mnote-danger mb-4" role="alert"><i class="icon-circle-alert"></i>{formError}</div>{/if}
      <div class="grid gap-6 lg:grid-cols-[1fr_300px]">
        <div class="space-y-5 min-w-0">
          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">1</span>{t('extapi.admin.m.brand.secIdentity')}</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="b-name" class="mlabel">{t('extapi.admin.brand.name')}<b class="req">*</b></label>
                <input id="b-name" class="form-input !text-[13px] {fieldCls('name')}" bind:value={brandForm.name} maxlength="80" />
              </div>
              <div>
                <label for="b-slug" class="mlabel">{t('extapi.admin.brand.slug')}<b class="req">*</b></label>
                <input id="b-slug" class="form-input !text-[13px] font-mono {fieldCls('slug')}" bind:value={brandForm.slug} maxlength="41" placeholder="mitra-1" />
              </div>
              <div class="sm:col-span-2">
                <label for="b-tag" class="mlabel">{t('extapi.admin.brand.tagline')}</label>
                <input id="b-tag" class="form-input !text-[13px] {fieldCls('tagline')}" bind:value={brandForm.tagline} maxlength="200" />
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">2</span>{t('extapi.admin.m.brand.secDomain')}</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="b-domain" class="mlabel">{t('extapi.admin.brand.domain')}</label>
                <input id="b-domain" class="form-input !text-[13px] font-mono {fieldCls('domain')}" bind:value={brandForm.domainText} placeholder="docs.mitra.com" />
                <p class="mhint">{t('extapi.admin.brand.domainHint')}</p>
              </div>
              <div>
                <label for="b-base" class="mlabel">{t('extapi.admin.brand.baseUrl')}</label>
                <input id="b-base" class="form-input !text-[13px] font-mono {fieldCls('base_url')}" bind:value={brandForm.base_url} placeholder="https://api.mitra.com" />
                <p class="mhint">{t('extapi.admin.brand.baseUrlHint')}</p>
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">3</span>{t('extapi.admin.m.brand.secAppearance')}</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="b-accent" class="mlabel">{t('extapi.admin.brand.accent')}</label>
                <div class="flex items-center gap-2">
                  <input id="b-accent-pick" type="color" class="h-9 w-12 rounded border border-[var(--border-subtle)] bg-transparent p-0.5" bind:value={brandForm.accent} aria-label={t('extapi.admin.brand.accent')} />
                  <input id="b-accent" class="form-input !text-[13px] font-mono {fieldCls('accent')}" bind:value={brandForm.accent} maxlength="7" />
                </div>
                <div class="mt-2 flex flex-wrap gap-1.5">
                  {#each ['#06b6d4', '#2563eb', '#7c3aed', '#16a34a', '#ea580c', '#e11d48'] as c (c)}
                    <button type="button" class="size-6 rounded-full border-2 {brandForm.accent.toLowerCase() === c ? 'border-[var(--text-primary)]' : 'border-transparent'}" style="background:{c}" aria-label={c} onclick={() => (brandForm!.accent = c)}></button>
                  {/each}
                </div>
              </div>
              <div>
                <label for="b-logo" class="mlabel">{t('extapi.admin.brand.logo')}</label>
                <input id="b-logo" class="form-input !text-[13px] font-mono {fieldCls('logo_url')}" bind:value={brandForm.logo_url} placeholder="https://…/logo.svg" />
                <p class="mhint">{t('extapi.admin.brand.logoHint')}</p>
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title"><span class="msec-num">4</span>{t('extapi.admin.m.brand.secSupport')}</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label for="b-mail" class="mlabel">{t('extapi.admin.brand.supportEmail')}</label>
                <input id="b-mail" class="form-input !text-[13px] {fieldCls('support_email')}" bind:value={brandForm.support_email} />
              </div>
              <div>
                <label for="b-url" class="mlabel">{t('extapi.admin.brand.supportUrl')}</label>
                <input id="b-url" class="form-input !text-[13px] {fieldCls('support_url')}" bind:value={brandForm.support_url} />
              </div>
              <div class="sm:col-span-2">
                <label for="b-foot" class="mlabel">{t('extapi.admin.brand.footer')}</label>
                <input id="b-foot" class="form-input !text-[13px] {fieldCls('footer')}" bind:value={brandForm.footer} maxlength="500" />
              </div>
              <div class="sm:col-span-2">
                <label for="b-sand" class="mlabel">{t('extapi.admin.brand.sandboxNote')}</label>
                <textarea id="b-sand" class="form-input !text-[13px] {fieldCls('sandbox_note')}" rows="2" bind:value={brandForm.sandbox_note}></textarea>
              </div>
            </div>
          </section>

          <section class="msec">
            <h3 class="msec-title">
              <span class="msec-num">5</span>{t('extapi.admin.brand.hidden')}
              <span class="ms-auto text-[11px] font-medium text-[var(--text-tertiary)] normal-case tracking-normal">{t('extapi.admin.m.brand.hiddenCount', { hidden: formatNumber(hiddenN), total: formatNumber(groups.length) })}</span>
            </h3>
            <p class="mhint !mt-0 mb-2">{t('extapi.admin.brand.hiddenHint')}</p>
            <div class="mb-2 flex gap-2">
              <button type="button" class="mpreset" onclick={() => (brandForm!.hidden_modules = [])}>{t('extapi.admin.m.brand.showAll')}</button>
              <button type="button" class="mpreset" onclick={() => (brandForm!.hidden_modules = groups.map((g) => g.id))}>{t('extapi.admin.m.brand.hideAll')}</button>
            </div>
            <div class="grid gap-x-4 gap-y-1.5 sm:grid-cols-2 max-h-44 overflow-y-auto scroll-thin rounded-lg border border-[var(--border-subtle)] p-3">
              {#each groups as g (g.id)}
                <label class="flex items-center gap-2 text-[12.5px]">
                  <input type="checkbox" class="form-checkbox" checked={brandForm.hidden_modules.includes(g.id)} onchange={(e) => toggleHidden(g.id, e.currentTarget.checked)} />
                  <span class="truncate {brandForm.hidden_modules.includes(g.id) ? 'line-through text-[var(--text-tertiary)]' : ''}">{g.title}</span>
                  <span class="ms-auto text-[10.5px] text-[var(--text-tertiary)] tabular-nums">{g.count}</span>
                </label>
              {/each}
            </div>
          </section>
        </div>

        <!-- Pratinjau -->
        <aside class="space-y-4 lg:sticky lg:top-0 self-start">
          <div class="mlabel !mb-0">{t('extapi.admin.m.brand.previewTitle')}</div>
          <div class="preview-card">
            <div class="h-1.5" style="background:{brandForm.accent}"></div>
            <div class="p-4 space-y-3">
              <div class="flex items-center gap-3">
                {#if brandForm.logo_url}
                  <img src={brandForm.logo_url} alt="" class="size-10 rounded-lg object-contain bg-[var(--surface-sunken)]" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')} />
                {:else}
                  <span class="grid place-items-center size-10 rounded-lg font-bold text-white" style="background:{brandForm.accent}">{(brandForm.name || '?').slice(0, 1).toUpperCase()}</span>
                {/if}
                <div class="min-w-0">
                  <div class="font-semibold truncate">{brandForm.name || t('extapi.admin.m.brand.namePlaceholder')}</div>
                  <div class="text-[11px] font-mono text-[var(--text-tertiary)] truncate">{brandForm.domainText || brandForm.slug || 'slug'}</div>
                </div>
              </div>
              {#if brandForm.tagline}<p class="text-[12px] text-[var(--text-secondary)]">{brandForm.tagline}</p>{/if}
              <span class="inline-block rounded-md px-3 py-1.5 text-[12px] font-bold text-white" style="background:{brandForm.accent}">{t('extapi.admin.m.brand.sampleButton')}</span>
              {#if brandForm.support_email}<p class="text-[11px] text-[var(--text-tertiary)]"><i class="icon-mail me-1"></i>{brandForm.support_email}</p>{/if}
            </div>
          </div>
          <div class="space-y-3 rounded-xl border border-[var(--border-subtle)] p-4">
            <div class="flex items-start gap-3">
              <Switch bind:checked={brandForm.active} label={t('extapi.admin.active')} />
              <div>
                <div class="text-[13px] font-semibold">{t('extapi.admin.active')}</div>
                <p class="mhint !mt-0.5">{t('extapi.admin.m.brand.activeHint')}</p>
              </div>
            </div>
            <div class="flex items-start gap-3" title={t('extapi.admin.brand.isDefaultHint')}>
              <Switch bind:checked={brandForm.is_default} label={t('extapi.admin.brand.isDefault')} />
              <div>
                <div class="text-[13px] font-semibold">{t('extapi.admin.brand.isDefault')}</div>
                <p class="mhint !mt-0.5">{t('extapi.admin.brand.isDefaultHint')}</p>
              </div>
            </div>
          </div>
        </aside>
      </div>

      <div class="mfoot mt-5">
        <span class="mhint !mt-0"><b class="req">*</b> {t('extapi.admin.m.required')}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (brandForm = null)}>{t('extapi.admin.cancel')}</button>
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving || !brandForm.name.trim() || !brandForm.slug.trim()}>{saving ? t('extapi.admin.saving') : t('extapi.admin.save')}</button>
        </div>
      </div>
    </form>
  </Modal>
{/if}

<style>
  .cred-row { transition: background-color 0.2s; }
  .key-chip { transition: border-color 0.2s, transform 0.15s; cursor: pointer; }
  .key-chip:hover { border-color: var(--color-primary-400); }
  .key-chip:active { transform: scale(0.97); }
  .brand-card { transition: transform 0.25s ease, box-shadow 0.25s ease; }
  .brand-card:hover { transform: translateY(-3px); box-shadow: 0 12px 26px -14px rgb(15 23 42 / 0.28); }

  /* ---- popup ---- */
  .mhead { display: flex; align-items: center; gap: 0.85rem; margin-bottom: 1.1rem; padding: 0.85rem 1rem; border-radius: 0.85rem; background: var(--surface-sunken); }
  .mhead p { margin: 0; font-size: 12.5px; color: var(--text-secondary); }
  .mhead-ico { display: grid; place-items: center; flex: none; width: 2.5rem; height: 2.5rem; border-radius: 0.7rem; background: var(--color-primary-50); color: var(--color-primary-600); font-size: 18px; }
  .mhead-ico.warn { background: color-mix(in srgb, var(--color-warning-500) 16%, transparent); color: var(--color-warning-600); }
  .mhead-ico.danger { background: color-mix(in srgb, var(--color-danger-500) 14%, transparent); color: var(--color-danger-600); }
  .msec { padding: 1rem 1.1rem 1.1rem; border: 1px solid var(--border-subtle); border-radius: 0.85rem; }
  .msec-title { display: flex; align-items: center; gap: 0.6rem; margin-bottom: 0.85rem; font-size: 11.5px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-secondary); }
  .msec-num { display: grid; place-items: center; width: 1.3rem; height: 1.3rem; border-radius: 9999px; background: var(--color-primary-600); color: #fff; font-size: 10.5px; letter-spacing: 0; }
  .msec :global(input.form-input:not([type='color'])), .msec :global(textarea.form-input) { width: 100%; }
  .mlabel { display: block; margin-bottom: 0.4rem; font-size: 12.5px; font-weight: 600; }
  .req { margin-inline-start: 2px; color: var(--color-danger-500); }
  .mhint { margin-top: 0.4rem; font-size: 11.5px; color: var(--text-tertiary); }
  .mcard { display: flex; align-items: center; gap: 0.75rem; padding: 0.75rem 0.9rem; border: 1px solid var(--border-subtle); border-radius: 0.75rem; background: var(--surface-sunken); }
  .mnote { display: flex; align-items: flex-start; gap: 0.6rem; margin-top: 0.6rem; padding: 0.65rem 0.8rem; border-radius: 0.65rem; font-size: 12.5px; }
  .mnote i { margin-top: 2px; }
  .mnote-info { background: color-mix(in srgb, var(--color-info-500) 10%, transparent); color: var(--color-info-600); }
  .mnote-warn { background: color-mix(in srgb, var(--color-warning-500) 12%, transparent); border: 1px solid color-mix(in srgb, var(--color-warning-500) 35%, transparent); color: var(--text-primary); }
  .mnote-warn i { color: var(--color-warning-600); }
  .mnote-danger { background: color-mix(in srgb, var(--color-danger-500) 10%, transparent); color: var(--color-danger-600); }
  .mfoot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0.75rem; padding-top: 0.9rem; border-top: 1px solid var(--border-subtle); }
  .mpreset { padding: 0.2rem 0.65rem; border-radius: 9999px; border: 1px solid var(--border-subtle); font-size: 11.5px; font-weight: 600; color: var(--text-secondary); transition: all 0.15s; }
  .mpreset:hover { border-color: var(--color-primary-400); color: var(--color-primary-600); }
  .mpreset.on { background: var(--color-primary-600); border-color: var(--color-primary-600); color: #fff; }
  .ipchip { display: inline-flex; align-items: center; gap: 0.3rem; padding: 0.15rem 0.55rem; border-radius: 9999px; font: 600 11.5px ui-monospace, monospace; background: color-mix(in srgb, var(--color-success-500) 12%, transparent); color: var(--color-success-600); }
  .ipchip.bad { background: color-mix(in srgb, var(--color-danger-500) 12%, transparent); color: var(--color-danger-600); }
  .ipbox { border: 1px solid var(--border-subtle); border-radius: 0.75rem; background: var(--surface-base); transition: border-color 0.15s, box-shadow 0.15s; }
  .ipbox:focus-within { border-color: var(--color-primary-500); box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 18%, transparent); }
  .ipbox.bad { border-color: var(--color-danger-500); }
  .ipbox-list { display: flex; flex-wrap: wrap; gap: 0.4rem; min-height: 2.75rem; padding: 0.6rem 0.7rem; align-content: flex-start; }
  .ipbox-empty { display: inline-flex; align-items: center; gap: 0.45rem; font-size: 12px; color: var(--text-tertiary); }
  .ipbox-add { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.6rem; border-top: 1px dashed var(--border-subtle); background: var(--surface-sunken); border-radius: 0 0 0.75rem 0.75rem; }
  .ipbox-input { flex: 1; min-width: 0; padding: 0.4rem 0.5rem; border: 0 !important; outline: 0; background: transparent !important; font: 13px ui-monospace, monospace; color: var(--text-primary); box-shadow: none !important; }
  .ipchip { padding-inline-end: 0.2rem; }
  .ipchip-x { display: grid; place-items: center; width: 1.1rem; height: 1.1rem; margin-inline-start: 0.1rem; border-radius: 9999px; font-size: 10px; opacity: 0.7; transition: opacity 0.15s, background 0.15s; }
  .ipchip-x:hover { opacity: 1; background: color-mix(in srgb, currentColor 16%, transparent); }
  .gopt { display: flex; align-items: flex-start; gap: 0.75rem; padding: 0.7rem 0.9rem; border: 1px solid var(--border-subtle); border-radius: 0.75rem; transition: all 0.15s; }
  .gopt:hover { border-color: var(--color-primary-400); }
  .gopt.on { border-color: var(--color-primary-600); background: var(--color-primary-50); box-shadow: 0 0 0 1px var(--color-primary-600); }
  .gopt-dot { flex: none; width: 1rem; height: 1rem; margin-top: 2px; border-radius: 9999px; border: 2px solid var(--border-strong, #94a3b8); transition: all 0.15s; }
  .gopt.on .gopt-dot { border-color: var(--color-primary-600); background: radial-gradient(circle, var(--color-primary-600) 45%, transparent 50%); }
  .secret-row { display: flex; align-items: stretch; gap: 0.5rem; }
  .secret-row code { flex: 1; min-width: 0; padding: 0.55rem 0.8rem; border-radius: 0.6rem; border: 1px solid var(--border-subtle); background: var(--surface-sunken); font: 12.5px ui-monospace, monospace; word-break: break-all; text-align: start; }
  .secret-dark code { background: #0b1220; border-color: #1e293b; color: #e2e8f0; letter-spacing: 0.02em; }
  .preview-card { overflow: hidden; border: 1px solid var(--border-subtle); border-radius: 0.85rem; background: var(--surface-base); }
  .success-ring { animation: ring 1.4s ease-out 1; }
  @keyframes ring { 0% { box-shadow: 0 0 0 0 rgb(34 197 94 / 0.45); } 100% { box-shadow: 0 0 0 16px rgb(34 197 94 / 0); } }
  .btn-danger-solid { background: var(--color-danger-600); border-color: var(--color-danger-600); color: #fff; }
  .btn-danger-solid:hover { filter: brightness(1.08); }

  @media (prefers-reduced-motion: reduce) {
    .brand-card, .key-chip, .cred-row, .mpreset, .gopt, .gopt-dot { transition: none; }
    .brand-card:hover { transform: none; }
    .success-ring { animation: none; }
  }
</style>
