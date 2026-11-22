package platformadmin

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"aciraba/internal/audit"
	"aciraba/internal/erapay"
	gen "aciraba/internal/gen"
	"aciraba/internal/platform/db"
	"aciraba/internal/platform/httpx"
)

// ERAPay dipenuhi *erapay.Service.
type ERAPay interface {
	StatusForTenant(ctx context.Context, tenant uuid.UUID) (erapay.EcosystemStatus, error)
	ActivateForTenant(ctx context.Context, tenant uuid.UUID) (erapay.EcosystemStatus, error)
	SaveKeyForTenant(ctx context.Context, tenant uuid.UUID, adminName, apiKey string) (erapay.Config, error)
	IssueKeyForTenant(ctx context.Context, tenant uuid.UUID, adminName string, fees *erapay.Fees) (erapay.Config, error)
	ConfigForTenant(ctx context.Context, tenant uuid.UUID) (erapay.Config, error)
	SetPartnerPaymentForTenant(ctx context.Context, tenant uuid.UUID, on bool) (erapay.Config, error)
	EnableQRISGateForTenant(ctx context.Context, tenant uuid.UUID) error
}

func (h *Handler) erapayFail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "NOT_FOUND", "Tenant atau pemilik aktif tidak ditemukan.")
	case errors.Is(err, erapay.ErrBadKey), errors.Is(err, erapay.ErrKeyInvalid):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_INVALID", "API key ditolak EraPay (format salah, tidak dikenal, dicabut, atau kedaluwarsa).")
	case errors.Is(err, erapay.ErrKeyScope):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_SCOPE", "API key harus punya izin payment:create dan payment:read.")
	case errors.Is(err, erapay.ErrKeyEnvironment):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_KEY_SANDBOX", "API key sandbox tidak boleh dipakai di server production.")
	case errors.Is(err, erapay.ErrMerchantNotApproved):
		httpx.Error(w, http.StatusConflict, "MERCHANT_NOT_APPROVED", "Merchant belum disetujui EraPay. Key bisa dibuat setelah disetujui.")
	case errors.Is(err, erapay.ErrBadFees):
		httpx.Error(w, http.StatusUnprocessableEntity, "ERAPAY_FEES_INVALID", "Tarif tidak valid. Periksa persen dan nominal (bilangan bulat rupiah, aturan transaksi kecil diisi berpasangan).")
	case errors.Is(err, erapay.ErrMerchantState):
		httpx.Error(w, http.StatusConflict, "ERAPAY_STATE", "Merchant tidak bisa disetujui dari statusnya sekarang (mis. ditolak atau ditangguhkan). Tangani di dashboard EraPay.")
	case errors.Is(err, erapay.ErrNotConfigured):
		httpx.Error(w, http.StatusConflict, "ERAPAY_NOT_CONFIGURED", "Pasang API key EraPay tenant lebih dulu.")
	case errors.Is(err, erapay.ErrKeyExists):
		httpx.Error(w, http.StatusConflict, "API_KEY_EXISTS", "Key otomatis sudah pernah dibuat di EraPay. Cabut atau putar key itu dari dashboard EraPay, atau tempel key manual.")
	case errors.Is(err, erapay.ErrEcosystemSSO):
		httpx.Error(w, http.StatusConflict, "ERAPAY_NOT_LINKED", "Tenant belum terdaftar di EraPay.")
	case errors.Is(err, erapay.ErrDisabled):
		httpx.Error(w, http.StatusServiceUnavailable, "ERAPAY_DISABLED", "Integrasi EraPay belum diaktifkan di server.")
	case errors.Is(err, erapay.ErrEcosystemEmail):
		httpx.Error(w, http.StatusConflict, "OWNER_EMAIL_UNVERIFIED", "Email pemilik belum diverifikasi.")
	case errors.Is(err, erapay.ErrEcosystemAmbiguous):
		httpx.Error(w, http.StatusConflict, "ERAPAY_ECOSYSTEM_AMBIGUOUS", "Lebih dari satu merchant EraPay cocok. Hubungi dukungan EraPay.")
	case errors.Is(err, erapay.ErrEcosystemNotConfigured):
		httpx.Error(w, http.StatusServiceUnavailable, "ERAPAY_ECOSYSTEM_UNAVAILABLE", "Koneksi Ekosistem EraPay belum dikonfigurasi di server.")
	case errors.Is(err, erapay.ErrUnavailable):
		h.Log.Warn("erapay tidak tersedia", "err", err)
		httpx.Error(w, http.StatusBadGateway, "ERAPAY_UNAVAILABLE", "EraPay tidak dapat dihubungi, coba lagi.")
	default:
		h.internal(w, r, "erapay platform gagal", err)
	}
}

func (h *Handler) ERAPayStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	st, err := h.ERAPay.StatusForTenant(r.Context(), id)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	cfg, err := h.ERAPay.ConfigForTenant(r.Context(), id)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"active": st.Active, "merchant_status": st.MerchantStatus, "key_configured": cfg.Configured, "key_hint": cfg.KeyHint, "environment": cfg.Environment, "partner_payment": cfg.PartnerPayment})
}

// ERAPayPartner: PUT /platform/tenants/{id}/erapay/partner. Menyalakan tombol PARTNER PAYMENT di kasir tenant.
func (h *Handler) ERAPayPartner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 1024) {
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	cfg, err := h.ERAPay.SetPartnerPaymentForTenant(r.Context(), id, in.Enabled)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if err := h.svc.recordERAPay(r.Context(), actor(r), id, ActionERAPayPartner, audit.ActionPlatformERAPayPartner, map[string]any{"enabled": cfg.PartnerPayment}); err != nil {
		h.Log.Error("audit erapay gagal", "err", err)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"partner_payment": cfg.PartnerPayment})
}

func (h *Handler) ERAPayActivate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		OwnerAgreed bool `json:"owner_agreed"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 1024) {
		return
	}
	if !in.OwnerAgreed {
		httpx.ValidationError(w, map[string]string{"owner_agreed": "REQUIRED"})
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	st, err := h.ERAPay.ActivateForTenant(r.Context(), id)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if err := h.svc.recordERAPay(r.Context(), actor(r), id, ActionERAPay, audit.ActionPlatformERAPay, map[string]any{"owner_agreed": true}); err != nil {
		h.Log.Error("audit erapay gagal", "err", err) // pendaftaran sudah terjadi; jangan gagalkan respons
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) ERAPaySaveKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		APIKey   string `json:"api_key"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 4<<10) {
		return
	}
	a := actor(r)
	if !h.stepUp(w, r, a, in.Password, in.Code) {
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	cfg, err := h.ERAPay.SaveKeyForTenant(r.Context(), id, a.Name, in.APIKey)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if err := h.svc.recordERAPay(r.Context(), a, id, ActionERAPayKey, audit.ActionPlatformERAPayKey, map[string]any{"key_hint": cfg.KeyHint, "environment": cfg.Environment}); err != nil {
		h.Log.Error("audit erapay gagal", "err", err)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"key_configured": cfg.Configured, "key_hint": cfg.KeyHint, "environment": cfg.Environment})
}

// stepUp memverifikasi ulang password + TOTP baru milik admin yang sedang login (dengan pembatasan percobaan).
// false = jawaban sudah ditulis.
func (h *Handler) stepUp(w http.ResponseWriter, r *http.Request, a Actor, password, code string) bool {
	fail, done, ok := h.attempt(w, r, "platform-stepup:"+a.ID.String())
	if !ok {
		return false
	}
	if password == "" || len(password) > maxPassword {
		h.invalidCode(w, fail)
		return false
	}
	switch err := h.svc.ConfirmStepUp(r.Context(), a, password, code); {
	case errors.Is(err, ErrInvalidCode), errors.Is(err, ErrInvalidCredentials):
		h.invalidCode(w, fail)
		return false
	case err != nil:
		h.internal(w, r, "verifikasi ulang gagal", err)
		return false
	}
	done()
	return true
}

func (h *Handler) ERAPayIssueKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Password string       `json:"password"`
		Code     string       `json:"code"`
		Fees     *erapay.Fees `json:"fees"` // ada = setujui merchant dulu bila belum
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 2<<10) {
		return
	}
	a := actor(r)
	if !h.stepUp(w, r, a, in.Password, in.Code) {
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	cfg, err := h.ERAPay.IssueKeyForTenant(r.Context(), id, a.Name, in.Fees)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if err := h.svc.recordERAPay(r.Context(), a, id, ActionERAPayKey, audit.ActionPlatformERAPayKey, map[string]any{"key_hint": cfg.KeyHint, "environment": cfg.Environment, "issued": "auto", "fees": in.Fees}); err != nil {
		h.Log.Error("audit erapay gagal", "err", err)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"key_configured": cfg.Configured, "key_hint": cfg.KeyHint, "environment": cfg.Environment})
}

// ERAPayQuickstart: satu tombol untuk menyiapkan tenant ke EraPay/QRISGate: daftar, setujui dengan tarif 0 (plan
// qrisgate-direct), terbitkan key, nyalakan QRISGate. Langkah yang sudah selesai dilewati, jadi aman diulang.
// Terakhir, gateway qrisgate dinyalakan dan jalur pembayaran diarahkan ke sana (butuh EraPay yang punya endpoint enable).
func (h *Handler) ERAPayQuickstart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		OwnerAgreed bool   `json:"owner_agreed"`
		Password    string `json:"password"`
		Code        string `json:"code"`
	}
	if !httpx.DecodeJSONLimit(w, r, &in, 2<<10) {
		return
	}
	if !in.OwnerAgreed {
		httpx.ValidationError(w, map[string]string{"owner_agreed": "REQUIRED"})
		return
	}
	a := actor(r)
	if !h.stepUp(w, r, a, in.Password, in.Code) {
		return
	}
	if h.ERAPay == nil {
		h.erapayFail(w, r, erapay.ErrEcosystemNotConfigured)
		return
	}
	st, err := h.ERAPay.StatusForTenant(r.Context(), id)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if !st.Active {
		if st, err = h.ERAPay.ActivateForTenant(r.Context(), id); err != nil {
			h.erapayFail(w, r, err)
			return
		}
		if err := h.svc.recordERAPay(r.Context(), a, id, ActionERAPay, audit.ActionPlatformERAPay, map[string]any{"owner_agreed": true, "via": "quickstart"}); err != nil {
			h.Log.Error("audit erapay gagal", "err", err)
		}
	}
	cfg, err := h.ERAPay.ConfigForTenant(r.Context(), id)
	if err != nil {
		h.erapayFail(w, r, err)
		return
	}
	if !cfg.Configured {
		fees := erapay.Fees{} // semua 0 = plan qrisgate-direct
		if cfg, err = h.ERAPay.IssueKeyForTenant(r.Context(), id, a.Name, &fees); err != nil {
			h.erapayFail(w, r, err)
			return
		}
		if err := h.svc.recordERAPay(r.Context(), a, id, ActionERAPayKey, audit.ActionPlatformERAPayKey, map[string]any{"key_hint": cfg.KeyHint, "environment": cfg.Environment, "issued": "quickstart", "fees": fees}); err != nil {
			h.Log.Error("audit erapay gagal", "err", err)
		}
	}
	if err := h.ERAPay.EnableQRISGateForTenant(r.Context(), id); err != nil {
		h.erapayFail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"active": st.Active, "merchant_status": "approved", "key_configured": cfg.Configured, "key_hint": cfg.KeyHint, "environment": cfg.Environment})
}

func (s *Service) recordERAPay(ctx context.Context, a Actor, tenantID uuid.UUID, action, auditAction string, details map[string]any) error {
	return db.WithTenant(ctx, s.Pool, tenantID, func(tx pgx.Tx) error {
		t, err := gen.New(tx).PlatformTenantGet(ctx, tenantID)
		if err != nil {
			return err
		}
		if err := s.recordTx(ctx, tx, a, action, tenantID, t.Name, details); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Actor{TenantID: tenantID, Name: "Platform: " + a.Name}, audit.Entry{
			Action: auditAction, Entity: audit.EntityTenant, EntityID: tenantID.String(),
			Details: map[string]any{"admin_id": a.ID.String(), "detail": details},
		})
	})
}
