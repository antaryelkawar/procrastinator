package commons

import "testing"

func TestIsValidAmount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", "19999.99", true},
		{"valid-whole", "5", true},
		{"valid-decimal", "12.5", true},
		{"valid-small", "0.01", true},
		{"valid-hundred", "100", true},
		{"invalid-empty", "", false},
		{"invalid-zero", "0", false},
		{"invalid-negative", "-5", false},
		{"invalid-double-dot", "12.5.3", false},
		{"invalid-exponent", "1e3", false},
		{"invalid-float-repr", "1.2e3", false},
		{"invalid-zero-decimal", "0.0", false},
		{"invalid-trailing-dot", "5.", false},
		{"invalid-leading-dot", ".5", false},
		{"invalid-space", " ", false},
		{"invalid-multi-dot", "1.2.3.4", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsValidAmount(tc.in)
			if got != tc.want {
				t.Fatalf("IsValidAmount(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsValidCurrency(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid-inr", "INR", true},
		{"valid-lower", "usd", true},
		{"invalid-short", "IN", false},
		{"invalid-digit", "INR1", false},
		{"invalid-empty", "", false},
		{"invalid-long", "INRR", false},
		{"invalid-space", "INR X", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsValidCurrency(tc.in)
			if got != tc.want {
				t.Fatalf("IsValidCurrency(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeDescription(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"mixed-cases", "  Reliance   Digital ", "reliance digital"},
		{"empty", "", ""},
		{"whitespace-only", "   ", ""},
		{"upper-multiple", "SAMSUNG  GW990", "samsung gw990"},
		{"already-normalized", "already normalized", "already normalized"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeDescription(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeDescription(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
