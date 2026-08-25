package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ContentPart is a single piece of user message content, either text or an
// image URL reference (OpenAI multi-modal format).
type ContentPart struct {
	Type     string // "text" or "image_url"
	Text     string
	ImageURL string
}

// Client is an OpenAI-compatible chat completion client.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// New creates a Client targeting the given OpenAI-compatible base URL. A
// trailing slash on baseURL is trimmed.
func New(baseURL, apiKey, model string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	Temperature    float64        `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type contentPartJSON struct {
	Type     string    `json:"type"`
	Text     *string   `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Chat sends the system prompt and user content parts to the model and
// returns the assistant's response content.
func (c *Client) Chat(ctx context.Context, systemPrompt string, userContent []ContentPart) (string, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildContentParts(userContent)},
		},
		ResponseFormat: responseFormat{Type: "json_object"},
		Temperature:    0,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm: chat request failed: status %d: %s", resp.StatusCode, body)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("llm: decode response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm: no choices in response")
	}

	return parsed.Choices[0].Message.Content, nil
}

func buildContentParts(parts []ContentPart) []contentPartJSON {
	result := make([]contentPartJSON, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			result = append(result, contentPartJSON{
				Type: "text",
				Text: &p.Text,
			})
		case "image_url":
			result = append(result, contentPartJSON{
				Type:     "image_url",
				ImageURL: &imageURL{URL: p.ImageURL},
			})
		}
	}
	return result
}
