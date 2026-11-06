// Package profile: profil pribadi pengguna yang sedang masuk (identitas, foto profil, banner, ganti password).
// Semua operasi hanya untuk diri sendiri: id pengguna selalu dari token, tidak pernah dari request.
package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/item"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/imaging"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/platform/storage"
)

var (
	ErrNotFound    = errors.New("pengguna tidak ditemukan")
	ErrForbidden   = errors.New("tidak boleh mengubah profil ini")
	ErrNoStorage   = errors.New("penyimpanan foto tidak tersedia")
	ErrBadPassword = errors.New("password saat ini salah")
)

type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

const (
	maxName     = 100
	maxJobTitle = 80
	maxLocation = 80
	maxBio      = 1000
)

// Kind = jenis gambar profil.
type Kind string

const (
	Avatar Kind = "avatar"
	Banner Kind = "banner"
)

type Service struct {
	pool     *pgxpool.Pool
	store    storage.Store // nil = foto profil mati (ErrNoStorage)
	sessions *pauth.Sessions
	perms    *authz.Resolver
	now      func() time.Time
}

func NewService(pool *pgxpool.Pool, store storage.Store, sessions *pauth.Sessions, perms *authz.Resolver) *Service {
	return &Service{pool: pool, store: store, sessions: sessions, perms: perms, now: time.Now}
}

// Profile = tampilan profil lengkap. Kelengkapan dihitung di server agar web dan mobile konsisten.
type Profile struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Email         string     `json:"email"`
	Phone         string     `json:"phone"`
	JobTitle      string     `json:"job_title"`
	Bio           string     `json:"bio"`
	Location      string     `json:"location"`
	AvatarID      *string    `json:"avatar_id"`
	BannerID      *string    `json:"banner_id"`
	Role          string     `json:"role"`
	Tenant        string     `json:"tenant"`
	TenantCode    string     `json:"tenant_code"`
	OutletCount   int        `json:"outlet_count"`
	EmailVerified bool       `json:"email_verified"`
	MFAEnabled    bool       `json:"mfa_enabled"`
	Actions30d    int        `json:"actions_30d"`
	Logins30d     int        `json:"logins_30d"`
	GoogleLinked  bool       `json:"google_linked"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	Completeness  int        `json:"completeness"`
	// Missing = kunci isian yang belum diisi, urut prioritas (untuk saran di UI).
	Missing []string `json:"missing"`
}

func uuidStr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := uuid.UUID(u.Bytes).String()
	return &s
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func completeness(p *Profile) {
	fields := []struct {
		key string
		ok  bool
	}{
		{"avatar", p.AvatarID != nil},
		{"phone", p.Phone != ""},
		{"job_title", p.JobTitle != ""},
		{"location", p.Location != ""},
		{"bio", p.Bio != ""},
		{"banner", p.BannerID != nil},
		{"email_verified", p.EmailVerified},
		{"mfa", p.MFAEnabled},
	}
	done := 0
	p.Missing = []string{}
	for _, f := range fields {
		if f.ok {
			done++
		} else {
			p.Missing = append(p.Missing, f.key)
		}
	}
	// Nama dan email selalu ada sehingga dihitung sebagai dua isian terpenuhi.
	p.Completeness = (done + 2) * 100 / (len(fields) + 2)
}

func load(ctx context.Context, pool *pgxpool.Pool, tx pgx.Tx, tenant, id uuid.UUID) (*Profile, error) {
	r, err := gen.New(tx).ProfileGet(ctx, gen.ProfileGetParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := &Profile{
		ID: r.ID.String(), Name: r.Name, Email: r.Email, Phone: r.Phone.String, JobTitle: r.JobTitle.String,
		Bio: r.Bio.String, Location: r.Location.String, AvatarID: uuidStr(r.AvatarImageID), BannerID: uuidStr(r.BannerImageID),
		Role: r.RoleName, Tenant: r.TenantName, TenantCode: r.TenantCode, OutletCount: int(r.OutletCount),
		EmailVerified: r.EmailVerified, MFAEnabled: r.MfaEnabled, LastLoginAt: timePtr(r.LastLoginAt), CreatedAt: r.CreatedAt.Time,
	}
	if err := pool.QueryRow(ctx, "SELECT auth_identity_has('google', $1)", id).Scan(&p.GoogleLinked); err != nil {
		return nil, err
	}
	st, err := gen.New(tx).ProfileStats(ctx, gen.ProfileStatsParams{TenantID: tenant, ActorID: pgtype.UUID{Bytes: id, Valid: true}, Since: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -30), Valid: true}})
	if err != nil {
		return nil, err
	}
	p.Actions30d, p.Logins30d = int(st.Actions), int(st.Logins)
	completeness(p)
	return p, nil
}

// Activity = satu baris jejak aktivitas pengguna sendiri.
type Activity struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	Entity    string    `json:"entity"`
	EntityID  string    `json:"entity_id"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

type ActivityPage struct {
	Data []Activity `json:"data"`
	// Next = kursor halaman berikutnya (kosong bila habis): "<unix_nano>.<id>".
	Next string `json:"next"`
}

// ActivityList membaca jejak aktivitas pemanggil sendiri (keyset, tanpa OFFSET).
func (s *Service) ActivityList(ctx context.Context, a authz.Actor, limit int, cursor string) (*ActivityPage, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var before pgtype.Timestamptz
	var beforeID int64
	if cursor != "" {
		ns, id, ok := strings.Cut(cursor, ".")
		n, e1 := strconv.ParseInt(ns, 10, 64)
		bid, e2 := strconv.ParseInt(id, 10, 64)
		if !ok || e1 != nil || e2 != nil {
			return nil, FieldErrors{"cursor": sanitize.Invalid}
		}
		before, beforeID = pgtype.Timestamptz{Time: time.Unix(0, n).UTC(), Valid: true}, bid
	}
	page := &ActivityPage{Data: []Activity{}}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).ProfileActivity(ctx, gen.ProfileActivityParams{TenantID: a.TenantID, ActorID: pgtype.UUID{Bytes: a.UserID, Valid: true},
			BeforeAt: before, BeforeID: beforeID, Lim: int32(limit + 1)})
		if err != nil {
			return err
		}
		for i, r := range rows {
			if i == limit {
				last := page.Data[limit-1]
				page.Next = strconv.FormatInt(last.CreatedAt.UnixNano(), 10) + "." + strconv.FormatInt(last.ID, 10)
				break
			}
			page.Data = append(page.Data, Activity{ID: r.ID, Action: r.Action, Entity: r.Entity, EntityID: r.EntityID, IP: r.Ip, CreatedAt: r.CreatedAt.Time})
		}
		return nil
	})
	return page, err
}

func (s *Service) Get(ctx context.Context, a authz.Actor) (*Profile, error) {
	var p *Profile
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
		p, err = load(ctx, s.pool, tx, a.TenantID, a.UserID)
		return
	})
	return p, err
}

// UpdateInput: isian teks profil. String kosong = hapus isian opsional.
type UpdateInput struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	JobTitle string `json:"job_title"`
	Bio      string `json:"bio"`
	Location string `json:"location"`
}

func optText(s string, max int) (pgtype.Text, string) {
	v, ok := sanitize.Text(s)
	switch {
	case !ok:
		return pgtype.Text{}, sanitize.Invalid
	case v == "":
		return pgtype.Text{}, ""
	case len([]rune(v)) > max:
		return pgtype.Text{}, sanitize.TooLong
	}
	return pgtype.Text{String: v, Valid: true}, ""
}

func (s *Service) Update(ctx context.Context, a authz.Actor, in UpdateInput) (*Profile, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	f := FieldErrors{}
	name, code := sanitize.Name(in.Name, maxName)
	if code != "" {
		f["name"] = code
	}
	var phone pgtype.Text
	if in.Phone != "" {
		v, c := sanitize.Phone(in.Phone)
		if c != "" {
			f["phone"] = c
		} else {
			phone = pgtype.Text{String: v, Valid: true}
		}
	}
	job, c := optText(in.JobTitle, maxJobTitle)
	if c != "" {
		f["job_title"] = c
	}
	loc, c := optText(in.Location, maxLocation)
	if c != "" {
		f["location"] = c
	}
	var bio pgtype.Text
	if v, c := sanitize.Multiline(in.Bio, maxBio); c != "" {
		f["bio"] = c
	} else if v != "" {
		bio = pgtype.Text{String: v, Valid: true}
	}
	if len(f) > 0 {
		return nil, f
	}
	var p *Profile
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		n, err := gen.New(tx).ProfileUpdate(ctx, gen.ProfileUpdateParams{TenantID: a.TenantID, ID: a.UserID,
			Name: name, Phone: phone, JobTitle: job, Bio: bio, Location: loc})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionUserProfile, Entity: audit.EntityUser, EntityID: a.UserID.String()}); err != nil {
			return err
		}
		p, err = load(ctx, s.pool, tx, a.TenantID, a.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

func fullKey(tenant, id uuid.UUID) string { return tenant.String() + "/user-" + id.String() + ".jpg" }
func thumbKey(tenant, id uuid.UUID) string {
	return tenant.String() + "/user-" + id.String() + "-t.jpg"
}

func (s *Service) removeFiles(ctx context.Context, tenant, id uuid.UUID) {
	_ = s.store.Delete(ctx, fullKey(tenant, id))
	_ = s.store.Delete(ctx, thumbKey(tenant, id))
}

func idOf(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

func setImage(ctx context.Context, q *gen.Queries, k Kind, tenant, user uuid.UUID, img pgtype.UUID) error {
	if k == Banner {
		return q.ProfileSetBanner(ctx, gen.ProfileSetBannerParams{TenantID: tenant, ID: user, ImageID: img})
	}
	return q.ProfileSetAvatar(ctx, gen.ProfileSetAvatarParams{TenantID: tenant, ID: user, ImageID: img})
}

func current(r gen.ProfileImagesRow, k Kind) *uuid.UUID {
	if k == Banner {
		return idOf(r.BannerImageID)
	}
	return idOf(r.AvatarImageID)
}

// SetImage memproses dan menyimpan foto profil/banner, menggantikan yang lama (pipeline gambar yang sama dengan item).
func (s *Service) SetImage(ctx context.Context, a authz.Actor, k Kind, r io.Reader) (*Profile, error) {
	if s.store == nil {
		return nil, ErrNoStorage
	}
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	res, err := imaging.ProcessLimited(ctx, r, item.MaxUploadBytes)
	if err != nil {
		return nil, item.MapImageErr(err)
	}
	imgID := uuid.New()
	if err := s.store.Put(ctx, fullKey(a.TenantID, imgID), res.Full); err != nil {
		return nil, fmt.Errorf("simpan foto profil: %w", err)
	}
	if err := s.store.Put(ctx, thumbKey(a.TenantID, imgID), res.Thumb); err != nil {
		s.removeFiles(ctx, a.TenantID, imgID)
		return nil, fmt.Errorf("simpan thumbnail profil: %w", err)
	}
	var p *Profile
	var old *uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.ProfileImages(ctx, gen.ProfileImagesParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		old = current(cur, k)
		if err := setImage(ctx, q, k, a.TenantID, a.UserID, pgtype.UUID{Bytes: imgID, Valid: true}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionUserProfileImage, Entity: audit.EntityUser,
			EntityID: a.UserID.String(), Details: map[string]any{"kind": string(k), "action": "set", "bytes": len(res.Full)}}); err != nil {
			return err
		}
		p, err = load(ctx, s.pool, tx, a.TenantID, a.UserID)
		return err
	})
	if err != nil {
		s.removeFiles(ctx, a.TenantID, imgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if old != nil {
		s.removeFiles(ctx, a.TenantID, *old)
	}
	return p, nil
}

func (s *Service) RemoveImage(ctx context.Context, a authz.Actor, k Kind) (*Profile, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrForbidden
	}
	var p *Profile
	var old *uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.ProfileImages(ctx, gen.ProfileImagesParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		old = current(cur, k)
		if old != nil {
			if err := setImage(ctx, q, k, a.TenantID, a.UserID, pgtype.UUID{}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionUserProfileImage, Entity: audit.EntityUser,
				EntityID: a.UserID.String(), Details: map[string]any{"kind": string(k), "action": "remove"}}); err != nil {
				return err
			}
		}
		p, err = load(ctx, s.pool, tx, a.TenantID, a.UserID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if old != nil && s.store != nil {
		s.removeFiles(ctx, a.TenantID, *old)
	}
	return p, nil
}

// OpenImage membuka berkas foto profil/banner milik pengguna di tenant pemanggil (owner = pemilik foto).
// Lintas tenant ditolak RLS + filter tenant_id.
func (s *Service) OpenImage(ctx context.Context, a authz.Actor, owner uuid.UUID, k Kind, thumb bool) (io.ReadSeekCloser, string, error) {
	if s.store == nil {
		return nil, "", ErrNoStorage
	}
	var img *uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).ProfileGet(ctx, gen.ProfileGetParams{TenantID: a.TenantID, ID: owner})
		if err != nil {
			return err
		}
		if k == Banner {
			img = idOf(r.BannerImageID)
		} else {
			img = idOf(r.AvatarImageID)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if img == nil {
		return nil, "", ErrNotFound
	}
	key := fullKey(a.TenantID, *img)
	if thumb {
		key = thumbKey(a.TenantID, *img)
	}
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, "", ErrNotFound
	}
	return f, img.String(), err
}

// ChangePassword mengganti password sendiri: wajib password saat ini. Berhasil → semua token akses dan sesi refresh
// dicabut (pengguna harus masuk lagi). Percobaan salah dibatasi rate limit pada rute.
func (s *Service) ChangePassword(ctx context.Context, a authz.Actor, current, next string) error {
	if a.Impersonator != uuid.Nil {
		return ErrForbidden
	}
	f := FieldErrors{}
	if current == "" {
		f["current_password"] = sanitize.Required
	}
	var email string
	if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, err := gen.New(tx).ProfileGet(ctx, gen.ProfileGetParams{TenantID: a.TenantID, ID: a.UserID})
		email = r.Email
		return err
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if c := sanitize.Password(next, email); c != "" {
		f["password"] = c
	} else if next == current {
		f["password"] = "SAME_AS_CURRENT"
	}
	if len(f) > 0 {
		return f
	}
	hash, err := pauth.HashPassword(next)
	if err != nil {
		return err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		old, err := q.ProfileCredentials(ctx, gen.ProfileCredentialsParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		if ok, _ := pauth.VerifyPassword(current, old); !ok {
			return ErrBadPassword
		}
		if _, err := q.ProfileSetPassword(ctx, gen.ProfileSetPasswordParams{TenantID: a.TenantID, ID: a.UserID, PasswordHash: hash,
			ValidAfter: pgtype.Timestamptz{Time: s.now(), Valid: true}}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionUserPasswordChange, Entity: audit.EntityUser, EntityID: a.UserID.String()})
	})
	if err != nil {
		return err
	}
	s.perms.InvalidateTenant(a.TenantID)
	return s.sessions.RevokeUser(ctx, a.UserID.String())
}
