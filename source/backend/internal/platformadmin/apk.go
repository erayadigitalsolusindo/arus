package platformadmin

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/storage"
)

// APK Kasir (ARUS Mobile) yang diunggah Platform Admin dan diunduh publik dari halaman login web.
const (
	ActionAPKUpload = "platform.apk_upload"
	ActionAPKDelete = "platform.apk_delete"

	apkKey  = "mobile/arus-kasir.apk"
	infoKey = "mobile/arus-kasir-info.json"

	// MaxAPKBytes = batas ukuran APK. Proxy di depan API harus mengizinkan body sebesar ini.
	MaxAPKBytes = 150 << 20
)

var apkVersionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)

var (
	errAPKTooLarge = errors.New("apk terlalu besar")
	errAPKInvalid  = errors.New("bukan berkas APK")
)

// APKInfo = metadata APK aktif. Available false bila belum pernah diunggah.
type APKInfo struct {
	Available  bool      `json:"available"`
	Size       int64     `json:"size,omitempty"`
	Version    string    `json:"version,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	UploadedAt time.Time `json:"uploaded_at,omitempty"`
}

// apkStore membungkus penyimpanan APK + metadatanya.
type apkStore struct{ fs *storage.Local }

func (a *apkStore) info(ctx context.Context) APKInfo {
	f, err := a.fs.Open(ctx, infoKey)
	if err != nil {
		return APKInfo{}
	}
	defer f.Close()
	var in APKInfo
	if err := json.NewDecoder(io.LimitReader(f, 64<<10)).Decode(&in); err != nil {
		return APKInfo{}
	}
	in.Available = true
	return in
}

// checkAPK memastikan berkas adalah ZIP yang memuat AndroidManifest.xml (APK = ZIP).
func checkAPK(path string, size int64) error {
	if size > MaxAPKBytes {
		return errAPKTooLarge
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return errAPKInvalid
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			return nil
		}
	}
	return errAPKInvalid
}

// save menyimpan APK dari r (sudah dibatasi pemanggil) dan menulis metadatanya.
func (a *apkStore) save(ctx context.Context, r io.Reader, version string) (APKInfo, error) {
	h := sha256.New()
	n, err := a.fs.PutStream(ctx, apkKey, io.TeeReader(io.LimitReader(r, MaxAPKBytes+1), h), checkAPK)
	if err != nil {
		return APKInfo{}, err
	}
	in := APKInfo{Available: true, Size: n, Version: version, SHA256: hex.EncodeToString(h.Sum(nil)), UploadedAt: time.Now().UTC()}
	raw, err := json.Marshal(in)
	if err != nil {
		return APKInfo{}, err
	}
	if err := a.fs.Put(ctx, infoKey, raw); err != nil {
		return APKInfo{}, err
	}
	return in, nil
}

func (a *apkStore) remove(ctx context.Context) error {
	if err := a.fs.Delete(ctx, apkKey); err != nil {
		return err
	}
	return a.fs.Delete(ctx, infoKey)
}

// ---- HTTP ----

// APKInfoPublic: GET /public/mobile/apk/info — dipakai halaman login untuk menampilkan tombol unduh.
func (h *Handler) APKInfoPublic(w http.ResponseWriter, r *http.Request) {
	if h.APK == nil {
		httpx.JSON(w, http.StatusOK, APKInfo{})
		return
	}
	httpx.JSON(w, http.StatusOK, (&apkStore{h.APK}).info(r.Context()))
}

// APKDownload: GET /public/mobile/apk — tanpa login.
func (h *Handler) APKDownload(w http.ResponseWriter, r *http.Request) {
	if h.APK == nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "APK belum tersedia.")
		return
	}
	f, err := h.APK.Open(r.Context(), apkKey)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "APK belum tersedia.")
		return
	}
	defer f.Close()
	in := (&apkStore{h.APK}).info(r.Context())
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "arus-kasir.apk"}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	modTime := in.UploadedAt
	http.ServeContent(w, r, "arus-kasir.apk", modTime, f)
}

// APKUpload: PUT /platform/mobile/apk — multipart/form-data: "version" (opsional, harus lebih dulu) lalu "file".
// Dibaca streaming (tanpa ParseMultipartForm) dan dibatasi MaxBytesReader.
func (h *Handler) APKUpload(w http.ResponseWriter, r *http.Request) {
	if h.APK == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "APK_DISABLED", "Penyimpanan APK tidak aktif.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAPKBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_BODY", "Permintaan harus multipart/form-data.")
		return
	}
	version := ""
	store := &apkStore{h.APK}
	var saved *APKInfo
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			h.apkBodyErr(w, r, err)
			return
		}
		switch p.FormName() {
		case "version":
			b, _ := io.ReadAll(io.LimitReader(p, 64))
			version = string(b)
			if version != "" && !apkVersionRe.MatchString(version) {
				httpx.Error(w, http.StatusUnprocessableEntity, "INVALID_VERSION", "Versi hanya huruf, angka, titik, minus, plus (maks 32).")
				return
			}
		case "file":
			in, err := store.save(r.Context(), p, version)
			if err != nil {
				h.apkBodyErr(w, r, err)
				return
			}
			saved = &in
		}
	}
	if saved == nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "FILE_REQUIRED", "Pilih berkas APK.")
		return
	}
	h.svc.record(r.Context(), actor(r), ActionAPKUpload, uuid.Nil, "", map[string]any{
		"version": saved.Version, "size": saved.Size, "sha256": saved.SHA256,
	})
	httpx.JSON(w, http.StatusOK, saved)
}

func (h *Handler) apkBodyErr(w http.ResponseWriter, r *http.Request, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig), errors.Is(err, errAPKTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "Ukuran APK maksimal 150 MB.")
	case errors.Is(err, errAPKInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "INVALID_APK", "Berkas bukan APK yang sah.")
	default:
		h.internal(w, r, "unggah APK gagal", err)
	}
}

// APKDelete: DELETE /platform/mobile/apk.
func (h *Handler) APKDelete(w http.ResponseWriter, r *http.Request) {
	if h.APK == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := (&apkStore{h.APK}).remove(r.Context()); err != nil {
		h.internal(w, r, "hapus APK gagal", err)
		return
	}
	h.svc.record(r.Context(), actor(r), ActionAPKDelete, uuid.Nil, "", nil)
	w.WriteHeader(http.StatusNoContent)
}
