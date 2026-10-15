package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// API selalu mencap waktu "sekarang". Agar data tampak seperti sebulan berjalan, tiap batch (outlet × hari) dijalankan
// lewat API sungguhan, lalu SEMUA kolom waktu tenant yang jatuh di jendela batch itu dipetakan linear ke jam buka-tutup
// hari tersebut (urutan antar-baris tetap). Kolom tanggal murni (tanggal faktur/jatuh tempo) digeser selisih harinya.
// Dijalankan lewat koneksi pemilik skema (melewati RLS), terbatas pada satu tenant.

// kolom tanggal yang ikut digeser (baris dipilih lewat created_at di jendela batch).
var dateCols = map[string][]string{
	"purchases":        {"purchase_date", "due_date"},
	"purchase_returns": {"return_date"},
	"sales_returns":    {"return_date"},
	"receivables":      {"doc_date", "due_date"},
	"payables":         {"due_date"},
}

// tabel yang tidak boleh disentuh (bukan data transaksi / cap waktu perlu tetap asli).
var skipTables = map[string]bool{"users": true, "mcp_grants": true, "tenants": true}

type tsTable struct {
	name string
	cols []string
}

type backdater struct {
	admin    *pgxpool.Pool
	tenant   uuid.UUID
	tables   []tsTable
	hasCreat map[string]bool
}

func newBackdater(ctx context.Context, admin *pgxpool.Pool, tenant uuid.UUID) (*backdater, error) {
	rows, err := admin.Query(ctx, `
		SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
		WHERE c.table_schema = 'public' AND c.data_type = 'timestamp with time zone'
		  AND c.table_name IN (SELECT table_name FROM information_schema.columns WHERE table_schema = 'public' AND column_name = 'tenant_id')
		ORDER BY c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	b := &backdater{admin: admin, tenant: tenant, hasCreat: map[string]bool{}}
	idx := map[string]int{}
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		if skipTables[t] {
			continue
		}
		i, ok := idx[t]
		if !ok {
			i = len(b.tables)
			idx[t] = i
			b.tables = append(b.tables, tsTable{name: t})
		}
		b.tables[i].cols = append(b.tables[i].cols, c)
		if c == "created_at" {
			b.hasCreat[t] = true
		}
	}
	return b, rows.Err()
}

func (b *backdater) dbNow(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := b.admin.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t)
	return t, err
}

// shift memetakan jendela nyata [t0,t1] ke [start, start+span] dan menggeser tanggal murni sebanyak offsetDays.
func (b *backdater) shift(ctx context.Context, t0, t1, start time.Time, span time.Duration, offsetDays int) (int64, error) {
	real := t1.Sub(t0)
	if real <= 0 {
		real = time.Millisecond
	}
	k := float64(span) / float64(real)
	var total int64
	err := pgx.BeginFunc(ctx, b.admin, func(tx pgx.Tx) error {
		for _, tb := range b.tables {
			var sets, conds []string
			for _, c := range tb.cols {
				sets = append(sets, fmt.Sprintf(`%[1]s = CASE WHEN %[1]s >= $2::timestamptz AND %[1]s <= $3::timestamptz THEN $4::timestamptz + (%[1]s - $2::timestamptz) * $5::float8 ELSE %[1]s END`, quote(c)))
				conds = append(conds, fmt.Sprintf(`(%[1]s >= $2::timestamptz AND %[1]s <= $3::timestamptz)`, quote(c)))
			}
			args := []any{b.tenant, t0, t1, start, k}
			if dc := dateCols[tb.name]; len(dc) > 0 && b.hasCreat[tb.name] {
				for _, c := range dc {
					sets = append(sets, fmt.Sprintf(`%[1]s = CASE WHEN created_at >= $2::timestamptz AND created_at <= $3::timestamptz THEN %[1]s + $6::int ELSE %[1]s END`, quote(c)))
				}
				args = append(args, int32(offsetDays)) // $6 hanya dikirim bila dipakai (Postgres menolak parameter berlebih)
			}
			q := fmt.Sprintf(`UPDATE %s SET %s WHERE tenant_id = $1 AND (%s)`, quote(tb.name), strings.Join(sets, ", "), strings.Join(conds, " OR "))
			tag, err := tx.Exec(ctx, q, args...)
			if err != nil {
				return fmt.Errorf("geser waktu %s: %w", tb.name, err)
			}
			total += tag.RowsAffected()
		}
		return nil
	})
	return total, err
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// ---- nomor dokumen ----
//
// Nomor memuat tanggal HARI INI (PREFIX-OUTLET-YYMMDD-NNNN). Setelah jam dimundurkan, nomor ditulis ulang per
// (outlet, hari lokal) berurutan menurut waktu, lalu semua rujukan teks (stok, piutang, deposit, ...) ikut diganti.

var docTables = []string{"sales", "purchases", "sales_returns", "purchase_returns", "cash_shifts", "receivable_payments", "payable_payments"}

// rujukan teks ke nomor dokumen: tabel.kolom (cocok persis).
var docRefs = [][2]string{
	{"receivables", "doc_no"}, {"stock_movements", "note"}, {"member_deposit_movements", "doc_no"},
	{"supplier_credit_movements", "doc_no"}, {"receivable_settlements", "doc_no"}, {"payable_settlements", "doc_no"},
}

func (b *backdater) renumber(ctx context.Context) error {
	return pgx.BeginFunc(ctx, b.admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE docmap (tbl text, id uuid, old text, new text) ON COMMIT DROP`); err != nil {
			return err
		}
		for _, t := range docTables {
			// primary key tabel = id; outlet lewat outlet_id (semua tabel di atas memilikinya).
			q := fmt.Sprintf(`
				INSERT INTO docmap (tbl, id, old, new)
				SELECT '%[1]s', x.id, x.doc_no,
				       regexp_replace(x.doc_no, '-[0-9]{6}-[0-9]{4}$', '-' || to_char(x.d, 'YYMMDD') || '-' || lpad(x.rn::text, 4, '0'))
				FROM (
					SELECT t.id, t.doc_no, (t.created_at AT TIME ZONE o.timezone)::date AS d,
					       row_number() OVER (PARTITION BY t.outlet_id, (t.created_at AT TIME ZONE o.timezone)::date ORDER BY t.created_at, t.id) AS rn
					FROM %[1]s t JOIN outlets o ON o.tenant_id = t.tenant_id AND o.id = t.outlet_id
					WHERE t.tenant_id = $1 AND t.doc_no ~ '-[0-9]{6}-[0-9]{4}$'
				) x`, t)
			if _, err := tx.Exec(ctx, q, b.tenant); err != nil {
				return fmt.Errorf("peta nomor %s: %w", t, err)
			}
		}
		for _, t := range docTables {
			// fase 1: nama sementara (hindari tabrakan UNIQUE saat nomor baru = nomor lama baris lain)
			if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s t SET doc_no = 'tmp~' || t.id::text FROM docmap m WHERE m.tbl = '%s' AND m.id = t.id AND t.tenant_id = $1`, t, t), b.tenant); err != nil {
				return fmt.Errorf("nomor sementara %s: %w", t, err)
			}
			if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s t SET doc_no = m.new FROM docmap m WHERE m.tbl = '%s' AND m.id = t.id AND t.tenant_id = $1`, t, t), b.tenant); err != nil {
				return fmt.Errorf("nomor baru %s: %w", t, err)
			}
		}
		for _, r := range docRefs {
			q := fmt.Sprintf(`UPDATE %[1]s t SET %[2]s = m.new FROM (SELECT DISTINCT old, new FROM docmap) m WHERE t.tenant_id = $1 AND t.%[2]s = m.old`, r[0], r[1])
			if _, err := tx.Exec(ctx, q, b.tenant); err != nil {
				return fmt.Errorf("rujukan %s.%s: %w", r[0], r[1], err)
			}
		}
		return nil
	})
}
