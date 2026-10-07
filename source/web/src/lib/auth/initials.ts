/** Inisial untuk avatar: huruf pertama dari maksimal dua kata pertama. */
export function initials(name: string | null | undefined): string {
  const words = (name ?? '').trim().split(/\s+/).filter(Boolean);
  return words
    .slice(0, 2)
    .map((w) => Array.from(w)[0]!.toUpperCase())
    .join('');
}
