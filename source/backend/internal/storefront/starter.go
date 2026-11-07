package storefront

// Templat awal yang ditawarkan editor saat tenant belum pernah menyimpan. Hanya memakai class Tailwind dan penanda
// templat, jadi sekaligus contoh sintaks. Ditest agar lolos penyaring tanpa satu pun peringatan.

const StarterHTML = `<header class="bg-emerald-600 text-white">
  <div class="mx-auto max-w-5xl px-6 py-14 text-center">
    <h1 class="text-4xl font-bold tracking-tight">{namatoko}</h1>
    <p class="mt-3 text-lg text-emerald-50">Belanja mudah, harga bersahabat.</p>
    <a href="#produk" class="mt-6 inline-block rounded-full bg-white px-6 py-2.5 font-semibold text-emerald-700 hover:bg-emerald-50">Lihat produk</a>
  </div>
</header>

<main id="produk" class="mx-auto max-w-5xl px-6 py-12">
  <h2 class="text-2xl font-bold text-slate-800">Produk pilihan</h2>
  <div class="mt-6 grid grid-cols-2 gap-5 md:grid-cols-4">
    {#barang limit=8 urut=terbaru}
    <article class="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
      <img src="{gambar}" alt="{namabarang}" class="aspect-square w-full bg-slate-100 object-cover">
      <div class="p-3">
        <h3 class="font-semibold text-slate-800">{namabarang}</h3>
        <p class="text-xs text-slate-500">{kategoribarang}</p>
        <p class="mt-1 font-bold text-emerald-700">{harga}</p>
      </div>
    </article>
    {/barang}
  </div>

  {#kategori limit=3}
  <h2 class="mt-12 text-2xl font-bold text-slate-800">{namakategori}</h2>
  <ul class="mt-4 divide-y divide-slate-200 rounded-xl border border-slate-200 bg-white">
    {#barang kategori=ini limit=5}
    <li class="flex items-center justify-between px-4 py-3">
      <span class="text-slate-700">{namabarang}</span>
      <span class="font-semibold text-slate-900">{harga}</span>
    </li>
    {/barang}
  </ul>
  {/kategori}
</main>

<footer class="border-t border-slate-200 bg-slate-50 py-6 text-center text-sm text-slate-500">
  &copy; {tahun} {namatoko}
</footer>`

const StarterCSS = `body { background: #f8fafc; font-family: system-ui, sans-serif; }`
