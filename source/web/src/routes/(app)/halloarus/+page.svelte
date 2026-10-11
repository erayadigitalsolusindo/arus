<script lang="ts">
  import { onMount, tick } from 'svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';
  import Select from '#lib/components/Select.svelte';
  import { ai, type AiSettings, type ChatMessage, type Connection, type Provider, type ToolCall } from '#lib/ai/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t, tryT, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // HalloArus: asisten AI hanya-baca. Riwayat percakapan hanya hidup di halaman ini (tidak disimpan di server);
  // yang dikirim ke server hanya teks user/assistant, hasil tool selalu dibuat server.
  type Turn = ChatMessage & { tools?: ToolCall[]; tokens?: { input: number; output: number }; failed?: boolean };

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

  // Form pengaturan
  let keyInput = $state('');
  let provSel = $state<Provider>('anthropic');
  let modelSel = $state('');
  let saving = $state(false);
  let saveMsg = $state('');
  const canManage = can('ai_assistant', 'update');

  async function loadSettings() {
    try {
      settings = await ai.settings();
      provSel = settings.provider;
      modelSel = cur?.model ?? '';
      showSettings = !settings.active && canManage;
    } catch (e) {
      error = errorMessage(e);
      settings = { configured: false, active: false, free: { enabled: false, limit: 0, remaining: 0, model: '' }, provider: 'anthropic', providers: [] };
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
    if (!cur?.configured && !keyInput.trim()) {
      error = t('ai.setup.needKey', { provider: provName });
      return;
    }
    saving = true;
    try {
      settings = await ai.save({ provider: provSel, api_key: keyInput.trim() || undefined, model: modelSel });
      provSel = settings.provider;
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

  async function send(text = input) {
    text = text.trim();
    if (!text || sending || !settings?.active) return;
    error = '';
    input = '';
    turns.push({ role: 'user', content: text });
    // Hanya teks percakapan yang dikirim; turn yang gagal tidak ikut.
    const history: ChatMessage[] = turns.filter((m) => !m.failed).map((m) => ({ role: m.role, content: m.content }));
    sending = true;
    abort = new AbortController();
    void scrollDown();
    try {
      const res = await ai.chat(history, abort.signal);
      turns.push({ role: 'assistant', content: res.reply, tools: res.tools_used, tokens: { input: res.usage.input_tokens, output: res.usage.output_tokens } });
      if (res.free && settings) settings.free = res.free;
    } catch (e) {
      if ((e as Error)?.name === 'AbortError') {
        turns.pop();
        input = text;
      } else {
        turns[turns.length - 1].failed = true;
        error = errorMessage(e);
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
  }

  const toolLabel = (n: string) => tryT(`ai.tools.${n}`) ?? n;
  const uniqueTools = (calls: ToolCall[] = []) => [...new Set(calls.map((c) => c.name))];
  const cur = $derived(settings?.providers.find((p) => p.id === provSel));
  const provName = $derived(tryT(`ai.providers.${provSel}.name`) ?? provSel);
  const modelOptions = $derived((cur?.models ?? []).map((m) => ({ value: m, label: tryT(`ai.models.${m}`) ?? m })));
  const providerOptions = $derived(
    (settings?.providers ?? []).map((p) => ({ value: p.id, label: (tryT(`ai.providers.${p.id}.name`) ?? p.id) + (p.configured ? (p.id === settings?.provider ? ' ✓' : ' •') : '') }))
  );
  // Ganti penyedia di form: model ikut penyedia itu, isian key dikosongkan.
  function pickProvider() {
    keyInput = '';
    modelSel = cur?.model ?? '';
  }
  const suggestionKeys = ['s1', 's2', 's3', 's4', 's5', 's6'] as const;
</script>

<main class="p-4 lg:p-6 max-w-4xl mx-auto w-full min-w-0 flex flex-col gap-4" style="min-height: calc(100dvh - 5rem)">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="flex items-start gap-3">
      <span class="grid place-items-center size-11 shrink-0 rounded-xl bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)] text-[var(--color-primary-600)]"><i class="icon-sparkles text-[22px]"></i></span>
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="font-display font-bold text-[19px] leading-tight">{t('ai.title')}</h1>
          <span class="badge badge-soft badge-success badge-small" title={t('ai.readOnlyHint')}><i class="icon-shield-check"></i> {t('ai.readOnly')}</span>
        </div>
        <p class="text-[12.5px] mt-0.5 text-[var(--text-tertiary)]">{t('ai.subtitle')}</p>
      </div>
    </div>
    <div class="flex items-center gap-2">
      {#if settings?.connector_url}
        <button type="button" class="btn btn-sm" aria-expanded={showConnector} onclick={toggleConnector}><i class="icon-plug"></i> {t('ai.connector.button')}</button>
      {/if}
      {#if turns.length > 0}
        <button type="button" class="btn btn-sm" onclick={reset}><i class="icon-square-pen"></i> {t('ai.input.clear')}</button>
      {/if}
      {#if canManage && settings?.active}
        <button type="button" class="btn btn-sm" aria-expanded={showSettings} onclick={() => (showSettings = !showSettings)}><i class="icon-settings"></i> {t('ai.settings')}</button>
      {/if}
    </div>
  </div>

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
    {#if showSettings && canManage}
      <form class="surface-card p-5 space-y-4" onsubmit={(e) => { e.preventDefault(); void saveSettings(); }}>
        <div class="flex items-start gap-3">
          <span class="grid place-items-center size-9 shrink-0 rounded-lg bg-[var(--surface-sunken)] text-[var(--color-primary-600)]"><i class="icon-key-round text-[18px]"></i></span>
          <div class="min-w-0">
            <h2 class="font-semibold text-[14px]">{t('ai.setup.title')}</h2>
            <p class="text-[12.5px] mt-1 leading-relaxed text-[var(--text-secondary)]">{settings.configured ? '' : t('ai.setup.notConfiguredManage') + ' '}{t('ai.setup.intro', { console: tryT(`ai.providers.${provSel}.console`) ?? '' })}</p>
          </div>
        </div>
        <div class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.providerLabel')}</span>
          <div class="mt-1"><Select bind:value={provSel} options={providerOptions} ariaLabel={t('ai.setup.providerLabel')} onchange={pickProvider} /></div>
        </div>
        <label class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.keyLabel', { provider: provName })}</span>
          <input type="password" bind:value={keyInput} autocomplete="off" spellcheck="false" maxlength="300" placeholder={tryT(`ai.providers.${provSel}.placeholder`) ?? ''}
            class="mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] font-mono" />
          <span class="block mt-1 text-[11.5px] text-[var(--text-tertiary)]">
            {cur?.configured ? t('ai.setup.keySaved', { hint: cur.key_hint ?? '' }) + ' ' + t('ai.setup.keyReplaceHint') : t('ai.setup.keyNone')}
          </span>
        </label>
        <div class="block text-[12.5px]">
          <span class="font-medium">{t('ai.setup.modelLabel')}</span>
          <div class="mt-1"><Select bind:value={modelSel} options={modelOptions} ariaLabel={t('ai.setup.modelLabel')} /></div>
        </div>
        <div class="flex flex-wrap items-center gap-2 pt-1">
          <button type="submit" class="btn btn-primary" disabled={saving}>{saving ? t('ai.setup.saving') : t('ai.setup.save')}</button>
          {#if cur?.configured}
            <button type="button" class="btn" onclick={removeKey} disabled={saving}><i class="icon-trash-2"></i> {t('ai.setup.removeKey', { provider: provName })}</button>
          {/if}
        </div>
      </form>
    {/if}

    {#if !settings.active}
      {#if !canManage}
        <div class="surface-card p-4 flex items-center gap-3 text-[13px] text-[var(--text-secondary)]">
          <i class="icon-key-round text-[20px] text-[var(--text-tertiary)]"></i><span>{t('ai.setup.notConfigured')}</span>
        </div>
      {/if}
    {:else}
      {#if settings.free.enabled && !settings.configured}
        <div class="flex flex-wrap items-center justify-between gap-2 rounded-lg px-3 py-2 text-[12.5px] bg-[var(--surface-sunken)] text-[var(--text-secondary)]">
          <span><i class="icon-gift"></i> {settings.free.remaining > 0 ? t('ai.free.banner', { remaining: settings.free.remaining, limit: settings.free.limit }) : t('ai.free.exhausted')}</span>
          {#if canManage}<button type="button" class="btn btn-sm" onclick={() => (showSettings = true)}><i class="icon-key-round"></i> {t('ai.free.useOwn')}</button>{/if}
        </div>
      {/if}
      <div bind:this={scroller} class="surface-card flex-1 overflow-y-auto space-y-4 !p-4" style="min-height: 18rem; max-height: calc(100dvh - 22rem)" aria-live="polite">
        {#if turns.length === 0}
          <div class="flex flex-col items-center text-center gap-3 py-8">
            <span class="grid place-items-center size-14 rounded-full bg-[var(--surface-sunken)] text-[var(--color-primary-600)]"><i class="icon-bot text-[26px]"></i></span>
            <div>
              <p class="font-semibold text-[14px]">{t('ai.empty.title')}</p>
              <p class="text-[12.5px] mt-1 max-w-md text-[var(--text-tertiary)]">{t('ai.empty.hint')}</p>
            </div>
            <div class="flex flex-wrap justify-center gap-2 max-w-2xl mt-1">
              {#each suggestionKeys as k (k)}
                <button type="button" class="btn btn-sm text-start" onclick={() => send(t(`ai.suggestions.${k}`))} disabled={sending}>{t(`ai.suggestions.${k}`)}</button>
              {/each}
            </div>
          </div>
        {/if}

        {#each turns as m, i (i)}
          {#if m.role === 'user'}
            <div class="flex justify-end">
              <div class="max-w-[85%] rounded-2xl rounded-ee-sm px-3.5 py-2.5 text-[13.5px] whitespace-pre-wrap bg-[var(--color-primary-600)] text-white" class:opacity-60={m.failed}>{m.content}</div>
            </div>
          {:else}
            <div class="flex gap-2.5">
              <span class="grid place-items-center size-8 shrink-0 rounded-full bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)] text-[var(--color-primary-600)]"><i class="icon-sparkles text-[16px]"></i></span>
              <div class="min-w-0 flex-1">
                <div class="text-[13.5px] leading-relaxed"><MarkdownView source={m.content} /></div>
                {#if m.tools?.length}
                  <div class="mt-2 flex flex-wrap items-center gap-1.5 text-[11.5px] text-[var(--text-tertiary)]">
                    <span>{t('ai.dataRead')}:</span>
                    {#each uniqueTools(m.tools) as name (name)}
                      <span class="badge badge-soft badge-neutral badge-small">{toolLabel(name)}</span>
                    {/each}
                  </div>
                {/if}
                {#if m.tokens}
                  <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">{t('ai.usage', { input: formatNumber(m.tokens.input), output: formatNumber(m.tokens.output) })}</p>
                {/if}
              </div>
            </div>
          {/if}
        {/each}

        {#if sending}
          <div class="flex items-center gap-2.5 text-[12.5px] text-[var(--text-tertiary)]" role="status">
            <i class="icon-loader-circle animate-spin text-[16px]"></i><span>{t('ai.thinking')}</span>
          </div>
        {/if}
      </div>

      <div class="space-y-1.5">
        <div class="flex items-end gap-2">
          <textarea bind:value={input} onkeydown={onKey} rows="2" maxlength="6000" disabled={sending || (settings.free.enabled && !settings.configured && settings.free.remaining <= 0)} placeholder={t('ai.input.placeholder')} aria-label={t('ai.input.placeholder')}
            class="flex-1 resize-none px-3 py-2.5 rounded-xl border border-[var(--border-default)] bg-[var(--surface-base)] text-[13.5px]"></textarea>
          {#if sending}
            <button type="button" class="btn" onclick={() => abort?.abort()}><i class="icon-x"></i> {t('ai.input.stop')}</button>
          {:else}
            <button type="button" class="btn btn-primary" onclick={() => send()} disabled={!input.trim()}><i class="icon-send"></i> {t('ai.input.send')}</button>
          {/if}
        </div>
        <p class="text-[11px] text-[var(--text-tertiary)]">{t('ai.privacy')} {t('ai.disclaimer')}</p>
      </div>
    {/if}
  {/if}
</main>
