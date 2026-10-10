<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  import { onMount, untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { can, logout, posOnly, session, switchOutlet } from '#lib/auth/session.svelte.ts';
  import OutletApprovalModal from '#lib/components/OutletApprovalModal.svelte';
  import { t, formatCurrency, formatDateTime, type MessageKey } from '#lib/i18n/index.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';
  import { items, type Row, type BarcodeMatch } from '#lib/items/api.ts';
  import { accessibleOutlets, refreshOutlets } from '#lib/outlets/store.svelte.ts';
  import { toCents, toMilli, centsToNumber, lineTotal } from '#lib/pos/money.ts';
  import { cartStorageKey, loadCart, saveCart, type CostEntry, type StoredCart, type StoredMember, type StoredSalesperson } from '#lib/pos/cart-store.ts';
  import { loadPending, savePending, nextPendingNo, pendingStorageKey, MAX_PENDING, MAX_LABEL, type PendingNote } from '#lib/pos/pending-store.ts';
  import MemberPicker from '#lib/components/MemberPicker.svelte';
  import MemberCover from '#lib/components/MemberCover.svelte';
  import type { Lookup } from '#lib/members/api.ts';
  import { ApiError } from '#lib/api/client.ts';
  import { fieldMessage } from '#lib/i18n/errors.ts';
  import { formatNumber } from '#lib/i18n/index.ts';
  import AuthImage from '#lib/components/AuthImage.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import TodaySalesModal from '#lib/components/TodaySalesModal.svelte';
  import Combobox from '#lib/components/Combobox.svelte';
  import { salespeopleLookup } from '#lib/catalog/api.ts';
  import PayModal from '#lib/components/PayModal.svelte';
  import ShiftOpenModal from '#lib/components/ShiftOpenModal.svelte';
  import ShiftCloseModal from '#lib/components/ShiftCloseModal.svelte';
  import { shifts, type Shift } from '#lib/shift/api.ts';
  import VoucherModal from '#lib/components/VoucherModal.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import { fitText } from '#lib/fitText.ts';
  import { focusOnMount } from '#lib/focus.ts';
  import { shortcuts as slotApi, SHORTCUT_SLOTS, type Shortcut } from '#lib/pos/shortcuts.ts';
  import PriceOverrideModal from '#lib/components/PriceOverrideModal.svelte';
  import type { Approver } from '#lib/approval/api.ts';
  import { sales, type Quote, type SaleInput } from '#lib/sales/api.ts';
  import { approvals as approvalApi } from '#lib/approval/api.ts';
  import { members as memberApi } from '#lib/members/api.ts';
  import { page } from '$app/state';
  import LanguageSwitcher from '#lib/components/LanguageSwitcher.svelte';
  import { initials } from '#lib/auth/initials.ts';

  // Satu baris keranjang. `price` = harga per satuan baris (string desimal dari API); `qty` = string yang diketik pengguna.
  type Line = { key: string; id: string; override: string | null; disc: string | null; discTotal: boolean; unitId: string | null; sku: string; name: string; unit: string; price: string; qty: string; goods: boolean; imageId: string | null };

  const PAGE = 24;
  // Daftar shortcut yang ditampilkan di bantuan (F1); urutan = urutan kerja kasir.
  const KEY_GROUPS: { title: MessageKey; keys: [string, MessageKey][] }[] = [
    {
      title: 'pos.keys.groupMain',
      keys: [
        ['F1', 'pos.keys.help'],
        ['F2', 'pos.keys.search'],
        ['F3', 'pos.keys.qty'],
        ['F4', 'pos.keys.member'],
        ['F6', 'pos.keys.salesperson'],
        ['Ctrl+K', 'pos.keys.voucher'],
        ['F7', 'pos.keys.hold'],
        ['F8', 'pos.keys.pending'],
        ['F9', 'pos.keys.today'],
        ['F10 / Ctrl+Enter', 'pos.keys.pay'],
        ['Esc', 'pos.keys.escape']
      ]
    },
    {
      title: 'pos.keys.groupCart',
      keys: [
        ['Ctrl+↑ / Ctrl+↓', 'pos.keys.select'],
        ['Ctrl++ / Ctrl+-', 'pos.keys.qtyStep'],
        ['Ctrl+E', 'pos.keys.editPrice'],
        ['Ctrl+Del', 'pos.keys.remove']
      ]
    },
    {
      title: 'pos.keys.groupPay',
      keys: [
        ['F1', 'pos.mode.cash'],
        ['F2', 'pos.mode.credit'],
        ['F3', 'pos.mode.noncash'],
        ['F4', 'pos.mode.split'],
        ['End', 'pos.keys.save']
      ]
    }
  ];
  const money = (c: bigint) => formatCurrency(centsToNumber(c), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  // ---------- Katalog ----------
  let q = $state('');
  let rows = $state<Row[]>([]);
  let exact = $state<Row[]>([]); // kode/barcode persis dari halaman pertama (Enter langsung memasukkannya)
  let cursor = $state(''); // kosong = tidak ada halaman berikutnya
  let loading = $state(false);
  let loadError = $state('');
  let searchEl = $state<HTMLInputElement>();
  let seq = 0; // hanya respons terbaru yang dipakai (pencarian cepat berurutan)

  async function load(reset: boolean) {
    const mine = ++seq;
    loading = true;
    loadError = '';
    try {
      const res = await items.search(q.trim(), reset ? '' : cursor, PAGE);
      if (mine !== seq) return;
      if (reset) exact = res.exact;
      rows = mergeRows(reset ? exact : rows, res.data);
      cursor = res.next_cursor;
    } catch (e) {
      if (mine === seq) loadError = errorMessage(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }

  /** Tambahkan baris baru tanpa duplikat (barang kode persis tampil paling atas dan bisa muncul lagi di daftar). */
  function mergeRows(base: Row[], more: Row[]): Row[] {
    const ids = new Set(base.map((r) => r.id));
    return [...base, ...more.filter((r) => !ids.has(r.id))];
  }

  let debounce: ReturnType<typeof setTimeout>;
  function onSearchInput() {
    clearTimeout(debounce);
    debounce = setTimeout(() => void load(true), 300);
  }

  onMount(() => {
    void load(true);
    void refreshOutlets();
    void loadSlots();
    void loadShift(!page.url.searchParams.get('edit'));
    searchEl?.focus();
    const editId = page.url.searchParams.get('edit');
    if (editId) {
      void goto('/kasir', { replaceState: true });
      void startEdit(editId);
    } else if (restored) flash(t('pos.restored', { count: restored.lines.length }));
    return () => clearTimeout(debounce);
  });

  // ---------- Keranjang ----------
  // Keranjang disimpan di browser (per tenant+outlet+kasir) agar selamat dari tab tertutup / mati lampu.
  const storeKey = $derived(session.tenant && session.outlet && session.user ? cartStorageKey(session.tenant.id, session.outlet.id, session.user.id) : '');
  const restored = session.tenant && session.outlet && session.user ? loadCart(cartStorageKey(session.tenant.id, session.outlet.id, session.user.id)) : null;
  let cart = $state<Line[]>(restored ? restored.lines.map((l) => ({ ...l, override: null, disc: null, discTotal: false })) : []);

  const cartSnapshot = (): StoredCart => ({ lines: cart.map(({ override: _o, disc: _d, discTotal: _t, ...l }) => l), otherCost, costs: $state.snapshot(costs), taxOn, note, member: $state.snapshot(member), redeem, salesperson: $state.snapshot(salesperson), vouchers: $state.snapshot(vouchers) });

  $effect(() => {
    const snapshot = cartSnapshot();
    if (storeKey && !editSale) saveCart(storeKey, snapshot);
  });

  // ---------- Nota pending ----------
  // Keranjang diparkir di browser (per tenant+outlet+kasir); dibuka lagi kapan saja. Harga & stok dihitung ulang oleh quote.
  const pendingKey = $derived(session.tenant && session.outlet && session.user ? pendingStorageKey(session.tenant.id, session.outlet.id, session.user.id) : '');
  let pending = $state<PendingNote[]>([]); // diisi efek di bawah
  let pendingOpen = $state(false);
  let todayOpen = $state(false);
  $effect(() => {
    const k = pendingKey; // pindah outlet/kasir = daftar lain
    untrack(() => (pending = k ? loadPending(k) : []));
  });
  function setPending(list: PendingNote[]) {
    pending = list;
    if (pendingKey) savePending(pendingKey, $state.snapshot(list));
  }
  /** Memarkir keranjang sekarang ke `list`; mengembalikan daftar baru + nomornya (null bila keranjang kosong). */
  function parkInto(list: PendingNote[], label = ''): { list: PendingNote[]; no: number } | null {
    if (!cart.length) return null;
    const no = nextPendingNo(list);
    const n: PendingNote = { ...cartSnapshot(), id: crypto.randomUUID(), no, at: Date.now(), label: label.trim().slice(0, MAX_LABEL), total: fresh ? centsToNumber(grand).toFixed(2) : null };
    return { list: [n, ...list], no };
  }
  let holdAsk = $state(false);
  let holdLabel = $state('');
  function askHold() {
    if (!cart.length) return;
    if (pending.length >= MAX_PENDING) return flash(t('pos.pendingFull', { max: MAX_PENDING }));
    holdLabel = member?.name ?? ''; // usulan: nama member bila ada
    holdAsk = true;
  }
  function holdCart() {
    holdAsk = false;
    if (pending.length >= MAX_PENDING) return flash(t('pos.pendingFull', { max: MAX_PENDING }));
    const r = parkInto(pending, holdLabel);
    if (!r) return;
    setPending(r.list);
    resetCart();
    flash(t('pos.pendingSaved', { no: r.no }));
  }
  function openPending(n: PendingNote) {
    // Keranjang yang sedang berisi otomatis diparkir dulu (menggantikan slot nota yang dibuka).
    const rest = pending.filter((p) => p.id !== n.id);
    const parked = parkInto(rest);
    setPending(parked ? parked.list : rest);
    cart = n.lines.map((l) => ({ ...l, override: null, disc: null, discTotal: false }));
    otherCost = n.otherCost;
    costs = n.costs ?? [];
    taxOn = n.taxOn;
    note = n.note;
    member = n.member ?? null;
    redeem = n.redeem ?? '';
    salesperson = n.salesperson ?? null;
    vouchers = n.vouchers ?? [];
    approval = null;
    pendingOpen = false;
    flash(parked ? t('pos.pendingOpenedSwapped', { no: n.no, saved: parked.no }) : t('pos.pendingOpened', { no: n.no }));
    searchEl?.focus();
  }
  function renamePending(n: PendingNote, label: string) {
    const v = label.trim().slice(0, MAX_LABEL);
    if (v !== n.label) setPending(pending.map((p) => (p.id === n.id ? { ...p, label: v } : p)));
  }
  function deletePending(n: PendingNote) {
    if (!confirm(t('pos.pendingDeleteAsk', { no: n.no }))) return;
    setPending(pending.filter((p) => p.id !== n.id));
  }

  /** `qty` (string desimal, opsional) = jumlah yang langsung masuk; kosong/tak valid = 1. */
  function add(l: Omit<Line, 'qty' | 'key' | 'override' | 'disc' | 'discTotal'> & { key?: string }, qty = '') {
    const key = l.key ?? `${l.id}:${l.unit}`;
    const m = toMilli(qty) > 0n ? toMilli(qty) : 1000n;
    const existing = cart.find((x) => x.key === key);
    if (existing) {
      existing.qty = fmtMilli(toMilli(existing.qty) + m);
      // Barang yang baru ditambah selalu naik ke paling atas supaya kasir langsung melihatnya.
      cart = [existing, ...cart.filter((x) => x !== existing)];
    } else {
      cart.unshift({ ...l, key, qty: fmtMilli(m), override: null, disc: null, discTotal: false });
    }
    highlight(key);
  }

  let cartEl = $state<HTMLElement>();
  let hotKey = $state('');
  let selKey = $state(''); // baris keranjang terpilih untuk shortcut Ctrl+↑/↓, Ctrl+±, Ctrl+Del, Ctrl+E
  let hotTimer: ReturnType<typeof setTimeout>;
  function highlight(key: string) {
    hotKey = key;
    selKey = key;
    cartEl?.scrollTo({ top: 0, behavior: 'smooth' });
    clearTimeout(hotTimer);
    hotTimer = setTimeout(() => (hotKey = ''), 1400);
  }

  function addRow(r: Row, qty = '') {
    add({ id: r.id, unitId: null, sku: r.sku, name: r.name, unit: r.unit, price: r.price, goods: r.kind === 'goods', imageId: r.main_image_id }, qty);
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
  // Rincian biaya lain-lain: bila ada, totalnya menggantikan isian tunggal `otherCost`.
  let costs = $state<CostEntry[]>(restored?.costs ?? []);
  let costsOpen = $state(false);
  const costsCents = $derived(costs.reduce((s, c) => s + toCents(c.amount), 0n));
  const centsStr = (c: bigint) => `${c / 100n}.${String(c % 100n).padStart(2, '0')}`;
  const otherCostValue = $derived(costs.length ? (costsCents > 0n ? centsStr(costsCents) : '') : otherCost.trim().replace(',', '.'));
  const otherCostCents = $derived(toCents(otherCostValue));
  function openCosts() {
    if (!costs.length) {
      // Isian tunggal yang sudah ada menjadi baris pertama rincian agar tidak hilang.
      costs = [{ label: '', amount: otherCostCents > 0n ? otherCostValue : '' }];
      otherCost = '';
    }
    costsOpen = true;
  }
  function closeCosts() {
    costs = costs.filter((c) => c.label.trim() !== '' || toCents(c.amount) > 0n);
    costsOpen = false;
  }
  /** Keterangan nota apa adanya; rincian biaya lain-lain dikirim terstruktur lewat `other_costs` (bukan ditempel ke teks). */
  const saleNote = () => note.trim();
  /** Rincian biaya lain-lain yang dikirim ke server; kosong = pakai isian tunggal `other_cost`. */
  const costRows = $derived(costs.filter((c) => toCents(c.amount) > 0n).map((c) => ({ name: c.label.trim(), amount: centsStr(toCents(c.amount)) })));
  const outletTax = $derived(accessibleOutlets.items.find((o) => o.id === session.outlet?.id));
  let taxOn = $state(restored?.taxOn ?? false);
  const pct = (v: string | number) => formatNumber(Number(v), { maximumFractionDigits: 2 });
  const calcTax = () => (taxOn = true);
  const cancelTax = () => (taxOn = false);

  let note = $state(restored?.note ?? '');

  // Member terpilih + poin yang ditukar. Saldo poin SELALU dari quote server (quote.member.points), bukan dari penyimpanan lokal.
  let member = $state<StoredMember | null>(restored?.member ?? null);
  let redeem = $state(restored?.redeem ?? '');
  // Salesman nota (opsional; kosong = Umum). Hanya label untuk laporan/komisi.
  let salesperson = $state<StoredSalesperson | null>(restored?.salesperson ?? null);
  // Kupon belanja (kode); potongan dihitung server pada quote. Dipakai sebelum pajak.
  let vouchers = $state<string[]>(restored?.vouchers ?? []);
  let voucherOpen = $state(false);
  let pickingSalesperson = $state(false);
  let spValue = $state('');
  let spLabel = $state('');
  function openSalesperson() {
    spValue = salesperson?.id ?? '';
    spLabel = salesperson?.name ?? '';
    pickingSalesperson = true;
  }
  function applySalesperson() {
    salesperson = spValue ? { id: spValue, name: spLabel } : null;
    pickingSalesperson = false;
    searchEl?.focus();
  }
  let pickingMember = $state(false);
  const redeemPoints = $derived(Math.max(0, Math.min(Number.parseInt(redeem, 10) || 0, 10_000_000)));
  function pickMember(m: Lookup) {
    member = { id: m.id, code: m.code, name: m.name, level: m.level, spend_per_point: m.spend_per_point, point_value: m.point_value, cover_image_id: m.cover_image_id };
    redeem = '';
    pickingMember = false;
    searchEl?.focus();
  }
  let redeemOpen = $state(false);
  function clearMember() {
    member = null;
    redeem = '';
  }
  const cartQty = $derived(cart.length);

  // forPay: sertakan persetujuan (PIN) untuk dikirim saat simpan nota; quote TIDAK pernah membawa PIN.
  const buildSale = (withNote = true, forPay = false): Omit<SaleInput, 'payments'> => ({
    lines: cart.map((l) => ({ item_id: l.id, ...(l.unitId ? { unit_id: l.unitId } : {}), qty: fmtMilli(toMilli(l.qty)), ...(l.override ? { unit_price: l.override } : {}), ...(l.disc ? { discount: discountOf(l) } : {}) })),
    ...(forPay && editSale && editApproverId ? { approval: { user_id: editApproverId, pin: editPin } } : forPay && approval && needsApproval() ? { approval: { user_id: approval.id, pin: approval.pin } } : {}),
    ...(costRows.length ? { other_costs: costRows } : otherCostValue && !costs.length ? { other_cost: otherCostValue } : {}),
    apply_tax: taxOn,
    ...(salesperson ? { salesperson_id: salesperson.id } : {}),
    ...(vouchers.length ? { voucher_codes: vouchers } : {}),
    ...(member ? { member_id: member.id, ...(redeemPoints > 0 ? { redeem_points: redeemPoints } : {}) } : {}),
    ...(withNote && saleNote() ? { note: saleNote() } : {})
  });

  // Persetujuan ubah harga: penyetuju + PIN hanya di memori halaman ini; dibuang saat nota selesai/keranjang dikosongkan.
  let approval = $state<{ id: string; name: string; pin: string } | null>(null);
  let editing = $state<number | null>(null); // indeks baris yang sedang diubah harganya

  // ---------- Mode EDIT nota ----------
  // Nota diedit lewat revisi: keranjang diisi dari nota lama, quote memakai /sales/{id}/quote (harga baris lama dipertahankan,
  // stok/poin/kupon nota lama sudah diperhitungkan), simpan = PUT /sales/{id}. Penyetuju (PIN) hanya di memori halaman ini.
  type EditCtx = { id: string; docNo: string; revision: number; outletId: string };
  let editSale = $state<EditCtx | null>(null);
  let editReason = $state('');
  let editApprovers = $state<Approver[]>([]);
  let editApproverId = $state('');
  let editPin = $state('');
  const editReady = $derived(!editSale || (editReason.trim().length >= 3 && editApproverId !== '' && /^[0-9]{6}$/.test(editPin)));
  const approverOptions = $derived([{ value: '', label: t('pos.edit.approverPlaceholder'), disabled: true }, ...editApprovers.map((a) => ({ value: a.id, label: a.name }))]);

  function clearEdit() {
    editSale = null;
    editReason = '';
    editApproverId = '';
    editPin = '';
    editApprovers = [];
  }
  async function startEdit(id: string) {
    try {
      const s = await sales.get(id);
      if (s.status !== 'completed') return flash(errorMessage(new ApiError(409, 'SALE_NOT_EDITABLE', '')));
      if (s.outlet_id !== session.outlet?.id) return flash(errorMessage(new ApiError(409, 'OUTLET_MISMATCH', '')));
      // Keranjang yang sedang berisi diparkir dulu (tidak hilang).
      const parked = parkInto(pending, '');
      if (parked) setPending(parked.list);
      cart = s.lines.map((l) => {
        const hasDisc = Number(l.discount) > 0;
        return { key: l.item_id + ':' + l.unit, id: l.item_id, override: null, disc: hasDisc ? l.discount : null, discTotal: hasDisc, unitId: l.unit_id, sku: l.sku, name: l.name, unit: l.unit, price: l.unit_price, qty: l.qty, goods: true, imageId: null };
      });
      costs = s.other_costs.map((c) => ({ label: c.name, amount: c.amount }));
      otherCost = costs.length === 0 && Number(s.other_cost) > 0 ? s.other_cost : '';
      taxOn = Number(s.tax_store_pct) > 0 || Number(s.tax_gov_pct) > 0;
      note = s.note;
      salesperson = s.salesperson ?? null;
      vouchers = s.vouchers.map((v) => v.code);
      redeem = s.points_redeemed > 0 ? String(s.points_redeemed) : '';
      member = null;
      if (s.member) {
        const found = (await memberApi.lookup(s.member.code).catch(() => [])).find((m) => m.id === s.member?.id);
        member = found
          ? { id: found.id, code: found.code, name: found.name, level: found.level, spend_per_point: found.spend_per_point, point_value: found.point_value, cover_image_id: found.cover_image_id }
          : { id: s.member.id, code: s.member.code, name: s.member.name, level: '', spend_per_point: '0', point_value: '0', cover_image_id: null };
      }
      approval = null;
      editSale = { id: s.id, docNo: s.doc_no, revision: s.revision, outletId: s.outlet_id };
      editApprovers = await approvalApi.approvers('sale_edit').catch(() => []);
    } catch (e) {
      flash(t('pos.edit.loadFailed') + ' ' + errorMessage(e));
    }
  }
  function cancelEdit() {
    if (!confirm(t('pos.edit.cancelAsk'))) return;
    clearEdit();
    resetCart();
    if (can('sales_list', 'view')) void goto('/sales');
  }
  // Pindah outlet saat mengedit = edit batal (nota hanya bisa diedit dari outletnya).
  $effect(() => {
    const oid = editSale?.outletId;
    if (oid && session.outlet?.id && session.outlet.id !== oid) {
      untrack(() => {
        clearEdit();
        resetCart();
      });
    }
  });
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
  const liveMember = $derived(quote?.member ?? null);
  const pointValue = $derived(Number(member?.point_value ?? 0));
  // Maksimum yang boleh ditukar: saldo poin, dan tidak melebihi nilai belanja (selain potongan lain).
  const redeemCap = $derived.by(() => {
    if (!quote || !liveMember || pointValue <= 0) return 0;
    const base = toCents(quote.subtotal) - (toCents(quote.discount) - toCents(quote.redeem_amount));
    return Math.max(0, Math.min(liveMember.points, Math.floor(Number(base) / 100 / pointValue)));
  });
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
  /** Pesan galat validasi quote: kupon yang tak lagi sah disebut dengan kodenya agar kasir tahu mana yang harus dilepas. */
  function validationText(e: ApiError): string {
    for (const [k, c] of Object.entries(e.fields)) {
      if (!k.startsWith('voucher_codes')) continue;
      const code = vouchers[Number(k.split('.')[1])];
      const msg = fieldMessage(c) ?? errorMessage(e);
      return code ? t('vouchers.pos.stale', { code, reason: msg }) : msg;
    }
    return fieldMessage(e.fields.redeem_points ?? e.fields.member_id) ?? errorMessage(e);
  }
  const hasIssue = $derived(fresh && !!quote?.lines.some((l) => l.issue));
  const lineTotalOf = (i: number) => (fresh && quote?.lines[i] ? toCents(quote.lines[i].line_total) : null);

  // Keranjang kosong = transaksi baru: biaya lain, pajak, dan keterangan sisa nota sebelumnya tidak ikut terbawa.
  $effect(() => {
    if (cart.length === 0) {
      untrack(() => {
        otherCost = '';
        costs = [];
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
        const body = JSON.parse(key);
        const r = editSale ? await sales.quoteEdit(editSale.id, body) : await sales.quote(body);
        if (mine !== qseq) return;
        const before = quote?.lines.filter((l) => l.issue).length ?? 0;
        quote = r;
        quoteError = '';
        const bad = r.lines.findIndex((l) => l.issue);
        if (bad >= 0 && r.lines.filter((l) => l.issue).length > before) flash(`${cart[bad]?.name ?? ''}: ${issueTextOf(r, bad)}`);
      } catch (e) {
        if (mine !== qseq) return;
        quote = null;
        quoteError = e instanceof ApiError && e.code === 'VALIDATION' ? validationText(e) : errorMessage(e);
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

  // ---------- Shift kasir ----------
  // Nota baru hanya bisa disimpan bila kasir punya shift terbuka di outlet aktif (server menegakkan: SHIFT_REQUIRED).
  // undefined = belum dimuat; null = belum ada shift terbuka.
  let shift = $state<Shift | null | undefined>(undefined);
  let shiftOpenDlg = $state(false);
  let shiftCloseId = $state<string | null>(null); // dipegang terpisah: layar "shift ditutup" tetap tampil setelah shift = null
  async function loadShift(ask: boolean) {
    try {
      shift = await shifts.current();
      if (!shift && ask && !editSale) shiftOpenDlg = true;
    } catch (e) {
      flash(errorMessage(e));
    }
  }
  /** Bayar: tanpa shift terbuka, buka shift dulu (edit nota tidak butuh shift). */
  function startPay() {
    if (!editSale && shift === null) {
      shiftOpenDlg = true;
      return;
    }
    paying = true;
  }
  function shiftOpened(s: Shift) {
    shift = s;
    shiftOpenDlg = false;
    flash(t('shift.open.done', { doc: s.doc_no }));
    searchEl?.focus();
  }
  function resetCart() {
    cart = [];
    approval = null;
    note = '';
    otherCost = '';
    costs = [];
    taxOn = false;
    member = null;
    redeem = '';
    salesperson = null;
    vouchers = [];
    searchEl?.focus();
  }
  function saleDone() {
    paying = false;
    const wasEdit = editSale !== null;
    clearEdit();
    resetCart();
    void load(true); // stok berubah
    if (wasEdit && can('sales_list', 'view')) void goto('/sales');
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

  function addMatch(m: BarcodeMatch, qty = '') {
    add({ id: m.id, unitId: m.matched === 'unit' ? m.unit_id : null, sku: m.sku, name: m.name, unit: m.unit, price: m.price, goods: true, imageId: null }, qty);
  }

  // Kolom QTY di sebelah pencarian: jumlah yang langsung masuk keranjang saat Enter (kosong = 1).
  let qtyIn = $state('');
  const focusQty = () => document.getElementById('pos-qty')?.focus();

  async function onEnter() {
    const code = q.trim();
    if (!code) {
      focusQty(); // dua-duanya kosong: Enter memindahkan fokus ke QTY
      return;
    }
    clearTimeout(debounce);
    const qty = qtyIn;
    try {
      const matches = (await items.byBarcode(code)).filter((m) => m.active);
      if (matches.length === 1) {
        addMatch(matches[0], qty);
        q = '';
        qtyIn = '';
        void load(true);
      } else if (matches.length > 1) {
        picks = matches;
      } else {
        // Bukan barcode: kode barang persis, atau pencarian nama yang menyisakan tepat satu barang, langsung ditambahkan.
        await load(true);
        const only = exact.length === 1 ? exact[0] : rows.length === 1 ? rows[0] : null;
        if (only) {
          addRow(only, qty);
          q = '';
          qtyIn = '';
          void load(true);
        } else if (rows.length === 0) flash(t('pos.notFound', { code }));
      }
    } catch (e) {
      flash(errorMessage(e));
    }
  }

  function onQtyEnter() {
    if (q.trim()) void onEnter();
    else searchEl?.focus(); // QTY terisi/kosong tanpa kode: kembali ke pencarian
  }

  function pickMatch(m: BarcodeMatch) {
    addMatch(m, qtyIn);
    picks = null;
    q = '';
    qtyIn = '';
    void load(true);
    searchEl?.focus();
  }

  // ---------- Tabel keranjang (hanya lihat) ----------
  let tableOpen = $state(false);
  let tableQ = $state('');
  const tableRows = $derived.by(() => {
    const s = tableQ.trim().toLowerCase();
    return cart.map((l, i) => ({ l, i })).filter(({ l }) => !s || l.name.toLowerCase().includes(s) || l.sku.toLowerCase().includes(s));
  });

  // ---------- Slot pintasan barang (16 slot per kasir, tersimpan di server) ----------
  let slots = $state<(Shortcut | null)[]>(Array(SHORTCUT_SLOTS).fill(null));
  function applySlots(list: Shortcut[]) {
    const next: (Shortcut | null)[] = Array(SHORTCUT_SLOTS).fill(null);
    for (const s of list) if (s.slot >= 1 && s.slot <= SHORTCUT_SLOTS) next[s.slot - 1] = s;
    slots = next;
  }
  async function loadSlots() {
    try {
      applySlots((await slotApi.list()).data);
    } catch (e) {
      flash(errorMessage(e));
    }
  }
  function addSlot(s: Shortcut) {
    if (!s.active) return flash(t('pos.slots.inactive'));
    add({ id: s.item_id, unitId: null, sku: s.sku, name: s.name, unit: s.unit, price: s.price, goods: s.kind === 'goods', imageId: s.main_image_id });
  }

  // Klik slot terisi = masuk keranjang; klik kanan / tekan lama (layar sentuh) = menu ganti/kosongkan; slot kosong = pilih barang.
  let slotsOpen = $state(false); // popup 16 tombol
  let slotMenu = $state<number | null>(null); // indeks slot (0-based) yang menunya terbuka
  let pressTimer: ReturnType<typeof setTimeout>;
  let longPressed = false;
  function slotPointerDown(e: PointerEvent, i: number) {
    longPressed = false;
    if (e.pointerType === 'mouse' || !slots[i]) return;
    pressTimer = setTimeout(() => ((longPressed = true), (slotMenu = i)), 550);
  }
  const slotPointerEnd = () => clearTimeout(pressTimer);
  function slotClick(i: number) {
    if (longPressed) return (longPressed = false);
    const s = slots[i];
    if (!s) return openAssign(i);
    addSlot(s);
    if (s.active) {
      slotsOpen = false; // satu klik = barang masuk keranjang, popup menutup
      searchEl?.focus();
    }
  }
  function slotContext(e: MouseEvent, i: number) {
    e.preventDefault();
    if (slots[i]) slotMenu = i;
    else openAssign(i);
  }
  async function clearSlot(i: number) {
    slotMenu = null;
    try {
      applySlots((await slotApi.clear(i + 1)).data);
    } catch (e) {
      flash(errorMessage(e));
    }
  }

  // Pemilih barang untuk mengisi slot.
  let assign = $state<number | null>(null); // indeks slot yang sedang diisi
  let quickQ = $state('');
  let quickRows = $state<Row[]>([]);
  let quickCursor = $state('');
  let quickLoading = $state(false);
  let quickSeq = 0;
  async function loadQuick(reset: boolean) {
    const mine = ++quickSeq;
    quickLoading = true;
    try {
      const res = await items.search(quickQ.trim(), reset ? '' : quickCursor, 24);
      if (mine !== quickSeq) return;
      quickRows = mergeRows(reset ? res.exact : quickRows, res.data);
      quickCursor = res.next_cursor;
    } catch (e) {
      if (mine === quickSeq) flash(errorMessage(e));
    } finally {
      if (mine === quickSeq) quickLoading = false;
    }
  }
  let quickDebounce: ReturnType<typeof setTimeout>;
  function openAssign(i: number) {
    slotMenu = null;
    assign = i;
    quickQ = '';
    void loadQuick(true);
  }
  function closeAssign() {
    assign = null;
    clearTimeout(quickDebounce);
    searchEl?.focus();
  }
  async function assignItem(r: Row) {
    const i = assign;
    if (i === null) return;
    try {
      applySlots((await slotApi.set(i + 1, r.id)).data);
      closeAssign();
    } catch (e) {
      flash(errorMessage(e));
    }
  }

  // ---------- Header / pemilih outlet / tema ----------
  let now = $state(new Date());
  $effect(() => {
    const id = setInterval(() => (now = new Date()), 1000);
    return () => clearInterval(id);
  });

  let outletBusy = $state(false);
  let outletSel = $state(session.outlet?.id ?? '');
  // Pindah outlet dari kasir butuh PIN Owner/Supervisor, kecuali pelakunya sendiri pemegang izin penyetuju.
  // Server memverifikasi ulang; modal hanya mengumpulkan penyetuju + PIN.
  let outletAsk = $state<{ id: string; name: string } | null>(null);
  async function changeOutlet(id: string) {
    if (!id || id === session.outlet?.id) return;
    if (can('outlet_switch', 'approve')) {
      await doSwitch(id);
      return;
    }
    outletAsk = { id, name: outletOptions.find((o) => o.id === id)?.name ?? '' };
  }
  function cancelOutletAsk() {
    outletAsk = null;
    outletSel = session.outlet?.id ?? '';
  }
  /** Melempar galat agar modal bisa menampilkannya; tanpa modal (penyetuju sendiri) galat jadi toast. */
  async function doSwitch(id: string, ap?: { user_id: string; pin: string }, rethrow = false) {
    outletBusy = true;
    try {
      await switchOutlet(id, { approval: ap });
      cart = []; // harga & stok berbeda per cabang: keranjang tidak dibawa pindah
      approval = null;
      cancelTax();
      await load(true);
      void loadSlots(); // harga efektif mengikuti outlet
      void loadShift(true); // shift berlaku per outlet
    } catch (err) {
      outletSel = session.outlet?.id ?? '';
      if (rethrow) throw err;
      flash(errorMessage(err));
    } finally {
      outletBusy = false;
    }
  }
  async function approveSwitch(approver: Approver, pin: string) {
    if (!outletAsk) return;
    await doSwitch(outletAsk.id, { user_id: approver.id, pin }, true);
    outletAsk = null;
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

  // Di bawah 1024 px layar dibagi tiga tab (Main / Barang / Keranjang); di atasnya tiga kolom berdampingan.
  type MobileTab = 'main' | 'items' | 'cart';
  const TAB_KEY = 'pos.mobileTab';
  let mobileTab = $state<MobileTab>('items');
  try {
    const saved = sessionStorage.getItem(TAB_KEY);
    if (saved === 'main' || saved === 'items' || saved === 'cart') mobileTab = saved;
  } catch {
    /* penyimpanan diblokir: abaikan */
  }
  function setTab(tab: MobileTab) {
    mobileTab = tab;
    try {
      sessionStorage.setItem(TAB_KEY, tab);
    } catch {
      /* penyimpanan diblokir: abaikan */
    }
  }
  const tabs: { id: MobileTab; label: MessageKey; icon: string }[] = [
    { id: 'main', label: 'pos.tabs.main', icon: 'icon-layout-dashboard' },
    { id: 'items', label: 'pos.tabs.items', icon: 'icon-package' },
    { id: 'cart', label: 'pos.tabs.cart', icon: 'icon-shopping-cart' }
  ];

  function toggleFullscreen() {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void document.documentElement.requestFullscreen?.();
  }

  // ---------- Shortcut keyboard (daftar lengkap: F1) ----------
  let keysOpen = $state(false);
  const payDisabled = $derived(!fresh || quoting || hasIssue || grand <= 0n || !editReady);

  function selectLine(step: number) {
    if (!cart.length) return;
    const i = cart.findIndex((l) => l.key === selKey);
    const next = i < 0 ? 0 : (i + step + cart.length) % cart.length;
    selKey = cart[next].key;
    document.getElementById(`cart-line-${next}`)?.scrollIntoView({ block: 'nearest' });
  }

  function onkeydown(e: KeyboardEvent) {
    // Dialog terbuka (bayar, member, pending, …): biarkan dialog yang menangani tombolnya sendiri.
    if (document.querySelector('[role="dialog"][aria-modal="true"]')) {
      if (e.key === 'F1') e.preventDefault(); // F1 = bantuan peramban
      return;
    }
    const ctrl = e.ctrlKey || e.metaKey;
    const sel = cart.findIndex((l) => l.key === selKey);
    const act = (fn: () => void) => {
      e.preventDefault();
      fn();
    };
    switch (e.key) {
      case 'F1':
        return act(() => (keysOpen = true));
      case 'F2':
        return act(() => {
          searchEl?.focus();
          searchEl?.select();
        });
      case 'F3':
        return act(() => (document.getElementById('pos-qty') as HTMLInputElement | null)?.select());
      case 'F4':
        return act(() => (pickingMember = true));
      case 'F6':
        return act(openSalesperson);
      case 'F7':
        return act(() => {
          if (cart.length && !editSale) askHold();
        });
      case 'F8':
        return act(() => (pendingOpen = true));
      case 'F9':
        return act(() => (todayOpen = true));
      case 'F10':
        return act(() => {
          if (!payDisabled) startPay();
        });
      case 'Escape':
        if (q) act(() => ((q = ''), void load(true), searchEl?.focus()));
        return;
    }
    if (!ctrl) return;
    if (e.key === 'Enter') return act(() => !payDisabled && startPay());
    if (e.key.toLowerCase() === 'k') return act(() => (voucherOpen = true));
    if (e.key === 'ArrowUp') return act(() => selectLine(-1));
    if (e.key === 'ArrowDown') return act(() => selectLine(1));
    if (sel < 0) return;
    const line = cart[sel];
    if (e.code === 'Equal' || e.code === 'NumpadAdd') return act(() => issueOf(sel) !== 'STOCK_INSUFFICIENT' && (line.qty = bump(line.qty, 1)));
    if (e.code === 'Minus' || e.code === 'NumpadSubtract') return act(() => (line.qty = bump(line.qty, -1)));
    if (e.key === 'Delete') return act(() => {
      remove(line.key);
      selKey = cart[Math.min(sel, cart.length - 1)]?.key ?? '';
    });
    if (e.key.toLowerCase() === 'e') return act(() => (editing = sel));
  }

  // Item pertama yang ditambahkan tidak perlu memicu muat ulang katalog; efek ini hanya menjaga fokus setelah modal.
  $effect(() => {
    if (!picks) untrack(() => searchEl?.focus());
  });
</script>

<svelte:head><title>{t('pos.docTitle')}</title></svelte:head>
<svelte:window {onkeydown} />

<div class="flex flex-col h-dvh overflow-hidden bg-[var(--surface-sunken)] text-[var(--text-primary)]">
  <!-- Bilah atas -->
  <header class="flex items-center gap-2 h-12 px-3 shrink-0 bg-[var(--surface-card)] border-b border-[var(--border-subtle)]">
    {#if posOnly()}
      <button type="button" class="header-icon-btn" aria-label={t('pos.logout')} title={t('pos.logout')} onclick={logout}><i class="icon-log-out text-[18px]"></i></button>
    {:else}
      <a href="/dashboard" class="header-icon-btn" aria-label={t('pos.back')} title={t('pos.back')}><i class="icon-arrow-left text-[18px]"></i></a>
    {/if}
    <span class="font-display font-extrabold tracking-tight text-[15px] uppercase text-[var(--color-primary-600)] truncate">{session.tenant?.name}</span>
    <span class="grow"></span>
    <button type="button" class="header-icon-btn hidden sm:inline-flex" aria-label={t('pos.keys.title')} title={`${t('pos.keys.title')} (F1)`} onclick={() => (keysOpen = true)}>
      <i class="icon-keyboard text-[17px]"></i>
    </button>
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
    {/if}
    <aside class="{mobileTab === 'main' ? 'flex' : 'max-lg:hidden'} {sideCollapsed ? 'lg:hidden' : 'lg:flex'} max-lg:flex-1 flex-col gap-3 p-3 min-h-0 overflow-y-auto bg-[var(--surface-card)] border-e border-[var(--border-subtle)]">
      <div class="flex items-center gap-1">
        <div class="grow text-center text-[11px] font-bold uppercase tracking-wide">{t('pos.info')} : {session.outlet?.code}</div>
        <button type="button" class="header-icon-btn max-lg:hidden" aria-label={t('pos.collapseInfo')} title={t('pos.collapseInfo')} onclick={toggleSide}>
          <i class="icon-chevrons-left text-[18px]"></i>
        </button>
      </div>
      <Select
        ariaLabel={t('pos.switchOutlet')}
        disabled={outletBusy || outletOptions.length < 2}
        bind:value={outletSel}
        onchange={changeOutlet}
        options={outletOptions.map((o) => ({ value: o.id, label: o.name }))}
      />

      <section class="rounded-lg border border-[var(--border-subtle)] p-3 space-y-2">
        <div class="text-[15px] font-bold text-[var(--color-primary-600)]">{t('pos.newReceipt')}</div>
        <dl class="text-[12px] space-y-0.5 text-[var(--text-secondary)]">
          <div>{formatDateTime(now, { dateStyle: 'short', timeStyle: 'medium' })}</div>
          <div>{t('pos.cashier')} : {session.user?.name}</div>
          {#if shift}
            <div title={t('shift.openAt', { time: shift.opened_local })}>{t('shift.button')} : <span class="font-semibold">{shift.doc_no}</span></div>
          {:else if shift === null}
            <div class="font-semibold text-[var(--color-danger-600)]">{t('shift.none')}</div>
          {/if}
          <div>
            {t('pos.salesperson')} :
            <button type="button" class="font-semibold underline decoration-dotted underline-offset-2 hover:text-[var(--color-primary-600)]" title={t('pos.salespersonPick')} onclick={openSalesperson}>{salesperson?.name ?? t('pos.salespersonNone')}</button>
          </div>
        </dl>
        <div class="flex gap-2 pt-1">
          <button type="button" class="btn btn-primary btn-sm" onclick={() => (pendingOpen = true)}>
            {t('pos.pendingReceipt')}{#if pending.length}<span class="ms-1.5 min-w-5 px-1 rounded-full bg-white/25 text-[11px] tabular-nums">{pending.length}</span>{/if}
          </button>
          {#if shift}
            <button type="button" class="btn btn-sm" onclick={() => (shiftCloseId = shift?.id ?? null)}><i class="icon-lock"></i> {t('shift.close')}</button>
          {:else if shift === null}
            <button type="button" class="btn btn-sm" onclick={() => (shiftOpenDlg = true)}><i class="icon-lock-open"></i> {t('shift.open.submit')}</button>
          {/if}
        </div>
      </section>

      <button type="button" class="flex items-start gap-3 p-2.5 rounded-lg border border-[var(--border-subtle)] text-start hover:bg-[var(--surface-sunken)]" onclick={() => (todayOpen = true)}>
        <span class="grid place-items-center size-8 shrink-0 rounded-full text-[11px] font-bold bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-700)]">PH</span>
        <span class="min-w-0">
          <span class="block text-[12px] font-semibold">{t('pos.shortcuts.today')}</span>
          <span class="block text-[10.5px] leading-snug text-[var(--text-tertiary)]">{t('pos.shortcuts.todayDesc')}</span>
        </span>
      </button>

      <span class="grow"></span>
      <button type="button" class="btn btn-sm w-full" disabled title={t('pos.soon')}>{t('pos.orderStatus')}</button>
    </aside>

    <!-- Kolom tengah: pencarian + katalog -->
    <main class="{mobileTab === 'items' ? 'flex' : 'max-lg:hidden'} lg:flex max-lg:flex-1 flex-col min-h-0 min-w-0">
      <div class="flex items-center gap-2 p-2.5 shrink-0 bg-[var(--surface-card)] border-b border-[var(--border-subtle)]">
        <MoneyInput
          id="pos-qty"
          bind:value={qtyIn}
          decimals={3}
          pad={false}
          placeholder={t('pos.qtyField')}
          title={t('pos.qtyFieldHint')}
          aria-label={t('pos.qtyField')}
          onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), onQtyEnter())}
          class="w-20 h-10 px-2 text-center rounded-md text-[14px] font-bold tabular-nums border border-[var(--border-default)] bg-[var(--surface-base)] text-[var(--color-primary-600)] outline-none focus:border-[var(--color-primary-500)]"
        />
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
        <button type="button" class="header-icon-btn relative" aria-label={t('pos.cartTable.button')} title={t('pos.cartTable.button')} onclick={() => ((tableQ = ''), (tableOpen = true))}>
          <i class="icon-table text-[16px]"></i>
          {#if cartQty}<span class="absolute -top-1 -end-1 min-w-4 h-4 px-1 rounded-full grid place-items-center text-[10px] font-bold text-white bg-[var(--color-primary-600)]">{cartQty}</span>{/if}
        </button>
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

        {#if cursor}
          <div class="text-center py-4">
            <button type="button" class="btn btn-sm" disabled={loading} onclick={() => load(false)}>{loading ? t('pos.loading') : t('pos.loadMore')}</button>
          </div>
        {/if}
      </div>

      <div class="shrink-0 flex items-center gap-2 p-2.5 bg-[var(--surface-card)] border-t border-[var(--border-subtle)]">
        <button type="button" class="grid place-items-center size-12 shrink-0 rounded-md text-white bg-[var(--color-primary-600)] hover:bg-[var(--color-primary-700)] transition-colors" aria-label={t('pos.slots.button')} title={t('pos.slots.button')} onclick={() => (slotsOpen = true)}>
          <i class="icon-layout-grid text-[20px]"></i>
        </button>
        <label class="block grow">
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
    <section class="{mobileTab === 'cart' ? 'flex' : 'max-lg:hidden'} lg:flex max-lg:flex-1 flex-col min-h-0 overflow-y-auto lg:overflow-visible bg-[var(--surface-card)] lg:border-s border-[var(--border-subtle)]">
      <div class="flex items-center gap-2 p-3 shrink-0 border-b border-[var(--border-subtle)]">
        <button type="button" class="header-icon-btn" aria-label={t('pos.pickCustomer')} title={t('members.pos.choose')} onclick={() => (pickingMember = true)}><i class="icon-user text-[16px]"></i></button>
        {#if member}
          <!-- Foto member menggantikan tombol daftar nota pending selama member terpilih; klik = ganti/lepas member. -->
          <button type="button" class="shrink-0 rounded-full" aria-label={t('members.pos.change')} title={t('members.pos.change')} onclick={() => (pickingMember = true)}>
            <MemberCover memberId={member.id} coverId={member.cover_image_id ?? null} name={member.name} class="size-9 rounded-full object-cover text-[12px] ring-2 ring-[var(--color-primary-600)]" />
          </button>
        {:else}
          <button type="button" class="header-icon-btn relative" aria-label={t('pos.pendingReceipt')} title={t('pos.pendingReceipt')} onclick={() => (pendingOpen = true)}>
            <i class="icon-list text-[16px]"></i>
            {#if pending.length}<span class="absolute -top-1 -end-1 min-w-4 h-4 px-1 grid place-items-center rounded-full bg-[var(--color-warning-500)] text-black text-[10px] font-bold tabular-nums">{pending.length}</span>{/if}
          </button>
        {/if}
        <div class="min-w-0 ps-1 leading-tight">
          <div class="text-[11.5px] font-semibold">{t('pos.customer')}</div>
          <div class="text-[11px] truncate {member ? 'font-bold uppercase text-[var(--text-secondary)]' : 'text-[var(--text-tertiary)]'}">{member ? `${member.name} [${member.code}]` : t('pos.customerGeneral')}</div>
          {#if member}
            <div class="text-[10.5px] truncate text-[var(--text-tertiary)]">
              {member.level}{liveMember ? ` · ${t('members.pos.points', { points: formatNumber(liveMember.points) })}` : ''}{#if fresh && quote && quote.points_earn > 0}<span class="font-semibold text-[var(--color-success-600)]"> · {t('members.pos.earned', { points: formatNumber(quote.points_earn) })}</span>{/if}
            </div>
          {/if}
        </div>
        {#if member && cart.length}
          <button
            type="button"
            class="relative ms-auto header-icon-btn"
            aria-label={t('members.pos.redeem')}
            title={t('members.pos.redeem')}
            onclick={() => (redeemOpen = true)}
          >
            <i class="icon-gift text-[15px]"></i>
            {#if redeemPoints > 0}<span class="absolute -top-1 -end-1 rounded-full bg-[var(--color-success-600)] px-1 text-[9.5px] font-bold leading-4 text-white">−{formatNumber(redeemPoints)}</span>{/if}
          </button>
        {/if}
        {#if cart.length}
          <button type="button" class="relative {member ? '' : 'ms-auto '}header-icon-btn" aria-label={t('vouchers.pos.button')} title={t('vouchers.pos.button')} onclick={() => (voucherOpen = true)}>
            <i class="icon-ticket-percent text-[15px]"></i>
            {#if vouchers.length}<span class="absolute -top-1 -end-1 min-w-4 h-4 px-1 grid place-items-center rounded-full bg-[var(--color-success-600)] text-[9.5px] font-bold leading-4 text-white">{vouchers.length}</span>{/if}
          </button>
        {/if}
        {#if cart.length}
          <button type="button" class="header-icon-btn" aria-label={t('pos.clearCart')} title={t('pos.clearCart')} onclick={() => ((cart = []), (approval = null), (redeem = ''), (vouchers = []))}>
            <i class="icon-trash-2 text-[15px]"></i>
          </button>
        {/if}
      </div>

      {#if editSale}
        <div class="mx-3 mt-2 shrink-0 space-y-2 rounded-lg border border-[var(--color-warning-500)] bg-[color-mix(in_oklab,var(--color-warning-500)_12%,transparent)] p-3 text-[12px]">
          <div class="flex items-start gap-2">
            <i class="icon-pencil mt-0.5 text-[15px] text-[var(--color-warning-600)]"></i>
            <div class="min-w-0 grow">
              <div class="truncate text-[13px] font-bold">{t('pos.edit.banner', { docNo: editSale.docNo })}</div>
              <div class="text-[11.5px] text-[var(--text-secondary)]">{t('pos.edit.revisionNext', { n: editSale.revision + 1 })}</div>
            </div>
            <button type="button" class="btn btn-sm shrink-0" onclick={cancelEdit}>{t('pos.edit.cancel')}</button>
          </div>
          <p class="text-[11.5px] leading-snug text-[var(--text-secondary)]">{t('pos.edit.hint')}</p>
          <input bind:value={editReason} maxlength="200" autocomplete="off" aria-label={t('pos.edit.reason')} placeholder={t('pos.edit.reasonPlaceholder')} class="h-9 w-full rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2 text-[12.5px] outline-none focus:border-[var(--color-primary-500)]" />
          {#if editApprovers.length === 0}
            <p class="text-[11.5px] text-[var(--color-danger-600)]">{t('pos.edit.noApprover')}</p>
          {:else}
            <div class="grid grid-cols-[minmax(0,1fr)_112px] gap-2">
              <Select bind:value={editApproverId} options={approverOptions} ariaLabel={t('pos.edit.approver')} />
              <input type="password" inputmode="numeric" autocomplete="off" maxlength="6" bind:value={editPin} aria-label={t('pos.edit.pin')} placeholder={t('pos.edit.pin')} class="h-9 min-w-0 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2 text-center text-[12.5px] tracking-widest outline-none focus:border-[var(--color-primary-500)]" />
            </div>
          {/if}
        </div>
      {/if}

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
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
          <div id={`cart-line-${i}`} onclick={() => (selKey = l.key)} class="rounded-md border p-2.5 transition-shadow {hotKey === l.key ? 'ring-2 ring-[var(--color-primary-500)] shadow-[var(--shadow-md)]' : selKey === l.key ? 'ring-1 ring-[var(--color-primary-400)]' : ''} {issueOf(i) ? 'border-[var(--color-danger-500)] bg-[color-mix(in_oklab,var(--color-danger-500)_6%,transparent)]' : 'border-[var(--border-subtle)]'}">
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
        <div class="flex items-center gap-2">
          <span class="w-28 shrink-0">{t('pos.otherCosts')}</span>
          {#if costs.length}
            <output class="grow h-8 px-2 flex items-center justify-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-sunken)]">{money(costsCents)}</output>
          {:else}
            <MoneyInput bind:value={otherCost} aria-label={t('pos.otherCosts')} placeholder="0" class="grow h-8 px-2 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] outline-none focus:border-[var(--color-primary-500)]" />
          {/if}
          <button type="button" class="grid place-items-center size-8 shrink-0 rounded border border-[var(--border-default)] text-[var(--color-primary-600)] bg-[var(--surface-base)]" aria-label={t('pos.costs.button')} title={t('pos.costs.button')} onclick={openCosts}>
            <i class="icon-list-plus text-[14px]"></i>
          </button>
        </div>
        {#if costs.length}
          <ul class="ms-[7.5rem] me-10 space-y-0.5 text-[11px] text-[var(--text-secondary)]">
            {#each costs as c, ci (ci)}
              {#if toCents(c.amount) > 0n}
                <li class="flex justify-between gap-2"><span class="truncate">{c.label.trim() || t('pos.costs.unnamed')}</span><span class="tabular-nums">{money(toCents(c.amount))}</span></li>
              {/if}
            {/each}
          </ul>
        {/if}
        {#if fresh && quote && quote.vouchers.length}
          <div class="flex items-center gap-2">
            <span class="w-28 shrink-0">{t('vouchers.pos.row')}</span>
            <output class="grow h-8 px-2 flex items-center justify-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-sunken)] text-[var(--color-success-600)] font-semibold" title={quote.vouchers.map((v) => v.code).join(', ')}>−{money(toCents(quote.voucher_amount))}</output>
          </div>
        {/if}
        <label class="flex items-center gap-2">
          <span class="w-28 shrink-0 leading-tight">{t('pos.storeTax')}{#if outletTax}<span class="block text-[10.5px] text-[var(--text-tertiary)]">{pct(outletTax.tax_store_pct)}%</span>{/if}</span>
          <output class="grow h-8 px-2 flex items-center justify-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-sunken)]" title={t('pos.taxPreview')}>{money(taxStore)}</output>
        </label>
        <label class="flex items-center gap-2">
          <span class="w-28 shrink-0 leading-tight">{t('pos.govTax')}{#if outletTax}<span class="block text-[10.5px] text-[var(--text-tertiary)]">{pct(outletTax.tax_gov_pct)}%</span>{/if}</span>
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

      <div class="shrink-0 grid grid-cols-[96px_1fr] gap-2 p-3 max-lg:sticky max-lg:bottom-0 max-lg:bg-[var(--surface-card)] max-lg:border-t max-lg:border-[var(--border-subtle)]">
        <button type="button" class="h-12 rounded text-[12px] font-bold bg-[var(--color-warning-500)] text-black disabled:opacity-60" disabled={!cart.length || !!editSale} onclick={askHold}>
          <i class="icon-pause block mx-auto mb-0.5 text-[13px]"></i>{t('pos.pending')}
        </button>
        <button type="button" class="h-12 rounded text-[16px] font-extrabold text-white bg-[var(--color-success-600)] disabled:opacity-60 tabular-nums" disabled={!fresh || quoting || hasIssue || grand <= 0n || !editReady} title={editReady ? '' : t('pos.edit.notReady')} onclick={startPay}>
          <i class="{editSale ? 'icon-save' : 'icon-wallet'} me-1.5"></i>{editSale ? t('pos.edit.save') : t('pos.pay')} : {money(grand)}
        </button>
      </div>
    </section>
  </div>

  <!-- Tab bawah (HP/tablet kecil) -->
  <nav class="lg:hidden shrink-0 grid grid-cols-3 bg-[var(--surface-card)] border-t border-[var(--border-subtle)] pb-[env(safe-area-inset-bottom)]" aria-label={t('pos.tabs.label')}>
    {#each tabs as tab (tab.id)}
      <button
        type="button"
        class="relative flex flex-col items-center justify-center gap-0.5 h-14 text-[11.5px] font-semibold transition-colors {mobileTab === tab.id ? 'text-[var(--color-primary-600)] border-t-2 border-[var(--color-primary-600)] -mt-px' : 'text-[var(--text-secondary)]'}"
        aria-current={mobileTab === tab.id ? 'page' : undefined}
        onclick={() => setTab(tab.id)}
      >
        <i class="{tab.icon} text-[20px]"></i>
        {t(tab.label)}
        {#if tab.id === 'cart' && cartQty}
          <span class="absolute top-1.5 start-1/2 ms-2 min-w-4 h-4 px-1 rounded-full grid place-items-center text-[10px] font-bold text-white bg-[var(--color-danger-600)]">{cartQty}</span>
        {/if}
      </button>
    {/each}
  </nav>
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

{#if outletAsk}
  <OutletApprovalModal outletId={outletAsk.id} outletName={outletAsk.name} onclose={cancelOutletAsk} onapprove={approveSwitch} />
{/if}

{#if pickingSalesperson}
  <Modal title={t('pos.salespersonPick')} onclose={() => (pickingSalesperson = false)}>
    <div class="space-y-3">
      <Combobox bind:value={spValue} bind:label={spLabel} search={salespeopleLookup.search} placeholder={t('pos.salespersonNone')} />
      <p class="text-[11.5px] text-[var(--text-tertiary)]">{t('pos.salespersonHint')}</p>
      <div class="flex justify-end">
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={applySalesperson}>{t('pos.salespersonDone')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if pickingMember}
  <MemberPicker onpick={pickMember} onclose={() => (pickingMember = false)} current={member?.name ?? ''} onclear={member ? () => { clearMember(); pickingMember = false; } : undefined} />
{/if}

{#if voucherOpen}
  <VoucherModal
    codes={vouchers}
    applied={fresh && quote ? quote.vouchers : []}
    money={(a) => money(toCents(a))}
    check={(codes) => sales.quote({ ...JSON.parse(cartKey), voucher_codes: codes })}
    onadd={(c) => (vouchers = [...vouchers, c])}
    onremove={(c) => (vouchers = vouchers.filter((x) => x !== c))}
    onclose={() => {
      voucherOpen = false;
      searchEl?.focus();
    }}
  />
{/if}

{#if redeemOpen && member}
  <Modal title={t('members.pos.redeem')} onclose={() => (redeemOpen = false)}>
    <div class="space-y-3">
      <p class="text-[12.5px] text-[var(--text-secondary)]">
        {member.name} [{member.code}]{liveMember ? ` · ${t('members.pos.points', { points: formatNumber(liveMember.points) })}` : ''}
      </p>
      {#if pointValue > 0}
        <p class="text-[12px] text-[var(--text-tertiary)]">{t('members.pos.redeemAvailable', { points: formatNumber(redeemCap), value: money(toCents(member.point_value)) })}</p>
        <div class="flex items-center gap-2">
          <input
            inputmode="numeric"
            maxlength="8"
            placeholder={t('members.pos.redeemPlaceholder')}
            aria-label={t('members.pos.redeem')}
            class="h-10 w-32 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 text-end tabular-nums outline-none focus:border-[var(--color-primary-500)]"
            value={redeem}
            oninput={(e) => (redeem = e.currentTarget.value.replace(/\D/g, ''))}
            use:focusOnMount
          />
          <button type="button" class="h-10 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-3 font-semibold disabled:opacity-50" disabled={redeemCap <= 0} onclick={() => (redeem = String(redeemCap))}>{t('members.pos.redeemAll')}</button>
          {#if redeemPoints > 0 && fresh && quote}<span class="ms-auto font-semibold text-[var(--color-success-600)]">{t('members.pos.redeemValue', { amount: money(toCents(quote.redeem_amount)) })}</span>{/if}
        </div>
        {#if quoteError}<p role="alert" class="text-[12px] text-[var(--color-danger-600)]">{quoteError}</p>{/if}
      {:else}
        <p class="text-[12.5px] text-[var(--text-tertiary)]">{t('members.pos.redeemNone')}</p>
      {/if}
      <div class="flex justify-end gap-2 pt-1">
        {#if redeemPoints > 0}<button type="button" class="btn !text-[12.5px]" onclick={() => (redeem = '')}>{t('members.pos.redeemReset')}</button>{/if}
        <button type="button" class="btn btn-primary !text-[12.5px]" onclick={() => (redeemOpen = false)}>{t('members.pos.redeemDone')}</button>
      </div>
    </div>
  </Modal>
{/if}

{#if keysOpen}
  <Modal title={t('pos.keys.title')} onclose={() => (keysOpen = false)}>
    <p class="text-[12.5px] text-[var(--text-secondary)] mb-3">{t('pos.keys.hint')}</p>
    {#each KEY_GROUPS as g (g.title)}
      <h3 class="mt-3 mb-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">{t(g.title)}</h3>
      <dl class="text-[13px] divide-y divide-[var(--border-subtle)]">
        {#each g.keys as [k, label] (k)}
          <div class="flex items-center justify-between gap-3 py-1.5">
            <dt>{t(label)}</dt>
            <dd class="flex gap-1">{#each k.split(' / ') as part (part)}<kbd class="px-1.5 py-0.5 rounded border border-[var(--border-default)] bg-[var(--surface-sunken)] font-mono text-[11.5px] whitespace-nowrap">{part}</kbd>{/each}</dd>
          </div>
        {/each}
      </dl>
    {/each}
  </Modal>
{/if}

{#if shiftOpenDlg}
  <ShiftOpenModal onclose={() => (shiftOpenDlg = false)} onopened={shiftOpened} />
{/if}

{#if shiftCloseId}
  <ShiftCloseModal
    shiftId={shiftCloseId}
    onclose={() => ((shiftCloseId = null), searchEl?.focus())}
    onclosed={() => (shift = null)}
    onopennew={() => ((shiftCloseId = null), (shiftOpenDlg = true))}
  />
{/if}

{#if paying}
  <PayModal total={grand} edit={editSale ? { id: editSale.id, reason: editReason.trim() } : undefined} build={() => buildSale(true, true)} lineName={(i) => cart[i]?.name ?? ''} onclose={() => (paying = false)} ondone={saleDone} member={quote?.member ?? null} credit={quote?.credit ?? null} />
{/if}

{#if costsOpen}
  <Modal title={t('pos.costs.title')} onclose={closeCosts}>
    <p class="text-[12.5px] text-[var(--text-secondary)] mb-3">{t('pos.costs.hint')}</p>
    <div class="space-y-2">
      {#each costs as c, ci (ci)}
        <div class="flex items-center gap-2">
          <input
            bind:value={c.label}
            maxlength="40"
            aria-label={t('pos.costs.name')}
            placeholder={t('pos.costs.namePlaceholder')}
            class="grow min-w-0 h-9 px-2.5 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]"
          />
          <MoneyInput bind:value={c.amount} aria-label={t('pos.costs.amount')} placeholder="0" class="w-32 h-9 px-2 text-end tabular-nums rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]" />
          <button type="button" class="grid place-items-center size-9 shrink-0 rounded bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)] text-[var(--color-danger-600)]" aria-label={t('pos.costs.remove')} title={t('pos.costs.remove')} onclick={() => (costs = costs.filter((_, k) => k !== ci))}>
            <i class="icon-trash-2 text-[14px]"></i>
          </button>
        </div>
      {/each}
    </div>
    <button type="button" class="btn btn-sm mt-3" disabled={costs.length >= 20} onclick={() => (costs = [...costs, { label: '', amount: '' }])}><i class="icon-plus me-1 text-[12px]"></i>{t('pos.costs.add')}</button>
    <div class="flex items-center justify-between mt-4 pt-3 border-t border-[var(--border-subtle)] text-[13px] font-bold">
      <span>{t('pos.costs.total')}</span><span class="tabular-nums">{money(costsCents)}</span>
    </div>
    <div class="flex justify-between gap-2 mt-4">
      <button type="button" class="btn btn-sm" onclick={() => ((costs = []), (costsOpen = false))}>{t('pos.costs.clear')}</button>
      <button type="button" class="btn btn-primary btn-sm" onclick={closeCosts}>{t('pos.costs.done')}</button>
    </div>
  </Modal>
{/if}

{#if tableOpen}
  <Modal title={t('pos.cartTable.title')} wide onclose={() => (tableOpen = false)}>
    <input
      bind:value={tableQ}
      type="search"
      autocomplete="off"
      placeholder={t('pos.cartTable.search')}
      aria-label={t('pos.cartTable.search')}
      class="w-full h-9 px-3 mb-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]"
    />
    {#if cart.length === 0}
      <p class="text-center text-[13px] text-[var(--text-tertiary)] py-8">{t('pos.cartTable.empty')}</p>
    {:else}
      <div class="overflow-x-auto rounded border border-[var(--border-subtle)]">
        <table class="w-full text-[12px] tabular-nums">
          <thead class="bg-[var(--surface-sunken)] text-[var(--text-secondary)] uppercase text-[10.5px] tracking-wide">
            <tr>
              <th class="px-2 py-2 text-start">{t('pos.cartTable.no')}</th>
              <th class="px-2 py-2 text-start">{t('pos.cartTable.code')}</th>
              <th class="px-2 py-2 text-start">{t('pos.cartTable.name')}</th>
              <th class="px-2 py-2 text-start">{t('pos.cartTable.unit')}</th>
              <th class="px-2 py-2 text-end">{t('pos.qty')}</th>
              <th class="px-2 py-2 text-end">{t('pos.cartTable.price')}</th>
              <th class="px-2 py-2 text-end">{t('pos.discount')}</th>
              <th class="px-2 py-2 text-end">{t('pos.cartTable.total')}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[var(--border-subtle)]">
            {#each tableRows as { l, i } (l.key)}
              <tr class={issueOf(i) ? 'bg-[color-mix(in_oklab,var(--color-danger-500)_8%,transparent)]' : ''}>
                <td class="px-2 py-1.5">{i + 1}</td>
                <td class="px-2 py-1.5 break-all">{l.sku}</td>
                <td class="px-2 py-1.5 font-semibold uppercase">{l.name}{#if issueOf(i)}<span class="block normal-case font-normal text-[11px] text-[var(--color-danger-600)]">{issueText(i)}</span>{/if}</td>
                <td class="px-2 py-1.5">{l.unit}</td>
                <td class="px-2 py-1.5 text-end">{l.qty}</td>
                <td class="px-2 py-1.5 text-end">{money(toCents(fresh && quote?.lines[i] ? quote.lines[i].unit_price : l.price))}</td>
                <td class="px-2 py-1.5 text-end">{money(fresh && quote?.lines[i]?.discount ? toCents(quote.lines[i].discount) : 0n)}</td>
                <td class="px-2 py-1.5 text-end font-semibold">{lineTotalOf(i) !== null ? money(lineTotalOf(i) ?? 0n) : '…'}</td>
              </tr>
            {:else}
              <tr><td colspan="8" class="px-2 py-6 text-center text-[var(--text-tertiary)]">{t('pos.cartTable.noMatch')}</td></tr>
            {/each}
          </tbody>
          {#if fresh && quote}
            <tfoot class="border-t-2 border-[var(--border-default)] text-[12px]">
              <tr><td colspan="7" class="px-2 pt-2 text-end text-[var(--text-secondary)]">{t('pos.subtotal')}</td><td class="px-2 pt-2 text-end">{money(toCents(quote.subtotal))}</td></tr>
              <tr><td colspan="7" class="px-2 text-end text-[var(--text-secondary)]">{t('pos.discount')}</td><td class="px-2 text-end">{money(toCents(quote.discount))}</td></tr>
              <tr><td colspan="7" class="px-2 text-end text-[var(--text-secondary)]">{t('pos.otherCosts')}</td><td class="px-2 text-end">{money(toCents(quote.other_cost))}</td></tr>
              <tr><td colspan="7" class="px-2 text-end text-[var(--text-secondary)]">{t('pos.storeTax')} + {t('pos.govTax')}</td><td class="px-2 text-end">{money(taxStore + taxGov)}</td></tr>
              <tr class="font-extrabold text-[14px]"><td colspan="7" class="px-2 py-2 text-end">{t('pos.payTotalLabel')}</td><td class="px-2 py-2 text-end text-[var(--color-danger-600)]">{money(grand)}</td></tr>
            </tfoot>
          {/if}
        </table>
      </div>
    {/if}
  </Modal>
{/if}

{#if slotsOpen}
  <Modal title={t('pos.slots.title')} wide onclose={() => (slotsOpen = false)}>
    <p class="text-[11.5px] text-[var(--text-tertiary)] mb-3">{t('pos.slots.hint')}</p>
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
      {#each slots as sc, i (i)}
        <button
          type="button"
          class="relative h-20 px-2 rounded-md text-[12px] font-bold uppercase leading-tight text-center transition-colors {sc
            ? sc.active
              ? 'text-white bg-[var(--color-primary-600)] hover:bg-[var(--color-primary-700)]'
              : 'text-[var(--text-tertiary)] bg-[var(--surface-sunken)] line-through'
            : 'normal-case font-semibold text-[var(--text-tertiary)] border-2 border-dashed border-[var(--border-default)] hover:border-[var(--color-primary-500)] hover:text-[var(--color-primary-600)]'}"
          title={sc ? t('pos.slots.filled', { name: sc.name }) : t('pos.slots.empty', { n: i + 1 })}
          aria-label={sc ? sc.name : t('pos.slots.empty', { n: i + 1 })}
          onclick={() => slotClick(i)}
          oncontextmenu={(e) => slotContext(e, i)}
          onpointerdown={(e) => slotPointerDown(e, i)}
          onpointerup={slotPointerEnd}
          onpointerleave={slotPointerEnd}
          onpointercancel={slotPointerEnd}
        >
          <span class="absolute top-1 start-1.5 text-[10px] font-semibold opacity-70">{i + 1}</span>
          <span class="line-clamp-3">{sc ? sc.name : '+'}</span>
        </button>
      {/each}
    </div>
  </Modal>
{/if}

{#if slotMenu !== null && slots[slotMenu]}
  {@const mi = slotMenu}
  <Modal title={t('pos.slots.menuTitle', { n: mi + 1 })} onclose={() => (slotMenu = null)}>
    <p class="text-[13px] font-bold uppercase mb-3">{slots[mi]?.name}</p>
    <div class="flex flex-col gap-2">
      <button type="button" class="btn btn-primary btn-sm" onclick={() => openAssign(mi)}>{t('pos.slots.replace')}</button>
      <button type="button" class="btn btn-sm" onclick={() => clearSlot(mi)}>{t('pos.slots.clear')}</button>
    </div>
  </Modal>
{/if}

{#if assign !== null}
  <Modal title={t('pos.slots.assignTitle', { n: assign + 1 })} wide onclose={closeAssign}>
    <input
      bind:value={quickQ}
      oninput={() => (clearTimeout(quickDebounce), (quickDebounce = setTimeout(() => void loadQuick(true), 300)))}
      use:focusOnMount
      type="search"
      autocomplete="off"
      placeholder={t('pos.slots.search')}
      aria-label={t('pos.slots.search')}
      class="w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[13px] outline-none focus:border-[var(--color-primary-500)]"
    />
    <p class="mt-2 mb-3 text-[11.5px] text-[var(--text-tertiary)]">{t('pos.slots.assignHint')}</p>
    {#if !quickLoading && quickRows.length === 0}
      <p class="text-center text-[13px] text-[var(--text-tertiary)] py-8">{t('pos.noItems')}</p>
    {/if}
    <div class="grid gap-2 grid-cols-[repeat(auto-fill,minmax(190px,1fr))]">
      {#each quickRows as r (r.id)}
        <button type="button" class="text-start rounded-md border border-[var(--border-default)] p-2.5 hover:border-[var(--color-primary-500)] hover:bg-[var(--surface-sunken)] transition-colors" onclick={() => assignItem(r)}>
          <span class="block text-[12.5px] font-bold uppercase leading-tight line-clamp-2">{r.name}</span>
          <span class="block mt-1 text-[11px] text-[var(--text-tertiary)] break-all">{r.sku} · {r.unit}</span>
          <span class="block mt-1 text-[11.5px] font-bold tabular-nums">{money(toCents(r.price))}</span>
        </button>
      {/each}
    </div>
    {#if quickCursor}
      <div class="text-center pt-3"><button type="button" class="btn btn-sm" disabled={quickLoading} onclick={() => loadQuick(false)}>{quickLoading ? t('pos.loading') : t('pos.loadMore')}</button></div>
    {/if}
    <div class="flex justify-end mt-4"><button type="button" class="btn btn-sm" onclick={closeAssign}>{t('pos.slots.cancel')}</button></div>
  </Modal>
{/if}

{#if toast}
  <div role="status" class="fixed bottom-20 left-1/2 -translate-x-1/2 z-[200] px-4 py-2 rounded-md text-[13px] text-white bg-[var(--color-danger-600)] shadow-[var(--shadow-lg)]">{toast}</div>
{/if}

{#if holdAsk}
  <Modal title={t('pos.pending')} onclose={() => (holdAsk = false)}>
    <form onsubmit={(e) => { e.preventDefault(); holdCart(); }} class="space-y-3">
      <label class="block text-[12.5px] font-semibold">
        {t('pos.pendingLabel')}
        <input class="mt-1 w-full h-10 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2" maxlength={MAX_LABEL} bind:value={holdLabel} placeholder={t('pos.pendingLabelPh')} use:focusOnMount />
      </label>
      <p class="text-[12px] text-[var(--text-tertiary)]">{t('pos.pendingLabelHint')}</p>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-sm" onclick={() => (holdAsk = false)}>{t('pos.pendingCancel')}</button>
        <button type="submit" class="btn btn-primary btn-sm">{t('pos.pendingSave')}</button>
      </div>
    </form>
  </Modal>
{/if}

{#if todayOpen}
  <TodaySalesModal onclose={() => (todayOpen = false)} />
{/if}

{#if pendingOpen}
  <Modal title={t('pos.pendingTitle')} wide onclose={() => (pendingOpen = false)}>
    {#if pending.length === 0}
      <p class="py-8 text-center text-[13px] text-[var(--text-tertiary)]">{t('pos.pendingEmpty')}</p>
    {:else}
      <p class="mb-3 text-[12px] text-[var(--text-tertiary)]">{t('pos.pendingHint')}</p>
      <ul class="divide-y divide-[var(--border-subtle)] rounded-lg border border-[var(--border-subtle)]">
        {#each pending as n (n.id)}
          <li class="flex items-center gap-3 p-3">
            <div class="min-w-0 grow leading-tight">
              <div class="text-[13.5px] font-bold">{n.label || t('pos.pendingNo', { no: n.no })}
                <span class="ms-1 font-normal text-[12px] text-[var(--text-tertiary)]">{formatDateTime(new Date(n.at), { timeStyle: 'short' })}</span>
              </div>
              <div class="text-[12px] text-[var(--text-secondary)] truncate">
                {#if n.label}{t('pos.pendingNo', { no: n.no })} · {/if}{t('pos.pendingLines', { count: n.lines.length })} · {n.member ? n.member.name : t('pos.customerGeneral')}
              </div>
              <div class="text-[11.5px] text-[var(--text-tertiary)] truncate">{n.lines.slice(0, 3).map((l) => l.name).join(', ')}{n.lines.length > 3 ? ', …' : ''}</div>
            </div>
            <input
              class="w-32 sm:w-44 text-[12px] h-8 shrink-0 rounded border border-[var(--border-default)] bg-[var(--surface-base)] px-2"
              maxlength={MAX_LABEL}
              value={n.label}
              placeholder={t('pos.pendingLabelPh')}
              aria-label={t('pos.pendingLabel')}
              onchange={(e) => renamePending(n, e.currentTarget.value)}
              onkeydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
            />
            {#if n.total}<span class="shrink-0 font-bold tabular-nums text-[13px]">{formatCurrency(Number(n.total), 'IDR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</span>{/if}
            <button type="button" class="btn btn-primary btn-sm" onclick={() => openPending(n)}>{t('pos.pendingOpen')}</button>
            <button type="button" class="header-icon-btn" aria-label={t('pos.pendingDelete')} title={t('pos.pendingDelete')} onclick={() => deletePending(n)}><i class="icon-trash-2 text-[15px]"></i></button>
          </li>
        {/each}
      </ul>
    {/if}
  </Modal>
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
