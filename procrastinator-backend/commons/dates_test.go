package commons

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

func TestAddWarrantyEnd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		purchase    *time.Time
		duration    string
		explicitEnd *time.Time
		want        *time.Time
		ok          bool
	}{
		{
			name:     "N years full form",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "2 years",
			want:     &[]time.Time{time.Date(2028, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "N months full form",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "24 months",
			want:     &[]time.Time{time.Date(2028, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:        "explicit end wins",
			purchase:    &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration:    "2 years",
			explicitEnd: &[]time.Time{time.Date(2027, time.June, 30, 0, 0, 0, 0, time.UTC)}[0],
			want:        &[]time.Time{time.Date(2027, time.June, 30, 0, 0, 0, 0, time.UTC)}[0],
			ok:           true,
		},
		{
			name:     "non-matching duration",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "sometime",
			ok:       false,
		},
		{
			name:     "ISO 1 year",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "P1Y",
			want:     &[]time.Time{time.Date(2027, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "ISO years and months",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "P2Y6M",
			want:     &[]time.Time{time.Date(2028, time.July, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "N days full form",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "30 days",
			want:     &[]time.Time{time.Date(2026, time.February, 14, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "abbreviated days",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "30d",
			want:     &[]time.Time{time.Date(2026, time.February, 14, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "abbreviated years",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "2 yr",
			want:     &[]time.Time{time.Date(2028, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "abbreviated months",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "2 mo",
			want:     &[]time.Time{time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "nil purchase",
			duration: "2 years",
			ok:       false,
		},
		{
			name:     "empty duration",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			ok:       false,
		},
		{
			name:     "ISO days",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "P30D",
			want:     &[]time.Time{time.Date(2026, time.February, 14, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
		{
			name:     "zero duration",
			purchase: &[]time.Time{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)}[0],
			duration: "P0Y",
			ok:       false,
		},
		{
			name:     "end-of-month clamping",
			purchase: &[]time.Time{time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)}[0],
			duration: "1 month",
			want:     &[]time.Time{time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC)}[0],
			ok:       true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := AddWarrantyEnd(tc.purchase, tc.duration, tc.explicitEnd)
			if ok != tc.ok {
				t.Fatalf("for duration %q: expected ok=%v, got ok=%v (got=%v)", tc.duration, tc.ok, ok, got)
			}
			if tc.ok {
				if got == nil {
					t.Fatalf("for duration %q: expected non-nil time, got nil", tc.duration)
				}
				if !got.Equal(*tc.want) {
					t.Fatalf("for duration %q: expected %v, got %v", tc.duration, tc.want, got)
				}
			} else {
				if got != nil {
					t.Fatalf("for duration %q: expected nil time, got %v", tc.duration, got)
				}
			}
		})
	}
}
