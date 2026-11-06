package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateBaseURL(t *testing.T) {
	ok := map[string]string{
		"https://free.openkey.id/v1":      "https://free.openkey.id/v1",
		"  https://api.example.com/v1/ ":  "https://api.example.com/v1",
		"https://api.example.com:8443/v1": "https://api.example.com:8443/v1",
		"https://8.8.8.8/v1":              "https://8.8.8.8/v1",
	}
	for in, want := range ok {
		got, err := validateBaseURL(in)
		if err != nil || got != want {
			t.Errorf("%q → %q, %v (mau %q)", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "http://free.openkey.id/v1", "ftp://x.com/v1", "free.openkey.id/v1", "https://", "https://user:pw@x.com/v1", "https://x.com/v1?key=1", "https://x.com/v1#f",
		"https://localhost/v1", "https://app.localhost/v1", "https://db.internal/v1", "https://x.com:0/v1", "https://x.com:999999/v1", "https://x.com/a b", `https://x.com\@evil.com/v1`,
		"javascript:alert(1)", "https://" + strings.Repeat("a", 300) + ".com/v1",
	} {
		if got, err := validateBaseURL(in); err == nil {
			t.Errorf("%q harus ditolak, got %q", in, got)
		}
	}
}

func TestBlockedIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.5", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1", "::1", "fe80::1", "fc00::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "224.0.0.1", "240.0.0.1", "198.18.0.1", "255.255.255.255", "64:ff9b::1"} {
		if !blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s harus diblokir", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "104.16.0.1", "2606:4700:4700::1111"} {
		if blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s harus diizinkan", s)
		}
	}
}

// Pertahanan utama: klien aman menolak MENYAMBUNG ke alamat internal walau URL lolos validasi bentuk (mis. DNS menunjuk ke IP privat).
func TestSafeClientRefusesPrivateAndRedirects(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	resp, err := safeHTTPClient().Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("koneksi ke 127.0.0.1 harus ditolak")
	}
	if hit {
		t.Fatal("permintaan tidak boleh sampai ke server internal")
	}
	// Redirect tidak diikuti (respons 302 dikembalikan apa adanya).
	c := safeHTTPClient()
	if c.CheckRedirect == nil || c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect tidak boleh diikuti")
	}
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("proxy lingkungan tidak boleh dipakai")
	}
}

func TestSpecOfCustom(t *testing.T) {
	if _, ok := specOf("custom_ab12cd34"); !ok {
		t.Fatal("id kustom sah harus dikenal")
	}
	for _, bad := range []string{"custom_", "custom_ABCDEFGH", "custom_ab12cd3", "custom_ab12cd345", "custom_ab12cd3!", "xcustom_ab12cd34", "custom_ab12cd34\n"} {
		if _, ok := specOf(bad); ok || IsCustom(bad) {
			t.Errorf("%q harus ditolak", bad)
		}
	}
	sp, _ := specOf("custom_ab12cd34")
	for _, m := range []string{"sonnet-5", "gpt-4o-mini", "meta-llama/Llama-3.3-70B-Instruct", "qwen3:32b"} {
		if !sp.validModel(m) {
			t.Errorf("model %q harus sah", m)
		}
	}
	for _, m := range []string{"", " ", "a b", "-x", "m;rm -rf", "x\ny", strings.Repeat("a", 101)} {
		if sp.validModel(m) {
			t.Errorf("model %q harus ditolak", m)
		}
	}
	if customKeyOK("short") || customKeyOK("has space in it") || !customKeyOK("sk-0123456789") {
		t.Error("aturan key kustom")
	}
}

// Integrasi (tenant sementara): simpan terenkripsi, verifikasi ke endpoint, ganti base URL wajib key baru, batas jumlah, chat lewat endpoint kustom.
func TestCustomProviderFlow(t *testing.T) {
	e := newActEnv(t)
	ctx := context.Background()

	var sawAuth []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = append(sawAuth, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer KEY-OPENKEY-123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key","code":"invalid_api_key"}}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "sonnet-5" && req["model"] != "model-b" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"halo dari kustom"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer srv.Close()
	origClient, origResolve := newCustomClient, checkResolve
	newCustomClient = func(base string) *openaiClient {
		return &openaiClient{http: srv.Client(), base: base, label: "T", probe: true}
	}
	checkResolve = func(context.Context, string) error { return nil }
	t.Cleanup(func() { newCustomClient, checkResolve = origClient, origResolve })

	base := srv.URL + "/v1"
	in := CustomInput{Label: "OpenKey", BaseURL: base, Model: "sonnet-5", APIKey: "KEY-OPENKEY-123"}

	// Isian salah / key ditolak / model tak ada → tidak ada yang tersimpan.
	for name, bad := range map[string]CustomInput{
		"label kosong": {Label: " ", BaseURL: base, Model: "sonnet-5", APIKey: "KEY-OPENKEY-123"},
		"model aneh":   {Label: "X", BaseURL: base, Model: "a b", APIKey: "KEY-OPENKEY-123"},
		"http biasa":   {Label: "X", BaseURL: strings.Replace(base, "https", "http", 1), Model: "sonnet-5", APIKey: "KEY-OPENKEY-123"},
		"id sembarang": {ID: "custom_ZZ", Label: "X", BaseURL: base, Model: "sonnet-5", APIKey: "KEY-OPENKEY-123"},
	} {
		if _, err := e.svc.SaveCustom(ctx, e.cashier, bad); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{Label: "X", BaseURL: base, Model: "sonnet-5", APIKey: "KEY-SALAH-12345"}); err != ErrBadKey {
		t.Fatalf("key ditolak endpoint: %v", err)
	}
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{Label: "X", BaseURL: base, Model: "tidak-ada", APIKey: "KEY-OPENKEY-123"}); err == nil {
		t.Fatal("model yang tidak ada di endpoint harus ditolak")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_settings WHERE tenant_id = $1`, e.tenant); n != 0 {
		t.Fatalf("gagal verifikasi tidak boleh menyimpan, ada %d", n)
	}

	id, err := e.svc.SaveCustom(ctx, e.cashier, in)
	if err != nil || !IsCustom(id) {
		t.Fatalf("simpan: %q %v", id, err)
	}
	var enc string
	if err := e.admin.QueryRow(ctx, `SELECT api_key_enc FROM ai_settings WHERE tenant_id = $1 AND provider = $2`, e.tenant, id).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if enc == "" || strings.Contains(enc, "KEY-OPENKEY") {
		t.Fatal("key harus tersimpan terenkripsi, bukan plaintext")
	}
	st, err := e.svc.Settings(ctx, e.cashier)
	if err != nil || st.Provider != id || !st.Configured || !st.Active {
		t.Fatalf("settings: %+v %v", st, err)
	}
	var info *ProviderInfo
	for i := range st.Providers {
		if st.Providers[i].ID == id {
			info = &st.Providers[i]
		}
	}
	if info == nil || !info.Custom || info.Label != "OpenKey" || info.BaseURL != base || info.Model != "sonnet-5" || info.KeyHint != "-123" {
		t.Fatalf("info penyedia kustom: %+v", info)
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), "KEY-OPENKEY") {
		t.Fatal("key tidak boleh ikut ke klien")
	}

	// Chat sungguhan lewat endpoint kustom; key dikirim sebagai Bearer.
	res, err := e.svc.Chat(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "Halo!"}})
	if err != nil || res.Reply != "halo dari kustom" || res.Provider != id {
		t.Fatalf("chat: %+v %v", res, err)
	}
	if got := sawAuth[len(sawAuth)-1]; got != "Bearer KEY-OPENKEY-123" {
		t.Fatalf("Authorization = %q", got)
	}
	// Foto nota tidak didukung penyedia kustom.
	if _, err := e.svc.Chat(ctx, e.cashier, []ChatMessage{{Role: "user", Content: "x", Images: []ChatImage{{MediaType: "image/png", Data: tinyPNG}}}}); err != ErrImageProvider {
		t.Fatalf("foto di penyedia kustom: %v", err)
	}

	// Ubah model tanpa key (base sama) boleh; ganti base URL tanpa key baru ditolak (key lama tak boleh pindah host).
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{ID: id, Label: "OpenKey 2", BaseURL: base, Model: "model-b"}); err != nil {
		t.Fatalf("ubah model: %v", err)
	}
	if c, err := e.svc.loadCreds(ctx, e.cashier); err != nil || c.model != "model-b" || c.baseURL != base || c.key != "KEY-OPENKEY-123" {
		t.Fatalf("creds setelah ubah: %+v %v", c, err)
	}
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{ID: id, Label: "OpenKey", BaseURL: base + "/lain", Model: "model-b"}); err != ErrBadKey {
		t.Fatalf("ganti base tanpa key: %v", err)
	}
	if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{ID: "custom_00000000", Label: "X", BaseURL: base, Model: "model-b"}); err != ErrNotConfigured && err != ErrBadKey {
		t.Fatalf("id tak ada: %v", err)
	}
	// Key penyedia kustom tidak boleh disimpan lewat jalur Save biasa.
	if err := e.svc.Save(ctx, e.cashier, id, "KEY-OPENKEY-123", "model-b"); err != ErrBadInput {
		t.Fatalf("Save biasa dengan key kustom: %v", err)
	}
	// Tenant lain tidak melihat penyedia ini (RLS).
	other := e.cashier
	other.TenantID = uuid.New()
	if st2, err := e.svc.Settings(ctx, other); err != nil || len(st2.Providers) != len(providerSpecs) {
		t.Fatalf("tenant lain: %+v %v", st2.Providers, err)
	}

	// Batas jumlah penyedia kustom.
	for i := 1; i < maxCustomProviders; i++ {
		if _, err := e.svc.SaveCustom(ctx, e.cashier, CustomInput{Label: "P", BaseURL: base, Model: "sonnet-5", APIKey: "KEY-OPENKEY-123"}); err != nil {
			t.Fatalf("penyedia ke-%d: %v", i+1, err)
		}
	}
	if _, err := e.svc.SaveCustom(ctx, e.cashier, in); err != ErrTooManyCustom {
		t.Fatalf("melebihi batas: %v", err)
	}
	// Tepat satu aktif; hapus yang aktif → yang lain menggantikan; aktifkan lagi tanpa key baru.
	if n := e.count(t, `SELECT count(*) FROM ai_settings WHERE tenant_id = $1 AND active`, e.tenant); n != 1 {
		t.Fatalf("aktif harus tepat 1, got %d", n)
	}
	if err := e.svc.Save(ctx, e.cashier, id, "", "model-b"); err != nil {
		t.Fatalf("aktifkan tanpa key: %v", err)
	}
	if err := e.svc.RemoveKey(ctx, e.cashier, id); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_settings WHERE tenant_id = $1 AND active`, e.tenant); n != 1 {
		t.Fatalf("setelah hapus aktif: harus ada 1 pengganti, got %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.settings'`, e.tenant); n < 2 {
		t.Fatalf("perubahan harus diaudit, got %d", n)
	}
}
