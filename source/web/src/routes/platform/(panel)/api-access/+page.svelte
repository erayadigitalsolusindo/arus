<script lang="ts">
  import { platformApi, type ExtCredential, type ExtBrand, type ExtBrandInput, type ExtActors, type ExtGroup, type ExtCreated } from '#lib/platform/api.ts';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Modal from '#lib/components/Modal.svelte';
  import Select from '#lib/components/Select.svelte';
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
    formError = '';
    credForm = blankCred();
    actors = { users: [], outlets: [] };
  }
  function openEdit(c: ExtCredential) {
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
    if (!confirm(t('extapi.admin.cred.deleteConfirm'))) return;
    try {
      await platformApi.extDeleteCredential(c.id);
      await loadCreds();
    } catch (e) {
      error = errorMessage(e);
    }
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
    if (!confirm(t('extapi.admin.brand.deleteConfirm'))) return;
    try {
      await platformApi.extDeleteBrand(b.id);
      await loadBrands();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  $effect(() => {
    void loadAll();
  });
  $effect(() => {
    void filterTenant;
    if (!loading) loadCreds().catch((e) => (error = errorMessage(e)));
  });

  const fieldCls = (f: string) => (brandFields[f] ? '!border-[var(--color-danger-500)]' : '');
</script>

<svelte:head><title>{t('extapi.admin.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="font-display font-bold text-[22px] tracking-tight">{t('extapi.admin.title')}</h1>
      <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">{t('extapi.admin.subtitle')}</p>
    </div>
    <a href="/developers" target="_blank" rel="noopener" class="btn btn-outline !text-[12.5px]"><i class="icon-external-link text-[13px]"></i>{t('extapi.admin.portalLink')}</a>
  </div>

  <div class="flex gap-1 border-b border-[var(--border-subtle)]" role="tablist">
    {#each [['credentials', 'extapi.admin.tabs.credentials'], ['branding', 'extapi.admin.tabs.branding']] as const as [id, key] (id)}
      <button
        type="button"
        role="tab"
        aria-selected={tab === id}
        class="px-4 py-2.5 text-[13px] font-semibold -mb-px border-b-2 transition-colors {tab === id
          ? 'border-[var(--color-primary-600)] text-[var(--color-primary-600)]'
          : 'border-transparent text-[var(--text-tertiary)] hover:text-[var(--text-primary)]'}"
        onclick={() => (tab = id)}
      >
        {t(key)}
      </button>
    {/each}
  </div>

  {#if notice}
    <div class="rounded-lg bg-[var(--color-success-500)]/10 border border-[var(--color-success-500)]/20 px-4 py-3 text-[13px] text-[var(--color-success-600)]">{notice}</div>
  {/if}
  {#if error}
    <div class="rounded-lg bg-[var(--color-danger-500)]/10 border border-[var(--color-danger-500)]/20 px-4 py-3 text-[13px] text-[var(--color-danger-600)]">{error}</div>
  {/if}

  {#if loading}
    <div class="text-[13px] text-[var(--text-tertiary)]">{t('extapi.admin.loading')}</div>
  {:else if tab === 'credentials'}
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="w-72">
        <Select
          bind:value={filterTenant}
          ariaLabel={t('extapi.admin.cred.tenant')}
          options={[{ value: '', label: t('extapi.admin.cred.filterTenant') }, ...tenants.map((x) => ({ value: x.id, label: x.name }))]}
        />
      </div>
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={openCreate}><i class="icon-plus text-[13px]"></i>{t('extapi.admin.cred.add')}</button>
    </div>

    {#if creds.length === 0}
      <div class="surface-card p-8 text-center text-[13px] text-[var(--text-tertiary)]">{t('extapi.admin.cred.empty')}</div>
    {:else}
      <div class="surface-card overflow-x-auto">
        <table class="w-full text-[13px]">
          <thead>
            <tr class="border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)] text-start">
              <th class="px-4 py-2.5 text-start font-semibold">{t('extapi.admin.cred.label')}</th>
              <th class="px-4 py-2.5 text-start font-semibold">{t('extapi.admin.cred.keyId')}</th>
              <th class="px-4 py-2.5 text-start font-semibold">{t('extapi.admin.cred.ips')}</th>
              <th class="px-4 py-2.5 text-start font-semibold">{t('extapi.admin.cred.lastUsed')}</th>
              <th class="px-4 py-2.5 text-start font-semibold">{t('extapi.admin.cred.status')}</th>
              <th class="px-4 py-2.5"></th>
            </tr>
          </thead>
          <tbody>
            {#each creds as c (c.id)}
              <tr class="border-b border-[var(--border-subtle)] last:border-0 hover:bg-[var(--surface-sunken)] align-top">
                <td class="px-4 py-3">
                  <div class="font-semibold">{c.label}</div>
                  <div class="text-[11.5px] text-[var(--text-tertiary)]">{c.tenant_name} · {c.user_name} · {c.outlet_name}</div>
                </td>
                <td class="px-4 py-3 font-mono text-[12px]">{c.key_id}</td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap gap-1 max-w-[260px]">
                    {#each c.ip_allowlist as ip (ip)}<span class="font-mono text-[11px] px-1.5 py-0.5 rounded bg-[var(--surface-sunken)] border border-[var(--border-subtle)]">{ip}</span>{/each}
                  </div>
                  <div class="text-[11px] text-[var(--text-tertiary)] mt-1">{c.rate_limit_min}/min</div>
                </td>
                <td class="px-4 py-3 text-[var(--text-secondary)]">
                  {#if c.last_used_at}
                    {formatDateTime(c.last_used_at)}<div class="text-[11px] font-mono text-[var(--text-tertiary)]">{c.last_used_ip}</div>
                  {:else}{t('extapi.admin.cred.never')}{/if}
                </td>
                <td class="px-4 py-3">
                  {#if c.active}<span class="badge-success">{t('extapi.admin.active')}</span>{:else}<span class="badge-warning">{t('extapi.admin.inactive')}</span>{/if}
                  {#if c.rotating}<span class="badge-info ms-1">{t('extapi.admin.cred.rotating')}</span>{/if}
                </td>
                <td class="px-4 py-3 text-end whitespace-nowrap">
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
    <div class="flex justify-end">
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => openBrand()}><i class="icon-plus text-[13px]"></i>{t('extapi.admin.brand.add')}</button>
    </div>
    {#if brands.length === 0}
      <div class="surface-card p-8 text-center text-[13px] text-[var(--text-tertiary)]">{t('extapi.admin.brand.empty')}</div>
    {:else}
      <div class="grid gap-4 md:grid-cols-2">
        {#each brands as b (b.id)}
          <div class="surface-card overflow-hidden">
            <div class="h-1.5" style="background:{b.accent}"></div>
            <div class="p-4 space-y-3">
              <div class="flex items-start justify-between gap-3">
                <div class="flex items-center gap-3 min-w-0">
                  {#if b.logo_url}
                    <img src={b.logo_url} alt="" class="size-9 rounded object-contain bg-[var(--surface-sunken)]" />
                  {:else}
                    <span class="grid place-items-center size-9 rounded font-bold text-white text-[14px]" style="background:{b.accent}">{b.name.slice(0, 1).toUpperCase()}</span>
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
              {#if b.hidden_modules.length}
                <div class="flex flex-wrap gap-1">
                  {#each b.hidden_modules as m (m)}<span class="font-mono text-[10.5px] px-1.5 py-0.5 rounded bg-[var(--surface-sunken)] line-through text-[var(--text-tertiary)]">{m}</span>{/each}
                </div>
              {/if}
              <div class="flex justify-end gap-2">
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
  <Modal title={credForm.id ? t('extapi.admin.cred.editTitle') : t('extapi.admin.cred.createTitle')} onclose={() => (credForm = null)} wide>
    <form
      class="space-y-4"
      novalidate
      onsubmit={(e) => {
        e.preventDefault();
        void saveCred();
      }}
    >
      {#if formError}<div class="rounded-lg bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]">{formError}</div>{/if}
      <div>
        <label for="c-label" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.label')}</label>
        <input id="c-label" class="form-input !text-[13px]" bind:value={credForm.label} maxlength="100" placeholder={t('extapi.admin.cred.labelHint')} />
      </div>
      {#if !credForm.id}
        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <span class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.tenant')}</span>
            <Select
              value={credForm.tenant_id}
              placeholder={t('extapi.admin.cred.chooseTenant')}
              onchange={(v) => void pickTenant(v)}
              options={tenants.map((x) => ({ value: x.id, label: x.name }))}
            />
          </div>
          <div>
            <span class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.serviceUser')}</span>
            <Select
              bind:value={credForm.user_id}
              placeholder={t('extapi.admin.cred.choose')}
              disabled={!credForm.tenant_id}
              options={actors.users.map((u) => ({ value: u.id, label: `${u.name} · ${u.role}`, hint: u.email }))}
            />
          </div>
          <div>
            <span class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.outlet')}</span>
            <Select
              bind:value={credForm.outlet_id}
              placeholder={t('extapi.admin.cred.choose')}
              disabled={!credForm.tenant_id}
              options={actors.outlets.map((o) => ({ value: o.id, label: o.name }))}
            />
          </div>
        </div>
        <p class="text-[11px] text-[var(--text-tertiary)] -mt-2">{t('extapi.admin.cred.serviceUserHint')}</p>
      {/if}
      <div>
        <label for="c-ips" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.ips')}</label>
        <textarea id="c-ips" class="form-input !text-[13px] font-mono" rows="3" bind:value={credForm.ips} placeholder={'203.0.113.7\n198.51.100.0/24'}></textarea>
        <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('extapi.admin.cred.ipsHint')}</p>
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label for="c-rate" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.rate')}</label>
          <input id="c-rate" type="number" min="1" max="100000" class="form-input !text-[13px]" bind:value={credForm.rate} />
        </div>
        <div>
          <label for="c-exp" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.cred.expires')}</label>
          <input id="c-exp" type="date" class="form-input !text-[13px]" bind:value={credForm.expires} />
        </div>
      </div>
      {#if credForm.id}
        <div class="flex items-center gap-3">
          <Switch bind:checked={credForm.active} label={t('extapi.admin.active')} />
          <span class="text-[13px]">{t('extapi.admin.active')}</span>
        </div>
      {/if}
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (credForm = null)}>{t('extapi.admin.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving || !credForm.label.trim() || (!credForm.id && (!credForm.tenant_id || !credForm.user_id || !credForm.outlet_id))}>
          {saving ? t('extapi.admin.saving') : t('extapi.admin.save')}
        </button>
      </div>
    </form>
  </Modal>
{/if}

{#if rotating}
  <Modal title={t('extapi.admin.cred.rotateTitle')} onclose={() => (rotating = null)}>
    <div class="space-y-4">
      <p class="text-[13px] text-[var(--text-secondary)]">{t('extapi.admin.cred.rotateText')}</p>
      <Select
        bind:value={grace}
        options={[
          { value: '0', label: t('extapi.admin.cred.graceNone') },
          { value: '60', label: t('extapi.admin.cred.grace1h') },
          { value: '1440', label: t('extapi.admin.cred.grace24h') },
          { value: '10080', label: t('extapi.admin.cred.grace7d') }
        ]}
      />
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (rotating = null)}>{t('extapi.admin.cancel')}</button>
        <button type="button" class="btn btn-primary !text-[12.5px]" disabled={saving} onclick={doRotate}>{t('extapi.admin.cred.rotate')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if created}
  <Modal title={t('extapi.admin.cred.revealTitle')} onclose={() => (created = null)}>
    <div class="space-y-4">
      <div class="rounded-lg border border-[var(--color-warning-500)]/40 bg-[var(--color-warning-500)]/10 px-3 py-2 text-[12.5px]">{t('extapi.admin.cred.revealText')}</div>
      <div>
        <div class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1">{t('extapi.admin.cred.keyId')}</div>
        <div class="font-mono text-[13px] break-all">{created.key_id}</div>
      </div>
      <div>
        <div class="text-[11.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1">{t('extapi.admin.cred.secret')}</div>
        <div class="flex items-stretch gap-2">
          <code class="flex-1 font-mono text-[12.5px] break-all rounded-lg bg-[var(--surface-sunken)] border border-[var(--border-subtle)] px-3 py-2">{created.secret}</code>
          <button type="button" class="btn btn-outline !text-[12px]" onclick={copySecret}>
            <i class="icon-{copied ? 'check' : 'copy'} text-[13px]"></i>{copied ? t('extapi.admin.cred.copied') : t('extapi.admin.cred.copy')}
          </button>
        </div>
      </div>
      <div class="flex justify-end"><button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => (created = null)}>{t('extapi.admin.cred.done')}</button></div>
    </div>
  </Modal>
{/if}

{#if brandForm}
  <Modal title={brandForm.id ? t('extapi.admin.brand.editTitle') : t('extapi.admin.brand.createTitle')} onclose={() => (brandForm = null)} wide>
    <form
      class="space-y-4"
      novalidate
      onsubmit={(e) => {
        e.preventDefault();
        void saveBrand();
      }}
    >
      {#if formError}<div class="rounded-lg bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px] text-[var(--color-danger-600)]">{formError}</div>{/if}
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label for="b-name" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.name')}</label>
          <input id="b-name" class="form-input !text-[13px] {fieldCls('name')}" bind:value={brandForm.name} maxlength="80" />
        </div>
        <div>
          <label for="b-slug" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.slug')}</label>
          <input id="b-slug" class="form-input !text-[13px] font-mono {fieldCls('slug')}" bind:value={brandForm.slug} maxlength="41" placeholder="mitra-1" />
        </div>
        <div class="sm:col-span-2">
          <label for="b-domain" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.domain')}</label>
          <input id="b-domain" class="form-input !text-[13px] font-mono {fieldCls('domain')}" bind:value={brandForm.domainText} placeholder="docs.mitra.com" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('extapi.admin.brand.domainHint')}</p>
        </div>
        <div class="sm:col-span-2">
          <label for="b-tag" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.tagline')}</label>
          <input id="b-tag" class="form-input !text-[13px] {fieldCls('tagline')}" bind:value={brandForm.tagline} maxlength="200" />
        </div>
        <div>
          <label for="b-accent" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.accent')}</label>
          <div class="flex items-center gap-2">
            <input id="b-accent-pick" type="color" class="h-9 w-12 rounded border border-[var(--border-subtle)] bg-transparent p-0.5" bind:value={brandForm.accent} aria-label={t('extapi.admin.brand.accent')} />
            <input id="b-accent" class="form-input !text-[13px] font-mono {fieldCls('accent')}" bind:value={brandForm.accent} maxlength="7" />
          </div>
        </div>
        <div>
          <label for="b-base" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.baseUrl')}</label>
          <input id="b-base" class="form-input !text-[13px] font-mono {fieldCls('base_url')}" bind:value={brandForm.base_url} placeholder="https://api.mitra.com" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('extapi.admin.brand.baseUrlHint')}</p>
        </div>
        <div class="sm:col-span-2">
          <label for="b-logo" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.logo')}</label>
          <input id="b-logo" class="form-input !text-[13px] font-mono {fieldCls('logo_url')}" bind:value={brandForm.logo_url} placeholder="https://…/logo.svg" />
          <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('extapi.admin.brand.logoHint')}</p>
        </div>
        <div>
          <label for="b-mail" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.supportEmail')}</label>
          <input id="b-mail" class="form-input !text-[13px] {fieldCls('support_email')}" bind:value={brandForm.support_email} />
        </div>
        <div>
          <label for="b-url" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.supportUrl')}</label>
          <input id="b-url" class="form-input !text-[13px] {fieldCls('support_url')}" bind:value={brandForm.support_url} />
        </div>
        <div class="sm:col-span-2">
          <label for="b-foot" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.footer')}</label>
          <input id="b-foot" class="form-input !text-[13px] {fieldCls('footer')}" bind:value={brandForm.footer} maxlength="500" />
        </div>
        <div class="sm:col-span-2">
          <label for="b-sand" class="block text-[12.5px] font-medium mb-1.5">{t('extapi.admin.brand.sandboxNote')}</label>
          <textarea id="b-sand" class="form-input !text-[13px] {fieldCls('sandbox_note')}" rows="2" bind:value={brandForm.sandbox_note}></textarea>
        </div>
      </div>

      <div>
        <div class="text-[12.5px] font-medium mb-1">{t('extapi.admin.brand.hidden')}</div>
        <p class="text-[11px] text-[var(--text-tertiary)] mb-2">{t('extapi.admin.brand.hiddenHint')}</p>
        <div class="grid gap-x-4 gap-y-1 sm:grid-cols-3 max-h-40 overflow-y-auto scroll-thin rounded-lg border border-[var(--border-subtle)] p-3">
          {#each groups as g (g.id)}
            <label class="flex items-center gap-2 text-[12.5px]">
              <input type="checkbox" class="form-checkbox" checked={brandForm.hidden_modules.includes(g.id)} onchange={(e) => toggleHidden(g.id, e.currentTarget.checked)} />
              <span class="truncate">{g.title}</span>
            </label>
          {/each}
        </div>
      </div>

      <div class="flex flex-wrap gap-6">
        <div class="flex items-center gap-3">
          <Switch bind:checked={brandForm.active} label={t('extapi.admin.active')} />
          <span class="text-[13px]">{t('extapi.admin.active')}</span>
        </div>
        <div class="flex items-center gap-3" title={t('extapi.admin.brand.isDefaultHint')}>
          <Switch bind:checked={brandForm.is_default} label={t('extapi.admin.brand.isDefault')} />
          <span class="text-[13px]">{t('extapi.admin.brand.isDefault')}</span>
        </div>
      </div>

      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => (brandForm = null)}>{t('extapi.admin.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving}>{saving ? t('extapi.admin.saving') : t('extapi.admin.save')}</button>
      </div>
    </form>
  </Modal>
{/if}
