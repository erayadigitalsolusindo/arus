<script lang="ts">
  // Menampilkan markdown sebagai HTML aman. renderMarkdown mematikan HTML mentah, gambar, dan URL berbahaya
  // (lihat #lib/markdown.ts), jadi {@html} di sini dikecualikan dari aturan "jangan {@html} untuk data pengguna".
  import { renderMarkdown } from '#lib/markdown.ts';

  let { source, class: cls = '' }: { source: string; class?: string } = $props();
</script>

<div class="md {cls}">{@html renderMarkdown(source)}</div>

<style>
  .md { overflow-wrap: anywhere; }
  .md > :global(:first-child) { margin-top: 0; }
  .md > :global(:last-child) { margin-bottom: 0; }
  .md :global(h1) { font-size: 1.25rem; font-weight: 700; margin: 0.5rem 0; }
  .md :global(h2) { font-size: 1.1rem; font-weight: 700; margin: 0.5rem 0; }
  .md :global(h3) { font-size: 1rem; font-weight: 600; margin: 0.5rem 0; }
  .md :global(p) { margin: 0.4rem 0; }
  /* Stylesheet template memaksa `list-style: none !important` di layer dasar (menang atas gaya biasa), jadi penanda daftar digambar sendiri. */
  .md :global(ul), .md :global(ol) { margin: 0.4rem 0; margin-inline-start: 1.4rem; }
  .md :global(ul > li), .md :global(ol > li) { position: relative; }
  .md :global(ul > li)::before { content: "•"; position: absolute; inset-inline-start: -1rem; }
  .md :global(ol) { counter-reset: md-item; }
  .md :global(ol > li) { counter-increment: md-item; }
  .md :global(ol > li)::before { content: counter(md-item) "."; position: absolute; inset-inline-start: -1.4rem; font-variant-numeric: tabular-nums; }
  .md :global(a) { color: var(--color-primary-600); text-decoration: underline; }
  .md :global(code) { font-family: ui-monospace, monospace; background: var(--surface-sunken); padding: 0.05rem 0.3rem; border-radius: 4px; }
  .md :global(pre) { background: var(--surface-sunken); padding: 0.6rem; border-radius: 6px; overflow-x: auto; }
  .md :global(blockquote) { border-inline-start: 3px solid var(--border-subtle); padding-inline-start: 0.75rem; color: var(--text-tertiary); }
</style>
