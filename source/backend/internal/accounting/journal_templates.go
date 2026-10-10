package accounting

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
)

const maxTemplates = 100

var (
	ErrTemplateNameTaken = errors.New("nama template sudah dipakai")
	ErrTemplateLimit     = errors.New("jumlah template mencapai batas")
)

type TemplateLineInput struct {
	AccountID uuid.UUID `json:"account_id"`
	Side      string    `json:"side"` // debit | credit
	Memo      string    `json:"memo"`
}

type TemplateInput struct {
	Name      string              `json:"name"`
	Type      string              `json:"type"`
	Narration string              `json:"narration"`
	Lines     []TemplateLineInput `json:"lines"`
}

type TemplateLine struct {
	AccountID   uuid.UUID `json:"account_id"`
	AccountCode string    `json:"account_code"`
	AccountName string    `json:"account_name"`
	Active      bool      `json:"active"`
	Side        string    `json:"side"`
	Memo        string    `json:"memo"`
}

type JournalTemplate struct {
	ID        uuid.UUID      `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Narration string         `json:"narration"`
	Lines     []TemplateLine `json:"lines"`
}

func (in TemplateInput) validate() (TemplateInput, FieldErrors) {
	fe := FieldErrors{}
	out := TemplateInput{Type: in.Type}
	var c string
	if out.Name, c = cleanText(in.Name, 80, true); c != "" {
		fe["name"] = c
	}
	if out.Narration, c = cleanText(in.Narration, 500, false); c != "" {
		fe["narration"] = c
	}
	if !manualTypes[in.Type] {
		fe["type"] = sanitize.Invalid
	}
	if len(in.Lines) < 2 || len(in.Lines) > maxLines {
		fe["lines"] = sanitize.Invalid
	}
	var hasD, hasC bool
	for _, l := range in.Lines {
		m, c := cleanText(l.Memo, 200, false)
		if c != "" || (l.Side != "debit" && l.Side != "credit") || l.AccountID == uuid.Nil {
			fe["lines"] = sanitize.Invalid
			break
		}
		hasD, hasC = hasD || l.Side == "debit", hasC || l.Side == "credit"
		out.Lines = append(out.Lines, TemplateLineInput{AccountID: l.AccountID, Side: l.Side, Memo: m})
	}
	if _, bad := fe["lines"]; !bad && (!hasD || !hasC) {
		fe["lines"] = sanitize.Invalid
	}
	return out, fe
}

func (s *Service) ListTemplates(ctx context.Context, a authz.Actor) ([]JournalTemplate, error) {
	out := []JournalTemplate{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT t.id, t.name, t.type, t.narration, l.account_id, ac.code, ac.name, ac.active, l.side, l.memo
			FROM journal_templates t
			JOIN journal_template_lines l ON l.tenant_id = t.tenant_id AND l.template_id = t.id
			JOIN accounts ac ON ac.tenant_id = l.tenant_id AND ac.id = l.account_id
			WHERE t.tenant_id = $1
			ORDER BY lower(t.name), t.id, l.line_no`, a.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var name, typ, narr string
			var l TemplateLine
			if err := rows.Scan(&id, &name, &typ, &narr, &l.AccountID, &l.AccountCode, &l.AccountName, &l.Active, &l.Side, &l.Memo); err != nil {
				return err
			}
			if n := len(out); n == 0 || out[n-1].ID != id {
				out = append(out, JournalTemplate{ID: id, Name: name, Type: typ, Narration: narr})
			}
			out[len(out)-1].Lines = append(out[len(out)-1].Lines, l)
		}
		return rows.Err()
	})
	return out, err
}

// SaveTemplate membuat (id == Nil) atau mengganti seluruh isi template.
func (s *Service) SaveTemplate(ctx context.Context, a authz.Actor, id uuid.UUID, raw TemplateInput) (JournalTemplate, error) {
	in, fe := raw.validate()
	if len(fe) > 0 {
		return JournalTemplate{}, fe
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		distinct := map[uuid.UUID]bool{}
		ids := make([]uuid.UUID, 0, len(in.Lines))
		for _, l := range in.Lines {
			if !distinct[l.AccountID] {
				distinct[l.AccountID] = true
				ids = append(ids, l.AccountID)
			}
		}
		var ok int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE tenant_id = $1 AND id = ANY($2) AND kind = 'ledger' AND active`,
			a.TenantID, ids).Scan(&ok); err != nil {
			return err
		}
		if ok != len(ids) {
			return ErrAccountInvalid
		}
		if id == uuid.Nil {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_templates WHERE tenant_id = $1`, a.TenantID).Scan(&n); err != nil {
				return err
			}
			if n >= maxTemplates {
				return ErrTemplateLimit
			}
			err := tx.QueryRow(ctx, `INSERT INTO journal_templates (tenant_id, name, type, narration, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				a.TenantID, in.Name, in.Type, in.Narration, a.UserID).Scan(&id)
			if pgCode(err) == "23505" {
				return ErrTemplateNameTaken
			}
			if err != nil {
				return err
			}
		} else {
			tag, err := tx.Exec(ctx, `UPDATE journal_templates SET name = $3, type = $4, narration = $5 WHERE tenant_id = $1 AND id = $2`,
				a.TenantID, id, in.Name, in.Type, in.Narration)
			if pgCode(err) == "23505" {
				return ErrTemplateNameTaken
			}
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrNotFound
			}
			if _, err := tx.Exec(ctx, `DELETE FROM journal_template_lines WHERE tenant_id = $1 AND template_id = $2`, a.TenantID, id); err != nil {
				return err
			}
		}
		for i, l := range in.Lines {
			if _, err := tx.Exec(ctx, `INSERT INTO journal_template_lines (tenant_id, template_id, line_no, account_id, side, memo) VALUES ($1, $2, $3, $4, $5, $6)`,
				a.TenantID, id, i+1, l.AccountID, l.Side, l.Memo); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return JournalTemplate{}, err
	}
	all, err := s.ListTemplates(ctx, a)
	if err != nil {
		return JournalTemplate{}, err
	}
	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}
	return JournalTemplate{}, ErrNotFound
}

func (s *Service) DeleteTemplate(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM journal_templates WHERE tenant_id = $1 AND id = $2`, a.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListTemplates(r.Context(), actor(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": res})
}

func (h *Handler) saveTemplate(w http.ResponseWriter, r *http.Request, id uuid.UUID, status int) {
	var in TemplateInput
	if !httpx.DecodeJSONLimit(w, r, &in, 64<<10) {
		return
	}
	res, err := h.svc.SaveTemplate(r.Context(), actor(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, status, res)
}

func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	h.saveTemplate(w, r, uuid.Nil, http.StatusCreated)
}

func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		h.saveTemplate(w, r, id, http.StatusOK)
	}
}

func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), actor(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
