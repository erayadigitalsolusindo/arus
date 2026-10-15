package main

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/shopspring/decimal"
)

type pendingRecv struct {
	id     string
	member string
	payOn  time.Time // tanggal simulasi
	rest   decimal.Decimal
}

type sim struct {
	recv   []pendingRecv
	recent []recentSale // nota tunai hari ini, calon retur
}

type recentSale struct {
	id    string
	lines int
}

var simState sim

func (d *demo) simulate(days int) error {
	d.logf("[3/4] Simulasi %d hari (penjualan, pembelian, hutang, piutang) …", days)
	todayMid := civil(d.today)
	for off := days; off >= 1; off-- {
		date := todayMid.AddDate(0, 0, -off)
		for oi := range d.outlets {
			if err := d.runBatch(oi, date, off == days, false); err != nil {
				return fmt.Errorf("%s %s: %w", date.Format("2006-01-02"), d.outlets[oi].name, err)
			}
		}
		if off%5 == 0 || off == 1 {
			d.logf("  … sampai %s: %d nota, %d faktur beli", date.Format("2006-01-02"), d.stats.sales, d.stats.purchases)
		}
	}
	// hari ini: sebagian hari berjalan (jam buka → sekarang), shift dibiarkan terbuka agar dasbor "hari ini" terisi
	for oi, o := range d.outlets {
		if d.today.Hour() < o.openHour+1 {
			continue
		}
		if err := d.runBatch(oi, todayMid, false, true); err != nil {
			return fmt.Errorf("hari ini %s: %w", o.name, err)
		}
	}
	return nil
}

// civil = tanggal kalender (jam 00:00 UTC) dari waktu lokal; dipakai untuk selisih hari.
func civil(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (d *demo) runBatch(oi int, date time.Time, first, today bool) error {
	o := d.outlets[oi]
	if err := d.api.useOutlet(o.id); err != nil {
		return err
	}
	t0, err := d.bd.dbNow(d.ctx)
	if err != nil {
		return err
	}
	realDay := civil(t0.In(d.loc))

	if first {
		if err := d.restock(oi, true); err != nil {
			return fmt.Errorf("stok awal: %w", err)
		}
	}
	// shift kasir
	var sh struct {
		ID     string           `json:"id"`
		Counts []map[string]any `json:"counts"`
	}
	if err := d.api.post("/shifts/open", map[string]any{"opening_cash": num(500000)}, &sh); err != nil {
		return fmt.Errorf("buka shift: %w", err)
	}
	simState.recent = nil

	if !first && int(date.Weekday()) == int(time.Monday) {
		if err := d.restock(oi, false); err != nil {
			return fmt.Errorf("restock: %w", err)
		}
	}
	if err := d.payReceivables(date); err != nil {
		return fmt.Errorf("bayar piutang: %w", err)
	}

	n := d.salesCount(o, date)
	if today {
		n = n * 6 / 10
	}
	before := d.stats.sales
	defer func() {
		d.logf("    %s %s: %d/%d nota, lewati %d baris", date.Format("01-02"), o.code, d.stats.sales-before, n, d.stats.skippedLines)
	}()
	for i := 0; i < n; i++ {
		if err := d.oneSale(oi, date); err != nil {
			return fmt.Errorf("nota: %w", err)
		}
	}
	d.maybeReturns()
	d.maybePettyCash()

	if !today {
		// tutup shift: hitung fisik = seharusnya (selisih nol). API menghitung rekap dengan jam proses Go, sedangkan nota
		// dicap jam database; bila jam keduanya sedikit berbeda (dev/Docker) rekap bisa terpotong → baca ulang & coba lagi.
		if err := d.closeShift(); err != nil {
			return fmt.Errorf("tutup shift: %w", err)
		}
		d.stats.shifts++
	}
	// hutang pemasok dibayar lewat bank oleh pemilik di luar shift kasir (agar rekap laci kasir tak negatif)
	if err := d.payPayables(date); err != nil {
		return fmt.Errorf("bayar hutang: %w", err)
	}

	t1, err := d.bd.dbNow(d.ctx)
	if err != nil {
		return err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), o.openHour, 0, 0, 0, d.loc).Add(time.Duration(d.rng.Intn(8)) * time.Minute)
	span := time.Duration(o.closeHour-o.openHour)*time.Hour - 20*time.Minute
	if today {
		span = d.today.Sub(start) - 3*time.Minute
	}
	offsetDays := int(date.Sub(realDay).Hours() / 24)
	if _, err := d.bd.shift(d.ctx, t0, t1, start, span, offsetDays); err != nil {
		return err
	}
	return nil
}

func (d *demo) salesCount(o outletInfo, date time.Time) int {
	base := float64(o.salesPerDay)
	switch date.Weekday() {
	case time.Saturday, time.Sunday:
		base *= 1.45
	case time.Friday:
		base *= 1.15
	case time.Tuesday:
		base *= 0.9
	}
	n := int(math.Round(base * (0.75 + d.rng.Float64()*0.5)))
	if n < 3 {
		n = 3
	}
	return n
}

// restock membeli barang dari pemasok (satu faktur per pemasok). Awal: stok sebulan; berikutnya tiap Senin barang laris.
func (d *demo) restock(oi int, initial bool) error {
	o := d.outlets[oi]
	realToday := civil(d.today)
	bySup := map[int][]map[string]any{}
	for i, it := range itemDefs {
		var qty int
		if initial {
			qty = int(math.Round(float64(it.weight*8+30) * o.stockFactor))
		} else {
			if it.weight < 5 {
				continue
			}
			qty = int(math.Round(float64(it.weight*9) * o.stockFactor))
		}
		if qty < 6 {
			continue
		}
		// HPP bergerak ±3% antar pembelian (dibulatkan 50 rupiah)
		cost := float64(it.cost) * (0.97 + d.rng.Float64()*0.06)
		cost = math.Round(cost/50) * 50
		bySup[it.supplier] = append(bySup[it.supplier], map[string]any{
			"item_id": d.items[i], "qty_display": num(qty), "qty_warehouse": num(0), "unit_price": num(int(cost)),
		})
	}
	for s := 0; s < len(d.supplier); s++ {
		lines := bySup[s]
		if len(lines) == 0 {
			continue
		}
		credit := d.rng.Float64() < 0.65
		body := map[string]any{
			"supplier_id": d.supplier[s], "supplier_invoice_no": fmt.Sprintf("INV-%s-%03d", suppliers[s].code[4:], d.rng.Intn(900)+100),
			"lines": lines, "payment_type": "cash",
		}
		if credit {
			body["payment_type"] = "credit"
			body["due_date"] = realToday.AddDate(0, 0, []int{14, 21, 30}[d.rng.Intn(3)]).Format("2006-01-02")
		}
		if err := d.api.post("/purchases", body, nil); err != nil {
			return fmt.Errorf("pembelian %s: %w", suppliers[s].name, err)
		}
		d.stats.purchases++
	}
	return nil
}

func listItems(a *api, path string) ([]map[string]any, error) {
	var out map[string]json.RawMessage
	if err := a.get(path, &out); err != nil {
		return nil, err
	}
	for _, k := range []string{"items", "data", "rows"} {
		if raw, ok := out[k]; ok {
			var rows []map[string]any
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, err
			}
			return rows, nil
		}
	}
	return nil, nil
}

// payPayables melunasi hutang pemasok yang jatuh tempo (≤ tanggal simulasi) lewat Transfer.
func (d *demo) payPayables(date time.Time) error {
	rows, err := listItems(d.api, "/payables?status=open&limit=200")
	if err != nil {
		return err
	}
	for _, r := range rows {
		due := str(r, "due_date")
		if due == "" || due > date.Format("2006-01-02") {
			continue
		}
		if str(r, "supplier_code") == "SUP-004" || d.rng.Float64() < 0.15 {
			continue // pemasok ini sengaja dibiarkan menunggak (demo hutang jatuh tempo); sisanya kadang telat sehari
		}
		bal := dec(str(r, "balance"))
		if bal.Sign() <= 0 {
			continue
		}
		body := map[string]any{"method_id": d.method["Transfer"], "amount": json.Number(bal.String()), "note": "Pelunasan hutang pemasok"}
		if err := d.api.post("/payables/"+str(r, "id")+"/payments", body, nil); err != nil {
			return fmt.Errorf("payment hutang %s: %w", str(r, "doc_no"), err)
		}
		d.stats.payPay++
	}
	return nil
}

func (d *demo) payReceivables(date time.Time) error {
	keep := simState.recv[:0]
	for _, p := range simState.recv {
		if p.payOn.After(date) {
			keep = append(keep, p)
			continue
		}
		amt := p.rest
		method := []string{"Tunai", "Tunai", "Transfer", "E-Wallet"}[d.rng.Intn(4)]
		partial := d.rng.Float64() < 0.3 && p.rest.GreaterThan(decimal.NewFromInt(40000))
		if partial {
			amt = p.rest.Div(decimal.NewFromInt(2)).Round(0)
		}
		body := map[string]any{"method_id": d.method[method], "amount": json.Number(amt.String()), "note": "Pembayaran piutang"}
		if err := d.api.post("/receivables/"+p.id+"/payments", body, nil); err != nil {
			return fmt.Errorf("bayar piutang %s: %w", p.id, err)
		}
		d.stats.recvPay++
		if v := d.memberTop[p.member]; v.Sign() > 0 {
			d.memberTop[p.member] = decimal.Max(v.Sub(amt), decimal.Zero)
		}
		if partial {
			p.rest = p.rest.Sub(amt)
			p.payOn = date.AddDate(0, 0, 7)
			keep = append(keep, p)
		}
	}
	simState.recv = keep
	return nil
}

func (d *demo) oneSale(oi int, date time.Time) error {
	// baris
	nLines := []int{1, 2, 3, 4, 5}[d.pick([]int{30, 30, 20, 12, 8})]
	weights := make([]int, len(itemDefs))
	for i, it := range itemDefs {
		weights[i] = it.weight
	}
	chosen := map[int]bool{}
	var lines []map[string]any
	for len(lines) < nLines {
		i := d.pick(weights)
		if chosen[i] {
			weights[i] = 0
			if allZero(weights) {
				break
			}
			continue
		}
		chosen[i] = true
		qty := 1 + d.rng.Intn(itemDefs[i].maxQty)
		lines = append(lines, map[string]any{"item_id": d.items[i], "qty": num(qty)})
	}

	// member (40%) — kredit hanya untuk member
	member := ""
	if d.rng.Float64() < 0.4 {
		member = d.members[d.rng.Intn(len(d.members))]
	}
	req := map[string]any{"lines": lines, "apply_tax": false}
	if member != "" {
		req["member_id"] = member
	}
	var q map[string]any
	if err := d.api.post("/sales/quote", req, &q); err != nil {
		return fmt.Errorf("quote: %w", err)
	}
	// buang baris yang stoknya kurang
	if ql, ok := q["lines"].([]any); ok {
		var keep []map[string]any
		for idx, l := range ql {
			m, _ := l.(map[string]any)
			if m != nil && str(m, "issue") != "" {
				d.stats.skippedLines++
				continue
			}
			keep = append(keep, lines[idx])
		}
		if len(keep) == 0 {
			return nil
		}
		if len(keep) != len(lines) {
			lines = keep
			req["lines"] = lines
			if err := d.api.post("/sales/quote", req, &q); err != nil {
				return fmt.Errorf("quote ulang: %w", err)
			}
		}
	}
	total := dec(str(q, "total"))
	if total.Sign() <= 0 {
		return nil
	}

	pays, credit := d.payments(total, member)
	req["payments"] = pays
	if credit {
		req["credit"] = true
	}
	var sale map[string]any
	err := d.api.post("/sales", req, &sale)
	if err != nil && credit {
		// ditolak (limit piutang dll.) → bayar tunai penuh
		req["credit"] = false
		req["payments"] = []map[string]any{{"method_id": d.method["Tunai"], "amount": json.Number(total.String())}}
		credit = false
		sale = nil
		err = d.api.post("/sales", req, &sale)
	}
	if err != nil {
		return err
	}
	d.stats.sales++
	if credit {
		if c, ok := sale["credit"].(map[string]any); ok && str(c, "id") != "" {
			rest := dec(str(c, "balance"))
			d.memberTop[member] = d.memberTop[member].Add(rest)
			simState.recv = append(simState.recv, pendingRecv{id: str(c, "id"), member: member, rest: rest,
				payOn: date.AddDate(0, 0, 5+d.rng.Intn(15))})
		}
	} else if len(pays) == 1 && pays[0]["method_id"] == d.method["Tunai"] {
		simState.recent = append(simState.recent, recentSale{id: str(sale, "id"), lines: len(lines)})
	}
	return nil
}

func allZero(w []int) bool {
	for _, x := range w {
		if x > 0 {
			return false
		}
	}
	return true
}

// payments memilih cara bayar: tunai (dengan kembalian), transfer, debit, e-wallet, split, atau kredit member.
func (d *demo) payments(total decimal.Decimal, member string) ([]map[string]any, bool) {
	r := d.rng.Float64()
	pay := func(m string, amt decimal.Decimal) map[string]any {
		return map[string]any{"method_id": d.method[m], "amount": json.Number(amt.String())}
	}
	switch {
	case member != "" && r > 0.86 && total.GreaterThanOrEqual(decimal.NewFromInt(30000)) &&
		d.memberTop[member].Add(total).LessThan(decimal.NewFromInt(1400000)):
		dp := total.Mul(decimal.NewFromFloat(d.rng.Float64() * 0.3)).Round(-3)
		if dp.Sign() <= 0 {
			return []map[string]any{}, true
		}
		return []map[string]any{pay("Tunai", dp)}, true
	case r < 0.58:
		tender := total
		if d.rng.Float64() < 0.55 {
			step := int64(5000)
			if total.GreaterThan(decimal.NewFromInt(50000)) {
				step = 10000
			}
			tender = total.Div(decimal.NewFromInt(step)).Ceil().Mul(decimal.NewFromInt(step))
		}
		return []map[string]any{pay("Tunai", tender)}, false
	case r < 0.72:
		return []map[string]any{pay("Transfer", total)}, false
	case r < 0.84:
		return []map[string]any{pay("Debit", total)}, false
	case r < 0.86:
		return []map[string]any{pay("E-Wallet", total)}, false
	default: // split tunai + e-wallet
		first := total.Div(decimal.NewFromInt(2)).Round(-2)
		return []map[string]any{pay("Tunai", first), pay("E-Wallet", total.Sub(first))}, false
	}
}

// maybeReturns: sesekali satu barang dari nota tunai hari ini diretur (uang kembali tunai).
func (d *demo) maybeReturns() {
	for _, s := range simState.recent {
		if d.rng.Float64() > 0.05 {
			continue
		}
		body := map[string]any{
			"sale_id": s.id, "lines": []map[string]any{{"position": 1, "qty": num(1)}},
			"note": "Barang rusak / tidak sesuai", "refund_method_id": d.method["Tunai"],
		}
		if err := d.api.post("/sales-returns", body, nil); err != nil {
			d.logf("  ! retur dilewati: %v", err)
			continue
		}
		d.stats.returns++
	}
}

var pettyWarned bool

func (d *demo) maybePettyCash() {
	if d.rng.Float64() > 0.5 {
		return
	}
	notes := []string{"Beli plastik belanja", "Ongkos kirim barang", "Beli galon air minum", "Perlengkapan kebersihan toko"}
	body := map[string]any{"direction": "out", "amount": num(10000 * (1 + d.rng.Intn(6))), "category": "operasional", "note": notes[d.rng.Intn(len(notes))]}
	if err := d.api.post("/shifts/petty-cash", body, nil); err != nil && !pettyWarned {
		pettyWarned = true
		d.logf("  ! kas kecil dilewati: %v", err)
	}
}

func (d *demo) closeShift() error {
	var lastErr error
	for try := 0; try < 5; try++ {
		if try > 0 {
			time.Sleep(time.Duration(try) * 1500 * time.Millisecond)
		}
		var cur struct {
			Shift struct {
				ID     string           `json:"id"`
				Counts []map[string]any `json:"counts"`
			} `json:"shift"`
		}
		if err := d.api.get("/shifts/current", &cur); err != nil {
			return err
		}
		counts := []map[string]any{}
		for _, c := range cur.Shift.Counts {
			counts = append(counts, map[string]any{"method_id": c["method_id"], "counted": json.Number(str(c, "expected"))})
		}
		err := d.api.post("/shifts/"+cur.Shift.ID+"/close", map[string]any{"counts": counts, "note": ""}, nil)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isCode(err, "SHIFT_RECAP_CHANGED") && !isCode(err, "SHIFT_DIFF_NOTE_REQUIRED") && !isCode(err, "VALIDATION") {
			return err
		}
	}
	return lastErr
}
