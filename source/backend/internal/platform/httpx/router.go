package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter menyiapkan chi dengan middleware dasar. Route modul didaftarkan pemanggil.
func NewRouter(log *slog.Logger, corsOrigins []string, trustProxy bool) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// RealIP memercayai X-Forwarded-For dari klien mana pun, sehingga bisa dipalsukan (bypass rate limit).
	// Aktifkan hanya bila API di belakang reverse proxy tepercaya.
	if trustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(SecurityHeaders)
	r.Use(requestLogger(log))
	r.Use(recoverer(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Idempotency-Key", CSRFHeader, "Accept-Language", "X-Client"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		Error(w, http.StatusNotFound, "NOT_FOUND", "Endpoint tidak ditemukan.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Metode tidak diizinkan.")
	})
	return r
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"dur_ms", time.Since(start).Milliseconds(),
				"req_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "recover", rec, "path", r.URL.Path, "req_id", middleware.GetReqID(r.Context()))
					Error(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
