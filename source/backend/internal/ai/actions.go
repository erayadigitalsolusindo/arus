package ai

// Aksi tulis HalloArus. PRINSIP KEAMANAN (jangan dilonggarkan):
//   - AI tidak pernah menulis. Tool `propose_*` hanya menyimpan USULAN di Redis (TTL singkat) dan mengembalikan id; ringkasan
//     usulan dibuat SERVER dari data/parameter tervalidasi, bukan dari teks model.
//   - Yang menulis adalah pengguna: tombol Setujui di UI memanggil ConfirmAction, yang menjalankan use-case resmi (shift.Service)
//     dengan hak pengguna itu sendiri: transaksi, idempotensi, audit, dan cek izin berlaku seperti layar biasa.
//   - Pembatalan (void) wajib PIN penyetuju yang diketik pengguna di UI dan dikirim langsung ke server; PIN tidak pernah lewat
//     percakapan sehingga tidak terlihat model dan tidak tersimpan di riwayat.
//   - Hanya pengguna dengan izin ai_assistant.create yang diberi tool usulan, dan usulan terikat ke pengguna + outlet pembuatnya.
//   - Jalur MCP (connector) tidak memuat tool ini; ia tetap hanya-baca.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"aciraba/internal/approval"
	"aciraba/internal/audit"
	"aciraba/internal/authz"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/sanitize"
	"aciraba/internal/shift"
)

const (
	actionTTL = 15 * time.Minute

	KindPettyAdd  = "petty_cash_add"
	KindPettyVoid = "petty_cash_void"

	pettyCategoryMax = 40
	pettyNoteMax     = 300
)

var (
	ErrActionNotFound = errors.New("usulan aksi tidak ditemukan atau sudah kedaluwarsa")
	ErrActionForbid   = errors.New("tidak berhak menjalankan aksi ini")
)

// PendingAction = usulan yang ditampilkan UI sebagai kartu konfirmasi.
type PendingAction struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	NeedsPin  bool              `json:"needs_pin"`
	ExpiresAt time.Time         `json:"expires_at"`
	Fields    map[string]string `json:"fields"` // ringkasan buatan server (direction, amount, category, note, created)
}

// storedAction = usulan di Redis, lengkap dengan pemilik dan muatan yang akan dijalankan saat disetujui.
type storedAction struct {
	PendingAction
	UserID   uuid.UUID       `json:"user_id"`
	OutletID uuid.UUID       `json:"outlet_id"`
	Payload  json.RawMessage `json:"payload"`
}

type pettyAddPayload struct {
	Direction string `json:"direction"`
	Amount    string `json:"amount"`
	Category  string `json:"category"`
	Note      string `json:"note"`
}

type pettyVoidPayload struct {
	MovementID uuid.UUID `json:"movement_id"`
}

// ActionResult = hasil persetujuan.
type ActionResult struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// WithActions mengaktifkan aksi tulis (butuh Redis untuk menyimpan usulan).
func (s *Service) WithActions(shifts *shift.Service, approvals *approval.Service) *Service {
	s.shifts, s.approvals = shifts, approvals
	return s
}

// canWrite = pengguna ini diberi tool usulan.
func (s *Service) canWrite(a authz.Actor) bool {
	return s.shifts != nil && s.approvals != nil && s.rdb != nil && a.OutletID != uuid.Nil &&
		a.Impersonator == uuid.Nil && a.Perms.Has(Module, authz.ActCreate)
}

// writeAllowed = canWrite + platform mengizinkan aksi tulis AI (berlaku juga untuk usulan yang sudah terlanjur dibuat).
func (s *Service) writeAllowed(ctx context.Context, a authz.Actor) bool {
	if !s.canWrite(a) {
		return false
	}
	pol, err := s.mustPolicy(ctx, a)
	return err == nil && pol.AllowWrite
}

func writeToolDefs() []toolDef {
	return []toolDef{
		{
			Name: "propose_petty_cash_entry",
			Description: "Propose recording ONE petty-cash movement (cash out or cash in of the drawer, e.g. an expense from a receipt photo) in the user's open cashier shift. " +
				"This does NOT save anything: it creates a proposal card the user must approve on screen. Use one entry per receipt total, not per line item. " +
				"Never invent numbers: if the amount or date is unreadable or ambiguous, ask the user instead of proposing.",
			InputSchema: schema(map[string]any{
				"direction": enum("`out` = money leaves the drawer (expense); `in` = money added to the drawer.", "out", "in"),
				"amount":    map[string]any{"type": "string", "description": "Positive amount in rupiah, digits only with optional 2 decimals, e.g. \"85000\" or \"85000.50\"."},
				"category":  map[string]any{"type": "string", "description": "Short category, max 40 chars, e.g. Operasional, Makan, Transport, Perlengkapan."},
				"note":      map[string]any{"type": "string", "description": "Short description: store name, receipt date/number, what was bought. Max 300 chars."},
			}, "direction", "amount", "category"),
		},
		{
			Name: "propose_void_petty_cash_entry",
			Description: "Propose cancelling (voiding) one petty-cash entry of a still-open shift. This is a deletion: it does NOT run now; the user must approve on screen " +
				"AND type an approver PIN there. Never ask for the PIN in chat. Use list_petty_cash first to get the entry id.",
			InputSchema: schema(map[string]any{
				"movement_id": map[string]any{"type": "string", "description": "Petty-cash entry id (uuid) from list_petty_cash."},
			}, "movement_id"),
		},
	}
}

func isWriteTool(name string) bool {
	return name == "propose_petty_cash_entry" || name == "propose_void_petty_cash_entry"
}

func (s *Service) actKey(tenant uuid.UUID, id string) string {
	return "ai:act:" + tenant.String() + ":" + id
}

// readTx menjalankan fn di transaksi READ ONLY milik tenant (dipakai untuk memvalidasi usulan).
func (s *Service) readTx(ctx context.Context, a authz.Actor, fn func(tx pgx.Tx) error) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := db.SetTenant(ctx, tx, a.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '"+queryTimeout+"'"); err != nil {
			return err
		}
		return fn(tx)
	})
}

// propose memvalidasi parameter dari model, menyimpan usulan, dan mengembalikan teks tool_result untuk model.
func (s *Service) propose(ctx context.Context, a authz.Actor, name string, raw json.RawMessage) (PendingAction, string, error) {
	if !s.writeAllowed(ctx, a) {
		return PendingAction{}, "", errors.New("pengguna tidak punya izin menjalankan aksi tulis lewat AI")
	}
	p := params{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return PendingAction{}, "", errors.New("parameter bukan JSON objek yang valid")
		}
	}
	var (
		st  storedAction
		err error
	)
	switch name {
	case "propose_petty_cash_entry":
		st, err = s.proposePettyAdd(ctx, a, p)
	case "propose_void_petty_cash_entry":
		st, err = s.proposePettyVoid(ctx, a, p)
	default:
		err = fmt.Errorf("tool %q tidak dikenal", name)
	}
	if err != nil {
		return PendingAction{}, "", err
	}
	st.ID = uuid.NewString()
	st.ExpiresAt = time.Now().UTC().Add(actionTTL)
	st.UserID, st.OutletID = a.UserID, a.OutletID
	b, err := json.Marshal(st)
	if err != nil {
		return PendingAction{}, "", err
	}
	if err := s.rdb.Set(ctx, s.actKey(a.TenantID, st.ID), b, actionTTL).Err(); err != nil {
		return PendingAction{}, "", err
	}
	out, _ := json.Marshal(map[string]any{
		"status":     "awaiting_user_approval",
		"action_id":  st.ID,
		"summary":    st.Fields,
		"needs_pin":  st.NeedsPin,
		"expires_in": "15 minutes",
		"note":       "NOT saved yet. The user must press Approve on the card shown on screen" + map[bool]string{true: " and enter the approver PIN there (never ask for it in chat)", false: ""}[st.NeedsPin] + ". Tell the user this; do not claim it is saved.",
	})
	return st.PendingAction, string(out), nil
}

func amountParam(p params) (decimal.Decimal, error) {
	var s string
	switch v := p["amount"].(type) {
	case string:
		s = v
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	}
	d, err := decimal.NewFromString(s)
	if err != nil || !d.IsPositive() || !d.Equal(d.Round(2)) || d.GreaterThanOrEqual(decimal.NewFromInt(1_000_000_000_000)) {
		return decimal.Zero, errors.New("`amount` harus angka positif rupiah (maks 2 desimal); bila angka di nota tidak jelas, tanyakan pengguna")
	}
	return d, nil
}

func (s *Service) proposePettyAdd(ctx context.Context, a authz.Actor, p params) (storedAction, error) {
	if !a.Perms.Has(shift.ModuleCashier, authz.ActCreate) {
		return storedAction{}, errors.New("pengguna tidak punya izin mencatat kas kecil")
	}
	dir, err := p.oneOf("direction", "", "out", "in")
	if err != nil || dir == "" {
		return storedAction{}, errors.New("`direction` harus `out` atau `in`")
	}
	amt, err := amountParam(p)
	if err != nil {
		return storedAction{}, err
	}
	cat, code := sanitize.Name(p.str("category"), pettyCategoryMax)
	if code != "" {
		return storedAction{}, errors.New("`category` wajib diisi (maks 40 karakter, tanpa karakter aneh)")
	}
	note, code := sanitize.Multiline(p.str("note"), pettyNoteMax)
	if code != "" {
		return storedAction{}, errors.New("`note` tidak valid (maks 300 karakter)")
	}
	needsPin := false
	err = s.readTx(ctx, a, func(tx pgx.Tx) error {
		var limit decimal.Decimal
		err := tx.QueryRow(ctx, `SELECT o.petty_cash_approval_limit FROM cash_shifts c
			JOIN outlets o ON o.tenant_id = c.tenant_id AND o.id = c.outlet_id
			WHERE c.tenant_id = $1 AND c.outlet_id = $2 AND c.user_id = $3 AND c.status = 'open'`, a.TenantID, a.OutletID, a.UserID).Scan(&limit)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("pengguna belum membuka shift kasir di outlet ini; minta pengguna membuka shift di menu Kasir dulu")
		}
		needsPin = err == nil && dir == "out" && amt.GreaterThan(limit)
		return err
	})
	if err != nil {
		return storedAction{}, err
	}
	payload, _ := json.Marshal(pettyAddPayload{Direction: dir, Amount: amt.StringFixed(2), Category: cat, Note: note})
	return storedAction{
		PendingAction: PendingAction{Kind: KindPettyAdd, NeedsPin: needsPin,
			Fields: map[string]string{"direction": dir, "amount": amt.StringFixed(2), "category": cat, "note": note}},
		Payload: payload,
	}, nil
}

func (s *Service) proposePettyVoid(ctx context.Context, a authz.Actor, p params) (storedAction, error) {
	id, err := uuid.Parse(p.str("movement_id"))
	if err != nil {
		return storedAction{}, errors.New("`movement_id` bukan uuid; ambil dari list_petty_cash")
	}
	var fields map[string]string
	err = s.readTx(ctx, a, func(tx pgx.Tx) error {
		var outlet, owner uuid.UUID
		var status, dir, cat, note string
		var amt decimal.Decimal
		var voided *time.Time
		var created time.Time
		err := tx.QueryRow(ctx, `SELECT m.outlet_id, c.user_id, c.status, m.direction, m.amount, m.category, m.note, m.voided_at, m.created_at
			FROM cash_movements m JOIN cash_shifts c ON c.tenant_id = m.tenant_id AND c.id = m.shift_id
			WHERE m.tenant_id = $1 AND m.id = $2`, a.TenantID, id).Scan(&outlet, &owner, &status, &dir, &amt, &cat, &note, &voided, &created)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && outlet != a.OutletID && !a.Outlets[outlet]) {
			return errors.New("catatan kas kecil tidak ditemukan")
		}
		if err != nil {
			return err
		}
		own := owner == a.UserID && outlet == a.OutletID
		if !own && !a.Perms.Has(shift.Module, authz.ActUpdate) {
			return errors.New("pengguna tidak berhak membatalkan catatan ini")
		}
		if voided != nil {
			return errors.New("catatan kas kecil ini sudah dibatalkan")
		}
		if status != "open" {
			return errors.New("shift catatan ini sudah ditutup; pembatalan hanya bisa selama shift terbuka")
		}
		fields = map[string]string{"direction": dir, "amount": amt.StringFixed(2), "category": cat, "note": note, "created": created.UTC().Format(time.RFC3339)}
		return nil
	})
	if err != nil {
		return storedAction{}, err
	}
	payload, _ := json.Marshal(pettyVoidPayload{MovementID: id})
	return storedAction{PendingAction: PendingAction{Kind: KindPettyVoid, NeedsPin: true, Fields: fields}, Payload: payload}, nil
}

func (s *Service) loadAction(ctx context.Context, a authz.Actor, id string) (storedAction, error) {
	if _, err := uuid.Parse(id); err != nil || s.rdb == nil {
		return storedAction{}, ErrActionNotFound
	}
	raw, err := s.rdb.Get(ctx, s.actKey(a.TenantID, id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return storedAction{}, ErrActionNotFound
	}
	if err != nil {
		return storedAction{}, err
	}
	var st storedAction
	if err := json.Unmarshal(raw, &st); err != nil || st.UserID != a.UserID || st.OutletID != a.OutletID {
		return storedAction{}, ErrActionNotFound // milik pengguna/outlet lain diperlakukan sama dengan tidak ada
	}
	return st, nil
}

// ConfirmAction menjalankan usulan atas persetujuan pengguna (tombol di UI). Gagal PIN/validasi tidak menghapus usulan,
// sehingga pengguna bisa mencoba lagi selama masih berlaku.
func (s *Service) ConfirmAction(ctx context.Context, a authz.Actor, id string, ap *shift.ApprovalIn) (*ActionResult, error) {
	if a.Impersonator != uuid.Nil {
		return nil, ErrImpersonating
	}
	if !s.writeAllowed(ctx, a) {
		return nil, ErrActionForbid
	}
	st, err := s.loadAction(ctx, a, id)
	if err != nil {
		return nil, err
	}
	var approver *uuid.UUID
	switch st.Kind {
	case KindPettyAdd:
		if !a.Perms.Has(shift.ModuleCashier, authz.ActCreate) {
			return nil, ErrActionForbid
		}
		var pl pettyAddPayload
		if err := json.Unmarshal(st.Payload, &pl); err != nil {
			return nil, ErrActionNotFound
		}
		// Kunci idempotensi diturunkan dari id usulan: klik ganda / kirim ulang tidak mencatat dua kali.
		if _, err := s.shifts.AddMovement(ctx, a, "ai-"+id, shift.MovementInput{
			Direction: pl.Direction, Amount: json.Number(pl.Amount), Category: pl.Category, Note: pl.Note, Approval: ap,
		}); err != nil {
			return nil, err
		}
		if ap != nil && st.NeedsPin {
			approver = &ap.UserID
		}
	case KindPettyVoid:
		var pl pettyVoidPayload
		if err := json.Unmarshal(st.Payload, &pl); err != nil {
			return nil, ErrActionNotFound
		}
		if ap == nil {
			return nil, approval.ErrPinRequired
		}
		if err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
			_, err := s.approvals.VerifyFor(ctx, tx, a, shift.ModuleApprove, a.OutletID, ap.UserID, ap.Pin)
			return err
		}); err != nil {
			return nil, err
		}
		approver = &ap.UserID
		if err := s.shifts.VoidMovement(ctx, a, pl.MovementID); err != nil {
			return nil, err
		}
	default:
		return nil, ErrActionNotFound
	}
	s.rdb.Del(ctx, s.actKey(a.TenantID, id))
	s.setActionStatus(ctx, a, id, "done")
	s.auditAction(ctx, a, st, approver)
	return &ActionResult{ID: id, Kind: st.Kind}, nil
}

// CancelAction membuang usulan (idempoten).
func (s *Service) CancelAction(ctx context.Context, a authz.Actor, id string) error {
	if _, err := s.loadAction(ctx, a, id); err != nil {
		if errors.Is(err, ErrActionNotFound) {
			return nil
		}
		return err
	}
	s.setActionStatus(ctx, a, id, "cancelled")
	return s.rdb.Del(ctx, s.actKey(a.TenantID, id)).Err()
}

func (s *Service) auditAction(ctx context.Context, a authz.Actor, st storedAction, approver *uuid.UUID) {
	det := map[string]any{"kind": st.Kind, "action_id": st.ID, "fields": st.Fields}
	if approver != nil {
		det["approved_by"] = approver.String()
	}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return audit.Record(ctx, tx, audit.FromActor(a), audit.Entry{Action: audit.ActionAIAction, Entity: audit.EntityAI, EntityID: st.ID, Details: det})
	})
	if err != nil {
		s.log.WarnContext(ctx, "ai audit aksi gagal", "err", err)
	}
}
