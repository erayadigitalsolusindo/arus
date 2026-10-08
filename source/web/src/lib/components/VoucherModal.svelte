<script lang="ts">
  // Kupon belanja di kasir: ketik kode → server memvalidasi lewat quote (kode ditambahkan hanya bila lolos).
  // Potongan per kupon berasal dari quote server; layar tidak menghitung sendiri.
  import { ApiError } from '#lib/api/client.ts';
  import { t } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import Modal from '#lib/components/Modal.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  type Applied = { code: string; name: string; kind: string; value: string; amount: string };

  let {
    codes,
    applied,
    money,
    check,
    onadd,
    onremove,
    onclose
  }: {
    /** Kode yang sudah dipakai di keranjang. */
    codes: string[];
    /** Rincian dari quote terbaru (kosong bila quote basi/gagal). */
    applied: Applied[];
    money: (amount: string) => string;
    /** Memvalidasi daftar kode di server; melempar ApiError bila ada yang ditolak. */
    check: (codes: string[]) => Promise<unknown>;
    onadd: (code: string) => void;
    onremove: (code: string) => void;
    onclose: () => void;
  } = $props();

  const MAX = 5;
  let input = $state('');
  let busy = $state(false);
  let error = $state('');

  const detail = (code: string) => applied.find((a) => a.code === code);
  const total = $derived(applied.reduce((s, a) => s + Math.round(Number(a.amount) * 100), 0) / 100);

  async function add(ev: SubmitEvent) {
    ev.preventDefault();
    const code = input.trim().toUpperCase();
    if (!code || busy) return;
    error = '';
    if (codes.includes(code)) {
      error = fieldMessage('DUPLICATE') ?? '';
      return;
    }
    if (codes.length >= MAX) {
      error = fieldMessage('TOO_MANY') ?? '';
      return;
    }
    busy = true;
    try {
      await check([...codes, code]);
      onadd(code);
      input = '';
    } catch (e) {
      if (e instanceof ApiError && e.code === 'VALIDATION') {
        // Kode baru ada di indeks terakhir; galat gabungan (mis. potongan melebihi belanja) tanpa indeks.
        const c = e.fields[`voucher_codes.${codes.length}`] ?? e.fields.voucher_codes ?? Object.values(e.fields)[0];
        error = fieldMessage(c) ?? errorMessage(e);
      } else {
        error = errorMessage(e);
      }
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('vouchers.pos.title')} {onclose}>
  <div class="space-y-3.5">
    <form class="flex items-start gap-2" onsubmit={add} novalidate>
      <div class="grow">
        <label class="sr-only" for="pos-voucher">{t('vouchers.pos.codeLabel')}</label>
        <input
          id="pos-voucher"
          class="w-full field-control font-mono uppercase"
          placeholder={t('vouchers.pos.codePlaceholder')}
          bind:value={input}
          maxlength="32"
          autocomplete="off"
          aria-invalid={!!error}
          use:focusOnMount
        />
        {#if error}<p role="alert" class="text-[11.5px] mt-1 text-[var(--color-danger-600)]">{error}</p>{/if}
      </div>
      <button type="submit" class="btn btn-primary !text-[12.5px] h-10" disabled={busy || !input.trim()}>{busy ? t('vouchers.pos.checking') : t('vouchers.pos.apply')}</button>
    </form>

    <div>
      <div class="text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 text-[var(--text-tertiary)]">{t('vouchers.pos.applied')}</div>
      {#if codes.length === 0}
        <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('vouchers.pos.none')}</p>
      {:else}
        <ul class="space-y-1.5">
          {#each codes as c (c)}
            {@const d = detail(c)}
            <li class="flex items-center gap-2 rounded-lg border border-[var(--border-default)] px-3 py-2">
              <i class="icon-ticket-percent text-[16px] text-[var(--color-success-600)]"></i>
              <div class="min-w-0 grow leading-tight">
                <div class="font-mono font-bold text-[12.5px]">{c}</div>
                <div class="text-[11px] truncate text-[var(--text-tertiary)]">{d?.name ?? ''}</div>
              </div>
              <div class="font-semibold tabular-nums text-[13px] text-[var(--color-success-600)]">{d ? `−${money(d.amount)}` : '…'}</div>
              <button type="button" class="header-icon-btn !size-8" aria-label={t('vouchers.pos.remove')} title={t('vouchers.pos.remove')} onclick={() => onremove(c)}>
                <i class="icon-x text-[13px]"></i>
              </button>
            </li>
          {/each}
        </ul>
        {#if applied.length}
          <div class="flex justify-between mt-2 text-[12.5px] font-semibold">
            <span>{t('vouchers.pos.total')}</span>
            <span class="tabular-nums text-[var(--color-success-600)]">−{money(total.toFixed(2))}</span>
          </div>
        {/if}
      {/if}
    </div>

    <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('vouchers.pos.hint')}</p>
    <div class="flex justify-end">
      <button type="button" class="btn btn-primary !text-[12.5px]" onclick={onclose}>{t('vouchers.pos.done')}</button>
    </div>
  </div>
</Modal>
