package entity

import "testing"

func TestValidScopeType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"personal", "personal", true},
		{"household", "household", true},
		{"empty", "", false},
		{"shared", "shared", false},
		{"PERSONAL-uppercase", "PERSONAL", false},
		{"Personal-capitalized", "Personal", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidScopeType(tc.in)
			if got != tc.want {
				t.Fatalf("ValidScopeType(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestScopeConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ScopePersonal", ScopePersonal, "personal"},
		{"ScopeHousehold", ScopeHousehold, "household"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("constant %s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}
