package sales

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	gen "aciraba/internal/gen"
	"aciraba/internal/member"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/stock"
	"aciraba/internal/voucher"
)

// Edit & batal nota. Nota TIDAK pernah ditimpa:
//   - batal  = nota ber-status 'void' + alasan; dampaknya (stok, poin, kupon) dibalik.
//   - edit   = nota lama jadi 'superseded' dan dibuat revisi baru (nomor sama + "-R2", "-R3", …) yang tertaut ke nota lama.
//
// Semuanya satu transaksi: kalau satu langkah gagal (mis. stok revisi kurang), nota lama utuh seperti semula. Stok dibalik
// lewat movement SALE_VOID (ledger tetap append-only) sebelum revisi mencatat movement SALE barunya.
var (
	ErrNotEditable    = errors.New("nota tidak dapat diubah lagi")
	ErrEditWindow     = errors.New("batas waktu edit nota sudah lewat")
	ErrOutletMismatch = errors.New("nota milik outlet lain; pindah ke outlet nota itu dulu")
)

const (
	minReason = 3
	maxReason = 200
)

// editCtx = konteks revisi untuk save().
type editCtx struct {
	orig     gen.SalesLockForEditRow
	rootID   uuid.UUID
	localDay pgtype.Date // hari bisnis nota asli (untuk validasi kupon/member)
	docNo    string
	reason   string
	editor   approval.Approver
}

// VoidInput = alasan + persetujuan (Owner/Supervisor berizin sale_edit.approve beserta PIN-nya).
type VoidInput struct {
	Reason   string      `json:"reason"`
	Approval *ApprovalIn `json:"approval"`
}

// EditRequest = isi nota baru (sama seperti nota baru) + alasan. `approval` menyetujui edit (dan sekaligus ubah harga/potongan
// BARU bila ada; baris yang tidak berubah mempertahankan harga, HPP, dan potongan nota asli tanpa persetujuan ulang).
type EditRequest struct {
	Request
	Reason string `json:"reason"`
}

var revSuffix = regexp.MustCompile(`-R\d+$`)

func revisionDocNo(docNo string, rev int) string {
	return fmt.Sprintf("%s-R%d", revSuffix.ReplaceAllString(docNo, ""), rev)
}

func cleanReason(raw string) (string, FieldErrors) {
	r, ok := sanitize.Text(raw)
	switch {
	case strings.TrimSpace(raw) == "":
		return "", FieldErrors{"reason": sanitize.Required}
	case !ok || utf8.RuneCountInString(r) < minReason || utf8.RuneCountInString(r) > maxReason:
		return "", FieldErrors{"reason": sanitize.Invalid}
	}
	return r, nil
}

// lockForEdit mengunci nota dan memeriksa aturannya: masih berstatus completed, outlet, dan batas hari edit tenant.
// sameOutlet: edit harus dari outlet nota itu (harga/pajak/stok memakai outlet aktif); batal cukup akses ke outletnya.
func lockForEdit(ctx context.Context, q *gen.Queries, a authz.Actor, id uuid.UUID, sameOutlet bool) (gen.SalesLockForEditRow, error) {
	row, err := q.SalesLockForEdit(ctx, gen.SalesLockForEditParams{TenantID: a.TenantID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if row.OutletID != a.OutletID {
		if !a.Outlets[row.OutletID] {
			return row, ErrNotFound
		}
		if sameOutlet {
			return row, ErrOutletMismatch
		}
	}
	if row.Status != "completed" {
		return row, ErrNotEditable
	}
	if days := int(row.Today.Time.Sub(row.LocalDay.Time).Hours() / 24); days > int(row.SaleEditWindowDays) {
		return row, ErrEditWindow
	}
	return row, nil
}

func (s *Service) approveEdit(ctx context.Context, tx pgx.Tx, a authz.Actor, outletID uuid.UUID, ap *ApprovalIn) (approval.Approver, error) {
	if s.approvals == nil || ap == nil {
		return approval.Approver{}, approval.ErrPinRequired
	}
	return s.approvals.VerifyFor(ctx, tx, a, approval.ModuleSaleEdit, outletID, ap.UserID, ap.PIN)
}

// reverseEffects membalik semua dampak nota di dalam transaksi pemanggil: stok kembali ke display (movement SALE_VOID),
// poin member dikoreksi, dan pemakaian kupon dikembalikan. Pengurangan stok yang dilakukan ulang oleh revisi dijaga
// oleh stock.Apply seperti penjualan biasa.
func reverseEffects(ctx context.Context, tx pgx.Tx, q *gen.Queries, a authz.Actor, orig gen.SalesLockForEditRow) error {
	ls, err := q.SalesLinesForReverse(ctx, gen.SalesLinesForReverseParams{TenantID: a.TenantID, SaleID: orig.ID})
	if err != nil {
		return err
	}
	var moves []stock.Movement
	for _, l := range ls {
		if l.Kind != "goods" {
			continue
		}
		moves = append(moves, stock.Movement{TenantID: a.TenantID, OutletID: orig.OutletID, ItemID: l.ItemID, Bucket: stock.BucketDisplay,
			Delta: l.Qty.Mul(l.Factor), RefType: stock.RefSaleVoid, RefID: orig.ID, Note: orig.DocNo, ActorID: a.UserID})
	}
	if moves = mergeMoves(moves); len(moves) > 0 {
		if _, err := stock.ApplyAll(ctx, tx, moves); err != nil {
			return err
		}
	}
	if err := member.ReverseSale(ctx, tx, a, orig.ID, orig.DocNo); err != nil {
		return err
	}
	vids, err := q.SalesVoucherIDs(ctx, gen.SalesVoucherIDsParams{TenantID: a.TenantID, SaleID: orig.ID})
	if err != nil {
		return err
	}
	slices.SortFunc(vids, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) }) // urutan tetap = tanpa deadlock
	for _, vid := range vids {
		if err := voucher.Release(ctx, tx, a.TenantID, vid); err != nil {
			return err
		}
	}
	return nil
}

// Void membatalkan satu nota (alasan + PIN penyetuju wajib); stok, poin, dan kupon dibalik.
func (s *Service) Void(ctx context.Context, a authz.Actor, id uuid.UUID, in VoidInput) (Sale, error) {
	reason, fe := cleanReason(in.Reason)
	if len(fe) > 0 {
		return Sale{}, fe
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		orig, err := lockForEdit(ctx, q, a, id, false)
		if err != nil {
			return err
		}
		ap, err := s.approveEdit(ctx, tx, a, orig.OutletID, in.Approval)
		if err != nil {
			return err
		}
		if err := reverseEffects(ctx, tx, q, a, orig); err != nil {
			return err
		}
		n, err := q.SalesMarkVoid(ctx, gen.SalesMarkVoidParams{TenantID: a.TenantID, ID: id, Reason: pgtype.Text{String: reason, Valid: true},
			VoidedBy: pgtype.UUID{Bytes: a.UserID, Valid: a.UserID != uuid.Nil}})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotEditable
		}
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionSaleVoid, Entity: audit.EntitySale, EntityID: id.String(),
			Details: map[string]any{"doc_no": orig.DocNo, "reason": reason, "total": orig.Total.String(), "approver_id": ap.ID.String(), "approver": ap.Name}})
	})
	if err != nil {
		return Sale{}, err
	}
	return s.Get(ctx, a, id)
}

// Edit menggantikan nota dengan revisi baru. replayed=true bila Idempotency-Key yang sama sudah pernah menyimpan revisi ini.
func (s *Service) Edit(ctx context.Context, a authz.Actor, id uuid.UUID, key string, in EditRequest) (sale Sale, replayed bool, err error) {
	if !idemKeyPattern.MatchString(key) {
		return Sale{}, false, ErrKeyRequired
	}
	reason, fe := cleanReason(in.Reason)
	n, f := normalize(in.Request)
	for k, v := range fe {
		f[k] = v
	}
	if len(f) > 0 {
		return Sale{}, false, f
	}
	sum := sha256.Sum256([]byte("edit|" + id.String() + "|" + reason + "|" + n.hash()))
	h := hex.EncodeToString(sum[:])

	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if ex, e := q.SalesByIdemKey(ctx, gen.SalesByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key}); e == nil {
			if ex.RequestHash != h {
				return ErrKeyMismatch
			}
			return replay{ex.ID}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		orig, e := lockForEdit(ctx, q, a, id, true)
		if e != nil {
			return e
		}
		editor, e := s.approveEdit(ctx, tx, a, orig.OutletID, in.Approval)
		if e != nil {
			return e
		}
		olds, e := q.SalesLines(ctx, gen.SalesLinesParams{TenantID: a.TenantID, SaleID: orig.ID})
		if e != nil {
			return e
		}
		n.keep = make(map[string][]gen.SalesLinesRow, len(olds))
		for _, l := range olds {
			k := keepKey(l.ItemID, l.UnitID)
			n.keep[k] = append(n.keep[k], l)
		}
		root := orig.ID
		if orig.RootID.Valid {
			root = uuid.UUID(orig.RootID.Bytes)
		}
		if e := reverseEffects(ctx, tx, q, a, orig); e != nil {
			return e
		}
		return s.save(ctx, tx, a, n, key, h, in.Approval, &editCtx{orig: orig, rootID: root, localDay: orig.LocalDay,
			docNo: revisionDocNo(orig.DocNo, int(orig.Revision)+1), reason: reason, editor: editor})
	})
	var rp replay
	if errors.As(err, &rp) {
		sale, err = s.Get(ctx, a, rp.id)
		return sale, true, err
	}
	if err != nil {
		return Sale{}, false, err
	}
	var newID uuid.UUID
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		ex, e := gen.New(tx).SalesByIdemKey(ctx, gen.SalesByIdemKeyParams{TenantID: a.TenantID, IdempotencyKey: key})
		newID = ex.ID
		return e
	})
	if err != nil {
		return Sale{}, false, err
	}
	sale, err = s.Get(ctx, a, newID)
	return sale, false, err
}

// ---- HTTP ----

// Edit: PUT /sales/{id} (header Idempotency-Key wajib). 201 = revisi baru; 200 + Idempotent-Replay = kunci sama.
func (h *Handler) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req EditRequest
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	sale, replayed, err := h.svc.Edit(r.Context(), actor(r), id, r.Header.Get("Idempotency-Key"), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusCreated
	if replayed {
		w.Header().Set("Idempotent-Replay", "true")
		status = http.StatusOK
	}
	httpx.JSON(w, status, sale)
}

// Void: POST /sales/{id}/void
func (h *Handler) Void(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req VoidInput
	if !httpx.DecodeJSONLimit(w, r, &req, 16<<10) {
		return
	}
	sale, err := h.svc.Void(r.Context(), actor(r), id, req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sale)
}

// QuoteEdit: POST /sales/{id}/quote — pratinjau hitung edit (tidak menyimpan apa pun).
func (h *Handler) QuoteEdit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.")
		return
	}
	var req Request
	if !httpx.DecodeJSONLimit(w, r, &req, 256<<10) {
		return
	}
	q, err := h.svc.QuoteEdit(r.Context(), actor(r), id, req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, q)
}
