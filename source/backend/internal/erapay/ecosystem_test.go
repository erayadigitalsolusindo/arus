package erapay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestProvisioningClientSSO(t *testing.T) {
	tenantID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/integrations/arus/tenants/"+tenantID.String()+"/sso" || r.Header.Get("Authorization") != "Bearer server-token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"url":"https://pay.test/sso?code=abc","expires_in":60}`))
	}))
	defer server.Close()

	client := NewProvisioningClient(server.URL, "server-token")
	if url, err := client.SSO(context.Background(), tenantID); err != nil || url != "https://pay.test/sso?code=abc" {
		t.Fatalf("sso = %q, err = %v", url, err)
	}
	if _, err := client.SSO(context.Background(), uuid.New()); !errors.Is(err, ErrEcosystemSSO) {
		t.Fatalf("unlinked tenant error = %v, want ErrEcosystemSSO", err)
	}
}

func TestProvisioningClientIssueKey(t *testing.T) {
	tenant := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	status, body := http.StatusCreated, `{"secret":"epk_prd_aaaaaaaa_x"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/integrations/arus/tenants/"+tenant.String()+"/api-key" || r.Header.Get("Authorization") != "Bearer key-token" {
			t.Errorf("path/token salah: %s %q", r.URL.Path, r.Header.Get("Authorization")) // harus token key, bukan provisioning
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewProvisioningClient(srv.URL, "provision-token")
	if _, err := c.IssueKey(t.Context(), tenant); !errors.Is(err, ErrEcosystemNotConfigured) {
		t.Fatalf("tanpa token key harus mati: %v", err)
	}
	c.keyToken = "key-token"
	if got, err := c.IssueKey(t.Context(), tenant); err != nil || got != "epk_prd_aaaaaaaa_x" {
		t.Fatalf("issue: %q %v", got, err)
	}
	for code, want := range map[string]error{"MERCHANT_NOT_APPROVED": ErrMerchantNotApproved, "API_KEY_EXISTS": ErrKeyExists} {
		status, body = http.StatusConflict, `{"error":{"code":"`+code+`"}}`
		if _, err := c.IssueKey(t.Context(), tenant); !errors.Is(err, want) {
			t.Fatalf("%s: %v", code, err)
		}
	}
	status, body = http.StatusNotFound, `{}`
	if _, err := c.IssueKey(t.Context(), tenant); !errors.Is(err, ErrEcosystemSSO) {
		t.Fatalf("404: %v", err)
	}
}

func TestProvisioningClientApprove(t *testing.T) {
	tenant := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	status, body := http.StatusOK, `{}`
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sent = string(b)
		if r.URL.Path != "/v1/integrations/arus/tenants/"+tenant.String()+"/approve" || r.Header.Get("Authorization") != "Bearer key-token" {
			t.Errorf("path/token salah: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewProvisioningClient(srv.URL, "provision-token")
	fees := Fees{MDRBPS: 70, PlatformBPS: 30, MDRFlatUpTo: 100000, MDRFlatFee: 500}
	if err := c.Approve(t.Context(), tenant, fees, "A"); !errors.Is(err, ErrEcosystemNotConfigured) {
		t.Fatalf("tanpa token key harus mati: %v", err)
	}
	c.keyToken = "key-token"
	if err := c.Approve(t.Context(), tenant, fees, "A"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, `"mdr_flat_fee":500`) || !strings.Contains(sent, `"admin":"A"`) {
		t.Fatalf("body: %s", sent)
	}
	for code, want := range map[string]error{"INVALID_STATE": ErrMerchantState, "VALIDATION_ERROR": ErrBadFees} {
		status, body = http.StatusConflict, `{"error":{"code":"`+code+`"}}`
		if err := c.Approve(t.Context(), tenant, fees, "A"); !errors.Is(err, want) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}
