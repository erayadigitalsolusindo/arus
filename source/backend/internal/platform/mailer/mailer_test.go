package mailer

import (
	"io"
	"log/slog"
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestTemplatesEscapeAndLanguage(t *testing.T) {
	m, err := VerifyEmail("en-US,en;q=0.9", "a@b.id", `<script>alert(1)</script>`, "https://app.test/verify-email#token=abc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Subject, "Verify") {
		t.Errorf("bahasa Inggris tidak dipilih: %q", m.Subject)
	}
	if strings.Contains(m.HTML, "<script>") {
		t.Error("nama pengguna tidak di-escape di HTML")
	}
	if !strings.Contains(m.Text, "https://app.test/verify-email#token=abc") {
		t.Error("tautan hilang dari teks")
	}
	id, _ := ResetPassword("id", "a@b.id", "Budi", "https://app.test/reset-password#token=x")
	if !strings.Contains(id.Subject, "Atur ulang") {
		t.Errorf("default harus Indonesia: %q", id.Subject)
	}
	if _, err := ResetPassword("id", "a@b.id", "Budi", "javascript:alert(1)"); err == nil {
		t.Error("skema bukan http(s) harus ditolak")
	}
}

func TestMessageValidationBlocksHeaderInjection(t *testing.T) {
	for name, m := range map[string]Message{
		"subjek CRLF":  {To: "a@b.id", Subject: "Hai\r\nBcc: x@y.id", Text: "x"},
		"tujuan CRLF":  {To: "a@b.id\r\nBcc: x@y.id", Subject: "s", Text: "x"},
		"tujuan rusak": {To: "bukan-email", Subject: "s", Text: "x"},
		"teks kosong":  {To: "a@b.id", Subject: "s", Text: " "},
	} {
		if _, err := m.validate(); err == nil {
			t.Errorf("%s: seharusnya ditolak", name)
		}
	}
	if _, err := (Message{To: "Budi <a@b.id>", Subject: "s", Text: "x"}).validate(); err != nil {
		t.Errorf("alamat bernama sah ditolak: %v", err)
	}
}

func TestBuildMIME(t *testing.T) {
	from, _ := mail.ParseAddress("ACIRABA <noreply@aciraba.id>")
	to, _ := mail.ParseAddress("a@b.id")
	raw, err := buildMIME(from, to, Message{To: "a@b.id", Subject: "Verifikasi é", Text: "halo", HTML: "<b>halo</b>"}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("pesan tidak bisa diurai: %v", err)
	}
	subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if subj != "Verifikasi é" {
		t.Errorf("subjek = %q", subj)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" || params["boundary"] == "" {
		t.Errorf("content-type: %q %v", mt, err)
	}
	body, _ := io.ReadAll(msg.Body)
	if !strings.Contains(string(body), "text/plain") || !strings.Contains(string(body), "text/html") {
		t.Error("bagian teks/HTML tidak lengkap")
	}
}

func TestNewSelectsImplementation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := New(Config{}, log)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.(*LogMailer); !ok {
		t.Errorf("tanpa host harus LogMailer, got %T", m)
	}
	if _, err := New(Config{Host: "smtp.test", From: "bukan alamat"}, log); err == nil {
		t.Error("SMTP_FROM rusak harus ditolak")
	}
	if m, err := New(Config{Host: "smtp.test", From: "ACIRABA <noreply@aciraba.id>"}, log); err != nil {
		t.Fatal(err)
	} else if _, ok := m.(*SMTP); !ok {
		t.Errorf("dengan host harus SMTP, got %T", m)
	}
}
