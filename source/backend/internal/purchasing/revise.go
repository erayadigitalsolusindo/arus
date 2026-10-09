package purchasing

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
)

// Edit dan batal pembelian (Fase 6.7). Nota tidak pernah ditimpa: edit = nota lama 'superseded' + revisi baru tertaut
// (nomor -R2, -R3, …); batal = 'void' + alasan. Dampak stok & HPP dibalik dengan movement PURCHASE_VOID dan HPP dihitung
// mundur. Batas hari memakai tenants.sale_edit_window_days, sama seperti penjualan.

const maxReasonLen = 200

// revision = data nota revisi yang menggantikan nota lama dalam rantai yang sama (diteruskan ke save).
type revision struct {
	id         uuid.UUID // id nota revisi, ditetapkan di muka agar nota lama bisa menunjuknya
	rootID     uuid.UUID
	number     int32
	supersedes uuid.UUID
	docNo      string
	reason     string
}

var docNoRe = regexp.MustCompile(`^(PB-.+?-\d{6}-\d{4})(-R\d+)?$`)

// baseDocNo: nomor dasar nota tanpa akhiran revisi (PB-MAIN-261009-0001-R2 → PB-MAIN-261009-0001).
func baseDocNo(doc string) string {
	if m := docNoRe.FindStringSubmatch(doc); m != nil {
		return m[1]
	}
	return doc
}

// checkReason memeriksa alasan edit/batal: 3–200 karakter setelah dipangkas.
func checkReason(reason string) (string, FieldErrors) {
	r := strings.TrimSpace(reason)
	f := FieldErrors{}
	switch n := utf8.RuneCountInString(r); {
	case n == 0:
		f["reason"] = sanitize.Required
	case n < 3:
		f["reason"] = sanitize.TooShort
	case n > maxReasonLen:
		f["reason"] = sanitize.TooLong
	}
	return r, f
}

// lockNota mengunci nota (FOR UPDATE) dan membaca konteks aturannya. Nota lintas tenant = ErrNotFound (RLS).
func lockNota(ctx context.Context, q *gen.Queries, a authz.Actor, id uuid.UUID) (gen.PurchaseLockForChangeRow, error) {
	nota, err := q.PurchaseLockForChange(ctx, gen.PurchaseLockForChangeParams{TenantID: a.TenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nota, ErrNotFound
	}
	return nota, err
}

// checkEditable: hanya nota 'completed' dalam batas hari edit tenant (dihitung menurut hari lokal outlet).
func checkEditable(nota gen.PurchaseLockForChangeRow) error {
	if nota.Status != "completed" {
		return ErrNotEditable
	}
	days := int32(nota.Today.Time.Sub(nota.CreatedDay.Time).Hours() / 24)
	if days > nota.WindowDays {
		return ErrEditWindowClosed
	}
	return nil
}

// reverseAvg: HPP rata-rata setelah pembelian dibatalkan, dihitung mundur dari HPP sekarang:
// (stok × HPP − Σ qty×HPP baris) ÷ (stok − qty). onHand = stok cabang sebelum pembalikan (sudah termasuk qty nota).
// Stok sisa ≤ 0 tidak bisa dihitung → ok=false dan HPP dibiarkan.
func reverseAvg(onHand, avg, qty, value dec) (dec, bool) {
	rem := onHand.Sub(qty)
	if !rem.IsPositive() {
		return avg, false
	}
	res := onHand.Mul(avg).Sub(value).DivRound(rem, 2)
	if res.IsNegative() {
		res = decimal.Zero
	}
	return res, true
}

// reverse membalikkan dampak satu nota: stok (movement PURCHASE_VOID, negatif), HPP rata-rata (dihitung mundur) dan
// HPP terakhir (kembali ke pembelian aktif sebelumnya). Barang dikunci terurut id, sama seperti pembelian.
func (s *Service) reverse(ctx context.Context, tx pgx.Tx, a authz.Actor, nota gen.PurchaseLockForChangeRow) error {
	q := gen.New(tx)
	lines, err := q.PurchaseLines(ctx, gen.PurchaseLinesParams{TenantID: a.TenantID, PurchaseID: nota.ID})
	if err != nil {
		return err
	}
	type agg struct{ disp, ware, qty, value dec }
	sums := map[uuid.UUID]*agg{}
	var ids []uuid.UUID
	for _, l := range lines {
		g := sums[l.ItemID]
		if g == nil {
			g = &agg{}
			sums[l.ItemID] = g
			ids = append(ids, l.ItemID)
		}
		g.disp = g.disp.Add(l.QtyDisplay)
		g.ware = g.ware.Add(l.QtyWarehouse)
		g.qty = g.qty.Add(l.Qty)
		g.value = g.value.Add(l.Qty.Mul(l.UnitCost))
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })

	type plan struct{ avg, last dec }
	plans := make(map[uuid.UUID]plan, len(ids))
	var moves []stock.Movement
	for _, id := range ids {
		if _, err := q.PurchaseItemLock(ctx, gen.PurchaseItemLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id}); err != nil {
			return err
		}
		// Baca ulang dengan pernyataan baru setelah kunci (lihat lockItems: EvalPlanQual).
		cur, err := q.PurchaseItemState(ctx, gen.PurchaseItemStateParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
		if err != nil {
			return err
		}
		g := sums[id]
		last := cur.LastCost
		prev, err := q.PurchaseLastUnitCost(ctx, gen.PurchaseLastUnitCostParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id, ExcludeID: nota.ID})
		if err == nil {
			last = prev
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		plans[id] = plan{avg: cur.AvgCost, last: last}

		if g.disp.IsPositive() {
			moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id, Bucket: stock.BucketDisplay,
				Delta: g.disp.Neg(), RefType: stock.RefPurchaseVoid, RefID: nota.ID, Note: nota.DocNo, ActorID: a.UserID})
		}
		if g.ware.IsPositive() {
			moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id, Bucket: stock.BucketWarehouse,
				Delta: g.ware.Neg(), RefType: stock.RefPurchaseVoid, RefID: nota.ID, Note: nota.DocNo, ActorID: a.UserID})
		}
	}
	if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
		return err
	}
	for _, id := range ids {
		pl := plans[id]
		// Stok dibaca SETELAH ApplyAll: baris saldo sudah terkunci, jadi penjualan bersamaan tak bisa menyela.
		// Stok sebelum pembalikan = stok sekarang + qty yang dibalik.
		now, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id})
		if err != nil {
			return err
		}
		g := sums[id]
		if next, ok := reverseAvg(now.Add(g.qty), pl.avg, g.qty, g.value); ok {
			pl.avg = next
		}
		if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: id, AvgCost: pl.avg, LastCost: pl.last}); err != nil {
			return err
		}
	}
	return nil
}

// Edit merevisi nota: nota lama dibalik (stok & HPP), ditandai digantikan, lalu revisi disimpan dengan aturan pembelian
// biasa. replayed=true bila Idempotency-Key yang sama sudah pernah dipakai (nota revisi itu dikembalikan, tanpa pembalikan ganda).
func (s *Service) Edit(ctx context.Context, a authz.Actor, id uuid.UUID, key string, in Request, reason string) (p Purchase, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Purchase{}, false, ErrKeyRequired
	}
	reason, f := checkReason(reason)
	if len(f) > 0 {
		return Purchase{}, false, f
	}
	n, f := normalize(in, true)
	if len(f) > 0 {
		return Purchase{}, false, f
	}
	am, f := computeAmounts(n)
	if len(f) > 0 {
		return Purchase{}, false, f
	}
	h := n.hash()

	var newID uuid.UUID
	err = s.tx(ctx, a, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if ex, e := q.PurchaseByIdemKey(ctx, gen.PurchaseByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); e == nil {
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return replay{ex.ID}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		nota, err := lockNota(ctx, q, a, id)
		if err != nil {
			return err
		}
		if nota.OutletID != a.OutletID {
			return ErrOutletMismatch
		}
		if err := checkEditable(nota); err != nil {
			return err
		}
		root := nota.ID
		if nota.RootID.Valid {
			root = uuid.UUID(nota.RootID.Bytes)
		}
		newID = uuid.New()
		number := nota.Revision + 1
		rv := &revision{id: newID, rootID: root, number: number, supersedes: nota.ID,
			docNo: baseDocNo(nota.DocNo) + "-R" + strconv.Itoa(int(number)), reason: reason}

		if err := s.reverse(ctx, tx, a, nota); err != nil {
			return err
		}
		// Tandai dulu (sebelum revisi di-insert) agar indeks unik nomor faktur (hanya 'completed') tidak bentrok.
		n2, err := q.PurchaseMarkSuperseded(ctx, gen.PurchaseMarkSupersededParams{TenantID: a.TenantID, ID: nota.ID,
			SupersededBy: nullUUID(newID), RevisedBy: nullUUID(a.UserID), Reason: reason})
		if err != nil {
			return err
		}
		if n2 == 0 {
			return ErrNotEditable
		}
		if err := q.PayableVoid(ctx, gen.PayableVoidParams{TenantID: a.TenantID, PurchaseID: nota.ID}); err != nil {
			return err
		}
		if _, err := s.save(ctx, tx, a, n, am, key, h, rv); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPurchaseSuperseded, Entity: audit.EntityPurchase, EntityID: nota.ID.String(),
			Details: map[string]any{"doc_no": nota.DocNo, "revision_id": newID.String(), "reason": reason},
		})
	})
	var rp replay
	if errors.As(err, &rp) {
		p, err = s.Get(ctx, a, rp.id)
		return p, true, err
	}
	if err != nil {
		return Purchase{}, false, err
	}
	p, err = s.Get(ctx, a, newID)
	return p, false, err
}

// Void membatalkan nota: dampak stok & HPP dibalik, nota tetap tercatat berstatus 'void' beserta alasannya.
func (s *Service) Void(ctx context.Context, a authz.Actor, id uuid.UUID, reason string) (Purchase, error) {
	reason, f := checkReason(reason)
	if len(f) > 0 {
		return Purchase{}, f
	}
	err := s.tx(ctx, a, func(tx pgx.Tx) error {
		q := gen.New(tx)
		nota, err := lockNota(ctx, q, a, id)
		if err != nil {
			return err
		}
		if !canAccessOutlet(a, nota.OutletID) {
			return ErrOutletForbidden
		}
		if err := checkEditable(nota); err != nil {
			return err
		}
		if err := s.reverse(ctx, tx, a, nota); err != nil {
			return err
		}
		n2, err := q.PurchaseSetVoided(ctx, gen.PurchaseSetVoidedParams{TenantID: a.TenantID, ID: id, Reason: reason, VoidedBy: nullUUID(a.UserID)})
		if err != nil {
			return err
		}
		if n2 == 0 {
			return ErrNotEditable
		}
		if err := q.PayableVoid(ctx, gen.PayableVoidParams{TenantID: a.TenantID, PurchaseID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPurchaseVoid, Entity: audit.EntityPurchase, EntityID: id.String(),
			Details: map[string]any{"doc_no": nota.DocNo, "reason": reason},
		})
	})
	if err != nil {
		return Purchase{}, err
	}
	return s.Get(ctx, a, id)
}
