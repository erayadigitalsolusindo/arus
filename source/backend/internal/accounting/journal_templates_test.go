package accounting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"aciraba/internal/accounting"
)

func TestJournalTemplates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := accounting.TemplateInput{Name: "Bayar gaji", Type: "KK", Narration: "Gaji bulanan", Lines: []accounting.TemplateLineInput{
		{AccountID: e.acc["6100"], Side: "debit"}, {AccountID: e.acc["1110"], Side: "credit", Memo: "dari kas"}}}

	tpl, err := e.svc.SaveTemplate(ctx, e.owner, uuid.Nil, in)
	if err != nil || len(tpl.Lines) != 2 || tpl.Lines[0].AccountCode != "6100" || tpl.Lines[1].Memo != "dari kas" {
		t.Fatalf("buat template: %+v, err %v", tpl, err)
	}
	// Nama unik per tenant (tanpa membedakan huruf besar/kecil).
	dup := in
	dup.Name = "BAYAR GAJI"
	if _, err := e.svc.SaveTemplate(ctx, e.owner, uuid.Nil, dup); !errors.Is(err, accounting.ErrTemplateNameTaken) {
		t.Errorf("nama ganda: err = %v", err)
	}
	// Ubah mengganti seluruh isi.
	in.Lines = []accounting.TemplateLineInput{{AccountID: e.acc["6100"], Side: "debit"}, {AccountID: e.acc["6100"], Side: "debit"}, {AccountID: e.acc["1110"], Side: "credit"}}
	in.Name = "Gaji"
	up, err := e.svc.SaveTemplate(ctx, e.owner, tpl.ID, in)
	if err != nil || up.Name != "Gaji" || len(up.Lines) != 3 {
		t.Fatalf("ubah template: %+v, err %v", up, err)
	}
	// Validasi: harus ada dua sisi, akun harus akun buku aktif.
	bad := accounting.TemplateInput{Name: "x", Type: "JU", Lines: []accounting.TemplateLineInput{{AccountID: e.acc["6100"], Side: "debit"}, {AccountID: e.acc["1110"], Side: "debit"}}}
	var fe accounting.FieldErrors
	if _, err := e.svc.SaveTemplate(ctx, e.owner, uuid.Nil, bad); !errors.As(err, &fe) {
		t.Errorf("satu sisi saja: err = %v", err)
	}
	bad.Lines[1] = accounting.TemplateLineInput{AccountID: uuid.New(), Side: "credit"}
	if _, err := e.svc.SaveTemplate(ctx, e.owner, uuid.Nil, bad); !errors.Is(err, accounting.ErrAccountInvalid) {
		t.Errorf("akun tak dikenal: err = %v", err)
	}
	// Isolasi tenant: tenant lain tidak melihat maupun menghapusnya.
	if list, err := e.svc.ListTemplates(ctx, e.otherOwner); err != nil || len(list) != 0 {
		t.Errorf("tenant lain melihat %d template (err %v)", len(list), err)
	}
	if err := e.svc.DeleteTemplate(ctx, e.otherOwner, tpl.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("hapus lintas tenant: err = %v", err)
	}
	if err := e.svc.DeleteTemplate(ctx, e.owner, tpl.ID); err != nil {
		t.Errorf("hapus: %v", err)
	}
	if list, _ := e.svc.ListTemplates(ctx, e.owner); len(list) != 0 {
		t.Errorf("setelah hapus masih %d template", len(list))
	}
}
