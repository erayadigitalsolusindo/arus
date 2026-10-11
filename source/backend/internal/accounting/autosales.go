package accounting

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Jurnal otomatis penjualan (FR-ACC-02, 8a.3 tahap 2). Satu draf jurnal SALES per outlet per hari (hari menurut zona waktu
// outlet) dari nota berstatus completed; akuntan yang memposting. Idempoten lewat (source_type, source_ref):
//   - draf belum ada -> dibuat; draf ada -> isinya dihitung ulang (nota baru/void/edit);
//   - draf sudah diposting -> selisih terhadap yang sudah terposting dibuat sebagai draf "selisih" terpisah
//     (source_ref ...:adj:N); jurnal terposting tidak pernah diubah.
//
// Rumus (urutan hitung nota PRD §7.4; total = subtotal − diskon + pajak + biaya lain):
//
//	D kas/bank/QRIS per metode = dibayar + biaya tambahan pelanggan − MDR (tunai dikurangi kembalian)
//	D beban admin metode       = MDR (ditanggung toko maupun pelanggan)
//	D piutang usaha            = sisa nota kredit
//	D HPP / K persediaan       = Σ harga pokok baris
//	K penjualan = Σ(subtotal − diskon); K PPN keluaran = pajak toko + negara; K pendapatan lain = biaya lain + biaya tambahan
const (
	SourceSalesDaily = "sales_daily"
	maxGenerateDays  = 93
)

// MissingMappingError = peran/metode bayar yang belum punya akun sehingga draf tidak bisa dibuat.
type MissingMappingError struct{ Keys, Methods []string }

func (e *MissingMappingError) Error() string {
	return "pemetaan akun belum lengkap: " + strings.Join(append(slices.Clone(e.Keys), e.Methods...), ", ")
}

type GenerateInput struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	OutletID uuid.UUID `json:"outlet_id"` // kosong = semua outlet yang boleh diakses
}

type GenerateItem struct {
	OutletID  uuid.UUID  `json:"outlet_id"`
	Date      string     `json:"date"`
	Action    string     `json:"action"` // created | updated | deleted | adjusted | unchanged
	JournalID *uuid.UUID `json:"journal_id,omitempty"`
}

type GenerateResult struct {
	Items     []GenerateItem `json:"items"`
	Created   int            `json:"created"`
	Updated   int            `json:"updated"`
	Deleted   int            `json:"deleted"`
	Adjusted  int            `json:"adjusted"`
	Unchanged int            `json:"unchanged"`
}

// GenerateSalesDrafts membuat/menyegarkan draf jurnal penjualan harian untuk rentang tanggal. Tiap (outlet, hari) satu transaksi.
func (s *Service) GenerateSalesDrafts(ctx context.Context, a authz.Actor, in GenerateInput) (GenerateResult, error) {
	from, ok1 := parseDate(in.From)
	to, ok2 := parseDate(in.To)
	fe := FieldErrors{}
	if !ok1 {
		fe["from"] = sanitize.Invalid
	}
	if !ok2 {
		fe["to"] = sanitize.Invalid
	}
	if ok1 && ok2 && (to.Before(from) || to.Sub(from) > time.Duration(maxGenerateDays-1)*24*time.Hour) {
		fe["to"] = sanitize.Invalid
	}
	if len(fe) > 0 {
		return GenerateResult{}, fe
	}

	var outlets []uuid.UUID
	if in.OutletID != uuid.Nil {
		if !outletAllowed(a, in.OutletID) {
			return GenerateResult{}, ErrOutletForbidden
		}
		outlets = []uuid.UUID{in.OutletID}
	} else {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id FROM outlets WHERE tenant_id = $1 AND active ORDER BY code`, a.TenantID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					return err
				}
				if outletAllowed(a, id) {
					outlets = append(outlets, id)
				}
			}
			return rows.Err()
		})
		if err != nil {
			return GenerateResult{}, err
		}
	}

	res := GenerateResult{Items: []GenerateItem{}}
	for _, outlet := range outlets {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			var item *GenerateItem
			err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
				item, err = generateSalesDay(ctx, tx, a, outlet, day)
				return err
			})
			if err != nil {
				return res, err
			}
			if item == nil {
				continue
			}
			res.Items = append(res.Items, *item)
			switch item.Action {
			case "created":
				res.Created++
			case "updated":
				res.Updated++
			case "deleted":
				res.Deleted++
			case "adjusted":
				res.Adjusted++
			default:
				res.Unchanged++
			}
		}
	}
	if len(res.Items) > 0 {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionJournalAutoGenerate, Entity: audit.EntityJournal,
				Details: map[string]any{"source": SourceSalesDaily, "from": in.From, "to": in.To, "created": res.Created, "updated": res.Updated,
					"deleted": res.Deleted, "adjusted": res.Adjusted}})
		})
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func autoRef(outlet uuid.UUID, day time.Time) string {
	return outlet.String() + ":" + day.Format(dateLayout)
}

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// generateSalesDay menjalankan algoritma di atas untuk satu outlet-hari. nil = tidak ada penjualan dan tidak ada draf.
func generateSalesDay(ctx context.Context, tx pgx.Tx, a authz.Actor, outlet uuid.UUID, day time.Time) (*GenerateItem, error) {
	q := gen.New(tx)
	ref := autoRef(outlet, day)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, a.TenantID.String()+"|"+SourceSalesDaily+"|"+ref); err != nil {
		return nil, err
	}
	w, err := q.AutoOutletWindow(ctx, gen.AutoOutletWindowParams{Day: pgDate(day), TenantID: a.TenantID, OutletID: outlet})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, FieldErrors{"outlet_id": sanitize.Invalid}
	}
	if err != nil {
		return nil, err
	}
	target, n, err := salesTargetLines(ctx, q, a.TenantID, outlet, w.T0, w.T1, day)
	if err != nil {
		return nil, err
	}
	base, err := q.AutoJournalByRef(ctx, gen.AutoJournalByRefParams{TenantID: a.TenantID, SourceType: pgText(SourceSalesDaily), SourceRef: pgText(ref)})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	hasBase := err == nil
	item := func(action string, id uuid.UUID) (*GenerateItem, error) {
		return &GenerateItem{OutletID: outlet, Date: day.Format(dateLayout), Action: action, JournalID: &id}, nil
	}
	narr := fmt.Sprintf("Penjualan %s · %s (%d nota)", day.Format(dateLayout), w.Name, n)

	switch {
	case !hasBase && n == 0:
		return nil, nil

	case !hasBase:
		date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
		if err != nil {
			return nil, err
		}
		id, err := newAutoDraft(ctx, q, a, outlet, date, narr, ref, target)
		if err != nil {
			return nil, err
		}
		return item("created", id)

	case base.Status == "draft":
		if n == 0 {
			if _, err := q.JournalDeleteDraft(ctx, gen.JournalDeleteDraftParams{TenantID: a.TenantID, ID: base.ID}); err != nil {
				return nil, err
			}
			return item("deleted", base.ID)
		}
		date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
		if err != nil {
			return nil, err
		}
		if err := rewriteAutoDraft(ctx, q, a, base.ID, outlet, date, narr, target); err != nil {
			return nil, err
		}
		return item("updated", base.ID)
	}

	// Sudah diposting: hanya selisih yang boleh dicatat, sebagai draf terpisah.
	posted, err := q.AutoJournalPostedNet(ctx, gen.AutoJournalPostedNetParams{TenantID: a.TenantID, SourceType: pgText(SourceSalesDaily), Column3: ref})
	if err != nil {
		return nil, err
	}
	net := map[uuid.UUID]dec{}
	for _, l := range target {
		net[l.account] = net[l.account].Add(l.debit).Sub(l.credit)
	}
	for _, p := range posted {
		net[p.AccountID] = net[p.AccountID].Sub(p.Net)
	}
	accs := make([]uuid.UUID, 0, len(net))
	for id, v := range net {
		if !v.IsZero() {
			accs = append(accs, id)
		}
	}
	slices.SortFunc(accs, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	delta := make([]normLine, 0, len(accs))
	for _, id := range accs {
		l := normLine{account: id, memo: "Selisih penjualan " + day.Format(dateLayout)}
		if net[id].IsPositive() {
			l.debit = net[id]
		} else {
			l.credit = net[id].Neg()
		}
		delta = append(delta, l)
	}
	pending, err := q.AutoJournalPendingAdj(ctx, gen.AutoJournalPendingAdjParams{TenantID: a.TenantID, SourceType: pgText(SourceSalesDaily), Column3: ref})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	hasPending := err == nil
	switch {
	case len(delta) == 0 && !hasPending:
		return item("unchanged", base.ID)
	case len(delta) == 0:
		if _, err := q.JournalDeleteDraft(ctx, gen.JournalDeleteDraftParams{TenantID: a.TenantID, ID: pending}); err != nil {
			return nil, err
		}
		return item("deleted", pending)
	}
	adjNarr := fmt.Sprintf("Selisih penjualan %s · %s (nota diubah/dibatalkan setelah diposting)", day.Format(dateLayout), w.Name)
	date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
	if err != nil {
		return nil, err
	}
	if hasPending {
		if err := rewriteAutoDraft(ctx, q, a, pending, outlet, date, adjNarr, delta); err != nil {
			return nil, err
		}
		return item("adjusted", pending)
	}
	cnt, err := q.AutoJournalAdjCount(ctx, gen.AutoJournalAdjCountParams{TenantID: a.TenantID, SourceType: pgText(SourceSalesDaily), Column3: ref})
	if err != nil {
		return nil, err
	}
	id, err := newAutoDraft(ctx, q, a, outlet, date, adjNarr, fmt.Sprintf("%s:adj:%d", ref, cnt+1), delta)
	if err != nil {
		return nil, err
	}
	return item("adjusted", id)
}

// autoDraftDate = tanggal jurnal otomatis: hari penjualan; bila periodenya sudah ditutup, hari ini (agar masih bisa diposting).
func autoDraftDate(ctx context.Context, q *gen.Queries, tenant uuid.UUID, day, today time.Time) (time.Time, error) {
	closed := func(d time.Time) (bool, error) {
		st, err := q.PeriodStatusGet(ctx, gen.PeriodStatusGetParams{TenantID: tenant, StartDate: pgDate(monthStart(d))})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return st == "closed", err
	}
	c, err := closed(day)
	if err != nil || !c {
		return day, err
	}
	if c, err = closed(today); err != nil {
		return day, err
	}
	if c {
		return day, ErrPeriodClosed
	}
	return today, nil
}

func newAutoDraft(ctx context.Context, q *gen.Queries, a authz.Actor, outlet uuid.UUID, date time.Time, narr, ref string, lines []normLine) (uuid.UUID, error) {
	if _, err := checkAccounts(ctx, q, a.TenantID, accountIDs(lines), true); err != nil {
		return uuid.Nil, err
	}
	id, err := q.JournalInsert(ctx, gen.JournalInsertParams{TenantID: a.TenantID, OutletID: outlet, EntryDate: pgDate(date), Type: "SALES",
		Narration: narr, SourceType: pgText(SourceSalesDaily), SourceRef: pgText(ref), CreatedBy: a.UserID})
	if err != nil {
		return uuid.Nil, err
	}
	return id, insertLines(ctx, q, a.TenantID, id, date, lines)
}

func rewriteAutoDraft(ctx context.Context, q *gen.Queries, a authz.Actor, id, outlet uuid.UUID, date time.Time, narr string, lines []normLine) error {
	if _, err := checkAccounts(ctx, q, a.TenantID, accountIDs(lines), true); err != nil {
		return err
	}
	if _, err := q.JournalUpdateDraft(ctx, gen.JournalUpdateDraftParams{TenantID: a.TenantID, ID: id, OutletID: outlet, EntryDate: pgDate(date), Narration: narr}); err != nil {
		return err
	}
	if err := q.JournalLinesDelete(ctx, gen.JournalLinesDeleteParams{TenantID: a.TenantID, EntryID: id}); err != nil {
		return err
	}
	return insertLines(ctx, q, a.TenantID, id, date, lines)
}

// salesTargetLines menghitung isi jurnal harian dari nota completed. n = jumlah nota (0 -> baris kosong).
func salesTargetLines(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, t0, t1 pgtype.Timestamptz, day time.Time) ([]normLine, int64, error) {
	tot, err := q.AutoSalesTotals(ctx, gen.AutoSalesTotalsParams{TenantID: tenant, OutletID: outlet, CreatedAt: t0, CreatedAt_2: t1})
	if err != nil {
		return nil, 0, err
	}
	if tot.N == 0 {
		return nil, 0, nil
	}
	pays, err := q.AutoSalesPayments(ctx, gen.AutoSalesPaymentsParams{TenantID: tenant, OutletID: outlet, CreatedAt: t0, CreatedAt_2: t1})
	if err != nil {
		return nil, 0, err
	}
	recv, err := q.AutoSalesReceivable(ctx, gen.AutoSalesReceivableParams{TenantID: tenant, OutletID: outlet, CreatedAt: t0, CreatedAt_2: t1})
	if err != nil {
		return nil, 0, err
	}
	cogs, err := q.AutoSalesCogs(ctx, gen.AutoSalesCogsParams{TenantID: tenant, OutletID: outlet, CreatedAt: t0, CreatedAt_2: t1})
	if err != nil {
		return nil, 0, err
	}
	maps, err := listMappings(ctx, q, tenant)
	if err != nil {
		return nil, 0, err
	}
	acc := map[string]uuid.UUID{}
	for _, m := range maps {
		if m.Account != nil {
			acc[m.Key] = m.Account.ID
		}
	}

	fee, surcharge := decimal.Zero, decimal.Zero
	for _, p := range pays {
		fee, surcharge = fee.Add(p.Fee), surcharge.Add(p.Surcharge)
	}
	otherIncome := tot.OtherCost.Add(surcharge)
	zero := decimal.Zero
	missing := &MissingMappingError{}
	need := func(key string, amount decimal.Decimal) uuid.UUID {
		if !amount.IsZero() {
			if id, ok := acc[key]; ok {
				return id
			}
			missing.Keys = append(missing.Keys, key)
		}
		return uuid.Nil
	}
	dayStr := day.Format(dateLayout)
	var lines []normLine
	add := func(account uuid.UUID, debit, credit decimal.Decimal, memo string) {
		if account != uuid.Nil && (!debit.IsZero() || !credit.IsZero()) {
			lines = append(lines, normLine{account: account, debit: debit, credit: credit, memo: memo})
		}
	}

	for _, p := range pays {
		net := p.Amount.Add(p.Surcharge).Sub(p.Fee) // uang yang benar-benar masuk setelah dipotong MDR
		if p.Kind == "cash" {
			net = net.Sub(tot.ChangeGiven) // kembalian keluar dari laci tunai
		}
		if net.IsZero() {
			continue
		}
		if !p.AccountID.Valid {
			missing.Methods = append(missing.Methods, p.Name)
			continue
		}
		memo := "Penerimaan " + p.Name
		if net.IsNegative() {
			add(uuid.UUID(p.AccountID.Bytes), zero, net.Neg(), memo)
		} else {
			add(uuid.UUID(p.AccountID.Bytes), net, zero, memo)
		}
	}
	add(need(MapFeeExpense, fee), fee, zero, "Biaya metode pembayaran (MDR)")
	add(need(MapReceivable, recv), recv, zero, "Piutang penjualan kredit")
	add(need(MapCOGS, cogs), cogs, zero, "Harga pokok penjualan")
	add(need(MapSalesRevenue, tot.Revenue), zero, tot.Revenue, "Penjualan "+dayStr)
	add(need(MapTaxOut, tot.Tax), zero, tot.Tax, "PPN / pajak penjualan")
	add(need(MapOtherIncome, otherIncome), zero, otherIncome, "Biaya lain & biaya tambahan metode bayar")
	add(need(MapInventory, cogs), zero, cogs, "Pengurangan persediaan")
	if len(missing.Keys)+len(missing.Methods) > 0 {
		return nil, 0, missing
	}

	var td, tc dec
	for _, l := range lines {
		td, tc = td.Add(l.debit), tc.Add(l.credit)
	}
	if !td.Equal(tc) {
		return nil, 0, fmt.Errorf("jurnal otomatis penjualan %s tidak seimbang (debit %s, kredit %s): %w", dayStr, td, tc, ErrJournalUnbalanced)
	}
	return lines, tot.N, nil
}

// BatchPostItem = hasil posting satu draf dalam posting massal.
type BatchPostItem struct {
	ID    uuid.UUID `json:"id"`
	OK    bool      `json:"ok"`
	Code  string    `json:"code,omitempty"`
	DocNo string    `json:"doc_no,omitempty"`
}

const maxBatchPost = 100

// PostBatch memposting banyak draf; tiap draf transaksi sendiri sehingga satu kegagalan tidak membatalkan yang lain.
func (s *Service) PostBatch(ctx context.Context, a authz.Actor, ids []uuid.UUID) ([]BatchPostItem, error) {
	if len(ids) == 0 || len(ids) > maxBatchPost {
		return nil, FieldErrors{"ids": sanitize.Invalid}
	}
	out := make([]BatchPostItem, 0, len(ids))
	for _, id := range ids {
		j, err := s.Post(ctx, a, id)
		if err != nil {
			out = append(out, BatchPostItem{ID: id, Code: postErrorCode(err)})
			continue
		}
		out = append(out, BatchPostItem{ID: id, OK: true, DocNo: j.DocNo})
	}
	return out, nil
}

func postErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrOutletForbidden):
		return "OUTLET_FORBIDDEN"
	case errors.Is(err, ErrJournalPosted):
		return "JOURNAL_POSTED"
	case errors.Is(err, ErrJournalUnbalanced):
		return "JOURNAL_UNBALANCED"
	case errors.Is(err, ErrJournalShape):
		return "JOURNAL_SHAPE_INVALID"
	case errors.Is(err, ErrAccountInvalid):
		return "ACCOUNT_INVALID"
	case errors.Is(err, ErrPeriodClosed):
		return "PERIOD_CLOSED"
	}
	return "INTERNAL"
}
