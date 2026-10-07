// Command seeddemo mengisi satu tenant dengan data demo (satuan, kategori, brand, dan 3 barang berharga grosir) lewat
// service yang sama dengan API, jadi semua aturan (validasi, RLS, audit) ikut berlaku. Aman dijalankan berulang:
// master/barang yang sudah ada dilewati.
//
//	cd source/backend && go run ./cmd/seeddemo
//
// Tenant: env SEED_TENANT_CODE; kosong = dipakai satu-satunya tenant bila hanya ada satu.
// Butuh DATABASE_URL (role aplikasi, RLS berlaku) dan MIGRATE_DATABASE_URL (pemilik skema, hanya untuk mencari id tenant).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/authz"
	"aciraba/internal/catalog"
	"aciraba/internal/item"
	"aciraba/internal/platform/config"
	"aciraba/internal/platform/db"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func kind(path string) catalog.Kind {
	for _, k := range catalog.Kinds {
		if k.Path == path {
			return k
		}
	}
	panic("kind tidak dikenal: " + path)
}

func run() error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	actor, err := resolveActor(ctx)
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	cat, items := catalog.NewService(pool), item.NewService(pool, nil)

	// ensure membuat master bila belum ada; mengembalikan id-nya.
	ensure := func(path, name string) (string, error) {
		k := kind(path)
		list, _, err := cat.List(ctx, actor, k, catalog.ListParams{Q: name})
		if err != nil {
			return "", err
		}
		for _, e := range list {
			if strings.EqualFold(e.Name, name) {
				return e.ID.String(), nil
			}
		}
		e, err := cat.Create(ctx, actor, k, name)
		if err != nil {
			return "", fmt.Errorf("buat %s %q: %w", path, name, err)
		}
		fmt.Printf("  + %s: %s\n", path, name)
		return e.ID.String(), nil
	}

	ids := map[string]string{}
	for _, m := range [][2]string{
		{"units", "Pcs"}, {"units", "Dus"}, {"units", "Karung"},
		{"categories", "Minuman"}, {"categories", "Makanan"}, {"categories", "Sembako"},
		{"brands", "Kapal Api"}, {"brands", "Indomie"}, {"brands", "Topi Koki"},
	} {
		id, err := ensure(m[0], m[1])
		if err != nil {
			return err
		}
		ids[m[0]+"/"+m[1]] = id
	}
	id := func(path, name string) string { return ids[path+"/"+name] }

	t := func(pairs ...string) []item.TierInput {
		out := []item.TierInput{}
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, item.TierInput{MinQty: pairs[i], Price: pairs[i+1]})
		}
		return out
	}

	demo := []item.Input{
		{ // grosir bertingkat 3, plus Dus isi 12 dengan harga sendiri
			Name: "Kopi Bubuk Kapal Api 200 gr", Barcode: "8991001000011", Weight: "200", Cost: "8000", SellPrice: "12000",
			UnitID: id("units", "Pcs"), CategoryID: id("categories", "Minuman"), BrandID: id("brands", "Kapal Api"),
			Description: "Kopi bubuk hitam kemasan **200 gr**.\n\n- Aroma kuat\n- Cocok untuk kopi tubruk",
			Wholesale:   &item.WholesaleInput{Default: t("3", "11500", "6", "11000", "12", "10500")},
			Units:       &[]item.UnitInput{{UnitID: id("units", "Dus"), Factor: "12", Barcode: "8991001000028", SellPrice: "125000"}},
		},
		{ // grosir curam untuk volume besar; Dus isi 40 dengan harga otomatis (40 × harga satuan)
			Name: "Mie Goreng Instan Indomie", Barcode: "8998866200011", Weight: "85", Cost: "2500", SellPrice: "3500",
			UnitID: id("units", "Pcs"), CategoryID: id("categories", "Makanan"), BrandID: id("brands", "Indomie"),
			Description: "Mie instan goreng original, **per bungkus 85 gr**.",
			Wholesale:   &item.WholesaleInput{Default: t("5", "3300", "20", "3100", "40", "3000")},
			Units:       &[]item.UnitInput{{UnitID: id("units", "Dus"), Factor: "40", Barcode: "8998866200028"}},
		},
		{ // barang berat: grosir mulai 2 karung
			Name: "Beras Premium 5 kg", Barcode: "8997777000015", Weight: "5000", Cost: "62000", SellPrice: "72000",
			UnitID: id("units", "Karung"), CategoryID: id("categories", "Sembako"), BrandID: id("brands", "Topi Koki"),
			Description: "Beras putih premium pulen, kemasan **5 kg**.",
			Wholesale:   &item.WholesaleInput{Default: t("2", "71000", "5", "70000", "10", "68500")},
		},
	}
	for _, in := range demo {
		existing, _, err := items.List(ctx, actor, item.ListParams{Q: in.Name})
		if err != nil {
			return err
		}
		if hasName(existing, in.Name) {
			fmt.Printf("  = sudah ada: %s\n", in.Name)
			continue
		}
		it, err := items.Create(ctx, actor, in)
		if err != nil {
			var fe item.FieldErrors
			if errors.As(err, &fe) {
				return fmt.Errorf("buat %q: validasi %v", in.Name, map[string]string(fe))
			}
			return fmt.Errorf("buat %q: %w", in.Name, err)
		}
		fmt.Printf("  + item: %s (%s) grosir %d tier\n", it.Name, it.SKU, len(it.Wholesale.Default))
	}
	fmt.Println("selesai.")
	return nil
}

func hasName(rows []item.Row, name string) bool {
	for _, r := range rows {
		if strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

// resolveActor mencari tenant, pemilik, dan outlet-nya lewat koneksi pemilik skema (melewati RLS).
func resolveActor(ctx context.Context) (authz.Actor, error) {
	adminURL := os.Getenv("MIGRATE_DATABASE_URL")
	if adminURL == "" {
		return authz.Actor{}, errors.New("MIGRATE_DATABASE_URL wajib diisi (untuk mencari id tenant)")
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return authz.Actor{}, err
	}
	defer admin.Close()

	code := os.Getenv("SEED_TENANT_CODE")
	rows, err := admin.Query(ctx, `SELECT id, code, name FROM tenants WHERE active AND ($1 = '' OR code = $1) ORDER BY created_at`, code)
	if err != nil {
		return authz.Actor{}, err
	}
	type tenant struct {
		id         uuid.UUID
		code, name string
	}
	var ts []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.id, &t.code, &t.name); err != nil {
			return authz.Actor{}, err
		}
		ts = append(ts, t)
	}
	rows.Close()
	switch {
	case len(ts) == 0:
		return authz.Actor{}, errors.New("tenant tidak ditemukan")
	case len(ts) > 1:
		return authz.Actor{}, fmt.Errorf("ada %d tenant; set SEED_TENANT_CODE", len(ts))
	}
	t := ts[0]

	var uid uuid.UUID
	var uname string
	if err := admin.QueryRow(ctx, `SELECT u.id, u.name FROM users u JOIN roles r ON r.tenant_id = u.tenant_id AND r.id = u.role_id
		WHERE u.tenant_id = $1 AND r.is_system AND u.active ORDER BY u.created_at LIMIT 1`, t.id).Scan(&uid, &uname); err != nil {
		return authz.Actor{}, fmt.Errorf("pemilik tenant tidak ditemukan: %w", err)
	}
	orows, err := admin.Query(ctx, `SELECT id FROM outlets WHERE tenant_id = $1 AND active ORDER BY created_at`, t.id)
	if err != nil {
		return authz.Actor{}, err
	}
	defer orows.Close()
	outlets := map[uuid.UUID]bool{}
	var first uuid.UUID
	for orows.Next() {
		var oid uuid.UUID
		if err := orows.Scan(&oid); err != nil {
			return authz.Actor{}, err
		}
		if first == uuid.Nil {
			first = oid
		}
		outlets[oid] = true
	}
	if first == uuid.Nil {
		return authz.Actor{}, errors.New("tenant belum punya outlet aktif")
	}
	fmt.Printf("tenant: %s (%s), pemilik: %s\n", t.name, t.code, uname)
	return authz.Actor{TenantID: t.id, UserID: uid, OutletID: first, Name: uname, Perms: authz.Permissions{All: true}, Outlets: outlets}, nil
}
