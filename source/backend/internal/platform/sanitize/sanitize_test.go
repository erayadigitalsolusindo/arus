package sanitize

import (
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	cases := []struct {
		in, want, code string
	}{
		{"  Toko   Maju  Jaya ", "Toko Maju Jaya", ""},
		{"Kopi Éntah", "Kopi Éntah", ""},
		{"", "", Required},
		{"   ", "", Required},
		{"<script>alert(1)</script>", "", Invalid},
		{"Toko <b>Maju</b>", "", Invalid},
		{"Toko\x00Maju", "", Invalid},
		{"Toko​Maju", "", Invalid},      // zero-width space
		{"Toko‮Maju", "", Invalid},      // bidi override
		{"Toko\nMaju", "Toko Maju", ""}, // whitespace dipadatkan, bukan ditolak
		{"\xff\xfe", "", Invalid},       // UTF-8 tidak valid
		{strings.Repeat("a", 101), "", TooLong},
	}
	for _, c := range cases {
		got, code := Name(c.in, 100)
		if got != c.want || code != c.code {
			t.Errorf("Name(%q) = (%q,%q), want (%q,%q)", c.in, got, code, c.want, c.code)
		}
	}
}

func TestEmail(t *testing.T) {
	cases := []struct {
		in, want, code string
	}{
		{" Budi@Toko.CO.id ", "budi@toko.co.id", ""},
		{"a+tag@example.com", "a+tag@example.com", ""},
		{"", "", Required},
		{"tanpa-at.com", "", Invalid},
		{"a@b", "", Invalid},
		{"a..b@example.com", "", Invalid},
		{"Budi <budi@example.com>", "", Invalid}, // display name
		{"budi@example.com\r\nBcc: x@y.com", "", Invalid},
		{"<script>@example.com", "", Invalid},
		{"budi@exa mple.com", "", Invalid},
		{"büdi@example.com", "", Invalid}, // non-ASCII
		{strings.Repeat("a", 250) + "@x.co", "", TooLong},
	}
	for _, c := range cases {
		got, code := Email(c.in)
		if got != c.want || code != c.code {
			t.Errorf("Email(%q) = (%q,%q), want (%q,%q)", c.in, got, code, c.want, c.code)
		}
	}
}

func TestPhone(t *testing.T) {
	cases := []struct {
		in, want, code string
	}{
		{"0812-3456-7890", "+6281234567890", ""},
		{"+62 812 3456 7890", "+6281234567890", ""},
		{"(021) 555 1234", "+62215551234", ""},
		{"", "", Required},
		{"12345", "", Invalid},
		{"08123abc4567", "", Invalid},
		{"+62812345678901234567", "", Invalid},
		{"0812;DROP TABLE", "", Invalid},
		{"081234567+89", "", Invalid}, // + hanya di awal
	}
	for _, c := range cases {
		got, code := Phone(c.in)
		if got != c.want || code != c.code {
			t.Errorf("Phone(%q) = (%q,%q), want (%q,%q)", c.in, got, code, c.want, c.code)
		}
	}
}

func TestPassword(t *testing.T) {
	cases := []struct {
		in, email, code string
	}{
		{"abcdefgh12", "x@y.com", ""},
		{"Sandi Panjang 99!", "x@y.com", ""},
		{"", "x@y.com", Required},
		{"abc123", "x@y.com", TooShort},
		{"hanyahurufsaja", "x@y.com", Weak},
		{"12345678901", "x@y.com", Weak},
		{"abcdefgh12\x00", "x@y.com", Invalid},
		{strings.Repeat("a1", 65), "x@y.com", TooLong},
	}
	for _, c := range cases {
		if code := Password(c.in, c.email); code != c.code {
			t.Errorf("Password(%q) = %q, want %q", c.in, code, c.code)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Toko Maju Jaya", "toko-maju-jaya"},
		{"Kopi Éntah!!", "kopi-entah"},
		{"../../etc/passwd", "etc-passwd"},
		{"日本語", ""},
		{strings.Repeat("a", 40), strings.Repeat("a", 24)},
	}
	for _, c := range cases {
		if got := Slug(c.in, 24); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
