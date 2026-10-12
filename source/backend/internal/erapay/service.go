package erapay

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"aciraba/internal/authz"
	"aciraba/internal/paymentmethod"
	"aciraba/internal/platform/db"
)

// MethodName = nama metode pembayaran yang dibuat saat key disimpan.
const MethodName = "EraPay QRIS"

const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"

	maxAmount = 100_000_000 // batas wajar satu tagihan QRIS (Rp)
)

var (
	ErrDisabled       = errors.New("integrasi EraPay belum diaktifkan di server")
	ErrNotConfigured  = errors.New("API key EraPay belum dipasang untuk toko ini")
	ErrNotFound       = errors.New("tagihan tidak ditemukan")
	ErrAmount         = errors.New("nominal QRIS harus rupiah bulat dan di atas nol")
	ErrKeyConflict    = errors.New("kunci idempotensi sudah dipakai untuk nominal lain")
	ErrChargeNotOK    = errors.New("pembayaran EraPay belum lunas, tidak cocok, atau sudah dipakai nota lain")
	ErrUnavailable    = errors.New("EraPay tidak dapat dihubungi, coba lagi")
	ErrBadKey         = errors.New("format API key EraPay tidak valid")
	ErrKeyScope       = errors.New("API key EraPay harus punya izin payment:create dan payment:read")
	ErrKeyEnvironment = errors.New("API key sandbox tidak boleh dipakai di server production")
	ErrImpersonating  = errors.New("tidak tersedia pada sesi masuk-sebagai")
	keyFormat         = regexp.MustCompile(`^epk_(prd|sbx)_[0-9a-f]{8}_[0-9a-f]{64}$`)
)

// Service: tiap tenant memakai API key EraPay-nya sendiri (tersimpan terenkripsi). Alamat API EraPay sama untuk semua (env server).
type Service struct {
	pool        *pgxpool.Pool
	baseURL     string // kosong = integrasi mati
	box         *secretBox
	methods     *paymentmethod.Service
	requireProd bool // di luar development: hanya key production yang diterima
}

func NewService(pool *pgxpool.Pool, baseURL, secret string, requireProd bool, methods *paymentmethod.Service) (*Service, error) {
	box, err := newSecretBox(secret)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), box: box, methods: methods, requireProd: requireProd}, nil
}

// Charge = bentuk respons API ke kasir.
type Charge struct {
	ID        uuid.UUID  `json:"id"`
	Status    string     `json:"status"`
	Amount    string     `json:"amount"`
	QRValue   string     `json:"qr_value,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type chargeRow struct {
	Charge
	methodID  uuid.UUID
	paymentID string
	saleID    *uuid.UUID
	amountDec decimal.Decimal
}

const chargeCols = `id, method_id, status, amount, coalesce(erapay_payment_id, ''), coalesce(qr_value, ''), expires_at, sale_id`

func scanCharge(row pgx.Row) (chargeRow, error) {
	var c chargeRow
	if err := row.Scan(&c.ID, &c.methodID, &c.Status, &c.amountDec, &c.paymentID, &c.QRValue, &c.ExpiresAt, &c.saleID); err != nil {
		return chargeRow{}, err
	}
	c.Amount = c.amountDec.StringFixed(2)
	return c, nil
}

// clientTx membangun klien dari key tenant (nil bila belum dipasang / integrasi mati).
func (s *Service) clientTx(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (*Client, error) {
	if s.baseURL == "" {
		return nil, nil
	}
	var enc string
	if err := tx.QueryRow(ctx, `SELECT api_key_enc FROM erapay_settings WHERE tenant_id = $1`, tenant).Scan(&enc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	key, err := s.box.open(enc, tenant.String())
	if err != nil {
		return nil, err
	}
	return NewClient(s.baseURL, key), nil
}

func (s *Service) client(ctx context.Context, tenant uuid.UUID) (c *Client, err error) {
	err = db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		c, err = s.clientTx(ctx, tx, tenant)
		return err
	})
	return c, err
}

// Config = status integrasi untuk kasir dan halaman pengaturan.
type Config struct {
	Enabled     bool       `json:"enabled"`               // kasir bisa memakai QRIS (server siap + key terpasang + metode aktif)
	MethodID    *uuid.UUID `json:"method_id,omitempty"`   // metode "EraPay QRIS" milik tenant
	ServerReady bool       `json:"server_ready"`          // alamat EraPay terpasang di server
	Configured  bool       `json:"configured"`            // tenant sudah menyimpan API key
	KeyHint     string     `json:"key_hint,omitempty"`    // 4 karakter terakhir key
	Environment string     `json:"environment,omitempty"` // "sandbox" | "production"
}

func (s *Service) Config(ctx context.Context, a authz.Actor) (Config, error) {
	cfg := Config{ServerReady: s.baseURL != ""}
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT key_hint, environment FROM erapay_settings WHERE tenant_id = $1`, a.TenantID).Scan(&cfg.KeyHint, &cfg.Environment)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		cfg.Configured = err == nil
		var id uuid.UUID
		err = tx.QueryRow(ctx, `SELECT m.method_id FROM erapay_methods m JOIN payment_methods p ON p.id = m.method_id AND p.active WHERE m.tenant_id = $1`, a.TenantID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		cfg.MethodID = &id
		cfg.Enabled = cfg.ServerReady && cfg.Configured
		return nil
	})
	return cfg, err
}

// SaveKey memverifikasi API key ke EraPay (dikenal, aktif, punya izin payment:create + payment:read, lingkungan sesuai), menyimpannya
// terenkripsi, lalu menyiapkan metode "EraPay QRIS" bila belum ada. Mengganti key lama bila sudah ada.
func (s *Service) SaveKey(ctx context.Context, a authz.Actor, apiKey string) (Config, error) {
	if a.Impersonator != uuid.Nil {
		return Config{}, ErrImpersonating
	}
	if s.baseURL == "" {
		return Config{}, ErrDisabled
	}
	apiKey = strings.TrimSpace(apiKey)
	if !keyFormat.MatchString(apiKey) {
		return Config{}, ErrBadKey
	}
	info, err := NewClient(s.baseURL, apiKey).Self(ctx)
	switch {
	case errors.Is(err, ErrKeyInvalid):
		return Config{}, err
	case err != nil:
		return Config{}, ErrUnavailable
	case !slices.Contains(info.Scopes, "payment:create") || !slices.Contains(info.Scopes, "payment:read"):
		return Config{}, ErrKeyScope
	case s.requireProd && info.Environment != "production":
		return Config{}, ErrKeyEnvironment
	}
	enc, err := s.box.seal(apiKey, a.TenantID.String())
	if err != nil {
		return Config{}, err
	}
	err = db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO erapay_settings (tenant_id, api_key_enc, key_hint, environment, updated_by) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant_id) DO UPDATE SET api_key_enc = EXCLUDED.api_key_enc, key_hint = EXCLUDED.key_hint,
			environment = EXCLUDED.environment, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			a.TenantID, enc, apiKey[len(apiKey)-4:], info.Environment, a.UserID)
		return err
	})
	if err != nil {
		return Config{}, err
	}
	if err := s.ensureMethod(ctx, a); err != nil {
		return Config{}, err
	}
	return s.Config(ctx, a)
}

// RemoveKey menghapus key tenant: kasir tak lagi bisa membuat QR baru. Metode dan charge lama tetap ada (menempel di nota lama).
func (s *Service) RemoveKey(ctx context.Context, a authz.Actor) error {
	if a.Impersonator != uuid.Nil {
		return ErrImpersonating
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM erapay_settings WHERE tenant_id = $1`, a.TenantID)
		return err
	})
}

// ensureMethod membuat metode pembayaran "EraPay QRIS" (e-wallet, tanpa biaya) lalu menandainya. Idempotent.
func (s *Service) ensureMethod(ctx context.Context, a authz.Actor) error {
	var have bool
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM erapay_methods m JOIN payment_methods p ON p.id = m.method_id AND p.active WHERE m.tenant_id = $1)`, a.TenantID).Scan(&have)
	})
	if err != nil || have {
		return err
	}
	m, err := s.methods.Create(ctx, a, paymentmethod.Input{Name: MethodName, Kind: paymentmethod.KindEWallet})
	if errors.Is(err, paymentmethod.ErrNameTaken) { // sudah ada (dibuat manual atau diarsipkan): pakai yang itu
		list, _, lerr := s.methods.List(ctx, a, paymentmethod.ListParams{Q: MethodName, Limit: 100})
		if lerr != nil {
			return lerr
		}
		for i := range list {
			if list[i].Name == MethodName && !paymentmethod.IsInternal(list[i].Kind) && list[i].Kind != paymentmethod.KindCash { // jenis boleh sudah diganti owner (mis. Debit)
				m, err = &list[i], nil
				if !list[i].Active {
					m, err = s.methods.SetActive(ctx, a, list[i].ID, true)
				}
				break
			}
		}
	}
	if err != nil {
		return err
	}
	return db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO erapay_methods (tenant_id, method_id) VALUES ($1, $2) ON CONFLICT (tenant_id) DO UPDATE SET method_id = EXCLUDED.method_id`, a.TenantID, m.ID)
		return err
	})
}

// CreateCharge membuat (atau mengembalikan, bila kunci sama) tagihan QRIS. key dibuat kasir per percobaan bayar.
func (s *Service) CreateCharge(ctx context.Context, a authz.Actor, key string, amount decimal.Decimal) (Charge, error) {
	if s.baseURL == "" {
		return Charge{}, ErrDisabled
	}
	if !amount.IsPositive() || !amount.Equal(amount.Truncate(0)) || amount.GreaterThan(decimal.NewFromInt(maxAmount)) {
		return Charge{}, ErrAmount
	}
	var c chargeRow
	var cl *Client
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		if cl, err = s.clientTx(ctx, tx, a.TenantID); err != nil {
			return err
		}
		if cl == nil {
			return ErrNotConfigured
		}
		var mid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT method_id FROM erapay_methods WHERE tenant_id = $1`, a.TenantID).Scan(&mid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotConfigured
			}
			return err
		}
		var outlet *uuid.UUID
		if a.OutletID != uuid.Nil {
			outlet = &a.OutletID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO erapay_charges (tenant_id, outlet_id, method_id, idempotency_key, amount, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`, a.TenantID, outlet, mid, key, amount, a.UserID); err != nil {
			return err
		}
		c, err = scanCharge(tx.QueryRow(ctx, `SELECT `+chargeCols+` FROM erapay_charges WHERE tenant_id = $1 AND idempotency_key = $2`, a.TenantID, key))
		return err
	})
	if err != nil {
		return Charge{}, err
	}
	if !c.amountDec.Equal(amount) {
		return Charge{}, ErrKeyConflict
	}
	if c.paymentID == "" { // belum pernah sampai ke EraPay (atau gagal sebelumnya): kirim; Idempotency-Key = id charge, aman diulang
		p, err := cl.Create(ctx, c.ID.String(), createReq{
			MerchantReference: c.ID.String(), Amount: amount.IntPart(), Currency: "IDR", PaymentMethod: "QRIS", Description: "Kasir ARUS",
		})
		if errors.Is(err, ErrRejected) {
			return Charge{}, err
		}
		if err != nil {
			return Charge{}, ErrUnavailable
		}
		if c, err = s.store(ctx, a.TenantID, c.ID, p); err != nil {
			return Charge{}, err
		}
	}
	return c.Charge, nil
}

// store menyimpan hasil EraPay ke charge dan mengembalikan barisnya.
func (s *Service) store(ctx context.Context, tenant, id uuid.UUID, p Payment) (chargeRow, error) {
	var c chargeRow
	err := db.WithTenant(ctx, s.pool, tenant, func(tx pgx.Tx) error {
		var err error
		c, err = storeTx(ctx, tx, tenant, id, p)
		return err
	})
	return c, err
}

func storeTx(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, p Payment) (chargeRow, error) {
	var qr *string
	if p.QRCode != nil && p.QRCode.Value != "" {
		qr = &p.QRCode.Value
	}
	// Status akhir (lunas/gagal/kedaluwarsa) tidak diturunkan lagi ke pending.
	return scanCharge(tx.QueryRow(ctx, `UPDATE erapay_charges SET
			erapay_payment_id = coalesce(erapay_payment_id, $3),
			status = CASE WHEN status IN ('succeeded','failed','expired','cancelled') THEN status ELSE $4 END,
			qr_value = coalesce($5, qr_value), expires_at = coalesce($6, expires_at), updated_at = now()
		WHERE id = $1 AND tenant_id = $2 RETURNING `+chargeCols, id, tenant, p.ID, normStatus(p.Status), qr, p.ExpiresAt))
}

func normStatus(s string) string {
	switch s {
	case "succeeded", "failed", "expired", "cancelled", "pending":
		return s
	case "created":
		return StatusPending
	}
	return "unknown"
}

// GetCharge membaca tagihan; bila masih menunggu, status dimutakhirkan dari EraPay (memakai key tenant).
func (s *Service) GetCharge(ctx context.Context, a authz.Actor, id uuid.UUID) (Charge, error) {
	var c chargeRow
	err := db.WithTenant(ctx, s.pool, a.TenantID, func(tx pgx.Tx) error {
		var err error
		c, err = scanCharge(tx.QueryRow(ctx, `SELECT `+chargeCols+` FROM erapay_charges WHERE id = $1 AND tenant_id = $2`, id, a.TenantID))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Charge{}, ErrNotFound
	}
	if err != nil {
		return Charge{}, err
	}
	if c.paymentID != "" && (c.Status == StatusPending || c.Status == "unknown") {
		cl, err := s.client(ctx, a.TenantID)
		if err != nil {
			return Charge{}, err
		}
		if cl != nil {
			if p, gerr := cl.Get(ctx, c.paymentID); gerr == nil {
				if c, err = s.store(ctx, a.TenantID, c.ID, p); err != nil {
					return Charge{}, err
				}
			} // gagal menghubungi EraPay: tampilkan status terakhir yang tersimpan, kasir tetap bisa polling lagi
		}
	}
	return c.Charge, nil
}

// ---- Dipakai penyimpanan nota (sales) ----

// VerifyPayment memastikan pembayaran berjenis EraPay QRIS di nota benar-benar lunas. isEra=false bila methodID bukan metode EraPay
// milik tenant (tak ada yang diperiksa). Bila isEra, ref harus id charge yang LUNAS, nominal sama, dan belum dipakai nota lain
// (reuseSale = nota yang sedang direvisi; boleh memakai ulang charge miliknya). Baris charge dikunci sampai transaksi selesai.
func (s *Service) VerifyPayment(ctx context.Context, tx pgx.Tx, tenant, methodID uuid.UUID, ref string, amount decimal.Decimal, reuseSale *uuid.UUID) (chargeID uuid.UUID, isEra bool, err error) {
	var mid uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT method_id FROM erapay_methods WHERE tenant_id = $1`, tenant).Scan(&mid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	if mid != methodID {
		return uuid.Nil, false, nil
	}
	id, perr := uuid.Parse(ref)
	if perr != nil {
		return uuid.Nil, true, ErrChargeNotOK
	}
	c, err := scanCharge(tx.QueryRow(ctx, `SELECT `+chargeCols+` FROM erapay_charges WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, true, ErrChargeNotOK
	}
	if err != nil {
		return uuid.Nil, true, err
	}
	if c.Status != StatusSucceeded && c.paymentID != "" { // mungkin sudah lunas tapi belum sempat di-poll
		cl, cerr := s.clientTx(ctx, tx, tenant)
		if cerr != nil {
			return uuid.Nil, true, cerr
		}
		if cl != nil {
			if p, gerr := cl.Get(ctx, c.paymentID); gerr == nil {
				if c, err = storeTx(ctx, tx, tenant, c.ID, p); err != nil {
					return uuid.Nil, true, err
				}
			}
		}
	}
	if c.Status != StatusSucceeded || !c.amountDec.Equal(amount) || c.methodID != methodID ||
		(c.saleID != nil && (reuseSale == nil || *c.saleID != *reuseSale)) {
		return uuid.Nil, true, ErrChargeNotOK
	}
	return c.ID, true, nil
}

// LinkPayment menandai charge terpakai oleh nota saleID (dipanggil setelah nota disimpan, di transaksi yang sama).
func (s *Service) LinkPayment(ctx context.Context, tx pgx.Tx, tenant, chargeID, saleID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE erapay_charges SET sale_id = $3, updated_at = now() WHERE id = $1 AND tenant_id = $2`, chargeID, tenant, saleID)
	return err
}
