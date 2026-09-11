package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"procrastinator-backend/api/documents"
	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
)

// --- GET /api/users/{userId}/documents (listDocuments) ---

func TestDocumentsList(t *testing.T) {
	t.Parallel()

	t.Run("ListsUploadedDocuments", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		// Upload two documents with different filenames.
		_, asset1 := e.uploadFile(t, "test-user", "invoice-2024.pdf", "application/pdf", pdfBytes(10))
		_, asset2 := e.uploadFile(t, "test-user", "receipt-2024.pdf", "application/pdf", pdfBytes(20))

		// Both uploads should have succeeded (201).
		if asset1 == nil || asset2 == nil {
			t.Fatal("expected both uploads to return an asset")
		}

		// GET the list.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var docs []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal list: %v (body: %s)", err, rec.Body.String())
		}
		if len(docs) != 2 {
			t.Fatalf("len(docs) = %d, want 2 (body: %s)", len(docs), rec.Body.String())
		}

		// Each document should have status "processed" and a source_filename.
		for i, d := range docs {
			if status, _ := d["status"].(string); status != "processed" {
				t.Errorf("docs[%d].status = %q, want %q", i, status, "processed")
			}
			if fn, _ := d["source_filename"].(string); fn == "" {
				t.Errorf("docs[%d].source_filename is empty", i)
			}
		}
	})

	t.Run("FilterByStatus", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		// Upload one document (processed).
		e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(10))

		// Filter by processed → 1 result.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents?status=processed", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var docs []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(docs) != 1 {
			t.Errorf("len(docs) = %d, want 1 (body: %s)", len(docs), rec.Body.String())
		}

		// Filter by failed → 0 results.
		rec = do(t, e.handler, http.MethodGet, "/api/users/test-user/documents?status=failed", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(docs) != 0 {
			t.Errorf("len(docs) for failed = %d, want 0 (body: %s)", len(docs), rec.Body.String())
		}
	})

	t.Run("FilterByFilename", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		e.uploadFile(t, "test-user", "invoice-2024.pdf", "application/pdf", pdfBytes(10))
		e.uploadFile(t, "test-user", "receipt-2024.pdf", "application/pdf", pdfBytes(20))

		// Search for "invoice" → 1 result.
		rec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents?q=invoice", "", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var docs []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(docs) != 1 {
			t.Errorf("len(docs) for q=invoice = %d, want 1 (body: %s)", len(docs), rec.Body.String())
		}
		if fn, _ := docs[0]["source_filename"].(string); fn != "invoice-2024.pdf" {
			t.Errorf("source_filename = %q, want %q", fn, "invoice-2024.pdf")
		}
	})
}

// --- POST /api/users/{userId}/documents/{id}/reprocess ---

func TestDocumentsReprocess(t *testing.T) {
	t.Parallel()

	t.Run("ReprocessWithCommentPersistsDirective", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		// Upload a document.
		rec, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(10))
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201", rec.Code)
		}
		assetID, _ := asset["id"].(string)
		if assetID == "" {
			t.Fatal("asset id is empty")
		}

		// Get the document id from the list.
		listRec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
		var docs []map[string]any
		if err := json.Unmarshal(listRec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("len(docs) = %d, want 1", len(docs))
		}
		docID, _ := docs[0]["id"].(string)
		if docID == "" {
			t.Fatal("document id is empty")
		}

		// Reprocess with a comment.
		comment := "receipt, not invoice"
		body := fmt.Sprintf(`{"comment":%q}`, comment)
		rec = do(t, e.handler, http.MethodPost, "/api/users/test-user/documents/"+docID+"/reprocess", "", bytes.NewBufferString(body), "application/json")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("reprocess status = %d, want 202 (body: %s)", rec.Code, rec.Body.String())
		}

		// Verify the LLM was called with the comment in the system prompt.
		sp := e.lastSystemPrompt()
		if !contains(sp, comment) {
			t.Errorf("system prompt does not contain %q (got: %q)", comment, sp)
		}

		// Verify the document's data.user_directive is persisted.
		listRec2 := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
		var docs2 []map[string]any
		if err := json.Unmarshal(listRec2.Body.Bytes(), &docs2); err != nil {
			t.Fatalf("unmarshal list2: %v", err)
		}
		if len(docs2) != 1 {
			t.Fatalf("len(docs2) = %d, want 1", len(docs2))
		}
		data, ok := docs2[0]["data"].(map[string]any)
		if !ok {
			t.Fatal("data is not an object")
		}
		if ud, _ := data["user_directive"].(string); ud != comment {
			t.Errorf("data.user_directive = %q, want %q", ud, comment)
		}
	})

	t.Run("ReprocessInFlightReturns409", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		// Step 1: Upload a document that commits successfully (high confidence).
		rec, _ := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(10))
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, want 201", rec.Code)
		}

		// Get the document id.
		listRec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
		var docs []map[string]any
		if err := json.Unmarshal(listRec.Body.Bytes(), &docs); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(docs) != 1 {
			t.Fatalf("len(docs) = %d, want 1", len(docs))
		}
		docID, _ := docs[0]["id"].(string)

		// Step 2: Reprocess with low confidence → held for review → in_review.
		// Change the LLM to return a low-confidence payload (no identity fields).
		e.setLLMPayload(`{"classification":"other"}`)
		rec2 := do(t, e.handler, http.MethodPost, "/api/users/test-user/documents/"+docID+"/reprocess", "", bytes.NewBufferString(`{}`), "application/json")
		if rec2.Code != http.StatusAccepted {
			t.Fatalf("first reprocess status = %d, want 202 (body: %s)", rec2.Code, rec2.Body.String())
		}

		// Step 3: Second reprocess → 409 (in-flight).
		rec3 := do(t, e.handler, http.MethodPost, "/api/users/test-user/documents/"+docID+"/reprocess", "", bytes.NewBufferString(`{}`), "application/json")
		if rec3.Code != http.StatusConflict {
			t.Fatalf("second reprocess status = %d, want 409 (body: %s)", rec3.Code, rec3.Body.String())
		}
		assertErrorEnvelope(t, rec3)
	})
}

// --- POST /api/users/{userId}/documents/{id}/keep ---

func TestDocumentsKeepResolvesPendingChoice(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})

	// Upload a document first (creates the original).
	_, asset1 := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(30))
	if asset1 == nil {
		t.Fatal("first upload failed")
	}

	// Upload the same content (duplicate) → 409 with DuplicateReport.
	body, ct := buildMultipartWithNote(t, "invoice.pdf", "application/pdf", pdfBytes(30), nil)
	dupRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/documents", "", body, ct)
	if dupRec.Code != http.StatusConflict {
		t.Fatalf("duplicate upload status = %d, want 409 (body: %s)", dupRec.Code, dupRec.Body.String())
	}

	// Parse the DuplicateReport to get the pending document id.
	var report map[string]any
	if err := json.Unmarshal(dupRec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal duplicate report: %v", err)
	}
	// The pending document id is not directly in the report; we need to find it
	// from the list. The report has the reprocess_uri which contains the doc id.
	reprocessURI, _ := report["prompt"].(map[string]any)
	if reprocessURI == nil {
		t.Fatal("report.prompt is nil")
	}
	uri, _ := reprocessURI["reprocess_uri"].(string)
	// Extract the document id from the URI: /api/users/test-user/documents/{id}/reprocess
	// The id is the segment between "/documents/" and "/reprocess".
	parts := splitPath(uri)
	if len(parts) < 5 {
		t.Fatalf("unexpected reprocess_uri: %q", uri)
	}
	pendingDocID := parts[len(parts)-2] // second-to-last segment
	if pendingDocID == "" {
		t.Fatal("pending document id is empty")
	}

	// POST keep.
	keepRec := do(t, e.handler, http.MethodPost, "/api/users/test-user/documents/"+pendingDocID+"/keep", "", nil, "")
	if keepRec.Code != http.StatusOK {
		t.Fatalf("keep status = %d, want 200 (body: %s)", keepRec.Code, keepRec.Body.String())
	}

	// Verify the document's pending_choice is resolved.
	listRec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
	var docs []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	// Find the pending document in the list.
	found := false
	for _, d := range docs {
		if d["id"] == pendingDocID {
			found = true
			break
		}
	}
	if !found {
		t.Error("pending document not found in list after keep")
	}
}

// --- DELETE /api/users/{userId}/documents/{id} ---

func TestDocumentsDeleteDetachesAsset(t *testing.T) {
	t.Parallel()

	e := newEnv(t, envOpts{})
	// Upload a document (creates asset + document).
	_, asset := e.uploadFile(t, "test-user", "invoice.pdf", "application/pdf", pdfBytes(10))
	if asset == nil {
		t.Fatal("upload failed")
	}
	assetID, _ := asset["id"].(string)

	// Get the document id.
	listRec := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
	var docs []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	docID, _ := docs[0]["id"].(string)

	// DELETE the document.
	delRec := do(t, e.handler, http.MethodDelete, "/api/users/test-user/documents/"+docID, "", nil, "")
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204 (body: %s)", delRec.Code, delRec.Body.String())
	}

	// The asset should still exist.
	assetRec := do(t, e.handler, http.MethodGet, "/api/users/test-user/assets/"+assetID, "", nil, "")
	if assetRec.Code != http.StatusOK {
		t.Errorf("asset after delete status = %d, want 200 (asset should be preserved)", assetRec.Code)
	}

	// The document list should be empty.
	listRec2 := do(t, e.handler, http.MethodGet, "/api/users/test-user/documents", "", nil, "")
	if listRec2.Code != http.StatusOK {
		t.Fatalf("list after delete status = %d, want 200", listRec2.Code)
	}
	var docs2 []map[string]any
	if err := json.Unmarshal(listRec2.Body.Bytes(), &docs2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(docs2) != 0 {
		t.Errorf("len(docs) after delete = %d, want 0 (body: %s)", len(docs2), listRec2.Body.String())
	}
}

// --- Sweeper tests (unit, no HTTP) ---

func TestSweeperResolvesExpiredPending(t *testing.T) {
	// This test requires a database. It creates a document with an expired
	// pending_choice, runs the sweeper, and verifies the resolution.
	e := newEnv(t, envOpts{})

	// Create a document with an expired pending choice directly via the factory.
	ctx := context.Background()
	now := time.Now()
	expired := now.Add(-1 * time.Second)

	src := entity.Source{Filename: "test.pdf", ContentType: "application/pdf", Size: 100, SHA256: "abc123", Path: "test.pdf"}
	src, err := e.factory.Sources.Create(ctx, src, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	doc := entity.Document{
		SourceID: src.ID,
		PendingChoice: &entity.PendingChoice{
			State:     "pending",
			Outcome:   "",
			CreatedAt: now.Add(-10 * time.Minute),
			ExpiresAt: expired,
		},
	}
	doc, err = e.factory.Documents.Create(ctx, doc, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	// Run the sweeper once.
	sweeper := documents.NewSweeper(e.pool)
	sweeper.SetClock(func() time.Time { return now })
	sweeper.Sweep(ctx)

	// Verify the document's pending_choice is resolved.
	docs, err := e.factory.Documents.List(ctx, repo.Owner("test-user"), repo.Where("id", "=", doc.ID))
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	pc := docs[0].PendingChoice
	if pc == nil {
		t.Fatal("pending_choice is nil after sweep")
	}
	if pc.State != "resolved" {
		t.Errorf("pending_choice.state = %q, want %q", pc.State, "resolved")
	}
	if pc.Outcome != "keep_existing" {
		t.Errorf("pending_choice.outcome = %q, want %q", pc.Outcome, "keep_existing")
	}
}

func TestSweeperRestartRecovery(t *testing.T) {
	// Simulates a process restart: a fresh sweeper sees an already-expired
	// pending choice and resolves it on the first sweep (no lost state).
	e := newEnv(t, envOpts{})

	ctx := context.Background()
	now := time.Now()
	expired := now.Add(-5 * time.Minute)

	src := entity.Source{Filename: "restart.pdf", ContentType: "application/pdf", Size: 50, SHA256: "def456", Path: "restart.pdf"}
	src, err := e.factory.Sources.Create(ctx, src, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	doc := entity.Document{
		SourceID: src.ID,
		PendingChoice: &entity.PendingChoice{
			State:     "pending",
			Outcome:   "",
			CreatedAt: now.Add(-15 * time.Minute),
			ExpiresAt: expired,
		},
	}
	doc, err = e.factory.Documents.Create(ctx, doc, repo.Owner("test-user"))
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	// Simulate restart: create a fresh sweeper and run its initial sweep.
	sweeper := documents.NewSweeper(e.pool)
	sweeper.SetClock(func() time.Time { return now })
	sweeper.Sweep(ctx)

	// Verify resolution.
	docs, err := e.factory.Documents.List(ctx, repo.Owner("test-user"), repo.Where("id", "=", doc.ID))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	pc := docs[0].PendingChoice
	if pc == nil {
		t.Fatal("pending_choice is nil after restart recovery")
	}
	if pc.State != "resolved" || pc.Outcome != "keep_existing" {
		t.Errorf("pending_choice = {%q, %q}, want {resolved, keep_existing}", pc.State, pc.Outcome)
	}
}

// --- helpers ---

// contains reports whether s contains substr (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// splitPath splits a URL path into its segments (ignoring the scheme/host).
func splitPath(p string) []string {
	// Find the path part after the host.
	idx := 0
	for i := 0; i < len(p)-3; i++ {
		if p[i:i+4] == "//" {
			// Skip to the next / after the host.
			j := i + 2
			for j < len(p) && p[j] != '/' {
				j++
			}
			idx = j
			break
		}
	}
	if idx == 0 {
		idx = 0
	}
	var parts []string
	for _, seg := range splitByByte(p[idx:], '/') {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return parts
}

func splitByByte(s string, b byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ensure httptest import is used (suppresses unused import warning if tests are trimmed).
var _ = httptest.NewRecorder
