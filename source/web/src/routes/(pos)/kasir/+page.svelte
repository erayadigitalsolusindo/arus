<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { session, switchOutlet } from '#lib/auth/session.svelte.ts';
  import { t, formatCurrency, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { items, type Row, type BarcodeMatch } from '#lib/items/api.ts';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { toCents, toMilli, centsToNumber, lineTotal } from '#lib/pos/money.ts';
  import { cartStorageKey, loadCart, saveCart } from '#lib/pos/cart-store.ts';
  import AuthImage from '#lib/components/AuthImage.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import PayModal from '#lib/components/PayModal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { fitText } from '#lib/fitText.ts';
  import PriceOverrideModal from '#lib/components/PriceOverrideModal.svelte';
  import type { Approver } from '#lib/approval/api.ts';
  import { sales, type Quote, type SaleInput } from '#lib/sales/api.ts';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import { initials } from '#lib/auth/initials.ts';

  // Satu baris keranjang. `price` = harga per satuan baris (string desimal dari API); `qty` = string yang diketik pengguna.
  type Line = { key: string; id: string; override: string | null; disc: string | null; discTotal: boolean; unitId: string | null; sku: string; name: string; unit: string; price: string; qty: string; goods: boolean; imageId: string | null };

  const PAGE = 24;
  const money = (c: bigint) => formatCurrency(centsToNumber(c), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  // ---------- Katalog ----------
  let q = $state('');
  let rows = $state<Row[]>([]);
  let total = $state(0);
  let loading = $state(false);
  let loadError = $state('');
  let searchEl = $state<HTMLInputElement>();
  let seq = 0; // hanya respons terbaru yang dipakai (pencarian cepat berurutan)

  async function load(reset: boolean) {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await items.list({ q: q.trim() || undefined, active: true, limit: PAGE, offset: reset ? 0 : rows.length });
      if (mine !== seq) return;
      rows = reset ? res.data : [...rows, ...res.data];
      total = res.total;
    } catch (e) {
      if (mine === seq) loadError = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  let debounce: ReturnType<typeof setTimeout>;
  function onSearchInput() {
    clearTimeout(debounce);
    debounce = setTimeout(() => void load(true), 300);
  }

  onMount(() => {
    void load(true);
    void refreshOutlets();
    searchEl?.focus();
    if (restored) flash(t('pos.restored', { count: restored.lines.length }));
    return () => clearTimeout(debounce);
  });

  // ---------- Keranjang ----------
  // Keranjang disimpan di browser (per tenant+outlet+kasir) agar selamat dari tab tertutup / mati lampu.
  const storeKey = $derived(session.tenant && session.outlet && session.user ? cartStorageKey(session.tenant.id, session.outlet.id, session.user.id) : '');
  const restored = session.tenant && session.outlet && session.user ? loadCart(cartStorageKey(session.tenant.id, session.outlet.id, session.user.id)) : null;
  let cart = $state<Line[]>(restored ? restored.lines.map((l) => ({ ...l, override: null, disc: null, discTotal: false })) : []);

  $effect(() => {
    const snapshot = { lines: cart.map(({ override: _o, disc: _d, discTotal: _t, ...l }) => l), otherCost, taxOn, note };
    if (storeKey) saveCart(storeKey, snapshot);
  });

  function add(l: Omit<Line, 'qty' | 'key' | 'override' | 'disc' | 'discTotal'> & { key?: string }) {
    const key = l.key ?? `${l.id}:${l.unit}`;
    const existing = cart.find((x) => x.key === key);
    if (existing) {
      existing.qty = bump(existing.qty, 1);
      // Barang yang baru ditambah selalu naik ke paling atas supaya kasir langsung melihatnya.
      cart = [existing, ...cart.filter((x) => x !== existing)];
    } else {
      cart.unshift({ ...l, key, qty: '1', override: null, disc: null, discTotal: false });
    }
    highlight(key);
  }

  let cartEl = $state<HTMLElement>();
  let hotKey = $state('');
  let hotTimer: ReturnType<typeof setTimeout>;
  function highlight(key: string) {
    hotKey = key;
    cartEl?.scrollTo({ top: 0, behavior: 'smooth' });
    clearTimeout(hotTimer);
    hotTimer = setTimeout(() => (hotKey = ''), 1400);
  }

  function addRow(r: Row) {
    add({ id: r.id, unitId: null, sku: r.sku, name: r.name, unit: r.unit, price: r.price, goods: r.kind === 'goods', imageId: r.main_image_id });
  }

  /** Tambah/kurangi qty memakai per-mil agar tidak ada galat float (mis. 0.1 + 0.2). */
  function bump(qty: string, by: number): string {
    const next = toMilli(qty) + BigInt(by * 1000);
    return next > 0n ? fmtMilli(next) : '1';
  }

  function fmtMilli(m: bigint): string {
    const i = m / 1000n;
    const f = (m % 1000n).toString().padStart(3, '0').replace(/0+$/, '');
    return f ? `${i}.${f}` : `${i}`;
  }

  const remove = (key: string) => {
    cart = cart.filter((l) => l.key !== key);
    if (!needsApproval()) approval = null;
  };

  function normalizeQty(l: Line) {
    const m = toMilli(l.qty);
    l.qty = m > 0n ? fmtMilli(m) : '1';
  }

  // Harga, grosir, satuan, dan pajak dihitung SERVER (POST /sales/quote); layar hanya menampilkannya.
  let otherCost = $state(restored?.otherCost ?? '');
  const outletTax = $derived(accessibleOutlets.items.find((o) => o.id === session.outlet?.id));
  let taxOn = $state(restored?.taxOn ?? false);
  const calcTax = () => (taxOn = true);
  const cancelTax = () => (taxOn = false);

  let note = $state(restored?.note ?? '');
  const cartQty = $derived(cart.length);

  // forPay: sertakan persetujuan (PIN) untuk dikirim saat simpan nota; quote TIDAK pernah membawa PIN.
  const buildSale = (withNote = true, forPay = false): Omit<SaleInput, 'payments'> => ({
    lines: cart.map((l) => ({ item_id: l.id, ...(l.unitId ? { unit_id: l.unitId } : {}), qty: fmtMilli(toMilli(l.qty)), ...(l.override ? { unit_price: l.override } : {}), ...(l.disc ? { discount: discountOf(l) } : {}) })),
    ...(forPay && approval && needsApproval() ? { approval: { user_id: approval.id, pin: approval.pin } } : {}),
    ...(otherCost.trim() ? { other_cost: otherCost.trim().replace(',', '.') } : {}),
    apply_tax: taxOn,
    ...(withNote && note.trim() ? { note: note.trim() } : {})
  });

  // Persetujuan ubah harga: penyetuju + PIN hanya di memori halaman ini; dibuang saat nota selesai/keranjang dikosongkan.
  let approval = $state<{ id: string; name: string; pin: string } | null>(null);
  let editing = $state<number | null>(null); // indeks baris yang sedang diubah harganya
  // Potongan manual disimpan per satuan (Rp) agar ikut menyesuaikan saat qty diubah; server menerima total potongan baris.
  function discountOf(l: Line): string {
    const c = l.discTotal ? toCents(l.disc ?? '0') : lineTotal(toCents(l.disc ?? '0'), toMilli(l.qty));
    return `${c / 100n}.${String(c % 100n).padStart(2, '0')}`;
  }
  const needsApproval = () => cart.some((l) => l.override || l.disc);
  function applyChange(i: number, patch: { override?: string | null; disc?: string | null; discTotal?: boolean }, ap: Approver, pin: string) {
    if (patch.override !== undefined) cart[i].override = patch.override;
    if (patch.disc !== undefined) cart[i].disc = patch.disc;
    if (patch.discTotal !== undefined) cart[i].discTotal = patch.discTotal;
    approval = { id: ap.id, name: ap.name, pin };
    editing = null;
  }
  function resetOverride(i: number) {
    cart[i].override = null;
    if (!needsApproval()) approval = null;
  }
  function resetDiscount(i: number) {
    cart[i].disc = null;
    cart[i].discTotal = false;
    if (!needsApproval()) approval = null;
  }

  let quote = $state<Quote | null>(null);
  let quoteFor = $state(''); // isi keranjang yang dihitung quote; beda dengan keranjang sekarang = quote basi
  let quoteError = $state('');
  let quoting = $state(false);
  let qseq = 0;
  const cartKey = $derived(JSON.stringify(buildSale(false)));
  const fresh = $derived(quote !== null && quoteFor === cartKey && !quoteError);
  const grand = $derived(quote ? toCents(quote.total) : 0n);
  const taxStore = $derived(quote ? toCents(quote.tax_store) : 0n);
  const taxGov = $derived(quote ? toCents(quote.tax_gov) : 0n);
  const issueTextOf = (r: Quote, i: number) => {
    const l = r.lines[i];
    if (l.issue === 'BELOW_COST') return t('pos.issue.belowCost');
    return toMilli(l.available ?? '0') <= 0n ? t('pos.issue.stockNone') : t('pos.issue.stock', { available: l.available ?? '0' });
  };
  const issueOf = (i: number) => (fresh ? (quote?.lines[i]?.issue ?? null) : null);
  const issueText = (i: number) => {
    const l = quote?.lines[i];
    if (!l?.issue) return '';
    if (l.issue === 'BELOW_COST') return t('pos.issue.belowCost');
    return toMilli(l.available ?? '0') <= 0n ? t('pos.issue.stockNone') : t('pos.issue.stock', { available: l.available ?? '0' });
  };
  const hasIssue = $derived(fresh && !!quote?.lines.some((l) => l.issue));
  const lineTotalOf = (i: number) => (fresh && quote?.lines[i] ? toCents(quote.lines[i].line_total) : null);

  // Keranjang kosong = transaksi baru: biaya lain, pajak, dan keterangan sisa nota sebelumnya tidak ikut terbawa.
  $effect(() => {
    if (cart.length === 0) {
      untrack(() => {
        otherCost = '';
        taxOn = false;
        note = '';
      });
    }
  });

  $effect(() => {
    const key = cartKey;
    if (!cart.length) {
      quote = null;
      quoteFor = key;
      quoteError = '';
      return;
    }
    const mine = ++qseq;
    quoting = true;
    const timer = setTimeout(async () => {
      try {
        const r = await sales.quote(JSON.parse(key));
        if (mine !== qseq) return;
        const before = quote?.lines.filter((l) => l.issue).length ?? 0;
        quote = r;
        quoteError = '';
        const bad = r.lines.findIndex((l) => l.issue);
        if (bad >= 0 && r.lines.filter((l) => l.issue).length > before) flash(`${cart[bad]?.name ?? ''}: ${issueTextOf(r, bad)}`);
      } catch (e) {
        if (mine !== qseq) return;
        quote = null;
        quoteError = errorMessage(e);
      } finally {
        if (mine === qseq) {
          quoteFor = key;
          quoting = false;
        }
      }
    }, 200);
    return () => clearTimeout(timer);
  });

  // ---------- Bayar ----------
  let paying = $state(false);
  function saleDone() {
    paying = false;
    cart = [];
    approval = null;
    note = '';
    otherCost = '';
    taxOn = false;
    void load(true); // stok berubah
    searchEl?.focus();
  }

  // ---------- Pindai / Enter ----------
  let picks = $state<BarcodeMatch[] | null>(null);
  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout>;
  function flash(msg: string) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), 3500);
  }

  function addMatch(m: BarcodeMatch) {
    add({ id: m.id, unitId: m.matched === 'unit' ? m.unit_id : null, sku: m.sku, name: m.name, unit: m.unit, price: m.price, goods: true, imageId: null });
  }

  async function onEnter() {
    const code = q.trim();
    if (!code) return;
    clearTimeout(debounce);
    try {
      const matches = (await items.byBarcode(code)).filter((m) => m.active);
      if (matches.length === 1) {
        addMatch(matches[0]);
        q = '';
        void load(true);
      } else if (matches.length > 1) {
        picks = matches;
      } else {
        // Bukan barcode: bila pencarian nama menyisakan tepat satu barang, langsung tambahkan.
        await load(true);
        if (rows.length === 1) {
          addRow(rows[0]);
          q = '';
          void load(true);
        } else if (rows.length === 0) flash(t('pos.notFound', { code }));
      }
    } catch (e) {
      flash(errorMessage(e));
    }
  }

  function pickMatch(m: BarcodeMatch) {
    addMatch(m);
    picks = null;
    q = '';
    void load(true);
    searchEl?.focus();
  }

  // ---------- Header / pemilih outlet / tema ----------
  let now = $state(new Date());
  $effect(() => {
    const id = setInterval(() => (now = new Date()), 1000);
    return () => clearInterval(id);
  });

  let outletBusy = $state(false);
  async function changeOutlet(e: Event & { currentTarget: HTMLSelectElement }) {
    const el = e.currentTarget;
    const id = el.value;
    if (!id || id === session.outlet?.id) return;
    outletBusy = true;
    try {
      await switchOutlet(id);
      cart = []; // harga & stok berbeda per cabang: keranjang tidak dibawa pindah
      approval = null;
      cancelTax();
      await load(true);
    } catch (err) {
      el.value = session.outlet?.id ?? '';
      flash(errorMessage(err));
    } finally {
      outletBusy = false;
    }
  }
  const outletOptions = $derived(
    accessibleOutlets.items.length ? accessibleOutlets.items : session.outlet ? [{ id: session.outlet.id, name: session.outlet.name }] : []
  );

  let theme = $state<'light' | 'dark'>(document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light');
  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem('theme', theme);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }

  const SIDE_KEY = 'pos.sideCollapsed';
  let sideCollapsed = $state(true); // awalnya tertutup; pilihan pengguna (buka) diingat
  try {
    sideCollapsed = localStorage.getItem(SIDE_KEY) !== '0';
  } catch {
    /* penyimpanan diblokir: abaikan */
  }
  function toggleSide() {
    sideCollapsed = !sideCollapsed;
    try {
      localStorage.setItem(SIDE_KEY, sideCollapsed ? '1' : '0');
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }

  function toggleFullscreen() {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void document.documentElement.requestFullscreen?.();
  }

  const shortcuts = [
    { abbr: 'DG', title: 'pos.shortcuts.wallet', desc: 'pos.shortcuts.walletDesc' },
    { abbr: 'DA', title: 'pos.shortcuts.members', desc: 'pos.shortcuts.membersDesc' },
    { abbr: 'DS', title: 'pos.shortcuts.salespeople', desc: 'pos.shortcuts.salespeopleDesc' },
    { abbr: 'PH', title: 'pos.shortcuts.today', desc: 'pos.shortcuts.todayDesc' }
  ] as const;

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'F2') {
      e.preventDefault();
      searchEl?.focus();
      searchEl?.select();
    }
  }

  // Item pertama yang ditambahkan tidak perlu memicu muat ulang katalog; efek ini hanya menjaga fokus setelah modal.
  $effect(() => {
    if (!picks) untrack(() => searchEl?.focus());
  });
</script>

<svelte:head><title>{t('pos.docTitle')}</title></svelte:head>
<svelte:window {onkeydown} />

<div class="flex flex-col min-h-dvh lg:h-dvh lg:overflow-hidden bg-[var(--surface-sunken)] text-[var(--text-primary)]">
  <!-- Bilah atas -->
  <header class="flex items-center gap-2 h-12 px-3 shrink-0 bg-[var(--surface-card)] border-b border-[var(--border-subtle)]">
    <a href="/dashboard" class="header-icon-btn" aria-label={t('pos.back')} title={t('pos.back')}><i class="icon-arrow-left text-[18px]"></i></a>
    <span class="font-display font-extrabold tracking-tight text-[15px] uppercase text-[var(--color-primary-600)] truncate">{session.tenant?.name}</span>
    <span class="grow"></span>
    <LanguageSwitcher />
    <button type="button" class="header-icon-btn hidden sm:inline-flex" aria-label={t('pos.fullscreen')} title={t('pos.fullscreen')} onclick={toggleFullscreen}>
      <i class="icon-maximize text-[16px]"></i>
    </button>
    <button type="button" class="header-icon-btn" aria-label={t('pos.toggleTheme')} title={t('pos.toggleTheme')} onclick={toggleTheme}>
      <i class="{theme === 'dark' ? 'icon-sun' : 'icon-moon'} text-[16px]"></i>
    </button>
    <span class="flex items-center gap-2 ps-1">
      <span class="grid place-items-center size-8 rounded-full bg-[var(--color-primary-600)] text-white text-[12px] font-bold">{initials(session.user?.name)}</span>
      <span class="hidden sm:block text-[12.5px] font-semibold max-w-40 truncate">{session.user?.name}</span>
    </span>
  </header>

  <div class="flex-1 min-h-0 flex flex-col lg:grid {sideCollapsed ? 'lg:grid-cols-[48px_minmax(0,1fr)_400px]' : 'lg:grid-cols-[270px_minmax(0,1fr)_400px]'}">
    <!-- Kolom kiri: informasi outlet (bisa diciutkan agar katalog lebih lebar) -->
    {#if sideCollapsed}
      <aside class="hidden lg:flex flex-col items-center gap-3 py-3 min-h-0 bg-[var(--surface-card)] border-e border-[var(--border-subtle)]">
        <button type="button" class="header-icon-btn" aria-label={t('pos.expandInfo')} title={t('pos.expandInfo')} onclick={toggleSide}>
          <i class="icon-chevrons-right text-[18px]"></i>
        </button>
        <span class="text-[11px] font-bold uppercase tracking-wide text-[var(--text-secondary)] [writing-mode:vertical-rl] rotate-180">{session.outlet?.code}</span>
      </aside>
    {:else}
    <aside class="hidden lg:flex flex-col gap-3 p-3 min-h-0 overflow-y-auto bg-[var(--surface-card)] border-e border-[var(--border-subtle)]">
      <div class="flex items-center gap-1">
        <div class="grow text-center text-[11px] font-bold uppercase tracking-wide">{t('pos.info')} : {session.outlet?.code}</div>
        <button type="button" class="header-icon-btn" aria-label={t('pos.collapseInfo')} title={t('pos.collapseInfo')} onclick={toggleSide}>
          <i class="icon-chevrons-left text-[18px]"></i>
        </button>
      </div>
      <select
        class="w-full rounded-md px-2.5 py-2 text-[12.5px] border border-[var(--border-default)] bg-[var(--surface-base)] disabled:opacity-60"
        aria-label={t('pos.switchOutlet')}
        disabled={outletBusy || outletOptions.length < 2}
        value={session.outlet?.id}
        onchange={changeOutlet}
      >
        {#each outletOptions as o (o.id)}<option value={o.id}>{o.name}</option>{/each}
      </select>

      <section class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-2">
        <div class="text-[15px] font-bold text-[var(--color-primary-600)]">{t('pos.newReceipt')}</div>
        <dl class="text-[12px] space-y-0.5 text-[var(--text-secondary)]">
          <div>{formatDateTime(now, { dateStyle: 'short', timeStyle: 'medium' })}</div>
          <div>{t('pos.cashier')} : {session.user?.name}</div>
          <div>{t('pos.salesperson')} : {t('pos.salespersonNone')}</div>
        </dl>
        <div class="flex gap-2 pt-1">
          <button type="button" class="btn btn-primary btn-sm" disabled title={t('pos.soon')}>{t('pos.pendingReceipt')}</button>
          <button type="button" class="btn btn-sm" disabled title={t('pos.soon')}>{t('pos.cashDrawer')}</button>
        </div>
      </section>

      <ul class="rounded-lg border border-[var(--border-subtle)] divide-y divide-[var(--border-subtle)] overflow-hidden">
        {#each shortcuts as s (s.abbr)}
          <li class="flex items-start gap-3 p-2.5 opacity-70" title={t('pos.soon')}>
            <span class="grid place-items-center size-8 shrink-0 rounded-full text-[11px] font-bold bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-700)]">{s.abbr}</span>
            <span class="min-w-0">
              <span class="block text-[12px] font-semibold">{t(s.title)}</span>
              <span class="block text-[10.5px] leading-snug text-[var(--text-tertiary)]">{t(s.desc)}</span>
            </span>
          </li>
        {/each}
      </ul>

      <span class="grow"></span>
      <button type="button" class="btn btn-sm w-full" disabled title={t('pos.soon')}>{t('pos.orderStatus')}</button>
    </aside>
    {/if}

    <!-- Kolom tengah: pencarian + katalog -->
    <main class="flex flex-col min-h-0 min-w-0 max-h-[70dvh] lg:max-h-none">
      <div class="flex items-center gap-2 p-2.5 shrink-0 bg-[var(--surface-card)] border-b border-[var(--border-subtle)]">
        <span class="inline-flex items-center gap-1.5 h-10 px-3 rounded-md border border-[var(--border-default)] text-[13px] font-semibold text-[var(--color-primary-600)] bg-[var(--surface-base)]" title={t('pos.cartCount', { count: cartQty })}>
          <i class="icon-shopping-cart text-[15px]"></i>{cartQty}
        </span>
        <label class="relative grow">
          <span class="sr-only">{t('pos.search')}</span>
          <i class="icon-search absolute start-3 top-1/2 -translate-y-1/2 text-[14px] text-[var(--color-primary-600)] pointer-events-none"></i>
          <input
            bind:this={searchEl}
            bind:value={q}
            oninput={onSearchInput}
            onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), void onEnter())}
            type="search"
            enterkeyhint="search"
            autocomplete="off"
            placeholder={t('pos.search')}
            title={t('pos.searchHint')}
            class="w-full h-10 ps-9 pe-3 rounded-md text-[13px] border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
          />
        </label>
        <button type="button" class="header-icon-btn" aria-label={t('pos.refresh')} title={t('pos.refresh')} onclick={() => load(true)}>
          <i class="icon-refresh-cw text-[16px] {loading ? 'animate-spin' : ''}"></i>
        </button>
        <button type="button" class="header-icon-btn" aria-label={t('pos.scan')} title={t('pos.scan')} onclick={() => searchEl?.focus()}>
          <i class="icon-scan-barcode text-[18px]"></i>
        </button>
      </div>

      <div class="flex-1 min-h-0 overflow-y-auto p-3">
        {#if loadError}
          <p role="alert" class="text-[13px] text-[var(--color-danger-600)]">{t('pos.loadFailed')} {loadError}</p>
        {:else if !loading && rows.length === 0}
          <p class="text-center text-[13px] text-[var(--text-tertiary)] py-10">{t('pos.noItems')}</p>
        {/if}

        <div class="grid gap-3 grid-cols-[repeat(auto-fill,minmax(250px,1fr))]">
          {#each rows as r (r.id)}
            <div class="flex flex-col rounded-md overflow-hidden bg-[var(--surface-card)] border border-[var(--border-subtle)] shadow-[var(--shadow-sm)]">
              <div class="flex min-h-[150px]">
                <div class="w-[38%] shrink-0 bg-[var(--surface-sunken)]">
                  {#if r.main_image_id}
                    <AuthImage itemId={r.id} imageId={r.main_image_id} alt={r.name} class="size-full object-cover" />
                  {:else}
                    <span class="grid place-items-center size-full text-[var(--text-tertiary)]"><i class="icon-image text-[26px]"></i></span>
                  {/if}
                </div>
                <div class="flex flex-col min-w-0 grow p-2.5 text-[12px]">
                  <h3 class="font-bold uppercase leading-tight text-[13px] line-clamp-3">{r.name}</h3>
                  <p class="mt-1.5 text-[11px] leading-snug text-[var(--text-secondary)] break-all"><b>{t('pos.itemCode')} :</b> {r.sku}</p>
                  <p class="text-[11px] font-semibold {r.kind === 'goods' && Number(r.stock.display) <= 0 ? 'text-[var(--color-danger-600)]' : 'text-[var(--text-secondary)]'}">
                    {t('pos.stock')} : {r.kind === 'goods' ? r.stock.display : t('pos.service')}
                  </p>
                </div>
              </div>
              <button type="button" class="flex items-center justify-center gap-1.5 h-8 text-[11.5px] font-semibold text-white bg-[var(--color-primary-600)] hover:bg-[var(--color-primary-700)] transition-colors" onclick={() => addRow(r)}>
                <i class="icon-circle-plus text-[13px]"></i>{t('pos.addToCart')}
              </button>
              <div class="py-1.5 text-center font-display font-extrabold text-[17px] tabular-nums bg-[var(--surface-sunken)]">
                {t('pos.price')}: {money(toCents(r.price))}
              </div>
            </div>
          {/each}
        </div>

        {#if rows.length < total}
          <div class="text-center py-4">
            <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(false)}>{loading ? t('pos.loading') : t('pos.loadMore')}</button>
          </div>
        {/if}
      </div>

      <div class="shrink-0 p-2.5 bg-[var(--surface-card)] border-t border-[var(--border-subtle)]">
        <label class="block">
          <span class="sr-only">{t('pos.noteLabel')}</span>
          <input
            bind:value={note}
            maxlength="500"
            placeholder={t('pos.note')}
            class="w-full h-12 px-4 rounded-md text-[16px] border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]"
          />
        </label>
      </div>
    </main>

    <!-- Kolom kanan: pelanggan, keranjang, total, bayar -->
    <section class="flex flex-col min-h-0 bg-[var(--surface-card)] border-t lg:border-t-0 lg:border-s border-[var(--border-subtle)]">
      <div class="flex items-center gap-2 p-3 shrink-0 border-b border-[var(--border-subtle)]">
        <button type="button" class="header-icon-btn" disabled aria-label={t('pos.pickCustomer')} title={t('pos.soon')}><i class="icon-user text-[16px]"></i></button>
        <button type="button" class="header-icon-btn" disabled aria-label={t('pos.pendingReceipt')} title={t('pos.soon')}><i class="icon-list text-[16px]"></i></button>
        <div class="min-w-0 ps-1 leading-tight">
          <div class="text-[11.5px] font-semibold">{t('pos.customer')}</div>
          <div class="text-[11px] text-[var(--text-tertiary)] truncate">{t('pos.customerGeneral')} [1001]</div>
        </div>
        {#if cart.length}
          <button type="button" class="ms-auto header-icon-btn" aria-label={t('pos.clearCart')} title={t('pos.clearCart')} onclick={() => ((cart = []), (approval = null))}>
            <i class="icon-trash-2 text-[15px]"></i>
          </button>
        {/if}
      </div>

      <!-- Total besar -->
      <div class="px-4 py-3 text-end shrink-0" aria-live="polite">
        <span use:fitText={{ max: 44, min: 16, text: money(grand) }} class="inline-block whitespace-nowrap font-mono font-extrabold tabular-nums text-[44px] leading-none tracking-tight text-[var(--color-danger-600)] [text-shadow:0_0_1px_currentColor]">{money(grand)}</span>
        {#if quoteError}<p role="alert" class="mt-1 text-[11.5px] font-normal text-[var(--color-danger-600)] text-start">{quoteError}</p>{/if}
      </div>

      <!-- Baris keranjang -->
      <div bind:this={cartEl} class="flex-1 min-h-[140px] overflow-y-auto px-3 pb-2 space-y-2">
        {#if cart.length === 0}
          <div class="flex flex-col items-center justify-center gap-4 py-8 px-4 text-center">
            <div class="empty-scan relative flex h-24 w-24 items-center justify-center rounded-2xl border-2 border-dashed border-[var(--color-primary-500)] text-[var(--color-primary-500)]">
              <svg viewBox="0 0 24 24" class="h-12 w-12" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <circle cx="9" cy="20" r="1.4" /><circle cx="18" cy="20" r="1.4" />
                <path d="M2.5 3h2.8l2.2 11.2a1.6 1.6 0 0 0 1.6 1.3h8.1a1.6 1.6 0 0 0 1.6-1.2L20.5 7H6.2" />
              </svg>
              <span class="empty-scan-line absolute left-2 right-2 h-0.5 rounded bg-[var(--color-primary-500)]"></span>
            </div>
            <p class="empty-text text-[16px] font-semibold leading-snug text-[var(--text-secondary)]">{t('pos.emptyCart')}</p>
            <span class="empty-arrow text-[var(--color-primary-500)]" aria-hidden="true">
              <svg viewBox="0 0 24 24" class="h-7 w-7" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M11 6l-6 6 6 6" /></svg>
            </span>
          </div>
        {/if}
        {#each cart as l, i (l.key)}
          <div class="rounded-md border p-2.5 transition-shadow {hotKey === l.key ? 'ring-2 ring-[var(--color-primary-500)] shadow-[var(--shadow-md)]' : ''} {issueOf(i) ? 'border-[var(--color-danger-500)] bg-[color-mix(in_oklab,var(--color-danger-500)_6%,transparent)]' : 'border-[var(--border-subtle)]'}">
            <div class="flex items-start gap-2">
              <span class="grid place-items-center size-8 shrink-0 rounded bg-[var(--surface-sunken)] text-[var(--text-tertiary)]"><i class="icon-package text-[14px]"></i></span>
              <div class="min-w-0 grow leading-tight">
                <div class="text-[12px] font-bold uppercase truncate">{l.name}</div>
                <div class="text-[11px] text-[var(--text-tertiary)]">{t('pos.unitPrice', { price: money(toCents(fresh && quote?.lines[i] ? quote.lines[i].unit_price : l.price)) })} / {l.unit}</div>
              </div>
              <button type="button" class="grid place-items-center size-7 rounded bg-[var(--surface-sunken)] text-[var(--text-secondary)]" aria-label={t('pos.override.button')} title={t('pos.override.button')} onclick={() => (editing = i)}>
                <i class="icon-pencil text-[13px]"></i>
              </button>
              <button type="button" class="grid place-items-center size-7 rounded bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)] text-[var(--color-danger-600)]" aria-label={t('pos.remove')} title={t('pos.remove')} onclick={() => remove(l.key)}>
                <i class="icon-trash-2 text-[13px]"></i>
              </button>
            </div>
            {#if l.override}
              <p class="mt-1.5 flex items-center gap-1.5 text-[11.5px] font-semibold text-[var(--color-warning-600)]">
                <i class="icon-badge-check"></i>{t('pos.override.badge')}{approval ? ` · ${t('pos.override.approvedBy', { name: approval.name })}` : ''}
                <button type="button" class="ms-auto underline font-normal" onclick={() => resetOverride(i)}>{t('pos.override.reset')}</button>
              </p>
            {/if}
            {#if l.disc}
              <p class="mt-1.5 flex items-center gap-1.5 text-[11.5px] font-semibold text-[var(--color-warning-600)]">
                <i class="icon-badge-percent"></i>{t(l.discTotal ? 'pos.discountBadgeTotal' : 'pos.discountBadge', { amount: money(toCents(l.disc)) })}{!l.override && approval ? ` · ${t('pos.override.approvedBy', { name: approval.name })}` : ''}
                <button type="button" class="ms-auto underline font-normal" onclick={() => resetDiscount(i)}>{t('pos.discountReset')}</button>
              </p>
            {/if}
            {#if issueOf(i)}<p role="alert" class="mt-1.5 text-[11.5px] font-semibold text-[var(--color-danger-600)]"><i class="icon-triangle-alert me-1"></i>{issueText(i)}</p>{/if}
            <div class="mt-2 flex items-end justify-between gap-2">
              <div class="flex items-stretch h-8 rounded overflow-hidden border border-[var(--border-default)]">
                <button type="button" class="w-8 grid place-items-center text-white bg-[var(--color-primary-600)]" aria-label={t('pos.qtyDec')} onclick={() => (l.qty = bump(l.qty, -1))}><i class="icon-minus text-[13px]"></i></button>
                <MoneyInput
                  bind:value={l.qty}
                  decimals={3}
                  pad={false}
                  onblur={() => normalizeQty(l)}
                  aria-label={t('pos.qty')}
                  class="w-16 text-center text-[13px] font-semibold tabular-nums bg-[var(--surface-base)] outline-none"
                />
                <button type="button" class="w-8 grid place-items-center text-white bg-[var(--color-primary-600)]" aria-label={t('pos.qtyInc')} disabled={issueOf(i) === 'STOCK_INSUFFICIENT'} onclick={() => (l.qty = bump(l.qty, 1))}><i class="icon-plus text-[13px]"></i></button>
              </div>
              <div class="text-end text-[11px] leading-tight">
                <div class="font-semibold text-[var(--text-secondary)]">{t('pos.subtotal')} : <span class="tabular-nums">{lineTotalOf(i) !== null ? money(lineTotalOf(i) ?? 0n) : '…'}</span></div>
                <div class="text-[var(--text-tertiary)]">{t('pos.discount')} : {money(fresh && quote?.lines[i]?.discount ? toCents(quote.lines[i].discount) : 0n)}</div>
              </div>
            </div>
          </div>
        {/each}
      </div>

      <!-- Biaya & pajak -->
      <div class="shrink-0 px-3 pt-2 space-y-1.5 border-t border-[var(--border-subtle)] text-[12px]">
        <label class="flex items-center gap-2">
          <span class="w-28 shrink-0">{t('pos.otherCosts')}</span>
          <MoneyInput bind:value={otherCost} placeholder="0" class="grow h-8 px-2 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
        </label>
        <label class="flex items-center gap-2">
          <span class="w-28 shrink-0">{t('pos.storeTax')}</span>
          <output class="grow h-8 px-2 flex items-center justify-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-sunken)]" title={t('pos.taxPreview')}>{money(taxStore)}</output>
        </label>
        <label class="flex items-center gap-2">
          <span class="w-28 shrink-0">{t('pos.govTax')}</span>
          <output class="grow h-8 px-2 flex items-center justify-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-sunken)]" title={t('pos.taxPreview')}>{money(taxGov)}</output>
        </label>
        <div class="grid grid-cols-2 gap-2 pt-1">
          <button type="button" class="h-8 rounded text-[11.5px] font-bold text-white bg-[var(--color-danger-600)] disabled:opacity-50" disabled={!cart.length || !outletTax} onclick={calcTax}>
            <i class="icon-calculator me-1 text-[12px]"></i>{t('pos.calcTax')}
          </button>
          <button type="button" class="h-8 rounded text-[11.5px] font-bold text-white bg-[var(--color-success-600)]" onclick={cancelTax}>
            <i class="icon-x me-1 text-[12px]"></i>{t('pos.cancelCalc')}
          </button>
        </div>
      </div>

      <div class="shrink-0 grid grid-cols-[96px_1fr] gap-2 p-3">
        <button type="button" class="h-12 rounded text-[12px] font-bold bg-[var(--color-warning-500)] text-black disabled:opacity-60" disabled title={t('pos.soon')}>
          <i class="icon-pause block mx-auto mb-0.5 text-[13px]"></i>{t('pos.pending')}
        </button>
        <button type="button" class="h-12 rounded text-[16px] font-extrabold text-white bg-[var(--color-success-600)] disabled:opacity-60 tabular-nums" disabled={!fresh || quoting || hasIssue || grand <= 0n} onclick={() => (paying = true)}>
          <i class="icon-wallet me-1.5"></i>{t('pos.pay')} : {money(grand)}
        </button>
      </div>
    </section>
  </div>
</div>

{#if editing !== null && cart[editing]}
  {@const ei = editing}
  <PriceOverrideModal
    name={cart[ei].name}
    discount={cart[ei].disc ?? ''}
    discountTotal={cart[ei].discTotal}
    listPrice={fresh && quote?.lines[ei]?.list_price ? quote.lines[ei].list_price : cart[ei].price}
    current={cart[ei].override ?? ''}
    onclose={() => (editing = null)}
    onapply={(patch, ap, pin) => applyChange(ei, patch, ap, pin)}
  />
{/if}

{#if paying}
  <PayModal total={grand} build={() => buildSale(true, true)} lineName={(i) => cart[i]?.name ?? ''} onclose={() => (paying = false)} ondone={saleDone} />
{/if}

{#if toast}
  <div role="status" class="fixed bottom-20 left-1/2 -translate-x-1/2 z-50 px-4 py-2 rounded-md text-[13px] text-white bg-[var(--color-danger-600)] shadow-[var(--shadow-lg)]">{toast}</div>
{/if}

{#if picks}
  <Modal title={t('pos.pickTitle')} onclose={() => (picks = null)}>
    <p class="text-[13px] text-[var(--text-secondary)] mb-3">{t('pos.pickBody')}</p>
    <ul class="space-y-2">
      {#each picks as m (m.id + m.unit)}
        <li>
          <button type="button" class="w-full text-start rounded-md border border-[var(--border-default)] p-3 hover:border-[var(--color-primary-500)]" onclick={() => pickMatch(m)}>
            <span class="block text-[13px] font-bold uppercase">{m.name}</span>
            <span class="block text-[11.5px] text-[var(--text-tertiary)]">{m.sku}{m.origin ? ` · ${m.origin}` : ''} · {m.unit} · {money(toCents(m.price))}</span>
          </button>
        </li>
      {/each}
    </ul>
  </Modal>
{/if}

<style>
  .empty-scan { animation: empty-pulse 2.4s ease-in-out infinite; }
  .empty-scan-line { top: 10%; animation: empty-scan 1.8s ease-in-out infinite; box-shadow: 0 0 8px currentColor; }
  .empty-text { animation: empty-fade 2.4s ease-in-out infinite; }
  .empty-arrow { display: inline-block; animation: empty-nudge 1.2s ease-in-out infinite; }
  @keyframes empty-scan { 0%, 100% { top: 10%; } 50% { top: 85%; } }
  @keyframes empty-pulse { 0%, 100% { transform: scale(1); } 50% { transform: scale(1.06); } }
  @keyframes empty-fade { 0%, 100% { opacity: 0.75; } 50% { opacity: 1; } }
  @keyframes empty-nudge { 0%, 100% { transform: translateX(0); } 50% { transform: translateX(-8px); } }
  @media (prefers-reduced-motion: reduce) {
    .empty-scan, .empty-scan-line, .empty-text, .empty-arrow { animation: none; }
  }
</style>
