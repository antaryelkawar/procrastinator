package data

import "testing"

func TestCollapseWhitespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace-only", "   ", ""},
		{"mixed", "  a  b \t c ", "a b c"},
		{"single", "single", "single"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CollapseWhitespace(tc.in)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeSerial(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace-only", " \t\n ", ""},
		{"mixed-case", "  sn-123  abc ", "SN-123 ABC"},
		{"tabs-newlines", "s n \t\n  x", "S N X"},
		{"already-normalized", "SN-123 ABC", "SN-123 ABC"},
		{"lowercase", "abc", "ABC"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeSerial(tc.in)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace-only", "  \t ", ""},
		{"samsung", "  Samsung ", "samsung"},
		{"model", "  WW90   T534DAW ", "ww90 t534daw"},
		{"already-lower", "samsung", "samsung"},
		{"uppercase", "ABC", "abc"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeName(tc.in)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNormalizeSerial_EquivalenceAcrossFormatting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{"spaces-vs-clean", "  sn-123  ABC ", "SN-123 abc", "SN-123 ABC"},
		{"tabs-newlines-vs-lower", "\tSN-123\n  ABC", "sn-123 abc", "SN-123 ABC"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotA := NormalizeSerial(tc.a)
			gotB := NormalizeSerial(tc.b)

			if gotA != tc.want {
				t.Fatalf("NormalizeSerial(%q) = %q, want %q", tc.a, gotA, tc.want)
			}
			if gotB != tc.want {
				t.Fatalf("NormalizeSerial(%q) = %q, want %q", tc.b, gotB, tc.want)
			}
			if gotA != gotB {
				t.Fatalf("NormalizeSerial(%q) = %q != NormalizeSerial(%q) = %q", tc.a, gotA, tc.b, gotB)
			}
		})
	}
}
