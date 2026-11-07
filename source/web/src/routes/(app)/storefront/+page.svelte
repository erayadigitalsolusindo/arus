<script lang="ts">
  import { onMount } from 'svelte';
  import { storefront, publicUrl, type StorefrontState, type VersionInfo, type TemplateInfo } from '#lib/storefront/api.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  let loading = $state(true);
  let loadError = $state('');
  let state_ = $state<StorefrontState | null>(null);

  let slug = $state('');
  let enabled = $state(false);
  let title = $state('');
  let html = $state('');
  let css = $state('');
  let version = $state(0);
  let savedSnapshot = $state('');

  // Tiga tab kerja: dua berkas kode dan pratinjau. Editor memakai seluruh lebar; pratinjau punya tab sendiri.
  let tab = $state<'html' | 'css' | 'preview'>('html');
  let device = $state<'desktop' | 'tablet' | 'mobile'>('desktop');
  let saving = $state(false);
  let notice = $state('');
  let saveError = $state('');
  let fields = $state<Record<string, string>>({});
  let warnings = $state<string[]>([]);
  let versions = $state<VersionInfo[]>([]);
  let panel = $state<'' | 'help' | 'history' | 'templates'>('');
  let templates = $state<TemplateInfo[]>([]);
  let tplLoading = $state(false);
  let tplBusy = $state('');

  let previewDoc = $state('');
  let previewError = $state('');
  let previewing = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let seq = 0;
  let gutter: HTMLElement | undefined = $state();

  const canEdit = $derived(can('storefront', 'update'));
  const snapshot = () => JSON.stringify([slug, enabled, title, html, css]);
  const dirty = $derived(savedSnapshot !== '' && snapshot() !== savedSnapshot);
  const url = $derived(slug ? publicUrl(slug) : '');
  const source = $derived(tab === 'css' ? css : html);
  const lineCount = $derived(source.split('\n').length);
  const limit = $derived(tab === 'css' ? 20000 : 60000);
  const frameWidth = $derived({ desktop: '100%', tablet: '820px', mobile: '390px' }[device]);

  function apply(c: { slug: string; enabled: boolean; title: string; html: string; css: string; version: number }) {
    ({ slug, enabled, title, html, css, version } = c);
    savedSnapshot = snapshot();
  }

  async function load() {
    try {
      state_ = await storefront.get();
      if (state_.config) apply(state_.config);
      else {
        html = state_.starter.html;
        css = state_.starter.css;
        savedSnapshot = '';
      }
      void loadVersions();
    } catch (err) {
      loadError = errorMessage(err);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    return () => clearTimeout(timer);
  });

  async function openTemplates() {
    panel = panel === 'templates' ? '' : 'templates';
    if (panel !== 'templates' || templates.length) return;
    tplLoading = true;
    try {
      templates = await storefront.templates();
    } catch (err) {
      saveError = errorMessage(err);
    } finally {
      tplLoading = false;
    }
  }

  async function useTemplate(tp: TemplateInfo) {
    if (!confirm(t('storefront.tpl.useConfirm', { name: tp.name }))) return;
    tplBusy = tp.id;
    try {
      const full = await storefront.template(tp.id);
      html = full.html;
      css = full.css;
      panel = '';
      tab = 'preview';
      notice = t('storefront.tpl.loaded', { name: tp.name });
      void renderPreview();
    } catch (err) {
      saveError = errorMessage(err) || t('storefront.tpl.loadFailed');
    } finally {
      tplBusy = '';
    }
  }

  async function loadVersions() {
    try {
      versions = await storefront.versions();
    } catch {
      versions = [];
    }
  }

  async function renderPreview() {
    const mine = ++seq;
    previewing = true;
    try {
      const r = await storefront.preview({ html, css, title, slug });
      if (mine === seq) {
        previewDoc = r.html;
        previewError = '';
      }
    } catch (err) {
      if (mine === seq) previewError = err instanceof ApiError && err.fields ? Object.keys(err.fields).map((k) => tryT(`storefront.fe.${k}`) ?? k).join(' · ') || errorMessage(err) : errorMessage(err);
    } finally {
      if (mine === seq) previewing = false;
    }
  }
  // Pratinjau hanya dirender saat tabnya dibuka (dan diperbarui bila isi berubah selagi terbuka).
  $effect(() => {
    void html, css, title;
    if (loading || tab !== 'preview') return;
    clearTimeout(timer);
    timer = setTimeout(renderPreview, 400);
  });

  async function save() {
    saving = true;
    notice = saveError = '';
    fields = {};
    warnings = [];
    try {
      const r = await storefront.save({ slug: slug.trim().toLowerCase(), enabled, title, html, css });
      apply(r.config);
      warnings = r.warnings ?? [];
      notice = t('storefront.saved', { version: r.config.version });
      void loadVersions();
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) fields = err.fields;
      saveError = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  async function restore(v: number) {
    if (dirty && !confirm(t('storefront.restoreConfirm'))) return;
    try {
      const c = await storefront.version(v);
      html = c.html;
      css = c.css;
      title = c.title;
      panel = '';
      notice = t('storefront.restored', { version: v });
    } catch (err) {
      saveError = errorMessage(err);
    }
  }

  function resetStarter() {
    if (!state_ || !confirm(t('storefront.starterConfirm'))) return;
    html = state_.starter.html;
    css = state_.starter.css;
  }

  async function copyUrl() {
    try {
      await navigator.clipboard.writeText(url);
      notice = t('storefront.copied');
    } catch {
      /* clipboard tak tersedia */
    }
  }

  // Tab di textarea menyisipkan indentasi, bukan memindah fokus. Ctrl/Cmd+S = simpan.
  function onKey(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      if (canEdit && !saving) void save();
      return;
    }
    if (e.key !== 'Tab') return;
    e.preventDefault();
    const el = e.currentTarget as HTMLTextAreaElement;
    const s = el.selectionStart;
    const v = el.value.slice(0, s) + '  ' + el.value.slice(el.selectionEnd);
    if (tab === 'css') css = v;
    else html = v;
    queueMicrotask(() => (el.selectionStart = el.selectionEnd = s + 2));
  }
  const syncScroll = (e: Event) => {
    if (gutter) gutter.scrollTop = (e.currentTarget as HTMLElement).scrollTop;
  };

  const tags: [string, string][] = [
    ['{namatoko}', 'storefront.help.namatoko'],
    ['{tahun}', 'storefront.help.tahun'],
    ['{kategori1}', 'storefront.help.kategoriN'],
    ['{namabarang}  {namabarang3}', 'storefront.help.namabarang'],
    ['{harga}  {hargaangka}', 'storefront.help.harga'],
    ['{kodebarang}  {kategoribarang}  {deskripsi}', 'storefront.help.misc'],
    ['{gambar}', 'storefront.help.gambar'],
    ['{#barang kategori=1 limit=8 urut=terbaru} … {/barang}', 'storefront.help.barang'],
    ['{#kategori limit=6} {namakategori} {/kategori}', 'storefront.help.kategori'],
    ['{#barang kategori=ini}', 'storefront.help.ini']
  ];
</script>

<svelte:head><title>{t('storefront.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('storefront.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('storefront.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-[1600px] mx-auto w-full">
  <!-- Kepala: judul, status tayang, alamat, simpan -->
  <header class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex items-center gap-3 min-w-0">
      <span class="grid size-11 shrink-0 place-items-center rounded-2xl bg-gradient-to-br from-[var(--color-primary-500)] to-[var(--color-primary-700)] text-white shadow-sm"><i class="icon-layout-template text-[20px]"></i></span>
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <h1 class="font-display font-bold text-[19px] leading-tight">{t('storefront.title')}</h1>
          {#if version > 0}
            <span class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-semibold {enabled ? 'bg-[var(--color-success-500)]/15 text-[var(--color-success-600)]' : 'bg-[var(--surface-sunken)] text-[var(--text-tertiary)] border border-[var(--border-default)]'}">
              <span class="size-1.5 rounded-full {enabled ? 'bg-[var(--color-success-500)]' : 'bg-[var(--text-tertiary)]'}"></span>{enabled ? t('storefront.statusLive') : t('storefront.statusDraft')}
            </span>
          {/if}
        </div>
        <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)] truncate">{t('storefront.subtitle')}</p>
      </div>
    </div>
    {#if canEdit}
      <div class="flex items-center gap-2">
        {#if dirty}<span class="text-[12px] font-medium text-[var(--color-warning-600)] flex items-center gap-1.5"><span class="size-1.5 rounded-full bg-[var(--color-warning-500)]"></span>{t('storefront.unsaved')}</span>{/if}
        <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={saving || loading} onclick={save} title="Ctrl+S">
          <i class="icon-save text-[13px]"></i>{saving ? t('storefront.saving') : t('storefront.save')}
        </button>
      </div>
    {/if}
  </header>

  {#if notice}
    <div class="rounded-xl border border-[var(--color-success-500)]/30 bg-[var(--color-success-500)]/10 px-3 py-2 text-[12.5px] flex items-center gap-2" role="status">
      <i class="icon-check text-[14px]"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if saveError}
    <div class="rounded-xl border border-[var(--color-danger-500)]/30 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px]" role="alert">
      {saveError}
      {#if Object.keys(fields).length}
        <ul class="mt-1 list-disc ps-5">
          {#each Object.entries(fields) as [k, code] (k)}<li>{tryT(`storefront.fe.${k}`) ?? k}: {fieldMessage(code)}</li>{/each}
        </ul>
      {/if}
    </div>
  {/if}
  {#if warnings.length}
    <div class="rounded-xl border border-[var(--color-warning-500)]/40 bg-[var(--color-warning-500)]/10 px-3 py-2 text-[12.5px]" role="status">
      <p class="font-semibold">{t('storefront.warnings')}</p>
      <ul class="mt-1 list-disc ps-5">{#each warnings as w (w)}<li>{w}</li>{/each}</ul>
    </div>
  {/if}
  {#if loadError}
    <div class="rounded-xl border border-[var(--color-danger-500)]/30 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px]" role="alert">{loadError}</div>
  {/if}

  {#if loading}
    <div class="surface-card p-8 text-center text-[13px] text-[var(--text-tertiary)]">{t('storefront.loading')}</div>
  {:else if !loadError}
    <!-- Pengaturan ringkas satu baris -->
    <section class="surface-card p-3 lg:p-4 grid gap-3 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)_auto] items-end">
      <div>
        <label class="lbl" for="sf-slug">{t('storefront.slug')}</label>
        <div class="field">
          <span class="field-prefix">/s/</span>
          <input id="sf-slug" class="field-input font-mono" bind:value={slug} disabled={!canEdit} maxlength="40" placeholder="toko-saya" autocomplete="off" spellcheck="false" />
        </div>
      </div>
      <div>
        <label class="lbl" for="sf-title">{t('storefront.pageTitle')}</label>
        <div class="field"><input id="sf-title" class="field-input" bind:value={title} disabled={!canEdit} maxlength="80" placeholder={t('storefront.pageTitleHelp')} /></div>
      </div>
      <div class="flex items-center gap-2.5 pb-1.5">
        <button type="button" role="switch" aria-checked={enabled} aria-label={t('storefront.enabled')} class="switch {enabled ? 'on' : ''}" disabled={!canEdit} onclick={() => (enabled = !enabled)}><span></span></button>
        <span class="text-[13px] font-medium text-[var(--text-primary)]">{t('storefront.enabled')}</span>
      </div>
      <p class="lg:col-span-3 text-[11.5px] text-[var(--text-tertiary)] -mt-1">{t('storefront.slugHelp')}</p>
      {#if url && version > 0}
        <div class="lg:col-span-3 flex flex-wrap items-center gap-2 rounded-xl bg-[var(--surface-sunken)] border border-[var(--border-subtle)] px-3 py-2 text-[12px]">
          <i class="icon-globe text-[14px] text-[var(--text-tertiary)]"></i>
          <span class="text-[var(--text-tertiary)]">{enabled ? t('storefront.liveAt') : t('storefront.offAt')}</span>
          <a class="font-mono text-[var(--color-primary-600)] hover:underline break-all" href={url} target="_blank" rel="noopener noreferrer">{url}</a>
          <span class="ms-auto flex items-center gap-1">
            <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px]" onclick={copyUrl}><i class="icon-copy text-[12px]"></i>{t('storefront.copy')}</button>
            <a class="btn btn-ghost !px-2 !py-1 !text-[12px]" href={url} target="_blank" rel="noopener noreferrer"><i class="icon-external-link text-[12px]"></i>{t('storefront.openTab')}</a>
          </span>
        </div>
      {/if}
    </section>

    <!-- Meja kerja: editor selebar halaman; pratinjau di tab sendiri -->
    <section class="bench">
      <div class="bench-bar">
        <div class="tabs" role="tablist">
          <button type="button" role="tab" aria-selected={tab === 'html'} class="tabbtn {tab === 'html' ? 'on' : ''}" onclick={() => (tab = 'html')}><i class="icon-code text-[13px]"></i>index.html</button>
          <button type="button" role="tab" aria-selected={tab === 'css'} class="tabbtn {tab === 'css' ? 'on' : ''}" onclick={() => (tab = 'css')}><i class="icon-palette text-[13px]"></i>style.css</button>
          <button type="button" role="tab" aria-selected={tab === 'preview'} class="tabbtn {tab === 'preview' ? 'on' : ''}" onclick={() => { tab = 'preview'; void renderPreview(); }}><i class="icon-eye text-[13px]"></i>{t('storefront.preview')}</button>
        </div>
        <div class="flex items-center gap-1 ms-auto">
          <button type="button" class="tool {panel === 'templates' ? 'on' : ''}" onclick={openTemplates}><i class="icon-layout-template text-[13px]"></i><span>{t('storefront.templates')}</span></button>
          <button type="button" class="tool {panel === 'help' ? 'on' : ''}" onclick={() => (panel = panel === 'help' ? '' : 'help')}><i class="icon-braces text-[13px]"></i><span>{t('storefront.tags')}</span></button>
          <button type="button" class="tool {panel === 'history' ? 'on' : ''}" onclick={() => (panel = panel === 'history' ? '' : 'history')}><i class="icon-history text-[13px]"></i><span>{t('storefront.history')} ({versions.length})</span></button>
          {#if canEdit}<button type="button" class="tool" onclick={resetStarter}><i class="icon-rotate-ccw text-[13px]"></i><span>{t('storefront.starter')}</span></button>{/if}
        </div>
      </div>

      {#if panel === 'templates'}
        <div class="drawer">
          <p class="mb-3 text-[var(--text-tertiary)]">{t('storefront.tpl.galleryIntro')}</p>
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {#each templates as tp (tp.id)}
              <article class="rounded-xl border border-[var(--border-default)] bg-[var(--surface-card)] p-4 flex flex-col gap-2">
                <div class="flex items-start justify-between gap-2">
                  <h3 class="font-bold text-[13.5px]">{tp.name}</h3>
                  {#if tp.category}<span class="badge-soft badge-info shrink-0">{tp.category}</span>{/if}
                </div>
                <p class="text-[12px] text-[var(--text-secondary)] grow">{tp.description}</p>
                <div class="flex items-center justify-between gap-2 text-[11.5px] text-[var(--text-tertiary)]">
                  <span>{Math.max(1, Math.round(tp.bytes / 1024))} KB</span>
                  {#if canEdit}<button type="button" class="btn btn-primary !px-3 !py-1.5 !text-[12px] disabled:opacity-60" disabled={tplBusy === tp.id} onclick={() => useTemplate(tp)}>{t('storefront.tpl.use')}</button>{/if}
                </div>
              </article>
            {:else}
              <p class="text-[var(--text-tertiary)]">{tplLoading ? t('storefront.loading') : t('storefront.tpl.noTemplates')}</p>
            {/each}
          </div>
        </div>
      {:else if panel === 'help'}
        <div class="drawer">
          <p class="mb-2 text-[var(--text-tertiary)]">{t('storefront.helpIntro')}</p>
          <dl class="grid gap-x-4 gap-y-1.5 md:grid-cols-[auto_1fr]">
            {#each tags as [code, key] (code)}
              <dt><code class="chip">{code}</code></dt>
              <dd class="text-[var(--text-secondary)]">{tryT(key) ?? key}</dd>
            {/each}
          </dl>
          <p class="mt-3 rounded-lg border border-[var(--color-warning-500)]/40 bg-[var(--color-warning-500)]/10 px-3 py-2">{t('storefront.safety')}</p>
        </div>
      {:else if panel === 'history'}
        <div class="drawer !p-0">
          {#each versions as v (v.version)}
            <div class="flex items-center gap-3 px-4 py-2 border-b border-[var(--border-subtle)] last:border-0">
              <span class="font-mono font-semibold">v{v.version}</span>
              <span class="text-[var(--text-tertiary)]">{formatDateTime(v.created_at)}</span>
              <span class="text-[var(--text-tertiary)]">{Math.max(1, Math.round(v.bytes / 1024))} KB</span>
              {#if v.version === version}<span class="badge-soft badge-info">{t('storefront.current')}</span>{:else if canEdit}
                <button type="button" class="ms-auto font-medium text-[var(--color-primary-600)] hover:underline" onclick={() => restore(v.version)}>{t('storefront.restore')}</button>
              {/if}
            </div>
          {:else}
            <p class="px-4 py-3 text-[var(--text-tertiary)]">{t('storefront.noHistory')}</p>
          {/each}
        </div>
      {/if}

      {#if tab !== 'preview'}
        <div class="editor">
          <div class="gutter" bind:this={gutter} aria-hidden="true">{#each { length: lineCount } as _, i (i)}<span>{i + 1}</span>{/each}</div>
          {#if tab === 'html'}
            <textarea class="code" style="background-color:#0d1424 !important;color:#e2e8f0 !important" spellcheck="false" autocapitalize="off" autocomplete="off" aria-label="HTML" readonly={!canEdit} bind:value={html} onkeydown={onKey} onscroll={syncScroll}></textarea>
          {:else}
            <textarea class="code" style="background-color:#0d1424 !important;color:#e2e8f0 !important" spellcheck="false" autocapitalize="off" autocomplete="off" aria-label="CSS" readonly={!canEdit} bind:value={css} onkeydown={onKey} onscroll={syncScroll}></textarea>
          {/if}
        </div>
        <div class="statusbar">
          <span>{tab === 'html' ? t('storefront.htmlHelp') : t('storefront.cssHelp')}</span>
          <span class="ms-auto tabular-nums">{lineCount} {t('storefront.lines')} · {source.length.toLocaleString()} / {limit.toLocaleString()}</span>
        </div>
      {:else}
        <div class="pv-bar">
          <span class="inline-flex rounded-lg border border-[var(--border-default)] overflow-hidden text-[12px]">
            {#each [['desktop', 'icon-monitor', 'storefront.desktop'], ['tablet', 'icon-tablet', 'storefront.tablet'], ['mobile', 'icon-smartphone', 'storefront.mobile']] as const as [d, ic, lab] (d)}
              <button type="button" class="px-3 py-1.5 {device === d ? 'bg-[var(--color-primary-600)] text-white' : 'text-[var(--text-secondary)]'}" aria-label={t(lab)} aria-pressed={device === d} onclick={() => (device = d)}><i class="{ic} text-[13px]"></i></button>
            {/each}
          </span>
          {#if previewing}<span class="text-[11.5px] text-[var(--text-tertiary)]">{t('storefront.rendering')}</span>{/if}
          <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px] ms-auto" onclick={renderPreview}><i class="icon-rotate-ccw text-[12px]"></i>{t('storefront.refresh')}</button>
        </div>
        {#if previewError}<p class="px-4 py-2 text-[12px] text-[var(--color-danger-600)]" role="alert">{previewError}</p>{/if}
        <div class="stage">
          <!-- sandbox tanpa allow-scripts / allow-same-origin: isi tenant tak bisa menjalankan script maupun menyentuh aplikasi ini. -->
          <iframe title={t('storefront.preview')} sandbox="allow-popups allow-popups-to-escape-sandbox" srcdoc={previewDoc} style="width:{frameWidth}"></iframe>
        </div>
        <div class="statusbar"><span>{t('storefront.previewNote')}</span></div>
      {/if}
    </section>
  {/if}
</main>

<style>
  .lbl {
    display: block;
    margin-bottom: 4px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--text-tertiary);
  }
  .field {
    display: flex;
    align-items: stretch;
    border: 1px solid var(--border-default);
    border-radius: 10px;
    background: var(--surface-card);
    overflow: hidden;
  }
  .field:focus-within {
    border-color: var(--color-primary-500);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-500) 18%, transparent);
  }
  .field-prefix {
    display: grid;
    place-items: center;
    padding: 0 10px;
    font: 12px ui-monospace, Menlo, Consolas, monospace;
    color: var(--text-tertiary);
    background: var(--surface-sunken);
    border-inline-end: 1px solid var(--border-default);
  }
  .field-input {
    flex: 1;
    min-width: 0;
    padding: 9px 12px;
    font-size: 13px;
    background: transparent !important;
    color: var(--text-primary);
    border: 0 !important;
    outline: none;
    box-shadow: none !important;
  }

  .switch {
    position: relative;
    width: 42px;
    height: 24px;
    flex-shrink: 0;
    border-radius: 999px;
    background: var(--border-strong, #cbd5e1);
    transition: background 0.15s;
  }
  .switch span {
    position: absolute;
    top: 3px;
    inset-inline-start: 3px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 3px rgb(0 0 0 / 0.3);
    transition: transform 0.15s;
  }
  .switch.on {
    background: var(--color-success-500);
  }
  .switch.on span {
    transform: translateX(18px);
  }
  :global([dir='rtl']) .switch.on span {
    transform: translateX(-18px);
  }
  .switch:disabled {
    opacity: 0.5;
  }

  /* Meja kerja */
  .bench {
    border: 1px solid var(--border-default);
    border-radius: 16px;
    background: var(--surface-card);
    overflow: hidden;
    box-shadow: 0 1px 2px rgb(15 23 42 / 0.05), 0 8px 24px -12px rgb(15 23 42 / 0.12);
  }
  .bench-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface-sunken);
  }
  .tabs {
    display: inline-flex;
    gap: 4px;
    padding: 3px;
    border-radius: 12px;
    background: color-mix(in srgb, var(--text-primary) 7%, transparent);
  }
  .tabbtn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border-radius: 9px;
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-secondary);
    transition: background 0.12s, color 0.12s;
  }
  .tabbtn:hover {
    color: var(--text-primary);
  }
  .tabbtn.on {
    background: var(--surface-card);
    color: var(--color-primary-600);
    box-shadow: 0 1px 2px rgb(15 23 42 / 0.12);
  }
  .tool {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    border-radius: 9px;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .tool:hover,
  .tool.on {
    background: var(--surface-card);
    color: var(--color-primary-600);
  }
  @media (max-width: 640px) {
    .tool span {
      display: none;
    }
  }
  .drawer {
    padding: 12px 16px;
    font-size: 12px;
    max-height: 280px;
    overflow: auto;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface-sunken);
  }
  .chip {
    font: 11.5px ui-monospace, Menlo, Consolas, monospace;
    padding: 2px 6px;
    border-radius: 6px;
    background: var(--surface-card);
    border: 1px solid var(--border-subtle);
    color: var(--text-primary);
  }

  /* Editor kode: selalu gelap (kontras sama di tema terang maupun gelap) */
  .editor {
    display: flex;
    height: clamp(460px, 70vh, 900px);
    background: #0d1424;
  }
  .gutter {
    flex: 0 0 auto;
    min-width: 48px;
    padding: 14px 10px 14px 12px;
    overflow: hidden;
    text-align: right;
    user-select: none;
    font: 12.5px/1.6 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    color: #475569;
    background: #0a1120;
    border-inline-end: 1px solid #1e293b;
  }
  .gutter span {
    display: block;
  }
  .code {
    flex: 1 1 auto;
    min-width: 0;
    resize: none;
    padding: 14px 16px;
    font: 12.5px/1.6 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace !important;
    background: #0d1424 !important;
    color: #e2e8f0 !important;
    caret-color: #4ade80;
    border: 0 !important;
    border-radius: 0 !important;
    box-shadow: none !important;
    outline: none;
    tab-size: 2;
    white-space: pre;
    overflow: auto;
  }
  .code::selection {
    background: #1d4ed8;
    color: #fff;
  }
  .statusbar {
    display: flex;
    gap: 12px;
    padding: 6px 14px;
    font-size: 11.5px;
    color: var(--text-tertiary);
    border-top: 1px solid var(--border-subtle);
    background: var(--surface-sunken);
  }

  .pv-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .stage {
    display: flex;
    justify-content: center;
    height: clamp(460px, 70vh, 900px);
    padding: 16px;
    overflow: auto;
    background:
      radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--text-primary) 14%, transparent) 1px, transparent 0) 0 0 / 18px 18px,
      var(--surface-sunken);
  }
  .stage iframe {
    max-width: 100%;
    height: 100%;
    background: #fff;
    border: 1px solid var(--border-default);
    border-radius: 12px;
    box-shadow: 0 10px 30px -12px rgb(15 23 42 / 0.35);
    transition: width 0.2s;
  }
</style>
