// Package aipolicy: kebijakan HalloArus yang ditetapkan PLATFORM (bukan tenant). Satu baris global + pengecualian per tenant
// (field nil = ikut global). Aturan platform selalu menang atas izin role tenant; role hanya bisa mempersempit.
// Dipakai bersama oleh modul ai (menerapkan) dan platformadmin (mengelola).
package aipolicy

import (
	"slices"
	"strings"
)

// Domain data yang boleh dibaca AI. Kunci stabil (tersimpan di DB).
const (
	DomainSales      = "sales"
	DomainMembers    = "members"
	DomainStock      = "stock"
	DomainDebts      = "debts"
	DomainPurchases  = "purchases"
	DomainCash       = "cash"
	DomainAccounting = "accounting"
)

// Domains = urutan tampil di panel platform.
var Domains = []string{DomainSales, DomainMembers, DomainStock, DomainDebts, DomainPurchases, DomainCash, DomainAccounting}

// toolDomain memetakan tool baca ke domainnya. Tool yang tidak ada di sini (list_outlets) selalu boleh.
var toolDomain = map[string]string{
	"sales_summary": DomainSales, "top_items": DomainSales, "sales_by_category": DomainSales, "returns_summary": DomainSales, "member_overview": DomainMembers,
	"stock_overview": DomainStock, "stock_items": DomainStock, "slow_moving_items": DomainStock, "stock_opname_summary": DomainStock,
	"receivables_summary": DomainDebts, "payables_summary": DomainDebts,
	"purchases_summary":   DomainPurchases,
	"cash_shifts_summary": DomainCash, "petty_cash_summary": DomainCash, "list_petty_cash": DomainCash,
	"accounting_balances": DomainAccounting,
}

// DomainTools = daftar tool per domain (urut nama), untuk ditampilkan di panel platform.
func DomainTools() map[string][]string {
	out := map[string][]string{}
	for _, name := range ToolNames() {
		d := toolDomain[name]
		out[d] = append(out[d], name)
	}
	return out
}

// DomainOf = domain sebuah tool ("" = tak terikat domain). ToolNames dipakai test agar tool baru tidak lolos tanpa domain.
func DomainOf(tool string) string { return toolDomain[tool] }

func ToolNames() []string {
	out := make([]string, 0, len(toolDomain))
	for k := range toolDomain {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

const (
	DefaultChatLimit    = 20
	DefaultMaxRangeDays = 366
)

// Effective = kebijakan hasil gabungan untuk satu tenant. Nol-value bukan "tanpa batas": selalu lewat Default/Resolve.
type Effective struct {
	Enabled        bool
	Domains        []string
	AllowWrite     bool
	AllowCost      bool
	AllowBYOK      bool
	AllowCustom    bool
	AllowImages    bool
	AllowHistory   bool
	AllowMCP       bool
	FreeEnabled    bool
	FreeDailyLimit *int   // nil = nilai env AI_FREE_DAILY_LIMIT
	FreeModel      string // "" = model murah bawaan
	ChatLimit      int
	MaxRangeDays   int
}

// Default = tanpa pembatasan (perilaku sebelum kebijakan ada); dipakai bila baris global belum terbaca.
func Default() Effective {
	return Effective{Enabled: true, Domains: slices.Clone(Domains), AllowWrite: true, AllowCost: true, AllowBYOK: true, AllowCustom: true,
		AllowImages: true, AllowHistory: true, AllowMCP: true, FreeEnabled: true, ChatLimit: DefaultChatLimit, MaxRangeDays: DefaultMaxRangeDays}
}

// Values = satu baris kebijakan; pada pengecualian tenant, nil = ikut global.
type Values struct {
	Enabled        *bool    `json:"enabled"`
	Domains        []string `json:"domains"`
	AllowWrite     *bool    `json:"allow_write"`
	AllowCost      *bool    `json:"allow_cost"`
	AllowBYOK      *bool    `json:"allow_byok"`
	AllowCustom    *bool    `json:"allow_custom"`
	AllowImages    *bool    `json:"allow_images"`
	AllowHistory   *bool    `json:"allow_history"`
	AllowMCP       *bool    `json:"allow_mcp"`
	FreeEnabled    *bool    `json:"free_enabled"`
	FreeDailyLimit *int     `json:"free_daily_limit"`
	FreeModel      *string  `json:"free_model"`
	ChatLimit      *int     `json:"chat_limit"`
	MaxRangeDays   *int     `json:"max_range_days"`
}

// Apply menimpa field Effective dengan field Values yang terisi.
func (e Effective) Apply(v Values) Effective {
	set := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}
	set(&e.Enabled, v.Enabled)
	set(&e.AllowWrite, v.AllowWrite)
	set(&e.AllowCost, v.AllowCost)
	set(&e.AllowBYOK, v.AllowBYOK)
	set(&e.AllowCustom, v.AllowCustom)
	set(&e.AllowImages, v.AllowImages)
	set(&e.AllowHistory, v.AllowHistory)
	set(&e.AllowMCP, v.AllowMCP)
	set(&e.FreeEnabled, v.FreeEnabled)
	if v.Domains != nil {
		e.Domains = slices.Clone(v.Domains)
	}
	if v.FreeDailyLimit != nil {
		e.FreeDailyLimit = v.FreeDailyLimit
	}
	if v.FreeModel != nil {
		e.FreeModel = strings.TrimSpace(*v.FreeModel)
	}
	if v.ChatLimit != nil {
		e.ChatLimit = *v.ChatLimit
	}
	if v.MaxRangeDays != nil {
		e.MaxRangeDays = *v.MaxRangeDays
	}
	return e
}

// DomainAllowed: tool tanpa domain selalu boleh.
func (e Effective) DomainAllowed(tool string) bool {
	d := DomainOf(tool)
	return d == "" || slices.Contains(e.Domains, d)
}

// Blocked = domain yang dimatikan (urutan tetap), untuk catatan di prompt sistem.
func (e Effective) Blocked() []string {
	var out []string
	for _, d := range Domains {
		if !slices.Contains(e.Domains, d) {
			out = append(out, d)
		}
	}
	return out
}

// Validate memeriksa satu baris. global=true: semua field wajib terisi (kecuali free_daily_limit/free_model: nil = bawaan).
// Hasil: peta field → kode galat singkat.
func (v Values) Validate(global bool, validFreeModel func(string) bool) map[string]string {
	f := map[string]string{}
	if global {
		for name, nilBool := range map[string]bool{
			"enabled": v.Enabled == nil, "allow_write": v.AllowWrite == nil, "allow_cost": v.AllowCost == nil, "allow_byok": v.AllowBYOK == nil,
			"allow_custom": v.AllowCustom == nil, "allow_images": v.AllowImages == nil, "allow_history": v.AllowHistory == nil,
			"allow_mcp": v.AllowMCP == nil, "free_enabled": v.FreeEnabled == nil, "domains": v.Domains == nil,
			"chat_limit": v.ChatLimit == nil, "max_range_days": v.MaxRangeDays == nil,
		} {
			if nilBool {
				f[name] = "REQUIRED"
			}
		}
	}
	for _, d := range v.Domains {
		if !slices.Contains(Domains, d) {
			f["domains"] = "INVALID"
		}
	}
	if v.FreeDailyLimit != nil && (*v.FreeDailyLimit < 0 || *v.FreeDailyLimit > 1000) {
		f["free_daily_limit"] = "RANGE"
	}
	if v.ChatLimit != nil && (*v.ChatLimit < 1 || *v.ChatLimit > 200) {
		f["chat_limit"] = "RANGE"
	}
	if v.MaxRangeDays != nil && (*v.MaxRangeDays < 1 || *v.MaxRangeDays > 366) {
		f["max_range_days"] = "RANGE"
	}
	if v.FreeModel != nil && strings.TrimSpace(*v.FreeModel) != "" && !validFreeModel(strings.TrimSpace(*v.FreeModel)) {
		f["free_model"] = "INVALID"
	}
	return f
}
