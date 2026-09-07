package entity

import "testing"

func TestValidDocType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"invoice", "invoice", true},
		{"receipt", "receipt", true},
		{"warranty", "warranty", true},
		{"amc", "amc", true},
		{"statement", "statement", true},
		{"other", "other", true},
		{"empty", "", false},
		{"contract", "contract", false},
		{"Invoice-capitalized", "Invoice", false},
		{"INVOICE-uppercase", "INVOICE", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidDocType(tc.in)
			if got != tc.want {
				t.Fatalf("ValidDocType(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDocTypeConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"DocTypeInvoice", DocTypeInvoice, "invoice"},
		{"DocTypeReceipt", DocTypeReceipt, "receipt"},
		{"DocTypeWarranty", DocTypeWarranty, "warranty"},
		{"DocTypeAMC", DocTypeAMC, "amc"},
		{"DocTypeStatement", DocTypeStatement, "statement"},
		{"DocTypeOther", DocTypeOther, "other"},
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
