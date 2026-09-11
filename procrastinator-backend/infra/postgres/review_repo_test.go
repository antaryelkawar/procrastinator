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

	// A fake rowScanner returning exactly 8 values in the ingest_reviews
	// codec selectCols order (id, owner_id, owner_household_id, source_id,
	// deleted_at, created_at, updated_at, payload). All the data portion
	// lives in the payload's data object; numbers arrive as json.Number
	// (UseNumber decode output).
	fake := fakeRow{
		values: []any{
			"rev-id",
			"owner-id",
			strPtr("hh-id"),
			"src-id",
			(*time.Time)(nil),
			created,
			(*time.Time)(nil),
			[]byte(`{"data":{"doc_type":"invoice","candidate_fields":{"brand":"Samsung","model":"WF80A"},"raw_extraction":"raw llm output","confidence":0.42,"best_matched_asset_id":"asset-id","state":"pending","provenance":{"workers":[{"model":"gpt-4","result":"ok"}],"candidates":["a1","a2"]}}}`),
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

// TestReviewCodecMarshal verifies reviewCodec.marshal includes only
// non-zero/non-nil fields in the data.* object and that columns() returns the
// non-payload column values (owner_household_id, source_id).
func TestReviewCodecMarshal(t *testing.T) {
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

	data, clear := reviewCodec.marshal(r)
	if len(clear) != 0 {
		t.Errorf("clear = %v, want empty (reviews have no clearing semantics)", clear)
	}

	// Scalar data keys compared by value.
	wantScalars := map[string]any{
		"doc_type":              "invoice",
		"raw_extraction":        "raw",
		"best_matched_asset_id": "asset-id",
		"state":                 "approved",
		"decided_by":            "decider",
		"decided_at":            "2025-06-01T12:30:45Z",
	}
	for k, v := range wantScalars {
		got, ok := data[k]
		if !ok {
			t.Fatalf("missing data key %q: %v", k, data)
		}
		if got != v {
			t.Errorf("data[%q] = %v, want %v", k, got, v)
		}
	}

	// Pointer-backed keys compared by dereferenced value.
	if cf, ok := data["candidate_fields"].(map[string]any); !ok || cf["brand"] != "Samsung" {
		t.Errorf("data[candidate_fields] = %v, want brand=Samsung", data["candidate_fields"])
	}
	if prov, ok := data["provenance"].(map[string]any); !ok || prov["workers"] == nil {
		t.Errorf("data[provenance] = %v, want workers map entry", data["provenance"])
	}
	assertDataFloat64(t, "confidence", data, 0.9)

	// columns() carries the non-payload column values.
	cols := reviewCodec.columns(r)
	if len(cols) != 2 {
		t.Fatalf("columns = %v, want owner_household_id + source_id", cols)
	}
	assertPtrString(t, "owner_household_id", cols["owner_household_id"], "hh-id")
	if cols["source_id"] != "src-id" {
		t.Errorf("columns[source_id] = %v, want src-id", cols["source_id"])
	}
}

// TestReviewCodecMarshalZero verifies reviewCodec.marshal on a zero entity
// produces an empty data object and empty columns (no clearing keys).
func TestReviewCodecMarshalZero(t *testing.T) {
	t.Parallel()

	data, clear := reviewCodec.marshal(entity.IngestReview{})
	if len(data) != 0 {
		t.Errorf("data = %v, want empty", data)
	}
	if len(clear) != 0 {
		t.Errorf("clear = %v, want empty", clear)
	}
	cols := reviewCodec.columns(entity.IngestReview{})
	if len(cols) != 0 {
		t.Errorf("columns = %v, want empty", cols)
	}
}

// assertDataFloat64 checks a numeric data-map value against a float64.
func assertDataFloat64(t *testing.T, key string, data map[string]any, want float64) {
	t.Helper()
	f, ok := numFloat64(data[key])
	if !ok {
		t.Fatalf("data[%q] = %v, want numeric %v", key, data[key], want)
	}
	if f != want {
		t.Errorf("data[%q] = %v, want %v", key, f, want)
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
