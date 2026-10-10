// Package approval: persetujuan atasan lewat PIN (Owner/Supervisor) untuk tindakan sensitif di kasir, mulai dari ubah
// harga jual (FR-POS-04). Penyetuju = pengguna aktif pemegang izin `price_override.approve` yang punya akses ke outlet
// aktif. PIN 6 digit disimpan sebagai argon2id atas HMAC(rahasia server, tenant|pengguna|PIN) sehingga bocornya tabel
// `users` saja tidak cukup untuk menebak PIN secara offline. Percobaan salah dibatasi per penyetuju (Redis).
package approval

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/db"
)

// Module = modul izin penyetuju ubah harga (authz.Modules).
const Module = "price_override"

const (
	maxFailures = 5
	lockWindow  = 15 * time.Minute
)

var (
	ErrInvalidPin  = errors.New("penyetuju atau PIN salah")
	ErrPinRequired = errors.New("persetujuan penyetuju (PIN) wajib")
	ErrPinWeak     = errors.New("PIN terlalu mudah ditebak")
	ErrPinFormat   = errors.New("PIN harus 6 digit angka")
	ErrBadPassword = errors.New("password salah")
	ErrForbidden   = errors.New("tidak berhak memiliki PIN penyetuju")
)

// LockedError = terlalu banyak percobaan; RetryAfter = sisa waktu kunci.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string { return "terlalu banyak percobaan PIN" }

type Service struct {
	pool   *pgxpool.Pool
	rdb    *redis.Client // nil = tanpa pembatasan percobaan (hanya untuk test); main selalu mengisinya
	secret []byte
}

func NewService(pool *pgxpool.Pool, rdb *redis.Client, secret string) *Service {
	return &Service{pool: pool, rdb: rdb, secret: []byte(secret)}
}

type Approver struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

var pinPattern = regexp.MustCompile(`^[0-9]{6}$`)

// dummyHash dipakai untuk menyamakan waktu respons saat penyetuju tidak valid.
var dummyHash = func() string {
	h, _ := pauth.HashPassword("dummy")
	return h
}()

// ValidatePin: tepat 6 digit, bukan angka kembar (111111) atau deret naik/turun (123456, 654321).
func ValidatePin(pin string) error {
	if !pinPattern.MatchString(pin) {
		return ErrPinFormat
	}
	same, up, down := true, true, true
	for i := 1; i < len(pin); i++ {
		if pin[i] != pin[0] {
			same = false
		}
		if pin[i] != pin[i-1]+1 {
			up = false
		}
		if pin[i] != pin[i-1]-1 {
			down = false
		}
	}
	if same || up || down {
		return ErrPinWeak
	}
	return nil
}

func (s *Service) material(tenant, user uuid.UUID, pin string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(tenant.String() + "|" + user.String() + "|" + pin))
	return hex.EncodeToString(m.Sum(nil))
}

// ModuleOutletSwitch = modul izin penyetuju pindah outlet di kasir. PIN-nya sama dengan PIN penyetuju ubah harga
// (satu PIN per pengguna); yang berbeda hanya izin yang harus dimiliki.
const ModuleOutletSwitch = "outlet_switch"

// ValidModule: hanya modul penyetuju yang dikenal yang boleh diminta lewat API.
// ModuleSaleEdit = modul izin penyetuju edit/batal nota (PIN yang sama).
const ModuleSaleEdit = "sale_edit"

// ModuleCreditLimit = modul izin penyetuju penjualan kredit yang melewati limit piutang member (PIN yang sama).
const ModuleCreditLimit = "credit_limit"

// ModuleShiftClose = modul izin penyetuju tutup shift kasir yang memiliki selisih (PIN yang sama).
const ModuleShiftClose = "shift_close"

func ValidModule(m string) bool {
	return m == Module || m == ModuleOutletSwitch || m == ModuleSaleEdit || m == ModuleCreditLimit || m == ModuleShiftClose
}

// canApproveAny = pemegang salah satu izin penyetuju (boleh punya PIN; satu PIN untuk semua jenis persetujuan).
func canApproveAny(p authz.Permissions) bool {
	return p.Has(Module, authz.ActApprove) || p.Has(ModuleOutletSwitch, authz.ActApprove) || p.Has(ModuleSaleEdit, authz.ActApprove) || p.Has(ModuleCreditLimit, authz.ActApprove) || p.Has(ModuleShiftClose, authz.ActApprove)
}

func eligible(perms []byte, outletOK bool, module string) bool {
	return outletOK && authz.ParseStored(perms).Has(module, authz.ActApprove)
}

// Approvers = daftar penyetuju ubah harga yang bisa dipilih kasir di outlet aktif.
func (s *Service) Approvers(ctx context.Context, a authz.Actor) ([]Approver, error) {
	return s.ApproversFor(ctx, a, Module, a.OutletID)
}

// ApproversFor = penyetuju pemegang izin `module.approve` yang punya akses ke outlet `outletID`.
func (s *Service) ApproversFor(ctx context.Context, a authz.Actor, module string, outletID uuid.UUID) ([]Approver, error) {
	out := []Approver{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).ApprovalUsers(ctx, gen.ApprovalUsersParams{TenantID: a.TenantID, OutletID: outletID})
		for _, r := range rows {
			if eligible(r.Permissions, r.OutletOk, module) {
				out = append(out, Approver{ID: r.ID, Name: r.Name})
			}
		}
		return err
	})
	return out, err
}

func (s *Service) failKey(tenant, approver uuid.UUID) string {
	return "pin:fail:" + tenant.String() + ":" + approver.String()
}

// locked mengembalikan *LockedError bila kunci percobaan sudah mencapai batas.
func (s *Service) locked(ctx context.Context, key string) error {
	if s.rdb == nil {
		return nil
	}
	if n, err := s.rdb.Get(ctx, key).Int(); err == nil && n >= maxFailures {
		ttl, _ := s.rdb.TTL(ctx, key).Result()
		return &LockedError{RetryAfter: max(ttl, time.Second)}
	}
	return nil
}

func (s *Service) fail(ctx context.Context, key string) {
	if s.rdb == nil {
		return
	}
	if n, err := s.rdb.Incr(ctx, key).Result(); err == nil && n == 1 {
		s.rdb.Expire(ctx, key, lockWindow)
	}
}

// Verify memeriksa penyetuju + PIN di dalam transaksi pemanggil. Salah apa pun (penyetuju tak ada, nonaktif, tak
// berizin, tanpa PIN, PIN salah) menghasilkan ErrInvalidPin yang sama agar tidak membocorkan siapa yang berhak.
func (s *Service) Verify(ctx context.Context, tx pgx.Tx, a authz.Actor, approverID uuid.UUID, pin string) (Approver, error) {
	return s.VerifyFor(ctx, tx, a, Module, a.OutletID, approverID, pin)
}

// VerifyFor = Verify untuk izin `module.approve` dan akses penyetuju ke outlet `outletID` (mis. outlet tujuan).
func (s *Service) VerifyFor(ctx context.Context, tx pgx.Tx, a authz.Actor, module string, outletID, approverID uuid.UUID, pin string) (Approver, error) {
	if approverID == uuid.Nil || pin == "" {
		return Approver{}, ErrPinRequired
	}
	key := s.failKey(a.TenantID, approverID)
	if err := s.locked(ctx, key); err != nil {
		return Approver{}, err
	}
	u, err := gen.New(tx).ApprovalUserGet(ctx, gen.ApprovalUserGetParams{TenantID: a.TenantID, ID: approverID, OutletID: outletID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Approver{}, err
	}
	ok := false
	if err == nil && u.Active && u.PinHash.Valid && eligible(u.Permissions, u.OutletOk, module) {
		ok, _ = pauth.VerifyPassword(s.material(a.TenantID, approverID, pin), u.PinHash.String)
	} else {
		_, _ = pauth.VerifyPassword("x", dummyHash) // samakan waktu respons
	}
	if !ok {
		s.fail(ctx, key)
		return Approver{}, ErrInvalidPin
	}
	if s.rdb != nil {
		s.rdb.Del(ctx, key)
	}
	return Approver{ID: u.ID, Name: u.Name}, nil
}

// Check = Verify tanpa efek lain (kasir memeriksa PIN lebih dulu sebelum menyimpan nota).
func (s *Service) Check(ctx context.Context, a authz.Actor, approverID uuid.UUID, pin string) (Approver, error) {
	var out Approver
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.Verify(ctx, tx, a, approverID, pin)
		return err
	})
	return out, err
}

// PinStatus = apakah pengguna ini sudah punya PIN dan boleh menjadi penyetuju.
func (s *Service) PinStatus(ctx context.Context, a authz.Actor) (hasPin, canApprove bool, err error) {
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		r, e := gen.New(tx).ApprovalSelf(ctx, gen.ApprovalSelfParams{TenantID: a.TenantID, ID: a.UserID})
		perms := authz.ParseStored(r.Permissions)
		hasPin, canApprove = r.HasPin, canApproveAny(perms)
		return e
	})
	return
}

// SetPin menetapkan/mengganti PIN milik pelaku sendiri; wajib password akun (bukan hanya token sesi) dan hanya untuk
// pemegang izin penyetuju. Percobaan password salah dibatasi seperti PIN.
func (s *Service) SetPin(ctx context.Context, a authz.Actor, password, pin string) error {
	if a.UserID == uuid.Nil || a.Impersonator != uuid.Nil {
		return ErrForbidden
	}
	if err := ValidatePin(pin); err != nil {
		return err
	}
	key := "pin:pw:" + a.TenantID.String() + ":" + a.UserID.String()
	if err := s.locked(ctx, key); err != nil {
		return err
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		self, err := q.ApprovalSelf(ctx, gen.ApprovalSelfParams{TenantID: a.TenantID, ID: a.UserID})
		if err != nil {
			return err
		}
		if !canApproveAny(authz.ParseStored(self.Permissions)) {
			return ErrForbidden
		}
		if ok, _ := pauth.VerifyPassword(password, self.PasswordHash); !ok {
			s.fail(ctx, key)
			return ErrBadPassword
		}
		if s.rdb != nil {
			s.rdb.Del(ctx, key)
		}
		hash, err := pauth.HashPassword(s.material(a.TenantID, a.UserID, pin))
		if err != nil {
			return err
		}
		if err := q.ApprovalSetPin(ctx, gen.ApprovalSetPinParams{TenantID: a.TenantID, ID: a.UserID, PinHash: pgtype.Text{String: hash, Valid: true}}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionUserPinSet, Entity: audit.EntityUser, EntityID: a.UserID.String(),
		})
	})
}
