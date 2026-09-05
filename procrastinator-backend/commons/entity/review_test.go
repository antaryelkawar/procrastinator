package entity

import "testing"

// TestIngestReviewStateConstants pins the state constants to the exact values
// enforced by the ingest_reviews.state CHECK constraint in migration 00005.
// If the DB constraint and the Go constants ever drift, the list/decision
// endpoints silently stop matching rows.
func TestIngestReviewStateConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state IngestReviewState
		want  string
	}{
		{"pending", ReviewStatePending, "pending"},
		{"approved", ReviewStateApproved, "approved"},
		{"rejected", ReviewStateRejected, "rejected"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.state) != tc.want {
				t.Fatalf("IngestReviewState(%q) = %q, want %q", tc.name, tc.state, tc.want)
			}
		})
	}
}

func TestIngestReviewZeroValue(t *testing.T) {
	t.Parallel()

	r := IngestReview{}
	if r.OwnerHouseholdID != nil {
		t.Fatalf("IngestReview.OwnerHouseholdID = %v, want nil for zero value", *r.OwnerHouseholdID)
	}
	if r.Confidence != nil {
		t.Fatalf("IngestReview.Confidence = %v, want nil for zero value", *r.Confidence)
	}
	if r.BestMatchedAssetID != nil {
		t.Fatalf("IngestReview.BestMatchedAssetID = %v, want nil for zero value", *r.BestMatchedAssetID)
	}
	if r.DecidedAt != nil {
		t.Fatalf("IngestReview.DecidedAt = %v, want nil for zero value", *r.DecidedAt)
	}
	if r.DecidedBy != nil {
		t.Fatalf("IngestReview.DecidedBy = %v, want nil for zero value", *r.DecidedBy)
	}
}
