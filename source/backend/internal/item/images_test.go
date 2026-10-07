package item

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func testJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func testPNG(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func readAll(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fileCount(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func newItem(t *testing.T, e *env, name string) (*Item, uuid.UUID) {
	t.Helper()
	unit := e.master(t, "units", e.a.TenantID, "Pcs-"+name, true)
	it, err := e.svc.Create(context.Background(), e.a, Input{Name: name, UnitID: unit.String()})
	if err != nil {
		t.Fatal(err)
	}
	return it, unit
}

func TestImageLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it, _ := newItem(t, e, "Kopi")

	// Gambar pertama otomatis utama; hasil JPG terkompres 1200 px; berkas penuh + thumbnail tersimpan.
	img1, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(testJPEG(3000, 2000)))
	if err != nil || !img1.IsMain || img1.Position != 1 || img1.Width != 1200 || img1.Height != 800 || img1.Bytes <= 0 || img1.ThumbBytes <= 0 {
		t.Fatalf("gambar pertama: %+v %v", img1, err)
	}
	full, _, err := e.svc.OpenImage(ctx, e.a, it.ID, img1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jpeg.DecodeConfig(bytes.NewReader(readAll(t, full))); err != nil || got.Width != 1200 {
		t.Errorf("berkas penuh: %+v %v", got, err)
	}
	thumb, _, err := e.svc.OpenImage(ctx, e.a, it.ID, img1.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jpeg.DecodeConfig(bytes.NewReader(readAll(t, thumb))); err != nil || got.Width != 320 {
		t.Errorf("thumbnail: %+v %v", got, err)
	}

	// PNG transparan diterima dan diubah menjadi JPG; gambar kedua bukan utama.
	img2, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(testPNG(50, 50)))
	if err != nil || img2.IsMain || img2.Position != 2 {
		t.Fatalf("gambar kedua: %+v %v", img2, err)
	}

	// Ganti gambar utama: tepat satu yang utama.
	if err := e.svc.SetMainImage(ctx, e.a, it.ID, img2.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, e.a, it.ID)
	mains := 0
	for _, im := range got.Images {
		if im.IsMain {
			mains++
			if im.ID != img2.ID {
				t.Errorf("gambar utama = %s, want %s", im.ID, img2.ID)
			}
		}
	}
	if mains != 1 || len(got.Images) != 2 || got.Images[0].ID != img2.ID {
		t.Errorf("urutan/utama: %+v", got.Images)
	}
	if rows, _, _ := e.svc.List(ctx, e.a, ListParams{}); len(rows) != 1 || rows[0].MainImageID == nil || *rows[0].MainImageID != img2.ID {
		t.Errorf("daftar harus memuat id gambar utama: %+v", rows)
	}

	// Hapus gambar utama: yang lain dipromosikan; berkas dihapus dari storage.
	if err := e.svc.DeleteImage(ctx, e.a, it.ID, img2.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.Get(ctx, e.a, it.ID)
	if len(got.Images) != 1 || got.Images[0].ID != img1.ID || !got.Images[0].IsMain {
		t.Errorf("setelah hapus utama: %+v", got.Images)
	}
	if _, _, err := e.svc.OpenImage(ctx, e.a, it.ID, img2.ID, false); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("buka gambar terhapus: %v", err)
	}
	if n := fileCount(t, e.storeRoot); n != 2 { // img1: penuh + thumbnail
		t.Errorf("berkas di storage = %d, want 2", n)
	}
	if e.auditCount(t, e.a.TenantID, "item.image_add") != 2 || e.auditCount(t, e.a.TenantID, "item.image_main") != 1 || e.auditCount(t, e.a.TenantID, "item.image_delete") != 1 {
		t.Error("audit gambar tidak lengkap")
	}
	if err := e.svc.DeleteImage(ctx, e.a, it.ID, uuid.New()); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("hapus gambar tak ada: %v", err)
	}
}

func TestImageLimitAndRejections(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it, _ := newItem(t, e, "Banyak")
	for i := 0; i < MaxImages; i++ {
		if _, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(testJPEG(64, 64))); err != nil {
			t.Fatalf("gambar %d: %v", i+1, err)
		}
	}
	before := fileCount(t, e.storeRoot)
	if _, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(testJPEG(64, 64))); !errors.Is(err, ErrImageLimit) {
		t.Errorf("gambar ke-%d: %v, want ErrImageLimit", MaxImages+1, err)
	}
	if fileCount(t, e.storeRoot) != before {
		t.Error("unggahan yang ditolak tidak boleh meninggalkan berkas")
	}

	it2, _ := newItem(t, e, "Tolak")
	for name, c := range map[string]struct {
		data []byte
		want error
	}{
		"teks":          {[]byte("bukan gambar"), ErrImageUnsupported},
		"svg":           {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ErrImageUnsupported},
		"terpotong":     {testJPEG(200, 200)[:300], ErrImageCorrupt},
		"terlalu besar": {append(testJPEG(10, 10), bytes.Repeat([]byte{0}, MaxUploadBytes)...), ErrImageTooLarge},
	} {
		if _, err := e.svc.AddImage(ctx, e.a, it2.ID, bytes.NewReader(c.data)); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if got, _ := e.svc.Get(ctx, e.a, it2.ID); len(got.Images) != 0 {
		t.Errorf("tidak boleh ada gambar tersimpan setelah ditolak: %+v", got.Images)
	}
	if _, err := e.svc.AddImage(ctx, e.a, uuid.New(), bytes.NewReader(testJPEG(10, 10))); !errors.Is(err, ErrNotFound) {
		t.Errorf("item tak ada: %v", err)
	}
}

func TestImageTenantIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it, _ := newItem(t, e, "Milik A")
	img, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(testJPEG(64, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.OpenImage(ctx, e.b, it.ID, img.ID, false); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("baca gambar lintas tenant: %v", err)
	}
	if err := e.svc.DeleteImage(ctx, e.b, it.ID, img.ID); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("hapus gambar lintas tenant: %v", err)
	}
	if err := e.svc.SetMainImage(ctx, e.b, it.ID, img.ID); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("ubah utama lintas tenant: %v", err)
	}
	if _, err := e.svc.AddImage(ctx, e.b, it.ID, bytes.NewReader(testJPEG(64, 64))); !errors.Is(err, ErrNotFound) {
		t.Errorf("unggah ke item tenant lain: %v", err)
	}
	// Gambar item lain di tenant yang sama tidak bisa dibaca lewat id item yang salah.
	other, _ := newItem(t, e, "Lain")
	if _, _, err := e.svc.OpenImage(ctx, e.a, other.ID, img.ID, false); !errors.Is(err, ErrImageNotFound) {
		t.Errorf("gambar milik item lain: %v", err)
	}
}

func TestConcurrentUploadsRespectLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	it, _ := newItem(t, e, "Paralel")
	data := testJPEG(64, 64)
	var wg sync.WaitGroup
	results := make(chan error, 9)
	for range 9 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.AddImage(ctx, e.a, it.ID, bytes.NewReader(data))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, limited := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrImageLimit):
			limited++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != MaxImages || limited != 9-MaxImages {
		t.Errorf("berhasil=%d ditolak=%d, want %d dan %d", ok, limited, MaxImages, 9-MaxImages)
	}
	if n := fileCount(t, e.storeRoot); n != MaxImages*2 {
		t.Errorf("berkas = %d, want %d (unggahan yang kalah balapan harus membersihkan berkasnya)", n, MaxImages*2)
	}
	got, _ := e.svc.Get(ctx, e.a, it.ID)
	mains := 0
	for _, im := range got.Images {
		if im.IsMain {
			mains++
		}
	}
	if len(got.Images) != MaxImages || mains != 1 {
		t.Errorf("gambar=%d utama=%d, want %d dan 1", len(got.Images), mains, MaxImages)
	}
}

func TestImagesDisabledWithoutStorage(t *testing.T) {
	e := newEnv(t)
	svc := NewService(e.svc.pool, nil)
	it, _ := newItem(t, e, "Tanpa storage")
	if _, err := svc.AddImage(context.Background(), e.a, it.ID, strings.NewReader("x")); !errors.Is(err, ErrNoStorage) {
		t.Errorf("tanpa storage: %v", err)
	}
}
