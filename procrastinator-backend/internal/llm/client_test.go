package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"procrastinator-backend/internal/config"
)

func newTestClient(url string, timeout time.Duration) *Client {
	return New(config.Config{
		LLMBaseURL: url,
		LLMAPIKey:  "test-key",
		LLMModel:   "test-model",
		LLMTimeout: timeout,
	})
}

func TestExtract_Success(t *testing.T) {
	t.Parallel()

	const rawResponse = `{"document_type":"invoice","brand":"Acme"}`

	tests := []struct {
		name        string
		contentType string
		data        []byte
	}{
		{
			name:        "image/png",
			contentType: "image/png",
			data:        []byte("fake-png-bytes"),
		},
		{
			name:        "image/jpeg",
			contentType: "image/jpeg",
			data:        []byte("fake-jpeg-bytes"),
		},
		{
			name:        "application/pdf",
			contentType: "application/pdf",
			data:        []byte("%PDF-1.4 fake pdf content"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Assert method and path
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/chat/completions" {
					t.Errorf("expected /chat/completions, got %s", r.URL.Path)
				}

				// Assert Authorization header
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("expected Authorization Bearer test-key, got %s", r.Header.Get("Authorization"))
				}

				// Decode body and assert
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode request body: %v", err)
				}

				if body["model"] != "test-model" {
					t.Errorf("expected model test-model, got %v", body["model"])
				}

				respFormat, ok := body["response_format"].(map[string]any)
				if !ok || respFormat["type"] != "json_object" {
					t.Errorf("expected response_format type json_object, got %v", body["response_format"])
				}

				messages, ok := body["messages"].([]any)
				if !ok || len(messages) < 2 {
					t.Fatalf("expected at least 2 messages, got %v", body["messages"])
				}

				// Assert system message
				sysMsg, ok := messages[0].(map[string]any)
				if !ok || sysMsg["role"] != "system" || sysMsg["content"] == "" {
					t.Errorf("invalid system message: %v", messages[0])
				}

				// Assert user message
				userMsg, ok := messages[1].(map[string]any)
				if !ok || userMsg["role"] != "user" {
					t.Errorf("invalid user message: %v", messages[1])
				}

				// Assert document content in user message
				userContent, ok := userMsg["content"].([]any)
				if !ok {
					t.Fatalf("expected user content to be an array of parts, got %v", userMsg["content"])
				}

				foundContent := false
				base64Data := base64.StdEncoding.EncodeToString(tt.data)

				for _, part := range userContent {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}

					if imageUrl, ok := partMap["image_url"].(map[string]any); ok {
						if url, ok := imageUrl["url"].(string); ok {
							prefix := "data:" + tt.contentType + ";base64,"
							if strings.HasPrefix(url, prefix) && strings.Contains(url, base64Data) {
								foundContent = true
								break
							}
						}
					}
				}

				if !foundContent {
					t.Errorf("document content not found in user message for %s", tt.name)
				}

				// Respond with success
				resp := map[string]any{
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": rawResponse,
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			client := newTestClient(srv.URL, 1*time.Second)
			got, err := client.Extract(context.Background(), tt.contentType, tt.data)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != rawResponse {
				t.Errorf("expected %s, got %s", rawResponse, string(got))
			}
		})
	}
}

func TestExtract_FailureModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setupSrv func(t *testing.T) *httptest.Server
		timeout  time.Duration
	}{
		{
			name: "non-2xx status",
			setupSrv: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":"boom"}`))
				}))
			},
			timeout: 1 * time.Second,
		},
		{
			name: "unparseable body",
			setupSrv: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("this is not json"))
				}))
			},
			timeout: 1 * time.Second,
		},
		{
			name: "empty choices",
			setupSrv: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"choices":[]}`))
				}))
			},
			timeout: 1 * time.Second,
		},
		{
			name: "timeout",
			setupSrv: func(t *testing.T) *httptest.Server {
				t.Helper()
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-time.After(2 * time.Second):
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
					case <-r.Context().Done():
					}
				}))
			},
			timeout: 200 * time.Millisecond,
		},
		{
			name: "unreachable server",
			setupSrv: func(t *testing.T) *httptest.Server {
				t.Helper()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				srv.Close()
				return srv
			},
			timeout: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := tt.setupSrv(t)
			defer srv.Close()

			client := newTestClient(srv.URL, tt.timeout)

			if tt.name == "timeout" {
				start := time.Now()
				got, err := client.Extract(context.Background(), "image/png", []byte("data"))
				elapsed := time.Since(start)

				if err == nil {
					t.Fatal("expected error due to timeout, got nil")
				}
				var e *ExtractionError
				if !errors.As(err, &e) {
					t.Errorf("expected *ExtractionError, got %T: %v", err, err)
				}
				if len(got) != 0 {
					t.Errorf("expected empty bytes, got %v", got)
				}
				if elapsed >= 1500*time.Millisecond {
					t.Errorf("expected timeout to happen quickly, but took %v", elapsed)
				}
				return
			}

			got, err := client.Extract(context.Background(), "image/png", []byte("data"))

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var e *ExtractionError
			if !errors.As(err, &e) {
				t.Errorf("expected *ExtractionError, got %T: %v", err, err)
			}
			if len(got) != 0 {
				t.Errorf("expected empty bytes, got %v", got)
			}
		})
	}
}

func TestExtract_TrailingSlashBaseURL(t *testing.T) {
	t.Parallel()

	const rawResponse = `{"document_type":"invoice"}`

	var (
		recordedPath   string
		recordedMethod string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedPath = r.URL.Path
		recordedMethod = r.Method

		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"content": rawResponse,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Simulate the real Gemini base URL which ends with a trailing slash
	// (e.g. https://generativelanguage.googleapis.com/v1beta/openai/).
	client := newTestClient(srv.URL+"/", 2*time.Second)

	got, err := client.Extract(context.Background(), "image/png", []byte("fake-png"))
	if err != nil {
		t.Fatalf("unexpected error: %v (recorded path=%q method=%s)", err, recordedPath, recordedMethod)
	}
	if string(got) != rawResponse {
		t.Errorf("expected raw response %q, got %q", rawResponse, string(got))
	}
	if recordedPath != "/chat/completions" {
		t.Errorf("expected recorded path %q, got %q", "/chat/completions", recordedPath)
	}
	if recordedMethod != http.MethodPost {
		t.Errorf("expected recorded method %s, got %s", http.MethodPost, recordedMethod)
	}
}

func TestSystemPrompt_Contract(t *testing.T) {
	t.Parallel()

	var capturedSystemContent string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		messages, ok := body["messages"].([]any)
		if !ok || len(messages) == 0 {
			t.Fatal("no messages in request")
		}

		sysMsg, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatal("system message is not a map")
		}

		if content, ok := sysMsg["content"].(string); ok {
			capturedSystemContent = content
		} else {
			t.Fatal("system message content is not a string")
		}

		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"content": `{"document_type":"invoice"}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL, 1*time.Second)
	_, err := client.Extract(context.Background(), "image/png", []byte("fake-png"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the system prompt contains explicit instructions to prevent array output
	tests := []struct {
		name     string
		contains string
	}{
		{
			name:     "requires single object",
			contains: "single JSON object",
		},
		{
			name:     "explicitly forbids array",
			contains: "not an array",
		},
		{
			name:     "forbids markdown fences",
			contains: "no markdown fences",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(capturedSystemContent, tt.contains) {
				t.Errorf("system prompt does not contain %q\nactual prompt: %s", tt.contains, capturedSystemContent)
			}
		})
	}
}
