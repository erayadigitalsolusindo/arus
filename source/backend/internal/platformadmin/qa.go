package platformadmin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/platform/httpx"
)

// Checklist QA/QC: run (satu putaran uji) + hasil per item katalog (qa_catalog.go).
const (
	ActionQARunCreate = "platform.qa_run_create"
	ActionQARunClose  = "platform.qa_run_close"
	ActionQARunDelete = "platform.qa_run_delete"

	qaMaxName = 120
	qaMaxNote = 1000
)

var (
	ErrQAInvalid = errors.New("data QA tidak valid")
	ErrQAClosed  = errors.New("run QA sudah ditutup")
)

// QARun = satu putaran uji beserta hitungan hasilnya. Total item dari katalog; sisanya belum diuji.
type QARun struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	Pass      int        `json:"pass"`
	Fail      int        `json:"fail"`
	Skip      int        `json:"skip"`
}

// QAResult = hasil satu item pada satu run.
type QAResult struct {
	Key       string    `json:"key"`
	Status    string    `json:"status"` // pass | fail | skip
	Note      string    `json:"note"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// qaTotal = jumlah item di katalog.
func qaTotal() int { return len(qaItemKeys) }

func (s *Service) ListQARuns(ctx context.Context, a Actor, id *uuid.UUID) ([]QARun, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, name, created_by_name, created_at, closed_at, pass_count, fail_count, skip_count
		FROM platform_qa_run_list($1, $2)`, a.ID, id)
	if err != nil {
		if pgCode(err) == "42501" {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	defer rows.Close()
	out := []QARun{}
	for rows.Next() {
		var (
			r                QARun
			closed           pgtype.Timestamptz
			pass, fail, skip int64
		)
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedBy, &r.CreatedAt, &closed, &pass, &fail, &skip); err != nil {
			return nil, err
		}
		if closed.Valid {
			r.ClosedAt = &closed.Time
		}
		r.Pass, r.Fail, r.Skip = int(pass), int(fail), int(skip)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) CreateQARun(ctx context.Context, a Actor, rawName string) (uuid.UUID, error) {
	name := strings.TrimSpace(rawName)
	if name == "" || utf8.RuneCountInString(name) > qaMaxName {
		return uuid.Nil, ErrQAInvalid
	}
	var id uuid.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT platform_qa_run_create($1, $2)`, a.ID, name).Scan(&id); err != nil {
		if pgCode(err) == "42501" {
			return uuid.Nil, ErrInvalidSession
		}
		return uuid.Nil, err
	}
	s.record(ctx, a, ActionQARunCreate, uuid.Nil, "", map[string]any{"run": name})
	return id, nil
}

func (s *Service) CloseQARun(ctx context.Context, a Actor, id uuid.UUID, closed bool) error {
	runs, err := s.ListQARuns(ctx, a, &id)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return ErrNotFound
	}
	if _, err := s.Pool.Exec(ctx, `SELECT platform_qa_run_close($1, $2, $3)`, a.ID, id, closed); err != nil {
		return err
	}
	s.record(ctx, a, ActionQARunClose, uuid.Nil, "", map[string]any{"run": runs[0].Name, "closed": closed})
	return nil
}

func (s *Service) DeleteQARun(ctx context.Context, a Actor, id uuid.UUID) error {
	runs, err := s.ListQARuns(ctx, a, &id)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return ErrNotFound
	}
	if _, err := s.Pool.Exec(ctx, `SELECT platform_qa_run_delete($1, $2)`, a.ID, id); err != nil {
		return err
	}
	s.record(ctx, a, ActionQARunDelete, uuid.Nil, "", map[string]any{"run": runs[0].Name})
	return nil
}

func (s *Service) QAResults(ctx context.Context, a Actor, run uuid.UUID) ([]QAResult, error) {
	runs, err := s.ListQARuns(ctx, a, &run)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT item_key, status, note, updated_by_name, updated_at FROM platform_qa_result_list($1, $2)`, a.ID, run)
	if err != nil {
		if pgCode(err) == "42501" {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	defer rows.Close()
	out := []QAResult{}
	for rows.Next() {
		var r QAResult
		if err := rows.Scan(&r.Key, &r.Status, &r.Note, &r.UpdatedBy, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if _, ok := qaItemKeys[r.Key]; ok { // item yang sudah dihapus dari katalog diabaikan
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

// SetQAResult menyimpan hasil satu item. status: pass | fail | skip | todo (todo = hapus hasil).
func (s *Service) SetQAResult(ctx context.Context, a Actor, run uuid.UUID, key, status, note string) error {
	if _, ok := qaItemKeys[key]; !ok {
		return ErrQAInvalid
	}
	switch status {
	case "pass", "fail", "skip", "todo":
	default:
		return ErrQAInvalid
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > qaMaxNote {
		return ErrQAInvalid
	}
	var res string
	if err := s.Pool.QueryRow(ctx, `SELECT platform_qa_result_set($1, $2, $3, $4, $5)`, a.ID, run, key, status, note).Scan(&res); err != nil {
		if pgCode(err) == "42501" {
			return ErrInvalidSession
		}
		return err
	}
	switch res {
	case "not_found":
		return ErrNotFound
	case "closed":
		return ErrQAClosed
	}
	return nil
}

// ---- HTTP ----

func (h *Handler) qaError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Run QA tidak ditemukan.")
	case errors.Is(err, ErrQAInvalid):
		httpx.ValidationError(w, map[string]string{"body": "INVALID"})
	case errors.Is(err, ErrQAClosed):
		httpx.Error(w, http.StatusConflict, "RUN_CLOSED", "Run sudah ditutup; buka kembali untuk mengubah hasil.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	default:
		h.internal(w, r, msg, err)
	}
}

func (h *Handler) QACatalog(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"apps": qaCatalog, "total": qaTotal()})
}

func (h *Handler) ListQARuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.svc.ListQARuns(r.Context(), actor(r), nil)
	if err != nil {
		h.qaError(w, r, "daftar run QA gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": runs, "total": qaTotal()})
}

type qaRunRequest struct {
	Name string `json:"name"`
}

func (h *Handler) CreateQARun(w http.ResponseWriter, r *http.Request) {
	var req qaRunRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, err := h.svc.CreateQARun(r.Context(), actor(r), req.Name)
	if err != nil {
		h.qaError(w, r, "buat run QA gagal", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

type qaCloseRequest struct {
	Closed bool `json:"closed"`
}

func (h *Handler) PatchQARun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req qaCloseRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.CloseQARun(r.Context(), actor(r), id, req.Closed); err != nil {
		h.qaError(w, r, "ubah run QA gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteQARun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteQARun(r.Context(), actor(r), id); err != nil {
		h.qaError(w, r, "hapus run QA gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) QAResults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.QAResults(r.Context(), actor(r), id)
	if err != nil {
		h.qaError(w, r, "hasil QA gagal dibaca", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": res})
}

type qaResultRequest struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (h *Handler) SetQAResult(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req qaResultRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetQAResult(r.Context(), actor(r), id, req.Key, req.Status, req.Note); err != nil {
		h.qaError(w, r, "simpan hasil QA gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
