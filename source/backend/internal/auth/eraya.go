package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const erayaProviderID = "eraya"

var ErrErayaDisabled = errors.New("login eraya tidak dikonfigurasi")

// Eraya = klien OIDC untuk SSO Eraya (Authorization Code + PKCE). SPA mengarahkan pengguna ke Issuer, lalu
// menukar `code` lewat server (Exchange) dan memakai id_token-nya seperti token Google (Verifier).
type Eraya struct {
	Issuer, ClientID, ClientSecret string
	Verifier                       GoogleVerifier
	http                           *http.Client
}

// NewEraya mengembalikan nil (fitur mati) bila issuer atau client ID kosong. ClientSecret kosong = client publik.
func NewEraya(issuer, clientID, clientSecret string) *Eraya {
	issuer, clientID = strings.TrimRight(strings.TrimSpace(issuer), "/"), strings.TrimSpace(clientID)
	if issuer == "" || clientID == "" {
		return nil
	}
	v := &googleVerifier{
		audiences: []string{clientID}, issuers: []string{issuer},
		client: &http.Client{Timeout: 10 * time.Second}, url: issuer + "/.well-known/jwks.json", now: time.Now,
	}
	return &Eraya{Issuer: issuer, ClientID: clientID, ClientSecret: strings.TrimSpace(clientSecret), Verifier: v, http: v.client}
}

// Exchange menukar authorization code dengan id_token. Galat penolakan dari Eraya (code salah/kedaluwarsa) = ErrGoogleInvalidToken.
func (e *Eraya) Exchange(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier},
		"redirect_uri": {redirectURI}, "client_id": {e.ClientID},
	}
	if e.ClientSecret != "" {
		form.Set("client_secret", e.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := e.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		return "", errors.New("eraya token: status " + res.Status)
	}
	var out struct {
		IDToken string `json:"id_token"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&out) != nil || out.IDToken == "" {
		return "", ErrGoogleInvalidToken
	}
	return out.IDToken, nil
}
