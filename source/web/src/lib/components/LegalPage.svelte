<script lang="ts">
  import { onMount } from 'svelte';
  import { t, type MessageKey } from '#lib/i18n/index.ts';
  import LoginFooter from '#lib/components/LoginFooter.svelte';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';

  // Satu komponen untuk Syarat Layanan dan Kebijakan Privasi; isi ada di kamus `legal.*` (id/en/ja).
  // Tampilan: jendela terminal di atas latar "hujan kode", dengan pencarian dan daftar isi yang menyorot bagian aktif.
  // Format badan teks: paragraf dipisah baris kosong; baris "- " = butir; baris "| a | b" = tabel (baris pertama = judul kolom).
  let { doc, sections }: { doc: 'terms' | 'privacy' | 'help' | 'delete'; sections: readonly string[] } = $props();
  const key = (s: string, part: 'title' | 'body') => `legal.${doc}.${s}.${part}` as MessageKey;

  type Block = { kind: 'p'; text: string } | { kind: 'ul'; items: string[] } | { kind: 'table'; head: string[]; rows: string[][] };

  function parse(body: string): Block[] {
    const out: Block[] = [];
    for (const chunk of body.split(/\n\s*\n/)) {
      const lines = chunk.split('\n').map((l) => l.trimEnd()).filter(Boolean);
      if (!lines.length) continue;
      if (lines.every((l) => l.startsWith('| '))) {
        const cells = lines.map((l) => l.slice(2).split(' | ').map((c) => c.trim()));
        out.push({ kind: 'table', head: cells[0], rows: cells.slice(1) });
      } else if (lines.every((l) => l.startsWith('- '))) {
        out.push({ kind: 'ul', items: lines.map((l) => l.slice(2)) });
      } else {
        out.push({ kind: 'p', text: lines.join(' ') });
      }
    }
    return out;
  }

  // ---- pencarian ----
  let q = $state('');
  let input: HTMLInputElement | undefined = $state();
  const needle = $derived(q.trim().toLowerCase());
  const searching = $derived(needle.length >= 2);

  const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const countIn = (text: string) => (searching ? text.toLowerCase().split(needle).length - 1 : 0);

  const sectionText = (s: string) => `${t(key(s, 'title'))}\n${t(key(s, 'body'))}`;
  const hits = $derived(Object.fromEntries(sections.map((s) => [s, countIn(sectionText(s))])));
  const total = $derived(Object.values(hits).reduce((a, b) => a + b, 0));
  const visible = $derived(searching ? sections.filter((s) => hits[s] > 0) : sections);

  // Potong teks menjadi bagian biasa / cocok pencarian / alamat email.
  const EMAIL = /([\w.+-]+@[\w-]+(?:\.[\w-]+)+)/;
  type Chunk = { s: string; hit: boolean; mail: boolean };
  function chunks(text: string): Chunk[] {
    const out: Chunk[] = [];
    text.split(EMAIL).forEach((seg, i) => {
      if (!seg) return;
      if (i % 2 === 1) {
        out.push({ s: seg, hit: searching && seg.toLowerCase().includes(needle), mail: true });
      } else if (searching) {
        seg.split(new RegExp(`(${esc(needle)})`, 'i')).forEach((p, j) => {
          if (p) out.push({ s: p, hit: j % 2 === 1, mail: false });
        });
      } else {
        out.push({ s: seg, hit: false, mail: false });
      }
    });
    return out;
  }

  function onKey(e: KeyboardEvent) {
    const typing = e.target instanceof HTMLElement && ['INPUT', 'TEXTAREA'].includes(e.target.tagName);
    if (e.key === '/' && !typing && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      input?.focus();
    } else if (e.key === 'Escape' && q) {
      q = '';
    }
  }

  // ---- bagian aktif ----
  let active = $state('');
  let docEl: HTMLElement | undefined = $state();
  let lockUntil = 0;

  // Aktif = bagian terakhir yang kepalanya sudah melewati 30% tinggi kontainer; di dasar gulir = bagian terakhir.
  function sync() {
    if (!docEl || Date.now() < lockUntil || !visible.length) return;
    const box = docEl.getBoundingClientRect();
    let cur = visible[0];
    for (const s of visible) {
      const el = document.getElementById(`sec-${s}`);
      if (el && el.getBoundingClientRect().top - box.top <= box.height * 0.3) cur = s;
    }
    if (docEl.scrollTop + docEl.clientHeight >= docEl.scrollHeight - 4) cur = visible[visible.length - 1];
    active = cur;
  }

  function jump(e: MouseEvent, s: string) {
    e.preventDefault();
    const el = document.getElementById(`sec-${s}`);
    if (!el || !docEl) return;
    active = s;
    lockUntil = Date.now() + 700;
    const top = el.getBoundingClientRect().top - docEl.getBoundingClientRect().top + docEl.scrollTop - 12;
    docEl.scrollTo({ top, behavior: 'smooth' });
  }

  $effect(() => {
    void needle;
    if (docEl) docEl.scrollTop = 0;
    queueMicrotask(sync);
  });

  // ---- tema (sama dengan tombol di footer: atribut data-theme + localStorage 'theme') ----
  let dark = $state(true);
  const readTheme = () => (document.documentElement.dataset.theme ?? 'light') === 'dark';

  function toggleTheme() {
    dark = !dark;
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
    try {
      localStorage.setItem('theme', dark ? 'dark' : 'light');
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }

  // ---- latar hujan kode ----
  let canvas: HTMLCanvasElement | undefined = $state();

  onMount(() => {
    dark = readTheme();
    const mo = new MutationObserver(() => (dark = readTheme()));
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    sync();
    const c = canvas;
    const ctx = c?.getContext('2d');
    if (!c || !ctx || matchMedia('(prefers-reduced-motion: reduce)').matches) return () => mo.disconnect();

    const glyphs = '01{}[]<>/=;:$#ABCDEFabcdef'.split('');
    const size = 15;
    let cols = 0;
    let drops: number[] = [];
    let raf = 0;
    let last = 0;

    const resize = () => {
      c.width = innerWidth;
      c.height = innerHeight;
      cols = Math.ceil(c.width / size);
      drops = Array.from({ length: cols }, () => Math.random() * -40);
    };
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      if (now - last < 70) return;
      last = now;
      ctx.fillStyle = dark ? 'rgba(5, 10, 20, 0.16)' : 'rgba(241, 245, 249, 0.2)';
      ctx.fillRect(0, 0, c.width, c.height);
      ctx.font = `${size - 2}px ui-monospace, Consolas, monospace`;
      for (let i = 0; i < cols; i++) {
        const y = drops[i] * size;
        ctx.fillStyle = Math.random() > 0.96 ? (dark ? '#bbf7d0' : '#15803d') : dark ? '#22c55e' : '#4ade80';
        ctx.fillText(glyphs[Math.floor(Math.random() * glyphs.length)], i * size, y);
        if (y > c.height && Math.random() > 0.975) drops[i] = 0;
        drops[i] += 1;
      }
    };
    const onVis = () => {
      cancelAnimationFrame(raf);
      if (!document.hidden) raf = requestAnimationFrame(frame);
    };

    resize();
    raf = requestAnimationFrame(frame);
    addEventListener('resize', resize);
    document.addEventListener('visibilitychange', onVis);
    return () => {
      mo.disconnect();
      cancelAnimationFrame(raf);
      removeEventListener('resize', resize);
      document.removeEventListener('visibilitychange', onVis);
    };
  });
</script>

<svelte:head><title>{t(`legal.${doc}.docTitle` as MessageKey)}</title></svelte:head>
<svelte:window onkeydown={onKey} />

{#snippet hl(text: string)}{#each chunks(text) as c, i (i)}{#if c.mail}<a href="mailto:{c.s}" class="mail">{#if c.hit}<mark>{c.s}</mark>{:else}{c.s}{/if}</a>{:else if c.hit}<mark>{c.s}</mark>{:else}{c.s}{/if}{/each}{/snippet}

<div class="page min-h-screen flex flex-col px-4 sm:px-6 py-10 relative overflow-hidden">
  <canvas bind:this={canvas} class="rain" aria-hidden="true"></canvas>
  <div class="grid-bg" aria-hidden="true"></div>
  <div class="glow g1" aria-hidden="true"></div>
  <div class="glow g2" aria-hidden="true"></div>
  <div class="scan" aria-hidden="true"></div>

  <div class="relative flex-1 w-full mx-auto max-w-[1000px]">
    <div class="ctrl">
      <LanguageSwitcher />
      <button type="button" class="ctrl-btn" onclick={toggleTheme} aria-label={dark ? t('common.theme.light') : t('common.theme.dark')} title={dark ? t('common.theme.light') : t('common.theme.dark')}>
        <i class={dark ? 'icon-sun-medium' : 'icon-moon'}></i>
      </button>
    </div>
    <a href="/login" class="flex justify-center" aria-label="ARUS">
      <img src="/logo_dengan_text-no-bg.svg" alt="ARUS" class="h-14 w-auto logo" />
    </a>

    <div class="term mt-6">
      <div class="term-bar">
        <span class="d" style="background:#ef4444"></span>
        <span class="d" style="background:#f59e0b"></span>
        <span class="d" style="background:#22c55e"></span>
        <span class="term-title">arus@{doc === 'help' ? 'help' : 'legal'}: ~/{doc}.md</span>
      </div>

      <div class="searchbar">
        <label for="legal-q" class="sb-prompt"><span class="ps">$</span> grep -i</label>
        <input
          id="legal-q"
          bind:this={input}
          bind:value={q}
          type="search"
          autocomplete="off"
          spellcheck="false"
          placeholder={t(doc === 'terms' ? 'legal.search.placeholderTerms' : doc === 'help' ? 'legal.search.placeholderHelp' : doc === 'delete' ? 'legal.search.placeholderDelete' : 'legal.search.placeholder')}
          class="sb-input"
        />
        {#if searching}
          <span class="sb-count" class:none={total === 0} role="status" aria-live="polite">
            {total === 0 ? t('legal.search.none') : t('legal.search.count', { count: total, sections: visible.length })}
          </span>
        {:else}
          <kbd class="sb-key" title={t('legal.search.hint')}>/</kbd>
        {/if}
        {#if q}
          <button type="button" class="sb-clear" onclick={() => { q = ''; input?.focus(); }} aria-label={t('legal.search.clear')}>esc</button>
        {/if}
      </div>

      <div class="term-wrap">
        <nav class="toc" aria-label={t(`legal.${doc}.title` as MessageKey)}>
          <p class="toc-h">$ ls sections/</p>
          {#each sections as s (s)}
            <a
              href="#sec-{s}"
              class="toc-a"
              class:on={active === s && (!searching || hits[s] > 0)}
              class:dim={searching && hits[s] === 0}
              onclick={(e) => jump(e, s)}
            >
              <span class="toc-n">{s.slice(1).padStart(2, '0')}</span>
              <span class="toc-t">{t(key(s, 'title')).replace(/^\d+\.\s*/, '')}</span>
              {#if searching && hits[s] > 0}<span class="toc-c">{hits[s]}</span>{/if}
            </a>
          {/each}
        </nav>

        <article class="doc scroll-thin" bind:this={docEl} onscroll={sync}>
          <p class="prompt"><span class="ps">$</span> cat {doc}.md{#if searching}{` | grep -i "${q.trim()}"`}{/if}</p>
          <h1 class="doc-title"><span class="hash">#</span> {t(`legal.${doc}.title` as MessageKey)}</h1>
          {#if doc !== 'help'}
            <p class="meta"><span class="kk">effective</span>: <span class="vv">{t('legal.updated')}</span></p>
          {/if}

          {#if searching && !visible.length}
            <p class="empty"><span class="lv">ERR</span> {t('legal.search.empty', { q: q.trim() })}</p>
          {/if}

          {#each visible as s (s)}
            <section id="sec-{s}" class="sec">
              <h2><span class="hash">##</span> {@render hl(t(key(s, 'title')))}</h2>
              {#each parse(t(key(s, 'body'))) as b, bi (bi)}
                {#if b.kind === 'p'}
                  <p>{@render hl(b.text)}</p>
                {:else if b.kind === 'ul'}
                  <ul>
                    {#each b.items as it, ii (ii)}<li>{@render hl(it)}</li>{/each}
                  </ul>
                {:else}
                  <div class="tbl-wrap">
                    <table>
                      <thead><tr>{#each b.head as h, hi (hi)}<th>{@render hl(h)}</th>{/each}</tr></thead>
                      <tbody>
                        {#each b.rows as r, ri (ri)}
                          <tr>{#each r as cell, ci (ci)}<td data-label={b.head[ci]}>{@render hl(cell)}</td>{/each}</tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                {/if}
              {/each}
            </section>
          {/each}

          <p class="prompt end"><span class="ps">$</span> <span class="cursor"></span></p>
        </article>
      </div>
    </div>

    <button type="button" class="back" onclick={() => history.back()}>← {t('legal.back')}</button>
  </div>
  <div class="relative w-full mx-auto max-w-[1000px] foot">
    <LoginFooter />
  </div>
</div>

<style>
  /* Dua mode: gelap (bawaan terminal) dan terang. Variabel diganti oleh data-theme di <html>. */
  .page {
    --bg: #f1f5f9; --win: rgb(255 255 255 / 0.94); --bar: #e2e8f0; --bar2: #f8fafc; --bar3: #f1f5f9;
    --line: #cbd5e1; --line2: #e2e8f0; --fg: #334155; --muted: #64748b; --dim: #94a3b8; --head: #0f172a;
    --acc: #16a34a; --acc-bg: rgb(22 163 74 / 0.1); --h2: #6d28d9; --key: #0369a1; --val: #15803d;
    --hover: #eef2f7; --mark-bg: rgb(250 204 21 / 0.5); --mark-fg: #713f12; --err: #dc2626; --err-bg: rgb(220 38 38 / 0.08);
    --count: #a16207; --count-bg: rgb(234 179 8 / 0.2); --grid: rgb(100 116 139 / 0.1); --rain: 0.1; --glow: 0.12; --scan: 0;
    --shadow: 0 24px 60px -28px rgb(15 23 42 / 0.35), 0 0 50px -34px rgb(22 163 74 / 0.6);
    background: var(--bg); color: var(--fg); isolation: isolate;
  }
  :global(:root[data-theme='dark']) .page {
    --bg: #050a14; --win: rgb(11 18 32 / 0.94); --bar: #0f172a; --bar2: #0b1220; --bar3: #0d1628;
    --line: #1e293b; --line2: #16213a; --fg: #cbd5e1; --muted: #64748b; --dim: #475569; --head: #f1f5f9;
    --acc: #4ade80; --acc-bg: rgb(34 197 94 / 0.1); --h2: #c4b5fd; --key: #7dd3fc; --val: #86efac;
    --hover: #0f1a30; --mark-bg: rgb(250 204 21 / 0.28); --mark-fg: #fde68a; --err: #f87171; --err-bg: rgb(248 113 113 / 0.06);
    --count: #facc15; --count-bg: rgb(250 204 21 / 0.16); --grid: rgb(148 163 184 / 0.06); --rain: 0.16; --glow: 0.22; --scan: 0.35;
    --shadow: 0 0 0 1px rgb(34 197 94 / 0.06), 0 24px 70px -26px rgb(0 0 0 / 0.9), 0 0 60px -30px rgb(34 197 94 / 0.5);
  }
  .rain { position: fixed; inset: 0; width: 100%; height: 100%; opacity: var(--rain); z-index: -4; pointer-events: none; }
  .grid-bg { position: fixed; inset: 0; z-index: -3; pointer-events: none; background-image: linear-gradient(var(--grid) 1px, transparent 1px), linear-gradient(90deg, var(--grid) 1px, transparent 1px); background-size: 44px 44px; mask-image: radial-gradient(ellipse 80% 70% at 50% 30%, #000 30%, transparent 80%); }
  .glow { position: fixed; z-index: -2; pointer-events: none; width: 46rem; height: 46rem; border-radius: 9999px; filter: blur(110px); opacity: var(--glow); }
  .g1 { top: -16rem; left: -12rem; background: #22c55e; }
  .g2 { bottom: -18rem; right: -14rem; background: #7c3aed; }
  .scan { position: fixed; inset: 0; z-index: -1; pointer-events: none; background: repeating-linear-gradient(0deg, rgb(0 0 0 / 0.16) 0 1px, transparent 1px 3px); opacity: var(--scan); }
  /* Stroke putih tipis (4 bayangan tanpa blur) + cahaya hijau, supaya logo biru tetap terbaca di latar gelap. */
  .logo { filter: drop-shadow(1.5px 0 0 #fff) drop-shadow(-1.5px 0 0 #fff) drop-shadow(0 1.5px 0 #fff) drop-shadow(0 -1.5px 0 #fff) drop-shadow(0 0 16px rgb(34 197 94 / 0.35)); }
  .foot :global(.text-tertiary) { color: var(--muted); }
  .foot :global(a:hover), .foot :global(button:hover) { color: var(--acc); }

  .ctrl { position: relative; z-index: 30; display: flex; justify-content: flex-end; align-items: center; gap: 0.4rem; color: var(--muted); }
  .ctrl :global(button:hover) { color: var(--acc); }
  .ctrl-btn { display: grid; place-items: center; width: 2rem; height: 2rem; border-radius: 9999px; font-size: 15px; color: var(--muted); }

  .term { overflow: hidden; border-radius: 0.9rem; border: 1px solid var(--line); background: var(--win); box-shadow: var(--shadow); font-family: ui-monospace, 'JetBrains Mono', 'Cascadia Code', Menlo, Consolas, monospace; backdrop-filter: blur(6px); }
  .term-bar { display: flex; align-items: center; gap: 0.4rem; padding: 0.55rem 0.9rem; background: var(--bar); border-bottom: 1px solid var(--line); }
  .term-bar .d { width: 0.7rem; height: 0.7rem; border-radius: 9999px; }
  .term-title { margin-inline-start: 0.6rem; font-size: 12px; font-weight: 600; color: var(--muted); }

  .searchbar { display: flex; align-items: center; gap: 0.6rem; padding: 0.5rem 0.9rem; background: var(--bar2); border-bottom: 1px solid var(--line); }
  .sb-prompt { flex: none; font-size: 12px; color: var(--muted); }
  input#legal-q.sb-input { flex: 1; min-width: 0; background-color: transparent !important; border: 0 !important; outline: 0 !important; box-shadow: none !important; color: var(--acc) !important; font: inherit; font-size: 13px; caret-color: var(--acc); padding: 0.2rem 0; -webkit-appearance: none; appearance: none; }
  input#legal-q.sb-input::placeholder { color: var(--dim); }
  .sb-input::-webkit-search-cancel-button { display: none; }
  .searchbar:focus-within { background: var(--bar3); box-shadow: inset 0 -1px 0 var(--acc); }
  .sb-count { flex: none; font-size: 11.5px; color: var(--val); }
  .sb-count.none { color: var(--err); }
  .sb-key, .sb-clear { flex: none; padding: 0 0.4rem; border-radius: 0.3rem; border: 1px solid var(--line); background: var(--bar3); color: var(--muted); font: inherit; font-size: 11px; line-height: 1.6; }
  .sb-clear { cursor: pointer; }
  .sb-clear:hover { color: var(--head); }

  .term-wrap { display: grid; grid-template-columns: 1fr; }
  @media (min-width: 820px) { .term-wrap { grid-template-columns: 260px 1fr; } }

  .toc { display: none; border-inline-end: 1px solid var(--line); padding: 1rem 0.6rem; align-self: start; max-height: 78vh; overflow: auto; }
  @media (min-width: 820px) { .toc { display: block; } }
  .toc-h { margin: 0 0 0.6rem 0.5rem; font-size: 11.5px; color: var(--muted); }
  .toc-a { display: flex; gap: 0.55rem; align-items: baseline; padding: 0.38rem 0.5rem; border-radius: 0.4rem; border-inline-start: 2px solid transparent; font-size: 12px; line-height: 1.4; color: var(--muted); text-decoration: none; transition: background-color 0.15s, color 0.15s; }
  .toc-a:hover { background: var(--hover); color: var(--head); }
  .toc-a.on { background: var(--acc-bg); border-inline-start-color: var(--acc); color: var(--val); }
  .toc-a.dim { opacity: 0.35; }
  .toc-n { flex: none; color: var(--dim); }
  .toc-a.on .toc-n { color: var(--acc); }
  .toc-t { flex: 1; min-width: 0; }
  .toc-c { flex: none; padding: 0 0.35rem; border-radius: 9999px; background: var(--count-bg); color: var(--count); font-size: 10.5px; font-weight: 700; }

  .doc { padding: 1.4rem 1.2rem 1.8rem; font-size: 13px; line-height: 1.75; max-height: 78vh; overflow: auto; scroll-behavior: smooth; }
  @media (min-width: 820px) { .doc { padding: 1.6rem 2.2rem 2rem; } }
  .prompt { margin: 0; color: var(--muted); font-size: 12px; word-break: break-all; }
  .prompt.end { margin-top: 1.6rem; }
  .ps { color: var(--acc); font-weight: 700; }
  .hash { color: var(--dim); font-weight: 400; margin-inline-end: 0.15em; }
  .doc-title { margin: 0.6rem 0 0.3rem; font-size: 24px; font-weight: 800; color: var(--head); letter-spacing: -0.01em; }
  .meta { margin: 0; font-size: 12px; color: var(--muted); }
  .kk { color: var(--key); }
  .vv { color: var(--val); }
  .lv { display: inline-block; margin-inline-end: 0.5rem; padding: 0 0.35rem; border-radius: 0.25rem; font-weight: 700; font-size: 10.5px; background: var(--err-bg); color: var(--err); }
  .empty { margin: 1.4rem 0 0; padding: 0.6rem 0.8rem; border-radius: 0.5rem; border: 1px solid var(--err); background: var(--err-bg); color: var(--err); font-size: 12.5px; }

  .sec { margin-top: 1.9rem; scroll-margin-top: 0.8rem; font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif; }
  .sec h2 { margin: 0 0 0.45rem; font-family: ui-monospace, 'JetBrains Mono', 'Cascadia Code', Menlo, Consolas, monospace; font-size: 15px; font-weight: 700; color: var(--h2); }
  .sec p { margin: 0 0 0.7rem; font-size: 13.5px; line-height: 1.8; }
  .sec ul { margin: 0 0 0.8rem; padding: 0; list-style: none; }
  .sec li { position: relative; margin: 0.25rem 0; padding-inline-start: 1.3rem; font-size: 13.5px; line-height: 1.75; }
  .sec li::before { content: '›'; position: absolute; inset-inline-start: 0.25rem; color: var(--acc); font-weight: 700; }
  .mail { color: var(--key); text-decoration: underline; text-underline-offset: 2px; }
  :global(.sec mark) { background: var(--mark-bg); color: var(--mark-fg); border-radius: 0.2rem; padding: 0 0.1rem; }

  .tbl-wrap { margin: 0.2rem 0 0.9rem; overflow-x: auto; border: 1px solid var(--line); border-radius: 0.6rem; }
  table { width: 100%; border-collapse: collapse; font-size: 12.5px; line-height: 1.6; }
  th { text-align: start; padding: 0.5rem 0.75rem; background: var(--bar); color: var(--key); font-family: ui-monospace, 'JetBrains Mono', Consolas, monospace; font-size: 11.5px; font-weight: 700; white-space: nowrap; border-bottom: 1px solid var(--line); }
  td { white-space: normal; overflow-wrap: anywhere; padding: 0.5rem 0.75rem; vertical-align: top; border-top: 1px solid var(--line2); }
  tbody tr:first-child td { border-top: 0; }
  tbody tr:hover td { background: var(--hover); }
  td:first-child { color: var(--head); font-weight: 600; }
  @media (max-width: 600px) {
    thead { display: none; }
    tr, td { display: block; }
    tr { padding: 0.45rem 0; border-top: 1px solid var(--line); }
    tbody tr:first-child { border-top: 0; }
    td { border: 0; padding: 0.1rem 0.75rem; }
    td::before { content: attr(data-label) ' · '; color: var(--muted); font-size: 11px; }
    td:first-child::before { content: ''; }
  }

  .back { margin-top: 1.2rem; padding: 0.4rem 0.9rem; border-radius: 0.5rem; border: 1px solid var(--line); background: var(--win); color: var(--muted); font: 12.5px ui-monospace, Consolas, monospace; cursor: pointer; }
  .back:hover { color: var(--acc); border-color: var(--acc); }

  .cursor { display: inline-block; width: 0.55em; height: 1.05em; vertical-align: text-bottom; background: var(--acc); animation: blink 1.1s steps(1) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
  @media (prefers-reduced-motion: reduce) { .cursor { animation: none; } .doc { scroll-behavior: auto; } .rain { display: none; } }
</style>
