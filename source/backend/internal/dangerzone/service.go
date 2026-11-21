// Package dangerzone: pembersihan data transaksi satu tenant oleh Owner (penjualan, hutang-piutang-pembelian, reset metode bayar).
// Penghapusan nyata dilakukan fungsi SQL danger_* (migrasi 00120); paket ini memeriksa Owner + kata sandi + kata konfirmasi
// dan mencatat audit di transaksi yang sama.
package dangerzone

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

const (
	ScopeSales          = "sales"
	ScopePurchasing     = "purchasing"
	ScopePaymentMethods = "payment-methods"

	// ConfirmWord harus diketik persis oleh Owner.
	ConfirmWord = "HAPUS"
)

var (
	ErrOwnerOnly     = errors.New("hanya Owner")
	ErrImpersonating = errors.New("tidak tersedia pada sesi masuk-sebagai")
	ErrConfirm       = errors.New("kata konfirmasi salah")
	ErrPassword      = errors.New("kata sandi salah")
	ErrScope         = errors.New("cakupan tidak dikenal")
)

// fn = fungsi SQL per cakupan (nama tetap, bukan input pengguna).
var fn = map[string]string{
	ScopeSales:          "danger_clear_sales",
	ScopePurchasing:     "danger_clear_purchasing",
	ScopePaymentMethods: "danger_reset_payment_methods",
}

// Restorer memulihkan metode yang dibuat integrasi (EraPay) setelah reset metode pembayaran.
type Restorer interface {
	RestoreMethod(ctx context.Context, a authz.Actor) error
}

type Service struct {
	pool     *pgxpool.Pool
	restorer Restorer
}

func NewService(pool *pgxpool.Pool, r Restorer) *Service { return &Service{pool: pool, restorer: r} }

// Clear menjalankan satu cakupan. Mengembalikan jumlah baris terhapus per tabel. Pembatas percobaan ada di handler (per pengguna).
func (s *Service) Clear(ctx context.Context, a authz.Actor, scope, password, confirm string) (map[string]any, error) {
	name, ok := fn[scope]
	if !ok {
		return nil, ErrScope
	}
	if a.Impersonator != uuid.Nil {
		return nil, ErrImpersonating
	}
	if !a.Perms.All {
		return nil, ErrOwnerOnly
	}
	if confirm != ConfirmWord {
		return nil, ErrConfirm
	}
	var counts map[string]any
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var hash string
		if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE tenant_id = $1 AND id = $2`, a.TenantID, a.UserID).Scan(&hash); err != nil {
			return err
		}
		if ok, err := auth.VerifyPassword(password, hash); err != nil || !ok {
			return ErrPassword
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT `+name+`($1)::text`, a.TenantID).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &counts); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionDangerClear, Entity: audit.EntityDangerZone, EntityID: scope, Details: map[string]any{"scope": scope, "deleted": counts},
		})
	})
	if err != nil {
		return nil, err
	}
	if scope == ScopePaymentMethods && s.restorer != nil {
		if err := s.restorer.RestoreMethod(ctx, a); err != nil { // data sudah bersih; metode EraPay bisa dibuat ulang dari pengaturan
			counts["erapay_restore_error"] = err.Error()
		}
	}
	return counts, nil
}
