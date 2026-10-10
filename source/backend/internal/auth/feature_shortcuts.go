package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

const maxFeatureShortcuts = 8
const maxFeatureIconBytes = 128 << 10

type FeatureShortcut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Icon  string `json:"icon"`
}

type featureShortcutsResponse struct {
	Shortcuts []FeatureShortcut `json:"shortcuts"`
}

func (h *Handler) GetFeatureShortcuts(w http.ResponseWriter, r *http.Request) {
	a := actor(r)
	var raw []byte
	err := db.WithTenant(r.Context(), h.svc.Pool, a.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(), `SELECT shortcuts FROM user_feature_shortcuts WHERE tenant_id = $1 AND user_id = $2`, a.TenantID, a.UserID).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		h.internal(w, r, "gagal membaca pintasan fitur", err)
		return
	}
	shortcuts := []FeatureShortcut{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &shortcuts); err != nil {
			h.internal(w, r, "data pintasan fitur tidak valid", err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, featureShortcutsResponse{Shortcuts: shortcuts})
}

func (h *Handler) PutFeatureShortcuts(w http.ResponseWriter, r *http.Request) {
	var req featureShortcutsResponse
	if !httpx.DecodeJSONLimit(w, r, &req, 2<<20) {
		return
	}
	if fields := validateFeatureShortcuts(req.Shortcuts); fields != nil {
		httpx.ValidationError(w, fields)
		return
	}
	data, err := json.Marshal(req.Shortcuts)
	if err != nil {
		h.internal(w, r, "gagal membaca pintasan fitur", err)
		return
	}
	a := actor(r)
	err = db.WithTenant(r.Context(), h.svc.Pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `
			INSERT INTO user_feature_shortcuts (tenant_id, user_id, shortcuts)
			VALUES ($1, $2, $3::jsonb)
			ON CONFLICT (tenant_id, user_id) DO UPDATE SET shortcuts = EXCLUDED.shortcuts, updated_at = now()`, a.TenantID, a.UserID, string(data))
		return err
	})
	if err != nil {
		h.internal(w, r, "gagal menyimpan pintasan fitur", err)
		return
	}
	httpx.JSON(w, http.StatusOK, featureShortcutsResponse{Shortcuts: req.Shortcuts})
}

func validateFeatureShortcuts(shortcuts []FeatureShortcut) FieldErrors {
	if len(shortcuts) > maxFeatureShortcuts {
		return FieldErrors{"shortcuts": "TOO_MANY"}
	}
	ids := make(map[string]bool, len(shortcuts))
	for i, item := range shortcuts {
		field := fmt.Sprintf("shortcuts.%d", i)
		if item.ID == "" || len(item.ID) > 64 || ids[item.ID] {
			return FieldErrors{field + ".id": "INVALID"}
		}
		ids[item.ID] = true
		if title := strings.TrimSpace(item.Title); title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 40 {
			return FieldErrors{field + ".title": "INVALID"}
		}
		if !validFeatureURL(item.URL) {
			return FieldErrors{field + ".url": "INVALID"}
		}
		if item.Icon != "" && !validFeatureIcon(item.Icon) {
			return FieldErrors{field + ".icon": "INVALID"}
		}
	}
	return nil
}

func validFeatureURL(value string) bool {
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\\\r\n\x00") {
		return false
	}
	if strings.HasPrefix(value, "/") {
		return !strings.HasPrefix(value, "//")
	}
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func validFeatureIcon(value string) bool {
	kind, encoded, ok := strings.Cut(value, ",")
	if !ok || !(kind == "data:image/png;base64" || kind == "data:image/jpeg;base64" || kind == "data:image/webp;base64") {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxFeatureIconBytes {
		return false
	}
	detected := http.DetectContentType(data)
	return strings.HasPrefix(detected, strings.TrimPrefix(strings.TrimSuffix(kind, ";base64"), "data:"))
}
