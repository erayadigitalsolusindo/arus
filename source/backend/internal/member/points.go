package member

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// Jenis movement poin (sama dengan CHECK di DB).
const (
	KindEarn     = "EARN"
	KindRedeem   = "REDEEM"
	KindAdjust   = "ADJUST"
	KindReversal = "REVERSAL"

	RefSale   = "SALE"
	RefManual = "MANUAL"

	maxAdjust = 1_000_000
)

var (
	ErrInsufficientPoints = errors.New("poin tidak cukup")
	ErrMemberInactive     = errors.New("member tidak aktif atau sudah kedaluwarsa")
)

// SaleMember = member yang terkunci (FOR UPDATE) di dalam transaksi penjualan beserta aturan poin levelnya.
type SaleMember struct {
	ID             uuid.UUID
	Code, Name     string
	Points         int
	LifetimePoints int
	SpendPerPoint  decimal.Decimal
	PointValue     decimal.Decimal
}

// LockForSale mengunci baris member (lock=true, untuk menyimpan nota) atau hanya membacanya (lock=false, pratinjau) dan
// memastikan boleh dipakai bertransaksi pada hari localDay (zona waktu outlet).
// pgx.ErrNoRows → member tidak ada di tenant ini; ErrMemberInactive → nonaktif/kedaluwarsa.
func LockForSale(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, localDay pgtype.Date, lock bool) (SaleMember, error) {
	q := gen.New(tx)
	var r gen.MemberLockForSaleRow
	var err error
	if lock {
		r, err = q.MemberLockForSale(ctx, gen.MemberLockForSaleParams{TenantID: tenant, ID: id})
	} else {
		var rr gen.MemberReadForSaleRow
		rr, err = q.MemberReadForSale(ctx, gen.MemberReadForSaleParams{TenantID: tenant, ID: id})
		r = gen.MemberLockForSaleRow(rr)
	}
	if err != nil {
		return SaleMember{}, err
	}
	if !r.Active || (r.ValidUntil.Valid && localDay.Valid && r.ValidUntil.Time.Before(localDay.Time)) {
		return SaleMember{}, ErrMemberInactive
	}
	return SaleMember{ID: r.ID, Code: r.Code, Name: r.Name, Points: int(r.Points), LifetimePoints: int(r.LifetimePoints),
		SpendPerPoint: r.SpendPerPoint, PointValue: r.PointValue}, nil
}

// PointsFor = poin yang diperoleh dari dasar belanja: floor(base ÷ spendPerPoint); 0 bila aturan 0 (tidak ada pembagian nol).
func PointsFor(base, spendPerPoint decimal.Decimal) int {
	if !spendPerPoint.IsPositive() || !base.IsPositive() {
		return 0
	}
	n := base.Div(spendPerPoint).Floor()
	if n.GreaterThan(decimal.NewFromInt(maxAdjust * 100)) {
		return maxAdjust * 100
	}
	return int(n.IntPart())
}

// RedeemAmount = nilai potongan dari penukaran `points` poin.
func RedeemAmount(points int, pointValue decimal.Decimal) decimal.Decimal {
	return pointValue.Mul(decimal.NewFromInt(int64(points)))
}

type move struct {
	tenant, member uuid.UUID
	kind           string
	delta          int32
	lifetimeDelta  int32
	refType        string
	refID          uuid.UUID
	note           string
	actor          uuid.UUID
}

// apply mengubah saldo dan menulis ledger; harus dipanggil dalam transaksi pemanggil. Saldo/lifetime tidak boleh minus.
func apply(ctx context.Context, q *gen.Queries, m move) (int, error) {
	if m.delta == 0 {
		return 0, errors.New("movement poin nol")
	}
	res, err := q.MemberPointsApply(ctx, gen.MemberPointsApplyParams{TenantID: m.tenant, ID: m.member, Delta: m.delta, LifetimeDelta: m.lifetimeDelta})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInsufficientPoints
	}
	if err != nil {
		return 0, err
	}
	_, err = q.MemberPointInsert(ctx, gen.MemberPointInsertParams{TenantID: m.tenant, MemberID: m.member, Kind: m.kind, Points: m.delta,
		LifetimeDelta: m.lifetimeDelta, BalanceAfter: res.Points, RefType: m.refType,
		RefID: pgtype.UUID{Bytes: m.refID, Valid: m.refID != uuid.Nil}, Note: m.note, ActorID: pgtype.UUID{Bytes: m.actor, Valid: m.actor != uuid.Nil}})
	return int(res.Points), err
}

// ApplySale mencatat tukar poin (−redeem) lalu perolehan poin (+earn) dari satu nota. Member harus sudah dikunci
// (LockForSale) di transaksi yang sama.
func ApplySale(ctx context.Context, tx pgx.Tx, a authz.Actor, memberID, saleID uuid.UUID, docNo string, redeem, earn int) error {
	q := gen.New(tx)
	if redeem > 0 {
		if _, err := apply(ctx, q, move{tenant: a.TenantID, member: memberID, kind: KindRedeem, delta: -int32(redeem),
			refType: RefSale, refID: saleID, note: docNo, actor: a.UserID}); err != nil {
			return err
		}
	}
	if earn > 0 {
		if _, err := apply(ctx, q, move{tenant: a.TenantID, member: memberID, kind: KindEarn, delta: int32(earn), lifetimeDelta: int32(earn),
			refType: RefSale, refID: saleID, note: docNo, actor: a.UserID}); err != nil {
			return err
		}
	}
	return nil
}

// ReverseSale membalik semua poin sebuah nota (untuk void/retur/edit, Fase 5.3): poin yang ditukar dikembalikan,
// poin yang diperoleh ditarik. Idempoten (nota yang sudah dibalik tidak dibalik lagi). Bila poin hasil nota sudah
// terpakai, penarikan dibatasi sampai saldo 0 (saldo tidak pernah minus); yang dicatat adalah selisih sebenarnya.
func ReverseSale(ctx context.Context, tx pgx.Tx, a authz.Actor, saleID uuid.UUID, note string) error {
	q := gen.New(tx)
	moves, err := q.MemberPointsBySale(ctx, gen.MemberPointsBySaleParams{TenantID: a.TenantID, RefID: pgtype.UUID{Bytes: saleID, Valid: true}})
	if err != nil {
		return err
	}
	for _, m := range moves {
		if m.Kind == KindReversal {
			return nil // sudah pernah dibalik
		}
	}
	for _, m := range moves {
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: m.MemberID})
		if err != nil {
			return err
		}
		delta, life := -m.Points, -m.LifetimeDelta
		if delta < -cur.Points {
			delta = -cur.Points
		}
		if life < -cur.LifetimePoints {
			life = -cur.LifetimePoints
		}
		if delta == 0 && life == 0 {
			continue
		}
		if delta == 0 { // hanya lifetime yang berubah: ledger butuh points ≠ 0, jadi lifetime disesuaikan tanpa baris
			if _, err := q.MemberPointsApply(ctx, gen.MemberPointsApplyParams{TenantID: a.TenantID, ID: m.MemberID, Delta: 0, LifetimeDelta: life}); err != nil {
				return err
			}
			continue
		}
		if _, err := apply(ctx, q, move{tenant: a.TenantID, member: m.MemberID, kind: KindReversal, delta: delta, lifetimeDelta: life,
			refType: RefSale, refID: saleID, note: note, actor: a.UserID}); err != nil {
			return err
		}
	}
	return nil
}

// ---- penyesuaian manual & riwayat ----

// Adjust menambah/mengurangi poin secara manual (alasan wajib, tercatat di ledger dan audit). Penambahan juga
// menambah lifetime_points (mis. saldo awal poin member lama); pengurangan tidak mengurangi lifetime.
func (s *Service) Adjust(ctx context.Context, a authz.Actor, id uuid.UUID, delta int, rawNote string) (*Member, error) {
	f := FieldErrors{}
	if delta == 0 || delta > maxAdjust || delta < -maxAdjust {
		f["points"] = sanitize.Invalid
	}
	note, code := plain(rawNote, 200)
	if code == "" && note == "" {
		code = sanitize.Required
	}
	if code != "" {
		f["note"] = code
	}
	if len(f) > 0 {
		return nil, f
	}
	var m Member
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		life := int32(0)
		if delta > 0 {
			life = int32(delta)
		}
		after, err := apply(ctx, q, move{tenant: a.TenantID, member: id, kind: KindAdjust, delta: int32(delta), lifetimeDelta: life,
			refType: RefManual, note: note, actor: a.UserID})
		if errors.Is(err, ErrInsufficientPoints) {
			return FieldErrors{"points": "POINTS_INSUFFICIENT"}
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionMemberPointsAdjust, Entity: audit.EntityMember, EntityID: id.String(),
			Details: map[string]any{"code": cur.Code, "name": cur.Name, "points": delta, "balance_before": cur.Points, "balance_after": after, "note": note}}); err != nil {
			return err
		}
		m, err = load(ctx, tx, a.TenantID, id)
		return err
	})
	if err = mapWriteErr(err); err != nil {
		return nil, err
	}
	return &m, nil
}

type PointMove struct {
	ID           int64      `json:"id"`
	Kind         string     `json:"kind"`
	Points       int        `json:"points"`
	BalanceAfter int        `json:"balance_after"`
	RefType      string     `json:"ref_type"`
	RefID        *uuid.UUID `json:"ref_id"`
	DocNo        string     `json:"doc_no"`
	Note         string     `json:"note"`
	Actor        string     `json:"actor"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Points = riwayat poin member (terbaru dulu).
func (s *Service) Points(ctx context.Context, a authz.Actor, id uuid.UUID, limit, offset int) ([]PointMove, int, error) {
	if limit <= 0 {
		limit = defLimit
	}
	out := []PointMove{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if _, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id}); err != nil {
			return err
		}
		rows, err := q.MemberPointList(ctx, gen.MemberPointListParams{TenantID: a.TenantID, MemberID: id, PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(offset, 0))})
		for _, r := range rows {
			out = append(out, PointMove{ID: r.ID, Kind: r.Kind, Points: int(r.Points), BalanceAfter: int(r.BalanceAfter), RefType: r.RefType,
				RefID: uuidPtr(r.RefID), DocNo: r.DocNo, Note: r.Note, Actor: r.ActorName, CreatedAt: r.CreatedAt.Time})
			total = int(r.Total)
		}
		return err
	})
	return out, total, mapWriteErr(err)
}

// SaleRow = satu transaksi member pada riwayat.
type SaleRow struct {
	ID             uuid.UUID `json:"id"`
	DocNo          string    `json:"doc_no"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	Outlet         string    `json:"outlet"`
	Cashier        string    `json:"cashier"`
	Total          string    `json:"total"`
	LineCount      int       `json:"line_count"`
	Items          string    `json:"items"`
	PointsEarned   int       `json:"points_earned"`
	PointsRedeemed int       `json:"points_redeemed"`
}

// Sales = riwayat transaksi member (terbaru dulu).
func (s *Service) Sales(ctx context.Context, a authz.Actor, id uuid.UUID, limit, offset int) ([]SaleRow, int, error) {
	if limit <= 0 {
		limit = defLimit
	}
	out := []SaleRow{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if _, err := q.MemberGetForUpdate(ctx, gen.MemberGetForUpdateParams{TenantID: a.TenantID, ID: id}); err != nil {
			return err
		}
		rows, err := q.MemberSaleList(ctx, gen.MemberSaleListParams{TenantID: a.TenantID, MemberID: pgtype.UUID{Bytes: id, Valid: true}, PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(offset, 0))})
		for _, r := range rows {
			out = append(out, SaleRow{ID: r.ID, DocNo: r.DocNo, Status: r.Status, CreatedAt: r.CreatedAt.Time, Outlet: r.OutletName, Cashier: r.Cashier,
				Total: r.Total.StringFixed(2), LineCount: int(r.LineCount), Items: r.Items, PointsEarned: int(r.PointsEarned), PointsRedeemed: int(r.PointsRedeemed)})
			total = int(r.TotalRows)
		}
		return err
	})
	return out, total, mapWriteErr(err)
}
