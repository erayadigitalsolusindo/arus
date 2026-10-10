package payable

// Pelunasan kolektif per pemasok: satu dokumen (payable_settlements) yang membagi uang ke beberapa nota hutang.
//   - mode "auto"   : sistem melunasi nota TERLAMA dulu (tanggal pembelian, lalu nomor dokumen); nota terakhir boleh sebagian.
//   - mode "manual" : pemakai memilih nota + jumlah per nota (cicilan boleh).
//
// Setiap alokasi tetap berupa baris payable_payments (settlement_id terisi), jadi saldo per nota tetap dihitung dari pembayaran.
// Semua nota terkait dikunci (nota pembelian, terurut id) sebelum saldo dibaca → aman dari bayar/edit/batal bersamaan.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/wallet"
)

const maxAllocations = 200

type SettleAlloc struct {
	PayableID uuid.UUID   `json:"payable_id"`
	Amount    json.Number `json:"amount"`
}

type SettleInput struct {
	SupplierID  uuid.UUID     `json:"supplier_id"`
	Mode        string        `json:"mode"` // auto | manual
	MethodID    uuid.UUID     `json:"method_id"`
	Amount      json.Number   `json:"amount"` // total uang (mode auto)
	Allocations []SettleAlloc `json:"allocations"`
	RefNo       string        `json:"ref_no"`
	Note        string        `json:"note"`
}

type Allocation struct {
	PayableID         uuid.UUID `json:"payable_id"`
	PurchaseDocNo     string    `json:"purchase_doc_no"`
	SupplierInvoiceNo string    `json:"supplier_invoice_no"`
	PurchaseDate      string    `json:"purchase_date"`
	DueDate           *string   `json:"due_date,omitempty"`
	Balance           string    `json:"balance,omitempty"`       // sisa sebelum pelunasan ini (hanya pratinjau)
	Amount            string    `json:"amount"`                  // bagian yang dibayar
	BalanceAfter      string    `json:"balance_after,omitempty"` // sisa sesudahnya (hanya pratinjau)
}

// Plan = pratinjau pembagian uang (tidak menulis apa pun).
type Plan struct {
	Mode        string       `json:"mode"`
	SupplierID  uuid.UUID    `json:"supplier_id"`
	Total       string       `json:"total"`
	Outstanding string       `json:"outstanding"` // total tunggakan pemasok ini (mode auto)
	OpenCount   int          `json:"open_count"`
	Allocations []Allocation `json:"allocations"`
}

type Settlement struct {
	ID           uuid.UUID    `json:"id"`
	DocNo        string       `json:"doc_no"`
	SupplierID   uuid.UUID    `json:"supplier_id"`
	SupplierName string       `json:"supplier_name"`
	Mode         string       `json:"mode"`
	MethodName   string       `json:"method_name"`
	Total        string       `json:"total"`
	RefNo        string       `json:"ref_no"`
	Note         string       `json:"note"`
	PaidBy       string       `json:"paid_by"`
	CreatedAt    time.Time    `json:"created_at"`
	Allocations  []Allocation `json:"allocations"`
}

type settleNorm struct {
	supplier uuid.UUID
	mode     string
	method   uuid.UUID
	total    dec // mode auto
	allocs   []allocNorm
	ref      string
	note     string
}

type allocNorm struct {
	id     uuid.UUID
	amount dec
}

func parseMoney(n json.Number) (dec, string) {
	str := strings.TrimSpace(n.String())
	d, err := decimal.NewFromString(str)
	switch {
	case str == "":
		return dec{}, sanitize.Required
	case err != nil || !d.IsPositive() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxAmount)):
		return dec{}, sanitize.Invalid
	}
	return d, ""
}

func normalizeSettle(in SettleInput, needMethod bool) (settleNorm, FieldErrors) {
	f := FieldErrors{}
	n := settleNorm{supplier: in.SupplierID, mode: in.Mode, method: in.MethodID}
	if in.SupplierID == uuid.Nil {
		f["supplier_id"] = sanitize.Required
	}
	if needMethod && in.MethodID == uuid.Nil {
		f["method_id"] = sanitize.Required
	}
	switch in.Mode {
	case "auto":
		d, code := parseMoney(in.Amount)
		if code != "" {
			f["amount"] = code
		}
		n.total = d
	case "manual":
		if len(in.Allocations) == 0 {
			f["allocations"] = sanitize.Required
		}
		if len(in.Allocations) > maxAllocations {
			f["allocations"] = "TOO_MANY"
		}
		for i, al := range in.Allocations {
			d, code := parseMoney(al.Amount)
			if code != "" {
				f[fmt.Sprintf("allocations.%d.amount", i)] = code
			}
			if al.PayableID == uuid.Nil {
				f[fmt.Sprintf("allocations.%d.payable_id", i)] = sanitize.Required
			}
			n.allocs = append(n.allocs, allocNorm{id: al.PayableID, amount: d})
		}
	default:
		f["mode"] = sanitize.Invalid
	}
	ref, ok := sanitize.Text(in.RefNo)
	if !ok || utf8.RuneCountInString(ref) > 100 {
		f["ref_no"] = sanitize.Invalid
	}
	n.ref = ref
	note, ok := sanitize.Text(in.Note)
	if !ok || utf8.RuneCountInString(note) > 200 {
		f["note"] = sanitize.Invalid
	}
	n.note = note
	return n, f
}

func (n settleNorm) hash() string {
	parts := []string{n.supplier.String(), n.mode, n.method.String(), n.total.String(), n.ref, n.note}
	for _, al := range n.allocs {
		parts = append(parts, al.id.String()+":"+al.amount.String())
	}
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type cand struct {
	id          uuid.UUID
	amount      dec
	paid        dec
	docNo       string
	invoiceNo   string
	date        pgtype.Date
	due         pgtype.Date
	balance     dec
	allocated   dec
	outletID    uuid.UUID
	purchaseDoc string
}

// plan memuat nota hutang pemasok (dikunci bila lock=true), membaca saldo SESUDAH kunci didapat, lalu membagi uang.
func (s *Service) plan(ctx context.Context, tx pgx.Tx, a authz.Actor, n settleNorm, lock bool) (Plan, []cand, error) {
	var ids []uuid.UUID
	if n.mode == "manual" {
		for _, al := range n.allocs {
			ids = append(ids, al.id)
		}
	}
	q := `
		SELECT pb.id, pb.amount, pu.doc_no, pu.supplier_invoice_no, pu.purchase_date, pb.due_date, pu.status
		FROM payables pb JOIN purchases pu ON pu.tenant_id = pb.tenant_id AND pu.id = pb.purchase_id
		WHERE pb.tenant_id = $1 AND pb.supplier_id = $2 AND pb.voided_at IS NULL AND pb.outlet_id = ANY($3::uuid[])
		  AND ($4::uuid[] IS NULL OR pb.id = ANY($4::uuid[]))
		ORDER BY pu.id`
	if lock {
		q += ` FOR UPDATE OF pu`
	}
	rows, err := tx.Query(ctx, q, a.TenantID, n.supplier, outletIDs(a), ids)
	if err != nil {
		return Plan{}, nil, err
	}
	var all []*cand
	byID := map[uuid.UUID]*cand{}
	for rows.Next() {
		c := &cand{}
		var status string
		if err := rows.Scan(&c.id, &c.amount, &c.docNo, &c.invoiceNo, &c.date, &c.due, &status); err != nil {
			rows.Close()
			return Plan{}, nil, err
		}
		if status != "completed" {
			continue
		}
		all = append(all, c)
		byID[c.id] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Plan{}, nil, err
	}
	if len(all) > 0 {
		cids := make([]uuid.UUID, len(all))
		for i, c := range all {
			cids[i] = c.id
		}
		// paid = pembayaran + potongan retur pembelian aktif (keduanya mengurangi saldo).
		prow, err := tx.Query(ctx, `
			SELECT pb.id, (SELECT coalesce(sum(x.amount), 0) FROM payable_payments x WHERE x.tenant_id = pb.tenant_id AND x.payable_id = pb.id) + `+returnedSQL+`
			FROM payables pb WHERE pb.tenant_id = $1 AND pb.id = ANY($2::uuid[])`, a.TenantID, cids)
		if err != nil {
			return Plan{}, nil, err
		}
		for prow.Next() {
			var id uuid.UUID
			var sum dec
			if err := prow.Scan(&id, &sum); err != nil {
				prow.Close()
				return Plan{}, nil, err
			}
			byID[id].paid = sum
		}
		prow.Close()
		if err := prow.Err(); err != nil {
			return Plan{}, nil, err
		}
	}
	for _, c := range all {
		c.balance = c.amount.Sub(c.paid)
	}

	plan := Plan{Mode: n.mode, SupplierID: n.supplier, Allocations: []Allocation{}}
	var used []*cand
	switch n.mode {
	case "auto":
		open := make([]*cand, 0, len(all))
		outstanding := decimal.Zero
		for _, c := range all {
			if c.balance.IsPositive() {
				open = append(open, c)
				outstanding = outstanding.Add(c.balance)
			}
		}
		sort.Slice(open, func(i, j int) bool {
			x, y := open[i], open[j]
			if !x.date.Time.Equal(y.date.Time) {
				return x.date.Time.Before(y.date.Time)
			}
			if x.docNo != y.docNo {
				return x.docNo < y.docNo
			}
			return x.id.String() < y.id.String()
		})
		plan.Outstanding, plan.OpenCount = outstanding.StringFixed(2), len(open)
		if len(open) == 0 {
			return plan, nil, FieldErrors{"amount": "SETTLED"}
		}
		if n.total.IsPositive() && n.total.GreaterThan(outstanding) {
			return plan, nil, FieldErrors{"amount": "OVERPAID"}
		}
		remaining := n.total
		for _, c := range open {
			if !remaining.IsPositive() {
				break
			}
			take := decimal.Min(c.balance, remaining)
			c.allocated = take
			remaining = remaining.Sub(take)
			used = append(used, c)
		}
	case "manual":
		seen := map[uuid.UUID]bool{}
		fe := FieldErrors{}
		for i, al := range n.allocs {
			c, ok := byID[al.id]
			switch {
			case !ok:
				fe[fmt.Sprintf("allocations.%d.payable_id", i)] = sanitize.Invalid
			case seen[al.id]:
				fe[fmt.Sprintf("allocations.%d.payable_id", i)] = "DUPLICATE"
			case !c.balance.IsPositive():
				fe[fmt.Sprintf("allocations.%d.amount", i)] = "SETTLED"
			case al.amount.GreaterThan(c.balance):
				fe[fmt.Sprintf("allocations.%d.amount", i)] = "OVERPAID"
			default:
				c.allocated = al.amount
				used = append(used, c)
			}
			seen[al.id] = true
		}
		if len(fe) > 0 {
			return plan, nil, fe
		}
	}
	total := decimal.Zero
	out := make([]cand, 0, len(used))
	for _, c := range used {
		total = total.Add(c.allocated)
		plan.Allocations = append(plan.Allocations, Allocation{PayableID: c.id, PurchaseDocNo: c.docNo, SupplierInvoiceNo: c.invoiceNo,
			PurchaseDate: c.date.Time.Format("2006-01-02"), DueDate: dateStr(c.due), Balance: c.balance.StringFixed(2),
			Amount: c.allocated.StringFixed(2), BalanceAfter: c.balance.Sub(c.allocated).StringFixed(2)})
		out = append(out, *c)
	}
	plan.Total = total.StringFixed(2)
	return plan, out, nil
}

// SettleQuote = pratinjau pembagian uang tanpa menyimpan apa pun.
func (s *Service) SettleQuote(ctx context.Context, a authz.Actor, in SettleInput) (Plan, error) {
	n, f := normalizeSettle(in, false)
	if len(f) > 0 {
		return Plan{}, f
	}
	var out Plan
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		p, _, e := s.plan(ctx, tx, a, n, false)
		out = p
		return e
	})
	return out, err
}

// Settle mencatat pelunasan kolektif di outlet aktif pemanggil. replayed=true bila Idempotency-Key yang sama sudah pernah dicatat.
func (s *Service) Settle(ctx context.Context, a authz.Actor, key string, in SettleInput) (out Settlement, replayed bool, err error) {
	if !idemKey.MatchString(key) {
		return Settlement{}, false, ErrKeyRequired
	}
	n, f := normalizeSettle(in, true)
	if len(f) > 0 {
		return Settlement{}, false, f
	}
	h := n.hash()
	var setID uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		check := func() error {
			var rh string
			var id uuid.UUID
			e := tx.QueryRow(ctx, `SELECT request_hash, id FROM payable_settlements WHERE tenant_id = $1 AND idempotency_key = $2`, a.TenantID, key).Scan(&rh, &id)
			if errors.Is(e, pgx.ErrNoRows) {
				return nil
			}
			if e != nil {
				return e
			}
			if rh != h {
				return ErrKeyMismatch
			}
			setID = id
			return replay{}
		}
		if e := check(); e != nil {
			return e
		}
		var supplierName string
		if e := tx.QueryRow(ctx, `SELECT name FROM suppliers WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.supplier).Scan(&supplierName); errors.Is(e, pgx.ErrNoRows) {
			return FieldErrors{"supplier_id": sanitize.Invalid}
		} else if e != nil {
			return e
		}
		plan, used, e := s.plan(ctx, tx, a, n, true)
		if e != nil {
			return e
		}
		// Pengiriman ganda bersamaan: yang kedua menunggu kunci nota, jadi kunci idempotensinya baru terlihat sekarang.
		if e := check(); e != nil {
			return e
		}
		total, _ := decimal.NewFromString(plan.Total)
		var (
			mKind, mName string
			mActive      bool
		)
		e = tx.QueryRow(ctx, `SELECT kind, name, active FROM payment_methods WHERE tenant_id = $1 AND id = $2`, a.TenantID, n.method).Scan(&mKind, &mName, &mActive)
		switch {
		case errors.Is(e, pgx.ErrNoRows):
			return FieldErrors{"method_id": sanitize.Invalid}
		case e != nil:
			return e
		case !mActive:
			return FieldErrors{"method_id": "METHOD_INACTIVE"}
		case mKind == wallet.KindDeposit:
			return FieldErrors{"method_id": sanitize.Invalid}
		}
		var (
			code   string
			active bool
			day    pgtype.Date
		)
		e = tx.QueryRow(ctx, `SELECT code, active, (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.OutletID).Scan(&code, &active, &day)
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !active) {
			return ErrOutletGone
		}
		if e != nil {
			return e
		}
		var no int64
		if e := tx.QueryRow(ctx, `
			INSERT INTO payable_payment_counters (tenant_id, outlet_id, day, last_no) VALUES ($1, $2, $3, 1)
			ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = payable_payment_counters.last_no + 1
			RETURNING last_no`, a.TenantID, a.OutletID, day).Scan(&no); e != nil {
			return e
		}
		docNo := fmt.Sprintf("PH-%s-%s-%04d", strings.ToUpper(code), day.Time.Format("060102"), no)
		user := pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}
		if e := tx.QueryRow(ctx, `
			INSERT INTO payable_settlements (tenant_id, outlet_id, supplier_id, doc_no, idempotency_key, request_hash, mode, method, method_id, method_name, total, ref_no, note, paid_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id`,
			a.TenantID, a.OutletID, n.supplier, docNo, key, h, n.mode, mKind, n.method, mName, total, n.ref, n.note, user).Scan(&setID); e != nil {
			return e
		}
		allocs := make([]map[string]string, 0, len(used))
		for i, c := range used {
			if _, e := tx.Exec(ctx, `
				INSERT INTO payable_payments (tenant_id, payable_id, outlet_id, doc_no, idempotency_key, request_hash, method, method_id, method_name, amount, ref_no, note, paid_by, settlement_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
				a.TenantID, c.id, a.OutletID, fmt.Sprintf("%s/%d", docNo, i+1), fmt.Sprintf("set:%s:%d", setID, i+1), h, mKind, n.method, mName,
				c.allocated, n.ref, n.note, user, setID); e != nil {
				return e
			}
			allocs = append(allocs, map[string]string{"payable_id": c.id.String(), "purchase_doc_no": c.docNo, "amount": c.allocated.String(),
				"balance_after": c.balance.Sub(c.allocated).String()})
		}
		if mKind == wallet.KindSupplierCredit {
			if _, e := wallet.SupplierCredit.Apply(ctx, tx, wallet.Move{TenantID: a.TenantID, OwnerID: n.supplier, OutletID: a.OutletID,
				Kind: wallet.CrPayablePayment, Amount: total.Neg(), RefID: setID, DocNo: docNo, ActorID: a.UserID}); e != nil {
				if errors.Is(e, wallet.ErrInsufficient) {
					return FieldErrors{"amount": "CREDIT_INSUFFICIENT"}
				}
				return e
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionPayableSettle, Entity: audit.EntityPayable, EntityID: setID.String(),
			Details: map[string]any{"doc_no": docNo, "supplier": supplierName, "mode": n.mode, "method": mName, "total": total.String(),
				"allocations": allocs, "outlet_id": a.OutletID.String()}})
	})
	var rp replay
	if errors.As(err, &rp) {
		out, err = s.GetSettlement(ctx, a, setID)
		return out, true, err
	}
	if err != nil {
		return Settlement{}, false, err
	}
	out, err = s.GetSettlement(ctx, a, setID)
	return out, false, err
}

// GetSettlement = satu pelunasan kolektif beserta alokasinya.
func (s *Service) GetSettlement(ctx context.Context, a authz.Actor, id uuid.UUID) (Settlement, error) {
	var out Settlement
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var outletID uuid.UUID
		var total dec
		e := tx.QueryRow(ctx, `
			SELECT st.id, st.outlet_id, st.doc_no, st.supplier_id, sp.name, st.mode, st.method_name, st.total, st.ref_no, st.note, coalesce(u.name, ''), st.created_at
			FROM payable_settlements st JOIN suppliers sp ON sp.tenant_id = st.tenant_id AND sp.id = st.supplier_id
			LEFT JOIN users u ON u.tenant_id = st.tenant_id AND u.id = st.paid_by
			WHERE st.tenant_id = $1 AND st.id = $2`, a.TenantID, id).Scan(&out.ID, &outletID, &out.DocNo, &out.SupplierID, &out.SupplierName, &out.Mode,
			&out.MethodName, &total, &out.RefNo, &out.Note, &out.PaidBy, &out.CreatedAt)
		if errors.Is(e, pgx.ErrNoRows) || (e == nil && !a.Outlets[outletID]) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		out.Total = total.StringFixed(2)
		out.Allocations = []Allocation{}
		rows, e := tx.Query(ctx, `
			SELECT pp.payable_id, pu.doc_no, pu.supplier_invoice_no, pu.purchase_date, pb.due_date, pp.amount
			FROM payable_payments pp JOIN payables pb ON pb.tenant_id = pp.tenant_id AND pb.id = pp.payable_id
			JOIN purchases pu ON pu.tenant_id = pb.tenant_id AND pu.id = pb.purchase_id
			WHERE pp.tenant_id = $1 AND pp.settlement_id = $2 ORDER BY pp.doc_no`, a.TenantID, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var al Allocation
			var pdate, due pgtype.Date
			var amt dec
			if e := rows.Scan(&al.PayableID, &al.PurchaseDocNo, &al.SupplierInvoiceNo, &pdate, &due, &amt); e != nil {
				return e
			}
			al.PurchaseDate, al.DueDate, al.Amount = pdate.Time.Format("2006-01-02"), dateStr(due), amt.StringFixed(2)
			out.Allocations = append(out.Allocations, al)
		}
		return rows.Err()
	})
	return out, err
}
