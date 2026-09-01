package entity

import "testing"

func TestValidKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"expense", "expense", true},
		{"income", "income", true},
		{"transfer", "transfer", true},
		{"debit", "debit", false},
		{"credit", "credit", false},
		{"empty", "", false},
		{"Transfer-capitalized", "Transfer", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidKind(tc.in)
			if got != tc.want {
				t.Fatalf("ValidKind(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestKindConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"KindExpense", KindExpense, "expense"},
		{"KindIncome", KindIncome, "income"},
		{"KindTransfer", KindTransfer, "transfer"},
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

func TestOriginConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"OriginManual", OriginManual, "manual"},
		{"OriginImport", OriginImport, "import"},
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

func TestLinkCreatorConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"LinkCreatorManual", LinkCreatorManual, "manual"},
		{"LinkCreatorAuto", LinkCreatorAuto, "auto"},
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
