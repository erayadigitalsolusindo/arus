package platformadmin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/platform/httpx"
)

// Domain kustom (white label) per tenant. Tenant tetap ditentukan dari token login; domain hanya alamat lain ke aplikasi
// yang sama. Alur: admin menambah host (pending) -> klien memasang CNAME ke DomainTarget -> verifikasi DNS -> active ->
// reverse proxy (Caddy on-demand TLS) bertanya ke /public/domain-check sebelum menerbitkan sertifikat.
const (
	ActionDomainAdd     = "platform.domain_add"
	ActionDomainVerify  = "platform.domain_verify"
	ActionDomainDisable = "platform.domain_disable"
	ActionDomainDelete  = "platform.domain_delete"

	dnsTimeout = 5 * time.Second
)

var (
	ErrDomainTaken    = errors.New("domain sudah terpakai")
	ErrDomainInvalid  = errors.New("domain tidak valid")
	ErrDomainNoTarget = errors.New("tujuan CNAME belum dikonfigurasi")
	domainHostPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$`)
)

// dnsResolver dipenuhi *net.Resolver; diganti di test.
type dnsResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// DomainRow = satu domain kustom milik tenant.
type DomainRow struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	TenantCode     string     `json:"tenant_code"`
	TenantName     string     `json:"tenant_name"`
	Host           string     `json:"host"`
	Status         string     `json:"status"` // pending | active | disabled
	VerifiedAt     *time.Time `json:"verified_at"`
	LastCheckAt    *time.Time `json:"last_check_at"`
	LastCheckError string     `json:"last_check_error"`
	CreatedAt      time.Time  `json:"created_at"`
}

// normalizeHost: huruf kecil, tanpa spasi/titik akhir; ErrDomainInvalid bila bukan nama host yang sah.
func normalizeHost(raw string) (string, error) {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if len(h) > 253 || !domainHostPattern.MatchString(h) {
		return "", ErrDomainInvalid
	}
	return h, nil
}

func (s *Service) ListDomains(ctx context.Context, a Actor, tenant, id *uuid.UUID) ([]DomainRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, tenant_id, tenant_code, tenant_name, host, status, verified_at, last_check_at,
		last_check_error, created_at FROM platform_domain_list($1, $2, $3)`, a.ID, tenant, id)
	if err != nil {
		if pgCode(err) == "42501" {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	defer rows.Close()
	out := []DomainRow{}
	for rows.Next() {
		var (
			d                pgtype.UUID
			verified, lastAt pgtype.Timestamptz
			r                DomainRow
		)
		if err := rows.Scan(&d, &r.TenantID, &r.TenantCode, &r.TenantName, &r.Host, &r.Status, &verified, &lastAt, &r.LastCheckError, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.ID = uuid.UUID(d.Bytes)
		if verified.Valid {
			r.VerifiedAt = &verified.Time
		}
		if lastAt.Valid {
			r.LastCheckAt = &lastAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) getDomain(ctx context.Context, a Actor, id uuid.UUID) (DomainRow, error) {
	list, err := s.ListDomains(ctx, a, nil, &id)
	if err != nil {
		return DomainRow{}, err
	}
	if len(list) == 0 {
		return DomainRow{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Service) AddDomain(ctx context.Context, a Actor, tenant uuid.UUID, rawHost string) (uuid.UUID, error) {
	host, err := normalizeHost(rawHost)
	if err != nil {
		return uuid.Nil, err
	}
	// Alamat aplikasi sendiri tidak boleh didaftarkan sebagai domain tenant.
	if host == s.DomainTarget {
		return uuid.Nil, ErrDomainInvalid
	}
	var id uuid.UUID
	err = s.Pool.QueryRow(ctx, `SELECT platform_domain_add($1, $2, $3)`, a.ID, tenant, host).Scan(&id)
	switch pgCode(err) {
	case "23505":
		return uuid.Nil, ErrDomainTaken
	case "23503": // tenant tidak ada
		return uuid.Nil, ErrNotFound
	case "23514":
		return uuid.Nil, ErrDomainInvalid
	case "42501":
		return uuid.Nil, ErrInvalidSession
	}
	if err != nil {
		return uuid.Nil, err
	}
	s.record(ctx, a, ActionDomainAdd, tenant, "", map[string]any{"host": host})
	return id, nil
}

// VerifyDomain memeriksa DNS host lalu mencatat hasilnya. Mengembalikan baris terbaru; pesan kegagalan DNS ada di
// LastCheckError (bukan error), karena itu hasil pemeriksaan yang normal.
func (s *Service) VerifyDomain(ctx context.Context, a Actor, id uuid.UUID) (DomainRow, error) {
	d, err := s.getDomain(ctx, a, id)
	if err != nil {
		return DomainRow{}, err
	}
	if s.DomainTarget == "" {
		return DomainRow{}, ErrDomainNoTarget
	}
	dnsErr := s.checkDomainDNS(ctx, d.Host)
	msg := ""
	if dnsErr != nil {
		msg = dnsErr.Error()
	}
	if _, err := s.Pool.Exec(ctx, `SELECT platform_domain_mark($1, $2, $3, $4)`, a.ID, id, dnsErr == nil, msg); err != nil {
		if pgCode(err) == "42501" {
			return DomainRow{}, ErrInvalidSession
		}
		return DomainRow{}, err
	}
	s.record(ctx, a, ActionDomainVerify, d.TenantID, d.TenantName, map[string]any{"host": d.Host, "ok": dnsErr == nil})
	return s.getDomain(ctx, a, id)
}

// checkDomainDNS: host harus CNAME ke DomainTarget, atau (untuk proxy/ALIAS) berbagi alamat IP dengannya.
func (s *Service) checkDomainDNS(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	cname, cerr := s.dns.LookupCNAME(ctx, host)
	cname = strings.TrimSuffix(strings.ToLower(cname), ".")
	if cerr == nil && cname == s.DomainTarget {
		return nil
	}
	hostIPs, herr := s.dns.LookupHost(ctx, host)
	if herr != nil {
		var dnsErr *net.DNSError
		if errors.As(herr, &dnsErr) && dnsErr.IsNotFound {
			return fmt.Errorf("DNS %s belum ada; buat CNAME ke %s", host, s.DomainTarget)
		}
		return fmt.Errorf("DNS %s tidak bisa dibaca; coba lagi sebentar lagi", host)
	}
	if targetIPs, terr := s.dns.LookupHost(ctx, s.DomainTarget); terr == nil {
		seen := make(map[string]struct{}, len(targetIPs))
		for _, ip := range targetIPs {
			seen[ip] = struct{}{}
		}
		for _, ip := range hostIPs {
			if _, ok := seen[ip]; ok {
				return nil
			}
		}
	}
	if cname != "" && cname != host {
		return fmt.Errorf("CNAME %s mengarah ke %s, seharusnya %s", host, cname, s.DomainTarget)
	}
	return fmt.Errorf("%s belum mengarah ke %s; buat CNAME ke %s", host, s.DomainTarget, s.DomainTarget)
}

// SetDomainDisabled: true = matikan (sertifikat baru tidak diterbitkan, branding mati); false = kembali pending (verifikasi ulang).
func (s *Service) SetDomainDisabled(ctx context.Context, a Actor, id uuid.UUID, disabled bool) error {
	d, err := s.getDomain(ctx, a, id)
	if err != nil {
		return err
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_domain_disable($1, $2, $3)`, a.ID, id, disabled).Scan(&ok); err != nil {
		if pgCode(err) == "42501" {
			return ErrInvalidSession
		}
		return err
	}
	if !ok {
		return ErrNotFound
	}
	s.record(ctx, a, ActionDomainDisable, d.TenantID, d.TenantName, map[string]any{"host": d.Host, "disabled": disabled})
	return nil
}

func (s *Service) DeleteDomain(ctx context.Context, a Actor, id uuid.UUID) error {
	d, err := s.getDomain(ctx, a, id)
	if err != nil {
		return err
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT platform_domain_delete($1, $2)`, a.ID, id).Scan(&ok); err != nil {
		if pgCode(err) == "42501" {
			return ErrInvalidSession
		}
		return err
	}
	if !ok {
		return ErrNotFound
	}
	s.record(ctx, a, ActionDomainDelete, d.TenantID, d.TenantName, map[string]any{"host": d.Host})
	return nil
}

// PublicDomain = identitas minimal tenant pemilik domain aktif (tanpa data sensitif).
type PublicDomain struct {
	TenantCode string `json:"tenant_code"`
	TenantName string `json:"tenant_name"`
}

// LookupPublicDomain: ErrNotFound bila host bukan domain aktif milik tenant aktif.
func (s *Service) LookupPublicDomain(ctx context.Context, rawHost string) (PublicDomain, error) {
	host, err := normalizeHost(rawHost)
	if err != nil {
		return PublicDomain{}, ErrNotFound
	}
	var p PublicDomain
	err = s.Pool.QueryRow(ctx, `SELECT tenant_code, tenant_name FROM public_domain_lookup($1)`, host).Scan(&p.TenantCode, &p.TenantName)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicDomain{}, ErrNotFound
	}
	return p, err
}

// ---- HTTP ----

func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
	var tenant *uuid.UUID
	if q := r.URL.Query().Get("tenant_id"); q != "" {
		id, err := uuid.Parse(q)
		if err != nil {
			httpx.ValidationError(w, map[string]string{"tenant_id": "INVALID"})
			return
		}
		tenant = &id
	}
	list, err := h.svc.ListDomains(r.Context(), actor(r), tenant, nil)
	if errors.Is(err, ErrInvalidSession) {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
		return
	} else if err != nil {
		h.internal(w, r, "daftar domain gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": list, "target": h.svc.DomainTarget})
}

type addDomainRequest struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Host     string    `json:"host"`
}

func (h *Handler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var req addDomainRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.TenantID == uuid.Nil {
		httpx.ValidationError(w, map[string]string{"tenant_id": "REQUIRED"})
		return
	}
	id, err := h.svc.AddDomain(r.Context(), actor(r), req.TenantID, req.Host)
	switch {
	case errors.Is(err, ErrDomainInvalid):
		httpx.ValidationError(w, map[string]string{"host": "INVALID"})
	case errors.Is(err, ErrDomainTaken):
		httpx.Error(w, http.StatusConflict, "DOMAIN_TAKEN", "Domain sudah terdaftar.")
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "tambah domain gagal", err)
	default:
		httpx.JSON(w, http.StatusCreated, map[string]string{"id": id.String()})
	}
}

func (h *Handler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.VerifyDomain(r.Context(), actor(r), id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Domain tidak ditemukan.")
	case errors.Is(err, ErrDomainNoTarget):
		httpx.Error(w, http.StatusConflict, "NO_TARGET", "Tujuan CNAME belum dikonfigurasi di server.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "verifikasi domain gagal", err)
	default:
		httpx.JSON(w, http.StatusOK, d)
	}
}

type patchDomainRequest struct {
	Disabled bool `json:"disabled"`
}

func (h *Handler) PatchDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req patchDomainRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	err := h.svc.SetDomainDisabled(r.Context(), actor(r), id, req.Disabled)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Domain tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "ubah domain gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := h.svc.DeleteDomain(r.Context(), actor(r), id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Domain tidak ditemukan.")
	case errors.Is(err, ErrInvalidSession):
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi tidak valid.")
	case err != nil:
		h.internal(w, r, "hapus domain gagal", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// DomainCheck: dipanggil reverse proxy (Caddy `on_demand_tls { ask }`) sebelum menerbitkan sertifikat TLS.
// 200 hanya untuk domain aktif milik tenant aktif; selain itu 404 (sertifikat tidak diterbitkan).
func (h *Handler) DomainCheck(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.LookupPublicDomain(r.Context(), r.URL.Query().Get("domain")); err != nil {
		if errors.Is(err, ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.internal(w, r, "cek domain gagal", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// DomainBranding: identitas tenant untuk host yang sedang dibuka (halaman login domain white label).
func (h *Handler) DomainBranding(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.LookupPublicDomain(r.Context(), r.URL.Query().Get("host"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Domain tidak dikenal.")
		return
	} else if err != nil {
		h.internal(w, r, "branding domain gagal", err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
