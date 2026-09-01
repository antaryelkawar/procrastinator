package entity

import "testing"

func TestValidStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", "valid", true},
		{"duplicate", "duplicate", true},
		{"possible-duplicate", "possible-duplicate", true},
		{"error", "error", true},
		{"empty", "", false},
		{"pending", "pending", false},
		{"VALID-capitalized", "VALID", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidStatus(tc.in)
			if got != tc.want {
				t.Fatalf("ValidStatus(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestStatusConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"LineStatusValid", LineStatusValid, "valid"},
		{"LineStatusDuplicate", LineStatusDuplicate, "duplicate"},
		{"LineStatusPossibleDuplicate", LineStatusPossibleDuplicate, "possible-duplicate"},
		{"LineStatusError", LineStatusError, "error"},
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
