package storefront

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var classAttr = regexp.MustCompile(`class="([^"]*)"`)

// Template bawaan harus lolos penyaring tanpa peringatan, dan setiap class-nya harus ada: di stylesheet Tailwind hasil
// build, atau didefinisikan di CSS template itu sendiri (kalau tidak, bagian itu tampil polos).
func TestBuiltinTemplatesAreCleanAndStyled(t *testing.T) {
	css := string(tailwindCSS)
	for _, b := range builtins {
		if _, err := SanitizeCSS(b.CSS); err != nil {
			t.Errorf("%s: CSS ditolak: %v", b.Code, err)
		}
		if _, issues := SanitizeHTML(b.HTML, true); len(issues) != 0 {
			t.Errorf("%s: HTML memicu peringatan: %v", b.Code, issues)
		}
		if len(b.HTML) > 60000 || len(b.CSS) > 20000 {
			t.Errorf("%s: melebihi batas ukuran (%d / %d)", b.Code, len(b.HTML), len(b.CSS))
		}
		if _, err := parse(b.HTML); err != nil {
			t.Errorf("%s: %v", b.Code, err)
		}
		own := map[string]bool{}
		for _, m := range regexp.MustCompile(`\.([A-Za-z_][\w-]*)`).FindAllStringSubmatch(b.CSS, -1) {
			own[m[1]] = true
		}
		for _, m := range classAttr.FindAllStringSubmatch(b.HTML, -1) {
			for _, cl := range strings.Fields(m[1]) {
				esc := strings.NewReplacer(":", `\:`, "/", `\/`, ".", `\.`, "[", `\[`, "]", `\]`, "!", `\!`).Replace(cl)
				if !own[cl] && !strings.Contains(css, "."+esc) {
					t.Errorf("%s: class %q tidak ada di tailwind.css maupun CSS template", b.Code, cl)
				}
			}
		}
		out, err := Render(context.Background(), b.HTML, sampleSource{}, Site{Name: "Contoh", Slug: "contoh"})
		if err != nil {
			t.Fatal(err)
		}
		body, issues := SanitizeHTML(out, false)
		for _, is := range issues {
			if is != "atribut src pada <img> dibuang" { // barang contoh tanpa gambar: src kosong memang dibuang
				t.Errorf("%s: hasil render memicu peringatan: %v", b.Code, is)
			}
		}
		if strings.Contains(strings.ToLower(body), "<script") || strings.Contains(body, "{namatoko}") {
			t.Errorf("%s: hasil render tak bersih", b.Code)
		}
	}
	if len(builtins) < 2 || builtins[0].Code != "kopi-ekspor" {
		t.Fatal("template Kopi Ekspor harus ada")
	}
}

func TestPreviewSample(t *testing.T) {
	s := &Service{}
	doc, err := s.PreviewSample(context.Background(), `<h1 onclick="x()">{namatoko}</h1><script>1</script>{#barang limit=2}<p>{namabarang}</p>{/barang}`, `h1{color:red}`, "http://x.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<h1>Nusantara Coffee Export</h1>", "<p>Gayo Arabica Grade 1</p>", "<style>h1{color:red}</style>", `href="http://x.test/public/s-assets/tailwind.css?v=`} {
		if !strings.Contains(doc, want) {
			t.Errorf("tak memuat %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "onclick") || strings.Contains(doc, "<script") {
		t.Errorf("tidak tersaring: %s", doc)
	}
	if _, err := s.PreviewSample(context.Background(), "x", `a{background:url(x)}`, ""); err == nil {
		t.Error("CSS berbahaya harus ditolak")
	}
}

func TestTemplatesLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.svc.SeedTemplates(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SeedTemplates(ctx); err != nil { // idempoten
		t.Fatal(err)
	}
	list, err := e.svc.ListTemplates(ctx, true, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tp := range list {
		if tp.Code == "kopi-ekspor" {
			found = true
			if tp.HTML != "" || tp.Bytes < 1000 {
				t.Errorf("daftar tak boleh memuat isi: %+v", tp)
			}
		}
	}
	if !found {
		t.Fatal("kopi-ekspor tak ada")
	}

	// Tulis hanya oleh Platform Admin aktif.
	code := "uji-" + uuid.NewString()[:8]
	in := TemplateInput{Code: code, Name: "Uji", Category: "Uji", HTML: "<h1>{namatoko}</h1><script>x</script>", CSS: "h1{color:red}", Active: true, Sort: 999}
	if _, _, err := e.svc.SaveTemplate(ctx, uuid.New(), in); !errors.Is(err, ErrInvalidAdmin) {
		t.Fatalf("bukan admin = %v, want ErrInvalidAdmin", err)
	}
	admin := uuid.New()
	e.sql(t, `INSERT INTO platform_admins (id, email, name, password_hash) VALUES ($1, $2, 'Uji SF', 'x')`, admin, "uji-sf-"+admin.String()+"@example.test")
	t.Cleanup(func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM storefront_templates WHERE code = $1`, code)
		_, _ = e.admin.Exec(ctx, `DELETE FROM platform_admins WHERE id = $1`, admin)
	})
	if _, _, err := e.svc.SaveTemplate(ctx, admin, TemplateInput{Code: code, Name: "X", HTML: "x", CSS: "a{background:url(x)}"}); err == nil {
		t.Fatal("CSS berbahaya harus ditolak")
	}
	id, warn, err := e.svc.SaveTemplate(ctx, admin, in)
	if err != nil || len(warn) == 0 {
		t.Fatalf("simpan: %v warn=%v", err, warn)
	}
	if _, _, err := e.svc.SaveTemplate(ctx, admin, in); !errors.Is(err, ErrTemplateCodeTaken) {
		t.Fatalf("kode kembar = %v", err)
	}
	// Nonaktif: tak terlihat tenant, tetap terlihat admin.
	in.ID, in.Active = &id, false
	if _, _, err := e.svc.SaveTemplate(ctx, admin, in); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetTemplate(ctx, id, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("template nonaktif untuk tenant = %v", err)
	}
	if got, err := e.svc.GetTemplate(ctx, id, false); err != nil || got.Active || got.HTML == "" {
		t.Fatalf("admin: %+v %v", got, err)
	}
	if err := e.svc.DeleteTemplate(ctx, uuid.New(), id); !errors.Is(err, ErrInvalidAdmin) {
		t.Fatalf("hapus bukan admin = %v", err)
	}
	if err := e.svc.DeleteTemplate(ctx, admin, id); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteTemplate(ctx, admin, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hapus ulang = %v", err)
	}
	// Aplikasi (role runtime) tidak boleh menulis langsung.
	if _, err := e.svc.pool.Exec(ctx, `UPDATE storefront_templates SET name = 'x' WHERE code = 'kopi-ekspor'`); err == nil {
		t.Fatal("role aplikasi tidak boleh UPDATE storefront_templates langsung")
	}
}
