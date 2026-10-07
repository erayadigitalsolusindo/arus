// Package mailer mengirim email transaksional (verifikasi, reset password). Implementasi dipilih dari konfigurasi:
// SMTP bila host diisi, selain itu LogMailer (hanya mencatat ke log; untuk pengembangan lokal).
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
)

type Message struct {
	To      string
	Subject string
	Text    string // wajib
	HTML    string // opsional
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string // mis. "ACIRABA <noreply@contoh.id>"
}

// New memilih implementasi. SMTP aktif bila Host terisi.
func New(cfg Config, log *slog.Logger) (Mailer, error) {
	if cfg.Host == "" {
		return &LogMailer{log: log}, nil
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("SMTP_FROM tidak valid: %w", err)
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

// validate menolak pesan yang bisa dipakai untuk menyuntik header (CR/LF) dan alamat tujuan yang tidak sah.
func (m Message) validate() (*mail.Address, error) {
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return nil, fmt.Errorf("header mengandung karakter baris baru")
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return nil, fmt.Errorf("alamat tujuan tidak valid: %w", err)
	}
	if strings.TrimSpace(m.Text) == "" {
		return nil, fmt.Errorf("isi teks kosong")
	}
	return to, nil
}
