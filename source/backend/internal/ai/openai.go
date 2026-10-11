package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Klien untuk API chat/completions gaya OpenAI: dipakai OpenAI (ChatGPT) dan Qwen (Alibaba Model Studio, mode kompatibel).
// Percakapan internal berbentuk Anthropic diterjemahkan ke pesan role system/user/assistant/tool + function calling.

const (
	openaiBase = "https://api.openai.com/v1"
	// Endpoint internasional (Singapura); key Model Studio terikat wilayah, key Beijing tidak berlaku di sini.
	qwenBase = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
)

type openaiClient struct {
	http  *http.Client
	base  string
	label string
	// maxCompletion: OpenAI memakai max_completion_tokens (max_tokens ditolak model baru); Qwen memakai max_tokens.
	maxCompletion bool
	// probe: verifikasi key dengan chat 1 token (penyedia tanpa /models yang andal), bukan GET /models.
	probe bool
}

func newOpenAIClient() *openaiClient {
	return &openaiClient{http: &http.Client{Timeout: 120 * time.Second}, base: openaiBase, label: "OpenAI", maxCompletion: true}
}

func newQwenClient() *openaiClient {
	return &openaiClient{http: &http.Client{Timeout: 120 * time.Second}, base: qwenBase, label: "Qwen", probe: true}
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaRequest struct {
	Model               string      `json:"model"`
	Messages            []oaMessage `json:"messages"`
	Tools               []oaTool    `json:"tools,omitempty"`
	MaxTokens           int         `json:"max_tokens,omitempty"`
	MaxCompletionTokens int         `json:"max_completion_tokens,omitempty"`
}

func strp(s string) *string { return &s }

func (c *openaiClient) toRequest(req apiRequest) oaRequest {
	out := oaRequest{Model: req.Model}
	if c.maxCompletion {
		out.MaxCompletionTokens = req.MaxTokens
	} else {
		out.MaxTokens = req.MaxTokens
	}
	out.Messages = append(out.Messages, oaMessage{Role: "system", Content: strp(req.System)})
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			am := oaMessage{Role: "assistant"}
			var txt strings.Builder
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					txt.WriteString(b.Text)
				case "tool_use":
					tc := oaToolCall{ID: b.ID, Type: "function"}
					tc.Function.Name = b.Name
					tc.Function.Arguments = string(b.Input)
					if tc.Function.Arguments == "" {
						tc.Function.Arguments = "{}"
					}
					am.ToolCalls = append(am.ToolCalls, tc)
				}
			}
			if txt.Len() > 0 {
				am.Content = strp(txt.String())
			}
			out.Messages = append(out.Messages, am)
			continue
		}
		for _, b := range m.Content {
			switch b.Type {
			case "tool_result":
				out.Messages = append(out.Messages, oaMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: strp(b.Content)})
			case "text":
				out.Messages = append(out.Messages, oaMessage{Role: "user", Content: strp(b.Text)})
			}
		}
	}
	for _, t := range req.Tools {
		var ot oaTool
		ot.Type = "function"
		ot.Function.Name, ot.Function.Description, ot.Function.Parameters = t.Name, t.Description, t.InputSchema
		out.Tools = append(out.Tools, ot)
	}
	return out
}

type oaResponse struct {
	Choices []struct {
		Message struct {
			Content   *string      `json:"content"`
			ToolCalls []oaToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *openaiClient) post(ctx context.Context, apiKey string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("content-type", "application/json")
	hr.Header.Set("authorization", "Bearer "+apiKey)
	return doJSON(ctx, c.http, c.label, hr)
}

func (c *openaiClient) send(ctx context.Context, apiKey string, req apiRequest) (*apiResponse, error) {
	raw, err := c.post(ctx, apiKey, c.toRequest(req))
	if err != nil {
		return nil, err
	}
	var r oaResponse
	if err := json.Unmarshal(raw, &r); err != nil || len(r.Choices) == 0 {
		return nil, unavailable("respons " + c.label + " tidak dikenali")
	}
	msg := r.Choices[0].Message
	out := &apiResponse{StopReason: "end_turn", Usage: usage{InputTokens: r.Usage.PromptTokens, OutputTokens: r.Usage.CompletionTokens}}
	if msg.Content != nil && *msg.Content != "" {
		out.Content = append(out.Content, block{Type: "text", Text: *msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		in := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(in) {
			in = json.RawMessage("{}")
		}
		out.Content = append(out.Content, block{Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: in})
		out.StopReason = "tool_use"
	}
	return out, nil
}

func (c *openaiClient) verify(ctx context.Context, apiKey, model string) error {
	if c.probe {
		_, err := c.post(ctx, apiKey, oaRequest{Model: model, MaxTokens: 1, Messages: []oaMessage{{Role: "user", Content: strp("hi")}}})
		return verifyErr(err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/models", nil)
	if err != nil {
		return err
	}
	hr.Header.Set("authorization", "Bearer "+apiKey)
	_, err = doJSON(ctx, c.http, c.label, hr)
	return verifyErr(err)
}

// verifyErr: key ditolak → ErrBadKey; galat lain (jaringan, kuota, model) diteruskan apa adanya.
func verifyErr(err error) error {
	if up, ok := err.(*upstreamError); ok && up.Code == "AI_KEY_INVALID" {
		return ErrBadKey
	}
	return err
}
