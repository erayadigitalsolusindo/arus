package imaging

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func jpegBytes(img image.Image) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("hasil harus JPEG valid: %v", err)
	}
	return img
}

func TestResizeAndThumbnail(t *testing.T) {
	res, err := ProcessBytes(jpegBytes(solid(3000, 2000, color.RGBA{200, 30, 30, 255})))
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 1200 || res.Height != 800 {
		t.Errorf("ukuran penuh = %dx%d, want 1200x800", res.Width, res.Height)
	}
	if b := decode(t, res.Full).Bounds(); b.Dx() != 1200 || b.Dy() != 800 {
		t.Errorf("isi file penuh = %v", b)
	}
	if b := decode(t, res.Thumb).Bounds(); b.Dx() != 320 || b.Dy() != 213 {
		t.Errorf("thumbnail = %v, want 320x213", b)
	}
	// Gambar kecil tidak diperbesar.
	small, err := ProcessBytes(jpegBytes(solid(100, 80, color.RGBA{0, 0, 255, 255})))
	if err != nil || small.Width != 100 || small.Height != 80 {
		t.Errorf("gambar kecil: %+v %v", small, err)
	}
	if len(res.Full) >= 3000*2000 {
		t.Error("hasil harus jauh lebih kecil dari piksel mentah")
	}
}

func TestTransparencyFlattenedToWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 60, 60)) // sepenuhnya transparan
	res, err := ProcessBytes(pngBytes(src))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decode(t, res.Full).At(30, 30).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("latar transparan harus putih, got %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

// exifJPEG menyisipkan APP1 EXIF berisi tag orientasi ke JPEG.
func exifJPEG(t *testing.T, base []byte, orientation uint16) []byte {
	t.Helper()
	var tiff bytes.Buffer
	tiff.WriteString("II")
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(42))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(1))      // jumlah entri
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x0112)) // tag orientasi
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(3))      // tipe SHORT
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(&tiff, binary.LittleEndian, orientation)
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(0)) // IFD berikutnya
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	out := append([]byte{}, base[:2]...) // SOI
	out = append(out, seg...)
	return append(out, base[2:]...)
}

func TestEXIFOrientationApplied(t *testing.T) {
	landscape := solid(40, 20, color.RGBA{10, 200, 10, 255})
	for orientation, wantW := range map[uint16]int{1: 40, 3: 40, 6: 20, 8: 20} {
		res, err := ProcessBytes(exifJPEG(t, jpegBytes(landscape), orientation))
		if err != nil {
			t.Fatalf("orientasi %d: %v", orientation, err)
		}
		if res.Width != wantW {
			t.Errorf("orientasi %d: lebar = %d, want %d", orientation, res.Width, wantW)
		}
		if bytes.Contains(res.Full, []byte("Exif")) || bytes.Contains(res.Thumb, []byte("Exif")) {
			t.Errorf("orientasi %d: metadata EXIF harus dibuang", orientation)
		}
	}
}

func TestRejects(t *testing.T) {
	g := new(bytes.Buffer)
	_ = gif.Encode(g, solid(4, 4, color.White), nil)
	trunc := jpegBytes(solid(200, 200, color.RGBA{1, 2, 3, 255}))
	trunc = trunc[:len(trunc)/2]

	cases := map[string]struct {
		data []byte
		want error
	}{
		"teks":           {[]byte("bukan gambar"), ErrUnsupported},
		"html":           {[]byte("<html><script>alert(1)</script></html>"), ErrUnsupported},
		"svg":            {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ErrUnsupported},
		"gif":            {g.Bytes(), ErrUnsupported},
		"kosong":         {nil, ErrUnsupported},
		"jpeg terpotong": {trunc, ErrCorrupt},
	}
	for name, c := range cases {
		if _, err := ProcessBytes(c.data); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}

	// Terlalu besar (batas byte) — dibaca terbatas, bukan seluruhnya.
	if _, err := Process(strings.NewReader(strings.Repeat("a", 2048)), 1024); !errors.Is(err, ErrTooLarge) {
		t.Errorf("terlalu besar: %v", err)
	}
}

// Header PNG yang mengaku 30000×30000 harus ditolak dari header, tanpa mengalokasikan memori.
func TestDecompressionBombRejected(t *testing.T) {
	b := pngBytes(solid(1, 1, color.White))
	binary.BigEndian.PutUint32(b[16:20], 30000)
	binary.BigEndian.PutUint32(b[20:24], 30000)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	if _, err := ProcessBytes(b); !errors.Is(err, ErrDimensions) {
		t.Errorf("bom dimensi: err = %v, want ErrDimensions", err)
	}
	// Banyak piksel walau tiap sisi di bawah batas (6000×6000 = 36 MP > 25 MP).
	binary.BigEndian.PutUint32(b[16:20], 6000)
	binary.BigEndian.PutUint32(b[20:24], 6000)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	if _, err := ProcessBytes(b); !errors.Is(err, ErrDimensions) {
		t.Errorf("terlalu banyak piksel: err = %v, want ErrDimensions", err)
	}
}

func TestWebPAccepted(t *testing.T) {
	raw, _ := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	res, err := ProcessBytes(raw)
	if err != nil || res.Width != 1 || res.Height != 1 {
		t.Fatalf("webp: %+v %v", res, err)
	}
	decode(t, res.Full)
}
