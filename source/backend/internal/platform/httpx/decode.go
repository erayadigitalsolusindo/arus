package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

// MaxBodyBytes membatasi ukuran body JSON (proteksi memori/DoS).
const MaxBodyBytes = 16 << 10

// DecodeJSON membaca body JSON secara ketat: Content-Type harus application/json, ukuran dibatasi,
// field tak dikenal ditolak, dan hanya satu nilai JSON yang diizinkan. Bila gagal, respons error
// sudah ditulis dan false dikembalikan.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return DecodeJSONLimit(w, r, dst, MaxBodyBytes)
}

// DecodeJSONLimit = DecodeJSON dengan batas ukuran sendiri, untuk body yang sah lebih besar (mis. teks keterangan item).
func DecodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			Error(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Ukuran data terlalu besar.")
			return false
		}
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "Format JSON tidak valid.")
		return false
	}
	// Tolak data tambahan setelah objek JSON pertama.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		Error(w, http.StatusBadRequest, "BAD_REQUEST", "Format JSON tidak valid.")
		return false
	}
	return true
}
