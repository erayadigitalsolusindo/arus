package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	googleJWKSURL    = "https://www.googleapis.com/oauth2/v3/certs"
	googleJWKSTTL    = time.Hour
	googleJWKSRetry  = time.Minute // jeda minimum antar pengambilan ulang saat kid tak dikenal
	googleProviderID = "google"
)

var (
	ErrGoogleDisabled        = errors.New("login google tidak dikonfigurasi")
	ErrGoogleInvalidToken    = errors.New("token google tidak valid")
	ErrGoogleEmailUnverified = errors.New("email google belum terverifikasi")
	ErrGoogleNotRegistered   = errors.New("akun google belum terdaftar")
	ErrGoogleLinked          = errors.New("akun sudah tertaut ke akun google lain")
)

// GoogleClaims = isi ID token Google yang sudah diverifikasi server.
type GoogleClaims struct {
	Subject string
	Email   string
	Name    string
}

// GoogleVerifier memverifikasi ID token Google. Antarmuka agar test tidak menghubungi Google.
type GoogleVerifier interface {
	Verify(ctx context.Context, idToken string) (GoogleClaims, error)
}

// NewGoogleVerifier membuat verifier berbasis JWKS Google. audiences = daftar OAuth client ID yang diterima
// (web, dan server client ID untuk mobile). Kosong → nil (fitur mati).
func NewGoogleVerifier(audiences []string) GoogleVerifier {
	return NewGoogleVerifierWith(audiences, nil)
}

// AudienceSource menyediakan client ID tambahan saat runtime (mis. client ID per domain kustom dari database).
type AudienceSource func(ctx context.Context) ([]string, error)

const extraAudTTL = time.Minute

// NewGoogleVerifierWith = NewGoogleVerifier + sumber audience dinamis. Dengan sumber dinamis fitur tetap hidup walau
// GOOGLE_CLIENT_IDS kosong; client ID domain baru berlaku paling lambat satu menit setelah disimpan.
func NewGoogleVerifierWith(audiences []string, extra AudienceSource) GoogleVerifier {
	var aud []string
	for _, a := range audiences {
		if a = strings.TrimSpace(a); a != "" {
			aud = append(aud, a)
		}
	}
	if len(aud) == 0 && extra == nil {
		return nil
	}
	return &googleVerifier{audiences: aud, issuers: []string{"https://accounts.google.com", "accounts.google.com"}, extra: extra, client: &http.Client{Timeout: 10 * time.Second}, url: googleJWKSURL, now: time.Now}
}

type googleVerifier struct {
	audiences []string
	issuers   []string
	extra     AudienceSource
	extraAud  []string
	extraAt   time.Time
	client    *http.Client
	url       string
	now       func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func (v *googleVerifier) Verify(ctx context.Context, idToken string) (GoogleClaims, error) {
	if len(idToken) == 0 || len(idToken) > 4096 {
		return GoogleClaims{}, ErrGoogleInvalidToken
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired(), jwt.WithLeeway(time.Minute), jwt.WithTimeFunc(v.now))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "jwks") {
			return GoogleClaims{}, fmt.Errorf("verifikasi google: %w", err)
		}
		return GoogleClaims{}, ErrGoogleInvalidToken
	}
	if iss, _ := claims["iss"].(string); !slices.Contains(v.issuers, iss) {
		return GoogleClaims{}, ErrGoogleInvalidToken
	}
	auds, _ := claims.GetAudience()
	allowed := v.allAudiences(ctx)
	if !slices.ContainsFunc(auds, func(a string) bool { return slices.Contains(allowed, a) }) {
		return GoogleClaims{}, ErrGoogleInvalidToken
	}
	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	if sub == "" || email == "" {
		return GoogleClaims{}, ErrGoogleInvalidToken
	}
	switch ev := claims["email_verified"].(type) {
	case bool:
		if !ev {
			return GoogleClaims{}, ErrGoogleEmailUnverified
		}
	case string:
		if ev != "true" {
			return GoogleClaims{}, ErrGoogleEmailUnverified
		}
	default:
		return GoogleClaims{}, ErrGoogleEmailUnverified
	}
	return GoogleClaims{Subject: sub, Email: email, Name: name}, nil
}

// allAudiences = audience dari env + client ID domain (di-cache sebentar; gagal baca = pakai yang terakhir).
func (v *googleVerifier) allAudiences(ctx context.Context) []string {
	if v.extra == nil {
		return v.audiences
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.now().Sub(v.extraAt) >= extraAudTTL {
		if ids, err := v.extra(ctx); err == nil {
			v.extraAud = ids
		}
		v.extraAt = v.now()
	}
	return append(slices.Clone(v.audiences), v.extraAud...)
}

// key mengembalikan kunci publik untuk kid; JWKS di-cache dan hanya diambil ulang bila kedaluwarsa atau kid baru.
func (v *googleVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	if k, ok := v.keys[kid]; ok && now.Sub(v.fetchedAt) < googleJWKSTTL {
		return k, nil
	}
	if v.keys == nil || now.Sub(v.fetchedAt) >= googleJWKSTTL || now.Sub(v.fetchedAt) >= googleJWKSRetry {
		if err := v.refresh(ctx); err != nil {
			return nil, fmt.Errorf("jwks: %w", err)
		}
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("kid tidak dikenal")
}

func (v *googleVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return errors.New("tidak ada kunci")
	}
	v.keys, v.fetchedAt = keys, v.now()
	return nil
}
