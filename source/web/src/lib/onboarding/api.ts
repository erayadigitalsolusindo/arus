// Klien API wizard penyiapan awal.
import { api } from '#lib/api/client.ts';

export type OnboardingStatus = {
  completed: boolean;
  email_verified: boolean;
  account_count: number;
  methods_unlinked: number;
  item_count: number;
  outlet_count: number;
};

export const onboarding = {
  status: () => api<OnboardingStatus>('/onboarding/'),
  complete: (skipped: boolean) => api<void>('/onboarding/complete', { method: 'POST', body: JSON.stringify({ skipped }) })
};
