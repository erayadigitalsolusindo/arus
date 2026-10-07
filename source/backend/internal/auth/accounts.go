package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	gen "aciraba/internal/gen"
)

// account = baris akun lengkap (user + role + tenant + outlet aktif pertama).
type account = gen.GetAccountInTenantRow

const accountColumns = `user_id, user_name, email, password_hash, user_active, role_name,
	tenant_id, tenant_code, tenant_name, tenant_active, outlet_id, outlet_code, outlet_name`

// lookupAccount memanggil fungsi SECURITY DEFINER (db/migrations/00004_rls.sql) yang melewati RLS. Dipakai hanya
// saat tenant belum diketahui: login (email) dan refresh (user id dari Redis). Selebihnya wajib lewat
// db.WithTenant + query biasa. Dibuat manual karena sqlc tidak membaca tipe hasil fungsi RETURNS TABLE.
func lookupAccount(ctx context.Context, pool *pgxpool.Pool, fn string, arg any) (account, error) {
	var a account
	err := pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM "+fn+"($1)", arg).Scan(
		&a.UserID, &a.UserName, &a.Email, &a.PasswordHash, &a.UserActive, &a.RoleName,
		&a.TenantID, &a.TenantCode, &a.TenantName, &a.TenantActive, &a.OutletID, &a.OutletCode, &a.OutletName)
	return a, err
}

func accountByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (account, error) {
	return lookupAccount(ctx, pool, "auth_account_by_email", email)
}

func accountByUserID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (account, error) {
	return lookupAccount(ctx, pool, "auth_account_by_id", id)
}

var errNoAccount = pgx.ErrNoRows

func isNoAccount(err error) bool { return errors.Is(err, errNoAccount) }
