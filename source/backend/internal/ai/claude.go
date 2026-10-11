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

func validModel(m string) bool {
	for _, x := range Models {
		if x == m {
			return true
		}
	}
	return false
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
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
func (c *claudeClient) verify(ctx context.Context, apiKey string) error {
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

func mapUpstream(status int, raw []byte) error {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	msg := e.Error.Message
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &upstreamError{Status: status, Code: "AI_KEY_INVALID", Message: msg}
	case status == http.StatusTooManyRequests:
		return &upstreamError{Status: status, Code: "AI_UPSTREAM_LIMIT", Message: msg}
	case status == http.StatusBadRequest && strings.Contains(strings.ToLower(msg), "credit balance"):
		return &upstreamError{Status: status, Code: "AI_NO_CREDIT", Message: msg}
	case status == http.StatusNotFound:
		return &upstreamError{Status: status, Code: "AI_MODEL_UNAVAILABLE", Message: msg}
	default:
		return &upstreamError{Status: status, Code: "AI_UNAVAILABLE", Message: msg}
	}
}
