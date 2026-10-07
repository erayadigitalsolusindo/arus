package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTTL = umur token akses (AGENTS.md §6: ≤15 menit).
const AccessTTL = 15 * time.Minute

// ImpersonationTTL = umur token "masuk sebagai" milik Platform Admin (tanpa refresh; untuk lanjut, masuk lagi).
const ImpersonationTTL = 30 * time.Minute

const (
	issuer         = "aciraba"
	platformIssuer = "aciraba-platform"
)

var (
	ErrTokenExpired = errors.New("token kedaluwarsa")
	ErrTokenInvalid = errors.New("token tidak valid")
)

// Claims: sub = user, tid = tenant, oid = outlet aktif, role = nama role.
// imp = id Platform Admin bila ini token "masuk sebagai" (sub juga berisi id Platform Admin, bukan pengguna tenant).
type Claims struct {
	TenantID     string `json:"tid"`
	OutletID     string `json:"oid"`
	Role         string `json:"role"`
	Impersonator string `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	secret []byte
	iss    string
}

func NewTokenIssuer(secret string) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), iss: issuer}
}

// NewPlatformTokenIssuer = penerbit token akses Platform Admin: issuer dan kunci turunan berbeda, sehingga token
// tenant tidak pernah lolos di rute platform (dan sebaliknya) walau memakai JWT_SECRET yang sama.
func NewPlatformTokenIssuer(secret string) *TokenIssuer {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("aciraba-platform-token-v1"))
	return &TokenIssuer{secret: mac.Sum(nil), iss: platformIssuer}
}

func (t *TokenIssuer) sign(c Claims) (string, error) {
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("tanda tangan token: %w", err)
	}
	return s, nil
}

func (t *TokenIssuer) Issue(userID, tenantID, outletID, role string, now time.Time) (string, error) {
	return t.sign(Claims{
		TenantID: tenantID, OutletID: outletID, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: t.iss, Subject: userID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
		},
	})
}

// IssueImpersonation menerbitkan token tenant untuk Platform Admin (hanya server yang memegang kunci penandatangan).
func (t *TokenIssuer) IssueImpersonation(adminID, tenantID, outletID string, now time.Time) (string, error) {
	return t.sign(Claims{
		TenantID: tenantID, OutletID: outletID, Role: "platform", Impersonator: adminID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: t.iss, Subject: adminID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ImpersonationTTL)),
		},
	})
}

// Parse memverifikasi tanda tangan (hanya HS256: mencegah alg=none dan confusion), issuer, dan kedaluwarsa.
func (t *TokenIssuer) Parse(token string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(t.iss), jwt.WithExpirationRequired())
	if errors.Is(err, jwt.ErrTokenExpired) {
		return nil, ErrTokenExpired
	}
	if err != nil {
		return nil, ErrTokenInvalid
	}
	return &c, nil
}
