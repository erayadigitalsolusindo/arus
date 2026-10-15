<script lang="ts">
  import { t } from '#lib/i18n/index.ts';
  import { highlight } from './highlight.ts';

  // Blok kode gaya terminal. `tabs` opsional: beberapa bahasa dalam satu blok (nilai `code` per tab).
  type Tab = { id: string; label: string; code: string; lang: string };
  let { code = '', lang = 'text', title = '', tabs = [], tab = $bindable('') }: { code?: string; lang?: string; title?: string; tabs?: Tab[]; tab?: string } = $props();

  const active = $derived(tabs.length ? (tabs.find((x) => x.id === tab) ?? tabs[0]) : null);
  const shown = $derived(active ? active.code : code);
  const shownLang = $derived(active ? active.lang : lang);
  const html = $derived(highlight(shown, shownLang));

  let copied = $state(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(shown);
      copied = true;
      setTimeout(() => (copied = false), 1400);
    } catch {
      /* clipboard diblokir: pengguna menyalin manual */
    }
  }
</script>

<div class="dv-code">
  <div class="dv-code-head">
    <span class="dv-dots" aria-hidden="true"><i></i><i></i><i></i></span>
    {#if tabs.length}
      <div class="dv-tabs" role="tablist">
        {#each tabs as x (x.id)}
          <button type="button" class="dv-tab" role="tab" aria-selected={x.id === active?.id} onclick={() => (tab = x.id)}>{x.label}</button>
        {/each}
      </div>
    {:else if title}
      <span class="mono">{title}</span>
    {/if}
    <button type="button" class="dv-copy" onclick={copy}>
      <i class="icon-{copied ? 'check' : 'copy'}" style="font-size:12px"></i>{copied ? t('extapi.portal.copied') : t('extapi.portal.copy')}
    </button>
  </div>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- highlight() meng-escape semua teks -->
  <pre><code>{@html html}</code></pre>
</div>
