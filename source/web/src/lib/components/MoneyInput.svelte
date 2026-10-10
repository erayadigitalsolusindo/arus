<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { i18n } from '#lib/i18n/index.ts';

  type Props = Omit<HTMLInputAttributes, 'value' | 'type' | 'inputmode'> & {
    /** Nilai kanonik: string desimal bertitik ("1250000.5"), kosong bila belum diisi. Inilah yang dikirim ke API. */
    value?: string;
    /** Maks digit di belakang koma (uang 2, qty 3). */
    decimals?: number;
    /** Saat kolom ditinggalkan, tampilan dipadatkan sampai `decimals` digit ("1.250.000,00"). Nilai kanonik tidak berubah. */
    pad?: boolean;
  };

  let { value = $bindable(''), decimals = 2, pad = true, onblur, onfocus: onfocusProp, oninput: oninputProp, class: cls = '', ...rest }: Props = $props();

  const MAX_INT_DIGITS = 15;

  // Pemisah mengikuti bahasa aktif: id → "." ribuan & "," desimal; en → "," ribuan & ".".
  const seps = $derived.by(() => {
    const parts = new Intl.NumberFormat(i18n.intl).formatToParts(1234567.5);
    return {
      group: parts.find((p) => p.type === 'group')?.value ?? '.',
      decimal: parts.find((p) => p.type === 'decimal')?.value ?? ','
    };
  });

  const trimZeros = (c: string) => (c.includes('.') ? c.replace(/\.?0+$/, '') : c);
  const group = (int: string) => int.replace(/\B(?=(\d{3})+(?!\d))/g, seps.group);

  /** kanonik "123.45" → tampilan. `trailing` mempertahankan koma yang baru diketik. */
  function show(canon: string, padTo = 0, trailing = false): string {
    if (canon === '') return '';
    const [int, frac = ''] = canon.split('.');
    let out = group(int || '0');
    const f = padTo > 0 ? frac.padEnd(padTo, '0') : frac;
    if (f !== '' || trailing) out += seps.decimal + f;
    return out;
  }

  /** Teks mentah → { kanonik, ada koma di ujung }. Hanya angka dan satu pemisah desimal yang lolos. */
  function parse(raw: string, pasted: boolean): { canon: string; trailing: boolean } {
    let text = raw;
    // Tempelan gaya kanonik ("1250000.50" saat bahasa id): satu titik + 1–3 digit di ujung = desimal.
    if (pasted && seps.decimal !== '.' && /^\D*\d+\.\d{1,2}\D*$/.test(text) && !text.includes(seps.decimal)) text = text.replace('.', seps.decimal);
    let int = '';
    let frac = '';
    let seenDec = false;
    for (const ch of text) {
      if (ch >= '0' && ch <= '9') {
        if (seenDec) frac += ch;
        else int += ch;
      } else if (ch === seps.decimal && decimals > 0 && !seenDec) {
        seenDec = true;
      }
    }
    if (int === '' && !seenDec && frac === '') return { canon: '', trailing: false };
    int = int.replace(/^0+(?=\d)/, '').slice(0, MAX_INT_DIGITS);
    frac = frac.slice(0, decimals);
    return { canon: frac ? `${int || '0'}.${frac}` : int || '0', trailing: seenDec && frac === '' };
  }

  let text = $state('');
  let focused = $state(false);

  // Nilai dari luar (reset, data server, tombol "uang pas") atau ganti bahasa (pemisah berubah) → tampilan;
  // abaikan bila setara dengan yang sedang diketik.
  $effect(() => {
    const ext = value;
    const cur = parse(text, false).canon;
    if (trimZeros(ext) !== trimZeros(cur)) text = show(ext, focused ? 0 : pad ? decimals : 0);
    else if (!focused && pad && ext !== '') text = show(ext, decimals);
  });

  function oninput(e: Event & { currentTarget: HTMLInputElement }) {
    const el = e.currentTarget;
    const pasted = (e as unknown as InputEvent).inputType === 'insertFromPaste';
    const caret = el.selectionStart ?? el.value.length;
    // Jumlah karakter bermakna (angka/koma) sebelum kursor, untuk memulihkan posisi setelah diformat ulang.
    const before = [...el.value.slice(0, caret)].filter((c) => (c >= '0' && c <= '9') || c === seps.decimal).length;
    const { canon, trailing } = parse(el.value, pasted);
    const next = show(canon, 0, trailing);
    el.value = next;
    text = next;
    value = canon;
    let pos = 0;
    let seen = 0;
    while (pos < next.length && seen < before) {
      const c = next[pos];
      if ((c >= '0' && c <= '9') || c === seps.decimal) seen++;
      pos++;
    }
    el.setSelectionRange(pos, pos);
    oninputProp?.(e);
  }

  function onfocus(e: FocusEvent & { currentTarget: EventTarget & HTMLInputElement }) {
    focused = true;
    onfocusProp?.(e);
  }
  function handleBlur(e: FocusEvent & { currentTarget: EventTarget & HTMLInputElement }) {
    focused = false;
    const { canon } = parse(text, false);
    text = pad ? show(canon, decimals) : show(canon);
    onblur?.(e);
  }
</script>

<input
  {...rest}
  class={cls}
  inputmode={decimals > 0 ? 'decimal' : 'numeric'}
  autocomplete="off"
  value={text}
  {oninput}
  {onfocus}
  onblur={handleBlur}
/>
