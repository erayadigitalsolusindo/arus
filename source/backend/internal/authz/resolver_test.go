package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := Require("items", ActCreate)(ok)

	kasir, _ := Normalize(map[string][]string{"items": {"view"}})
	cases := []struct {
		name  string
		actor *Actor
		want  int
	}{
		{"tanpa actor (Authenticate belum jalan)", nil, 403},
		{"izin kurang", &Actor{Perms: kasir}, 403},
		{"izin cukup", &Actor{Perms: Permissions{Grants: map[string][]string{"items": {"view", "create"}}}}, 204},
		{"owner", &Actor{Perms: Permissions{All: true}}, 204},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/", nil)
		if c.actor != nil {
			req = req.WithContext(context.WithValue(req.Context(), actorKey{}, *c.actor))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
	}
}
