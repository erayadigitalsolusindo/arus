// Package erapay: pembayaran QRIS lewat EraPay (EraPay meneruskan ke DOKU). ARUS hanya bicara ke API EraPay dengan
// API key (Bearer); nota baru boleh memakai pembayaran QRIS bila charge-nya sudah lunas di EraPay (lihat Verify).
package erapay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client = klien HTTP tipis untuk API EraPay (POST /v1/payments, GET /v1/payments/{id}).
type Client struct {
	base, key string
	http      *http.Client
}

// NewClient mengembalikan nil bila baseURL atau key kosong (integrasi mati).
func NewClient(baseURL, apiKey string) *Client {
	baseURL, apiKey = strings.TrimRight(strings.TrimSpace(baseURL), "/"), strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return nil
	}
	return &Client{base: baseURL, key: apiKey, http: &http.Client{Timeout: 20 * time.Second}}
}

// Payment = subset respons EraPay yang dipakai ARUS.
type Payment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	QRCode *struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"qr_code"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type createReq struct {
	MerchantReference string `json:"merchant_reference"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	PaymentMethod     string `json:"payment_method"`
	Description       string `json:"description,omitempty"`
	OutletID          string `json:"outlet_id,omitempty"`
	ExpiresAt         string `json:"expires_at,omitempty"`
}

// ErrRejected = EraPay/gateway menolak tagihan (4xx); pesan aman untuk ditampilkan.
var ErrRejected = errors.New("EraPay menolak tagihan QRIS")

// Create membuat tagihan QRIS. idemKey dikirim sebagai Idempotency-Key: percobaan ulang tidak membuat tagihan ganda.
func (c *Client) Create(ctx context.Context, idemKey string, r createReq) (Payment, error) {
	body, _ := json.Marshal(r)
	return c.do(ctx, http.MethodPost, "/v1/payments", idemKey, body)
}

// Get membaca status tagihan.
func (c *Client) Get(ctx context.Context, paymentID string) (Payment, error) {
	return c.do(ctx, http.MethodGet, "/v1/payments/"+paymentID, "", nil)
}

func (c *Client) do(ctx context.Context, method, path, idemKey string, body []byte) (Payment, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return Payment{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Payment{}, fmt.Errorf("erapay: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// 201/200 = berhasil; 202 = status belum pasti (badan berisi pembayaran, cek ulang lewat Get); 422 dengan "payment" = ditolak gateway.
	var env struct {
		Payment
		Inner *Payment `json:"payment"`
	}
	_ = json.Unmarshal(raw, &env)
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted:
		if env.ID == "" {
			return Payment{}, fmt.Errorf("erapay: respons tanpa id (HTTP %d)", resp.StatusCode)
		}
		return env.Payment, nil
	case resp.StatusCode == http.StatusUnprocessableEntity, resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusForbidden:
		return Payment{}, ErrRejected
	case resp.StatusCode == http.StatusNotFound && method == http.MethodGet:
		return Payment{}, errNotFound
	}
	return Payment{}, fmt.Errorf("erapay: HTTP %d", resp.StatusCode)
}

var errNotFound = errors.New("erapay: pembayaran tidak ditemukan")
