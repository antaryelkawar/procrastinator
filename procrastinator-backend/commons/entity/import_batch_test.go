package entity

import "testing"

func TestValidState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"preview", "preview", true},
		{"committed", "committed", true},
		{"discarded", "discarded", true},
		{"empty", "", false},
		{"pending", "pending", false},
		{"Preview-capitalized", "Preview", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidState(tc.in)
			if got != tc.want {
				t.Fatalf("ValidState(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestStateConstantValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"BatchStatePreview", BatchStatePreview, "preview"},
		{"BatchStateCommitted", BatchStateCommitted, "committed"},
		{"BatchStateDiscarded", BatchStateDiscarded, "discarded"},
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

func TestCanTransition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"preview-to-preview", "preview", "preview", false},
		{"preview-to-committed", "preview", "committed", true},
		{"preview-to-discarded", "preview", "discarded", true},
		{"committed-to-preview", "committed", "preview", false},
		{"committed-to-committed", "committed", "committed", false},
		{"committed-to-discarded", "committed", "discarded", false},
		{"discarded-to-preview", "discarded", "preview", false},
		{"discarded-to-committed", "discarded", "committed", false},
		{"discarded-to-discarded", "discarded", "discarded", false},
		{"empty-to-committed", "", "committed", false},
		{"pending-to-committed", "pending", "committed", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CanTransition(tc.from, tc.to)
			if got != tc.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
