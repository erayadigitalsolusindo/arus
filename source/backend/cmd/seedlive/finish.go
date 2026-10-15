package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// finish: nomor dokumen disesuaikan dengan tanggal baru, modal awal dicatat, jurnal otomatis dibuat dari transaksi
// (penjualan, pembelian, retur, piutang, hutang, kas kecil, dll. — sumber yang sama dengan tombol "Buat Draf Otomatis"),
// lalu semua draf diposting oleh "akuntan".
func (d *demo) finish(days int) error {
	d.logf("[4/4] Nomor dokumen & akuntansi …")
	if err := d.bd.renumber(d.ctx); err != nil {
		return err
	}
	if err := d.api.useOutlet(d.outlets[0].id); err != nil {
		return err
	}
	todayMid := civil(d.today)
	first := todayMid.AddDate(0, 0, -days)
	last := todayMid // termasuk hari ini (hari berjalan) agar buku besar = buku pembantu

	if err := d.openingCapital(first); err != nil {
		d.logf("  ! modal awal dilewati: %v", err)
	}

	// jurnal otomatis per potongan ≤ 90 hari
	for from := first; !from.After(last); {
		to := from.AddDate(0, 0, 89)
		if to.After(last) {
			to = last
		}
		var res struct {
			Items []map[string]any `json:"items"`
		}
		if err := d.api.post("/accounting/auto/generate", map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")}, &res); err != nil {
			return fmt.Errorf("jurnal otomatis: %w", err)
		}
		d.logf("  draf otomatis %s .. %s: %d entri", from.Format("2006-01-02"), to.Format("2006-01-02"), len(res.Items))
		from = to.AddDate(0, 0, 1)
	}

	// posting semua draf (≤100 per panggilan)
	posted := 0
	for round := 0; round < 200; round++ {
		rows, err := listItems(d.api, "/accounting/journals?status=draft&limit=100&from="+first.Format("2006-01-02")+"&to="+last.Format("2006-01-02"))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, str(r, "id"))
		}
		var res struct {
			Items []map[string]any `json:"items"`
		}
		if err := d.api.post("/accounting/journals/post-batch", map[string]any{"ids": ids}, &res); err != nil {
			return fmt.Errorf("posting: %w", err)
		}
		okCount := 0
		for _, it := range res.Items {
			if str(it, "error") == "" {
				okCount++
			}
		}
		posted += okCount
		if okCount == 0 {
			d.logf("  ! posting berhenti: %d draf tidak bisa diposting (contoh: %v)", len(rows), res.Items)
			break
		}
	}
	d.logf("  %d jurnal diposting.", posted)

	// ringkasan neraca saldo
	var tb map[string]any
	if err := d.api.get("/accounting/reports/trial-balance?from="+first.Format("2006-01-02")+"&to="+last.Format("2006-01-02")+"&outlet=all", &tb); err == nil {
		raw, _ := json.Marshal(tb)
		var t struct {
			Totals map[string]any `json:"totals"`
		}
		_ = json.Unmarshal(raw, &t)
		d.logf("  neraca saldo: %v", t.Totals)
	}
	return nil
}

func (d *demo) openingCapital(first time.Time) error {
	rows, err := listItems(d.api, "/accounting/accounts?limit=500")
	if err != nil {
		return err
	}
	find := func(class string, names ...string) string {
		for _, n := range names {
			for _, r := range rows {
				if str(r, "class") == class && strings.Contains(strings.ToLower(str(r, "name")), n) {
					return str(r, "id")
				}
			}
		}
		return ""
	}
	cash := find("asset", "kas besar", "kas")
	bank := find("asset", "bank")
	equity := find("equity", "modal")
	if cash == "" || bank == "" || equity == "" {
		return fmt.Errorf("akun kas/bank/modal tidak ditemukan (kas=%q bank=%q modal=%q)", cash, bank, equity)
	}
	date := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	cap := decimal.NewFromInt(200000000)
	body := map[string]any{
		"date": date.Format("2006-01-02"), "narration": "Modal awal pemilik",
		"lines": []map[string]any{
			{"account_id": cash, "debit": json.Number("40000000"), "credit": json.Number("0"), "memo": "Kas awal"},
			{"account_id": bank, "debit": json.Number("160000000"), "credit": json.Number("0"), "memo": "Saldo bank awal"},
			{"account_id": equity, "debit": json.Number("0"), "credit": json.Number(cap.String()), "memo": "Modal pemilik"},
		},
	}
	return d.api.post("/accounting/opening", body, nil)
}
