package audit

import "aciraba/internal/authz"

// FromActor mengubah Actor otorisasi menjadi pelaku audit (satu tempat agar semua modul konsisten).
func FromActor(a authz.Actor) Actor {
	return Actor{TenantID: a.TenantID, UserID: a.UserID, OutletID: a.OutletID, Name: a.Name}
}
