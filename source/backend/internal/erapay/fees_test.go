package erapay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestMerchantFees(t *testing.T) {
	tenant := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/integrations/arus/tenants/"+tenant.String()+"/fees" || r.Header.Get("Authorization") != "Bearer kt" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"mdr_bps":70,"mdr_fixed":100,"mdr_flat_up_to":100000,"mdr_flat_fee":500}`))
	}))
	defer srv.Close()
	c := NewProvisioningClient(srv.URL, "pt")
	c.keyToken = "kt"
	f, found, err := c.MerchantFees(context.Background(), tenant)
	if err != nil || !found || f.MDRBPS != 70 || f.MDRFixed != 100 || f.MDRFlatUpTo != 100000 || f.MDRFlatFee != 500 {
		t.Fatalf("fees: %+v found=%v err=%v", f, found, err)
	}
	if _, found, err := c.MerchantFees(context.Background(), uuid.New()); err != nil || found {
		t.Fatalf("tenant tak dikenal harus found=false tanpa galat: found=%v err=%v", found, err)
	}
}
