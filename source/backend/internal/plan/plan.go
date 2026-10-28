// Package plan: paket langganan tenant dan penegakan batasnya (transaksi per hari, jumlah pengguna). Paket dinamis
// (tabel plans, diatur Platform Admin). Tenant tanpa langganan aktif memakai paket default (FREE). Batas NULL = tanpa batas.
// Paket ini sengaja tidak bergantung pada modul lain agar sales dan iam boleh mengimpornya.
package plan

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrTxLimit   = errors.New("batas transaksi harian paket tercapai")
	ErrUserLimit = errors.New("batas jumlah pengguna paket tercapai")
)

// Effective = paket yang berlaku sekarang untuk satu tenant, sudah memperhitungkan override dan kedaluwarsa.
type Effective struct {
	PlanID      uuid.UUID  `json:"plan_id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	MaxTxPerDay *int       `json:"max_tx_per_day"` // nil = tanpa batas
	MaxUsers    *int       `json:"max_users"`      // nil = tanpa batas
	ExpiresAt   *time.Time `json:"expires_at"`     // nil = tanpa kedaluwarsa
	IsDefault   bool       `json:"is_default"`
	// Modules = id modul izin yang boleh dipakai paket ini; nil = semua modul.
	Modules []string `json:"modules"`
}

// Langganan yang kedaluwarsa dilewati sehingga tenant jatuh ke paket default tanpa job apa pun.
const effectiveSQL = `
WITH s AS (
    SELECT plan_id, expires_at, override_max_tx_per_day o_tx, override_max_users o_users
      FROM tenant_subscriptions
     WHERE tenant_id = $1 AND (expires_at IS NULL OR expires_at > now())
)
SELECT p.id, p.code, p.name,
       coalesce((SELECT o_tx FROM s), p.max_tx_per_day),
       coalesce((SELECT o_users FROM s), p.max_users),
       (SELECT expires_at FROM s), p.is_default, p.modules
  FROM plans p
 WHERE p.id = coalesce((SELECT plan_id FROM s), (SELECT id FROM plans WHERE is_default))`

// Get membaca paket efektif tenant di dalam transaksi pemanggil.
func Get(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (Effective, error) {
	var (
		e         Effective
		maxTx, maxUsr pgtype.Int4
		expiresAt pgtype.Timestamptz
	)
	if err := tx.QueryRow(ctx, effectiveSQL, tenant).Scan(&e.PlanID, &e.Code, &e.Name, &maxTx, &maxUsr, &expiresAt, &e.IsDefault, &e.Modules); err != nil {
		return Effective{}, err
	}
	e.MaxTxPerDay, e.MaxUsers = intPtr(maxTx), intPtr(maxUsr)
	if expiresAt.Valid {
		t := expiresAt.Time
		e.ExpiresAt = &t
	}
	return e, nil
}

func intPtr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

// ConsumeTx menambah hitungan transaksi hari itu dan menolak (ErrTxLimit) bila batas paket sudah tercapai. Dipanggil di DALAM
// transaksi pembuatan nota: nota gagal/dibatalkan ikut membatalkan hitungan. `day` = tanggal lokal outlet. Penambahan dan
// pemeriksaan batas satu pernyataan atomik (kunci baris counter menyerialkan kasir yang bersamaan).
func ConsumeTx(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, day pgtype.Date) error {
	e, err := Get(ctx, tx, tenant)
	if err != nil {
		return err
	}
	var limit any // NULL = tanpa batas
	if e.MaxTxPerDay != nil {
		limit = int32(*e.MaxTxPerDay)
	}
	var n int
	err = tx.QueryRow(ctx, `
		INSERT INTO tenant_daily_tx (tenant_id, day, tx_count) VALUES ($1, $2, 1)
		ON CONFLICT (tenant_id, day) DO UPDATE SET tx_count = tenant_daily_tx.tx_count + 1
		 WHERE $3::int IS NULL OR tenant_daily_tx.tx_count < $3::int
		RETURNING tx_count`, tenant, day, limit).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTxLimit
	}
	return err
}

// CheckUserSlot menolak (ErrUserLimit) bila menambah satu pengguna aktif lagi akan melewati batas paket. Kunci advisory per
// tenant menyerialkan dua penambahan bersamaan. Panggil di dalam transaksi yang menambah / mengaktifkan pengguna.
func CheckUserSlot(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('plan_users:' || $1::text, 0))`, tenant); err != nil {
		return err
	}
	e, err := Get(ctx, tx, tenant)
	if err != nil {
		return err
	}
	if e.MaxUsers == nil {
		return nil
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE tenant_id = $1 AND active`, tenant).Scan(&active); err != nil {
		return err
	}
	if active >= *e.MaxUsers {
		return ErrUserLimit
	}
	return nil
}

// Usage = pemakaian saat ini untuk halaman langganan.
type Usage struct {
	TxToday int `json:"tx_today"`
	Users   int `json:"users"`
}

// UsageFor membaca pemakaian hari ini (tanggal lokal outlet `day`) dan jumlah pengguna aktif.
func UsageFor(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, day pgtype.Date) (Usage, error) {
	var u Usage
	if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT tx_count FROM tenant_daily_tx WHERE tenant_id = $1 AND day = $2), 0),
		(SELECT count(*) FROM users WHERE tenant_id = $1 AND active)`, tenant, day).Scan(&u.TxToday, &u.Users); err != nil {
		return Usage{}, err
	}
	return u, nil
}
