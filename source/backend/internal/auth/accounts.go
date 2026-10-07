package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// account = akun lengkap (user + role + tenant + outlet aktif). OutletID == uuid.Nil berarti pengguna tidak punya
// outlet aktif yang boleh diakses.
type account struct {
	UserID                    uuid.UUID
	UserName, Email           string
	PasswordHash              string
	UserActive, EmailVerified bool
	RoleName                  string
	TenantID                  uuid.UUID
	TenantCode, TenantName    string
	TenantActive              bool
	OutletID                  uuid.UUID
	OutletCode, OutletName    string
}

const accountColumns = `user_id, user_name, email, password_hash, user_active, email_verified, role_name,
	tenant_id, tenant_code, tenant_name, tenant_active, outlet_id, outlet_code, outlet_name`

// lookupAccount memanggil fungsi SECURITY DEFINER (db/migrations/00005) yang melewati RLS. Dipakai hanya saat tenant
// belum diketahui: login (email), refresh/lupa password/verifikasi (user id). Selebihnya wajib lewat db.WithTenant.
// Ditulis manual karena sqlc tidak membaca tipe hasil fungsi RETURNS TABLE.
func lookupAccount(ctx context.Context, pool *pgxpool.Pool, call string, args ...any) (account, error) {
	var (
		a      account
		oid    pgtype.UUID
		oc, on pgtype.Text
	)
	err := pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM "+call, args...).Scan(
		&a.UserID, &a.UserName, &a.Email, &a.PasswordHash, &a.UserActive, &a.EmailVerified, &a.RoleName,
		&a.TenantID, &a.TenantCode, &a.TenantName, &a.TenantActive, &oid, &oc, &on)
	if err != nil {
		return account{}, err
	}
	if oid.Valid {
		a.OutletID, a.OutletCode, a.OutletName = oid.Bytes, oc.String, on.String
	}
	return a, nil
}

func accountByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (account, error) {
	return lookupAccount(ctx, pool, "auth_account_by_email($1)", email)
}

// accountByUserID: preferredOutlet (boleh uuid.Nil) dipakai bila pengguna boleh mengaksesnya; selain itu outlet pertama.
func accountByUserID(ctx context.Context, pool *pgxpool.Pool, id, preferredOutlet uuid.UUID) (account, error) {
	var pref any
	if preferredOutlet != uuid.Nil {
		pref = preferredOutlet
	}
	return lookupAccount(ctx, pool, "auth_account_by_id($1, $2)", id, pref)
}

func isNoAccount(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
