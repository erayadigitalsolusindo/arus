package platformadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Invariant checklist QA:
//  1. Kunci item katalog unik dan berpola "<aplikasi>.…"; tiap kelompok punya item.
//  2. Input tidak sah (kunci tak dikenal, status asing, catatan kepanjangan) ditolak sebelum menyentuh DB.
//  3. Hasil tersimpan per run; status "todo" menghapus; run tertutup menolak perubahan.

func TestQACatalogKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range qaCatalog {
		if len(a.Groups) == 0 {
			t.Errorf("aplikasi %s tanpa kelompok", a.Key)
		}
		for _, g := range a.Groups {
			if len(g.Items) == 0 {
				t.Errorf("kelompok %s/%s kosong", a.Key, g.Key)
			}
			for _, it := range g.Items {
				if seen[it.Key] {
					t.Errorf("kunci item ganda: %s", it.Key)
				}
				seen[it.Key] = true
				if strings.TrimSpace(it.Title) == "" || len(it.Key) > 80 {
					t.Errorf("item %q tidak sah", it.Key)
				}
			}
		}
	}
	if len(seen) != qaTotal() {
		t.Errorf("qaTotal=%d, katalog=%d", qaTotal(), len(seen))
	}
}

func TestSetQAResultRejectsInvalidInput(t *testing.T) {
	s := &Service{}
	ctx := context.Background()
	run := uuid.New()
	var anyKey string
	for k := range qaItemKeys {
		anyKey = k
		break
	}
	cases := map[string][3]string{
		"kunci asing":     {"tidak.ada", "pass", ""},
		"status asing":    {anyKey, "done", ""},
		"catatan panjang": {anyKey, "fail", strings.Repeat("x", qaMaxNote+1)},
	}
	for name, c := range cases {
		if err := s.SetQAResult(ctx, Actor{}, run, c[0], c[1], c[2]); !errors.Is(err, ErrQAInvalid) {
			t.Errorf("%s: err=%v, want ErrQAInvalid", name, err)
		}
	}
	if _, err := s.CreateQARun(ctx, Actor{}, "   "); !errors.Is(err, ErrQAInvalid) {
		t.Errorf("nama kosong: err=%v", err)
	}
}

func TestQARunLifecycle(t *testing.T) {
	e := newEnv(t, "")
	a := mustActor(t, e)
	ctx := context.Background()

	id, err := e.svc.CreateQARun(ctx, a, "Rilis uji")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.svc.DeleteQARun(context.Background(), a, id) })

	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.cash", "pass", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.void", "fail", "stok tidak kembali"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.cash", "fail", "ditimpa"); err != nil { // upsert
		t.Fatal(err)
	}
	runs, err := e.svc.ListQARuns(ctx, a, &id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	if r := runs[0]; r.Pass != 0 || r.Fail != 2 || r.Skip != 0 {
		t.Errorf("hitungan salah: %+v", r)
	}
	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.cash", "todo", ""); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.QAResults(ctx, a, id)
	if err != nil || len(res) != 1 || res[0].Key != "web.pos.void" || res[0].Note != "stok tidak kembali" {
		t.Fatalf("res=%+v err=%v", res, err)
	}

	if err := e.svc.CloseQARun(ctx, a, id, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.cash", "pass", ""); !errors.Is(err, ErrQAClosed) {
		t.Errorf("run tertutup harus menolak: %v", err)
	}
	if err := e.svc.CloseQARun(ctx, a, id, false); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetQAResult(ctx, a, id, "web.pos.cash", "skip", ""); err != nil {
		t.Errorf("run dibuka kembali harus menerima: %v", err)
	}
	if err := e.svc.SetQAResult(ctx, a, uuid.New(), "web.pos.cash", "pass", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("run tak ada: %v", err)
	}
}
