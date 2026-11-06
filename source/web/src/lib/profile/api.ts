// Profil pribadi pengguna yang sedang masuk (backend/internal/profile). Foto butuh token → diunduh lewat apiBlob.
import { api, apiBlob } from '#lib/api/client.ts';

export type Profile = {
  id: string;
  name: string;
  email: string;
  phone: string;
  job_title: string;
  bio: string;
  location: string;
  avatar_id: string | null;
  banner_id: string | null;
  role: string;
  tenant: string;
  tenant_code: string;
  outlet_count: number;
  email_verified: boolean;
  mfa_enabled: boolean;
  actions_30d: number;
  logins_30d: number;
  google_linked: boolean;
  last_login_at: string | null;
  created_at: string;
  /** 0–100, dihitung server. */
  completeness: number;
  /** Kunci isian yang belum diisi, urut prioritas: avatar, phone, job_title, location, bio, banner, email_verified. */
  missing: string[];
};

export type Activity = { id: number; action: string; entity: string; entity_id: string; ip: string; created_at: string };
export type ActivityPage = { data: Activity[]; next: string };
export type MfaStatus = { enabled: boolean; pending: boolean; recovery_remaining: number };
export type MfaSetup = { secret: string; url: string };

export type ProfileInput = Pick<Profile, 'name' | 'phone' | 'job_title' | 'bio' | 'location'>;
export type ImageKind = 'avatar' | 'banner';

export const MAX_BIO = 1000;
export const MAX_IMAGE_BYTES = 10 * 1024 * 1024;

const json = (v: unknown) => JSON.stringify(v);

export const profileApi = {
  get: () => api<Profile>('/profile/'),
  update: (input: ProfileInput) => api<Profile>('/profile/', { method: 'PUT', body: json(input) }),
  /** Berhasil → semua sesi dicabut (server); pemanggil harus mengarahkan ke halaman masuk. */
  changePassword: (current_password: string, password: string) =>
    api<void>('/profile/password', { method: 'POST', body: json({ current_password, password }) }),
  upload: (kind: ImageKind, file: File) => {
    const form = new FormData();
    form.append('file', file);
    return api<Profile>(`/profile/${kind}`, { method: 'POST', body: form });
  },
  activity: (cursor = '', limit = 15) => api<ActivityPage>(`/profile/activity?limit=${limit}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`),
  mfa: {
    status: () => api<MfaStatus>('/profile/2fa/'),
    begin: () => api<MfaSetup>('/profile/2fa/begin', { method: 'POST' }),
    enable: (code: string) => api<{ recovery_codes: string[] }>('/profile/2fa/enable', { method: 'POST', body: json({ code }) }),
    recovery: (code: string) => api<{ recovery_codes: string[] }>('/profile/2fa/recovery-codes', { method: 'POST', body: json({ code }) }),
    disable: (password: string, code: string) => api<void>('/profile/2fa/disable', { method: 'POST', body: json({ password, code }) })
  },
  removeImage: (kind: ImageKind) => api<Profile>(`/profile/${kind}`, { method: 'DELETE' })
};

// Id gambar berubah tiap diganti, jadi satu unduhan per id cukup.
const urlCache = new Map<string, Promise<string>>();

export function profileImageUrl(userId: string, imageId: string, kind: ImageKind, size: 'thumb' | 'full'): Promise<string> {
  const key = `${imageId}:${size}`;
  let p = urlCache.get(key);
  if (!p) {
    p = apiBlob(`/profile/${userId}/${kind}?size=${size}`).then((b) => URL.createObjectURL(b));
    p.catch(() => urlCache.delete(key));
    urlCache.set(key, p);
  }
  return p;
}
