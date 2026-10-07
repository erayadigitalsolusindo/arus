package audit

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/platform/httpx"
)

// ModuleID = modul izin untuk melihat audit log (authz.Modules).
const ModuleID = "audit_log"

var (
	entityRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	actionRe = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)
)

type Handler struct {
	svc      *Service
	resolver *authz.Resolver
	tokens   *pauth.TokenIssuer
	log      *slog.Logger
}

func NewHandler(svc *Service, resolver *authz.Resolver, tokens *pauth.TokenIssuer, log *slog.Logger) *Handler {
	return &Handler{svc: svc, resolver: resolver, tokens: tokens, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	r.With(httpx.RequireAuth(h.tokens), h.resolver.Authenticate, authz.Require(ModuleID, authz.ActView)).Get("/audit-log", h.List)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.ActorFrom(r.Context())
	q := r.URL.Query()
	bad := map[string]string{}

	f := Filter{Entity: q.Get("entity"), EntityID: q.Get("entity_id"), ActionPrefix: q.Get("action"), Cursor: q.Get("cursor")}
	if f.Entity != "" && !entityRe.MatchString(f.Entity) {
		bad["entity"] = "INVALID"
	}
	if f.ActionPrefix != "" && !actionRe.MatchString(f.ActionPrefix) {
		bad["action"] = "INVALID"
	}
	if len(f.EntityID) > 100 {
		bad["entity_id"] = "TOO_LONG"
	}
	if v := q.Get("actor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			bad["actor_id"] = "INVALID"
		}
		f.ActorID = id
	}
	for _, p := range []struct {
		key string
		dst *time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if v := q.Get(p.key); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				bad[p.key] = "INVALID"
			}
			*p.dst = t
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			bad["limit"] = "INVALID"
		}
		f.Limit = n
	}
	if len(bad) > 0 {
		httpx.ValidationError(w, bad)
		return
	}

	page, err := h.svc.List(r.Context(), actor.TenantID, f)
	switch {
	case errors.Is(err, ErrBadCursor):
		httpx.ValidationError(w, map[string]string{"cursor": "INVALID"})
	case err != nil:
		h.log.Error("audit list gagal", "err", err, "req_id", middleware.GetReqID(r.Context()))
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
	default:
		httpx.JSON(w, http.StatusOK, page)
	}
}
