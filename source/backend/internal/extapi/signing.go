// Package extapi: gateway API eksternal bertanda tangan HMAC-SHA256 + portal dokumentasi white label.
//
// Alur: klien menandatangani tiap request dengan secret kredensialnya; gateway (/ext/v1/*) memverifikasi tanda tangan,
// IP, timestamp, nonce, dan rate limit, lalu meneruskan request ke handler yang SUDAH ada dengan JWT internal berumur
// pendek atas nama akun layanan kredensial itu. Jadi izin, outlet, audit, dan idempotensi memakai jalur yang sama
// dengan aplikasi web; tidak ada handler yang diubah.
package extapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// Algorithm = label algoritma yang ditulis di baris pertama string kanonik.
const Algorithm = "HMAC-SHA256"

// Header permintaan bertanda tangan. Sengaja netral (tanpa merek) agar cocok untuk white label.
const (
	HeaderKey       = "X-Api-Key"
	HeaderTimestamp = "X-Api-Timestamp"
	HeaderNonce     = "X-Api-Nonce"
	HeaderSignature = "X-Api-Signature"
)

// CanonicalQuery mengurutkan parameter query (kunci, lalu nilai) dan meng-encode-nya. Query kosong = "".
func CanonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

// BodyHash = SHA-256 heksadesimal dari badan request mentah (string kosong untuk badan kosong).
func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// StringToSign menyusun string kanonik yang ditandatangani. Tujuh baris, dipisah "\n", tanpa "\n" penutup:
//
//	HMAC-SHA256
//	<METHOD huruf besar>
//	<path persis seperti dikirim, mis. /ext/v1/accounting/journals>
//	<query kanonik>
//	<timestamp unix detik>
//	<nonce>
//	<sha256 hex badan>
func StringToSign(method, path string, query url.Values, timestamp, nonce, bodyHash string) string {
	return strings.Join([]string{Algorithm, strings.ToUpper(method), path, CanonicalQuery(query), timestamp, nonce, bodyHash}, "\n")
}

// Sign = HMAC-SHA256(secret, stringToSign) dalam heksadesimal huruf kecil.
func Sign(secret, stringToSign string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return hex.EncodeToString(mac.Sum(nil))
}

// Equal membandingkan dua tanda tangan dalam waktu konstan.
func Equal(a, b string) bool {
	return hmac.Equal([]byte(strings.ToLower(a)), []byte(strings.ToLower(b)))
}
