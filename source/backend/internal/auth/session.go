package auth

import (
	"time"

	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
)

type Identity struct {
	ID    string `json:"id"`
	Code  string `json:"code,omitempty"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Profile = identitas pengguna yang sedang masuk (dipakai klien untuk tampilan; bukan sumber otorisasi).
type Profile struct {
	// Permissions: izin efektif untuk menyaring menu/tombol di UI. Penegakan tetap di server (authz.Require).
	Permissions authz.Permissions `json:"permissions"`
	// EmailVerified: false → UI menampilkan ajakan verifikasi (login tidak diblokir).
	EmailVerified bool     `json:"email_verified"`
	User          Identity `json:"user"`
	Tenant        Identity `json:"tenant"`
	Outlet        Identity `json:"outlet"`
}

type Session struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Profile
	// RefreshToken kosong pada jendela grace atau pindah outlet (cookie tidak diubah).
	RefreshToken string `json:"-"`
	Remember     bool   `json:"-"`
}

func profileOf(a account, perms authz.Permissions) Profile {
	return Profile{
		Permissions:   perms,
		EmailVerified: a.EmailVerified,
		User:          Identity{ID: a.UserID.String(), Name: a.UserName, Email: a.Email},
		Tenant:        Identity{ID: a.TenantID.String(), Code: a.TenantCode, Name: a.TenantName},
		Outlet:        Identity{ID: a.OutletID.String(), Code: a.OutletCode, Name: a.OutletName},
	}
}

func buildSession(a account, perms authz.Permissions, access, refresh string, remember bool) *Session {
	return &Session{
		AccessToken: access, ExpiresIn: int(pauth.AccessTTL.Seconds()), Profile: profileOf(a, perms),
		RefreshToken: refresh, Remember: remember,
	}
}

// issueAccess menerbitkan token akses untuk akun (klaim sub/tid/oid/role).
func (s *Service) issueAccess(a account, now time.Time) (string, error) {
	return s.Tokens.Issue(a.UserID.String(), a.TenantID.String(), a.OutletID.String(), a.RoleName, now)
}
