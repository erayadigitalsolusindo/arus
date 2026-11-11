package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// Foto nota dikirim sebagai image_url (OpenAI/Qwen/kustom) dan inlineData (Gemini), digabung dengan teks dalam satu pesan.
func TestImageMapping(t *testing.T) {
	req := apiRequest{Model: "m", System: "s", Messages: []message{{Role: "user", Content: []block{
		{Type: "image", Source: &imageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}},
		{Type: "text", Text: "baca nota"},
	}}}}
	ob, _ := json.Marshal(newOpenAIClient().toRequest(req))
	for _, w := range []string{`"image_url":{"url":"data:image/png;base64,QUJD"}`, `"text":"baca nota"`} {
		if !strings.Contains(string(ob), w) {
			t.Errorf("openai: %s tidak ada di %s", w, ob)
		}
	}
	gb, _ := json.Marshal(newGeminiClient().toRequest(req))
	if !strings.Contains(string(gb), `"inlineData":{"mimeType":"image/png","data":"QUJD"}`) {
		t.Errorf("gemini: %s", gb)
	}
	if imageCapable(ProviderQwen, "qwen3-max") || !imageCapable(ProviderQwen, "qwen3-vl-plus") || !imageCapable(ProviderOpenAI, "gpt-5") {
		t.Error("imageCapable salah")
	}
}
