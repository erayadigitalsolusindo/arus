package mailer

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
)

// Bahasa email mengikuti Accept-Language saat permintaan; selain "en" memakai Indonesia (bahasa dasar aplikasi).
func normLang(lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		return "en"
	}
	return "id"
}

type content struct {
	Subject string
	Greet   string // sapaan dengan {{.Name}}
	Intro   string
	Button  string
	Outro   string
}

// Teks per jenis email dan bahasa. Menambah jenis/bahasa baru = tambah entri di sini (tanpa mengubah pengirim).
var texts = map[string]map[string]content{
	"verify": {
		"id": {
			Subject: "Verifikasi email ACIRABA Anda",
			Greet:   "Halo %s,",
			Intro:   "Terima kasih telah mendaftar di ACIRABA. Klik tombol di bawah untuk memverifikasi alamat email Anda. Tautan berlaku 24 jam.",
			Button:  "Verifikasi email",
			Outro:   "Jika Anda tidak merasa mendaftar, abaikan email ini.",
		},
		"en": {
			Subject: "Verify your ACIRABA email",
			Greet:   "Hello %s,",
			Intro:   "Thanks for signing up for ACIRABA. Click the button below to verify your email address. The link is valid for 24 hours.",
			Button:  "Verify email",
			Outro:   "If you did not sign up, you can ignore this email.",
		},
	},
	"reset": {
		"id": {
			Subject: "Atur ulang password ACIRABA",
			Greet:   "Halo %s,",
			Intro:   "Kami menerima permintaan untuk mengatur ulang password akun Anda. Klik tombol di bawah untuk membuat password baru. Tautan berlaku 30 menit dan hanya bisa dipakai sekali.",
			Button:  "Atur ulang password",
			Outro:   "Jika bukan Anda yang meminta, abaikan email ini; password Anda tidak berubah.",
		},
		"en": {
			Subject: "Reset your ACIRABA password",
			Greet:   "Hello %s,",
			Intro:   "We received a request to reset your account password. Click the button below to choose a new one. The link is valid for 30 minutes and can be used once.",
			Button:  "Reset password",
			Outro:   "If you did not request this, ignore this email; your password stays unchanged.",
		},
	},
}

var htmlTpl = htmltemplate.Must(htmltemplate.New("mail").Parse(`<!doctype html>
<html><body style="font-family:Arial,Helvetica,sans-serif;color:#1e211f;line-height:1.5">
<p>{{.Greet}}</p>
<p>{{.Intro}}</p>
<p><a href="{{.Link}}" style="display:inline-block;padding:10px 18px;background:#1d4ed8;color:#fff;text-decoration:none;border-radius:6px">{{.Button}}</a></p>
<p style="font-size:12px;color:#555">{{.Link}}</p>
<p style="font-size:12px;color:#555">{{.Outro}}</p>
</body></html>`))

// render menyusun Message dari teks + tautan. name dan link di-escape pada bagian HTML (html/template).
func render(kind, lang, to, name, link string) (Message, error) {
	c, ok := texts[kind][normLang(lang)]
	if !ok {
		return Message{}, fmt.Errorf("jenis email %q tidak dikenal", kind)
	}
	if !strings.HasPrefix(link, "https://") && !strings.HasPrefix(link, "http://") {
		return Message{}, fmt.Errorf("tautan harus http(s)")
	}
	greet := fmt.Sprintf(c.Greet, strings.TrimSpace(name))
	text := greet + "\n\n" + c.Intro + "\n\n" + link + "\n\n" + c.Outro + "\n"

	var buf bytes.Buffer
	err := htmlTpl.Execute(&buf, map[string]any{
		"Greet": greet, "Intro": c.Intro, "Button": c.Button, "Outro": c.Outro,
		"Link": htmltemplate.URL(link), // sudah divalidasi skema; atribut href tetap di-escape
	})
	if err != nil {
		return Message{}, err
	}
	return Message{To: to, Subject: c.Subject, Text: text, HTML: buf.String()}, nil
}

// VerifyEmail = email verifikasi alamat. link = URL halaman verifikasi lengkap dengan token.
func VerifyEmail(lang, to, name, link string) (Message, error) {
	return render("verify", lang, to, name, link)
}

// ResetPassword = email atur ulang password.
func ResetPassword(lang, to, name, link string) (Message, error) {
	return render("reset", lang, to, name, link)
}
