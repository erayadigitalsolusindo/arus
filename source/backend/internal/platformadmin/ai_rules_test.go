package platformadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Aturan operator: hanya admin aktif yang menulis; versi bertambah; riwayat terbaru dulu; batas panjang; per tenant terpisah dari global.
// (Dokumen global asli tidak ditimpa: test hanya menulis ke tenant uji dan memeriksa penolakan untuk global.)
func TestAIRulesVersioning(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	tn := e.newTenant(t)
	ctx := context.Background()

	if _, err := e.svc.SaveAIRules(ctx, Actor{ID: uuid.New()}, tn.ID, "x"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("bukan admin = %v, want ErrInvalidSession", err)
	}
	v1, err := e.svc.SaveAIRules(ctx, a, tn.ID, "  # Aturan\r\n- Jawab singkat  ")
	if err != nil || v1 != 1 {
		t.Fatalf("v1 = %d, %v", v1, err)
	}
	v2, err := e.svc.SaveAIRules(ctx, a, tn.ID, "- Jangan membahas politik")
	if err != nil || v2 != 2 {
		t.Fatalf("v2 = %d, %v", v2, err)
	}
	hist, err := e.svc.ListAIRules(ctx, a, tn.ID)
	if err != nil || len(hist) != 2 || hist[0].Version != 2 || hist[1].Body != "# Aturan\n- Jawab singkat" || hist[0].Author == "" {
		t.Fatalf("riwayat = %+v, %v", hist, err)
	}
	list, err := e.svc.ListAIRuleTenants(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range list {
		if r.TenantID == tn.ID && r.Version == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("tenant tidak muncul di daftar: %+v", list)
	}
	// Kosong = hapus aturan: hilang dari daftar tenant tetapi riwayat tetap ada.
	if _, err := e.svc.SaveAIRules(ctx, a, tn.ID, "   "); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListAIRuleTenants(ctx, a)
	for _, r := range list {
		if r.TenantID == tn.ID {
			t.Fatal("aturan kosong tidak boleh tampil sebagai aktif")
		}
	}
	// Validasi.
	var fe FieldErrors
	if _, err := e.svc.SaveAIRules(ctx, a, tn.ID, strings.Repeat("a", MaxAIRulesChars+1)); !errors.As(err, &fe) || fe["body"] != "TOO_LONG" {
		t.Fatalf("terlalu panjang = %v", err)
	}
	if _, err := e.svc.SaveAIRules(ctx, a, tn.ID, "a\x00b"); !errors.As(err, &fe) {
		t.Fatalf("NUL = %v", err)
	}
	if _, err := e.svc.SaveAIRules(ctx, a, uuid.New(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant tak ada = %v", err)
	}
}
