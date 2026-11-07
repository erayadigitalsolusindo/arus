<script lang="ts">
  import { onMount, tick } from 'svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';
  import Select from '#lib/components/Select.svelte';
  import AiActionCard from '#lib/components/AiActionCard.svelte';
  import AiOrb from '#lib/components/AiOrb.svelte';
  import AiThinking from '#lib/components/AiThinking.svelte';
  import AiHistory from '#lib/components/AiHistory.svelte';
  import { ai, type ActionStatus, type AiSettings, type ChatImage, type ChatMessage, type Connection, type PendingAction, type Provider, type ToolCall } from '#lib/ai/api.ts';
  import { IMAGE_ACCEPT, MAX_IMAGES, prepareImage } from '#lib/ai/image.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // HalloArus: asisten AI. Konteks percakapan dikirim klien (teks user/assistant + foto nota di pesan terakhir); hasil tool selalu
  // dibuat server. Riwayat disimpan server TERENKRIPSI bila saklar tenant menyala dan percakapan tidak "sementara". AI tidak menulis
  // data: ia mengusulkan aksi yang tampil sebagai kartu (AiActionCard) dan baru berlaku setelah pengguna menyetujui.
  type Turn = Omit<ChatMessage, 'images'> & {
    tools?: ToolCall[];
    tokens?: { input: number; output: number };
    failed?: boolean;
    actions?: (PendingAction & { status?: ActionStatus })[];
    previews?: string[];
    hadImages?: boolean;
  };

  let settings = $state<AiSettings | undefined>();
  let turns = $state<Turn[]>([]);
  let input = $state('');
  let sending = $state(false);
  let error = $state('');
  let abort: AbortController | null = null;
  let scroller: HTMLElement | undefined = $state();
  let showSettings = $state(false);
  let showConnector = $state(false);
  let connections = $state<Connection[] | undefined>();
  let copied = $state(false);
  let copiedKey = $state('');
  let attached = $state<{ image: ChatImage; preview: string }[]>([]);
  let fileInput: HTMLInputElement | undefined = $state();
  const canWrite = can('ai_assistant', 'create');
  // Riwayat: id percakapan tersimpan yang sedang dibuka, mode sementara, kunci penyegar daftar, dan panel di layar sempit.
  let conversationId = $state('');
  let temporary = $state(false);
  let historyKey = $state(0);
  let showHistory = $state(false);
  let loadingConv = $state(false);
  let typed = $state('');
  const reduceMotion = typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  // Partikel hiasan: posisi/durasi deterministik (tanpa random) agar tidak berubah tiap render.
  const particles = Array.from({ length: 16 }, (_, i) => ({ x: (i * 37 + 11) % 100, y: (i * 53 + 7) % 100, s: 2 + (i % 3), d: 7 + (i % 5) * 2, delay: -(i * 0.9) }));

  // Form pengaturan
  let keyInput = $state('');
  let provSel = $state<Provider>('anthropic');
  let modelSel = $state('');
  // Penyedia kustom (OpenAI-compatible): pilihan "tambah" memakai id khusus; isian nama/base URL/model diambil dari penyedia terpilih.
  const NEW = '__new_custom__';
  let labelInput = $state('');
  let baseUrlInput = $state('');
  let saving = $state(false);
  let saveMsg = $state('');
  const canManage = can('ai_assistant', 'update');

  async function loadSettings() {
    try {
      settings = await ai.settings();
      provSel = settings.provider;
      fillFromCurrent();
      showSettings = !settings.active && canManage && settings.policy.enabled && (settings.policy.allow_byok || settings.policy.allow_custom);
    } catch (e) {
      error = errorMessage(e);
      settings = { configured: false, active: false, free: { enabled: false, limit: 0, remaining: 0, model: '' }, provider: 'anthropic', providers: [], history: { available: false, enabled: false, days: 90 },
        policy: { enabled: true, domains: [], allow_write: true, allow_cost: true, allow_byok: true, allow_custom: true, allow_images: true, allow_history: true, allow_mcp: true, chat_limit: 20, max_range_days: 366 } };
    }
  }

  onMount(() => {
    document.title = t('ai.docTitle');
    void loadSettings();
    return () => abort?.abort();
  });

  async function saveSettings() {
    if (saving) return;
    error = saveMsg = '';
    if (isCustom) {
      const base = baseUrlInput.trim().replace(/\/+$/, '');
      if (!labelInput.trim() || !base || !modelSel.trim()) {
        error = t('ai.custom.needFields');
        return;
      }
      if (!keyInput.trim() && (provSel === NEW || base !== (cur?.base_url ?? ''))) {
        error = t(provSel === NEW ? 'ai.setup.needKey' : 'ai.custom.needKeyForUrl', { provider: provName });
        return;
      }
    } else if (!cur?.configured && !keyInput.trim()) {
      error = t('ai.setup.needKey', { provider: provName });
      return;
    }
    saving = true;
    try {
      settings = isCustom
        ? await ai.saveCustom(provSel === NEW ? undefined : provSel, { label: labelInput.trim(), base_url: baseUrlInput.trim(), model: modelSel.trim(), api_key: keyInput.trim() || undefined })
        : await ai.save({ provider: provSel, api_key: keyInput.trim() || undefined, model: modelSel });
      provSel = settings.provider;
      fillFromCurrent();
      keyInput = '';
      saveMsg = t('ai.setup.saved');
      showSettings = false;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function removeKey() {
    if (saving || !confirm(t('ai.setup.removeConfirm', { provider: provName }))) return;
    saving = true;
    error = saveMsg = '';
    try {
      await ai.removeKey(provSel);
      turns = [];
      await loadSettings();
      showSettings = !settings?.configured;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      saving = false;
    }
  }

  async function loadConnections() {
    try {
      connections = await ai.connections();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  function toggleConnector() {
    showConnector = !showConnector;
    if (showConnector) void loadConnections();
  }

  async function copyUrl() {
    try {
      await navigator.clipboard.writeText(settings?.connector_url ?? '');
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      /* clipboard tidak tersedia: pengguna bisa menyalin manual dari kolom */
    }
  }

  async function copyText(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text);
      copiedKey = key;
      setTimeout(() => (copiedKey = ''), 2000);
    } catch {
      /* clipboard tidak tersedia: pengguna bisa memilih teks perintah secara manual */
    }
  }

  // Klien MCP yang didukung; Gemini CLI dan Qwen Code didaftarkan lewat perintah dengan alamat connector terisi.
  const connectorApps = $derived.by(() => {
    const url = settings?.connector_url ?? '';
    return [
      { id: 'claude', cmd: undefined as string | undefined },
      { id: 'chatgpt', cmd: undefined as string | undefined },
      { id: 'gemini', cmd: `gemini mcp add --transport http arus ${url}` as string | undefined },
      { id: 'qwen', cmd: `qwen mcp add --transport http arus ${url}` as string | undefined }
    ];
  });

  async function revoke(c: Connection) {
    if (!confirm(t('ai.connector.revokeConfirm'))) return;
    try {
      await ai.revokeConnection(c.id);
      saveMsg = t('ai.connector.revoked');
      await loadConnections();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function scrollDown() {
    await tick();
    scroller?.scrollTo({ top: scroller.scrollHeight, behavior: 'smooth' });
  }

  async function addFiles(files: Iterable<File>) {
    error = '';
    for (const f of files) {
      if (attached.length >= MAX_IMAGES) {
        error = t('ai.image.tooMany', { count: MAX_IMAGES });
        break;
      }
      const p = await prepareImage(f);
      if (p) attached.push(p);
      else error = t('ai.image.invalid');
    }
  }

  function onPaste(e: ClipboardEvent) {
    const files = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith('image/'));
    if (files.length) {
      e.preventDefault();
      void addFiles(files);
    }
  }

  async function send(text = input) {
    text = text.trim();
    const imgs = attached;
    if ((!text && imgs.length === 0) || sending || !settings?.active) return;
    if (!text) text = t('ai.image.defaultPrompt');
    error = '';
    input = '';
    attached = [];
    turns.push({ role: 'user', content: text, previews: imgs.map((i) => i.preview), hadImages: imgs.length > 0 });
    // Hanya teks percakapan yang dikirim (foto hanya di pesan terakhir); turn yang gagal tidak ikut.
    const history: ChatMessage[] = turns.filter((m) => !m.failed).map((m) => ({ role: m.role, content: m.content }));
    if (imgs.length) history[history.length - 1].images = imgs.map((i) => i.image);
    sending = true;
    abort = new AbortController();
    void scrollDown();
    try {
      const res = await ai.chat(history, abort.signal, { conversation_id: conversationId || undefined, temporary });
      if (res.conversation_id) {
        conversationId = res.conversation_id;
        historyKey++;
      }
      turns.push({ role: 'assistant', content: res.reply, tools: res.tools_used, actions: res.actions, tokens: { input: res.usage.input_tokens, output: res.usage.output_tokens } });
      if (res.free && settings) settings.free = res.free;
    } catch (e) {
      if ((e as Error)?.name === 'AbortError') {
        turns.pop();
        input = text;
        attached = imgs;
      } else {
        turns[turns.length - 1].failed = true;
        error = errorMessage(e);
        if (e instanceof ApiError && e.code === 'AI_CONVERSATION_NOT_FOUND') conversationId = ''; // percakapan hilang/kedaluwarsa: pesan berikutnya membuka yang baru
        if (e instanceof ApiError && e.code === 'AI_FREE_QUOTA_EXHAUSTED') {
          if (settings) settings.free.remaining = 0;
          showSettings = canManage;
        }
      }
    } finally {
      sending = false;
      abort = null;
      void scrollDown();
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      void send();
    }
  }

  function reset() {
    abort?.abort();
    turns = [];
    error = '';
    input = '';
    attached = [];
    conversationId = '';
    showHistory = false;
  }

  async function openConversation(id: string) {
    if (loadingConv || sending) return;
    loadingConv = true;
    error = '';
    try {
      const c = await ai.conversation(id);
      turns = c.messages.map((m) => ({
        role: m.role,
        content: m.content,
        tools: m.tools?.map((name) => ({ name })),
        actions: m.actions,
        hadImages: m.had_images
      }));
      conversationId = c.id;
      temporary = false;
      showHistory = false;
      void scrollDown();
    } catch (e) {
      error = errorMessage(e);
      historyKey++; // daftar disegarkan (mungkin sudah dihapus/kedaluwarsa)
    } finally {
      loadingConv = false;
    }
  }

  function onHistoryDeleted(id: string) {
    if (id === 'all' || id === conversationId) reset();
    if (id === 'all') historyKey++;
  }

  async function toggleHistory(on: boolean) {
    error = saveMsg = '';
    try {
      settings = await ai.setHistory(on);
      saveMsg = t('ai.history.saved');
    } catch (e) {
      error = errorMessage(e);
    }
  }

  // Teks mengetik di panggung kosong (hiasan): menelusuri contoh pertanyaan; klik mengisinya ke kolom.
  $effect(() => {
    if (turns.length > 0 || !settings?.active) return;
    const phrases = suggestionKeys.map((k) => t(`ai.suggestions.${k}`));
    if (reduceMotion) {
      typed = phrases[0];
      return;
    }
    let si = 0;
    let ci = 0;
    let dir = 1;
    let timer: ReturnType<typeof setTimeout>;
    const step = () => {
      const full = phrases[si];
      ci += dir;
      typed = full.slice(0, ci);
      let delay = dir === 1 ? 34 : 14;
      if (dir === 1 && ci >= full.length) {
        dir = -1;
        delay = 2000;
      } else if (dir === -1 && ci <= 0) {
        dir = 1;
        si = (si + 1) % phrases.length;
        delay = 450;
      }
      timer = setTimeout(step, delay);
    };
    timer = setTimeout(step, 500);
    return () => clearTimeout(timer);
  });

  const activeInfo = $derived(settings?.providers.find((p) => p.id === settings?.provider));
  const liveLabel = $derived.by(() => {
    if (!settings?.active) return '';
    if (settings.configured && activeInfo) {
      const name = activeInfo.custom ? (activeInfo.label ?? activeInfo.id) : (tryT(`ai.providers.${activeInfo.id}.name`) ?? activeInfo.id);
      return `${name} · ${tryT(`ai.models.${activeInfo.model}`) ?? activeInfo.model}`;
    }
    return `${t('ai.liveFree')} · ${settings.free.arus ? 'API ARUS' : settings.free.model}`;
  });
  const historyOn = $derived(!!settings?.history.available && !!settings.history.enabled);

  const toolLabel = (n: string) => tryT(`ai.tools.${n}`) ?? n;
  const uniqueTools = (calls: ToolCall[] = []) => [...new Set(calls.map((c) => c.name))];
  const cur = $derived(settings?.providers.find((p) => p.id === provSel));
  const isCustom = $derived(provSel === NEW || !!cur?.custom);
  const provName = $derived(provSel === NEW ? t('ai.custom.name') : cur?.custom ? (cur.label ?? provSel) : (tryT(`ai.providers.${provSel}.name`) ?? provSel));
  const modelOptions = $derived((cur?.models ?? []).map((m) => ({ value: m, label: tryT(`ai.models.${m}`) ?? m })));
  const providerOptions = $derived(
    [
      ...(settings?.providers ?? []).map((p) => ({ value: p.id, label: (p.custom ? (p.label ?? p.id) : (tryT(`ai.providers.${p.id}.name`) ?? p.id)) + (p.configured ? (p.id === settings?.provider ? ' ✓' : ' •') : '') })),
      { value: NEW, label: t('ai.custom.add') }
    ]
  );
  // Ganti penyedia di form: model ikut penyedia itu, isian key dikosongkan.
  function pickProvider() {
    keyInput = '';
    fillFromCurrent();
  }
  function fillFromCurrent() {
    modelSel = cur?.model ?? '';
    labelInput = cur?.label ?? '';
    baseUrlInput = cur?.base_url ?? '';
  }
  const suggestionKeys = ['s1', 's2', 's3', 's4', 's5', 's6'] as const;
</script>

<main class="ha p-4 lg:p-6 max-w-6xl mx-auto w-full min-w-0 flex flex-col gap-4" style="min-height: calc(100dvh - 5rem)">
  <header class="ha-head">
    <div class="ha-head-main">
      <span class="ha-logo" aria-hidden="true"><AiOrb size={46} busy={sending} /></span>
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="ha-title font-display">{t('ai.title')}</h1>
          <span class="ha-chip ha-chip-safe" title={canWrite ? t('ai.approvalModeHint') : t('ai.readOnlyHint')}><i class="icon-shield-check"></i> {canWrite ? t('ai.approvalMode') : t('ai.readOnly')}</span>
          {#if liveLabel}
            <span class="ha-chip ha-chip-live" title={liveLabel}><i class="ha-dot"></i> {t('ai.live')} <span class="ha-live-name">{liveLabel}</span></span>
          {/if}
        </div>
        <p class="ha-sub">{t('ai.subtitle')}</p>
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-2">
      {#if historyOn}
        <button type="button" class="btn btn-sm ha-hist-toggle" aria-expanded={showHistory} onclick={() => (showHistory = !showHistory)}><i class="icon-history"></i> {t('ai.history.title')}</button>
      {/if}
      {#if settings?.connector_url}
        <button type="button" class="btn btn-sm" aria-expanded={showConnector} onclick={toggleConnector}><i class="icon-plug"></i> {t('ai.connector.button')}</button>
      {/if}
      {#if turns.length > 0}
        <button type="button" class="btn btn-sm" onclick={reset}><i class="icon-square-pen"></i> {t('ai.input.clear')}</button>
      {/if}
      {#if canManage && settings?.active && (settings.policy.allow_byok || settings.policy.allow_custom)}
        <button type="button" class="btn btn-sm" aria-expanded={showSettings} onclick={() => (showSettings = !showSettings)}><i class="icon-settings"></i> {t('ai.settings')}</button>
      {/if}
    </div>
  </header>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}
  {#if saveMsg}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{saveMsg}</span>
    </div>
  {/if}

  {#if showConnector && settings?.connector_url}
    <section class="surface-card p-5 space-y-4">
      <div class="flex items-start gap-3">
        <span class="grid place-items-center size-9 shrink-0 rounded-lg bg-[var(--surface-sunken)] text-[var(--color-primary-600)]"><i class="icon-plug text-[18px]"></i></span>
        <div class="min-w-0">
          <h2 class="font-semibold text-[14px]">{t('ai.connector.title')}</h2>
          <p class="text-[12.5px] mt-1 leading-relaxed text-[var(--text-secondary)]">{t('ai.connector.intro')}</p>
        </div>
      </div>
      <div class="block text-[12.5px]">
        <span class="font-medium">{t('ai.connector.urlLabel')}</span>
        <div class="mt-1 flex items-center gap-2">
          <input readonly value={settings.connector_url} onfocus={(e) => e.currentTarget.select()} aria-label={t('ai.connector.urlLabel')}
            class="flex-1 min-w-0 h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] font-mono text-[12px]" />
          <button type="button" class="btn btn-sm" onclick={copyUrl}><i class={copied ? 'icon-check' : 'icon-copy'}></i> {copied ? t('ai.connector.copied') : t('ai.connector.copy')}</button>
        </div>
      </div>
      <div class="space-y-2">
        <h3 class="font-medium text-[12.5px]">{t('ai.connector.appsTitle')}</h3>
        {#each connectorApps as app (app.id)}
          <details class="rounded-lg border border-[var(--border-subtle)] group" open={app.id === 'claude'}>
            <summary class="cursor-pointer select-none px-3 py-2.5 text-[12.5px] font-medium">{tryT(`ai.connector.apps.${app.id}.name`)}</summary>
            <div class="px-3 pb-3 space-y-2 text-[12.5px] text-[var(--text-secondary)]">
              <ol class="space-y-1">
                <li>1. {tryT(`ai.connector.apps.${app.id}.s1`)}</li>
                {#if app.cmd}
                  <li class="flex items-center gap-2">
                    <code class="flex-1 min-w-0 overflow-x-auto whitespace-nowrap px-2.5 py-2 rounded bg-[var(--surface-sunken)] font-mono text-[11.5px]" aria-label={t('ai.connector.cmdLabel')}>{app.cmd}</code>
                    <button type="button" class="btn btn-sm shrink-0" onclick={() => copyText(app.cmd!, app.id)}><i class={copiedKey === app.id ? 'icon-check' : 'icon-copy'}></i> {copiedKey === app.id ? t('ai.connector.copied') : t('ai.connector.copy')}</button>
                  </li>
                {/if}
                <li>2. {tryT(`ai.connector.apps.${app.id}.s2`)}</li>
                <li>3. {tryT(`ai.connector.apps.${app.id}.s3`)}</li>
              </ol>
            </div>
          </details>
        {/each}
        <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('ai.connector.appsNote')}</p>
      </div>
      <div>
        <h3 class="font-medium text-[12.5px] mb-2">{t('ai.connector.listTitle')}</h3>
        {#if connections === undefined}
          <div class="h-10 rounded-lg bg-[var(--surface-sunken)] animate-pulse" aria-busy="true"></div>
        {:else if connections.length === 0}
          <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('ai.connector.none')}</p>
        {:else}
          <ul class="divide-y divide-[var(--border-subtle)] rounded-lg border border-[var(--border-subtle)]">
            {#each connections as c (c.id)}
              <li class="flex flex-wrap items-center justify-between gap-2 px-3 py-2.5 text-[12.5px]">
                <div class="min-w-0">
                  <p class="font-medium truncate">{c.client} <span class="font-normal text-[var(--text-tertiary)]">· {t('ai.connector.by', { name: c.user_name })}</span></p>
                  <p class="text-[11.5px] text-[var(--text-tertiary)]">{c.last_used_at ? t('ai.connector.lastUsed', { when: formatDateTime(c.last_used_at) }) : t('ai.connector.never')}</p>
                </div>
                <button type="button" class="btn btn-sm" onclick={() => revoke(c)}><i class="icon-trash-2"></i> {t('ai.connector.revoke')}</button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </section>
  {/if}

  {#if settings === undefined}
    <div class="space-y-3" aria-busy="true">
      <div class="h-24 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>
      <div class="h-40 rounded-xl bg-[var(--surface-sunken)] animate-pulse"></div>
    </div>
  {:else}
    {#if showSettings && canManage && settings.policy.enabled && (settings.policy.allow_byok || settings.policy.allow_custom)}
      <form class="surface-card p-5 space-y-4" onsubmit={(e) => { e.preventDefault(); void saveSettings(); }}>
        <div class="flex items-start gap-3">
          <span class="grid place-items-center size-9 shrink-0 rounded-lg bg-[var(--surface-sunken)] text-[var(--color-primary-600)]"><i class="icon-key-round text-[18px]"></i></span>
          <div class="min-w-0">
            <h2 class="font-semibold text-[14px]">{t('ai.setup.title')}</h2>
            <p class="text-[12.5px] mt-1 leading-relaxed text-[var(--text-secondary)]">{settings.configured ? '' : t('ai.setup.notConfiguredManage') + ' '}{isCustom ? t('ai.custom.intro') : t('ai.setup.intro', { console: tryT(`ai.providers.${provSel}.console`) ?? '' })}</p>
          </div>
        </div>
        <div class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.providerLabel')}</span>
          <div class="mt-1"><Select bind:value={provSel} options={providerOptions} ariaLabel={t('ai.setup.providerLabel')} onchange={pickProvider} /></div>
        </div>
        {#if isCustom}
          <label class="block text-[12.5px]">
            <span class="font-medium">{t('ai.custom.labelField')}</span>
            <input bind:value={labelInput} maxlength="40" autocomplete="off" placeholder={t('ai.custom.labelPlaceholder')}
              class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)]" />
          </label>
          <label class="block text-[12.5px]">
            <span class="font-medium">{t('ai.custom.baseUrl')}</span>
            <input bind:value={baseUrlInput} maxlength="300" autocomplete="off" spellcheck="false" inputmode="url" placeholder="https://free.openkey.id/v1"
              class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] font-mono" />
            <span class="block mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('ai.custom.baseUrlHint')}</span>
          </label>
        {/if}
        <label class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.keyLabel', { provider: provName })}</span>
          <input type="password" bind:value={keyInput} autocomplete="off" spellcheck="false" maxlength="300" placeholder={isCustom ? '' : (tryT(`ai.providers.${provSel}.placeholder`) ?? '')}
            class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] font-mono" />
          <span class="block mt-1 text-[11.5px] text-[var(--text-tertiary)]">
            {cur?.configured && provSel !== NEW ? t('ai.setup.keySaved', { hint: cur.key_hint ?? '' }) + ' ' + t('ai.setup.keyReplaceHint') : t('ai.setup.keyNone')}
          </span>
        </label>
        <div class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.modelLabel')}</span>
          {#if isCustom}
            <input bind:value={modelSel} maxlength="100" autocomplete="off" spellcheck="false" placeholder="sonnet-5" aria-label={t('ai.setup.modelLabel')}
              class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] font-mono" />
            <span class="block mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('ai.custom.modelHint')}</span>
          {:else}
            <div class="mt-1"><Select bind:value={modelSel} options={modelOptions} ariaLabel={t('ai.setup.modelLabel')} /></div>
          {/if}
        </div>
        {#if settings.history.available && settings.policy.allow_history}
          <label class="flex items-start gap-2.5 rounded-lg border border-[var(--border-subtle)] p-3 text-[12.5px] cursor-pointer">
            <input type="checkbox" class="mt-0.5" checked={settings.history.enabled} onchange={(e) => toggleHistory(e.currentTarget.checked)} />
            <span><span class="font-medium">{t('ai.history.switchLabel')}</span><span class="block mt-0.5 text-[11.5px] text-[var(--text-tertiary)]">{t('ai.history.switchHint', { days: settings.history.days })}</span></span>
          </label>
        {/if}
        <div class="flex flex-wrap items-center gap-2 pt-1">
          <button type="submit" class="btn btn-primary" disabled={saving}>{saving ? t('ai.setup.saving') : t('ai.setup.save')}</button>
          {#if cur?.configured && provSel !== NEW}
            <button type="button" class="btn" onclick={removeKey} disabled={saving}><i class="icon-trash-2"></i> {t('ai.setup.removeKey', { provider: provName })}</button>
          {/if}
        </div>
      </form>
    {/if}

    {#if !settings.policy.enabled}
      <div class="surface-card p-4 flex items-center gap-3 text-[13px] text-[var(--text-secondary)]" role="status">
        <i class="icon-lock text-[20px] text-[var(--text-tertiary)]"></i><span>{t('ai.policy.disabled')}</span>
      </div>
    {:else if !settings.active}
      {#if !canManage}
        <div class="surface-card p-4 flex items-center gap-3 text-[13px] text-[var(--text-secondary)]">
          <i class="icon-key-round text-[20px] text-[var(--text-tertiary)]"></i><span>{t('ai.setup.notConfigured')}</span>
        </div>
      {/if}
    {:else}
      {#if settings.policy.domains.length < 6 || !settings.policy.allow_cost || !settings.policy.allow_write || !settings.policy.allow_images}
        <div class="flex items-center gap-2 rounded-lg px-3 py-2 text-[12.5px] bg-[var(--surface-sunken)] text-[var(--text-secondary)]" role="note"><i class="icon-info"></i><span>{t('ai.policy.limited')}</span></div>
      {/if}
      {#if settings.free.enabled && !settings.configured}
        <div class="flex flex-wrap items-center justify-between gap-2 rounded-lg px-3 py-2 text-[12.5px] bg-[var(--surface-sunken)] text-[var(--text-secondary)]">
          <span><i class="icon-gift"></i> {settings.free.remaining > 0 ? t('ai.free.banner', { remaining: settings.free.remaining, limit: settings.free.limit }) : t('ai.free.exhausted')}</span>
          {#if canManage}<button type="button" class="btn btn-sm" onclick={() => (showSettings = true)}><i class="icon-key-round"></i> {t('ai.free.useOwn')}</button>{/if}
        </div>
      {/if}
      <div class="ha-grid" class:with-hist={historyOn}>
        {#if historyOn}
          <aside class="ha-side" class:open={showHistory} aria-label={t('ai.history.title')}>
            <AiHistory activeId={conversationId} refreshKey={historyKey} onopen={openConversation} onnew={reset} ondeleted={onHistoryDeleted} onerror={(m) => (error = m)} onnotice={(m) => (saveMsg = m)} />
          </aside>
        {/if}

        <section class="ha-chat">
          <div bind:this={scroller} class="ha-stage" class:busy={sending} aria-live="polite">
            <div class="ha-bg" aria-hidden="true">
              {#each particles as pt, k (k)}
                <i style="left:{pt.x}%;top:{pt.y}%;width:{pt.s}px;height:{pt.s}px;animation-duration:{pt.d}s;animation-delay:{pt.delay}s"></i>
              {/each}
            </div>

            {#if loadingConv}
              <div class="ha-loading" role="status"><i class="icon-loader-circle animate-spin"></i> {t('ai.history.loading')}</div>
            {/if}

            {#if turns.length === 0 && !loadingConv}
              <div class="ha-hero">
                <AiOrb size={168} busy={sending} />
                <p class="ha-eyebrow">{t('ai.hero.eyebrow')}</p>
                <h2 class="ha-hero-title font-display">{t('ai.empty.title')}</h2>
                <p class="ha-hero-hint">{t('ai.empty.hint')}</p>
                <button type="button" class="ha-typed" onclick={() => (input = typed)} aria-label={t('ai.hero.tryThis')}>
                  <i class="icon-terminal"></i><span>{typed}</span><b class="ha-caret"></b>
                </button>
                <div class="ha-chips">
                  {#each suggestionKeys as k, idx (k)}
                    <button type="button" class="ha-sug" style="animation-delay:{idx * 70}ms" onclick={() => send(t(`ai.suggestions.${k}`))} disabled={sending}>
                      <i class={['icon-activity', 'icon-database', 'icon-zap', 'icon-scan-line', 'icon-cpu', 'icon-brain-circuit'][idx]}></i>
                      <span>{t(`ai.suggestions.${k}`)}</span>
                    </button>
                  {/each}
                </div>
              </div>
            {/if}

            <div class="ha-msgs">
              {#each turns as m, i (i)}
                {#if m.role === 'user'}
                  <div class="ha-row ha-row-user">
                    <div class="ha-user" class:opacity-60={m.failed}>
                      {#if m.previews?.length}
                        <div class="flex flex-wrap gap-1.5 mb-2">
                          {#each m.previews as src, k (k)}<img {src} alt={t('ai.image.alt')} class="h-20 w-auto rounded-md object-cover bg-white" />{/each}
                        </div>
                      {:else if m.hadImages}
                        <p class="ha-had-img"><i class="icon-image-plus"></i> {t('ai.history.hadImages')}</p>
                      {/if}
                      {m.content}
                    </div>
                  </div>
                {:else}
                  <div class="ha-row">
                    <span class="ha-avatar" aria-hidden="true"><i class="icon-sparkles"></i></span>
                    <div class="ha-ai">
                      <div class="text-[13.5px] leading-relaxed"><MarkdownView source={m.content} /></div>
                      {#each m.actions ?? [] as act (act.id)}
                        <AiActionCard action={act} initialStatus={act.status ?? 'pending'} />
                      {/each}
                      {#if m.tools?.length}
                        <div class="ha-tools">
                          <span><i class="icon-terminal"></i> {t('ai.dataRead')}</span>
                          {#each uniqueTools(m.tools) as name, k (name)}
                            <span class="ha-tool" style="animation-delay:{k * 60}ms">{toolLabel(name)}</span>
                          {/each}
                        </div>
                      {/if}
                      {#if m.tokens}
                        <p class="ha-tokens">{t('ai.usage', { input: formatNumber(m.tokens.input), output: formatNumber(m.tokens.output) })}</p>
                      {/if}
                    </div>
                  </div>
                {/if}
              {/each}

              {#if sending}
                <div class="ha-row">
                  <span class="ha-avatar busy" aria-hidden="true"><i class="icon-sparkles"></i></span>
                  <div class="flex-1 min-w-0"><AiThinking /></div>
                </div>
              {/if}
            </div>
          </div>

          <div class="ha-composer-wrap">
            {#if attached.length}
              <div class="flex flex-wrap gap-2 mb-2">
                {#each attached as im, k (k)}
                  <div class="relative">
                    <img src={im.preview} alt={t('ai.image.alt')} class="h-16 w-auto rounded-md border border-[var(--border-default)] object-cover" />
                    <button type="button" class="absolute -top-1.5 -end-1.5 grid place-items-center size-5 rounded-full bg-[var(--surface-base)] border border-[var(--border-default)]" aria-label={t('ai.image.remove')} onclick={() => attached.splice(k, 1)}><i class="icon-x text-[11px]"></i></button>
                  </div>
                {/each}
              </div>
            {/if}
            <div class="ha-composer" class:busy={sending}>
              <input bind:this={fileInput} type="file" accept={IMAGE_ACCEPT} multiple class="hidden" onchange={(e) => { void addFiles(e.currentTarget.files ?? []); e.currentTarget.value = ''; }} />
              {#if settings.policy.allow_images}<button type="button" class="ha-ic" title={t('ai.image.attach')} aria-label={t('ai.image.attach')} disabled={sending || attached.length >= MAX_IMAGES} onclick={() => fileInput?.click()}><i class="icon-image-plus"></i></button>{/if}
              <textarea bind:value={input} onkeydown={onKey} onpaste={onPaste} rows="2" maxlength="6000" disabled={sending || (settings.free.enabled && !settings.configured && settings.free.remaining <= 0)} placeholder={t('ai.input.placeholder')} aria-label={t('ai.input.placeholder')}></textarea>
              {#if sending}
                <button type="button" class="ha-send ha-stop" onclick={() => abort?.abort()}><i class="icon-x"></i> {t('ai.input.stop')}</button>
              {:else}
                <button type="button" class="ha-send" onclick={() => send()} disabled={!input.trim() && attached.length === 0}><i class="icon-send"></i> {t('ai.input.send')}</button>
              {/if}
            </div>
            <div class="ha-foot">
              {#if settings.history.available && settings.history.enabled}
                <button type="button" class="ha-temp" class:on={temporary} aria-pressed={temporary} disabled={turns.length > 0}
                  title={turns.length > 0 ? t('ai.history.tempLocked') : t('ai.history.tempHint')} onclick={() => (temporary = !temporary)}>
                  <i class={temporary ? 'icon-eye-off' : 'icon-history'}></i> {temporary ? t('ai.history.temp') : t('ai.history.saving')}
                </button>
              {:else if settings.history.available}
                <span class="ha-temp on"><i class="icon-eye-off"></i> {t('ai.history.off')}</span>
              {/if}
              <p>{t('ai.privacy')} {t('ai.history.noPersonal')} {t('ai.disclaimer')}</p>
            </div>
          </div>
        </section>
      </div>
    {/if}
  {/if}
</main>

<style>
  .ha {
    --ha-a: var(--color-primary-500, #3b82f6);
    --ha-b: #22d3ee;
    --ha-c: #a78bfa;
  }

  /* ---- header ---- */
  .ha-head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
    position: relative;
    padding: 0.9rem 1rem;
    border-radius: 1rem;
    border: 1px solid color-mix(in oklab, var(--ha-a) 28%, var(--border-default));
    background:
      radial-gradient(600px 160px at 0% 0%, color-mix(in oklab, var(--ha-a) 16%, transparent), transparent 70%),
      radial-gradient(500px 140px at 100% 0%, color-mix(in oklab, var(--ha-c) 12%, transparent), transparent 70%),
      var(--surface-base);
    overflow: hidden;
  }
  .ha-head::after {
    content: '';
    position: absolute;
    inset: 0 0 auto 0;
    height: 2px;
    background: linear-gradient(90deg, transparent, var(--ha-b), var(--ha-a), var(--ha-c), transparent);
    background-size: 200% 100%;
    animation: ha-flow 6s linear infinite;
  }
  .ha-head-main {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    min-width: 0;
  }
  .ha-logo {
    display: grid;
    place-items: center;
    width: 46px;
    height: 46px;
    flex-shrink: 0;
  }
  .ha-title {
    font-weight: 800;
    font-size: 21px;
    line-height: 1.1;
    letter-spacing: -0.01em;
    background: linear-gradient(100deg, var(--text-primary), var(--ha-b) 45%, var(--ha-a) 70%, var(--ha-c));
    background-size: 220% 100%;
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    animation: ha-flow 9s linear infinite;
  }
  .ha-sub {
    margin-top: 2px;
    font-size: 12.5px;
    color: var(--text-tertiary);
  }
  .ha-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.18rem 0.55rem;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.03em;
    border: 1px solid transparent;
    max-width: 100%;
  }
  .ha-chip-safe {
    color: var(--color-success-600, #16a34a);
    background: color-mix(in oklab, #22c55e 12%, transparent);
    border-color: color-mix(in oklab, #22c55e 30%, transparent);
  }
  .ha-chip-live {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--text-secondary);
    background: color-mix(in oklab, var(--ha-b) 10%, transparent);
    border-color: color-mix(in oklab, var(--ha-b) 32%, transparent);
  }
  .ha-live-name {
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 16rem;
  }
  .ha-dot {
    width: 7px;
    height: 7px;
    border-radius: 999px;
    background: #22c55e;
    box-shadow: 0 0 0 0 color-mix(in oklab, #22c55e 60%, transparent);
    animation: ha-ping 2s ease-out infinite;
  }

  /* ---- layout ---- */
  .ha-grid {
    display: grid;
    gap: 1rem;
    grid-template-columns: minmax(0, 1fr);
    flex: 1;
    min-height: 0;
  }
  .ha-grid.with-hist {
    grid-template-columns: 16.5rem minmax(0, 1fr);
  }
  .ha-side {
    border-radius: 1rem;
    border: 1px solid var(--border-default);
    background: var(--surface-base);
    padding: 0.8rem;
    max-height: calc(100dvh - 12rem);
    min-height: 18rem;
    position: sticky;
    top: 0.75rem;
    align-self: start;
    width: 100%;
  }
  @media (max-width: 900px) {
    .ha-grid.with-hist {
      grid-template-columns: minmax(0, 1fr);
    }
    .ha-side {
      display: none;
      position: static;
      max-height: 22rem;
      min-height: 0;
    }
    .ha-side.open {
      display: block;
      animation: ha-rise 0.25s ease both;
    }
  }
  @media (min-width: 901px) {
    .ha-hist-toggle {
      display: none;
    }
  }
  .ha-chat {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    min-width: 0;
  }

  /* ---- stage ---- */
  .ha-stage {
    position: relative;
    flex: 1;
    min-height: 22rem;
    max-height: calc(100dvh - 19rem);
    overflow-y: auto;
    padding: 1.1rem;
    border-radius: 1rem;
    border: 1px solid var(--border-default);
    background:
      radial-gradient(700px 320px at 50% -10%, color-mix(in oklab, var(--ha-a) 13%, transparent), transparent 70%),
      radial-gradient(circle, color-mix(in oklab, var(--text-tertiary) 22%, transparent) 1px, transparent 1.2px) 0 0 / 22px 22px,
      var(--surface-base);
    animation: ha-grid 24s linear infinite;
    scroll-behavior: smooth;
  }
  .ha-stage.busy {
    border-color: color-mix(in oklab, var(--ha-a) 45%, var(--border-default));
    box-shadow: 0 0 0 1px color-mix(in oklab, var(--ha-a) 20%, transparent), 0 0 32px -8px color-mix(in oklab, var(--ha-a) 45%, transparent);
  }
  .ha-bg {
    position: absolute;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }
  .ha-bg i {
    position: absolute;
    border-radius: 999px;
    background: var(--ha-b);
    opacity: 0;
    box-shadow: 0 0 8px var(--ha-b);
    animation: ha-float linear infinite;
  }
  .ha-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 2rem 0;
    font-size: 12.5px;
    color: var(--text-tertiary);
  }

  /* ---- hero (panggung kosong) ---- */
  .ha-hero {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: 0.45rem;
    padding: 0.8rem 0 0.4rem;
    animation: ha-rise 0.5s ease both;
  }
  .ha-eyebrow {
    margin-top: 0.3rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11px;
    letter-spacing: 0.22em;
    text-transform: uppercase;
    color: var(--ha-b);
  }
  .ha-hero-title {
    font-weight: 800;
    font-size: 22px;
    letter-spacing: -0.01em;
  }
  .ha-hero-hint {
    max-width: 34rem;
    font-size: 12.5px;
    color: var(--text-tertiary);
  }
  .ha-typed {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.4rem;
    max-width: 100%;
    min-height: 2.5rem;
    padding: 0.5rem 0.9rem;
    border-radius: 0.8rem;
    cursor: pointer;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12.5px;
    text-align: start;
    color: var(--text-secondary);
    border: 1px solid color-mix(in oklab, var(--ha-b) 35%, var(--border-default));
    background: color-mix(in oklab, var(--ha-b) 6%, var(--surface-base));
    transition: border-color 0.2s, transform 0.2s;
  }
  .ha-typed:hover {
    border-color: var(--ha-b);
    transform: translateY(-1px);
  }
  .ha-typed i {
    color: var(--ha-b);
    flex-shrink: 0;
  }
  .ha-caret {
    width: 2px;
    height: 1.1em;
    background: var(--ha-b);
    animation: ha-blink 1s steps(2, start) infinite;
  }
  .ha-chips {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: 0.6rem;
    width: 100%;
    max-width: 46rem;
    margin-top: 0.8rem;
  }
  .ha-sug {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    padding: 0.7rem 0.8rem;
    border-radius: 0.8rem;
    text-align: start;
    font-size: 12.5px;
    line-height: 1.35;
    cursor: pointer;
    color: var(--text-secondary);
    border: 1px solid var(--border-default);
    background: color-mix(in oklab, var(--surface-base) 88%, transparent);
    backdrop-filter: blur(4px);
    transition: transform 0.2s, border-color 0.2s, box-shadow 0.2s, color 0.2s;
    animation: ha-rise 0.5s ease both;
  }
  .ha-sug i {
    margin-top: 1px;
    font-size: 16px;
    color: var(--ha-a);
    flex-shrink: 0;
  }
  .ha-sug:hover:not(:disabled) {
    transform: translateY(-2px);
    color: var(--text-primary);
    border-color: color-mix(in oklab, var(--ha-a) 55%, var(--border-default));
    box-shadow: 0 8px 24px -12px color-mix(in oklab, var(--ha-a) 60%, transparent);
  }

  /* ---- pesan ---- */
  .ha-msgs {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .ha-row {
    display: flex;
    gap: 0.65rem;
    animation: ha-rise 0.35s ease both;
  }
  .ha-row-user {
    justify-content: flex-end;
  }
  .ha-user {
    max-width: 85%;
    padding: 0.65rem 0.9rem;
    border-radius: 1rem;
    border-end-end-radius: 0.3rem;
    font-size: 13.5px;
    white-space: pre-wrap;
    color: #fff;
    background: linear-gradient(135deg, var(--ha-a), color-mix(in oklab, var(--ha-c) 75%, var(--ha-a)));
    box-shadow: 0 8px 20px -10px color-mix(in oklab, var(--ha-a) 70%, transparent);
  }
  .ha-had-img {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-bottom: 0.3rem;
    font-size: 11.5px;
    opacity: 0.85;
  }
  .ha-avatar {
    display: grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    flex-shrink: 0;
    border-radius: 999px;
    color: #fff;
    background: linear-gradient(135deg, var(--ha-b), var(--ha-a) 60%, var(--ha-c));
    box-shadow: 0 0 14px -2px color-mix(in oklab, var(--ha-a) 70%, transparent);
  }
  .ha-avatar.busy {
    animation: ha-pulse 1.1s ease-in-out infinite;
  }
  .ha-ai {
    flex: 1;
    min-width: 0;
    padding: 0.7rem 0.9rem;
    border-radius: 1rem;
    border-start-start-radius: 0.3rem;
    border: 1px solid var(--border-default);
    border-inline-start: 2px solid color-mix(in oklab, var(--ha-a) 70%, transparent);
    background: color-mix(in oklab, var(--surface-base) 92%, transparent);
  }
  .ha-tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
    margin-top: 0.6rem;
    font-size: 11px;
    color: var(--text-tertiary);
  }
  .ha-tool {
    padding: 0.1rem 0.5rem;
    border-radius: 0.4rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 10.5px;
    color: var(--text-secondary);
    border: 1px solid color-mix(in oklab, var(--ha-b) 28%, var(--border-default));
    background: color-mix(in oklab, var(--ha-b) 7%, transparent);
    animation: ha-rise 0.35s ease both;
  }
  .ha-tokens {
    margin-top: 0.25rem;
    font-size: 10.5px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--text-tertiary);
  }

  /* ---- composer ---- */
  .ha-composer {
    position: relative;
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    padding: 0.5rem;
    border-radius: 1rem;
    border: 1px solid var(--border-default);
    background: var(--surface-base);
    transition: border-color 0.2s, box-shadow 0.2s;
  }
  .ha-composer:focus-within {
    border-color: color-mix(in oklab, var(--ha-a) 65%, var(--border-default));
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--ha-a) 16%, transparent);
  }
  .ha-composer.busy {
    border-color: transparent;
    background:
      linear-gradient(var(--surface-base), var(--surface-base)) padding-box,
      linear-gradient(100deg, var(--ha-b), var(--ha-a), var(--ha-c), var(--ha-b)) border-box;
    background-size: 100% 100%, 300% 100%;
    animation: ha-flow 2.4s linear infinite;
  }
  .ha-composer textarea {
    flex: 1;
    min-width: 0;
    resize: none;
    padding: 0.5rem 0.4rem;
    font-size: 13.5px;
    background: transparent;
    border: 0;
    outline: 0;
    color: inherit;
  }
  .ha-ic {
    display: grid;
    place-items: center;
    width: 2.4rem;
    height: 2.4rem;
    flex-shrink: 0;
    border-radius: 0.7rem;
    border: 1px solid var(--border-default);
    background: transparent;
    cursor: pointer;
    color: var(--text-secondary);
    transition: color 0.2s, border-color 0.2s;
  }
  .ha-ic:hover:not(:disabled) {
    color: var(--ha-a);
    border-color: var(--ha-a);
  }
  .ha-send {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    height: 2.4rem;
    padding: 0 1rem;
    flex-shrink: 0;
    border-radius: 0.7rem;
    border: 0;
    cursor: pointer;
    font-weight: 600;
    font-size: 13px;
    color: #fff;
    background: linear-gradient(135deg, var(--ha-a), color-mix(in oklab, var(--ha-c) 70%, var(--ha-a)));
    box-shadow: 0 6px 18px -8px color-mix(in oklab, var(--ha-a) 80%, transparent);
    transition: transform 0.15s, filter 0.2s, opacity 0.2s;
  }
  .ha-send:hover:not(:disabled) {
    transform: translateY(-1px);
    filter: brightness(1.08);
  }
  .ha-send:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  .ha-stop {
    background: var(--surface-sunken);
    color: var(--text-primary);
    box-shadow: none;
    border: 1px solid var(--border-default);
  }
  .ha-foot {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin-top: 0.4rem;
  }
  .ha-foot p {
    font-size: 11px;
    color: var(--text-tertiary);
  }
  .ha-temp {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.2rem 0.6rem;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    cursor: pointer;
    color: var(--text-secondary);
    border: 1px solid var(--border-default);
    background: transparent;
    transition: all 0.2s;
  }
  .ha-temp:disabled {
    cursor: default;
    opacity: 0.7;
  }
  .ha-temp.on {
    color: var(--color-warning-700, #b45309);
    border-color: color-mix(in oklab, #f59e0b 45%, transparent);
    background: color-mix(in oklab, #f59e0b 10%, transparent);
  }

  /* ---- keyframes ---- */
  @keyframes ha-flow {
    to {
      background-position: 200% 0;
    }
  }
  @keyframes ha-rise {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  @keyframes ha-ping {
    0% {
      box-shadow: 0 0 0 0 color-mix(in oklab, #22c55e 55%, transparent);
    }
    70%,
    100% {
      box-shadow: 0 0 0 8px transparent;
    }
  }
  @keyframes ha-pulse {
    0%,
    100% {
      transform: scale(1);
    }
    50% {
      transform: scale(1.12);
    }
  }
  @keyframes ha-blink {
    to {
      visibility: hidden;
    }
  }
  @keyframes ha-grid {
    to {
      background-position: 0 0, 22px 22px, 0 0;
    }
  }
  @keyframes ha-float {
    0% {
      transform: translateY(0) scale(0.6);
      opacity: 0;
    }
    20% {
      opacity: 0.75;
    }
    80% {
      opacity: 0.5;
    }
    100% {
      transform: translateY(-90px) scale(1.2);
      opacity: 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .ha-head::after,
    .ha-title,
    .ha-dot,
    .ha-stage,
    .ha-bg i,
    .ha-hero,
    .ha-sug,
    .ha-row,
    .ha-tool,
    .ha-avatar.busy,
    .ha-caret,
    .ha-composer.busy {
      animation: none;
    }
    .ha-bg {
      display: none;
    }
  }
</style>
