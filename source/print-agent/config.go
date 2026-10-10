package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// Config dibaca dari print-agent.json di folder yang sama dengan program; dibuat dengan nilai bawaan bila belum ada.
type Config struct {
	// Listen = alamat lokal yang didengar. Selalu loopback: agent tidak boleh bisa dihubungi dari jaringan.
	Listen string `json:"listen"`
	// Origins = alamat aplikasi ARUS yang boleh meminta cetak (persis skema://host[:port], tanpa garis miring akhir).
	Origins []string `json:"origins"`
	// Printer = nama printer Windows; kosong = printer default.
	Printer string `json:"printer"`
	// Device = berkas perangkat printer (non-Windows), mis. /dev/usb/lp0.
	Device string `json:"device,omitempty"`
}

func defaultConfig() Config {
	return Config{
		Listen:  "127.0.0.1:9100",
		Origins: []string{"https://arus.erayadigital.co.id", "http://localhost:5173"},
	}
}

// loadConfig membaca berkas konfigurasi; bila belum ada, menulis bawaan agar mudah diubah pengguna.
func loadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		b, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			return cfg, fmt.Errorf("tulis konfigurasi bawaan: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s tidak valid: %w", path, err)
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen %q: %w", c.Listen, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("listen %q harus alamat loopback (127.0.0.1)", c.Listen)
	}
	clean := c.Origins[:0]
	for _, o := range c.Origins {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" {
			return fmt.Errorf("origin %q tidak valid (contoh: https://arus.contoh.id)", o)
		}
		clean = append(clean, o)
	}
	if len(clean) == 0 {
		return errors.New("origins kosong: isi alamat aplikasi ARUS")
	}
	c.Origins = clean
	return nil
}
