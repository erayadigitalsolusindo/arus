package httpx

import (
	"context"
	"net/http"
	"time"
)

// Pinger dipenuhi oleh pgxpool.Pool; adapter Redis dibuat di main.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Healthz memeriksa dependensi. 200 bila semua ok, 503 bila ada yang gagal.
func Healthz(deps map[string]Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := make(map[string]string, len(deps))
		status, code := "ok", http.StatusOK
		for name, d := range deps {
			if err := d.Ping(ctx); err != nil {
				checks[name], status, code = "down", "degraded", http.StatusServiceUnavailable
				continue
			}
			checks[name] = "ok"
		}
		JSON(w, code, map[string]any{"status": status, "checks": checks})
	}
}
