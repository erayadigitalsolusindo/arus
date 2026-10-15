<script lang="ts">
  import { t } from '#lib/i18n/index.ts';
  import type { Endpoint } from './api.ts';
  import CodeBlock from './CodeBlock.svelte';
  import { LANGS, snippet, type Lang } from './snippets.ts';
  import { signRequest } from './sign.ts';
  import { tryCreds } from './try-state.svelte.ts';
  import { highlight } from './highlight.ts';

  let {
    ep,
    base,
    prefix,
    open,
    ontoggle
  }: { ep: Endpoint; base: string; prefix: string; open: boolean; ontoggle: () => void } = $props();

  const hasBody = $derived(['POST', 'PUT', 'PATCH'].includes(ep.method));
  const pathParams = $derived((ep.params ?? []).filter((p) => p.in === 'path'));
  const queryParams = $derived((ep.params ?? []).filter((p) => p.in === 'query'));
  const segments = $derived(ep.path.split(/(\{[^}]+\})/).filter(Boolean));

  const pretty = (v: unknown) => (v === undefined ? '' : JSON.stringify(v, null, 2));

  // Kolom isian "Coba": nilai awal diambil dari contoh request / daftar parameter query.
  let pathVals = $state<Record<string, string>>({});
  let queryRows = $state<{ k: string; v: string }[]>([]);
  let bodyText = $state('');
  let lang = $state<Lang>('curl');
  let tryOpen = $state(false);
  let busy = $state(false);
  let result = $state<{ status: number; ms: number; text: string; signed: string } | null>(null);
  let problem = $state('');
  let seeded = false;

  $effect(() => {
    if (!open || seeded) return;
    seeded = true;
    queryRows = queryParams.map((p) => ({ k: p.name, v: '' }));
    bodyText = hasBody ? (ep.request?.body !== undefined ? pretty(ep.request.body) : '{}') : '';
  });

  const livePath = $derived(prefix + ep.path.replace(/\{([^}]+)\}/g, (_, n: string) => pathVals[n]?.trim() || `YOUR_${n.toUpperCase()}`));
  const pathReady = $derived(pathParams.every((p) => /^[A-Za-z0-9._-]+$/.test(pathVals[p.name]?.trim() ?? '')));
  const liveQuery = $derived(queryRows.filter((r) => r.k.trim() !== '' && r.v !== '').map((r) => [r.k.trim(), r.v] as [string, string]));
  const liveBody = $derived(hasBody ? bodyText.trim() : '');

  const tabs = $derived(
    LANGS.map((l) => ({
      id: l.id,
      label: l.label,
      lang: l.id === 'javascript' ? 'js' : l.id === 'curl' ? 'bash' : l.id,
      code: snippet(l.id, { base, method: ep.method, path: livePath, query: liveQuery, body: liveBody })
    }))
  );

  const exampleResponses = $derived(ep.responses ?? []);

  async function send() {
    problem = '';
    result = null;
    if (!tryCreds.keyId.trim() || !tryCreds.secret) {
      problem = t('extapi.portal.try.needCreds');
      return;
    }
    if (!pathReady) {
      problem = t('extapi.portal.try.needPath');
      return;
    }
    busy = true;
    const started = performance.now();
    try {
      const { headers, stringToSign } = await signRequest(tryCreds.keyId.trim(), tryCreds.secret, { method: ep.method, path: livePath, query: liveQuery, body: liveBody });
      const qs = new URLSearchParams(liveQuery).toString();
      // URL memakai query yang SAMA dengan yang ditandatangani (urutan boleh beda; server mengurutkan ulang).
      const res = await fetch(`${base}${livePath}${qs ? `?${qs}` : ''}`, {
        method: ep.method,
        headers: { ...headers, Accept: 'application/json', ...(liveBody ? { 'Content-Type': 'application/json' } : {}) },
        body: liveBody || undefined
      });
      const raw = await res.text();
      let text = raw;
      try {
        text = JSON.stringify(JSON.parse(raw), null, 2);
      } catch {
        /* bukan JSON: tampilkan apa adanya */
      }
      result = { status: res.status, ms: Math.round(performance.now() - started), text, signed: stringToSign };
    } catch {
      problem = t('extapi.portal.try.networkError');
    } finally {
      busy = false;
    }
  }
</script>

<article class="dv-ep" id={`ep-${ep.method}-${ep.path}`} data-open={open}>
  <button type="button" class="dv-ep-head" aria-expanded={open} onclick={ontoggle}>
    <span class="dv-m" data-m={ep.method}>{ep.method}</span>
    <span class="dv-ep-path">{#each segments as s, i (i)}{#if s.startsWith('{')}<b>{s}</b>{:else}{s}{/if}{/each}</span>
    <span class="dv-ep-sum">{ep.summary}</span>
    <i class="icon-chevron-down" style="font-size:15px;opacity:.6;transition:transform .15s;{open ? 'transform:rotate(180deg)' : ''}"></i>
  </button>

  {#if open}
    <div class="dv-ep-body">
      <div style="min-width:0;padding-top:14px">
        <h3 class="dv-h3" style="margin-top:0">{ep.summary}</h3>
        {#if ep.description}<p>{ep.description}</p>{/if}

        <div style="margin:10px 0 14px">
          <span class="dv-chip accent">{t(`extapi.portal.ref.modes.${ep.auth}`)}</span>
          {#if ep.auth === 'jwt'}
            <div class="dv-note" style="margin-top:6px">{t('extapi.portal.ref.jwtNote')}</div>
          {:else if (ep.permissions ?? []).length}
            <div class="mono" style="font-size:11px;color:var(--dv-muted);margin:6px 0 4px">
              {t('extapi.portal.ref.permissions')}{(ep.permissions ?? []).length > 1 ? ` · ${t('extapi.portal.ref.permAny')}` : ''}
            </div>
            {#each ep.permissions ?? [] as p (p.module + p.action)}<span class="dv-chip"><i class="icon-key-round" style="font-size:11px"></i>{p.module}.{p.action}</span>{/each}
          {:else}
            <span class="dv-chip">{t('extapi.portal.ref.noPerm')}</span>
          {/if}
        </div>

        {#if (ep.params ?? []).length}
          <h3 class="dv-h3">{t('extapi.portal.ref.parameters')}</h3>
          <div style="overflow-x:auto">
            <table class="dv-table">
              <thead><tr><th>{t('extapi.portal.ref.colName')}</th><th>{t('extapi.portal.ref.colIn')}</th><th>{t('extapi.portal.ref.colType')}</th><th>{t('extapi.portal.ref.colDesc')}</th></tr></thead>
              <tbody>
                {#each ep.params ?? [] as p (p.in + p.name)}
                  <tr>
                    <td class="nw"><code>{p.name}</code>{#if p.required}<span class="dv-req">{t('extapi.portal.ref.required')}</span>{/if}</td>
                    <td class="mono nw" style="font-size:12px;color:var(--dv-muted)">{p.in}</td>
                    <td class="mono nw" style="font-size:12px">{p.type ?? ''}</td>
                    <td>{p.description ?? ''}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        {#if (ep.errors ?? []).length}
          <h3 class="dv-h3">{t('extapi.portal.ref.errors')}</h3>
          <table class="dv-table">
            <tbody>
              {#each ep.errors ?? [] as e (e.code)}
                <tr><td class="mono nw" style="width:60px">{e.status}</td><td class="nw"><code>{e.code}</code></td><td>{e.description}</td></tr>
              {/each}
            </tbody>
          </table>
        {/if}

        {#if (ep.notes ?? []).length}
          <h3 class="dv-h3">{t('extapi.portal.ref.notes')}</h3>
          <ul style="margin:0;padding-left:18px;color:var(--dv-muted)">{#each ep.notes ?? [] as n (n)}<li>{n}</li>{/each}</ul>
        {/if}

        {#if !ep.documented}
          <div class="dv-note" style="margin-top:16px">{t('extapi.portal.ref.undocumented')}</div>
        {/if}
      </div>

      <div style="min-width:0;padding-top:14px;display:grid;gap:14px;align-content:start">
        {#if ep.auth === 'hmac'}
          <CodeBlock {tabs} bind:tab={lang} />
        {/if}

        {#each exampleResponses as r, i (i)}
          <div>
            <div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:5px">
              {t('extapi.portal.ref.response')} <span class="dv-status {(r.status ?? 200) < 400 ? 'ok' : 'bad'}">{r.status ?? 200}</span> {r.description ?? ''}
            </div>
            <CodeBlock code={JSON.stringify(r.body ?? {}, null, 2)} lang="json" title="application/json" />
          </div>
        {/each}

        {#if ep.auth === 'hmac'}
          <div class="dv-card" style="padding:0">
            <button type="button" class="dv-ep-head" aria-expanded={tryOpen} onclick={() => (tryOpen = !tryOpen)}>
              <i class="icon-terminal" style="color:var(--dv-accent)"></i><b>{t('extapi.portal.try.title')}</b>
              <i class="icon-chevron-down" style="margin-left:auto;font-size:14px;opacity:.6;{tryOpen ? 'transform:rotate(180deg)' : ''}"></i>
            </button>
            {#if tryOpen}
              <div style="padding:2px 14px 14px;border-top:1px solid var(--dv-border-soft)">
                <div class="dv-warn" style="margin:12px 0">{t('extapi.portal.try.secretHint')}</div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px">
                  <label class="dv-field"><span>{t('extapi.portal.try.keyId')}</span><input class="dv-input" bind:value={tryCreds.keyId} autocomplete="off" spellcheck="false" placeholder="ak_…" /></label>
                  <label class="dv-field"><span>{t('extapi.portal.try.secret')}</span><input class="dv-input" type="password" bind:value={tryCreds.secret} autocomplete="off" placeholder="sk_…" /></label>
                </div>
                {#if pathParams.length}
                  <div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:6px">{t('extapi.portal.try.pathParams')}</div>
                  {#each pathParams as p (p.name)}
                    <label class="dv-field"><span>{p.name}</span><input class="dv-input" bind:value={pathVals[p.name]} autocomplete="off" spellcheck="false" /></label>
                  {/each}
                {/if}
                <div class="mono" style="font-size:11px;color:var(--dv-muted);margin:4px 0 6px">{t('extapi.portal.try.queryParams')}</div>
                {#each queryRows as row, i (i)}
                  <div style="display:grid;grid-template-columns:1fr 1.4fr auto;gap:6px;margin-bottom:6px">
                    <input class="dv-input" bind:value={row.k} aria-label="name" placeholder="name" spellcheck="false" />
                    <input class="dv-input" bind:value={row.v} aria-label="value" placeholder="value" spellcheck="false" />
                    <button type="button" class="dv-iconbtn" aria-label="remove" onclick={() => (queryRows = queryRows.filter((_, j) => j !== i))}><i class="icon-x" style="font-size:12px"></i></button>
                  </div>
                {/each}
                <button type="button" class="dv-btn ghost" style="padding:4px 10px;font-size:12px;margin-bottom:10px" onclick={() => (queryRows = [...queryRows, { k: '', v: '' }])}>
                  <i class="icon-plus" style="font-size:12px"></i>{t('extapi.portal.try.addParam')}
                </button>
                {#if hasBody}
                  <label class="dv-field"><span>{t('extapi.portal.try.body')}</span><textarea class="dv-input" rows="6" bind:value={bodyText} spellcheck="false"></textarea></label>
                {/if}
                <button type="button" class="dv-btn" disabled={busy} onclick={send}>
                  <i class="icon-send" style="font-size:13px"></i>{busy ? t('extapi.portal.try.sending') : t('extapi.portal.try.send')}
                </button>
                {#if problem}<div class="dv-warn" style="margin-top:10px">{problem}</div>{/if}
                {#if result}
                  <div style="margin-top:14px">
                    <div style="display:flex;gap:10px;align-items:center;margin-bottom:6px">
                      <span class="mono" style="font-size:11px;color:var(--dv-muted)">{t('extapi.portal.try.status')}</span>
                      <span class="dv-status {result.status < 400 ? 'ok' : 'bad'}">{result.status}</span>
                      <span class="mono" style="font-size:11px;color:var(--dv-muted)">{t('extapi.portal.try.time', { ms: result.ms })}</span>
                    </div>
                    <CodeBlock code={result.text || '∅'} lang="json" title={t('extapi.portal.try.result')} />
                    <details style="margin-top:8px">
                      <summary class="mono" style="font-size:11px;color:var(--dv-muted);cursor:pointer">{t('extapi.portal.try.signed')}</summary>
                      <!-- eslint-disable-next-line svelte/no-at-html-tags -- highlight() meng-escape semua teks -->
                      <pre class="mono" style="font-size:11.5px;white-space:pre-wrap;word-break:break-all;color:var(--dv-muted)">{@html highlight(result.signed, 'text')}</pre>
                    </details>
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</article>
