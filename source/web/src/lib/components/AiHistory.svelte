<script lang="ts">
  import { onMount } from 'svelte';
  import { ai, type ConversationSummary } from '#lib/ai/api.ts';
  import { formatDateTime, t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  // Daftar riwayat percakapan milik pengguna (terenkripsi di server, hanya pemilik yang bisa membaca). Keyset: "muat lebih banyak".
  let {
    activeId = '',
    refreshKey = 0,
    onopen,
    onnew,
    ondeleted,
    onerror,
    onnotice
  }: {
    activeId?: string;
    refreshKey?: number;
    onopen: (id: string) => void;
    onnew: () => void;
    ondeleted: (id: string | 'all') => void;
    onerror: (msg: string) => void;
    onnotice: (msg: string) => void;
  } = $props();

  let items = $state<ConversationSummary[] | undefined>();
  let next = $state('');
  let loadingMore = $state(false);
  let editing = $state('');
  let draft = $state('');

  async function load() {
    try {
      const p = await ai.conversations();
      items = p.items;
      next = p.next ?? '';
    } catch (e) {
      items = items ?? [];
      onerror(errorMessage(e));
    }
  }

  async function more() {
    if (!next || loadingMore) return;
    loadingMore = true;
    try {
      const p = await ai.conversations(next);
      items = [...(items ?? []), ...p.items];
      next = p.next ?? '';
    } catch (e) {
      onerror(errorMessage(e));
    } finally {
      loadingMore = false;
    }
  }

  onMount(() => void load());
  $effect(() => {
    void refreshKey;
    if (refreshKey > 0) void load();
  });

  function startRename(c: ConversationSummary) {
    editing = c.id;
    draft = c.title;
  }

  async function commitRename(c: ConversationSummary) {
    const title = draft.trim();
    editing = '';
    if (!title || title === c.title) return;
    try {
      await ai.renameConversation(c.id, title);
      c.title = title;
    } catch (e) {
      onerror(errorMessage(e));
    }
  }

  async function remove(c: ConversationSummary) {
    if (!confirm(t('ai.history.deleteConfirm'))) return;
    try {
      await ai.deleteConversation(c.id);
      items = (items ?? []).filter((x) => x.id !== c.id);
      ondeleted(c.id);
    } catch (e) {
      onerror(errorMessage(e));
    }
  }

  async function removeAll() {
    if (!confirm(t('ai.history.deleteAllConfirm'))) return;
    try {
      const r = await ai.deleteAllConversations();
      items = [];
      next = '';
      onnotice(t('ai.history.deleted', { count: r.deleted }));
      ondeleted('all');
    } catch (e) {
      onerror(errorMessage(e));
    }
  }
</script>

<div class="hist">
  <div class="top">
    <h2><i class="icon-history"></i> {t('ai.history.title')}</h2>
    <button type="button" class="btn btn-sm" onclick={onnew}><i class="icon-square-pen"></i> {t('ai.history.new')}</button>
  </div>

  {#if items === undefined}
    <div class="skel" aria-busy="true"><span></span><span></span><span></span></div>
  {:else if items.length === 0}
    <p class="empty">{t('ai.history.empty')}</p>
  {:else}
    <ul class="list">
      {#each items as c (c.id)}
        <li class:active={c.id === activeId}>
          {#if editing === c.id}
            <input class="rename" bind:value={draft} maxlength="60" aria-label={t('ai.history.rename')}
              onblur={() => commitRename(c)} onkeydown={(e) => { if (e.key === 'Enter') e.currentTarget.blur(); else if (e.key === 'Escape') { editing = ''; } }} />
          {:else}
            <button type="button" class="open" onclick={() => onopen(c.id)} title={c.title}>
              <span class="ttl">{c.title}</span>
              <span class="meta">{formatDateTime(c.updated_at)}</span>
            </button>
            <span class="acts">
              <button type="button" class="ic" title={t('ai.history.rename')} aria-label={t('ai.history.rename')} onclick={() => startRename(c)}><i class="icon-pencil"></i></button>
              <button type="button" class="ic" title={t('ai.history.delete')} aria-label={t('ai.history.delete')} onclick={() => remove(c)}><i class="icon-trash-2"></i></button>
            </span>
          {/if}
        </li>
      {/each}
    </ul>
    {#if next}
      <button type="button" class="btn btn-sm more" onclick={more} disabled={loadingMore}>{t('ai.history.more')}</button>
    {/if}
  {/if}

  <div class="foot">
    <p>{t('ai.history.retention')}</p>
    {#if items && items.length > 0}
      <button type="button" class="link" onclick={removeAll}><i class="icon-trash-2"></i> {t('ai.history.deleteAll')}</button>
    {/if}
  </div>
</div>

<style>
  .hist {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-height: 0;
    height: 100%;
  }
  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  h2 {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 13px;
    font-weight: 700;
    color: var(--text-secondary);
    letter-spacing: 0.02em;
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
    min-height: 0;
    flex: 1;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    position: relative;
    display: flex;
    align-items: center;
    border-radius: 0.6rem;
    border: 1px solid transparent;
    transition: background 0.15s, border-color 0.15s;
  }
  li:hover {
    background: var(--surface-sunken);
  }
  li.active {
    background: color-mix(in oklab, var(--ha-a) 12%, var(--surface-base));
    border-color: color-mix(in oklab, var(--ha-a) 40%, transparent);
  }
  .open {
    flex: 1;
    min-width: 0;
    text-align: start;
    padding: 0.5rem 0.6rem;
    background: transparent;
    border: 0;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .ttl {
    font-size: 12.5px;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .meta {
    font-size: 10.5px;
    color: var(--text-tertiary);
  }
  .acts {
    display: none;
    gap: 2px;
    padding-inline-end: 0.3rem;
  }
  li:hover .acts,
  li:focus-within .acts {
    display: flex;
  }
  .ic {
    background: transparent;
    border: 0;
    cursor: pointer;
    color: var(--text-tertiary);
    padding: 0.25rem;
    border-radius: 0.4rem;
  }
  .ic:hover {
    color: var(--text-primary);
    background: var(--surface-base);
  }
  .rename {
    width: 100%;
    margin: 0.25rem;
    height: 2rem;
    padding: 0 0.5rem;
    font-size: 12.5px;
    border-radius: 0.4rem;
    border: 1px solid var(--border-default);
    background: var(--surface-base);
  }
  .empty {
    font-size: 12.5px;
    color: var(--text-tertiary);
    padding: 0.5rem 0.2rem;
  }
  .more {
    align-self: center;
  }
  .foot {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    font-size: 11px;
    color: var(--text-tertiary);
  }
  .link {
    align-self: flex-start;
    background: transparent;
    border: 0;
    padding: 0;
    cursor: pointer;
    font-size: 11.5px;
    color: var(--text-secondary);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .skel {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .skel span {
    height: 2.4rem;
    border-radius: 0.6rem;
    background: linear-gradient(90deg, var(--surface-sunken), color-mix(in oklab, var(--ha-a) 10%, var(--surface-sunken)), var(--surface-sunken));
    background-size: 200% 100%;
    animation: hist-shimmer 1.4s linear infinite;
  }
  @keyframes hist-shimmer {
    to {
      background-position: -200% 0;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .skel span {
      animation: none;
    }
  }
</style>
