<script lang="ts">
  import { items as api, type ImportResult } from '#lib/items/api.ts';
  import { t, formatNumber } from '#lib/i18n/index.ts';
  import { tryT } from '#lib/i18n/i18n.svelte.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';

  const MAX_BYTES = 5 << 20;

  let file = $state<File | null>(null);
  let preview = $state<ImportResult | null>(null);
  let result = $state<ImportResult | null>(null);
  let busy = $state<'check' | 'save' | null>(null);
  let error = $state('');

  function save(blob: Blob, name: string) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function template(format: 'xlsx' | 'csv') {
    error = '';
    try {
      save(await api.importTemplate(format), `template-barang.${format}`);
    } catch (e) {
      error = errorMessage(e);
    }
  }

  function pick(e: Event) {
    const f = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    preview = result = null;
    error = f && f.size > MAX_BYTES ? t('items.import.tooLarge') : '';
    file = error ? null : f;
  }

  async function check() {
    if (!file || busy) return;
    busy = 'check';
    error = '';
    try {
      preview = await api.importPreview(file);
    } catch (e) {
      preview = null;
      error = errorMessage(e);
    } finally {
      busy = null;
    }
  }

  async function commit() {
    if (!file || !preview?.valid || busy) return;
    busy = 'save';
    error = '';
    try {
      result = await api.importCommit(file);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = null;
    }
  }

  function reset() {
    file = preview = result = null;
    error = '';
  }

  const problemText = (code: string) => tryT(`items.import.code.${code}`) ?? fieldMessage(code) ?? code;
  const colText = (col: string) => tryT(`items.import.col.${col}`) ?? col;

  /** Laporan masalah (CSV, UTF-8 BOM agar terbuka benar di Excel). Sel diawali ' bila berawalan karakter rumus. */
  function downloadErrors(r: ImportResult) {
    const cell = (v: string) => `"${(/^[=+\-@\t\r]/.test(v) ? "'" + v : v).replaceAll('"', '""')}"`;
    const lines = [[t('items.import.line'), t('items.import.col.sku'), t('items.import.col.name'), t('items.import.column'), t('items.import.problem')].map(cell).join(',')];
    for (const row of r.errors) {
      for (const [col, code] of Object.entries(row.errors)) {
        lines.push([String(row.line), row.sku, row.name, colText(col), problemText(code)].map(cell).join(','));
      }
    }
    save(new Blob(['﻿' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' }), 'laporan-impor-barang.csv');
  }

  const shown = $derived((result ?? preview)?.errors.length ?? 0);
</script>

<svelte:head><title>{t('items.import.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('items.import.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/items" class="text-[var(--color-primary-600)]">{t('items.title')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('items.import.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-4 max-w-[900px] mx-auto w-full">
  <div>
    <h1 class="font-display font-bold text-[19px]">{t('items.import.title')}</h1>
    <p class="text-[12px] mt-0.5 text-[var(--text-tertiary)]">{t('items.import.subtitle')}</p>
  </div>

  {#if error}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{error}</span>
    </div>
  {/if}

  {#if result}
    <section class="surface-card p-4 space-y-3">
      <h3 class="font-display font-bold text-[14px]">{t('items.import.done')}</h3>
      <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
        <i class="icon-circle-check text-[14px] shrink-0"></i><span>{t('items.import.doneCreated', { count: formatNumber(result.created) })}</span>
      </div>
      {#if result.invalid > 0}
        <p class="text-[12.5px]">{t('items.import.doneSkipped', { count: formatNumber(result.invalid) })}</p>
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => downloadErrors(result!)}>
          <i class="icon-download text-[13px]"></i>{t('items.import.downloadErrors')}
        </button>
      {/if}
      <div class="flex flex-wrap gap-2">
        <a href="/items" class="btn btn-primary !text-[12.5px]">{t('items.import.toList')}</a>
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={reset}>{t('items.import.again')}</button>
      </div>
    </section>
  {:else}
    <section class="surface-card p-4 space-y-3">
      <h3 class="font-display font-bold text-[14px]">{t('items.import.step1')}</h3>
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('items.import.step1Hint')}</p>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => template('xlsx')}><i class="icon-download text-[13px]"></i>{t('items.import.downloadXlsx')}</button>
        <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => template('csv')}><i class="icon-download text-[13px]"></i>{t('items.import.downloadCsv')}</button>
      </div>
    </section>

    <section class="surface-card p-4 space-y-3">
      <h3 class="font-display font-bold text-[14px]">{t('items.import.step2')}</h3>
      <p class="text-[12.5px] text-[var(--text-secondary)]">{t('items.import.fileHint')}</p>
      <div class="flex flex-wrap items-center gap-2">
        <input type="file" accept=".xlsx,.csv" aria-label={t('items.import.choose')} onchange={pick} class="text-[12.5px]" />
        <button type="button" class="btn btn-primary !text-[12.5px]" disabled={!file || busy !== null} onclick={check}>
          {busy === 'check' ? t('items.import.checking') : t('items.import.check')}
        </button>
      </div>
    </section>

    {#if preview}
      <section class="surface-card p-4 space-y-3">
        <h3 class="font-display font-bold text-[14px]">{t('items.import.step3')}</h3>
        <dl class="grid grid-cols-3 gap-3 text-center">
          <div class="rounded-lg border border-[var(--border-subtle)] p-3"><dt class="text-[11.5px] text-[var(--text-tertiary)]">{t('items.import.total')}</dt><dd class="font-bold text-[18px]">{formatNumber(preview.total)}</dd></div>
          <div class="rounded-lg border border-[var(--border-subtle)] p-3"><dt class="text-[11.5px] text-[var(--text-tertiary)]">{t('items.import.valid')}</dt><dd class="font-bold text-[18px]">{formatNumber(preview.valid)}</dd></div>
          <div class="rounded-lg border border-[var(--border-subtle)] p-3"><dt class="text-[11.5px] text-[var(--text-tertiary)]">{t('items.import.invalid')}</dt><dd class="font-bold text-[18px]">{formatNumber(preview.invalid)}</dd></div>
        </dl>

        {#if preview.errors.length}
          <h4 class="font-display font-bold text-[13px]">{t('items.import.problems')}</h4>
          <div class="overflow-x-auto max-h-[360px] overflow-y-auto">
            <table class="w-full text-[12.5px]">
              <thead>
                <tr class="text-left text-[var(--text-tertiary)]">
                  <th class="py-1.5 pr-3 font-medium">{t('items.import.line')}</th>
                  <th class="py-1.5 pr-3 font-medium">{t('items.import.col.name')}</th>
                  <th class="py-1.5 pr-3 font-medium">{t('items.import.column')}</th>
                  <th class="py-1.5 font-medium">{t('items.import.problem')}</th>
                </tr>
              </thead>
              <tbody>
                {#each preview.errors as row (row.line)}
                  {#each Object.entries(row.errors) as [col, code], i (col)}
                    <tr class="border-t border-[var(--border-subtle)]">
                      <td class="py-1.5 pr-3 tabular-nums">{i === 0 ? row.line : ''}</td>
                      <td class="py-1.5 pr-3">{i === 0 ? row.name || row.sku : ''}</td>
                      <td class="py-1.5 pr-3">{colText(col)}</td>
                      <td class="py-1.5">{problemText(code)}</td>
                    </tr>
                  {/each}
                {/each}
              </tbody>
            </table>
          </div>
          {#if preview.invalid > shown}
            <p class="text-[12px] text-[var(--text-tertiary)]">{t('items.import.moreProblems', { shown: formatNumber(shown) })}</p>
          {/if}
          <button type="button" class="btn btn-outline !text-[12.5px]" onclick={() => downloadErrors(preview!)}>
            <i class="icon-download text-[13px]"></i>{t('items.import.downloadErrors')}
          </button>
        {/if}

        {#if preview.valid > 0}
          <p class="text-[12px] text-[var(--text-tertiary)]">{t('items.import.saveHint')}</p>
          <button type="button" class="btn btn-primary !text-[12.5px]" disabled={busy !== null} onclick={commit}>
            {busy === 'save' ? t('items.import.saving') : t('items.import.save', { count: formatNumber(preview.valid) })}
          </button>
        {:else}
          <p class="text-[12.5px]">{t('items.import.noneValid')}</p>
        {/if}
      </section>
    {/if}
  {/if}
</main>
