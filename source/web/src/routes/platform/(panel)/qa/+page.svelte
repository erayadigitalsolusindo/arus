<script lang="ts">
  import { platformApi, type QAApp, type QARun, type QAResult, type QAStatus } from '#lib/platform/api.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import StatCard from '#lib/platform/StatCard.svelte';

  type Filter = 'all' | 'todo' | 'fail';

  let apps = $state<QAApp[]>([]);
  let runs = $state<QARun[]>([]);
  let runId = $state('');
  let results = $state<Record<string, QAResult>>({});
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let activeApp = $state('');
  let filter = $state<Filter>('all');
  let query = $state('');
  let newName = $state('');
  let showNew = $state(false);
  let creating = $state(false);
  let openNote = $state('');
  let draft = $state('');
  let busyKey = $state('');
  let collapsed = $state<Record<string, boolean>>({});
  let copied = $state(false);

  const run = $derived(runs.find((r) => r.id === runId));
  const readonly = $derived(!!run?.closed_at);
  const currentApp = $derived(apps.find((a) => a.key === activeApp) ?? apps[0]);

  const allItems = $derived(apps.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ ...i, app: a.title, appKey: a.key, group: g.title })))));
  const total = $derived(allItems.length);
  const counts = $derived.by(() => {
    const c = { pass: 0, fail: 0, skip: 0 };
    for (const i of allItems) {
      const s = results[i.key]?.status;
      if (s) c[s]++;
    }
    return { ...c, todo: total - c.pass - c.fail - c.skip };
  });
  const percent = $derived(total ? Math.round(((counts.pass + counts.skip) / total) * 100) : 0);
  const failed = $derived(allItems.filter((i) => results[i.key]?.status === 'fail'));

  // Kesimpulan satu kalimat: gagal > belum diuji > siap.
  const verdict = $derived(
    counts.fail > 0
      ? { tone: 'danger', icon: 'triangle-alert', text: t('platform.qa.verdictFail', { n: counts.fail }) }
      : counts.todo > 0
        ? { tone: 'warning', icon: 'clock', text: t('platform.qa.verdictTodo', { n: counts.todo }) }
        : { tone: 'success', icon: 'rocket', text: t('platform.qa.verdictReady') }
  );

  // Cincin kesiapan: tiga busur berurutan (lulus, lewati, gagal) dalam satu lingkaran.
  const R = 52;
  const C = 2 * Math.PI * R;
  const arcs = $derived.by(() => {
    let cum = 0;
    return (
      [
        ['pass', counts.pass, '#34d399'],
        ['skip', counts.skip, '#94a3b8'],
        ['fail', counts.fail, '#fb7185']
      ] as const
    ).map(([key, n, color]) => {
      const len = total ? (n / total) * C : 0;
      const arc = { key, color, len, offset: -cum };
      cum += len;
      return arc;
    });
  });

  const legend = $derived([
    { key: 'pass', n: counts.pass, color: '#34d399', label: t('platform.qa.statPass') },
    { key: 'skip', n: counts.skip, color: '#94a3b8', label: t('platform.qa.statSkip') },
    { key: 'fail', n: counts.fail, color: '#fb7185', label: t('platform.qa.statFail') },
    { key: 'todo', n: counts.todo, color: 'rgb(255 255 255 / 0.25)', label: t('platform.qa.statTodo') }
  ]);

  function appStats(a: QAApp) {
    const keys = a.groups.flatMap((g) => g.items.map((i) => i.key));
    let pass = 0,
      fail = 0,
      skip = 0;
    for (const k of keys) {
      const s = results[k]?.status;
      if (s === 'pass') pass++;
      else if (s === 'fail') fail++;
      else if (s === 'skip') skip++;
    }
    return { total: keys.length, pass, fail, skip, pct: keys.length ? Math.round(((pass + skip) / keys.length) * 100) : 0 };
  }

  const matches = (key: string, title: string) => {
    const s = results[key]?.status;
    if (filter === 'todo' && s) return false;
    if (filter === 'fail' && s !== 'fail') return false;
    const q = query.trim().toLowerCase();
    return !q || title.toLowerCase().includes(q) || key.toLowerCase().includes(q);
  };

  async function loadRuns(select?: string) {
    const res = await platformApi.qaRuns();
    runs = res.items;
    if (select) runId = select;
    else if (!runs.some((r) => r.id === runId)) runId = (runs.find((r) => !r.closed_at) ?? runs[0])?.id ?? '';
  }

  async function loadResults() {
    if (!runId) {
      results = {};
      return;
    }
    const res = await platformApi.qaResults(runId);
    results = Object.fromEntries(res.items.map((r) => [r.key, r]));
  }

  async function init() {
    loading = true;
    error = '';
    try {
      const [cat] = await Promise.all([platformApi.qaCatalog(), loadRuns()]);
      apps = cat.apps;
      activeApp = apps[0]?.key ?? '';
      await loadResults();
      showNew = runs.length === 0;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  init();

  async function pick(id: string) {
    runId = id;
    openNote = '';
    try {
      await loadResults();
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function createRun() {
    const name = newName.trim();
    if (!name) return;
    creating = true;
    error = '';
    try {
      const { id } = await platformApi.createQARun(name);
      newName = '';
      showNew = false;
      await loadRuns(id);
      results = {};
      notice = t('platform.qa.created');
    } catch (e) {
      error = errorMessage(e);
    } finally {
      creating = false;
    }
  }

  async function toggleClosed() {
    if (!run) return;
    error = '';
    try {
      await platformApi.closeQARun(run.id, !run.closed_at);
      await loadRuns(run.id);
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function removeRun() {
    if (!run || !confirm(t('platform.qa.deleteConfirm'))) return;
    error = '';
    try {
      await platformApi.deleteQARun(run.id);
      runId = '';
      await loadRuns();
      await loadResults();
      showNew = runs.length === 0;
      notice = t('platform.qa.deleted');
    } catch (e) {
      error = errorMessage(e);
    }
  }

  /** Simpan hasil satu item, lalu muat ulang hasil dari server agar nama pengisi dan waktu akurat. */
  async function mark(key: string, status: QAStatus | 'todo', note?: string) {
    if (!runId || readonly) return;
    const nextNote = note ?? results[key]?.note ?? '';
    busyKey = key;
    error = '';
    try {
      await platformApi.setQAResult(runId, { key, status, note: nextNote });
      await Promise.all([loadRuns(runId), loadResults()]);
    } catch (e) {
      error = errorMessage(e);
      await loadResults().catch(() => {});
    } finally {
      busyKey = '';
    }
  }

  function toggleNote(key: string) {
    if (openNote === key) {
      openNote = '';
      return;
    }
    openNote = key;
    draft = results[key]?.note ?? '';
  }

  async function saveNote(key: string) {
    const cur = results[key];
    if (!cur) return;
    await mark(key, cur.status, draft);
    openNote = '';
  }

  function jumpTo(appKey: string) {
    activeApp = appKey;
    filter = 'fail';
    query = '';
  }

  /** Laporan teks ringkas untuk ditempel ke tiket/chat. */
  async function copyReport() {
    if (!run) return;
    const lines = [
      `QA — ${run.name}`,
      `${t('platform.qa.statPass')} ${counts.pass} · ${t('platform.qa.statFail')} ${counts.fail} · ${t('platform.qa.statSkip')} ${counts.skip} · ${t('platform.qa.statTodo')} ${counts.todo} (${percent}%)`
    ];
    if (failed.length) {
      lines.push('', `${t('platform.qa.findings')}:`);
      for (const i of failed) lines.push(`- [${i.app}] ${i.title}${results[i.key]?.note ? ` — ${results[i.key].note}` : ''}`);
    }
    try {
      await navigator.clipboard.writeText(lines.join('\n'));
      copied = true;
      setTimeout(() => (copied = false), 1800);
    } catch {
      /* clipboard diblokir: abaikan */
    }
  }

  const statuses: QAStatus[] = ['pass', 'fail', 'skip'];
  const statusIcon: Record<QAStatus, string> = { pass: 'check', fail: 'x', skip: 'minus' };
  const statusTone: Record<QAStatus, string> = { pass: 'success', fail: 'danger', skip: 'info' };
  const dateOpts = { dateStyle: 'medium', timeStyle: 'short' } as const;
</script>

<svelte:head><title>{t('platform.qa.title')} | ACIRABA</title></svelte:head>

<div class="space-y-5">
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}
  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}

  <!-- ============ Hero ============ -->
  <section class="qa-hero relative isolate overflow-hidden rounded-2xl border border-[var(--border-default)] bg-gradient-to-br from-[#06122b] via-[#0a1f4a] to-[#0d2b63] text-white shadow-[var(--shadow-sm)]">
    <div class="pointer-events-none absolute -right-24 -top-24 size-72 rounded-full bg-sky-400/10 blur-3xl"></div>
    <div class="flex flex-wrap items-center justify-between gap-6 p-5 lg:p-7">
      <div class="min-w-0 flex-1 basis-[22rem]">
        <p class="flex items-center gap-2 text-[10.5px] font-bold uppercase tracking-[0.14em] text-sky-200"><i class="icon-clipboard-check text-[14px]"></i>{t('platform.qa.title')}</p>

        {#if loading}
          <div class="mt-3 h-7 w-64 animate-pulse rounded bg-white/10"></div>
          <div class="mt-2 h-4 w-96 max-w-full animate-pulse rounded bg-white/10"></div>
        {:else if run}
          <div class="mt-2 flex flex-wrap items-center gap-2.5">
            <h1 class="font-display font-bold text-[24px] lg:text-[28px] leading-tight tracking-tight truncate max-w-full">{run.name}</h1>
            <span class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[10.5px] font-bold uppercase tracking-wider {readonly ? 'border-white/20 bg-white/10 text-slate-200' : 'border-emerald-300/30 bg-emerald-400/15 text-emerald-200'}">
              <i class="icon-{readonly ? 'lock' : 'play'} text-[10px]"></i>{readonly ? t('platform.qa.runClosed') : t('platform.qa.runActive')}
            </span>
          </div>
          <p class="mt-1.5 text-[12px] text-sky-100/75">
            {t('platform.qa.createdBy', { name: run.created_by, at: formatDateTime(run.created_at, dateOpts) })}
            {#if run.closed_at} · {t('platform.qa.closedAt', { at: formatDateTime(run.closed_at, dateOpts) })}{/if}
          </p>
          <div class="mt-4 inline-flex items-center gap-2 rounded-xl border border-white/15 bg-white/10 px-3.5 py-2 text-[13px] font-semibold">
            <i class="icon-{verdict.icon} text-[16px] {verdict.tone === 'danger' ? 'text-rose-300' : verdict.tone === 'warning' ? 'text-amber-300' : 'text-emerald-300'}"></i>
            {verdict.text}
          </div>
        {:else}
          <h1 class="mt-2 font-display font-bold text-[24px] lg:text-[28px] leading-tight tracking-tight">{t('platform.qa.title')}</h1>
          <p class="mt-1.5 text-[12.5px] text-sky-100/80 max-w-xl">{t('platform.qa.subtitle')}</p>
        {/if}

        <div class="mt-4 flex flex-wrap items-center gap-2">
          {#if runs.length}
            <label class="sr-only" for="qa-run">{t('platform.qa.run')}</label>
            <select id="qa-run" class="rounded-lg border border-white/25 px-2.5 py-1.5 text-[12.5px] font-medium w-60 max-w-full [color-scheme:dark]" style="background-color:#0a1f4a !important;color:#fff;border-radius:8px !important" value={runId} onchange={(e) => pick(e.currentTarget.value)}>
              {#each runs as r (r.id)}
                <option class="bg-[#0a1f4a] text-white" value={r.id}>{r.name}{r.closed_at ? ` · ${t('platform.qa.closedTag')}` : ''}</option>
              {/each}
            </select>
          {/if}
          <button type="button" class="qa-btn qa-btn-solid" onclick={() => (showNew = !showNew)}>
            <i class="icon-plus text-[13px]"></i>{t('platform.qa.newRun')}
          </button>
          {#if run}
            <button type="button" class="qa-btn qa-btn-ghost" onclick={copyReport}>
              <i class="icon-{copied ? 'check' : 'copy'} text-[13px]"></i>{copied ? t('platform.qa.copied') : t('platform.qa.copyReport')}
            </button>
            <button type="button" class="qa-btn qa-btn-ghost" onclick={toggleClosed}>
              <i class="icon-{readonly ? 'rotate-ccw' : 'lock'} text-[13px]"></i>{readonly ? t('platform.qa.reopen') : t('platform.qa.closeRun')}
            </button>
            <button type="button" class="qa-btn qa-btn-danger" onclick={removeRun}>
              <i class="icon-trash-2 text-[13px]"></i>{t('platform.qa.delete')}
            </button>
          {/if}
        </div>
      </div>

      {#if run}
        <!-- Cincin kesiapan -->
        <div class="flex items-center gap-5 shrink-0">
          <div class="relative size-36">
            <svg viewBox="0 0 128 128" class="size-full -rotate-90" role="img" aria-label="{t('platform.qa.readiness')} {percent}%">
              <circle cx="64" cy="64" r={R} fill="none" stroke="rgb(255 255 255 / 0.12)" stroke-width="12" />
              {#each arcs as a (a.key)}
                {#if a.len > 0}
                  <circle cx="64" cy="64" r={R} fill="none" stroke={a.color} stroke-width="12" stroke-linecap="butt" stroke-dasharray="{a.len} {C - a.len}" stroke-dashoffset={a.offset} />
                {/if}
              {/each}
            </svg>
            <div class="absolute inset-0 grid place-content-center text-center">
              <p class="font-display font-bold text-[32px] leading-none tabular-nums [text-shadow:0_0_18px_rgb(125_211_252/0.45)]">{percent}<span class="text-[16px] text-sky-200">%</span></p>
              <p class="mt-1 text-[10px] font-bold uppercase tracking-widest text-sky-200/80">{t('platform.qa.readiness')}</p>
            </div>
          </div>
          <dl class="space-y-1.5 text-[12px]">
            {#each legend as l (l.key)}
              <div class="flex items-center gap-2">
                <span class="size-2.5 rounded-sm" style="background:{l.color}"></span>
                <dt class="text-sky-100/80 w-24">{l.label}</dt>
                <dd class="font-mono font-semibold tabular-nums">{formatNumber(l.n)}</dd>
              </div>
            {/each}
          </dl>
        </div>
      {/if}
    </div>
  </section>

  <!-- ============ Form run baru ============ -->
  {#if showNew}
    <form
      class="surface-card p-4"
      onsubmit={(e) => {
        e.preventDefault();
        void createRun();
      }}
    >
      <label class="text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]" for="qa-new">{t('platform.qa.runName')}</label>
      <div class="mt-1.5 flex flex-wrap items-center gap-3">
        <input id="qa-new" class="field-control qa-field flex-1 basis-72" maxlength="120" placeholder={t('platform.qa.runNamePh')} bind:value={newName} />
        <div class="flex items-center gap-2">
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={creating || !newName.trim()}>
            <i class="icon-plus text-[13px]"></i>{creating ? t('platform.qa.creating') : t('platform.qa.create')}
          </button>
          {#if runs.length}
            <button type="button" class="btn btn-secondary !text-[12.5px]" onclick={() => (showNew = false)}>{t('platform.qa.cancel')}</button>
          {/if}
        </div>
      </div>
      <p class="mt-2 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.qa.runNameHelp', { n: total })}</p>
    </form>
  {/if}

  {#if loading}
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
      {#each [0, 1, 2, 3] as i (i)}<div class="surface-card h-[76px] animate-pulse"></div>{/each}
    </div>
  {:else if !run}
    {#if !showNew}
      <div class="rounded-2xl border border-dashed border-[var(--border-default)] p-12 text-center">
        <i class="icon-list-checks text-[30px] text-[var(--text-tertiary)]"></i>
        <p class="mt-2 text-[13px] text-[var(--text-tertiary)]">{t('platform.qa.noRuns')}</p>
      </div>
    {/if}
  {:else}
    <!-- ============ Ringkasan ============ -->
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-3">
      <StatCard label={t('platform.qa.statPass')} value={formatNumber(counts.pass)} icon="circle-check" tone="success" hint={t('platform.qa.ofTotal', { n: total })} />
      <StatCard label={t('platform.qa.statFail')} value={formatNumber(counts.fail)} icon="circle-x" tone="danger" hint={counts.fail ? t('platform.qa.needsFix') : t('platform.qa.noneFailed')} />
      <StatCard label={t('platform.qa.statSkip')} value={formatNumber(counts.skip)} icon="minus" tone="info" hint={t('platform.qa.ofTotal', { n: total })} />
      <StatCard label={t('platform.qa.statTodo')} value={formatNumber(counts.todo)} icon="clock" tone="warning" hint={t('platform.qa.tested', { done: total - counts.todo, total })} />
    </div>

    <div class="grid gap-5 lg:grid-cols-[17rem_minmax(0,1fr)] items-start">
      <!-- ============ Sidebar: aplikasi + temuan ============ -->
      <aside class="space-y-4 lg:sticky lg:top-4">
        <div class="surface-card p-2">
          <p class="px-2 pt-1 pb-2 text-[10.5px] font-bold uppercase tracking-[0.12em] text-[var(--text-tertiary)]">{t('platform.qa.apps')}</p>
          <div class="flex gap-1.5 overflow-x-auto lg:flex-col lg:overflow-visible" role="tablist" aria-orientation="vertical">
            {#each apps as a (a.key)}
              {@const st = appStats(a)}
              {@const active = currentApp?.key === a.key}
              <button
                type="button"
                role="tab"
                aria-selected={active}
                class="group relative shrink-0 min-w-44 lg:min-w-0 rounded-xl px-3 py-2.5 text-left transition-colors {active ? 'bg-[var(--color-primary-50)]' : 'hover:bg-[var(--bg-subtle,rgb(148_163_184/0.12))]'}"
                onclick={() => (activeApp = a.key)}
              >
                {#if active}<span class="absolute inset-y-2 left-0 w-[3px] rounded-full bg-[var(--color-primary-600)]"></span>{/if}
                <span class="flex items-center justify-between gap-2">
                  <span class="text-[12.5px] font-semibold truncate {active ? 'text-[var(--color-primary-600)]' : ''}">{a.title}</span>
                  <span class="flex items-center gap-1.5 shrink-0">
                    {#if st.fail}<span class="rounded-full px-1.5 py-px text-[10px] font-bold" style="background:var(--color-danger-50);color:var(--color-danger-600)">{st.fail}</span>{/if}
                    <span class="font-mono text-[11px] tabular-nums text-[var(--text-tertiary)]">{st.pct}%</span>
                  </span>
                </span>
                <span class="mt-1.5 flex h-1.5 overflow-hidden rounded-full bg-[rgb(148_163_184/0.22)]">
                  <span style="width:{(st.pass / st.total) * 100}%;background:var(--color-success-600)"></span>
                  <span style="width:{(st.skip / st.total) * 100}%;background:var(--color-info-600)"></span>
                  <span style="width:{(st.fail / st.total) * 100}%;background:var(--color-danger-600)"></span>
                </span>
              </button>
            {/each}
          </div>
        </div>

        <div class="surface-card p-3">
          <p class="flex items-center justify-between pb-2 text-[10.5px] font-bold uppercase tracking-[0.12em] text-[var(--text-tertiary)]">
            <span>{t('platform.qa.findings')}</span>
            {#if failed.length}<span class="rounded-full px-1.5 py-px text-[10px]" style="background:var(--color-danger-50);color:var(--color-danger-600)">{failed.length}</span>{/if}
          </p>
          {#if failed.length === 0}
            <p class="flex items-center gap-2 text-[12px] text-[var(--text-tertiary)]"><i class="icon-circle-check text-[14px]" style="color:var(--color-success-600)"></i>{t('platform.qa.noFindings')}</p>
          {:else}
            <ul class="space-y-1">
              {#each failed.slice(0, 8) as f (f.key)}
                <li>
                  <button type="button" class="w-full rounded-lg px-2 py-1.5 text-left hover:bg-[var(--bg-subtle,rgb(148_163_184/0.12))]" onclick={() => jumpTo(f.appKey)}>
                    <span class="block text-[10px] font-bold uppercase tracking-wider text-[var(--text-tertiary)]">{f.app}</span>
                    <span class="block text-[12px] leading-snug line-clamp-2">{f.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
            {#if failed.length > 8}<p class="px-2 pt-1 text-[11px] text-[var(--text-tertiary)]">{t('platform.qa.moreFindings', { n: failed.length - 8 })}</p>{/if}
          {/if}
        </div>
      </aside>

      <!-- ============ Daftar item ============ -->
      <div class="min-w-0 space-y-4">
        {#if currentApp}
          {@const cs = appStats(currentApp)}
          <div class="surface-card overflow-hidden">
            <div class="flex items-end justify-between gap-4 px-4 pt-4 pb-3">
              <div class="min-w-0">
                <h2 class="font-display font-bold text-[18px] leading-tight truncate">{currentApp.title}</h2>
                <p class="mt-0.5 text-[12px] text-[var(--text-tertiary)]">{t('platform.qa.tested', { done: cs.pass + cs.fail + cs.skip, total: cs.total })}</p>
              </div>
              <p class="font-display font-bold text-[24px] leading-none tabular-nums shrink-0">{cs.pct}<span class="text-[13px] text-[var(--text-tertiary)]">%</span></p>
            </div>
            <div class="mx-4 mb-3 flex h-1.5 overflow-hidden rounded-full bg-[rgb(148_163_184/0.25)]">
              <span style="width:{(cs.pass / cs.total) * 100}%;background:var(--color-success-600)"></span>
              <span style="width:{(cs.skip / cs.total) * 100}%;background:var(--color-info-600)"></span>
              <span style="width:{(cs.fail / cs.total) * 100}%;background:var(--color-danger-600)"></span>
            </div>
            <div class="flex flex-wrap items-center gap-3 border-t border-[var(--border-default)] bg-[var(--surface-sunken)] px-4 py-3">
              <div class="inline-flex rounded-lg border border-[var(--border-strong)] bg-[var(--surface-base)] p-0.5" role="group" aria-label={t('platform.qa.filter')}>
                {#each ['all', 'todo', 'fail'] as const as f (f)}
                  <button type="button" aria-pressed={filter === f} class="rounded-md px-3 py-1.5 text-[12px] font-semibold transition-colors {filter === f ? 'bg-[var(--color-primary-600)] text-white shadow-sm' : 'hover:bg-[rgb(148_163_184/0.18)]'}" onclick={() => (filter = f)}>
                    {t(`platform.qa.filter_${f}`)}
                  </button>
                {/each}
              </div>
              <label class="relative block flex-1 basis-56">
                <i class="icon-search pointer-events-none absolute top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)]" style="inset-inline-start:0.8rem"></i>
                <input
                  type="search"
                  class="field-control qa-field w-full"
                  style="padding-inline-start:2.4rem"
                  placeholder={t('platform.qa.search')}
                  bind:value={query}
                  aria-label={t('platform.qa.search')}
                />
              </label>
            </div>
          </div>

          {@const visible = currentApp.groups.map((g) => ({ g, items: g.items.filter((i) => matches(i.key, i.title)) })).filter((x) => x.items.length)}
          {#if visible.length === 0}
            <div class="rounded-2xl border border-dashed border-[var(--border-default)] p-10 text-center text-[13px] text-[var(--text-tertiary)]">{t('platform.qa.emptyFilter')}</div>
          {/if}

          {#each visible as { g, items } (g.key)}
            {@const done = g.items.filter((i) => results[i.key]).length}
            {@const open = !collapsed[`${currentApp.key}.${g.key}`]}
            <section class="surface-card overflow-hidden">
              <button type="button" class="flex w-full items-center gap-3 bg-[var(--surface-sunken)] px-4 py-3 text-left hover:bg-[rgb(148_163_184/0.16)]" aria-expanded={open} onclick={() => (collapsed = { ...collapsed, [`${currentApp.key}.${g.key}`]: open })}>
                <i class="icon-chevron-down text-[14px] text-[var(--text-tertiary)] transition-transform {open ? '' : '-rotate-90'}"></i>
                <h3 class="text-[13px] font-bold">{g.title}</h3>
                <span class="ml-auto flex items-center gap-2.5">
                  <span class="hidden sm:flex h-1.5 w-24 overflow-hidden rounded-full bg-[rgb(148_163_184/0.22)]">
                    <span style="width:{(done / g.items.length) * 100}%;background:var(--color-primary-600)"></span>
                  </span>
                  <span class="font-mono text-[11px] tabular-nums text-[var(--text-tertiary)]">{done}/{g.items.length}</span>
                </span>
              </button>

              {#if open}
                <ul class="divide-y divide-[var(--border-default)] border-t border-[var(--border-default)]">
                  {#each items as it (it.key)}
                    {@const r = results[it.key]}
                    <li class="relative px-4 py-3 pl-5">
                      <span class="absolute inset-y-0 left-0 w-[3px]" style="background:{r ? `var(--color-${statusTone[r.status]}-600)` : 'transparent'}"></span>
                      <div class="flex flex-wrap items-start gap-x-4 gap-y-2">
                        <div class="min-w-0 flex-1 basis-64">
                          <p class="text-[13px] leading-snug {r?.status === 'pass' ? 'text-[var(--text-secondary,inherit)]' : ''}">{it.title}</p>
                          <p class="mt-1 flex flex-wrap items-center gap-x-2 text-[10.5px] text-[var(--text-tertiary)]">
                            <code class="font-mono">{it.key}</code>
                            {#if r}<span class="inline-flex items-center gap-1"><i class="icon-user text-[10px]"></i>{r.updated_by} · {formatDateTime(r.updated_at, dateOpts)}</span>{/if}
                          </p>
                          {#if r?.note && openNote !== it.key}
                            <p class="mt-2 flex gap-2 rounded-lg border-l-2 px-2.5 py-1.5 text-[12px] whitespace-pre-wrap" style="border-color:var(--color-warning-600);background:var(--color-warning-50)">
                              <i class="icon-message-square text-[12px] mt-0.5 shrink-0" style="color:var(--color-warning-600)"></i><span>{r.note}</span>
                            </p>
                          {/if}
                        </div>

                        <div class="flex items-center gap-2 shrink-0">
                          <div class="inline-flex overflow-hidden rounded-lg border border-[var(--border-default)]" role="group" aria-label={it.title}>
                            {#each statuses as s (s)}
                              {@const on = r?.status === s}
                              <button
                                type="button"
                                class="inline-flex h-8 items-center gap-1.5 px-2.5 text-[12px] font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50 not-first:border-l not-first:border-[var(--border-default)] {on ? 'text-white' : 'hover:bg-[var(--bg-subtle,rgb(148_163_184/0.14))]'}"
                                style={on ? `background:var(--color-${statusTone[s]}-600)` : ''}
                                disabled={readonly || busyKey === it.key}
                                aria-pressed={on}
                                title={t(`platform.qa.status_${s}`)}
                                onclick={() => mark(it.key, on ? 'todo' : s)}
                              >
                                <i class="icon-{statusIcon[s]} text-[13px]"></i><span class="hidden md:inline">{t(`platform.qa.status_${s}`)}</span>
                              </button>
                            {/each}
                          </div>
                          <button
                            type="button"
                            class="inline-flex size-8 items-center justify-center rounded-lg border border-[var(--border-default)] hover:bg-[var(--bg-subtle,rgb(148_163_184/0.14))]"
                            style={r?.note ? 'color:var(--color-warning-600)' : ''}
                            title={t('platform.qa.note')}
                            aria-label={t('platform.qa.note')}
                            aria-expanded={openNote === it.key}
                            onclick={() => toggleNote(it.key)}
                          >
                            <i class="icon-message-square text-[14px]"></i>
                          </button>
                        </div>
                      </div>

                      {#if openNote === it.key}
                        <div class="mt-2.5 flex flex-wrap gap-2">
                          <textarea class="field-control qa-field flex-1 basis-64" rows="2" maxlength="1000" placeholder={t('platform.qa.notePh')} bind:value={draft} disabled={readonly || !r}></textarea>
                          <div class="flex flex-col justify-between items-end gap-1">
                            <span class="font-mono text-[10.5px] text-[var(--text-tertiary)]">{draft.length}/1000</span>
                            <button type="button" class="btn btn-primary !text-[12.5px]" disabled={readonly || !r || busyKey === it.key} onclick={() => saveNote(it.key)}>{t('platform.qa.saveNote')}</button>
                          </div>
                        </div>
                        {#if !r}<p class="mt-1 text-[11.5px] text-[var(--text-tertiary)]">{t('platform.qa.noteNeedsStatus')}</p>{/if}
                      {/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </section>
          {/each}
        {/if}

        {#if readonly}
          <div class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-info"><i class="icon-lock text-[14px] shrink-0"></i><span>{t('platform.qa.closedHint')}</span></div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  /* CSS template memberi warna/latar sendiri pada h1, button, textarea (bahkan !important); aturan ini sengaja eksplisit. */
  .qa-hero h1 {
    color: #fff;
  }
  .qa-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    border-radius: 0.5rem;
    padding: 0.375rem 0.75rem;
    font-size: 12.5px;
    font-weight: 600;
    transition: background-color 0.15s;
  }
  .qa-btn i {
    color: inherit;
  }
  .qa-btn-solid {
    background: #fff;
    color: #0a1f4a;
  }
  .qa-btn-solid:hover {
    background: #e0f2fe;
  }
  .qa-btn-ghost {
    background: rgb(255 255 255 / 0.12);
    color: #fff;
    border: 1px solid rgb(255 255 255 / 0.28);
  }
  .qa-btn-ghost:hover {
    background: rgb(255 255 255 / 0.22);
  }
  .qa-btn-danger {
    background: rgb(251 113 133 / 0.14);
    color: #fecdd3;
    border: 1px solid rgb(253 164 175 / 0.4);
  }
  .qa-btn-danger:hover {
    background: rgb(251 113 133 / 0.26);
  }
  .qa-field {
    color: var(--text-primary);
    background-color: var(--surface-raised) !important;
    border-color: var(--border-strong);
  }
  .qa-field::placeholder {
    color: var(--text-tertiary);
    opacity: 1;
  }
  .qa-field:focus {
    border-color: var(--color-primary-600);
  }
</style>
