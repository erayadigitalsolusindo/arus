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

// CreateQRIS membuat tagihan QRIS sederhana (dipakai penagihan paket langganan). ref = merchant_reference; idemKey = Idempotency-Key.
func (c *Client) CreateQRIS(ctx context.Context, idemKey, ref string, amount int64, description string) (Payment, error) {
	return c.Create(ctx, idemKey, createReq{MerchantReference: ref, Amount: amount, Currency: "IDR", PaymentMethod: "QRIS", Description: description})
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

// ErrKeyInvalid = EraPay menolak API key (tidak dikenal, dicabut, atau kedaluwarsa).
var ErrKeyInvalid = errors.New("API key EraPay tidak valid")

// KeyInfo = identitas API key menurut EraPay (GET /v1/api-keys/self).
type KeyInfo struct {
	Environment string   `json:"environment"` // "sandbox" | "production"
	Scopes      []string `json:"scopes"`
}

// Self memeriksa key: dikenal, aktif, dan scope-nya.
func (c *Client) Self(ctx context.Context) (KeyInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/api-keys/self", nil)
	if err != nil {
		return KeyInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return KeyInfo{}, fmt.Errorf("erapay: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var k KeyInfo
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&k); err != nil {
			return KeyInfo{}, fmt.Errorf("erapay: respons tidak valid")
		}
		return k, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return KeyInfo{}, ErrKeyInvalid
	}
	return KeyInfo{}, fmt.Errorf("erapay: HTTP %d", resp.StatusCode)
}
