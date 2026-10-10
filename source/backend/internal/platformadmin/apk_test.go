package platformadmin

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aciraba/internal/platform/storage"
)

func fakeAPK(t *testing.T, withManifest bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	name := "classes.dex"
	if withManifest {
		name = "AndroidManifest.xml"
	}
	w, _ := zw.Create(name)
	_, _ = w.Write([]byte("isi"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newAPKStore(t *testing.T) *apkStore {
	t.Helper()
	l, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &apkStore{l}
}

func TestAPKSaveInfoRemove(t *testing.T) {
	ctx := context.Background()
	s := newAPKStore(t)
	if s.info(ctx).Available {
		t.Fatal("belum ada APK, Available harus false")
	}
	raw := fakeAPK(t, true)
	in, err := s.save(ctx, bytes.NewReader(raw), "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if in.Size != int64(len(raw)) || in.Version != "1.2.0" || len(in.SHA256) != 64 {
		t.Errorf("info tidak sesuai: %+v", in)
	}
	got := s.info(ctx)
	if !got.Available || got.SHA256 != in.SHA256 {
		t.Errorf("info dibaca ulang tidak sama: %+v", got)
	}
	if err := s.remove(ctx); err != nil {
		t.Fatal(err)
	}
	if s.info(ctx).Available {
		t.Error("setelah dihapus Available harus false")
	}
}

func TestAPKRejectsNonAPK(t *testing.T) {
	ctx := context.Background()
	s := newAPKStore(t)
	for name, data := range map[string][]byte{
		"teks biasa":        []byte("bukan zip"),
		"zip tanpa manifes": fakeAPK(t, false),
	} {
		if _, err := s.save(ctx, bytes.NewReader(data), ""); !errors.Is(err, errAPKInvalid) {
			t.Errorf("%s: harus errAPKInvalid, dapat %v", name, err)
		}
	}
	if s.info(ctx).Available {
		t.Error("unggahan ditolak tidak boleh meninggalkan APK aktif")
	}
}

func TestAPKRejectedUploadKeepsPrevious(t *testing.T) {
	ctx := context.Background()
	s := newAPKStore(t)
	if _, err := s.save(ctx, bytes.NewReader(fakeAPK(t, true)), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	before := s.info(ctx)
	if _, err := s.save(ctx, strings.NewReader("sampah"), "2.0.0"); err == nil {
		t.Fatal("harus ditolak")
	}
	if after := s.info(ctx); after.SHA256 != before.SHA256 || after.Version != "1.0.0" {
		t.Errorf("APK lama harus tetap: %+v", after)
	}
}

func TestAPKPublicDownload(t *testing.T) {
	ctx := context.Background()
	s := newAPKStore(t)
	h := &Handler{HandlerDeps: HandlerDeps{APK: s.fs}}

	rec := httptest.NewRecorder()
	h.APKDownload(rec, httptest.NewRequest(http.MethodGet, "/public/mobile/apk", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tanpa APK harus 404, dapat %d", rec.Code)
	}

	raw := fakeAPK(t, true)
	if _, err := s.save(ctx, bytes.NewReader(raw), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.APKDownload(rec, httptest.NewRequest(http.MethodGet, "/public/mobile/apk", nil))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("unduh harus 200 dengan isi sama, dapat %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.android.package-archive" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "arus-kasir.apk") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}
