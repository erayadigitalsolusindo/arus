package storefront

import (
	"context"
	"strings"
	"testing"
)

func TestRenderExtraMarkers(t *testing.T) {
	src := newFake()
	tmpl := `{jumlahmember}|{jumlahkategori}|{jumlahoutlet}|{tanggal}` +
		`|{#level}{nomor}.{namalevel}:{minpoin}:{belanjaperpoin}:{nilaipoin};{/level}` +
		`|{#kupon}{kodekupon}:{namakupon}:{potongan}:{maksimalpotongan}:{minbelanja}:{berlakuhingga}:{sisakuota}{/kupon}` +
		`|{#outlet}{namaoutlet},{/outlet}|{#pembayaran}{namapembayaran}={jenispembayaran}{/pembayaran}` +
		`|{namalevel}{kodekupon}{namaoutlet}`
	out, err := Render(context.Background(), tmpl, src, site())
	if err != nil {
		t.Fatal(err)
	}
	want := `1.234.567|2|2|7 November 2026` +
		`|1.Reguler:0:Rp10.000:Rp100;2.Gold:500:Rp6.000:Rp100;` +
		`|HEMAT10:&lt;i&gt;Hemat&lt;/i&gt;:10%:Rp25.000:Rp100.000:Tanpa batas:3` +
		`|Pusat,Cabang,|Tunai=Tunai` +
		`|` // penanda blok di luar blok induknya kosong, bukan teks mentah
	if out != want {
		t.Fatalf("got:  %s\nwant: %s", out, want)
	}
}

// Penanda yang tidak dipakai templat tidak boleh memicu query tambahan (halaman publik harus ringan).
func TestExtraMarkersOnlyQueryWhenUsed(t *testing.T) {
	src := newFake()
	if _, err := Render(context.Background(), `{namatoko}{#barang limit=1}{namabarang}{/barang}`, src, site()); err != nil {
		t.Fatal(err)
	}
	if src.queries != 1 {
		t.Fatalf("query = %d, want 1", src.queries)
	}
	src = newFake()
	if _, err := Render(context.Background(), strings.Repeat(`{#level}{namalevel}{/level}{jumlahmember}{jumlahoutlet}{#outlet}{namaoutlet}{/outlet}`, 20), src, site()); err != nil {
		t.Fatal(err)
	}
	if src.queries != 3 { // level + member + outlet, masing-masing sekali
		t.Fatalf("query = %d, want 3", src.queries)
	}
}
