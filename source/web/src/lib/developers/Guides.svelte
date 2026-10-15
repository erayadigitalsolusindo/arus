<script lang="ts">
  import { untrack } from 'svelte';
  import { t } from '#lib/i18n/index.ts';
  import type { SpecInfo } from './api.ts';
  import CodeBlock from './CodeBlock.svelte';
  import { LANGS, snippet, type Lang } from './snippets.ts';
  import { VECTOR, hmacHex, stringToSign, newNonce, canonicalQuery, sha256Hex } from './sign.ts';

  export type GuideView = 'overview' | 'quickstart' | 'auth' | 'playground' | 'errors' | 'limits' | 'conventions';
  let { view, spec, base }: { view: GuideView; spec: SpecInfo; base: string } = $props();

  // ---- vektor uji ----
  let vectorSts = $state('');
  let vectorCheck = $state<'' | 'ok' | 'bad'>('');
  $effect(() => {
    if (view !== 'auth') return;
    void stringToSign({ method: VECTOR.method, path: VECTOR.path, query: VECTOR.query, timestamp: VECTOR.timestamp, nonce: VECTOR.nonce, body: VECTOR.body }).then((s) => (vectorSts = s));
  });
  async function verifyVector() {
    vectorCheck = (await hmacHex(VECTOR.secret, vectorSts)) === VECTOR.signature ? 'ok' : 'bad';
  }

  // ---- playground ----
  let pg = $state({ secret: 'sk_test_0123456789abcdef', method: 'POST', path: untrack(() => `${spec.prefix}/items`), query: 'page=1\nq=kopi susu', body: '{"name":"Kopi"}', ts: String(Math.floor(Date.now() / 1000)), nonce: newNonce() });
  let pgSts = $state('');
  let pgSig = $state('');
  let pgBodyHash = $state('');
  const pgQuery = $derived(
    pg.query
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .map((l) => {
        const i = l.indexOf('=');
        return (i < 0 ? [l, ''] : [l.slice(0, i), l.slice(i + 1)]) as [string, string];
      })
  );
  $effect(() => {
    if (view !== 'playground') return;
    const snapshot = { ...pg };
    const q = pgQuery;
    void (async () => {
      const sts = await stringToSign({ method: snapshot.method, path: snapshot.path, query: q, timestamp: snapshot.ts, nonce: snapshot.nonce, body: snapshot.body });
      pgSts = sts;
      pgBodyHash = await sha256Hex(snapshot.body);
      pgSig = await hmacHex(snapshot.secret, sts);
    })();
  });

  let lang = $state<Lang>('curl');
  const sample = $derived(
    LANGS.map((l) => ({
      id: l.id,
      label: l.label,
      lang: l.id === 'javascript' ? 'js' : l.id === 'curl' ? 'bash' : l.id,
      code: snippet(l.id, { base, method: 'GET', path: `${spec.prefix}/items`, query: [['q', 'kopi']], body: '' })
    }))
  );

  const errorRows: { status: number; code: keyof typeof codeKeys }[] = [
    { status: 401, code: 'SIGNATURE_MISSING' },
    { status: 401, code: 'SIGNATURE_INVALID' },
    { status: 401, code: 'TIMESTAMP_INVALID' },
    { status: 401, code: 'TIMESTAMP_SKEW' },
    { status: 401, code: 'REPLAY_DETECTED' },
    { status: 403, code: 'IP_NOT_ALLOWED' },
    { status: 401, code: 'CREDENTIAL_DISABLED' },
    { status: 401, code: 'CREDENTIAL_EXPIRED' },
    { status: 429, code: 'RATE_LIMITED' },
    { status: 403, code: 'FORBIDDEN' },
    { status: 403, code: 'OUTLET_FORBIDDEN' },
    { status: 422, code: 'VALIDATION' },
    { status: 404, code: 'NOT_FOUND' },
    { status: 413, code: 'BODY_TOO_LARGE' },
    { status: 503, code: 'UNAVAILABLE' }
  ];
  const codeKeys = {
    SIGNATURE_MISSING: 1, SIGNATURE_INVALID: 1, TIMESTAMP_INVALID: 1, TIMESTAMP_SKEW: 1, REPLAY_DETECTED: 1, IP_NOT_ALLOWED: 1,
    CREDENTIAL_DISABLED: 1, CREDENTIAL_EXPIRED: 1, RATE_LIMITED: 1, FORBIDDEN: 1, OUTLET_FORBIDDEN: 1, VALIDATION: 1, NOT_FOUND: 1,
    BODY_TOO_LARGE: 1, UNAVAILABLE: 1
  } as const;

  const hdr = $derived([
    { name: spec.headers.key, text: t('extapi.portal.auth.hKey') },
    { name: spec.headers.timestamp, text: t('extapi.portal.auth.hTimestamp') },
    { name: spec.headers.nonce, text: t('extapi.portal.auth.hNonce') },
    { name: spec.headers.signature, text: t('extapi.portal.auth.hSignature') }
  ]);
  const stsLines = $derived([
    ['HMAC-SHA256', t('extapi.portal.auth.stsLines.algo')],
    ['METHOD', t('extapi.portal.auth.stsLines.method')],
    ['PATH', t('extapi.portal.auth.stsLines.path')],
    ['QUERY', t('extapi.portal.auth.stsLines.query')],
    ['TIMESTAMP', t('extapi.portal.auth.stsLines.ts')],
    ['NONCE', t('extapi.portal.auth.stsLines.nonce')],
    ['BODY_SHA256', t('extapi.portal.auth.stsLines.body')]
  ]);
  const errorExample = `{
  "error": {
    "code": "SIGNATURE_INVALID",
    "message": "Kredensial atau tanda tangan tidak valid."
  }
}`;
</script>

{#if view === 'overview'}
  <h2 class="dv-h2">{t('extapi.portal.overview.title')}</h2>
  <p class="dv-lead">{t('extapi.portal.overview.lead')}</p>
  <div class="dv-cards">
    {#each [['braces', 'rest'], ['shield-check', 'hmac'], ['network', 'ip'], ['layers', 'scope']] as const as [icon, key] (key)}
      <div class="dv-card">
        <div class="dv-ico"><i class="icon-{icon}" style="font-size:17px"></i></div>
        <b>{t(`extapi.portal.overview.${key}.title`)}</b>
        <span>{t(`extapi.portal.overview.${key}.text`)}</span>
      </div>
    {/each}
  </div>
  <h3 class="dv-h3">{t('extapi.portal.auth.title')}</h3>
  <CodeBlock tabs={sample} bind:tab={lang} />
{:else if view === 'quickstart'}
  <h2 class="dv-h2">{t('extapi.portal.quick.title')}</h2>
  <p class="dv-lead">{t('extapi.portal.quick.intro')}</p>
  <div class="dv-steps">
    {#each ['s1', 's2', 's3', 's4'] as const as s (s)}
      <div class="dv-step"><b>{t(`extapi.portal.quick.${s}.title`)}</b><span>{t(`extapi.portal.quick.${s}.text`)}</span></div>
    {/each}
  </div>
  <CodeBlock tabs={sample} bind:tab={lang} />
{:else if view === 'auth'}
  <h2 class="dv-h2">{t('extapi.portal.auth.title')}</h2>
  <p class="dv-lead">{t('extapi.portal.auth.intro')}</p>

  <h3 class="dv-h3">{t('extapi.portal.auth.headersTitle')}</h3>
  <table class="dv-table">
    <thead><tr><th>{t('extapi.portal.auth.colHeader')}</th><th>{t('extapi.portal.auth.colMeaning')}</th></tr></thead>
    <tbody>{#each hdr as h (h.name)}<tr><td><code>{h.name}</code></td><td>{h.text}</td></tr>{/each}</tbody>
  </table>

  <h3 class="dv-h3">{t('extapi.portal.auth.stsTitle')}</h3>
  <p>{t('extapi.portal.auth.stsIntro')}</p>
  <table class="dv-table">
    <tbody>
      {#each stsLines as [name, text], i (name)}
        <tr><td class="mono" style="width:34px;color:var(--dv-muted)">{i + 1}</td><td style="width:150px"><code>{name}</code></td><td>{text}</td></tr>
      {/each}
    </tbody>
  </table>
  <div style="margin-top:14px"><CodeBlock code={t('extapi.portal.auth.formula')} lang="text" title="formula" /></div>

  <h3 class="dv-h3">{t('extapi.portal.auth.vectorTitle')}</h3>
  <p>{t('extapi.portal.auth.vectorIntro')}</p>
  <div style="display:grid;gap:12px">
    <div>
      <div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">{t('extapi.portal.auth.vSecret')}</div>
      <CodeBlock code={VECTOR.secret} lang="text" title="secret" />
    </div>
    <div>
      <div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">{t('extapi.portal.auth.vSts')}</div>
      <CodeBlock code={vectorSts} lang="text" title="string-to-sign" />
    </div>
    <div>
      <div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">{t('extapi.portal.auth.vSig')}</div>
      <CodeBlock code={VECTOR.signature} lang="text" title="X-Api-Signature" />
    </div>
    <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap">
      <button type="button" class="dv-btn" onclick={verifyVector}><i class="icon-zap" style="font-size:13px"></i>{t('extapi.portal.auth.verify')}</button>
      {#if vectorCheck === 'ok'}<span class="dv-status ok">{t('extapi.portal.auth.match')}</span>{/if}
      {#if vectorCheck === 'bad'}<span class="dv-status bad">{t('extapi.portal.auth.mismatch')}</span>{/if}
    </div>
  </div>
  <p class="mono" style="margin-top:12px;font-size:11.5px;color:var(--dv-muted)">query = {canonicalQuery(VECTOR.query)}</p>
{:else if view === 'playground'}
  <h2 class="dv-h2">{t('extapi.portal.playground.title')}</h2>
  <p class="dv-lead">{t('extapi.portal.playground.intro')}</p>
  <div style="display:grid;gap:22px;grid-template-columns:repeat(auto-fit,minmax(320px,1fr))">
    <div>
      <label class="dv-field"><span>{t('extapi.portal.playground.secret')}</span><input class="dv-input" bind:value={pg.secret} spellcheck="false" autocomplete="off" /></label>
      <div style="display:grid;grid-template-columns:110px 1fr;gap:10px">
        <label class="dv-field"><span>{t('extapi.portal.playground.method')}</span><input class="dv-input" bind:value={pg.method} spellcheck="false" /></label>
        <label class="dv-field"><span>{t('extapi.portal.playground.path')}</span><input class="dv-input" bind:value={pg.path} spellcheck="false" /></label>
      </div>
      <label class="dv-field"><span>{t('extapi.portal.playground.query')}</span><textarea class="dv-input" rows="3" bind:value={pg.query} spellcheck="false"></textarea></label>
      <label class="dv-field"><span>{t('extapi.portal.playground.body')}</span><textarea class="dv-input" rows="4" bind:value={pg.body} spellcheck="false"></textarea></label>
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px">
        <label class="dv-field"><span>{t('extapi.portal.playground.timestamp')}</span><input class="dv-input" bind:value={pg.ts} spellcheck="false" /></label>
        <label class="dv-field"><span>{t('extapi.portal.playground.nonce')}</span><input class="dv-input" bind:value={pg.nonce} spellcheck="false" /></label>
      </div>
      <div style="display:flex;gap:8px">
        <button type="button" class="dv-btn ghost" onclick={() => (pg.ts = String(Math.floor(Date.now() / 1000)))}>{t('extapi.portal.playground.now')}</button>
        <button type="button" class="dv-btn ghost" onclick={() => (pg.nonce = newNonce())}>{t('extapi.portal.playground.regenerate')}</button>
      </div>
    </div>
    <div style="display:grid;gap:12px;align-content:start;min-width:0">
      <div><div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">{t('extapi.portal.playground.sts')}</div><CodeBlock code={pgSts} lang="text" title="string-to-sign" /></div>
      <div><div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">body sha256</div><CodeBlock code={pgBodyHash} lang="text" title="sha256" /></div>
      <div><div class="mono" style="font-size:11px;color:var(--dv-muted);margin-bottom:4px">{t('extapi.portal.playground.signature')}</div><CodeBlock code={pgSig} lang="text" title="X-Api-Signature" /></div>
    </div>
  </div>
{:else if view === 'errors'}
  <h2 class="dv-h2">{t('extapi.portal.errors.title')}</h2>
  <p class="dv-lead">{t('extapi.portal.errors.intro')}</p>
  <CodeBlock code={errorExample} lang="json" title="application/json" />
  <div style="height:14px"></div>
  <table class="dv-table">
    <thead><tr><th>{t('extapi.portal.errors.colStatus')}</th><th>{t('extapi.portal.errors.colCode')}</th><th>{t('extapi.portal.errors.colMeaning')}</th></tr></thead>
    <tbody>
      {#each errorRows as r (r.code)}
        <tr>
          <td><span class="dv-status {r.status < 400 ? 'ok' : 'bad'}">{r.status}</span></td>
          <td><code>{r.code}</code></td>
          <td>{t(`extapi.portal.errors.codes.${r.code}`)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{:else if view === 'limits'}
  <h2 class="dv-h2">{t('extapi.portal.limits.title')}</h2>
  <div class="dv-cards">
    {#each [['network', 'ip'], ['gauge', 'rate'], ['file-text', 'body'], ['rocket', 'idem'], ['lock', 'rotate'], ['shield-check', 'scope']] as const as [icon, key] (key)}
      <div class="dv-card">
        <div class="dv-ico"><i class="icon-{icon}" style="font-size:17px"></i></div>
        <b>{t(`extapi.portal.limits.${key}Title`)}</b>
        <span>{key === 'rate' ? t('extapi.portal.limits.rateText', { rate: spec.default_rate_per_min }) : t(`extapi.portal.limits.${key}Text`)}</span>
      </div>
    {/each}
  </div>
{:else if view === 'conventions'}
  <h2 class="dv-h2">{t('extapi.portal.conventions.title')}</h2>
  <div class="dv-steps" style="margin-top:22px">
    {#each ['money', 'qty', 'dates', 'ids', 'pagination', 'errorsFormat'] as const as k (k)}
      <div class="dv-step"><span style="color:var(--dv-text)">{t(`extapi.portal.conventions.${k}`)}</span></div>
    {/each}
  </div>
{/if}
