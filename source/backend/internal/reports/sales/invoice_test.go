package sales

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"INV-001": "INV-001",
		"50%":     `50\%`,
		"a_b":     `a\_b`,
		`x\y`:     `x\\y`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
