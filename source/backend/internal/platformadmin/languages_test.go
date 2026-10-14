package platformadmin

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Invariant fitur languages:
//  1. Seed nilai default selalu ada: id & en dengan protected = true, aktif, urut 1 lalu 2.
//  2. Kode duplikat ditolak (409 CODE_TAKEN).
//  3. Bahasa protected tidak dapat dihapus (409 PROTECTED); yang non-protected bisa.
//  4. Update hanya mengubah nama/aktif/urutan; code/protected tidak dapat diubah.
//  5. Role aplikasi tidak dapat menulis platform_languages langsung (hanya lewat fungsi).

// mustActor membuat Platform Admin uji lalu mengembalikan Actor-nya (platform_assert_admin butuh admin aktif).
func mustActor(t *testing.T, e *env) Actor {
	t.Helper()
	id, _, _ := e.newAdmin(t, "Pemilik")
	return Actor{ID: id, Name: "Pemilik"}
}

func TestLanguagesListSeed(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()
	langs, err := e.svc.ListLanguages(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) < 2 {
		t.Fatalf("harus ada minimal id+en, ada %d", len(langs))
	}
	byCode := map[string]Language{}
	for _, l := range langs {
		byCode[l.Code] = l
	}
	for _, code := range []string{"id", "en"} {
		l, ok := byCode[code]
		if !ok {
			t.Fatalf("bahasa %s tidak ada di seed", code)
		}
		if !l.Protected || !l.Active {
			t.Errorf("%s harus protected & aktif: %+v", code, l)
		}
	}
	if byCode["id"].SortOrder != 1 || byCode["en"].SortOrder != 2 {
		t.Errorf("urutan seed: id=%d en=%d, want 1/2", byCode["id"].SortOrder, byCode["en"].SortOrder)
	}
	// Urut menurut sort_order, code.
	last := 0
	for _, l := range langs {
		if l.SortOrder < last {
			t.Error("daftar tidak urut sort_order")
		}
		last = l.SortOrder
	}
}

func TestLanguagesCRUD(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()
	const (
		code = "ar"
		name = "العربية"
	)

	// Tambah: 201 dan muncul di daftar.
	if _, err := e.svc.CreateLanguage(ctx, a, code, name, "AR", "ar-SA", 3); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = e.svc.DeleteLanguage(context.Background(), a, code) })

	langs, err := e.svc.ListLanguages(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range langs {
		if l.Code == code {
			found = true
			if l.Protected || !l.Active {
				t.Errorf("bahasa baru harus non-protected & aktif: %+v", l)
			}
		}
	}
	if !found {
		t.Fatal("bahasa baru tidak muncul di daftar")
	}

	// Kode duplikat ditolak.
	if _, err := e.svc.CreateLanguage(ctx, a, code, "lagi", "AR", "ar-SA", 4); err != ErrLanguageTaken {
		t.Fatalf("duplikat: %v, want ErrLanguageTaken", err)
	}
	// Kode salah huruf besar ditolak.
	if _, err := e.svc.CreateLanguage(ctx, a, "US", "United States", "US", "en-US", 9); err == nil {
		t.Fatal("kode huruf besar harus ditolak")
	}
}

// TestLanguagesAktorBukanAdmin — fungsi DB menolak pelaku yang bukan Platform Admin aktif (42501 → ErrInvalidSession).
func TestLanguagesAktorBukanAdmin(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	if _, err := e.svc.CreateLanguage(ctx, Actor{ID: uuid.New(), Name: "Bukan Admin"}, "zz", "Zulu", "ZZ", "zz-ZZ", 10); err != ErrInvalidSession {
		t.Fatalf("create: %v, want ErrInvalidSession", err)
	}
	if err := e.svc.UpdateLanguage(ctx, Actor{ID: uuid.New(), Name: "Bukan Admin"}, "en", "English", true, 2); err != ErrInvalidSession {
		t.Fatalf("update: %v, want ErrInvalidSession", err)
	}
	if err := e.svc.DeleteLanguage(ctx, Actor{ID: uuid.New(), Name: "Bukan Admin"}, "fr"); err != ErrInvalidSession {
		t.Fatalf("delete: %v, want ErrInvalidSession", err)
	}
}

func TestLanguagesUpdateDelete(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()

	// Update bahasa buatan sendiri (non-protected): nama/aktif/urutan berubah; code & protected tetap.
	const code = "fr"
	if _, err := e.svc.CreateLanguage(ctx, a, code, "Français", "FR", "fr-FR", 40); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = e.svc.DeleteLanguage(context.Background(), a, code) })

	if err := e.svc.UpdateLanguage(ctx, a, code, "French", true, 41); err != nil {
		t.Fatalf("update: %v", err)
	}
	langs, _ := e.svc.ListLanguages(ctx, a)
	for _, l := range langs {
		if l.Code == code {
			if l.Name != "French" || l.SortOrder != 41 || !l.Active {
				t.Errorf("hasil update: %+v", l)
			}
		}
	}

	// Hapus bahasa protected (id/en) ditolak; tidak mengubah data seed.
	if err := e.svc.DeleteLanguage(ctx, a, "id"); err != ErrLanguageProtected {
		t.Fatalf("hapus id: %v, want ErrLanguageProtected", err)
	}
	if err := e.svc.DeleteLanguage(ctx, a, "en"); err != ErrLanguageProtected {
		t.Fatalf("hapus en: %v, want ErrLanguageProtected", err)
	}

	// Hapus bahasa non-protected berhasil; hilang dari daftar.
	if err := e.svc.DeleteLanguage(ctx, a, code); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, l := range mustList(t, e, a) {
		if l.Code == code {
			t.Error("bahasa tetap ada setelah dihapus")
		}
	}
}

func TestLanguagesRoleAplikasiTidakBisaMenulis(t *testing.T) {
	e := newEnv(t, "")
	ctx := context.Background()
	if _, err := e.app.Exec(ctx, `INSERT INTO platform_languages (code, name, short, intl) VALUES ('xx', 'X', 'XX', 'xx-XX')`); err == nil {
		t.Error("aciraba_app seharusnya tidak boleh INSERT ke platform_languages")
	}
	if _, err := e.app.Exec(ctx, `UPDATE platform_languages SET name = 'X'`); err == nil {
		t.Error("aciraba_app seharusnya tidak boleh UPDATE platform_languages")
	}
	if _, err := e.app.Exec(ctx, `DELETE FROM platform_languages`); err == nil {
		t.Error("aciraba_app seharusnya tidak boleh DELETE platform_languages")
	}
}

func mustList(t *testing.T, e *env, a Actor) []Language {
	t.Helper()
	l, err := e.svc.ListLanguages(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// TestLanguagesHTTP memastikan rute platform languages terproteksi (2FA aktif) dan membaca daftar.
func TestLanguagesHTTP(t *testing.T) {
	e := newEnv(t, "")
	_, email, pw := e.newAdmin(t, "Pemilik")
	tok := e.loginToken(t, email, pw)

	if rec := e.do("GET", "/platform/languages", tok, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":"id"`) {
		t.Fatalf("daftar bahasa: %d %s", rec.Code, rec.Body)
	}
}
