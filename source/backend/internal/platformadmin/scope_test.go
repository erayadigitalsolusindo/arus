package platformadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestScopeGuard(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	h := scopeGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	code := func(a Actor, path string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil).WithContext(context.WithValue(context.Background(), actorKey{}, a))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	scoped, all := Actor{Scope: []uuid.UUID{mine}}, Actor{}
	for _, c := range []struct {
		a    Actor
		path string
		want int
	}{
		{scoped, "/platform/tenants/" + mine.String() + "/audit", 200},
		{scoped, "/platform/tenants/" + other.String(), 404},
		{scoped, "/platform/ai/policies/tenants/" + other.String(), 404},
		{scoped, "/platform/tenants", 200},
		{Actor{Scope: []uuid.UUID{}}, "/platform/tenants/" + mine.String(), 404}, // kosong = tak satu pun
		{all, "/platform/tenants/" + other.String(), 200},
	} {
		if got := code(c.a, c.path); got != c.want {
			t.Errorf("%s: got %d, want %d", c.path, got, c.want)
		}
	}
}
