/** Action Svelte: fokus ke elemen setelah dipasang. Ditunda satu tick karena Modal memfokuskan dialognya sendiri saat dibuka. */
export function focusOnMount(node: HTMLElement) {
  const timer = setTimeout(() => node.focus(), 0);
  return { destroy: () => clearTimeout(timer) };
}
