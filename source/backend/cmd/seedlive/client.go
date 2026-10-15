package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// api = klien HTTP tipis ke API ARUS. Semua tulis lewat endpoint nyata (validasi, stok, hutang/piutang, audit ikut berlaku).
// Token akses hanya 15 menit: diperbarui otomatis (login ulang + pindah outlet) sebelum kedaluwarsa atau saat 401.
type api struct {
	base, origin, email, pass string
	tok                       string
	tokAt                     time.Time
	outlet                    string // outlet aktif yang diinginkan (kosong = bawaan login)
	refresh                   string // cookie sesi (switch-outlet butuh sesi)
	hc                        *http.Client
	calls                     int
}

type apiErr struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]any
}

func (e *apiErr) Error() string {
	return fmt.Sprintf("HTTP %d %s: %s %v", e.Status, e.Code, e.Message, e.Fields)
}

func newAPI(base, origin, email, pass string) *api {
	return &api{base: strings.TrimRight(base, "/"), origin: origin, email: email, pass: pass, hc: &http.Client{Timeout: 60 * time.Second}}
}

func (a *api) raw(method, path string, body any, token string, key string) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", a.origin)
	req.Header.Set("X-Requested-With", "seedlive")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if a.refresh != "" && strings.HasPrefix(path, "/auth/") {
		req.Header.Set("Cookie", "refresh_token="+a.refresh)
	}
	res, err := a.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "refresh_token" && c.Value != "" {
			a.refresh = c.Value
		}
	}
	b, err := io.ReadAll(res.Body)
	return res.StatusCode, b, err
}

func decodeErr(status int, b []byte) error {
	var e struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
			Diff    any            `json:"diff"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	if e.Error.Code == "" {
		e.Error.Message = strings.TrimSpace(string(b))
		if len(e.Error.Message) > 200 {
			e.Error.Message = e.Error.Message[:200]
		}
	}
	if e.Error.Diff != nil {
		e.Error.Message += fmt.Sprintf(" [diff=%v]", e.Error.Diff)
	}
	return &apiErr{Status: status, Code: e.Error.Code, Message: e.Error.Message, Fields: e.Error.Fields}
}

func (a *api) login() error {
	st, b, err := a.raw("POST", "/auth/login", map[string]string{"email": a.email, "password": a.pass}, "", "")
	if err != nil {
		return err
	}
	if st != 200 {
		return decodeErr(st, b)
	}
	var r struct {
		Token  string `json:"access_token"`
		Outlet struct {
			ID string `json:"id"`
		} `json:"outlet"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	a.tok, a.tokAt = r.Token, time.Now()
	if a.outlet != "" && a.outlet != r.Outlet.ID {
		st, b, err = a.raw("POST", "/auth/switch-outlet", map[string]string{"outlet_id": a.outlet}, a.tok, "")
		if err != nil {
			return err
		}
		if st != 200 {
			return decodeErr(st, b)
		}
		var s struct {
			Token string `json:"access_token"`
		}
		if err := json.Unmarshal(b, &s); err != nil || s.Token == "" {
			return fmt.Errorf("pindah outlet: token kosong")
		}
		a.tok = s.Token
	}
	return nil
}

// useOutlet memindahkan outlet aktif sesi (login ulang + switch-outlet).
func (a *api) useOutlet(id string) error {
	a.outlet = id
	if a.tok == "" || time.Since(a.tokAt) > 9*time.Minute {
		return a.login()
	}
	// sesi masih hidup: cukup pindah outlet (hemat login yang dibatasi laju)
	st, b, err := a.raw("POST", "/auth/switch-outlet", map[string]string{"outlet_id": id}, a.tok, "")
	if err != nil {
		return err
	}
	if st != 200 {
		return a.login()
	}
	var s struct {
		Token string `json:"access_token"`
	}
	if err := json.Unmarshal(b, &s); err != nil || s.Token == "" {
		return a.login()
	}
	a.tok = s.Token
	return nil
}

// do mengirim request dan men-decode JSON sukses ke out (boleh nil). Galat HTTP jadi *apiErr.
func (a *api) do(method, path string, body any, out any) error {
	if !strings.HasPrefix(path, "/auth/") && (a.tok == "" || time.Since(a.tokAt) > 9*time.Minute) {
		if err := a.login(); err != nil {
			return err
		}
	}
	key := ""
	if method == "POST" {
		key = uuid.NewString()
	}
	a.calls++
	st, b, err := a.raw(method, path, body, a.tok, key)
	if err != nil {
		return err
	}
	if st == 401 {
		if err := a.login(); err != nil {
			return err
		}
		if st, b, err = a.raw(method, path, body, a.tok, key); err != nil {
			return err
		}
	}
	if st < 200 || st > 299 {
		return decodeErr(st, b)
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (a *api) get(path string, out any) error        { return a.do("GET", path, nil, out) }
func (a *api) post(path string, body, out any) error { return a.do("POST", path, body, out) }
func (a *api) put(path string, body, out any) error  { return a.do("PUT", path, body, out) }
func isCode(err error, code string) bool {
	e, ok := err.(*apiErr)
	return ok && e.Code == code
}
