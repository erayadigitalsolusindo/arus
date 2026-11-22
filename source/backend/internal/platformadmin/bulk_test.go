package platformadmin

import "testing"

func TestCleanBulk(t *testing.T) {
	ok := BulkTenantInput{BusinessName: "Warung Ani", Email: " Ani@Gmail.com ", OwnerName: "Ani", OutletName: "Pusat", Phone: "0812-3456-7890"}
	c, f, _ := cleanBulk(ok)
	if f != "" || c.Email != "ani@gmail.com" || c.Phone != "+6281234567890" {
		t.Fatalf("valid: %+v field=%q", c, f)
	}
	ok.Phone = ""
	if _, f, _ := cleanBulk(ok); f != "" {
		t.Fatalf("telepon kosong harus boleh, field=%q", f)
	}
	for field, mut := range map[string]func(*BulkTenantInput){
		"business_name": func(i *BulkTenantInput) { i.BusinessName = "" },
		"email":         func(i *BulkTenantInput) { i.Email = "bukan-email" },
		"phone":         func(i *BulkTenantInput) { i.Phone = "abc" },
	} {
		in := ok
		in.Phone = "081234567890"
		mut(&in)
		if _, f, code := cleanBulk(in); f != field || code == "" {
			t.Errorf("%s: field=%q code=%q", field, f, code)
		}
	}
}
