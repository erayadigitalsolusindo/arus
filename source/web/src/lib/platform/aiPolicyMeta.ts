// Metadata kebijakan HalloArus yang dipakai bersama oleh formulir, panel "efektif", dan daftar pengecualian.
import type { MessageKey } from '#lib/i18n/index.ts';
import type { AiPolicyValues } from '#lib/platform/api.ts';

export type BoolKey = 'enabled' | 'allow_write' | 'allow_cost' | 'allow_images' | 'allow_history' | 'allow_mcp' | 'allow_byok' | 'allow_custom' | 'free_enabled';
export type NumKey = 'chat_limit' | 'max_range_days' | 'free_daily_limit';

export const FEATURES: { key: BoolKey; icon: string; label: MessageKey; hint: MessageKey; short: MessageKey; can: MessageKey; risky?: boolean }[] = [
  { key: 'enabled', icon: 'power', label: 'platform.ai.fEnabled', hint: 'platform.ai.fEnabledHint', short: 'platform.ai.shEnabled', can: 'platform.ai.cEnabled' },
  { key: 'allow_write', icon: 'pencil-line', label: 'platform.ai.fWrite', hint: 'platform.ai.fWriteHint', short: 'platform.ai.shWrite', can: 'platform.ai.cWrite', risky: true },
  { key: 'allow_cost', icon: 'banknote', label: 'platform.ai.fCost', hint: 'platform.ai.fCostHint', short: 'platform.ai.shCost', can: 'platform.ai.cCost', risky: true },
  { key: 'allow_images', icon: 'image-plus', label: 'platform.ai.fImages', hint: 'platform.ai.fImagesHint', short: 'platform.ai.shImages', can: 'platform.ai.cImages' },
  { key: 'allow_history', icon: 'history', label: 'platform.ai.fHistory', hint: 'platform.ai.fHistoryHint', short: 'platform.ai.shHistory', can: 'platform.ai.cHistory' },
  { key: 'allow_mcp', icon: 'plug', label: 'platform.ai.fMcp', hint: 'platform.ai.fMcpHint', short: 'platform.ai.shMcp', can: 'platform.ai.cMcp', risky: true },
  { key: 'allow_byok', icon: 'key-round', label: 'platform.ai.fByok', hint: 'platform.ai.fByokHint', short: 'platform.ai.shByok', can: 'platform.ai.cByok' },
  { key: 'allow_custom', icon: 'terminal', label: 'platform.ai.fCustom', hint: 'platform.ai.fCustomHint', short: 'platform.ai.shCustom', can: 'platform.ai.cCustom' },
  { key: 'free_enabled', icon: 'gift', label: 'platform.ai.fFree', hint: 'platform.ai.fFreeHint', short: 'platform.ai.shFree', can: 'platform.ai.cFree' }
];

export const DOMAIN_META: Record<string, { icon: string; label: MessageKey }> = {
  sales: { icon: 'shopping-cart', label: 'platform.ai.dSales' },
  members: { icon: 'users', label: 'platform.ai.dMembers' },
  stock: { icon: 'package', label: 'platform.ai.dStock' },
  debts: { icon: 'hand-coins', label: 'platform.ai.dDebts' },
  purchases: { icon: 'receipt', label: 'platform.ai.dPurchases' },
  cash: { icon: 'wallet', label: 'platform.ai.dCash' },
  accounting: { icon: 'landmark', label: 'platform.ai.dAccounting' }
};

export const KEYS = ['enabled', 'domains', 'allow_write', 'allow_cost', 'allow_byok', 'allow_custom', 'allow_images', 'allow_history', 'allow_mcp', 'free_enabled', 'free_daily_limit', 'free_model', 'chat_limit', 'max_range_days'] as const;

export const pick = (r: Partial<AiPolicyValues>): AiPolicyValues => Object.fromEntries(KEYS.map((k) => [k, r[k] ?? null])) as AiPolicyValues;
export const emptyPolicy = (): AiPolicyValues => pick({});

/** Postur: berapa kontrol (fitur + domain) yang dibatasi. */
export function posture(v: AiPolicyValues, allDomains: string[]) {
  const total = FEATURES.length + allDomains.length;
  const blockedFeatures = FEATURES.filter((f) => v[f.key] === false).length;
  const blockedDomains = allDomains.filter((d) => !(v.domains ?? []).includes(d)).length;
  const blocked = blockedFeatures + blockedDomains;
  const ratio = total ? blocked / total : 0;
  const level: 'open' | 'balanced' | 'strict' = ratio < 0.2 ? 'open' : ratio < 0.5 ? 'balanced' : 'strict';
  return { total, blocked, level };
}

/** Nilai akhir pengecualian: field pengecualian yang terisi menimpa global. */
export const mergeValues = (g: AiPolicyValues, o: AiPolicyValues): AiPolicyValues => Object.fromEntries(KEYS.map((k) => [k, o[k] ?? g[k]])) as AiPolicyValues;

/** Jumlah field yang diubah pada pengecualian. */
export const changedCount = (o: AiPolicyValues) => KEYS.filter((k) => o[k] !== null && o[k] !== undefined).length;
