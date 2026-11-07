package storefront

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Penyaring HTML/CSS buatan tenant. HTML dibaca dengan tokenizer lalu DITULIS ULANG dari token (bukan dibersihkan lewat
// regex), hanya tag + atribut dalam allowlist yang keluar, nilai selalu di-escape, dan tag dijaga seimbang. Tidak ada
// script, iframe, form, style inline, handler on*, atau URL javascript:/data:. Ini lapis pertama; lapis kedua adalah CSP
// ketat pada respons (script-src 'none') dan sandbox, lihat handler.go.

var allowedTags = setOf("a", "abbr", "address", "article", "aside", "b", "blockquote", "br", "caption", "cite", "code", "col", "colgroup",
	"dd", "del", "details", "dfn", "div", "dl", "dt", "em", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header",
	"hgroup", "hr", "i", "img", "ins", "kbd", "li", "main", "mark", "nav", "ol", "p", "pre", "q", "s", "samp", "section", "small", "span",
	"strong", "sub", "summary", "sup", "table", "tbody", "td", "tfoot", "th", "thead", "time", "tr", "u", "ul", "var", "wbr")

var voidTags = setOf("br", "col", "hr", "img", "wbr")

// Tag yang isinya ikut dibuang (bukan hanya tag-nya): isinya bisa berupa kode atau konten aktif.
var dropWithContent = setOf("script", "style", "iframe", "frame", "frameset", "object", "embed", "applet", "svg", "math", "template",
	"noscript", "textarea", "title", "select", "option", "head", "xmp", "noembed", "noframes", "plaintext", "audio", "video", "canvas",
	"form", "button", "input", "link", "meta", "base", "portal", "dialog")

var globalAttrs = setOf("class", "id", "title", "lang", "dir", "role")

var tagAttrs = map[string]map[string]bool{
	"a":          setOf("href", "target", "hreflang"),
	"img":        setOf("src", "alt", "width", "height", "loading"),
	"td":         setOf("colspan", "rowspan", "headers"),
	"th":         setOf("colspan", "rowspan", "scope", "headers"),
	"col":        setOf("span"),
	"ol":         setOf("start", "type", "reversed"),
	"time":       setOf("datetime"),
	"details":    setOf("open"),
	"q":          setOf("cite"),
	"blockquote": setOf("cite"),
}

var (
	classRe  = regexp.MustCompile(`^[A-Za-z0-9 _:\-/\[\]\.%#,()!&*>=@+~'$"^|{}]*$`)
	idRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	numRe    = regexp.MustCompile(`^[0-9]{1,5}%?$`)
	langRe   = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	ariaRe   = regexp.MustCompile(`^aria-[a-z]{3,30}$`)
	datetime = regexp.MustCompile(`^[0-9T:+\-.Z ]{1,40}$`)
)

func setOf(v ...string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		m[s] = true
	}
	return m
}

// SanitizeHTML menyaring potongan HTML. tmpl=true berarti masukan adalah TEMPLATE: nilai atribut URL yang memuat penanda
// {…} dibiarkan (URL-nya baru dikenal setelah diisi, lalu disaring lagi pada keluaran render dengan tmpl=false).
// issues = hal yang dibuang, untuk ditampilkan ke penulis.
func SanitizeHTML(in string, tmpl bool) (out string, issues []string) {
	seen := map[string]bool{}
	note := func(s string) {
		if !seen[s] && len(issues) < 30 {
			seen[s] = true
			issues = append(issues, s)
		}
	}
	var b strings.Builder
	var stack []string
	skip := "" // nama tag yang sedang dilewati beserta isinya
	skipDepth := 0
	z := xhtml.NewTokenizer(strings.NewReader(in))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		switch tt {
		case xhtml.TextToken:
			if skip == "" {
				b.WriteString(html.EscapeString(string(z.Text())))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			nameB, hasAttr := z.TagName()
			name := strings.ToLower(string(nameB))
			if skip != "" {
				if name == skip && tt == xhtml.StartTagToken {
					skipDepth++
				}
				continue
			}
			if dropWithContent[name] {
				note("<" + name + "> tidak diizinkan")
				if tt == xhtml.StartTagToken && !voidTags[name] && name != "input" && name != "link" && name != "meta" && name != "base" {
					skip, skipDepth = name, 1
				}
				continue
			}
			if name == "html" || name == "body" {
				continue // pembungkus dokumen penuh: dilewati diam-diam, isinya tetap diproses
			}
			if !allowedTags[name] {
				note("<" + name + "> tidak diizinkan")
				continue
			}
			b.WriteString("<" + name)
			b.WriteString(sanitizeAttrs(z, name, hasAttr, tmpl, note))
			b.WriteString(">")
			if !voidTags[name] && tt == xhtml.StartTagToken {
				stack = append(stack, name)
			} else if !voidTags[name] { // <div/> diperlakukan sebagai tag terbuka lalu tertutup
				b.WriteString("</" + name + ">")
			}
		case xhtml.EndTagToken:
			nameB, _ := z.TagName()
			name := strings.ToLower(string(nameB))
			if skip != "" {
				if name == skip {
					if skipDepth--; skipDepth == 0 {
						skip = ""
					}
				}
				continue
			}
			i := len(stack) - 1
			for i >= 0 && stack[i] != name {
				i--
			}
			if i < 0 {
				continue
			}
			for j := len(stack) - 1; j >= i; j-- {
				b.WriteString("</" + stack[j] + ">")
			}
			stack = stack[:i]
		case xhtml.CommentToken, xhtml.DoctypeToken:
			// dibuang
		}
	}
	for j := len(stack) - 1; j >= 0; j-- {
		b.WriteString("</" + stack[j] + ">")
	}
	return b.String(), issues
}

func sanitizeAttrs(z *xhtml.Tokenizer, tag string, hasAttr, tmpl bool, note func(string)) string {
	if !hasAttr {
		if tag == "img" {
			return ` alt="" loading="lazy"`
		}
		return ""
	}
	var b strings.Builder
	got := map[string]bool{}
	for {
		k, v, more := z.TagAttr()
		key, val := strings.ToLower(string(k)), string(v)
		if key != "" && !got[key] {
			if ok, nv := checkAttr(tag, key, val, tmpl); ok {
				got[key] = true
				if nv == "" && (key == "open" || key == "reversed") {
					b.WriteString(" " + key)
				} else {
					b.WriteString(" " + key + `="` + html.EscapeString(nv) + `"`)
				}
			} else {
				note(fmt.Sprintf("atribut %s pada <%s> dibuang", key, tag))
			}
		}
		if !more {
			break
		}
	}
	if tag == "a" {
		b.WriteString(` rel="noopener noreferrer nofollow"`)
	}
	if tag == "img" {
		if !got["alt"] {
			b.WriteString(` alt=""`)
		}
		if !got["loading"] {
			b.WriteString(` loading="lazy"`)
		}
	}
	return b.String()
}

func checkAttr(tag, key, val string, tmpl bool) (bool, string) {
	switch {
	case key == "class":
		return len(val) <= 600 && classRe.MatchString(val), strings.TrimSpace(val)
	case key == "id":
		return idRe.MatchString(val), val
	case key == "title" || key == "alt":
		return len(val) <= 300, val
	case key == "lang" || key == "hreflang":
		return langRe.MatchString(val), val
	case key == "dir":
		return val == "ltr" || val == "rtl" || val == "auto", val
	case key == "role":
		return len(val) <= 40 && regexp.MustCompile(`^[a-z-]+$`).MatchString(val), val
	case ariaRe.MatchString(key):
		return len(val) <= 300, val
	}
	if !tagAttrs[tag][key] {
		return false, ""
	}
	switch key {
	case "href", "src", "cite":
		if tmpl && strings.Contains(val, "{") {
			return len(val) <= 2000, val
		}
		nv, ok := SafeURL(val, key == "href")
		return ok, nv
	case "target":
		return val == "_blank", val
	case "loading":
		return val == "lazy" || val == "eager", val
	case "width", "height", "colspan", "rowspan", "span", "start":
		return numRe.MatchString(val), val
	case "scope":
		return val == "row" || val == "col" || val == "rowgroup" || val == "colgroup", val
	case "type":
		return val == "1" || val == "a" || val == "A" || val == "i" || val == "I", val
	case "datetime":
		return datetime.MatchString(val), val
	case "headers":
		return idRe.MatchString(strings.ReplaceAll(val, " ", "")), val
	case "open", "reversed":
		return true, ""
	}
	return false, ""
}

// SafeURL: hanya http(s) dengan host, mailto/tel (khusus tautan), "#anchor", atau path relatif satu-slash. Karakter kontrol,
// spasi di tengah, dan backslash ditolak (trik pemecah skema seperti "java\tscript:").
func SafeURL(v string, allowLinkSchemes bool) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 2000 {
		return "", false
	}
	for _, r := range v {
		if r < 0x21 || r == 0x7f || r == '\\' || r == '"' || r == '<' || r == '>' || r == '`' {
			return "", false
		}
	}
	if v[0] == '#' {
		return v, allowLinkSchemes
	}
	if v[0] == '/' {
		return v, !strings.HasPrefix(v, "//")
	}
	u, err := url.Parse(v)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return v, u.Host != ""
	case "mailto", "tel":
		return v, allowLinkSchemes
	}
	return "", false
}

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssBad     = []string{"@import", "@charset", "@namespace", "@font-face", "@property", "@-moz-document", "url(", "image-set(", "src(", "expression(",
		"javascript:", "vbscript:", "behavior", "-moz-binding", "<", "-webkit-image-set", "cross-fade(", "paint(", "element("}
)

// SanitizeCSS menolak (bukan memperbaiki) CSS yang memuat konstruk berbahaya atau yang bisa memanggil sumber luar: tanpa
// url(), @import, escape "\" (dipakai menyamarkan kata kunci), dan "<". Komentar dibuang lebih dulu agar "ur/**/l(" ikut tertangkap.
func SanitizeCSS(in string) (string, error) {
	if len(in) > 20000 {
		return "", errors.New("CSS terlalu panjang (maks 20.000 karakter)")
	}
	clean := strings.TrimSpace(cssComment.ReplaceAllString(in, ""))
	if strings.Contains(clean, "/*") || strings.ContainsRune(clean, '\\') || strings.ContainsRune(clean, 0) {
		return "", errors.New(`CSS memuat komentar tak tertutup atau karakter "\"`)
	}
	low := strings.ToLower(clean)
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			return -1
		}
		return r
	}, low)
	for _, bad := range cssBad {
		if strings.Contains(low, bad) || strings.Contains(compact, bad) {
			return "", fmt.Errorf("CSS memuat %q yang tidak diizinkan", bad)
		}
	}
	return clean, nil
}
