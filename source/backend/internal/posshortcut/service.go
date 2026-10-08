// Package posshortcut: 16 slot pintasan barang di layar kasir. Preferensi pribadi per kasir (bukan transaksi):
// satu klik slot memasukkan barangnya ke keranjang; isi slot tetap sampai diganti atau dikosongkan.
package posshortcut

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
)

// Slots = jumlah slot per kasir (sama dengan CHECK di DB).
const Slots = 16

var (
	ErrBadSlot = errors.New("slot di luar jangkauan")
	ErrNoItem  = errors.New("barang tidak ditemukan")
)

// Shortcut = satu slot terisi. Price = harga efektif outlet aktif (server tetap menghitung ulang saat kasir).
type Shortcut struct {
	Slot        int       `json:"slot"`
	ItemID      uuid.UUID `json:"item_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Unit        string    `json:"unit"`
	Kind        string    `json:"kind"`
	Active      bool      `json:"active"`
	Price       string    `json:"price"`
	MainImageID *string   `json:"main_image_id"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) List(ctx context.Context, a authz.Actor) ([]Shortcut, error) {
	out := []Shortcut{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).PosShortcutList(ctx, gen.PosShortcutListParams{TenantID: a.TenantID, UserID: a.UserID, OutletID: a.OutletID})
		for _, r := range rows {
			price := r.DefaultPrice
			if n := r.OutletPrice; n.Valid && !n.NaN && n.Int != nil {
				price = decimal.NewFromBigInt(n.Int, n.Exp)
			}
			sc := Shortcut{Slot: int(r.Slot), ItemID: r.ID, SKU: r.Sku, Name: r.Name, Unit: r.UnitName, Kind: r.Kind, Active: r.Active, Price: price.StringFixed(2)}
			if r.MainImageID.Valid {
				id := uuid.UUID(r.MainImageID.Bytes).String()
				sc.MainImageID = &id
			}
			out = append(out, sc)
		}
		return err
	})
	return out, err
}

func (s *Service) Set(ctx context.Context, a authz.Actor, slot int, itemID uuid.UUID) error {
	if slot < 1 || slot > Slots {
		return ErrBadSlot
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return gen.New(tx).PosShortcutSet(ctx, gen.PosShortcutSetParams{TenantID: a.TenantID, UserID: a.UserID, Slot: int16(slot), ItemID: itemID})
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23503" { // barang tidak ada di tenant ini
		return ErrNoItem
	}
	return err
}

func (s *Service) Clear(ctx context.Context, a authz.Actor, slot int) error {
	if slot < 1 || slot > Slots {
		return ErrBadSlot
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return gen.New(tx).PosShortcutClear(ctx, gen.PosShortcutClearParams{TenantID: a.TenantID, UserID: a.UserID, Slot: int16(slot)})
	})
}
