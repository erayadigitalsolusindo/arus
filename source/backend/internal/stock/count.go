package stock

import (
	"context"
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

// Stok opname (Fase 4.3). Sesi = satu outlet + satu bucket: draf → selesai (selisih jadi movement OPNAME) atau batal.
// Stok sistem saat barang ditambahkan = snapshot; saat selesai stok disesuaikan sebesar (hasil hitung − snapshot) terhadap
// saldo terkini, sehingga penjualan yang terjadi di sela penghitungan tidak tertimpa.

var (
	ErrCountNotDraft = errors.New("sesi opname sudah selesai atau dibatalkan")
	ErrCountEmpty    = errors.New("belum ada barang yang dihitung")
)

// maxCountLines = batas barang per sesi (jaga ukuran respons & lama transaksi penyelesaian).
const maxCountLines = 5000

const (
	KindSession = "session"
	KindQuick   = "quick"

	ModeReplace = "replace" // stok diganti sebesar yang dimasukkan
	ModeAdjust  = "adjust"  // jumlah yang dimasukkan = tambah (+) / kurang (−) terhadap stok
)

const (
	CountDraft     = "draft"
	CountCompleted = "completed"
	CountCancelled = "cancelled"
)

// CountSummary = ringkasan satu sesi (daftar).
type CountSummary struct {
	ID          uuid.UUID  `json:"id"`
	DocNo       string     `json:"doc_no"`
	Bucket      string     `json:"bucket"`
	Status      string     `json:"status"`
	Note        string     `json:"note"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	Lines       int64      `json:"lines"`
	Counted     int64      `json:"counted"`
	Differences int64      `json:"differences"`
	CompletedBy string     `json:"completed_by,omitempty"`
	CancelledBy string     `json:"cancelled_by,omitempty"`
	Kind        string     `json:"kind"`           // session | quick
	Mode        string     `json:"mode,omitempty"` // quick: replace | adjust
	DiffAmount  string     `json:"diff_amount"`    // Σ selisih × HPP baris yang sudah dihitung (nilai bersih)
}

// CountLine = satu barang dalam sesi. Counted nil = belum dihitung. Diff = hasil hitung − snapshot (pratinjau saat draf).
type CountLine struct {
	ItemID   uuid.UUID `json:"item_id"`
	SKU      string    `json:"sku"`
	Name     string    `json:"name"`
	Unit     string    `json:"unit"`
	Snapshot string    `json:"snapshot"`
	Current  string    `json:"current"`
	Counted  *string   `json:"counted"`
	Diff     *string   `json:"diff"`
	UnitCost string    `json:"unit_cost"`
	Value    *string   `json:"value"` // nilai selisih = diff × HPP (HPP saat selesai, atau HPP sekarang selagi draf)
}

// CountDetail = sesi lengkap beserta barisnya dan totalnya.
type CountDetail struct {
	CountSummary
	Items     []CountLine `json:"items"`
	DiffValue string      `json:"diff_value"` // Σ nilai selisih (positif = lebih, negatif = kurang)
	DiffPlus  string      `json:"diff_plus"`  // Σ selisih lebih
	DiffMinus string      `json:"diff_minus"` // Σ selisih kurang (nilai negatif)
}

// AddItemsInput = barang yang ditambahkan; penyaringnya digabung (union).
type AddItemsInput struct {
	All        bool        `json:"all"`
	ItemIDs    []uuid.UUID `json:"item_ids"`
	CategoryID uuid.UUID   `json:"category_id"`
	BrandID    uuid.UUID   `json:"brand_id"`
}

func numericOf(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func decimalOf(n pgtype.Numeric) (decimal.Decimal, bool) {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero, false
	}
	return decimal.NewFromBigInt(n.Int, n.Exp), true
}

func strPtr(n pgtype.Numeric) *string {
	d, ok := decimalOf(n)
	if !ok {
		return nil
	}
	s := d.String()
	return &s
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// parseCounted = hasil hitung ≥ 0, maks 3 desimal.
func parseCounted(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, false
	}
	q, err := decimal.NewFromString(s)
	if err != nil || q.IsNegative() || !q.Equal(q.Round(3)) || q.GreaterThanOrEqual(maxQty) {
		return decimal.Zero, false
	}
	return q, true
}

func (s *Service) accessible(a authz.Actor, outletID uuid.UUID) error {
	if outletID != a.OutletID && !a.Outlets[outletID] {
		return ErrOutletForbidden
	}
	return nil
}

// CreateCount membuat sesi draf kosong di outlet aktif.
func (s *Service) CreateCount(ctx context.Context, a authz.Actor, bucket, note string) (uuid.UUID, error) {
	f := FieldErrors{}
	if !validBuckets[bucket] {
		f["bucket"] = sanitize.Invalid
	}
	note, code := sanitize.Multiline(note, maxNote)
	if code != "" {
		f["note"] = code
	}
	if len(f) > 0 {
		return uuid.Nil, f
	}
	id := uuid.New()
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		out, err := q.StockConvOutletInfo(ctx, gen.StockConvOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !out.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		no, err := q.StockCountNextNo(ctx, gen.StockCountNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if err != nil {
			return err
		}
		docNo := fmt.Sprintf("OP-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		if err := q.StockCountInsert(ctx, gen.StockCountInsertParams{ID: id, TenantID: a.TenantID, OutletID: a.OutletID,
			DocNo: docNo, Bucket: bucket, Note: note, CreatedBy: nullUUID(a.UserID)}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockCountCreate, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(), "bucket": bucket}})
	})
	return id, err
}

// ListCounts = sesi opname outlet aktif, terbaru dulu. status kosong = semua.
func (s *Service) ListCounts(ctx context.Context, a authz.Actor, status, kind, q string, limit, offset int) ([]CountSummary, int, error) {
	if status != "" && status != CountDraft && status != CountCompleted && status != CountCancelled {
		return nil, 0, FieldErrors{"status": sanitize.Invalid}
	}
	if kind != "" && kind != KindSession && kind != KindQuick {
		return nil, 0, FieldErrors{"kind": sanitize.Invalid}
	}
	q = strings.TrimSpace(q)
	if len(q) > 64 {
		return nil, 0, FieldErrors{"q": sanitize.TooLong}
	}
	if limit <= 0 {
		limit = defLimit
	}
	out := []CountSummary{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).StockCountList(ctx, gen.StockCountListParams{TenantID: a.TenantID, OutletID: a.OutletID,
			Status: status, Kind: kind, Q: likeEscape(q), PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(offset, 0))})
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, CountSummary{ID: r.ID, DocNo: r.DocNo, Bucket: r.Bucket, Status: r.Status, Note: r.Note,
				CreatedBy: r.CreatedName, CreatedAt: r.CreatedAt.Time, CompletedAt: tsPtr(r.CompletedAt), CancelledAt: tsPtr(r.CancelledAt),
				Lines: r.LineCount, Counted: r.CountedCount, Differences: r.DiffCount,
				Kind: r.Kind, Mode: r.AdjustMode.String, DiffAmount: r.DiffValue.Round(2).StringFixed(2)})
			total = int(r.Total)
		}
		return nil
	})
	return out, total, err
}

// GetCount = sesi lengkap. Akses mengikuti outlet sesi.
func (s *Service) GetCount(ctx context.Context, a authz.Actor, id uuid.UUID) (CountDetail, error) {
	var out CountDetail
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		h, err := q.StockCountHeader(ctx, gen.StockCountHeaderParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := s.accessible(a, h.OutletID); err != nil {
			return err
		}
		lines, err := q.StockCountLines(ctx, gen.StockCountLinesParams{TenantID: a.TenantID, CountID: id})
		if err != nil {
			return err
		}
		out = CountDetail{CountSummary: CountSummary{ID: h.ID, DocNo: h.DocNo, Bucket: h.Bucket, Status: h.Status, Note: h.Note,
			CreatedBy: h.CreatedName, CreatedAt: h.CreatedAt.Time, CompletedAt: tsPtr(h.CompletedAt), CancelledAt: tsPtr(h.CancelledAt),
			CompletedBy: h.CompletedName, CancelledBy: h.CancelledName, Kind: h.Kind, Mode: h.AdjustMode.String}, Items: make([]CountLine, 0, len(lines))}
		plus, minus := decimal.Zero, decimal.Zero
		for _, l := range lines {
			cl := CountLine{ItemID: l.ItemID, SKU: l.Sku, Name: l.Name, Unit: l.UnitName,
				Snapshot: l.SnapshotQty.String(), Current: l.CurrentQty.String(), Counted: strPtr(l.CountedQty),
				UnitCost: l.ItemCost.StringFixed(2)}
			out.Lines++
			if counted, ok := decimalOf(l.CountedQty); ok {
				out.Counted++
				diff := counted.Sub(l.SnapshotQty)
				cost := l.ItemCost
				if h.Status == CountCompleted {
					if d, ok := decimalOf(l.Diff); ok {
						diff = d
					}
					if c, ok := decimalOf(l.UnitCost); ok {
						cost = c
					}
				}
				ds := diff.String()
				cl.Diff = &ds
				cl.UnitCost = cost.StringFixed(2)
				val := diff.Mul(cost).Round(2)
				vs := val.StringFixed(2)
				cl.Value = &vs
				if !diff.IsZero() {
					out.Differences++
					if val.IsPositive() {
						plus = plus.Add(val)
					} else {
						minus = minus.Add(val)
					}
				}
			}
			out.Items = append(out.Items, cl)
		}
		out.DiffPlus, out.DiffMinus = plus.StringFixed(2), minus.StringFixed(2)
		out.DiffValue = plus.Add(minus).StringFixed(2)
		return nil
	})
	return out, err
}

// lockDraft mengunci header sesi (exclusive=true untuk selesai/batal, berbagi untuk ubah baris) dan memastikan masih draf.
func (s *Service) lockDraft(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID, exclusive bool) (outletID uuid.UUID, docNo, bucket string, err error) {
	q := gen.New(tx)
	var row struct {
		OutletID uuid.UUID
		DocNo    string
		Bucket   string
		Status   string
	}
	if exclusive {
		r, e := q.StockCountLockUpdate(ctx, gen.StockCountLockUpdateParams{TenantID: a.TenantID, ID: id})
		row.OutletID, row.DocNo, row.Bucket, row.Status, err = r.OutletID, r.DocNo, r.Bucket, r.Status, e
	} else {
		r, e := q.StockCountLockShare(ctx, gen.StockCountLockShareParams{TenantID: a.TenantID, ID: id})
		row.OutletID, row.DocNo, row.Bucket, row.Status, err = r.OutletID, r.DocNo, r.Bucket, r.Status, e
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", "", ErrNotFound
	}
	if err != nil {
		return uuid.Nil, "", "", err
	}
	if err := s.accessible(a, row.OutletID); err != nil {
		return uuid.Nil, "", "", err
	}
	if row.Status != CountDraft {
		return uuid.Nil, "", "", ErrCountNotDraft
	}
	return row.OutletID, row.DocNo, row.Bucket, nil
}

// AddCountItems menambah barang ke sesi draf; barang yang sudah ada dilewati. Mengembalikan jumlah barang baru.
func (s *Service) AddCountItems(ctx context.Context, a authz.Actor, id uuid.UUID, in AddItemsInput) (int64, error) {
	if !in.All && len(in.ItemIDs) == 0 && in.CategoryID == uuid.Nil && in.BrandID == uuid.Nil {
		return 0, FieldErrors{"scope": sanitize.Required}
	}
	if len(in.ItemIDs) > maxCountLines {
		return 0, FieldErrors{"item_ids": "TOO_MANY"}
	}
	var added int64
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		outletID, docNo, bucket, err := s.lockDraft(ctx, tx, a, id, false)
		if err != nil {
			return err
		}
		q := gen.New(tx)
		ids := in.ItemIDs
		if ids == nil {
			ids = []uuid.UUID{}
		}
		added, err = q.StockCountAddItems(ctx, gen.StockCountAddItemsParams{CountID: id, OutletID: outletID, Bucket: bucket,
			TenantID: a.TenantID, AllItems: in.All, ItemIds: ids, CategoryID: nullUUID(in.CategoryID), BrandID: nullUUID(in.BrandID)})
		if err != nil {
			return err
		}
		n, err := q.StockCountLineCount(ctx, gen.StockCountLineCountParams{TenantID: a.TenantID, CountID: id})
		if err != nil {
			return err
		}
		if n > maxCountLines {
			return FieldErrors{"scope": "TOO_MANY"}
		}
		if added == 0 {
			return nil
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockCountItems, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "added": added}})
	})
	return added, err
}

// RemoveCountItem membuang satu barang dari sesi draf.
func (s *Service) RemoveCountItem(ctx context.Context, a authz.Actor, id, itemID uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if _, _, _, err := s.lockDraft(ctx, tx, a, id, false); err != nil {
			return err
		}
		n, err := gen.New(tx).StockCountRemoveItem(ctx, gen.StockCountRemoveItemParams{TenantID: a.TenantID, CountID: id, ItemID: itemID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SetCounted mengisi hasil hitung satu barang. counted kosong = kosongkan (kembali belum dihitung).
func (s *Service) SetCounted(ctx context.Context, a authz.Actor, id, itemID uuid.UUID, counted string) error {
	var num pgtype.Numeric
	if strings.TrimSpace(counted) != "" {
		q, ok := parseCounted(counted)
		if !ok {
			return FieldErrors{"counted_qty": sanitize.Invalid}
		}
		num = numericOf(q)
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if _, _, _, err := s.lockDraft(ctx, tx, a, id, false); err != nil {
			return err
		}
		n, err := gen.New(tx).StockCountSetQty(ctx, gen.StockCountSetQtyParams{CountedQty: num, ActorID: nullUUID(a.UserID),
			TenantID: a.TenantID, CountID: id, ItemID: itemID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CompleteCount menerapkan selisih (hasil hitung − snapshot) ke saldo terkini sebagai movement OPNAME, semuanya dalam
// satu transaksi: bila satu barang tak bisa disesuaikan (mis. stok jadi minus), seluruh penyelesaian dibatalkan.
func (s *Service) CompleteCount(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		outletID, docNo, bucket, err := s.lockDraft(ctx, tx, a, id, true)
		if err != nil {
			return err
		}
		q := gen.New(tx)
		cnt, err := q.StockCountCountedLines(ctx, gen.StockCountCountedLinesParams{TenantID: a.TenantID, CountID: id})
		if err != nil {
			return err
		}
		if cnt == 0 {
			return ErrCountEmpty
		}
		diffs, err := q.StockCountDiffLines(ctx, gen.StockCountDiffLinesParams{TenantID: a.TenantID, CountID: id})
		if err != nil {
			return err
		}
		ms := make([]Movement, 0, len(diffs))
		for _, d := range diffs {
			ms = append(ms, Movement{TenantID: a.TenantID, OutletID: outletID, ItemID: d.ItemID, Bucket: bucket, Delta: d.Delta,
				RefType: RefOpname, RefID: id, Note: docNo, ActorID: a.UserID})
		}
		if _, err := ApplyAll(ctx, tx, ms); err != nil {
			return err
		}
		if err := q.StockCountFinalizeLines(ctx, gen.StockCountFinalizeLinesParams{TenantID: a.TenantID, CountID: id}); err != nil {
			return err
		}
		if err := q.StockCountComplete(ctx, gen.StockCountCompleteParams{TenantID: a.TenantID, ID: id, ActorID: nullUUID(a.UserID)}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockCountComplete, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "outlet_id": outletID.String(), "bucket": bucket,
				"counted": cnt, "adjusted": len(ms)}})
	})
}

// CancelCount membatalkan sesi draf tanpa mengubah stok.
func (s *Service) CancelCount(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, docNo, _, err := s.lockDraft(ctx, tx, a, id, true)
		if err != nil {
			return err
		}
		q := gen.New(tx)
		if err := q.StockCountCancel(ctx, gen.StockCountCancelParams{TenantID: a.TenantID, ID: id, ActorID: nullUUID(a.UserID)}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockCountCancel, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo}})
	})
}

// CountsSummary = ringkasan 30 hari terakhir outlet aktif untuk kartu di halaman daftar.
type CountsSummary struct {
	Open      int64  `json:"open"`
	Done      int64  `json:"done"`
	DiffLines int64  `json:"diff_lines"`
	Plus      string `json:"plus"`
	Minus     string `json:"minus"`
}

func (s *Service) CountsSummary(ctx context.Context, a authz.Actor) (CountsSummary, error) {
	var out CountsSummary
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).StockCountSummary(ctx, gen.StockCountSummaryParams{TenantID: a.TenantID, OutletID: a.OutletID})
		if err != nil {
			return err
		}
		out = CountsSummary{Open: r.OpenCount, Done: r.DoneCount, DiffLines: r.DiffLines,
			Plus: r.PlusValue.Round(2).StringFixed(2), Minus: r.MinusValue.Round(2).StringFixed(2)}
		return nil
	})
	return out, err
}

// maxQuickLines = batas barang per opname langsung.
const maxQuickLines = 200

// QuickLine = satu barang pada opname langsung. Qty: mode replace = stok baru (≥ 0); mode adjust = tambah/kurang (≠ 0, boleh negatif).
type QuickLine struct {
	ItemID uuid.UUID `json:"item_id"`
	Qty    string    `json:"qty"`
}

type QuickInput struct {
	Bucket string      `json:"bucket"`
	Mode   string      `json:"mode"`
	Note   string      `json:"note"`
	Items  []QuickLine `json:"items"`
}

// QuickCount menerapkan opname langsung: tanpa draf, tanpa langkah konfirmasi. Seluruh barang diproses dalam satu transaksi
// dan dicatat sebagai satu dokumen berstatus selesai. Mode replace mengunci saldo bucket lebih dulu sehingga stok akhir
// tepat sama dengan yang dimasukkan walau ada penjualan bersamaan; mode adjust menambah/mengurangi secara atomik.
func (s *Service) QuickCount(ctx context.Context, a authz.Actor, in QuickInput) (uuid.UUID, error) {
	f := FieldErrors{}
	if !validBuckets[in.Bucket] {
		f["bucket"] = sanitize.Invalid
	}
	if in.Mode != ModeReplace && in.Mode != ModeAdjust {
		f["mode"] = sanitize.Invalid
	}
	note, code := sanitize.Multiline(in.Note, maxNote)
	if code != "" {
		f["note"] = code
	}
	if len(in.Items) == 0 {
		f["items"] = sanitize.Required
	} else if len(in.Items) > maxQuickLines {
		f["items"] = "TOO_MANY"
	}
	if len(f) > 0 {
		return uuid.Nil, f
	}
	type line struct {
		id  uuid.UUID
		qty decimal.Decimal
	}
	lines := make([]line, 0, len(in.Items))
	seen := map[uuid.UUID]bool{}
	for i, it := range in.Items {
		key := fmt.Sprintf("items.%d", i)
		if it.ItemID == uuid.Nil {
			f[key+".item_id"] = sanitize.Required
			continue
		}
		if seen[it.ItemID] {
			f[key+".item_id"] = "DUPLICATE"
			continue
		}
		seen[it.ItemID] = true
		q, err := decimal.NewFromString(strings.TrimSpace(it.Qty))
		switch {
		case err != nil || !q.Equal(q.Round(3)) || q.Abs().GreaterThanOrEqual(maxQty):
			f[key+".qty"] = sanitize.Invalid
		case in.Mode == ModeReplace && q.IsNegative():
			f[key+".qty"] = sanitize.Invalid
		case in.Mode == ModeAdjust && q.IsZero():
			f[key+".qty"] = sanitize.Invalid
		}
		lines = append(lines, line{it.ItemID, q})
	}
	if len(f) > 0 {
		return uuid.Nil, f
	}
	// Barang dikunci menurut urutan id (sama dengan ApplyAll) → tidak deadlock dengan transaksi lain.
	sort.Slice(lines, func(i, j int) bool { return lines[i].id.String() < lines[j].id.String() })

	id := uuid.New()
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		out, err := q.StockConvOutletInfo(ctx, gen.StockConvOutletInfoParams{TenantID: a.TenantID, ID: a.OutletID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !out.Active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		no, err := q.StockCountNextNo(ctx, gen.StockCountNextNoParams{TenantID: a.TenantID, OutletID: a.OutletID, Day: out.LocalDay})
		if err != nil {
			return err
		}
		docNo := fmt.Sprintf("OP-%s-%s-%04d", strings.ToUpper(out.Code), out.LocalDay.Time.Format("060102"), no)
		if err := q.StockCountInsertQuick(ctx, gen.StockCountInsertQuickParams{ID: id, TenantID: a.TenantID, OutletID: a.OutletID,
			DocNo: docNo, Bucket: in.Bucket, Note: note, AdjustMode: pgtype.Text{String: in.Mode, Valid: true}, ActorID: nullUUID(a.UserID)}); err != nil {
			return err
		}
		adjusted := 0
		for _, l := range lines {
			it, err := q.StockConvItemLock(ctx, gen.StockConvItemLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ID: l.id})
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrItemNotFound
			}
			if err != nil {
				return err
			}
			if it.Kind != "goods" {
				return ErrNotStocked
			}
			var before, delta decimal.Decimal
			if in.Mode == ModeReplace {
				if err := q.StockBalanceEnsure(ctx, gen.StockBalanceEnsureParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: l.id, Bucket: in.Bucket}); err != nil {
					return err
				}
				before, err = q.StockBalanceLock(ctx, gen.StockBalanceLockParams{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: l.id, Bucket: in.Bucket})
				if err != nil {
					return err
				}
				delta = l.qty.Sub(before)
			} else {
				delta = l.qty
			}
			after := before.Add(delta)
			if !delta.IsZero() {
				res, err := Apply(ctx, tx, Movement{TenantID: a.TenantID, OutletID: a.OutletID, ItemID: l.id, Bucket: in.Bucket, Delta: delta,
					RefType: RefOpname, RefID: id, Note: docNo, ActorID: a.UserID})
				if err != nil {
					return err
				}
				after = res.BalanceAfter
				before = after.Sub(delta)
				adjusted++
			}
			if err := q.StockCountLineInsertDone(ctx, gen.StockCountLineInsertDoneParams{TenantID: a.TenantID, CountID: id, ItemID: l.id,
				SnapshotQty: before, CountedQty: numericOf(after), Diff: numericOf(delta), UnitCost: numericOf(it.AvgCost), ActorID: nullUUID(a.UserID)}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionStockCountQuick, Entity: audit.EntityStock, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "outlet_id": a.OutletID.String(), "bucket": in.Bucket, "mode": in.Mode,
				"items": len(lines), "adjusted": adjusted}})
	})
	return id, err
}
