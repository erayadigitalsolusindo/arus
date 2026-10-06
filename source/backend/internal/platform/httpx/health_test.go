package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		deps       map[string]Pinger
		wantStatus int
		wantBody   string
	}{
		{"semua ok", map[string]Pinger{"postgres": fakePinger{}, "redis": fakePinger{}}, http.StatusOK, `"status":"ok"`},
		{"redis down", map[string]Pinger{"postgres": fakePinger{}, "redis": fakePinger{errors.New("x")}}, http.StatusServiceUnavailable, `"redis":"down"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Healthz(tc.deps)(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body %q tidak memuat %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}
