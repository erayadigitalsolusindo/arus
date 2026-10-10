package sales

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/member"
	"aciraba/internal/platform/db"
	"aciraba/internal/wallet"
)

// VoidSaleReturn membatalkan retur penjualan (keputusan pengguna 2026-10-10, mengikuti aturan batal retur pembelian):
//   - ditolak bila dana kembali sudah diserahkan ke pelanggan lewat metode biasa (uang yang keluar tak boleh hilang dari
//     catatan); dana kembali ke DEPOSIT member boleh — saldo deposit ditarik lagi (ditolak bila sudah terpakai);
//   - ditolak bila piutang nota dibayar sesudah retur dibuat (potongan piutang sudah dipakai sebagai dasar pembayaran);
//   - barang keluar lagi dari bucket Retur outlet retur (ditolak bila stok Retur kurang), HPP cabang dihitung mundur;
//   - poin member dikembalikan seperti sebelum retur (poin nota diperoleh lagi, poin yang dikembalikan ditarik lagi).
//
// Kunci: nota asal dulu (sama dengan buat retur, bayar piutang, edit/batal nota), lalu dokumen retur.
func (s *Service) VoidSaleReturn(ctx context.Context, a authz.Actor, id uuid.UUID, reason string) (SaleReturn, error) {
	reason, fe := cleanReason(reason)
	if len(fe) > 0 {
		return SaleReturn{}, fe
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var saleID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT sale_id FROM sales_returns WHERE tenant_id = $1 AND id = $2`, a.TenantID, id).Scan(&saleID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := loadSaleReturnSale(ctx, tx, a.TenantID, saleID, true); err != nil {
			return err
		}
		var (
			outlet, saleRef           uuid.UUID
			memberID                  pgtype.UUID
			status, docNo, refundKind string
			createdAt                 time.Time
			refund, cut               dec
			earned, redeemed          int
		)
		if err := tx.QueryRow(ctx, `SELECT outlet_id, sale_id, member_id, status, doc_no, coalesce(refund_method, ''), created_at, refund,
			receivable_cut, points_earned_reversed, points_redeemed_restored
			FROM sales_returns WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, a.TenantID, id).Scan(&outlet, &saleRef, &memberID, &status, &docNo,
			&refundKind, &createdAt, &refund, &cut, &earned, &redeemed); err != nil {
			return err
		}
		if outlet != a.OutletID && !a.Outlets[outlet] {
			return ErrOutletForbidden
		}
		if status != "completed" {
			return ErrSaleReturnInactive
		}
		if refund.IsPositive() && refundKind != wallet.KindDeposit {
			return ErrSaleReturnRefunded
		}
		if cut.IsPositive() {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM receivable_payments rp JOIN receivables r ON r.tenant_id = rp.tenant_id AND r.id = rp.receivable_id
				WHERE r.tenant_id = $1 AND r.sale_id = $2 AND rp.created_at >= $3`, a.TenantID, saleRef, createdAt).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrSaleReturnLocked
			}
		}
		rows, err := tx.Query(ctx, `SELECT rl.item_id, i.kind, rl.qty, rl.base_qty, rl.unit_cost
			FROM sales_return_lines rl JOIN items i ON i.tenant_id = rl.tenant_id AND i.id = rl.item_id
			WHERE rl.tenant_id = $1 AND rl.return_id = $2 ORDER BY rl.position`, a.TenantID, id)
		if err != nil {
			return err
		}
		var lines []saleReturnCalcLine
		for rows.Next() {
			var l saleReturnCalcLine
			if err := rows.Scan(&l.source.itemID, &l.source.kind, &l.qty, &l.baseQty, &l.source.unitCost); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if err := applySaleReturnStock(ctx, tx, a, outlet, id, docNo+" (batal)", lines, true); err != nil {
			return err
		}
		if memberID.Valid {
			if err := member.ApplySaleReturn(ctx, tx, a, uuid.UUID(memberID.Bytes), saleRef, id, docNo+" (batal)", -earned, -redeemed); err != nil {
				return err
			}
		}
		if refund.IsPositive() { // dana kembali ke deposit: ditarik lagi
			if _, err := wallet.MemberDeposit.Apply(ctx, tx, wallet.Move{TenantID: a.TenantID, OwnerID: uuid.UUID(memberID.Bytes), OutletID: outlet,
				Kind: wallet.DepSaleReturnVoid, Amount: refund.Neg(), RefID: id, DocNo: docNo, Note: "batal retur", ActorID: a.UserID}); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE sales_returns SET status = 'void', void_reason = $3, voided_at = now(), voided_by = $4
			WHERE tenant_id = $1 AND id = $2 AND status = 'completed'`, a.TenantID, id, reason, optionalUUID(a.UserID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrSaleReturnInactive
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleReturnVoid, Entity: audit.EntitySaleReturn,
			EntityID: id.String(), Details: map[string]any{"doc_no": docNo, "sale_id": saleRef.String(), "reason": reason,
				"refund_to_deposit": refund.StringFixed(2)}})
	})
	if err != nil {
		return SaleReturn{}, err
	}
	return s.GetSaleReturn(ctx, a, id)
}
