package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetTenant menetapkan tenant aktif untuk transaksi berjalan (RLS: policy memakai app_tenant_id()).
// is_local = true: nilai hilang saat commit/rollback sehingga tidak bocor ke pemakai koneksi berikutnya di pool.
func SetTenant(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
		return fmt.Errorf("set tenant: %w", err)
	}
	return nil
}

// WithTenant menjalankan fn dalam satu transaksi yang dibatasi ke satu tenant (AGENTS.md §3.1, §3.2).
// tenantID berasal dari token (klaim `tid`), bukan dari body request. Rollback bila fn mengembalikan error.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := SetTenant(ctx, tx, tenantID); err != nil {
			return err
		}
		return fn(tx)
	})
}
