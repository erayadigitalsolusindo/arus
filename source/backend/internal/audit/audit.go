// Package audit mencatat siapa mengubah apa (append-only, per tenant). Pencatatan dilakukan DI DALAM transaksi
// yang sama dengan perubahan bisnisnya (audit.Record(ctx, tx, ...)), sehingga catatan dan perubahan selalu
// berhasil atau gagal bersama (AGENTS.md §3.2).
package audit

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"aciraba/internal/platform/httpx"
)

// Actor = pelaku perubahan. UserID/OutletID kosong (uuid.Nil) disimpan sebagai NULL (aksi sistem).
type Actor struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	OutletID uuid.UUID
	Name     string
}

// Entry = satu kejadian. Details berisi ringkasan perubahan (mis. nilai sebelum/sesudah); kunci yang berbau
// rahasia (password, token, hash, secret) dibuang otomatis. Jangan memasukkan data pribadi yang tidak perlu.
type Entry struct {
	Action   string // mis. "role.update" (lihat actions.go)
	Entity   string // mis. "role"
	EntityID string
	Details  map[string]any
}

// Meta = konteks permintaan HTTP yang ikut dicatat.
type Meta struct {
	IP        string
	RequestID string
}

type metaKey struct{}

func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

func MetaFrom(ctx context.Context) Meta {
	m, _ := ctx.Value(metaKey{}).(Meta)
	return m
}

// CaptureMeta menyimpan IP klien dan request id ke context agar Record dapat melampirkannya. Dipasang global.
func CaptureMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := Meta{IP: httpx.ClientIP(r), RequestID: middleware.GetReqID(r.Context())}
		next.ServeHTTP(w, r.WithContext(WithMeta(r.Context(), m)))
	})
}
