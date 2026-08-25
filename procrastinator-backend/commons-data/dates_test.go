package data

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"ISO 8601 date", "2023-11-03", time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC), false},
		{"RFC3339 with Z", "2023-11-03T10:00:00Z", time.Date(2023, time.November, 3, 10, 0, 0, 0, time.UTC), false},
		{"DD-MMM-YY", "03-NOV-23", time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC), false},
		{"DD-MM-YY", "03-11-23", time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC), false},
		{"DD.MM.YYYY", "08.03.2018", time.Date(2018, time.March, 8, 0, 0, 0, 0, time.UTC), false},
		{"DD-MMM-YYYY", "01-NOV-2023", time.Date(2023, time.November, 1, 0, 0, 0, 0, time.UTC), false},
		{"surrounding whitespace", "  2023-11-03  ", time.Date(2023, time.November, 3, 0, 0, 0, 0, time.UTC), false},
		{"empty string is zero time", "", time.Time{}, false},
		{"whitespace only is zero time", "   ", time.Time{}, false},
		{"garbage words", "not a date", time.Time{}, true},
		{"wrong separator", "2023/11/03", time.Time{}, true},
		{"out-of-range month", "2023-13-01", time.Time{}, true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDate(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.in)
				}
				if !got.IsZero() {
					t.Fatalf("expected zero time on error for %q, got %v", tc.in, got)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.in, err)
				}
				if !got.Equal(tc.want) {
					t.Fatalf("for %q: expected %v, got %v", tc.in, tc.want, got)
				}
			}
		})
	}
}
