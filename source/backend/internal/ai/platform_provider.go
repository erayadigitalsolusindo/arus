package ai

// "API ARUS": penyedia AI milik platform untuk akun gratis HalloArus, diatur Platform Admin dari panel (migrasi 00099).
// Berbicara OpenAI-compatible lewat klien ber-SSRF-guard yang sama dengan penyedia kustom tenant. Belum diatur → akun gratis
// memakai AI_PLATFORM_KEY (Claude) dari env. Key tidak pernah keluar dari server (panel hanya melihat 4 karakter terakhir).

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	platformAAD = "platform"
	ppTTL       = 10 * time.Second
	// ProviderArus = penanda penyedia pada hasil/audit untuk akun gratis lewat API ARUS.
	ProviderArus = "arus"
)

type platformProv struct{ base, model, key string }

type ppCache struct {
	mu  sync.Mutex
	pp  *platformProv
	exp time.Time
}

// PlatformProvider = konfigurasi API ARUS untuk panel operator (tanpa key).
type PlatformProvider struct {
	Configured bool      `json:"configured"`
	BaseURL    string    `json:"base_url"`
	Model      string    `json:"model"`
	KeyHint    string    `json:"key_hint"`
	UpdatedAt  time.Time `json:"updated_at"`
	Author     string    `json:"author"`
}

type PlatformProviderInput struct {
	BaseURL string
	Model   string
	APIKey  string // kosong = pertahankan key lama (hanya bila base URL tidak berubah)
}

// platformProv = konfigurasi API ARUS aktif (cache singkat). nil = belum diatur atau gagal dibaca (jatuh ke env).
func (s *Service) platformProv(ctx context.Context) *platformProv {
	if s.pool == nil || s.ppc == nil {
		return nil
	}
	s.ppc.mu.Lock()
	defer s.ppc.mu.Unlock()
	if time.Now().Before(s.ppc.exp) {
		return s.ppc.pp
	}
	var base, model, enc string
	err := s.pool.QueryRow(ctx, `SELECT base_url, model, api_key_enc FROM ai_platform_provider_load()`).Scan(&base, &model, &enc)
	var p *platformProv
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		s.log.WarnContext(ctx, "ai: API ARUS gagal dibaca", "err", err)
	default:
		if key, err := s.box.open(enc, platformAAD); err != nil {
			s.log.ErrorContext(ctx, "ai: key API ARUS tak bisa dibuka (JWT_SECRET berubah?)", "err", err)
		} else {
			p = &platformProv{base: base, model: model, key: key}
		}
	}
	s.ppc.pp, s.ppc.exp = p, time.Now().Add(ppTTL)
	return p
}

// ppCached = hasil terakhir platformProv tanpa membaca DB (dipakai pemeriksaan sinkron; panggil platformProv dulu).
func (s *Service) ppCached() *platformProv {
	if s.ppc == nil {
		return nil
	}
	s.ppc.mu.Lock()
	defer s.ppc.mu.Unlock()
	return s.ppc.pp
}

func (s *Service) ppReset() {
	if s.ppc == nil {
		return
	}
	s.ppc.mu.Lock()
	s.ppc.pp, s.ppc.exp = nil, time.Time{}
	s.ppc.mu.Unlock()
}

// GetPlatformProvider = konfigurasi API ARUS saat ini untuk panel operator.
func (s *Service) GetPlatformProvider(ctx context.Context, adminID uuid.UUID) (PlatformProvider, error) {
	var out PlatformProvider
	err := s.pool.QueryRow(ctx, `SELECT base_url, model, key_hint, updated_at, author FROM platform_ai_provider_get($1)`, adminID).
		Scan(&out.BaseURL, &out.Model, &out.KeyHint, &out.UpdatedAt, &out.Author)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlatformProvider{}, nil
	}
	out.Configured = err == nil
	return out, err
}

// SavePlatformProvider memverifikasi (chat 1 token ke endpoint-nya) lalu menyimpan. Ganti base URL → wajib key baru.
func (s *Service) SavePlatformProvider(ctx context.Context, adminID uuid.UUID, in PlatformProviderInput) error {
	model := strings.TrimSpace(in.Model)
	apiKey := strings.TrimSpace(in.APIKey)
	if !customModel.MatchString(model) || (apiKey != "" && !customKeyOK(apiKey)) {
		return ErrBadInput
	}
	base, err := validateBaseURL(in.BaseURL)
	if err != nil {
		return err
	}
	u, _ := url.Parse(base)
	if err := checkResolve(ctx, u.Hostname()); err != nil {
		return err
	}
	var oldBase, oldModel, oldEnc string
	_ = s.pool.QueryRow(ctx, `SELECT base_url, model, api_key_enc FROM ai_platform_provider_load()`).Scan(&oldBase, &oldModel, &oldEnc)
	verifyKey := apiKey
	if apiKey == "" {
		if oldEnc == "" || oldBase != base {
			return ErrBadKey // key hanya boleh dikirim ke host yang tersimpan
		}
		if verifyKey, err = s.box.open(oldEnc, platformAAD); err != nil {
			return err
		}
	}
	if err := newCustomClient(base).verify(ctx, verifyKey, model); err != nil {
		return err
	}
	enc, hint := oldEnc, ""
	if apiKey != "" {
		if enc, err = s.box.seal(apiKey, platformAAD); err != nil {
			return err
		}
		hint = apiKey[max(len(apiKey)-4, 0):]
	} else {
		hint = s.keyHintOf(ctx, adminID)
	}
	if _, err := s.pool.Exec(ctx, `SELECT platform_ai_provider_save($1, $2, $3, $4, $5)`, adminID, base, model, enc, hint); err != nil {
		return err
	}
	s.ppReset()
	return nil
}

func (s *Service) keyHintOf(ctx context.Context, adminID uuid.UUID) string {
	var h string
	_ = s.pool.QueryRow(ctx, `SELECT key_hint FROM platform_ai_provider_get($1)`, adminID).Scan(&h)
	return h
}

// ClearPlatformProvider menghapus konfigurasi API ARUS (akun gratis kembali ke AI_PLATFORM_KEY bila ada). found=false: memang kosong.
func (s *Service) ClearPlatformProvider(ctx context.Context, adminID uuid.UUID) (bool, error) {
	var found bool
	if err := s.pool.QueryRow(ctx, `SELECT platform_ai_provider_clear($1)`, adminID).Scan(&found); err != nil {
		return false, err
	}
	s.ppReset()
	return found, nil
}

// IsUpstream = galat berasal dari penyedia AI (jaringan/penolakan), bukan masukan operator.
func IsUpstream(err error) bool {
	var up *upstreamError
	return errors.As(err, &up)
}
