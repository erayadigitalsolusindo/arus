package member

import (
	"encoding/json"
	"testing"
)

// Form mengirim "" untuk isian angka yang dikosongkan; itu tidak boleh membuat permintaan ditolak (json.Number menolaknya).
func TestRequestAcceptsEmptyAndNumericForms(t *testing.T) {
	var r request
	if err := json.Unmarshal([]byte(`{"name":"A","credit_limit":"","due_days":""}`), &r); err != nil {
		t.Fatalf("string kosong ditolak: %v", err)
	}
	if in := r.input(); in.CreditLimit != "" || in.DueDays != "" {
		t.Errorf("input = %+v", in)
	}
	if err := json.Unmarshal([]byte(`{"credit_limit":1500000.5,"due_days":14}`), &r); err != nil {
		t.Fatal(err)
	}
	if in := r.input(); in.CreditLimit != "1500000.5" || in.DueDays != "14" {
		t.Errorf("angka: %+v", in)
	}
	if err := json.Unmarshal([]byte(`{"credit_limit":"250.00","due_days":null}`), &r); err != nil {
		t.Fatal(err)
	}
	if in := r.input(); in.CreditLimit != "250.00" || in.DueDays != "" {
		t.Errorf("string/null: %+v", in)
	}
	var l levelRequest
	if err := json.Unmarshal([]byte(`{"name":"X","min_points":"10","spend_per_point":"","point_value":""}`), &l); err != nil {
		t.Fatalf("level dengan isian kosong ditolak: %v", err)
	}
}
