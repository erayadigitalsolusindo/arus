// Package stock: ledger stok (Fase 4.1). Semua perubahan stok lewat Apply/ApplyAll di dalam transaksi pemanggil,
// sehingga saldo, movement, dan dokumen sumber (penjualan, pembelian, opname, ...) commit atau batal bersama.
// Qty selalu dalam satuan dasar item. Pemanggil bertanggung jawab atas izin & akses outlet.
package stock

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"context"

	gen "aciraba/internal/gen"
)

const (
	BucketDisplay   = "display"
	BucketWarehouse = "warehouse"
	BucketReturns   = "returns"
)

// Jenis referensi movement (harus sama dengan CHECK di DB).
const (
	RefOpening        = "OPENING"
	RefSale           = "SALE"
	RefSaleVoid       = "SALE_VOID"
	RefSaleReturn     = "SALE_RETURN"
	RefPurchase       = "PURCHASE"
	RefPurchaseReturn = "PURCHASE_RETURN"
	RefOpname         = "OPNAME"
	RefTransferOut    = "TRANSFER_OUT"
	RefTransferIn     = "TRANSFER_IN"
	RefUnitConversion = "UNIT_CONVERSION"
	RefAdjustment     = "ADJUSTMENT"
)

var (
	ErrInsufficient  = errors.New("stok tidak cukup")
	ErrNotStocked    = errors.New("barang ini tidak memiliki stok")
	ErrItemNotFound  = errors.New("item tidak ditemukan")
	ErrInvalid       = errors.New("movement stok tidak valid")
	ErrOpeningLocked = errors.New("saldo awal sudah dikunci")
)

var (
	validBuckets = map[string]bool{BucketDisplay: true, BucketWarehouse: true, BucketReturns: true}
	validRefs    = map[string]bool{RefOpening: true, RefSale: true, RefSaleVoid: true, RefSaleReturn: true, RefPurchase: true,
		RefPurchaseReturn: true, RefOpname: true, RefTransferOut: true, RefTransferIn: true, RefUnitConversion: true, RefAdjustment: true}
	maxQty = decimal.New(1, 12)
)

// Movement = satu perubahan stok. Delta positif menambah, negatif mengurangi.
type Movement struct {
	TenantID uuid.UUID
	OutletID uuid.UUID
	ItemID   uuid.UUID
	Bucket   string
	Delta    decimal.Decimal
	RefType  string
	RefID    uuid.UUID // dokumen sumber; uuid.Nil = tanpa dokumen
	Note     string
	ActorID  uuid.UUID // uuid.Nil = sistem
}

// Result = saldo bucket setelah movement dan id baris ledger.
type Result struct {
	MovementID   int64
	BalanceAfter decimal.Decimal
}

// InsufficientError menyebut movement mana yang ditolak (untuk pesan "stok X kurang" di nota).
type InsufficientError struct{ ItemID uuid.UUID }

func (e *InsufficientError) Error() string {
	return fmt.Sprintf("%s: item %s", ErrInsufficient, e.ItemID)
}
func (e *InsufficientError) Unwrap() error { return ErrInsufficient }

func (m Movement) validate() error {
	switch {
	case m.TenantID == uuid.Nil || m.OutletID == uuid.Nil || m.ItemID == uuid.Nil:
		return fmt.Errorf("%w: tenant/outlet/item wajib", ErrInvalid)
	case !validBuckets[m.Bucket]:
		return fmt.Errorf("%w: bucket %q", ErrInvalid, m.Bucket)
	case !validRefs[m.RefType]:
		return fmt.Errorf("%w: ref_type %q", ErrInvalid, m.RefType)
	case m.Delta.IsZero():
		return fmt.Errorf("%w: qty nol", ErrInvalid)
	case !m.Delta.Equal(m.Delta.Round(3)):
		return fmt.Errorf("%w: qty maksimal 3 desimal", ErrInvalid)
	case m.Delta.Abs().GreaterThanOrEqual(maxQty):
		return fmt.Errorf("%w: qty terlalu besar", ErrInvalid)
	case len([]rune(m.Note)) > 500:
		return fmt.Errorf("%w: catatan terlalu panjang", ErrInvalid)
	}
	return nil
}

func nullUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

// Apply menerapkan satu movement di transaksi tx (yang sudah di-SetTenant). Pengurangan melewati nol hanya
// bila item mengizinkan stok minus DAN bucket-nya display; selain itu ErrInsufficient dan transaksi dibatalkan
// oleh pemanggil. Dua kasir bersamaan menjual barang yang sama aman: baris saldo dikunci oleh satu pernyataan atomik.
func Apply(ctx context.Context, tx pgx.Tx, m Movement) (Result, error) {
	if err := m.validate(); err != nil {
		return Result{}, err
	}
	q := gen.New(tx)
	if m.RefType == RefOpening {
		// Saldo awal hanya boleh sebelum tanggal mulai operasional outlet dikunci (FR-ONB-07).
		st, err := q.StockOutletLockState(ctx, gen.StockOutletLockStateParams{TenantID: m.TenantID, ID: m.OutletID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Result{}, err
		}
		if st.StockLockedAt.Valid {
			return Result{}, ErrOpeningLocked
		}
	}
	info, err := q.StockItemInfo(ctx, gen.StockItemInfoParams{TenantID: m.TenantID, ID: m.ItemID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrItemNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if info.Kind != "goods" {
		return Result{}, ErrNotStocked
	}
	var bal decimal.Decimal
	if m.Delta.IsPositive() || (info.AllowNegativeStock && m.Bucket == BucketDisplay) {
		bal, err = q.StockAddDelta(ctx, gen.StockAddDeltaParams{TenantID: m.TenantID, OutletID: m.OutletID, ItemID: m.ItemID, Bucket: m.Bucket, Delta: m.Delta})
	} else {
		bal, err = q.StockSubtractGuarded(ctx, gen.StockSubtractGuardedParams{TenantID: m.TenantID, OutletID: m.OutletID, ItemID: m.ItemID, Bucket: m.Bucket, Delta: m.Delta})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, &InsufficientError{ItemID: m.ItemID}
	}
	if err != nil {
		return Result{}, err
	}
	id, err := q.StockMovementInsert(ctx, gen.StockMovementInsertParams{
		TenantID: m.TenantID, OutletID: m.OutletID, ItemID: m.ItemID, Bucket: m.Bucket, QtyDelta: m.Delta, BalanceAfter: bal,
		RefType: m.RefType, RefID: nullUUID(m.RefID), Note: m.Note, ActorID: nullUUID(m.ActorID),
	})
	if err != nil {
		return Result{}, err
	}
	return Result{MovementID: id, BalanceAfter: bal}, nil
}

// ApplyAll menerapkan banyak movement (nota dengan banyak baris, mutasi dua sisi). Urutan penguncian dibuat
// deterministik (outlet, item, bucket) agar dua transaksi yang menyentuh barang yang sama tidak saling deadlock;
// hasil dikembalikan menurut urutan masukan. Semua-atau-tidak-sama-sekali bila pemanggil membatalkan tx.
func ApplyAll(ctx context.Context, tx pgx.Tx, ms []Movement) ([]Result, error) {
	order := make([]int, len(ms))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := ms[order[a]], ms[order[b]]
		if x.OutletID != y.OutletID {
			return x.OutletID.String() < y.OutletID.String()
		}
		if x.ItemID != y.ItemID {
			return x.ItemID.String() < y.ItemID.String()
		}
		return x.Bucket < y.Bucket
	})
	out := make([]Result, len(ms))
	for _, i := range order {
		r, err := Apply(ctx, tx, ms[i])
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}

// Balances = saldo per bucket sebuah item di sebuah outlet (bucket tanpa baris = 0, tidak dikembalikan).
func Balances(ctx context.Context, tx pgx.Tx, tenantID, outletID, itemID uuid.UUID) (map[string]decimal.Decimal, error) {
	rows, err := gen.New(tx).StockBalancesByItem(ctx, gen.StockBalancesByItemParams{TenantID: tenantID, OutletID: outletID, ItemID: itemID})
	if err != nil {
		return nil, err
	}
	out := make(map[string]decimal.Decimal, len(rows))
	for _, r := range rows {
		out[r.Bucket] = r.Qty
	}
	return out, nil
}
