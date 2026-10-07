package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

type SMTP struct {
	cfg  Config
	from *mail.Address
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := m.validate()
	if err != nil {
		return err
	}
	body, err := buildMIME(s.from, to, m, time.Now())
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.cfg.Port == 465 { // TLS implisit
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("sambung SMTP: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("klien SMTP: %w", err)
	}
	defer c.Close()

	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("STARTTLS: %w", err)
			}
		}
	}
	// PlainAuth menolak mengirim kredensial lewat koneksi tanpa TLS (kecuali localhost).
	if s.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)); err != nil {
			return fmt.Errorf("auth SMTP: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("tulis pesan: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("akhiri pesan: %w", err)
	}
	return c.Quit()
}

// buildMIME menyusun pesan multipart/alternative (teks + HTML opsional) dengan header yang aman dan subjek UTF-8.
func buildMIME(from, to *mail.Address, m Message, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	boundaryRaw := make([]byte, 12)
	if _, err := rand.Read(boundaryRaw); err != nil {
		return nil, err
	}
	msgID := hex.EncodeToString(boundaryRaw)

	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	h("From", from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", now.UTC().Format(time.RFC1123Z))
	h("Message-ID", "<"+msgID+"@aciraba>")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")

	mw := multipart.NewWriter(&buf)
	h("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", mw.Boundary()))
	buf.WriteString("\r\n")

	part := func(contentType, body string) error {
		p, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {contentType + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return err
		}
		qp := quotedprintable.NewWriter(p)
		if _, err := qp.Write([]byte(body)); err != nil {
			return err
		}
		return qp.Close()
	}
	if err := part("text/plain", m.Text); err != nil {
		return nil, err
	}
	if m.HTML != "" {
		if err := part("text/html", m.HTML); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
