package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"procrastinator-backend/core/processing"
)

func TestClientChatter(t *testing.T) {
	t.Parallel()

	const (
		wantContent = `{"brand":"LG"}`
		model       = "m"
	)

	var (
		gotPath  string
		gotModel string
		userPart any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body := struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		gotModel = body.Model
		// The user message content (second message) is the parts array.
		if len(body.Messages) == 2 {
			userPart = body.Messages[1].Content
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": wantContent}},
			},
		})
	}))
	defer server.Close()

	client := New(server.URL, "test-key", model, 5*time.Second)
	chatter := NewClientChatter(client)

	const docData = "fake-image-bytes"
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(docData))

	content := []processing.ContentPart{
		{Kind: "text", Text: "Extract all fields from this document."},
		{Kind: "image_url", ImageURL: dataURI},
	}

	got, err := chatter.Chat(context.Background(), "system prompt", content)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if got != wantContent {
		t.Fatalf("Chat = %q, want %q", got, wantContent)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotModel != model {
		t.Errorf("model = %q, want %q", gotModel, model)
	}

	parts, ok := userPart.([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("user content = %v (type %T), want a 2-part array", userPart, userPart)
	}
	// Part 0 is the text part.
	textPart, ok := parts[0].(map[string]any)
	if !ok || textPart["type"] != "text" {
		t.Errorf("parts[0] = %v, want a text part", parts[0])
	}
	// Part 1 is the image_url part with a data URI matching our input.
	imgPart, ok := parts[1].(map[string]any)
	if !ok || imgPart["type"] != "image_url" {
		t.Errorf("parts[1] = %v, want an image_url part", parts[1])
	}
}
