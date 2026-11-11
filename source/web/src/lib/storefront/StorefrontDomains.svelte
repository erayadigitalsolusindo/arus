<script lang="ts">
  import { onMount } from 'svelte';
  import { storefront, type DomainInfo } from '#lib/storefront/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let domains = $state<DomainInfo[]>([]);
  let target = $state('');
  let parents = $state<string[]>([]);
  let loading = $state(true);
  let busy = $state('');
  let error = $state('');
  let base = $state('');
  let label = $state('');

  const canEdit = $derived(can('storefront', 'update'));
  const roots = $derived(domains.filter((d) => !d.tenant_made && d.status === 'active'));
  const modes = ['app', 'both', 'landing'] as const;
  const modeOf = (d: DomainInfo) => (d.app && d.landing ? 'both' : d.landing ? 'landing' : 'app');

  async function load() {
    try {
      const r = await storefront.domains();
      domains = r.data;
      target = r.target;
      parents = r.parents ?? [];
      if (!roots.some((d) => d.host === base)) base = roots[0]?.host ?? '';
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(load);

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key;
    error = '';
    try {
      await fn();
      await load();
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = '';
    }
  }
  const setMode = (d: DomainInfo, m: (typeof modes)[number]) => run(d.id, () => storefront.setDomainMode(d.id, m));
  const add = () =>
    run('add', async () => {
      await storefront.addSubdomain(base, label.trim().toLowerCase());
      label = '';
    });
  const addParent = (host: string) => run('parent', () => storefront.addParent(host));
  const remove = (d: DomainInfo) => confirm(t('storefront.domains.confirmDelete', { host: d.host })) && run(d.id, () => storefront.deleteSubdomain(d.id));
</script>

{#if !loading && (domains.length > 0 || error)}
  <section class="surface-card p-3 lg:p-4 space-y-3">
    <div>
      <h2 class="text-[13.5px] font-semibold">{t('storefront.domains.title')}</h2>
      <p class="text-[12px] text-[var(--text-tertiary)]">{t('storefront.domains.help')}</p>
    </div>

    {#if error}
      <div class="rounded-xl border border-[var(--color-danger-500)]/30 bg-[var(--color-danger-500)]/10 px-3 py-2 text-[12.5px]" role="alert">{error}</div>
    {/if}

    <ul class="divide-y divide-[var(--border-subtle)] rounded-xl border border-[var(--border-subtle)]">
      {#each domains as d (d.id)}
        <li class="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
          <div class="min-w-0">
            <p class="font-mono text-[12.5px] break-all">{d.host}</p>
            {#if d.status !== 'active'}
              <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('storefront.domains.notActive')}</p>
            {:else if d.tenant_made && target}
              <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('storefront.domains.dnsHint', { target })}</p>
            {/if}
          </div>
          <div class="flex items-center gap-2">
            {#if d.status === 'active'}
              <div class="inline-flex overflow-hidden rounded-lg border border-[var(--border-default)]" role="group" aria-label={d.host}>
                {#each modes as m}
                  <button
                    type="button"
                    class="px-2.5 py-1 text-[12px] disabled:opacity-60 {modeOf(d) === m ? 'bg-[var(--color-primary-600)] text-white' : 'bg-[var(--surface-card)]'}"
                    disabled={!canEdit || busy === d.id}
                    aria-pressed={modeOf(d) === m}
                    onclick={() => modeOf(d) !== m && setMode(d, m)}
                  >{t(`storefront.domains.mode.${m}`)}</button>
                {/each}
              </div>
              {#if d.landing}
                <a class="btn btn-ghost !px-2 !py-1 !text-[12px]" href="https://{d.host}/" target="_blank" rel="noopener noreferrer"><i class="icon-external-link text-[12px]"></i>{t('storefront.openTab')}</a>
              {/if}
            {/if}
            {#if d.tenant_made && canEdit}
              <button type="button" class="btn btn-ghost !px-2 !py-1 !text-[12px]" disabled={busy === d.id} aria-label={t('storefront.domains.delete')} onclick={() => remove(d)}><i class="icon-trash-2 text-[13px]"></i></button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>

    {#if target}
      <details class="rounded-xl border border-[var(--border-subtle)] px-3 py-2 text-[12.5px]">
        <summary class="cursor-pointer font-semibold">{t('storefront.domains.dnsTitle')}</summary>
        <ul class="mt-2 list-disc space-y-1 ps-5 text-[var(--text-secondary)]">
          <li>{t('storefront.domains.dnsSub')} <code class="font-mono">CNAME &nbsp;<em>{t('storefront.domains.dnsName')}</em> → {target}</code></li>
          <li>{t('storefront.domains.dnsWildcard')} <code class="font-mono">CNAME &nbsp;* → {target}</code></li>
          <li>{t('storefront.domains.dnsApex', { target })}</li>
          <li>{t('storefront.domains.dnsCloudflare')}</li>
          <li>{t('storefront.domains.dnsWait')}</li>
        </ul>
      </details>
    {/if}

    {#if canEdit && parents.length > 0}
      <div class="flex flex-wrap items-center gap-2 rounded-xl bg-[var(--surface-sunken)] px-3 py-2 text-[12.5px]">
        <span class="text-[var(--text-secondary)]">{t('storefront.domains.parentHelp')}</span>
        {#each parents as p (p)}
          <button type="button" class="btn btn-outline !px-2 !py-1 !text-[12px] font-mono" disabled={busy === 'parent'} onclick={() => addParent(p)}>+ {p}</button>
        {/each}
      </div>
    {/if}

    {#if canEdit && roots.length > 0}
      <form class="flex flex-wrap items-end gap-2" onsubmit={(e) => { e.preventDefault(); add(); }}>
        <label class="field min-w-[8rem] flex-1">
          <span class="text-[12px] text-[var(--text-secondary)]">{t('storefront.domains.addLabel')}</span>
          <div class="flex items-center gap-1">
            <input class="field-input font-mono" bind:value={label} maxlength="63" placeholder="landing" autocomplete="off" spellcheck="false" required />
            <span class="font-mono text-[12.5px]">.</span>
            <select class="field-input font-mono" bind:value={base} aria-label={t('storefront.domains.baseLabel')}>
              {#each roots as r (r.id)}<option value={r.host}>{r.host}</option>{/each}
            </select>
          </div>
        </label>
        <button type="submit" class="btn btn-outline !text-[12.5px]" disabled={busy === 'add' || !label.trim()}>{t('storefront.domains.add')}</button>
      </form>
    {/if}
  </section>
{/if}
