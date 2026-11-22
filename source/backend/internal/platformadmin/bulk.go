package platformadmin

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"aciraba/internal/auth"
	"aciraba/internal/platform/sanitize"
)

// ActionTenantCreate = audit platform untuk tenant yang didaftarkan operator (massal).
const ActionTenantCreate = "platform.tenant_create"

// MaxBulkTenants = batas baris per permintaan (tiap baris = satu transaksi + satu hash password).
const MaxBulkTenants = 100

// BulkDefaultPassword = password awal semua tenant massal; pemilik wajib menggantinya sendiri (login Google tetap bisa).
const BulkDefaultPassword = "SayaCintaIndonesia1945"

type BulkTenantInput struct {
	BusinessName string `json:"business_name"`
	Email        string `json:"email"`
	OwnerName    string `json:"owner_name"`  // kosong = bagian lokal email
	OutletName   string `json:"outlet_name"` // kosong = nama bisnis
	Phone        string `json:"phone"`       // opsional
}

type BulkTenantResult struct {
	Row      int       `json:"row"` // 1-based, sesuai urutan kiriman
	Email    string    `json:"email"`
	OK       bool      `json:"ok"`
	TenantID uuid.UUID `json:"tenant_id,omitempty"`
	Error    string    `json:"error,omitempty"` // EMAIL_TAKEN | INVALID:<field> | INTERNAL
}

var ErrBulkLimit = errors.New("jumlah baris tidak valid")

// BulkCreateTenants mendaftarkan banyak tenant sekaligus, langsung aktif. Tiap baris berdiri sendiri: baris gagal
// tidak membatalkan baris lain. Pemilik masuk lewat Google dengan email yang sama (tertaut otomatis saat login pertama).
func (s *Service) BulkCreateTenants(ctx context.Context, a Actor, authSvc *auth.Service, rows []BulkTenantInput) ([]BulkTenantResult, error) {
	if len(rows) == 0 || len(rows) > MaxBulkTenants {
		return nil, ErrBulkLimit
	}
	out := make([]BulkTenantResult, len(rows))
	for i, in := range rows {
		res := BulkTenantResult{Row: i + 1, Email: strings.TrimSpace(in.Email)}
		in.OwnerName = firstNonEmpty(in.OwnerName, strings.SplitN(in.Email, "@", 2)[0])
		in.OutletName = firstNonEmpty(in.OutletName, in.BusinessName)
		c, bad := cleanBulk(in)
		if bad != "" {
			res.Error = "INVALID:" + bad
			out[i] = res
			continue
		}
		res.Email, c.Password = c.Email, BulkDefaultPassword
		tid, err := authSvc.ProvisionTenant(ctx, c)
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			res.Error = "EMAIL_TAKEN"
		case err != nil:
			res.Error = "INTERNAL"
		default:
			res.OK, res.TenantID = true, tid
			s.record(ctx, a, ActionTenantCreate, tid, c.BusinessName, map[string]any{"owner_email": c.Email, "bulk": true})
		}
		out[i] = res
	}
	return out, nil
}

func cleanBulk(in BulkTenantInput) (c auth.CleanRegister, badField string) {
	var code string
	for _, f := range []struct {
		name string
		do   func() string
	}{
		{"business_name", func() (e string) { c.BusinessName, e = sanitize.Name(in.BusinessName, maxName); return }},
		{"owner_name", func() (e string) { c.OwnerName, e = sanitize.Name(in.OwnerName, maxName); return }},
		{"outlet_name", func() (e string) { c.OutletName, e = sanitize.Name(in.OutletName, maxName); return }},
		{"email", func() (e string) { c.Email, e = sanitize.Email(in.Email); return }},
		{"phone", func() (e string) {
			if strings.TrimSpace(in.Phone) == "" {
				return ""
			}
			c.Phone, e = sanitize.Phone(in.Phone)
			return
		}},
	} {
		if code = f.do(); code != "" {
			return auth.CleanRegister{}, f.name
		}
	}
	return c, ""
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
