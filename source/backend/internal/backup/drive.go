package backup

// Klien Google Drive minimal (REST + OAuth2 refresh token), tanpa SDK. Cakupan `drive.file`: aplikasi hanya melihat berkas/folder
// yang ia buat sendiri, jadi Drive lain milik akun tidak terbaca. Unggah memakai sesi resumable per potongan 8 MiB (aliran tanpa
// ukuran awal, cocok untuk pipa pg_dump → enkripsi → Drive tanpa berkas sementara).

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	authURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenURL  = "https://oauth2.googleapis.com/token"
	filesURL  = "https://www.googleapis.com/drive/v3/files"
	uploadURL = "https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable"
	scope     = "https://www.googleapis.com/auth/drive.file openid email"
	upChunk   = 8 << 20 // kelipatan 256 KiB (syarat Drive)
)

var ErrDrive = errors.New("google drive menolak permintaan")

type drive struct {
	hc                           *http.Client
	clientID, secret, refreshTok string
	mu                           sync.Mutex
	access                       string
	exp                          time.Time
}

func newDrive(clientID, secret, refresh string) *drive {
	return &drive{hc: &http.Client{Timeout: 5 * time.Minute}, clientID: clientID, secret: secret, refreshTok: refresh}
}

func authLink(clientID, redirect, state string) string {
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {scope},
		"access_type": {"offline"}, "prompt": {"consent"}, "state": {state}}
	return authURL + "?" + q.Encode()
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func (d *drive) tokenCall(ctx context.Context, form url.Values) (tokenResp, error) {
	form.Set("client_id", d.clientID)
	form.Set("client_secret", d.secret)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.hc.Do(req)
	if err != nil {
		return tokenResp{}, err
	}
	defer resp.Body.Close()
	var t tokenResp
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t)
	if resp.StatusCode != http.StatusOK || t.AccessToken == "" {
		return t, fmt.Errorf("%w: token %s %s", ErrDrive, t.Error, t.ErrorDesc)
	}
	return t, nil
}

// exchange menukar kode OAuth dengan refresh token; mengembalikan juga email akun dari id_token.
func (d *drive) exchange(ctx context.Context, code, redirect string) (refresh, email string, err error) {
	t, err := d.tokenCall(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return "", "", err
	}
	if t.RefreshToken == "" {
		return "", "", fmt.Errorf("%w: tidak ada refresh token (cabut akses ARUS di akun Google lalu hubungkan ulang)", ErrDrive)
	}
	return t.RefreshToken, emailOf(t.IDToken), nil
}

// emailOf membaca klaim email dari id_token yang baru diterima langsung dari Google lewat TLS (tanpa verifikasi tanda tangan).
func emailOf(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(raw, &c)
	if len(c.Email) > 200 {
		return ""
	}
	return c.Email
}

func (d *drive) token(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.access != "" && time.Now().Before(d.exp) {
		return d.access, nil
	}
	t, err := d.tokenCall(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {d.refreshTok}})
	if err != nil {
		return "", err
	}
	d.access, d.exp = t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn-60)*time.Second)
	return d.access, nil
}

func (d *drive) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	tok, err := d.token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return d.hc.Do(req.WithContext(ctx))
}

func driveErr(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("%w: HTTP %d %s", ErrDrive, resp.StatusCode, strings.TrimSpace(string(b)))
}

// ensureFolder memastikan folder backup ada (bukan di tempat sampah); membuat baru bila hilang. changed=true bila id berubah.
func (d *drive) ensureFolder(ctx context.Context, id string) (folder string, changed bool, err error) {
	if id != "" {
		req, _ := http.NewRequest(http.MethodGet, filesURL+"/"+url.PathEscape(id)+"?fields=id,trashed", nil)
		resp, err := d.do(ctx, req)
		if err != nil {
			return "", false, err
		}
		defer resp.Body.Close()
		var f struct{ Trashed bool }
		if resp.StatusCode == http.StatusOK {
			_ = json.NewDecoder(resp.Body).Decode(&f)
			if !f.Trashed {
				return id, false, nil
			}
		} else if resp.StatusCode != http.StatusNotFound {
			return "", false, driveErr(resp)
		}
	}
	body, _ := json.Marshal(map[string]any{"name": "ARUS Backup", "mimeType": "application/vnd.google-apps.folder"})
	req, _ := http.NewRequest(http.MethodPost, filesURL+"?fields=id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.do(ctx, req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, driveErr(resp)
	}
	var out struct{ ID string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		return "", false, ErrDrive
	}
	return out.ID, true, nil
}

// upload mengunggah seluruh isi r sebagai berkas baru di folder; mengembalikan id dan jumlah byte.
func (d *drive) upload(ctx context.Context, folder, name string, r io.Reader) (string, int64, error) {
	meta, _ := json.Marshal(map[string]any{"name": name, "parents": []string{folder}})
	req, _ := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(meta))
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := d.do(ctx, req)
	if err != nil {
		return "", 0, err
	}
	resp.Body.Close()
	session := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusOK || session == "" {
		return "", 0, fmt.Errorf("%w: sesi unggah HTTP %d", ErrDrive, resp.StatusCode)
	}
	cur, next := make([]byte, upChunk), make([]byte, upChunk)
	cn, err := io.ReadFull(r, cur)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", 0, err
	}
	var sent int64
	for {
		nn := 0
		if cn == upChunk { // mungkin masih ada data: intip potongan berikutnya agar tahu mana yang terakhir
			if nn, err = io.ReadFull(r, next); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				return "", 0, err
			}
		}
		last := nn == 0
		total := "*"
		if last {
			total = fmt.Sprint(sent + int64(cn))
		}
		rng := fmt.Sprintf("bytes %d-%d/%s", sent, sent+int64(cn)-1, total)
		if cn == 0 {
			rng = "bytes */" + total
		}
		id, err := d.putChunk(ctx, session, cur[:cn], rng)
		if err != nil {
			return "", 0, err
		}
		sent += int64(cn)
		if last {
			if id == "" {
				return "", 0, fmt.Errorf("%w: unggah tidak selesai", ErrDrive)
			}
			return id, sent, nil
		}
		cur, next, cn = next, cur, nn
	}
}

// putChunk mengirim satu potongan (diulang sampai 3 kali bila gangguan jaringan/5xx). id terisi hanya pada potongan terakhir.
func (d *drive) putChunk(ctx context.Context, session string, b []byte, rng string) (string, error) {
	var last error
	for try := 0; try < 3; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(try) * 5 * time.Second):
			}
		}
		req, _ := http.NewRequest(http.MethodPut, session, bytes.NewReader(b))
		req.Header.Set("Content-Range", rng)
		resp, err := d.do(ctx, req)
		if err != nil {
			last = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusPermanentRedirect: // 308: potongan diterima, lanjut
			resp.Body.Close()
			return "", nil
		case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
			var out struct{ ID string }
			_ = json.NewDecoder(resp.Body).Decode(&out)
			resp.Body.Close()
			return out.ID, nil
		case resp.StatusCode >= 500:
			last = driveErr(resp)
			resp.Body.Close()
		default:
			err := driveErr(resp)
			resp.Body.Close()
			return "", err
		}
	}
	return "", last
}

func (d *drive) download(ctx context.Context, id string) (io.ReadCloser, error) {
	req, _ := http.NewRequest(http.MethodGet, filesURL+"/"+url.PathEscape(id)+"?alt=media", nil)
	resp, err := d.do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, driveErr(resp)
	}
	return resp.Body, nil
}

// remove menghapus berkas; 404 dianggap sudah terhapus.
func (d *drive) remove(ctx context.Context, id string) error {
	req, _ := http.NewRequest(http.MethodDelete, filesURL+"/"+url.PathEscape(id), nil)
	resp, err := d.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return driveErr(resp)
}
