package platformadmin

import "testing"

func TestImageType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"png", png, "image/png"},
		{"svg bersih", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`), "image/svg+xml"},
		{"svg dengan skrip", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ""},
		{"svg dengan handler", []byte(`<svg onload="x()"></svg>`), ""},
		{"html", []byte(`<html><body>hai</body></html>`), ""},
		{"teks", []byte("bukan gambar"), ""},
	}
	for _, c := range cases {
		if got := imageType(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCleanBrandText(t *testing.T) {
	if v, ok := cleanBrandText("  Kotak Cantik  ", 60); !ok || v != "Kotak Cantik" {
		t.Errorf("got %q %v", v, ok)
	}
	if _, ok := cleanBrandText("a\nb", 60); ok {
		t.Error("karakter kontrol harus ditolak")
	}
	long := make([]rune, 61)
	for i := range long {
		long[i] = 'x'
	}
	if _, ok := cleanBrandText(string(long), 60); ok {
		t.Error("lebih dari batas harus ditolak")
	}
}
