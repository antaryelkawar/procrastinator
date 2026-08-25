package llm

import (
	"context"
	"encoding/base64"
	"fmt"

	data "procrastinator-backend/commons-data"
)

// Extractor adapts the LLM Client to the data.Extractor interface for
// document extraction.
type Extractor struct {
	client *Client
}

// Compile-time guard: Extractor must satisfy data.Extractor.
var _ data.Extractor = (*Extractor)(nil)

// NewExtractor creates an Extractor backed by the given LLM client.
func NewExtractor(client *Client) *Extractor {
	return &Extractor{client: client}
}

// Extract sends the document to the LLM and returns the raw JSON response.
func (e *Extractor) Extract(ctx context.Context, contentType string, docData []byte) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(docData)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, b64)

	parts := []ContentPart{
		{Type: "text", Text: "Extract all fields from this document."},
		{Type: "image_url", ImageURL: dataURI},
	}

	return e.client.Chat(ctx, SystemPrompt(), parts)
}
