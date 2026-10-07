package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/authz"
)

// authzActor meniru authz.Resolver.Authenticate: hak akses terkini dari DB + outlet dari klaim token.
func authzActor(t *testing.T, h *harness, tenant, user, outlet string) authz.Actor {
	t.Helper()
	tid, uid, oid := uuid.MustParse(tenant), uuid.MustParse(user), uuid.MustParse(outlet)
	access, err := h.svc.Perms.For(context.Background(), tid, uid)
	if err != nil {
		t.Fatal(err)
	}
	return authz.Actor{TenantID: tid, UserID: uid, OutletID: oid, Name: access.Name, Perms: access.Perms, Outlets: access.Outlets}
}
