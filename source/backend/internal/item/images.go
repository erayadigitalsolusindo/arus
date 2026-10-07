package item

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/imaging"
	"aciraba/internal/platform/storage"
)

const (
	// MaxImages = jumlah gambar per item.
	MaxImages = 5
	// MaxUploadBytes = ukuran unggahan mentah per gambar (sebelum dikompres server).
	MaxUploadBytes = 10 << 20
)

var (
	ErrImageLimit       = errors.New("jumlah gambar item sudah maksimal")
	ErrImageNotFound    = errors.New("gambar tidak ditemukan")
	ErrImageTooLarge    = errors.New("ukuran gambar terlalu besar")
	ErrImageUnsupported = errors.New("format gambar tidak didukung")
	ErrImageDimensions  = errors.New("dimensi gambar terlalu besar")
	ErrImageCorrupt     = errors.New("gambar rusak")
	ErrNoStorage        = errors.New("penyimpanan gambar tidak dikonfigurasi")
)

// Pemrosesan gambar memakai CPU/memori besar: dibatasi dua sekaligus agar beberapa unggahan bersamaan tidak
// menghabiskan memori server.
var processSlots = make(chan struct{}, 2)

type Image struct {
	ID         uuid.UUID `json:"id"`
	IsMain     bool      `json:"is_main"`
	Position   int       `json:"position"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	Bytes      int       `json:"bytes"`
	ThumbBytes int       `json:"thumb_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

func imageOf(id uuid.UUID, pos int32, main bool, w, h, fb, tb int32, created time.Time) Image {
	return Image{ID: id, IsMain: main, Position: int(pos), Width: int(w), Height: int(h), Bytes: int(fb), ThumbBytes: int(tb), CreatedAt: created}
}

func fullKey(tenant, id uuid.UUID) string  { return tenant.String() + "/" + id.String() + ".jpg" }
func thumbKey(tenant, id uuid.UUID) string { return tenant.String() + "/" + id.String() + "-t.jpg" }

func (s *Service) removeFiles(ctx context.Context, tenant, id uuid.UUID) {
	// Berkas yatim tidak membahayakan (tak ada baris yang merujuknya); kegagalan hapus sengaja tidak menggagalkan permintaan.
	_ = s.store.Delete(ctx, fullKey(tenant, id))
	_ = s.store.Delete(ctx, thumbKey(tenant, id))
}

func mapImageErr(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig), errors.Is(err, imaging.ErrTooLarge):
		return ErrImageTooLarge
	case errors.Is(err, imaging.ErrUnsupported):
		return ErrImageUnsupported
	case errors.Is(err, imaging.ErrDimensions):
		return ErrImageDimensions
	case errors.Is(err, imaging.ErrCorrupt):
		return ErrImageCorrupt
	}
	return err
}

// AddImage memproses (validasi isi, resize, JPG, thumbnail) dan menyimpan satu gambar. Gambar pertama item otomatis
// menjadi gambar utama. Berkas ditulis lebih dulu, baris DB di transaksi; bila transaksi gagal berkas dibersihkan.
func (s *Service) AddImage(ctx context.Context, a authz.Actor, itemID uuid.UUID, r io.Reader) (*Image, error) {
	if s.store == nil {
		return nil, ErrNoStorage
	}
	// Pemeriksaan murah dulu (item ada, belum penuh) supaya CPU tidak terbuang untuk unggahan yang pasti ditolak.
	if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if _, err := q.ItemGetForUpdate(ctx, gen.ItemGetForUpdateParams{TenantID: a.TenantID, ID: itemID}); err != nil {
			return err
		}
		n, err := q.ItemImageCount(ctx, gen.ItemImageCountParams{TenantID: a.TenantID, ItemID: itemID})
		if err == nil && n >= MaxImages {
			return ErrImageLimit
		}
		return err
	}); err != nil {
		return nil, mapWriteErr(err)
	}

	select {
	case processSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	res, err := imaging.Process(r, MaxUploadBytes)
	<-processSlots
	if err != nil {
		return nil, mapImageErr(err)
	}

	id := uuid.New()
	if err := s.store.Put(ctx, fullKey(a.TenantID, id), res.Full); err != nil {
		return nil, fmt.Errorf("simpan gambar: %w", err)
	}
	if err := s.store.Put(ctx, thumbKey(a.TenantID, id), res.Thumb); err != nil {
		s.removeFiles(ctx, a.TenantID, id)
		return nil, fmt.Errorf("simpan thumbnail: %w", err)
	}

	var img Image
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		// Kunci item lagi: dua unggahan bersamaan tidak boleh sama-sama lolos batas jumlah.
		it, err := q.ItemGetForUpdate(ctx, gen.ItemGetForUpdateParams{TenantID: a.TenantID, ID: itemID})
		if err != nil {
			return err
		}
		n, err := q.ItemImageCount(ctx, gen.ItemImageCountParams{TenantID: a.TenantID, ItemID: itemID})
		if err != nil {
			return err
		}
		if n >= MaxImages {
			return ErrImageLimit
		}
		row, err := q.ItemImageInsert(ctx, gen.ItemImageInsertParams{ID: id, TenantID: a.TenantID, ItemID: itemID,
			Width: int32(res.Width), Height: int32(res.Height), FullBytes: int32(len(res.Full)), ThumbBytes: int32(len(res.Thumb))})
		if err != nil {
			return err
		}
		img = imageOf(row.ID, row.Position, row.IsMain, row.Width, row.Height, row.FullBytes, row.ThumbBytes, row.CreatedAt.Time)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemImageAdd, Entity: audit.EntityItem, EntityID: itemID.String(),
			Details: map[string]any{"sku": it.Sku, "name": it.Name, "image_id": id.String(), "main": row.IsMain, "bytes": len(res.Full)},
		})
	})
	if err != nil {
		s.removeFiles(ctx, a.TenantID, id)
		return nil, mapWriteErr(err)
	}
	return &img, nil
}

// SetMainImage menjadikan satu gambar sebagai gambar utama item.
func (s *Service) SetMainImage(ctx context.Context, a authz.Actor, itemID, imageID uuid.UUID) error {
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		it, err := q.ItemGetForUpdate(ctx, gen.ItemGetForUpdateParams{TenantID: a.TenantID, ID: itemID})
		if err != nil {
			return err
		}
		cur, err := q.ItemImageGet(ctx, gen.ItemImageGetParams{TenantID: a.TenantID, ItemID: itemID, ID: imageID})
		if err != nil {
			return err
		}
		if cur.IsMain {
			return nil
		}
		if err := q.ItemImageClearMain(ctx, gen.ItemImageClearMainParams{TenantID: a.TenantID, ItemID: itemID}); err != nil {
			return err
		}
		if err := q.ItemImageSetMain(ctx, gen.ItemImageSetMainParams{TenantID: a.TenantID, ItemID: itemID, ID: imageID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemImageMain, Entity: audit.EntityItem, EntityID: itemID.String(),
			Details: map[string]any{"sku": it.Sku, "name": it.Name, "image_id": imageID.String()},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrImageNotFound
	}
	return mapWriteErr(err)
}

// DeleteImage menghapus gambar (baris + berkas). Bila yang dihapus gambar utama, gambar berikutnya dipromosikan.
func (s *Service) DeleteImage(ctx context.Context, a authz.Actor, itemID, imageID uuid.UUID) error {
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		it, err := q.ItemGetForUpdate(ctx, gen.ItemGetForUpdateParams{TenantID: a.TenantID, ID: itemID})
		if err != nil {
			return err
		}
		wasMain, err := q.ItemImageDelete(ctx, gen.ItemImageDeleteParams{TenantID: a.TenantID, ItemID: itemID, ID: imageID})
		if err != nil {
			return err
		}
		if wasMain {
			if err := q.ItemImagePromote(ctx, gen.ItemImagePromoteParams{TenantID: a.TenantID, ItemID: itemID}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionItemImageDelete, Entity: audit.EntityItem, EntityID: itemID.String(),
			Details: map[string]any{"sku": it.Sku, "name": it.Name, "image_id": imageID.String()},
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrImageNotFound
	}
	if err != nil {
		return mapWriteErr(err)
	}
	if s.store != nil {
		s.removeFiles(ctx, a.TenantID, imageID) // setelah commit; berkas yatim tak berbahaya bila gagal
	}
	return nil
}

// OpenImage membuka berkas gambar (penuh atau thumbnail) setelah memastikan baris gambar milik item di tenant pemanggil.
func (s *Service) OpenImage(ctx context.Context, a authz.Actor, itemID, imageID uuid.UUID, thumb bool) (io.ReadSeekCloser, time.Time, error) {
	if s.store == nil {
		return nil, time.Time{}, ErrNoStorage
	}
	var created time.Time
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		row, err := gen.New(tx).ItemImageGet(ctx, gen.ItemImageGetParams{TenantID: a.TenantID, ItemID: itemID, ID: imageID})
		created = row.CreatedAt.Time
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, time.Time{}, ErrImageNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	key := fullKey(a.TenantID, imageID)
	if thumb {
		key = thumbKey(a.TenantID, imageID)
	}
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, time.Time{}, ErrImageNotFound
	}
	return f, created, err
}

func (s *Service) listImages(ctx context.Context, q *gen.Queries, a authz.Actor, itemID uuid.UUID) ([]Image, error) {
	rows, err := q.ItemImageList(ctx, gen.ItemImageListParams{TenantID: a.TenantID, ItemID: itemID})
	if err != nil {
		return nil, err
	}
	out := make([]Image, 0, len(rows))
	for _, r := range rows {
		out = append(out, imageOf(r.ID, r.Position, r.IsMain, r.Width, r.Height, r.FullBytes, r.ThumbBytes, r.CreatedAt.Time))
	}
	return out, nil
}
