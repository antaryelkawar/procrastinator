package entity

import (
	"testing"
)

func TestExtractionConfidence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		conf     *float64
		wantNil  bool
		wantVal  float64
	}{
		{"absent", nil, true, 0},
		{"present", ptrToFloat(0.85), false, 0.85},
		{"zero", ptrToFloat(0.0), false, 0.0},
		{"one", ptrToFloat(1.0), false, 1.0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Extraction{Confidence: tc.conf}
			if tc.wantNil && e.Confidence != nil {
				t.Fatalf("Confidence = %v, want nil", *e.Confidence)
			}
			if !tc.wantNil && (e.Confidence == nil || *e.Confidence != tc.wantVal) {
				t.Fatalf("Confidence = %v, want %v", ptrFloatDisplay(e.Confidence), tc.wantVal)
			}
		})
	}
}

func ptrToFloat(v float64) *float64 { return &v }

func ptrFloatDisplay(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
