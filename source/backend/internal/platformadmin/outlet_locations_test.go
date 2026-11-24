package platformadmin

import (
	"strings"
	"testing"
)

func TestLocFilterWhere(t *testing.T) {
	w, args := locFilter{}.where()
	if w != "TRUE" || len(args) != 0 {
		t.Fatalf("kosong: %q %v", w, args)
	}
	w, args = locFilter{q: "a_b%", status: "inactive", loc: "missing"}.where()
	if !strings.Contains(w, "NOT l.active") || !strings.Contains(w, "l.latitude IS NULL") || !strings.Contains(w, "$2") {
		t.Fatalf("where: %q", w)
	}
	if len(args) != 1 || args[0] != `%a\_b\%%` {
		t.Fatalf("args (wildcard harus di-escape): %v", args)
	}
}
