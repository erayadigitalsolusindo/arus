package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Integrasi: API ARUS (akun gratis lewat gateway yang diatur operator). Memakai tenant dan admin SEMENTARA; tabel pengaturan hanya
// diuji bila masih kosong (tidak menimpa konfigurasi nyata) dan dikosongkan lagi sesudahnya.
func TestPlatformProviderFlow(t *testing.T) {
	e := newActEnv(t)
	ctx := context.Background()

	var n int
	if err := e.admin.QueryRow(ctx, `SELECT count(*) FROM ai_platform_provider`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > 0 {
		t.Skip("API ARUS sudah dikonfigurasi di DB ini; tes tidak menimpanya")
	}
	adminID := uuid.New()
	if _, err := e.admin.Exec(ctx, `INSERT INTO platform_admins (id, email, name, password_hash) VALUES ($1, $2, 'Uji ARUS', 'x')`, adminID, "uji-"+adminID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.admin.Exec(ctx, `DELETE FROM ai_platform_provider`)
		_, _ = e.admin.Exec(ctx, `DELETE FROM platform_admins WHERE id = $1`, adminID)
	})

	var fail atomic.Bool
	var sawAuth atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth.Store(r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer KEY-ARUS-12345" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key","code":"invalid_api_key"}}`))
			return
		}
		if fail.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limit gateway-rahasia"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"halo dari arus"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer srv.Close()
	origClient, origResolve := newCustomClient, checkResolve
	newCustomClient = func(base string) *openaiClient {
		return &openaiClient{http: srv.Client(), base: base, label: "T", probe: true}
	}
	checkResolve = func(context.Context, string) error { return nil }
	t.Cleanup(func() { newCustomClient, checkResolve = origClient, origResolve })

	base := srv.URL + "/v1"
	svc := *e.svc
	svc.free = Free{Limit: 3} // tanpa key env: hanya API ARUS yang bisa melayani akun gratis

	// Bukan admin aktif / key salah → tidak ada yang tersimpan.
	if err := svc.SavePlatformProvider(ctx, uuid.New(), PlatformProviderInput{BaseURL: base, Model: "m-gratis", APIKey: "KEY-ARUS-12345"}); err == nil {
		t.Fatal("bukan admin: harus ditolak")
	}
	if err := svc.SavePlatformProvider(ctx, adminID, PlatformProviderInput{BaseURL: base, Model: "m-gratis", APIKey: "KEY-SALAH-12345"}); !errors.Is(err, ErrBadKey) {
		t.Fatalf("key salah = %v, want ErrBadKey", err)
	}
	if err := svc.SavePlatformProvider(ctx, adminID, PlatformProviderInput{BaseURL: "http://x.test/v1", Model: "m-gratis", APIKey: "KEY-ARUS-12345"}); !errors.Is(err, ErrBadBaseURL) {
		t.Fatalf("http biasa = %v, want ErrBadBaseURL", err)
	}
	if got, _ := svc.GetPlatformProvider(ctx, adminID); got.Configured {
		t.Fatalf("setelah penolakan tidak boleh ada konfigurasi: %+v", got)
	}

	if err := svc.SavePlatformProvider(ctx, adminID, PlatformProviderInput{BaseURL: base, Model: "m-gratis", APIKey: "KEY-ARUS-12345"}); err != nil {
		t.Fatal(err)
	}
	var enc string
	if err := e.admin.QueryRow(ctx, `SELECT api_key_enc FROM ai_platform_provider`).Scan(&enc); err != nil || enc == "" || strings.Contains(enc, "KEY-ARUS-12345") {
		t.Fatalf("key harus terenkripsi: %q %v", enc, err)
	}
	if got, err := svc.GetPlatformProvider(ctx, adminID); err != nil || !got.Configured || got.BaseURL != base || got.Model != "m-gratis" || got.KeyHint != "2345" || got.Author != "Uji ARUS" {
		t.Fatalf("info = %+v %v", got, err)
	}
	// Ganti base URL tanpa key baru ditolak; base sama tanpa key = key lama dipertahankan.
	if err := svc.SavePlatformProvider(ctx, adminID, PlatformProviderInput{BaseURL: base + "/lain", Model: "m-gratis"}); !errors.Is(err, ErrBadKey) {
		t.Fatalf("ganti URL tanpa key = %v, want ErrBadKey", err)
	}
	if err := svc.SavePlatformProvider(ctx, adminID, PlatformProviderInput{BaseURL: base, Model: "m-gratis-2"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetPlatformProvider(ctx, adminID); got.Model != "m-gratis-2" || got.KeyHint != "2345" {
		t.Fatalf("setelah ubah model = %+v", got)
	}

	// Akun gratis dilayani lewat gateway; model dirahasiakan dari pelanggan; kuota berkurang.
	res, err := svc.ChatWith(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "halo"}}, ChatOptions{Temporary: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reply != "halo dari arus" || res.Provider != ProviderArus || res.Model != "" || res.Free == nil || !res.Free.Arus || res.Free.Model != "" || res.Free.Remaining != 2 {
		t.Fatalf("hasil = %+v free=%+v", res, res.Free)
	}
	if sawAuth.Load() != "Bearer KEY-ARUS-12345" {
		t.Fatalf("gateway menerima %v", sawAuth.Load())
	}

	// Gateway menolak (mis. rate limit) → pesan ramah tanpa bocoran, jatah dikembalikan.
	fail.Store(true)
	_, err = svc.ChatWith(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "lagi"}}, ChatOptions{Temporary: true})
	var up *upstreamError
	if !errors.As(err, &up) || up.Code != "AI_UNAVAILABLE" || strings.Contains(up.Message, "rahasia") || !strings.Contains(up.Message, "penuh") {
		t.Fatalf("galat gateway = %v", err)
	}
	st, err := svc.Settings(ctx, e.cashier)
	if err != nil || !st.Free.Arus || st.Free.Remaining != 2 || st.Free.Model != "" {
		t.Fatalf("setelah gagal: %+v %v", st.Free, err)
	}

	// Hapus → akun gratis mati (tanpa key env).
	if found, err := svc.ClearPlatformProvider(ctx, adminID); err != nil || !found {
		t.Fatalf("hapus = %v %v", found, err)
	}
	if st, _ := svc.Settings(ctx, e.cashier); st.Free.Enabled || st.Free.Arus {
		t.Fatalf("setelah hapus: %+v", st.Free)
	}
}
