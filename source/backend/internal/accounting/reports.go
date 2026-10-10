package accounting

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Laporan keuangan (fase B, FR-ACC-03): neraca saldo, laba rugi, neraca, kas/bank, jurnal umum. Semuanya baca-saja dan hanya
// jurnal terposting. Saldo diambil dari agregat account_period_balances (bulan penuh) ditambah baris jurnal ≤ 31 hari di bulan
// berjalan, sehingga biaya tidak tumbuh bersama riwayat jurnal.

type sums struct{ debit, credit decimal.Decimal }

// cumulativeBefore = Σdebit/Σkredit per akun untuk semua jurnal terposting dengan entry_date < before.
func cumulativeBefore(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, outlets []uuid.UUID, before time.Time) (map[uuid.UUID]sums, error) {
	rows, err := tx.Query(ctx, `
		SELECT account_id, sum(d), sum(c) FROM (
			SELECT account_id, debit AS d, credit AS c FROM account_period_balances
			WHERE tenant_id = $1 AND period_month < $2 AND ($4::uuid[] IS NULL OR outlet_id = ANY($4))
			UNION ALL
			SELECT l.account_id, l.debit, l.credit
			FROM journal_lines l JOIN journal_entries h ON h.tenant_id = l.tenant_id AND h.id = l.entry_id
			WHERE l.tenant_id = $1 AND l.entry_date >= $2 AND l.entry_date < $3 AND h.status = 'posted'
			  AND ($4::uuid[] IS NULL OR h.outlet_id = ANY($4))
		) x GROUP BY account_id`,
		tenant, pgDate(monthStart(before)), pgDate(before), outlets)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]sums{}
	for rows.Next() {
		var id uuid.UUID
		var s sums
		if err := rows.Scan(&id, &s.debit, &s.credit); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

// reportData = akun buku + saldo kumulatif sebelum `from` dan sampai `to` (inklusif), dalam satu transaksi (snapshot konsisten).
type reportData struct {
	accounts []Account
	opening  map[uuid.UUID]sums // entry_date < from
	closing  map[uuid.UUID]sums // entry_date <= to
}

func loadReport(ctx context.Context, s *Service, a authz.Actor, outlet uuid.UUID, from, to time.Time) (reportData, error) {
	outlets, err := scopeOutlets(a, outlet)
	if err != nil {
		return reportData{}, err
	}
	var rd reportData
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, parent_id, code, name, kind, class, normal_side, is_cash_bank, active FROM accounts
			WHERE tenant_id = $1 AND kind = 'ledger' ORDER BY code`, a.TenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var acc Account
			var parent *uuid.UUID
			if err := rows.Scan(&acc.ID, &parent, &acc.Code, &acc.Name, &acc.Kind, &acc.Class, &acc.NormalSide, &acc.IsCashBank, &acc.Active); err != nil {
				rows.Close()
				return err
			}
			acc.ParentID = parent
			rd.accounts = append(rd.accounts, acc)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if rd.opening, err = cumulativeBefore(ctx, tx, a.TenantID, outlets, from); err != nil {
			return err
		}
		rd.closing, err = cumulativeBefore(ctx, tx, a.TenantID, outlets, to.AddDate(0, 0, 1))
		return err
	})
	return rd, err
}

func (rd reportData) movement(id uuid.UUID) sums {
	c, o := rd.closing[id], rd.opening[id]
	return sums{c.debit.Sub(o.debit), c.credit.Sub(o.credit)}
}

// net = debit − kredit; sisi tampil: positif → debit, negatif → kredit.
func split(net decimal.Decimal) (debit, credit decimal.Decimal) {
	if net.IsNegative() {
		return decimal.Zero, net.Neg()
	}
	return net, decimal.Zero
}

func str(d decimal.Decimal) string { return d.StringFixed(2) }

func rangeFor(from, to string) (time.Time, time.Time, error) { return parseRange(from, to) }

// ---------------------------------------------------------------- neraca saldo

type TrialRow struct {
	AccountID     uuid.UUID `json:"account_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Class         string    `json:"class"`
	OpeningDebit  string    `json:"opening_debit"`
	OpeningCredit string    `json:"opening_credit"`
	Debit         string    `json:"debit"`
	Credit        string    `json:"credit"`
	ClosingDebit  string    `json:"closing_debit"`
	ClosingCredit string    `json:"closing_credit"`
}

type TrialBalance struct {
	From     string     `json:"from"`
	To       string     `json:"to"`
	Rows     []TrialRow `json:"rows"`
	Totals   TrialRow   `json:"totals"`
	Balanced bool       `json:"balanced"`
}

// TrialBalance = saldo awal, mutasi, dan saldo akhir per akun buku pada rentang; akun tanpa saldo/mutasi tidak ditampilkan.
func (s *Service) TrialBalance(ctx context.Context, a authz.Actor, outlet uuid.UUID, fromS, toS string) (TrialBalance, error) {
	from, to, err := rangeFor(fromS, toS)
	if err != nil {
		return TrialBalance{}, err
	}
	rd, err := loadReport(ctx, s, a, outlet, from, to)
	if err != nil {
		return TrialBalance{}, err
	}
	out := TrialBalance{Rows: []TrialRow{}}
	out.From, out.To = from.Format(dateLayout), to.Format(dateLayout)
	var tOD, tOC, tD, tC, tCD, tCC decimal.Decimal
	for _, acc := range rd.accounts {
		mv := rd.movement(acc.ID)
		o, c := rd.opening[acc.ID], rd.closing[acc.ID]
		onet, cnet := o.debit.Sub(o.credit), c.debit.Sub(c.credit)
		if onet.IsZero() && cnet.IsZero() && mv.debit.IsZero() && mv.credit.IsZero() {
			continue
		}
		od, oc := split(onet)
		cd, cc := split(cnet)
		out.Rows = append(out.Rows, TrialRow{AccountID: acc.ID, Code: acc.Code, Name: acc.Name, Class: acc.Class,
			OpeningDebit: str(od), OpeningCredit: str(oc), Debit: str(mv.debit), Credit: str(mv.credit), ClosingDebit: str(cd), ClosingCredit: str(cc)})
		tOD, tOC, tD, tC, tCD, tCC = tOD.Add(od), tOC.Add(oc), tD.Add(mv.debit), tC.Add(mv.credit), tCD.Add(cd), tCC.Add(cc)
	}
	out.Totals = TrialRow{OpeningDebit: str(tOD), OpeningCredit: str(tOC), Debit: str(tD), Credit: str(tC), ClosingDebit: str(tCD), ClosingCredit: str(tCC)}
	out.Balanced = tOD.Equal(tOC) && tD.Equal(tC) && tCD.Equal(tCC)
	return out, nil
}

// ---------------------------------------------------------------- laba rugi

type StatementLine struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Amount    string    `json:"amount"`
}

type StatementSection struct {
	Key   string          `json:"key"`
	Lines []StatementLine `json:"lines"`
	Total string          `json:"total"`
}

type IncomeStatement struct {
	From        string           `json:"from"`
	To          string           `json:"to"`
	Revenue     StatementSection `json:"revenue"`
	COGS        StatementSection `json:"cogs"`
	GrossProfit string           `json:"gross_profit"`
	Expenses    StatementSection `json:"expenses"`
	NetProfit   string           `json:"net_profit"`
}

func section(key string, rd reportData, class string, amountOf func(Account) decimal.Decimal) (StatementSection, decimal.Decimal) {
	sec := StatementSection{Key: key, Lines: []StatementLine{}}
	total := decimal.Zero
	for _, acc := range rd.accounts {
		if acc.Class != class {
			continue
		}
		amt := amountOf(acc)
		if amt.IsZero() {
			continue
		}
		sec.Lines = append(sec.Lines, StatementLine{AccountID: acc.ID, Code: acc.Code, Name: acc.Name, Amount: str(amt)})
		total = total.Add(amt)
	}
	sec.Total = str(total)
	return sec, total
}

// IncomeStatement = pendapatan − harga pokok − beban pada rentang (mutasi periode, bukan saldo kumulatif).
func (s *Service) IncomeStatement(ctx context.Context, a authz.Actor, outlet uuid.UUID, fromS, toS string) (IncomeStatement, error) {
	from, to, err := rangeFor(fromS, toS)
	if err != nil {
		return IncomeStatement{}, err
	}
	rd, err := loadReport(ctx, s, a, outlet, from, to)
	if err != nil {
		return IncomeStatement{}, err
	}
	amount := func(acc Account) decimal.Decimal {
		mv := rd.movement(acc.ID)
		return signed(acc.NormalSide, mv.debit, mv.credit)
	}
	out := IncomeStatement{From: from.Format(dateLayout), To: to.Format(dateLayout)}
	var rev, cogs, exp decimal.Decimal
	out.Revenue, rev = section("revenue", rd, "revenue", amount)
	out.COGS, cogs = section("cogs", rd, "cogs", amount)
	out.Expenses, exp = section("expenses", rd, "expense", amount)
	out.GrossProfit = str(rev.Sub(cogs))
	out.NetProfit = str(rev.Sub(cogs).Sub(exp))
	return out, nil
}

// ---------------------------------------------------------------- neraca

type BalanceSheet struct {
	AsOf        string           `json:"as_of"`
	Assets      StatementSection `json:"assets"`
	Liabilities StatementSection `json:"liabilities"`
	Equity      StatementSection `json:"equity"`
	// Laba yang belum ditutup ke ekuitas: Σ pendapatan − harga pokok − beban sejak awal pembukuan (nol bila sudah ada jurnal penutup).
	UnclosedProfit  string `json:"unclosed_profit"`
	TotalAssets     string `json:"total_assets"`
	TotalLiabEquity string `json:"total_liabilities_equity"`
	Balanced        bool   `json:"balanced"`
}

// BalanceSheet = posisi per tanggal `asOf` (saldo kumulatif). Aset = Kewajiban + Ekuitas + laba berjalan yang belum ditutup.
func (s *Service) BalanceSheet(ctx context.Context, a authz.Actor, outlet uuid.UUID, asOfS string) (BalanceSheet, error) {
	asOf, ok := parseDate(asOfS)
	if !ok {
		return BalanceSheet{}, FieldErrors{"as_of": sanitize.Invalid}
	}
	rd, err := loadReport(ctx, s, a, outlet, asOf, asOf)
	if err != nil {
		return BalanceSheet{}, err
	}
	bal := func(acc Account) decimal.Decimal {
		c := rd.closing[acc.ID]
		return signed(acc.NormalSide, c.debit, c.credit)
	}
	out := BalanceSheet{AsOf: asOf.Format(dateLayout)}
	var assets, liab, eq decimal.Decimal
	out.Assets, assets = section("assets", rd, "asset", bal)
	out.Liabilities, liab = section("liabilities", rd, "liability", bal)
	out.Equity, eq = section("equity", rd, "equity", bal)
	profit := decimal.Zero
	for _, acc := range rd.accounts {
		switch acc.Class {
		case "revenue":
			profit = profit.Add(bal(acc))
		case "cogs", "expense":
			profit = profit.Sub(bal(acc))
		}
	}
	out.UnclosedProfit = str(profit)
	out.TotalAssets = str(assets)
	rhs := liab.Add(eq).Add(profit)
	out.TotalLiabEquity = str(rhs)
	out.Balanced = assets.Equal(rhs)
	return out, nil
}

// ---------------------------------------------------------------- kas dan bank

type CashBankRow struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Opening   string    `json:"opening"`
	Debit     string    `json:"debit"`  // masuk
	Credit    string    `json:"credit"` // keluar
	Closing   string    `json:"closing"`
}

type CashBank struct {
	From  string        `json:"from"`
	To    string        `json:"to"`
	Rows  []CashBankRow `json:"rows"`
	Total CashBankRow   `json:"total"`
}

// CashBank = saldo awal, kas masuk/keluar, dan saldo akhir tiap akun kas/bank aktif pada rentang.
func (s *Service) CashBank(ctx context.Context, a authz.Actor, outlet uuid.UUID, fromS, toS string) (CashBank, error) {
	from, to, err := rangeFor(fromS, toS)
	if err != nil {
		return CashBank{}, err
	}
	rd, err := loadReport(ctx, s, a, outlet, from, to)
	if err != nil {
		return CashBank{}, err
	}
	out := CashBank{Rows: []CashBankRow{}, From: from.Format(dateLayout), To: to.Format(dateLayout)}
	var tO, tD, tC, tE decimal.Decimal
	for _, acc := range rd.accounts {
		if !acc.IsCashBank {
			continue
		}
		mv := rd.movement(acc.ID)
		o, c := rd.opening[acc.ID], rd.closing[acc.ID]
		open, end := o.debit.Sub(o.credit), c.debit.Sub(c.credit)
		if !acc.Active && open.IsZero() && end.IsZero() && mv.debit.IsZero() && mv.credit.IsZero() {
			continue
		}
		out.Rows = append(out.Rows, CashBankRow{AccountID: acc.ID, Code: acc.Code, Name: acc.Name, Opening: str(open), Debit: str(mv.debit), Credit: str(mv.credit), Closing: str(end)})
		tO, tD, tC, tE = tO.Add(open), tD.Add(mv.debit), tC.Add(mv.credit), tE.Add(end)
	}
	out.Total = CashBankRow{Opening: str(tO), Debit: str(tD), Credit: str(tC), Closing: str(tE)}
	return out, nil
}

// ---------------------------------------------------------------- jurnal umum

type GeneralJournalEntry struct {
	ID        uuid.UUID `json:"id"`
	DocNo     string    `json:"doc_no"`
	Date      string    `json:"date"`
	Type      string    `json:"type"`
	Narration string    `json:"narration"`
	Lines     []Line    `json:"lines"`
}

type GeneralJournalPage struct {
	Items      []GeneralJournalEntry `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

// GeneralJournal = jurnal terposting berurutan menurut tanggal naik beserta baris-barisnya; keyset (entry_date, id), rentang dibatasi.
func (s *Service) GeneralJournal(ctx context.Context, a authz.Actor, outlet uuid.UUID, fromS, toS, cursor string, limitIn int) (GeneralJournalPage, error) {
	from, to, err := parseRange(fromS, toS)
	if err != nil {
		return GeneralJournalPage{}, err
	}
	outlets, err := scopeOutlets(a, outlet)
	if err != nil {
		return GeneralJournalPage{}, err
	}
	limit := pageSize(limitIn)
	if limit > 50 { // setiap entri membawa barisnya; batasi lebih ketat daripada daftar biasa
		limit = 50
	}
	cd, cid, hasCursor := from, uuid.Nil, false
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(raw), "|")
		var d time.Time
		var ok bool
		var id uuid.UUID
		var e2 error
		if e == nil && len(parts) == 2 {
			d, ok = parseDate(parts[0])
			id, e2 = uuid.Parse(parts[1])
		}
		if !ok || e2 != nil {
			return GeneralJournalPage{}, FieldErrors{"cursor": sanitize.Invalid}
		}
		cd, cid, hasCursor = d, id, true
	}
	out := GeneralJournalPage{Items: []GeneralJournalEntry{}}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT h.id, COALESCE(h.doc_no, ''), h.entry_date, h.type, h.narration
			FROM journal_entries h
			WHERE h.tenant_id = $1 AND h.entry_date >= $2 AND h.entry_date <= $3 AND h.status = 'posted'
			  AND ($4::bool = false OR (h.entry_date, h.id) > ($5, $6))
			  AND ($7::uuid[] IS NULL OR h.outlet_id = ANY($7))
			ORDER BY h.entry_date, h.id LIMIT $8`,
			a.TenantID, pgDate(from), pgDate(to), hasCursor, pgDate(cd), cid, outlets, limit+1)
		if err != nil {
			return err
		}
		ids := []uuid.UUID{}
		byID := map[uuid.UUID]int{}
		for rows.Next() {
			var e GeneralJournalEntry
			var date time.Time
			if err := rows.Scan(&e.ID, &e.DocNo, &date, &e.Type, &e.Narration); err != nil {
				rows.Close()
				return err
			}
			if len(out.Items) == limit { // baris ke-(limit+1) hanya penanda halaman berikutnya
				last := out.Items[len(out.Items)-1]
				out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.Date + "|" + last.ID.String()))
				break
			}
			e.Date, e.Lines = date.Format(dateLayout), []Line{}
			byID[e.ID] = len(out.Items)
			ids = append(ids, e.ID)
			out.Items = append(out.Items, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil || len(ids) == 0 {
			return err
		}
		lines, err := tx.Query(ctx, `
			SELECT l.entry_id, l.line_no, l.account_id, a.code, a.name, l.debit, l.credit, l.memo
			FROM journal_lines l JOIN accounts a ON a.tenant_id = l.tenant_id AND a.id = l.account_id
			WHERE l.tenant_id = $1 AND l.entry_id = ANY($2) ORDER BY l.entry_id, l.line_no`, a.TenantID, ids)
		if err != nil {
			return err
		}
		defer lines.Close()
		for lines.Next() {
			var entry uuid.UUID
			var l Line
			var d, c decimal.Decimal
			if err := lines.Scan(&entry, &l.LineNo, &l.AccountID, &l.AccountCode, &l.AccountName, &d, &c, &l.Memo); err != nil {
				return err
			}
			l.Debit, l.Credit = str(d), str(c)
			i := byID[entry]
			out.Items[i].Lines = append(out.Items[i].Lines, l)
		}
		return lines.Err()
	})
	return out, err
}
