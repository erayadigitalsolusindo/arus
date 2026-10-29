<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type DomainRow } from '#lib/platform/api.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
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
  let form = $state({ tenantCode: '', host: '' });
  let saving = $state(false);
  let modalError = $state('');
  let lookup = $state<{ state: 'idle' | 'checking' | 'found' | 'missing'; id: string; name: string }>({ state: 'idle', id: '', name: '' });

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
    form = { tenantCode: '', host: '' };
    lookup = { state: 'idle', id: '', name: '' };
    modalError = '';
    showModal = true;
  }

  // Cari tenant berdasarkan kode (debounce), supaya salah ketik kode terlihat sebelum disimpan.
  let lookupSeq = 0;
  $effect(() => {
    const code = form.tenantCode.trim().toLowerCase();
    if (!showModal || !code) {
      lookup = { state: 'idle', id: '', name: '' };
      return;
    }
    lookup = { state: 'checking', id: '', name: '' };
    const my = ++lookupSeq;
    const timer = setTimeout(async () => {
      try {
        const found = (await platformApi.tenants(code, 10, 0)).items.find((x) => x.code === code);
        if (my === lookupSeq) lookup = found ? { state: 'found', id: found.id, name: found.name } : { state: 'missing', id: '', name: '' };
      } catch {
        if (my === lookupSeq) lookup = { state: 'idle', id: '', name: '' };
      }
    }, 350);
    return () => clearTimeout(timer);
  });

  async function save() {
    if (lookup.state !== 'found') return;
    saving = true;
    modalError = '';
    notice = '';
    error = '';
    try {
      await platformApi.createDomain({ tenant_id: lookup.id, host: form.host.trim() });
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
  <section class="rounded-2xl border border-[var(--border-subtle)] bg-gradient-to-br from-[#06122b] via-[#0a1f4a] to-[#0d2b63] text-white p-4 lg:p-5">
    <div class="flex flex-wrap items-start justify-between gap-5">
      <div class="min-w-0">
        <p class="flex items-center gap-2 text-[10.5px] font-bold uppercase tracking-[0.14em] text-sky-200"><i class="icon-terminal text-[13px]"></i>{t('platform.domains.targetTitle')}</p>
        {#if target}
          <div class="mt-2 flex flex-wrap items-center gap-3">
            <code class="font-mono text-[18px] lg:text-[20px] font-semibold text-sky-100 [text-shadow:0_0_14px_rgb(125_211_252/0.45)] break-all"><span class="text-emerald-300">$</span> {target}</code>
            {@render copyBtn(target, 'target')}
          </div>
          <p class="mt-1.5 text-[12px] text-sky-100/75">{t('platform.domains.targetHelp')} {t('platform.domains.cloudflareNote')}</p>
        {:else if !loading}
          <p class="mt-2 flex items-center gap-2 text-[14px] font-semibold text-amber-300"><i class="icon-circle-alert text-[16px]"></i>{t('platform.domains.noTarget')}</p>
          <p class="mt-1 text-[12px] text-sky-100/75 max-w-xl">{t('platform.domains.noTargetHelp')}</p>
        {/if}
      </div>
      <ol class="flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[11.5px]" aria-label={t('platform.domains.howTitle')}>
        {#each [t('platform.domains.how1'), t('platform.domains.how2'), t('platform.domains.how3'), t('platform.domains.how4')] as step, i (i)}
          {#if i > 0}<i class="icon-chevron-right text-[12px] text-sky-300/60" aria-hidden="true"></i>{/if}
          <li class="inline-flex items-center gap-1.5 rounded-full border border-white/15 bg-white/10 px-2.5 py-1">
            <span class="grid size-4 place-items-center rounded-full text-[10px] font-bold" style="background:#7dd3fc;color:#06122b">{i + 1}</span>{step}
          </li>
        {/each}
      </ol>
    </div>
  </section>

  <!-- Filter -->
  <div class="flex flex-wrap items-center gap-3">
    <div class="relative flex-1 min-w-60">
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
    <div class="surface-card px-4 py-14 text-center">
      <span class="mx-auto mb-3 grid size-12 place-items-center rounded-full bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-globe text-[22px]"></i></span>
      <p class="font-semibold">{t('platform.domains.emptyTitle')}</p>
      <p class="mt-1 text-[12.5px] text-[var(--text-tertiary)] max-w-md mx-auto">{t('platform.domains.emptyHint')}</p>
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
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true" aria-label={t('platform.domains.addTitle')}>
    <div class="surface-card w-full max-w-lg rounded-xl shadow-xl !p-0">
      <div class="flex items-center justify-between border-b border-[var(--border-subtle)] px-5 py-4">
        <h2 class="font-semibold text-[15px]">{t('platform.domains.addTitle')}</h2>
        <button type="button" aria-label={t('platform.domains.close')} class="header-icon-btn !size-7" onclick={() => (showModal = false)}><i class="icon-x text-[14px]"></i></button>
      </div>
      <form onsubmit={(e) => { e.preventDefault(); save(); }} novalidate class="p-5 space-y-4">
        {#if modalError}
          <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{modalError}</span></div>
        {/if}
        <div>
          <label for="dom-tenant" class="block text-[12.5px] font-medium mb-1.5">{t('platform.domains.tenantCode')}</label>
          <input id="dom-tenant" type="text" bind:value={form.tenantCode} class="form-input !text-[13px] font-mono" autocomplete="off" placeholder="kotakcantik" />
          <p class="mt-1 text-[11.5px] min-h-4 {lookup.state === 'found' ? 'text-[var(--color-success-600)] font-semibold' : lookup.state === 'missing' ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-tertiary)]'}">
            {#if lookup.state === 'found'}<i class="icon-circle-check text-[12px]"></i> {t('platform.domains.tenantFound', { name: lookup.name })}
            {:else if lookup.state === 'missing'}<i class="icon-circle-x text-[12px]"></i> {t('platform.domains.tenantNotFound')}
            {:else if lookup.state === 'checking'}{t('platform.domains.tenantChecking')}
            {:else}{t('platform.domains.tenantCodeHelp')}{/if}
          </p>
        </div>
        <div>
          <label for="dom-host" class="block text-[12.5px] font-medium mb-1.5">{t('platform.domains.host')}</label>
          <input id="dom-host" type="text" bind:value={form.host} class="form-input !text-[13px] font-mono" placeholder="arus.kotakcantik.com" autocomplete="off" spellcheck="false" />
          <p class="mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.domains.hostHelp')}</p>
        </div>

        {#if hostLooksValid(form.host) && target}
          <div class="rounded-xl bg-[#0b1220] p-3 text-slate-200">
            <div class="flex items-center justify-between gap-2">
              <p class="text-[10.5px] font-bold uppercase tracking-wider text-sky-300">{t('platform.domains.dnsRecord')}</p>
              {@render copyBtn(instruction(form.host.trim().toLowerCase()), 'ins-new')}
            </div>
            <p class="mt-2 font-mono text-[12.5px] break-all"><span class="text-emerald-300">CNAME</span>  {label(form.host.trim().toLowerCase())}  <span class="text-slate-500">→</span>  <span class="text-sky-200">{target}</span></p>
          </div>
        {/if}

        <div class="flex justify-end gap-2 pt-1">
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (showModal = false)}>{t('platform.domains.cancel')}</button>
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving || lookup.state !== 'found' || !hostLooksValid(form.host)}>{saving ? t('platform.domains.saving') : t('platform.domains.save')}</button>
        </div>
      </form>
    </div>
  </div>
{/if}
