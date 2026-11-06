<script lang="ts">
  // Profil pribadi: banner + foto profil, ringkasan akun, kelengkapan profil, form informasi, keamanan, preferensi.
  // Semua operasi hanya pada akun sendiri (server memakai id dari token). Penjaga "belum disimpan" lewat `guard`.
  import { onMount } from 'svelte';
  import { ApiError } from '#lib/api/client.ts';
  import { logout, session } from '#lib/auth/session.svelte.ts';
  import { initials } from '#lib/auth/initials.ts';
  import { guard } from '#lib/tabs/guard.svelte.ts';
  import { LOCALES, i18n, setLocale, t, formatDate, localeCodes } from '#lib/i18n/index.ts';
  import { errorMessage, fieldMessage } from '#lib/i18n/errors.ts';
  import { checkName, checkPhone, checkPassword, cleanText, passwordStrength } from '#lib/validation.ts';
  import { MAX_BIO, MAX_IMAGE_BYTES, profileApi, profileImageUrl, type ImageKind, type Profile } from '#lib/profile/api.ts';
  import { me } from '#lib/profile/me.svelte.ts';
  import MarkdownEditor from '#lib/components/MarkdownEditor.svelte';
  import MarkdownView from '#lib/components/MarkdownView.svelte';
  import ProfileMfa from '#lib/components/ProfileMfa.svelte';
  import ProfileActivity from '#lib/components/ProfileActivity.svelte';

  type Tab = 'overview' | 'info' | 'security' | 'prefs';
  type Field = 'name' | 'phone' | 'job_title' | 'location' | 'bio';

  let profile = $state<Profile | null>(null);
  let loading = $state(true);
  let loadError = $state('');
  let tab = $state<Tab>('overview');

  // ---- form informasi ----
  let name = $state('');
  let phone = $state('');
  let jobTitle = $state('');
  let location = $state('');
  let bio = $state('');
  let saving = $state(false);
  let saved = $state(false);
  let formError = $state('');
  let fieldErr = $state<Partial<Record<Field, string>>>({});

  // Nomor tersimpan berbentuk +62…; bandingkan setelah dinormalkan agar "0812…" tidak dianggap berubah.
  const norm = (s: string) => cleanText(s) ?? s;
  const dirty = $derived(
    !!profile &&
      (norm(name) !== profile.name ||
        (phone.trim() === '' ? '' : (checkPhone(phone).value || phone.trim())) !== profile.phone ||
        norm(jobTitle) !== profile.job_title ||
        norm(location) !== profile.location ||
        bio.trim() !== profile.bio)
  );

  $effect(() => guard.register(() => dirty));

  function fill(p: Profile) {
    name = p.name;
    phone = p.phone;
    jobTitle = p.job_title;
    location = p.location;
    bio = p.bio;
  }

  function apply(p: Profile, refill = false) {
    profile = p;
    me.set(p);
    if (session.user && session.user.name !== p.name) session.user = { ...session.user, name: p.name };
    if (refill) fill(p);
  }

  async function load() {
    loading = true;
    loadError = '';
    try {
      const p = await profileApi.get();
      apply(p, true);
    } catch (e) {
      loadError = errorMessage(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  async function save(e: Event) {
    e.preventDefault();
    formError = '';
    saved = false;
    const errs: Partial<Record<Field, string>> = {};
    const n = checkName(name);
    if (n.code) errs.name = n.code;
    if (phone.trim() !== '') {
      const ph = checkPhone(phone);
      if (ph.code) errs.phone = ph.code;
    }
    fieldErr = errs;
    if (Object.keys(errs).length) return;
    saving = true;
    try {
      const p = await profileApi.update({ name: n.value, phone: phone.trim(), job_title: jobTitle, location, bio });
      apply(p, true);
      saved = true;
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') fieldErr = err.fields as Partial<Record<Field, string>>;
      else formError = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  function discard() {
    if (profile) fill(profile);
    fieldErr = {};
    formError = '';
  }

  // ---- foto profil & banner ----
  let avatarSrc = $state('');
  let bannerSrc = $state('');
  let busyKind = $state<ImageKind | null>(null);
  let imageError = $state('');
  let avatarInput = $state<HTMLInputElement>();
  let bannerInput = $state<HTMLInputElement>();

  $effect(() => {
    const p = profile;
    let live = true;
    if (!p?.avatar_id) avatarSrc = '';
    else profileImageUrl(p.id, p.avatar_id, 'avatar', 'thumb').then((u) => live && (avatarSrc = u), () => live && (avatarSrc = ''));
    if (!p?.banner_id) bannerSrc = '';
    else profileImageUrl(p.id, p.banner_id, 'banner', 'full').then((u) => live && (bannerSrc = u), () => live && (bannerSrc = ''));
    return () => (live = false);
  });

  async function pick(kind: ImageKind, e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    imageError = '';
    if (!/^image\/(jpeg|png|webp)$/.test(file.type)) {
      imageError = t('profile.image.badType');
      return;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      imageError = t('profile.image.tooBig');
      return;
    }
    busyKind = kind;
    try {
      apply(await profileApi.upload(kind, file));
    } catch (err) {
      imageError = errorMessage(err);
    } finally {
      busyKind = null;
    }
  }

  async function removeImage(kind: ImageKind) {
    imageError = '';
    busyKind = kind;
    try {
      apply(await profileApi.removeImage(kind));
    } catch (err) {
      imageError = errorMessage(err);
    } finally {
      busyKind = null;
    }
  }

  // ---- keamanan ----
  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let showPw = $state(false);
  let pwBusy = $state(false);
  let pwError = $state('');
  let pwDone = $state(false);
  let pwErr = $state<Record<string, string>>({});

  const strength = $derived(next === '' ? -1 : passwordStrength(next));
  const strengthKey = $derived((['weak', 'good', 'strong'] as const)[Math.max(strength, 0)]);
  const strengthColor = ['var(--color-danger-600)', 'var(--color-warning-600)', 'var(--color-success-600)'];

  async function changePassword(e: Event) {
    e.preventDefault();
    pwError = '';
    pwErr = {};
    if (current === '') {
      pwErr = { current_password: 'REQUIRED' };
      return;
    }
    const code = checkPassword(next, (profile?.email ?? '').toLowerCase());
    if (code) {
      pwErr = { password: code };
      return;
    }
    if (next !== confirm) {
      pwError = t('profile.security.mismatch');
      return;
    }
    pwBusy = true;
    try {
      await profileApi.changePassword(current, next);
      pwDone = true;
      current = next = confirm = '';
      // Server mencabut semua sesi: tampilkan pesan sebentar lalu keluar ke halaman masuk.
      setTimeout(() => void logout(), 1800);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'VALIDATION') pwErr = err.fields;
      else pwError = errorMessage(err);
    } finally {
      pwBusy = false;
    }
  }

  // ---- tampilan ----
  const STEP_TARGET: Record<string, 'avatar' | 'banner' | Tab> = { avatar: 'avatar', banner: 'banner', phone: 'info', job_title: 'info', location: 'info', bio: 'info', email_verified: 'info', mfa: 'security' };

  function goStep(key: string) {
    const target = STEP_TARGET[key];
    if (target === 'avatar') avatarInput?.click();
    else if (target === 'banner') bannerInput?.click();
    else tab = target;
  }

  const RING = 2 * Math.PI * 34;

  // Ringkasan akses dari izin sesi (hanya tampilan; penegakan tetap di server).
  const access = $derived.by(() => {
    const p = session.permissions;
    const full = p['*'] === true;
    const modules = Object.keys(p).filter((k) => !k.startsWith('_') && k !== 'pos_only' && Array.isArray(p[k]) && (p[k] as string[]).length > 0).length;
    const locked = Array.isArray(p._locked) ? p._locked.length : 0;
    const posOnly = Array.isArray(p.pos_only) && p.pos_only.includes('view');
    return { full, modules, locked, posOnly };
  });
  const tenureDays = $derived(profile ? Math.max(0, Math.floor((Date.now() - new Date(profile.created_at).getTime()) / 86_400_000)) : 0);
  const stats = $derived(
    profile
      ? [
          { key: 'actions', icon: 'activity', value: profile.actions_30d },
          { key: 'logins', icon: 'log-in', value: profile.logins_30d },
          { key: 'outlets', icon: 'building-2', value: profile.outlet_count },
          { key: 'tenure', icon: 'calendar', value: t('profile.stats.days', { count: tenureDays }) }
        ]
      : []
  );

  /** Segarkan profil diam-diam (mis. setelah 2FA berubah) tanpa menimpa isian form yang sedang diedit. */
  async function reloadQuiet() {
    try {
      apply(await profileApi.get());
    } catch {
      /* data lama tetap tampil */
    }
  }
  const tabs: { id: Tab; icon: string }[] = [
    { id: 'overview', icon: 'layout-dashboard' },
    { id: 'info', icon: 'user' },
    { id: 'security', icon: 'shield-check' },
    { id: 'prefs', icon: 'languages' }
  ];

  function onTabKey(e: KeyboardEvent) {
    const i = tabs.findIndex((x) => x.id === tab);
    if (e.key === 'ArrowRight') tab = tabs[(i + 1) % tabs.length].id;
    else if (e.key === 'ArrowLeft') tab = tabs[(i + tabs.length - 1) % tabs.length].id;
    else return;
    e.preventDefault();
    (document.getElementById(`ptab-${tab}`) as HTMLElement | null)?.focus();
  }

  const inputCls = 'mt-1 w-full h-10 px-3 rounded border border-[var(--border-default)] bg-[var(--surface-base)] text-[var(--text-primary)] text-[13.5px] focus-visible:outline-2 focus-visible:outline-[var(--color-primary-600)]';
  const glassBtn = 'inline-flex items-center gap-1.5 h-8 px-3 rounded-full text-[12px] font-semibold text-white backdrop-blur disabled:opacity-60';
</script>

<svelte:head><title>{t('profile.docTitle')}</title></svelte:head>

<main class="p-4 lg:p-6 max-w-5xl w-full mx-auto space-y-5">
  {#if loading}
    <div class="surface-card overflow-hidden animate-pulse" aria-busy="true">
      <div class="h-48 bg-[var(--surface-sunken)]"></div>
      <div class="p-5 flex items-end gap-4">
        <div class="size-28 -mt-16 rounded-full bg-[var(--surface-overlay)] border-4 border-[var(--surface-raised)]"></div>
        <div class="space-y-2 flex-1"><div class="h-5 w-48 rounded bg-[var(--surface-sunken)]"></div><div class="h-3.5 w-32 rounded bg-[var(--surface-sunken)]"></div></div>
      </div>
    </div>
  {:else if loadError || !profile}
    <div class="surface-card p-8 text-center space-y-3" role="alert">
      <i class="icon-triangle-alert text-[28px] text-[var(--color-danger-600)]"></i>
      <p class="text-[14px]">{loadError || t('profile.loadFailed')}</p>
      <button type="button" class="btn btn-outline" onclick={load}>{t('profile.retry')}</button>
    </div>
  {:else}
    <!-- ===== Header: banner + avatar + identitas ===== -->
    <section class="surface-card overflow-hidden" style="padding:0">
      <div class="relative h-44 sm:h-56 group" style="background:linear-gradient(120deg,var(--color-primary-700),var(--color-primary-500) 55%,var(--color-primary-300))">
        {#if bannerSrc}
          <img src={bannerSrc} alt={t('profile.banner.alt')} class="absolute inset-0 size-full object-cover" decoding="async" />
        {:else}
          <!-- Hiasan bawaan saat belum ada banner -->
          <svg class="absolute inset-0 size-full opacity-25" aria-hidden="true" preserveAspectRatio="xMidYMid slice" viewBox="0 0 800 224">
            <circle cx="680" cy="30" r="150" fill="none" stroke="#fff" stroke-width="1.5" />
            <circle cx="680" cy="30" r="105" fill="none" stroke="#fff" stroke-width="1.5" />
            <circle cx="680" cy="30" r="60" fill="none" stroke="#fff" stroke-width="1.5" />
            <circle cx="90" cy="230" r="120" fill="#fff" fill-opacity=".15" />
          </svg>
        {/if}
        <div class="absolute inset-0 pointer-events-none" style="background:linear-gradient(to top,rgb(0 0 0/.28),transparent 55%)"></div>

        <div class="absolute top-3 flex gap-2" style="inset-inline-end:0.75rem">
          <button type="button" class={glassBtn} style="background:rgb(0 0 0/.42)" disabled={busyKind !== null} onclick={() => bannerInput?.click()}>
            <i class="{busyKind === 'banner' ? 'icon-loader-circle animate-spin' : 'icon-camera'} text-[14px]"></i>
            {busyKind === 'banner' ? t('profile.image.uploading') : profile.banner_id ? t('profile.banner.change') : t('profile.banner.add')}
          </button>
          {#if profile.banner_id}
            <button type="button" class={glassBtn} style="background:rgb(0 0 0/.42)" disabled={busyKind !== null} aria-label={t('profile.banner.remove')} title={t('profile.banner.remove')} onclick={() => removeImage('banner')}>
              <i class="icon-trash-2 text-[14px]"></i>
            </button>
          {/if}
        </div>
        <input bind:this={bannerInput} type="file" accept="image/jpeg,image/png,image/webp" class="sr-only" tabindex="-1" aria-hidden="true" onchange={(e) => pick('banner', e)} />
      </div>

      <div class="px-5 sm:px-7 pb-5">
        <div class="flex flex-col sm:flex-row sm:items-end gap-4">
          <!-- Avatar menimpa tepi banner -->
          <div class="relative -mt-16 shrink-0 self-start">
            <div class="size-32 rounded-full overflow-hidden grid place-items-center text-[38px] font-display font-bold text-white" style="box-shadow:0 0 0 5px var(--surface-raised),var(--shadow-sm);background:linear-gradient(135deg,var(--color-primary-600),var(--color-primary-400))">
              {#if avatarSrc}
                <img src={avatarSrc} alt={t('profile.avatar.alt', { name: profile.name })} class="size-full object-cover" decoding="async" />
              {:else}
                <span aria-hidden="true">{initials(profile.name)}</span>
              {/if}
              {#if busyKind === 'avatar'}
                <span class="absolute inset-0 grid place-items-center" style="background:rgb(0 0 0/.45)"><i class="icon-loader-circle animate-spin text-[26px]"></i></span>
              {/if}
            </div>
            <button
              type="button"
              class="absolute bottom-1 grid place-items-center size-9 rounded-full text-white shadow-md hover:scale-105 transition-transform disabled:opacity-60"
              style="inset-inline-end:0.25rem;background:var(--color-primary-600);box-shadow:0 0 0 3px var(--surface-raised)"
              aria-label={profile.avatar_id ? t('profile.avatar.change') : t('profile.avatar.add')}
              title={profile.avatar_id ? t('profile.avatar.change') : t('profile.avatar.add')}
              disabled={busyKind !== null}
              onclick={() => avatarInput?.click()}
            >
              <i class="icon-camera text-[15px]"></i>
            </button>
            <input bind:this={avatarInput} type="file" accept="image/jpeg,image/png,image/webp" class="sr-only" tabindex="-1" aria-hidden="true" onchange={(e) => pick('avatar', e)} />
          </div>

          <div class="min-w-0 flex-1 sm:pb-1">
            <div class="flex flex-wrap items-center gap-2">
              <h1 class="font-display font-bold text-[22px] leading-tight truncate max-w-full">{profile.name}</h1>
              <span class="inline-flex items-center h-6 px-2.5 rounded-full text-[11.5px] font-semibold bg-[color-mix(in_oklab,var(--color-primary-600)_14%,transparent)] text-[var(--color-primary-600)]">{profile.role}</span>
            </div>
            <p class="text-[13.5px] text-[var(--text-secondary)] mt-0.5 truncate">{profile.job_title || profile.tenant}</p>
            <div class="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-[12.5px] text-[var(--text-tertiary)]">
              <span class="inline-flex items-center gap-1.5"><i class="icon-store text-[13px]"></i>{profile.tenant}</span>
              <span class="inline-flex items-center gap-1.5"><i class="icon-building-2 text-[13px]"></i>{t('profile.outlets', { count: profile.outlet_count })}</span>
              {#if profile.location}<span class="inline-flex items-center gap-1.5"><i class="icon-map-pin text-[13px]"></i>{profile.location}</span>{/if}
            </div>
          </div>

          {#if profile.avatar_id}
            <button type="button" class="btn btn-outline !text-[12.5px] self-start sm:self-end" disabled={busyKind !== null} onclick={() => removeImage('avatar')}>
              <i class="icon-trash-2 text-[14px]"></i>{t('profile.avatar.remove')}
            </button>
          {/if}
        </div>
        {#if imageError}<p role="alert" class="mt-3 text-[12.5px] text-[var(--color-danger-600)]">{imageError}</p>
        {:else}<p class="mt-3 text-[11.5px] text-[var(--text-tertiary)]">{t('profile.image.hint')} {t('profile.image.bannerHint')}</p>{/if}
      </div>
    </section>

    <ul class="grid grid-cols-2 lg:grid-cols-4 gap-3" aria-label={t('profile.overview.activity.title')}>
      {#each stats as st (st.key)}
        <li class="surface-card p-4 flex items-center gap-3" style="padding:1rem">
          <span class="grid place-items-center size-10 rounded-xl bg-[color-mix(in_oklab,var(--color-primary-600)_12%,transparent)] text-[var(--color-primary-600)]"><i class="icon-{st.icon} text-[18px]"></i></span>
          <div class="min-w-0">
            <p class="font-display font-bold text-[20px] leading-none tabular-nums text-[var(--text-primary)]">{st.value}</p>
            <p class="text-[11.5px] mt-1 text-[var(--text-tertiary)] truncate">{t(`profile.stats.${st.key}` as 'profile.stats.actions')}</p>
          </div>
        </li>
      {/each}
    </ul>

    <div class="grid gap-5 xl:grid-cols-[320px_minmax(0,1fr)] items-start">
      <!-- ===== Kolom kiri: kelengkapan + tentang ===== -->
      <aside class="space-y-5">
        <section class="surface-card p-5" aria-labelledby="pc-title">
          <div class="flex items-center gap-4">
            <div class="relative size-20 shrink-0">
              <svg viewBox="0 0 80 80" class="size-full -rotate-90" aria-hidden="true">
                <circle cx="40" cy="40" r="34" fill="none" stroke="var(--surface-overlay)" stroke-width="8" />
                <circle cx="40" cy="40" r="34" fill="none" stroke={profile.completeness >= 100 ? 'var(--color-success-600)' : 'var(--color-primary-600)'} stroke-width="8" stroke-linecap="round" stroke-dasharray={RING} stroke-dashoffset={RING * (1 - profile.completeness / 100)} style="transition:stroke-dashoffset .6s ease" />
              </svg>
              <span class="absolute inset-0 grid place-items-center font-display font-bold text-[16px]" role="img" aria-label={t('profile.completeness.percent', { value: profile.completeness })}>{t('profile.completeness.percent', { value: profile.completeness })}</span>
            </div>
            <div class="min-w-0">
              <h2 id="pc-title" class="font-display font-bold text-[15px]">{t('profile.completeness.title')}</h2>
              <p class="text-[12.5px] text-[var(--text-secondary)] mt-0.5">{profile.missing.length === 0 ? t('profile.completeness.done') : t('profile.completeness.next')}</p>
            </div>
          </div>
          {#if profile.missing.length > 0}
            <ul class="mt-4 space-y-1.5">
              {#each profile.missing.slice(0, 4) as key (key)}
                <li>
                  <button type="button" class="w-full flex items-center gap-2.5 text-start px-2.5 py-2 rounded-lg text-[13px] text-[var(--text-primary)] hover:bg-[var(--surface-sunken)] transition-colors" onclick={() => goStep(key)}>
                    <i class="icon-circle text-[14px] text-[var(--text-tertiary)]"></i>
                    <span class="flex-1">{t(`profile.completeness.step.${key}` as 'profile.completeness.step.avatar')}</span>
                    <i class="icon-chevron-right text-[12px] text-[var(--text-tertiary)] rtl:rotate-180"></i>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="surface-card p-5 space-y-4" aria-labelledby="pa-title">
          <h2 id="pa-title" class="font-display font-bold text-[15px]">{t('profile.about.title')}</h2>
          {#if profile.bio}
            <MarkdownView source={profile.bio} class="text-[13.5px] leading-relaxed" />
          {:else}
            <p class="text-[13px] text-[var(--text-tertiary)]">{t('profile.about.empty')}</p>
            <button type="button" class="text-[13px] font-semibold text-[var(--color-primary-600)]" onclick={() => (tab = 'info')}>{t('profile.about.addBio')}</button>
          {/if}
          <dl class="space-y-2.5 pt-3 border-t border-[var(--border-subtle)] text-[13px]">
            <div class="flex items-center gap-2.5"><i class="icon-mail text-[15px] text-[var(--text-tertiary)]"></i><dt class="sr-only">{t('profile.info.email')}</dt><dd class="truncate">{profile.email}</dd></div>
            {#if profile.phone}<div class="flex items-center gap-2.5"><i class="icon-phone text-[15px] text-[var(--text-tertiary)]"></i><dt class="sr-only">{t('profile.info.phone')}</dt><dd>{profile.phone}</dd></div>{/if}
            <div class="flex items-center gap-2.5">
              <i class="{profile.email_verified ? 'icon-badge-check text-[var(--color-success-600)]' : 'icon-triangle-alert text-[var(--color-warning-600)]'} text-[15px]"></i>
              <dd class={profile.email_verified ? 'text-[var(--color-success-600)]' : 'text-[var(--color-warning-600)]'}>{profile.email_verified ? t('profile.verified') : t('profile.unverified')}</dd>
            </div>
            <div class="flex items-center gap-2.5"><i class="icon-calendar text-[15px] text-[var(--text-tertiary)]"></i><dd>{t('profile.memberSince', { date: formatDate(profile.created_at) })}</dd></div>
            <div class="flex items-center gap-2.5"><i class="icon-clock text-[15px] text-[var(--text-tertiary)]"></i><dd>{profile.last_login_at ? t('profile.lastLogin', { date: formatDate(profile.last_login_at, { dateStyle: 'medium', timeStyle: 'short' }) }) : t('profile.noLogin')}</dd></div>
          </dl>
        </section>
      </aside>

      <!-- ===== Kolom kanan: tab ===== -->
      <section class="surface-card p-0 overflow-hidden" style="padding:0">
        <div class="flex gap-1 px-3 pt-2 border-b border-[var(--border-subtle)] overflow-x-auto" role="tablist" aria-label={t('profile.tabs.label')} tabindex="-1" onkeydown={onTabKey}>
          {#each tabs as x (x.id)}
            <button
              type="button"
              role="tab"
              id="ptab-{x.id}"
              aria-selected={tab === x.id}
              aria-controls="ppanel-{x.id}"
              tabindex={tab === x.id ? 0 : -1}
              class="!bg-transparent inline-flex items-center gap-2 px-3.5 h-11 text-[13.5px] font-semibold whitespace-nowrap border-b-2 -mb-px transition-colors {tab === x.id ? 'border-[var(--color-primary-600)] text-[var(--color-primary-600)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary,inherit)]'}"
              onclick={() => (tab = x.id)}
            >
              <i class="icon-{x.icon} text-[15px]"></i>{t(`profile.tabs.${x.id}` as 'profile.tabs.info')}
              {#if x.id === 'info' && dirty}<span class="size-2 rounded-full bg-[var(--color-warning-600)]" title={t('profile.info.dirty')}></span>{/if}
            </button>
          {/each}
        </div>

        <!-- Ringkasan -->
        {#if tab === 'overview'}
          <div id="ppanel-overview" role="tabpanel" aria-labelledby="ptab-overview" class="p-5 sm:p-6 space-y-6">
            <div class="grid gap-4 sm:grid-cols-2">
              <section class="p-4 rounded-xl border border-[var(--border-subtle)] space-y-2" aria-labelledby="acc-title">
                <h3 id="acc-title" class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-shield-check text-[15px] text-[var(--color-primary-600)]"></i>{t('profile.overview.access.title')}</h3>
                <p class="text-[13px]"><span class="text-[var(--text-tertiary)]">{t('profile.overview.access.role')}:</span> <span class="font-semibold text-[var(--text-primary)]">{profile.role}</span></p>
                <p class="text-[12.5px] text-[var(--text-secondary)]">{access.full ? t('profile.overview.access.all') : t('profile.overview.access.modules', { count: access.modules })}</p>
                {#if access.posOnly}<p class="text-[12.5px] text-[var(--color-warning-600)]">{t('profile.overview.access.posOnly')}</p>{/if}
                {#if access.locked > 0}<p class="text-[12.5px] text-[var(--text-tertiary)]">{t('profile.overview.access.locked', { count: access.locked })}</p>{/if}
              </section>
              <section class="p-4 rounded-xl border border-[var(--border-subtle)] space-y-2" aria-labelledby="out-title">
                <h3 id="out-title" class="font-display font-bold text-[14px] flex items-center gap-2"><i class="icon-building-2 text-[15px] text-[var(--color-primary-600)]"></i>{t('profile.overview.outlet.title')}</h3>
                <p class="text-[13px]"><span class="font-semibold text-[var(--text-primary)]">{session.outlet?.name}</span> <span class="text-[12px] text-[var(--text-tertiary)]">· {t('profile.overview.outlet.current')}</span></p>
                <p class="text-[12.5px] text-[var(--text-secondary)]">{t('profile.overview.outlet.total', { count: profile.outlet_count })}</p>
              </section>
            </div>
            <ProfileActivity />
          </div>

        <!-- Informasi -->
        {:else if tab === 'info'}
          <div id="ppanel-info" role="tabpanel" aria-labelledby="ptab-info">
          <form class="p-5 sm:p-6 space-y-5" onsubmit={save} autocomplete="off" novalidate>
            <header>
              <h2 class="font-display font-bold text-[16px]">{t('profile.info.title')}</h2>
              <p class="text-[13px] text-[var(--text-secondary)] mt-0.5">{t('profile.info.subtitle')}</p>
            </header>

            <div class="grid gap-4 sm:grid-cols-2">
              <label class="block text-[13px] sm:col-span-2">
                <span class="font-semibold">{t('profile.info.name')}</span>
                <input class={inputCls} bind:value={name} maxlength="100" placeholder={t('profile.info.namePh')} autocomplete="name" aria-invalid={!!fieldErr.name} />
                {#if fieldErr.name}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.name)}</span>{/if}
              </label>

              <label class="block text-[13px]">
                <span class="font-semibold">{t('profile.info.jobTitle')}</span>
                <input class={inputCls} bind:value={jobTitle} maxlength="80" placeholder={t('profile.info.jobTitlePh')} autocomplete="organization-title" />
                {#if fieldErr.job_title}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.job_title)}</span>{/if}
              </label>

              <label class="block text-[13px]">
                <span class="font-semibold">{t('profile.info.location')}</span>
                <input class={inputCls} bind:value={location} maxlength="80" placeholder={t('profile.info.locationPh')} autocomplete="address-level2" />
                {#if fieldErr.location}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.location)}</span>{/if}
              </label>

              <label class="block text-[13px]">
                <span class="font-semibold">{t('profile.info.phone')}</span>
                <input class={inputCls} bind:value={phone} type="tel" inputmode="tel" maxlength="20" placeholder={t('profile.info.phonePh')} autocomplete="tel" aria-invalid={!!fieldErr.phone} />
                {#if fieldErr.phone}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.phone)}</span>
                {:else}<span class="text-[11.5px] text-[var(--text-tertiary)]">{t('profile.info.phoneHint')}</span>{/if}
              </label>

              <label class="block text-[13px]">
                <span class="font-semibold">{t('profile.info.email')}</span>
                <input class="{inputCls} opacity-70 cursor-not-allowed" value={profile.email} readonly tabindex="-1" aria-readonly="true" />
                <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('profile.info.emailHint')}</span>
              </label>

              <div class="block text-[13px] sm:col-span-2">
                <label for="bio-md" class="font-semibold">{t('profile.info.bio')}</label>
                <div class="mt-1">
                  <MarkdownEditor id="bio-md" bind:value={bio} rows={6} maxlength={MAX_BIO} placeholder={t('profile.info.bioPh')} invalid={!!fieldErr.bio} />
                </div>
                <span class="text-[11.5px] text-[var(--text-tertiary)]">{t('profile.info.bioHint')}</span>
                {#if fieldErr.bio}<span class="block text-[12px] text-[var(--color-danger-600)]">{fieldMessage(fieldErr.bio)}</span>{/if}
              </div>
            </div>

            {#if formError}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{formError}</p>{/if}

            <div class="flex flex-wrap items-center gap-3 pt-4 border-t border-[var(--border-subtle)]">
              <button type="submit" class="btn btn-primary disabled:opacity-60" disabled={saving || !dirty || bio.length > MAX_BIO}>
                {#if saving}<i class="icon-loader-circle animate-spin text-[14px]"></i>{t('profile.info.saving')}{:else}<i class="icon-save text-[14px]"></i>{t('profile.info.save')}{/if}
              </button>
              {#if dirty}
                <button type="button" class="btn btn-outline" onclick={discard}>{t('profile.info.discard')}</button>
                <span class="text-[12.5px] text-[var(--color-warning-600)]">{t('profile.info.dirty')}</span>
              {:else if saved}
                <span role="status" class="inline-flex items-center gap-1.5 text-[12.5px] text-[var(--color-success-600)]"><i class="icon-circle-check text-[14px]"></i>{t('profile.info.saved')}</span>
              {/if}
            </div>
          </form>
          </div>

        <!-- Keamanan -->
        {:else if tab === 'security'}
          <div id="ppanel-security" role="tabpanel" aria-labelledby="ptab-security" class="p-5 sm:p-6 space-y-7">
            <form class="space-y-4 max-w-md" onsubmit={changePassword} autocomplete="off" novalidate>
              <header>
                <h2 class="font-display font-bold text-[16px]">{t('profile.security.title')}</h2>
                <p class="text-[13px] text-[var(--text-secondary)] mt-0.5">{t('profile.security.subtitle')}</p>
              </header>

              {#if pwDone}
                <p role="status" class="flex items-center gap-2 p-3 rounded-lg text-[13px] text-[var(--color-success-600)] bg-[color-mix(in_oklab,var(--color-success-600)_10%,transparent)]"><i class="icon-circle-check text-[16px]"></i>{t('profile.security.logoutNote')}</p>
              {:else}
                <label class="block text-[13px]">
                  <span class="font-semibold">{t('profile.security.current')}</span>
                  <input class={inputCls} type={showPw ? 'text' : 'password'} bind:value={current} autocomplete="current-password" aria-invalid={!!pwErr.current_password} />
                  {#if pwErr.current_password}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(pwErr.current_password)}</span>{/if}
                </label>
                <label class="block text-[13px]">
                  <span class="font-semibold">{t('profile.security.next')}</span>
                  <input class={inputCls} type={showPw ? 'text' : 'password'} bind:value={next} autocomplete="new-password" aria-invalid={!!pwErr.password} />
                  {#if pwErr.password}<span class="text-[12px] text-[var(--color-danger-600)]">{fieldMessage(pwErr.password)}</span>
                  {:else}<span class="text-[11.5px] text-[var(--text-tertiary)]">{t('profile.security.hint')}</span>{/if}
                </label>
                {#if strength >= 0}
                  <div class="flex items-center gap-3" aria-live="polite">
                    <div class="flex gap-1 flex-1" aria-hidden="true">
                      {#each [0, 1, 2] as i (i)}
                        <span class="h-1.5 flex-1 rounded-full" style="background:{i <= strength ? strengthColor[strength] : 'var(--surface-overlay)'}"></span>
                      {/each}
                    </div>
                    <span class="text-[12px] font-semibold" style="color:{strengthColor[strength]}">{t('profile.security.strength.label')}: {t(`profile.security.strength.${strengthKey}`)}</span>
                  </div>
                {/if}
                <label class="block text-[13px]">
                  <span class="font-semibold">{t('profile.security.confirm')}</span>
                  <input class={inputCls} type={showPw ? 'text' : 'password'} bind:value={confirm} autocomplete="new-password" />
                </label>
                <label class="inline-flex items-center gap-2 text-[12.5px] cursor-pointer"><input type="checkbox" bind:checked={showPw} />{t('profile.security.show')}</label>
                {#if pwError}<p role="alert" class="text-[12.5px] text-[var(--color-danger-600)]">{pwError}</p>{/if}
                <p class="text-[12px] text-[var(--text-tertiary)] flex gap-2"><i class="icon-lock text-[13px] mt-0.5"></i>{t('profile.security.logoutNote')}</p>
                <button type="submit" class="btn btn-primary disabled:opacity-60" disabled={pwBusy}>
                  {#if pwBusy}<i class="icon-loader-circle animate-spin text-[14px]"></i>{t('profile.security.saving')}{:else}<i class="icon-key-round text-[14px]"></i>{t('profile.security.submit')}{/if}
                </button>
              {/if}
            </form>

            <div class="pt-6 border-t border-[var(--border-subtle)]">
              <ProfileMfa onchange={reloadQuiet} />
            </div>

            <div class="pt-6 border-t border-[var(--border-subtle)] space-y-3">
              <h3 class="font-display font-bold text-[15px]">{t('profile.security.accounts.title')}</h3>
              <ul class="grid gap-3 2xl:grid-cols-2">
                <li class="flex items-start gap-3 p-3.5 rounded-xl border border-[var(--border-subtle)]">
                  <span class="grid place-items-center size-9 rounded-lg bg-[var(--surface-sunken)]"><i class="icon-globe text-[16px]"></i></span>
                  <div class="min-w-0">
                    <p class="text-[13.5px] font-semibold flex items-center gap-2">{t('profile.security.accounts.google')}
                      <span class="text-[11px] px-2 h-5 inline-flex items-center whitespace-nowrap rounded-full {profile.google_linked ? 'text-[var(--color-success-600)] bg-[color-mix(in_oklab,var(--color-success-600)_12%,transparent)]' : 'text-[var(--text-tertiary)] bg-[var(--surface-sunken)]'}">{profile.google_linked ? t('profile.security.accounts.googleLinked') : t('profile.security.accounts.googleNotLinked')}</span>
                    </p>
                    <p class="text-[12px] text-[var(--text-secondary)] mt-0.5">{t('profile.security.accounts.googleNote')}</p>
                  </div>
                </li>
                <li class="flex items-start gap-3 p-3.5 rounded-xl border border-[var(--border-subtle)]">
                  <span class="grid place-items-center size-9 rounded-lg bg-[var(--surface-sunken)]"><i class="icon-fingerprint text-[16px]"></i></span>
                  <div class="min-w-0">
                    <p class="text-[13.5px] font-semibold">{t('profile.security.accounts.pin')}</p>
                    <p class="text-[12px] text-[var(--text-secondary)] mt-0.5">{t('profile.security.accounts.pinNote')}</p>
                    <a href="/pin" class="inline-block mt-1.5 text-[12.5px] font-semibold text-[var(--color-primary-600)]">{t('profile.security.accounts.pinManage')}</a>
                  </div>
                </li>
              </ul>
            </div>
          </div>

        <!-- Preferensi -->
        {:else}
          <div id="ppanel-prefs" role="tabpanel" aria-labelledby="ptab-prefs" class="p-5 sm:p-6 space-y-4">
            <header>
              <h2 class="font-display font-bold text-[16px]">{t('profile.prefs.title')}</h2>
              <p class="text-[13px] text-[var(--text-secondary)] mt-0.5">{t('profile.prefs.subtitle')}</p>
            </header>
            <div role="radiogroup" aria-label={t('profile.prefs.language')} class="grid gap-3 sm:grid-cols-3">
              {#each localeCodes as code (code)}
                <button
                  type="button"
                  role="radio"
                  aria-checked={i18n.locale === code}
                  class="flex items-center gap-3 p-3.5 rounded-xl border text-start transition-colors {i18n.locale === code ? 'border-[var(--color-primary-600)] bg-[color-mix(in_oklab,var(--color-primary-600)_8%,transparent)]' : 'border-[var(--border-subtle)] hover:bg-[var(--surface-sunken)]'}"
                  onclick={() => setLocale(code)}
                >
                  <span class="grid place-items-center size-9 rounded-lg text-[12px] font-bold bg-[var(--surface-sunken)]">{LOCALES[code].short}</span>
                  <span class="flex-1 text-[13.5px] font-semibold">{LOCALES[code].name}</span>
                  {#if i18n.locale === code}<i class="icon-check text-[16px] text-[var(--color-primary-600)]"></i>{/if}
                </button>
              {/each}
            </div>
            <p class="text-[12px] text-[var(--text-tertiary)]">{t('profile.prefs.languageNote')}</p>
          </div>
        {/if}
      </section>
    </div>
  {/if}
</main>
