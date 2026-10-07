package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTTL = umur token akses (AGENTS.md §6: ≤15 menit).
const AccessTTL = 15 * time.Minute

const issuer = "aciraba"

var (
	ErrTokenExpired = errors.New("token kedaluwarsa")
	ErrTokenInvalid = errors.New("token tidak valid")
)

// Claims: sub = user, tid = tenant, oid = outlet aktif, role = nama role.
type Claims struct {
	TenantID string `json:"tid"`
	OutletID string `json:"oid"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type TokenIssuer struct{ secret []byte }

func NewTokenIssuer(secret string) *TokenIssuer { return &TokenIssuer{secret: []byte(secret)} }

func (t *TokenIssuer) Issue(userID, tenantID, outletID, role string, now time.Time) (string, error) {
	c := Claims{
		TenantID: tenantID, OutletID: outletID, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: userID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("tanda tangan token: %w", err)
	}
	return s, nil
}

// Parse memverifikasi tanda tangan (hanya HS256: mencegah alg=none dan confusion), issuer, dan kedaluwarsa.
func (t *TokenIssuer) Parse(token string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer), jwt.WithExpirationRequired())
	if errors.Is(err, jwt.ErrTokenExpired) {
		return nil, ErrTokenExpired
	}
	if err != nil {
		return nil, ErrTokenInvalid
	}
	return &c, nil
}
