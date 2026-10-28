// Menu yang terkunci paket langganan: klik di sidebar membuka dialog ajakan upgrade (bukan pindah halaman).
import type { MessageKey } from '#lib/i18n/index.ts';

export const lockPrompt = $state<{ labelKey: MessageKey | null }>({ labelKey: null });

export function showLockPrompt(labelKey: MessageKey) {
  lockPrompt.labelKey = labelKey;
}
