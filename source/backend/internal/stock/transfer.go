package stock

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
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

// Mutasi stok (Fase 4.3 / 6.5): memindahkan barang antar CABANG dalam tenant yang sama dan/atau antar BUCKET.
// Antar cabang dua tahap: kirim (stok asal berkurang, status 'sent') → terima (stok tujuan bertambah sebesar qty
// diterima; selisih tercatat) atau batal (stok kembali ke asal). Antar bucket di cabang yang sama langsung selesai.
// Batas tenant dijaga DB (FK + RLS): cabang tenant lain tidak pernah terlihat.

var (
	ErrTransferNotPending = errors.New("mutasi sudah diterima atau dibatalkan")
)

// maxTransferLines = batas barang per dokumen mutasi.
const maxTransferLines = 200

const (
	TransferSent      = "sent"
	TransferReceived  = "received"
	TransferCancelled = "cancelled"
)

type TransferLineInput struct {
	ItemID uuid.UUID `json:"item_id"`
	Qty    string    `json:"qty"`
}

// TransferInput = permintaan kirim. Cabang asal selalu cabang aktif sesi (dari token).
type TransferInput struct {
	ToOutletID uuid.UUID           `json:"to_outlet_id"`
	FromBucket string              `json:"from_bucket"`
	ToBucket   string              `json:"to_bucket"`
	Note       string              `json:"note"`
	Lines      []TransferLineInput `json:"lines"`
}

type TransferOutlet struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

type TransferLine struct {
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	QtySent     string    `json:"qty_sent"`
	QtyReceived *string   `json:"qty_received"` // nil = belum diterima
	UnitCost    string    `json:"unit_cost"`
}

type Transfer struct {
	ID           uuid.UUID      `json:"id"`
	DocNo        string         `json:"doc_no"`
	From         TransferOutlet `json:"from"`
	FromBucket   string         `json:"from_bucket"`
	To           TransferOutlet `json:"to"`
	ToBucket     string         `json:"to_bucket"`
	Status       string         `json:"status"`
	Note         string         `json:"note"`
	SentBy       string         `json:"sent_by"`
	SentAt       time.Time      `json:"sent_at"`
	ReceivedBy   string         `json:"received_by,omitempty"`
	ReceivedAt   *time.Time     `json:"received_at"`
	CancelledBy  string         `json:"cancelled_by,omitempty"`
	CancelledAt  *time.Time     `json:"cancelled_at"`
	CancelReason string         `json:"cancel_reason,omitempty"`
	LineCount    int64          `json:"line_count"`
	QtySent      string         `json:"qty_sent"`
	QtyShort     string         `json:"qty_short"` // selisih kirim − terima (hanya berarti setelah diterima)
	Lines        []TransferLine `json:"lines,omitempty"`
}

type transferNorm struct {
	to         uuid.UUID
	fromB, toB string
	note       string
	items      []uuid.UUID
	qty        map[uuid.UUID]decimal.Decimal
}

func (n transferNorm) hash(from uuid.UUID) string {
	ids := append([]uuid.UUID(nil), n.items...)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%s|%s|%s", from, n.to, n.fromB, n.toB, n.note)
	for _, id := range ids {
		fmt.Fprintf(&b, "|%s=%s", id, n.qty[id])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func normalizeTransfer(in TransferInput, from uuid.UUID) (transferNorm, FieldErrors) {
	f := FieldErrors{}
	n := transferNorm{to: in.ToOutletID, fromB: in.FromBucket, toB: in.ToBucket, qty: map[uuid.UUID]decimal.Decimal{}}
	if n.to == uuid.Nil {
		f["to_outlet_id"] = sanitize.Required
	}
	if !validBuckets[n.fromB] {
		f["from_bucket"] = sanitize.Invalid
	}
	if !validBuckets[n.toB] {
		f["to_bucket"] = sanitize.Invalid
	}
	if len(f) == 0 && n.to == from && n.fromB == n.toB {
		f["to_bucket"] = "SAME_LOCATION" // asal dan tujuan sama persis
	}
	note, code := sanitize.Multiline(in.Note, maxNote)
	if code != "" {
		f["note"] = code
	}
	n.note = note
	if len(in.Lines) == 0 {
		f["lines"] = sanitize.Required
	} else if len(in.Lines) > maxTransferLines {
		f["lines"] = "TOO_MANY"
	}
	for i, l := range in.Lines {
		key := fmt.Sprintf("lines.%d.", i)
		if l.ItemID == uuid.Nil {
			f[key+"item_id"] = sanitize.Required
			continue
		}
		if _, dup := n.qty[l.ItemID]; dup {
			f[key+"item_id"] = "DUPLICATE"
			continue
		}
		q, c := parseQty(l.Qty)
		if c != "" {
			f[key+"qty"] = c
			continue
		}
		n.qty[l.ItemID] = q
		n.items = append(n.items, l.ItemID)
	}
	if len(f) > 0 {
		return n, f
	}
	return n, nil
}

func (s *Service) canAccess(a authz.Actor, outlet uuid.UUID) bool {
	return a.OutletID == outlet || a.Outlets[outlet]
}

func sortedIDs(ids []uuid.UUID) []uuid.UUID {
	out := append([]uuid.UUID(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func numToDecimal(n pgtype.Numeric) *decimal.Decimal {
	if !n.Valid || n.Int == nil {
		return nil
	}
	d := decimal.NewFromBigInt(n.Int, n.Exp)
	return &d
}

func decToNumeric(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

type transferReplay struct{ id uuid.UUID }

func (transferReplay) Error() string { return "replay" }

// Destinations = cabang aktif tenant yang bisa jadi tujuan (cabang aktif sesi ikut, untuk mutasi antar bucket).
func (s *Service) Destinations(ctx context.Context, a authz.Actor) ([]TransferOutlet, error) {
	out := []TransferOutlet{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).StockTrDestinations(ctx, a.TenantID)
		for _, r := range rows {
			out = append(out, TransferOutlet{ID: r.ID, Code: r.Code, Name: r.Name})
		}
		return err
	})
	return out, err
}

// SendTransfer membuat dokumen mutasi dari cabang aktif. Antar cabang: stok asal berkurang, status 'sent'.
// Antar bucket di cabang yang sama: langsung 'received'. replayed=true bila Idempotency-Key sama sudah pernah dipakai.
func (s *Service) SendTransfer(ctx context.Context, a authz.Actor, key string, in TransferInput) (t Transfer, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Transfer{}, false, ErrKeyRequired
	}
	n, f := normalizeTransfer(in, a.OutletID)
	if f != nil {
		return Transfer{}, false, f
	}
	h := n.hash(a.OutletID)
	local := n.to == a.OutletID

	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if ex, err := q.StockTrByIdemKey(ctx, gen.StockTrByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); err == nil {
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return transferReplay{ex.ID}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		from, err := q.StockTrOutlet(ctx, gen.StockTrOutletParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !from.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		if !local {
			to, err := q.StockTrOutlet(ctx, gen.StockTrOutletParams{TenantID: a.TenantID, ID: n.to})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !to.Active) {
				return FieldErrors{"to_outlet_id": sanitize.Invalid} // cabang tenant lain tidak terlihat → sama dengan tidak ada
			}
			if err != nil {
				return err
			}
		}

		// Kunci barang terurut id (HPP asal dibaca sesudah kunci didapat).
		costs := map[uuid.UUID]decimal.Decimal{}
		for _, id := range sortedIDs(n.items) {
			it, err := q.StockTrItemLock(ctx, gen.StockTrItemLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: id})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !it.Active) {
				return FieldErrors{lineKey(in, id): sanitize.Invalid}
			}
			if err != nil {
				return err
			}
			if it.Kind != "goods" {
				return ErrNotStocked
			}
			costs[id] = it.AvgCost
		}

		no, err := q.StockTrNextNo(ctx, gen.StockTrNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: from.LocalDay})
		if err != nil {
			return err
		}
		docNo := fmt.Sprintf("MT-%s-%s-%04d", strings.ToUpper(from.Code), from.LocalDay.Time.Format("060102"), no)
		id := uuid.New()
		status := TransferSent
		p := gen.StockTrInsertParams{ID: id, TenantID: a.TenantID, DocNo: docNo, IdempotencyKey: key, RequestHash: h,
			FromOutletID: a.OutletID, FromBucket: n.fromB, ToOutletID: n.to, ToBucket: n.toB, Status: status, Note: n.note,
			SentBy: nullUUID(a.UserID)}
		if local {
			p.Status = TransferReceived
			p.ReceivedBy = nullUUID(a.UserID)
			p.ReceivedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
		if _, err := q.StockTrInsert(ctx, p); errors.Is(err, pgx.ErrNoRows) {
			// Kiriman ganda bersamaan: yang lain menang. Batalkan transaksi ini (nomor tak terpakai).
			ex, e2 := q.StockTrByIdemKey(ctx, gen.StockTrByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
			if e2 != nil {
				return e2
			}
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return transferReplay{ex.ID}
		} else if err != nil {
			return err
		}

		ms := make([]Movement, 0, len(n.items)*2)
		for _, itemID := range n.items {
			qty := n.qty[itemID]
			lp := gen.StockTrLineInsertParams{TenantID: a.TenantID, TransferID: id, ItemID: itemID, QtySent: qty, UnitCost: costs[itemID].Round(2)}
			if local {
				lp.QtyReceived = decToNumeric(qty)
			}
			if err := q.StockTrLineInsert(ctx, lp); err != nil {
				return err
			}
			ms = append(ms, Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: itemID, Bucket: n.fromB, Delta: qty.Neg(),
				RefType: RefTransferOut, RefID: id, Note: docNo, ActorID: a.UserID})
			if local {
				ms = append(ms, Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: itemID, Bucket: n.toB, Delta: qty,
					RefType: RefTransferIn, RefID: id, Note: docNo, ActorID: a.UserID})
			}
		}
		if _, err := ApplyAll(ctx, tx, ms); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockTransferSend, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "from_outlet_id": a.OutletID.String(), "to_outlet_id": n.to.String(),
				"from_bucket": n.fromB, "to_bucket": n.toB, "lines": len(n.items), "status": p.Status}})
	})
	var rp transferReplay
	if errors.As(err, &rp) {
		t, err = s.GetTransfer(ctx, a, rp.id)
		return t, true, err
	}
	if err != nil {
		return Transfer{}, false, err
	}
	var id uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		ex, err := gen.New(tx).StockTrByIdemKey(ctx, gen.StockTrByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		id = ex.ID
		return err
	})
	if err != nil {
		return Transfer{}, false, err
	}
	t, err = s.GetTransfer(ctx, a, id)
	return t, false, err
}

// lineKey = nama field galat untuk barang id pada permintaan in.
func lineKey(in TransferInput, id uuid.UUID) string {
	for i, l := range in.Lines {
		if l.ItemID == id {
			return fmt.Sprintf("lines.%d.item_id", i)
		}
	}
	return "lines"
}

// ReceiveLine = qty yang benar-benar diterima untuk satu barang (0 ≤ qty ≤ dikirim).
type ReceiveLine struct {
	ItemID uuid.UUID `json:"item_id"`
	Qty    string    `json:"qty"`
}

func parseQtyZero(s string) (decimal.Decimal, string) {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero, sanitize.Required
	}
	q, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil || q.IsNegative() || !q.Equal(q.Round(3)) || q.GreaterThanOrEqual(maxQty) {
		return decimal.Zero, sanitize.Invalid
	}
	return q, ""
}

// ReceiveTransfer menerima kiriman di cabang tujuan. lines kosong = semua diterima penuh; bila diisi harus memuat setiap
// barang dokumen tepat sekali. Stok tujuan bertambah sebesar qty diterima dan HPP tujuan dihitung rata-rata tertimbang
// dari HPP asal (stok sebelumnya dibaca SETELAH kunci saldo didapat).
func (s *Service) ReceiveTransfer(ctx context.Context, a authz.Actor, id uuid.UUID, lines []ReceiveLine) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		hd, err := q.StockTrLock(ctx, gen.StockTrLockParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !s.canAccess(a, hd.ToOutletID) {
			return ErrOutletForbidden
		}
		if hd.Status != TransferSent {
			return ErrTransferNotPending
		}
		ls, err := q.StockTrLinesForMove(ctx, gen.StockTrLinesForMoveParams{TenantID: a.TenantID, TransferID: id})
		if err != nil {
			return err
		}
		recv := map[uuid.UUID]decimal.Decimal{}
		if len(lines) == 0 {
			for _, l := range ls {
				recv[l.ItemID] = l.QtySent
			}
		} else {
			sent := map[uuid.UUID]decimal.Decimal{}
			for _, l := range ls {
				sent[l.ItemID] = l.QtySent
			}
			f := FieldErrors{}
			for i, l := range lines {
				key := fmt.Sprintf("lines.%d.", i)
				mx, known := sent[l.ItemID]
				if !known {
					f[key+"item_id"] = sanitize.Invalid
					continue
				}
				if _, dup := recv[l.ItemID]; dup {
					f[key+"item_id"] = "DUPLICATE"
					continue
				}
				qv, c := parseQtyZero(l.Qty)
				if c != "" {
					f[key+"qty"] = c
					continue
				}
				if qv.GreaterThan(mx) {
					f[key+"qty"] = "OVER_SENT"
					continue
				}
				recv[l.ItemID] = qv
			}
			if len(f) == 0 && len(recv) != len(ls) {
				f["lines"] = sanitize.Invalid // harus memuat semua barang dokumen
			}
			if len(f) > 0 {
				return f
			}
		}

		// Kunci barang terurut (serialisasi penulis HPP lain), simpan HPP & HPP terakhir tujuan sebelum perubahan.
		type dest struct{ avg, last decimal.Decimal }
		dst := map[uuid.UUID]dest{}
		ids := make([]uuid.UUID, 0, len(ls))
		for _, l := range ls {
			ids = append(ids, l.ItemID)
		}
		for _, itemID := range sortedIDs(ids) {
			it, err := q.StockTrItemLock(ctx, gen.StockTrItemLockParams{TenantID: a.TenantID, OutletID: hd.ToOutletID, ID: itemID})
			if err != nil {
				return err
			}
			dst[itemID] = dest{it.AvgCost, it.LastCost}
		}

		ms := make([]Movement, 0, len(ls))
		for _, l := range ls {
			if r := recv[l.ItemID]; r.IsPositive() {
				ms = append(ms, Movement{TenantID: a.TenantID, OutletID: hd.ToOutletID, ItemID: l.ItemID, Bucket: hd.ToBucket, Delta: r,
					RefType: RefTransferIn, RefID: id, Note: hd.DocNo, ActorID: a.UserID})
			}
		}
		if _, err := ApplyAll(ctx, tx, ms); err != nil {
			return err
		}

		short := decimal.Zero
		for _, l := range ls {
			r := recv[l.ItemID]
			short = short.Add(l.QtySent.Sub(r))
			if err := q.StockTrLineSetReceived(ctx, gen.StockTrLineSetReceivedParams{TenantID: a.TenantID, TransferID: id, ItemID: l.ItemID, QtyReceived: decToNumeric(r)}); err != nil {
				return err
			}
			if !r.IsPositive() || hd.FromOutletID == hd.ToOutletID {
				continue
			}
			// Stok cabang tujuan sebelum penerimaan = stok sekarang (baris saldo sudah terkunci) − qty yang baru masuk.
			after, err := q.StockOutletQty(ctx, gen.StockOutletQtyParams{TenantID: a.TenantID, OutletID: hd.ToOutletID, ItemID: l.ItemID})
			if err != nil {
				return err
			}
			before := after.Sub(r)
			d := dst[l.ItemID]
			avg := l.UnitCost
			if before.IsPositive() && d.avg.IsPositive() {
				avg = before.Mul(d.avg).Add(r.Mul(l.UnitCost)).Div(before.Add(r)).Round(2)
			}
			if err := q.StockSetCost(ctx, gen.StockSetCostParams{TenantID: a.TenantID, OutletID: hd.ToOutletID, ItemID: l.ItemID, AvgCost: avg, LastCost: d.last}); err != nil {
				return err
			}
		}
		if err := q.StockTrSetReceived(ctx, gen.StockTrSetReceivedParams{TenantID: a.TenantID, ID: id, ActorID: nullUUID(a.UserID)}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockTransferReceive, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": hd.DocNo, "to_outlet_id": hd.ToOutletID.String(), "lines": len(ls), "short_qty": short.String()}})
	})
}

// CancelTransfer membatalkan kiriman yang belum diterima: stok dikembalikan ke cabang & bucket asal. Boleh oleh pihak
// pengirim maupun penerima (menolak kiriman).
func (s *Service) CancelTransfer(ctx context.Context, a authz.Actor, id uuid.UUID, reason string) error {
	reason, code := sanitize.Name(reason, 200)
	if code != "" || len([]rune(reason)) < 3 {
		return FieldErrors{"reason": sanitize.Invalid}
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		hd, err := q.StockTrLock(ctx, gen.StockTrLockParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !s.canAccess(a, hd.FromOutletID) && !s.canAccess(a, hd.ToOutletID) {
			return ErrOutletForbidden
		}
		if hd.Status != TransferSent {
			return ErrTransferNotPending
		}
		ls, err := q.StockTrLinesForMove(ctx, gen.StockTrLinesForMoveParams{TenantID: a.TenantID, TransferID: id})
		if err != nil {
			return err
		}
		ms := make([]Movement, 0, len(ls))
		for _, l := range ls {
			ms = append(ms, Movement{TenantID: a.TenantID, OutletID: hd.FromOutletID, ItemID: l.ItemID, Bucket: hd.FromBucket, Delta: l.QtySent,
				RefType: RefTransferIn, RefID: id, Note: hd.DocNo + " (batal)", ActorID: a.UserID})
		}
		if _, err := ApplyAll(ctx, tx, ms); err != nil {
			return err
		}
		if err := q.StockTrSetCancelled(ctx, gen.StockTrSetCancelledParams{TenantID: a.TenantID, ID: id, ActorID: nullUUID(a.UserID), Reason: reason}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockTransferCancel, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": hd.DocNo, "reason": reason}})
	})
}

func trOutlet(id uuid.UUID, code, name string) TransferOutlet {
	return TransferOutlet{ID: id, Code: code, Name: name}
}

func fixedQty(d decimal.Decimal) string { return d.StringFixed(3) }

// GetTransfer = dokumen lengkap dengan baris. Boleh dilihat pihak pengirim maupun penerima.
func (s *Service) GetTransfer(ctx context.Context, a authz.Actor, id uuid.UUID) (Transfer, error) {
	var out Transfer
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		r, err := q.StockTrGet(ctx, gen.StockTrGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !s.canAccess(a, r.FromOutletID) && !s.canAccess(a, r.ToOutletID) {
			return ErrOutletForbidden
		}
		out = Transfer{ID: r.ID, DocNo: r.DocNo, From: trOutlet(r.FromOutletID, r.FromCode, r.FromName), FromBucket: r.FromBucket,
			To: trOutlet(r.ToOutletID, r.ToCode, r.ToName), ToBucket: r.ToBucket, Status: r.Status, Note: r.Note,
			SentBy: r.SentByName, SentAt: r.SentAt.Time, ReceivedBy: r.ReceivedByName, ReceivedAt: tsPtr(r.ReceivedAt),
			CancelledBy: r.CancelledByName, CancelledAt: tsPtr(r.CancelledAt), CancelReason: r.CancelReason}
		ls, err := q.StockTrLines(ctx, gen.StockTrLinesParams{TenantID: a.TenantID, TransferID: id})
		if err != nil {
			return err
		}
		sent, short := decimal.Zero, decimal.Zero
		for _, l := range ls {
			tl := TransferLine{ItemID: l.ItemID, SKU: l.Sku, Name: l.Name, Unit: l.UnitName, QtySent: fixedQty(l.QtySent), UnitCost: l.UnitCost.StringFixed(2)}
			sent = sent.Add(l.QtySent)
			if rv := numToDecimal(l.QtyReceived); rv != nil {
				v := fixedQty(*rv)
				tl.QtyReceived = &v
				short = short.Add(l.QtySent.Sub(*rv))
			}
			out.Lines = append(out.Lines, tl)
		}
		out.LineCount = int64(len(ls))
		out.QtySent = fixedQty(sent)
		out.QtyShort = fixedQty(short)
		return nil
	})
	return out, err
}

// TransferListParams: Direction "out" (keluar dari cabang aktif, default) atau "in" (masuk ke cabang aktif).
type TransferListParams struct {
	Direction string
	Status    string
	Q         string
	Cursor    string
	Limit     int
}

type TransferSummary struct {
	ToReceive int64 `json:"to_receive"` // kiriman dari cabang lain yang menunggu diterima di cabang ini
	InTransit int64 `json:"in_transit"` // kiriman cabang ini yang belum diterima
}

type TransferPage struct {
	Data       []Transfer      `json:"data"`
	NextCursor string          `json:"next_cursor"`
	HasMore    bool            `json:"has_more"`
	Summary    TransferSummary `json:"summary"`
}

func encodeTrCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeTrCursor(c string) (time.Time, uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, false
	}
	p := strings.SplitN(string(raw), "|", 2)
	if len(p) != 2 {
		return time.Time{}, uuid.Nil, false
	}
	t, e1 := time.Parse(time.RFC3339Nano, p[0])
	id, e2 := uuid.Parse(p[1])
	return t, id, e1 == nil && e2 == nil
}

// ListTransfers = mutasi yang keluar/masuk cabang aktif, terbaru dulu (keyset).
func (s *Service) ListTransfers(ctx context.Context, a authz.Actor, p TransferListParams) (TransferPage, error) {
	dir := p.Direction
	if dir == "" {
		dir = "out"
	}
	f := FieldErrors{}
	if dir != "out" && dir != "in" {
		f["direction"] = sanitize.Invalid
	}
	if p.Status != "" && p.Status != TransferSent && p.Status != TransferReceived && p.Status != TransferCancelled {
		f["status"] = sanitize.Invalid
	}
	arg := gen.StockTrListParams{TenantID: a.TenantID, Direction: dir, OutletID: a.OutletID, Status: p.Status, Q: likeEscape(strings.TrimSpace(p.Q))}
	if p.Cursor != "" {
		at, id, ok := decodeTrCursor(p.Cursor)
		if !ok {
			f["cursor"] = sanitize.Invalid
		} else {
			arg.CursorAt = pgtype.Timestamptz{Time: at, Valid: true}
			arg.CursorID = pgtype.UUID{Bytes: id, Valid: true}
		}
	}
	if len(f) > 0 {
		return TransferPage{}, f
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	limit = min(limit, maxLimit)
	arg.PageLimit = int32(limit + 1)

	out := TransferPage{Data: []Transfer{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		rows, err := q.StockTrList(ctx, arg)
		if err != nil {
			return err
		}
		if len(rows) > limit {
			out.HasMore = true
			rows = rows[:limit]
		}
		for _, r := range rows {
			out.Data = append(out.Data, Transfer{ID: r.ID, DocNo: r.DocNo, From: trOutlet(r.FromOutletID, r.FromCode, r.FromName), FromBucket: r.FromBucket,
				To: trOutlet(r.ToOutletID, r.ToCode, r.ToName), ToBucket: r.ToBucket, Status: r.Status, Note: r.Note, SentBy: r.SentByName,
				SentAt: r.SentAt.Time, ReceivedAt: tsPtr(r.ReceivedAt), CancelledAt: tsPtr(r.CancelledAt),
				LineCount: r.LineCount, QtySent: fixedQty(r.QtySent), QtyShort: fixedQty(r.QtyShort)})
		}
		if out.HasMore && len(rows) > 0 {
			last := rows[len(rows)-1]
			out.NextCursor = encodeTrCursor(last.SentAt.Time, last.ID)
		}
		sm, err := q.StockTrSummary(ctx, gen.StockTrSummaryParams{TenantID: a.TenantID, OutletID: a.OutletID})
		out.Summary = TransferSummary{ToReceive: sm.ToReceive, InTransit: sm.InTransit}
		return err
	})
	return out, err
}
