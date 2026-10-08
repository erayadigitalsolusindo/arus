import type { Action } from 'svelte/action';

type Options = { max: number; min?: number; /** Berubah saat teks berubah, memicu ukur ulang. */ text: string };

/**
 * Perkecil font satu baris teks otomatis agar muat selebar induknya (mis. total besar di kasir).
 * Ukuran maksimum = `max` px; tidak lebih kecil dari `min` px.
 */
export const fitText: Action<HTMLElement, Options> = (node, initial) => {
  let opts = initial;
  const parent = node.parentElement;

  function fit() {
    if (!parent) return;
    const cs = getComputedStyle(parent);
    const avail = parent.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
    node.style.fontSize = `${opts.max}px`;
    if (avail <= 0) return;
    const need = node.scrollWidth;
    if (need > avail) node.style.fontSize = `${Math.max(opts.min ?? 14, Math.floor((opts.max * avail) / need))}px`;
  }

  const ro = new ResizeObserver(fit);
  if (parent) ro.observe(parent);
  fit();

  return {
    update(next) {
      opts = next;
      fit();
    },
    destroy() {
      ro.disconnect();
    }
  };
};
