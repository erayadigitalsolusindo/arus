package paymentmethod

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gen "aciraba/internal/gen"
	"aciraba/internal/platform/sanitize"
)

// Pemetaan metode -> akun COA (FR-ACC-02). Metode baru mendapat akun sendiri di bawah Aset Lancar (1100) bila tenant sudah
// punya bagan akun; bila belum, account_id kosong dan terisi saat template retail dipasang (PaymentMethodLinkDefaultAccounts,
// yang kodenya harus sama dengan defaultAccountCode di bawah dan migration 00061).
const (
	currentAssetsCode = "1100"
	ownAccountFrom    = 1150 // kode akun otomatis: 1150..1199 (1140 = Piutang Kartu / EDC, 1210+ = piutang)
	ownAccountTo      = 1199
)

var defaultAccountCode = map[string]string{
	KindCash: "1110", KindTransfer: "1120", KindDebit: "1120", KindEWallet: "1130", KindCreditCard: "1140",
	KindDeposit: "2140", KindSupplierCredit: "1320",
}

// AccountRef = akun COA yang terhubung ke metode.
type AccountRef struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

func accountRef(id pgtype.UUID, code, name pgtype.Text) *AccountRef {
	if !id.Valid {
		return nil
	}
	return &AccountRef{ID: uuid.UUID(id.Bytes), Code: code.String, Name: name.String}
}

// fillAccount mengisi Method.Account dari DB (dipanggil di transaksi yang sama setelah create/update/ubah status).
func fillAccount(ctx context.Context, q *gen.Queries, tenant uuid.UUID, m *Method) error {
	r, err := q.PaymentMethodAccountGet(ctx, gen.PaymentMethodAccountGetParams{TenantID: tenant, ID: m.ID})
	if err != nil {
		return err
	}
	m.Account = accountRef(r.AccountID, r.AccountCode, r.AccountName)
	return nil
}

// checkAccount: akun pilihan pengguna harus akun buku aktif; metode biasa -> kelas aset, metode saldo titipan -> aset/kewajiban.
func checkAccount(ctx context.Context, q *gen.Queries, tenant, id uuid.UUID, kind string) error {
	a, err := q.AccountGet(ctx, gen.AccountGetParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return FieldErrors{"account_id": sanitize.Invalid}
	}
	if err != nil {
		return err
	}
	okClass := a.Class == "asset" || (IsInternal(kind) && a.Class == "liability")
	if a.Kind != "ledger" || !a.Active || !okClass {
		return FieldErrors{"account_id": sanitize.Invalid}
	}
	return nil
}

// newOwnAccount membuat akun buku baru di bawah Aset Lancar untuk metode baru. Mengembalikan pgtype.UUID{} (tak valid) bila
// tenant belum punya bagan akun / induk 1100 atau kode otomatis habis; pemanggil lalu memakai akun bawaan menurut jenis.
func newOwnAccount(ctx context.Context, q *gen.Queries, tenant uuid.UUID, name, kind string) (pgtype.UUID, error) {
	parent, err := q.AccountGetByCode(ctx, gen.AccountGetByCodeParams{TenantID: tenant, Code: currentAssetsCode})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (parent.Kind != "group" || parent.Class != "asset")) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	all, err := q.AccountList(ctx, tenant)
	if err != nil {
		return pgtype.UUID{}, err
	}
	used := make(map[string]bool, len(all))
	for _, r := range all {
		used[r.Code] = true
	}
	for n := ownAccountFrom; n <= ownAccountTo; n++ {
		code := strconv.Itoa(n)
		if used[code] {
			continue
		}
		row, err := q.AccountCreate(ctx, gen.AccountCreateParams{TenantID: tenant, ParentID: pgtype.UUID{Bytes: parent.ID, Valid: true}, Code: code, Name: name,
			Kind: "ledger", Class: "asset", NormalSide: "debit", IsCashBank: kind != KindCreditCard, IsSystem: false})
		if err != nil {
			return pgtype.UUID{}, err
		}
		return pgtype.UUID{Bytes: row.ID, Valid: true}, nil
	}
	return pgtype.UUID{}, nil
}

// defaultAccount = akun bawaan menurut jenis (tak valid bila tenant belum punya bagan akun).
func defaultAccount(ctx context.Context, q *gen.Queries, tenant uuid.UUID, kind string) (pgtype.UUID, error) {
	a, err := q.AccountGetByCode(ctx, gen.AccountGetByCodeParams{TenantID: tenant, Code: defaultAccountCode[kind]})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: a.ID, Valid: true}, nil
}

// syncOwnedAccount menjaga akun otomatis tetap searah dengan metodenya: hanya bila akun itu bukan akun sistem, tidak dipakai
// metode lain, dan namanya masih sama dengan nama metode (artinya belum diubah manual di COA).
func syncOwnedAccount(ctx context.Context, q *gen.Queries, tenant, methodID uuid.UUID, oldName, newName string, active *bool) error {
	r, err := q.PaymentMethodAccountGet(ctx, gen.PaymentMethodAccountGetParams{TenantID: tenant, ID: methodID})
	if err != nil {
		return err
	}
	if !r.AccountID.Valid || r.AccountName.String != oldName {
		return nil
	}
	acc := uuid.UUID(r.AccountID.Bytes)
	others, err := q.PaymentMethodAccountOtherUsers(ctx, gen.PaymentMethodAccountOtherUsersParams{TenantID: tenant, AccountID: pgtype.UUID{Bytes: acc, Valid: true}, ID: methodID})
	if err != nil || others > 0 {
		return err
	}
	if newName != oldName {
		if err := q.PaymentMethodRenameAccount(ctx, gen.PaymentMethodRenameAccountParams{TenantID: tenant, ID: acc, Name: newName}); err != nil {
			return err
		}
	}
	if active != nil {
		return q.PaymentMethodSetAccountActive(ctx, gen.PaymentMethodSetAccountActiveParams{TenantID: tenant, ID: acc, Active: *active})
	}
	return nil
}

// LinkDefaultAccounts menghubungkan metode yang belum punya akun ke akun bawaan menurut jenis. Dipanggil modul akuntansi
// setelah template retail dipasang, di transaksi yang sama.
func LinkDefaultAccounts(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) error {
	return gen.New(tx).PaymentMethodLinkDefaultAccounts(ctx, tenant)
}
