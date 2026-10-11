package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Percakapan berbentuk Anthropic (teks → tool_use → tool_result) untuk menguji terjemahan tiap penyedia.
func toolConversation() apiRequest {
	return apiRequest{
		Model: "m", MaxTokens: 100, System: "sys",
		Tools: []toolDef{{Name: "list_outlets", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}},
		Messages: []message{
			{Role: "user", Content: []block{{Type: "text", Text: "berapa outlet?"}}},
			{Role: "assistant", Content: []block{{Type: "tool_use", ID: "call_1", Name: "list_outlets", Input: json.RawMessage(`{}`), Sig: "SIG"}}},
			{Role: "user", Content: []block{{Type: "tool_result", ToolUseID: "call_1", ToolName: "list_outlets", Content: `{"rows":[]}`}}},
		},
	}
}

func TestOpenAIAdapter(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("content-type", "application/json")
		if strings.Contains(string(b), `"role":"tool"`) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Ada 2 outlet."}}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"list_outlets","arguments":"{}"}}]}}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	}))
	defer srv.Close()
	for _, maxField := range []string{"max_completion_tokens", "max_tokens"} {
		c := &openaiClient{http: srv.Client(), base: srv.URL, label: "T", maxCompletion: maxField == "max_completion_tokens"}
		req := toolConversation()
		req.Messages = req.Messages[:1]
		resp, err := c.send(context.Background(), "sk-test", req)
		if err != nil || resp.StopReason != "tool_use" || len(resp.Content) != 1 || resp.Content[0].Name != "list_outlets" || resp.Content[0].ID != "c1" {
			t.Fatalf("tool call: %+v %v", resp, err)
		}
		if auth != "Bearer sk-test" || got[maxField] == nil {
			t.Fatalf("header/%s: %q %v", maxField, auth, got)
		}
		resp, err = c.send(context.Background(), "sk-test", toolConversation())
		if err != nil || resp.StopReason != "end_turn" || resp.Content[0].Text != "Ada 2 outlet." || resp.Usage.InputTokens != 11 {
			t.Fatalf("jawaban: %+v %v", resp, err)
		}
		msgs := got["messages"].([]any)
		// system, user, assistant(tool_calls), tool
		if len(msgs) != 4 || msgs[2].(map[string]any)["tool_calls"] == nil || msgs[3].(map[string]any)["tool_call_id"] != "call_1" {
			t.Fatalf("terjemahan pesan: %v", msgs)
		}
	}
}

func TestGeminiAdapter(t *testing.T) {
	var got gmRequest
	var path, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, key = r.URL.Path, r.Header.Get("x-goog-api-key")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("content-type", "application/json")
		if strings.Contains(string(b), "functionResponse") {
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Ada 2 outlet."}]}}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"list_outlets","args":{}},"thoughtSignature":"SIG"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`))
	}))
	defer srv.Close()
	c := &geminiClient{http: srv.Client(), base: srv.URL}
	req := toolConversation()
	req.Messages = req.Messages[:1]
	resp, err := c.send(context.Background(), "AIzaKey", req)
	if err != nil || resp.StopReason != "tool_use" || resp.Content[0].Name != "list_outlets" || resp.Content[0].Sig != "SIG" {
		t.Fatalf("tool call: %+v %v", resp, err)
	}
	if path != "/models/m:generateContent" || key != "AIzaKey" {
		t.Fatalf("path/key: %s %s", path, key)
	}
	resp, err = c.send(context.Background(), "AIzaKey", toolConversation())
	if err != nil || resp.Content[0].Text != "Ada 2 outlet." || resp.Usage.OutputTokens != 3 {
		t.Fatalf("jawaban: %+v %v", resp, err)
	}
	if len(got.Contents) != 3 || got.Contents[1].Role != "model" || got.Contents[1].Parts[0].ThoughtSignature != "SIG" ||
		got.Contents[2].Parts[0].FunctionResponse == nil || got.Contents[2].Parts[0].FunctionResponse.Name != "list_outlets" {
		t.Fatalf("terjemahan: %+v", got.Contents)
	}
}

func TestMapUpstreamProviders(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
	}{
		{401, `{"error":{"message":"Incorrect API key","code":"invalid_api_key"}}`, "AI_KEY_INVALID"},
		{400, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`, "AI_KEY_INVALID"},
		{429, `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`, "AI_NO_CREDIT"},
		{429, `{"error":{"message":"Rate limit","type":"requests"}}`, "AI_UPSTREAM_LIMIT"},
		{400, `{"error":{"message":"Access denied, please make sure your account is in good standing.","code":"Arrearage"}}`, "AI_NO_CREDIT"},
		{404, `{"error":{"message":"model not found"}}`, "AI_MODEL_UNAVAILABLE"},
		{500, `{}`, "AI_UNAVAILABLE"},
	}
	for _, c := range cases {
		up, ok := mapUpstream(c.status, []byte(c.body)).(*upstreamError)
		if !ok || up.Code != c.code {
			t.Errorf("%d %s → %+v, mau %s", c.status, c.body, up, c.code)
		}
	}
}

func TestProviderSpecs(t *testing.T) {
	for _, id := range []string{ProviderAnthropic, ProviderOpenAI, ProviderGemini, ProviderQwen} {
		p, ok := specOf(id)
		if !ok || len(p.Models) == 0 || !p.validModel(p.Models[0]) || p.validModel("bukan-model") {
			t.Errorf("spec %s tidak sah", id)
		}
	}
	if _, ok := specOf("xyz"); ok {
		t.Error("penyedia tak dikenal harus ditolak")
	}
}

// Banyak penyedia per tenant: key terpisah, tepat satu aktif; pindah tanpa key baru; hapus yang aktif → yang lain menggantikan.
func TestMultiProviderSettings(t *testing.T) {
	it := newIT(t)
	ctx := context.Background()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ok.Close()
	it.svc.claude.url = ok.URL + "/v1/messages"
	it.svc.clients[ProviderOpenAI] = &openaiClient{http: ok.Client(), base: ok.URL, label: "T", maxCompletion: true}
	t.Cleanup(func() {
		_ = it.svc.RemoveKey(ctx, it.actor, ProviderAnthropic)
		_ = it.svc.RemoveKey(ctx, it.actor, ProviderOpenAI)
	})

	activeOf := func() (string, int) {
		st, err := it.svc.Settings(ctx, it.actor)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, p := range st.Providers {
			if p.Configured {
				n++
			}
		}
		return st.Provider, n
	}
	if err := it.svc.Save(ctx, it.actor, ProviderAnthropic, "sk-ant-"+strings.Repeat("a", 30), "claude-haiku-5-5"); err != nil {
		t.Fatal(err)
	}
	if err := it.svc.Save(ctx, it.actor, ProviderOpenAI, "sk-"+strings.Repeat("b", 30), "gpt-5"); err != nil {
		t.Fatal(err)
	}
	if p, n := activeOf(); p != ProviderOpenAI || n != 2 {
		t.Fatalf("aktif=%s terpasang=%d", p, n)
	}
	if err := it.svc.Save(ctx, it.actor, ProviderGemini, "", "gemini-2.5-pro"); err != ErrNotConfigured {
		t.Fatalf("pindah ke penyedia tanpa key: %v", err)
	}
	if err := it.svc.Save(ctx, it.actor, ProviderAnthropic, "", "claude-sonnet-5-5"); err != nil {
		t.Fatal(err)
	}
	if p, _ := activeOf(); p != ProviderAnthropic {
		t.Fatalf("pindah penyedia: %s", p)
	}
	if err := it.svc.Save(ctx, it.actor, ProviderOpenAI, "sk-"+strings.Repeat("b", 30), "claude-haiku-5-5"); err != ErrBadInput {
		t.Fatalf("model milik penyedia lain harus ditolak: %v", err)
	}
	if err := it.svc.Save(ctx, it.actor, ProviderGemini, "sk-ant-"+strings.Repeat("a", 30), "gemini-2.5-pro"); err != ErrBadKey {
		t.Fatalf("format key salah: %v", err)
	}
	if err := it.svc.RemoveKey(ctx, it.actor, ProviderAnthropic); err != nil {
		t.Fatal(err)
	}
	if p, n := activeOf(); p != ProviderOpenAI || n != 1 {
		t.Fatalf("setelah hapus yang aktif: aktif=%s terpasang=%d", p, n)
	}
	key, err := func() (string, error) { _, k, _, e := it.svc.loadKey(ctx, it.actor); return k, e }()
	if err != nil || key != "sk-"+strings.Repeat("b", 30) {
		t.Fatalf("loadKey: %q %v", key, err)
	}
}
