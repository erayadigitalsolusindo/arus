// Package imaging memproses gambar unggahan pengguna menjadi JPG yang hemat ruang dan aman disimpan:
//   - isi file divalidasi (bukan ekstensi/Content-Type klien); hanya JPEG, PNG, dan WebP yang diterima;
//   - dimensi dicek dari header SEBELUM didecode (pencegah "decompression bomb");
//   - orientasi EXIF diterapkan, lalu semua metadata (EXIF/GPS) hilang karena gambar di-encode ulang;
//   - latar transparan diratakan ke putih;
//   - hasilnya selalu JPEG: satu versi penuh (sisi terpanjang ≤ FullMax) dan satu thumbnail (≤ ThumbMax).
package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"

	xdraw "golang.org/x/image/draw"
	// Decoder untuk image.Decode/DecodeConfig. image/jpeg diimpor di atas; PNG dan WebP didaftarkan di sini. Jangan
	// dihapus: test mengimpor image/png sendiri sehingga decoder yang hilang tidak akan ketahuan di test.
	_ "golang.org/x/image/webp"
	_ "image/png"
)

const (
	FullMax      = 1200 // sisi terpanjang versi penuh (px); gambar lebih kecil tidak diperbesar
	ThumbMax     = 320  // sisi terpanjang thumbnail (px)
	FullQuality  = 80
	ThumbQuality = 75

	// MaxPixels membatasi memori decode (RGBA = 4 byte/piksel → ~100 MB pada batas ini).
	MaxPixels = 25_000_000
	MaxSide   = 20_000
)

var (
	ErrTooLarge    = errors.New("file terlalu besar")
	ErrUnsupported = errors.New("format gambar tidak didukung")
	ErrDimensions  = errors.New("dimensi gambar terlalu besar")
	ErrCorrupt     = errors.New("gambar rusak")
)

// Result = hasil pemrosesan. Width/Height = ukuran versi penuh.
type Result struct {
	Full          []byte
	Thumb         []byte
	Width, Height int
}

// slots membatasi pemrosesan gambar (CPU + memori decode besar) menjadi dua sekaligus untuk SELURUH aplikasi,
// apa pun modul pemanggilnya (item, member, ...).
var slots = make(chan struct{}, 2)

// ProcessLimited = Process dengan antrean global: menunggu giliran (atau batal bila ctx selesai).
func ProcessLimited(ctx context.Context, r io.Reader, maxBytes int64) (*Result, error) {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-slots }()
	return Process(r, maxBytes)
}

// Process membaca maksimal maxBytes dari r. Mengembalikan ErrTooLarge bila lebih besar.
func Process(r io.Reader, maxBytes int64) (*Result, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("baca gambar: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, ErrTooLarge
	}
	return ProcessBytes(raw)
}

func ProcessBytes(raw []byte) (*Result, error) {
	switch http.DetectContentType(raw) {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return nil, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrCorrupt
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxSide || cfg.Height > MaxSide || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrDimensions
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrCorrupt
	}
	src = orient(src, jpegOrientation(raw))
	src = flatten(src)

	full := fit(src, FullMax)
	thumb := fit(full, ThumbMax)
	fb, err := encode(full, FullQuality)
	if err != nil {
		return nil, err
	}
	tb, err := encode(thumb, ThumbQuality)
	if err != nil {
		return nil, err
	}
	b := full.Bounds()
	return &Result{Full: fb, Thumb: tb, Width: b.Dx(), Height: b.Dy()}, nil
}

func encode(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// flatten meratakan transparansi ke latar putih dan mengembalikan gambar RGBA opak.
func flatten(src image.Image) image.Image {
	if o, ok := src.(interface{ Opaque() bool }); ok && o.Opaque() {
		return src // sudah opak (mis. JPEG): hindari salinan RGBA penuh
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

// fit memperkecil gambar sehingga sisi terpanjang ≤ max (rasio tetap). Tidak pernah memperbesar.
func fit(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	var nw, nh int
	if w >= h {
		nw, nh = max, max*h/w
	} else {
		nw, nh = max*w/h, max
	}
	nw, nh = max1(nw), max1(nh)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// jpegOrientation membaca tag orientasi EXIF (1–8) dari JPEG; 1 (tanpa transformasi) bila tidak ada/rusak.
// Decoder JPEG Go mengabaikan EXIF, sehingga foto ponsel yang dipotret miring akan tampil terputar tanpa ini.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xDA || marker == 0xD9 { // mulai data gambar / akhir
			return 1
		}
		size := int(b[i+2])<<8 | int(b[i+3])
		if size < 2 || i+2+size > len(b) {
			return 1
		}
		if marker == 0xE1 { // APP1
			if o := exifOrientation(b[i+4 : i+2+size]); o != 0 {
				return o
			}
		}
		i += 2 + size
	}
	return 1
}

func exifOrientation(seg []byte) int {
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}
	t := seg[6:]
	var u16 func([]byte) int
	var u32 func([]byte) int
	switch string(t[:2]) {
	case "II":
		u16 = func(b []byte) int { return int(b[0]) | int(b[1])<<8 }
		u32 = func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24 }
	case "MM":
		u16 = func(b []byte) int { return int(b[0])<<8 | int(b[1]) }
		u32 = func(b []byte) int { return int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3]) }
	default:
		return 0
	}
	if u16(t[2:4]) != 42 || len(t) < 8 {
		return 0
	}
	off := u32(t[4:8])
	if off < 8 || off+2 > len(t) {
		return 0
	}
	n := u16(t[off : off+2])
	for e := 0; e < n; e++ {
		p := off + 2 + e*12
		if p+12 > len(t) {
			return 0
		}
		if u16(t[p:p+2]) == 0x0112 { // Orientation
			if v := u16(t[p+8 : p+10]); v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// orient menerapkan orientasi EXIF (nilai standar 1–8) sehingga gambar tampil tegak.
func orient(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var dst *image.RGBA
	if o >= 5 { // transposisi: lebar ↔ tinggi
		dst = image.NewRGBA(image.Rect(0, 0, h, w))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2: // cermin horizontal
				dx, dy = w-1-x, y
			case 3: // putar 180°
				dx, dy = w-1-x, h-1-y
			case 4: // cermin vertikal
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // putar 90° searah jarum jam
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // putar 90° berlawanan jarum jam
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
