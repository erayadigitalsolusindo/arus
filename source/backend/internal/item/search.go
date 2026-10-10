package item

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Pencarian barang untuk katalog besar (dipakai kasir). Beda dengan List:
//   - tanpa total (count(*) OVER () memaksa seluruh barang yang cocok dihitung dan di-join),
//   - paginasi keyset (nama, id), bukan OFFSET,
//   - setiap kata harus muncul di nama/kode/barcode (urutan kata bebas: "lip wardah" = "wardah lip"),
//   - kandidat dicari lewat fungsi SECURITY DEFINER item_search (00051) agar indeks trigram terpakai di bawah RLS.
const (
	searchDefLimit  = 30
	searchMaxLimit  = 100
	searchMaxTokens = 8
)

type SearchParams struct {
	Q      string
	Cursor string
	Limit  int
	// CategoryID (uuid, opsional): hanya barang kategori ini (filter kasir mobile).
	CategoryID string
}

// SearchPage: Exact (hanya halaman pertama) = barang yang kode/barcode-nya persis sama dengan q; barang yang sama
// bisa juga muncul di Data — klien membuang duplikatnya. NextCursor kosong = tidak ada halaman berikutnya.
type SearchPage struct {
	Data       []Row  `json:"data"`
	Exact      []Row  `json:"exact"`
	NextCursor string `json:"next_cursor"`
}

// searchPatterns memecah q per spasi menjadi pola ILIKE '%kata%' (escape % _ \), tanpa kata ganda.
func searchPatterns(q string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, w := range strings.Fields(strings.ToLower(q)) {
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, "%"+likeEscape(w)+"%")
	}
	return out
}

func encodeCursor(name string, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id.String() + name))
}

func decodeCursor(c string) (string, uuid.UUID, bool) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil || len(b) < 36 || !utf8.Valid(b) {
		return "", uuid.Nil, false
	}
	id, err := uuid.Parse(string(b[:36]))
	if err != nil {
		return "", uuid.Nil, false
	}
	return string(b[36:]), id, true
}

// Search: barang AKTIF outlet aktif sesi. Harga/stok/HPP sama dengan List (harga cabang bila ada).
func (s *Service) Search(ctx context.Context, a authz.Actor, p SearchParams) (SearchPage, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || utf8.RuneCountInString(q) > maxName {
		return SearchPage{}, FieldErrors{"q": sanitize.Invalid}
	}
	pats := searchPatterns(q)
	if len(pats) > searchMaxTokens {
		return SearchPage{}, FieldErrors{"q": sanitize.TooLong}
	}
	var afterName pgtype.Text
	var afterID pgtype.UUID
	if p.Cursor != "" {
		name, id, ok := decodeCursor(p.Cursor)
		if !ok {
			return SearchPage{}, FieldErrors{"cursor": sanitize.Invalid}
		}
		afterName, afterID = pgtype.Text{String: name, Valid: true}, pgtype.UUID{Bytes: id, Valid: true}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = searchDefLimit
	}
	limit = min(limit, searchMaxLimit)

	var category pgtype.UUID
	if p.CategoryID != "" {
		cid, err := uuid.Parse(p.CategoryID)
		if err != nil {
			return SearchPage{}, FieldErrors{"category_id": sanitize.Invalid}
		}
		category = pgtype.UUID{Bytes: cid, Valid: true}
	}

	page := SearchPage{Data: []Row{}, Exact: []Row{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		ids, keys, err := searchIDs(ctx, tx, pats, afterName, afterID, category, limit+1)
		if err != nil {
			return err
		}
		if len(ids) > limit {
			ids, keys = ids[:limit], keys[:limit]
			page.NextCursor = encodeCursor(keys[limit-1], ids[limit-1])
		}
		if page.Data, err = rowsByIDs(ctx, tx, a, ids); err != nil {
			return err
		}
		// Kode/barcode persis hanya relevan untuk satu kata di halaman pertama (kasir menekan Enter).
		if p.Cursor != "" || len(strings.Fields(q)) != 1 {
			return nil
		}
		exact, err := exactIDs(ctx, tx, q)
		if err != nil {
			return err
		}
		page.Exact, err = rowsByIDs(ctx, tx, a, exact)
		return err
	})
	return page, err
}

func searchIDs(ctx context.Context, tx pgx.Tx, pats []string, afterName pgtype.Text, afterID pgtype.UUID, category pgtype.UUID, limit int) ([]uuid.UUID, []string, error) {
	rows, err := tx.Query(ctx, `SELECT id, sort_name FROM item_search($1, true, $2, $3, $4, $5)`, pats, afterName, afterID, limit, category)
	if err != nil {
		return nil, nil, err
	}
	var ids []uuid.UUID
	var keys []string
	for rows.Next() {
		var id uuid.UUID
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, nil, err
		}
		ids, keys = append(ids, id), append(keys, key)
	}
	return ids, keys, rows.Err()
}

func exactIDs(ctx context.Context, tx pgx.Tx, code string) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM item_find_exact($1, true)`, code)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// rowsByIDs membaca baris lengkap (di bawah RLS) dengan urutan sama seperti ids.
func rowsByIDs(ctx context.Context, tx pgx.Tx, a authz.Actor, ids []uuid.UUID) ([]Row, error) {
	out := []Row{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := gen.New(tx).ItemRowsByIDs(ctx, gen.ItemRowsByIDsParams{TenantID: a.TenantID, OutletID: a.OutletID, Ids: ids})
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]Row, len(rows))
	for _, r := range rows {
		price, override := r.DefaultPrice, false
		if op, ok := numeric(r.OutletPrice); ok {
			price, override = op, true
		}
		byID[r.ID] = Row{ID: r.ID, SKU: r.Sku, Barcode: r.Barcode.String, Origin: r.Origin, Name: r.Name, Kind: r.Kind, Active: r.Active,
			Unit: r.UnitName, Category: r.CategoryName.String, Brand: r.BrandName.String,
			Price: price.StringFixed(2), PriceOverride: override, MainImageID: uuidPtr(r.MainImageID),
			AvgCost: r.AvgCost.StringFixed(2), LastCost: r.LastCost.StringFixed(2),
			Stock: StockQty{Display: r.StockDisplay.String(), Warehouse: r.StockWarehouse.String(), Returns: r.StockReturns.String(),
				Total: r.StockDisplay.Add(r.StockWarehouse).Add(r.StockReturns).String()}}
	}
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
