package llm

import (
	"context"

	"procrastinator-backend/core/processing"
)

// ClientChatter adapts *Client to processing.Chatter so core/processing can
// issue chat requests without importing infra (ARC-001).
type ClientChatter struct {
	client *Client
}

// Compile-time guard: ClientChatter must satisfy processing.Chatter.
var _ processing.Chatter = (*ClientChatter)(nil)

// NewClientChatter creates a ClientChatter backed by the given LLM client.
func NewClientChatter(client *Client) *ClientChatter {
	return &ClientChatter{client: client}
}

// Chat converts processing content parts to llm wire parts and delegates to
// Client.Chat (one OpenAI /v1/chat/completions request).
func (c *ClientChatter) Chat(ctx context.Context, systemPrompt string, content []processing.ContentPart) (string, error) {
	parts := make([]ContentPart, len(content))
	for i, p := range content {
		parts[i] = ContentPart{
			Type:     p.Kind,
			Text:     p.Text,
			ImageURL: p.ImageURL,
		}
	}

	return c.client.Chat(ctx, systemPrompt, parts)
}
