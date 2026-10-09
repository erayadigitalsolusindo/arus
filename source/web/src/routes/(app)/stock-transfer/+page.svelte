<script lang="ts">
  import { onMount } from 'svelte';
  import { fade, fly, scale } from 'svelte/transition';
  import { flip } from 'svelte/animate';
  import { transfers, BUCKETS, type Bucket, type OpeningRow, type Transfer, type TransferOutlet, type TransferStatus } from '#lib/stock/api.ts';
  import { can, session } from '#lib/auth/session.svelte.ts';
  import { t, formatDateTime, formatNumber } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { ApiError } from '#lib/api/client.ts';
  import Combobox, { type Option } from '#lib/components/Combobox.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import Select from '#lib/components/Select.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { clearDraft, draftKey, loadDraft, saveDraft, type TransferDraft } from '#lib/stock/transfer-draft.ts';

  const PAGE = 20;
  const pill: Record<TransferStatus, string> = { sent: 'badge-warning', received: 'badge-success', cancelled: 'badge-neutral' };
  const statusIcon: Record<TransferStatus, string> = { sent: 'icon-truck', received: 'icon-package-check', cancelled: 'icon-ban' };
  const bucketIcon: Record<Bucket, string> = { display: 'icon-layout-grid', warehouse: 'icon-warehouse', returns: 'icon-undo-2' };
  const bucketTone: Record<Bucket, string> = { display: 'tr-b-display', warehouse: 'tr-b-warehouse', returns: 'tr-b-returns' };
  const num = (s: string) => Number(s.replace(',', '.'));
  const q3 = (s: string | number) => formatNumber(typeof s === 'number' ? s : num(s), { maximumFractionDigits: 3 });
  const label = 'text-[11.5px] font-semibold uppercase tracking-wide flex items-center gap-1.5 text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] text-[var(--color-danger-600)]';

  const canSend = $derived(can('stock_transfer', 'create'));
  const canApprove = $derived(can('stock_transfer', 'approve'));

  // ---- Form kirim ----
  type FormLine = { row: OpeningRow; qty: string };
  // Draf per pengguna+cabang di browser: dipulihkan saat halaman dibuka (lihat lib/stock/transfer-draft.ts).
  const dkey = session.tenant && session.outlet && session.user ? draftKey(session.tenant.id, session.outlet.id, session.user.id) : '';
  const restored = dkey ? loadDraft(dkey) : null;
  let draftNotice = $state(restored !== null);

  let destinations = $state<TransferOutlet[]>([]);
  let toOutlet = $state(restored?.toOutlet ?? '');
  let fromBucket = $state<Bucket>(restored?.fromBucket ?? 'display');
  let toBucket = $state<Bucket>(restored?.toBucket ?? 'display');
  let lines = $state<FormLine[]>(restored?.lines ?? []);
  let note = $state(restored?.note ?? '');
  let pickId = $state('');
  let pickLabel = $state('');
  let busy = $state(false);
  let formError = $state('');
  let fieldErrors = $state<Record<string, string>>({});
  let notice = $state('');

  const local = $derived(toOutlet !== '' && toOutlet === session.outlet?.id);
  const destName = $derived(destinations.find((d) => d.id === toOutlet)?.name ?? '');
  const bucketOptions = $derived(BUCKETS.map((b) => ({ value: b, label: t(`stock.opening.bucket.${b}`) })));
  const destOptions = $derived([
    { value: '', label: t('stock.transfer.form.toPick'), disabled: true },
    ...destinations.map((d) => ({
      value: d.id,
      label: d.id === session.outlet?.id ? t('stock.transfer.form.toHere', { name: d.name }) : `${d.name} (${d.code})`
    }))
  ]);
  const stockOf = (r: OpeningRow) => num(r[fromBucket]);
  const totalQty = $derived(lines.reduce((s, l) => s + (num(l.qty) || 0), 0));

  async function search(q: string): Promise<Option[]> {
    const res = await transfers.items(q.trim());
    return res.data.map((r) => {
      cache[r.id] = r;
      return { id: r.id, name: `${r.sku} — ${r.name}` };
    });
  }
  let cache: Record<string, OpeningRow> = {};

  $effect(() => {
    if (!pickId) return;
    const row = cache[pickId];
    pickId = '';
    pickLabel = '';
    if (!row) return;
    if (lines.some((l) => l.row.id === row.id)) {
      formError = t('stock.transfer.form.duplicate');
      return;
    }
    formError = '';
    lines = [{ row, qty: '' }, ...lines];
  });

  // Buang draf: kosongkan seluruh isian form.
  function discardDraft() {
    lines = [];
    note = '';
    toOutlet = '';
    fromBucket = 'display';
    toBucket = 'display';
    formError = '';
    fieldErrors = {};
    attempt = null;
    draftNotice = false;
    if (dkey) clearDraft(dkey);
  }

  // Simpan otomatis dengan jeda 500 ms dan hanya bila isi berubah. Timer tidak dibatalkan saat komponen dilepas,
  // supaya perubahan terakhir tetap tersimpan.
  let savedSig = '';
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    if (!dkey) return;
    const d: TransferDraft = { toOutlet, fromBucket, toBucket, note, lines: $state.snapshot(lines) };
    const sig = JSON.stringify(d);
    if (sig === savedSig) return;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      savedSig = sig;
      saveDraft(dkey, d);
    }, 500);
  });

  // Kunci idempotensi per isi permintaan: klik ganda memakai kunci yang sama; isi berubah → kunci baru.
  let attempt: { sig: string; key: string } | null = null;
  function keyFor(sig: string): string {
    if (!attempt || attempt.sig !== sig) attempt = { sig, key: `mt-${crypto.randomUUID()}` };
    return attempt.key;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    formError = '';
    fieldErrors = {};
    const input = {
      to_outlet_id: toOutlet,
      from_bucket: fromBucket,
      to_bucket: toBucket,
      ...(note.trim() ? { note } : {}),
      lines: lines.map((l) => ({ item_id: l.row.id, qty: l.qty.trim().replace(',', '.') }))
    };
    try {
      const doc = await transfers.send(input, keyFor(JSON.stringify(input)));
      notice = t(doc.status === 'received' ? 'stock.transfer.form.doneLocal' : 'stock.transfer.form.done', { doc: doc.doc_no });
      lines = [];
      note = '';
      attempt = null;
      draftNotice = false;
      if (dkey) clearDraft(dkey);
      await load(true);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        fieldErrors = Object.fromEntries(Object.entries(err.fields).map(([k, v]) => [k, fieldMessage(v) ?? errorMessage(err)]));
      } else {
        formError = errorMessage(err);
      }
    } finally {
      busy = false;
    }
  }

  // ---- Daftar ----
  let direction = $state<'out' | 'in'>('out');
  let status = $state('');
  let search_ = $state('');
  let rows = $state<Transfer[]>([]);
  let cursor = $state('');
  let hasMore = $state(false);
  let summary = $state({ to_receive: 0, in_transit: 0 });
  let loading = $state(true);
  let loadError = $state('');
  let seq = 0;

  async function load(reset = true) {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await transfers.list({ direction, status, q: search_.trim(), cursor: reset ? '' : cursor, limit: PAGE });
      if (mine !== seq) return;
      rows = reset ? res.data : [...rows, ...res.data];
      cursor = res.next_cursor;
      hasMore = res.has_more;
      summary = res.summary;
    } catch (err) {
      if (mine === seq) loadError = errorMessage(err);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  onMount(async () => {
    if (canSend) {
      try {
        destinations = (await transfers.destinations()).data;
        // Tujuan dari draf yang sudah tidak ada/aktif dikosongkan.
        if (toOutlet && !destinations.some((d) => d.id === toOutlet)) toOutlet = '';
      } catch {
        /* daftar tujuan hanya untuk form; error ditampilkan saat kirim */
      }
      void refreshRestoredStock();
    }
    await load(true);
  });

  // Stok pada baris draf bisa basi: segarkan dari server (server tetap otoritatif saat kirim).
  async function refreshRestoredStock() {
    if (!restored) return;
    for (const l of restored.lines) {
      try {
        const res = await transfers.items(l.row.sku, 5);
        const fresh = res.data.find((r) => r.id === l.row.id);
        const cur = lines.find((x) => x.row.id === l.row.id);
        if (fresh && cur) cur.row = fresh;
      } catch {
        /* stok di layar hanya petunjuk */
      }
    }
  }

  let timer: ReturnType<typeof setTimeout>;
  function refilter() {
    clearTimeout(timer);
    timer = setTimeout(() => load(true), 300);
  }

  // Muat ulang saat cabang aktif berpindah (daftar & stok per cabang).
  let lastOutlet = session.outlet?.id;
  $effect(() => {
    const id = session.outlet?.id;
    if (id === lastOutlet) return;
    lastOutlet = id;
    lines = [];
    toOutlet = '';
    void load(true);
  });

  function setDirection(d: 'out' | 'in') {
    direction = d;
    void load(true);
  }

  // ---- Detail / terima / batal ----
  let detail = $state<Transfer | null>(null);
  let recv = $state<Record<string, string>>({});
  let mode = $state<'view' | 'receive' | 'cancel'>('view');
  let reason = $state('');
  let dBusy = $state(false);
  let dError = $state('');

  const fullRecv = (d: Transfer) => Object.fromEntries((d.lines ?? []).map((l) => [l.item_id, l.qty_sent.replace(/\.?0+$/, '')]));

  async function open(id: string) {
    dError = '';
    mode = 'view';
    reason = '';
    try {
      const d = await transfers.get(id);
      recv = fullRecv(d);
      detail = d;
    } catch (err) {
      loadError = errorMessage(err);
    }
  }

  const shortQty = $derived(detail?.lines ? detail.lines.reduce((s, l) => s + Math.max(0, num(l.qty_sent) - (num(recv[l.item_id] ?? '0') || 0)), 0) : 0);
  // Cabang aktif boleh menerima hanya bila ia cabang tujuan; pembatalan boleh dari pengirim maupun penerima.
  const canReceiveHere = $derived(!!detail && detail.status === 'sent' && canApprove && detail.to.id === session.outlet?.id && detail.from.id !== detail.to.id);
  const canCancelHere = $derived(!!detail && detail.status === 'sent' && canApprove && (detail.to.id === session.outlet?.id || detail.from.id === session.outlet?.id));

  async function doReceive() {
    if (!detail || dBusy) return;
    dBusy = true;
    dError = '';
    try {
      detail = await transfers.receive(detail.id, (detail.lines ?? []).map((l) => ({ item_id: l.item_id, qty: (recv[l.item_id] || '0').replace(',', '.') })));
      mode = 'view';
      await load(true);
    } catch (err) {
      dError = errorMessage(err);
    } finally {
      dBusy = false;
    }
  }

  async function doCancel() {
    if (!detail || dBusy) return;
    dBusy = true;
    dError = '';
    try {
      detail = await transfers.cancel(detail.id, reason.trim());
      mode = 'view';
      await load(true);
    } catch (err) {
      dError = errorMessage(err);
    } finally {
      dBusy = false;
    }
  }

  // Tahap linimasa dokumen: 0 = dikirim, 1 = dalam perjalanan, 2 = selesai (diterima / dibatalkan).
  const stage = (d: Transfer) => (d.status === 'sent' ? 1 : 2);
  const sameOutlet = (d: Transfer) => d.from.id === d.to.id;
</script>

<svelte:head><title>{t('stock.transfer.docTitle')}</title></svelte:head>

<div class="px-4 lg:px-6 py-3 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
  <div class="flex flex-wrap items-center justify-between gap-2 max-w-[1600px] mx-auto">
    <h2 class="font-display font-bold text-[13px] tracking-wide uppercase">{t('stock.transfer.title')}</h2>
    <div class="flex items-center gap-1.5 text-[12px] font-medium">
      <a href="/dashboard" class="text-[var(--color-primary-600)]">{t('nav.dashboard')}</a>
      <span class="text-[var(--text-tertiary)]">/</span>
      <span class="text-[var(--text-tertiary)]">{t('stock.transfer.title')}</span>
    </div>
  </div>
</div>

<main class="p-4 lg:p-6 space-y-5 max-w-[1200px] mx-auto w-full">
  <!-- Hero: judul, adegan truk pengantar antar cabang, dan dua angka ringkas -->
  <section class="tr-hero rounded-2xl text-white relative overflow-hidden" in:fade={{ duration: 250 }}>
    <div class="relative z-[1] p-5 lg:p-6 grid gap-5 lg:grid-cols-[1fr_minmax(0,420px)] lg:items-center">
      <div class="min-w-0 space-y-3">
        <div class="flex items-center gap-3">
          <span class="tr-hero-icon grid place-items-center size-11 rounded-xl"><i class="icon-arrow-right-left text-[21px]"></i></span>
          <div class="min-w-0">
            <h1 class="font-display font-bold text-[20px] leading-tight">{t('stock.transfer.title')}</h1>
            {#if session.outlet}<p class="text-[12px] opacity-80 inline-flex items-center gap-1.5"><i class="icon-map-pin text-[12px]"></i>{session.outlet.name}</p>{/if}
          </div>
        </div>
        <p class="text-[12.5px] leading-relaxed opacity-85 max-w-[560px]">{t('stock.transfer.subtitle')}</p>
        <div class="flex flex-wrap gap-2.5 pt-1">
          <div class="tr-glass rounded-xl px-3.5 py-2.5 flex items-center gap-3">
            <span class="relative grid place-items-center size-9 rounded-lg bg-white/15">
              <i class="icon-package-check text-[17px]"></i>
              {#if summary.to_receive > 0}<span class="tr-ping absolute -top-0.5 -end-0.5 size-2.5 rounded-full bg-amber-300"></span>{/if}
            </span>
            <div>
              <div class="font-display font-bold text-[20px] leading-none tabular-nums">{formatNumber(summary.to_receive)}</div>
              <div class="text-[11px] opacity-80 mt-0.5">{t('stock.transfer.summary.toReceive')}</div>
            </div>
          </div>
          <div class="tr-glass rounded-xl px-3.5 py-2.5 flex items-center gap-3">
            <span class="relative grid place-items-center size-9 rounded-lg bg-white/15">
              <i class="icon-truck text-[17px]"></i>
              {#if summary.in_transit > 0}<span class="tr-ping absolute -top-0.5 -end-0.5 size-2.5 rounded-full bg-sky-300"></span>{/if}
            </span>
            <div>
              <div class="font-display font-bold text-[20px] leading-none tabular-nums">{formatNumber(summary.in_transit)}</div>
              <div class="text-[11px] opacity-80 mt-0.5">{t('stock.transfer.summary.inTransit')}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- Adegan animasi: dua cabang dan truk yang bolak-balik -->
      <div class="tr-scene hidden sm:block" aria-hidden="true">
        <div class="tr-node"><i class="icon-store text-[22px]"></i></div>
        <div class="tr-track">
          <span class="tr-truck"><i class="icon-truck text-[17px]"></i></span>
          <span class="tr-pkg tr-pkg-1"><i class="icon-package text-[12px]"></i></span>
          <span class="tr-pkg tr-pkg-2"><i class="icon-package text-[12px]"></i></span>
        </div>
        <div class="tr-node"><i class="icon-store text-[22px]"></i></div>
      </div>
    </div>
    <span class="tr-orb tr-orb-1"></span><span class="tr-orb tr-orb-2"></span>
  </section>

  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-xl px-3.5 py-3 text-[12.5px] badge-success" in:fly={{ y: -8, duration: 220 }}>
      <i class="icon-circle-check text-[16px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}

  {#if canSend && draftNotice}
    <div role="status" class="flex flex-wrap items-center gap-3 rounded-xl px-3.5 py-2.5 text-[12.5px] badge-warning" in:fly={{ y: -8, duration: 220 }}>
      <i class="icon-save text-[15px] shrink-0"></i>
      <span class="grow">{t('stock.transfer.form.draftRestored')}</span>
      <button type="button" class="btn btn-sm" onclick={discardDraft}><i class="icon-trash-2 text-[13px]"></i>{t('stock.transfer.form.draftDiscard')}</button>
    </div>
  {/if}

  {#if canSend}
    <form class="surface-card !p-0 overflow-hidden" onsubmit={submit} novalidate in:fly={{ y: 12, duration: 280 }}>
      <header class="flex items-center gap-2.5 px-4 lg:px-5 py-3.5 border-b border-[var(--border-subtle)]">
        <span class="grid place-items-center size-8 rounded-lg tr-tile-primary"><i class="icon-send text-[15px]"></i></span>
        <h3 class="font-display font-bold text-[14px]">{t('stock.transfer.form.title')}</h3>
      </header>

      <div class="p-4 lg:p-5 space-y-6">
        <!-- 1 · Rute -->
        <section class="space-y-3">
          <h4 class="tr-step"><span class="tr-step-no">1</span>{t('stock.transfer.form.step1')}</h4>
          <div class="tr-route rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3.5 grid gap-3 md:grid-cols-[1fr_auto_1fr] md:items-center">
            <div class="min-w-0">
              <div class="text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1">{t('stock.transfer.form.from')}</div>
              <div class="flex items-center gap-2 min-w-0">
                <span class="grid place-items-center size-9 rounded-lg shrink-0 tr-tile-primary"><i class="icon-store text-[16px]"></i></span>
                <div class="min-w-0">
                  <div class="font-semibold text-[13px] truncate">{session.outlet?.name ?? '—'}</div>
                  <span class="tr-chip {bucketTone[fromBucket]}"><i class="{bucketIcon[fromBucket]} text-[11px]"></i>{t(`stock.opening.bucket.${fromBucket}`)}</span>
                </div>
              </div>
            </div>
            <div class="tr-arrow justify-self-center" aria-hidden="true">
              <i class="icon-arrow-down md:hidden text-[18px]"></i><i class="icon-move-right hidden md:inline text-[22px]"></i>
            </div>
            <div class="min-w-0">
              <div class="text-[10.5px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1">{t('stock.transfer.form.to')}</div>
              <div class="flex items-center gap-2 min-w-0">
                <span class="grid place-items-center size-9 rounded-lg shrink-0 {toOutlet ? 'tr-tile-success' : 'tr-tile-muted'}"><i class="{local ? 'icon-layers' : 'icon-store'} text-[16px]"></i></span>
                <div class="min-w-0">
                  <div class="font-semibold text-[13px] truncate {toOutlet ? '' : 'text-[var(--text-tertiary)]'}">{toOutlet ? destName : t('stock.transfer.form.destUnset')}</div>
                  <span class="tr-chip {bucketTone[toBucket]}"><i class="{bucketIcon[toBucket]} text-[11px]"></i>{t(`stock.opening.bucket.${toBucket}`)}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="grid gap-3 md:grid-cols-3">
            <div class="space-y-1.5">
              <label for="tr-to" class={label}><i class="icon-map-pin text-[12px]"></i>{t('stock.transfer.form.to')}</label>
              <Select id="tr-to" bind:value={toOutlet} options={destOptions} invalid={!!fieldErrors.to_outlet_id} />
              {#if fieldErrors.to_outlet_id}<p class={errClass} role="alert">{fieldErrors.to_outlet_id}</p>{/if}
            </div>
            <div class="space-y-1.5">
              <label for="tr-fb" class={label}><i class="icon-log-out text-[12px]"></i>{t('stock.transfer.form.fromBucket')}</label>
              <Select id="tr-fb" bind:value={fromBucket} options={bucketOptions} invalid={!!fieldErrors.from_bucket} />
            </div>
            <div class="space-y-1.5">
              <label for="tr-tb" class={label}><i class="icon-log-in text-[12px]"></i>{t('stock.transfer.form.toBucket')}</label>
              <Select id="tr-tb" bind:value={toBucket} options={bucketOptions} invalid={!!fieldErrors.to_bucket} />
              {#if fieldErrors.to_bucket}<p class={errClass} role="alert">{fieldErrors.to_bucket}</p>{/if}
            </div>
          </div>
          {#if toOutlet}
            <p class="flex items-start gap-2 text-[11.5px] leading-relaxed rounded-lg px-3 py-2 {local ? 'badge-success' : 'badge-info'}" in:fly={{ y: -6, duration: 200 }}>
              <i class="{local ? 'icon-zap' : 'icon-info'} text-[13px] mt-px shrink-0"></i>{local ? t('stock.transfer.form.hintLocal') : t('stock.transfer.form.hintBranch')}
            </p>
          {/if}
        </section>

        <!-- 2 · Barang -->
        <section class="space-y-3">
          <h4 class="tr-step">
            <span class="tr-step-no">2</span>{t('stock.transfer.form.step2')}
            {#if lines.length > 0}<span class="ms-auto text-[11.5px] font-medium normal-case tracking-normal text-[var(--text-tertiary)]" in:scale={{ duration: 180 }}>{t('stock.transfer.items', { count: lines.length, qty: q3(totalQty) })}</span>{/if}
          </h4>
          <div class="space-y-1.5">
            <label for="tr-item" class="sr-only">{t('stock.transfer.form.addItem')}</label>
            <Combobox id="tr-item" bind:value={pickId} bind:label={pickLabel} {search} placeholder={t('stock.transfer.form.pickItem')} clearable={false} />
          </div>
          {#if fieldErrors.lines}<p class={errClass} role="alert">{fieldErrors.lines}</p>{/if}

          <div class="space-y-2">
            {#each lines as l, i (l.row.id)}
              {@const err = fieldErrors[`lines.${i}.qty`] ?? fieldErrors[`lines.${i}.item_id`]}
              {@const stock = stockOf(l.row)}
              {@const over = num(l.qty) > stock}
              {@const pct = stock > 0 ? Math.min(100, ((num(l.qty) || 0) / stock) * 100) : 0}
              <div class="tr-line rounded-xl border bg-[var(--surface-base)] px-3.5 py-3 {err || over ? 'border-[var(--color-danger-600)]' : 'border-[var(--border-subtle)]'}" animate:flip={{ duration: 220 }} in:fly={{ y: -10, duration: 220 }} out:fade={{ duration: 140 }}>
                <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
                  <span class="grid place-items-center size-9 rounded-lg shrink-0 tr-tile-muted"><i class="icon-package text-[16px]"></i></span>
                  <div class="min-w-0 grow basis-[200px]">
                    <div class="font-semibold text-[12.5px] truncate">{l.row.name}</div>
                    <div class="text-[11.5px] text-[var(--text-tertiary)] flex flex-wrap items-center gap-x-2">
                      <span class="font-mono">{l.row.sku}</span>
                      <span class="tr-chip {bucketTone[fromBucket]}"><i class="{bucketIcon[fromBucket]} text-[10px]"></i>{q3(l.row[fromBucket])} {l.row.unit}</span>
                    </div>
                  </div>
                  <MoneyInput
                    class="field-control !text-end w-28 {err || over ? '!border-[var(--color-danger-600)]' : ''}"
                    decimals={3}
                    pad={false}
                    aria-label="{l.row.name} – {t('stock.transfer.form.qty')}"
                    placeholder={t('stock.transfer.form.qty')}
                    bind:value={l.qty}
                  />
                  <span class="w-12 shrink-0 text-[12px] font-semibold truncate">{l.row.unit}</span>
                  <button type="button" class="header-icon-btn tr-del" aria-label={t('stock.transfer.form.remove')} onclick={() => (lines = lines.filter((x) => x.row.id !== l.row.id))}>
                    <i class="icon-trash-2 text-[14px]"></i>
                  </button>
                </div>
                <div class="tr-bar mt-2.5" role="presentation"><span class={over ? 'is-over' : ''} style="width:{pct}%"></span></div>
                {#if err}<p class="{errClass} mt-1.5" role="alert">{err}</p>{:else if over}<p class="{errClass} mt-1.5">{t('errors.STOCK_INSUFFICIENT')}</p>{/if}
              </div>
            {:else}
              <div class="tr-empty rounded-xl border border-dashed border-[var(--border-default)] px-4 py-7 text-center" in:fade={{ duration: 200 }}>
                <span class="tr-float inline-grid place-items-center size-11 rounded-2xl tr-tile-muted"><i class="icon-package-open text-[22px]"></i></span>
                <p class="mt-2.5 text-[12.5px] text-[var(--text-tertiary)]">{t('stock.transfer.form.empty')}</p>
              </div>
            {/each}
          </div>
        </section>

        <!-- 3 · Catatan -->
        <section class="space-y-2">
          <h4 class="tr-step"><span class="tr-step-no">3</span>{t('stock.transfer.form.step3')}</h4>
          <label for="tr-note" class="sr-only">{t('stock.transfer.form.note')}</label>
          <textarea id="tr-note" class="field-control w-full" rows="2" maxlength="1000" placeholder={t('stock.transfer.form.note')} bind:value={note}></textarea>
          {#if fieldErrors.note}<p class={errClass} role="alert">{fieldErrors.note}</p>{/if}
        </section>

        {#if formError}
          <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger" in:fly={{ y: -6, duration: 200 }}>
            <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{formError}</span>
          </div>
        {/if}
      </div>

      <footer class="flex items-center justify-end gap-3 px-4 lg:px-5 py-3.5 border-t border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
        <button type="submit" class="btn btn-primary !text-[12.5px] disabled:opacity-60 tr-send" disabled={busy || !toOutlet || lines.length === 0 || lines.some((l) => !(num(l.qty) > 0))}>
          {#if busy}<i class="icon-loader-circle text-[14px] tr-spin"></i>{:else}<i class="icon-send text-[14px] tr-send-icon"></i>{/if}
          {busy ? t('stock.transfer.form.submitting') : t('stock.transfer.form.submit')}
        </button>
      </footer>
    </form>
  {/if}

  {#if loadError}
    <div role="alert" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] shrink-0"></i><span>{t('stock.transfer.loadFailed')} {loadError}</span>
    </div>
  {/if}

  <!-- Riwayat -->
  <div class="surface-card !p-0 overflow-hidden" in:fly={{ y: 14, duration: 300, delay: 60 }}>
    <div class="flex flex-wrap items-center gap-3 px-4 py-3 border-b border-[var(--border-subtle)]">
      <span class="grid place-items-center size-8 rounded-lg tr-tile-primary"><i class="icon-history text-[15px]"></i></span>
      <h3 class="font-display font-bold text-[14px] me-auto">{t('stock.transfer.history')}</h3>
      <div class="tr-tabs" role="tablist" data-dir={direction}>
        <span class="tr-tabs-pill" aria-hidden="true"></span>
        {#each ['out', 'in'] as const as d (d)}
          <button type="button" role="tab" aria-selected={direction === d} class="tr-tab" onclick={() => setDirection(d)}>
            <i class="{d === 'out' ? 'icon-arrow-up-right' : 'icon-arrow-down-left'} text-[13px]"></i>{t(`stock.transfer.tab.${d}`)}
            {#if (d === 'out' ? summary.in_transit : summary.to_receive) > 0}
              <span class="tr-count" in:scale={{ duration: 160 }}>{d === 'out' ? summary.in_transit : summary.to_receive}</span>
            {/if}
          </button>
        {/each}
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-2 px-4 py-2.5 border-b border-[var(--border-subtle)] bg-[var(--surface-sunken)]">
      <div class="relative grow min-w-[160px] max-w-[300px]">
        <i class="icon-search text-[13px] absolute top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]" style="inset-inline-start:0.7rem"></i>
        <input type="search" class="field-control w-full !text-[12.5px]" style="padding-inline-start:2rem" placeholder={t('stock.transfer.search')} bind:value={search_} oninput={refilter} />
      </div>
      <Select
        class="w-[190px]"
        ariaLabel={t('stock.transfer.col.status')}
        bind:value={status}
        onchange={() => load(true)}
        options={[{ value: '', label: t('stock.transfer.filter.all') }, ...(['sent', 'received', 'cancelled'] as const).map((s) => ({ value: s, label: t(`stock.transfer.status.${s}`) }))]}
      />
    </div>

    <div class="overflow-x-auto scroll-thin">
      <table class="w-full text-[12.5px] min-w-[760px]">
        <thead>
          <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)]">
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.transfer.col.doc')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.transfer.col.route')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.transfer.col.items')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.transfer.col.status')}</th>
            <th class="px-4 py-2.5 text-start font-semibold" scope="col">{t('stock.transfer.col.by')}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as r, i (r.id)}
            <tr class="tr-row border-t border-[var(--border-subtle)] align-middle" in:fly={{ y: 8, duration: 220, delay: Math.min(i, 8) * 25 }}>
              <td class="px-4 py-3 whitespace-nowrap">
                <button type="button" class="font-mono text-[12px] font-semibold text-[var(--color-primary-600)] hover:underline" onclick={() => open(r.id)}>{r.doc_no}</button>
                <div class="text-[11.5px] text-[var(--text-tertiary)] flex items-center gap-1"><i class="icon-clock text-[11px]"></i>{formatDateTime(new Date(r.sent_at))}</div>
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span class="inline-flex items-center gap-1.5 font-medium"><i class="icon-store text-[13px] text-[var(--text-tertiary)]"></i>{r.from.name}</span>
                  <i class="icon-move-right text-[13px] text-[var(--color-primary-600)]"></i>
                  <span class="inline-flex items-center gap-1.5 font-medium"><i class="icon-store text-[13px] text-[var(--text-tertiary)]"></i>{r.to.name}</span>
                </div>
                <div class="flex items-center gap-1.5 mt-1">
                  <span class="tr-chip {bucketTone[r.from_bucket]}"><i class="{bucketIcon[r.from_bucket]} text-[10px]"></i>{t(`stock.opening.bucket.${r.from_bucket}`)}</span>
                  <i class="icon-chevron-right text-[10px] text-[var(--text-tertiary)]"></i>
                  <span class="tr-chip {bucketTone[r.to_bucket]}"><i class="{bucketIcon[r.to_bucket]} text-[10px]"></i>{t(`stock.opening.bucket.${r.to_bucket}`)}</span>
                </div>
              </td>
              <td class="px-4 py-3 whitespace-nowrap">
                <span class="inline-flex items-center gap-1.5"><i class="icon-boxes text-[13px] text-[var(--text-tertiary)]"></i>{t('stock.transfer.items', { count: r.line_count, qty: q3(r.qty_sent) })}</span>
                {#if r.status === 'received' && num(r.qty_short) > 0}
                  <div class="text-[11.5px] text-[var(--color-danger-600)] inline-flex items-center gap-1"><i class="icon-triangle-alert text-[11px]"></i>{t('stock.transfer.short', { qty: q3(r.qty_short) })}</div>
                {/if}
              </td>
              <td class="px-4 py-3 whitespace-nowrap">
                <span class="badge-soft {pill[r.status]} inline-flex items-center gap-1.5">
                  {#if r.status === 'sent'}<span class="tr-dot"></span>{/if}
                  <i class="{statusIcon[r.status]} text-[12px]"></i>{t(`stock.transfer.status.${r.status}`)}
                </span>
              </td>
              <td class="px-4 py-3">
                <span class="inline-flex items-center gap-2"><span class="grid place-items-center size-6 rounded-full text-[10px] font-bold tr-tile-muted">{r.sent_by.slice(0, 1).toUpperCase()}</span>{r.sent_by}</span>
              </td>
            </tr>
          {:else}
            {#if loading}
              {#each [0, 1, 2] as k (k)}
                <tr class="border-t border-[var(--border-subtle)]"><td colspan="5" class="px-4 py-3"><div class="tr-skel h-10 rounded-lg"></div></td></tr>
              {/each}
            {:else}
              <tr>
                <td colspan="5" class="px-4 py-10 text-center">
                  <span class="tr-float inline-grid place-items-center size-12 rounded-2xl tr-tile-muted"><i class="icon-inbox text-[24px]"></i></span>
                  <p class="mt-3 font-semibold text-[13px]">{t('stock.transfer.empty')}</p>
                  <p class="mt-1 text-[12px] text-[var(--text-tertiary)] max-w-[360px] mx-auto">{t('stock.transfer.emptyHint')}</p>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
    {#if hasMore}
      <div class="flex justify-center px-4 py-3 border-t border-[var(--border-subtle)]">
        <button type="button" class="btn !text-[12px]" disabled={loading} onclick={() => load(false)}><i class="icon-chevrons-down text-[13px]"></i>{t('stock.transfer.loadMore')}</button>
      </div>
    {/if}
  </div>
</main>

{#if detail}
  {@const d = detail}
  <Modal title={t('stock.transfer.detail.title', { doc: d.doc_no })} wide onclose={() => (detail = null)}>
    <div class="space-y-5 text-[12.5px]">
      <!-- Linimasa -->
      {#if !sameOutlet(d)}
        {@const st = stage(d)}
        <ol class="tr-timeline" style="--fill:{st === 1 ? 50 : 100}%">
          <li class="is-done"><span class="tr-tl-dot"><i class="icon-send text-[13px]"></i></span><b>{t('stock.transfer.step.sent')}</b><small>{formatDateTime(new Date(d.sent_at))}</small></li>
          <li class={d.status === 'sent' ? 'is-now' : 'is-done'}><span class="tr-tl-dot"><i class="icon-truck text-[13px]"></i></span><b>{t('stock.transfer.step.transit')}</b></li>
          <li class={d.status === 'received' ? 'is-done is-ok' : d.status === 'cancelled' ? 'is-done is-bad' : ''}>
            <span class="tr-tl-dot"><i class="{d.status === 'cancelled' ? 'icon-ban' : 'icon-package-check'} text-[13px]"></i></span>
            <b>{d.status === 'cancelled' ? t('stock.transfer.step.cancelled') : t('stock.transfer.step.received')}</b>
            {#if d.received_at}<small>{formatDateTime(new Date(d.received_at))}</small>{:else if d.cancelled_at}<small>{formatDateTime(new Date(d.cancelled_at))}</small>{/if}
          </li>
        </ol>
      {:else}
        <div class="flex items-center gap-2"><span class="badge-soft {pill[d.status]} inline-flex items-center gap-1.5"><i class="{statusIcon[d.status]} text-[12px]"></i>{t(`stock.transfer.status.${d.status}`)}</span></div>
      {/if}

      <div class="rounded-xl border border-[var(--border-subtle)] bg-[var(--surface-sunken)] p-3.5 grid gap-3 sm:grid-cols-[1fr_auto_1fr] sm:items-center">
        <div class="flex items-center gap-2.5 min-w-0">
          <span class="grid place-items-center size-9 rounded-lg shrink-0 tr-tile-primary"><i class="icon-store text-[16px]"></i></span>
          <div class="min-w-0"><div class="font-semibold truncate">{d.from.name}</div><span class="tr-chip {bucketTone[d.from_bucket]}"><i class="{bucketIcon[d.from_bucket]} text-[10px]"></i>{t(`stock.opening.bucket.${d.from_bucket}`)}</span></div>
        </div>
        <i class="icon-move-right text-[20px] justify-self-center tr-arrow hidden sm:inline"></i>
        <div class="flex items-center gap-2.5 min-w-0">
          <span class="grid place-items-center size-9 rounded-lg shrink-0 tr-tile-success"><i class="icon-store text-[16px]"></i></span>
          <div class="min-w-0"><div class="font-semibold truncate">{d.to.name}</div><span class="tr-chip {bucketTone[d.to_bucket]}"><i class="{bucketIcon[d.to_bucket]} text-[10px]"></i>{t(`stock.opening.bucket.${d.to_bucket}`)}</span></div>
        </div>
      </div>

      <ul class="space-y-1 text-[var(--text-tertiary)]">
        <li class="flex items-center gap-2"><i class="icon-user text-[12px]"></i>{t('stock.transfer.detail.sentBy', { name: d.sent_by, at: formatDateTime(new Date(d.sent_at)) })}</li>
        {#if d.received_at}<li class="flex items-center gap-2"><i class="icon-circle-check text-[12px]"></i>{t('stock.transfer.detail.receivedBy', { name: d.received_by ?? '', at: formatDateTime(new Date(d.received_at)) })}</li>{/if}
        {#if d.cancelled_at}
          <li class="flex items-center gap-2"><i class="icon-ban text-[12px]"></i>{t('stock.transfer.detail.cancelledBy', { name: d.cancelled_by ?? '', at: formatDateTime(new Date(d.cancelled_at)) })}</li>
          <li class="flex items-center gap-2"><i class="icon-message-square text-[12px]"></i>{t('stock.transfer.detail.reason', { reason: d.cancel_reason ?? '' })}</li>
        {/if}
      </ul>
      {#if d.note}<p class="whitespace-pre-line rounded-lg px-3 py-2 bg-[var(--surface-sunken)] border-s-2 border-[var(--color-primary-600)]">{d.note}</p>{/if}

      <div class="overflow-x-auto scroll-thin rounded-xl border border-[var(--border-subtle)]">
        <table class="w-full min-w-[480px]">
          <thead>
            <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-tertiary)] bg-[var(--surface-sunken)]">
              <th class="px-3 py-2 text-start font-semibold" scope="col">{t('stock.transfer.detail.colItem')}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.transfer.detail.colSent')}</th>
              <th class="px-3 py-2 text-end font-semibold" scope="col">{t('stock.transfer.detail.colReceived')}</th>
            </tr>
          </thead>
          <tbody>
            {#each d.lines ?? [] as l, i (l.item_id)}
              <tr class="border-t border-[var(--border-subtle)]" in:fly={{ y: 6, duration: 200, delay: Math.min(i, 8) * 30 }}>
                <td class="px-3 py-2.5">
                  <div class="flex items-center gap-2.5"><span class="grid place-items-center size-8 rounded-lg shrink-0 tr-tile-muted"><i class="icon-package text-[14px]"></i></span><div><div class="font-semibold">{l.name}</div><div class="text-[11.5px] text-[var(--text-tertiary)] font-mono">{l.sku}</div></div></div>
                </td>
                <td class="px-3 py-2.5 text-end whitespace-nowrap">{q3(l.qty_sent)} {l.unit}</td>
                <td class="px-3 py-2.5 text-end whitespace-nowrap">
                  {#if mode === 'receive'}
                    <div class="inline-flex items-center gap-1.5">
                      <MoneyInput class="field-control !text-end w-24" decimals={3} pad={false} aria-label="{l.name} – {t('stock.transfer.detail.colReceived')}" bind:value={recv[l.item_id]} />
                      <span>{l.unit}</span>
                    </div>
                  {:else if l.qty_received !== null}
                    {@const short = num(l.qty_received) < num(l.qty_sent)}
                    <span class="inline-flex items-center gap-1 font-semibold {short ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600,#16a34a)]'}"><i class="{short ? 'icon-triangle-alert' : 'icon-check'} text-[12px]"></i>{q3(l.qty_received)} {l.unit}</span>
                  {:else}<span class="text-[var(--text-tertiary)]">—</span>{/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      {#if mode === 'receive'}
        <p class="flex items-start gap-2 text-[var(--text-tertiary)]"><i class="icon-info text-[13px] mt-px shrink-0"></i>{t('stock.transfer.detail.receiveHint')}</p>
        {#if shortQty > 0}
          <p class="flex items-start gap-2 rounded-lg px-3 py-2 badge-warning" in:fly={{ y: -6, duration: 180 }}><i class="icon-triangle-alert text-[14px] mt-px shrink-0"></i><span>{t('stock.transfer.detail.shortWarn', { qty: q3(shortQty) })}</span></p>
        {/if}
      {:else if mode === 'cancel'}
        <div class="space-y-1.5" in:fly={{ y: -6, duration: 180 }}>
          <label for="tr-reason" class={label}><i class="icon-message-square text-[12px]"></i>{t('stock.transfer.detail.cancelReason')}</label>
          <input id="tr-reason" class="field-control w-full" maxlength="200" bind:value={reason} />
          <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('stock.transfer.detail.cancelHint')}</p>
        </div>
      {/if}

      {#if dError}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 badge-danger"><i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{dError}</span></div>
      {/if}

      <div class="flex flex-wrap justify-end gap-2">
        {#if mode === 'view'}
          {#if canCancelHere}<button type="button" class="btn !text-[12.5px]" onclick={() => (mode = 'cancel')}><i class="icon-ban text-[13px]"></i>{t('stock.transfer.detail.cancel')}</button>{/if}
          {#if canReceiveHere}<button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => (mode = 'receive')}><i class="icon-package-check text-[13px]"></i>{t('stock.transfer.detail.receive')}</button>{/if}
          <button type="button" class="btn !text-[12.5px]" onclick={() => (detail = null)}>{t('stock.transfer.detail.close')}</button>
        {:else if mode === 'receive'}
          <button type="button" class="btn !text-[12.5px]" onclick={() => (recv = fullRecv(d))}><i class="icon-check-check text-[13px]"></i>{t('stock.transfer.detail.receiveAll')}</button>
          <button type="button" class="btn !text-[12.5px]" onclick={() => (mode = 'view')}>{t('stock.transfer.detail.back')}</button>
          <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={dBusy} onclick={doReceive}>
            {#if dBusy}<i class="icon-loader-circle text-[14px] tr-spin"></i>{:else}<i class="icon-package-check text-[13px]"></i>{/if}{dBusy ? t('stock.transfer.detail.receiving') : t('stock.transfer.detail.receiveSave')}
          </button>
        {:else}
          <button type="button" class="btn !text-[12.5px]" onclick={() => (mode = 'view')}>{t('stock.transfer.detail.back')}</button>
          <button type="button" class="btn btn-primary !text-[12.5px] disabled:opacity-60" disabled={dBusy || reason.trim().length < 3} onclick={doCancel}>
            {#if dBusy}<i class="icon-loader-circle text-[14px] tr-spin"></i>{:else}<i class="icon-ban text-[13px]"></i>{/if}{dBusy ? t('stock.transfer.detail.cancelling') : t('stock.transfer.detail.cancelConfirm')}
          </button>
        {/if}
      </div>
    </div>
  </Modal>
{/if}

<style>
  /* ---- Hero ---- */
  .tr-hero {
    background: linear-gradient(125deg, #1d4ed8 0%, #4338ca 55%, #6d28d9 100%);
    box-shadow: 0 10px 30px -12px rgb(67 56 202 / 0.55);
  }
  .tr-hero-icon { background: rgb(255 255 255 / 0.16); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.22); }
  .tr-glass { background: rgb(255 255 255 / 0.12); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.18); backdrop-filter: blur(6px); }
  .tr-orb { position: absolute; border-radius: 9999px; background: radial-gradient(circle, rgb(255 255 255 / 0.18), transparent 70%); pointer-events: none; }
  .tr-orb-1 { width: 260px; height: 260px; top: -90px; inset-inline-end: -60px; animation: tr-drift 9s ease-in-out infinite; }
  .tr-orb-2 { width: 180px; height: 180px; bottom: -80px; inset-inline-start: 30%; animation: tr-drift 11s ease-in-out infinite reverse; }

  /* Adegan truk: dua cabang + jalur bergaris yang bergerak + truk yang bolak-balik */
  .tr-scene { display: flex; align-items: center; gap: 0.6rem; }
  .tr-node {
    flex: none; display: grid; place-items: center; width: 3.1rem; height: 3.1rem; border-radius: 0.9rem;
    background: rgb(255 255 255 / 0.16); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.25);
  }
  .tr-track {
    position: relative; flex: 1; height: 2.4rem;
    background-image: linear-gradient(90deg, rgb(255 255 255 / 0.5) 50%, transparent 0);
    background-size: 14px 2px; background-repeat: repeat-x; background-position: 0 50%;
    animation: tr-dash 1.2s linear infinite;
  }
  .tr-truck {
    position: absolute; top: 50%; left: 0; width: 2rem; height: 2rem; margin-top: -1rem; border-radius: 0.6rem;
    display: grid; place-items: center; color: #1e3a8a; background: #fff;
    box-shadow: 0 6px 14px -4px rgb(0 0 0 / 0.4); animation: tr-drive 3.6s cubic-bezier(0.45, 0, 0.25, 1) infinite;
  }
  .tr-pkg {
    position: absolute; top: 50%; margin-top: -1.9rem; display: grid; place-items: center; width: 1.2rem; height: 1.2rem;
    border-radius: 0.35rem; background: rgb(251 191 36); color: #78350f; opacity: 0; animation: tr-pop 3.6s ease-in-out infinite;
  }
  .tr-pkg-1 { left: 28%; }
  .tr-pkg-2 { left: 62%; animation-delay: 0.5s; }

  @keyframes tr-drive { 0% { left: 0; } 45%, 55% { left: calc(100% - 2rem); } 100% { left: 0; } }
  @keyframes tr-dash { to { background-position: -14px 50%; } }
  @keyframes tr-pop { 0%, 100% { opacity: 0; transform: translateY(6px) scale(0.6); } 30%, 70% { opacity: 1; transform: translateY(0) scale(1); } }
  @keyframes tr-drift { 0%, 100% { transform: translate(0, 0); } 50% { transform: translate(-14px, 10px); } }
  @keyframes tr-ping { 0% { box-shadow: 0 0 0 0 currentColor; } 80%, 100% { box-shadow: 0 0 0 7px transparent; } }
  @keyframes tr-float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-5px); } }
  @keyframes tr-spin { to { transform: rotate(360deg); } }
  @keyframes tr-shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
  @keyframes tr-nudge { 0%, 100% { transform: translateX(0); } 50% { transform: translateX(4px); } }

  .tr-ping { animation: tr-ping 1.6s ease-out infinite; color: rgb(251 191 36 / 0.7); }
  .tr-float { animation: tr-float 3.2s ease-in-out infinite; }
  .tr-spin { animation: tr-spin 0.9s linear infinite; }
  .tr-arrow { color: var(--color-primary-600); animation: tr-nudge 1.6s ease-in-out infinite; }
  .tr-dot { width: 6px; height: 6px; border-radius: 9999px; background: currentColor; animation: tr-ping 1.6s ease-out infinite; }

  /* ---- Ubin ikon & chip bucket ---- */
  .tr-tile-primary { background: color-mix(in srgb, var(--color-primary-600) 14%, transparent); color: var(--color-primary-600); }
  .tr-tile-success { background: color-mix(in srgb, #16a34a 15%, transparent); color: #16a34a; }
  .tr-tile-muted { background: var(--surface-sunken); color: var(--text-tertiary); box-shadow: inset 0 0 0 1px var(--border-subtle); }
  .tr-chip { display: inline-flex; align-items: center; gap: 0.3rem; padding: 0.08rem 0.5rem; border-radius: 9999px; font-size: 11px; font-weight: 600; line-height: 1.5; }
  .tr-b-display { background: color-mix(in srgb, #2563eb 14%, transparent); color: #2563eb; }
  .tr-b-warehouse { background: color-mix(in srgb, #d97706 15%, transparent); color: #b45309; }
  .tr-b-returns { background: color-mix(in srgb, #dc2626 13%, transparent); color: #dc2626; }

  /* ---- Form ---- */
  .tr-step { display: flex; align-items: center; gap: 0.6rem; font-size: 11.5px; font-weight: 700; letter-spacing: 0.05em; text-transform: uppercase; color: var(--text-secondary, inherit); }
  .tr-step-no {
    display: grid; place-items: center; width: 1.45rem; height: 1.45rem; border-radius: 9999px; font-size: 11px;
    color: #fff; background: linear-gradient(135deg, var(--color-primary-600), #6d28d9);
  }
  .tr-line { transition: border-color 0.15s, box-shadow 0.15s; }
  .tr-line:focus-within { box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary-600) 16%, transparent); }
  .tr-bar { height: 4px; border-radius: 9999px; background: var(--surface-sunken); overflow: hidden; }
  .tr-bar > span { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, #3b82f6, #6366f1); transition: width 0.3s ease; }
  .tr-bar > span.is-over { background: #dc2626; }
  .tr-del:hover { color: var(--color-danger-600); }
  .tr-send-icon { transition: transform 0.25s ease; }
  .tr-send:not(:disabled):hover .tr-send-icon { transform: translate(3px, -3px) rotate(8deg); }

  /* ---- Tab Keluar/Masuk dengan indikator geser ---- */
  .tr-tabs { position: relative; display: grid; grid-template-columns: 1fr 1fr; padding: 3px; border-radius: 0.65rem; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--border-subtle); }
  .tr-tabs-pill {
    position: absolute; inset-block: 3px; inset-inline-start: 3px; width: calc(50% - 3px); border-radius: 0.5rem;
    background: var(--color-primary-600); box-shadow: 0 4px 10px -4px color-mix(in srgb, var(--color-primary-600) 70%, transparent);
    transition: transform 0.28s cubic-bezier(0.3, 0.9, 0.3, 1);
  }
  .tr-tabs[data-dir='in'] .tr-tabs-pill { transform: translateX(100%); }
  :global([dir='rtl']) .tr-tabs[data-dir='in'] .tr-tabs-pill { transform: translateX(-100%); }
  .tr-tab { position: relative; z-index: 1; display: inline-flex; align-items: center; justify-content: center; gap: 0.4rem; padding: 0.35rem 0.9rem; font-size: 12.5px; font-weight: 600; color: var(--text-tertiary); transition: color 0.2s; white-space: nowrap; }
  .tr-tab[aria-selected='true'] { color: #fff; }
  .tr-count { display: inline-grid; place-items: center; min-width: 1.15rem; height: 1.15rem; padding: 0 0.3rem; border-radius: 9999px; font-size: 10.5px; font-weight: 700; background: #f59e0b; color: #fff; }

  /* ---- Tabel ---- */
  .tr-row { transition: background-color 0.15s; }
  .tr-row:hover { background: color-mix(in srgb, var(--color-primary-600) 5%, transparent); }
  .tr-skel { background: linear-gradient(90deg, var(--surface-sunken) 25%, color-mix(in srgb, var(--surface-sunken) 55%, var(--border-subtle)) 50%, var(--surface-sunken) 75%); background-size: 200% 100%; animation: tr-shimmer 1.4s linear infinite; }

  /* ---- Linimasa di rincian ---- */
  .tr-timeline { position: relative; display: grid; grid-template-columns: repeat(3, 1fr); text-align: center; padding-top: 0.2rem; }
  .tr-timeline::before, .tr-timeline::after { content: ''; position: absolute; top: 1.15rem; inset-inline-start: 16.66%; height: 3px; border-radius: 9999px; }
  .tr-timeline::before { width: 66.66%; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--border-subtle); }
  .tr-timeline::after { width: calc(66.66% * var(--fill) / 100); background: linear-gradient(90deg, #3b82f6, #6366f1); transition: width 0.7s cubic-bezier(0.3, 0.9, 0.3, 1); }
  .tr-timeline li { position: relative; z-index: 1; display: flex; flex-direction: column; align-items: center; gap: 0.2rem; color: var(--text-tertiary); }
  .tr-timeline b { font-size: 12px; font-weight: 600; }
  .tr-timeline small { font-size: 11px; }
  .tr-tl-dot {
    display: grid; place-items: center; width: 2.1rem; height: 2.1rem; border-radius: 9999px; margin-bottom: 0.15rem;
    background: var(--surface-base); box-shadow: inset 0 0 0 2px var(--border-default); transition: all 0.4s ease;
  }
  .tr-timeline li.is-done { color: var(--text-primary, inherit); }
  .tr-timeline li.is-done .tr-tl-dot { background: #3b82f6; color: #fff; box-shadow: none; }
  .tr-timeline li.is-now .tr-tl-dot { background: #f59e0b; color: #fff; box-shadow: 0 0 0 5px color-mix(in srgb, #f59e0b 25%, transparent); animation: tr-float 1.8s ease-in-out infinite; }
  .tr-timeline li.is-ok .tr-tl-dot { background: #16a34a; }
  .tr-timeline li.is-bad .tr-tl-dot { background: #6b7280; }

  @media (prefers-reduced-motion: reduce) {
    .tr-orb, .tr-track, .tr-truck, .tr-pkg, .tr-ping, .tr-float, .tr-spin, .tr-arrow, .tr-dot, .tr-skel, .tr-timeline li.is-now .tr-tl-dot { animation: none !important; }
    .tr-pkg { opacity: 1; }
    .tr-tabs-pill, .tr-bar > span, .tr-timeline::after, .tr-tl-dot, .tr-send-icon { transition: none !important; }
  }
</style>
