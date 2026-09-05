package llm

import (
	"strings"
	"testing"
)

func TestSystemPrompt_RequestsConfidence(t *testing.T) {
	prompt := SystemPrompt()
	if !strings.Contains(prompt, "confidence:") {
		t.Fatalf("SystemPrompt() missing \"confidence:\" — prompt does not instruct the LLM to emit a confidence field")
	}
}
