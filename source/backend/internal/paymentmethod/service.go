// Package paymentmethod: master metode pembayaran per tenant. Nama bebas, tetapi tiap metode punya jenis dasar
// (cash, debit, credit_card, ewallet, transfer) yang menentukan perilaku di kasir. Tunai bawaan (is_system) tak bisa
// diarsipkan atau diganti jenisnya; metode lain tidak dihapus, hanya diarsipkan (menempel di nota lama).
package paymentmethod

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

// ModuleID = id modul izin (menu "Metode Pembayaran"). Kasir (sales_orders.create) boleh membaca lookup-nya.
const ModuleID = "payment_methods"

const (
	BearerStore    = "store"    // biaya ditanggung toko (dipotong dari uang masuk)
	BearerCustomer = "customer" // biaya ditagihkan ke pelanggan di atas total nota

	KindCash       = "cash"
	KindDebit      = "debit"
	KindCreditCard = "credit_card"
	KindEWallet    = "ewallet"
	KindTransfer   = "transfer"
	// Jenis internal (satu metode sistem per tenant, tanpa biaya, tak bisa dibuat/diganti jenisnya lewat API):
	KindDeposit        = "deposit"         // Deposit Member: bayar nota/piutang member, tujuan dana kembali retur penjualan
	KindSupplierCredit = "supplier_credit" // Kredit Pemasok: bayar hutang pemasok, tujuan dana kembali retur pembelian

	maxName  = 60
	maxLimit = 100
	defLimit = 25
	maxTotal = 30 // batas jumlah metode per tenant (termasuk arsip) agar layar kasir tetap muat
)

var (
	ErrNotFound  = errors.New("metode pembayaran tidak ditemukan")
	ErrNameTaken = errors.New("nama metode pembayaran sudah dipakai")
	ErrLocked    = errors.New("metode bawaan tidak dapat diarsipkan")
	ErrTooMany   = errors.New("jumlah metode pembayaran sudah mencapai batas")
)

// Kinds = jenis dasar yang sah (urutan tampil).
var Kinds = []string{KindCash, KindTransfer, KindDebit, KindCreditCard, KindEWallet}

// IsInternal: jenis saldo titipan (deposit member / kredit pemasok).
func IsInternal(k string) bool { return k == KindDeposit || k == KindSupplierCredit }

// Konteks lookup: menentukan jenis internal mana yang ikut tampil. Kosong = hanya jenis dasar.
const (
	ForSale           = "sale"            // kasir (bayar nota) — + deposit
	ForReceivable     = "receivable"      // bayar piutang member — + deposit
	ForSaleReturn     = "sale_return"     // dana kembali retur penjualan — + deposit
	ForPayable        = "payable"         // bayar hutang pemasok — + kredit pemasok
	ForPurchaseReturn = "purchase_return" // dana kembali retur pembelian — + kredit pemasok
	ForWallet         = "wallet"          // top-up/tarik deposit, pencairan kredit: hanya jenis dasar
)

// LookupFor: apakah metode berjenis kind tampil untuk konteks ctx.
func LookupFor(ctx, kind string) bool {
	switch kind {
	case KindDeposit:
		return ctx == ForSale || ctx == ForReceivable || ctx == ForSaleReturn
	case KindSupplierCredit:
		return ctx == ForPayable || ctx == ForPurchaseReturn
	}
	return true
}

// IsKind: apakah k jenis dasar yang sah.
func IsKind(k string) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// FieldErrors = kode galat per field (diterjemahkan klien: errors.FIELD_<kode>).
type FieldErrors map[string]string

func (f FieldErrors) Error() string { return "input tidak valid" }

// Method = bentuk respons API.
type Method struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Kind   string    `json:"kind"`
	System bool      `json:"is_system"`
	Active bool      `json:"active"`
	// Biaya (MDR / biaya admin) ditanggung toko: jumlah dibayar × FeePct% + FeeFlat. "0.00" = tanpa biaya.
	FeePct  string `json:"fee_pct,omitempty"`
	FeeFlat string `json:"fee_flat,omitempty"`
	// FeeBearer = penanggung biaya: "store" atau "customer".
	FeeBearer string    `json:"fee_bearer,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"` // kosong di lookup
}

// Input = isi form. Saat mengubah, `kind` boleh dikosongkan; bila diisi harus sama dengan jenis yang sekarang.
type Input struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	FeePct  Num    `json:"fee_pct"`  // kosong = 0; 0–100, maks 2 desimal
	FeeFlat Num    `json:"fee_flat"` // kosong = 0; rupiah per transaksi, maks 2 desimal
	// FeeBearer: "store" (bawaan) atau "customer". Saat mengubah, kosong = tetap seperti sekarang.
	FeeBearer string `json:"fee_bearer"`
}

// Num = angka opsional dari JSON: boleh angka (0.7), teks ("0.7"), teks kosong (""), atau null — semuanya dibaca apa adanya
// (kosong = tidak diisi), agar klien yang mengirim kolom kosong tidak ditolak.
type Num string

func (n *Num) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*n = ""
		return nil
	}
	if strings.HasPrefix(s, "\"") {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		s = strings.TrimSpace(v)
	}
	*n = Num(s)
	return nil
}

func (n Num) String() string { return string(n) }

var maxFlat = decimal.New(1, 9) // < 1 miliar per transaksi

// fees memeriksa tarif biaya. Tunai tidak boleh berbiaya (tak ada potongan pada uang tunai).
// bearer memeriksa penanggung biaya; kosong -> def. Tunai selalu ditanggung toko (tak ada biaya pada tunai).
func bearer(in Input, kind, def string, f FieldErrors) string {
	b := strings.TrimSpace(in.FeeBearer)
	if b == "" {
		b = def
	}
	switch {
	case b != BearerStore && b != BearerCustomer:
		f["fee_bearer"] = sanitize.Invalid
	case (kind == KindCash || IsInternal(kind)) && b != BearerStore:
		f["fee_bearer"] = "CASH_NO_FEE"
	}
	return b
}

func fees(in Input, kind string, f FieldErrors) (pct, flat decimal.Decimal) {
	parse := func(n Num, field string, max decimal.Decimal) decimal.Decimal {
		s := strings.TrimSpace(n.String())
		if s == "" {
			return decimal.Zero
		}
		d, err := decimal.NewFromString(s)
		if err != nil || d.IsNegative() || !d.Equal(d.Round(2)) || d.GreaterThan(max) {
			f[field] = sanitize.Invalid
			return decimal.Zero
		}
		return d
	}
	pct = parse(in.FeePct, "fee_pct", decimal.NewFromInt(100))
	flat = parse(in.FeeFlat, "fee_flat", maxFlat)
	if (kind == KindCash || IsInternal(kind)) && (pct.IsPositive() || flat.IsPositive()) {
		f["fee_pct"] = "CASH_NO_FEE"
	}
	return pct, flat
}

var defaults = []struct{ name, kind string }{
	{"Tunai", KindCash}, {"Transfer", KindTransfer}, {"Debit", KindDebit}, {"Kartu Kredit", KindCreditCard}, {"E-Wallet", KindEWallet},
	{"Deposit Member", KindDeposit}, {"Kredit Pemasok", KindSupplierCredit},
}

// SeedDefaults mengisi metode bawaan; dipanggil Register di dalam transaksi pembuatan tenant.
func SeedDefaults(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	q := gen.New(tx)
	for _, d := range defaults {
		if _, err := q.PaymentMethodCreate(ctx, gen.PaymentMethodCreateParams{TenantID: tenant, Name: d.name, Kind: d.kind, IsSystem: d.kind == KindCash || IsInternal(d.kind),
			FeePct: decimal.Zero, FeeFlat: decimal.Zero, FeeBearer: BearerStore}); err != nil {
			return err
		}
	}
	return nil
}

func view(id uuid.UUID, name, kind string, system, active bool, pct, flat decimal.Decimal, bearer string, created pgtype.Timestamptz) Method {
	return Method{ID: id, Name: name, Kind: kind, System: system, Active: active, FeePct: pct.StringFixed(2), FeeFlat: flat.StringFixed(2), FeeBearer: bearer, CreatedAt: created.Time}
}

func conflict(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "payment_methods_tenant_name_key":
		return ErrNameTaken
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	}
	return err
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type ListParams struct {
	Q      string
	Active *bool
	Limit  int
	Offset int
}

func likeEscape(s string) string {
	return strings.NewReplacer("\\", "\\\\", `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Service) List(ctx context.Context, a authz.Actor, p ListParams) ([]Method, int, error) {
	q, ok := sanitize.Text(p.Q)
	if !ok || len([]rune(q)) > maxName {
		return nil, 0, FieldErrors{"q": sanitize.Invalid}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defLimit
	}
	arg := gen.PaymentMethodListParams{TenantID: a.TenantID, Q: likeEscape(q), PageLimit: int32(min(limit, maxLimit)), PageOffset: int32(max(p.Offset, 0))}
	if p.Active != nil {
		arg.Active = pgtype.Bool{Bool: *p.Active, Valid: true}
	}
	out := []Method{}
	total := 0
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).PaymentMethodList(ctx, arg)
		for _, r := range rows {
			out = append(out, view(r.ID, r.Name, r.Kind, r.IsSystem, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer, r.CreatedAt))
			total = int(r.Total)
		}
		return err
	})
	return out, total, err
}

// Lookup = metode aktif (urut tampil di kasir: Tunai dulu, lalu menurut nama) untuk konteks forCtx (lihat LookupFor).
func (s *Service) Lookup(ctx context.Context, a authz.Actor, forCtx string) ([]Method, error) {
	out := []Method{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).PaymentMethodActiveList(ctx, a.TenantID)
		for _, r := range rows {
			if !LookupFor(forCtx, r.Kind) {
				continue
			}
			out = append(out, Method{ID: r.ID, Name: r.Name, Kind: r.Kind, System: r.IsSystem, Active: true,
				FeePct: r.FeePct.StringFixed(2), FeeFlat: r.FeeFlat.StringFixed(2), FeeBearer: r.FeeBearer})
		}
		return err
	})
	return out, err
}

func details(m Method) map[string]any {
	return map[string]any{"name": m.Name, "kind": m.Kind, "fee_pct": m.FeePct, "fee_flat": m.FeeFlat, "fee_bearer": m.FeeBearer}
}

func (m Method) detail() map[string]any { return details(m) }

func (s *Service) Create(ctx context.Context, a authz.Actor, in Input) (*Method, error) {
	name, code := sanitize.Name(in.Name, maxName)
	f := FieldErrors{}
	if code != "" {
		f["name"] = code
	}
	switch {
	case in.Kind == "":
		f["kind"] = sanitize.Required
	case !IsKind(in.Kind) || in.Kind == KindCash: // Tunai sudah ada sebagai metode bawaan; tidak ada "tunai kedua"
		f["kind"] = sanitize.Invalid
	}
	feePct, feeFlat := fees(in, in.Kind, f)
	feeBearer := bearer(in, in.Kind, BearerStore, f)
	if len(f) > 0 {
		return nil, f
	}
	var out Method
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		n, err := q.PaymentMethodCount(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if n >= maxTotal {
			return ErrTooMany
		}
		r, err := q.PaymentMethodCreate(ctx, gen.PaymentMethodCreateParams{TenantID: a.TenantID, Name: name, Kind: in.Kind, FeePct: feePct, FeeFlat: feeFlat, FeeBearer: feeBearer})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Name, r.Kind, r.IsSystem, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPaymentMethodCreate, Entity: audit.EntityPaymentMethod, EntityID: r.ID.String(), Details: details(out),
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update mengganti nama, jenis (hanya antar jenis non-tunai), dan biaya. Nota lama menyimpan snapshot jenis & nama, jadi perubahan
// hanya berlaku untuk nota berikutnya; identitas tetap `id` (dasar pemetaan akun akuntansi nanti).
func (s *Service) Update(ctx context.Context, a authz.Actor, id uuid.UUID, in Input) (*Method, error) {
	name, code := sanitize.Name(in.Name, maxName)
	var out Method
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.PaymentMethodGetForUpdate(ctx, gen.PaymentMethodGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		f := FieldErrors{}
		if code != "" {
			f["name"] = code
		}
		// Jenis boleh diganti di antara jenis non-tunai. Tunai terkunci (satu-satunya yang memberi kembalian).
		kind := cur.Kind
		if in.Kind != "" && in.Kind != cur.Kind {
			switch {
			case !IsKind(in.Kind):
				f["kind"] = sanitize.Invalid
			case in.Kind == KindCash || cur.Kind == KindCash || IsInternal(in.Kind) || IsInternal(cur.Kind):
				f["kind"] = "KIND_LOCKED"
			default:
				kind = in.Kind
			}
		}
		feePct, feeFlat := fees(in, kind, f)
		feeBearer := bearer(in, kind, cur.FeeBearer, f)
		if len(f) > 0 {
			return f
		}
		if cur.Name == name && cur.Kind == kind && cur.FeePct.Equal(feePct) && cur.FeeFlat.Equal(feeFlat) && cur.FeeBearer == feeBearer {
			out = view(cur.ID, cur.Name, cur.Kind, cur.IsSystem, cur.Active, cur.FeePct, cur.FeeFlat, cur.FeeBearer, cur.CreatedAt)
			return nil
		}
		r, err := q.PaymentMethodUpdate(ctx, gen.PaymentMethodUpdateParams{TenantID: a.TenantID, ID: id, Name: name, FeePct: feePct, FeeFlat: feeFlat, FeeBearer: feeBearer, Kind: kind})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Name, r.Kind, r.IsSystem, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPaymentMethodUpdate, Entity: audit.EntityPaymentMethod, EntityID: id.String(),
			Details: map[string]any{"before": view(cur.ID, cur.Name, cur.Kind, cur.IsSystem, cur.Active, cur.FeePct, cur.FeeFlat, cur.FeeBearer, cur.CreatedAt).detail(), "after": out.detail()},
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) SetActive(ctx context.Context, a authz.Actor, id uuid.UUID, active bool) (*Method, error) {
	var out Method
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.PaymentMethodGetForUpdate(ctx, gen.PaymentMethodGetForUpdateParams{TenantID: a.TenantID, ID: id})
		if err != nil {
			return err
		}
		if cur.IsSystem && !active {
			return ErrLocked
		}
		if cur.Active == active {
			out = view(cur.ID, cur.Name, cur.Kind, cur.IsSystem, cur.Active, cur.FeePct, cur.FeeFlat, cur.FeeBearer, cur.CreatedAt)
			return nil
		}
		r, err := q.PaymentMethodSetActive(ctx, gen.PaymentMethodSetActiveParams{TenantID: a.TenantID, ID: id, Active: active})
		if err != nil {
			return err
		}
		out = view(r.ID, r.Name, r.Kind, r.IsSystem, r.Active, r.FeePct, r.FeeFlat, r.FeeBearer, r.CreatedAt)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{
			Action: audit.ActionPaymentMethodActive, Entity: audit.EntityPaymentMethod, EntityID: id.String(),
			Details: map[string]any{"name": out.Name, "active": active},
		})
	})
	if err = conflict(err); err != nil {
		return nil, err
	}
	return &out, nil
}
