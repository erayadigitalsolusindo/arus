<script lang="ts">
  import Select from '#lib/components/Select.svelte';
  // Form tambah/ubah member (satu komponen untuk /members/new dan /members/[id]). Tata letak mengikuti halaman member
  // legacy: cover foto di atas, empat kartu ringkasan, lalu kartu "Informasi" bertab (Biodata / Pengaturan / Poin).
  // Validasi di sini hanya untuk umpan balik cepat; server memvalidasi ulang semuanya (backend/internal/member).
  import { goto } from '$app/navigation';
  import { ApiError } from '#lib/api/client.ts';
  import { members as api, type Gender, type Member, type MemberInput, type MemberSale, type PointMove } from '#lib/members/api.ts';
  import { can } from '#lib/auth/session.svelte.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import { t, formatCurrency, formatNumber, formatDateTime } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName } from '#lib/validation.ts';
  import DatePicker from '#lib/components/DatePicker.svelte';
  import MemberCover from '#lib/components/MemberCover.svelte';
  import MoneyInput from '#lib/components/MoneyInput.svelte';
  import MarkdownEditor from '#lib/components/MarkdownEditor.svelte';
  import Modal from '#lib/components/Modal.svelte';
  import { focusOnMount } from '#lib/focus.ts';

  let { member }: { member: Member | null } = $props();

  type Field =
    | 'code' | 'name' | 'gender' | 'phone' | 'email' | 'address' | 'district' | 'city' | 'province' | 'postal_code'
    | 'credit_limit' | 'due_days' | 'valid_until' | 'notes';
  const TAB_OF: Record<Field, 'bio' | 'settings'> = {
    code: 'bio', name: 'bio', gender: 'bio', phone: 'bio', email: 'bio', address: 'bio', district: 'bio', city: 'bio',
    province: 'bio', postal_code: 'bio', notes: 'bio', credit_limit: 'settings', due_days: 'settings', valid_until: 'settings'
  };

  const CODE_RE = /^[A-Za-z0-9][A-Za-z0-9._/-]{0,39}$/;
  const MONEY_RE = /^\d{1,12}(\.\d{1,2})?$/;
  const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  const MAX_COVER = 10 << 20;

  // Nilai awal diambil sekali dari `member` (komponen dipasang ulang lewat {#key} bila member berganti).
  /* svelte-ignore state_referenced_locally */
  const init = member;
  /* svelte-ignore state_referenced_locally */
  let m = $state<Member | null>(member); // salinan hidup (poin/cover berubah lewat aksi langsung ke server)
  const canWrite = init ? can('members', 'update') : can('members', 'create');
  const canAdjust = can('members', 'approve');

  let tab = $state<'bio' | 'settings' | 'points' | 'sales'>('bio');
  let active = $state(init?.active ?? true);
  let code = $state(init?.code ?? '');
  let name = $state(init?.name ?? '');
  let gender = $state<Gender>(init?.gender ?? '');
  let phone = $state(init?.phone ?? '');
  let email = $state(init?.email ?? '');
  let address = $state(init?.address ?? '');
  let district = $state(init?.district ?? '');
  let city = $state(init?.city ?? '');
  let province = $state(init?.province ?? '');
  let postalCode = $state(init?.postal_code ?? '');
  let notes = $state(init?.notes ?? '');
  let creditLimit = $state(init ? trimZeros(init.credit_limit) : '');
  let dueDays = $state(init && init.due_days ? String(init.due_days) : '');
  let validity = $state<'always' | 'until'>(init?.valid_until ? 'until' : 'always');
  let validUntil = $state(init?.valid_until ?? '');

  let saving = $state(false);
  let error = $state('');
  let errors = $state<Partial<Record<Field, string>>>({});

  // Foto cover: mode ubah → aksi langsung ke server; mode tambah → file ditahan lalu diunggah setelah member tersimpan.
  let pendingCover = $state<File | null>(null);
  let pendingUrl = $state('');
  let coverBusy = $state(false);
  let coverError = $state('');
  let fileInput = $state<HTMLInputElement>();

  function trimZeros(s: string): string {
    return s.includes('.') ? s.replace(/0+$/, '').replace(/\.$/, '') : s;
  }

  const snapshot = () =>
    JSON.stringify([active, code, name, gender, phone, email, address, district, city, province, postalCode, notes, creditLimit, dueDays, validity, validUntil, !!pendingCover]);
  const initialSnapshot = snapshot();
  let saved = false;
  $effect(() => guard.register(() => canWrite && !saved && snapshot() !== initialSnapshot));

  $effect(() => {
    if (!pendingCover) {
      pendingUrl = '';
      return;
    }
    const u = URL.createObjectURL(pendingCover);
    pendingUrl = u;
    return () => URL.revokeObjectURL(u);
  });

  async function pickCover(ev: Event) {
    const input = ev.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    coverError = '';
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      coverError = t('members.cover.notImage');
      return;
    }
    if (file.size > MAX_COVER) {
      coverError = t('members.cover.tooBig');
      return;
    }
    if (!init) {
      pendingCover = file;
      return;
    }
    coverBusy = true;
    try {
      m = await api.uploadCover(init.id, file);
    } catch (err) {
      coverError = errorMessage(err);
    } finally {
      coverBusy = false;
    }
  }

  async function removeCover() {
    coverError = '';
    if (!init) {
      pendingCover = null;
      return;
    }
    coverBusy = true;
    try {
      m = await api.removeCover(init.id);
    } catch (err) {
      coverError = errorMessage(err);
    } finally {
      coverBusy = false;
    }
  }

  const hasCover = $derived(!!pendingUrl || !!m?.cover_image_id);

  async function save(ev: SubmitEvent) {
    ev.preventDefault();
    if (!canWrite) return;
    error = '';
    const next: Partial<Record<Field, string>> = {};
    const n = checkName(name, 200);
    if (n.code) next.name = fieldMessage(n.code);
    const c = code.trim();
    if (c && !CODE_RE.test(c)) next.code = fieldMessage('INVALID');
    if (init && !c) next.code = fieldMessage('REQUIRED');
    const digits = phone.replace(/[\s().-]/g, '');
    if (digits && !/^\+?\d{8,15}$/.test(digits)) next.phone = fieldMessage('INVALID');
    if (email.trim() && !EMAIL_RE.test(email.trim())) next.email = fieldMessage('INVALID');
    if (/[<>]/.test(address + district + city + province)) next.address = fieldMessage('INVALID');
    if (!/^[A-Za-z0-9 -]{0,10}$/.test(postalCode.trim())) next.postal_code = fieldMessage('INVALID');
    if (creditLimit.trim() && !MONEY_RE.test(creditLimit.trim())) next.credit_limit = fieldMessage('INVALID');
    if (dueDays.trim() && !/^\d{1,4}$/.test(dueDays.trim())) next.due_days = fieldMessage('INVALID');
    if (validity === 'until' && !/^\d{4}-\d{2}-\d{2}$/.test(validUntil)) next.valid_until = fieldMessage(validUntil ? 'INVALID' : 'REQUIRED');
    errors = next;
    const first = (Object.keys(next) as Field[])[0];
    if (first) {
      if (tab === 'bio' || tab === 'settings') tab = TAB_OF[first];
      return;
    }

    const body: MemberInput = {
      code: c,
      name: n.value,
      gender,
      phone: phone.trim(),
      email: email.trim(),
      address: address.trim(),
      district: district.trim(),
      city: city.trim(),
      province: province.trim(),
      postal_code: postalCode.trim(),
      credit_limit: creditLimit.trim(),
      due_days: dueDays.trim(),
      valid_until: validity === 'until' ? validUntil : '',
      notes: notes.replace(/\r\n?/g, '\n'),
      active
    };
    saving = true;
    try {
      if (init) {
        await api.update(init.id, body);
        saved = true;
        await goto('/members?notice=saved');
      } else {
        const created = await api.create(body);
        saved = true; // member sudah dibuat di server; keluar dari halaman ini tidak lagi membuang apa pun
        if (pendingCover) {
          try {
            await api.uploadCover(created.id, pendingCover);
          } catch {
            await goto(`/members/${created.id}?notice=cover_failed`);
            return;
          }
        }
        await goto('/members?notice=created');
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') {
        for (const [k, v] of Object.entries(err.fields)) errors[k as Field] = fieldMessage(v);
        const f = (Object.keys(err.fields) as Field[]).find((k) => TAB_OF[k]);
        if (f) tab = TAB_OF[f];
      } else if (err instanceof ApiError && err.code === 'CODE_TAKEN') {
        errors.code = errorMessage(err);
        tab = 'bio';
      } else {
        error = errorMessage(err);
      }
    } finally {
      saving = false;
    }
  }

  // ---- poin ----
  let moves = $state<PointMove[]>([]);
  let movesTotal = $state(0);
  let movesLoading = $state(false);
  let movesError = $state('');
  let movesLoaded = false;
  const MOVES_PAGE = 20;

  async function loadMoves(reset: boolean) {
    if (!init) return;
    movesLoading = true;
    movesError = '';
    try {
      const res = await api.points(init.id, { limit: MOVES_PAGE, offset: reset ? 0 : moves.length });
      moves = reset ? res.data : [...moves, ...res.data];
      movesTotal = res.total;
      movesLoaded = true;
    } catch (err) {
      movesError = errorMessage(err);
    } finally {
      movesLoading = false;
    }
  }

  $effect(() => {
    if (tab === 'points' && !movesLoaded) void loadMoves(true);
  });

  // ---- riwayat transaksi ----
  let sales = $state<MemberSale[]>([]);
  let salesTotal = $state(0);
  let salesLoading = $state(false);
  let salesError = $state('');
  let salesLoaded = false;

  async function loadSales(reset: boolean) {
    if (!init) return;
    salesLoading = true;
    salesError = '';
    try {
      const res = await api.sales(init.id, { limit: MOVES_PAGE, offset: reset ? 0 : sales.length });
      sales = reset ? res.data : [...sales, ...res.data];
      salesTotal = res.total;
      salesLoaded = true;
    } catch (err) {
      salesError = errorMessage(err);
    } finally {
      salesLoading = false;
    }
  }

  $effect(() => {
    if (tab === 'sales' && !salesLoaded) void loadSales(true);
  });

  let adjusting = $state(false);
  let adjPoints = $state('');
  let adjNote = $state('');
  let adjError = $state('');
  let adjBusy = $state(false);

  function openAdjust() {
    adjPoints = adjNote = adjError = '';
    adjusting = true;
  }

  async function submitAdjust(ev: SubmitEvent) {
    ev.preventDefault();
    if (!init) return;
    const n = Number(adjPoints.replace('−', '-').trim());
    if (!Number.isInteger(n) || n === 0 || Math.abs(n) > 1_000_000) {
      adjError = fieldMessage('INVALID') ?? '';
      return;
    }
    if (!adjNote.trim()) {
      adjError = fieldMessage('REQUIRED') ?? '';
      return;
    }
    adjBusy = true;
    adjError = '';
    try {
      m = await api.adjust(init.id, n, adjNote.trim());
      adjusting = false;
      notice = t('members.points.adjusted');
      await loadMoves(true);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') adjError = Object.values(err.fields).map((c) => fieldMessage(c)).join(' ');
      else adjError = errorMessage(err);
    } finally {
      adjBusy = false;
    }
  }

  let notice = $state('');

  const inputClass = 'w-full field-control';
  const labelClass = 'text-[11.5px] font-semibold uppercase tracking-wide mb-1.5 block text-[var(--text-tertiary)]';
  const errClass = 'text-[11.5px] mt-1 text-[var(--color-danger-600)]';
  const hintClass = 'text-[11px] mt-1 text-[var(--text-tertiary)]';
  const tabClass = (on: boolean) =>
    `px-4 py-3 text-[13px] font-semibold border-b-2 -mb-px transition-colors ${on ? 'border-[var(--color-primary-600)] text-[var(--color-primary-600)]' : 'border-transparent text-[var(--text-tertiary)] hover:text-[inherit]'}`;
  const kindClass = (k: PointMove['kind']) => (k === 'EARN' ? 'badge-success' : k === 'REDEEM' ? 'badge-warning' : k === 'REVERSAL' ? 'badge-danger' : 'badge-info');
  const signed = (n: number) => (n > 0 ? `+${formatNumber(n)}` : formatNumber(n));
</script>

<form class="space-y-4" onsubmit={save} novalidate>
  {#if error}
    <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
      <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{error}</span>
    </div>
  {/if}
  {#if notice}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-success">
      <i class="icon-circle-check text-[14px] shrink-0"></i><span>{notice}</span>
      <button type="button" class="ms-auto" aria-label={t('common.close')} onclick={() => (notice = '')}><i class="icon-x text-[13px]"></i></button>
    </div>
  {/if}
  {#if !canWrite}
    <div role="status" class="flex items-center gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-info">
      <i class="icon-eye text-[14px] shrink-0"></i><span>{t('members.readOnly')}</span>
    </div>
  {/if}

  <!-- Cover -->
  <section class="surface-card !p-0 overflow-hidden">
    <div class="relative h-44 sm:h-52 bg-gradient-to-br from-[var(--color-primary-600)] to-[color-mix(in_oklab,var(--color-primary-600)_55%,black)]">
      {#if pendingUrl}
        <img src={pendingUrl} alt={t('members.cover.alt', { name: name || '…' })} class="absolute inset-0 size-full object-cover" />
      {:else if m?.cover_image_id && init}
        <MemberCover memberId={init.id} coverId={m.cover_image_id} name={name} size="full" alt={t('members.cover.alt', { name })} class="absolute inset-0 size-full object-cover" />
      {/if}
      <div class="absolute inset-0 bg-gradient-to-t from-black/55 via-black/10 to-transparent pointer-events-none"></div>

      {#if canWrite}
        <div class="absolute end-3 top-3 flex gap-2">
          <button type="button" class="btn !text-[12px] !bg-white/90 !text-slate-800 backdrop-blur" disabled={coverBusy} onclick={() => fileInput?.click()}>
            <i class="icon-camera text-[14px]"></i><span class="hidden sm:inline">{hasCover ? t('members.cover.change') : t('members.cover.add')}</span>
          </button>
          {#if hasCover}
            <button type="button" class="btn !text-[12px] !bg-white/90 !text-slate-800 backdrop-blur" disabled={coverBusy} onclick={removeCover} aria-label={t('members.cover.remove')} title={t('members.cover.remove')}>
              <i class="icon-trash-2 text-[14px]"></i>
            </button>
          {/if}
          <input bind:this={fileInput} type="file" accept="image/jpeg,image/png,image/webp" class="hidden" onchange={pickCover} />
        </div>
      {/if}

      <div class="absolute inset-x-0 bottom-0 flex items-end gap-3 p-4 text-white">
        {#if init}
          <MemberCover memberId={init.id} coverId={m?.cover_image_id ?? null} name={name || init.name} alt="" class="size-16 sm:size-20 shrink-0 rounded-full object-cover border-4 border-white/90 text-[22px] !bg-white" />
        {:else}
          <span class="inline-flex size-16 sm:size-20 shrink-0 items-center justify-center rounded-full border-4 border-white/90 bg-white text-[var(--color-primary-600)]"><i class="icon-user-round text-[28px]"></i></span>
        {/if}
        <div class="min-w-0 pb-1">
          <p class="font-display font-bold text-[18px] sm:text-[22px] leading-tight truncate drop-shadow">{name.trim() || t('members.newTitle')}</p>
          <p class="text-[12px] opacity-90 flex flex-wrap items-center gap-x-2 gap-y-1">
            {#if init}<span class="font-mono">{init.code}</span>{/if}
            {#if m}<span class="inline-flex items-center gap-1 rounded-full bg-white/20 px-2 py-0.5 font-semibold backdrop-blur"><i class="icon-award text-[12px]"></i>{m.level.name}</span>{/if}
            <span class="inline-flex items-center rounded-full px-2 py-0.5 font-semibold {active ? 'bg-[var(--color-success-600)]' : 'bg-[var(--color-danger-600)]'}">{active ? t('members.active') : t('members.inactive')}</span>
          </p>
        </div>
      </div>
    </div>
    {#if coverError}<p class="{errClass} px-4 py-2">{coverError}</p>{:else if !init && pendingCover}<p class="{hintClass} px-4 py-2">{t('members.cover.pending')}</p>{:else if canWrite}<p class="{hintClass} px-4 py-2">{t('members.cover.hint')}</p>{/if}
  </section>

  <!-- Ringkasan -->
  {#if m}
    {@const stats = [
      { label: t('members.stat.sales'), value: formatCurrency(Number(m.stats.total_sales)), icon: 'trending-up', tone: 'text-[var(--color-info-600)] bg-[color-mix(in_oklab,var(--color-info-500)_14%,transparent)]' },
      { label: t('members.stat.trx'), value: formatNumber(m.stats.total_trx), icon: 'receipt', tone: 'text-[var(--color-primary-600)] bg-[color-mix(in_oklab,var(--color-primary-500)_14%,transparent)]' },
      { label: t('members.stat.points'), value: formatNumber(m.points), icon: 'star', tone: 'text-[var(--color-success-600)] bg-[color-mix(in_oklab,var(--color-success-500)_14%,transparent)]' },
      { label: t('members.stat.deposit'), value: formatCurrency(Number(m.stats.deposit)), icon: 'wallet', tone: 'text-[var(--color-danger-600)] bg-[color-mix(in_oklab,var(--color-danger-500)_14%,transparent)]', sub: t('members.stat.depositSoon') }
    ]}
    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {#each stats as s (s.label)}
        <div class="surface-card !p-4 flex items-center justify-between gap-3">
          <div class="min-w-0">
            <p class="font-display font-bold text-[20px] leading-tight truncate tabular-nums">{s.value}</p>
            <p class="text-[12px] text-[var(--text-tertiary)]">{s.label}{#if s.sub}<span class="ms-1.5 opacity-70">· {s.sub}</span>{/if}</p>
          </div>
          <span class="inline-flex size-11 shrink-0 items-center justify-center rounded-full {s.tone}"><i class="icon-{s.icon} text-[18px]"></i></span>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Informasi -->
  <section class="surface-card !p-0">
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-subtle)] ps-4">
      <h3 class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-users text-[15px]"></i>{t('members.section.info')}</h3>
      <div class="flex" role="tablist" aria-label={t('members.tab.label')}>
        <button type="button" role="tab" aria-selected={tab === 'bio'} class={tabClass(tab === 'bio')} onclick={() => (tab = 'bio')}>{t('members.tab.bio')}</button>
        <button type="button" role="tab" aria-selected={tab === 'settings'} class={tabClass(tab === 'settings')} onclick={() => (tab = 'settings')}>{t('members.tab.settings')}</button>
        {#if init}<button type="button" role="tab" aria-selected={tab === 'sales'} class={tabClass(tab === 'sales')} onclick={() => (tab = 'sales')}>{t('members.tab.sales')}</button>{/if}
        {#if init}<button type="button" role="tab" aria-selected={tab === 'points'} class={tabClass(tab === 'points')} onclick={() => (tab = 'points')}>{t('members.tab.points')}</button>{/if}
      </div>
    </div>

    <div class="p-4 space-y-4">
      {#if tab === 'bio'}
        <div class="text-center space-y-2">
          <p class="font-display font-bold text-[15px]">{t('members.status.title')}</p>
          <div class="inline-flex w-full max-w-xl overflow-hidden rounded-lg border border-[var(--border-subtle)]" role="radiogroup" aria-label={t('members.status.title')}>
            <button type="button" role="radio" aria-checked={active} disabled={!canWrite} class="flex-1 px-3 py-2.5 text-[13px] font-semibold transition-colors {active ? 'bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-600)]' : 'text-[var(--text-tertiary)]'}" onclick={() => (active = true)}>{t('members.status.on')}</button>
            <button type="button" role="radio" aria-checked={!active} disabled={!canWrite} class="flex-1 px-3 py-2.5 text-[13px] font-semibold border-s border-[var(--border-subtle)] transition-colors {!active ? 'bg-[color-mix(in_oklab,var(--color-warning-500)_20%,transparent)] text-[var(--color-warning-600)]' : 'text-[var(--text-tertiary)]'}" onclick={() => (active = false)}>{t('members.status.off')}</button>
          </div>
        </div>

        <div class="grid gap-3.5 sm:grid-cols-2">
          <div>
            <label for="m-code" class={labelClass}>{t('members.field.code')}</label>
            <input id="m-code" class="{inputClass} font-mono" bind:value={code} maxlength="40" placeholder={init ? '' : 'MBR-000001'} disabled={!canWrite} aria-invalid={!!errors.code} autocomplete="off" />
            {#if errors.code}<p class={errClass}>{errors.code}</p>{:else if !init}<p class={hintClass}>{t('members.field.codeHint')}</p>{/if}
          </div>
          <div>
            <label for="m-name" class={labelClass}>{t('members.field.name')}</label>
            <input id="m-name" class={inputClass} bind:value={name} maxlength="200" disabled={!canWrite} aria-invalid={!!errors.name} />
            {#if errors.name}<p class={errClass}>{errors.name}</p>{/if}
          </div>
          <div>
            <label for="m-gender" class={labelClass}>{t('members.field.gender')}</label>
            <Select id="m-gender" bind:value={gender} disabled={!canWrite} options={[{ value: '', label: t('members.field.genderNone') }, { value: 'M', label: t('members.field.genderM') }, { value: 'F', label: t('members.field.genderF') }]} />
          </div>
          <div>
            <label for="m-phone" class={labelClass}>{t('members.field.phone')}</label>
            <input id="m-phone" class={inputClass} bind:value={phone} maxlength="20" inputmode="tel" disabled={!canWrite} aria-invalid={!!errors.phone} autocomplete="off" />
            {#if errors.phone}<p class={errClass}>{errors.phone}</p>{/if}
          </div>
          <div>
            <label for="m-email" class={labelClass}>{t('members.field.email')}</label>
            <input id="m-email" type="email" class={inputClass} bind:value={email} maxlength="254" disabled={!canWrite} aria-invalid={!!errors.email} autocomplete="off" />
            {#if errors.email}<p class={errClass}>{errors.email}</p>{/if}
          </div>
          <div>
            <span class={labelClass}>{t('members.field.country')}</span>
            <p class="field-control !bg-[var(--surface-sunken)]">{t('members.field.countryValue')}</p>
          </div>
          <div>
            <label for="m-province" class={labelClass}>{t('members.field.province')}</label>
            <input id="m-province" class={inputClass} bind:value={province} maxlength="100" disabled={!canWrite} aria-invalid={!!errors.province} />
            {#if errors.province}<p class={errClass}>{errors.province}</p>{/if}
          </div>
          <div>
            <label for="m-city" class={labelClass}>{t('members.field.city')}</label>
            <input id="m-city" class={inputClass} bind:value={city} maxlength="100" disabled={!canWrite} aria-invalid={!!errors.city} />
            {#if errors.city}<p class={errClass}>{errors.city}</p>{/if}
          </div>
          <div>
            <label for="m-district" class={labelClass}>{t('members.field.district')}</label>
            <input id="m-district" class={inputClass} bind:value={district} maxlength="100" disabled={!canWrite} aria-invalid={!!errors.district} />
            {#if errors.district}<p class={errClass}>{errors.district}</p>{/if}
          </div>
          <div>
            <label for="m-postal" class={labelClass}>{t('members.field.postalCode')}</label>
            <input id="m-postal" class={inputClass} bind:value={postalCode} maxlength="10" inputmode="numeric" disabled={!canWrite} aria-invalid={!!errors.postal_code} />
            {#if errors.postal_code}<p class={errClass}>{errors.postal_code}</p>{/if}
          </div>
          <div class="sm:col-span-2">
            <label for="m-address" class={labelClass}>{t('members.field.address')}</label>
            <input id="m-address" class={inputClass} bind:value={address} maxlength="300" disabled={!canWrite} aria-invalid={!!errors.address} />
            {#if errors.address}<p class={errClass}>{errors.address}</p>{:else}<p class={hintClass}>{t('members.field.addressHint')}</p>{/if}
          </div>
          <div class="sm:col-span-2">
            <label for="m-notes" class={labelClass}>{t('members.field.notes')}</label>
            <MarkdownEditor id="m-notes" bind:value={notes} rows={4} maxlength={1000} disabled={!canWrite} invalid={!!errors.notes} />
            {#if errors.notes}<p class={errClass}>{errors.notes}</p>{/if}
          </div>
        </div>
      {:else if tab === 'settings'}
        <div class="grid gap-3.5 sm:grid-cols-2">
          <div>
            <label for="m-credit" class={labelClass}>{t('members.field.creditLimit')}</label>
            <MoneyInput id="m-credit" class={inputClass} bind:value={creditLimit} placeholder="0" disabled={!canWrite} aria-invalid={!!errors.credit_limit} />
            {#if errors.credit_limit}<p class={errClass}>{errors.credit_limit}</p>{:else}<p class={hintClass}>{t('members.field.creditLimitHint')}</p>{/if}
          </div>
          <div>
            <label for="m-due" class={labelClass}>{t('members.field.dueDays')}</label>
            <div class="flex">
              <input id="m-due" class="{inputClass} !rounded-e-none" bind:value={dueDays} inputmode="numeric" maxlength="4" placeholder="0" disabled={!canWrite} aria-invalid={!!errors.due_days} />
              <span class="inline-flex items-center rounded-e-lg border border-s-0 border-[var(--border-subtle)] bg-[var(--color-warning-500)] px-3 text-[12px] font-semibold uppercase text-white">{t('members.field.dueDaysUnit')}</span>
            </div>
            {#if errors.due_days}<p class={errClass}>{errors.due_days}</p>{:else}<p class={hintClass}>{t('members.field.dueDaysHint')}</p>{/if}
          </div>
          <div>
            <span class={labelClass}>{t('members.field.level')}</span>
            <p class="field-control !bg-[var(--surface-sunken)] flex items-center gap-2"><i class="icon-award text-[14px] text-[var(--color-primary-600)]"></i>{m?.level.name ?? '—'}</p>
            <p class={hintClass}>{t('members.field.levelHint')}</p>
          </div>
          <div>
            <span class={labelClass}>{t('members.field.validity')}</span>
            <div class="inline-flex overflow-hidden rounded-lg border border-[var(--border-subtle)]" role="radiogroup" aria-label={t('members.field.validity')}>
              <button type="button" role="radio" aria-checked={validity === 'always'} disabled={!canWrite} class="px-3 py-2 text-[12.5px] font-semibold {validity === 'always' ? 'bg-[color-mix(in_oklab,var(--color-success-500)_18%,transparent)] text-[var(--color-success-600)]' : 'text-[var(--text-tertiary)]'}" onclick={() => (validity = 'always')}>{t('members.field.validityAlways')}</button>
              <button type="button" role="radio" aria-checked={validity === 'until'} disabled={!canWrite} class="px-3 py-2 text-[12.5px] font-semibold border-s border-[var(--border-subtle)] {validity === 'until' ? 'bg-[color-mix(in_oklab,var(--color-warning-500)_20%,transparent)] text-[var(--color-warning-600)]' : 'text-[var(--text-tertiary)]'}" onclick={() => (validity = 'until')}>{t('members.field.validityUntil')}</button>
            </div>
          </div>
          {#if validity === 'until'}
            <div>
              <label for="m-valid" class={labelClass}>{t('members.field.validUntil')}</label>
              <DatePicker id="m-valid" bind:value={validUntil} disabled={!canWrite} invalid={!!errors.valid_until} clearable={false} />
              {#if errors.valid_until}<p class={errClass}>{errors.valid_until}</p>{:else}<p class={hintClass}>{t('members.field.validUntilHint')}</p>{/if}
            </div>
          {/if}
        </div>
      {:else if tab === 'sales'}
        <h4 class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-receipt text-[15px]"></i>{t('members.sales.title')}</h4>
        {#if salesError}<p class={errClass}>{t('members.sales.loadFailed')} {salesError}</p>{/if}
        <div class="overflow-x-auto scroll-thin rounded-lg border border-[var(--border-subtle)]">
          <table class="w-full text-[12.5px] min-w-[760px]">
            <thead>
              <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)] bg-[var(--surface-sunken)]">
                <th class="p-2.5 text-start" scope="col">{t('members.sales.col.doc')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.sales.col.time')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.sales.col.items')}</th>
                <th class="p-2.5 text-end" scope="col">{t('members.sales.col.total')}</th>
                <th class="p-2.5 text-end" scope="col">{t('members.sales.col.points')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.sales.col.cashier')}</th>
              </tr>
            </thead>
            <tbody>
              {#each sales as sl (sl.id)}
                <tr class="border-t border-[var(--border-subtle)] {sl.status === 'void' ? 'opacity-60' : ''}">
                  <td class="p-2.5 font-mono text-[12px] whitespace-nowrap">{sl.doc_no}{#if sl.status === 'void'}<span class="badge-soft badge-danger ms-1.5">{t('members.sales.void')}</span>{/if}</td>
                  <td class="p-2.5 whitespace-nowrap">{formatDateTime(sl.created_at)}<div class="text-[11px] text-[var(--text-tertiary)]">{sl.outlet}</div></td>
                  <td class="p-2.5 max-w-[22rem]"><span class="line-clamp-2">{sl.items}{sl.line_count > 3 ? ` …` : ''}</span><div class="text-[11px] text-[var(--text-tertiary)]">{t('members.sales.lines', { count: sl.line_count })}</div></td>
                  <td class="p-2.5 text-end font-semibold tabular-nums whitespace-nowrap">{formatCurrency(Number(sl.total))}</td>
                  <td class="p-2.5 text-end tabular-nums whitespace-nowrap">
                    {#if sl.points_earned > 0}<div class="font-semibold text-[var(--color-success-600)]">+{formatNumber(sl.points_earned)}</div>{/if}
                    {#if sl.points_redeemed > 0}<div class="font-semibold text-[var(--color-warning-600)]">−{formatNumber(sl.points_redeemed)}</div>{/if}
                    {#if sl.points_earned === 0 && sl.points_redeemed === 0}<span class="text-[var(--text-tertiary)]">—</span>{/if}
                  </td>
                  <td class="p-2.5">{sl.cashier}</td>
                </tr>
              {:else}
                <tr><td colspan="6" class="p-5 text-center text-[var(--text-tertiary)]">{salesLoading ? '…' : t('members.sales.empty')}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if sales.length < salesTotal}
          <div class="text-center"><button type="button" class="btn !text-[12px]" disabled={salesLoading} onclick={() => loadSales(false)}>{salesLoading ? '…' : t('members.next')}</button></div>
        {/if}
      {:else if m}
        <div class="grid gap-3 sm:grid-cols-3">
          <div class="rounded-lg border border-[var(--border-subtle)] p-3">
            <p class={labelClass}>{t('members.points.balance')}</p>
            <p class="font-display font-bold text-[22px] tabular-nums">{formatNumber(m.points)}</p>
          </div>
          <div class="rounded-lg border border-[var(--border-subtle)] p-3">
            <p class={labelClass}>{t('members.points.lifetime')}</p>
            <p class="font-display font-bold text-[22px] tabular-nums">{formatNumber(m.lifetime_points)}</p>
          </div>
          <div class="rounded-lg border border-[var(--border-subtle)] p-3">
            <p class={labelClass}>{t('members.points.level')}</p>
            <p class="font-display font-bold text-[22px] flex items-center gap-2"><i class="icon-award text-[18px] text-[var(--color-primary-600)]"></i>{m.level.name}</p>
          </div>
        </div>
        <div class="space-y-1 text-[12.5px] text-[var(--text-secondary)]">
          {#if Number(m.level.spend_per_point) > 0}
            <p>{t('members.points.rule', { spend: formatCurrency(Number(m.level.spend_per_point)), value: formatCurrency(Number(m.level.point_value)) })}</p>
          {:else}
            <p>{t('members.points.ruleNone')}</p>
          {/if}
          <p>{m.next_level ? t('members.points.nextLevel', { name: m.next_level.name, need: formatNumber(m.next_level.points_needed), min: formatNumber(m.next_level.min_points) }) : t('members.points.topLevel')}</p>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-2 pt-2">
          <h4 class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-history text-[15px]"></i>{t('members.points.history')}</h4>
          {#if canAdjust}<button type="button" class="btn !text-[12.5px]" onclick={openAdjust}><i class="icon-sparkles text-[13px]"></i>{t('members.points.adjust')}</button>{/if}
        </div>
        {#if movesError}<p class={errClass}>{t('members.points.loadFailed')} {movesError}</p>{/if}
        <div class="overflow-x-auto scroll-thin rounded-lg border border-[var(--border-subtle)]">
          <table class="w-full text-[12.5px] min-w-[560px]">
            <thead>
              <tr class="text-[11.5px] uppercase tracking-wide text-[var(--text-secondary)] bg-[var(--surface-sunken)]">
                <th class="p-2.5 text-start" scope="col">{t('members.points.col.time')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.points.col.type')}</th>
                <th class="p-2.5 text-end" scope="col">{t('members.points.col.points')}</th>
                <th class="p-2.5 text-end" scope="col">{t('members.points.col.balance')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.points.col.ref')}</th>
                <th class="p-2.5 text-start" scope="col">{t('members.points.col.actor')}</th>
              </tr>
            </thead>
            <tbody>
              {#each moves as mv (mv.id)}
                <tr class="border-t border-[var(--border-subtle)]">
                  <td class="p-2.5 whitespace-nowrap">{formatDateTime(mv.created_at)}</td>
                  <td class="p-2.5"><span class="badge-soft {kindClass(mv.kind)}">{t(`members.points.kind.${mv.kind}`)}</span></td>
                  <td class="p-2.5 text-end font-semibold tabular-nums {mv.points < 0 ? 'text-[var(--color-danger-600)]' : 'text-[var(--color-success-600)]'}">{signed(mv.points)}</td>
                  <td class="p-2.5 text-end tabular-nums">{formatNumber(mv.balance_after)}</td>
                  <td class="p-2.5">{mv.ref_type === 'SALE' ? mv.doc_no : t('members.points.manual')}{#if mv.ref_type === 'MANUAL' && mv.note}<span class="text-[var(--text-tertiary)]"> · {mv.note}</span>{/if}</td>
                  <td class="p-2.5">{mv.actor}</td>
                </tr>
              {:else}
                <tr><td colspan="6" class="p-5 text-center text-[var(--text-tertiary)]">{movesLoading ? '…' : t('members.points.historyEmpty')}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if moves.length < movesTotal}
          <div class="text-center"><button type="button" class="btn !text-[12px]" disabled={movesLoading} onclick={() => loadMoves(false)}>{movesLoading ? '…' : t('members.next')}</button></div>
        {/if}
      {/if}
    </div>

    {#if tab !== 'points' && tab !== 'sales'}
      <div class="flex justify-end gap-2 border-t border-[var(--border-subtle)] p-4">
        <a href="/members" class="btn !text-[12.5px]">{canWrite ? t('members.cancel') : t('members.back')}</a>
        {#if canWrite}
          <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={saving}>{saving ? t('members.saving') : t('members.save')}</button>
        {/if}
      </div>
    {/if}
  </section>
</form>

{#if adjusting}
  <Modal title={t('members.points.adjustTitle')} onclose={() => (adjusting = false)}>
    <form class="space-y-3.5" onsubmit={submitAdjust} novalidate>
      {#if adjError}
        <div role="alert" class="flex items-start gap-2 rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">
          <i class="icon-circle-alert text-[14px] mt-px shrink-0"></i><span>{adjError}</span>
        </div>
      {/if}
      <p class={hintClass}>{t('members.points.adjustHint')}</p>
      <div>
        <label for="adj-points" class={labelClass}>{t('members.points.adjustPoints')}</label>
        <input id="adj-points" class={inputClass} bind:value={adjPoints} inputmode="numeric" maxlength="9" placeholder="+100 / -20" use:focusOnMount />
      </div>
      <div>
        <label for="adj-note" class={labelClass}>{t('members.points.adjustNote')}</label>
        <input id="adj-note" class={inputClass} bind:value={adjNote} maxlength="200" placeholder={t('members.points.adjustNotePh')} />
      </div>
      <div class="flex justify-end gap-2 pt-1">
        <button type="button" class="btn !text-[12.5px]" onclick={() => (adjusting = false)}>{t('members.cancel')}</button>
        <button type="submit" class="btn btn-primary !text-[12.5px]" disabled={adjBusy}>{adjBusy ? t('members.saving') : t('members.points.adjustSubmit')}</button>
      </div>
    </form>
  </Modal>
{/if}
