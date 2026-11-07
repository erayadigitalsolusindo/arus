package storefront

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/platform/db"
)

// Sumber data tambahan untuk penanda level/kupon/outlet/pembayaran/jumlah. Semuanya dibatasi LIMIT kecil, memakai
// indeks yang ada, dan hanya dipanggil bila templat memakai penandanya (lihat memo di template.go).

func (d *dbSource) Levels(ctx context.Context, limit int) ([]Level, error) {
	var out []Level
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT name, min_points, spend_per_point, point_value FROM member_levels
			WHERE tenant_id = $1 AND active ORDER BY min_points LIMIT $2`, d.tenant, int32(limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l Level
			if err := rows.Scan(&l.Name, &l.MinPoints, &l.SpendPerPoint, &l.PointValue); err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, err
}

// Coupons: hanya kupon aktif + bersaklar publik + dalam masa berlaku + kuota tersisa (indeks parsial vouchers_public_idx).
func (d *dbSource) Coupons(ctx context.Context, today time.Time, limit int) ([]Coupon, error) {
	var out []Coupon
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT code, name, kind, value, coalesce(max_discount, 0), min_spend, ends_on,
			       CASE WHEN max_uses IS NULL THEN NULL ELSE max_uses - used_count END
			  FROM vouchers
			 WHERE tenant_id = $1 AND show_public AND active
			   AND (starts_on IS NULL OR starts_on <= $2::date) AND (ends_on IS NULL OR ends_on >= $2::date)
			   AND (max_uses IS NULL OR used_count < max_uses)
			 ORDER BY ends_on NULLS LAST, id LIMIT $3`, d.tenant, today, int32(limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				c    Coupon
				ends pgtype.Date
				rem  *int32
			)
			if err := rows.Scan(&c.Code, &c.Name, &c.Kind, &c.Value, &c.MaxDiscount, &c.MinSpend, &ends, &rem); err != nil {
				return err
			}
			if ends.Valid {
				t := ends.Time
				c.EndsOn = &t
			}
			if rem != nil {
				n := int(*rem)
				c.Remaining = &n
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (d *dbSource) Outlets(ctx context.Context, limit int) ([]Outlet, error) {
	var out []Outlet
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT name FROM outlets WHERE tenant_id = $1 AND active ORDER BY lower(name), id LIMIT $2`, d.tenant, int32(limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o Outlet
			if err := rows.Scan(&o.Name); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

func (d *dbSource) PayMethods(ctx context.Context, limit int) ([]PayMethod, error) {
	var out []PayMethod
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT name, kind FROM payment_methods WHERE tenant_id = $1 AND active ORDER BY lower(name), id LIMIT $2`, d.tenant, int32(limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PayMethod
			if err := rows.Scan(&p.Name, &p.Kind); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// MemberTotal membaca penghitung nomor member (satu baris per tenant, kunci utama), bukan count(*) atas tabel members.
func (d *dbSource) MemberTotal(ctx context.Context) (int64, error) {
	var n int64
	err := db.WithTenant(ctx, d.pool, d.tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT last_no FROM member_counters WHERE tenant_id = $1`, d.tenant).Scan(&n)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return n, err
}
