// Penjaga "perubahan belum disimpan". Form mendaftarkan fungsi `isDirty` (dibaca saat dibutuhkan, bukan disalin ke state,
// agar nilainya selalu mutakhir tepat sebelum navigasi). Tindakan yang akan membuang form lewat `request()`.
class Guard {
  #isDirty: (() => boolean) | null = null;
  /** Tindakan yang menunggu keputusan pengguna; non-null = dialog tampil. */
  pending = $state<(() => void) | null>(null);

  get dirty(): boolean {
    return this.#isDirty?.() ?? false;
  }

  /** Dipanggil form saat dipasang; mengembalikan fungsi pelepas untuk cleanup `$effect`. */
  register(isDirty: () => boolean): () => void {
    this.#isDirty = isDirty;
    return () => {
      if (this.#isDirty === isDirty) this.#isDirty = null;
      this.pending = null;
    };
  }

  /** Jalankan `action` sekarang bila bersih; bila kotor, tampilkan dialog dan tunda. */
  request(action: () => void) {
    if (this.dirty) this.pending = action;
    else action();
  }

  confirm() {
    const action = this.pending;
    this.pending = null;
    // Lepas penjaga lebih dulu agar navigasi yang dipicu `action` tidak dicegat lagi.
    this.#isDirty = null;
    action?.();
  }

  cancel() {
    this.pending = null;
  }
}

export const guard = new Guard();
