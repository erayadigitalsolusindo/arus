// Foto profil pengguna yang sedang masuk, dipakai bersama header dan halaman profil agar selalu sinkron.
import { profileApi, profileImageUrl, type Profile } from './api.ts';

class MeState {
  profile = $state<Profile | null>(null);
  avatarSrc = $state('');
  #loading: Promise<void> | null = null;

  /** Menyimpan profil terbaru dan menyegarkan thumbnail avatar. */
  set(p: Profile | null) {
    this.profile = p;
    if (!p?.avatar_id) {
      this.avatarSrc = '';
      return;
    }
    const id = p.avatar_id;
    profileImageUrl(p.id, id, 'avatar', 'thumb').then(
      (u) => this.profile?.avatar_id === id && (this.avatarSrc = u),
      () => (this.avatarSrc = '')
    );
  }

  /** Memuat sekali (header memanggilnya); kegagalan diabaikan: avatar kembali ke inisial. */
  ensure(): Promise<void> {
    this.#loading ??= profileApi.get().then((p) => this.set(p), () => void (this.#loading = null));
    return this.#loading;
  }

  reset() {
    this.profile = null;
    this.avatarSrc = '';
    this.#loading = null;
  }
}

export const me = new MeState();
