// Package onboarding: wizard penyiapan awal setelah daftar (konfirmasi email, template akuntansi, kesiapan toko).
// Status diturunkan dari data nyata; hanya penanda "selesai/dilewati" yang disimpan (tenants.onboarding_completed_at).
package onboarding

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Status = kondisi penyiapan tenant saat ini.
type Status struct {
	Completed     bool `json:"completed"`
	EmailVerified bool `json:"email_verified"`
	// AccountCount > 0 berarti bagan akun (template akuntansi) sudah terpasang.
	AccountCount int `json:"account_count"`
	// MethodsUnlinked = metode bayar aktif yang belum terhubung ke akun kas/bank (bisa menggagalkan jurnal otomatis).
	MethodsUnlinked int `json:"methods_unlinked"`
	ItemCount       int `json:"item_count"`
	OutletCount     int `json:"outlet_count"`
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Get membaca status. Semua hitungan berjalan di bawah RLS tenant pelaku.
func (s *Service) Get(ctx context.Context, a authz.Actor) (Status, error) {
	var st Status
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT
				(SELECT onboarding_completed_at IS NOT NULL FROM tenants WHERE id = $1),
				(SELECT email_verified_at IS NOT NULL FROM users WHERE id = $2),
				(SELECT count(*) FROM accounts WHERE tenant_id = $1),
				(SELECT count(*) FROM payment_methods WHERE tenant_id = $1 AND active AND account_id IS NULL),
				(SELECT count(*) FROM items WHERE tenant_id = $1),
				(SELECT count(*) FROM outlets WHERE tenant_id = $1)`,
			a.TenantID, a.UserID).Scan(&st.Completed, &st.EmailVerified, &st.AccountCount, &st.MethodsUnlinked, &st.ItemCount, &st.OutletCount)
	})
	return st, err
}

// Complete menandai wizard selesai (idempotent: tanggal pertama dipertahankan).
func (s *Service) Complete(ctx context.Context, a authz.Actor, skipped bool) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tenants SET onboarding_completed_at = now() WHERE id = $1 AND onboarding_completed_at IS NULL`, a.TenantID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionOnboardingDone, Entity: audit.EntityTenant,
			EntityID: a.TenantID.String(), Details: map[string]any{"skipped": skipped}})
	})
}
