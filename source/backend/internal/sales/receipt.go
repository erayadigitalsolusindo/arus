package sales

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
)

// Receipt = satu model struk yang disusun server (FR-POS-15). Klien (browser kasir, print-agent, kasir mobile)
// hanya menata letak data ini, sehingga isi struk identik di semua perangkat.
type Receipt struct {
	Store ReceiptStore `json:"store"`
	Sale  Sale         `json:"sale"`
	// LocalTime = waktu nota menurut zona waktu outlet (format 02/01/2006 15:04), agar jam di struk tidak
	// bergantung pada jam/zona perangkat yang mencetak.
	LocalTime string `json:"local_time"`
	// Reprints = berapa kali struk ini sudah dicetak ulang (dari audit sale.reprint).
	Reprints int `json:"reprints"`
}

// ReceiptStore = identitas toko di kepala/kaki struk.
type ReceiptStore struct {
	TenantName string `json:"tenant_name"`
	OutletCode string `json:"outlet_code"`
	OutletName string `json:"outlet_name"`
	Address    string `json:"address"`
	Phone      string `json:"phone"`
	Header     string `json:"header"`
	Footer     string `json:"footer"`
}

const receiptTimeLayout = "02/01/2006 15:04"

// Receipt membaca model struk. Aturan akses sama dengan Get (cabang yang boleh diakses; nota kasir lain
// hanya bagi pemegang daftar penjualan).
func (s *Service) Receipt(ctx context.Context, a authz.Actor, id uuid.UUID) (Receipt, error) {
	sale, err := s.Get(ctx, a, id)
	if err != nil {
		return Receipt{}, err
	}
	out := Receipt{Sale: sale}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var tz string
		if err := tx.QueryRow(ctx, `
			SELECT t.name, o.code, o.name, o.address, o.phone, o.receipt_header, o.receipt_footer, o.timezone
			FROM outlets o JOIN tenants t ON t.id = o.tenant_id
			WHERE o.tenant_id = $1 AND o.id = $2`, a.TenantID, sale.OutletID).Scan(
			&out.Store.TenantName, &out.Store.OutletCode, &out.Store.OutletName, &out.Store.Address, &out.Store.Phone,
			&out.Store.Header, &out.Store.Footer, &tz); err != nil {
			return err
		}
		out.LocalTime = localTime(sale.CreatedAt, tz)
		n, err := reprintCount(ctx, tx, a.TenantID, id)
		out.Reprints = n
		return err
	})
	return out, err
}

// Reprint mencatat satu cetak ulang di audit dan mengembalikan nomor salinan (1 = cetak ulang pertama).
// Baris nota dikunci agar dua cetak ulang bersamaan tidak mendapat nomor yang sama.
func (s *Service) Reprint(ctx context.Context, a authz.Actor, id uuid.UUID) (int, error) {
	sale, err := s.Get(ctx, a, id)
	if err != nil {
		return 0, err
	}
	var copyNo int
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM sales WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, a.TenantID, id); err != nil {
			return err
		}
		n, err := reprintCount(ctx, tx, a.TenantID, id)
		if err != nil {
			return err
		}
		copyNo = n + 1
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionSaleReprint, Entity: audit.EntitySale, EntityID: id.String(),
			Details: map[string]any{"doc_no": sale.DocNo, "copy": copyNo},
		})
	})
	return copyNo, err
}

func reprintCount(ctx context.Context, tx pgx.Tx, tenantID, saleID uuid.UUID) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND entity = $2 AND entity_id = $3 AND action = $4`,
		tenantID, audit.EntitySale, saleID.String(), audit.ActionSaleReprint).Scan(&n)
	return n, err
}

func localTime(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format(receiptTimeLayout)
}
