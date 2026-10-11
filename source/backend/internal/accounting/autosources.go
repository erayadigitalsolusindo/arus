package accounting

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	gen "aciraba/internal/gen"
)

// Sumber jurnal otomatis (satu draf per outlet per hari per sumber). Hanya membaca tabel transaksi; tidak ada hook di modul lain.
// Nota/dokumen berstatus void atau superseded tidak dihitung, sehingga pembatalan setelah posting muncul sebagai draf selisih.
const (
	SourceSalesDaily           = "sales_daily"
	SourceSalesReturnsDaily    = "sales_returns_daily"
	SourcePurchasesDaily       = "purchases_daily"
	SourcePurchaseReturnsDaily = "purchase_returns_daily"
	SourceReceivablesDaily     = "receivable_payments_daily"
	SourcePayablesDaily        = "payable_payments_daily"
	SourceWalletDaily          = "wallet_daily"
)

var dailySources = []dailySource{
	{SourceSalesDaily, "SALES", "Penjualan", computeSales},
	{SourceSalesReturnsDaily, "SALES", "Retur penjualan", computeSalesReturns},
	{SourcePurchasesDaily, "PURCHASE", "Pembelian", computePurchases},
	{SourcePurchaseReturnsDaily, "PURCHASE", "Retur pembelian", computePurchaseReturns},
	{SourceReceivablesDaily, "AR", "Pelunasan piutang", computeReceivablePayments},
	{SourcePayablesDaily, "AP", "Pembayaran hutang", computePayablePayments},
	{SourceWalletDaily, "WALLET", "Deposit member & kredit pemasok", computeWallet},
}

// Penjualan (urutan hitung nota PRD §7.4; total = subtotal − diskon + pajak + biaya lain):
//
//	D kas/bank/QRIS per metode = dibayar + biaya tambahan pelanggan − MDR (tunai dikurangi kembalian)
//	D beban admin metode       = MDR (ditanggung toko maupun pelanggan)
//	D piutang usaha            = sisa nota kredit
//	D HPP / K persediaan       = Σ harga pokok baris
//	K penjualan = Σ(subtotal − diskon); K PPN keluaran = pajak toko + negara; K pendapatan lain = biaya lain + biaya tambahan
func computeSales(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	p := gen.AutoSalesTotalsParams{TenantID: tenant, OutletID: outlet, CreatedAt: w.t0, CreatedAt_2: w.t1}
	tot, err := q.AutoSalesTotals(ctx, p)
	if err != nil || tot.N == 0 {
		return 0, err
	}
	pays, err := q.AutoSalesPayments(ctx, gen.AutoSalesPaymentsParams(p))
	if err != nil {
		return 0, err
	}
	recv, err := q.AutoSalesReceivable(ctx, gen.AutoSalesReceivableParams(p))
	if err != nil {
		return 0, err
	}
	cogs, err := q.AutoSalesCogs(ctx, gen.AutoSalesCogsParams(p))
	if err != nil {
		return 0, err
	}
	fee, surcharge := decimal.Zero, decimal.Zero
	for _, x := range pays {
		fee, surcharge = fee.Add(x.Fee), surcharge.Add(x.Surcharge)
		net := x.Amount.Add(x.Surcharge).Sub(x.Fee) // uang yang benar-benar masuk setelah dipotong MDR
		if x.Kind == "cash" {
			net = net.Sub(tot.ChangeGiven) // kembalian keluar dari laci tunai
		}
		b.drMethod(x.AccountID, x.Name, net, "Penerimaan "+x.Name)
	}
	b.drRole(MapFeeExpense, fee, "Biaya metode pembayaran (MDR)")
	b.drRole(MapReceivable, recv, "Piutang penjualan kredit")
	b.drRole(MapCOGS, cogs, "Harga pokok penjualan")
	b.crRole(MapSalesRevenue, tot.Revenue, "Penjualan")
	b.crRole(MapTaxOut, tot.Tax, "PPN / pajak penjualan")
	b.crRole(MapOtherIncome, tot.OtherCost.Add(surcharge), "Biaya lain & biaya tambahan metode bayar")
	b.crRole(MapInventory, cogs, "Pengurangan persediaan")
	return tot.N, nil
}

// Retur penjualan (biaya tambahan metode dan biaya lain tidak dikembalikan, keputusan 2026-10-10):
//
//	D penjualan = Σ(subtotal − diskon); D PPN keluaran; K piutang (potong piutang); K metode refund (kas/bank/deposit)
//	D persediaan / K HPP = harga pokok baris yang dikembalikan
func computeSalesReturns(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	p := gen.AutoSalesReturnTotalsParams{TenantID: tenant, OutletID: outlet, ReturnDate: w.date}
	tot, err := q.AutoSalesReturnTotals(ctx, p)
	if err != nil || tot.N == 0 {
		return 0, err
	}
	refunds, err := q.AutoSalesReturnRefunds(ctx, gen.AutoSalesReturnRefundsParams(p))
	if err != nil {
		return 0, err
	}
	cost, err := q.AutoSalesReturnCost(ctx, gen.AutoSalesReturnCostParams(p))
	if err != nil {
		return 0, err
	}
	b.drRole(MapSalesRevenue, tot.Revenue, "Retur penjualan")
	b.drRole(MapTaxOut, tot.Tax, "PPN retur penjualan")
	b.crRole(MapReceivable, tot.ReceivableCut, "Pemotongan piutang oleh retur")
	for _, r := range refunds {
		b.crMethod(r.AccountID, r.Name, r.Amount, "Pengembalian dana "+r.Name)
	}
	b.drRole(MapInventory, cost, "Persediaan kembali")
	b.crRole(MapCOGS, cost, "Pembalikan HPP")
	return tot.N, nil
}

// Pembelian (biaya lain ikut menambah nilai persediaan; tunai dibayar dari metode tunai):
//
//	D persediaan = subtotal + biaya lain; D PPN masukan; K hutang usaha (nota kredit); K kas (nota tunai)
func computePurchases(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	tot, err := q.AutoPurchaseTotals(ctx, gen.AutoPurchaseTotalsParams{TenantID: tenant, OutletID: outlet, PurchaseDate: w.date})
	if err != nil || tot.N == 0 {
		return 0, err
	}
	b.drRole(MapInventory, tot.Inventory, "Pembelian barang dagang")
	b.drRole(MapTaxIn, tot.Tax, "PPN masukan")
	b.crRole(MapPayable, tot.CreditTotal, "Hutang pembelian kredit")
	if !tot.CashTotal.IsZero() {
		cash, err := q.AutoCashMethod(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			b.missing.Methods = append(b.missing.Methods, "cash")
		} else if err != nil {
			return 0, err
		} else {
			b.crMethod(cash.AccountID, cash.Name, tot.CashTotal, "Pembayaran pembelian tunai")
		}
	}
	return tot.N, nil
}

// Retur pembelian:
//
//	D hutang usaha (potong hutang); D metode refund (kas/bank/kredit pemasok)
//	K persediaan = harga pokok baris; K PPN masukan; selisih nilai retur vs harga pokok ke selisih stok
func computePurchaseReturns(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	p := gen.AutoPurchaseReturnTotalsParams{TenantID: tenant, OutletID: outlet, ReturnDate: w.date}
	tot, err := q.AutoPurchaseReturnTotals(ctx, p)
	if err != nil || tot.N == 0 {
		return 0, err
	}
	refunds, err := q.AutoPurchaseReturnRefunds(ctx, gen.AutoPurchaseReturnRefundsParams(p))
	if err != nil {
		return 0, err
	}
	cost, err := q.AutoPurchaseReturnCost(ctx, gen.AutoPurchaseReturnCostParams(p))
	if err != nil {
		return 0, err
	}
	b.drRole(MapPayable, tot.PayableCut, "Pemotongan hutang oleh retur")
	for _, r := range refunds {
		b.drMethod(r.AccountID, r.Name, r.Amount, "Pengembalian dana "+r.Name)
	}
	b.crRole(MapInventory, cost, "Persediaan keluar (retur)")
	b.crRole(MapTaxIn, tot.Tax, "PPN masukan retur")
	b.crRole(MapStockVariance, tot.Subtotal.Sub(cost), "Selisih nilai retur terhadap harga pokok")
	return tot.N, nil
}

// Pelunasan piutang (MDR dan biaya tambahan seperti penjualan):
//
//	D kas/bank per metode = dibayar + biaya tambahan pelanggan − MDR; D beban admin metode; K piutang usaha; K pendapatan lain
func computeReceivablePayments(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	rows, err := q.AutoReceivablePayments(ctx, gen.AutoReceivablePaymentsParams{TenantID: tenant, OutletID: outlet, CreatedAt: w.t0, CreatedAt_2: w.t1})
	if err != nil {
		return 0, err
	}
	var n int64
	amount, fee, surcharge := decimal.Zero, decimal.Zero, decimal.Zero
	for _, r := range rows {
		n += r.N
		amount, fee, surcharge = amount.Add(r.Amount), fee.Add(r.Fee), surcharge.Add(r.Surcharge)
		b.drMethod(r.AccountID, r.Name, r.Amount.Add(r.Surcharge).Sub(r.Fee), "Pelunasan piutang via "+r.Name)
	}
	b.drRole(MapFeeExpense, fee, "Biaya metode pembayaran (MDR)")
	b.crRole(MapReceivable, amount, "Pelunasan piutang usaha")
	b.crRole(MapOtherIncome, surcharge, "Biaya tambahan metode bayar")
	return n, nil
}

// Pembayaran hutang: D hutang usaha; K kas/bank/kredit pemasok per metode.
func computePayablePayments(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	rows, err := q.AutoPayablePayments(ctx, gen.AutoPayablePaymentsParams{TenantID: tenant, OutletID: outlet, CreatedAt: w.t0, CreatedAt_2: w.t1})
	if err != nil {
		return 0, err
	}
	var n int64
	total := decimal.Zero
	for _, r := range rows {
		n += r.N
		total = total.Add(r.Amount)
		b.crMethod(r.AccountID, r.Name, r.Amount, "Pembayaran hutang via "+r.Name)
	}
	b.drRole(MapPayable, total, "Pelunasan hutang usaha")
	return n, nil
}

// Deposit member (top-up: D kas/bank, K deposit member; tarik: kebalikannya) dan pencairan kredit pemasok
// (D kas/bank, K uang muka pemasok). Pemakaian saldo untuk bayar nota/hutang sudah tercatat di sumber masing-masing.
func computeWallet(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (int64, error) {
	p := gen.AutoDepositMovesParams{TenantID: tenant, OutletID: outlet, CreatedAt: w.t0, CreatedAt_2: w.t1}
	deposits, err := q.AutoDepositMoves(ctx, p)
	if err != nil {
		return 0, err
	}
	cashOuts, err := q.AutoSupplierCashOuts(ctx, gen.AutoSupplierCashOutsParams(p))
	if err != nil {
		return 0, err
	}
	var n int64
	for _, d := range deposits { // amount bertanda: top-up (+) debit kas, tarik (−) kredit kas
		n += d.N
		memo := "Top-up deposit member via " + d.Name
		if d.Kind == "WITHDRAW" {
			memo = "Tarik deposit member via " + d.Name
		}
		b.drMethod(d.AccountID, d.Name, d.Amount, memo)
		b.crRole(MapMemberDeposit, d.Amount, "Deposit member")
	}
	for _, c := range cashOuts {
		n += c.N
		b.drMethod(c.AccountID, c.Name, c.Amount, "Pencairan kredit pemasok via "+c.Name)
		b.crRole(MapSupplierAdvance, c.Amount, "Kredit pemasok")
	}
	return n, nil
}
