package accounting

import (
	"context"
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

// Jurnal otomatis harian (FR-ACC-02, 8a.3 tahap 2–3). Tiap sumber transaksi (autosources.go) menghasilkan SATU draf jurnal per outlet
// per hari (hari menurut zona waktu outlet / tanggal dokumen); akuntan yang memposting. Idempoten lewat (source_type, source_ref):
//   - draf belum ada -> dibuat; draf ada -> isinya dihitung ulang (dokumen baru/void/edit);
//   - draf sudah diposting -> selisih terhadap yang sudah terposting dibuat sebagai draf "selisih" terpisah
//     (source_ref ...:adj:N); jurnal terposting tidak pernah diubah.
const maxGenerateDays = 93

// MissingMappingError = peran/metode bayar yang belum punya akun sehingga draf tidak bisa dibuat.
type MissingMappingError struct{ Keys, Methods []string }

func (e *MissingMappingError) Error() string {
	return "pemetaan akun belum lengkap: " + strings.Join(append(slices.Clone(e.Keys), e.Methods...), ", ")
}

type GenerateInput struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	OutletID uuid.UUID `json:"outlet_id"` // kosong = semua outlet yang boleh diakses
	Sources  []string  `json:"sources"`   // kosong = semua sumber (lihat dailySources)
}

type GenerateItem struct {
	OutletID  uuid.UUID  `json:"outlet_id"`
	Source    string     `json:"source"`
	Date      string     `json:"date"`
	Action    string     `json:"action"` // created | updated | deleted | adjusted | unchanged
	JournalID *uuid.UUID `json:"journal_id,omitempty"`
}

type GenerateResult struct {
	Items     []GenerateItem `json:"items"`
	Created   int            `json:"created"`
	Updated   int            `json:"updated"`
	Deleted   int            `json:"deleted"`
	Adjusted  int            `json:"adjusted"`
	Unchanged int            `json:"unchanged"`
}

// dayWindow = jendela satu hari outlet: t0/t1 untuk kolom timestamptz, date untuk kolom tanggal dokumen.
type dayWindow struct {
	t0, t1 pgtype.Timestamptz
	date   pgtype.Date
}

type dailySource struct {
	key     string // journal_entries.source_type
	jtype   string // journal_entries.type
	label   string // awal keterangan jurnal
	compute func(ctx context.Context, q *gen.Queries, tenant, outlet uuid.UUID, w dayWindow, b *jb) (n int64, err error)
}

// GenerateDrafts membuat/menyegarkan draf jurnal otomatis untuk rentang tanggal. Tiap (sumber, outlet, hari) satu transaksi.
func (s *Service) GenerateDrafts(ctx context.Context, a authz.Actor, in GenerateInput) (GenerateResult, error) {
	from, ok1 := parseDate(in.From)
	to, ok2 := parseDate(in.To)
	fe := FieldErrors{}
	if !ok1 {
		fe["from"] = sanitize.Invalid
	}
	if !ok2 {
		fe["to"] = sanitize.Invalid
	}
	if ok1 && ok2 && (to.Before(from) || to.Sub(from) > time.Duration(maxGenerateDays-1)*24*time.Hour) {
		fe["to"] = sanitize.Invalid
	}
	srcs := dailySources
	if len(in.Sources) > 0 {
		srcs = nil
		for _, k := range in.Sources {
			i := slices.IndexFunc(dailySources, func(d dailySource) bool { return d.key == k })
			if i < 0 {
				fe["sources"] = sanitize.Invalid
				break
			}
			srcs = append(srcs, dailySources[i])
		}
	}
	if len(fe) > 0 {
		return GenerateResult{}, fe
	}

	var outlets []uuid.UUID
	if in.OutletID != uuid.Nil {
		if !outletAllowed(a, in.OutletID) {
			return GenerateResult{}, ErrOutletForbidden
		}
		outlets = []uuid.UUID{in.OutletID}
	} else {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id FROM outlets WHERE tenant_id = $1 AND active ORDER BY code`, a.TenantID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					return err
				}
				if outletAllowed(a, id) {
					outlets = append(outlets, id)
				}
			}
			return rows.Err()
		})
		if err != nil {
			return GenerateResult{}, err
		}
	}

	res := GenerateResult{Items: []GenerateItem{}}
	for _, outlet := range outlets {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			for _, src := range srcs {
				var item *GenerateItem
				err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) (err error) {
					item, err = generateDay(ctx, tx, a, src, outlet, day)
					return err
				})
				if err != nil {
					return res, err
				}
				if item == nil {
					continue
				}
				res.Items = append(res.Items, *item)
				switch item.Action {
				case "created":
					res.Created++
				case "updated":
					res.Updated++
				case "deleted":
					res.Deleted++
				case "adjusted":
					res.Adjusted++
				default:
					res.Unchanged++
				}
			}
		}
	}
	if len(res.Items) > 0 {
		err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionJournalAutoGenerate, Entity: audit.EntityJournal,
				Details: map[string]any{"from": in.From, "to": in.To, "sources": len(srcs), "created": res.Created, "updated": res.Updated,
					"deleted": res.Deleted, "adjusted": res.Adjusted}})
		})
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

func autoRef(outlet uuid.UUID, day time.Time) string {
	return outlet.String() + ":" + day.Format(dateLayout)
}

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// generateDay menjalankan algoritma di atas untuk satu sumber-outlet-hari. nil = tidak ada transaksi dan tidak ada draf.
func generateDay(ctx context.Context, tx pgx.Tx, a authz.Actor, src dailySource, outlet uuid.UUID, day time.Time) (*GenerateItem, error) {
	q := gen.New(tx)
	ref := autoRef(outlet, day)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, a.TenantID.String()+"|"+src.key+"|"+ref); err != nil {
		return nil, err
	}
	w, err := q.AutoOutletWindow(ctx, gen.AutoOutletWindowParams{Day: pgDate(day), TenantID: a.TenantID, OutletID: outlet})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, FieldErrors{"outlet_id": sanitize.Invalid}
	}
	if err != nil {
		return nil, err
	}
	b, err := newJB(ctx, q, a.TenantID)
	if err != nil {
		return nil, err
	}
	n, err := src.compute(ctx, q, a.TenantID, outlet, dayWindow{t0: w.T0, t1: w.T1, date: pgDate(day)}, b)
	if err != nil {
		return nil, err
	}
	var target []normLine
	if n > 0 {
		if target, err = b.finish(day); err != nil {
			return nil, err
		}
	}
	base, err := q.AutoJournalByRef(ctx, gen.AutoJournalByRefParams{TenantID: a.TenantID, SourceType: pgText(src.key), SourceRef: pgText(ref)})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	hasBase := err == nil
	item := func(action string, id uuid.UUID) (*GenerateItem, error) {
		return &GenerateItem{OutletID: outlet, Source: src.key, Date: day.Format(dateLayout), Action: action, JournalID: &id}, nil
	}
	narr := fmt.Sprintf("%s %s · %s (%d dokumen)", src.label, day.Format(dateLayout), w.Name, n)
	dayLabel := fmt.Sprintf("%s %s", src.label, day.Format(dateLayout))

	switch {
	case !hasBase && n == 0:
		return nil, nil

	case !hasBase:
		date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
		if err != nil {
			return nil, err
		}
		id, err := newAutoDraft(ctx, q, a, src, outlet, date, narr, ref, target)
		if err != nil {
			return nil, err
		}
		return item("created", id)

	case base.Status == "draft":
		if n == 0 {
			if _, err := q.JournalDeleteDraft(ctx, gen.JournalDeleteDraftParams{TenantID: a.TenantID, ID: base.ID}); err != nil {
				return nil, err
			}
			return item("deleted", base.ID)
		}
		date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
		if err != nil {
			return nil, err
		}
		if err := rewriteAutoDraft(ctx, q, a, base.ID, outlet, date, narr, target); err != nil {
			return nil, err
		}
		return item("updated", base.ID)
	}

	// Sudah diposting: hanya selisih yang boleh dicatat, sebagai draf terpisah.
	posted, err := q.AutoJournalPostedNet(ctx, gen.AutoJournalPostedNetParams{TenantID: a.TenantID, SourceType: pgText(src.key), Column3: ref})
	if err != nil {
		return nil, err
	}
	net := map[uuid.UUID]dec{}
	for _, l := range target {
		net[l.account] = net[l.account].Add(l.debit).Sub(l.credit)
	}
	for _, p := range posted {
		net[p.AccountID] = net[p.AccountID].Sub(p.Net)
	}
	accs := make([]uuid.UUID, 0, len(net))
	for id, v := range net {
		if !v.IsZero() {
			accs = append(accs, id)
		}
	}
	slices.SortFunc(accs, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	delta := make([]normLine, 0, len(accs))
	for _, id := range accs {
		l := normLine{account: id, memo: "Selisih " + dayLabel}
		if net[id].IsPositive() {
			l.debit = net[id]
		} else {
			l.credit = net[id].Neg()
		}
		delta = append(delta, l)
	}
	pending, err := q.AutoJournalPendingAdj(ctx, gen.AutoJournalPendingAdjParams{TenantID: a.TenantID, SourceType: pgText(src.key), Column3: ref})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	hasPending := err == nil
	switch {
	case len(delta) == 0 && !hasPending:
		return item("unchanged", base.ID)
	case len(delta) == 0:
		if _, err := q.JournalDeleteDraft(ctx, gen.JournalDeleteDraftParams{TenantID: a.TenantID, ID: pending}); err != nil {
			return nil, err
		}
		return item("deleted", pending)
	}
	adjNarr := fmt.Sprintf("Selisih %s · %s (dokumen diubah/dibatalkan setelah diposting)", dayLabel, w.Name)
	date, err := autoDraftDate(ctx, q, a.TenantID, day, w.Today.Time)
	if err != nil {
		return nil, err
	}
	if hasPending {
		if err := rewriteAutoDraft(ctx, q, a, pending, outlet, date, adjNarr, delta); err != nil {
			return nil, err
		}
		return item("adjusted", pending)
	}
	cnt, err := q.AutoJournalAdjCount(ctx, gen.AutoJournalAdjCountParams{TenantID: a.TenantID, SourceType: pgText(src.key), Column3: ref})
	if err != nil {
		return nil, err
	}
	id, err := newAutoDraft(ctx, q, a, src, outlet, date, adjNarr, fmt.Sprintf("%s:adj:%d", ref, cnt+1), delta)
	if err != nil {
		return nil, err
	}
	return item("adjusted", id)
}

// autoDraftDate = tanggal jurnal otomatis: hari transaksi; bila periodenya sudah ditutup, hari ini (agar masih bisa diposting).
func autoDraftDate(ctx context.Context, q *gen.Queries, tenant uuid.UUID, day, today time.Time) (time.Time, error) {
	closed := func(d time.Time) (bool, error) {
		st, err := q.PeriodStatusGet(ctx, gen.PeriodStatusGetParams{TenantID: tenant, StartDate: pgDate(monthStart(d))})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return st == "closed", err
	}
	c, err := closed(day)
	if err != nil || !c {
		return day, err
	}
	if c, err = closed(today); err != nil {
		return day, err
	}
	if c {
		return day, ErrPeriodClosed
	}
	return today, nil
}

func newAutoDraft(ctx context.Context, q *gen.Queries, a authz.Actor, src dailySource, outlet uuid.UUID, date time.Time, narr, ref string, lines []normLine) (uuid.UUID, error) {
	if _, err := checkAccounts(ctx, q, a.TenantID, accountIDs(lines), true); err != nil {
		return uuid.Nil, err
	}
	id, err := q.JournalInsert(ctx, gen.JournalInsertParams{TenantID: a.TenantID, OutletID: outlet, EntryDate: pgDate(date), Type: src.jtype,
		Narration: narr, SourceType: pgText(src.key), SourceRef: pgText(ref), CreatedBy: pgUUIDOrNull(a.UserID)})
	if err != nil {
		return uuid.Nil, err
	}
	return id, insertLines(ctx, q, a.TenantID, id, date, lines)
}

func rewriteAutoDraft(ctx context.Context, q *gen.Queries, a authz.Actor, id, outlet uuid.UUID, date time.Time, narr string, lines []normLine) error {
	if _, err := checkAccounts(ctx, q, a.TenantID, accountIDs(lines), true); err != nil {
		return err
	}
	if _, err := q.JournalUpdateDraft(ctx, gen.JournalUpdateDraftParams{TenantID: a.TenantID, ID: id, OutletID: outlet, EntryDate: pgDate(date), Narration: narr}); err != nil {
		return err
	}
	if err := q.JournalLinesDelete(ctx, gen.JournalLinesDeleteParams{TenantID: a.TenantID, EntryID: id}); err != nil {
		return err
	}
	return insertLines(ctx, q, a.TenantID, id, date, lines)
}

// ---- Pembangun baris jurnal ----

// jb mengumpulkan baris jurnal otomatis dan mencatat peran/metode bayar yang belum punya akun.
type jb struct {
	acc     map[string]uuid.UUID
	missing MissingMappingError
	lines   []normLine
}

func newJB(ctx context.Context, q *gen.Queries, tenant uuid.UUID) (*jb, error) {
	maps, err := listMappings(ctx, q, tenant)
	if err != nil {
		return nil, err
	}
	b := &jb{acc: map[string]uuid.UUID{}}
	for _, m := range maps {
		if m.Account != nil {
			b.acc[m.Key] = m.Account.ID
		}
	}
	return b, nil
}

// post menambah satu baris: signed > 0 = debit, < 0 = kredit, 0 = dilewati.
func (b *jb) post(account uuid.UUID, signed dec, memo string) {
	switch {
	case signed.IsZero() || account == uuid.Nil:
	case signed.IsPositive():
		b.lines = append(b.lines, normLine{account: account, debit: signed, memo: memo})
	default:
		b.lines = append(b.lines, normLine{account: account, credit: signed.Neg(), memo: memo})
	}
}

func (b *jb) role(key string) uuid.UUID {
	id, ok := b.acc[key]
	if !ok && !slices.Contains(b.missing.Keys, key) {
		b.missing.Keys = append(b.missing.Keys, key)
	}
	return id
}

func (b *jb) method(accountID pgtype.UUID, name string) uuid.UUID {
	if accountID.Valid {
		return uuid.UUID(accountID.Bytes)
	}
	if !slices.Contains(b.missing.Methods, name) {
		b.missing.Methods = append(b.missing.Methods, name)
	}
	return uuid.Nil
}

// drRole/crRole: akun dari pemetaan peran; jumlah negatif membalik sisi.
func (b *jb) drRole(key string, amount dec, memo string) {
	if !amount.IsZero() {
		b.post(b.role(key), amount, memo)
	}
}
func (b *jb) crRole(key string, amount dec, memo string) {
	if !amount.IsZero() {
		b.post(b.role(key), amount.Neg(), memo)
	}
}

// drMethod/crMethod: akun dari metode bayar.
func (b *jb) drMethod(accountID pgtype.UUID, name string, amount dec, memo string) {
	if !amount.IsZero() {
		b.post(b.method(accountID, name), amount, memo)
	}
}
func (b *jb) crMethod(accountID pgtype.UUID, name string, amount dec, memo string) {
	if !amount.IsZero() {
		b.post(b.method(accountID, name), amount.Neg(), memo)
	}
}

// finish memeriksa kelengkapan pemetaan dan keseimbangan, lalu mengembalikan baris jurnal.
func (b *jb) finish(day time.Time) ([]normLine, error) {
	if len(b.missing.Keys)+len(b.missing.Methods) > 0 {
		m := b.missing
		return nil, &m
	}
	var td, tc dec
	for _, l := range b.lines {
		td, tc = td.Add(l.debit), tc.Add(l.credit)
	}
	if !td.Equal(tc) {
		return nil, fmt.Errorf("jurnal otomatis %s tidak seimbang (debit %s, kredit %s): %w", day.Format(dateLayout), td, tc, ErrJournalUnbalanced)
	}
	return b.lines, nil
}

// ---- Posting massal ----

// BatchPostItem = hasil posting satu draf dalam posting massal.
type BatchPostItem struct {
	ID    uuid.UUID `json:"id"`
	OK    bool      `json:"ok"`
	Code  string    `json:"code,omitempty"`
	DocNo string    `json:"doc_no,omitempty"`
}

const maxBatchPost = 100

// PostBatch memposting banyak draf; tiap draf transaksi sendiri sehingga satu kegagalan tidak membatalkan yang lain.
func (s *Service) PostBatch(ctx context.Context, a authz.Actor, ids []uuid.UUID) ([]BatchPostItem, error) {
	if len(ids) == 0 || len(ids) > maxBatchPost {
		return nil, FieldErrors{"ids": sanitize.Invalid}
	}
	out := make([]BatchPostItem, 0, len(ids))
	for _, id := range ids {
		j, err := s.Post(ctx, a, id)
		if err != nil {
			out = append(out, BatchPostItem{ID: id, Code: postErrorCode(err)})
			continue
		}
		out = append(out, BatchPostItem{ID: id, OK: true, DocNo: j.DocNo})
	}
	return out, nil
}

func postErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrOutletForbidden):
		return "OUTLET_FORBIDDEN"
	case errors.Is(err, ErrJournalPosted):
		return "JOURNAL_POSTED"
	case errors.Is(err, ErrJournalUnbalanced):
		return "JOURNAL_UNBALANCED"
	case errors.Is(err, ErrJournalShape):
		return "JOURNAL_SHAPE_INVALID"
	case errors.Is(err, ErrAccountInvalid):
		return "ACCOUNT_INVALID"
	case errors.Is(err, ErrPeriodClosed):
		return "PERIOD_CLOSED"
	}
	return "INTERNAL"
}

// pgUUIDOrNull: uuid.Nil (aksi sistem/penjadwal) disimpan sebagai NULL.
func pgUUIDOrNull(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgUUID(id)
}
