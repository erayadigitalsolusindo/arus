package storefront

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Bahasa templat landing page. Penanda ditulis dalam kurung kurawal; semua nilai disisipkan SETELAH di-escape HTML, dan
// hasil akhirnya disaring lagi (SanitizeHTML, tmpl=false), jadi nama barang berisi "javascript:..." tak bisa jadi tautan.
//
//	{namatoko}  {tahun}                    nama toko, tahun berjalan
//	{kategori1} … {kategori20}             nama kategori urutan ke-N (urut nama)
//	{namabarang} {harga} {hargaangka}      barang "saat ini"; di luar blok = barang urutan 1 (urut nama)
//	{kodebarang} {kategoribarang}
//	{deskripsi} {gambar} {nomor}           tambahkan angka untuk barang urutan ke-N: {namabarang3}, {harga2}
//	{#barang kategori=1 limit=8 urut=nama} … {/barang}        ulang untuk tiap barang
//	{#kategori limit=6} {namakategori} {nomor} … {/kategori}  ulang untuk tiap kategori
//	  - kategori= angka (urutan), "Nama Kategori", atau ini (kategori dari blok {#kategori} di sekelilingnya)
//	  - urut= nama | terbaru | termurah | termahal
//	{satuan} {stok}                        satuan barang; stok = Tersedia | Habis (jasa/boleh minus selalu Tersedia)
//	{jumlahkategori} {jumlahoutlet} {jumlahmember} {tanggal}   angka ringkas toko (member = total terdaftar, dari penghitung)
//	{#level}  {namalevel} {minpoin} {belanjaperpoin} {nilaipoin} {nomor}          level member (aturan, bukan data orang)
//	{#kupon limit=6}  {kodekupon} {namakupon} {potongan} {maksimalpotongan} {minbelanja} {berlakuhingga} {sisakuota}
//	  - hanya kupon aktif, berlaku hari ini, kuota tersisa, dan diberi saklar "tampil di landing page"
//	{#outlet} {namaoutlet}      {#pembayaran} {namapembayaran} {jenispembayaran}
//	Data pribadi (nama/telepon/poin member) TIDAK pernah tersedia di penanda.

const (
	maxCategories  = 20
	maxIndexed     = 24
	maxBlockLimit  = 48
	defBlockLimit  = 8
	maxListed      = 20
	maxQueries     = 40
	maxNodes       = 6000
	maxOutputBytes = 2 << 20
)

type Category struct {
	ID   uuid.UUID
	Name string
}

type Item struct {
	ID          uuid.UUID
	SKU         string
	Name        string
	Description string
	Price       decimal.Decimal
	Category    string
	ImageID     *uuid.UUID
	Unit        string
	InStock     bool
}

type Level struct {
	Name          string
	MinPoints     int
	SpendPerPoint decimal.Decimal
	PointValue    decimal.Decimal
}

type Coupon struct {
	Code, Name, Kind string
	Value            decimal.Decimal
	MaxDiscount      decimal.Decimal // 0 = tanpa batas
	MinSpend         decimal.Decimal
	EndsOn           *time.Time
	Remaining        *int // nil = kuota tak terbatas
}

type Outlet struct{ Name string }

type PayMethod struct{ Name, Kind string }

type ItemQuery struct {
	CategoryID *uuid.UUID
	Limit      int
	Order      string // nama | terbaru | termurah | termahal
}

// Source = sumber data publik toko (DB pada produksi, tiruan pada test).
type Source interface {
	Categories(ctx context.Context, limit int) ([]Category, error)
	Items(ctx context.Context, q ItemQuery) ([]Item, error)
	Levels(ctx context.Context, limit int) ([]Level, error)
	Coupons(ctx context.Context, today time.Time, limit int) ([]Coupon, error)
	Outlets(ctx context.Context, limit int) ([]Outlet, error)
	PayMethods(ctx context.Context, limit int) ([]PayMethod, error)
	// MemberTotal = total member terdaftar, dibaca dari penghitung nomor member (O(1)), bukan count(*).
	MemberTotal(ctx context.Context) (int64, error)
}

// Site = data toko untuk satu render.
type Site struct {
	Name     string
	Slug     string
	Now      time.Time
	ImageURL func(id uuid.UUID) string // alamat gambar publik
}

type node struct {
	kind  byte // 't' teks, 'v' variabel, 'b' blok
	text  string
	name  string // v: nama variabel; b: barang | kategori
	args  map[string]string
	child []*node
}

var (
	tagRe    = regexp.MustCompile(`^(#?/?)([a-z][a-z0-9]*)((?:\s+[a-z]+=(?:"[^"{}]*"|[^\s"{}]+))*)\s*$`)
	argRe    = regexp.MustCompile(`([a-z]+)=(?:"([^"{}]*)"|([^\s"{}]+))`)
	indexRe  = regexp.MustCompile(`^([a-z]+?)([0-9]{1,2})$`)
	itemVars = map[string]bool{"namabarang": true, "harga": true, "hargaangka": true, "kodebarang": true, "kategoribarang": true, "deskripsi": true, "gambar": true, "nomor": true, "satuan": true, "stok": true}
	blocks   = map[string]bool{"barang": true, "kategori": true, "level": true, "kupon": true, "outlet": true, "pembayaran": true}
)

// parse memecah templat menjadi pohon. Penanda yang tidak dikenal dibiarkan sebagai teks biasa; blok yang tak seimbang
// ditutup otomatis di akhir.
func parse(src string) ([]*node, error) {
	root := &node{kind: 'b', name: "root"}
	stack := []*node{root}
	count := 0
	add := func(n *node) error {
		if count++; count > maxNodes {
			return fmt.Errorf("templat terlalu kompleks (maks %d penanda)", maxNodes)
		}
		top := stack[len(stack)-1]
		top.child = append(top.child, n)
		return nil
	}
	text := func(s string) error {
		if s == "" {
			return nil
		}
		return add(&node{kind: 't', text: s})
	}
	for len(src) > 0 {
		i := strings.IndexByte(src, '{')
		if i < 0 {
			if err := text(src); err != nil {
				return nil, err
			}
			break
		}
		if err := text(src[:i]); err != nil {
			return nil, err
		}
		src = src[i:]
		j := strings.IndexByte(src, '}')
		k := strings.IndexByte(src[1:], '{')
		if j < 0 || (k >= 0 && k+1 < j) || j > 200 { // bukan penanda ("{" lepas, mis. di CSS inline)
			if err := text("{"); err != nil {
				return nil, err
			}
			src = src[1:]
			continue
		}
		raw, m := src[1:j], tagRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(src[1:j])))
		src = src[j+1:]
		if m == nil {
			if err := text("{" + raw + "}"); err != nil {
				return nil, err
			}
			continue
		}
		prefix, name, rest := m[1], m[2], m[3]
		switch {
		case prefix == "#" && blocks[name]:
			args := map[string]string{}
			for _, a := range argRe.FindAllStringSubmatch(rest, -1) {
				v := a[2]
				if v == "" {
					v = a[3]
				}
				args[a[1]] = v
			}
			n := &node{kind: 'b', name: name, args: args}
			if err := add(n); err != nil {
				return nil, err
			}
			stack = append(stack, n)
		case prefix == "/" && blocks[name]:
			if len(stack) > 1 && stack[len(stack)-1].name == name {
				stack = stack[:len(stack)-1]
			}
		case prefix == "" && rest == "":
			if err := add(&node{kind: 'v', name: name, text: "{" + raw + "}"}); err != nil {
				return nil, err
			}
		default:
			if err := text("{" + raw + "}"); err != nil {
				return nil, err
			}
		}
	}
	return root.child, nil
}

// memo = daftar yang dimuat paling banyak sekali per render (dan hanya bila dipakai templat).
type memo[T any] struct {
	done bool
	v    []T
}

func load[T any](r *renderer, m *memo[T], fn func() ([]T, error)) []T {
	if !m.done {
		m.done = true
		if r.spend() {
			v, err := fn()
			r.fail(err)
			m.v = v
		}
	}
	return m.v
}

type renderer struct {
	levels  memo[Level]
	coupons memo[Coupon]
	outlets memo[Outlet]
	pays    memo[PayMethod]
	members int64
	memOK   bool

	ctx     context.Context
	src     Source
	site    Site
	cats    []Category
	catsOK  bool
	first   []Item // barang urutan 1..N (urut nama), dimuat sekali bila ada penanda berindeks
	firstOK bool
	queries int
	out     strings.Builder
	err     error
}

type scope struct {
	item *Item
	idx  int
	cat  *Category
	// nomor urut baris di blok terdalam (1-based)
	num    int
	level  *Level
	coupon *Coupon
	outlet *Outlet
	pay    *PayMethod
}

// Render mengisi templat. Keluaran BELUM disaring; pemanggil wajib menjalankan SanitizeHTML(tmpl=false).
func Render(ctx context.Context, tmpl string, src Source, site Site) (string, error) {
	nodes, err := parse(tmpl)
	if err != nil {
		return "", err
	}
	r := &renderer{ctx: ctx, src: src, site: site}
	r.walk(nodes, scope{})
	if r.err != nil {
		return "", r.err
	}
	return r.out.String(), nil
}

func (r *renderer) categories() []Category {
	if !r.catsOK {
		r.catsOK = true
		if r.spend() {
			c, err := r.src.Categories(r.ctx, maxCategories)
			r.fail(err)
			r.cats = c
		}
	}
	return r.cats
}

func (r *renderer) spend() bool {
	if r.queries >= maxQueries || r.err != nil {
		return false
	}
	r.queries++
	return true
}

func (r *renderer) fail(err error) {
	if err != nil && r.err == nil {
		r.err = err
	}
}

func (r *renderer) indexed(n int) *Item {
	if !r.firstOK {
		r.firstOK = true
		if r.spend() {
			it, err := r.src.Items(r.ctx, ItemQuery{Limit: maxIndexed, Order: "nama"})
			r.fail(err)
			r.first = it
		}
	}
	if n >= 1 && n <= len(r.first) {
		return &r.first[n-1]
	}
	return nil
}

func (r *renderer) walk(nodes []*node, sc scope) {
	for _, n := range nodes {
		if r.err != nil || r.out.Len() > maxOutputBytes {
			return
		}
		switch n.kind {
		case 't':
			r.out.WriteString(n.text)
		case 'v':
			if v, ok := r.value(n, sc); ok {
				r.out.WriteString(html.EscapeString(v))
			} else {
				r.out.WriteString(html.EscapeString(n.text)) // tak dikenal: tampil apa adanya
			}
		case 'b':
			switch n.name {
			case "kategori":
				r.kategoriBlock(n, sc)
			case "level":
				for i, x := range load(r, &r.levels, func() ([]Level, error) { return r.src.Levels(r.ctx, 10) }) {
					if i >= blockLimit(n) {
						break
					}
					r.walk(n.child, scope{level: &x, item: sc.item, cat: sc.cat, num: i + 1})
				}
			case "kupon":
				for i, x := range load(r, &r.coupons, func() ([]Coupon, error) { return r.src.Coupons(r.ctx, r.site.Now, maxListed) }) {
					if i >= blockLimit(n) {
						break
					}
					r.walk(n.child, scope{coupon: &x, item: sc.item, cat: sc.cat, num: i + 1})
				}
			case "outlet":
				for i, x := range load(r, &r.outlets, func() ([]Outlet, error) { return r.src.Outlets(r.ctx, maxListed) }) {
					if i >= blockLimit(n) {
						break
					}
					r.walk(n.child, scope{outlet: &x, item: sc.item, cat: sc.cat, num: i + 1})
				}
			case "pembayaran":
				for i, x := range load(r, &r.pays, func() ([]PayMethod, error) { return r.src.PayMethods(r.ctx, maxListed) }) {
					if i >= blockLimit(n) {
						break
					}
					r.walk(n.child, scope{pay: &x, item: sc.item, cat: sc.cat, num: i + 1})
				}
			default:
				r.barangBlock(n, sc)
			}
		}
	}
}

func blockLimit(n *node) int { return clamp(atoi(n.args["limit"], maxListed), 0, maxListed) }

func (r *renderer) kategoriBlock(n *node, sc scope) {
	cats := r.categories()
	limit := clamp(atoi(n.args["limit"], len(cats)), 0, maxCategories)
	for i := 0; i < len(cats) && i < limit; i++ {
		c := cats[i]
		r.walk(n.child, scope{cat: &c, item: sc.item, num: i + 1})
	}
}

func (r *renderer) barangBlock(n *node, sc scope) {
	q := ItemQuery{Limit: clamp(atoi(n.args["limit"], defBlockLimit), 1, maxBlockLimit), Order: "nama"}
	switch o := n.args["urut"]; o {
	case "nama", "terbaru", "termurah", "termahal":
		q.Order = o
	}
	if k := n.args["kategori"]; k != "" {
		var c *Category
		switch {
		case k == "ini":
			c = sc.cat
		default:
			cats := r.categories()
			if i, err := strconv.Atoi(k); err == nil {
				if i >= 1 && i <= len(cats) {
					c = &cats[i-1]
				}
			} else {
				for j := range cats {
					if strings.EqualFold(cats[j].Name, k) {
						c = &cats[j]
					}
				}
			}
		}
		if c == nil {
			return // kategori tak ada -> blok kosong, bukan semua barang
		}
		q.CategoryID = &c.ID
	}
	if !r.spend() {
		return
	}
	items, err := r.src.Items(r.ctx, q)
	r.fail(err)
	for i := range items {
		r.walk(n.child, scope{item: &items[i], cat: sc.cat, num: i + 1})
	}
}

func (r *renderer) value(n *node, sc scope) (string, bool) {
	name := n.name
	switch name {
	case "namatoko":
		return r.site.Name, true
	case "tahun":
		return strconv.Itoa(r.site.Now.Year()), true
	case "namakategori":
		if sc.cat != nil {
			return sc.cat.Name, true
		}
		return "", true
	case "nomor":
		return strconv.Itoa(sc.num), true
	case "tanggal":
		return indoDate(r.site.Now), true
	case "jumlahkategori":
		return strconv.Itoa(len(r.categories())), true
	case "jumlahoutlet":
		return strconv.Itoa(len(load(r, &r.outlets, func() ([]Outlet, error) { return r.src.Outlets(r.ctx, maxListed) }))), true
	case "jumlahmember":
		if !r.memOK {
			r.memOK = true
			if r.spend() {
				n, err := r.src.MemberTotal(r.ctx)
				r.fail(err)
				r.members = n
			}
		}
		return groupDigits(strconv.FormatInt(r.members, 10)), true
	}
	if v, ok := r.blockValue(name, sc); ok {
		return v, true
	}
	if m := indexRe.FindStringSubmatch(name); m != nil {
		num, _ := strconv.Atoi(m[2])
		if m[1] == "kategori" {
			if cats := r.categories(); num >= 1 && num <= len(cats) {
				return cats[num-1].Name, true
			}
			return "", num >= 1 && num <= maxCategories
		}
		if itemVars[m[1]] && m[1] != "nomor" {
			if it := r.indexed(num); it != nil {
				return r.itemField(m[1], it, num), true
			}
			return "", num >= 1 && num <= maxIndexed
		}
		return "", false
	}
	if itemVars[name] {
		if sc.item != nil {
			return r.itemField(name, sc.item, sc.num), true
		}
		if it := r.indexed(1); it != nil {
			return r.itemField(name, it, 1), true
		}
		return "", true
	}
	return "", false
}

func (r *renderer) blockValue(name string, sc scope) (string, bool) {
	if l := sc.level; l != nil {
		switch name {
		case "namalevel":
			return l.Name, true
		case "minpoin":
			return strconv.Itoa(l.MinPoints), true
		case "belanjaperpoin":
			return "Rp" + rupiah(l.SpendPerPoint), true
		case "nilaipoin":
			return "Rp" + rupiah(l.PointValue), true
		}
	}
	if c := sc.coupon; c != nil {
		switch name {
		case "kodekupon":
			return c.Code, true
		case "namakupon":
			return c.Name, true
		case "potongan":
			if c.Kind == "percent" {
				return strings.TrimRight(strings.TrimRight(c.Value.StringFixed(2), "0"), ".") + "%", true
			}
			return "Rp" + rupiah(c.Value), true
		case "maksimalpotongan":
			if c.MaxDiscount.IsPositive() {
				return "Rp" + rupiah(c.MaxDiscount), true
			}
			return "", true
		case "minbelanja":
			if c.MinSpend.IsPositive() {
				return "Rp" + rupiah(c.MinSpend), true
			}
			return "Tanpa minimum", true
		case "berlakuhingga":
			if c.EndsOn != nil {
				return indoDate(*c.EndsOn), true
			}
			return "Tanpa batas", true
		case "sisakuota":
			if c.Remaining != nil {
				return strconv.Itoa(*c.Remaining), true
			}
			return "Tak terbatas", true
		}
	}
	if sc.outlet != nil && name == "namaoutlet" {
		return sc.outlet.Name, true
	}
	if p := sc.pay; p != nil {
		switch name {
		case "namapembayaran":
			return p.Name, true
		case "jenispembayaran":
			return payKinds[p.Kind], true
		}
	}
	switch name {
	case "namalevel", "minpoin", "belanjaperpoin", "nilaipoin", "kodekupon", "namakupon", "potongan", "maksimalpotongan",
		"minbelanja", "berlakuhingga", "sisakuota", "namaoutlet", "namapembayaran", "jenispembayaran":
		return "", true // di luar blok induknya: kosong, bukan teks mentah
	}
	return "", false
}

var payKinds = map[string]string{"cash": "Tunai", "debit": "Kartu debit", "credit_card": "Kartu kredit", "ewallet": "E-wallet", "transfer": "Transfer"}

var indoMonths = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func indoDate(t time.Time) string {
	return strconv.Itoa(t.Day()) + " " + indoMonths[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

func groupDigits(s string) string {
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

func (r *renderer) itemField(name string, it *Item, num int) string {
	switch name {
	case "namabarang":
		return it.Name
	case "kodebarang":
		return it.SKU
	case "kategoribarang":
		return it.Category
	case "deskripsi":
		return plain(it.Description, 300)
	case "harga":
		return "Rp" + rupiah(it.Price)
	case "hargaangka":
		return it.Price.Round(0).String()
	case "gambar":
		if it.ImageID != nil && r.site.ImageURL != nil {
			return r.site.ImageURL(*it.ImageID)
		}
		return ""
	case "nomor":
		return strconv.Itoa(num)
	case "satuan":
		return it.Unit
	case "stok":
		if it.InStock {
			return "Tersedia"
		}
		return "Habis"
	}
	return ""
}

// rupiah memformat 12500 -> "12.500" (pembulatan ke rupiah penuh).
func rupiah(d decimal.Decimal) string {
	s := d.Round(0).String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

var mdNoise = regexp.MustCompile("[*_`#>~|\\[\\]]+")

// plain meringkas keterangan (markdown) jadi teks biasa satu baris.
func plain(s string, max int) string {
	s = strings.Join(strings.Fields(mdNoise.ReplaceAllString(s, " ")), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clamp(n, lo, hi int) int { return min(max(n, lo), hi) }
