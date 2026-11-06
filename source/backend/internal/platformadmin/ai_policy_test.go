package platformadmin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/aipolicy"
)

// Kebijakan HalloArus: hanya admin aktif yang menulis; pengecualian tenant disimpan/diubah/dihapus; global tak bisa dihapus
// dan tak boleh tidak lengkap; nilai di luar rentang ditolak. (Baris global asli tidak ditimpa oleh test.)
func TestAIPolicyOverrideCRUD(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	tn := e.newTenant(t)
	ctx := context.Background()
	f, five := false, 5

	v := aipolicy.Values{Enabled: &f, Domains: []string{"sales", "sales", "cash"}, ChatLimit: &five}
	if err := e.svc.SaveAIPolicy(ctx, Actor{ID: uuid.New()}, tn.ID, v, ""); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("bukan admin = %v, want ErrInvalidSession", err)
	}
	if err := e.svc.SaveAIPolicy(ctx, a, tn.ID, v, "uji"); err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.ListAIPolicies(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if list.Global.TenantID != nil || list.Global.Enabled == nil || list.Global.Domains == nil {
		t.Fatalf("global tidak lengkap: %+v", list.Global)
	}
	var got *AIPolicyRow
	for i := range list.Tenants {
		if list.Tenants[i].TenantID != nil && *list.Tenants[i].TenantID == tn.ID {
			got = &list.Tenants[i]
		}
	}
	if got == nil || got.Enabled == nil || *got.Enabled || got.AllowCost != nil || len(got.Domains) != 2 || *got.ChatLimit != 5 || got.Note != "uji" {
		t.Fatalf("pengecualian tersimpan = %+v", got)
	}
	// Ubah: kolom yang tidak dikirim kembali ke NULL (ikut global).
	if err := e.svc.SaveAIPolicy(ctx, a, tn.ID, aipolicy.Values{AllowCost: &f}, ""); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.ListAIPolicies(ctx, a)
	for _, r := range list.Tenants {
		if r.TenantID != nil && *r.TenantID == tn.ID && (r.Enabled != nil || r.AllowCost == nil || r.Domains != nil) {
			t.Fatalf("setelah ubah = %+v", r)
		}
	}
	// Validasi.
	var fe FieldErrors
	hundred, big := 999, 400
	if err := e.svc.SaveAIPolicy(ctx, a, tn.ID, aipolicy.Values{Domains: []string{"x"}, ChatLimit: &big, FreeDailyLimit: &hundred}, ""); !errors.As(err, &fe) || fe["domains"] == "" || fe["chat_limit"] == "" {
		t.Fatalf("validasi = %v", err)
	}
	if err := e.svc.SaveAIPolicy(ctx, a, uuid.Nil, aipolicy.Values{}, ""); !errors.As(err, &fe) {
		t.Fatalf("global tidak lengkap harus ditolak, got %v", err)
	}
	if err := e.svc.SaveAIPolicy(ctx, a, uuid.New(), aipolicy.Values{}, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tenant tak ada = %v", err)
	}
	// Hapus.
	if err := e.svc.DeleteAIPolicy(ctx, a, tn.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteAIPolicy(ctx, a, tn.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hapus kedua = %v", err)
	}
	if err := e.svc.DeleteAIPolicy(ctx, a, uuid.Nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("global tak boleh dihapus, got %v", err)
	}
}
