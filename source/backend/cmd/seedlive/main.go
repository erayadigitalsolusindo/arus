// Command seedlive membuat TENANT DEMO BARU berisi data satu bulan yang konsisten (penjualan, pembelian, stok, hutang,
// piutang, retur, shift kasir, dan jurnal akuntansi) untuk demo di server live.
//
//	cd /www/wwwroot/arus/api && ./seedlive            # dry-run: hanya cek koneksi, tidak menulis apa pun
//	cd /www/wwwroot/arus/api && ./seedlive --yes      # eksekusi
//
// Cara kerja: semua data ditulis lewat API ARUS yang sama dengan aplikasi (jadi stok = ledger, hutang/piutang, poin,
// idempotensi, audit, dan jurnal otomatis berlaku seperti data sungguhan), lalu waktunya dimundurkan ke 30 hari terakhir
// (lihat backdate.go). Tidak menyentuh tenant lain.
//
// Env: SEED_API_URL (bawaan http://127.0.0.1:8080 — jalankan di server), MIGRATE_DATABASE_URL (pemilik skema; untuk memundurkan waktu),
// SEED_ORIGIN (bawaan: CORS_ORIGINS pertama), DEMO_EMAIL (bawaan demo@arus.id), DEMO_PASSWORD (kosong = dibuat acak
// dan dicetak SEKALI di konsol), SEED_DAYS (bawaan 30), SEED_SEED (benih acak).
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

type outletInfo struct {
	id, code, name string
	openHour       int
	closeHour      int
	salesPerDay    int // rata-rata nota hari kerja
	stockFactor    float64
}

type demo struct {
	ctx    context.Context
	api    *api
	admin  *pgxpool.Pool
	bd     *backdater
	rng    *mrand.Rand
	loc    *time.Location
	tenant uuid.UUID

	outlets   []outletInfo
	unitID    map[string]string
	catID     map[string]string
	brandID   map[string]string
	supplier  []string
	items     []string // id barang (indeks = itemDefs)
	members   []string
	memberTop map[string]decimal.Decimal // piutang berjalan kasar per member (hindari ditolak limit)
	method    map[string]string          // nama metode → id
	accounts  map[string]string

	stats struct{ sales, returns, purchases, recvPay, payPay, shifts, skippedLines int }
	today time.Time // tanggal lokal "hari ini" (nyata)
}

func main() {
	yes := flag.Bool("yes", false, "eksekusi (tanpa ini hanya dry-run)")
	flag.Parse()
	_ = godotenv.Load()
	if err := run(*yes); err != nil {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func randPassword() string {
	const set = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		b[i] = set[n.Int64()]
	}
	return string(b) + "-9"
}

func run(yes bool) error {
	ctx := context.Background()
	adminURL := os.Getenv("MIGRATE_DATABASE_URL")
	if adminURL == "" {
		return errors.New("MIGRATE_DATABASE_URL wajib diisi (memundurkan waktu data)")
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return err
	}
	defer admin.Close()

	origin := env("SEED_ORIGIN", strings.TrimSpace(strings.Split(os.Getenv("CORS_ORIGINS"), ",")[0]))
	if origin == "" {
		return errors.New("SEED_ORIGIN atau CORS_ORIGINS wajib diisi")
	}
	email := strings.ToLower(env("DEMO_EMAIL", "demo@arus.id"))
	pass := os.Getenv("DEMO_PASSWORD")
	generated := false
	if pass == "" {
		pass, generated = randPassword(), true
	}
	days, _ := strconv.Atoi(env("SEED_DAYS", "30"))
	if days < 3 || days > 90 {
		return errors.New("SEED_DAYS harus 3..90")
	}
	seed, _ := strconv.ParseInt(env("SEED_SEED", "20261015"), 10, 64)

	a := newAPI(env("SEED_API_URL", "http://127.0.0.1:8080"), origin, email, pass)
	if st, _, err := a.raw("GET", "/healthz", nil, "", ""); err != nil || st != 200 {
		return fmt.Errorf("API tidak sehat di %s (status %d): %v", a.base, st, err)
	}
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = $1)`, email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("email %s sudah terdaftar; seedlive hanya membuat tenant baru. Pakai DEMO_EMAIL lain atau hapus tenant lama", email)
	}
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return err
	}
	fmt.Printf("API   : %s (origin %s)\nDemo  : %s\nRentang: %d hari terakhir (sampai kemarin)\n", a.base, origin, email, days)
	if !yes {
		fmt.Println("\nDRY-RUN: koneksi API & database baik. Jalankan lagi dengan --yes untuk membuat data.")
		return nil
	}

	d := &demo{ctx: ctx, api: a, admin: admin, rng: mrand.New(mrand.NewSource(seed)), loc: loc,
		unitID: map[string]string{}, catID: map[string]string{}, brandID: map[string]string{}, method: map[string]string{},
		accounts: map[string]string{}, memberTop: map[string]decimal.Decimal{}}
	d.today = time.Now().In(loc)
	started := time.Now()
	if err := d.setup(email, pass); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if err := d.simulate(days); err != nil {
		return fmt.Errorf("simulasi: %w", err)
	}
	if err := d.finish(days); err != nil {
		return fmt.Errorf("penutup: %w", err)
	}
	fmt.Printf("\nSELESAI dalam %s — %d nota, %d retur jual, %d faktur beli, %d bayar piutang, %d bayar hutang, %d shift, %d request API.\n",
		time.Since(started).Round(time.Second), d.stats.sales, d.stats.returns, d.stats.purchases, d.stats.recvPay, d.stats.payPay, d.stats.shifts, a.calls)
	fmt.Printf("Login demo: %s\n", email)
	if generated {
		fmt.Printf("Kata sandi (dibuat acak, catat sekarang — tidak ditampilkan lagi): %s\n", pass)
	}
	return nil
}

// ---- util ----

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		switch t := v.(type) {
		case string:
			return t
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		}
	}
	return ""
}

func (d *demo) logf(format string, a ...any) { fmt.Printf(format+"\n", a...) }

func dec(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return v
}

func num(v int) json.Number { return json.Number(strconv.Itoa(v)) }

func (d *demo) pick(w []int) int {
	sum := 0
	for _, x := range w {
		sum += x
	}
	r := d.rng.Intn(sum)
	for i, x := range w {
		if r < x {
			return i
		}
		r -= x
	}
	return len(w) - 1
}

// ---- setup ----

func (d *demo) setup(email, pass string) error {
	a := d.api
	d.logf("[1/4] Membuat tenant demo …")
	reg := map[string]any{
		"business_name": "Toko Demo ARUS", "owner_name": "Pemilik Demo", "email": email, "phone": "081200000000",
		"outlet_name": "Cabang Pusat", "password": pass, "accept_terms": true,
	}
	var rr struct {
		Tenant struct{ ID string }       `json:"tenant"`
		Outlet struct{ ID, Code string } `json:"outlet"`
	}
	if err := a.post("/auth/register", reg, &rr); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	tid, err := uuid.Parse(rr.Tenant.ID)
	if err != nil {
		return fmt.Errorf("id tenant: %w", err)
	}
	d.tenant = tid
	a.tok = ""
	if err := a.login(); err != nil {
		return err
	}
	d.bd, err = newBackdater(d.ctx, d.admin, tid)
	if err != nil {
		return err
	}
	// outlet kedua
	var o2 map[string]any
	if err := a.post("/outlets", map[string]any{"code": "mall", "name": "Cabang Mall", "timezone": "Asia/Jakarta",
		"tax_store_pct": 0, "tax_gov_pct": 0, "address": "Magelang Mall Lt. 1", "phone": "0293000111",
		"receipt_footer": "Terima kasih telah berbelanja"}, &o2); err != nil {
		return fmt.Errorf("outlet kedua: %w", err)
	}
	// alamat/struk outlet pertama
	_ = a.put("/outlets/"+rr.Outlet.ID, map[string]any{"code": rr.Outlet.Code, "name": "Cabang Pusat", "timezone": "Asia/Jakarta",
		"tax_store_pct": 0, "tax_gov_pct": 0, "address": "Jl. Pahlawan No. 10, Magelang", "phone": "0293000222",
		"receipt_footer": "Terima kasih telah berbelanja"}, nil)
	d.outlets = []outletInfo{
		{id: rr.Outlet.ID, code: rr.Outlet.Code, name: "Cabang Pusat", openHour: 8, closeHour: 20, salesPerDay: 15, stockFactor: 1},
		{id: str(o2, "id"), code: "mall", name: "Cabang Mall", openHour: 10, closeHour: 21, salesPerDay: 10, stockFactor: 0.7},
	}

	d.logf("[2/4] Bagan akun & master data …")
	if err := a.post("/accounting/accounts/seed-retail", nil, nil); err != nil {
		return fmt.Errorf("seed COA: %w", err)
	}
	mk := func(path, name string, store map[string]string) error {
		var m map[string]any
		if err := a.post("/catalog/"+path, map[string]any{"name": name}, &m); err != nil {
			return fmt.Errorf("%s %q: %w", path, name, err)
		}
		store[name] = str(m, "id")
		return nil
	}
	for _, n := range units {
		if err := mk("units", n, d.unitID); err != nil {
			return err
		}
	}
	for _, n := range categories {
		if err := mk("categories", n, d.catID); err != nil {
			return err
		}
	}
	for _, n := range brands {
		if err := mk("brands", n, d.brandID); err != nil {
			return err
		}
	}
	for _, s := range suppliers {
		var m map[string]any
		if err := a.post("/catalog/suppliers", map[string]any{"code": s.code, "name": s.name, "contact_name": s.contact, "phone": s.phone, "address": s.address}, &m); err != nil {
			return fmt.Errorf("supplier %s: %w", s.name, err)
		}
		d.supplier = append(d.supplier, str(m, "id"))
	}
	for i, n := range memberNames {
		var m map[string]any
		body := map[string]any{"name": n, "gender": []string{"M", "F"}[i%2], "phone": fmt.Sprintf("08521000%04d", i+1),
			"city": "Magelang", "credit_limit": 1500000, "due_days": 14}
		if err := a.post("/members", body, &m); err != nil {
			return fmt.Errorf("member %s: %w", n, err)
		}
		d.members = append(d.members, str(m, "id"))
	}
	for i, it := range itemDefs {
		body := map[string]any{
			"sku": "", "barcode": it.barcode, "name": it.name, "cost": num(it.cost), "sell_price": num(it.price),
			"unit_id": d.unitID[it.unit], "category_id": d.catID[it.cat], "brand_id": d.brandID[it.brand],
			"supplier_id": d.supplier[it.supplier], "min_stock": num(10),
		}
		var m map[string]any
		if err := a.post("/items", body, &m); err != nil {
			return fmt.Errorf("barang %s: %w", it.name, err)
		}
		d.items = append(d.items, str(m, "id"))
		_ = i
	}

	// metode pembayaran: ambil id; Debit diberi MDR toko 0,7% agar biaya admin muncul di akuntansi.
	var pm struct {
		Data []map[string]any `json:"data"`
	}
	if err := a.get("/payment-methods", &pm); err != nil {
		return err
	}
	for _, m := range pm.Data {
		d.method[str(m, "name")] = str(m, "id")
	}
	if id := d.method["Debit"]; id != "" {
		if err := a.put("/payment-methods/"+id, map[string]any{"name": "Debit", "kind": "debit", "fee_pct": "0.7", "fee_flat": "0", "fee_bearer": "store"}, nil); err != nil {
			d.logf("  ! gagal set MDR Debit: %v", err)
		}
	}
	for _, need := range []string{"Tunai", "Transfer", "Debit", "E-Wallet"} {
		if d.method[need] == "" {
			return fmt.Errorf("metode pembayaran %q tidak ada", need)
		}
	}
	d.logf("  %d barang, %d pemasok, %d member, %d outlet.", len(d.items), len(d.supplier), len(d.members), len(d.outlets))
	return nil
}
