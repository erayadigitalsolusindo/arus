package accounting

import (
	"context"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const (
	KindGroup  = "group"
	KindLedger = "ledger"
)

var (
	codeRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)
	classes = map[string]bool{"asset": true, "liability": true, "equity": true, "revenue": true, "cogs": true, "expense": true}
)

type Account struct {
	ID         uuid.UUID  `json:"id"`
	ParentID   *uuid.UUID `json:"parent_id"`
	Code       string     `json:"code"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Class      string     `json:"class"`
	NormalSide string     `json:"normal_side"`
	IsCashBank bool       `json:"is_cash_bank"`
	Active     bool       `json:"active"`
	IsSystem   bool       `json:"is_system"`
}

type accountRow interface {
	gen.AccountListRow | gen.AccountGetRow | gen.AccountGetByCodeRow | gen.AccountCreateRow | gen.AccountUpdateRow
}

func accountOf[R accountRow](row R) Account {
	r := gen.AccountListRow(row)
	return Account{ID: r.ID, ParentID: uuidPtr(r.ParentID), Code: r.Code, Name: r.Name, Kind: r.Kind, Class: r.Class,
		NormalSide: r.NormalSide, IsCashBank: r.IsCashBank, Active: r.Active, IsSystem: r.IsSystem}
}

// normalSide = saldo normal yang diturunkan dari kelas akun.
func normalSide(class string) string {
	switch class {
	case "asset", "cogs", "expense":
		return "debit"
	}
	return "credit"
}

type AccountInput struct {
	ParentID   *uuid.UUID `json:"parent_id"`
	Code       string     `json:"code"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"` // hanya saat membuat; tidak bisa diubah
	Class      string     `json:"class"`
	IsCashBank bool       `json:"is_cash_bank"`
	Active     *bool      `json:"active"` // hanya saat mengubah
}

func (in AccountInput) validate() (code, name string, fe FieldErrors) {
	fe = FieldErrors{}
	if !codeRe.MatchString(in.Code) {
		fe["code"] = sanitize.Invalid
	}
	var e string
	if name, e = sanitize.Name(in.Name, 120); e != "" {
		fe["name"] = e
	}
	if !classes[in.Class] {
		fe["class"] = sanitize.Invalid
	}
	return in.Code, name, fe
}

// sameParent: induk akun tidak berubah (akun sistem tidak boleh dipindah).
func sameParent(cur pgtype.UUID, in *uuid.UUID) bool {
	if in == nil {
		return !cur.Valid
	}
	return cur.Valid && uuid.UUID(cur.Bytes) == *in
}

// checkParent: induk harus akun grup dengan kelas yang sama, bukan akun itu sendiri atau turunannya.
func checkParent(all []gen.AccountListRow, parent *uuid.UUID, self uuid.UUID, class string) error {
	if parent == nil {
		return nil
	}
	byID := make(map[uuid.UUID]gen.AccountListRow, len(all))
	for _, r := range all {
		byID[r.ID] = r
	}
	p, ok := byID[*parent]
	if !ok || p.Kind != KindGroup || p.Class != class {
		return ErrParentInvalid
	}
	for cur, hops := p, 0; hops < 64; hops++ { // tolak siklus: induk tidak boleh turunan akun ini
		if cur.ID == self {
			return ErrParentInvalid
		}
		if !cur.ParentID.Valid {
			return nil
		}
		cur = byID[uuid.UUID(cur.ParentID.Bytes)]
	}
	return ErrParentInvalid
}

func (s *Service) ListAccounts(ctx context.Context, a authz.Actor) ([]Account, error) {
	out := []Account{}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		rows, err := gen.New(tx).AccountList(ctx, a.TenantID)
		for _, r := range rows {
			out = append(out, accountOf(r))
		}
		return err
	})
	return out, err
}

func (s *Service) CreateAccount(ctx context.Context, a authz.Actor, in AccountInput) (Account, error) {
	code, name, fe := in.validate()
	if in.Kind != KindGroup && in.Kind != KindLedger {
		fe["kind"] = sanitize.Invalid
	}
	if in.IsCashBank && (in.Kind != KindLedger || in.Class != "asset") {
		fe["is_cash_bank"] = sanitize.Invalid
	}
	if len(fe) > 0 {
		return Account{}, fe
	}
	var out Account
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		all, err := q.AccountList(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if err := checkParent(all, in.ParentID, uuid.Nil, in.Class); err != nil {
			return err
		}
		var parent pgtype.UUID
		if in.ParentID != nil {
			parent = pgUUID(*in.ParentID)
		}
		row, err := q.AccountCreate(ctx, gen.AccountCreateParams{TenantID: a.TenantID, ParentID: parent, Code: code, Name: name,
			Kind: in.Kind, Class: in.Class, NormalSide: normalSide(in.Class), IsCashBank: in.IsCashBank, IsSystem: false})
		if pgCode(err) == "23505" {
			return ErrCodeTaken
		}
		if err != nil {
			return err
		}
		out = accountOf(row)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAccountCreate, Entity: audit.EntityAccount,
			EntityID: out.ID.String(), Details: map[string]any{"code": out.Code, "name": out.Name, "kind": out.Kind, "class": out.Class}})
	})
	return out, err
}

func (s *Service) UpdateAccount(ctx context.Context, a authz.Actor, id uuid.UUID, in AccountInput) (Account, error) {
	code, name, fe := in.validate()
	if len(fe) > 0 {
		return Account{}, fe
	}
	var out Account
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.AccountGet(ctx, gen.AccountGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.IsSystem && (code != cur.Code || in.Class != cur.Class || in.IsCashBank != cur.IsCashBank ||
			(in.Active != nil && !*in.Active) || !sameParent(cur.ParentID, in.ParentID)) {
			return ErrAccountSystem
		}
		if in.IsCashBank && (cur.Kind != KindLedger || in.Class != "asset") {
			return FieldErrors{"is_cash_bank": sanitize.Invalid}
		}
		if in.Class != cur.Class { // kelas menentukan saldo normal: kunci bila sudah dipakai
			hasKids, err := q.AccountHasChildren(ctx, gen.AccountHasChildrenParams{TenantID: a.TenantID, ParentID: pgUUID(id)})
			if err != nil {
				return err
			}
			hasLines, err := q.AccountHasLines(ctx, gen.AccountHasLinesParams{TenantID: a.TenantID, AccountID: id})
			if err != nil {
				return err
			}
			if hasKids || hasLines {
				return ErrAccountInUse
			}
		}
		all, err := q.AccountList(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if err := checkParent(all, in.ParentID, id, in.Class); err != nil {
			return err
		}
		active := cur.Active
		if in.Active != nil {
			active = *in.Active
		}
		var parent pgtype.UUID
		if in.ParentID != nil {
			parent = pgUUID(*in.ParentID)
		}
		row, err := q.AccountUpdate(ctx, gen.AccountUpdateParams{TenantID: a.TenantID, ID: id, ParentID: parent, Code: code, Name: name,
			Class: in.Class, NormalSide: normalSide(in.Class), IsCashBank: in.IsCashBank, Active: active})
		if pgCode(err) == "23505" {
			return ErrCodeTaken
		}
		if err != nil {
			return err
		}
		out = accountOf(row)
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAccountUpdate, Entity: audit.EntityAccount,
			EntityID: id.String(), Details: map[string]any{"code": out.Code, "name": out.Name, "active": out.Active,
				"before_code": cur.Code, "before_name": cur.Name, "before_active": cur.Active}})
	})
	return out, err
}

// DeleteAccount: akun yang punya anak atau baris jurnal tidak bisa dihapus (FK RESTRICT); nonaktifkan saja.
func (s *Service) DeleteAccount(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := q.AccountGet(ctx, gen.AccountGetParams{TenantID: a.TenantID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if cur.IsSystem {
			return ErrAccountSystem
		}
		n, err := q.AccountDelete(ctx, gen.AccountDeleteParams{TenantID: a.TenantID, ID: id})
		if c := pgCode(err); c == "23503" || c == "23001" {
			return ErrAccountInUse
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAccountDelete, Entity: audit.EntityAccount,
			EntityID: id.String(), Details: map[string]any{"code": cur.Code, "name": cur.Name}})
	})
}
