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
