package accounting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
)

const maxLines = 200

// Jenis jurnal manual (OPENING lewat PostOpening; SALES/PURCHASE/AR/AP dibuat jurnal otomatis fase C).
var manualTypes = map[string]bool{"JU": true, "KM": true, "KK": true, "TK": true}

type LineInput struct {
	AccountID uuid.UUID   `json:"account_id"`
	Debit     json.Number `json:"debit"`
	Credit    json.Number `json:"credit"`
	Memo      string      `json:"memo"`
}

type JournalInput struct {
	OutletID  uuid.UUID   `json:"outlet_id"` // kosong = outlet aktif
	Date      string      `json:"date"`      // YYYY-MM-DD
	Type      string      `json:"type"`
	Narration string      `json:"narration"`
	Lines     []LineInput `json:"lines"`
}

type Line struct {
	LineNo      int       `json:"line_no"`
	AccountID   uuid.UUID `json:"account_id"`
	AccountCode string    `json:"account_code"`
	AccountName string    `json:"account_name"`
	Debit       string    `json:"debit"`
	Credit      string    `json:"credit"`
	Memo        string    `json:"memo"`
}

type Journal struct {
	ID          uuid.UUID  `json:"id"`
	OutletID    uuid.UUID  `json:"outlet_id"`
	DocNo       string     `json:"doc_no,omitempty"`
	Date        string     `json:"date"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Narration   string     `json:"narration"`
	ReversesID  *uuid.UUID `json:"reverses_id,omitempty"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	PostedAt    *time.Time `json:"posted_at,omitempty"`
	TotalDebit  string     `json:"total_debit"`
	TotalCredit string     `json:"total_credit"`
	Lines       []Line     `json:"lines"`
}

type normLine struct {
	account       uuid.UUID
	debit, credit dec
	memo          string
}

// normalize memvalidasi isi jurnal (bentuk, bukan keseimbangan: draf boleh belum seimbang).
func normalize(a authz.Actor, in JournalInput, allowed func(string) bool) (outlet uuid.UUID, date time.Time, narr string, lines []normLine, err error) {
	fe := FieldErrors{}
	outlet = in.OutletID
	if outlet == uuid.Nil {
		outlet = a.OutletID
	}
	if !outletAllowed(a, outlet) {
		return outlet, date, "", nil, ErrOutletForbidden
	}
	if !allowed(in.Type) {
		fe["type"] = sanitize.Invalid
	}
	var ok bool
	if date, ok = parseDate(in.Date); !ok {
		fe["date"] = sanitize.Invalid
	}
	var e string
	if narr, e = cleanText(in.Narration, 500, false); e != "" {
		fe["narration"] = e
	}
	if len(in.Lines) == 0 || len(in.Lines) > maxLines {
		fe["lines"] = sanitize.Invalid
	}
	for i, l := range in.Lines {
		key := fmt.Sprintf("lines.%d", i)
		d, okD := parseMoney(l.Debit)
		c, okC := parseMoney(l.Credit)
		memo, em := cleanText(l.Memo, 200, false)
		switch {
		case l.AccountID == uuid.Nil:
			fe[key+".account_id"] = sanitize.Required
		case !okD || !okC || d.IsPositive() == c.IsPositive():
			fe[key] = sanitize.Invalid // tepat satu sisi bernilai > 0
		case em != "":
			fe[key+".memo"] = em
		}
		lines = append(lines, normLine{account: l.AccountID, debit: d, credit: c, memo: memo})
	}
	if len(fe) > 0 {
		return outlet, date, narr, nil, fe
	}
	return outlet, date, narr, lines, nil
}

// checkAccounts: semua akun harus ada dan berupa akun buku (ledger); aktif bila requireActive. Mengembalikan is_cash_bank per akun.
func checkAccounts(ctx context.Context, q *gen.Queries, tenant uuid.UUID, ids []uuid.UUID, requireActive bool) (map[uuid.UUID]bool, error) {
	uniq := slices.Clone(ids)
	slices.SortFunc(uniq, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	uniq = slices.Compact(uniq)
	rows, err := q.AccountsByIDs(ctx, gen.AccountsByIDsParams{TenantID: tenant, Column2: uniq})
	if err != nil {
		return nil, err
	}
	cb := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		if r.Kind != KindLedger || (requireActive && !r.Active) {
			return nil, ErrAccountInvalid
		}
		cb[r.ID] = r.IsCashBank
	}
	if len(cb) != len(uniq) {
		return nil, ErrAccountInvalid
	}
	return cb, nil
}

func insertLines(ctx context.Context, q *gen.Queries, tenant, entry uuid.UUID, date time.Time, lines []normLine) error {
	for i, l := range lines {
		if err := q.JournalLineInsert(ctx, gen.JournalLineInsertParams{TenantID: tenant, EntryID: entry, EntryDate: pgDate(date),
			LineNo: int32(i + 1), AccountID: l.account, Debit: l.debit, Credit: l.credit, Memo: l.memo}); err != nil {
			return err
		}
	}
	return nil
}

func accountIDs(lines []normLine) []uuid.UUID {
	ids := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		ids[i] = l.account
	}
	return ids
}

type journalRow interface {
	gen.JournalGetRow | gen.JournalLockGetRow
}

func loadJournal[R journalRow](ctx context.Context, q *gen.Queries, tenant uuid.UUID, row R) (Journal, error) {
	r := gen.JournalGetRow(row)
	j := Journal{ID: r.ID, OutletID: r.OutletID, DocNo: r.DocNo.String, Date: r.EntryDate.Time.Format(dateLayout), Type: r.Type, Status: r.Status,
		Narration: r.Narration, ReversesID: uuidPtr(r.ReversesID), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.Time, Lines: []Line{}}
	if r.PostedAt.Valid {
		t := r.PostedAt.Time
		j.PostedAt = &t
	}
	rows, err := q.JournalLines(ctx, gen.JournalLinesParams{TenantID: tenant, EntryID: r.ID})
	if err != nil {
		return j, err
	}
	var td, tc dec
	for _, l := range rows {
		j.Lines = append(j.Lines, Line{LineNo: int(l.LineNo), AccountID: l.AccountID, AccountCode: l.AccountCode, AccountName: l.AccountName,
			Debit: l.Debit.StringFixed(2), Credit: l.Credit.StringFixed(2), Memo: l.Memo})
		td, tc = td.Add(l.Debit), tc.Add(l.Credit)
	}
	j.TotalDebit, j.TotalCredit = td.StringFixed(2), tc.StringFixed(2)
	return j, nil
}

func getJournal(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID) (Journal, error) {
	q := gen.New(tx)
	row, err := q.JournalGet(ctx, gen.JournalGetParams{TenantID: a.TenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Journal{}, ErrNotFound
	}
	if err != nil {
		return Journal{}, err
	}
	if !outletAllowed(a, row.OutletID) {
		return Journal{}, ErrOutletForbidden
	}
	return loadJournal(ctx, q, a.TenantID, row)
}

func (s *Service) Get(ctx context.Context, a authz.Actor, id uuid.UUID) (Journal, error) {
	var j Journal
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
		j, err = getJournal(ctx, tx, a, id)
		return err
	})
	return j, err
}

// SaveDraft membuat draf (id = uuid.Nil) atau mengubah draf yang ada. Draf boleh belum seimbang; jenis tidak bisa diubah.
func (s *Service) SaveDraft(ctx context.Context, a authz.Actor, id uuid.UUID, in JournalInput) (Journal, error) {
	outlet, date, narr, lines, err := normalize(a, in, func(t string) bool { return manualTypes[t] })
	if err != nil {
		return Journal{}, err
	}
	var out Journal
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := activeOutlet(ctx, tx, a.TenantID, outlet); err != nil {
			return err
		}
		if _, err := checkAccounts(ctx, q, a.TenantID, accountIDs(lines), true); err != nil {
			return err
		}
		if id == uuid.Nil {
			id, err = q.JournalInsert(ctx, gen.JournalInsertParams{TenantID: a.TenantID, OutletID: outlet, EntryDate: pgDate(date), Type: in.Type,
				Narration: narr, CreatedBy: a.UserID})
			if err != nil {
				return err
			}
		} else {
			cur, err := lockEntry(ctx, q, a.TenantID, id)
			if err != nil {
				return err
			}
			if !outletAllowed(a, cur.OutletID) {
				return ErrOutletForbidden
			}
			if cur.Status != "draft" {
				return ErrJournalPosted
			}
			if cur.Type != in.Type {
				return FieldErrors{"type": sanitize.Invalid}
			}
			if _, err := q.JournalUpdateDraft(ctx, gen.JournalUpdateDraftParams{TenantID: a.TenantID, ID: id, OutletID: outlet,
				EntryDate: pgDate(date), Narration: narr}); err != nil {
				return err
			}
			if err := q.JournalLinesDelete(ctx, gen.JournalLinesDeleteParams{TenantID: a.TenantID, EntryID: id}); err != nil {
				return err
			}
		}
		if err := insertLines(ctx, q, a.TenantID, id, date, lines); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionJournalSave, Entity: audit.EntityJournal,
			EntityID: id.String(), Details: map[string]any{"type": in.Type, "date": in.Date, "lines": len(lines)}}); err != nil {
			return err
		}
		out, err = getJournal(ctx, tx, a, id)
		return err
	})
	return out, err
}

// DeleteDraft menghapus draf. Jurnal terposting tidak bisa dihapus (koreksi lewat Reverse).
func (s *Service) DeleteDraft(ctx context.Context, a authz.Actor, id uuid.UUID) error {
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		cur, err := lockEntry(ctx, q, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, cur.OutletID) {
			return ErrOutletForbidden
		}
		if cur.Status != "draft" {
			return ErrJournalPosted
		}
		if _, err := q.JournalDeleteDraft(ctx, gen.JournalDeleteDraftParams{TenantID: a.TenantID, ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionJournalDelete, Entity: audit.EntityJournal,
			EntityID: id.String(), Details: map[string]any{"type": cur.Type, "date": cur.EntryDate.Time.Format(dateLayout)}})
	})
}

// Post memposting draf. Ditolak: tidak seimbang (JOURNAL_UNBALANCED), < 2 baris/susunan tak sesuai jenis, akun tidak sah,
// tanggal di periode tertutup (PERIOD_CLOSED).
func (s *Service) Post(ctx context.Context, a authz.Actor, id uuid.UUID) (Journal, error) {
	var out Journal
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		if err := postTx(ctx, tx, a, id, "journal.post"); err != nil {
			return err
		}
		var err error
		out, err = getJournal(ctx, tx, a, id)
		return err
	})
	return out, err
}

// postTx = inti posting, dipakai Post, Reverse, dan PostOpening di dalam transaksi pemanggil.
func postTx(ctx context.Context, tx pgx.Tx, a authz.Actor, id uuid.UUID, action string) error {
	q := gen.New(tx)
	e, err := lockEntry(ctx, q, a.TenantID, id)
	if err != nil {
		return err
	}
	if !outletAllowed(a, e.OutletID) {
		return ErrOutletForbidden
	}
	if e.Status != "draft" {
		return ErrJournalPosted
	}
	lines, err := q.JournalLines(ctx, gen.JournalLinesParams{TenantID: a.TenantID, EntryID: id})
	if err != nil {
		return err
	}
	var td, tc dec
	for _, l := range lines {
		td, tc = td.Add(l.Debit), tc.Add(l.Credit)
	}
	if !td.Equal(tc) {
		return ErrJournalUnbalanced
	}
	if len(lines) < 2 {
		return ErrJournalShape
	}
	reversal := e.ReversesID.Valid // jurnal balik meniru jurnal asal: tidak diuji ulang susunan/keaktifan akun
	ids := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		ids[i] = l.AccountID
	}
	cb, err := checkAccounts(ctx, q, a.TenantID, ids, !reversal)
	if err != nil {
		return err
	}
	if !reversal {
		if err := checkShape(e.Type, lines, cb); err != nil {
			return err
		}
	}
	month := monthStart(e.EntryDate.Time)
	if err := lockPeriodShare(ctx, q, a.TenantID, month); err != nil {
		return err
	}
	no, err := q.JournalNextNo(ctx, gen.JournalNextNoParams{TenantID: a.TenantID, Type: e.Type, Year: int32(e.EntryDate.Time.Year())})
	if err != nil {
		return err
	}
	docNo := fmt.Sprintf("%s/%d/%06d", e.Type, e.EntryDate.Time.Year(), no)
	n, err := q.JournalMarkPosted(ctx, gen.JournalMarkPostedParams{TenantID: a.TenantID, ID: id, DocNo: pgtype.Text{String: docNo, Valid: true}, PostedBy: pgUUID(a.UserID)})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrJournalPosted
	}
	// Saldo per akun/bulan: satu upsert per akun, terurut agar posting bersamaan tidak saling deadlock.
	sums := map[uuid.UUID][2]dec{}
	for _, l := range lines {
		s := sums[l.AccountID]
		sums[l.AccountID] = [2]dec{s[0].Add(l.Debit), s[1].Add(l.Credit)}
	}
	keys := make([]uuid.UUID, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	for _, k := range keys {
		if err := q.BalanceAdd(ctx, gen.BalanceAddParams{TenantID: a.TenantID, OutletID: e.OutletID, AccountID: k, PeriodMonth: pgDate(month),
			Debit: sums[k][0], Credit: sums[k][1]}); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: action, Entity: audit.EntityJournal, EntityID: id.String(),
		Details: map[string]any{"doc_no": docNo, "type": e.Type, "date": e.EntryDate.Time.Format(dateLayout), "total": td.StringFixed(2)}})
}

// checkShape menegakkan arti jenis jurnal kas: KM = uang masuk (kas/bank hanya di debit), KK = uang keluar (kas/bank hanya di kredit),
// TK = tepat dua baris kas/bank berbeda (satu debit, satu kredit).
func checkShape(typ string, lines []gen.JournalLinesRow, cashBank map[uuid.UUID]bool) error {
	var nCB, nOther int
	for _, l := range lines {
		if !cashBank[l.AccountID] {
			nOther++
			continue
		}
		nCB++
		switch typ {
		case "KM":
			if !l.Debit.IsPositive() {
				return ErrJournalShape
			}
		case "KK":
			if !l.Credit.IsPositive() {
				return ErrJournalShape
			}
		}
	}
	switch typ {
	case "KM", "KK":
		if nCB == 0 || nOther == 0 {
			return ErrJournalShape
		}
	case "TK":
		if len(lines) != 2 || nCB != 2 || lines[0].AccountID == lines[1].AccountID || lines[0].Debit.IsPositive() == lines[1].Debit.IsPositive() {
			return ErrJournalShape
		}
	}
	return nil
}

// Reverse membuat jurnal balik (debit/kredit ditukar) atas jurnal terposting, langsung diposting. Tanggal kosong = tanggal asal;
// bila periodenya tertutup, pilih tanggal di periode terbuka. Satu jurnal hanya bisa dibalik sekali.
func (s *Service) Reverse(ctx context.Context, a authz.Actor, id uuid.UUID, date, narration string) (Journal, error) {
	narr, e := cleanText(narration, 400, false)
	if e != "" {
		return Journal{}, FieldErrors{"narration": e}
	}
	var out Journal
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('acc-entry:' || $1::text, 0))`, id); err != nil {
			return err
		}
		orig, err := lockEntry(ctx, q, a.TenantID, id)
		if err != nil {
			return err
		}
		if !outletAllowed(a, orig.OutletID) {
			return ErrOutletForbidden
		}
		if orig.Status != "posted" {
			return ErrNotPosted
		}
		done, err := q.JournalReversalExists(ctx, gen.JournalReversalExistsParams{TenantID: a.TenantID, ReversesID: pgUUID(id)})
		if err != nil {
			return err
		}
		if done {
			return ErrAlreadyReversed
		}
		d := orig.EntryDate.Time
		if strings.TrimSpace(date) != "" {
			var ok bool
			if d, ok = parseDate(date); !ok {
				return FieldErrors{"date": sanitize.Invalid}
			}
		}
		rows, err := q.JournalLines(ctx, gen.JournalLinesParams{TenantID: a.TenantID, EntryID: id})
		if err != nil {
			return err
		}
		memo := "Balik " + orig.DocNo.String
		if narr != "" {
			memo += ": " + narr
		}
		newID, err := q.JournalInsert(ctx, gen.JournalInsertParams{TenantID: a.TenantID, OutletID: orig.OutletID, EntryDate: pgDate(d), Type: orig.Type,
			Narration: memo, ReversesID: pgUUID(id), CreatedBy: a.UserID})
		if err != nil {
			return err
		}
		swapped := make([]normLine, len(rows))
		for i, r := range rows {
			swapped[i] = normLine{account: r.AccountID, debit: r.Credit, credit: r.Debit, memo: r.Memo}
		}
		if err := insertLines(ctx, q, a.TenantID, newID, d, swapped); err != nil {
			return err
		}
		if err := postTx(ctx, tx, a, newID, audit.ActionJournalReverse); err != nil {
			return err
		}
		out, err = getJournal(ctx, tx, a, newID)
		return err
	})
	return out, err
}

type OpeningInput struct {
	OutletID  uuid.UUID   `json:"outlet_id"`
	Date      string      `json:"date"` // tanggal 1 (awal periode pertama)
	Narration string      `json:"narration"`
	Lines     []LineInput `json:"lines"`
}

// PostOpening mencatat saldo awal akun sebagai jurnal pembuka (OPENING), langsung diposting dan wajib seimbang.
// Satu jurnal pembuka aktif per tenant; mengoreksinya = jurnal balik lalu catat ulang.
func (s *Service) PostOpening(ctx context.Context, a authz.Actor, in OpeningInput) (Journal, error) {
	outlet, date, narr, lines, err := normalize(a, JournalInput{OutletID: in.OutletID, Date: in.Date, Type: "OPENING", Narration: in.Narration, Lines: in.Lines},
		func(t string) bool { return t == "OPENING" })
	if err != nil {
		return Journal{}, err
	}
	if date.Day() != 1 {
		return Journal{}, FieldErrors{"date": sanitize.Invalid}
	}
	var out Journal
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if err := activeOutlet(ctx, tx, a.TenantID, outlet); err != nil {
			return err
		}
		// Serialkan pencatatan pembuka bersamaan (cek-lalu-tulis) per tenant.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('acc-opening:' || $1::text, 0))`, a.TenantID); err != nil {
			return err
		}
		exists, err := q.JournalOpeningExists(ctx, a.TenantID)
		if err != nil {
			return err
		}
		if exists {
			return ErrOpeningExists
		}
		id, err := q.JournalInsert(ctx, gen.JournalInsertParams{TenantID: a.TenantID, OutletID: outlet, EntryDate: pgDate(date), Type: "OPENING",
			Narration: narr, CreatedBy: a.UserID})
		if err != nil {
			return err
		}
		if err := insertLines(ctx, q, a.TenantID, id, date, lines); err != nil {
			return err
		}
		if err := postTx(ctx, tx, a, id, audit.ActionJournalPost); err != nil {
			return err
		}
		out, err = getJournal(ctx, tx, a, id)
		return err
	})
	return out, err
}

// lockEntry membaca header jurnal dan, bila masih draf, menguncinya FOR UPDATE. Jurnal terposting tidak bisa dikunci oleh role
// aplikasi (policy RLS UPDATE hanya untuk draf) dan memang tidak perlu: ia immutable. Draf yang diposting transaksi lain selagi
// kita menunggu kunci terbaca sebagai terposting.
func lockEntry(ctx context.Context, q *gen.Queries, tenant, id uuid.UUID) (gen.JournalGetRow, error) {
	cur, err := q.JournalGet(ctx, gen.JournalGetParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return cur, ErrNotFound
	}
	if err != nil || cur.Status != "draft" {
		return cur, err
	}
	locked, err := q.JournalLockGet(ctx, gen.JournalLockGetParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		cur.Status = "posted"
		return cur, nil
	}
	return gen.JournalGetRow(locked), err
}
