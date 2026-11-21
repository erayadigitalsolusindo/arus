<script lang="ts">
  import { onMount } from 'svelte';
  import { danger, type DangerScope } from '#lib/danger/api.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';

  const WORD = 'HAPUS'; // sama dengan dangerzone.ConfirmWord di server
  const actions: { scope: DangerScope; title: string; desc: string }[] = [
    { scope: 'sales', title: 'danger.sales', desc: 'danger.salesDesc' },
    { scope: 'purchasing', title: 'danger.purchasing', desc: 'danger.purchasingDesc' },
    { scope: 'payment-methods', title: 'danger.methods', desc: 'danger.methodsDesc' }
  ];

  let open = $state<DangerScope | null>(null);
  let word = $state('');
  let password = $state('');
  let busy = $state(false);
  let error = $state('');
  let fields = $state<Record<string, string>>({});
  let done = $state('');

  onMount(() => (document.title = t('danger.docTitle')));

  function begin(scope: DangerScope) {
    open = scope;
    word = password = error = done = '';
    fields = {};
  }

  async function run() {
    if (!open || busy || word !== WORD || !password) return;
    busy = true;
    error = done = '';
    fields = {};
    try {
      const res = await danger.clear(open, { password, confirm: word });
      const n = Object.entries(res.deleted).reduce((s, [, v]) => s + (typeof v === 'number' ? v : 0), 0);
      done = t('danger.done', { n });
      open = null;
    } catch (e) {
      if (e instanceof ApiError && e.fields) fields = Object.fromEntries(Object.entries(e.fields).map(([k, c]) => [k, fieldMessage(c) ?? c]));
      else error = errorMessage(e);
    } finally {
      busy = false;
      password = '';
    }
  }
</script>

<main class="p-4 lg:p-6 max-w-3xl mx-auto w-full min-w-0 space-y-4">
  <header>
    <h1 class="font-display font-bold text-[20px] flex items-center gap-2 text-[var(--color-danger-600)]"><i class="icon-triangle-alert text-[22px]"></i>{t('danger.title')}</h1>
    <p class="text-[13px] mt-1 text-[var(--text-secondary)]">{t('danger.lead')}</p>
    <p class="text-[12px] mt-1 text-[var(--text-tertiary)]">{t('danger.untouched')}</p>
  </header>

  {#if done}
    <div role="status" class="rounded-lg px-3 py-2.5 text-[13px] badge-success">{done}</div>
  {/if}

  {#each actions as a (a.scope)}
    <section class="surface-card p-4 space-y-3 border border-[color-mix(in_oklab,var(--color-danger-600)_35%,transparent)]">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <h2 class="font-semibold text-[14.5px]">{t(a.title as never)}</h2>
          <p class="text-[12.5px] mt-1 leading-relaxed text-[var(--text-secondary)]">{t(a.desc as never)}</p>
        </div>
        {#if open !== a.scope}
          <button type="button" class="btn btn-sm btn-danger" onclick={() => begin(a.scope)}>{t('danger.run')}</button>
        {/if}
      </div>

      {#if open === a.scope}
        <form class="space-y-3 pt-3 border-t border-[var(--border)]" onsubmit={(e) => { e.preventDefault(); void run(); }}>
          <p class="text-[13px] font-semibold">{t('danger.confirmTitle', { action: t(a.title as never) })}</p>
          <div>
            <label for="dz-word" class="text-[11.5px] mb-1 block text-[var(--text-secondary)]">{t('danger.typeWord', { word: WORD })}</label>
            <input id="dz-word" class="w-full field-control" autocomplete="off" bind:value={word} aria-invalid={!!fields.confirm} />
            {#if fields.confirm}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fields.confirm}</p>{/if}
          </div>
          <div>
            <label for="dz-pw" class="text-[11.5px] mb-1 block text-[var(--text-secondary)]">{t('danger.password')}</label>
            <input id="dz-pw" type="password" class="w-full field-control" autocomplete="current-password" bind:value={password} aria-invalid={!!fields.password} />
            {#if fields.password}<p class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{fields.password}</p>{/if}
          </div>
          {#if error}
            <div role="alert" class="rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">{error}</div>
          {/if}
          <div class="flex gap-2">
            <button type="submit" class="btn btn-sm btn-danger" disabled={busy || word !== WORD || !password}>{busy ? t('danger.working') : t('danger.go')}</button>
            <button type="button" class="btn btn-sm" disabled={busy} onclick={() => (open = null)}>{t('danger.cancel')}</button>
          </div>
        </form>
      {/if}
    </section>
  {/each}
</main>
