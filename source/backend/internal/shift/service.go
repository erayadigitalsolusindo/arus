// Package shift: shift kasir (FR-POS-16). Kasir membuka shift dengan modal awal sebelum menyimpan nota; saat tutup,
// sistem menghitung uang yang seharusnya per metode (modal awal + penjualan + arus lain milik kasir itu di outlet itu
// selama rentang shift), kasir mengisi uang/bukti fisik, dan selisih ≠ 0 wajib catatan + PIN penyetuju. Rekap
// dibekukan (cash_shift_counts/flows) sehingga edit nota sesudahnya tidak mengubah shift yang sudah ditutup.
package shift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	// Module = izin daftar/rincian shift semua kasir (supervisor) dan menutup shift kasir lain (update).
	Module = "cash_shifts"
	// ModuleApprove = izin penyetuju tutup shift yang memiliki selisih (PIN yang sama dengan penyetuju lain).
	ModuleApprove = approval.ModuleShiftClose
	// ModuleCashier = kasir boleh membuka/menutup shift miliknya bila boleh membuat nota.
	ModuleCashier = "sales_orders"

	maxAmount  = 1_000_000_000_000_000 // < 1e15 (CHECK DB)
	noteMax    = 500
	listMax    = 100
	timeLayout = "02/01/2006 15:04"
)

var (
	ErrRequired       = errors.New("shift kasir belum dibuka")
	ErrAlreadyOpen    = errors.New("shift kasir sudah terbuka")
	ErrNotOpen        = errors.New("shift sudah ditutup")
	ErrNotFound       = errors.New("shift tidak ditemukan")
	ErrForbidden      = errors.New("tidak berhak atas shift ini")
	ErrOutletInactive = errors.New("outlet tidak aktif")
	ErrKeyRequired    = errors.New("Idempotency-Key wajib")
	ErrRecapChanged   = errors.New("rekap shift berubah")
	ErrKeyMismatch    = errors.New("Idempotency-Key sudah dipakai untuk shift lain")

	keyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)
)

// FieldErrors = galat validasi per field (kode stabil).
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

type Service struct {
	pool      *pgxpool.Pool
	approvals *approval.Service // nil = tutup shift berselisih tidak tersedia (tes tertentu)
	now       func() time.Time
}

func NewService(pool *pgxpool.Pool, approvals *approval.Service) *Service {
	return &Service{pool: pool, approvals: approvals, now: time.Now}
}

// Guard dipanggil sales.save di dalam transaksi nota: kasir harus punya shift terbuka di outlet aktif. Baris shift
// dikunci FOR SHARE sehingga tutup shift (FOR UPDATE) menunggu nota yang sedang disimpan, dan nota yang datang
// sesudah shift ditutup ditolak — rekap yang dibekukan selalu memuat semua nota shift itu.
func Guard(ctx context.Context, tx pgx.Tx, a authz.Actor) error {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM cash_shifts WHERE tenant_id = $1 AND outlet_id = $2 AND user_id = $3 AND status = 'open' FOR SHARE`,
		a.TenantID, a.OutletID, a.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequired
	}
	return err
}

// Store = identitas toko untuk kepala struk tutup shift.
type Store struct {
	TenantName string `json:"tenant_name"`
	OutletCode string `json:"outlet_code"`
	OutletName string `json:"outlet_name"`
	Address    string `json:"address"`
	Phone      string `json:"phone"`
}

// Shift = header shift (+ rekap: dibekukan bila closed, dihitung langsung bila open).
type Shift struct {
	ID              uuid.UUID  `json:"id"`
	DocNo           string     `json:"doc_no"`
	Status          string     `json:"status"`
	OutletID        uuid.UUID  `json:"outlet_id"`
	UserID          uuid.UUID  `json:"user_id"`
	UserName        string     `json:"user_name"`
	OpeningCash     string     `json:"opening_cash"`
	OpenedAt        time.Time  `json:"opened_at"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	ClosedByName    string     `json:"closed_by_name,omitempty"`
	ExpectedTotal   string     `json:"expected_total"`
	CountedTotal    *string    `json:"counted_total"`
	DiffTotal       *string    `json:"diff_total"`
	DiffAbs         *string    `json:"diff_abs"`
	SaleCount       int        `json:"sale_count"`
	VoidCount       int        `json:"void_count"`
	SalesTotal      string     `json:"sales_total"`
	ReceivableTotal string     `json:"receivable_total"`
	Note            string     `json:"note"`
	ApprovedByName  string     `json:"approved_by_name,omitempty"`
	Counts          []Count    `json:"counts"`
	Flows           []Flow     `json:"flows"`
	// Waktu menurut zona waktu outlet (untuk struk), format 02/01/2006 15:04.
	OpenedLocal string `json:"opened_local"`
	ClosedLocal string `json:"closed_local,omitempty"`
	Store       Store  `json:"store"`
}

// Count = rekap satu metode. Counted/Diff kosong selama shift masih terbuka.
type Count struct {
	MethodID uuid.UUID `json:"method_id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Sales    string    `json:"sales"`
	Flows    string    `json:"flows"`
	Opening  string    `json:"opening"`
	Expected string    `json:"expected"`
	Counted  *string   `json:"counted"`
	Diff     *string   `json:"diff"`
}

// Flow = arus uang lain per sumber + metode (bertanda: + masuk laci, − keluar).
type Flow struct {
	Source   string    `json:"source"`
	MethodID uuid.UUID `json:"method_id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Amount   string    `json:"amount"`
	Count    int       `json:"count"`
}

type dec = decimal.Decimal

func parseMoney(raw json.Number, allowZero bool) (dec, bool) {
	s := strings.TrimSpace(raw.String())
	if s == "" {
		if allowZero {
			return decimal.Zero, true
		}
		return dec{}, false
	}
	d, err := decimal.NewFromString(s)
	if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(maxAmount)) {
		return dec{}, false
	}
	return d, true
}

// ---------------------------------------------------------------- buka

type OpenInput struct {
	OpeningCash json.Number `json:"opening_cash"`
}

// Open membuka shift kasir di outlet aktif. Satu shift terbuka per kasir per outlet (indeks unik): pembukaan ganda
// bersamaan → satu berhasil, sisanya ErrAlreadyOpen.
func (s *Service) Open(ctx context.Context, a authz.Actor, in OpenInput) (Shift, error) {
	cash, ok := parseMoney(in.OpeningCash, true)
	if !ok {
		return Shift{}, FieldErrors{"opening_cash": sanitize.Invalid}
	}
	var id uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var code string
		var active bool
		var day time.Time
		err := tx.QueryRow(ctx, `SELECT code, active, (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&code, &active, &day)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
			return ErrOutletInactive
		}
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM cash_shifts WHERE tenant_id = $1 AND outlet_id = $2 AND user_id = $3 AND status = 'open')`,
			a.TenantID, a.OutletID, a.UserID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrAlreadyOpen
		}
		var no int64
		if err := tx.QueryRow(ctx, `INSERT INTO cash_shift_counters (tenant_id, outlet_id, day, last_no) VALUES ($1, $2, $3, 1)
			ON CONFLICT (tenant_id, outlet_id, day) DO UPDATE SET last_no = cash_shift_counters.last_no + 1 RETURNING last_no`,
			a.TenantID, a.OutletID, day).Scan(&no); err != nil {
			return err
		}
		docNo := fmt.Sprintf("SH-%s-%s-%04d", code, day.Format("060102"), no)
		// opened_at = jam saat baris ditulis (bukan awal transaksi) agar nota yang lolos Guard selalu ≥ opened_at.
		err = tx.QueryRow(ctx, `INSERT INTO cash_shifts (tenant_id, outlet_id, user_id, doc_no, opening_cash, opened_at)
			VALUES ($1, $2, $3, $4, $5, clock_timestamp())
			ON CONFLICT (tenant_id, outlet_id, user_id) WHERE status = 'open' DO NOTHING RETURNING id`,
			a.TenantID, a.OutletID, a.UserID, docNo, cash).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAlreadyOpen // pembukaan lain menang (nomor ikut batal karena transaksi dibatalkan)
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionShiftOpen, Entity: audit.EntityShift, EntityID: id.String(),
			Details: map[string]any{"doc_no": docNo, "opening_cash": cash.StringFixed(2)}})
	})
	if err != nil {
		return Shift{}, err
	}
	return s.get(ctx, a, id, false)
}

// Current = shift terbuka milik pemanggil di outlet aktif (dengan rekap berjalan), atau nil.
func (s *Service) Current(ctx context.Context, a authz.Actor) (*Shift, error) {
	var id uuid.UUID
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM cash_shifts WHERE tenant_id = $1 AND outlet_id = $2 AND user_id = $3 AND status = 'open'`,
			a.TenantID, a.OutletID, a.UserID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sh, err := s.get(ctx, a, id, false)
	if err != nil {
		return nil, err
	}
	return &sh, nil
}

// ---------------------------------------------------------------- rekap

// recapSQL: $1 tenant, $2 outlet, $3 kasir, $4..$5 rentang waktu [t0, t1). Penjualan = nota completed milik kasir itu
// (revisi membawa kasir & waktu nota asli sehingga tetap jatuh di shift aslinya). Arus lain = dokumen uang yang dibuat
// petugas itu di outlet itu. Metode saldo titipan (deposit/kredit pemasok) bukan uang laci.
const recapSQL = `
WITH x AS (
  SELECT 'sale' AS src, p.method_id, p.method AS kind,
         p.amount + CASE WHEN p.fee_bearer = 'customer' THEN p.fee_amount ELSE 0 END
           - CASE WHEN p.method = 'cash' THEN s.change ELSE 0 END AS amt  -- satu baris tunai per nota (metode unik per nota)
  FROM sales s JOIN sale_payments p ON p.tenant_id = s.tenant_id AND p.sale_id = s.id
  WHERE s.tenant_id = $1 AND s.outlet_id = $2 AND s.cashier_id = $3 AND s.status = 'completed' AND s.created_at >= $4 AND s.created_at < $5
  UNION ALL
  SELECT 'receivable_payment', rp.method_id, rp.method,
         rp.amount + CASE WHEN rp.fee_bearer = 'customer' THEN rp.fee_amount ELSE 0 END
  FROM receivable_payments rp
  WHERE rp.tenant_id = $1 AND rp.outlet_id = $2 AND rp.received_by = $3 AND rp.created_at >= $4 AND rp.created_at < $5
  UNION ALL
  SELECT CASE d.kind WHEN 'TOPUP' THEN 'deposit_topup' ELSE 'deposit_withdraw' END, d.method_id, d.method, d.amount
  FROM member_deposit_movements d
  WHERE d.tenant_id = $1 AND d.outlet_id = $2 AND d.method_id IS NOT NULL AND d.actor_id = $3 AND d.created_at >= $4 AND d.created_at < $5
  UNION ALL
  SELECT 'sale_return', r.refund_method_id, r.refund_method, -r.refund
  FROM sales_returns r
  WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.refund > 0 AND r.created_by = $3 AND r.created_at >= $4 AND r.created_at < $5
  UNION ALL
  SELECT 'payable_payment', pp.method_id, pp.method, -pp.amount
  FROM payable_payments pp
  WHERE pp.tenant_id = $1 AND pp.outlet_id = $2 AND pp.paid_by = $3 AND pp.created_at >= $4 AND pp.created_at < $5
  UNION ALL
  SELECT 'purchase_return', r.refund_method_id, r.refund_method, r.refund
  FROM purchase_returns r
  WHERE r.tenant_id = $1 AND r.outlet_id = $2 AND r.status = 'completed' AND r.refund > 0 AND r.created_by = $3 AND r.created_at >= $4 AND r.created_at < $5
  UNION ALL
  SELECT 'supplier_credit_cashout', c.method_id, c.method, -c.amount
  FROM supplier_credit_movements c
  WHERE c.tenant_id = $1 AND c.outlet_id = $2 AND c.kind = 'CASH_OUT' AND c.actor_id = $3 AND c.created_at >= $4 AND c.created_at < $5
)
SELECT x.src, x.method_id, pm.name, x.kind, sum(x.amt), count(*)
FROM x JOIN payment_methods pm ON pm.tenant_id = $1 AND pm.id = x.method_id
WHERE x.kind NOT IN ('deposit', 'supplier_credit')
GROUP BY x.src, x.method_id, pm.name, x.kind`

type recap struct {
	counts     []Count
	flows      []Flow
	expected   dec
	saleCount  int
	voidCount  int
	salesTotal dec
	recvTotal  dec
}

// computeRecap menghitung rekap shift [t0, t1) untuk kasir `user`. Baris tunai (metode tunai sistem) selalu ada karena
// modal awal; metode lain hanya bila ada uang lewat metode itu.
func computeRecap(ctx context.Context, tx pgx.Tx, tenant, outlet, user uuid.UUID, opening dec, t0, t1 time.Time) (recap, error) {
	var r recap
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'completed'), count(*) FILTER (WHERE status = 'void'),
		       coalesce(sum(total) FILTER (WHERE status = 'completed'), 0), coalesce(sum(receivable) FILTER (WHERE status = 'completed'), 0)
		FROM sales WHERE tenant_id = $1 AND outlet_id = $2 AND cashier_id = $3 AND created_at >= $4 AND created_at < $5`,
		tenant, outlet, user, t0, t1).Scan(&r.saleCount, &r.voidCount, &r.salesTotal, &r.recvTotal); err != nil {
		return r, err
	}
	type acc struct {
		c            Count
		sales, flows dec
	}
	by := map[uuid.UUID]*acc{}
	var order []uuid.UUID
	get := func(id uuid.UUID, name, kind string) *acc {
		if v, ok := by[id]; ok {
			return v
		}
		v := &acc{c: Count{MethodID: id, Name: name, Kind: kind}}
		by[id] = v
		order = append(order, id)
		return v
	}
	var cashID uuid.UUID
	var cashName string
	err := tx.QueryRow(ctx, `SELECT id, name FROM payment_methods WHERE tenant_id = $1 AND kind = 'cash' ORDER BY is_system DESC, created_at LIMIT 1`, tenant).Scan(&cashID, &cashName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	if err == nil {
		get(cashID, cashName, "cash")
	}

	rows, err := tx.Query(ctx, recapSQL, tenant, outlet, user, t0, t1)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var src, name, kind string
		var mid uuid.UUID
		var sum dec
		var n int64
		if err := rows.Scan(&src, &mid, &name, &kind, &sum, &n); err != nil {
			return r, err
		}
		a := get(mid, name, kind)
		if src == "sale" {
			a.sales = a.sales.Add(sum)
			continue
		}
		a.flows = a.flows.Add(sum)
		r.flows = append(r.flows, Flow{Source: src, MethodID: mid, Name: name, Kind: kind, Amount: sum.StringFixed(2), Count: int(n)})
	}
	if err := rows.Err(); err != nil {
		return r, err
	}
	for _, id := range order {
		a := by[id]
		op := decimal.Zero
		if id == cashID {
			op = opening
		}
		exp := op.Add(a.sales).Add(a.flows)
		a.c.Sales, a.c.Flows, a.c.Opening, a.c.Expected = a.sales.StringFixed(2), a.flows.StringFixed(2), op.StringFixed(2), exp.StringFixed(2)
		r.expected = r.expected.Add(exp)
		r.counts = append(r.counts, a.c)
	}
	sortCounts(r.counts)
	sort.SliceStable(r.flows, func(i, j int) bool {
		if r.flows[i].Source != r.flows[j].Source {
			return r.flows[i].Source < r.flows[j].Source
		}
		return strings.ToLower(r.flows[i].Name) < strings.ToLower(r.flows[j].Name)
	})
	if r.flows == nil {
		r.flows = []Flow{}
	}
	if r.counts == nil {
		r.counts = []Count{}
	}
	return r, nil
}

func sortCounts(c []Count) {
	sort.SliceStable(c, func(i, j int) bool {
		ci, cj := c[i].Kind == "cash", c[j].Kind == "cash"
		if ci != cj {
			return ci
		}
		return strings.ToLower(c[i].Name) < strings.ToLower(c[j].Name)
	})
}

// ---------------------------------------------------------------- baca

type header struct {
	id, outlet, user                 uuid.UUID
	docNo, status, userName, note    string
	opening                          dec
	openedAt                         time.Time
	closedAt                         *time.Time
	closedBy, approvedName           string
	expected, counted, diff, diffAbs *dec
	saleCount, voidCount             *int32
	salesTotal, recvTotal            *dec
	tz                               string
	store                            Store
}

func readHeader(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, lock bool) (header, error) {
	var h header
	q := `SELECT c.id, c.outlet_id, c.user_id, c.doc_no, c.status, coalesce(u.name, ''), c.note, c.opening_cash, c.opened_at, c.closed_at,
	             coalesce(cb.name, ''), coalesce(c.approved_name, ''), c.expected_total, c.counted_total, c.diff_total, c.diff_abs,
	             c.sale_count, c.void_count, c.sales_total, c.receivable_total,
	             o.timezone, t.name, o.code, o.name, o.address, o.phone
	      FROM cash_shifts c
	      JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
	      JOIN tenants t ON t.id = c.tenant_id
	      LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.user_id
	      LEFT JOIN users cb ON cb.tenant_id = c.tenant_id AND cb.id = c.closed_by
	      WHERE c.tenant_id = $1 AND c.id = $2`
	if lock {
		q += ` FOR UPDATE OF c`
	}
	err := tx.QueryRow(ctx, q, tenant, id).Scan(&h.id, &h.outlet, &h.user, &h.docNo, &h.status, &h.userName, &h.note, &h.opening, &h.openedAt, &h.closedAt,
		&h.closedBy, &h.approvedName, &h.expected, &h.counted, &h.diff, &h.diffAbs, &h.saleCount, &h.voidCount, &h.salesTotal, &h.recvTotal,
		&h.tz, &h.store.TenantName, &h.store.OutletCode, &h.store.OutletName, &h.store.Address, &h.store.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, ErrNotFound
	}
	return h, err
}

func localTime(t time.Time, tz string) string {
	if loc, err := time.LoadLocation(tz); err == nil {
		t = t.In(loc)
	}
	return t.Format(timeLayout)
}

func strp(d *dec) *string {
	if d == nil {
		return nil
	}
	s := d.StringFixed(2)
	return &s
}

// canSee: pemilik shift, atau pemegang cash_shifts.view yang boleh mengakses outlet shift itu.
func canSee(a authz.Actor, h header) bool {
	if h.user == a.UserID && h.outlet == a.OutletID {
		return true
	}
	return a.Perms.Has(Module, authz.ActView) && (h.outlet == a.OutletID || a.Outlets[h.outlet])
}

// Get = rincian shift: rekap beku bila tertutup, rekap berjalan bila terbuka.
func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Shift, error) {
	return s.get(ctx, a, id, true)
}

func (s *Service) get(ctx context.Context, a authz.Actor, id uuid.UUID, check bool) (Shift, error) {
	var out Shift
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		h, err := readHeader(ctx, tx, a.TenantID, id, false)
		if err != nil {
			return err
		}
		if check && !canSee(a, h) {
			if h.outlet != a.OutletID && !a.Outlets[h.outlet] {
				return ErrNotFound
			}
			return ErrForbidden
		}
		out = Shift{ID: h.id, DocNo: h.docNo, Status: h.status, OutletID: h.outlet, UserID: h.user, UserName: h.userName,
			OpeningCash: h.opening.StringFixed(2), OpenedAt: h.openedAt, ClosedAt: h.closedAt, ClosedByName: h.closedBy, Note: h.note,
			ApprovedByName: h.approvedName, OpenedLocal: localTime(h.openedAt, h.tz), Store: h.store}
		if h.status == "open" {
			r, err := computeRecap(ctx, tx, a.TenantID, h.outlet, h.user, h.opening, h.openedAt, s.now().Add(time.Second))
			if err != nil {
				return err
			}
			out.Counts, out.Flows, out.ExpectedTotal = r.counts, r.flows, r.expected.StringFixed(2)
			out.SaleCount, out.VoidCount, out.SalesTotal, out.ReceivableTotal = r.saleCount, r.voidCount, r.salesTotal.StringFixed(2), r.recvTotal.StringFixed(2)
			return nil
		}
		out.ClosedLocal = localTime(*h.closedAt, h.tz)
		out.ExpectedTotal, out.CountedTotal, out.DiffTotal, out.DiffAbs = h.expected.StringFixed(2), strp(h.counted), strp(h.diff), strp(h.diffAbs)
		out.SaleCount, out.VoidCount = int(*h.saleCount), int(*h.voidCount)
		out.SalesTotal, out.ReceivableTotal = h.salesTotal.StringFixed(2), h.recvTotal.StringFixed(2)
		rows, err := tx.Query(ctx, `SELECT method_id, method_name, kind, sales, flows, opening, expected, counted, diff
			FROM cash_shift_counts WHERE tenant_id = $1 AND shift_id = $2 ORDER BY position`, a.TenantID, id)
		if err != nil {
			return err
		}
		out.Counts = []Count{}
		for rows.Next() {
			var c Count
			var sa, fl, op, ex, co, di dec
			if err := rows.Scan(&c.MethodID, &c.Name, &c.Kind, &sa, &fl, &op, &ex, &co, &di); err != nil {
				rows.Close()
				return err
			}
			c.Sales, c.Flows, c.Opening, c.Expected = sa.StringFixed(2), fl.StringFixed(2), op.StringFixed(2), ex.StringFixed(2)
			c.Counted, c.Diff = strp(&co), strp(&di)
			out.Counts = append(out.Counts, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT source, method_id, method_name, kind, amount, doc_count
			FROM cash_shift_flows WHERE tenant_id = $1 AND shift_id = $2 ORDER BY position`, a.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.Flows = []Flow{}
		for rows.Next() {
			var f Flow
			var amt dec
			if err := rows.Scan(&f.Source, &f.MethodID, &f.Name, &f.Kind, &amt, &f.Count); err != nil {
				return err
			}
			f.Amount = amt.StringFixed(2)
			out.Flows = append(out.Flows, f)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------- tutup

type CountIn struct {
	MethodID uuid.UUID   `json:"method_id"`
	Counted  json.Number `json:"counted"`
}

type ApprovalIn struct {
	UserID uuid.UUID `json:"user_id"`
	Pin    string    `json:"pin"`
}

type CloseInput struct {
	Counts   []CountIn   `json:"counts"`
	Note     string      `json:"note"`
	Approval *ApprovalIn `json:"approval"`
}

// DiffError = ada selisih tetapi persetujuan (catatan dan/atau PIN) belum dipenuhi; klien menampilkan selisihnya.
type DiffError struct{ Diff string }

func (e *DiffError) Error() string { return "selisih tutup shift butuh catatan dan persetujuan" }

// Close menutup shift `id`. Pemilik shift boleh menutup shift miliknya (di outlet shift itu); orang lain butuh
// izin cash_shifts.update dan akses ke outletnya. Idempoten lewat kunci: kirim ulang dengan kunci sama → hasil yang sama.
// Setiap metode di rekap wajib diisi (ErrRecapChanged bila rekap berubah sejak dilihat kasir); metode lain boleh
// ditambahkan (seharusnya 0). Selisih per metode ≠ 0 → catatan ≥ 3 karakter + PIN penyetuju shift_close.
func (s *Service) Close(ctx context.Context, a authz.Actor, id uuid.UUID, key string, in CloseInput) (Shift, error) {
	if !keyPattern.MatchString(key) {
		return Shift{}, ErrKeyRequired
	}
	note, code := sanitize.Multiline(in.Note, noteMax)
	if code != "" {
		return Shift{}, FieldErrors{"note": code}
	}
	if len(in.Counts) > 50 {
		return Shift{}, FieldErrors{"counts": sanitize.Invalid}
	}
	counted := map[uuid.UUID]dec{}
	fe := FieldErrors{}
	for i, c := range in.Counts {
		v, ok := parseMoney(c.Counted, false)
		if !ok || c.MethodID == uuid.Nil {
			fe[fmt.Sprintf("counts.%d.counted", i)] = sanitize.Invalid
			continue
		}
		if _, dup := counted[c.MethodID]; dup {
			fe[fmt.Sprintf("counts.%d.method_id", i)] = "DUPLICATE"
			continue
		}
		counted[c.MethodID] = v
	}
	if len(fe) > 0 {
		return Shift{}, fe
	}

	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var prior uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM cash_shifts WHERE tenant_id = $1 AND close_key = $2`, a.TenantID, key).Scan(&prior); err == nil {
			if prior != id {
				return ErrKeyMismatch
			}
			return nil // sudah ditutup dengan kunci ini
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		h, err := readHeader(ctx, tx, a.TenantID, id, true)
		if err != nil {
			return err
		}
		own := h.user == a.UserID && h.outlet == a.OutletID
		if !own && !(a.Perms.Has(Module, authz.ActUpdate) && (h.outlet == a.OutletID || a.Outlets[h.outlet])) {
			if h.outlet != a.OutletID && !a.Outlets[h.outlet] {
				return ErrNotFound
			}
			return ErrForbidden
		}
		if h.status != "open" {
			return ErrNotOpen
		}
		// Jam tutup dibaca SETELAH kunci didapat: nota yang memegang FOR SHARE (Guard) sudah commit dan created_at-nya
		// pasti lebih awal; nota sesudahnya melihat status closed dan ditolak.
		var closedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&closedAt); err != nil {
			return err
		}
		r, err := computeRecap(ctx, tx, a.TenantID, h.outlet, h.user, h.opening, h.openedAt, closedAt)
		if err != nil {
			return err
		}
		inRecap := map[uuid.UUID]bool{}
		for _, c := range r.counts {
			inRecap[c.MethodID] = true
			if _, ok := counted[c.MethodID]; !ok {
				return ErrRecapChanged
			}
		}
		// Metode tambahan (tidak ada di rekap) dengan uang fisik > 0 = selisih lebih.
		var extra []uuid.UUID
		for mid, v := range counted {
			if !inRecap[mid] && v.IsPositive() {
				extra = append(extra, mid)
			}
		}
		for _, mid := range extra {
			var name, kind string
			err := tx.QueryRow(ctx, `SELECT name, kind FROM payment_methods WHERE tenant_id = $1 AND id = $2 AND kind NOT IN ('deposit', 'supplier_credit')`,
				a.TenantID, mid).Scan(&name, &kind)
			if errors.Is(err, pgx.ErrNoRows) {
				return FieldErrors{"counts": sanitize.Invalid}
			}
			if err != nil {
				return err
			}
			r.counts = append(r.counts, Count{MethodID: mid, Name: name, Kind: kind, Sales: "0.00", Flows: "0.00", Opening: "0.00", Expected: "0.00"})
		}
		sortCounts(r.counts)
		total, diffTotal, diffAbs := decimal.Zero, decimal.Zero, decimal.Zero
		for i := range r.counts {
			c := &r.counts[i]
			exp, _ := decimal.NewFromString(c.Expected)
			v := counted[c.MethodID]
			d := v.Sub(exp)
			cs, ds := v.StringFixed(2), d.StringFixed(2)
			c.Counted, c.Diff = &cs, &ds
			total, diffTotal, diffAbs = total.Add(v), diffTotal.Add(d), diffAbs.Add(d.Abs())
		}
		var approverID *uuid.UUID
		var approverName *string
		if !diffAbs.IsZero() {
			if len([]rune(note)) < 3 {
				return &DiffError{Diff: diffTotal.StringFixed(2)}
			}
			if in.Approval == nil || s.approvals == nil {
				return approval.ErrPinRequired
			}
			ap, err := s.approvals.VerifyFor(ctx, tx, a, ModuleApprove, h.outlet, in.Approval.UserID, in.Approval.Pin)
			if err != nil {
				return err
			}
			approverID, approverName = &ap.ID, &ap.Name
		}
		if _, err := tx.Exec(ctx, `UPDATE cash_shifts SET status = 'closed', closed_at = $3, closed_by = $4, expected_total = $5, counted_total = $6,
			diff_total = $7, diff_abs = $8, sale_count = $9, void_count = $10, sales_total = $11, receivable_total = $12, note = $13,
			approved_by = $14, approved_name = $15, close_key = $16
			WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, id, closedAt, a.UserID, r.expected, total, diffTotal, diffAbs, r.saleCount, r.voidCount, r.salesTotal, r.recvTotal, note,
			approverID, approverName, key); err != nil {
			return err
		}
		for i, c := range r.counts {
			if _, err := tx.Exec(ctx, `INSERT INTO cash_shift_counts (tenant_id, shift_id, position, method_id, method_name, kind, sales, flows, opening, expected, counted, diff)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
				a.TenantID, id, i+1, c.MethodID, c.Name, c.Kind, decimal.RequireFromString(c.Sales), decimal.RequireFromString(c.Flows),
				decimal.RequireFromString(c.Opening), decimal.RequireFromString(c.Expected), decimal.RequireFromString(*c.Counted), decimal.RequireFromString(*c.Diff)); err != nil {
				return err
			}
		}
		for i, f := range r.flows {
			if _, err := tx.Exec(ctx, `INSERT INTO cash_shift_flows (tenant_id, shift_id, position, source, method_id, method_name, kind, amount, doc_count)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				a.TenantID, id, i+1, f.Source, f.MethodID, f.Name, f.Kind, decimal.RequireFromString(f.Amount), f.Count); err != nil {
				return err
			}
		}
		det := map[string]any{"doc_no": h.docNo, "expected": r.expected.StringFixed(2), "counted": total.StringFixed(2), "diff": diffTotal.StringFixed(2)}
		if approverID != nil {
			det["approved_by"], det["approver_name"], det["note"] = approverID.String(), *approverName, note
		}
		if !own {
			det["owner_id"] = h.user.String()
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionShiftClose, Entity: audit.EntityShift, EntityID: id.String(), Details: det})
	})
	if err != nil {
		return Shift{}, err
	}
	return s.get(ctx, a, id, false)
}

// ---------------------------------------------------------------- daftar

type ListParams struct {
	From, To string // YYYY-MM-DD menurut zona waktu outlet (kosong = 30 hari terakhir)
	UserID   *uuid.UUID
	Status   string // open|closed|""
	DiffOnly bool
	Cursor   string
	Limit    int
}

type ListRow struct {
	ID            uuid.UUID  `json:"id"`
	DocNo         string     `json:"doc_no"`
	Status        string     `json:"status"`
	UserID        uuid.UUID  `json:"user_id"`
	UserName      string     `json:"user_name"`
	OpeningCash   string     `json:"opening_cash"`
	OpenedAt      time.Time  `json:"opened_at"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	ExpectedTotal *string    `json:"expected_total"`
	CountedTotal  *string    `json:"counted_total"`
	DiffTotal     *string    `json:"diff_total"`
	DiffAbs       *string    `json:"diff_abs"`
	SaleCount     *int32     `json:"sale_count"`
	SalesTotal    *string    `json:"sales_total"`
	Note          string     `json:"note"`
	ApprovedBy    string     `json:"approved_by_name,omitempty"`
}

type ListResult struct {
	Data       []ListRow `json:"data"`
	NextCursor string    `json:"next_cursor,omitempty"`
	HasMore    bool      `json:"has_more"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Cashiers   []Cashier `json:"cashiers"` // kasir yang punya shift di outlet ini (untuk filter)
}

type Cashier struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// List = daftar shift outlet aktif (supervisor), terbaru dulu, keyset (opened_at, id).
func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) (ListResult, error) {
	res := ListResult{Data: []ListRow{}, Cashiers: []Cashier{}}
	if p.Limit <= 0 || p.Limit > listMax {
		p.Limit = 50
	}
	if p.Status != "" && p.Status != "open" && p.Status != "closed" {
		return res, FieldErrors{"status": sanitize.Invalid}
	}
	var curAt time.Time
	var curID uuid.UUID
	hasCur := false
	if p.Cursor != "" {
		parts := strings.SplitN(p.Cursor, "_", 2)
		if len(parts) != 2 {
			return res, FieldErrors{"cursor": sanitize.Invalid}
		}
		t, err1 := time.Parse(time.RFC3339Nano, parts[0])
		id, err2 := uuid.Parse(parts[1])
		if err1 != nil || err2 != nil {
			return res, FieldErrors{"cursor": sanitize.Invalid}
		}
		curAt, curID, hasCur = t, id, true
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var tz string
		var today time.Time
		if err := tx.QueryRow(ctx, `SELECT timezone, (now() AT TIME ZONE timezone)::date FROM outlets WHERE tenant_id = $1 AND id = $2`,
			a.TenantID, a.OutletID).Scan(&tz, &today); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOutletInactive
			}
			return err
		}
		fd, td := today.AddDate(0, 0, -30), today
		fe := FieldErrors{}
		if p.From != "" {
			v, err := time.Parse("2006-01-02", p.From)
			if err != nil {
				fe["from"] = sanitize.Invalid
			}
			fd = v
		}
		if p.To != "" {
			v, err := time.Parse("2006-01-02", p.To)
			if err != nil {
				fe["to"] = sanitize.Invalid
			}
			td = v
		}
		if len(fe) == 0 && (td.Before(fd) || td.Sub(fd) > 366*24*time.Hour) {
			fe["to"] = sanitize.Invalid
		}
		if len(fe) > 0 {
			return fe
		}
		res.From, res.To = fd.Format("2006-01-02"), td.Format("2006-01-02")
		var uid any
		if p.UserID != nil {
			uid = *p.UserID
		}
		rows, err := tx.Query(ctx, `
			WITH b AS (SELECT ($3::date)::timestamp AT TIME ZONE $4 AS t0, ($5::date + 1)::timestamp AT TIME ZONE $4 AS t1)
			SELECT c.id, c.doc_no, c.status, c.user_id, coalesce(u.name, ''), c.opening_cash, c.opened_at, c.closed_at,
			       c.expected_total, c.counted_total, c.diff_total, c.diff_abs, c.sale_count, c.sales_total, c.note, coalesce(c.approved_name, '')
			FROM cash_shifts c CROSS JOIN b
			LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.user_id
			WHERE c.tenant_id = $1 AND c.outlet_id = $2 AND c.opened_at >= b.t0 AND c.opened_at < b.t1
			  AND ($6::uuid IS NULL OR c.user_id = $6) AND ($7 = '' OR c.status = $7) AND (NOT $8 OR c.diff_abs > 0)
			  AND (NOT $9 OR (c.opened_at, c.id) < ($10, $11))
			ORDER BY c.opened_at DESC, c.id DESC
			LIMIT $12`,
			a.TenantID, a.OutletID, fd, tz, td, uid, p.Status, p.DiffOnly, hasCur, curAt, curID, p.Limit+1)
		if err != nil {
			return err
		}
		for rows.Next() {
			var r ListRow
			var op dec
			var ex, co, di, da, st *dec
			if err := rows.Scan(&r.ID, &r.DocNo, &r.Status, &r.UserID, &r.UserName, &op, &r.OpenedAt, &r.ClosedAt, &ex, &co, &di, &da,
				&r.SaleCount, &st, &r.Note, &r.ApprovedBy); err != nil {
				rows.Close()
				return err
			}
			r.OpeningCash, r.ExpectedTotal, r.CountedTotal, r.DiffTotal, r.DiffAbs, r.SalesTotal = op.StringFixed(2), strp(ex), strp(co), strp(di), strp(da), strp(st)
			res.Data = append(res.Data, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(res.Data) > p.Limit {
			res.Data = res.Data[:p.Limit]
			last := res.Data[len(res.Data)-1]
			res.HasMore, res.NextCursor = true, last.OpenedAt.UTC().Format(time.RFC3339Nano)+"_"+last.ID.String()
		}
		crows, err := tx.Query(ctx, `SELECT u.id, u.name FROM users u
			WHERE u.tenant_id = $1 AND EXISTS (SELECT 1 FROM cash_shifts c WHERE c.tenant_id = u.tenant_id AND c.outlet_id = $2 AND c.user_id = u.id)
			ORDER BY u.name LIMIT 200`, a.TenantID, a.OutletID)
		if err != nil {
			return err
		}
		defer crows.Close()
		for crows.Next() {
			var c Cashier
			if err := crows.Scan(&c.ID, &c.Name); err != nil {
				return err
			}
			res.Cashiers = append(res.Cashiers, c)
		}
		return crows.Err()
	})
	return res, err
}
