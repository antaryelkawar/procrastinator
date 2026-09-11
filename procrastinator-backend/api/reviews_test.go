package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestListReviews_Empty(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/ingest/reviews", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var reviews []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &reviews); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(reviews) != 0 {
		t.Errorf("reviews = %d items, want 0", len(reviews))
	}
}

func TestListReviews_InvalidStatus(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/ingest/reviews?status=bogus", "", nil, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetReview_NotFound(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "GET", "/api/users/alice/ingest/reviews/00000000-0000-0000-0000-000000000000", "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestApproveReview_NotFound(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "POST", "/api/users/alice/ingest/reviews/00000000-0000-0000-0000-000000000000/approve", "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestRejectReview_NotFound(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	rec := do(t, e.handler, "POST", "/api/users/alice/ingest/reviews/00000000-0000-0000-0000-000000000000/reject", "", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestReview_Hold_Approve_Flow exercises the full hold → list → approve cycle.
func TestReview_Hold_Approve_Flow(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	// Upload with low confidence → 202
	e.llm.setPayload(`{"classification":"invoice","brand":"TestBrand","model":"M1","serial_number":"SN-HOLD-1","confidence":0.3}`)
	body, ct := buildMultipart(t, "file", "low.pdf", "application/pdf", pdfBytes(64))
	rec := do(t, e.handler, "POST", "/api/users/alice/documents", "", body, ct)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}
	var held struct {
		Id   string `json:"id"`
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &held); err != nil {
		t.Fatalf("unmarshal held review: %v", err)
	}
	if held.Id == "" {
		t.Fatal("held review id is empty")
	}
	if held.Data.State != "pending" {
		t.Fatalf("held state = %q, want \"pending\"", held.Data.State)
	}

	// List → should contain the held review
	rec = do(t, e.handler, "GET", "/api/users/alice/ingest/reviews", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	var list []struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least 1 review in list")
	}
	found := false
	for _, r := range list {
		if r.Id == held.Id {
			found = true
		}
	}
	if !found {
		t.Errorf("list does not contain held review %q", held.Id)
	}

	// Approve
	rec = do(t, e.handler, "POST", fmt.Sprintf("/api/users/alice/ingest/reviews/%s/approve", held.Id), "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var appr struct {
		Asset struct {
			Id string `json:"id"`
		} `json:"asset"`
		Review struct {
			Data struct {
				State string `json:"state"`
			} `json:"data"`
		} `json:"review"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &appr); err != nil {
		t.Fatalf("unmarshal approve: %v", err)
	}
	if appr.Asset.Id == "" {
		t.Error("approve response asset id is empty")
	}
	if appr.Review.Data.State != "approved" {
		t.Errorf("approve review state = %q, want \"approved\"", appr.Review.Data.State)
	}

	// Get → should be approved
	rec = do(t, e.handler, "GET", fmt.Sprintf("/api/users/alice/ingest/reviews/%s", held.Id), "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rec.Code)
	}
	var got struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if got.Data.State != "approved" {
		t.Errorf("get state = %q, want \"approved\"", got.Data.State)
	}

	// Approve again → 409
	rec = do(t, e.handler, "POST", fmt.Sprintf("/api/users/alice/ingest/reviews/%s/approve", held.Id), "", nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-approve status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestReview_Hold_Reject_Flow exercises the full hold → reject cycle.
func TestReview_Hold_Reject_Flow(t *testing.T) {
	e := newEnv(t, envOpts{})
	seedUser(t, e.pool, "alice")

	// Upload with low confidence → 202
	e.llm.setPayload(`{"classification":"invoice","brand":"TestBrand","model":"M2","serial_number":"SN-HOLD-2","confidence":0.3}`)
	body, ct := buildMultipart(t, "file", "low2.pdf", "application/pdf", pdfBytes(64))
	rec := do(t, e.handler, "POST", "/api/users/alice/documents", "", body, ct)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
	}
	var held struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &held); err != nil {
		t.Fatalf("unmarshal held review: %v", err)
	}

	// Reject
	rec = do(t, e.handler, "POST", fmt.Sprintf("/api/users/alice/ingest/reviews/%s/reject", held.Id), "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reject status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var rejected struct {
		Data struct {
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("unmarshal reject: %v", err)
	}
	if rejected.Data.State != "rejected" {
		t.Errorf("reject state = %q, want \"rejected\"", rejected.Data.State)
	}

	// Reject again → 409
	rec = do(t, e.handler, "POST", fmt.Sprintf("/api/users/alice/ingest/reviews/%s/reject", held.Id), "", nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-reject status = %d, want 409 (body: %s)", rec.Code, rec.Body.String())
	}
}
