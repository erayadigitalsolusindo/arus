package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
)

func TestValidateRegister(t *testing.T) {
	ok := RegisterInput{BusinessName: " Toko  Maju ", OwnerName: "Budi", Email: "BUDI@Toko.id", Phone: "0812 3456 7890", OutletName: "Pusat", Password: "sandi-aman-123", AcceptTerms: true}
	c, f := ValidateRegister(ok)
	if f != nil || c.BusinessName != "Toko Maju" || c.Email != "budi@toko.id" || c.Phone != "+6281234567890" {
		t.Fatalf("input valid ditolak/dinormalkan salah: %+v %v", c, f)
	}

	bad := RegisterInput{BusinessName: "<img src=x onerror=alert(1)>", OwnerName: "", Email: "bukan-email", Phone: "abc", OutletName: "Pusat\x00", Password: "pendek"}
	_, f = ValidateRegister(bad)
	for _, k := range []string{"business_name", "owner_name", "email", "phone", "outlet_name", "password", "accept_terms"} {
		if f[k] == "" {
			t.Errorf("field %s seharusnya ditolak", k)
		}
	}
}

func TestRegisterCreatesWorkspaceAndRejectsDuplicateEmail(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	email := fmt.Sprintf("uji-%d@register.test", os.Getpid())
	cleanup(t, pool, "%@register.test")

	sess, err := svc.Register(ctx, input("Satu", email), "id")
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" || sess.Tenant.Code == "" || sess.EmailVerified {
		t.Fatalf("sesi tidak lengkap: %+v", sess)
	}
	if n := count(t, pool, `SELECT count(*) FROM users u JOIN roles r ON r.id = u.role_id AND r.name = 'Owner' WHERE u.email = $1 AND u.password_hash LIKE '$argon2id$%' AND u.terms_version = $2 AND u.terms_accepted_at IS NOT NULL`, email, TermsVersion); n != 1 {
		t.Fatalf("owner user = %d, want 1 (dengan persetujuan syarat tercatat)", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM outlets WHERE tenant_id = $1`, sess.Tenant.ID); n != 1 {
		t.Fatalf("outlets = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'auth.register'`, sess.Tenant.ID); n != 1 {
		t.Fatalf("audit register = %d, want 1", n)
	}

	// Email sama ditolak, dan tidak meninggalkan tenant yatim (atomik).
	dup := input("Dua", email)
	if _, err := svc.Register(ctx, dup, "id"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM tenants WHERE name = 'UJI-Dua'`); n != 0 {
		t.Fatalf("tenant yatim = %d, want 0 (transaksi harus rollback)", n)
	}
}

func TestRegisterConcurrentSameEmail(t *testing.T) {
	svc, pool := newTestService(t)
	email := fmt.Sprintf("balap-%d@register.test", os.Getpid())
	cleanup(t, pool, "%@register.test")

	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Register(context.Background(), input(fmt.Sprintf("Balap%d", i), email), "id")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var ok, taken int
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrEmailTaken):
			taken++
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 || taken != 5 {
		t.Fatalf("ok=%d taken=%d, want 1 dan 5", ok, taken)
	}
	if n := count(t, pool, `SELECT count(*) FROM tenants WHERE name LIKE 'UJI-Balap%'`); n != 1 {
		t.Fatalf("tenants = %d, want 1", n)
	}
}
