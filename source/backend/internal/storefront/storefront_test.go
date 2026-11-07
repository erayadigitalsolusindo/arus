package storefront

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	xhtml "golang.org/x/net/html"
)

// Kumpulan payload XSS klasik + varian penyamaran. Keluaran penyaring tidak boleh memuat satu pun pola berbahaya, apa pun
// masukannya; pemeriksaan dilakukan pada hasil serialisasi (yang dikirim ke browser), bukan pada masukan.
var xssCorpus = []string{
	`<script>alert(1)</script>`,
	`<SCRIPT SRC=//evil.test/x.js></SCRIPT>`,
	`<img src=x onerror=alert(1)>`,
	`<img src="x" onerror="alert(1)" onload=alert(2)>`,
	`<a href="javascript:alert(1)">klik</a>`,
	`<a href="JaVaScRiPt:alert(1)">klik</a>`,
	"<a href=\"java\tscript:alert(1)\">klik</a>",
	"<a href=\"java\nscript:alert(1)\">klik</a>",
	`<a href=" javascript:alert(1)">klik</a>`,
	`<a href="&#106;avascript:alert(1)">klik</a>`,
	`<a href="&#x6A;avascript:alert(1)">klik</a>`,
	`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
	`<a href="vbscript:msgbox(1)">x</a>`,
	`<a href="//evil.test/phish">x</a>`,
	`<iframe src="https://evil.test"></iframe>`,
	`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
	`<object data="x.swf"></object><embed src="x.swf">`,
	`<svg onload=alert(1)><circle/></svg>`,
	`<svg><script>alert(1)</script></svg>`,
	`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
	`<form action="https://evil.test"><input name=pw type=password><button>Masuk</button></form>`,
	`<style>body{background:url(javascript:alert(1))}</style>`,
	`<div style="background:url(javascript:alert(1))">x</div>`,
	`<div style="width:expression(alert(1))">x</div>`,
	`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
	`<link rel=stylesheet href="https://evil.test/x.css">`,
	`<base href="https://evil.test/">`,
	`<body onload=alert(1)>`,
	`<div onmouseover="alert(1)" class="x">hover</div>`,
	`<details open ontoggle=alert(1)>`,
	`<p>a<!--<script>alert(1)</script>-->b</p>`,
	`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
	`<textarea><img src=x onerror=alert(1)></textarea>`,
	`<title><img src=x onerror=alert(1)></title>`,
	`<xmp><img src=x onerror=alert(1)></xmp>`,
	`<div class="a" onclick="alert(1)" class="b">x</div>`,
	`<img src="https://evil.test/x.png" alt="a" onerror=alert(1)//>`,
	`<a href="https://ok.test" target=_top>x</a>`,
	`<<script>alert(1)//<</script>`,
	`<scr<script>ipt>alert(1)</scr</script>ipt>`,
	`<a href='x' " onmouseover="alert(1)">x</a>`,
	`<template><script>alert(1)</script></template>`,
	`<video src=x onerror=alert(1)></video><audio src=x onerror=alert(1)></audio>`,
	`<select><option>x</option></select>`,
	`<div id="x" data-x="1" aria-label="a" role="button">ok</div>`,
}

var dangerous = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*(script|iframe|object|embed|svg|math|style|form|input|button|link|meta|base|template|video|audio|textarea|select)\b`),
	regexp.MustCompile(`(?i)(java|vb)script\s*:`),
	regexp.MustCompile(`(?i)\sstyle\s*=`),
	regexp.MustCompile(`(?i)data\s*:`),
	regexp.MustCompile(`(?i)href="//`),
	regexp.MustCompile(`(?i)expression\s*\(`),
}

// hasEventAttr memeriksa atribut sebenarnya lewat tokenizer (teks "onmouseover=" di dalam nilai atribut yang di-escape
// bukan atribut).
func hasEventAttr(out string) bool {
	z := xhtml.NewTokenizer(strings.NewReader(out))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return false
		}
		if tt == xhtml.StartTagToken || tt == xhtml.SelfClosingTagToken {
			_, more := z.TagName()
			for more {
				var k []byte
				k, _, more = z.TagAttr()
				if strings.HasPrefix(strings.ToLower(string(k)), "on") || strings.EqualFold(string(k), "style") {
					return true
				}
			}
		}
	}
}

func TestSanitizeHTMLNoDangerousOutput(t *testing.T) {
	for _, in := range xssCorpus {
		for _, tmpl := range []bool{false, true} {
			out, _ := SanitizeHTML(in, tmpl)
			for _, re := range dangerous {
				if re.MatchString(out) {
					t.Errorf("masukan %q (tmpl=%v) -> %q lolos pola %v", in, tmpl, out, re)
				}
			}
			if hasEventAttr(out) {
				t.Errorf("masukan %q (tmpl=%v) -> %q memuat atribut on*/style", in, tmpl, out)
			}
			// Keluaran yang disaring harus stabil: menyaring ulang tidak boleh mengubah apa pun (tanda tak ada mXSS).
			again, _ := SanitizeHTML(out, tmpl)
			if again != out {
				t.Errorf("penyaringan tidak idempoten untuk %q:\n 1: %q\n 2: %q", in, out, again)
			}
		}
	}
}

func TestSanitizeHTMLKeepsGoodMarkup(t *testing.T) {
	in := `<section class="mx-auto max-w-5xl p-6 md:grid-cols-4 hover:bg-emerald-50 w-[37px]"><h1 id="top">Halo &amp; selamat</h1>` +
		`<a href="https://wa.me/62812" target="_blank">WA</a><a href="#produk">ke produk</a><a href="/s/toko">dalam</a><a href="mailto:a@b.test">m</a>` +
		`<img src="/public/s/toko/img/1" alt="x" width="100"><ul><li>satu</li></ul></section>`
	out, issues := SanitizeHTML(in, false)
	if len(issues) != 0 {
		t.Fatalf("tidak boleh ada peringatan: %v", issues)
	}
	for _, want := range []string{`class="mx-auto max-w-5xl p-6 md:grid-cols-4 hover:bg-emerald-50 w-[37px]"`, `href="https://wa.me/62812" target="_blank" rel="noopener noreferrer nofollow"`,
		`href="#produk"`, `href="/s/toko"`, `href="mailto:a@b.test"`, `src="/public/s/toko/img/1" alt="x" width="100" loading="lazy"`, `Halo &amp; selamat`} {
		if !strings.Contains(out, want) {
			t.Errorf("hilang %q dalam %q", want, out)
		}
	}
}

func TestSanitizeHTMLBalancesTags(t *testing.T) {
	out, _ := SanitizeHTML(`<div><p>a<b>tebal</div></span>sisa<ul><li>x`, false)
	if out != `<div><p>a<b>tebal</b></p></div>sisa<ul><li>x</li></ul>` {
		t.Fatalf("tag tak seimbang: %q", out)
	}
}

func TestTemplateModeKeepsPlaceholderURLsButRenderDoesNot(t *testing.T) {
	in := `<a href="{linkjahat}">x</a><img src="{gambar}" alt="{namabarang}">`
	out, _ := SanitizeHTML(in, true)
	if !strings.Contains(out, `href="{linkjahat}"`) || !strings.Contains(out, `src="{gambar}"`) {
		t.Fatalf("mode templat harus membiarkan penanda: %q", out)
	}
	out, _ = SanitizeHTML(in, false)
	if strings.Contains(out, "href=") || strings.Contains(out, "src=") {
		t.Fatalf("mode render harus membuang URL berpenanda: %q", out)
	}
}

func TestSafeURL(t *testing.T) {
	ok := []string{"https://a.test/x?y=1", "http://a.test", "/s/toko", "#bawah", "mailto:a@b.test", "tel:+6281"}
	bad := []string{"", "javascript:alert(1)", "//evil.test", "/\\evil.test", "data:text/html,x", "ftp://a.test", "https://", "java script:x", "https://a.test/\"x", "file:///etc/passwd", "https://a.test/<x>"}
	for _, u := range ok {
		if _, g := SafeURL(u, true); !g {
			t.Errorf("%q seharusnya lolos", u)
		}
	}
	for _, u := range bad {
		if _, g := SafeURL(u, true); g {
			t.Errorf("%q seharusnya ditolak", u)
		}
	}
	if _, g := SafeURL("mailto:a@b.test", false); g {
		t.Error("mailto bukan untuk src")
	}
}

func TestSanitizeCSS(t *testing.T) {
	good := []string{`.hero{background:#10b981;color:white}`, `@media (min-width:768px){.a{display:grid}}`, `.x:hover>.y{transform:scale(1.05)}`, `@keyframes f{from{opacity:0}to{opacity:1}}`, ""}
	bad := []string{
		`@import url("https://evil.test/x.css");`, `@import "x.css";`, `.a{background:url(https://evil.test/x.png)}`, `.a{background:URL( x )}`,
		`.a{background:ur/**/l(x)}`, `.a{width:expression(alert(1))}`, `.a{background:u\72l(x)}`, `.a{behavior:url(x.htc)}`, `.a{-moz-binding:url(x)}`,
		`.a{color:red}</style><script>alert(1)</script>`, `@font-face{font-family:x;src:url(x.woff)}`, `.a{background:image-set("x" 1x)}`,
		`.a{content:"x";background:javascript:alert(1)}`, `.a{color:red} /* tak ditutup`, `.a { background : u r l ( x ) }`,
	}
	for _, c := range good {
		if _, err := SanitizeCSS(c); err != nil {
			t.Errorf("%q seharusnya lolos: %v", c, err)
		}
	}
	for _, c := range bad {
		if _, err := SanitizeCSS(c); err == nil {
			t.Errorf("%q seharusnya ditolak", c)
		}
	}
}

// ---- templat ----

type fakeSource struct {
	cats    []Category
	items   []Item
	queries int
}

func (f *fakeSource) Categories(_ context.Context, limit int) ([]Category, error) {
	f.queries++
	return f.cats, nil
}

func (f *fakeSource) Items(_ context.Context, q ItemQuery) ([]Item, error) {
	f.queries++
	var out []Item
	for _, it := range f.items {
		if q.CategoryID != nil && it.Category != nameOf(f.cats, *q.CategoryID) {
			continue
		}
		out = append(out, it)
		if len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeSource) Levels(context.Context, int) ([]Level, error) {
	f.queries++
	return []Level{{Name: "Reguler", SpendPerPoint: decimal.NewFromInt(10000), PointValue: decimal.NewFromInt(100)}, {Name: "Gold", MinPoints: 500, SpendPerPoint: decimal.NewFromInt(6000), PointValue: decimal.NewFromInt(100)}}, nil
}

func (f *fakeSource) Coupons(context.Context, time.Time, int) ([]Coupon, error) {
	f.queries++
	rem := 3
	return []Coupon{{Code: "HEMAT10", Name: "<i>Hemat</i>", Kind: "percent", Value: decimal.NewFromInt(10), MaxDiscount: decimal.NewFromInt(25000), MinSpend: decimal.NewFromInt(100000), Remaining: &rem}}, nil
}

func (f *fakeSource) Outlets(context.Context, int) ([]Outlet, error) {
	f.queries++
	return []Outlet{{Name: "Pusat"}, {Name: "Cabang"}}, nil
}

func (f *fakeSource) PayMethods(context.Context, int) ([]PayMethod, error) {
	f.queries++
	return []PayMethod{{Name: "Tunai", Kind: "cash"}}, nil
}

func (f *fakeSource) MemberTotal(context.Context) (int64, error) {
	f.queries++
	return 1234567, nil
}

func newFake() *fakeSource {
	minum, makan := Category{uuid.New(), "Minuman"}, Category{uuid.New(), "Makanan"}
	img := func() *uuid.UUID { id := uuid.New(); return &id }
	return &fakeSource{
		cats: []Category{makan, minum},
		items: []Item{
			{ID: uuid.New(), SKU: "A1", Name: "Kopi <b>Susu</b>", Price: decimal.NewFromInt(18500), Category: "Minuman", Description: "**Enak** dan _segar_", ImageID: img()},
			{ID: uuid.New(), SKU: "A2", Name: "javascript:alert(1)", Price: decimal.NewFromInt(1250000), Category: "Makanan", ImageID: img()},
			{ID: uuid.New(), SKU: "A3", Name: "Teh", Price: decimal.NewFromInt(5000), Category: "Minuman", ImageID: img()},
		},
	}
}

func site() Site {
	return Site{Name: `Toko "Maju" & Jaya`, Slug: "maju", Now: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC),
		ImageURL: func(id uuid.UUID) string { return "/public/s/maju/img/" + id.String() }}
}

func TestRenderTagsAndEscaping(t *testing.T) {
	src := newFake()
	out, err := Render(context.Background(), `<h1>{namatoko} {tahun}</h1><p>{namabarang} {harga} {hargaangka} {kodebarang}</p><p>{namabarang2}|{kategori1}|{kategori2}|{harga3}|{tidakada}|{namabarang9}</p>`, src, site())
	if err != nil {
		t.Fatal(err)
	}
	want := `<h1>Toko &#34;Maju&#34; &amp; Jaya 2026</h1><p>Kopi &lt;b&gt;Susu&lt;/b&gt; Rp18.500 18500 A1</p><p>javascript:alert(1)|Makanan|Minuman|Rp5.000|{tidakada}|</p>`
	if out != want {
		t.Fatalf("hasil render:\n got: %s\nwant: %s", out, want)
	}
	if src.queries != 2 { // kategori sekali + barang berindeks sekali, walau banyak penanda
		t.Fatalf("jumlah query = %d, want 2", src.queries)
	}
}

func TestRenderBlocks(t *testing.T) {
	src := newFake()
	out, err := Render(context.Background(), `{#barang limit=2}<li>{nomor}.{namabarang}</li>{/barang}|{#kategori}[{namakategori}:{#barang kategori=ini}{namabarang},{/barang}]{/kategori}|{#barang kategori="minuman" limit=1}{kodebarang}{/barang}|{#barang kategori=9}X{/barang}|{#barang kategori=2}{kodebarang}{/barang}`, src, site())
	if err != nil {
		t.Fatal(err)
	}
	want := `<li>1.Kopi &lt;b&gt;Susu&lt;/b&gt;</li><li>2.javascript:alert(1)</li>|[Makanan:javascript:alert(1),][Minuman:Kopi &lt;b&gt;Susu&lt;/b&gt;,Teh,]|A1|A1A3`
	// kategori=9 tidak ada -> kosong (bukan semua barang); "|" di antara blok tetap teks.
	want = `<li>1.Kopi &lt;b&gt;Susu&lt;/b&gt;</li><li>2.javascript:alert(1)</li>|[Makanan:javascript:alert(1),][Minuman:Kopi &lt;b&gt;Susu&lt;/b&gt;,Teh,]|A1||A1A3`
	if out != want {
		t.Fatalf("hasil blok:\n got: %s\nwant: %s", out, want)
	}
}

func TestRenderQueryBudgetAndLooseBraces(t *testing.T) {
	src := newFake()
	tmpl := strings.Repeat(`{#barang limit=1}x{/barang}`, 100) + ` a { b } c {x`
	out, err := Render(context.Background(), tmpl, src, site())
	if err != nil {
		t.Fatal(err)
	}
	if src.queries > maxQueries {
		t.Fatalf("anggaran query terlampaui: %d", src.queries)
	}
	if !strings.HasSuffix(out, ` a { b } c {x`) {
		t.Fatalf("kurung lepas harus tetap teks biasa: %q", out[len(out)-20:])
	}
}

// Nama barang berbahaya tak boleh menjadi tautan/atribut aktif setelah melewati render + penyaring.
func TestRenderThenSanitizeNeutralizesHostileItemData(t *testing.T) {
	src := newFake()
	src.items[1].Name = `x" onmouseover="alert(1)`
	out, err := Render(context.Background(), `<a href="{namabarang2}" title="{namabarang2}">{namabarang2}</a><img src="{gambar}" alt="{namabarang2}">`, src, site())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := SanitizeHTML(out, false)
	for _, re := range dangerous {
		if re.MatchString(body) {
			t.Fatalf("lolos %v: %s", re, body)
		}
	}
	if hasEventAttr(body) {
		t.Fatalf("nama barang berbahaya menjadi atribut aktif: %s", body)
	}
	if strings.Contains(body, "href=") {
		t.Fatalf("href berisi nama barang harus dibuang: %s", body)
	}
}

func TestStarterIsCleanAndRenders(t *testing.T) {
	if _, err := SanitizeCSS(StarterCSS); err != nil {
		t.Fatal(err)
	}
	if _, issues := SanitizeHTML(StarterHTML, true); len(issues) != 0 {
		t.Fatalf("templat awal memicu peringatan: %v", issues)
	}
	src := newFake()
	out, err := Render(context.Background(), StarterHTML, src, site())
	if err != nil {
		t.Fatal(err)
	}
	body, issues := SanitizeHTML(out, false)
	if len(issues) != 0 {
		t.Fatalf("hasil render templat awal memicu peringatan: %v", issues)
	}
	for _, want := range []string{"Kopi &lt;b&gt;Susu&lt;/b&gt;", "Rp18.500", "Minuman", "2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("hasil tak memuat %q", want)
		}
	}
	doc := Document("T", body, tailwindPath, "/public/s/maju/style.css?v=1", "")
	if strings.Contains(strings.ToLower(doc), "<script") {
		t.Fatal("dokumen tak boleh berisi script")
	}
}

// Setiap class di templat awal harus ada di stylesheet Tailwind hasil build (kalau tidak, halaman awal tampil polos).
func TestStarterClassesExistInTailwindCSS(t *testing.T) {
	css := string(tailwindCSS)
	for _, m := range regexp.MustCompile(`class="([^"]*)"`).FindAllStringSubmatch(StarterHTML, -1) {
		for _, cl := range strings.Fields(m[1]) {
			esc := strings.NewReplacer(":", `\:`, "/", `\/`, ".", `\.`, "[", `\[`, "]", `\]`).Replace(cl)
			if !strings.Contains(css, "."+esc) {
				t.Errorf("class %q tidak ada di tailwind.css; tambahkan ke tools/storefront-css/build.mjs", cl)
			}
		}
	}
	if strings.Contains(css, "url(") || strings.Contains(css, "@import") {
		t.Error("tailwind.css tak boleh memuat url()/@import (CSP style-src 'self', img-src terbatas)")
	}
}

// Urutan layer harus theme < base < utilities; kalau terbalik, preflight menimpa utility (text-4xl, p-6, font-bold tak berefek).
func TestTailwindLayerOrder(t *testing.T) {
	css := string(tailwindCSS)
	decl := strings.Index(css, "@layer theme, base, components, utilities;")
	util := strings.Index(css, "@layer utilities {")
	if decl < 0 || util < 0 || decl > util {
		t.Fatalf("deklarasi urutan layer hilang atau terlambat (decl=%d utilities=%d)", decl, util)
	}
}
