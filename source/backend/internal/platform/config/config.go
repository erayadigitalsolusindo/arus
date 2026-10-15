// Package config memuat konfigurasi proses dari environment variable.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	CORSOrigins []string
	JWTSecret   string
	// TrustProxy: percayai X-Forwarded-For (hanya bila di belakang proxy tepercaya).
	TrustProxy bool

	// UploadDir = folder penyimpanan gambar unggahan (disk lokal). Di luar kode dan di-gitignore; di produksi taruh di
	// volume yang ikut dicadangkan.
	UploadDir string

	// PlatformSetupToken = rahasia untuk membuat Platform Admin pertama lewat /platform/setup. Kosong = setup mati.
	// Hapus dari lingkungan server setelah admin pertama dibuat.
	PlatformSetupToken string

	// AutoJournal: penjadwal malam yang membuat DRAF jurnal otomatis (AUTO_JOURNAL=true|false). Bawaan: nyala di luar development
	// (di dev mati agar API yang dijalankan ulang tidak menulis draf ke data pengguna tanpa diminta).
	AutoJournal bool

	// AIPlatformKey = API key Claude milik platform untuk akun gratis HalloArus (kosong = tanpa akun gratis; tenant wajib memasang key sendiri).
	// AIFreeDailyLimit = jumlah pertanyaan gratis per tenant per hari (bawaan 10); akun gratis selalu memakai model murah (Haiku).
	// APIBaseURL = alamat PUBLIK API (tanpa slash akhir): dipakai metadata OAuth dan alamat connector MCP (/mcp) yang dimasukkan ke Claude.
	APIBaseURL       string
	AIPlatformKey    string
	AIFreeDailyLimit int

	// EraPay (QRIS): alamat API EraPay (sama untuk semua tenant); kosong = integrasi mati. API key dipasang PER TENANT oleh owner (disimpan terenkripsi di DB).
	EraPayBaseURL        string
	EraPayProvisionToken string // shared server-to-server secret; never sent to the browser

	// EraPayReconcile: penjadwal rekonsiliasi nota yang disimpan paksa (erapay_pending) ke EraPay.
	// Bawaan: nyala di luar development (di dev mati agar API yang dijalankan ulang tidak menulis ke data pengguna tanpa diminta).
	EraPayReconcile bool

	// AppBaseURL = alamat SPA yang dipakai membentuk tautan di email (verifikasi, reset password), tanpa slash akhir.
	AppBaseURL string
	// SMTP: kosongkan SMTPHost di dev agar email hanya dicatat ke log (LogMailer).
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

func (c Config) IsDev() bool { return c.Env == "development" }

// Load membaca .env (jika ada; tidak menimpa env yang sudah di-set) lalu memvalidasi.
func Load() (Config, error) {
	_ = godotenv.Load()

	c := Config{
		Env:                  getenv("APP_ENV", "development"),
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		RedisURL:             os.Getenv("REDIS_URL"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		TrustProxy:           os.Getenv("TRUST_PROXY") == "true",
		UploadDir:            getenv("UPLOAD_DIR", "./data/uploads"),
		PlatformSetupToken:   os.Getenv("PLATFORM_SETUP_TOKEN"),
		APIBaseURL:           strings.TrimRight(getenv("API_BASE_URL", "http://localhost:8080"), "/"),
		AIPlatformKey:        strings.TrimSpace(os.Getenv("AI_PLATFORM_KEY")),
		AIFreeDailyLimit:     10,
		EraPayBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("ERAPAY_BASE_URL")), "/"),
		EraPayProvisionToken: strings.TrimSpace(os.Getenv("ERAPAY_ARUS_PROVISION_TOKEN")),
		AppBaseURL:           strings.TrimRight(getenv("APP_BASE_URL", "http://localhost:5173"), "/"),
		SMTPHost:             os.Getenv("SMTP_HOST"),
		SMTPUser:             os.Getenv("SMTP_USER"),
		SMTPPass:             os.Getenv("SMTP_PASS"),
		SMTPFrom:             getenv("SMTP_FROM", "ACIRABA <noreply@localhost>"),
	}
	c.AutoJournal = !c.IsDev()
	if v := os.Getenv("AUTO_JOURNAL"); v != "" {
		c.AutoJournal = v == "true"
	}
	c.EraPayReconcile = !c.IsDev()
	if v := os.Getenv("ERAPAY_RECONCILE"); v != "" {
		c.EraPayReconcile = v == "true"
	}
	if v := os.Getenv("AI_FREE_DAILY_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			return Config{}, errors.New("AI_FREE_DAILY_LIMIT tidak valid (0-1000)")
		}
		c.AIFreeDailyLimit = n
	}
	for _, o := range strings.Split(os.Getenv("CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}

	if v := os.Getenv("SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, errors.New("SMTP_PORT tidak valid")
		}
		c.SMTPPort = n
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL wajib diisi"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL wajib diisi"))
	}
	if !c.IsDev() && !strings.HasPrefix(c.AppBaseURL, "https://") {
		errs = append(errs, errors.New("APP_BASE_URL wajib https:// di luar development"))
	}
	if !c.IsDev() && !strings.HasPrefix(c.APIBaseURL, "https://") {
		errs = append(errs, errors.New("API_BASE_URL wajib https:// di luar development (alamat publik API untuk connector MCP)"))
	}
	if !c.IsDev() && c.EraPayBaseURL != "" && !strings.HasPrefix(c.EraPayBaseURL, "https://") {
		errs = append(errs, errors.New("ERAPAY_BASE_URL wajib https:// di luar development"))
	}
	if c.EraPayProvisionToken != "" && len(c.EraPayProvisionToken) < 32 {
		errs = append(errs, errors.New("ERAPAY_ARUS_PROVISION_TOKEN minimal 32 karakter bila diisi"))
	}
	if !c.IsDev() && c.SMTPHost == "" {
		errs = append(errs, errors.New("SMTP_HOST wajib di luar development (email verifikasi dan reset password)"))
	}
	if c.PlatformSetupToken != "" && len(c.PlatformSetupToken) < 32 {
		errs = append(errs, errors.New("PLATFORM_SETUP_TOKEN minimal 32 karakter bila diisi"))
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET wajib diisi, minimal 32 karakter"))
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
