package erapay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestProvisioningClientStatusAndEnsure(t *testing.T) {
	tenantID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	merchantID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer server-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/integrations/arus/tenants/11111111-1111-4111-8111-111111111111":
			w.WriteHeader(http.StatusNotFound)
		case "POST /v1/integrations/arus/tenants":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["tenant_id"] != tenantID.String() || body["accepted_terms"] != true {
				t.Fatalf("request body = %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(remoteTenantLink{
				Linked: true, TenantID: tenantID, MerchantID: merchantID,
				MerchantStatus: "submitted", OwnerRole: "merchant_owner", Created: true,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewProvisioningClient(server.URL, "server-token")
	status, err := client.Status(context.Background(), tenantID)
	if err != nil || status.Linked {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	link, err := client.Ensure(context.Background(), tenantID, "Toko Contoh", "Pemilik Toko", "owner@example.test", "+628123456789")
	if err != nil || !link.Linked || link.OwnerRole != "merchant_owner" || !link.Created {
		t.Fatalf("ensure = %#v, err = %v", link, err)
	}
}

func TestProvisioningClientReportsAmbiguousLegacyMerchant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"MERCHANT_LINK_AMBIGUOUS"}}`))
	}))
	defer server.Close()

	client := NewProvisioningClient(server.URL, "server-token")
	_, err := client.Ensure(context.Background(), uuid.New(), "Toko Contoh", "Pemilik Toko", "owner@example.test", "")
	if !errors.Is(err, ErrEcosystemAmbiguous) {
		t.Fatalf("error = %v, want ambiguous merchant error", err)
	}
}
