package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	gen "aciraba/internal/gen"
)

// maxDetailsBytes membatasi ukuran satu catatan; rincian berlebih dipotong agar tabel tidak membengkak.
const maxDetailsBytes = 8 * 1024

var secretKey = regexp.MustCompile(`(?i)pass|token|secret|hash|^pin`)

// Record menulis satu catatan audit memakai koneksi/transaksi yang diberikan (idealnya transaksi perubahan bisnis,
// di bawah WithTenant). Error harus ikut menggagalkan transaksi pemanggil.
func Record(ctx context.Context, db gen.DBTX, a Actor, e Entry) error {
	if a.TenantID == uuid.Nil {
		return fmt.Errorf("audit: tenant kosong")
	}
	raw, err := json.Marshal(scrub(e.Details))
	if err != nil {
		return fmt.Errorf("audit: details: %w", err)
	}
	if len(raw) > maxDetailsBytes {
		raw = []byte(`{"truncated":true}`)
	}
	m := MetaFrom(ctx)
	return gen.New(db).AuditInsert(ctx, gen.AuditInsertParams{
		TenantID:  a.TenantID,
		OutletID:  nullUUID(a.OutletID),
		ActorID:   nullUUID(a.UserID),
		ActorName: a.Name,
		Action:    e.Action,
		Entity:    e.Entity,
		EntityID:  e.EntityID,
		Details:   raw,
		Ip:        m.IP,
		RequestID: m.RequestID,
	})
}

func nullUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

// scrub membuang kunci yang berbau rahasia (rekursif pada map bersarang).
func scrub(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if secretKey.MatchString(k) {
			continue
		}
		if sub, ok := v.(map[string]any); ok {
			v = scrub(sub)
		}
		out[k] = v
	}
	return out
}
