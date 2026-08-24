package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"procrastinator-backend/internal/config"
)

// ExtractionError represents an error that occurred during LLM extraction.
type ExtractionError struct {
	Msg string
	Err error
}

// Error returns the formatted error message.
func (e *ExtractionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	return e.Msg
}

// Unwrap returns the underlying error.
func (e *ExtractionError) Unwrap() error {
	return e.Err
}

// Client handles interactions with the LLM API for document extraction.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// New creates a new LLM Client. It trims any trailing slash from
// cfg.LLMBaseURL so that the /chat/completions path is joined correctly.
func New(cfg config.Config) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(cfg.LLMBaseURL, "/"),
		apiKey:  cfg.LLMAPIKey,
		model:   cfg.LLMModel,
		httpClient: &http.Client{
			Timeout: cfg.LLMTimeout,
		},
	}
}

const systemPrompt = `Extract the following fields from the document and return exactly one single JSON object (not an array, not multiple objects, no markdown fences, no text before or after the JSON): 
document_type (one of invoice, warranty, other), brand, model, serial_number, 
purchase_date, price, currency, warranty_start, warranty_end. 
Unknown values must be null. The response must be a single JSON object with these fields as keys.`

// Extract takes a document and extracts structured data using the LLM.
func (c *Client) Extract(ctx context.Context, contentType string, data []byte) ([]byte, error) {
	var docPart map[string]any

	b64Data := base64.StdEncoding.EncodeToString(data)

	switch contentType {
	case "image/png", "image/jpeg", "application/pdf":
		docPart = map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": fmt.Sprintf("data:%s;base64,%s", contentType, b64Data),
			},
		}
	default:
		return nil, &ExtractionError{Msg: "unsupported content type: " + contentType}
	}

	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role": "user",
				"content": []any{
					docPart,
					map[string]any{
						"type": "text",
						"text": "Please extract the fields from this document.",
					},
				},
			},
		},
		"response_format": map[string]any{
			"type": "json_object",
		},
		"temperature": 0,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &ExtractionError{Msg: "failed to marshal request body", Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, &ExtractionError{Msg: "failed to create request", Err: err}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ExtractionError{Msg: "http request failed", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &ExtractionError{
			Msg: fmt.Sprintf("llm api returned non-2xx status: %d, body: %s", resp.StatusCode, string(bodyBytes)),
		}
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, &ExtractionError{Msg: "failed to decode api response", Err: err}
	}

	if len(apiResp.Choices) == 0 {
		return nil, &ExtractionError{Msg: "api response contained no choices"}
	}

	content := apiResp.Choices[0].Message.Content
	if content == "" {
		return nil, &ExtractionError{Msg: "api response message content is empty"}
	}

	return []byte(content), nil
}
