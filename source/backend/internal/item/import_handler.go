package item

import (
	"errors"
	"io"
	"net/http"
	"time"

	"aciraba/internal/plan"
	"aciraba/internal/platform/httpx"
)

// readImportFile membaca bagian multipart "file" secara streaming (maks maxImportBytes) lalu mengurainya.
func (h *Handler) readImportFile(w http.ResponseWriter, r *http.Request) ([][]string, bool) {
	// ReadTimeout/WriteTimeout server pendek (30 dtk) untuk API JSON; impor besar butuh lebih lama.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(2 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+(64<<10))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus multipart/form-data.")
		return nil, false
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				httpx.Error(w, http.StatusRequestEntityTooLarge, "IMPORT_TOO_LARGE", "Berkas terlalu besar (maks 5 MB).")
				return nil, false
			}
			httpx.ValidationError(w, map[string]string{"file": "REQUIRED"})
			return nil, false
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, maxImportBytes+1))
		if err != nil || len(data) > maxImportBytes {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "IMPORT_TOO_LARGE", "Berkas terlalu besar (maks 5 MB).")
			return nil, false
		}
		rows, err := ParseImport(part.FileName(), data)
		if err != nil {
			h.importFail(w, r, err)
			return nil, false
		}
		return rows, true
	}
}

func (h *Handler) importFail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrImportFormat):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMPORT_FORMAT", "Berkas harus .xlsx atau .csv yang valid.")
	case errors.Is(err, ErrImportEmpty):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMPORT_EMPTY", "Berkas tidak berisi baris data.")
	case errors.Is(err, ErrImportTooMany):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMPORT_TOO_MANY", "Jumlah baris melebihi 3.000 per berkas; bagi berkas menjadi beberapa bagian.")
	case errors.Is(err, ErrImportNoHeader):
		httpx.Error(w, http.StatusUnprocessableEntity, "IMPORT_HEADER", "Kolom Nama dan Satuan wajib ada; gunakan template unduhan.")
	case errors.Is(err, plan.ErrItemLimit):
		httpx.Error(w, http.StatusPaymentRequired, "PLAN_ITEM_LIMIT_REACHED", "Batas jumlah barang paket tercapai.")
	default:
		h.fail(w, r, err)
	}
}

// ImportTemplate: GET /items/import/template?format=xlsx|csv
func (h *Handler) ImportTemplate(w http.ResponseWriter, r *http.Request) {
	format := "xlsx"
	ctype := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if r.URL.Query().Get("format") == "csv" {
		format, ctype = "csv", "text/csv; charset=utf-8"
	}
	data, err := ImportTemplate(format)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="template-barang.`+format+`"`)
	_, _ = w.Write(data)
}

// ImportPreview: POST /items/import/preview (multipart "file") → ImportResult tanpa menyimpan.
func (h *Handler) ImportPreview(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.readImportFile(w, r)
	if !ok {
		return
	}
	res, err := h.svc.ImportPreview(r.Context(), actor(r), rows)
	if err != nil {
		h.importFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// ImportCommit: POST /items/import/commit (multipart "file") → ImportResult; baris valid disimpan.
func (h *Handler) ImportCommit(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.readImportFile(w, r)
	if !ok {
		return
	}
	res, err := h.svc.ImportCommit(r.Context(), actor(r), rows)
	if err != nil {
		h.importFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
