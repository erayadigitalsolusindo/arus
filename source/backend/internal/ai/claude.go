package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Klien minimal untuk Anthropic Messages API (hanya yang dibutuhkan: teks + tool use, tanpa streaming).
// Tanpa dependensi SDK agar permukaan kode kecil dan mudah diuji dengan httptest.

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// Model yang boleh dipilih tenant (dijaga juga oleh CHECK di DB). Yang pertama = bawaan.
var Models = []string{"claude-sonnet-5-5", "claude-opus-5-5", "claude-fable-5-1", "claude-haiku-5-5"}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// Hanya di memori (tidak dikirim ke Anthropic): dipakai adaptor penyedia lain.
	ToolName string `json:"-"` // nama tool pada tool_result (Gemini mencocokkan hasil lewat nama)
	Sig      string `json:"-"` // thoughtSignature Gemini pada tool_use, wajib dikembalikan apa adanya
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type apiRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Tools     []toolDef `json:"tools,omitempty"`
	Messages  []message `json:"messages"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type apiResponse struct {
	Content    []block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Usage      usage   `json:"usage"`
}

// upstreamError = galat dari Anthropic yang dipetakan ke kode API stabil (kamus errors.<CODE> di web).
type upstreamError struct {
	Status  int
	Code    string
	Message string
}

func (e *upstreamError) Error() string { return fmt.Sprintf("anthropic %d: %s", e.Status, e.Message) }

type claudeClient struct {
	http *http.Client
	url  string
}

func newClaudeClient() *claudeClient {
	return &claudeClient{http: &http.Client{Timeout: 120 * time.Second}, url: anthropicURL}
}

func (c *claudeClient) send(ctx context.Context, apiKey string, req apiRequest) (*apiResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("content-type", "application/json")
	hr.Header.Set("x-api-key", apiKey)
	hr.Header.Set("anthropic-version", anthropicVersion)
	resp, err := c.http.Do(hr)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &upstreamError{Status: http.StatusBadGateway, Code: "AI_UNAVAILABLE", Message: "tidak dapat menghubungi Anthropic"}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &upstreamError{Status: http.StatusBadGateway, Code: "AI_UNAVAILABLE", Message: "respons Anthropic terputus"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, mapUpstream(resp.StatusCode, raw)
	}
	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, &upstreamError{Status: http.StatusBadGateway, Code: "AI_UNAVAILABLE", Message: "respons Anthropic tidak dikenali"}
	}
	return &out, nil
}

// verify memeriksa API key lewat daftar model (gratis, tanpa token). Key ditolak → ErrBadKey; gangguan lain → upstreamError.
func (c *claudeClient) verify(ctx context.Context, apiKey, _ string) error {
	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.url, "/messages")+"/models?limit=1", nil)
	if err != nil {
		return err
	}
	hr.Header.Set("x-api-key", apiKey)
	hr.Header.Set("anthropic-version", anthropicVersion)
	resp, err := c.http.Do(hr)
	if err != nil {
		return &upstreamError{Status: http.StatusBadGateway, Code: "AI_UNAVAILABLE", Message: "tidak dapat menghubungi Anthropic"}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrBadKey
	default:
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return mapUpstream(resp.StatusCode, raw)
	}
}

// mapUpstream memetakan galat penyedia mana pun (Anthropic, OpenAI, Gemini, Qwen) ke kode API stabil.
func mapUpstream(status int, raw []byte) error {
	var e struct {
		Error struct {
			Type    string          `json:"type"`
			Message string          `json:"message"`
			Status  string          `json:"status"`
			Code    json.RawMessage `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	msg := e.Error.Message
	low := strings.ToLower(msg + " " + e.Error.Type + " " + e.Error.Status + " " + string(e.Error.Code))
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(low, s) {
				return true
			}
		}
		return false
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden,
		status == http.StatusBadRequest && has("api key not valid", "api_key_invalid", "invalid_api_key"):
		return &upstreamError{Status: status, Code: "AI_KEY_INVALID", Message: msg}
	case status == http.StatusTooManyRequests && has("insufficient_quota"),
		status == http.StatusBadRequest && has("credit balance", "arrearage", "overdue"):
		return &upstreamError{Status: status, Code: "AI_NO_CREDIT", Message: msg}
	case status == http.StatusTooManyRequests:
		return &upstreamError{Status: status, Code: "AI_UPSTREAM_LIMIT", Message: msg}
	case status == http.StatusNotFound,
		status == http.StatusBadRequest && has("model_not_found", "model not exist", "model does not exist", "model not found"):
		return &upstreamError{Status: status, Code: "AI_MODEL_UNAVAILABLE", Message: msg}
	default:
		return &upstreamError{Status: status, Code: "AI_UNAVAILABLE", Message: msg}
	}
}

func unavailable(msg string) error {
	return &upstreamError{Status: http.StatusBadGateway, Code: "AI_UNAVAILABLE", Message: msg}
}

// doJSON dipakai klien non-Anthropic: kirim, batasi ukuran respons, petakan galat HTTP.
func doJSON(ctx context.Context, h *http.Client, label string, hr *http.Request) ([]byte, error) {
	resp, err := h.Do(hr)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, unavailable("tidak dapat menghubungi " + label)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, unavailable("respons " + label + " terputus")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, mapUpstream(resp.StatusCode, raw)
	}
	return raw, nil
}
