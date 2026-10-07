package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

const testPassword = "sandi-aman-123"

func registerUser(t *testing.T, svc *Service, name string) (email string) {
	t.Helper()
	email = fmt.Sprintf("%s-%d@login.test", name, os.Getpid())
	if _, err := svc.Register(context.Background(), input(name, email)); err != nil {
		t.Fatal(err)
	}
	return email
}

func TestLogin(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	cleanup(t, pool, "%@login.test")
	email := registerUser(t, svc, "Login")

	sess, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword, Remember: true})
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" || !sess.Remember || sess.User.Email != email || sess.Outlet.Code != "main" {
		t.Fatalf("sesi login tidak lengkap: %+v", sess)
	}
	claims, err := svc.tokens.Parse(sess.AccessToken)
	if err != nil || claims.Subject != sess.User.ID || claims.TenantID != sess.Tenant.ID || claims.OutletID != sess.Outlet.ID || claims.Role != "Owner" {
		t.Fatalf("klaim token salah: %+v %v", claims, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM users WHERE email = $1 AND last_login_at IS NOT NULL`, email); n != 1 {
		t.Fatalf("last_login_at tidak terisi")
	}

	// Email tidak peka huruf besar/kecil.
	if _, err := svc.Login(ctx, LoginInput{Email: fmt.Sprintf("LOGIN-%d@LOGIN.test", os.Getpid()), Password: testPassword}); err != nil {
		t.Fatalf("login email beda kapitalisasi: %v", err)
	}

	for name, in := range map[string]LoginInput{
		"password salah": {Email: email, Password: "salah-salah-123"},
		"email tak ada":  {Email: "tidak-ada@login.test", Password: testPassword},
	} {
		if _, err := svc.Login(ctx, in); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}

	// Akun nonaktif: hanya diketahui setelah password benar.
	if _, err := pool.Exec(ctx, `UPDATE users SET active = false WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword}); !errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("err = %v, want ErrAccountDisabled", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Email: email, Password: "salah-salah-123"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("password salah pada akun nonaktif harus tetap INVALID_CREDENTIALS, got %v", err)
	}
}

func TestRefreshRotationGraceAndReuse(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	cleanup(t, pool, "%@login.test")
	email := registerUser(t, svc, "Rotasi")

	first, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if first.Remember {
		t.Fatal("remember seharusnya false")
	}

	// Rotasi normal: token baru berbeda, sifat remember terbawa.
	second, err := svc.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == "" || second.RefreshToken == first.RefreshToken || second.Remember || second.AccessToken == "" {
		t.Fatalf("rotasi salah: %+v", second)
	}

	// Pemakaian ulang di jendela grace (dua tab bersamaan): token akses diberikan, tanpa token refresh baru.
	graced, err := svc.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("grace ditolak: %v", err)
	}
	if graced.RefreshToken != "" || graced.AccessToken == "" {
		t.Fatalf("grace harus tanpa refresh token baru: %+v", graced)
	}
	// Token terbaru tetap berlaku setelah grace.
	third, err := svc.Refresh(ctx, second.RefreshToken)
	if err != nil {
		t.Fatalf("token terbaru ditolak: %v", err)
	}

	// Pemakaian ulang di luar grace = pencurian: rantai dicabut, termasuk token terbaru.
	time.Sleep(11 * time.Second)
	if _, err := svc.Refresh(ctx, first.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("reuse err = %v, want ErrInvalidSession", err)
	}
	if _, err := svc.Refresh(ctx, third.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("rantai harus tercabut, err = %v", err)
	}
}

func TestRefreshConcurrentSingleWinner(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	cleanup(t, pool, "%@login.test")
	email := registerUser(t, svc, "Balap")
	sess, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword, Remember: true})
	if err != nil {
		t.Fatal(err)
	}

	const n = 8
	results := make(chan *Session, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := svc.Refresh(ctx, sess.RefreshToken)
			if err != nil {
				t.Errorf("refresh: %v", err)
			}
			results <- s
		}()
	}
	wg.Wait()
	close(results)
	rotated := 0
	for s := range results {
		if s != nil && s.RefreshToken != "" {
			rotated++
		}
	}
	if rotated != 1 {
		t.Fatalf("yang merotasi = %d, want tepat 1 (sisanya grace)", rotated)
	}
}

func TestLogoutAndDisabledRefresh(t *testing.T) {
	svc, pool := newTestService(t)
	ctx := context.Background()
	cleanup(t, pool, "%@login.test")
	email := registerUser(t, svc, "Keluar")

	sess, err := svc.Login(ctx, LoginInput{Email: email, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := svc.Refresh(ctx, sess.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, cur.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, cur.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("setelah logout err = %v, want ErrInvalidSession", err)
	}
	if err := svc.Logout(ctx, "token-tak-dikenal"); err != nil {
		t.Fatalf("logout token asing harus diam-diam sukses: %v", err)
	}

	// User dinonaktifkan setelah login: refresh berikutnya gagal.
	sess, err = svc.Login(ctx, LoginInput{Email: email, Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET active = false WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, sess.RefreshToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("refresh user nonaktif err = %v, want ErrInvalidSession", err)
	}
}
