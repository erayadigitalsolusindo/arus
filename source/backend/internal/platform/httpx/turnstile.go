package httpx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TurnstileHeader membawa token widget Cloudflare Turnstile dari klien web.
const TurnstileHeader = "X-Turnstile-Token"

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Turnstile memverifikasi token Cloudflare Turnstile di server. nil = fitur mati (Guard meneruskan semua permintaan).
type Turnstile struct {
	secret string
	url    string
	http   *http.Client
}

// NewTurnstile mengembalikan nil bila secret kosong.
func NewTurnstile(secret string) *Turnstile {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	return &Turnstile{secret: strings.TrimSpace(secret), url: turnstileVerifyURL, http: &http.Client{Timeout: 8 * time.Second}}
}

// Guard menolak permintaan tanpa token Turnstile yang sah. skip (boleh nil) melewati pemeriksaan untuk klien yang
// tidak bisa menampilkan widget (aplikasi mobile).
// shortcut: gagal-tutup — bila Cloudflare tak terjangkau, permintaan dijawab 503 (bukan diloloskan); skip hanya berdasar header X-Client, jadi bot yang mengaku mobile lolos — batasi lewat rate limit, upgrade ke attestation (Play Integrity) bila perlu.
func (t *Turnstile) Guard(skip func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if t == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip != nil && skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			token := strings.TrimSpace(r.Header.Get(TurnstileHeader))
			if token == "" || len(token) > 2048 {
				Error(w, http.StatusForbidden, "CAPTCHA_FAILED", "Verifikasi anti-robot gagal. Muat ulang halaman dan coba lagi.")
				return
			}
			ok, err := t.verify(r, token)
			switch {
			case err != nil:
				Error(w, http.StatusServiceUnavailable, "CAPTCHA_UNAVAILABLE", "Verifikasi anti-robot sementara tidak tersedia. Coba lagi.")
			case !ok:
				Error(w, http.StatusForbidden, "CAPTCHA_FAILED", "Verifikasi anti-robot gagal. Muat ulang halaman dan coba lagi.")
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

func (t *Turnstile) verify(r *http.Request, token string) (bool, error) {
	form := url.Values{"secret": {t.secret}, "response": {token}, "remoteip": {ClientIP(r)}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, t.url, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}
