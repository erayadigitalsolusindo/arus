package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"aciraba/internal/approval"
	"aciraba/internal/authz"
	pauth "aciraba/internal/platform/auth"
	"aciraba/internal/shift"
)

const (
	itPassword = "Rahasia-Uji-123"
	totpOK     = "123456"
)

// fakeMFA: 2FA aktif untuk semua pengguna kecuali yang ada di `off`; kode benar hanya totpOK.
type fakeMFA struct{ off map[uuid.UUID]bool }

func (f fakeMFA) Enabled(_ context.Context, _, user uuid.UUID) (bool, error) {
	return !f.off[user], nil
}
func (f fakeMFA) Verify(_ context.Context, _, _ uuid.UUID, code string) (bool, error) {
	return code == totpOK, nil
}

type actEnv struct {
	svc      *Service
	admin    *pgxpool.Pool
	tenant   uuid.UUID
	outlet   uuid.UUID
	cashier  authz.Actor
	other    authz.Actor // kasir lain, tanpa usulan
	approver authz.Actor
	pin      string
	rdb      *redis.Client
	app      *pgxpool.Pool
}

// newActEnv membuat TENANT SEMENTARA (dihapus saat selesai) berisi kasir bershift terbuka dan penyetuju ber-PIN. Tidak menyentuh data nyata.
func newActEnv(t *testing.T) *actEnv {
	t.Helper()
	appURL, adminURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_ADMIN_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if appURL == "" || adminURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_ADMIN_DATABASE_URL/TEST_REDIS_URL tidak di-set")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	appr := approval.NewService(app, rdb, "uji-rahasia-uji-rahasia-uji-rahasia")
	svc, err := NewService(app, rdb, "0123456789abcdef0123456789abcdef", Free{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	svc.WithActions(shift.NewService(app, appr), appr).WithMFA(fakeMFA{off: map[uuid.UUID]bool{}})

	e := &actEnv{app: app, rdb: rdb, svc: svc, admin: admin, tenant: uuid.New(), outlet: uuid.New(), pin: "482915"}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO tenants (id, code, name) VALUES ($1, $2, 'UJI-AI-AKSI')`, e.tenant, "ai-"+e.tenant.String()[:8])
	exec(`INSERT INTO outlets (id, tenant_id, code, name) VALUES ($1, $2, 'main', 'Pusat')`, e.outlet, e.tenant)
	exec(`INSERT INTO payment_methods (tenant_id, name, kind, is_system) VALUES ($1, 'Tunai', 'cash', true)`, e.tenant)
	t.Cleanup(func() {
		for _, tbl := range []string{"audit_log", "cash_movements", "cash_shift_flows", "cash_shift_counts", "cash_shifts", "cash_shift_counters", "payment_methods", "user_outlets", "users", "roles", "outlets"} {
			_, _ = admin.Exec(ctx, `DELETE FROM `+tbl+` WHERE tenant_id = $1`, e.tenant)
		}
		_, _ = admin.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, e.tenant)
	})
	person := func(name, perms string) authz.Actor {
		role, user := uuid.New(), uuid.New()
		hash, err := pauth.HashPassword(itPassword)
		if err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, $4::jsonb)`, role, e.tenant, name+"-"+role.String()[:6], perms)
		exec(`INSERT INTO users (id, tenant_id, role_id, email, name, password_hash) VALUES ($1, $2, $3, $4, $5, $6)`, user, e.tenant, role, name+"-"+user.String()+"@example.test", name, hash)
		exec(`INSERT INTO user_outlets (tenant_id, user_id, outlet_id) VALUES ($1, $2, $3)`, e.tenant, user, e.outlet)
		return authz.Actor{TenantID: e.tenant, UserID: user, OutletID: e.outlet, Name: name, Perms: authz.ParseStored([]byte(perms)), Outlets: map[uuid.UUID]bool{e.outlet: true}}
	}
	const kasirPerms = `{"sales_orders":["view","create"],"ai_assistant":["view","create"]}`
	e.cashier = person("Kasir", kasirPerms)
	e.other = person("KasirLain", kasirPerms)
	e.approver = person("Penyetuju", `{"ai_write":["approve"]}`)
	if err := appr.SetPin(ctx, e.approver, itPassword, e.pin); err != nil {
		t.Fatal(err)
	}
	if _, err := shift.NewService(app, appr).Open(ctx, e.cashier, shift.OpenInput{OpeningCash: "100000"}); err != nil {
		t.Fatal(err)
	}
	return e
}

// good = penyetuju + PIN yang benar.
func (e *actEnv) good() *shift.ApprovalIn {
	return &shift.ApprovalIn{UserID: e.approver.UserID, Pin: e.pin}
}

func (e *actEnv) propose(t *testing.T, a authz.Actor, tool, in string) (PendingAction, error) {
	t.Helper()
	pa, _, err := e.svc.propose(context.Background(), a, tool, json.RawMessage(in))
	return pa, err
}

func (e *actEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.admin.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAIPettyCashAddFlow(t *testing.T) {
	e := newActEnv(t)
	ctx := context.Background()

	// Usulan saja TIDAK menulis apa pun.
	pa, err := e.propose(t, e.cashier, "propose_petty_cash_entry", `{"direction":"out","amount":"85000","category":"Listrik","note":"Toko Listrik Jaya"}`)
	if err != nil || pa.Kind != KindPettyAdd || !pa.NeedsPin || !pa.NeedsTotp || pa.Fields["amount"] != "85000.00" {
		t.Fatalf("propose: %+v %v", pa, err)
	}
	if n := e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1`, e.tenant); n != 0 {
		t.Fatalf("usulan belum boleh menulis, ada %d baris", n)
	}
	// Pengguna lain tidak bisa menyetujui usulan orang.
	if _, err := e.svc.ConfirmAction(ctx, e.other, pa.ID, nil, ""); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("pengguna lain: %v", err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), totpOK); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1 AND amount = 85000 AND direction = 'out' AND category = 'Listrik' AND idem_key = $2 AND created_by = $3`,
		e.tenant, "ai-"+pa.ID, e.cashier.UserID); n != 1 {
		t.Fatalf("catatan kas kecil harus tercatat tepat 1 kali, got %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.action' AND entity_id = $2`, e.tenant, pa.ID); n != 1 {
		t.Fatalf("audit ai.action harus 1, got %d", n)
	}
	// Usulan sekali pakai: klik ulang tidak mencatat dua kali.
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), totpOK); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("confirm ulang: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1`, e.tenant); n != 1 {
		t.Fatalf("harus tetap 1 baris, got %d", n)
	}

	// Di atas batas persetujuan outlet → kartu butuh PIN; PIN salah ditolak tanpa menulis.
	big, err := e.propose(t, e.cashier, "propose_petty_cash_entry", `{"direction":"out","amount":"150000","category":"Perlengkapan"}`)
	if err != nil || !big.NeedsPin {
		t.Fatalf("kas keluar besar harus butuh PIN: %+v %v", big, err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, big.ID, nil, ""); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa PIN: %v", err)
	}
	bad := &shift.ApprovalIn{UserID: e.approver.UserID, Pin: "111222"}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, big.ID, bad, totpOK); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("PIN salah: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1`, e.tenant); n != 1 {
		t.Fatalf("PIN gagal tidak boleh menulis, got %d", n)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, big.ID, e.good(), totpOK); err != nil {
		t.Fatalf("PIN benar: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1 AND amount = 150000 AND approved_by = $2`, e.tenant, e.approver.UserID); n != 1 {
		t.Fatalf("catatan dengan penyetuju harus tercatat, got %d", n)
	}

	// Tanpa shift terbuka, usulan ditolak sejak awal (model diberi tahu, kartu tidak muncul).
	if _, err := e.propose(t, e.other, "propose_petty_cash_entry", `{"direction":"out","amount":"1000","category":"Makan"}`); err == nil {
		t.Fatal("tanpa shift terbuka harus ditolak")
	}
}

func TestAIPettyCashVoidNeedsPin(t *testing.T) {
	e := newActEnv(t)
	ctx := context.Background()

	add, err := e.propose(t, e.cashier, "propose_petty_cash_entry", `{"direction":"out","amount":"30000","category":"Makan","note":"salah catat"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, add.ID, e.good(), totpOK); err != nil {
		t.Fatal(err)
	}
	var movID uuid.UUID
	if err := e.admin.QueryRow(ctx, `SELECT id FROM cash_movements WHERE tenant_id = $1`, e.tenant).Scan(&movID); err != nil {
		t.Fatal(err)
	}

	if _, err := e.propose(t, e.cashier, "propose_void_petty_cash_entry", `{"movement_id":"bukan-uuid"}`); err == nil {
		t.Error("id bukan uuid harus ditolak")
	}
	if _, err := e.propose(t, e.cashier, "propose_void_petty_cash_entry", `{"movement_id":"`+uuid.NewString()+`"}`); err == nil {
		t.Error("id tak dikenal harus ditolak")
	}
	if _, err := e.propose(t, e.other, "propose_void_petty_cash_entry", `{"movement_id":"`+movID.String()+`"}`); err == nil {
		t.Error("kasir lain tanpa izin ubah shift tidak boleh mengusulkan pembatalan")
	}
	va, err := e.propose(t, e.cashier, "propose_void_petty_cash_entry", `{"movement_id":"`+movID.String()+`"}`)
	if err != nil || !va.NeedsPin || va.Kind != KindPettyVoid || va.Fields["note"] != "salah catat" {
		t.Fatalf("propose void: %+v %v", va, err)
	}
	voided := func() int {
		return e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1 AND id = $2 AND voided_at IS NOT NULL`, e.tenant, movID)
	}
	// Hapus WAJIB PIN: tanpa PIN / PIN salah / penyetuju tak berizin → tidak ada yang dibatalkan, usulan tetap ada.
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, va.ID, nil, ""); !errors.Is(err, approval.ErrPinRequired) {
		t.Fatalf("tanpa PIN: %v", err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, va.ID, &shift.ApprovalIn{UserID: e.approver.UserID, Pin: "111222"}, totpOK); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("PIN salah: %v", err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, va.ID, &shift.ApprovalIn{UserID: e.other.UserID, Pin: e.pin}, totpOK); !errors.Is(err, approval.ErrInvalidPin) {
		t.Fatalf("penyetuju tak berizin: %v", err)
	}
	if voided() != 0 {
		t.Fatal("tidak boleh terbatalkan tanpa PIN yang benar")
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, va.ID, &shift.ApprovalIn{UserID: e.approver.UserID, Pin: e.pin}, totpOK); err != nil {
		t.Fatalf("void dengan PIN benar: %v", err)
	}
	if voided() != 1 {
		t.Fatal("catatan harus terbatalkan (soft void, baris tetap ada)")
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'ai.action' AND entity_id = $2`, e.tenant, va.ID); n != 1 {
		t.Fatalf("audit ai.action void harus 1, got %d", n)
	}
	// Sudah dibatalkan: tidak bisa diusulkan lagi.
	if _, err := e.propose(t, e.cashier, "propose_void_petty_cash_entry", `{"movement_id":"`+movID.String()+`"}`); err == nil {
		t.Error("catatan yang sudah dibatalkan tidak boleh diusulkan lagi")
	}
}

// Semua aksi tulis wajib kode 2FA PENYETUJU: tanpa kode / kode salah / 2FA belum aktif ditolak tanpa menulis, dan percobaan salah dikunci.
func TestAIWriteNeedsTOTP(t *testing.T) {
	e := newActEnv(t)
	ctx := context.Background()
	rows := func() int { return e.count(t, `SELECT count(*) FROM cash_movements WHERE tenant_id = $1`, e.tenant) }
	e.rdb.Del(ctx, e.svc.totpKey(e.tenant, e.approver.UserID))
	t.Cleanup(func() { e.rdb.Del(ctx, e.svc.totpKey(e.tenant, e.approver.UserID)) })

	pa, err := e.propose(t, e.cashier, "propose_petty_cash_entry", `{"direction":"out","amount":"5000","category":"Makan"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), ""); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("tanpa TOTP: %v", err)
	}
	for i := 0; i < totpMaxFails; i++ {
		if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), "000000"); !errors.Is(err, ErrTOTPInvalid) {
			t.Fatalf("TOTP salah #%d: %v", i, err)
		}
	}
	var lk *TOTPLockedError
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), totpOK); !errors.As(err, &lk) {
		t.Fatalf("setelah %d salah harus terkunci walau kode benar: %v", totpMaxFails, err)
	}
	if rows() != 0 {
		t.Fatal("tidak boleh menulis tanpa TOTP yang benar")
	}
	e.rdb.Del(ctx, e.svc.totpKey(e.tenant, e.approver.UserID))
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pa.ID, e.good(), totpOK); err != nil {
		t.Fatalf("PIN + TOTP benar: %v", err)
	}
	if rows() != 1 {
		t.Fatalf("harus tercatat 1, got %d", rows())
	}

	// Penyetuju tanpa 2FA aktif tidak bisa menyetujui; pengguna yang bekerja tidak perlu 2FA.
	pb, err := e.propose(t, e.cashier, "propose_petty_cash_entry", `{"direction":"out","amount":"5000","category":"Makan"}`)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.mfa = fakeMFA{off: map[uuid.UUID]bool{e.approver.UserID: true}}
	if _, err := e.svc.ConfirmAction(ctx, e.cashier, pb.ID, e.good(), totpOK); !errors.Is(err, ErrMFANotEnabled) {
		t.Fatalf("penyetuju tanpa 2FA harus ditolak: %v", err)
	}
	if rows() != 1 {
		t.Fatalf("tidak boleh menulis, got %d", rows())
	}
}
