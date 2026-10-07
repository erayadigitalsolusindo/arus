package mailer

import (
	"context"
	"log/slog"
)

// LogMailer tidak mengirim apa pun; seluruh isi email (termasuk tautan) dicatat ke log agar alur verifikasi dan
// reset password bisa dicoba di mesin pengembangan tanpa server email. JANGAN dipakai di produksi.
type LogMailer struct{ log *slog.Logger }

func NewLog(log *slog.Logger) *LogMailer { return &LogMailer{log: log} }

func (l *LogMailer) Send(_ context.Context, m Message) error {
	if _, err := m.validate(); err != nil {
		return err
	}
	l.log.Info("email (LogMailer, tidak dikirim)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}
