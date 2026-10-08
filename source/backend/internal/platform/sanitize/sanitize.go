// Package sanitize menormalkan dan memvalidasi input teks dari pengguna. Server adalah penjaga terakhir:
// validasi di klien hanya untuk kenyamanan. Pencegahan XSS utama tetap output encoding di UI
// (Svelte meng-escape otomatis; jangan pernah memakai {@html} untuk data pengguna).
package sanitize

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Kode galat per field (diterjemahkan klien lewat errors/validation).
const (
	Required = "REQUIRED"
	Invalid  = "INVALID"
	TooLong  = "TOO_LONG"
	TooShort = "TOO_SHORT"
	Weak     = "WEAK"
)

// Text menormalkan NFC, menolak karakter kontrol/format tak terlihat (null, zero-width, bidi override),
// memadatkan whitespace, dan men-trim. Mengembalikan ok=false bila ada karakter yang tidak diizinkan.
func Text(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	s = norm.NFC.String(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r), r == unicode.ReplacementChar:
			return "", false
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String(), true
}

// Name memvalidasi nama (bisnis/orang/outlet): teks bersih, 1..max karakter, tanpa < > (markup).
// Sengaja menolak (bukan membuang) agar pengguna tahu inputnya tidak diterima.
func Name(s string, max int) (string, string) {
	v, ok := Text(s)
	switch {
	case !ok, strings.ContainsAny(v, "<>"):
		return "", Invalid
	case v == "":
		return "", Required
	case utf8.RuneCountInString(v) > max:
		return "", TooLong
	}
	return v, ""
}

var emailRe = regexp.MustCompile("^[a-z0-9.!#$%&'*+/=?^_{|}~-]{1,64}@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$")

// Email: huruf kecil, ASCII, satu alamat polos (tanpa display name/komentar), maksimum 254 karakter.
func Email(s string) (string, string) {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "":
		return "", Required
	case len(v) > 254:
		return "", TooLong
	case !emailRe.MatchString(v), strings.Contains(v, ".."):
		return "", Invalid
	}
	if a, err := mail.ParseAddress(v); err != nil || a.Address != v {
		return "", Invalid
	}
	return v, ""
}

// Phone menormalkan nomor HP ke digit dengan awalan + opsional (8–15 digit). Spasi, titik, tanda hubung,
// dan kurung diabaikan; karakter lain ditolak. Awalan 0 lokal diubah ke +62 (Indonesia).
func Phone(s string) (string, string) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", Required
	}
	var b strings.Builder
	for i, r := range v {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return "", Invalid
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "0") {
		out = "+62" + out[1:]
	}
	digits := strings.TrimPrefix(out, "+")
	if len(digits) < 8 || len(digits) > 15 {
		return "", Invalid
	}
	return out, ""
}

// Password: 10–128 karakter tanpa karakter kontrol, minimal satu huruf dan satu angka, tidak sama
// dengan email. Tidak di-trim/dinormalkan (dipakai apa adanya untuk hash).
func Password(s, email string) string {
	n := utf8.RuneCountInString(s)
	switch {
	case s == "":
		return Required
	case n > 128:
		return TooLong
	case n < 10:
		return TooShort
	case !utf8.ValidString(s), strings.EqualFold(s, email):
		return Weak
	}
	var letter, digit bool
	for _, r := range s {
		switch {
		case unicode.IsControl(r):
			return Invalid
		case unicode.IsLetter(r):
			letter = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if !letter || !digit {
		return Weak
	}
	return ""
}

var nonSlug = regexp.MustCompile("[^a-z0-9]+")

// Slug membuat potongan kode tenant ASCII ([a-z0-9-], maks. max karakter) dari nama bisnis.
func Slug(s string, max int) string {
	// Dekomposisi lalu buang tanda diakritik agar "Kopi Éntah" -> "kopi-entah".
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	v := strings.Trim(nonSlug.ReplaceAllString(b.String(), "-"), "-")
	if len(v) > max {
		v = strings.Trim(v[:max], "-")
	}
	return v
}

// Multiline membersihkan teks bebas (mis. markdown): NFC, CRLF→LF, tab/baris baru diizinkan, karakter kontrol,
// format tak terlihat (zero-width, bidi) dan karakter pengganti ditolak. Teks TIDAK dipadatkan per baris; hanya
// di-trim di ujung. Mengembalikan teks dan kode galat ("" bila valid; Invalid atau TooLong).
func Multiline(raw string, max int) (string, string) {
	if !utf8.ValidString(raw) {
		return "", Invalid
	}
	s := norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r), r == unicode.ReplacementChar:
			return "", Invalid
		}
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", TooLong
	}
	return s, ""
}
