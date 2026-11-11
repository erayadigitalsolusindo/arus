package item

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aciraba/internal/authz"
)

func multipartReq(t *testing.T, path, field, filename, content string) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw, content)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, path, &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

func TestImportHandlerRejectsBadUploads(t *testing.T) {
	h := NewHandler(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cases := []struct {
		name   string
		req    *http.Request
		status int
		code   string
	}{
		{"bukan multipart", httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")), http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
		{"bagian file hilang", multipartReq(t, "/", "lain", "a.csv", "x"), http.StatusUnprocessableEntity, "VALIDATION"},
		{"ekstensi asing", multipartReq(t, "/", "file", "a.pdf", "x"), http.StatusUnprocessableEntity, "IMPORT_FORMAT"},
		{"tanpa baris data", multipartReq(t, "/", "file", "a.csv", "Nama,Satuan\n"), http.StatusUnprocessableEntity, "IMPORT_EMPTY"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ImportPreview(w, c.req)
		var body struct{ Error struct{ Code string } }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != c.status || body.Error.Code != c.code {
			t.Errorf("%s: %d %s, want %d %s", c.name, w.Code, body.Error.Code, c.status, c.code)
		}
	}
}

func TestImportHandlerPreviewAndTemplate(t *testing.T) {
	e := newEnv(t)
	e.master(t, "units", e.a.TenantID, "Pcs", true)
	h := NewHandler(e.svc, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r := multipartReq(t, "/", "file", "b.csv", csvHeader+",,Teh,Pcs,,,,2000,,\n,,Salah,Lusin,,,,,,\n")
	r = r.WithContext(authz.WithActor(r.Context(), e.a))
	w := httptest.NewRecorder()
	h.ImportPreview(w, r)
	var res ImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || w.Code != http.StatusOK || res.Total != 2 || res.Valid != 1 || res.Errors[0].Errors["unit"] != codeNotFound {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ImportTemplate(w, httptest.NewRequest(http.MethodGet, "/?format=csv", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "template-barang.csv") || !strings.Contains(w.Body.String(), "Harga Jual") {
		t.Fatalf("template csv: %d %s", w.Code, w.Header())
	}
}
