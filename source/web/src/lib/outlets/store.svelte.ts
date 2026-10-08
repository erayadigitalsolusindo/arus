// Daftar outlet yang boleh dipilih pengguna (untuk pemilih outlet di sidebar). Dipakai bersama agar halaman Outlet
// dapat memuat ulang setelah membuat/mengubah outlet tanpa memuat ulang halaman.
import { outlets, type Outlet } from './api.ts';

class OutletList {
  items = $state<Outlet[]>([]);
}

export const accessibleOutlets = new OutletList();

/** Memuat ulang daftar; gagal = daftar lama dipertahankan (pemilih tetap menampilkan outlet aktif sesi). */
export async function refreshOutlets(): Promise<void> {
  try {
    accessibleOutlets.items = (await outlets.accessible()).outlets;
  } catch {
    /* abaikan */
  }
}

/**
 * Mode lihat 'Semua Cabang' (hanya untuk layar baca). Pilihan pengguna disimpan di browser; berlaku efektif hanya
 * pada halaman yang menyatakan dukungan lewat `supportAllOutlets()` — layar yang menulis data tetap satu cabang aktif.
 */
class OutletScope {
  #pref = $state(read());
  #count = 0; // bukan state: dibaca-tulis di dalam $effect
  #supported = $state(false);

  /** Pengguna memilih 'Semua Cabang' (belum tentu berlaku di halaman ini). */
  get preferAll() {
    return this.#pref;
  }
  /** Halaman saat ini mendukung mode semua cabang. */
  get supported() {
    return this.#supported;
  }
  /** Mode semua cabang berlaku di halaman ini. */
  get all() {
    return this.#pref && this.#supported;
  }
  setAll(v: boolean) {
    this.#pref = v;
    try {
      localStorage.setItem(KEY, v ? '1' : '0');
    } catch {
      /* abaikan */
    }
  }
  /** @internal */
  _support(delta: number) {
    this.#count += delta;
    this.#supported = this.#count > 0;
  }
}

const KEY = 'aciraba.outletScope';
function read(): boolean {
  try {
    return localStorage.getItem(KEY) === '1';
  } catch {
    return false;
  }
}

export const outletScope = new OutletScope();

/** Dipanggil dari komponen halaman yang mendukung mode semua cabang; dicabut otomatis saat halaman ditutup. */
export function supportAllOutlets(): void {
  $effect(() => {
    outletScope._support(1);
    return () => outletScope._support(-1);
  });
}
