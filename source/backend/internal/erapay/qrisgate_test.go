package erapay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQRISGateClient(t *testing.T) {
	status := "PENDING"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qg_live_goodkey1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/transactions":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["external_order_id"] != "ord-1" || b["amount"] != float64(15000) || b["payment_method"] != "QRIS" {
				t.Errorf("body = %v", b)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"transaction_id":"tx-1","status":"PENDING","qr_string":"000201...","expires_at":"2030-01-01T00:00:00Z"}}`))
		case r.URL.Path == "/api/v1/transactions/tx-1":
			_, _ = w.Write([]byte(`{"transaction_id":"tx-1","status":"` + status + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c := newQRISGateClient(srv.URL, "qg_live_goodkey1")

	p, err := c.Create(ctx, "", createReq{MerchantReference: "ord-1", Amount: 15000})
	if err != nil || p.ID != "tx-1" || p.Status != StatusPending || p.QRCode == nil || p.QRCode.Value != "000201..." || p.ExpiresAt == nil || p.Gateway != ProviderQRISGate {
		t.Fatalf("create = %+v, %v", p, err)
	}
	status = "PAID"
	if p, err = c.Get(ctx, "tx-1"); err != nil || p.Status != StatusSucceeded { // badan polos tanpa pembungkus data juga diterima
		t.Fatalf("get = %+v, %v", p, err)
	}
	if _, err = c.Get(ctx, "nope"); !errors.Is(err, errNotFound) {
		t.Fatalf("get missing err = %v", err)
	}
	if err = c.check(ctx); err != nil { // 404 pada id palsu = key diterima
		t.Fatalf("check good key: %v", err)
	}
	if err = newQRISGateClient(srv.URL, "qg_live_wrongkey").check(ctx); !errors.Is(err, ErrKeyInvalid) {
		t.Fatalf("check bad key err = %v", err)
	}
}

func TestQRISGateStatusMapping(t *testing.T) {
	for in, want := range map[string]string{"PAID": StatusSucceeded, "waived": StatusSucceeded, "PENDING": StatusPending, "EXPIRED": "expired", "CANCELLED": "cancelled", "FAILED": "failed", "???": "unknown"} {
		if got := qgStatus(in); got != want {
			t.Errorf("qgStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
