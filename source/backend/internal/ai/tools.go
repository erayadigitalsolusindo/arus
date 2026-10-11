package ai

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/authz"
)

// Tool baca untuk Asisten AI. PRINSIP KEAMANAN (jangan dilonggarkan):
//   - Setiap tool berjalan di transaksi READ ONLY milik tenant pemanggil (RLS aktif) + statement_timeout: database sendiri yang
//     menolak INSERT/UPDATE/DELETE, bukan hanya prompt.
//   - Claude tidak pernah menulis SQL; ia hanya memilih tool dan mengisi parameter terbatas (tanggal, outlet, limit, jenis kelompok).
//   - Setiap tool memeriksa izin `view` modul terkait milik pengguna dan hanya melihat outlet yang boleh diakses pengguna.
//   - Data pribadi pelanggan (nama, telepon, alamat, email) tidak pernah dikirim: pelanggan hanya muncul sebagai "Pelanggan N".
//   - HPP/laba/nilai persediaan hanya disertakan bila pengguna punya izin `sales_cost`.

const (
	maxRows        = 200   // baris maksimum per tool
	maxResultBytes = 60000 // hasil tool yang lebih besar ditolak (minta filter lebih sempit)
	maxRangeDays   = 366
	queryTimeout   = "8s"
)

type env struct {
	ctx     context.Context
	tx      pgx.Tx
	a       authz.Actor
	today   time.Time // tanggal lokal hari ini (zona waktu outlet aktif)
	canCost bool
}

type params map[string]any

func (p params) str(key string) string {
	s, _ := p[key].(string)
	return strings.TrimSpace(s)
}

func (p params) integer(key string, def, lo, hi int) int {
	v := def
	switch n := p[key].(type) {
	case float64:
		v = int(n)
	case string:
		if x, err := strconv.Atoi(n); err == nil {
			v = x
		}
	}
	return min(max(v, lo), hi)
}

// oneOf mengembalikan nilai p[key] bila termasuk `allowed`; kosong → def; selain itu galat (dibaca model lalu diperbaiki).
func (p params) oneOf(key, def string, allowed ...string) (string, error) {
	v := p.str(key)
	if v == "" {
		return def, nil
	}
	for _, x := range allowed {
		if x == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("parameter %q harus salah satu dari: %s", key, strings.Join(allowed, ", "))
}

// dateRange membaca from/to (YYYY-MM-DD). Bawaan: 30 hari terakhir sampai hari ini. Maksimum 366 hari.
func (e *env) dateRange(p params) (from, to time.Time, err error) {
	to = e.today
	if s := p.str("to"); s != "" {
		if to, err = time.Parse(time.DateOnly, s); err != nil {
			return from, to, errors.New("format `to` harus YYYY-MM-DD")
		}
	}
	from = to.AddDate(0, 0, -29)
	if s := p.str("from"); s != "" {
		if from, err = time.Parse(time.DateOnly, s); err != nil {
			return from, to, errors.New("format `from` harus YYYY-MM-DD")
		}
	}
	if to.Before(from) {
		return from, to, errors.New("`to` tidak boleh sebelum `from`")
	}
	if to.Sub(from) > maxRangeDays*24*time.Hour {
		return from, to, fmt.Errorf("rentang maksimum %d hari; pecah menjadi beberapa panggilan", maxRangeDays)
	}
	return from, to, nil
}

// outlets = outlet yang boleh dilihat: `outlet_id` bila diberikan (harus boleh diakses pengguna), selain itu semua outlet pengguna.
func (e *env) outlets(p params) ([]uuid.UUID, error) {
	if s := p.str("outlet_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil || !e.a.Outlets[id] {
			return nil, errors.New("outlet_id tidak dikenal atau tidak boleh diakses; panggil list_outlets")
		}
		return []uuid.UUID{id}, nil
	}
	ids := make([]uuid.UUID, 0, len(e.a.Outlets))
	for id, ok := range e.a.Outlets {
		if ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("pengguna tidak punya outlet aktif")
	}
	return ids, nil
}

// rows menjalankan query dan mengubah hasilnya menjadi peta nilai JSON (angka uang/qty sebagai string desimal).
// Membaca maksimal maxRows baris; `truncated` = masih ada baris lain.
func (e *env) rows(sql string, args ...any) ([]map[string]any, bool, error) {
	rs, err := e.tx.Query(e.ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rs.Close()
	names := make([]string, 0, 8)
	for _, f := range rs.FieldDescriptions() {
		names = append(names, f.Name)
	}
	out := make([]map[string]any, 0, 16)
	truncated := false
	for rs.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		vals, err := rs.Values()
		if err != nil {
			return nil, false, err
		}
		m := make(map[string]any, len(vals))
		for i, v := range vals {
			m[names[i]] = jsonValue(v)
		}
		out = append(out, m)
	}
	if err := rs.Err(); err != nil {
		return nil, false, err
	}
	return out, truncated, nil
}

func jsonValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case pgtype.Numeric:
		if !x.Valid {
			return nil
		}
		dv, err := x.Value()
		if err != nil {
			return nil
		}
		return dv
	case [16]byte:
		return uuid.UUID(x).String()
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case pgtype.Date:
		if !x.Valid {
			return nil
		}
		return x.Time.Format(time.DateOnly)
	case driver.Valuer:
		if dv, err := x.Value(); err == nil {
			return dv
		}
		return fmt.Sprint(x)
	case []byte:
		return string(x)
	default:
		return v
	}
}

// anonymize mengganti kunci `field` (nama pelanggan) pada baris yang berurutan dengan "Pelanggan N" dan membuang kolom pengenal.
func anonymize(rows []map[string]any, drop ...string) {
	for i, r := range rows {
		for _, d := range drop {
			delete(r, d)
		}
		r["pelanggan"] = fmt.Sprintf("Pelanggan %d", i+1)
	}
}

type tool struct {
	name   string
	desc   string
	schema json.RawMessage
	module string // izin `view` yang dibutuhkan ("" = cukup login)
	run    func(e *env, p params) (any, error)
}

func (t tool) allowed(a authz.Actor) bool {
	return t.module == "" || a.Perms.Has(t.module, authz.ActView)
}

// schema membangun JSON Schema objek dari peta properti; semua properti opsional kecuali yang disebut di `required`.
func schema(props map[string]any, required ...string) json.RawMessage {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	b, _ := json.Marshal(m)
	return b
}

var (
	pFrom   = map[string]any{"type": "string", "description": "Start date YYYY-MM-DD (local outlet date). Default: 29 days before `to`."}
	pTo     = map[string]any{"type": "string", "description": "End date YYYY-MM-DD, inclusive. Default: today."}
	pOutlet = map[string]any{"type": "string", "description": "Outlet id (uuid) from list_outlets. Omit to include all outlets the user may access."}
)

func enum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "enum": vals, "description": desc}
}

func intProp(desc string, lo, hi int) map[string]any {
	return map[string]any{"type": "integer", "minimum": lo, "maximum": hi, "description": desc}
}

// result membungkus daftar baris; model diberi tahu bila hasil dipotong.
func result(rows []map[string]any, truncated bool, extra map[string]any) map[string]any {
	m := map[string]any{"rows": rows, "row_count": len(rows)}
	if truncated {
		m["truncated"] = true
		m["note"] = fmt.Sprintf("Only the first %d rows are shown; narrow the filters for the rest.", maxRows)
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// execute menjalankan satu tool dan menyerialkan hasilnya (teks JSON untuk tool_result).
func execute(e *env, t tool, raw json.RawMessage) (string, error) {
	if !t.allowed(e.a) {
		return "", errors.New("pengguna tidak punya izin untuk data ini")
	}
	p := params{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", errors.New("parameter bukan JSON objek yang valid")
		}
	}
	res, err := t.run(e, p)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	if len(b) > maxResultBytes {
		return "", errors.New("hasil terlalu besar; persempit rentang tanggal atau filter, atau gunakan group_by yang lebih ringkas")
	}
	return string(b), nil
}
