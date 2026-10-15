package extapi

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"aciraba/internal/authz"
)

//go:embed catalog_meta.json
var metaJSON []byte

// Param = parameter path/query/header pada dokumentasi endpoint.
type Param struct {
	Name        string `json:"name"`
	In          string `json:"in"` // path | query | header | body
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// Example = contoh badan JSON request/respons.
type Example struct {
	Status      int    `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
	Body        any    `json:"body,omitempty"`
}

type ErrorRef struct {
	Status      int    `json:"status"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// endpointMeta = penjelasan tulisan tangan dari catalog_meta.json (kunci "METHOD /path").
type endpointMeta struct {
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	Query       []Param    `json:"query"`
	Body        []Param    `json:"body"`
	Request     *Example   `json:"request"`
	Responses   []Example  `json:"responses"`
	Errors      []ErrorRef `json:"errors"`
	Notes       []string   `json:"notes"`
}

type groupMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

type metaFile struct {
	Groups    map[string]groupMeta    `json:"groups"`
	Endpoints map[string]endpointMeta `json:"endpoints"`
}

// PermRef = izin yang dibutuhkan endpoint (cukup salah satu bila lebih dari satu).
type PermRef struct {
	Module string `json:"module"`
	Action string `json:"action"`
}

type Endpoint struct {
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	Group       string     `json:"group"`
	Summary     string     `json:"summary"`
	Description string     `json:"description,omitempty"`
	Auth        string     `json:"auth"` // hmac = lewat gateway /ext/v1 · jwt = hanya sesi pengguna (login)
	Permissions []PermRef  `json:"permissions,omitempty"`
	Params      []Param    `json:"params,omitempty"`
	Request     *Example   `json:"request,omitempty"`
	Responses   []Example  `json:"responses,omitempty"`
	Errors      []ErrorRef `json:"errors,omitempty"`
	Notes       []string   `json:"notes,omitempty"`
	Documented  bool       `json:"documented"`
}

type Group struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Count       int    `json:"count"`
	Documented  int    `json:"documented"`
	Order       int    `json:"-"`
}

type Catalog struct {
	Groups    []Group    `json:"groups"`
	Endpoints []Endpoint `json:"endpoints"`
}

var paramRe = regexp.MustCompile(`\{([A-Za-z0-9_]+)(?::[^}]*)?\}`)

// Catalog membaca seluruh rute dari router (chi.Walk), jadi tidak ada endpoint yang terlewat atau basi, lalu menempelkan
// penjelasan tulisan tangan bila ada. Izin ditemukan dengan menguji middleware tiap rute (lihat probe). Hasil di-cache.
type CatalogBuilder struct {
	routes chi.Routes
	once   sync.Once
	cat    *Catalog
}

func NewCatalogBuilder(routes chi.Routes) *CatalogBuilder { return &CatalogBuilder{routes: routes} }

func (b *CatalogBuilder) Get() *Catalog {
	b.once.Do(func() { b.cat = b.build() })
	return b.cat
}

func (b *CatalogBuilder) build() *Catalog {
	var meta metaFile
	_ = json.Unmarshal(metaJSON, &meta)

	skipPrefix := []string{"/platform", "/public", "/mcp", "/oauth", "/.well-known", "/healthz", Prefix}
	seen := map[string]bool{}
	var eps []Endpoint
	_ = chi.Walk(b.routes, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		if route == "" || method == http.MethodOptions || method == http.MethodHead {
			return nil
		}
		for _, p := range skipPrefix {
			if route == p || strings.HasPrefix(route, p+"/") {
				return nil
			}
		}
		key := method + " " + route
		if seen[key] {
			return nil
		}
		seen[key] = true

		ep := Endpoint{Method: method, Path: route, Group: groupOf(route), Auth: "hmac"}
		if Blocked(route) {
			ep.Auth = "jwt"
		} else {
			ep.Permissions = probePermissions(method, mws)
		}
		for _, m := range paramRe.FindAllStringSubmatch(route, -1) {
			ep.Params = append(ep.Params, Param{Name: m[1], In: "path", Type: "string", Required: true})
		}
		ep.Summary = autoSummary(method, route)
		if m, ok := meta.Endpoints[key]; ok {
			ep.Documented = true
			if m.Summary != "" {
				ep.Summary = m.Summary
			}
			ep.Description, ep.Notes, ep.Errors = m.Description, m.Notes, m.Errors
			for _, q := range m.Query {
				q.In = "query"
				ep.Params = append(ep.Params, q)
			}
			for _, q := range m.Body {
				q.In = "body"
				ep.Params = append(ep.Params, q)
			}
			ep.Request, ep.Responses = m.Request, m.Responses
		}
		eps = append(eps, ep)
		return nil
	})

	counts := map[string]*Group{}
	for i := range eps {
		g := counts[eps[i].Group]
		if g == nil {
			gm := meta.Groups[eps[i].Group]
			title := gm.Title
			if title == "" {
				title = strings.ToUpper(eps[i].Group[:1]) + eps[i].Group[1:]
			}
			order := gm.Order
			if order == 0 {
				order = 1000
			}
			g = &Group{ID: eps[i].Group, Title: title, Description: gm.Description, Order: order}
			counts[eps[i].Group] = g
		}
		g.Count++
		if eps[i].Documented {
			g.Documented++
		}
	}
	groups := make([]Group, 0, len(counts))
	for _, g := range counts {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Order != groups[j].Order {
			return groups[i].Order < groups[j].Order
		}
		return groups[i].ID < groups[j].ID
	})
	rank := map[string]int{}
	for i, g := range groups {
		rank[g.ID] = i
	}
	methodRank := map[string]int{"GET": 0, "POST": 1, "PUT": 2, "PATCH": 3, "DELETE": 4}
	sort.Slice(eps, func(i, j int) bool {
		a, c := eps[i], eps[j]
		if a.Group != c.Group {
			return rank[a.Group] < rank[c.Group]
		}
		if a.Path != c.Path {
			return a.Path < c.Path
		}
		return methodRank[a.Method] < methodRank[c.Method]
	})
	return &Catalog{Groups: groups, Endpoints: eps}
}

// Filter membuang grup yang disembunyikan profil white label.
func (c *Catalog) Filter(hidden []string) *Catalog {
	if len(hidden) == 0 {
		return c
	}
	hide := map[string]bool{}
	for _, h := range hidden {
		hide[h] = true
	}
	out := &Catalog{}
	for _, g := range c.Groups {
		if !hide[g.ID] {
			out.Groups = append(out.Groups, g)
		}
	}
	for _, e := range c.Endpoints {
		if !hide[e.Group] {
			out.Endpoints = append(out.Endpoints, e)
		}
	}
	return out
}

// groupOf = grup tampilan: segmen pertama path; untuk laporan dipecah per jenis ("reports-sales", ...).
func groupOf(route string) string {
	seg := strings.Split(strings.Trim(route, "/"), "/")
	if seg[0] == "reports" && len(seg) > 1 && !strings.HasPrefix(seg[1], "{") {
		return "reports-" + seg[1]
	}
	return seg[0]
}

var verbWords = map[string]string{
	"post": "Posting", "reverse": "Jurnal balik", "close": "Tutup", "reopen": "Buka kembali", "void": "Batalkan", "cancel": "Batalkan",
	"approve": "Setujui", "reject": "Tolak", "seed-retail": "Isi template ritel", "generate": "Buat otomatis", "login": "Masuk",
	"logout": "Keluar", "refresh": "Perbarui token", "register": "Daftar", "send": "Kirim", "pay": "Bayar", "payments": "Pembayaran",
}

// autoSummary = judul ringkas dari metode + path bila belum ada penjelasan tulisan tangan.
func autoSummary(method, route string) string {
	var segs []string
	last := ""
	endsParam := false
	for _, s := range strings.Split(strings.Trim(route, "/"), "/") {
		if strings.HasPrefix(s, "{") {
			endsParam = true
			continue
		}
		endsParam = false
		segs = append(segs, s)
		last = s
	}
	res := strings.ReplaceAll(strings.Join(lastN(segs, 2), " "), "-", " ")
	switch method {
	case http.MethodGet:
		if endsParam {
			return "Detail " + strings.ReplaceAll(last, "-", " ")
		}
		if len(segs) > 0 && segs[0] == "reports" {
			return "Laporan " + strings.Join(segs[1:], " ")
		}
		return "Daftar " + res
	case http.MethodPost:
		if v, ok := verbWords[last]; ok && len(segs) > 1 {
			return v + " " + strings.ReplaceAll(segs[len(segs)-2], "-", " ")
		}
		if v, ok := verbWords[last]; ok {
			return v
		}
		return "Buat " + res
	case http.MethodPut, http.MethodPatch:
		return "Ubah " + res
	case http.MethodDelete:
		return "Hapus " + res
	}
	return method + " " + route
}

func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// probePermissions menemukan izin (modul.aksi) yang dituntut middleware sebuah rute TANPA mengubah satu handler pun:
// tiap middleware dipanggil dengan request sintetis; yang menjawab 403 untuk pemanggil tanpa izin lalu diuji dengan
// pemanggil yang hanya memegang satu izin — izin yang meloloskan itulah syaratnya. Middleware lain (auth, CSRF, rate
// limit) tidak lolos pada uji ini sehingga diabaikan.
func probePermissions(method string, mws []func(http.Handler) http.Handler) []PermRef {
	var out []PermRef
	for _, mw := range mws {
		out = append(out, probeOne(method, mw)...)
	}
	return out
}

func probeOne(method string, mw func(http.Handler) http.Handler) (found []PermRef) {
	defer func() { _ = recover() }()
	passed := false
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passed = true }))
	newReq := func(a *authz.Actor) *http.Request {
		r := httptest.NewRequest(method, "/", nil)
		r.RemoteAddr = "192.0.2.1:1" // TEST-NET: penghitung rate limit (bila ada) tidak menyentuh klien nyata
		if a != nil {
			r = r.WithContext(authz.WithActor(r.Context(), *a))
		}
		return r
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(&authz.Actor{}))
	if passed || rec.Code != http.StatusForbidden {
		return nil
	}
	for _, m := range authz.Modules {
		for _, act := range m.Actions {
			passed = false
			h.ServeHTTP(httptest.NewRecorder(), newReq(&authz.Actor{Perms: authz.Permissions{Grants: map[string][]string{m.ID: {act}}}}))
			if passed {
				found = append(found, PermRef{Module: m.ID, Action: act})
			}
		}
	}
	if len(found) > 12 { // bukan middleware izin (mis. pemeriksaan yang meloloskan banyak pemanggil)
		return nil
	}
	return found
}
