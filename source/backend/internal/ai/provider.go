package ai

import (
	"context"
	"strings"
)

// Penyedia AI yang bisa dipakai chat HalloArus. Format percakapan internal = bentuk Anthropic (message/block); tiap klien
// menerjemahkannya ke API penyedianya. Tool, izin, dan jaminan hanya-baca sama untuk semua penyedia.
const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderGemini    = "gemini"
	ProviderQwen      = "qwen"
)

type chatClient interface {
	send(ctx context.Context, apiKey string, req apiRequest) (*apiResponse, error)
	// verify memeriksa API key tanpa (atau dengan sangat sedikit) biaya. Key ditolak → ErrBadKey.
	verify(ctx context.Context, apiKey, model string) error
}

type providerSpec struct {
	ID     string
	Models []string // yang pertama = bawaan
	keyOK  func(string) bool
}

var providerSpecs = []providerSpec{
	{ID: ProviderAnthropic, Models: Models, keyOK: func(k string) bool { return strings.HasPrefix(k, "sk-ant-") }},
	{ID: ProviderOpenAI, Models: []string{"gpt-5", "gpt-5-mini", "gpt-5.6-sol"}, keyOK: func(k string) bool { return strings.HasPrefix(k, "sk-") }},
	{ID: ProviderGemini, Models: []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-3-flash-preview"}, keyOK: func(k string) bool { return strings.HasPrefix(k, "AIza") }},
	{ID: ProviderQwen, Models: []string{"qwen3-max", "qwen-plus", "qwen-flash"}, keyOK: func(k string) bool { return strings.HasPrefix(k, "sk-") }},
}

func specOf(id string) (providerSpec, bool) {
	for _, p := range providerSpecs {
		if p.ID == id {
			return p, true
		}
	}
	return providerSpec{}, false
}

func (p providerSpec) validModel(m string) bool {
	for _, x := range p.Models {
		if x == m {
			return true
		}
	}
	return false
}
