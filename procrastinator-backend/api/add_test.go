package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"procrastinator-backend/commons/repo"
)

// addFile is one file part for a unified add request.
type addFile struct {
	filename    string
	contentType string
	content     []byte
}

// postAdd posts a unified add request (files and/or text and/or account_id) to
// /api/users/{userID}/add and returns the recorded response. Multiple file
// parts all share the "files" field name (the oapi-codegen runtime collects
// them into the Files slice); text and account_id are plain form fields.
func postAdd(t *testing.T, e *testEnv, userID string, files []addFile, text *string, accountID *string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range files {
		part, err := w.CreatePart(map[string][]string{
			"Content-Disposition": {fmt.Sprintf(`form-data; name="files"; filename="%s"`, f.filename)},
			"Content-Type":        {f.contentType},
		})
		if err != nil {
			t.Fatalf("create multipart part: %v", err)
		}
		if _, err := part.Write(f.content); err != nil {
			t.Fatalf("write multipart part: %v", err)
		}
	}
	if text != nil {
		if err := w.WriteField("text", *text); err != nil {
			t.Fatalf("write text field: %v", err)
		}
	}
	if accountID != nil {
		if err := w.WriteField("account_id", *accountID); err != nil {
			t.Fatalf("write account_id field: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return do(t, e.handler, http.MethodPost, "/api/users/"+userID+"/add", "", &buf, w.FormDataContentType())
}

// addOutcomes decodes the unified add 200 body into a slice of outcome
// objects, fatalling the test on a decode failure.
func addOutcomes(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal add outcomes: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

// TestAddItems exercises the unified add endpoint POST /api/users/{userId}/add.
func TestAddItems(t *testing.T) {
	t.Parallel()

	t.Run("ReceiptCommitted", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		rec := postAdd(t, e, "test-user", []addFile{{filename: "receipt.pdf", contentType: "application/pdf", content: pdfBytes(24)}}, nil, nil)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		out := addOutcomes(t, rec)
		if len(out) != 1 {
			t.Fatalf("outcome count = %d, want 1 (body: %s)", len(out), rec.Body.String())
		}
		if got := out[0]["kind"]; got != "asset_committed" && got != "held_for_review" {
			t.Errorf("kind = %q, want asset_committed (default env auto-commits)", got)
		}
		if got := strVal(out[0], "asset_id"); got == "" {
			t.Errorf("asset_id is empty, want non-empty (body: %s)", rec.Body.String())
		}
	})

	t.Run("StatementWithAccount", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		acc := createTestAccount(t, e, "test-user")
		csv := readTestdata(t, "statement_ok.csv")
		acct := acc
		rec := postAdd(t, e, "test-user", []addFile{{filename: "statement.csv", contentType: "text/csv", content: csv}}, nil, &acct)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		out := addOutcomes(t, rec)
		if len(out) != 1 {
			t.Fatalf("outcome count = %d, want 1 (body: %s)", len(out), rec.Body.String())
		}
		if got := out[0]["kind"]; got != "statement_preview" {
			t.Errorf("kind = %q, want statement_preview (body: %s)", got, rec.Body.String())
		}
		if got := strVal(out[0], "import_batch_id"); got == "" {
			t.Errorf("import_batch_id is empty, want non-empty (body: %s)", rec.Body.String())
		}
	})

	t.Run("StatementWithoutAccount", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		csv := readTestdata(t, "statement_ok.csv")
		rec := postAdd(t, e, "test-user", []addFile{{filename: "statement.csv", contentType: "text/csv", content: csv}}, nil, nil)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
		}
		assertErrorEnvelope(t, rec)
	})

	t.Run("ByteDuplicate", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		receipt := pdfBytes(24)

		rec1 := postAdd(t, e, "test-user", []addFile{{filename: "receipt.pdf", contentType: "application/pdf", content: receipt}}, nil, nil)
		if rec1.Code != http.StatusOK {
			t.Fatalf("first status = %d, want 200 (body: %s)", rec1.Code, rec1.Body.String())
		}
		out1 := addOutcomes(t, rec1)
		if len(out1) != 1 {
			t.Fatalf("first outcome count = %d, want 1 (body: %s)", len(out1), rec1.Body.String())
		}
		if got := out1[0]["kind"]; got != "asset_committed" && got != "held_for_review" {
			t.Fatalf("first kind = %q, want asset_committed/held_for_review (body: %s)", got, rec1.Body.String())
		}
		firstAsset := strVal(out1[0], "asset_id")
		if firstAsset == "" {
			t.Fatalf("first asset_id is empty (body: %s)", rec1.Body.String())
		}

		rec2 := postAdd(t, e, "test-user", []addFile{{filename: "receipt2.pdf", contentType: "application/pdf", content: receipt}}, nil, nil)
		if rec2.Code != http.StatusOK {
			t.Fatalf("second status = %d, want 200 (body: %s)", rec2.Code, rec2.Body.String())
		}
		out2 := addOutcomes(t, rec2)
		if len(out2) != 1 {
			t.Fatalf("second outcome count = %d, want 1 (body: %s)", len(out2), rec2.Body.String())
		}
		if got := out2[0]["kind"]; got != "duplicate" {
			t.Fatalf("second kind = %q, want duplicate (body: %s)", got, rec2.Body.String())
		}
		if got := strVal(out2[0], "duplicate_asset_id"); got != firstAsset {
			t.Errorf("duplicate_asset_id = %q, want first asset id %q (body: %s)", got, firstAsset, rec2.Body.String())
		}
	})

	t.Run("BadPayload", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		badBytes := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
		rec := postAdd(t, e, "test-user", []addFile{{filename: "bad.bin", contentType: "application/octet-stream", content: badBytes}}, nil, nil)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		out := addOutcomes(t, rec)
		if len(out) != 1 {
			t.Fatalf("outcome count = %d, want 1 (body: %s)", len(out), rec.Body.String())
		}
		if got := out[0]["kind"]; got != "failed" {
			t.Errorf("kind = %q, want failed (body: %s)", got, rec.Body.String())
		}
		if got := strVal(out[0], "reason"); got == "" {
			t.Errorf("reason is empty, want non-empty (body: %s)", rec.Body.String())
		}
	})

	t.Run("MixedBatch", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		acc := createTestAccount(t, e, "test-user")
		csv := readTestdata(t, "statement_ok.csv")
		acct := acc
		receipt1 := pdfBytes(24)
		receipt2 := pdfBytes(48)

		rec := postAdd(t, e, "test-user", []addFile{
			{filename: "r1.pdf", contentType: "application/pdf", content: receipt1},
			{filename: "r2.pdf", contentType: "application/pdf", content: receipt2},
			{filename: "stmt.csv", contentType: "text/csv", content: csv},
		}, nil, &acct)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		out := addOutcomes(t, rec)
		if len(out) != 3 {
			t.Fatalf("outcome count = %d, want 3 (body: %s)", len(out), rec.Body.String())
		}
		if got := out[0]["kind"]; got != "asset_committed" && got != "held_for_review" {
			t.Errorf("outcome 0 kind = %q, want asset_committed/held_for_review (body: %s)", got, rec.Body.String())
		}
		if got := out[1]["kind"]; got != "asset_committed" && got != "held_for_review" {
			t.Errorf("outcome 1 kind = %q, want asset_committed/held_for_review (body: %s)", got, rec.Body.String())
		}
		if got := out[2]["kind"]; got != "statement_preview" {
			t.Errorf("outcome 2 kind = %q, want statement_preview (body: %s)", got, rec.Body.String())
		}
	})

	t.Run("PastedText", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		txt := "ACME CORP invoice 12349 total 39999.99"
		rec := postAdd(t, e, "test-user", nil, &txt, nil)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		out := addOutcomes(t, rec)
		if len(out) != 1 {
			t.Fatalf("outcome count = %d, want 1 (body: %s)", len(out), rec.Body.String())
		}
		if got := out[0]["kind"]; got != "asset_committed" {
			t.Errorf("kind = %q, want asset_committed (body: %s)", got, rec.Body.String())
		}
		if got := strVal(out[0], "asset_id"); got == "" {
			t.Errorf("asset_id is empty, want non-empty (body: %s)", rec.Body.String())
		}

		// Spec: pasted text is stored as a text/plain source. Verify a source
		// row with content type text/plain was persisted for this item.
		srcs, err := e.factory.Sources.List(context.Background(), repo.Owner("test-user"))
		if err != nil {
			t.Fatalf("list sources: %v", err)
		}
		foundText := false
		for _, src := range srcs {
			if src.ContentType == "text/plain" {
				foundText = true
				break
			}
		}
		if !foundText {
			t.Errorf("no text/plain source found, want the pasted text stored as a text/plain source")
		}
	})
}
