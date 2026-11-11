<script lang="ts">
  import { onMount } from 'svelte';
  import { platformApi, type BackupConfig, type BackupRun } from '#lib/platform/api.ts';
  import { relativeTime } from '#lib/platform/relative.ts';
  import StatCard from '#lib/platform/StatCard.svelte';
  import { t, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  let cfg = $state<BackupConfig | null>(null);
  let runs = $state<BackupRun[]>([]);
  let error = $state('');
  let notice = $state('');
  let busy = $state(false);

  let clientId = $state('');
  let clientSecret = $state('');
  let enabled = $state(false);
  let runHour = $state(2);
  let keepCount = $state(14);
  let includeUploads = $state(true);

  let passphrase = $state('');
  let copied = $state(false);
  let manualOpen = $state<number | null | undefined>(undefined);

  const size = (n: number) => (n >= 1024 ** 3 ? `${(n / 1024 ** 3).toFixed(2)} GB` : n >= 1024 ** 2 ? `${(n / 1024 ** 2).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB`);
  const total = (r: BackupRun) => r.db_bytes + r.uploads_bytes;

  async function load(first = false) {
    try {
      const r = await platformApi.backup();
      cfg = r.config;
      runs = r.runs;
      if (first) {
        clientId = cfg.client_id;
        enabled = cfg.enabled;
        runHour = cfg.run_hour;
        keepCount = cfg.keep_count;
        includeUploads = cfg.include_uploads;
      }
    } catch (err) {
      error = errorMessage(err);
    }
  }
  onMount(() => load(true));

  const running = $derived(runs.some((r) => r.status === 'running'));
  // Riwayat diperbarui sendiri selama ada proses yang berjalan.
  $effect(() => {
    if (!running) return;
    const id = setInterval(() => load(), 4000);
    return () => clearInterval(id);
  });

  async function act(fn: () => Promise<unknown>, ok = '') {
    busy = true;
    error = notice = '';
    try {
      await fn();
      if (ok) notice = ok;
      await load();
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const save = () =>
    platformApi.saveBackup({ client_id: clientId.trim(), client_secret: clientSecret.trim(), enabled, run_hour: runHour, keep_count: keepCount, include_uploads: includeUploads }).then(() => {
      clientSecret = '';
    });
  const saveAll = () => act(save, t('platform.backup.saved'));
  const connect = () =>
    act(async () => {
      await save();
      window.location.href = (await platformApi.backupAuthUrl()).url;
    });
  const disconnect = () => confirm(t('platform.backup.disconnectConfirm')) && act(() => platformApi.backupDisconnect());
  const createPassphrase = () =>
    act(async () => {
      passphrase = (await platformApi.backupPassphrase()).passphrase;
      copied = false;
    });
  async function copy() {
    try {
      await navigator.clipboard.writeText(passphrase);
      copied = true;
    } catch {
      /* salin manual */
    }
  }
  const runNow = () => act(() => platformApi.backupRun(), t('platform.backup.runStarted'));
  const restoreTest = () => act(() => platformApi.backupRestoreTest(), t('platform.backup.restoreStarted'));

  // Backup nyata saja (uji restore tidak dihitung sebagai salinan).
  const real = $derived(runs.filter((r) => r.kind !== 'restore_test'));
  const lastRun = $derived(real[0]);
  const lastOk = $derived(real.find((r) => r.status === 'ok'));
  const copies = $derived(real.filter((r) => r.status === 'ok' && !r.deleted).length);

  const steps = $derived(
    cfg
      ? [
          { key: 'Server', icon: 'server', done: cfg.pg_dump && cfg.owner_url },
          { key: 'Drive', icon: 'cloud', done: cfg.connected },
          { key: 'Pass', icon: 'key-round', done: cfg.has_passphrase },
          { key: 'Schedule', icon: 'calendar-clock', done: cfg.enabled }
        ]
      : []
  );
  const pending = $derived(steps.filter((s) => !s.done).length);
  const ready = $derived(!!cfg && cfg.connected && cfg.has_passphrase && cfg.pg_dump && cfg.owner_url);
  const firstOpen = $derived(Math.max(0, steps.findIndex((s) => !s.done)));
  const open = $derived(manualOpen === undefined ? (pending === 0 ? null : firstOpen) : manualOpen);
  const toggle = (i: number) => (manualOpen = open === i ? null : i);

  const stale = $derived(!!lastOk && Date.now() - new Date(lastOk.started_at).getTime() > 48 * 3600 * 1000);
  const phase = $derived.by(() => {
    if (running) return 'running';
    if (!ready) return 'setup';
    if (lastRun?.status === 'failed' || (cfg?.enabled && stale)) return 'fail';
    if (lastOk) return 'ok';
    return 'idle';
  });
  const tone = $derived({ running: 'info', setup: 'warning', fail: 'danger', ok: 'success', idle: 'primary' }[phase] as 'info' | 'warning' | 'danger' | 'success' | 'primary');
  const heroIcon = $derived({ running: 'loader-circle', setup: 'shield-alert', fail: 'triangle-alert', ok: 'shield-check', idle: 'shield' }[phase]);
  const headline = $derived(t(`platform.backup.status${phase === 'ok' ? 'Ok' : phase === 'fail' ? 'Fail' : phase === 'setup' ? 'Setup' : phase === 'running' ? 'Running' : 'Idle'}` as 'platform.backup.statusOk'));
  const hint = $derived(
    phase === 'ok' && lastOk
      ? t('platform.backup.statusOkHint', { time: relativeTime(lastOk.started_at) })
      : phase === 'setup'
        ? t('platform.backup.statusSetupHint', { n: pending })
        : t(`platform.backup.status${phase === 'fail' ? 'Fail' : phase === 'running' ? 'Running' : 'Idle'}Hint` as 'platform.backup.statusIdleHint')
  );

  // Jadwal berikutnya dihitung dalam WIB (UTC+7), sama dengan penjadwal di server.
  const nextRun = $derived.by(() => {
    if (!cfg?.enabled || !ready) return t('platform.backup.off');
    const wib = new Date(Date.now() + 7 * 3600 * 1000);
    const doneToday = !!lastOk && new Date(new Date(lastOk.started_at).getTime() + 7 * 3600 * 1000).toISOString().slice(0, 10) === wib.toISOString().slice(0, 10);
    const day = wib.getUTCHours() < cfg.run_hour && !doneToday ? t('platform.backup.today') : t('platform.backup.tomorrow');
    return t('platform.backup.nextAt', { day, time: `${String(cfg.run_hour).padStart(2, '0')}:00` });
  });

  const bars = $derived.by(() => {
    const list = real.slice(0, 14).reverse();
    const max = Math.max(1, ...list.map(total));
    return list.map((r) => ({ r, h: r.status === 'failed' ? 14 : Math.max(10, Math.round((total(r) / max) * 100)) }));
  });

  const kindLabel = (k: BackupRun['kind']) => t(k === 'auto' ? 'platform.backup.kindAuto' : k === 'manual' ? 'platform.backup.kindManual' : 'platform.backup.kindRestoreTest');
  const kindIcon = (k: BackupRun['kind']) => (k === 'auto' ? 'clock' : k === 'manual' ? 'hand' : 'flask-conical');
  const dot = (s: BackupRun['status']) => (s === 'ok' ? 'bg-[var(--success)]' : s === 'failed' ? 'bg-[var(--danger)]' : 'bg-[var(--color-info-600)] animate-pulse');
  const input = 'block w-full mt-1 px-3 py-2.5 rounded-lg text-[12.5px] outline-none bg-sunken-bordered';
  const stepLabel = (k: string) => t(`platform.backup.step${k}` as 'platform.backup.stepServer');
</script>

<svelte:head><title>{t('platform.backup.title')} · {t('platform.docTitle')}</title></svelte:head>

<div class="space-y-5 max-w-4xl">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('platform.backup.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('platform.backup.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger"><i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span></div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success"><i class="icon-check text-[14px] shrink-0"></i><span>{notice}</span></div>
  {/if}

  {#if passphrase}
    <div class="rounded-xl p-4 space-y-2 badge-warning">
      <div class="text-[13px] font-bold flex items-center gap-2"><i class="icon-key-round text-[16px]"></i>{t('platform.backup.passShownTitle')}</div>
      <code class="block px-3 py-2.5 rounded-lg text-[16px] font-mono tracking-wider break-all bg-sunken-bordered select-all">{passphrase}</code>
      <p class="text-[12px]">{t('platform.backup.passShownWarn')}</p>
      <div class="flex gap-2">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={copy}>{copied ? t('platform.backup.passCopied') : t('platform.backup.passCopy')}</button>
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => (passphrase = '')}>{t('platform.backup.passDone')}</button>
      </div>
    </div>
  {/if}

  {#if cfg}
    <!-- Status utama -->
    <section class="surface-card p-5 flex flex-wrap items-center gap-4" style="border-color:var(--color-{tone}-200)">
      <span class="grid size-14 shrink-0 place-items-center rounded-2xl" style="background:var(--color-{tone}-50);color:var(--color-{tone}-600)">
        <i class="icon-{heroIcon} text-[28px] {phase === 'running' ? 'animate-spin' : ''}"></i>
      </span>
      <div class="min-w-0 flex-1 basis-56">
        <p class="font-display font-bold text-[18px] leading-tight">{headline}</p>
        <p class="text-[12.5px] mt-0.5 text-[var(--text-tertiary)]">{hint}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-primary !text-[13px]" disabled={busy || running || !ready} onclick={runNow}>
          <i class="icon-play text-[14px]"></i>{t('platform.backup.runNow')}
        </button>
        <button type="button" class="btn btn-outline !text-[13px]" disabled={busy || running || !ready || copies === 0} onclick={restoreTest} title={t('platform.backup.restoreHelp')}>
          <i class="icon-flask-conical text-[14px]"></i>{t('platform.backup.restoreTest')}
        </button>
      </div>
    </section>

    <!-- Angka ringkas -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <StatCard label={t('platform.backup.statLast')} value={lastOk ? relativeTime(lastOk.started_at) : t('platform.backup.never')} icon="history" tone={lastOk ? 'success' : 'warning'} hint={lastOk ? formatDateTime(lastOk.started_at) : ''} />
      <StatCard label={t('platform.backup.statNext')} value={nextRun} icon="calendar-clock" tone={cfg.enabled && ready ? 'info' : 'warning'} />
      <StatCard label={t('platform.backup.statCopies')} value={t('platform.backup.copiesOf', { n: copies, max: cfg.keep_count })} icon="cloud" tone="primary" hint={cfg.account} />
      <StatCard label={t('platform.backup.statSize')} value={lastOk ? size(total(lastOk)) : '–'} icon="hard-drive" tone="primary" />
    </div>

    <!-- Persiapan -->
    <section class="surface-card divide-y divide-[var(--border)]">
      <div class="px-4 py-3 flex items-center justify-between">
        <h2 class="font-display font-bold text-[14px]">{t('platform.backup.stepsTitle')}</h2>
        <span class="text-[12px] text-[var(--text-tertiary)]">{steps.length - pending}/{steps.length}</span>
      </div>

      {#each steps as s, i (s.key)}
        <div>
          <button type="button" class="w-full px-4 py-3 flex items-center gap-3 text-left" aria-expanded={open === i} onclick={() => toggle(i)}>
            <span class="grid size-8 place-items-center rounded-full shrink-0 {s.done ? 'badge-success' : 'bg-sunken-bordered text-[var(--text-tertiary)]'}">
              <i class="icon-{s.done ? 'check' : s.icon} text-[15px]"></i>
            </span>
            <span class="font-semibold text-[13px] flex-1">{stepLabel(s.key)}</span>
            <span class="text-[11.5px] {s.done ? 'text-[var(--success)]' : 'text-[var(--text-tertiary)]'}">{s.done ? t('platform.backup.stepDone') : t('platform.backup.stepSet')}</span>
            <i class="icon-chevron-down text-[14px] text-[var(--text-tertiary)] transition-transform {open === i ? 'rotate-180' : ''}"></i>
          </button>

          {#if open === i}
            <div class="px-4 pb-4 pl-[60px] space-y-3">
              {#if s.key === 'Server'}
                <ul class="text-[12.5px] space-y-1">
                  {#each [[cfg.pg_dump, t('platform.backup.srvPgDump')], [cfg.pg_restore, t('platform.backup.srvPgRestore')], [cfg.owner_url, t('platform.backup.srvOwner')]] as [ok, label]}
                    <li class="flex items-center gap-2">
                      <i class="{ok ? 'icon-circle-check text-[var(--success)]' : 'icon-circle-x text-[var(--danger)]'} text-[14px]"></i>{label}
                    </li>
                  {/each}
                </ul>
                <p class="text-[12px] text-[var(--text-tertiary)]">{t('platform.backup.srvHelp')}</p>
              {:else if s.key === 'Drive'}
                <label class="block text-[12.5px] font-semibold">
                  {t('platform.backup.clientId')}
                  <input class={input} maxlength="300" autocomplete="off" bind:value={clientId} disabled={busy} />
                </label>
                <label class="block text-[12.5px] font-semibold">
                  {t('platform.backup.clientSecret')}
                  <input class={input} type="password" maxlength="300" autocomplete="new-password" placeholder={cfg.has_secret ? t('platform.backup.secretSaved') : ''} bind:value={clientSecret} disabled={busy} />
                </label>
                <div class="text-[12.5px]">
                  <div class="font-semibold">{t('platform.backup.redirect')}</div>
                  <code class="block mt-1 px-3 py-2 rounded-lg text-[11.5px] break-all bg-sunken-bordered select-all">{cfg.redirect_uri}</code>
                  <p class="mt-1 text-[12px] text-[var(--text-tertiary)]">{t('platform.backup.redirectHelp')}</p>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="btn btn-primary !text-[12.5px]" disabled={busy || !clientId.trim() || (!cfg.has_secret && !clientSecret.trim())} onclick={connect}>
                    <i class="icon-cloud text-[14px]"></i>{t('platform.backup.connect')}
                  </button>
                  <button type="button" class="btn btn-outline !text-[12.5px]" disabled={busy || !clientId.trim()} onclick={saveAll}>{t('platform.backup.saveClient')}</button>
                  {#if cfg.connected}
                    <span class="text-[12.5px] text-[var(--success)] flex items-center gap-1.5"><i class="icon-circle-check text-[14px]"></i>{cfg.account ? t('platform.backup.connectedAs', { account: cfg.account }) : t('platform.backup.connectedNoEmail')}</span>
                    <button type="button" class="btn btn-outline !text-[12.5px] !text-[var(--danger)]" disabled={busy} onclick={disconnect}>{t('platform.backup.disconnect')}</button>
                  {/if}
                </div>
                <p class="text-[12px] text-[var(--text-tertiary)]">{t('platform.backup.folderNote')}</p>
              {:else if s.key === 'Pass'}
                <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.backup.passHelp')}</p>
                {#if cfg.has_passphrase}
                  <p class="text-[12.5px] flex items-center gap-2"><i class="icon-shield-check text-[14px] text-[var(--success)]"></i>{t('platform.backup.passSet')}</p>
                {:else}
                  <button type="button" class="btn btn-primary !text-[12.5px]" disabled={busy} onclick={createPassphrase}><i class="icon-key-round text-[14px]"></i>{t('platform.backup.passCreate')}</button>
                {/if}
              {:else}
                <label class="flex items-center gap-2 text-[12.5px] font-semibold">
                  <input type="checkbox" bind:checked={enabled} disabled={busy || !ready} />{t('platform.backup.enabled')}
                </label>
                <div class="flex flex-wrap gap-4">
                  <label class="block text-[12.5px] font-semibold">
                    {t('platform.backup.hour')}
                    <select class="{input} !w-[110px]" bind:value={runHour} disabled={busy}>
                      {#each Array.from({ length: 24 }, (_, h) => h) as h}<option value={h}>{String(h).padStart(2, '0')}:00</option>{/each}
                    </select>
                  </label>
                  <label class="block text-[12.5px] font-semibold">
                    {t('platform.backup.keep')}
                    <input class="{input} !w-[110px]" type="number" min="1" max="365" bind:value={keepCount} disabled={busy} />
                  </label>
                </div>
                <label class="flex items-center gap-2 text-[12.5px] font-semibold">
                  <input type="checkbox" bind:checked={includeUploads} disabled={busy} />{t('platform.backup.includeUploads')}
                </label>
                <button type="button" class="btn btn-primary !text-[12.5px]" disabled={busy} onclick={saveAll}>{t('platform.backup.saveSchedule')}</button>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </section>

    <!-- Riwayat -->
    <section class="surface-card p-4 space-y-4">
      <h2 class="font-display font-bold text-[14px]">{t('platform.backup.historyTitle')}</h2>

      {#if bars.length > 1}
        <div>
          <div class="flex items-end gap-1.5 h-16" aria-hidden="true">
            {#each bars as b (b.r.id)}
              <div class="flex-1 rounded-t-md {b.r.status === 'failed' ? 'bg-[var(--danger)]' : b.r.status === 'running' ? 'bg-[var(--color-info-600)] animate-pulse' : 'bg-[var(--color-primary-600)]'} opacity-80" style="height:{b.h}%" title="{formatDateTime(b.r.started_at)} · {total(b.r) ? size(total(b.r)) : ''}"></div>
            {/each}
          </div>
          <p class="text-[11px] mt-1 text-[var(--text-tertiary)]">{t('platform.backup.trend')}</p>
        </div>
      {/if}

      {#if runs.length === 0}
        <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('platform.backup.historyEmpty')}</p>
      {:else}
        <ol class="space-y-0">
          {#each runs as r, i (r.id)}
            <li class="flex gap-3">
              <div class="flex flex-col items-center">
                <span class="mt-1.5 size-2.5 rounded-full shrink-0 {dot(r.status)}"></span>
                {#if i < runs.length - 1}<span class="w-px flex-1 bg-[var(--border)]"></span>{/if}
              </div>
              <div class="pb-3.5 min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[12.5px]">
                  <i class="icon-{kindIcon(r.kind)} text-[13px] text-[var(--text-tertiary)]"></i>
                  <span class="font-semibold">{kindLabel(r.kind)}</span>
                  <span class="text-[var(--text-tertiary)]" title={formatDateTime(r.started_at)}>{relativeTime(r.started_at)}</span>
                  {#if total(r)}<span class="text-[var(--text-tertiary)]">· {size(total(r))}</span>{/if}
                  {#if r.deleted}<span class="text-[var(--text-tertiary)]">· {t('platform.backup.deleted')}</span>{/if}
                </div>
                {#if r.detail}<p class="text-[12px] mt-0.5 break-words {r.status === 'failed' ? 'text-[var(--danger)]' : 'text-[var(--text-tertiary)]'}">{r.detail}</p>{/if}
              </div>
            </li>
          {/each}
        </ol>
      {/if}
      <p class="text-[12px] text-[var(--text-tertiary)]">{t('platform.backup.restoreHow')}</p>
    </section>
  {/if}
</div>
