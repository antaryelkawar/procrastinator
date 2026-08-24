package identity

import "testing"

// TestNormalizeSerial ensures serial normalization trims leading and
// trailing whitespace, collapses internal whitespace runs to a single
// space, and uppercases the result.
func TestNormalizeSerial(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty input", in: "", want: ""},
		{name: "whitespace only", in: " \t\n ", want: ""},
		{name: "trims and uppercases", in: "  sn-123  ABC ", want: "SN-123 ABC"},
		{name: "collapses internal whitespace runs", in: "s n \t\n  x", want: "S N X"},
		{name: "already normalized is unchanged", in: "SN-123 ABC", want: "SN-123 ABC"},
		{name: "lowercase input", in: "abc", want: "ABC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeSerial(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeSerial(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeName ensures name normalization trims leading and
// trailing whitespace, collapses internal whitespace runs to a single
// space, and lowercases the result (brand/model case folding).
func TestNormalizeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty input", in: "", want: ""},
		{name: "whitespace only", in: "  \t ", want: ""},
		{name: "trims and folds case", in: "  SaMsUnG ", want: "samsung"},
		{name: "collapses internal whitespace runs", in: "  WW90   T534DAW ", want: "ww90 t534daw"},
		{name: "already normalized is unchanged", in: "samsung", want: "samsung"},
		{name: "uppercase input", in: "ABC", want: "abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeName(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeSerial_EquivalenceAcrossFormatting encodes the spec
// scenario that serial numbers match across formatting differences:
// different whitespace and casing produce the same normalized form.
func TestNormalizeSerial_EquivalenceAcrossFormatting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{
			name: "whitespace and case variants",
			a:    "  sn-123  ABC ",
			b:    "SN-123 abc",
			want: "SN-123 ABC",
		},
		{
			name: "tabs and newlines vs plain spaces",
			a:    "\tSN-123\n  ABC",
			b:    "sn-123 abc",
			want: "SN-123 ABC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotA := NormalizeSerial(tc.a)
			gotB := NormalizeSerial(tc.b)
			if gotA != gotB {
				t.Errorf("NormalizeSerial(%q) = %q != NormalizeSerial(%q) = %q", tc.a, gotA, tc.b, gotB)
			}
			if gotA != tc.want {
				t.Errorf("NormalizeSerial(%q) = %q, want %q", tc.a, gotA, tc.want)
			}
			if gotB != tc.want {
				t.Errorf("NormalizeSerial(%q) = %q, want %q", tc.b, gotB, tc.want)
			}
		})
	}
}
