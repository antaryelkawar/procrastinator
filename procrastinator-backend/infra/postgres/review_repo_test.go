package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// TestReviewRepositoryInterface pins the compile-time contract that
// ReviewRepository satisfies the generic repository interface. It is a no-op
// that documents intent and fails if the type ever stops satisfying it.
func TestReviewRepositoryInterface(t *testing.T) {
	t.Parallel()
	var _ repo.Repository[entity.IngestReview] = (*ReviewRepository)(nil)
}

// TestScanReviewDecodesJSONB verifies scanReview decodes the two jsonb columns
// (candidate_fields → map, raw_extraction → string) and maps pgx.ErrNoRows to
// repo.ErrNotFound.
func TestScanReviewDecodesJSONB(t *testing.T) {
	t.Parallel()

	created := time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)

	// A fake rowScanner returning exactly 14 values in ingest_reviews column order.
	fake := fakeRow{
		values: []any{
			"rev-id",
			"owner-id",
			strPtr("hh-id"),
			"src-id",
			"invoice",
			[]byte(`{"brand":"Samsung","model":"WF80A"}`),
			[]byte(`"raw llm output"`),
			f64ptr(0.42),
			strPtr("asset-id"),
			entity.ReviewStatePending,
			created,
			(*time.Time)(nil),
			(*string)(nil),
			[]byte(`{"workers":[{"model":"gpt-4","result":"ok"}],"candidates":["a1","a2"]}`),
		},
	}

	got, err := scanReview(fake)
	if err != nil {
		t.Fatalf("scanReview: %v", err)
	}
	if got.ID != "rev-id" {
		t.Errorf("ID = %q, want rev-id", got.ID)
	}
	if got.OwnerID != "owner-id" {
		t.Errorf("OwnerID = %q, want owner-id", got.OwnerID)
	}
	if got.SourceID != "src-id" {
		t.Errorf("SourceID = %q, want src-id", got.SourceID)
	}
	if got.DocType != "invoice" {
		t.Errorf("DocType = %q, want invoice", got.DocType)
	}
	if got.CandidateFields == nil || got.CandidateFields["brand"] != "Samsung" {
		t.Errorf("CandidateFields = %v, want brand=Samsung", got.CandidateFields)
	}
	if got.RawExtraction != "raw llm output" {
		t.Errorf("RawExtraction = %q, want %q", got.RawExtraction, "raw llm output")
	}
	if got.State != entity.ReviewStatePending {
		t.Errorf("State = %q, want pending", got.State)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	if got.OwnerHouseholdID == nil || *got.OwnerHouseholdID != "hh-id" {
		t.Errorf("OwnerHouseholdID = %v, want hh-id", got.OwnerHouseholdID)
	}
	if got.Confidence == nil || *got.Confidence != 0.42 {
		t.Errorf("Confidence = %v, want 0.42", got.Confidence)
	}
	if got.BestMatchedAssetID == nil || *got.BestMatchedAssetID != "asset-id" {
		t.Errorf("BestMatchedAssetID = %v, want asset-id", got.BestMatchedAssetID)
	}
	// Provenance JSONB decoded into a map.
	if got.Provenance == nil {
		t.Fatalf("Provenance = nil, want decoded map")
	}
	if w, ok := got.Provenance["workers"].([]any); !ok || len(w) == 0 {
		t.Errorf("Provenance[workers] = %v, want non-empty slice", got.Provenance["workers"])
	}
	if c, ok := got.Provenance["candidates"].([]any); !ok || len(c) != 2 {
		t.Errorf("Provenance[candidates] = %v, want 2-element slice", got.Provenance["candidates"])
	}
}

// TestScanReviewNoRows verifies scanReview maps pgx.ErrNoRows to repo.ErrNotFound.
func TestScanReviewNoRows(t *testing.T) {
	t.Parallel()

	fake := fakeRow{err: pgx.ErrNoRows}
	_, err := scanReview(fake)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("err = %v, want repo.ErrNotFound", err)
	}
}

// TestReviewToMapNonZero verifies reviewToMap includes only non-zero/non-nil
// fields and encodes the jsonb columns.
func TestReviewToMapNonZero(t *testing.T) {
	t.Parallel()

	created := time.Date(2025, 6, 1, 12, 30, 45, 0, time.UTC)
	r := entity.IngestReview{
		ID:                 "rev-id",
		OwnerID:            "owner-id",
		OwnerHouseholdID:   strPtr("hh-id"),
		SourceID:           "src-id",
		DocType:            "invoice",
		CandidateFields:    map[string]any{"brand": "Samsung"},
		RawExtraction:      "raw",
		State:              entity.ReviewStateApproved,
		CreatedAt:          created,
		Confidence:         f64ptr(0.9),
		BestMatchedAssetID: strPtr("asset-id"),
		DecidedAt:          &created,
		DecidedBy:          strPtr("decider"),
		Provenance:         map[string]any{"workers": []any{"w1", "w2"}},
	}

	m := reviewToMap(r)

	// Scalar (non-pointer) keys compared by value.
	wantScalars := map[string]any{
		"id":        "rev-id",
		"owner_id":  "owner-id",
		"source_id": "src-id",
		"doc_type":  "invoice",
		"state":     "approved",
	}
	for k, v := range wantScalars {
		got, ok := m[k]
		if !ok {
			t.Fatalf("missing key %q in map: %v", k, m)
		}
		if got != v {
			t.Errorf("key %q = %v, want %v", k, got, v)
		}
	}

	// Pointer keys compared by dereferenced value.
	assertPtrString(t, "owner_household_id", m["owner_household_id"], "hh-id")
	assertPtrFloat64(t, "confidence", m["confidence"], 0.9)
	assertPtrString(t, "best_matched_asset_id", m["best_matched_asset_id"], "asset-id")
	assertPtrString(t, "decided_by", m["decided_by"], "decider")
	gotAt, ok := m["decided_at"].(*time.Time)
	if !ok {
		t.Fatalf("decided_at = %T, want *time.Time", m["decided_at"])
	}
	if !gotAt.Equal(created) {
		t.Errorf("decided_at = %v, want %v", *gotAt, created)
	}

	// jsonb columns are JSON-encoded bytes.
	if cf, ok := m["candidate_fields"].([]byte); !ok {
		t.Fatalf("candidate_fields = %T, want []byte", m["candidate_fields"])
	} else if string(cf) != `{"brand":"Samsung"}` {
		t.Errorf("candidate_fields = %s, want encoded map", cf)
	}
	if re, ok := m["raw_extraction"].([]byte); !ok {
		t.Fatalf("raw_extraction = %T, want []byte", m["raw_extraction"])
	} else if string(re) != `"raw"` {
		t.Errorf("raw_extraction = %s, want encoded string", re)
	}
	if prov, ok := m["provenance"].([]byte); !ok {
		t.Fatalf("provenance = %T, want []byte", m["provenance"])
	} else if len(prov) == 0 {
		t.Errorf("provenance is empty, want encoded map")
	}
}

// TestReviewToMapZero verifies reviewToMap omits all zero-valued fields.
func TestReviewToMapZero(t *testing.T) {
	t.Parallel()

	m := reviewToMap(entity.IngestReview{})
	if len(m) != 0 {
		t.Fatalf("reviewToMap(zero) = %v, want empty map", m)
	}
}

// fakeRow is a rowScanner that writes a fixed set of values (or returns a fixed
// error). values[i] is written into the corresponding dest pointer; a nil entry
// leaves a pointer dest as nil (SQL NULL).
type fakeRow struct {
	values []any
	err    error
}

func (f fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	if len(dest) != len(f.values) {
		return errors.New("fakeRow: dest length mismatch")
	}
	for i, d := range dest {
		v := f.values[i]
		switch p := d.(type) {
		case *string:
			s, _ := v.(string)
			*p = s
		case *[]byte:
			b, _ := v.([]byte)
			*p = b
		case **string:
			s, _ := v.(*string)
			*p = s
		case **float64:
			flt, _ := v.(*float64)
			*p = flt
		case *time.Time:
			tm, _ := v.(time.Time)
			*p = tm
		case **time.Time:
			tm, _ := v.(*time.Time)
			*p = tm
		case *entity.IngestReviewState:
			st, _ := v.(entity.IngestReviewState)
			*p = st
		default:
			return errors.New("fakeRow: unsupported dest type")
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }

func f64ptr(f float64) *float64 { return &f }

func assertPtrString(t *testing.T, key string, got any, want string) {
	t.Helper()
	p, ok := got.(*string)
	if !ok {
		t.Fatalf("%s = %T, want *string", key, got)
	}
	if *p != want {
		t.Errorf("%s = %q, want %q", key, *p, want)
	}
}

func assertPtrFloat64(t *testing.T, key string, got any, want float64) {
	t.Helper()
	p, ok := got.(*float64)
	if !ok {
		t.Fatalf("%s = %T, want *float64", key, got)
	}
	if *p != want {
		t.Errorf("%s = %v, want %v", key, *p, want)
	}
}
