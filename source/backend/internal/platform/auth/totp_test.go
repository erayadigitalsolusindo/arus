package auth

import (
	"strings"
	"testing"
	"time"
)

// Vektor uji RFC 6238 (SHA1, rahasia ASCII "12345678901234567890"; di RFC 8 digit, di sini 6 digit terakhirnya).
func TestTOTPRFCVectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		got, err := totpAt(secret, c.unix/30)
		if err != nil || got != c.want {
			t.Errorf("t=%d: got %q err %v, want %q", c.unix, got, err, c.want)
		}
	}
}

func TestVerifyTOTPSkewAndStep(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_010, 0)
	cur, _ := totpAt(secret, TOTPStep(now))
	prev, _ := totpAt(secret, TOTPStep(now)-1)
	old, _ := totpAt(secret, TOTPStep(now)-5)
	if step, ok := VerifyTOTP(secret, cur, now); !ok || step != TOTPStep(now) {
		t.Errorf("kode sekarang: step %d ok %v", step, ok)
	}
	if step, ok := VerifyTOTP(secret, prev, now); !ok || step != TOTPStep(now)-1 {
		t.Errorf("kode periode lalu (toleransi jam): step %d ok %v", step, ok)
	}
	if _, ok := VerifyTOTP(secret, old, now); ok {
		t.Error("kode 5 periode lalu harus ditolak")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := VerifyTOTP(secret, bad, now); ok {
			t.Errorf("kode %q seharusnya ditolak", bad)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes, err := NewRecoveryCodes(10)
	if err != nil || len(codes) != 10 || len(hashes) != 10 {
		t.Fatalf("codes=%d hashes=%d err=%v", len(codes), len(hashes), err)
	}
	seen := map[string]bool{}
	for i, c := range codes {
		n, ok := NormalizeRecovery(strings.ToLower(c))
		if !ok || RecoveryHash(n) != hashes[i] || seen[c] {
			t.Errorf("kode %q tidak konsisten/duplikat", c)
		}
		seen[c] = true
	}
	if _, ok := NormalizeRecovery("bukan-kode!"); ok {
		t.Error("input acak harus ditolak")
	}
}

func TestTOTPBoxBindsToAccount(t *testing.T) {
	box, err := NewTOTPBox("rahasia-uji-rahasia-uji-rahasia-uji-123")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("JBSWY3DPEHPK3PXP", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := box.Open(sealed, "admin-1"); err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("open: %q %v", got, err)
	}
	if _, err := box.Open(sealed, "admin-2"); err == nil {
		t.Error("ciphertext tidak boleh bisa dibuka untuk akun lain")
	}
}
