package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractor_Extract(t *testing.T) {
	t.Parallel()

	docData := []byte("fake-invoice-bytes")
	wantB64 := base64.StdEncoding.EncodeToString(docData)
	wantDataURI := "data:image/jpeg;base64," + wantB64
	wantResponse := `{"classification":"invoice","price":"3999.99"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("messages len = %d, want 2", len(req.Messages))
		}

		// System message must carry the extraction system prompt.
		if req.Messages[0].Role != "system" {
			t.Errorf("messages[0].role = %q, want system", req.Messages[0].Role)
		}
		sysPrompt, ok := req.Messages[0].Content.(string)
		if !ok {
			t.Fatalf("messages[0].content not a string: %T", req.Messages[0].Content)
		}
		if sysPrompt != SystemPrompt() {
			t.Errorf("system prompt mismatch:\ngot:  %q\nwant: %q", sysPrompt, SystemPrompt())
		}

		// User message must be a content-parts array with text + image_url.
		if req.Messages[1].Role != "user" {
			t.Errorf("messages[1].role = %q, want user", req.Messages[1].Role)
		}
		raw, err := json.Marshal(req.Messages[1].Content)
		if err != nil {
			t.Fatalf("marshal user content: %v", err)
		}
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if err := json.Unmarshal(raw, &parts); err != nil {
			t.Fatalf("unmarshal user content: %v", err)
		}
		if len(parts) != 2 {
			t.Fatalf("user parts len = %d, want 2", len(parts))
		}
		if parts[0].Type != "text" || parts[0].Text != "Extract all fields from this document." {
			t.Errorf("parts[0] = %+v, want text part with extraction instruction", parts[0])
		}
		if parts[1].Type != "image_url" || parts[1].ImageURL.URL != wantDataURI {
			t.Errorf("parts[1] = %+v, want image_url part with %q", parts[1], wantDataURI)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, wantResponse)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", "test-model", 5*time.Second)
	extractor := NewExtractor(client)

	got, err := extractor.Extract(context.Background(), "image/jpeg", docData)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if got != wantResponse {
		t.Fatalf("Extract = %q, want %q", got, wantResponse)
	}
}

func TestExtractor_Extract_PropagatesClientError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", "test-model", 5*time.Second)
	extractor := NewExtractor(client)

	_, err := extractor.Extract(context.Background(), "application/pdf", []byte("pdf-bytes"))
	if err == nil {
		t.Fatal("expected error from HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err)
	}
}
