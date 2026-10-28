package platformadmin

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/plan"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

const (
	ActionPlanSave      = "platform.plan_save"
	ActionTenantPlanSet = "platform.tenant_plan"
)

var (
	ErrPlanCodeTaken = errors.New("kode paket sudah terpakai")
	ErrPlanInvalid   = errors.New("data paket tidak valid")
	planCodePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)
)

// PlanRow = satu paket (batas nil = tanpa batas; duration_days nil = tanpa kedaluwarsa).
type PlanRow struct {
	ID           uuid.UUID `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Price        string    `json:"price"`
	DurationDays *int      `json:"duration_days"`
	MaxTxPerDay  *int      `json:"max_tx_per_day"`
	MaxUsers     *int      `json:"max_users"`
	IsDefault    bool      `json:"is_default"`
	SelfService  bool      `json:"self_service"`
	Active       bool      `json:"active"`
	SortOrder    int       `json:"sort_order"`
	Modules      []string  `json:"modules"` // nil = semua modul
	Highlights   []string  `json:"highlights"` // poin keunggulan bebas untuk kartu paket
}

func int4(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func pgInt4(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}

func (s *Service) ListPlans(ctx context.Context) ([]PlanRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, code, name, description, price, duration_days, max_tx_per_day, max_users,
		is_default, self_service, active, sort_order, modules, highlights FROM plans ORDER BY sort_order, price, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanRow{}
	for rows.Next() {
		var (
			p              PlanRow
			price          decimal.Decimal
			dur, mtx, musr pgtype.Int4
			sort           int32
		)
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &price, &dur, &mtx, &musr, &p.IsDefault, &p.SelfService, &p.Active, &sort, &p.Modules, &p.Highlights); err != nil {
			return nil, err
		}
		p.Price, p.DurationDays, p.MaxTxPerDay, p.MaxUsers, p.SortOrder = price.StringFixed(0), int4(dur), int4(mtx), int4(musr), int(sort)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SavePlanInput: ID nil = paket baru. Code hanya dipakai saat membuat.
type SavePlanInput struct {
	ID                                  *uuid.UUID
	Code, Name, Description             string
	Price                               decimal.Decimal
	DurationDays, MaxTxPerDay, MaxUsers *int
	SelfService, Active                 bool
	SortOrder                           int
	Modules                             []string // nil = semua modul
	Highlights                          []string // poin keunggulan, satu per elemen
}

func (in SavePlanInput) validate() map[string]string {
	f := map[string]string{}
	if in.ID == nil && !planCodePattern.MatchString(in.Code) {
		f["code"] = "INVALID"
	}
	if n := len([]rune(in.Name)); n < 1 || n > 60 {
		f["name"] = "INVALID"
	}
	if len([]rune(in.Description)) > 300 {
		f["description"] = "INVALID"
	}
	if in.Price.IsNegative() || !in.Price.Equal(in.Price.Truncate(0)) {
		f["price"] = "INVALID"
	}
	for k, v := range map[string]*int{"duration_days": in.DurationDays, "max_tx_per_day": in.MaxTxPerDay, "max_users": in.MaxUsers} {
		if v != nil && (*v < 1 || *v > 1_000_000 || (k == "duration_days" && *v > 3650)) {
			f[k] = "INVALID"
		}
	}
	if in.SelfService && (!in.Price.IsPositive() || in.DurationDays == nil || !in.Active) {
		f["self_service"] = "INVALID" // dijual sendiri: wajib berharga, berdurasi, dan aktif
	}
	if in.Modules != nil && (len(in.Modules) > 200 || !authz.ValidModuleIDs(in.Modules)) {
		f["modules"] = "INVALID"
	}
	if len(in.Highlights) > 12 {
		f["highlights"] = "INVALID"
	}
	for _, h := range in.Highlights {
		if n := len([]rune(h)); n < 1 || n > 120 {
			f["highlights"] = "INVALID"
		}
	}
	if in.SortOrder < 0 || in.SortOrder > 10000 {
		f["sort_order"] = "INVALID"
	}
	return f
}

type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "validasi gagal" }

func (s *Service) SavePlan(ctx context.Context, a Actor, in SavePlanInput) (uuid.UUID, error) {
	if f := in.validate(); len(f) > 0 {
		return uuid.Nil, FieldErrors(f)
	}
	var id pgtype.UUID
	if in.ID != nil {
		id = pgtype.UUID{Bytes: *in.ID, Valid: true}
	}
	var out pgtype.UUID
	err := s.Pool.QueryRow(ctx, `SELECT platform_save_plan($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		a.ID, id, in.Code, in.Name, in.Description, in.Price, pgInt4(in.DurationDays), pgInt4(in.MaxTxPerDay), pgInt4(in.MaxUsers),
		in.SelfService, in.Active, int32(in.SortOrder), in.Modules, in.Highlights).Scan(&out)
	switch {
	case pgCode(err) == "42501":
		return uuid.Nil, ErrInvalidSession
	case pgCode(err) == "23505":
		return uuid.Nil, ErrPlanCodeTaken
	case pgCode(err) == "23514": // CHECK (mis. paket default tidak boleh dijual sendiri)
		return uuid.Nil, ErrPlanInvalid
	case err != nil:
		return uuid.Nil, err
	case !out.Valid:
		return uuid.Nil, ErrNotFound
	}
	pid := uuid.UUID(out.Bytes)
	s.record(ctx, a, ActionPlanSave, uuid.Nil, "", map[string]any{"plan_id": pid.String(), "code": in.Code, "name": in.Name, "price": in.Price.StringFixed(0),
		"duration_days": in.DurationDays, "max_tx_per_day": in.MaxTxPerDay, "max_users": in.MaxUsers, "self_service": in.SelfService, "active": in.Active, "modules": in.Modules, "highlights": in.Highlights, "create": in.ID == nil})
	return pid, nil
}

// TenantPlan = langganan tenant untuk panel platform: paket efektif sekarang + pengaturan manual yang tersimpan.
type TenantPlan struct {
	Effective plan.Effective `json:"effective"`
	// Baris langganan tersimpan (nil = tenant memakai paket default). Bisa sudah kedaluwarsa (Expired=true).
	Subscription *TenantSubscription `json:"subscription"`
	Usage        plan.Usage          `json:"usage"`
}

type TenantSubscription struct {
	PlanID      uuid.UUID  `json:"plan_id"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Expired     bool       `json:"expired"`
	OverrideTx  *int       `json:"override_max_tx_per_day"`
	OverrideUsr *int       `json:"override_max_users"`
	Note        string     `json:"note"`
}

func (s *Service) GetTenantPlan(ctx context.Context, tenantID uuid.UUID) (*TenantPlan, error) {
	var out TenantPlan
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1)`, tenantID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return pgx.ErrNoRows
		}
		var err error
		if out.Effective, err = plan.Get(ctx, tx, tenantID); err != nil {
			return err
		}
		var (
			sub    TenantSubscription
			exp    pgtype.Timestamptz
			otx    pgtype.Int4
			ousr   pgtype.Int4
			expird bool
		)
		err = tx.QueryRow(ctx, `SELECT plan_id, expires_at, override_max_tx_per_day, override_max_users, note, (expires_at IS NOT NULL AND expires_at <= now())
			FROM tenant_subscriptions WHERE tenant_id = $1`, tenantID).Scan(&sub.PlanID, &exp, &otx, &ousr, &sub.Note, &expird)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			if exp.Valid {
				t := exp.Time
				sub.ExpiresAt = &t
			}
			sub.OverrideTx, sub.OverrideUsr, sub.Expired = int4(otx), int4(ousr), expird
			out.Subscription = &sub
		}
		// Hari ini menurut zona waktu outlet aktif pertama.
		var day pgtype.Date
		if err := tx.QueryRow(ctx, `SELECT (now() AT TIME ZONE coalesce((SELECT timezone FROM outlets WHERE tenant_id = $1 AND active ORDER BY created_at LIMIT 1), 'Asia/Jakarta'))::date`, tenantID).Scan(&day); err != nil {
			return err
		}
		out.Usage, err = plan.UsageFor(ctx, tx, tenantID, day)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetTenantPlanInput: PlanID nil = kembali ke paket default (hapus langganan manual).
type SetTenantPlanInput struct {
	PlanID               *uuid.UUID
	ExpiresAt            *time.Time
	OverrideTx, OverrideU *int
	Note                 string
}

func (s *Service) SetTenantPlan(ctx context.Context, a Actor, tenantID uuid.UUID, in SetTenantPlanInput) error {
	f := map[string]string{}
	for k, v := range map[string]*int{"override_max_tx_per_day": in.OverrideTx, "override_max_users": in.OverrideU} {
		if v != nil && (*v < 1 || *v > 1_000_000) {
			f[k] = "INVALID"
		}
	}
	if len([]rune(in.Note)) > 300 {
		f["note"] = "INVALID"
	}
	if in.PlanID == nil && (in.ExpiresAt != nil || in.OverrideTx != nil || in.OverrideU != nil) {
		f["plan_id"] = "REQUIRED"
	}
	if len(f) > 0 {
		return FieldErrors(f)
	}
	var pid pgtype.UUID
	if in.PlanID != nil {
		pid = pgtype.UUID{Bytes: *in.PlanID, Valid: true}
	}
	var exp pgtype.Timestamptz
	if in.ExpiresAt != nil {
		exp = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}
	var name string
	err := db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, tenantID).Scan(&name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT platform_set_tenant_plan($1, $2, $3, $4, $5, $6, $7)`,
			a.ID, tenantID, pid, exp, pgInt4(in.OverrideTx), pgInt4(in.OverrideU), in.Note); err != nil {
			return err
		}
		details := map[string]any{"plan_id": in.PlanID, "expires_at": in.ExpiresAt, "override_max_tx_per_day": in.OverrideTx, "override_max_users": in.OverrideU, "note": in.Note}
		if err := s.recordTx(ctx, tx, a, ActionTenantPlanSet, tenantID, name, details); err != nil {
			return err
		}
		det := map[string]any{"admin_id": a.ID.String()}
		for k, v := range details {
			det[k] = v
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: audit.ActionPlatformTenantPlan, Entity: audit.EntityTenant, EntityID: tenantID.String(), Details: det,
		})
	})
	if err == nil && s.Perms != nil {
		s.Perms.InvalidateTenant(tenantID) // modul paket baru berlaku segera, bukan menunggu TTL cache izin
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case pgCode(err) == "42501":
		return ErrInvalidSession
	case pgCode(err) == "23503": // paket tidak ada
		return FieldErrors{"plan_id": "INVALID"}
	}
	return err
}

// ---- HTTP ----

func (h *Handler) planErr(w http.ResponseWriter, r *http.Request, msg string, err error) {
	var fe FieldErrors
	switch {
	case errors.As(err, &fe):
		httpx.ValidationError(w, fe)
	case errors.Is(err, ErrPlanCodeTaken):
		httpx.Error(w, http.StatusConflict, "CODE_TAKEN", "Kode paket sudah terpakai.")
	case errors.Is(err, ErrPlanInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "PLAN_INVALID", "Pengaturan paket tidak valid (paket default tidak boleh dijual sendiri).")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	default:
		h.internal(w, r, msg, err)
	}
}

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListPlans(r.Context())
	if err != nil {
		h.planErr(w, r, "daftar paket gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type planRequest struct {
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Price        decimal.Decimal `json:"price"`
	DurationDays *int            `json:"duration_days"`
	MaxTxPerDay  *int            `json:"max_tx_per_day"`
	MaxUsers     *int            `json:"max_users"`
	SelfService  bool            `json:"self_service"`
	Active       bool            `json:"active"`
	SortOrder    int             `json:"sort_order"`
	Modules      []string        `json:"modules"`
	Highlights   []string        `json:"highlights"`
}

func (h *Handler) savePlan(w http.ResponseWriter, r *http.Request, id *uuid.UUID) {
	var req planRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	pid, err := h.svc.SavePlan(r.Context(), actor(r), SavePlanInput{
		ID: id, Code: req.Code, Name: req.Name, Description: req.Description, Price: req.Price, DurationDays: req.DurationDays,
		MaxTxPerDay: req.MaxTxPerDay, MaxUsers: req.MaxUsers, SelfService: req.SelfService, Active: req.Active, SortOrder: req.SortOrder, Modules: dedupe(req.Modules), Highlights: cleanLines(req.Highlights),
	})
	if err != nil {
		h.planErr(w, r, "simpan paket gagal", err)
		return
	}
	status := http.StatusOK
	if id == nil {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, map[string]string{"id": pid.String()})
}

func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) { h.savePlan(w, r, nil) }

func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.savePlan(w, r, &id)
}

func (h *Handler) GetTenantPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tp, err := h.svc.GetTenantPlan(r.Context(), id)
	if err != nil {
		h.planErr(w, r, "baca paket tenant gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, tp)
}

func (h *Handler) SetTenantPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		PlanID     *uuid.UUID `json:"plan_id"`
		ExpiresAt  *time.Time `json:"expires_at"`
		OverrideTx *int       `json:"override_max_tx_per_day"`
		OverrideU  *int       `json:"override_max_users"`
		Note       string     `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.SetTenantPlan(r.Context(), actor(r), id, SetTenantPlanInput{
		PlanID: req.PlanID, ExpiresAt: req.ExpiresAt, OverrideTx: req.OverrideTx, OverrideU: req.OverrideU, Note: req.Note,
	}); err != nil {
		h.planErr(w, r, "ubah paket tenant gagal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dedupe membuang id kembar tanpa mengubah nil (semua modul) menjadi daftar kosong (tanpa modul).
func dedupe(in []string) []string {
	if in == nil {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, id := range in {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// PlanModules = registri modul izin yang bisa dipilih untuk sebuah paket.
func (h *Handler) PlanModules(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"modules": authz.Modules})
}

// cleanLines memangkas spasi dan membuang baris kosong; selalu mengembalikan slice non-nil (kolom NOT NULL).
func cleanLines(in []string) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
