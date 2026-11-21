package erapay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// QRISGate (qrisgate.web.id): QRIS dinamis berbasis deteksi mutasi; dana masuk langsung ke rekening toko.
// shortcut: hanya polling GET status (tanpa webhook HMAC), cukup karena kasir dan rekonsiliasi sudah polling; tambah webhook bila perlu real-time.
const (
	ProviderEraPay   = "erapay"
	ProviderQRISGate = "qrisgate"

	qrisgateBaseURL = "https://qrisgate.web.id"
)

var qrisgateKeyFormat = regexp.MustCompile(`^qg_live_[A-Za-z0-9_-]{8,128}$`)

type qrisgateClient struct {
	base, key string
	http      *http.Client
}

func newQRISGateClient(baseURL, key string) *qrisgateClient {
	return &qrisgateClient{base: strings.TrimRight(baseURL, "/"), key: key, http: &http.Client{Timeout: 20 * time.Second}}
}

// qgTx = isi `data` respons QRISGate (docs belum konsisten soal pembungkus, jadi `data` maupun badan polos diterima).
type qgTx struct {
	TransactionID string     `json:"transaction_id"`
	Status        string     `json:"status"`
	QRString      string     `json:"qr_string"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

// qgStatus memetakan status QRISGate ke status ARUS. WAIVED = lunas dengan biaya digratiskan (langganan). Status tak dikenal → "unknown".
func qgStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PAID", "SUCCESS", "SUCCEEDED", "SETTLED", "WAIVED":
		return StatusSucceeded
	case "PENDING", "CREATED":
		return StatusPending
	case "EXPIRED":
		return "expired"
	case "CANCELLED", "CANCELED":
		return "cancelled"
	case "FAILED":
		return "failed"
	}
	return "unknown"
}

func (c *qrisgateClient) Create(ctx context.Context, _ string, r createReq) (Payment, error) {
	// external_order_id = id charge ARUS: QRISGate idempoten per external_order_id, aman diulang.
	body, _ := json.Marshal(map[string]any{
		"external_order_id": r.MerchantReference, "amount": r.Amount, "payment_method": "QRIS", "include_qr_image": false,
	})
	return c.do(ctx, http.MethodPost, "/api/v1/transactions", body)
}

func (c *qrisgateClient) Get(ctx context.Context, id string) (Payment, error) {
	return c.do(ctx, http.MethodGet, "/api/v1/transactions/"+id, nil)
}

func (c *qrisgateClient) raw(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("qrisgate: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, b, nil
}

func (c *qrisgateClient) do(ctx context.Context, method, path string, body []byte) (Payment, error) {
	code, raw, err := c.raw(ctx, method, path, body)
	if err != nil {
		return Payment{}, err
	}
	switch {
	case code == http.StatusOK || code == http.StatusCreated:
		var env struct {
			qgTx
			Data *qgTx `json:"data"`
		}
		_ = json.Unmarshal(raw, &env)
		t := env.qgTx
		if env.Data != nil {
			t = *env.Data
		}
		if t.TransactionID == "" {
			return Payment{}, fmt.Errorf("qrisgate: respons tanpa transaction_id (HTTP %d)", code)
		}
		p := Payment{ID: t.TransactionID, Status: qgStatus(t.Status), Gateway: ProviderQRISGate, ExpiresAt: t.ExpiresAt}
		if t.QRString != "" {
			p.QRCode = &struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			}{Type: "qris", Value: t.QRString}
		}
		return p, nil
	case code == http.StatusNotFound && method == http.MethodGet:
		return Payment{}, errNotFound
	case code >= 400 && code < 500 && code != http.StatusTooManyRequests && code != http.StatusNotFound: // saldo kredit habis, nominal ditolak, key salah
		return Payment{}, ErrRejected
	}
	return Payment{}, fmt.Errorf("qrisgate: HTTP %d", code)
}

// check memeriksa API key tanpa membuat transaksi: QRISGate tak punya endpoint "self", jadi minta transaksi yang tak ada;
// 401/403 = key salah, selain itu (404/400/422) = key diterima.
func (c *qrisgateClient) check(ctx context.Context) error {
	code, _, err := c.raw(ctx, http.MethodGet, "/api/v1/transactions/00000000-0000-0000-0000-000000000000", nil)
	switch {
	case err != nil:
		return err
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ErrKeyInvalid
	case code >= 500 || code == http.StatusTooManyRequests:
		return fmt.Errorf("qrisgate: HTTP %d", code)
	}
	return nil
}
