package member

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/item"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/imaging"
	"aciraba/internal/platform/storage"
)

// Foto cover memakai pipeline gambar yang sama dengan item (validasi isi, resize, JPG, tanpa EXIF). Satu foto per
// member; mengganti foto membuat id baru (URL tidak pernah berubah isinya sehingga boleh di-cache lama).
var processSlots = make(chan struct{}, 2)

func fullKey(tenant, id uuid.UUID) string { return tenant.String() + "/member-" + id.String() + ".jpg" }
func thumbKey(tenant, id uuid.UUID) string {
	return tenant.String() + "/member-" + id.String() + "-t.jpg"
}

func (s *Service) removeFiles(ctx context.Context, tenant, id uuid.UUID) {
	_ = s.store.Delete(ctx, fullKey(tenant, id))
	_ = s.store.Delete(ctx, thumbKey(tenant, id))
}

func mapImageErr(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig), errors.Is(err, imaging.ErrTooLarge):
		return item.ErrImageTooLarge
	case errors.Is(err, imaging.ErrUnsupported):
		return item.ErrImageUnsupported
	case errors.Is(err, imaging.ErrDimensions):
		return item.ErrImageDimensions
	case errors.Is(err, imaging.ErrCorrupt):
		return item.ErrImageCorrupt
	}
	return err
}

// SetCover memproses dan menyimpan foto cover, menggantikan yang lama.
func (s *Service) SetCover(ctx context.Context, a authz.Actor, id uuid.UUID, r io.Reader) (*Member, error) {
	if s.store == nil {
		return nil, ErrNoStorage
	}
	if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := gen.New(tx).MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		return err
	}); err != nil {
		return nil, mapWriteErr(err)
	}
	select {
	case processSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	res, err := imaging.Process(r, item.MaxUploadBytes)
	<-processSlots
	if err != nil {
		return nil, mapImageErr(err)
	}
	imgID := uuid.New()
	if err := s.store.Put(ctx, fullKey(a.TenantID, imgID), res.Full); err != nil {
		return nil, fmt.Errorf("simpan cover: %w", err)
	}
	if err := s.store.Put(ctx, thumbKey(a.TenantID, imgID), res.Thumb); err != nil {
		s.removeFiles(ctx, a.TenantID, imgID)
		return nil, fmt.Errorf("simpan thumbnail cover: %w", err)
	}
	var m Member
	var old *uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		old = uuidPtr(cur.CoverImageID)
		if err := q.MemberSetCover(ctx, gen.MemberSetCoverParams{TenantID: a.TenantID, ID: id, CoverImageID: pgtype.UUID{Bytes: imgID, Valid: true}}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberCover, Entity: audit.EntityMember, EntityID: id.String(),
			Details: map[string]any{"code": cur.Code, "name": cur.Name, "action": "set", "bytes": len(res.Full)}}); err != nil {
			return err
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err != nil {
		s.removeFiles(ctx, a.TenantID, imgID)
		return nil, mapWriteErr(err)
	}
	if old != nil {
		s.removeFiles(ctx, a.TenantID, *old)
	}
	return &m, nil
}

// RemoveCover menghapus foto cover.
func (s *Service) RemoveCover(ctx context.Context, a authz.Actor, id uuid.UUID) (*Member, error) {
	var m Member
	var old *uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		old = uuidPtr(cur.CoverImageID)
		if old != nil {
			if err := q.MemberSetCover(ctx, gen.MemberSetCoverParams{TenantID: a.TenantID, ID: id}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberCover, Entity: audit.EntityMember, EntityID: id.String(),
				Details: map[string]any{"code": cur.Code, "name": cur.Name, "action": "remove"}}); err != nil {
				return err
			}
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err != nil {
		return nil, mapWriteErr(err)
	}
	if old != nil && s.store != nil {
		s.removeFiles(ctx, a.TenantID, *old)
	}
	return &m, nil
}

// OpenCover membuka berkas cover (penuh atau thumbnail) milik member di tenant pemanggil.
func (s *Service) OpenCover(ctx context.Context, a authz.Actor, id uuid.UUID, thumb bool) (io.ReadSeekCloser, string, error) {
	if s.store == nil {
		return nil, "", ErrNoStorage
	}
	var cover *uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).MemberGet(ctx, gen.MemberGetParams{TenantID: a.TenantID, ID: id})
		cover = uuidPtr(r.CoverImageID)
		return err
	})
	if err != nil {
		return nil, "", mapWriteErr(err)
	}
	if cover == nil {
		return nil, "", ErrNotFound
	}
	key := fullKey(a.TenantID, *cover)
	if thumb {
		key = thumbKey(a.TenantID, *cover)
	}
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, "", ErrNotFound
	}
	return f, cover.String(), err
}
