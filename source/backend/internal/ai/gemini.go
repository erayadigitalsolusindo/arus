package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Klien Google Gemini API (generateContent) dengan function calling. Percakapan internal berbentuk Anthropic diterjemahkan:
// role assistant → "model", tool_use → functionCall, tool_result → functionResponse (dicocokkan lewat nama tool).

const geminiBase = "https://generativelanguage.googleapis.com/v1beta"

type geminiClient struct {
	http *http.Client
	base string
}

func newGeminiClient() *geminiClient {
	return &geminiClient{http: &http.Client{Timeout: 120 * time.Second}, base: geminiBase}
}

type gmCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type gmResult struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type gmPart struct {
	Text             string    `json:"text,omitempty"`
	Thought          bool      `json:"thought,omitempty"`
	ThoughtSignature string    `json:"thoughtSignature,omitempty"`
	FunctionCall     *gmCall   `json:"functionCall,omitempty"`
	FunctionResponse *gmResult `json:"functionResponse,omitempty"`
}

type gmContent struct {
	Role  string   `json:"role,omitempty"`
	Parts []gmPart `json:"parts"`
}

type gmTool struct {
	FunctionDeclarations []gmDecl `json:"functionDeclarations"`
}

type gmDecl struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema,omitempty"`
}

type gmRequest struct {
	SystemInstruction *gmContent  `json:"systemInstruction,omitempty"`
	Contents          []gmContent `json:"contents"`
	Tools             []gmTool    `json:"tools,omitempty"`
	GenerationConfig  struct {
		MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
	} `json:"generationConfig"`
}

func (c *geminiClient) toRequest(req apiRequest) gmRequest {
	var out gmRequest
	out.GenerationConfig.MaxOutputTokens = req.MaxTokens
	if req.System != "" {
		out.SystemInstruction = &gmContent{Parts: []gmPart{{Text: req.System}}}
	}
	for _, m := range req.Messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		ct := gmContent{Role: role}
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					ct.Parts = append(ct.Parts, gmPart{Text: b.Text})
				}
			case "tool_use":
				args := b.Input
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				ct.Parts = append(ct.Parts, gmPart{FunctionCall: &gmCall{Name: b.Name, Args: args}, ThoughtSignature: b.Sig})
			case "tool_result":
				key := "result"
				if b.IsError {
					key = "error"
				}
				ct.Parts = append(ct.Parts, gmPart{FunctionResponse: &gmResult{Name: b.ToolName, Response: map[string]any{key: b.Content}}})
			}
		}
		if len(ct.Parts) > 0 {
			out.Contents = append(out.Contents, ct)
		}
	}
	if len(req.Tools) > 0 {
		var t gmTool
		for _, d := range req.Tools {
			t.FunctionDeclarations = append(t.FunctionDeclarations, gmDecl{Name: d.Name, Description: d.Description, ParametersJSONSchema: d.InputSchema})
		}
		out.Tools = []gmTool{t}
	}
	return out
}

type gmResponse struct {
	Candidates []struct {
		Content gmContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}

func (c *geminiClient) send(ctx context.Context, apiKey string, req apiRequest) (*apiResponse, error) {
	b, err := json.Marshal(c.toRequest(req))
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/models/"+url.PathEscape(req.Model)+":generateContent", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("content-type", "application/json")
	hr.Header.Set("x-goog-api-key", apiKey)
	raw, err := doJSON(ctx, c.http, "Gemini", hr)
	if err != nil {
		return nil, err
	}
	var r gmResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, unavailable("respons Gemini tidak dikenali")
	}
	out := &apiResponse{StopReason: "end_turn", Usage: usage{
		InputTokens:  r.UsageMetadata.PromptTokenCount,
		OutputTokens: r.UsageMetadata.CandidatesTokenCount + r.UsageMetadata.ThoughtsTokenCount,
	}}
	if len(r.Candidates) == 0 {
		return nil, unavailable("Gemini tidak memberi jawaban (diblokir atau kosong)")
	}
	var txt strings.Builder
	n := 0
	for _, p := range r.Candidates[0].Content.Parts {
		switch {
		case p.FunctionCall != nil:
			in := p.FunctionCall.Args
			if len(in) == 0 || !json.Valid(in) {
				in = json.RawMessage("{}")
			}
			n++
			out.Content = append(out.Content, block{Type: "tool_use", ID: fmt.Sprintf("call_%d", n), Name: p.FunctionCall.Name, Input: in, Sig: p.ThoughtSignature})
			out.StopReason = "tool_use"
		case p.Text != "" && !p.Thought:
			txt.WriteString(p.Text)
		}
	}
	if txt.Len() > 0 {
		out.Content = append([]block{{Type: "text", Text: txt.String()}}, out.Content...)
	}
	return out, nil
}

func (c *geminiClient) verify(ctx context.Context, apiKey, _ string) error {
	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/models?pageSize=1", nil)
	if err != nil {
		return err
	}
	hr.Header.Set("x-goog-api-key", apiKey)
	_, err = doJSON(ctx, c.http, "Gemini", hr)
	return verifyErr(err)
}
