<script lang="ts">
  // Pemilih rentang tanggal ("between") bergaya template: flatpickr mode range, dua bulan berdampingan.
  // CSS `.flatpickr-*` sudah ada di dreams-core.css; nilai from/to berbentuk YYYY-MM-DD (hari kalender, bukan zona waktu).
  import { onMount } from 'svelte';
  import flatpickr from 'flatpickr';
  import { Indonesian } from 'flatpickr/dist/l10n/id.js';
  import { i18n, t } from '#lib/i18n/index.ts';

  let {
    from = $bindable(''),
    to = $bindable(''),
    onchange,
    ariaLabel,
    placeholder = '',
    class: cls = ''
  }: { from?: string; to?: string; onchange?: (from: string, to: string) => void; ariaLabel?: string; placeholder?: string; class?: string } = $props();

  let input = $state<HTMLInputElement>();
  let fp: flatpickr.Instance | undefined;
  let isOpen = false;

  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  // flatpickr mem-parse string memakai dateFormat (d-m-Y), jadi berikan objek Date lokal, bukan string YYYY-MM-DD.
  const parse = (s: string) => {
    const [y, m, d] = s.split('-').map(Number);
    return new Date(y, m - 1, d);
  };
  // Selalu dua ujung (hari tunggal = [d, d]) agar klik berikutnya memulai rentang baru, bukan memperpanjang yang lama.
  const value = () => (from ? [from, to || from] : []);
  const dates = () => value().map(parse);

  onMount(() => {
    if (!input) return;
    fp = flatpickr(input, {
      mode: 'range',
      showMonths: 2,
      dateFormat: 'd-m-Y',
      locale: i18n.locale === 'id' ? Indonesian : undefined,
      defaultDate: dates(),
      disableMobile: true,
      onOpen: () => (isOpen = true),
      onClose: () => (isOpen = false),
      onChange: (dates) => {
        // Baru satu ujung terpilih: tunggu ujung kedua. Klik ganda pada hari yang sama = satu hari.
        if (dates.length < 2) return;
        from = ymd(dates[0]);
        to = ymd(dates[1]);
        onchange?.(from, to);
      }
    });
    // Escape menutup kalender saja, bukan dialog di belakangnya.
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        e.preventDefault();
        fp?.close();
      }
    };
    window.addEventListener('keydown', esc, true);
    return () => {
      window.removeEventListener('keydown', esc, true);
      fp?.destroy();
    };
  });

  // Nilai dari luar (mis. server memilih "hari ini") ikut tampil tanpa memicu onchange.
  $effect(() => {
    const v = value();
    if (!fp || isOpen) return;
    const cur = fp.selectedDates.map(ymd);
    if (cur.length === v.length && cur.every((x, i) => x === v[i])) return;
    fp.setDate(dates(), false);
  });
</script>

<div class="relative {cls}">
  <i class="icon-calendar absolute start-3 top-1/2 -translate-y-1/2 text-[14px] text-[var(--text-tertiary)] pointer-events-none"></i>
  <input
    bind:this={input}
    type="text"
    readonly
    aria-label={ariaLabel}
    placeholder={placeholder || t('common.dateRange')}
    class="btn btn-outline w-full cursor-pointer text-[12.5px] focus:!outline-none"
    style="padding-inline-start: 2.25rem; text-align: start"
  />
</div>
