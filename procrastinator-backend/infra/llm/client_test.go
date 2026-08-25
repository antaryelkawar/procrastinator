package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Chat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupServer func(t *testing.T) *httptest.Server
		modifyURL   func(url string) string
		timeout     time.Duration
		wantErr     bool
		wantContent string
	}{
		{
			name: "successful chat completion returns content",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", r.Method)
					}
					if r.URL.Path != "/v1/chat/completions" {
						t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
					}
					if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
						t.Errorf("Authorization = %q, want Bearer test-key", auth)
					}
					var req struct {
						Model          string            `json:"model"`
						Messages       []json.RawMessage `json:"messages"`
						ResponseFormat json.RawMessage   `json:"response_format"`
						Temperature    *float64          `json:"temperature"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Errorf("decode request: %v", err)
					}
					if req.Model != "test-model" {
						t.Errorf("model = %q, want test-model", req.Model)
					}
					if len(req.Messages) != 2 {
						t.Errorf("messages len = %d, want 2", len(req.Messages))
					}
					// Verify response_format
					var rf struct {
						Type string `json:"type"`
					}
					if err := json.Unmarshal(req.ResponseFormat, &rf); err != nil {
						t.Errorf("unmarshal response_format: %v", err)
					} else if rf.Type != "json_object" {
						t.Errorf("response_format.type = %q, want json_object", rf.Type)
					}
					// Verify temperature
					if req.Temperature == nil || *req.Temperature != 0 {
						t.Errorf("temperature = %v, want 0", req.Temperature)
					}

					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"classification\":\"invoice\"}"}}]}`)
				}))
			},
			modifyURL:   func(url string) string { return url },
			timeout:     5 * time.Second,
			wantErr:     false,
			wantContent: `{"classification":"invoice"}`,
		},
		{
			name: "HTTP 500 returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, `{"error":"internal server error"}`)
				}))
			},
			modifyURL: func(url string) string { return url },
			timeout:   5 * time.Second,
			wantErr:   true,
		},
		{
			name: "non-JSON response returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/plain")
					fmt.Fprint(w, "this is not json")
				}))
			},
			modifyURL: func(url string) string { return url },
			timeout:   5 * time.Second,
			wantErr:   true,
		},
		{
			name: "empty choices array returns error",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[]}`)
				}))
			},
			modifyURL: func(url string) string { return url },
			timeout:   5 * time.Second,
			wantErr:   true,
		},
		{
			name: "timeout is respected",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second)
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"content":"too late"}}]}`)
				}))
			},
			modifyURL: func(url string) string { return url },
			timeout:   100 * time.Millisecond,
			wantErr:   true,
		},
		{
			name: "base URL without trailing slash works",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}))
			},
			modifyURL:   func(url string) string { return url },
			timeout:     5 * time.Second,
			wantErr:     false,
			wantContent: "ok",
		},
		{
			name: "base URL with trailing slash works",
			setupServer: func(t *testing.T) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}))
			},
			modifyURL:   func(url string) string { return url + "/" },
			timeout:     5 * time.Second,
			wantErr:     false,
			wantContent: "ok",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := tt.setupServer(t)
			defer server.Close()

			baseURL := tt.modifyURL(server.URL)
			client := New(baseURL, "test-key", "test-model", tt.timeout)

			content, err := client.Chat(context.Background(), "system prompt", []ContentPart{
				{Type: "text", Text: "hello"},
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if content != tt.wantContent {
				t.Fatalf("content = %q, want %q", content, tt.wantContent)
			}
		})
	}
}
