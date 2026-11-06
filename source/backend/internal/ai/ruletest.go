package ai

// "Uji tanya" aturan operator: Platform Admin mencoba draf aturan (belum disimpan) pada satu pertanyaan contoh. TANPA tool dan TANPA
// data tenant: model hanya diminta menunjukkan gaya/larangan/format jawabannya (angka contoh berlabel [CONTOH]). Memakai API key
// platform (AI_PLATFORM_KEY) dan model akun gratis; dibatasi 10 uji per menit per admin.

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

var (
	// ErrPlatformKeyMissing = AI_PLATFORM_KEY belum diatur di server.
	ErrPlatformKeyMissing = errors.New("API key platform belum diatur")
	// ErrRuleTestFailed = layanan AI menolak/gagal; rincian hanya di log server.
	ErrRuleTestFailed = errors.New("layanan AI tidak dapat memproses uji")
)

const (
	ruleTestMaxMessage = 600
	ruleTestMaxDraft   = 6000
	ruleTestLimit      = 10
	ruleTestTokens     = 700
)

type TestRulesInput struct {
	AdminID  uuid.UUID
	TenantID uuid.UUID // uuid.Nil = uji dokumen global
	Draft    string    // isi dokumen yang sedang diedit (belum disimpan)
	Message  string
}

type TestRulesResult struct {
	Reply        string `json:"reply"`
	Model        string `json:"model"`
	ElapsedMs    int64  `json:"elapsed_ms"`
	RulesChars   int    `json:"rules_chars"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

const ruleTestNote = "\n\nMODE UJI ATURAN: tidak ada tool dan tidak ada data nyata. Jawab pertanyaan pengguna sesuai ATURAN OPERATOR di atas. " +
	"Bila jawabannya butuh data, tunjukkan gaya dan format jawabanmu memakai angka CONTOH yang jelas berlabel [CONTOH]; jangan mengaku punya data nyata."

// TestRules menjalankan satu pertanyaan uji dengan aturan operator efektif: dokumen global tersimpan + draf (global), atau
// dokumen global tersimpan + draf tenant (tenant). Kebijakan efektif tenant ikut memengaruhi catatan batasan di prompt.
func (s *Service) TestRules(ctx context.Context, in TestRulesInput) (*TestRulesResult, error) {
	msg := strings.TrimSpace(in.Message)
	draft := strings.TrimSpace(strings.ReplaceAll(in.Draft, "\r\n", "\n"))
	if msg == "" || utf8.RuneCountInString(msg) > ruleTestMaxMessage || utf8.RuneCountInString(draft) > ruleTestMaxDraft {
		return nil, ErrBadInput
	}
	if s.free.Key == "" {
		return nil, ErrPlatformKeyMissing
	}
	if s.rdb != nil {
		ok, _, err := httpx.Allow(ctx, s.rdb, "ai:ruletest:"+in.AdminID.String(), ruleTestLimit, time.Minute)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrRateLimited
		}
	}
	// Kebijakan efektif: tenant uji, atau aturan global (tenant nil: RLS hanya mengembalikan baris global).
	pol, err := s.policy(ctx, in.TenantID)
	if err != nil {
		return nil, err
	}
	rules := draft
	if in.TenantID != uuid.Nil {
		rules = strings.TrimSpace(s.loadRules(ctx, uuid.Nil) + "\n\n" + draft) // global tersimpan lebih dulu, lalu draf tenant
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	system := systemPrompt(today, "Penguji Platform", "Outlet Uji", "Asia/Jakarta", pol.AllowCost, false) + policyPrompt(pol) + rulesPrompt(rules) + ruleTestNote

	model := s.freeModelFor(pol)
	start := time.Now()
	resp, err := s.claude.send(ctx, s.free.Key, apiRequest{Model: model, MaxTokens: ruleTestTokens, System: system,
		Messages: []message{{Role: "user", Content: []block{{Type: "text", Text: msg}}}}})
	if err != nil {
		s.log.WarnContext(ctx, "ai: uji aturan gagal", "err", err)
		return nil, ErrRuleTestFailed
	}
	var sb strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return &TestRulesResult{Reply: strings.TrimSpace(sb.String()), Model: model, ElapsedMs: time.Since(start).Milliseconds(), RulesChars: utf8.RuneCountInString(rules),
		InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}, nil
}
