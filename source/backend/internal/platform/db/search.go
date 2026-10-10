package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SearchCap = jumlah kandidat maksimum dari fungsi *_search_ids (00052) yang dibawa ke query daftar sebagai "id = ANY(...)".
// Lebih dari itu (kata sangat umum), pemanggil memakai query ILIKE lama: menyusuri indeks urutan lebih cepat daripada
// membawa puluhan ribu id, dan ringkasan memang harus menjumlah banyak baris.
var SearchCap = 5000 // var agar test bisa memaksa jalur ILIKE (SearchCap = -1)

// SearchCandidates menjalankan query yang mengembalikan satu kolom uuid (fungsi *_search_ids dengan limit SearchCap+1)
// di dalam transaksi ber-tenant. ok=false berarti kandidat melebihi SearchCap dan ids tidak dipakai.
//
// Sisa transaksi dipaksa memakai custom plan: query daftar yang sama dipakai untuk mode biasa dan mode kandidat
// ("NOT @by_ids OR id = ANY(@ids)"); generic plan dari statement cache pgx tidak bisa memakai indeks id untuk itu
// dan akan menyusuri seluruh rentang tanggal.
func SearchCandidates(ctx context.Context, tx pgx.Tx, sql string, args ...any) (ids []uuid.UUID, ok bool, err error) {
	if _, err := tx.Exec(ctx, "SET LOCAL plan_cache_mode = force_custom_plan"); err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	ids, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, false, err
	}
	if len(ids) > SearchCap {
		return nil, false, nil
	}
	return ids, true, nil
}
