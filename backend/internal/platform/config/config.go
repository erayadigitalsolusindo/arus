// Package config memuat konfigurasi proses dari environment variable.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	CORSOrigins []string
}

func (c Config) IsDev() bool { return c.Env == "development" }

// Load membaca .env (jika ada; tidak menimpa env yang sudah di-set) lalu memvalidasi.
func Load() (Config, error) {
	_ = godotenv.Load()

	c := Config{
		Env:         getenv("APP_ENV", "development"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
	}
	for _, o := range strings.Split(os.Getenv("CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL wajib diisi"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL wajib diisi"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("konfigurasi tidak valid: %w", err)
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
