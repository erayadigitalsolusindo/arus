<script lang="ts">
  import { platformApi } from '#lib/platform/api.ts';
  import { errorMessage } from '#lib/i18n/errors.ts';

  const MAX = 100;
  // Satu baris per tenant: "Nama Toko, email@gmail.com[, Nama Pemilik]" (pisah koma atau tab, bisa tempel dari spreadsheet).
  let text = $state('');
  let busy = $state(false);
  let error = $state('');
  let results = $state<{ row: number; email: string; ok: boolean; error?: string }[]>([]);

  // CSV dari Excel ("Simpan sebagai CSV"): isinya dimuat ke kotak teks agar bisa diperiksa dulu. File .xlsx belum didukung (tempel saja dari Excel).
  async function onFile(e: Event) {
    const f = (e.currentTarget as HTMLInputElement).files?.[0];
    if (f) text = await f.text();
  }

  async function submit() {
    error = '';
    results = [];
    const items = text
      .split('\n')
      .map((l) => l.split(/[\t,;]/).map((x) => x.trim()))
      .filter((c) => c.some(Boolean) && c[1]?.includes('@')) // baris judul kolom (tanpa @) terlewati
      .map((c) => ({ business_name: c[0] ?? '', email: c[1] ?? '', owner_name: c[2] ?? '' }));
    if (items.length === 0 || items.length > MAX) {
      error = `Isi 1–${MAX} baris.`;
      return;
    }
    busy = true;
    try {
      results = (await platformApi.bulkCreateTenants(items)).results;
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex items-end justify-between gap-3">
  <div>
    <h1 class="font-display font-bold text-[22px] tracking-tight">Daftar tenant massal</h1>
    <p class="text-[12.5px] mt-1 text-[var(--text-tertiary)] max-w-2xl">
      Satu baris per tenant: <span class="font-mono">Nama Toko, email, Nama Pemilik (opsional)</span>. Tenant langsung aktif. Pemilik masuk dengan Google
      (email yang sama) atau password awal <span class="font-mono">SayaCintaIndonesia1945</span> lalu menggantinya.
    </p>
  </div>
  <a href="/platform/tenants" class="btn btn-outline !text-[12.5px]">Kembali</a>
</div>

<div class="surface-card space-y-3">
  <input type="file" accept=".csv,.txt,text/csv" onchange={onFile} aria-label="Impor file CSV" class="text-[12.5px]" />
  <textarea bind:value={text} rows="10" class="w-full field-control font-mono text-[12.5px]" aria-label="Daftar tenant" placeholder="Warung Bu Ani, ani@gmail.com, Ani&#10;Toko Maju, budi@gmail.com"></textarea>
  {#if error}<div role="alert" class="rounded-lg px-3 py-2.5 text-[12.5px] badge-danger">{error}</div>{/if}
  <button type="button" class="btn btn-primary disabled:opacity-60" disabled={busy || !text.trim()} onclick={submit}>{busy ? 'Memproses…' : 'Daftarkan'}</button>
</div>

{#if results.length}
  <div class="surface-card !p-0 overflow-x-auto">
    <table class="w-full text-[12.5px]">
      <tbody>
        {#each results as r (r.row)}
          <tr class="border-t border-[var(--border-subtle)]">
            <td class="px-4 py-2.5 tabular-nums">{r.row}</td>
            <td class="px-4 py-2.5">{r.email}</td>
            <td class="px-4 py-2.5 {r.ok ? '' : 'text-[var(--color-danger-600,#dc2626)]'}">{r.ok ? 'Berhasil' : r.error === 'EMAIL_TAKEN' ? 'Email sudah terdaftar' : `Gagal (${r.error})`}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
